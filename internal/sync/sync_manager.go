package sync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rican7/retry"
	"github.com/Rican7/retry/backoff"
	"github.com/Rican7/retry/strategy"
	"github.com/axiomesh/axiom-ledger/internal/components"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/gammazero/workerpool"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/internal/sync/full_sync"
	"github.com/axiomesh/axiom-ledger/internal/sync/snap_sync"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	network2 "github.com/axiomesh/axiom-p2p"
)

var _ common.Sync = (*SyncManager)(nil)

type SyncManager struct {
	mode                common.SyncMode
	fullValidate        bool
	started             atomic.Bool
	modeConstructor     common.ISyncConstructor
	conf                repo.Sync
	syncStatus          atomic.Bool         // sync status
	initPeers           []*common.Peer      // p2p set of latest epoch validatorSet with init
	peers               []*common.Peer      // p2p set of latest epoch validatorSet
	ensureOneCorrectNum uint64              // ensureOneCorrectNum of latest epoch validatorSet
	startHeight         uint64              // startHeight
	curHeight           uint64              // current commitData which we need sync
	targetHeight        uint64              // sync target commitData height
	recvBlockSize       atomic.Int64        // current chunk had received commitData size
	latestCheckedState  *pb.CheckpointState // latest checked commitData state
	requesters          sync.Map            // requester map
	requesterLen        atomic.Int64        // requester length
	idleRequesterLen    atomic.Uint64       // idle requester length

	quorumCheckpoint   *pb.QuorumCheckpoint // latest checkpoint from remote
	epochChanges       []*pb.EpochChange    // every epoch change which the node behind
	getChainMetaFunc   func() *types.ChainMeta
	getBlockFunc       func(height uint64) (*types.Block, error)
	getBlockHeaderFunc func(height uint64) (*types.BlockHeader, error)
	getReceiptsFunc    func(height uint64) ([]*types.Receipt, error)
	getEpochStateFunc  func(key []byte) []byte

	network               network.Network
	chainDataRequestPipe  network.Pipe
	chainDataResponsePipe network.Pipe
	blockRequestPipe      network.Pipe
	blockDataResponsePipe network.Pipe

	commitDataCache []common.CommitData // store commitData of a chunk temporary

	chunk            *common.Chunk                 // every chunk task
	recvStateCh      chan *common.WrapperStateResp // receive state from remote peer
	quitStateCh      chan error                    // quit state channel
	stateTaskDone    atomic.Bool                   // state task done signal
	requesterCh      chan struct{}                 // chunk task done signal
	validChunkTaskCh chan struct{}                 // start validate chunk task signal

	recvEventCh chan *common.LocalEvent // timeout or invalid of sync Block request、 get sync progress event

	ctx    context.Context
	cancel context.CancelFunc

	syncCtx    context.Context
	syncCancel context.CancelFunc

	logger logrus.FieldLogger
}

func NewSyncManager(logger logrus.FieldLogger, getChainMetaFn func() *types.ChainMeta, getBlockFn func(height uint64) (*types.Block, error), getBlockHeaderFun func(height uint64) (*types.BlockHeader, error),
	receiptFn func(height uint64) ([]*types.Receipt, error), getEpochStateFunc func(key []byte) []byte, network network.Network, cnf repo.Sync) (*SyncManager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	syncMgr := &SyncManager{
		fullValidate:       cnf.FullValidation,
		mode:               common.SyncModeFull,
		modeConstructor:    newModeConstructor(common.SyncModeFull, common.WithContext(ctx)),
		logger:             logger,
		recvEventCh:        make(chan *common.LocalEvent, cnf.ConcurrencyLimit),
		recvStateCh:        make(chan *common.WrapperStateResp, cnf.ConcurrencyLimit),
		validChunkTaskCh:   make(chan struct{}, 1),
		quitStateCh:        make(chan error, 1),
		requesterCh:        make(chan struct{}, cnf.ConcurrencyLimit),
		getChainMetaFunc:   getChainMetaFn,
		getBlockFunc:       getBlockFn,
		getBlockHeaderFunc: getBlockHeaderFun,
		getReceiptsFunc:    receiptFn,
		getEpochStateFunc:  getEpochStateFunc,
		network:            network,
		conf:               cnf,

		ctx:    ctx,
		cancel: cancel,
	}

	// init sync block pipe
	reqPipe, err := syncMgr.network.CreatePipe(syncMgr.ctx, common.SyncBlockRequestPipe)
	if err != nil {
		return nil, err
	}
	syncMgr.blockRequestPipe = reqPipe

	respPipe, err := syncMgr.network.CreatePipe(syncMgr.ctx, common.SyncBlockResponsePipe)
	if err != nil {
		return nil, err
	}
	syncMgr.blockDataResponsePipe = respPipe

	// init sync chainData pipe
	chainReqPipe, err := syncMgr.network.CreatePipe(syncMgr.ctx, common.SyncChainDataRequestPipe)
	if err != nil {
		return nil, err
	}
	syncMgr.chainDataRequestPipe = chainReqPipe

	chainRespPipe, err := syncMgr.network.CreatePipe(syncMgr.ctx, common.SyncChainDataResponsePipe)
	if err != nil {
		return nil, err
	}
	syncMgr.chainDataResponsePipe = chainRespPipe

	// init syncStatus
	syncMgr.syncStatus.Store(false)

	syncMgr.logger.Info("Init sync manager success")

	return syncMgr, nil
}

