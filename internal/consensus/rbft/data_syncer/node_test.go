package data_syncer

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axiomesh/axiom-kit/txpool/mock_txpool"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	rbfttypes "github.com/axiomesh/axiom-bft/types"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/adaptor"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/testutil"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	sync_comm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/internal/sync/common/mock_sync"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	p2p "github.com/axiomesh/axiom-p2p"
)

func TestNewNode(t *testing.T) {
	repoConfig := &repo.Config{Storage: repo.Storage{
		KvType:      repo.KVStorageTypeLeveldb,
		Sync:        false,
		KVCacheSize: repo.KVStorageCacheSize,
		Pebble:      repo.Pebble{},
	}, Monitor: repo.Monitor{Enable: false}}
	err := storagemgr.Initialize(repoConfig)
	assert.Nil(t, err)
	ctrl := gomock.NewController(t)
	logger := log.NewWithModule("data_syncer")
	logger.Logger.SetLevel(logrus.DebugLevel)
	consensusConf, pool := testutil.MockConsensusConfig(logger, ctrl, t)
	rbftConfig := rbft.Config{
		SelfP2PNodeID:           "node5",
		SyncStateTimeout:        1 * time.Minute,
		SyncStateRestartTimeout: 1 * time.Second,
		CheckPoolTimeout:        1 * time.Minute,
	}

	rbftAdaptor, err := adaptor.NewRBFTAdaptor(consensusConf)
	assert.Nil(t, err)

	_, err = NewNode[types.Transaction, *types.Transaction](rbftConfig, rbftAdaptor, nil, pool, logger)
	assert.Nil(t, err)
}

func TestNode_Start(t *testing.T) {
	testcases := []struct {
		name          string
		remoteHandler func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error
		setupMocks    func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller)
		expectResult  string
		expectErr     bool
	}{
		{
			name: "start with init sync state error",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				return nil
			},
			expectErr:    true,
			expectResult: "failed to init sync state",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				cnf.Repo.RepoRoot = filepath.Join(t.TempDir(), "repo")
				newStack, err := adaptor.NewRBFTAdaptor(cnf)
				assert.Nil(t, err)

				newStack.SetMsgPipes(map[int32]p2p.Pipe{})

				err = newStack.UpdateEpoch()
				assert.Nil(t, err)
				node.stack = newStack
			},
		},
		{
			name: "start with same state, but verify signature failed",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				fmt.Printf("receive sync state from %v: %v\n", msg.From, msg.Type)
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}

				cp := generateSignedCheckpoint(t, id, 1, hex.EncodeToString([]byte("block1")), "batchDigest1")
				cp.Signature = []byte("wrong signature")
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: cp,
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consensusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
				}
				data, err := consensusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}
				fmt.Printf("[%v-%v] send sync state response to %s", id, pipe.String(), to)
				return nil
			},
			expectResult: "invalid signature",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
			},
		},
		{
			name: "start with same state, then sync state timeout, retry to init sync state successfully",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				time.Sleep(100 * time.Millisecond)
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consnesusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
				}
				data, err := consnesusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}
				return nil
			},
			expectResult: "has reached state",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				err := node.timeMgr.CreateTimer(syncStateRestart, 10*time.Millisecond, node.handleTimeout)
				assert.Nil(t, err)
			},
		},
		{
			name: "start with same state successfully",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consnesusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
				}
				data, err := consnesusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}
				return nil
			},
			expectResult: "has reached state",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				return
			},
		},
		{
			name: "start with a backward state, start sync block without epoch changed",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}

				block2 := testutil.ConstructBlock("block2", uint64(2))
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 2, block2.Hash().String(), "batchDigest2"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consensusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
				}
				data, err := consensusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}
				return nil
			},
			expectResult: "finished move watermark to height:2",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSyncInStack(t, node, consensusMsgPipes, cnf, ctrl)
			},
		},
		{
			name: "start with a backward state, start sync block with epoch changed",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				if msg.Type == consensus.Type_SYNC_STATE {
					block110 := testutil.ConstructBlock("block110", uint64(110))
					resp := &consensus.SyncStateResponse{
						ReplicaId:        id,
						View:             110,
						SignedCheckpoint: generateSignedCheckpoint(t, id, 110, block110.Hash().String(), "batchDigest110"),
					}
					payload, err := resp.MarshalVT()
					assert.Nil(t, err)

					consensusMsg := &consensus.ConsensusMessage{
						Type:    consensus.Type_SYNC_STATE_RESPONSE,
						Payload: payload,
						Epoch:   2,
						From:    id,
					}
					data, err := consensusMsg.MarshalVT()
					assert.Nil(t, err)

					err = pipe.Send(context.Background(), to, data)
					assert.Nil(t, err)

					return nil
				}

				if msg.Type == consensus.Type_EPOCH_CHANGE_REQUEST {
					block100 := testutil.ConstructBlock("block100", uint64(100))
					changes := make([]*consensus.EpochChange, 0)
					changes = append(changes, &consensus.EpochChange{
						Checkpoint: genQuorumSignCheckpoint(t, 100, block100.Hash().String(), "batchDigest100"),
						Validators: &consensus.QuorumValidators{Validators: make([]*consensus.QuorumValidator, 0)},
					})
					proof := &consensus.EpochChangeProof{
						EpochChanges:       changes,
						Author:             id,
						GenesisBlockDigest: "genesisDigest",
					}

					payload, err := proof.MarshalVT()
					assert.Nil(t, err)
					consensusMsg := &consensus.ConsensusMessage{
						Type:    consensus.Type_EPOCH_CHANGE_PROOF,
						Payload: payload,
						Epoch:   2,
						From:    id,
					}
					data, err := consensusMsg.MarshalVT()
					assert.Nil(t, err)

					err = pipe.Send(context.Background(), to, data)
					assert.Nil(t, err)

					return nil
				}

				return fmt.Errorf("unexpected message type: %d", msg.Type)
			},
			expectResult: "finished move watermark to height:110",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSync := mock_sync.NewMockSync(ctrl)
				blockChan := make(chan any, 1)

				mockSync.EXPECT().StartSync(gomock.Any(), gomock.Any()).DoAndReturn(func(params *sync_comm.SyncParams, syncTaskDoneCh chan error) error {
					block := testutil.ConstructBlock(fmt.Sprintf("block%d", params.TargetHeight), params.TargetHeight)
					d := &sync_comm.BlockData{Block: block}
					blocks := make([]sync_comm.CommitData, 0)
					blocks = append(blocks, d)
					blockChan <- blocks
					syncTaskDoneCh <- nil
					return nil
				}).AnyTimes()

				mockSync.EXPECT().Commit().Return(blockChan).AnyTimes()

				cnf.BlockSync = mockSync

				cnf.Repo.RepoRoot = filepath.Join(t.TempDir(), "repo")
				newStack, err := adaptor.NewRBFTAdaptor(cnf)
				assert.Nil(t, err)

				newStack.SetMsgPipes(consensusMsgPipes)

				err = newStack.UpdateEpoch()
				assert.Nil(t, err)
				node.stack = newStack

				go func() {
					for {
						select {
						case block := <-newStack.BlockC:
							if block.Block.Height() == 100 {
								block100 := testutil.ConstructBlock("block100", uint64(100))
								epcState := &rbfttypes.ServiceSyncState{}
								epcState.ServiceState.MetaState = &rbfttypes.MetaState{
									Height: block100.Height(),
									Digest: block100.Hash().String(),
								}
								epcState.EpochChanged = true
								epcState.Epoch = 2

								currentEpochInfo := cnf.GenesisEpochInfo.Clone()
								currentEpochInfo.Epoch = 2
								cnf.ChainState.EpochInfo = currentEpochInfo
								node.ReportStateUpdated(epcState)
							}
							if block.Block.Height() == 110 {
								time.Sleep(1 * time.Millisecond)
								state := &rbfttypes.ServiceSyncState{}
								state.ServiceState.MetaState = &rbfttypes.MetaState{
									Height: block.Block.Height(),
									Digest: block.Block.Hash().String(),
								}
								state.EpochChanged = false
								state.Epoch = 2

								node.ReportStateUpdated(state)
							}
						}
					}
				}()
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			stack, cnf, pipes := mockConfig(t, ctrl)
			node := mockDataSyncerNode(stack, cnf, t)
			tc.setupMocks(node, stack, cnf, ctrl)

			// Start receiving messages on pipes
			for id, pipesM := range pipes {
				id := id
				pipesM := pipesM
				if id == node.chainState.SelfNodeInfo.ID {
					for _, pipe := range pipesM {
						pipe := pipe
						go func(pipe p2p.Pipe) {
							for {
								msg := pipe.Receive(context.Background())
								if msg == nil {
									return
								}
								consensusMsg := &consensus.ConsensusMessage{}
								err := consensusMsg.UnmarshalVT(msg.Data)
								assert.Nil(t, err)
								node.Step(context.Background(), consensusMsg)
							}
						}(pipe)
					}
				} else {
					go receiveMsg(t, pipesM, id, tc.remoteHandler)
				}
			}

			poolBatch, txs := generateBatch(t)
			lo.ForEach(txs, func(tx *types.Transaction, _ int) {
				err := cnf.TxPool.AddLocalTx(tx)
				assert.Nil(t, err)
			})
			remoteProposal(t, 1, repo.MockP2PIDs, pipes, poolBatch, 2, 2)
			// wait date sync node to receive messages
			time.Sleep(10 * time.Millisecond)

			for id := range pipes {
				if id != node.chainState.SelfNodeInfo.ID {
					remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
				}
			}

			// Setup log output capturing
			taskDoneCh := make(chan bool, 1)
			catchExpectLogOut(t, node.logger, tc.expectResult, taskDoneCh)
			// Start node
			err := node.Start()
			assert.Equal(t, err != nil, tc.expectErr)

			assert.True(t, <-taskDoneCh)
			node.Stop()
		})
	}
}

