package sync

//
//import (
//	"context"
//	"errors"
//	"fmt"
//	"reflect"
//	"strconv"
//	"testing"
//	"time"
//
//	"github.com/samber/lo"
//	"github.com/stretchr/testify/require"
//	"go.uber.org/mock/gomock"
//
//	"github.com/axiomesh/axiom-bft/common/consensus"
//	"github.com/axiomesh/axiom-kit/types"
//	"github.com/axiomesh/axiom-kit/types/pb"
//	"github.com/axiomesh/axiom-ledger/internal/network/mock_network"
//	"github.com/axiomesh/axiom-ledger/internal/sync/common"
//	"github.com/axiomesh/axiom-ledger/pkg/loggers"
//	"github.com/axiomesh/axiom-ledger/pkg/repo"
//	network2 "github.com/axiomesh/axiom-p2p"
//)
//
//func TestNewSyncManager(t *testing.T) {
//	t.Logf("Test new sync manager")
//	logger := loggers.Logger(loggers.BlockSync)
//	getChainMetaFn := func() *types.ChainMeta {
//		return nil
//	}
//	getBlockFn := func(height uint64) (*types.Block, error) {
//		return nil, nil
//	}
//	getBlockHeaderFn := func(height uint64) (*types.BlockHeader, error) {
//		return nil, nil
//	}
//	getReceiptsFn := func(height uint64) ([]*types.Receipt, error) {
//		return nil, nil
//	}
//	getEpochStateFn := func(key []byte) []byte {
//		return nil
//	}
//
//	ctrl := gomock.NewController(t)
//	net := mock_network.NewMockNetwork(ctrl)
//
//	cnf := repo.Sync{
//		TimeoutCountLimit: 1,
//	}
//
//	var createPipeErrCount = map[string]int{
//		common.SyncBlockRequestPipe:      1,
//		common.SyncBlockResponsePipe:     1,
//		common.SyncChainDataRequestPipe:  1,
//		common.SyncChainDataResponsePipe: 1,
//	}
//
//	net.EXPECT().CreatePipe(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, name string) (network2.Pipe, error) {
//		if createPipeErrCount[name] > 0 {
//			createPipeErrCount[name]--
//			return nil, errors.New("create pipe err")
//		}
//		return mock_network.NewMockPipe(ctrl), nil
//	}).AnyTimes()
//
//	for i := 0; i < len(createPipeErrCount); i++ {
//		_, err := NewSyncManager(logger, getChainMetaFn, getBlockFn, getBlockHeaderFn, getReceiptsFn, getEpochStateFn, net, cnf)
//		require.NotNil(t, err)
//		require.Contains(t, err.Error(), "create pipe err")
//	}
//}
//
//func TestStartSync(t *testing.T) {
//	t.Logf("Test start sync")
//	n := 4
//	syncs, ledgers := newMockBlockSyncs(t, n)
//	defer stopSyncs(syncs)
//
//	localId := "0"
//	// store blocks expect node 0
//	prepareLedger(t, ledgers, localId, 10)
//
//	for _, sync := range syncs {
//		sync.conf.TimeoutCountLimit = 1
//	}
//	// node0 start sync commitData
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	remoteId := "1"
//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//	remoteBlockHash := ledgers[remoteId].GetChainMeta().BlockHash.String()
//	quorumCkpt := &consensus.SignedCheckpoint{
//		Checkpoint: &consensus.Checkpoint{
//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//				Height: 10,
//				Digest: remoteBlockHash,
//			},
//		},
//	}
//
//	// test switch sync status err
//	err := syncs[0].switchSyncStatus(true)
//	require.Nil(t, err)
//	syncTaskDoneCh := make(chan error, 1)
//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 10, quorumCkpt), syncTaskDoneCh)
//	actualErr := waitSyncTaskDone(syncTaskDoneCh)
//	require.NotNil(t, actualErr)
//	require.Equal(t, actualErr, err)
//
//	require.Contains(t, err.Error(), "status is already true")
//	err = syncs[0].switchSyncStatus(false)
//	require.Nil(t, err)
//
//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 10, quorumCkpt), syncTaskDoneCh)
//	require.Nil(t, err)
//
//	// wait for reset Peers
//	time.Sleep(1 * time.Second)
//	// start sync model
//	for i := 0; i < n; i++ {
//		_, err = syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks := data.([]common.CommitData)
//		require.Equal(t, 9, len(blocks))
//		require.Equal(t, uint64(10), blocks[len(blocks)-1].GetHeight())
//	})
//}
//
//func TestStartSyncWithRemoteSendBlockResponseError(t *testing.T) {
//	t.Logf("Test start sync with remote send block response error")
//	n := 4
//	syncs, ledgers := newMockBlockSyncs(t, n, wrongTypeSendSyncBlockResponse, 0, 2)
//	defer stopSyncs(syncs)
//
//	localId := "0"
//	// store blocks expect node 0
//	prepareLedger(t, ledgers, localId, 10)
//
//	// start sync model
//	for i := 0; i < n; i++ {
//		_, err := syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//
//	// node0 start sync commitData
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	remoteId := "1"
//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//	remoteBlockHash := ledgers[remoteId].GetChainMeta().BlockHash.String()
//	quorumCkpt := &consensus.SignedCheckpoint{
//		Checkpoint: &consensus.Checkpoint{
//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//				Height: 10,
//				Digest: remoteBlockHash,
//			},
//		},
//	}
//
//	syncTaskDoneCh := make(chan error, 1)
//
//	err := syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 10, quorumCkpt), syncTaskDoneCh)
//	require.Nil(t, err)
//
//	err = waitSyncTaskDone(syncTaskDoneCh)
//	require.Nil(t, err)
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks := data.([]common.CommitData)
//		require.Equal(t, 9, len(blocks))
//		require.Equal(t, uint64(10), blocks[len(blocks)-1].GetHeight())
//	})
//}
//
//func TestMultiEpochSync(t *testing.T) {
//	t.Logf("Test multi epoch sync")
//	n := 4
//	syncs, ledgers := newMockBlockSyncs(t, n)
//	defer stopSyncs(syncs)
//
//	localId := "0"
//	// store blocks expect node 0
//	prepareLedger(t, ledgers, localId, 300)
//
//	// start sync model
//	for i := 0; i < n; i++ {
//		_, err := syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//
//	// node0 start sync commitData
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	remoteId := "1"
//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//	remoteBlockHash := ledgers[remoteId].GetChainMeta().BlockHash.String()
//	quorumCkpt300 := &pb.QuorumCheckpoint{
//		Epoch: 4,
//		State: &pb.ExecuteState{
//			Height: 300,
//			Digest: remoteBlockHash,
//		},
//	}
//
//	block99, err := ledgers[remoteId].GetBlock(99)
//	require.Nil(t, err)
//	block199, err := ledgers[remoteId].GetBlock(199)
//	require.Nil(t, err)
//	block299, err := ledgers[remoteId].GetBlock(299)
//	require.Nil(t, err)
//	epc1 := &pb.EpochChange{
//		QuorumCheckpoint: &pb.QuorumCheckpoint{
//			Epoch: 1,
//			State: &pb.ExecuteState{
//				Height: block99.Height(),
//				Digest: block99.Hash().String(),
//			},
//		},
//	}
//	epc2 := &pb.EpochChange{
//		QuorumCheckpoint: &pb.QuorumCheckpoint{
//			Epoch: 2,
//			State: &pb.ExecuteState{
//				Height: block199.Height(),
//				Digest: block199.Hash().String(),
//			},
//		},
//	}
//
//	epc3 := &pb.EpochChange{
//		QuorumCheckpoint: &pb.QuorumCheckpoint{
//			Epoch: 3,
//			State: &pb.ExecuteState{
//				Height: block299.Height(),
//				Digest: block299.Hash().String(),
//			},
//		},
//	}
//
//	syncTaskDoneCh := make(chan error, 1)
//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 300, quorumCkpt300, epc1, epc2, epc3), syncTaskDoneCh)
//	require.Nil(t, err)
//	err = waitSyncTaskDone(syncTaskDoneCh)
//	require.Nil(t, err)
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks1 := data.([]common.CommitData)
//		require.Equal(t, 99, len(blocks1))
//		require.Equal(t, uint64(100), blocks1[len(blocks1)-1].GetHeight())
//	})
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks2 := data.([]common.CommitData)
//		require.Equal(t, 100, len(blocks2))
//		require.Equal(t, uint64(200), blocks2[len(blocks2)-1].GetHeight())
//	})
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks3 := data.([]common.CommitData)
//		require.Equal(t, 100, len(blocks3))
//		require.Equal(t, uint64(300), blocks3[len(blocks3)-1].GetHeight())
//	})
//
//	require.False(t, syncs[0].syncStatus.Load())
//}
//
//func TestMultiEpochSyncWithWrongBlock(t *testing.T) {
//	t.Logf("TestMultiEpochSyncWithWrongBlock")
//	n := 4
//	syncs, ledgers := newMockBlockSyncs(t, n)
//	defer stopSyncs(syncs)
//
//	localId := "0"
//	// store blocks expect node 0
//	prepareLedger(t, ledgers, localId, 200)
//
//	// start sync model
//	for i := 0; i < n; i++ {
//		_, err := syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//
//	// node0 start sync commitData
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	remoteId := "1"
//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//
//	block100, err := ledgers[remoteId].GetBlock(100)
//	require.Nil(t, err)
//
//	epc1 := &consensus.EpochChange{
//		Checkpoint: &pb.QuorumCheckpoint{
//			Checkpoint: &consensus.Checkpoint{
//				ExecuteState: &consensus.Checkpoint_ExecuteState{
//					Height: block100.Height(),
//					Digest: block100.Hash().String(),
//				},
//			},
//		},
//	}
//
//	// wrong commitData is not epoch commitData
//	// mock wrong commitData
//	wrongHeight := uint64(7)
//	oldRightBlock, err := ledgers[remoteId].GetBlock(wrongHeight)
//	require.Nil(t, err)
//	parentBlock, err := ledgers[remoteId].GetBlock(wrongHeight - 1)
//	require.Nil(t, err)
//	wrongBlockMulti := &types.Block{
//		Header: &types.BlockHeader{
//			Number:     wrongHeight,
//			ParentHash: parentBlock.Hash(),
//		},
//	}
//
//	idx := wrongHeight % uint64(len(peers))
//	wrongRemoteId := peers[idx]
//
//	err = ledgers[strconv.FormatUint(wrongRemoteId.Id, 10)].PersistExecutionResult(wrongBlockMulti, genReceipts(wrongBlockMulti))
//	require.Nil(t, err)
//
//	block10, err := ledgers[remoteId].GetBlock(10)
//	require.Nil(t, err)
//
//	quorumCkpt10 := &consensus.SignedCheckpoint{
//		Checkpoint: &consensus.Checkpoint{
//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//				Height: block10.Height(),
//				Digest: block10.Hash().String(),
//			},
//		},
//	}
//	// start sync
//	syncTaskDoneCh := make(chan error, 1)
//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 10, quorumCkpt10), syncTaskDoneCh)
//	require.Nil(t, err)
//	err = waitSyncTaskDone(syncTaskDoneCh)
//	require.Nil(t, err)
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks1 := data.([]common.CommitData)
//		require.Equal(t, 9, len(blocks1))
//		require.Equal(t, uint64(10), blocks1[len(blocks1)-1].GetHeight())
//		require.Equal(t, block10.Hash().String(), blocks1[len(blocks1)-1].GetHash())
//		require.False(t, syncs[0].syncStatus.Load())
//	})
//
//	// reset right block
//	err = ledgers[strconv.FormatUint(wrongRemoteId.Id, 10)].PersistExecutionResult(oldRightBlock, genReceipts(oldRightBlock))
//	require.Nil(t, err)
//
//	require.False(t, syncs[0].syncStatus.Load())
//
//	// wrong block is epoch block
//	// mock wrong commitData
//	wrongHeight = uint64(100)
//	oldRightBlock, err = ledgers[remoteId].GetBlock(wrongHeight)
//	require.Nil(t, err)
//	parentBlock, err = ledgers[remoteId].GetBlock(wrongHeight - 1)
//	require.Nil(t, err)
//	wrongBlockMulti = &types.Block{
//		Header: &types.BlockHeader{
//			Number:     wrongHeight,
//			ParentHash: parentBlock.Hash(),
//		},
//	}
//	wrongRemotePeer := syncs[0].peers[int(wrongHeight)%len(syncs[0].peers)]
//	wrongRemoteId = &common.Node{
//		Id:     wrongRemotePeer.Id,
//		PeerID: wrongRemotePeer.PeerID,
//	}
//	err = ledgers[strconv.FormatUint(wrongRemoteId.Id, 10)].PersistExecutionResult(wrongBlockMulti, genReceipts(wrongBlockMulti))
//	require.Nil(t, err)
//
//	block101, err := ledgers[remoteId].GetBlock(101)
//	require.Nil(t, err)
//
//	quorumCkpt101 := &consensus.SignedCheckpoint{
//		Checkpoint: &consensus.Checkpoint{
//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//				Height: block101.Height(),
//				Digest: block101.Hash().String(),
//			},
//		},
//	}
//
//	// start sync
//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 101, quorumCkpt101, epc1), syncTaskDoneCh)
//	require.Nil(t, err)
//	err = waitSyncTaskDone(syncTaskDoneCh)
//	require.Nil(t, err)
//
//	waitCommitData(t, syncs[0].Commit(),
//		func(t *testing.T, data any) {
//			blocks1 := data.([]common.CommitData)
//			require.Equal(t, 99, len(blocks1))
//			require.Equal(t, uint64(100), blocks1[len(blocks1)-1].GetHeight())
//		})
//
//	waitCommitData(t, syncs[0].Commit(),
//		func(t *testing.T, data any) {
//			blocks2 := data.([]common.CommitData)
//			require.Equal(t, 1, len(blocks2))
//			require.Equal(t, uint64(101), blocks2[len(blocks2)-1].GetHeight())
//		})
//
//	// reset right block
//	err = ledgers[strconv.FormatUint(wrongRemoteId.Id, 10)].PersistExecutionResult(oldRightBlock, genReceipts(oldRightBlock))
//	require.Nil(t, err)
//	require.False(t, syncs[0].syncStatus.Load())
//}
//
//func TestMultiEpochSyncWithWrongCheckpoint(t *testing.T) {
//	n := 4
//	testCases := []struct {
//		name         string
//		setMocks     func(localId string, peers []*common.Node, ledgers map[string]*mockLedger) *common.SyncParams
//		exceptErr    bool
//		exceptResult string
//	}{
//		{
//			name:         "wrong checkpoint in first epoch",
//			exceptResult: "quorum checkpoint is not equal to current hash",
//			exceptErr:    true,
//			setMocks: func(localId string, peers []*common.Node, ledgers map[string]*mockLedger) *common.SyncParams {
//				// store blocks expect node 0
//				prepareLedger(t, ledgers, localId, 100)
//
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//
//				startHeight := ledgers[localId].GetChainMeta().Height + 1
//				targetHeight := startHeight + 10
//				wrongQuorumCkpt := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: targetHeight,
//							Digest: "wrong digest",
//						},
//					},
//				}
//
//				return genSyncParams(peers, latestBlockHash, 2, startHeight, targetHeight, wrongQuorumCkpt)
//			},
//		},
//		{
//			name:         "wrong checkpoint in second epoch",
//			exceptResult: "quorum checkpoint is not equal to current hash",
//			exceptErr:    true,
//			setMocks: func(localId string, peers []*common.Node, ledgers map[string]*mockLedger) *common.SyncParams {
//				prepareLedger(t, ledgers, localId, 200)
//				remoteId := "1"
//
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//
//				startHeight := ledgers[localId].GetChainMeta().Height + 1
//				targetHeight := ledgers[remoteId].GetChainMeta().Height
//				wrongQuorumCkpt := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: targetHeight,
//							Digest: "wrong digest",
//						},
//					},
//				}
//				block100, err := ledgers[remoteId].GetBlock(100)
//				require.Nil(t, err)
//				epc1 := &consensus.EpochChange{
//					Checkpoint: &pb.QuorumCheckpoint{
//						Checkpoint: &consensus.Checkpoint{
//							ExecuteState: &consensus.Checkpoint_ExecuteState{
//								Height: block100.Height(),
//								Digest: block100.Hash().String(),
//							},
//						},
//					},
//				}
//				return genSyncParams(peers, latestBlockHash, 2, startHeight, targetHeight, wrongQuorumCkpt, epc1)
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		t.Logf("test case: %s", tc.name)
//
//		syncs, ledgers := newMockBlockSyncs(t, n)
//
//		localId := "0"
//
//		// start sync model
//		for i := 0; i < n; i++ {
//			_, err := syncs[i].Prepare()
//			syncs[i].Start()
//			require.Nil(t, err)
//		}
//
//		// node0 start sync commitData
//		peers := []*common.Node{
//			{
//				Id:     1,
//				PeerID: "1",
//			},
//			{
//				Id:     2,
//				PeerID: "2",
//			},
//			{
//				Id:     3,
//				PeerID: "3",
//			},
//		}
//
//		syncTaskDoneCh := make(chan error, 1)
//		param := tc.setMocks(localId, peers, ledgers)
//		err := syncs[0].StartSync(param, syncTaskDoneCh)
//		require.Nil(t, err)
//		err = waitSyncTaskDone(syncTaskDoneCh)
//		if tc.exceptErr {
//			require.NotNil(t, err)
//			require.Contains(t, err.Error(), tc.exceptResult)
//		} else {
//			require.Nil(t, err)
//		}
//		stopSyncs(syncs)
//	}
//}
//
//func TestHandleTimeoutBlockMsg(t *testing.T) {
//	testCases := []struct {
//		name         string
//		setMocks     func(localId string, peers []*common.Node, ledgers map[string]*mockLedger, syncs []*SyncManager) *common.SyncParams
//		expectResult int
//	}{
//		{
//			name:         "sync TimeoutBlock with one time",
//			expectResult: 3,
//			setMocks: func(localId string, peers []*common.Node, ledgers map[string]*mockLedger, syncs []*SyncManager) *common.SyncParams {
//				// store blocks expect node 0
//				prepareLedger(t, ledgers, localId, 200)
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//
//				// timeout with one time
//				timeoutBlockHeight := uint64(7)
//				idx := int(timeoutBlockHeight % uint64(len(peers)))
//				wrongId := fmt.Sprintf("%d", idx+1)
//
//				delete(ledgers[wrongId].blockDb, timeoutBlockHeight)
//
//				block10, err := ledgers[wrongId].GetBlock(10)
//				require.Nil(t, err)
//
//				quorumCkpt10 := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block10.Height(),
//							Digest: block10.Hash().String(),
//						},
//					},
//				}
//
//				return genSyncParams(peers, latestBlockHash, 2, 2, 10, quorumCkpt10)
//			},
//		},
//		{
//			name:         "sync TimeoutBlock with many time, bigger than remove count",
//			expectResult: 2,
//			setMocks: func(localId string, peers []*common.Node, ledgers map[string]*mockLedger, syncs []*SyncManager) *common.SyncParams {
//				prepareLedger(t, ledgers, localId, 200)
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//
//				remoteId := "1"
//				block100, err := ledgers[remoteId].GetBlock(100)
//				require.Nil(t, err)
//
//				epc1 := &consensus.EpochChange{
//					Checkpoint: &pb.QuorumCheckpoint{
//						Checkpoint: &consensus.Checkpoint{
//							ExecuteState: &consensus.Checkpoint_ExecuteState{
//								Height: block100.Height(),
//								Digest: block100.Hash().String(),
//							},
//						},
//					},
//				}
//
//				//
//				syncs[1].Stop()
//				syncs[0].conf.ConcurrencyLimit = 100
//				quorumCkpt100 := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block100.Height(),
//							Digest: block100.Hash().String(),
//						},
//					},
//				}
//				return genSyncParams(peers, latestBlockHash, 2, 2, 100, quorumCkpt100, epc1)
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		t.Logf("test case: %s", tc.name)
//		n := 4
//		// mock syncs[0] which send sync request error
//		syncs, ledgers := newMockBlockSyncs(t, n)
//		defer stopSyncs(syncs)
//
//		localId := "0"
//
//		// start sync model
//		for i := 0; i < n; i++ {
//			_, err := syncs[i].Prepare()
//			require.Nil(t, err)
//			syncs[i].Start()
//		}
//		// node0 start sync commitData
//		peers := []*common.Node{
//			{
//				Id:     1,
//				PeerID: "1",
//			},
//			{
//				Id:     2,
//				PeerID: "2",
//			},
//			{
//				Id:     3,
//				PeerID: "3",
//			},
//		}
//
//		// start sync
//		syncTaskDoneCh := make(chan error, 1)
//		param := tc.setMocks(localId, peers, ledgers, syncs)
//		err := syncs[0].StartSync(param, syncTaskDoneCh)
//		require.Nil(t, err)
//		progress := syncs[0].GetSyncProgress()
//		require.True(t, progress.InSync)
//		require.False(t, progress.CatchUp)
//		require.Equal(t, uint64(2), progress.StartSyncBlock)
//
//		err = waitSyncTaskDone(syncTaskDoneCh)
//		require.Nil(t, err)
//		progress = syncs[0].GetSyncProgress()
//		require.False(t, progress.InSync)
//
//		waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//			require.Equal(t, tc.expectResult, len(syncs[0].peers), "not remove timeout peer because timeoutCount < timeoutCountLimit")
//		})
//
//		require.False(t, syncs[0].syncStatus.Load())
//	}
//}
//
//func TestHandleSyncErrMsg(t *testing.T) {
//	t.Logf("test case: handle sync error msg")
//	n := 4
//	// mock syncs[0] which send sync request error
//	syncs, ledgers := newMockBlockSyncs(t, n, wrongTypeSendSyncBlockRequest, 0, 1)
//	defer stopSyncs(syncs)
//	localId := "0"
//	// store blocks expect node 0
//	prepareLedger(t, ledgers, localId, 100)
//
//	// start sync model
//	for i := 0; i < n; i++ {
//		_, err := syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//	// node0 start sync commitData
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	remoteId := "1"
//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//	remoteBlockHash := ledgers[remoteId].GetChainMeta().BlockHash.String()
//	quorumCkpt := &consensus.SignedCheckpoint{
//		Checkpoint: &consensus.Checkpoint{
//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//				Height: 100,
//				Digest: remoteBlockHash,
//			},
//		},
//	}
//	syncTaskDoneCh := make(chan error, 1)
//	err := syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 100, quorumCkpt), syncTaskDoneCh)
//	require.Nil(t, err)
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, data any) {
//		blocks := data.([]common.CommitData)
//		require.Equal(t, 99, len(blocks))
//		require.Equal(t, uint64(100), blocks[len(blocks)-1].GetHeight())
//	})
//}
//
//func TestValidateChunk(t *testing.T) {
//	testCases := []struct {
//		name         string
//		setupMocks   func(*SyncManager, *mockLedger)
//		expectErr    error
//		expectResult []*common.InvalidMsg
//	}{
//		{
//			name: "validate block success",
//			setupMocks: func(sync *SyncManager, ledger *mockLedger) {
//				sync.fullValidate = true
//				tx, err := types.GenerateEmptyTransactionAndSigner()
//				require.Nil(t, err)
//				sync.increaseRequester(&requester{
//					peerID:      "1",
//					blockHeight: 1,
//					quitCh:      make(chan struct{}, 1),
//					commitData: &common.BlockData{
//						Block: ConstructBlock(1, ledger.GetChainMeta().BlockHash, tx),
//					},
//				}, 1)
//			},
//		},
//		{
//			name:      "no requester",
//			expectErr: fmt.Errorf("requester[height:1] is nil"),
//			setupMocks: func(sync *SyncManager, ledger *mockLedger) {
//			},
//		},
//
//		{
//			name:      "requester commitData is nil",
//			expectErr: nil,
//			expectResult: []*common.InvalidMsg{
//				{
//					NodeID: "1",
//					Height: 1,
//					Typ:    common.SyncMsgType_TimeoutBlock,
//				},
//			},
//			setupMocks: func(sync *SyncManager, ledger *mockLedger) {
//				sync.increaseRequester(&requester{
//					peerID:      "1",
//					blockHeight: 1,
//					quitCh:      make(chan struct{}, 1),
//				}, 1)
//			},
//		},
//
//		{
//			name:      "requester commitData is invalid, previous commitData had checked",
//			expectErr: nil,
//			expectResult: []*common.InvalidMsg{
//				{
//					NodeID: "1",
//					Height: 1,
//					Typ:    common.SyncMsgType_InvalidBlock,
//				},
//			},
//			setupMocks: func(sync *SyncManager, ledger *mockLedger) {
//				sync.increaseRequester(&requester{
//					peerID:      "1",
//					blockHeight: 1,
//					quitCh:      make(chan struct{}, 1),
//					commitData: &common.BlockData{
//						Block: ConstructBlock(2, generateHash("wrongHash")),
//					},
//				}, 1)
//			},
//		},
//		{
//			name:      "requester commitData is invalid, previous commitData had not checked",
//			expectErr: nil,
//			expectResult: []*common.InvalidMsg{
//				{
//					NodeID: "1",
//					Height: 1,
//					Typ:    common.SyncMsgType_InvalidBlock,
//				},
//				{
//					NodeID: "2",
//					Height: 2,
//					Typ:    common.SyncMsgType_InvalidBlock,
//				},
//			},
//			setupMocks: func(sync *SyncManager, ledger *mockLedger) {
//				sync.chunk = &common.Chunk{
//					ChunkSize: 2,
//				}
//				sync.increaseRequester(&requester{
//					peerID:      "1",
//					blockHeight: 1,
//					quitCh:      make(chan struct{}, 1),
//					commitData: &common.BlockData{
//						Block: ConstructBlock(1, ledger.GetChainMeta().BlockHash),
//					},
//				}, 1)
//				sync.increaseRequester(&requester{
//					peerID:      "2",
//					blockHeight: 2,
//					quitCh:      make(chan struct{}, 1),
//					commitData: &common.BlockData{
//						Block: ConstructBlock(2, generateHash("wrongHash")),
//					},
//				}, 2)
//			},
//		},
//		{
//			name: "validate Block body failed",
//			expectResult: []*common.InvalidMsg{
//				{
//					NodeID: "1",
//					Height: 1,
//					Typ:    common.SyncMsgType_InvalidBlock,
//				},
//			},
//			setupMocks: func(sync *SyncManager, ledger *mockLedger) {
//				sync.fullValidate = true
//				tx, err := types.GenerateEmptyTransactionAndSigner()
//				require.Nil(t, err)
//
//				block := ConstructBlock(1, ledger.GetChainMeta().BlockHash, tx)
//				block.Header.TxRoot = generateHash("wrongHash")
//				sync.increaseRequester(&requester{
//					peerID:      "1",
//					blockHeight: 1,
//					quitCh:      make(chan struct{}, 1),
//					commitData: &common.BlockData{
//						Block: block,
//					},
//				}, 1)
//			},
//		},
//	}
//
//	for _, testCase := range testCases {
//		t.Log(testCase.name)
//		syncs, ledgers := newMockBlockSyncs(t, 4)
//		localId := 0
//		sync, ledger := syncs[localId], ledgers[fmt.Sprintf("%d", localId)]
//
//		sync.latestCheckedState = &pb.CheckpointState{
//			Height: ledger.GetChainMeta().Height,
//			Digest: ledger.GetChainMeta().BlockHash.String(),
//		}
//		sync.curHeight = 1
//		sync.chunk = &common.Chunk{
//			ChunkSize: 1,
//		}
//
//		testCase.setupMocks(sync, ledger)
//
//		invalidMsgs, err := syncs[localId].validateChunk()
//		if testCase.expectErr != nil {
//			require.Contains(t, err.Error(), testCase.expectErr.Error())
//		}
//		if testCase.expectResult != nil {
//			require.Equal(t, len(testCase.expectResult), len(invalidMsgs))
//			for i := 0; i < len(testCase.expectResult); i++ {
//				require.True(t, reflect.DeepEqual(*testCase.expectResult[i], *invalidMsgs[i]))
//			}
//		}
//
//	}
//}
//
//func TestRequestState(t *testing.T) {
//	testCases := []struct {
//		name         string
//		expectResult string
//		expectErr    bool
//		setupMocks   func(localId string, n int, peers []*common.Node) ([]*SyncManager, *common.SyncParams)
//	}{
//		{
//			name:         "test request state with wrong localState",
//			expectResult: "quorum state is not equal to current state",
//			expectErr:    true,
//			setupMocks: func(localId string, n int, peers []*common.Node) ([]*SyncManager, *common.SyncParams) {
//				// mock syncs[0] which send sync request error
//				syncs, ledgers := newMockBlockSyncs(t, n, wrongTypeSendSyncBlockRequest, 0, 1)
//				// store blocks expect node 0
//				prepareLedger(t, ledgers, localId, 10)
//
//				remoteId := "1"
//				wrongLatestBlockHash := "wrong hash"
//				block10, err := ledgers[remoteId].GetBlock(10)
//				require.Nil(t, err)
//				quorumCkpt10 := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block10.Height(),
//							Digest: block10.Hash().String(),
//						},
//					},
//				}
//
//				param := genSyncParams(peers, wrongLatestBlockHash, 2, 2, 10, quorumCkpt10)
//				return syncs, param
//			},
//		},
//
//		{
//			name:      "request state with different state",
//			expectErr: false,
//			setupMocks: func(localId string, n int, peers []*common.Node) ([]*SyncManager, *common.SyncParams) {
//				// peer1 will latency send state request
//				syncs, ledgers := newMockBlockSyncs(t, n, latencyTypeSendState, 0, 1)
//				defer stopSyncs(syncs)
//
//				// peer2 will send wrong state request
//				wrongRemoteId := uint64(2)
//				// store blocks expect node 0
//				prepareLedger(t, ledgers, localId, 10)
//				wrongGenesisBlock := &types.Block{
//					Header: &types.BlockHeader{
//						Number: 1,
//					},
//				}
//				err := ledgers[strconv.FormatUint(wrongRemoteId, 10)].PersistExecutionResult(wrongGenesisBlock, genReceipts(wrongGenesisBlock))
//				require.Nil(t, err)
//
//				remoteId := "1"
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash
//				block10, err := ledgers[remoteId].GetBlock(10)
//				require.Nil(t, err)
//				quorumCkpt10 := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block10.Height(),
//							Digest: block10.Hash().String(),
//						},
//					},
//				}
//
//				param := genSyncParams(peers, latestBlockHash.String(), 2, 2, 10, quorumCkpt10)
//
//				return syncs, param
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		t.Log(tc.name)
//		n := 4
//		localId := "0"
//		// node0 start sync commitData
//		peers := []*common.Node{
//			{
//				Id:     1,
//				PeerID: "1",
//			},
//			{
//				Id:     2,
//				PeerID: "2",
//			},
//			{
//				Id:     3,
//				PeerID: "3",
//			},
//		}
//
//		syncs, param := tc.setupMocks(localId, n, peers)
//
//		// start sync model
//		for i := 0; i < n; i++ {
//			_, err := syncs[i].Prepare()
//			require.Nil(t, err)
//			syncs[i].Start()
//		}
//
//		// start sync
//		syncTaskDoneCh := make(chan error, 1)
//		err := syncs[0].StartSync(param, syncTaskDoneCh)
//		if tc.expectErr {
//			require.NotNil(t, err)
//			require.Contains(t, err.Error(), tc.expectResult)
//		} else {
//			require.Nil(t, err)
//		}
//	}
//
//	//t.Run("test request state with wrong localState", func(t *testing.T) {
//	//	n := 4
//	//	// mock syncs[0] which send sync request error
//	//	syncs, ledgers := newMockBlockSyncs(t, n, wrongTypeSendSyncBlockRequest, 0, 1)
//	//	defer stopSyncs(syncs)
//	//	localId := "0"
//	//	// store blocks expect node 0
//	//	prepareLedger(t, ledgers, localId, 10)
//	//
//	//	// start sync model
//	//	for i := 0; i < n; i++ {
//	//		_, err := syncs[i].Prepare()
//	//		require.Nil(t, err)
//	//		syncs[i].Start()
//	//	}
//	//	// node0 start sync commitData
//	//	peers := []*common.Node{
//	//		{
//	//			Id:     1,
//	//			PeerID: "1",
//	//		},
//	//		{
//	//			Id:     2,
//	//			PeerID: "2",
//	//		},
//	//		{
//	//			Id:     3,
//	//			PeerID: "3",
//	//		},
//	//	}
//	//	remoteId := "1"
//	//	wrongLatestBlockHash := "wrong hash"
//	//	block10, err := ledgers[remoteId].GetBlock(10)
//	//	require.Nil(t, err)
//	//	quorumCkpt10 := &consensus.SignedCheckpoint{
//	//		Checkpoint: &consensus.Checkpoint{
//	//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//	//				Height: block10.Height(),
//	//				Digest: block10.Hash().String(),
//	//			},
//	//		},
//	//	}
//	//	// start sync
//	//	syncTaskDoneCh := make(chan error, 1)
//	//	err = syncs[0].StartSync(genSyncParams(peers, wrongLatestBlockHash, 2, 2, 10, quorumCkpt10), syncTaskDoneCh)
//	//	require.NotNil(t, err)
//	//	require.Contains(t, err.Error(), "quorum state is not equal to current state")
//	//})
//	//
//	//t.Run("request state with different state", func(t *testing.T) {
//	//	n := 4
//	//	// peer1 will latency send state request
//	//	syncs, ledgers := newMockBlockSyncs(t, n, latencyTypeSendState, 0, 1)
//	//	defer stopSyncs(syncs)
//	//
//	//	// peer2 will send wrong state request
//	//	wrongRemoteId := uint64(2)
//	//	localId := "0"
//	//	// store blocks expect node 0
//	//	prepareLedger(t, ledgers, localId, 10)
//	//	wrongGensisBlock := &types.Block{
//	//		Header: &types.BlockHeader{
//	//			Number: 1,
//	//		},
//	//	}
//	//	err := ledgers[strconv.FormatUint(wrongRemoteId, 10)].PersistExecutionResult(wrongGensisBlock, genReceipts(wrongGensisBlock))
//	//	require.Nil(t, err)
//	//
//	//	// start sync model
//	//	for i := 0; i < n; i++ {
//	//		_, err := syncs[i].Prepare()
//	//		require.Nil(t, err)
//	//		syncs[i].Start()
//	//	}
//	//	// node0 start sync commitData
//	//	peers := []*common.Node{
//	//		{
//	//			Id:     1,
//	//			PeerID: "1",
//	//		},
//	//		{
//	//			Id:     2,
//	//			PeerID: "2",
//	//		},
//	//		{
//	//			Id:     3,
//	//			PeerID: "3",
//	//		},
//	//	}
//	//	remoteId := "1"
//	//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash
//	//	block10, err := ledgers[remoteId].GetBlock(10)
//	//	require.Nil(t, err)
//	//	quorumCkpt10 := &consensus.SignedCheckpoint{
//	//		Checkpoint: &consensus.Checkpoint{
//	//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//	//				Height: block10.Height(),
//	//				Digest: block10.Hash().String(),
//	//			},
//	//		},
//	//	}
//	//	// start sync
//	//	syncTaskDoneCh := make(chan error, 1)
//	//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash.String(), 2, 2, 10, quorumCkpt10), syncTaskDoneCh)
//	//	require.Nil(t, err)
//	//
//	//	err = waitSyncTaskDone(syncTaskDoneCh)
//	//	require.Nil(t, err)
//	//})
//}
//
//func TestSwitchMode(t *testing.T) {
//	t.Logf("TestSwitchMode")
//	n := 1
//	syncs, _ := newMockBlockSyncs(t, n)
//	defer stopSyncs(syncs)
//
//	originMode := syncs[0].mode
//	require.Equal(t, common.SyncModeFull, originMode)
//
//	wrongMode := common.SyncMode(1000)
//	err := syncs[0].SwitchMode(wrongMode)
//	require.NotNil(t, err)
//	require.Contains(t, err.Error(), "invalid newMode")
//
//	err = syncs[0].SwitchMode(originMode)
//	require.NotNil(t, err)
//	require.Contains(t, err.Error(), "current mode is same")
//
//	syncs[0].syncStatus.Store(true)
//	err = syncs[0].SwitchMode(common.SyncModeSnapshot)
//	require.NotNil(t, err)
//	require.Contains(t, err.Error(), "sync status is true")
//
//	syncs[0].syncStatus.Store(false)
//	err = syncs[0].SwitchMode(common.SyncModeSnapshot)
//	require.Nil(t, err)
//	require.Equal(t, syncs[0].mode, common.SyncModeSnapshot)
//
//	err = syncs[0].SwitchMode(common.SyncModeFull)
//	require.Nil(t, err)
//	require.Equal(t, syncs[0].mode, common.SyncModeFull)
//}
//
//func TestStartSyncWithSnapshotMode(t *testing.T) {
//	t.Logf("TestStartSyncWithSnapshotMode")
//	n := 4
//	// mock syncs[0] which send sync request error
//	syncs, ledgers := newMockBlockSyncs(t, n)
//	defer stopSyncs(syncs)
//	localId := "0"
//	// store blocks expect node 0
//	prepareLedger(t, ledgers, localId, 300)
//
//	// node0 start sync commitData
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	remoteId := "1"
//	latestBlockHash := ledgers[localId].GetChainMeta().BlockHash.String()
//	remoteBlockHash := ledgers[remoteId].GetChainMeta().BlockHash.String()
//	quorumCkpt300 := &consensus.SignedCheckpoint{
//		Checkpoint: &consensus.Checkpoint{
//			ExecuteState: &consensus.Checkpoint_ExecuteState{
//				Height: 300,
//				Digest: remoteBlockHash,
//			},
//		},
//	}
//
//	err := syncs[0].SwitchMode(common.SyncModeSnapshot)
//	require.Nil(t, err)
//
//	startEpcNum := uint64(1)
//	data, err := syncs[0].Prepare(common.WithPeers(peers),
//		common.WithStartEpochChangeNum(startEpcNum),
//		common.WithLatestPersistEpoch(0),
//		common.WithSnapCurrentEpoch(3),
//	)
//	require.Nil(t, err)
//	require.NotNil(t, data)
//	epcs := data.Data.([]*consensus.EpochChange)
//	require.Equal(t, 3, len(epcs))
//
//	// start sync model except node0 (other nodes should not start sync, but need prepare be listener)
//	for i := 1; i < n; i++ {
//		_, err = syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//
//	// start sync
//	syncTaskDoneCh := make(chan error, 1)
//	err = syncs[0].StartSync(genSyncParams(peers, latestBlockHash, 2, 2, 300, quorumCkpt300, epcs...), syncTaskDoneCh)
//	require.Nil(t, err)
//	err = waitSyncTaskDone(syncTaskDoneCh)
//	require.Nil(t, err)
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, chainData any) {
//		require.NotNil(t, chainData)
//		require.Equal(t, uint64(100), chainData.(*common.SnapCommitData).EpochState.Checkpoint.Height())
//	})
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, chainData any) {
//		require.NotNil(t, chainData)
//		require.Equal(t, uint64(200), chainData.(*common.SnapCommitData).EpochState.Checkpoint.Height())
//	})
//
//	waitCommitData(t, syncs[0].Commit(), func(t *testing.T, chainData any) {
//		require.NotNil(t, chainData)
//		require.Equal(t, uint64(300), chainData.(*common.SnapCommitData).EpochState.Checkpoint.Height())
//	})
//}
//
//func TestPickPeer(t *testing.T) {
//	t.Logf("Test pick random peer")
//	sm := &SyncManager{}
//	peers := []*common.Node{
//		{
//			Id:     1,
//			PeerID: "1",
//		},
//		{
//			Id:     2,
//			PeerID: "2",
//		},
//		{
//			Id:     3,
//			PeerID: "3",
//		},
//	}
//	sm.peers = []*common.Peer{
//		{Id: peers[0].Id, PeerID: peers[0].PeerID},
//		{Id: peers[1].Id, PeerID: peers[1].PeerID},
//		{Id: peers[2].Id, PeerID: peers[2].PeerID},
//	}
//	expectPeer := peers[0]
//	peer := sm.pickRandomPeer("not exist peer")
//	exist := false
//	lo.ForEach(peers, func(p *common.Node, _ int) {
//		if p.PeerID == peer {
//			exist = true
//		}
//	})
//	require.True(t, exist)
//
//	peer = sm.pickRandomPeer(expectPeer.PeerID)
//	require.NotEqual(t, expectPeer, peer)
//
//	sm.initPeers = []*common.Peer{
//		{Id: expectPeer.Id, PeerID: expectPeer.PeerID},
//	}
//	sm.removePeer(peers[1].PeerID)
//	sm.removePeer(peers[2].PeerID)
//
//	peer = sm.pickRandomPeer(expectPeer.PeerID)
//	require.Equal(t, expectPeer.PeerID, peer, "if we just set expectPeer in initPeers, it will always return expectPeer")
//}
//
//func TestUpdatePeers(t *testing.T) {
//	testCases := []struct {
//		name     string
//		setMocks func(peers []*common.Node, localId string, latestHeight int, ledgers map[string]*mockLedger) (*common.SyncParams, []string)
//	}{
//		{
//			name: "defaultLatestHeight equal target height",
//			setMocks: func(peers []*common.Node, localId string, latestHeight int, ledgers map[string]*mockLedger) (*common.SyncParams, []string) {
//				remoteId := "1"
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash
//				block10, err := ledgers[remoteId].GetBlock(10)
//				require.Nil(t, err)
//				quorumCkpt10 := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block10.Height(),
//							Digest: block10.Hash().String(),
//						},
//					},
//				}
//
//				targetHeight := uint64(latestHeight)
//				return genSyncParams(peers, latestBlockHash.String(), 2, 2, targetHeight, quorumCkpt10), nil
//			},
//		},
//		{
//			name: "defaultLatestHeight is bigger than target height",
//			setMocks: func(peers []*common.Node, localId string, latestHeight int, ledgers map[string]*mockLedger) (*common.SyncParams, []string) {
//				remoteId := "1"
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash
//				targetHeight := uint64(latestHeight - 1)
//				block, err := ledgers[remoteId].GetBlock(targetHeight)
//				require.Nil(t, err)
//				quorumCkpt := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block.Height(),
//							Digest: block.Hash().String(),
//						},
//					},
//				}
//				return genSyncParams(peers, latestBlockHash.String(), 2, 2, targetHeight, quorumCkpt), nil
//			},
//		},
//		{
//			name: "defaultLatestHeight is smaller than target height",
//			setMocks: func(peers []*common.Node, localId string, latestHeight int, ledgers map[string]*mockLedger) (*common.SyncParams, []string) {
//				remoteId := "1"
//				latestBlockHash := ledgers[localId].GetChainMeta().BlockHash
//				targetHeight := uint64(latestHeight - 1)
//				block, err := ledgers[remoteId].GetBlock(targetHeight)
//				require.Nil(t, err)
//
//				// remove targetHeight block in remote peers(N - removeCount < ensureOneCorrectNum)
//				N := len(peers)
//				quorum := 2
//				removeCount := N - quorum + 1
//
//				illegalPeers := make([]string, 0)
//				// decrease defaultLatestHeight(smaller than targetHeight)
//				for id, ledger := range ledgers {
//					if id == localId {
//						continue
//					}
//					ledger.chainMeta.Height = targetHeight - 1
//					removeCount--
//					illegalPeers = append(illegalPeers, id)
//					if removeCount == 0 {
//						break
//					}
//				}
//
//				quorumCkpt := &consensus.SignedCheckpoint{
//					Checkpoint: &consensus.Checkpoint{
//						ExecuteState: &consensus.Checkpoint_ExecuteState{
//							Height: block.Height(),
//							Digest: block.Hash().String(),
//						},
//					},
//				}
//				return genSyncParams(peers, latestBlockHash.String(), uint64(quorum), 2, targetHeight, quorumCkpt), illegalPeers
//			},
//		},
//	}
//
//	for _, tc := range testCases {
//		t.Logf("test update peers: %s", tc.name)
//		n := 4
//		localId := "0"
//		// node0 start sync commitData
//		peers := []*common.Node{
//			{
//				Id:     1,
//				PeerID: "1",
//			},
//			{
//				Id:     2,
//				PeerID: "2",
//			},
//			{
//				Id:     3,
//				PeerID: "3",
//			},
//		}
//		latestHeight := 10
//		syncs, ledgers := newMockBlockSyncs(t, n)
//		defer stopSyncs(syncs)
//		prepareLedger(t, ledgers, localId, latestHeight)
//
//		for i := 0; i < n; i++ {
//			_, err := syncs[i].Prepare()
//			require.Nil(t, err)
//			syncs[i].Start()
//		}
//		params, illegalPeers := tc.setMocks(peers, localId, latestHeight, ledgers)
//		// start sync
//		syncTaskDoneCh := make(chan error, 1)
//		err := syncs[0].StartSync(params, syncTaskDoneCh)
//		require.Nil(t, err)
//		err = waitSyncTaskDone(syncTaskDoneCh)
//		require.Nil(t, err)
//
//		if len(illegalPeers) == 0 {
//			// peers' latest height had been updated(count is bigger than ensureOneCorrectNum)
//			updatedCount := uint64(0)
//			lo.ForEach(syncs[0].peers, func(peer *common.Peer, _ int) {
//				if peer.LatestHeight == uint64(latestHeight) {
//					updatedCount++
//				}
//			})
//			require.True(t, updatedCount >= syncs[0].ensureOneCorrectNum)
//		} else {
//			var matchPeerId string
//			lo.ForEach(syncs[0].peers, func(peer *common.Peer, _ int) {
//				if peer.LatestHeight > uint64(latestHeight) {
//					matchPeerId = peer.PeerID
//				}
//			})
//
//			lo.ForEach(illegalPeers, func(illegalPeer string, _ int) {
//				require.NotEqual(t, matchPeerId, illegalPeer)
//			})
//		}
//	}
//}
//
//func TestTps(t *testing.T) {
//	t.Skip()
//	round := 20
//	localId := 0
//	begin := uint64(2)
//	syncCount := 1000
//	txCount := 500
//	end := begin + uint64(syncCount) - 1
//	epochInternal := 100
//	n := 4
//	snapDuration := make([]time.Duration, 0)
//	fullDuration := make([]time.Duration, 0)
//	syncs, epochChanges, allPeers := prepareBlockSyncs(t, epochInternal, localId, n, txCount, begin, end)
//	// start sync model
//	for i := 0; i < n; i++ {
//		_, err := syncs[i].Prepare()
//		require.Nil(t, err)
//		syncs[i].Start()
//	}
//
//	for i := 0; i < round; i++ {
//		// start sync
//		remotePeers := lo.Filter(allPeers, func(peer *common.Node, _ int) bool {
//			return peer.Id != uint64(localId)
//		})
//
//		latestBlock, err := syncs[localId].getBlockFunc(begin - 1)
//		require.Nil(t, err)
//		latestBlockHash := latestBlock.Hash().String()
//		remoteId := (localId + 1) % n
//		endBlock, err := syncs[remoteId].getBlockFunc(end)
//		require.Nil(t, err)
//		quorumCkpt := &consensus.SignedCheckpoint{
//			Checkpoint: &consensus.Checkpoint{
//				ExecuteState: &consensus.Checkpoint_ExecuteState{
//					Height: end,
//					Digest: endBlock.Hash().String(),
//				},
//			},
//		}
//		now := time.Now()
//		// start sync commitData
//		syncTaskDone := make(chan error, 1)
//		if i%2 == 0 {
//			err = syncs[localId].SwitchMode(common.SyncModeSnapshot)
//			require.Nil(t, err)
//		} else {
//			err = syncs[localId].SwitchMode(common.SyncModeFull)
//			require.Nil(t, err)
//		}
//
//		err = syncs[localId].StartSync(genSyncParams(remotePeers, latestBlockHash, 2, begin, end, quorumCkpt, epochChanges...), syncTaskDone)
//		require.Nil(t, err)
//		taskDoneCh := make(chan bool, 1)
//		go func(ch chan bool) {
//			for {
//				select {
//				case data := <-syncs[localId].Commit():
//					var height uint64
//					switch syncs[localId].mode {
//					case common.SyncModeSnapshot:
//						snapData, ok := data.(*common.SnapCommitData)
//						require.True(t, ok)
//						height = snapData.Data[len(snapData.Data)-1].GetHeight()
//					case common.SyncModeFull:
//						fullData, ok := data.([]common.CommitData)
//						require.True(t, ok)
//						height = fullData[len(fullData)-1].GetHeight()
//					}
//					if height == end {
//						ch <- true
//						return
//					}
//				}
//			}
//		}(taskDoneCh)
//
//		err = <-syncTaskDone
//		require.Nil(t, err)
//		<-taskDoneCh
//		cost := time.Since(now)
//		switch syncs[localId].mode {
//		case common.SyncModeSnapshot:
//			snapDuration = append(snapDuration, cost)
//		case common.SyncModeFull:
//			fullDuration = append(fullDuration, cost)
//		}
//		fmt.Printf("sync mode: %s, round%d cost time: %v\n", common.SyncModeMap[syncs[localId].mode], i, cost)
//		time.Sleep(1 * time.Millisecond)
//	}
//	caculateTps := func(duration []time.Duration, mode common.SyncMode) {
//		var sum time.Duration
//		lo.ForEach(duration, func(d time.Duration, _ int) {
//			sum += d
//		})
//		t.Logf("%s tps: %f", common.SyncModeMap[mode], float64(syncCount*txCount*round)/sum.Seconds())
//	}
//
//	caculateTps(snapDuration, common.SyncModeSnapshot)
//	caculateTps(fullDuration, common.SyncModeFull)
//}