func (sm *SyncManager) StartSync(params *common.SyncParams, syncTaskDoneCh chan error) error {
	now := time.Now()
	syncCount := params.TargetHeight - params.CurHeight + 1
	syncCtx, syncCancel := context.WithCancel(context.Background())
	sm.syncCtx = syncCtx
	sm.syncCancel = syncCancel

	// 1. filter active peers
	activePIds := sm.network.GetConnectedPeers(lo.FlatMap(params.Peers, func(p *common.Node, _ int) []string {
		return []string{p.PeerID}
	}))

	activePeers := lo.Filter(params.Peers, func(p *common.Node, _ int) bool {
		return lo.Contains(activePIds, p.PeerID)
	})

	// 2. update commitData sync info
	sm.InitBlockSyncInfo(activePeers, params.LatestBlockHash, params.Quorum, params.CurHeight, params.TargetHeight, params.QuorumCheckpoint, params.EpochChanges...)

	// 3. send sync state request to all validators, waiting for Quorum response
	if params.LatestBlockHash != (ethcommon.Hash{}).String() {
		err := sm.requestSyncState(sm.curHeight-1, params.LatestBlockHash)
		if err != nil {
			syncTaskDoneCh <- err
			return err
		}

		sm.logger.WithFields(logrus.Fields{
			"Quorum": sm.ensureOneCorrectNum,
		}).Info("Receive Quorum response")
	}

	// 4. switch sync status to true, if switch failed, return error
	if err := sm.switchSyncStatus(true); err != nil {
		syncTaskDoneCh <- err
		return err
	}

	// 5. start listen sync commitData response
	go sm.listenSyncCommitDataResponse()

	// 5. produce requesters for first chunk
	sm.produceRequester(sm.getConcurrencyLimit())

	// 6. start sync commitData task
	go func() {
		for {
			pendingRequester := uint64(sm.requesterLen.Load())
			switch {
			case pendingRequester >= sm.chunk.ChunkSize:
				sm.resetIdleRequester()
			WAIT_LOOP:
				for {
					select {
					case <-sm.syncCtx.Done():
						return
					case ev := <-sm.recvEventCh:
						switch ev.EventType {
						case common.EventType_InvalidMsg:
							req, ok := ev.Event.(*common.InvalidMsg)
							if !ok {
								sm.logger.Errorf("invalid event type: %v", ev)
								continue
							}
							sm.handleInvalidRequest(req)
						case common.EventType_GetSyncProgress:
							req, ok := ev.Event.(*common.GetSyncProgressReq)
							if !ok {
								sm.logger.Errorf("invalid event type: %v", ev)
								continue
							}
							req.Resp <- sm.handleSyncProgress()
						}
					case <-sm.validChunkTaskCh:
						// end sync if all task done
						if allTaskDone := sm.processChunkTask(syncCount, now, syncTaskDoneCh); allTaskDone {
							return
						}
						break WAIT_LOOP
					}
				}
			case sm.consumeIdleRequester():
				sm.makeRequesters(sm.curHeight + uint64(sm.requesterLen.Load()))
			default:
			LOCAL_LOOP:
				for {
					select {
					case ev := <-sm.recvEventCh:
						switch ev.EventType {
						case common.EventType_InvalidMsg:
							req, ok := ev.Event.(*common.InvalidMsg)
							if !ok {
								sm.logger.Errorf("invalid event type: %v", ev)
								continue
							}
							sm.handleInvalidRequest(req)
						case common.EventType_GetSyncProgress:
							req, ok := ev.Event.(*common.GetSyncProgressReq)
							if !ok {
								sm.logger.Errorf("invalid event type: %v", ev)
								continue
							}
							req.Resp <- sm.handleSyncProgress()
						}
					default:
						break LOCAL_LOOP
					}
				}

				time.Sleep(2 * time.Millisecond)
			}
		}

	}()

	return nil
}

func (sm *SyncManager) GetSyncProgress() *common.SyncProgress {
	if sm.status() {
		resp := make(chan *common.SyncProgress)
		ev := &common.LocalEvent{
			EventType: common.EventType_GetSyncProgress,
			Event: &common.GetSyncProgressReq{
				Resp: resp,
			},
		}
		sm.recvEventCh <- ev
		return <-resp
	}
	return &common.SyncProgress{TargetHeight: sm.targetHeight}
}

func (sm *SyncManager) handleSyncProgress() *common.SyncProgress {
	return &common.SyncProgress{
		InSync:            sm.syncStatus.Load(),
		StartSyncBlock:    sm.startHeight,
		CurrentSyncHeight: sm.curHeight,
		TargetHeight:      sm.targetHeight,
		SyncMode:          common.SyncModeMap[sm.mode],
		Peers: lo.FlatMap(sm.peers, func(p *common.Peer, _ int) []common.Node {
			return []common.Node{
				{
					Id:     p.Id,
					PeerID: p.PeerID,
				},
			}
		}),
	}
}

func newModeConstructor(mode common.SyncMode, opt ...common.ModeOption) common.ISyncConstructor {
	conf := &common.ModeConfig{}
	for _, o := range opt {
		o(conf)
	}
	switch mode {
	case common.SyncModeFull:
		return full_sync.NewFullSync()
	case common.SyncModeSnapshot:
		return snap_sync.NewSnapSync(conf.Logger, conf.Network)
	}
	return nil
}

func (sm *SyncManager) produceRequester(count uint64) {
	sm.idleRequesterLen.Add(count)
}

func (sm *SyncManager) consumeIdleRequester() bool {
	idleNum := sm.idleRequesterLen.Load()
	if idleNum > 0 {
		sm.idleRequesterLen.Store(sm.idleRequesterLen.Load() - 1)
	}
	return idleNum > 0
}

func (sm *SyncManager) resetIdleRequester() {
	sm.idleRequesterLen.Store(0)
}

func (sm *SyncManager) postInvalidMsg(msg *common.InvalidMsg) {
	ev := &common.LocalEvent{
		EventType: common.EventType_InvalidMsg,
		Event:     msg,
	}
	go func() {
		sm.recvEventCh <- ev
	}()
}