func TestNode_Step(t *testing.T) {
	testCase := []struct {
		name         string
		input        *consensus.ConsensusMessage
		setupMocks   func(node *Node[types.Transaction, *types.Transaction])
		expectResult any
	}{
		{
			name:  "receive msg but in epoch syncing",
			input: &consensus.ConsensusMessage{},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {
				node.statusMgr.On(InEpochSyncing)
			},
			expectResult: "is in epoch syncing status, reject consensus messages",
		},
		{
			name:         "receive wrong msg type",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Type: consensus.Type(10000), Payload: []byte("wrong_type"), Epoch: 1},
			expectResult: "unknown consensus message type",
		},
		{
			name:         "receive remote msg epoch bigger than current epoch, fetch epoch proof failed",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Type: consensus.Type_SYNC_STATE_RESPONSE, Epoch: 2, From: 2000},
			expectResult: "fetch epoch change proof failed",
		},
		{
			name:         "receive remote msg epoch bigger than current epoch, handle epoch proof failed",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Type: consensus.Type_EPOCH_CHANGE_PROOF, Payload: []byte("wrong_type"), Epoch: 2, From: 1},
			expectResult: "Unmarshal EpochChangeProof failed",
		},
		{
			name:         "receive remote msg epoch smaller than current epoch, ignore it",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Type: consensus.Type_EPOCH_CHANGE_PROOF, Epoch: 0, From: 1},
			expectResult: "received message with lower epoch, just ignore it..",
		},
		{
			name:         "receive pre-prepare msg, unmarshal error",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Epoch: 1, From: 1, Type: consensus.Type_PRE_PREPARE, Payload: []byte("wrong_type")},
			expectResult: "failed to handle consensus message",
		},

		{
			name:         "receive signed checkpoint msg, unmarshal error",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Epoch: 1, From: 1, Type: consensus.Type_SIGNED_CHECKPOINT, Payload: []byte("wrong_type")},
			expectResult: "failed to handle consensus message",
		},

		{
			name:         "receive sync response, unmarshal error",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Epoch: 1, From: 1, Type: consensus.Type_SYNC_STATE_RESPONSE, Payload: []byte("wrong_type")},
			expectResult: "failed to handle consensus message",
		},
		{
			name:         "receive fetch missing response, unmarshal error",
			setupMocks:   func(node *Node[types.Transaction, *types.Transaction]) {},
			input:        &consensus.ConsensusMessage{Epoch: 1, From: 1, Type: consensus.Type_FETCH_MISSING_RESPONSE, Payload: []byte("wrong_type")},
			expectResult: "failed to handle consensus message",
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			stack, cnf, _ := mockConfig(t, ctrl)
			node := mockDataSyncerNode(stack, cnf, t)
			tc.setupMocks(node)
			taskDoneCh := make(chan bool, 1)
			catchExpectLogOut(t, node.logger, tc.expectResult.(string), taskDoneCh)
			go node.listenEvent()
			node.Step(context.Background(), tc.input)
			assert.True(t, <-taskDoneCh)
		})
	}
}