func (sm *SyncManager) processChunkTask(syncCount uint64, startTime time.Time, syncTaskDoneCh chan error) bool {
	invalidReqs, err := sm.validateChunk()
	if err != nil {
		sm.logger.WithFields(logrus.Fields{
			"err": err,
		}).Error("Validate chunk failed")
		syncTaskDoneCh <- err
		return true
	}

	if len(invalidReqs) != 0 {
		lo.ForEach(invalidReqs, func(req *common.InvalidMsg, index int) {
			sm.postInvalidMsg(req)
			sm.logger.WithFields(logrus.Fields{
				"peer":   req.NodeID,
				"height": req.Height,
				"err":    req.ErrMsg,
			}).Warning("Receive Invalid commitData")
		})
		return false
	}

	lastR := sm.getRequester(sm.curHeight + sm.chunk.ChunkSize - 1)
	if lastR == nil {
		err = errors.New("get last requester failed")
		sm.logger.WithFields(logrus.Fields{
			"height": sm.curHeight + sm.chunk.ChunkSize - 1,
		}).Error(err.Error())
		syncTaskDoneCh <- err
		return true
	}
	err = sm.validateChunkState(lastR.commitData.GetHeight(), lastR.commitData.GetHash())
	if err != nil {
		sm.logger.WithFields(logrus.Fields{
			"err": err,
		}).Error("Validate last chunk state failed")
		invalidMsg := &common.InvalidMsg{
			NodeID: lastR.peerID,
			Height: lastR.commitData.GetHeight(),
			Typ:    common.SyncMsgType_InvalidBlock,
		}
		sm.postInvalidMsg(invalidMsg)
		return false
	}

	// release requester and send commitData to commitDataCh
	sm.requesters.Range(func(height, r any) bool {
		if r.(*requester).commitData == nil {
			err = errors.New("requester commitData is nil")
			sm.logger.WithFields(logrus.Fields{
				"height": sm.curHeight + sm.chunk.ChunkSize - 1,
			}).Error(err.Error())
			syncTaskDoneCh <- err
			return false
		}

		sm.commitDataCache[height.(uint64)-sm.curHeight] = r.(*requester).commitData
		return true
	})

	// if commitDataCache is not full, continue to receive commitDataCache
	if len(sm.commitDataCache) != int(sm.chunk.ChunkSize) {
		return false
	}

	blocksLen := len(sm.commitDataCache)
	sm.updateLatestCheckedState(sm.commitDataCache[blocksLen-1].GetHeight(), sm.commitDataCache[blocksLen-1].GetHash())

	// if valid chunk task done, release all requester
	lo.ForEach(sm.commitDataCache, func(commitData common.CommitData, index int) {
		sm.releaseRequester(commitData.GetHeight())
	})

	if sm.chunk.CheckPoint != nil {
		idx := int(sm.chunk.CheckPoint.Height - sm.curHeight)
		if idx < 0 || idx > blocksLen-1 {
			sm.logger.Errorf("chunk checkpoint index out of range, checkpoint height:%d, current Height:%d, "+
				"commitData len:%d", sm.chunk.CheckPoint.Height, sm.curHeight, blocksLen)
			return false
		}

		// if checkpoint is not equal to last commitDataCache, it means we sync wrong commitDataCache, panic it
		if err = sm.verifyChunkCheckpoint(sm.commitDataCache[idx]); err != nil {
			sm.logger.Errorf("verify chunk checkpoint failed: %s", err)
			syncTaskDoneCh <- err
			return false
		}
	}

	sm.modeConstructor.PostCommitData(sm.commitDataCache)

	if sm.curHeight+sm.chunk.ChunkSize-1 == sm.targetHeight {
		if err = sm.stopSync(); err != nil {
			sm.logger.WithFields(logrus.Fields{
				"err": err,
			}).Error("Stop sync failed")
		}
		sm.logger.WithFields(logrus.Fields{
			"count":  syncCount,
			"target": sm.targetHeight,
			"elapse": time.Since(startTime).Seconds(),
		}).Info("Block sync done")
		blockSyncDuration.WithLabelValues(strconv.Itoa(int(syncCount))).Observe(time.Since(startTime).Seconds())
		syncTaskDoneCh <- nil
		return true
	} else {
		sm.logger.WithFields(logrus.Fields{
			"start":  sm.curHeight,
			"target": sm.curHeight + sm.chunk.ChunkSize - 1,
			"cost":   time.Since(sm.chunk.Time),
		}).Info("chunk task has done")
		sm.updateStatus()
		// produce many new requesters for new chunk
		sm.produceRequester(sm.getConcurrencyLimit())
	}
	return false
}

func (sm *SyncManager) validateChunkState(localHeight uint64, localHash string) error {
	return sm.requestSyncState(localHeight, localHash)
}

func (sm *SyncManager) validateChunk() ([]*common.InvalidMsg, error) {
	parentHash := sm.latestCheckedState.Digest
	for i := sm.curHeight; i < sm.curHeight+sm.chunk.ChunkSize; i++ {
		r := sm.getRequester(i)
		if r == nil {
			return nil, fmt.Errorf("requester[height:%d] is nil", i)
		}
		cd := r.commitData
		if cd == nil {
			sm.logger.WithFields(logrus.Fields{
				"height": i,
			}).Error("Block is nil")
			return []*common.InvalidMsg{
				{
					NodeID: r.peerID,
					Height: i,
					Typ:    common.SyncMsgType_TimeoutBlock,
				},
			}, nil
		}

		// 1. validate block hash consecutively
		if cd.GetParentHash() != parentHash {
			sm.logger.WithFields(logrus.Fields{
				"height":               i,
				"expect parent hash":   parentHash,
				"expect parent height": i - 1,
				"actual parent hash":   cd.GetParentHash(),
			}).Error("Block parent hash is not equal to latest checked state")

			invalidMsgs := make([]*common.InvalidMsg, 0)

			// if we have not previous requester, it means we had already checked previous commitData,
			// so we just return current invalid commitData
			prevR := sm.getRequester(i - 1)
			if prevR != nil {
				// we are not sure which commitData is wrong,
				// maybe parent commitData has wrong hash, maybe current commitData has wrong parent hash, so we return two invalidMsg
				prevInvalidMsg := &common.InvalidMsg{
					NodeID: prevR.peerID,
					Height: i - 1,
					Typ:    common.SyncMsgType_InvalidBlock,
				}
				invalidMsgs = append(invalidMsgs, prevInvalidMsg)
			}
			invalidMsgs = append(invalidMsgs, &common.InvalidMsg{
				NodeID: r.peerID,
				Height: i,
				Typ:    common.SyncMsgType_InvalidBlock,
			})

			return invalidMsgs, nil
		}

		// 2. validate block body
		if err := sm.validateBlockBody(cd); err != nil {
			sm.logger.WithFields(logrus.Fields{
				"height": i,
				"err":    err,
			}).Warning("Block body is invalid")
			return []*common.InvalidMsg{
				{
					NodeID: r.peerID,
					Height: i,
					Typ:    common.SyncMsgType_InvalidBlock,
				},
			}, nil
		}

		parentHash = cd.GetHash()
	}
	return nil, nil
}

func (sm *SyncManager) validateBlockBody(d common.CommitData) error {
	defer func(startTime time.Time) {
		validateBlockDuration.Observe(time.Since(startTime).Seconds())
	}(time.Now())

	// if fullValidate is false, we don't need to validate block body
	if !sm.fullValidate {
		return nil
	}

	block := d.GetBlock()

	// validate txRoot
	txRoot, err := components.CalcTxsMerkleRoot(block.Transactions)
	if err != nil {
		return err
	}
	if txRoot.String() != block.Header.TxRoot.String() {
		return fmt.Errorf("invalid txs root, caculate txRoot is %s, but remote block txRoot is %s", txRoot, block.Header.TxRoot)
	}

	return nil
}

func (sm *SyncManager) updateLatestCheckedState(height uint64, digest string) {
	sm.latestCheckedState = &pb.CheckpointState{
		Height: height,
		Digest: digest,
	}
}

func (sm *SyncManager) InitBlockSyncInfo(peers []*common.Node, latestBlockHash string, quorum, curHeight, targetHeight uint64,
	quorumCheckpoint *pb.QuorumCheckpoint, epc ...*pb.EpochChange) {
	sm.peers = make([]*common.Peer, len(peers))
	sm.initPeers = make([]*common.Peer, len(peers))
	lo.ForEach(peers, func(p *common.Node, index int) {
		sm.peers[index] = &common.Peer{
			Id:           p.Id,
			PeerID:       p.PeerID,
			TimeoutCount: 0,
			LatestHeight: targetHeight,
		}
	})
	copy(sm.initPeers, sm.peers)
	if quorum < 1 {
		quorum = 1
	}
	sm.ensureOneCorrectNum = quorum
	sm.startHeight = curHeight
	sm.curHeight = curHeight
	sm.targetHeight = targetHeight
	sm.quorumCheckpoint = quorumCheckpoint
	sm.epochChanges = epc
	sm.recvBlockSize.Store(0)

	sm.updateLatestCheckedState(curHeight-1, latestBlockHash)

	// init chunk
	sm.initChunk()

	sm.commitDataCache = make([]common.CommitData, sm.chunk.ChunkSize)
}

func (sm *SyncManager) initChunk() {
	chunkSize := sm.targetHeight - sm.curHeight + 1
	if chunkSize > sm.conf.MaxChunkSize {
		chunkSize = sm.conf.MaxChunkSize
	}

	// if we have epoch change, chunk size need smaller than epoch size
	if len(sm.epochChanges) != 0 {
		latestEpoch := sm.epochChanges[0]
		epochSize := latestEpoch.GetQuorumCheckpoint().GetState().GetHeight() - sm.curHeight + 1
		if epochSize < chunkSize {
			chunkSize = epochSize
		}
	}

	sm.chunk = &common.Chunk{
		ChunkSize: chunkSize,
	}

	chunkMaxHeight := sm.curHeight + chunkSize - 1

	var chunkCheckpoint *pb.CheckpointState

	if len(sm.epochChanges) != 0 {
		chunkCheckpoint = &pb.CheckpointState{
			Height: sm.epochChanges[0].GetQuorumCheckpoint().GetState().GetHeight(),
			Digest: sm.epochChanges[0].GetQuorumCheckpoint().GetState().GetDigest(),
		}
	} else {
		chunkCheckpoint = &pb.CheckpointState{
			Height: sm.quorumCheckpoint.State.Height,
			Digest: sm.quorumCheckpoint.State.Digest,
		}
	}
	sm.chunk.FillCheckPoint(chunkMaxHeight, chunkCheckpoint)
	sm.chunk.Time = time.Now()
}

func (sm *SyncManager) switchSyncStatus(status bool) error {
	if sm.syncStatus.Load() == status {
		return fmt.Errorf("status is already %v", status)
	}
	sm.syncStatus.Store(status)
	sm.logger.Info("SwitchSyncStatus: status is ", status)
	return nil
}

func (sm *SyncManager) listenSyncStateResp(ctx context.Context, height uint64, localHash string) {
	diffState := make(map[string][]string)

	for {
		select {
		case <-ctx.Done():
			return
		case resp := <-sm.recvStateCh:
			// update remote peers' latest block height,
			// if we request block height ig bigger than remote's latest height, we should pick a new peer
			sm.updatePeers(resp.PeerID, resp.Resp.CheckpointState.LatestHeight)
			sm.handleSyncStateResp(resp, diffState, height, localHash)
		}
	}
}

func (sm *SyncManager) requestSyncState(height uint64, localHash string) error {
	sm.logger.WithFields(logrus.Fields{
		"height":              height,
		"ensureOneCorrectNum": sm.ensureOneCorrectNum,
	}).Info("Prepare request sync state")
	sm.stateTaskDone.Store(false)

	// 1. start listen sync state response
	stateCtx, stateCancel := context.WithCancel(context.Background())
	defer stateCancel()
	go sm.listenSyncStateResp(stateCtx, height, localHash)

	wp := workerpool.New(len(sm.peers))
	// send sync state request to all validators, check our local state(latest commitData) is equal to Quorum state
	req := &pb.SyncStateRequest{
		Height: height,
	}
	data, err := req.MarshalVT()
	if err != nil {
		return err
	}

	// 2. send sync state request to all validators asynchronously
	// because Peers num maybe too small to cannot reach Quorum, so we use initPeers to send sync state
	lo.ForEach(sm.initPeers, func(p *common.Peer, index int) {
		select {
		case <-stateCtx.Done():
			wp.Stop()
			sm.logger.Debug("receive quit signal, Quit request state")
			return
		default:
			wp.Submit(func() {
				if err = retry.Retry(func(attempt uint) error {
					select {
					case <-stateCtx.Done():
						sm.logger.WithFields(logrus.Fields{
							"peer":   p.Id,
							"height": height,
						}).Debug("receive quit signal, Quit request state")
						return nil
					default:
						sm.logger.WithFields(logrus.Fields{
							"peer":   p.Id,
							"height": height,
						}).Debug("start send sync state request")

						resp, err := sm.network.Send(p.PeerID, &pb.Message{
							Type: pb.Message_SYNC_STATE_REQUEST,
							Data: data,
						})

						if err != nil {
							sm.logger.WithFields(logrus.Fields{
								"peer": p.Id,
								"err":  err,
							}).Warn("Send sync state request failed")
							return err
						}

						if err = sm.isValidSyncResponse(resp, p.PeerID); err != nil {
							sm.logger.WithFields(logrus.Fields{
								"peer": p.Id,
								"err":  err,
							}).Warn("Invalid sync state response")

							return fmt.Errorf("invalid sync state response: %s", err)
						}

						stateResp := &pb.SyncStateResponse{}
						if err = stateResp.UnmarshalVT(resp.Data); err != nil {
							return fmt.Errorf("unmarshal sync state response failed: %s", err)
						}

						select {
						case <-stateCtx.Done():
							sm.logger.WithFields(logrus.Fields{
								"peer":   p.Id,
								"height": height,
							}).Debug("receive quit signal, Quit request state")
							return nil
						default:
							if stateResp.Status == pb.Status_ERROR && strings.Contains(stateResp.Error, ledger.ErrNotFound.Error()) {
								sm.logger.WithFields(logrus.Fields{
									"height": height,
									"peerID": p.Id,
								}).Error("Block not found")
								sm.updatePeers(resp.From, stateResp.CheckpointState.LatestHeight)
								return errors.New("block not found")
							}

							if stateResp.Status == pb.Status_SUCCESS {
								ckps := pb.CheckpointState{
									Height: stateResp.CheckpointState.Height,
									Digest: stateResp.CheckpointState.Digest,
								}
								d, _ := ckps.MarshalVT()
								hash := sha256.Sum256(d)
								sm.recvStateCh <- &common.WrapperStateResp{
									PeerID: resp.From,
									Hash:   types.NewHash(hash[:]).String(),
									Resp:   stateResp,
								}
							}
						}
						return nil
					}
				}, strategy.Limit(common.MaxRetryCount), strategy.Backoff(backoff.Fibonacci(500*time.Millisecond))); err != nil {
					sm.logger.Errorf("Retry send sync state request failed: %s", err)
					return
				}
				sm.logger.WithFields(logrus.Fields{
					"peer":   p.Id,
					"height": height,
				}).Debug("Send sync state request success")
			})
		}
	})

	for {
		select {
		case stateErr := <-sm.quitStateCh:
			if stateErr != nil {
				return fmt.Errorf("request state failed: %s", stateErr)
			}
			return nil
		case <-time.After(sm.conf.WaitStatesTimeout.ToDuration()):
			sm.logger.WithFields(logrus.Fields{
				"height": height,
			}).Warn("Request state timeout")

			sm.stateTaskDone.Store(true)
			return fmt.Errorf("request state timeout: height:%d", height)
		}
	}
}