func TestNode_ArchiveMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	consensusMsgPipes, cnf, _ := mockConfig(t, ctrl)
	node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
	assert.True(t, node.ArchiveMode())
}

func TestNode_Stop(t *testing.T) {
	ctrl := gomock.NewController(t)
	stack, cnf, _ := mockConfig(t, ctrl)
	node := mockDataSyncerNode(stack, cnf, t)
	err := node.Start()
	assert.Nil(t, err)

	node.Stop()
}

func TestNode_Status(t *testing.T) {
	testCase := []struct {
		name         string
		setupMocks   func(node *Node[types.Transaction, *types.Transaction], pipes map[uint64]map[string]p2p.Pipe, consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller)
		expectResult rbft.StatusType
	}{
		{
			name:         "before start",
			expectResult: rbft.Pending,
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], pipes map[uint64]map[string]p2p.Pipe, consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
			},
		},
		{
			name:         "after start in sync State",
			expectResult: rbft.InSyncState,
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], pipes map[uint64]map[string]p2p.Pipe, consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				err := node.Start()
				assert.Nil(t, err)
			},
		},
		{
			name:         "after start in sync transferring",
			expectResult: rbft.StateTransferring,
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], pipes map[uint64]map[string]p2p.Pipe, consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSync := mock_sync.NewMockSync(ctrl)
				mockSync.EXPECT().StartSync(gomock.Any(), gomock.Any()).DoAndReturn(func(params *sync_comm.SyncParams, syncTaskDoneCh chan error) error {
					syncTaskDoneCh <- nil
					return nil
				}).AnyTimes()

				block2 := testutil.ConstructBlock("block2", uint64(2))
				blockChan := make(chan any, 1)
				d := &sync_comm.BlockData{
					Block: block2,
				}
				blocks := make([]sync_comm.CommitData, 0)
				blocks = append(blocks, d)
				blockChan <- blocks

				mockSync.EXPECT().Commit().Return(blockChan).AnyTimes()
				cnf.BlockSync = mockSync

				cnf.Repo.RepoRoot = filepath.Join(t.TempDir(), "repo")
				newStack, err := adaptor.NewRBFTAdaptor(cnf)
				assert.Nil(t, err)

				newStack.SetMsgPipes(consensusMsgPipes)

				err = newStack.UpdateEpoch()
				assert.Nil(t, err)
				node.stack = newStack

				waitSyncRespCh := make(chan bool, 1)

				catchExpectLogOut(t, node.logger, " try state update to 2", waitSyncRespCh)

				remoteHandler := func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
					if msg.Type == consensus.Type_SYNC_STATE {
						resp := &consensus.SyncStateResponse{
							ReplicaId:        id,
							View:             2,
							SignedCheckpoint: generateSignedCheckpoint(t, id, 2, block2.Hash().String(), "batchDigest2"),
						}
						payload, err := resp.MarshalVT()
						assert.Nil(t, err)

						consensusMsg := &consensus.ConsensusMessage{
							Type:    consensus.Type_SYNC_STATE_RESPONSE,
							Payload: payload,
							Epoch:   1,
							From:    id,
						}
						data, err := consensusMsg.MarshalVT()
						assert.Nil(t, err)

						err = pipe.Send(context.Background(), to, data)
						assert.Nil(t, err)

						return nil
					}
					return nil
				}

				// Start receiving messages on pipes
				for id, pipesM := range pipes {
					if id == node.chainState.SelfNodeInfo.ID {
						for _, pipe := range pipesM {
							go func(pipe p2p.Pipe) {
								for {
									msg := pipe.Receive(context.Background())
									if msg == nil {
										return
									}
									consensusMsg := &consensus.ConsensusMessage{}
									err := consensusMsg.UnmarshalVT(msg.Data)
									assert.Nil(t, err)
									node.Step(context.Background(), consensusMsg)
								}
							}(pipe)
						}
					} else {
						go receiveMsg(t, pipesM, id, remoteHandler)
					}
				}
				err = node.Start()
				assert.Nil(t, err)
				<-waitSyncRespCh
			},
		},

		{
			name:         "after start in same state",
			expectResult: rbft.Normal,
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], pipes map[uint64]map[string]p2p.Pipe, consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				waitSyncRespCh := make(chan bool, 1)
				catchExpectLogOut(t, node.logger, "has reached state", waitSyncRespCh)
				meta := node.chainState.ChainMeta

				remoteHandler := func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
					if msg.Type == consensus.Type_SYNC_STATE {
						resp := &consensus.SyncStateResponse{
							ReplicaId:        id,
							View:             1,
							SignedCheckpoint: generateSignedCheckpoint(t, id, meta.Height, meta.BlockHash.String(), "batchDigest1"),
						}
						payload, err := resp.MarshalVT()
						assert.Nil(t, err)

						consensusMsg := &consensus.ConsensusMessage{
							Type:    consensus.Type_SYNC_STATE_RESPONSE,
							Payload: payload,
							Epoch:   1,
							From:    id,
						}
						data, err := consensusMsg.MarshalVT()
						assert.Nil(t, err)

						err = pipe.Send(context.Background(), to, data)
						assert.Nil(t, err)

						return nil
					}
					return nil
				}

				// Start receiving messages on pipes
				for id, pipesM := range pipes {
					if id == node.chainState.SelfNodeInfo.ID {
						for _, pipe := range pipesM {
							go func(pipe p2p.Pipe) {
								for {
									msg := pipe.Receive(context.Background())
									if msg == nil {
										return
									}
									consensusMsg := &consensus.ConsensusMessage{}
									err := consensusMsg.UnmarshalVT(msg.Data)
									assert.Nil(t, err)
									node.Step(context.Background(), consensusMsg)
								}
							}(pipe)
						}
					} else {
						go receiveMsg(t, pipesM, id, remoteHandler)
					}
				}
				err := node.Start()
				assert.Nil(t, err)
				<-waitSyncRespCh
			},
		},

		{
			name:         "in epoch sync State",
			expectResult: rbft.InEpochSyncing,
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], pipes map[uint64]map[string]p2p.Pipe, consensusMsgPipes map[int32]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSync := mock_sync.NewMockSync(ctrl)
				mockSync.EXPECT().StartSync(gomock.Any(), gomock.Any()).DoAndReturn(func(params *sync_comm.SyncParams, syncTaskDoneCh chan error) error {
					syncTaskDoneCh <- nil
					return nil
				}).AnyTimes()

				block100 := testutil.ConstructBlock("block100", uint64(100))
				blockChan := make(chan any, 1)
				d := &sync_comm.BlockData{
					Block: block100,
				}
				blocks := make([]sync_comm.CommitData, 0)
				blocks = append(blocks, d)
				blockChan <- blocks

				mockSync.EXPECT().Commit().Return(blockChan).AnyTimes()
				cnf.BlockSync = mockSync

				cnf.Repo.RepoRoot = filepath.Join(t.TempDir(), "repo")
				newStack, err := adaptor.NewRBFTAdaptor(cnf)
				assert.Nil(t, err)

				newStack.SetMsgPipes(consensusMsgPipes)

				err = newStack.UpdateEpoch()
				assert.Nil(t, err)
				node.stack = newStack

				err = node.Start()
				assert.Nil(t, err)

				changes := make([]*consensus.EpochChange, 0)
				changes = append(changes, &consensus.EpochChange{
					Checkpoint: genQuorumSignCheckpoint(t, 100, block100.Hash().String(), "batchDigest100"),
					Validators: &consensus.QuorumValidators{Validators: make([]*consensus.QuorumValidator, 0)},
				})
				proof := &consensus.EpochChangeProof{
					EpochChanges:       changes,
					Author:             1,
					GenesisBlockDigest: "genesisDigest",
				}

				payload, err := proof.MarshalVT()
				assert.Nil(t, err)
				node.Step(context.Background(), &consensus.ConsensusMessage{Type: consensus.Type_EPOCH_CHANGE_PROOF, Payload: payload, Epoch: 2, From: 1})
				waitEpochProofCh := make(chan bool, 1)
				catchExpectLogOut(t, node.logger, "try epoch sync", waitEpochProofCh)
				<-waitEpochProofCh
			},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			consensusMsgPipes, cnf, pipes := mockConfig(t, ctrl)
			node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
			tc.setupMocks(node, pipes, consensusMsgPipes, cnf, ctrl)
			status := node.Status()
			assert.Equal(t, tc.expectResult, status.Status)
			node.Stop()
		})
	}
}