func (sm *SyncManager) isValidSyncResponse(msg *pb.Message, id string) error {
	if msg == nil || msg.Data == nil {
		return errors.New("sync response is nil")
	}

	if msg.From != id {
		return fmt.Errorf("receive different peer sync response, expect peer id is %s,"+
			" but receive peer id is %s", id, msg.From)
	}

	return nil
}

func (sm *SyncManager) handleCommitDataRequest(msg *network2.PipeMsg, mode common.SyncMode) ([]byte, uint64, error) {
	generateResp := func(mode common.SyncMode, data []byte) ([]byte, error) {
		msgTyp := common.CommitDataResponseType[mode]
		resp := &pb.Message{
			From: sm.network.PeerID(),
			Type: msgTyp,
			Data: data,
		}

		return resp.MarshalVT()
	}

	genFailedResp := func(height uint64, err error) ([]byte, error) {
		var resp common.SyncResponseMessage
		switch mode {
		case common.SyncModeFull:
			resp = &pb.SyncBlockResponse{
				Height: height,
				Status: pb.Status_ERROR,
				Error:  err.Error(),
			}
		case common.SyncModeSnapshot:
			resp = &pb.SyncChainDataResponse{
				Height: height,
				Status: pb.Status_ERROR,
				Error:  err.Error(),
			}
		}

		val, err := resp.MarshalVT()
		if err != nil {
			return nil, err
		}
		return generateResp(mode, val)
	}

	var respErr error

	req := common.CommitDataRequestConstructor[mode]()
	if err := req.UnmarshalVT(msg.Data); err != nil {
		sm.logger.Errorf("Unmarshal sync commitData request failed: %s", err)
		return nil, 0, err
	}
	requestHeight := req.GetHeight()

	block, respErr := sm.getBlockFunc(requestHeight)
	if respErr != nil {
		sm.logger.WithFields(logrus.Fields{
			"from":   msg.From,
			"height": requestHeight,
			"err":    respErr,
		}).Error("Get commitData failed")

		failedResp, err := genFailedResp(requestHeight, respErr)
		return failedResp, 0, err
	}

	blockBytes, respErr := block.Marshal()
	if respErr != nil {
		sm.logger.Errorf("Marshal commitData failed: %s", respErr)
		failedResp, err := genFailedResp(requestHeight, respErr)
		return failedResp, 0, err
	}

	var (
		msgResp []byte
		err     error
	)
	switch mode {
	case common.SyncModeFull:
		commitDataResp := &pb.SyncBlockResponse{Height: requestHeight, Block: blockBytes, Status: pb.Status_SUCCESS}
		commitDataBytes, respErr := commitDataResp.MarshalVT()
		if respErr != nil {
			sm.logger.Errorf("Marshal sync commitData response failed: %s", respErr)
			failedResp, err := genFailedResp(requestHeight, respErr)
			return failedResp, 0, err
		}

		msgResp, err = generateResp(mode, commitDataBytes)

	case common.SyncModeSnapshot:
		commitDataResp := &pb.SyncChainDataResponse{Height: requestHeight, Block: blockBytes}

		// get receipts by block height
		receipts, respErr := sm.getReceiptsFunc(requestHeight)
		if respErr != nil {
			sm.logger.WithFields(logrus.Fields{
				"from": msg.From,
				"err":  respErr,
			}).Error("Get receipts failed")
			failedResp, err := genFailedResp(requestHeight, respErr)
			return failedResp, 0, err
		}
		receiptsBytes, respErr := types.MarshalReceipts(receipts)
		if respErr != nil {
			sm.logger.Errorf("Marshal receipts failed: %s", respErr)
			failedResp, err := genFailedResp(requestHeight, respErr)
			return failedResp, 0, err
		}
		commitDataResp.Receipts = receiptsBytes
		commitDataResp.Status = pb.Status_SUCCESS
		commitDataBytes, respErr := commitDataResp.MarshalVT()
		if respErr != nil {
			sm.logger.Errorf("Marshal sync commitData response failed: %s", respErr)
			failedResp, err := genFailedResp(requestHeight, respErr)
			return failedResp, 0, err
		}

		msgResp, err = generateResp(mode, commitDataBytes)
	}

	return msgResp, requestHeight, err
}

func (sm *SyncManager) listenSyncChainDataRequest() {
	for {
		msg := sm.chainDataRequestPipe.Receive(sm.ctx)
		if msg == nil {
			sm.logger.Info("Stop listen sync chainData request")
			return
		}

		data, height, err := sm.handleCommitDataRequest(msg, common.SyncModeSnapshot)
		if err != nil {
			sm.logger.Errorf("Handle sync chainData request failed: %s", err)
			continue
		}
		if err = sm.sendCommitDataResponse(common.SyncModeSnapshot, msg.From, data, height); err != nil {
			sm.logger.Errorf("Send sync commitData response failed: %s", err)
			continue
		}
	}
}

func (sm *SyncManager) listenSyncBlockDataRequest() {
	for {
		msg := sm.blockRequestPipe.Receive(sm.ctx)
		if msg == nil {
			sm.logger.Info("Stop listen sync block request")
			return
		}

		data, height, err := sm.handleCommitDataRequest(msg, common.SyncModeFull)
		if err != nil {
			sm.logger.Errorf("Handle sync block request failed: %s", err)
			continue
		}

		if err = sm.sendCommitDataResponse(common.SyncModeFull, msg.From, data, height); err != nil {
			sm.logger.Errorf("Send sync commitData response failed: %s", err)
			continue
		}
	}
}

func (sm *SyncManager) sendCommitDataResponse(mode common.SyncMode, to string, respData []byte, height uint64) error {
	var pipe network.Pipe
	switch mode {
	case common.SyncModeFull:
		pipe = sm.blockDataResponsePipe
	case common.SyncModeSnapshot:
		pipe = sm.chainDataResponsePipe
	}
	if err := retry.Retry(func(attempt uint) error {
		err := pipe.Send(sm.ctx, to, respData)
		if err != nil {
			sm.logger.WithFields(logrus.Fields{
				"to":  to,
				"err": err,
			}).Error("Send sync commitData response failed")
			return err
		}
		sm.logger.WithFields(logrus.Fields{
			"to":     to,
			"height": height,
		}).Debug("Send sync commitData response success")
		return nil
	}, strategy.Limit(common.MaxRetryCount), strategy.Wait(500*time.Millisecond)); err != nil {
		sm.logger.Errorf("Retry send sync chainData response failed: %s", err)
		return err
	}
	return nil
}

func (sm *SyncManager) precheckResponse(p2pMsg *pb.Message) (common.SyncMessage, *requester, error) {
	// 1. check message type
	switch sm.mode {
	case common.SyncModeFull:
		if p2pMsg.Type != pb.Message_SYNC_BLOCK_RESPONSE {
			return nil, nil, fmt.Errorf("receive invalid sync block response type: %s", p2pMsg.Type)
		}

	case common.SyncModeSnapshot:
		if p2pMsg.Type != pb.Message_SYNC_CHAIN_DATA_RESPONSE {
			return nil, nil, fmt.Errorf("receive invalid sync chainData response type: %s", p2pMsg.Type)
		}
	}

	// 2. check response data
	resp := common.CommitDataResponseConstructor[sm.mode]()

	if err := resp.UnmarshalVT(p2pMsg.Data); err != nil {
		return nil, nil, fmt.Errorf("unmarshal sync commitData response failed: %s", err)
	}

	// 3. check requester
	dataRequester := sm.getRequester(resp.GetHeight())
	if dataRequester == nil {
		return nil, nil, fmt.Errorf("requester not found in height:%d", resp.GetHeight())
	}
	if dataRequester.peerID != p2pMsg.From {
		return nil, nil, fmt.Errorf("receive commitData which not distribute requester, height:%d, "+
			"receive from:%s, expect from:%s, we will ignore this commitData", resp.GetHeight(), p2pMsg.From, dataRequester.peerID)
	}

	if dataRequester.commitData != nil {
		return nil, nil, fmt.Errorf("receive duplicated commitData, height:%d", resp.GetHeight())
	}

	if resp.GetStatus() != pb.Status_SUCCESS {
		return resp, dataRequester, fmt.Errorf("receive invalid commitData response: %s, error: %s", resp.GetStatus().String(), resp.GetError())
	}

	return resp, dataRequester, nil
}

func (sm *SyncManager) listenSyncCommitDataResponse() {
	retrySendRequest := func(req *requester, err error) {
		sm.postInvalidMsg(&common.InvalidMsg{
			NodeID: req.peerID,
			Height: req.blockHeight,
			ErrMsg: err,
			Typ:    common.SyncMsgType_ErrorMsg,
		})
	}
	for {
		select {
		case <-sm.syncCtx.Done():
			return
		default:
			var msg *network2.PipeMsg
			switch sm.mode {
			case common.SyncModeFull:
				msg = sm.blockDataResponsePipe.Receive(sm.syncCtx)
			case common.SyncModeSnapshot:
				msg = sm.chainDataResponsePipe.Receive(sm.syncCtx)
			}
			if msg == nil {
				return
			}

			p2pMsg := &pb.Message{}
			if err := p2pMsg.UnmarshalVT(msg.Data); err != nil {
				sm.logger.Errorf("Unmarshal sync commitData response failed: %s", err)
				continue
			}

			// precheck response
			resp, dataRequester, err := sm.precheckResponse(p2pMsg)
			if err != nil {
				if dataRequester != nil {
					retrySendRequest(dataRequester, err)
				} else {
					sm.logger.Warnf("precheck response failed: %s", err)
				}
				continue
			}

			var commitData common.CommitData
			switch sm.mode {
			case common.SyncModeFull:
				blockResp, ok := resp.(*pb.SyncBlockResponse)
				if !ok {
					retrySendRequest(dataRequester, fmt.Errorf("convert sync block response failed"))
				}

				block := &types.Block{}
				if err = block.Unmarshal(blockResp.GetBlock()); err != nil {
					retrySendRequest(dataRequester, fmt.Errorf("unmarshal block failed: %w", err))
				}
				commitData = &common.BlockData{Block: block}

			case common.SyncModeSnapshot:
				chainResp, ok := resp.(*pb.SyncChainDataResponse)
				if !ok {
					retrySendRequest(dataRequester, fmt.Errorf("convert sync chainData response failed"))
				}
				chainData := &common.ChainData{}
				block := &types.Block{}
				if err = block.Unmarshal(chainResp.GetBlock()); err != nil {
					retrySendRequest(dataRequester, fmt.Errorf("unmarshal block failed: %w", err))
				}
				receipts, err := types.UnmarshalReceipts(chainResp.GetReceipts())
				if err != nil {
					retrySendRequest(dataRequester, fmt.Errorf("unmarshal receipts failed: %w", err))
				}
				chainData.Block = block
				chainData.Receipts = receipts
				commitData = chainData
			}

			sm.addCommitData(dataRequester, commitData, msg.From)
		}
	}
}

func (sm *SyncManager) verifyChunkCheckpoint(checkCommitData common.CommitData) error {
	if sm.chunk.CheckPoint.Digest != checkCommitData.GetHash() {
		return fmt.Errorf("quorum checkpoint is not equal to current hash:[height:%d quorum hash:%s, current hash:%s]",
			sm.chunk.CheckPoint.Height, sm.chunk.CheckPoint.Digest, checkCommitData.GetHash())
	}
	return nil
}

func (sm *SyncManager) addCommitData(req *requester, commitData common.CommitData, from string) {
	req.setCommitData(commitData)
	sm.increaseBlockSize()

	// requester task done, enable assign new requester
	sm.produceRequester(1)

	sm.logger.WithFields(logrus.Fields{
		"height":         commitData.GetHeight(),
		"from":           from,
		"add_commitData": commitData.GetHash(),
		"hash":           req.commitData.GetHash(),
		"size":           sm.recvBlockSize.Load(),
	}).Debug("Receive commitData success")

	if sm.collectChunkTaskDone() {
		sm.logger.WithFields(logrus.Fields{
			"latest commitData": commitData.GetHeight(),
			"hash":              commitData.GetHash(),
			"peer":              from,
		}).Debug("Receive chunk commitData success")
		// send valid chunk task signal
		sm.validChunkTaskCh <- struct{}{}
		sm.logger.Debug("Send valid chunk task signal")
	}
}