func TestNode_GetLowWatermark(t *testing.T) {
	ctrl := gomock.NewController(t)
	consensusMsgPipes, cnf, _ := mockConfig(t, ctrl)
	node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
	assert.Equal(t, uint64(0), node.GetLowWatermark())

	taskDoneCh := make(chan bool, 1)
	catchExpectLogOut(t, node.logger, "finished move watermark to height", taskDoneCh)

	err := node.Start()
	assert.Nil(t, err)

	block := testutil.ConstructBlock("block", uint64(12))
	node.checkpointCache.insert(generateSignedCheckpoint(t, node.chainState.SelfNodeInfo.ID, block.Height(), block.Hash().String(), "batchDigest"), false)
	node.highStateTarget = &stateUpdateTarget{
		metaState: &rbfttypes.MetaState{
			Height: block.Height(),
			Digest: block.Hash().String(),
		},
	}
	node.statusMgr.On(StateTransferring)
	state := &rbfttypes.ServiceSyncState{}
	state.ServiceState.MetaState = &rbfttypes.MetaState{
		Height: block.Height(),
		Digest: block.Hash().String(),
	}
	state.EpochChanged = false
	state.Epoch = 1

	node.ReportStateUpdated(state)

	assert.True(t, <-taskDoneCh)
	assert.Equal(t, uint64(12), node.GetLowWatermark())
}