func (sm *SyncManager) collectChunkTaskDone() bool {
	if sm.chunk.ChunkSize == 0 {
		return true
	}

	return sm.recvBlockSize.Load() >= int64(sm.chunk.ChunkSize)
}

func (sm *SyncManager) handleSyncStateResp(msg *common.WrapperStateResp, diffState map[string][]string, localHeight uint64,
	localHash string) {
	sm.logger.WithFields(logrus.Fields{
		"peer":      msg.PeerID,
		"height":    msg.Resp.CheckpointState.Height,
		"digest":    msg.Resp.CheckpointState.Digest,
		"diffState": diffState,
	}).Debug("Receive sync state response")

	if sm.stateTaskDone.Load() || localHeight != msg.Resp.CheckpointState.Height {
		sm.logger.WithFields(logrus.Fields{
			"peer":   msg.PeerID,
			"height": msg.Resp.CheckpointState.Height,
			"digest": msg.Resp.CheckpointState.Digest,
		}).Debug("Receive state response after state task done, we ignore it")
		return
	}
	diffState[msg.Hash] = append(diffState[msg.Hash], msg.PeerID)

	// if Quorum state is enough, update Quorum state
	if len(diffState[msg.Hash]) >= int(sm.ensureOneCorrectNum) {
		// verify Quorum state failed, we will quit state with false
		if msg.Resp.CheckpointState.Digest != localHash {
			sm.quitState(fmt.Errorf("quorum state is not equal to current state:[height:%d quorum hash:%s, current hash:%s]",
				msg.Resp.CheckpointState.Height, msg.Resp.CheckpointState.Digest, localHash))
			return
		}

		delete(diffState, msg.Hash)
		// remove Peers which not in Quorum state
		if len(diffState) != 0 {
			wrongPeers := lo.Values(diffState)
			lo.ForEach(lo.Flatten(wrongPeers), func(peer string, _ int) {
				if empty := sm.removePeer(peer); empty {
					sm.logger.Warning("available peer is empty, will reset the Peers")
					sm.resetPeers()
				}
				sm.logger.WithFields(logrus.Fields{
					"peer":   peer,
					"height": msg.Resp.CheckpointState.Height,
				}).Warn("Remove peer which not in Quorum state")
			})
		}

		// For example, if the validator set is 4, and the Quorum is 3:
		// 1. if the current node is forked,
		// 2. validator need send state which obtaining low watermark height commitData,
		// 3. validator have different low watermark height commitData due to network latency,
		// 4. it can lead to state inconsistency, and the node will be stuck in the state sync process.
		sm.logger.Debug("Receive Quorum state from Peers")
		sm.quitState(nil)
	}
}

func (sm *SyncManager) status() bool {
	return sm.syncStatus.Load()
}

func (sm *SyncManager) SwitchMode(newMode common.SyncMode) error {
	if sm.mode == newMode {
		return fmt.Errorf("current mode is same as switch newMode: %s", common.SyncModeMap[sm.mode])
	}
	if sm.status() {
		return fmt.Errorf("switch newMode failed, sync status is %v", sm.status())
	}

	var (
		newConstructor common.ISyncConstructor
	)
	switch newMode {
	case common.SyncModeFull:
		sm.fullValidate = sm.conf.FullValidation
		newConstructor = newModeConstructor(common.SyncModeFull, common.WithContext(sm.ctx))
	case common.SyncModeSnapshot:
		// snapshot mode need not validate block
		sm.fullValidate = false
		newConstructor = newModeConstructor(common.SyncModeSnapshot, common.WithContext(sm.ctx), common.WithLogger(sm.logger), common.WithNetwork(sm.network))
	default:
		return fmt.Errorf("invalid newMode: %d", newMode)
	}
	sm.modeConstructor.Stop()
	sm.mode = newMode
	sm.modeConstructor = newConstructor
	// start listening commitData in new newMode
	sm.modeConstructor.Start()
	return nil
}

func (sm *SyncManager) Prepare(opts ...common.Option) (*common.PrepareData, error) {
	// register message handler
	err := sm.network.RegisterMsgHandler(pb.Message_SYNC_STATE_REQUEST, sm.handleSyncState)
	if err != nil {
		return nil, err
	}
	// for snap sync
	err = sm.network.RegisterMsgHandler(pb.Message_FETCH_EPOCH_STATE_REQUEST, sm.handleFetchEpochState)
	if err != nil {
		return nil, err
	}

	conf := &common.Config{}
	for _, opt := range opts {
		opt(conf)
	}
	return sm.modeConstructor.Prepare(conf)
}

func (sm *SyncManager) Start() {
	if sm.started.Load() {
		return
	}
	// start handle sync block request in full mode
	go sm.listenSyncBlockDataRequest()

	// start handle sync chain data request in snap mode
	go sm.listenSyncChainDataRequest()
	sm.logger.Info("Start listen sync request")

	sm.modeConstructor.Start()
	sm.started.Store(true)
}

func (sm *SyncManager) makeRequesters(height uint64) {
	peerID := sm.pickPeer(height)
	var pipe network.Pipe
	switch sm.mode {
	case common.SyncModeFull:
		pipe = sm.blockRequestPipe
	case common.SyncModeSnapshot:
		pipe = sm.chainDataRequestPipe
	}
	request := newRequester(sm.mode, sm.ctx, peerID, height, sm.recvEventCh, pipe)
	sm.increaseRequester(request, height)
	request.start(sm.conf.RequesterRetryTimeout.ToDuration())
}

func (sm *SyncManager) increaseRequester(r *requester, height uint64) {
	oldR, loaded := sm.requesters.LoadOrStore(height, r)
	if !loaded {
		sm.requesterLen.Add(1)
		requesterNumber.Inc()
	} else {
		sm.logger.WithFields(logrus.Fields{
			"height": height,
		}).Warn("Make requester Error, requester is not nil, we will reset the old requester")
		oldR.(*requester).quitCh <- struct{}{}
	}
}

func (sm *SyncManager) updateStatus() {
	sm.curHeight += sm.chunk.ChunkSize
	if len(sm.epochChanges) != 0 {
		sm.epochChanges = sm.epochChanges[1:]
	}
	sm.initChunk()
	sm.resetBlockSize()

	sm.commitDataCache = make([]common.CommitData, sm.chunk.ChunkSize)
}