func TestNode_HandleCheckpoint(t *testing.T) {
	missingTxsCache := &sync.Map{}
	recvCount := 0
	testCases := []struct {
		name          string
		remoteHandler func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error
		setupMocks    func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller)
		expectResult  string
		expectErr     bool
	}{
		{
			name:         "commit and executed normally",
			expectResult: "execute block 2 succeed",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consensusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
					From:    id,
				}
				data, err := consensusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}
				return nil
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				proposer := uint64(1)
				poolBatch, txs := generateBatch(t)
				lo.ForEach(txs, func(tx *types.Transaction, _ int) {
					err := cnf.TxPool.AddLocalTx(tx)
					assert.Nil(t, err)
				})
				remoteProposal(t, proposer, repo.MockP2PIDs, pipes, poolBatch, 2, 2)

				// wait date sync node to receive messages
				time.Sleep(10 * time.Millisecond)

				for id := range pipes {
					if id != node.chainState.SelfNodeInfo.ID {
						remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
					}
				}

				go func() {
					for {
						select {
						case r := <-node.stack.(*adaptor.RBFTAdaptor).ReadyC:
							block := testutil.ConstructBlock(fmt.Sprintf("block%d", r.Height), r.Height)
							state := &rbfttypes.ServiceState{
								MetaState: &rbfttypes.MetaState{
									Height: r.Height,
									Digest: block.Hash().String(),
								},
							}
							node.ReportExecuted(state)
						}
					}
				}()
			},
		},
		{
			name:         "receive quorum checkpoint, but status is in sync state, ignore it",
			expectResult: "current status is InSyncState, ignore commit event",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consensusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
					From:    id,
				}
				data, err := consensusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}

				return nil
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSyncInStack(t, node, consensusMsgPipes, cnf, ctrl)
				proposer := uint64(1)
				poolBatch, txs := generateBatch(t)
				lo.ForEach(txs, func(tx *types.Transaction, _ int) {
					err := cnf.TxPool.AddLocalTx(tx)
					assert.Nil(t, err)
				})
				remoteProposal(t, proposer, repo.MockP2PIDs, pipes, poolBatch, 2, 2)

				// wait date sync node to receive messages
				time.Sleep(10 * time.Millisecond)

				node.statusMgr.On(InSyncState)
				for id := range pipes {
					if id != node.chainState.SelfNodeInfo.ID {
						remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
					}
				}
			},
		},
		{
			name:         "receive quorum checkpoint, but have not preprepare message",
			expectResult: "finished stateUpdate, height: 2",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				if msg.Type != consensus.Type_SYNC_STATE {
					return fmt.Errorf("invalid msg type: %v", msg.Type)
				}
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}

				consensusMsg := &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
					From:    id,
				}
				data, err := consensusMsg.MarshalVT()
				if err != nil {
					return err
				}
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}

				return nil
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSyncInStack(t, node, consensusMsgPipes, cnf, ctrl)
				poolBatch, txs := generateBatch(t)
				lo.ForEach(txs, func(tx *types.Transaction, _ int) {
					err := cnf.TxPool.AddLocalTx(tx)
					assert.Nil(t, err)
				})

				for id := range pipes {
					if id != node.chainState.SelfNodeInfo.ID {
						remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
					}
				}
			},
		},
		{
			name:         "receive quorum checkpoint, but missing txs in txpool",
			expectResult: "execute block 2 succeed",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				var respMsg *consensus.ConsensusMessage

				switch msg.Type {
				case consensus.Type_SYNC_STATE:
					resp := &consensus.SyncStateResponse{
						ReplicaId:        id,
						View:             1,
						SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
					}
					payload, err := resp.MarshalVT()
					if err != nil {
						return err
					}

					respMsg = &consensus.ConsensusMessage{
						Type:    consensus.Type_SYNC_STATE_RESPONSE,
						Payload: payload,
						Epoch:   1,
						From:    id,
					}
				case consensus.Type_FETCH_MISSING_REQUEST:
					req := &consensus.FetchMissingRequest{}
					err := req.UnmarshalVT(msg.Payload)
					assert.Nil(t, err)
					v, ok := missingTxsCache.Load(req.BatchDigest)
					assert.True(t, ok)
					missingTxsM, ok := v.(map[uint64]*types.Transaction)
					assert.True(t, ok)
					missingTxHashM := lo.MapValues(missingTxsM, func(tx *types.Transaction, index uint64) string {
						return tx.RbftGetTxHash()
					})
					missingTxs := lo.MapValues(missingTxsM, func(tx *types.Transaction, index uint64) []byte {
						data, err := tx.RbftMarshal()
						assert.Nil(t, err)
						return data
					})
					resp := &consensus.FetchMissingResponse{
						ReplicaId:            id,
						View:                 2,
						SequenceNumber:       2,
						BatchDigest:          req.BatchDigest,
						MissingRequestHashes: missingTxHashM,
						MissingRequests:      missingTxs,
					}
					payload, err := resp.MarshalVT()
					assert.Nil(t, err)

					respMsg = &consensus.ConsensusMessage{
						Type:    consensus.Type_FETCH_MISSING_RESPONSE,
						Payload: payload,
						Epoch:   1,
						From:    id,
					}
				default:
					return nil
				}

				data, err := respMsg.MarshalVT()
				assert.Nil(t, err)
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}

				return nil
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				proposer := uint64(1)
				poolBatch, txs := generateBatch(t)
				missingTxM := make(map[uint64]*types.Transaction)
				// lack of tx0 in txpool
				missingTxM[0] = txs[0]
				missingTxsCache.Store(poolBatch.BatchHash, missingTxM)
				txs = txs[1:]
				lo.ForEach(txs, func(tx *types.Transaction, _ int) {
					err := cnf.TxPool.AddLocalTx(tx)
					assert.Nil(t, err)
				})
				remoteProposal(t, proposer, repo.MockP2PIDs, pipes, poolBatch, 2, 2)

				// wait date sync node to receive messages
				time.Sleep(10 * time.Millisecond)

				for id := range pipes {
					if id != node.chainState.SelfNodeInfo.ID {
						remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
					}
				}

				go func() {
					for {
						select {
						case r := <-node.stack.(*adaptor.RBFTAdaptor).ReadyC:
							block := testutil.ConstructBlock(fmt.Sprintf("block%d", r.Height), r.Height)
							state := &rbfttypes.ServiceState{
								MetaState: &rbfttypes.MetaState{
									Height: r.Height,
									Digest: block.Hash().String(),
								},
							}
							node.ReportExecuted(state)
						}
					}
				}()
			},
		},
		{
			name:         "wait for fetch missing request timeout,start sync state",
			expectResult: "receive state updated event, finished move watermark to height:2",
			remoteHandler: func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
				block2 := testutil.ConstructBlock("block2", 2)
				var respMsg *consensus.ConsensusMessage
				switch msg.Type {
				case consensus.Type_SYNC_STATE:
					var resp *consensus.SyncStateResponse
					if recvCount < maxRetryCount {
						resp = &consensus.SyncStateResponse{
							ReplicaId:        id,
							View:             1,
							SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
						}
					} else {
						resp = &consensus.SyncStateResponse{
							ReplicaId:        id,
							View:             2,
							SignedCheckpoint: generateSignedCheckpoint(t, id, 2, block2.Hash().String(), "batchDigest2"),
						}
					}

					payload, err := resp.MarshalVT()
					if err != nil {
						return err
					}
					respMsg = &consensus.ConsensusMessage{
						Type:    consensus.Type_SYNC_STATE_RESPONSE,
						Payload: payload,
						Epoch:   1,
						From:    id,
					}
				case consensus.Type_FETCH_MISSING_REQUEST:
					recvCount++
					return nil
				default:
					return nil
				}
				data, err := respMsg.MarshalVT()
				assert.Nil(t, err)
				err = pipe.Send(context.Background(), to, data)
				if err != nil {
					return err
				}

				return nil
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				mockSyncInStack(t, node, consensusMsgPipes, cnf, ctrl)
				err := node.timeMgr.RemoveTimer(fetchMissingTxsResp)
				assert.Nil(t, err)
				err = node.timeMgr.RemoveTimer(syncStateRestart)
				assert.Nil(t, err)

				// make sure not trigger sync state
				err = node.timeMgr.CreateTimer(syncStateRestart, 10000*time.Second, node.handleTimeout)
				assert.Nil(t, err)
				err = node.timeMgr.CreateTimer(fetchMissingTxsResp, 100*time.Millisecond, node.handleTimeout)
				assert.Nil(t, err)

				proposer := uint64(1)
				poolBatch, txs := generateBatch(t)
				missingTxM := make(map[uint64]*types.Transaction)
				// lack of tx0 in txpool
				missingTxM[0] = txs[0]
				missingTxsCache.Store(poolBatch.BatchHash, missingTxM)
				txs = txs[1:]
				lo.ForEach(txs, func(tx *types.Transaction, _ int) {
					err := cnf.TxPool.AddLocalTx(tx)
					assert.Nil(t, err)
				})
				remoteProposal(t, proposer, repo.MockP2PIDs, pipes, poolBatch, 2, 2)

				// wait date sync node to receive messages
				time.Sleep(10 * time.Millisecond)

				for id := range pipes {
					if id != node.chainState.SelfNodeInfo.ID {
						remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
					}
				}

				go func() {
					for {
						select {
						case r := <-node.stack.(*adaptor.RBFTAdaptor).ReadyC:
							block := testutil.ConstructBlock(fmt.Sprintf("block%d", r.Height), r.Height)
							state := &rbfttypes.ServiceState{
								MetaState: &rbfttypes.MetaState{
									Height: r.Height,
									Digest: block.Hash().String(),
								},
							}
							node.ReportExecuted(state)
						}
					}
				}()
			},
		},
	}

	for _, tc := range testCases {
		t.Log(tc.name)
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, pipes := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		// Start receiving messages on pipes
		for id, pipesM := range pipes {
			if id == node.chainState.SelfNodeInfo.ID {
				for name, pipe := range pipesM {
					go func(pipe p2p.Pipe, name string) {
						for {
							msg := pipe.Receive(context.Background())
							if msg == nil {
								return
							}
							consensusMsg := &consensus.ConsensusMessage{}
							err := consensusMsg.UnmarshalVT(msg.Data)
							assert.Nil(t, err)
							node.Step(context.Background(), consensusMsg)
						}
					}(pipe, name)
				}
			} else {
				go receiveMsg(t, pipesM, id, tc.remoteHandler)
			}
		}

		// Setup log output capturing
		taskDoneCh := make(chan bool, 1)
		catchExpectLogOut(t, node.logger, tc.expectResult, taskDoneCh)
		// Start node
		err := node.Start()
		assert.Equal(t, err != nil, tc.expectErr)

		tc.setupMocks(node, consensusMsgPipes, pipes, cnf, ctrl)
		assert.True(t, <-taskDoneCh)
	}
}