// todo: add metrics
func (sm *SyncManager) increaseBlockSize() {
	sm.recvBlockSize.Add(1)
	recvBlockNumber.WithLabelValues(strconv.Itoa(int(sm.chunk.ChunkSize))).Inc()
}

func (sm *SyncManager) decreaseBlockSize() {
	sm.recvBlockSize.Add(-1)
	recvBlockNumber.WithLabelValues(strconv.Itoa(int(sm.chunk.ChunkSize))).Dec()
}

func (sm *SyncManager) resetBlockSize() {
	sm.recvBlockSize.Store(0)
	sm.logger.WithFields(logrus.Fields{
		"blockSize": sm.recvBlockSize.Load(),
	}).Debug("Reset commitData size")
	recvBlockNumber.WithLabelValues(strconv.Itoa(int(sm.chunk.ChunkSize))).Set(0)
}

func (sm *SyncManager) quitState(err error) {
	sm.quitStateCh <- err
	sm.stateTaskDone.Store(true)
}

func (sm *SyncManager) releaseRequester(height uint64) {
	r, loaded := sm.requesters.LoadAndDelete(height)
	if !loaded {
		sm.logger.WithFields(logrus.Fields{
			"height": height,
		}).Warn("Release requester Error, requester is nil")
	} else {
		sm.requesterLen.Add(-1)
		requesterNumber.Dec()
	}
	r.(*requester).quitCh <- struct{}{}
}

func (sm *SyncManager) handleInvalidRequest(msg *common.InvalidMsg) {
	// retry request
	r := sm.getRequester(msg.Height)
	if r == nil {
		sm.logger.Errorf("Retry request commitData Error, requester[height:%d] is nil", msg.Height)
		return
	}
	switch msg.Typ {
	case common.SyncMsgType_ErrorMsg:
		sm.logger.WithFields(logrus.Fields{
			"height": msg.Height,
			"peer":   msg.NodeID,
			"err":    msg.ErrMsg,
		}).Warn("Handle error msg Block")

		invalidBlockNumber.WithLabelValues("send_request_err").Inc()
		newPeer := sm.pickRandomPeer(msg.NodeID)
		r.retryCh <- newPeer

	case common.SyncMsgType_InvalidBlock:
		sm.logger.WithFields(logrus.Fields{
			"height": msg.Height,
			"peer":   msg.NodeID,
		}).Warn("Handle invalid commitData")

		invalidBlockNumber.WithLabelValues("invalid_block").Inc()

		r.clearBlock()
		sm.decreaseBlockSize()

		newPeer := sm.pickRandomPeer(msg.NodeID)
		r.retryCh <- newPeer
	case common.SyncMsgType_TimeoutBlock:
		sm.logger.WithFields(logrus.Fields{
			"height": msg.Height,
			"peer":   msg.NodeID,
		}).Warn("Handle timeout block")

		invalidBlockNumber.WithLabelValues("timeout_response").Inc()

		sm.addPeerTimeoutCount(msg.NodeID)
		newPeer := sm.pickRandomPeer(msg.NodeID)
		r.retryCh <- newPeer
	}
}

func (sm *SyncManager) addPeerTimeoutCount(peerID string) {
	lo.ForEach(sm.peers, func(p *common.Peer, _ int) {
		if p.PeerID == peerID {
			p.TimeoutCount++
			if p.TimeoutCount >= sm.conf.TimeoutCountLimit {
				if empty := sm.removePeer(p.PeerID); empty {
					sm.logger.Warningf("remove peer[id:%d] err: available peer is empty, will reset peer", p.Id)
					sm.resetPeers()
					return
				}
			}
		}
	})
}

func (sm *SyncManager) getRequester(height uint64) *requester {
	r, loaded := sm.requesters.Load(height)
	if !loaded {
		return nil
	}
	return r.(*requester)
}

func (sm *SyncManager) pickPeer(height uint64) string {
	var peerId string
	idx := height % uint64(len(sm.peers))
	for i := 0; i < len(sm.peers); i++ {
		peer := sm.peers[idx]
		if peer.LatestHeight < height {
			idx = (height + uint64(i+1)) % uint64(len(sm.peers))
			sm.logger.WithFields(logrus.Fields{
				"height":                height,
				"illegal latest height": peer.LatestHeight,
				"illegal peer":          peer.Id,
				"new peer":              sm.peers[idx].Id,
			}).Warning("pick peer failed, peer is not exist block")
		} else {
			peerId = sm.peers[idx].PeerID
			break
		}
	}
	if peerId == "" {
		panic(fmt.Errorf("pick peer failed, all peers are not exist block[height:%d]", height))
	}
	return peerId
}

func (sm *SyncManager) pickRandomPeer(exceptPeerId string) string {
	newPeers := lo.Filter(sm.peers, func(p *common.Peer, _ int) bool {
		return p.PeerID != exceptPeerId
	})
	if len(newPeers) == 0 {
		sm.resetPeers()
		newPeers = sm.peers
	}
	return newPeers[rand.Intn(len(newPeers))].PeerID
}

func (sm *SyncManager) removePeer(peerId string) bool {
	var exist bool
	newPeers := lo.Filter(sm.peers, func(p *common.Peer, _ int) bool {
		if p.PeerID == peerId {
			exist = true
		}
		return p.PeerID != peerId
	})
	if !exist {
		sm.logger.WithField("peer", peerId).Warn("Remove peer failed, peer not exist")
		return false
	}

	sm.peers = newPeers
	return len(sm.peers) == 0
}

func (sm *SyncManager) resetPeers() {
	sm.peers = sm.initPeers
}

func (sm *SyncManager) Stop() {
	sm.started.Store(false)
	sm.cancel()
}

func (sm *SyncManager) stopSync() error {
	if err := sm.switchSyncStatus(false); err != nil {
		return err
	}
	sm.syncCancel()
	return nil
}

func (sm *SyncManager) Commit() chan any {
	return sm.modeConstructor.Commit()
}

func (sm *SyncManager) updatePeers(peerId string, latestHeight uint64) {
	lo.ForEach(sm.peers, func(p *common.Peer, _ int) {
		if p.PeerID == peerId {
			if p.LatestHeight > latestHeight {
				sm.logger.WithFields(logrus.Fields{
					"peer":          peerId,
					"pre height":    p.LatestHeight,
					"latest height": latestHeight,
				}).Warn("decrease peer latest height")
			}
			p.LatestHeight = latestHeight
		}
	})
}

func (sm *SyncManager) getConcurrencyLimit() uint64 {
	if sm.conf.ConcurrencyLimit < sm.chunk.ChunkSize {
		return sm.conf.ConcurrencyLimit
	}
	return sm.chunk.ChunkSize
}