func TestNode_Notify(t *testing.T) {
	t.Parallel()
	t.Run("notifyGenerateBatch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, _ := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		node.notifyGenerateBatch(1)
	})

	t.Run("notifyFindNextBatch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, _ := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		node.notifyFindNextBatch()
	})
}

func TestNode_HandleTimeout(t *testing.T) {
	testCases := []struct {
		name         string
		input        timer.TimeoutEvent
		setupMocks   func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller)
		expectResult string
	}{
		{
			name:         "sync state restart error",
			input:        syncStateRestart,
			expectResult: "timer syncStateRestart doesn't exist",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				err := node.timeMgr.RemoveTimer(syncStateRestart)
				assert.Nil(t, err)
			},
		},
		{
			name:         "fetchMissingTxsResp restart error",
			input:        fetchMissingTxsResp,
			expectResult: "timer fetchMissingTxsResp doesn't exist",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				err := node.timeMgr.RemoveTimer(fetchMissingTxsResp)
				assert.Nil(t, err)
				batch, txs := generateBatch(t)
				missingTxsM := make(map[uint64]*types.Transaction)
				missingTxsM[0] = txs[0]
				missingTxHashM := lo.MapValues(missingTxsM, func(tx *types.Transaction, index uint64) string {
					return tx.RbftGetTxHash()
				})
				node.missingBatchesInFetching = &wrapFetchMissingRequest{
					request: &consensus.FetchMissingRequest{
						ReplicaId:            node.chainState.SelfNodeInfo.ID,
						View:                 2,
						SequenceNumber:       2,
						BatchDigest:          batch.BatchHash,
						MissingRequestHashes: missingTxHashM,
					},
					proposer: 1,
				}
			},
		},
		{
			name:         "omit fetchMissingTxs request",
			input:        fetchMissingTxsResp,
			expectResult: "no request found",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
			},
		},
		{
			name:         "fetchMissingTxs error",
			input:        fetchMissingTxsResp,
			expectResult: "replica 100 not in nodeIds chainConfig",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				batch, txs := generateBatch(t)
				missingTxsM := make(map[uint64]*types.Transaction)
				missingTxsM[0] = txs[0]
				missingTxHashM := lo.MapValues(missingTxsM, func(tx *types.Transaction, index uint64) string {
					return tx.RbftGetTxHash()
				})
				node.missingBatchesInFetching = &wrapFetchMissingRequest{
					request: &consensus.FetchMissingRequest{
						ReplicaId:            node.chainState.SelfNodeInfo.ID,
						View:                 2,
						SequenceNumber:       2,
						BatchDigest:          batch.BatchHash,
						MissingRequestHashes: missingTxHashM,
					},
					proposer: 100,
				}
			},
		},
		{
			name:         "syncStateResp restart error(timer syncStateResp doesn't exist)",
			input:        syncStateResp,
			expectResult: "timer syncStateResp doesn't exist",
			setupMocks: func(node *Node[types.Transaction, *types.Transaction], consensusMsgPipes map[int32]p2p.Pipe, pipes map[uint64]map[string]p2p.Pipe, cnf *common.Config, ctrl *gomock.Controller) {
				err := node.timeMgr.RemoveTimer(syncStateResp)
				assert.Nil(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			consensusMsgPipes, cnf, pipes := mockConfig(t, ctrl)
			node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
			tc.setupMocks(node, consensusMsgPipes, pipes, cnf, ctrl)
			taskDoneCh := make(chan bool, 1)
			catchExpectLogOut(t, node.logger, tc.expectResult, taskDoneCh)
			node.handleTimeout(tc.input)
			assert.True(t, <-taskDoneCh)
		})
	}
}

func TestNode_FetchMissingTxs(t *testing.T) {
	proposer := uint64(1)
	poolBatch, txs := generateBatch(t)
	missingTxM := make(map[uint64]*types.Transaction)
	// lack of tx0 in txpool
	missingTxM[0] = txs[0]
	missingData, err := missingTxM[0].RbftMarshal()
	assert.Nil(t, err)
	txs = txs[1:]
	testCases := []struct {
		name         string
		input        *consensus.FetchMissingResponse
		expectResult string
	}{
		{
			name: "success",
			input: &consensus.FetchMissingResponse{
				Status:         consensus.FetchMissingResponse_Success,
				ReplicaId:      proposer,
				View:           2,
				SequenceNumber: 2,
				BatchDigest:    poolBatch.BatchHash,
				MissingRequestHashes: map[uint64]string{
					0: missingTxM[0].RbftGetTxHash(),
				},
				MissingRequests: map[uint64][]byte{
					0: missingData,
				},
			},
			expectResult: "commit height: 2, proposer 1",
		},
		{
			name: "failed status",
			input: &consensus.FetchMissingResponse{
				Status:         consensus.FetchMissingResponse_Failure,
				ReplicaId:      proposer,
				View:           2,
				SequenceNumber: 2,
				BatchDigest:    poolBatch.BatchHash,
			},
			expectResult: "received fetchMissingResponse with failed status",
		},
		{
			name: "wrong proposer",
			input: &consensus.FetchMissingResponse{
				ReplicaId:      proposer + 1,
				View:           2,
				SequenceNumber: 2,
				BatchDigest:    poolBatch.BatchHash,
			},
			expectResult: "replica 2 which is not primary, ignore it",
		},
		{
			name: "wrong height",
			input: &consensus.FetchMissingResponse{
				ReplicaId:      proposer,
				View:           1,
				SequenceNumber: 1,
			},
			expectResult: "ignore fetchMissingResponse",
		},
		{
			name: "wrong missing txs length",
			input: &consensus.FetchMissingResponse{
				ReplicaId:      proposer,
				View:           2,
				SequenceNumber: 2,
				MissingRequestHashes: map[uint64]string{
					1: "txHash1",
					2: "txHash2",
				},
				MissingRequests: map[uint64][]byte{
					1: []byte("tx1"),
				},
			},
			expectResult: "received mismatch length fetchMissingResponse",
		},
		{
			name: "wrong missing txs type",
			input: &consensus.FetchMissingResponse{
				ReplicaId:      proposer,
				View:           2,
				SequenceNumber: 2,
				MissingRequestHashes: map[uint64]string{
					1: "txHash1",
				},
				MissingRequests: map[uint64][]byte{
					1: []byte("tx1"),
				},
			},
			expectResult: "tx unmarshal Error",
		},
		{
			name: "wrong batch Hash",
			input: &consensus.FetchMissingResponse{
				Status:         consensus.FetchMissingResponse_Success,
				ReplicaId:      proposer,
				View:           2,
				SequenceNumber: 2,
				BatchDigest:    "wrongBatchHash",
				MissingRequestHashes: map[uint64]string{
					0: missingTxM[0].RbftGetTxHash(),
				},
				MissingRequests: map[uint64][]byte{
					0: missingData,
				},
			},
			expectResult: "find something wrong with fetchMissingResponse",
		},
	}

	for _, tc := range testCases {
		t.Log(tc.name)
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, pipes := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		remoteHandler := func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
			var respMsg *consensus.ConsensusMessage

			switch msg.Type {
			case consensus.Type_SYNC_STATE:
				resp := &consensus.SyncStateResponse{
					ReplicaId:        id,
					View:             1,
					SignedCheckpoint: generateSignedCheckpoint(t, id, 1, types.NewHashByStr(hex.EncodeToString([]byte("block1"))).String(), "batchDigest1"),
				}
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}
				respMsg = &consensus.ConsensusMessage{
					Type:    consensus.Type_SYNC_STATE_RESPONSE,
					Payload: payload,
					Epoch:   1,
					From:    id,
				}
			case consensus.Type_FETCH_MISSING_REQUEST:
				resp := tc.input
				payload, err := resp.MarshalVT()
				if err != nil {
					return err
				}
				respMsg = &consensus.ConsensusMessage{
					Type:    consensus.Type_FETCH_MISSING_RESPONSE,
					Payload: payload,
					Epoch:   1,
					From:    id,
				}

			default:
				return nil
			}
			data, err := respMsg.MarshalVT()
			assert.Nil(t, err)
			err = pipe.Send(context.Background(), to, data)
			if err != nil {
				return err
			}
			return nil
		}

		// Start receiving messages on pipes
		for id, pipesM := range pipes {
			if id == node.chainState.SelfNodeInfo.ID {
				for name, pipe := range pipesM {
					go func(pipe p2p.Pipe, name string) {
						for {
							msg := pipe.Receive(context.Background())
							if msg == nil {
								return
							}
							consensusMsg := &consensus.ConsensusMessage{}
							err := consensusMsg.UnmarshalVT(msg.Data)
							assert.Nil(t, err)
							node.Step(context.Background(), consensusMsg)
						}
					}(pipe, name)
				}
			} else {
				go receiveMsg(t, pipesM, id, remoteHandler)
			}
		}

		lo.ForEach(txs, func(tx *types.Transaction, _ int) {
			err := cnf.TxPool.AddLocalTx(tx)
			assert.Nil(t, err)
		})
		remoteProposal(t, proposer, repo.MockP2PIDs, pipes, poolBatch, 2, 2)

		// wait date sync node to receive messages
		time.Sleep(10 * time.Millisecond)

		for id := range pipes {
			if id != node.chainState.SelfNodeInfo.ID {
				remoteReportCheckpoint(t, id, repo.MockP2PIDs, pipes, poolBatch.BatchHash, 2)
			}
		}

		// Setup log output capturing
		taskDoneCh := make(chan bool, 1)
		catchExpectLogOut(t, node.logger, tc.expectResult, taskDoneCh)
		// Start node
		err := node.Start()
		assert.Nil(t, err)

		assert.True(t, <-taskDoneCh)
		node.Stop()
	}
}

func TestNode_ProcessOutOfDateReqs(t *testing.T) {
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	txs := testutil.ConstructTxs(s, 20)

	testCases := []struct {
		name                 string
		setMocks             func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction])
		expectHandleReqCount int
	}{
		{
			name:                 "empty out of date requests",
			expectHandleReqCount: 0,
			setMocks: func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction]) {
				mockTxpool := mock_txpool.NewMockTxPool[types.Transaction, *types.Transaction](ctrl)
				mockTxpool.EXPECT().FilterOutOfDateRequests(gomock.Any()).Return(nil)
				node.txpool = mockTxpool
			},
		},
		{
			name:                 "1 set out of date requests",
			expectHandleReqCount: 4,
			setMocks: func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction]) {
				mockTxpool := mock_txpool.NewMockTxPool[types.Transaction, *types.Transaction](ctrl)
				mockTxpool.EXPECT().FilterOutOfDateRequests(gomock.Any()).Return(txs[:1])
				node.txpool = mockTxpool
			},
		},
		{
			name:                 "2 set out of date requests",
			expectHandleReqCount: 4 * 2,
			setMocks: func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction]) {
				mockTxpool := mock_txpool.NewMockTxPool[types.Transaction, *types.Transaction](ctrl)
				mockTxpool.EXPECT().FilterOutOfDateRequests(gomock.Any()).Return(txs)
				node.txpool = mockTxpool
			},
		},
	}

	for _, tc := range testCases {
		t.Log(tc.name)
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, pipes := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		tc.setMocks(ctrl, node)

		var actualHandleReqCount atomic.Uint64
		remoteHandler := func(msg *consensus.ConsensusMessage, pipe p2p.Pipe, to string, id uint64) error {
			switch msg.Type {
			case consensus.Type_REBROADCAST_REQUEST_SET:
				req := &consensus.ReBroadcastRequestSet{}
				err = req.UnmarshalVT(msg.Payload)
				assert.Nil(t, err)
				assert.Equal(t, req.ReplicaId, node.chainState.SelfNodeInfo.ID)
				actualHandleReqCount.Add(1)
			default:
				return nil
			}

			return nil
		}
		// start remote handler
		for id, pipesM := range pipes {
			if id != node.chainState.SelfNodeInfo.ID {
				go receiveMsg(t, pipesM, id, remoteHandler)
			}
		}

		// Start node
		err = node.processOutOfDateReqs()
		assert.Nil(t, err)

		// wait remote node handle request messages
		time.Sleep(20 * time.Millisecond)
		assert.Equal(t, uint64(tc.expectHandleReqCount), actualHandleReqCount.Load())

		node.Stop()
	}
}
func TestNode_HandleRebroadcastTxSet(t *testing.T) {
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	txs := testutil.ConstructTxs(s, 10)

	testCases := []struct {
		name      string
		input     *consensus.ReBroadcastRequestSet
		expectErr bool
		setMocks  func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction], outPut chan<- *types.Transaction)
	}{
		{
			name:      "wrong type",
			expectErr: true,
			setMocks: func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction], outPut chan<- *types.Transaction) {
			},
		},
		{
			name:      "success receive rebroadcast request set",
			expectErr: false,
			setMocks: func(ctrl *gomock.Controller, node *Node[types.Transaction, *types.Transaction], outPut chan<- *types.Transaction) {
				mockTxpool := mock_txpool.NewMockTxPool[types.Transaction, *types.Transaction](ctrl)
				mockTxpool.EXPECT().AddRebroadcastTxs(gomock.Any()).Do(func(txs []*types.Transaction) {
					for _, tx := range txs {
						outPut <- tx
					}
				})
				node.txpool = mockTxpool
			},
		},
	}

	for _, tc := range testCases {
		t.Log(tc.name)
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, _ := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		outputCh := make(chan *types.Transaction, 10)
		tc.setMocks(ctrl, node, outputCh)

		data, err := tc.input.MarshalVT()
		assert.Nil(t, err)

		event := &localEvent{
			EventType: eventType_consensusMessage,
			Event: &consensus.ConsensusMessage{
				Type:    consensus.Type_REBROADCAST_REQUEST_SET,
				Epoch:   node.chainState.EpochInfo.Epoch,
				Payload: data,
			},
		}
		// Start node
		node.processEvent(event)
		if !tc.expectErr {
			close(outputCh)
			index := 0
			for actualTx := range outputCh {
				assert.Equal(t, txs[index], actualTx)
				index++
			}
		}

		node.Stop()
	}
}

func TestNode_ProcessEvent(t *testing.T) {
	testCases := []struct {
		name         string
		input        *localEvent
		setupMocks   func(node *Node[types.Transaction, *types.Transaction])
		expectResult string
	}{
		{
			name: "wrong consensus msg event",
			input: &localEvent{
				EventType: eventType_consensusMessage,
				Event:     "wrong event",
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {},

			expectResult: "invalid event type",
		},
		{
			name: "wrong checkpoint event",
			input: &localEvent{
				EventType: eventType_syncBlock,
				Event:     "wrong event",
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {},

			expectResult: "invalid event type",
		},
		{
			name: "wrong sync state event",
			input: &localEvent{
				EventType: eventType_epochSync,
				Event:     "wrong event",
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {},

			expectResult: "invalid event type",
		},
		{
			name: "wrong state updated event",
			input: &localEvent{
				EventType: eventType_stateUpdated,
				Event:     "wrong event",
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {},

			expectResult: "invalid event type",
		},
		{
			name: "wrong executed event",
			input: &localEvent{
				EventType: eventType_executed,
				Event:     "wrong event",
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {},

			expectResult: "invalid event type",
		},
		{
			name: "handle null request",
			input: &localEvent{
				EventType: eventType_consensusMessage,
				Event: &consensus.ConsensusMessage{
					Type:  consensus.Type_NULL_REQUEST,
					Epoch: 1,
				},
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {},

			expectResult: "Replica 5 need to start sync state progress",
		},

		{
			name: "handle null request",
			input: &localEvent{
				EventType: eventType_consensusMessage,
				Event: &consensus.ConsensusMessage{
					Type:  consensus.Type_NULL_REQUEST,
					Epoch: 1,
				},
			},
			setupMocks: func(node *Node[types.Transaction, *types.Transaction]) {
				node.statusMgr.On(InSyncState)
			},
			expectResult: "Replica 5 is in syncing status, ignore it...",
		},
	}

	for _, tc := range testCases {
		t.Log(tc.name)
		ctrl := gomock.NewController(t)
		consensusMsgPipes, cnf, _ := mockConfig(t, ctrl)
		node := mockDataSyncerNode(consensusMsgPipes, cnf, t)
		tc.setupMocks(node)
		// Setup log output capturing
		taskDoneCh := make(chan bool, 1)
		catchExpectLogOut(t, node.logger, tc.expectResult, taskDoneCh)
		go node.listenEvent()
		node.postEvent(tc.input)
		assert.True(t, <-taskDoneCh)
	}
}
