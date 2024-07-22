package data_syncer

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	rbfttypes "github.com/axiomesh/axiom-bft/types"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/components/status"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
)

type Node[T any, Constraint types.TXConstraint[T]] struct {
	selfP2PNodeID            string
	config                   rbft.Config
	chainState               *chainstate.ChainState
	chainConfig              *chainConfig
	consensusHandlers        map[consensus.Type]func(msg *consensus.ConsensusMessage) error
	logger                   logrus.FieldLogger
	batchCache               map[string]*consensus.HashBatch
	msgNonce                 map[consensus.Type]int64
	missingTxsInFetchingLock sync.RWMutex
	missingBatchesInFetching *wrapFetchMissingRequest

	// Set to the highest weak checkpoint cert we have observed
	highStateTarget *stateUpdateTarget

	// It is persisted after updating to epochs
	epochProofCache map[uint64]*consensus.EpochChange

	syncRespStore     map[uint64]*consensus.SyncStateResponse
	stack             rbft.ExternalStack[T, Constraint]
	txpool            txpool.TxPool[T, Constraint]
	missingTxsRespCh  chan *consensus.FetchMissingResponse
	epochChangeRespCh chan *consensus.EpochChangeProof

	timeMgr timer.Timer

	statusMgr *status.StatusMgr

	recvCh chan *localEvent

	checkpointCache *checkpointStore

	lastCommitHeight uint64

	started atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewNode[T any, Constraint types.TXConstraint[T]](rbftConfig rbft.Config, stack rbft.ExternalStack[T, Constraint], chainState *chainstate.ChainState, pool txpool.TxPool[T, Constraint], log logrus.FieldLogger) (*Node[T, Constraint], error) {
	ctx, cancel := context.WithCancel(context.Background())
	node := &Node[T, Constraint]{
		config:            rbftConfig,
		chainState:        chainState,
		selfP2PNodeID:     rbftConfig.SelfP2PNodeID,
		stack:             stack,
		txpool:            pool,
		missingTxsRespCh:  make(chan *consensus.FetchMissingResponse, 1),
		epochChangeRespCh: make(chan *consensus.EpochChangeProof, 1),
		syncRespStore:     make(map[uint64]*consensus.SyncStateResponse),

		logger:          log,
		batchCache:      make(map[string]*consensus.HashBatch),
		msgNonce:        make(map[consensus.Type]int64),
		checkpointCache: newCheckpointQueue(log),
		epochProofCache: make(map[uint64]*consensus.EpochChange),
		chainConfig: &chainConfig{
			epochInfo: &types.EpochInfo{},
		},
		consensusHandlers: make(map[consensus.Type]func(msg *consensus.ConsensusMessage) error),
		recvCh:            make(chan *localEvent, 1024),
		statusMgr:         status.NewStatusMgr(),
		timeMgr:           timer.NewTimerManager(log),

		ctx:    ctx,
		cancel: cancel,
	}

	lo.ForEach(common.DataSyncerRequestName, func(name string, _ int) {
		node.updateMsgNonce(consensus.Type(consensus.Type_value[name]))
	})

	if err := node.initTimer(); err != nil {
		return nil, err
	}

	node.statusMgr.On(Pending)
	node.logger.Infof("new data_syncer node: %s", rbftConfig.SelfP2PNodeID)
	return node, nil
}

func (n *Node[T, Constraint]) initTimer() error {
	if err := n.timeMgr.CreateTimer(syncStateRestart, n.config.SyncStateRestartTimeout, n.handleTimeout); err != nil {
		return err
	}
	if err := n.timeMgr.CreateTimer(syncStateResp, n.config.SyncStateTimeout, n.handleTimeout); err != nil {
		return err
	}
	if err := n.timeMgr.CreateTimer(fetchMissingTxsResp, waitResponseTimeout, n.handleTimeout); err != nil {
		return err
	}
	return nil
}

func (n *Node[T, Constraint]) updateEpochConfig(epochInfo *types.EpochInfo) {
	n.chainConfig.epochInfo = epochInfo.Clone()
}

func (n *Node[T, Constraint]) moveWatermark(height uint64, newEpoch bool) error {
	sckptList := n.checkpointCache.getItems(height)
	if len(sckptList) == 0 {
		return errors.New("no checkpoint to remove")
	}

	removeDigests := n.checkpointCache.moveWatermarks(height)
	n.removeFromBatchCache(removeDigests)
	n.txpool.RemoveBatches(removeDigests)

	n.chainConfig.lock.Lock()
	defer n.chainConfig.lock.Unlock()
	if newEpoch {
		newEpochInfo, err := n.stack.GetCurrentEpochInfo()
		if err != nil {
			return err
		}
		n.updateEpochConfig(newEpochInfo)
	}
	n.chainConfig.H = height

	return nil
}

func (n *Node[T, Constraint]) notifyGenerateBatch(_ int) {
	// do nothing, because we can not generate batch in data_syncer mode
}

func (n *Node[T, Constraint]) notifyFindNextBatch(_ ...string) {
	// do nothing
}

func (n *Node[T, Constraint]) Init() error {
	epochInfo, err := n.stack.GetCurrentEpochInfo()
	if err != nil {
		return err
	}
	n.updateEpochConfig(epochInfo)
	n.setCommitHeight(n.chainState.ChainMeta.Height)
	n.txpool.Init(txpool.ConsensusConfig{
		NotifyGenerateBatchFn: n.notifyGenerateBatch,
		NotifyFindNextBatchFn: n.notifyFindNextBatch,
	})

	// read in the consensus.toml
	n.checkpointCache.size = maxCacheSize
	n.checkpointCache.lastPersistedHeight = n.chainState.ChainMeta.Height
	return nil
}

func (n *Node[T, Constraint]) Start() error {
	go n.listenEvent()
	if err := n.initSyncState(); err != nil {
		n.logger.Errorf("failed to init sync state: %s", err)
		return fmt.Errorf("failed to init sync state: %w", err)
	}
	n.started.Store(true)
	return nil
}

func (n *Node[T, Constraint]) Step(_ context.Context, msg *consensus.ConsensusMessage) {
	if n.statusMgr.In(InEpochSyncing) {
		n.logger.Debugf("Replica %d is in epoch syncing status, reject consensus messages", n.chainState.SelfNodeInfo.ID)
		return
	}
	n.postEvent(&localEvent{EventType: eventType_consensusMessage, Event: msg})
}

func (n *Node[T, Constraint]) ArchiveMode() bool {
	return true
}

func (n *Node[T, Constraint]) Stop() {
	n.timeMgr.Stop()
	n.cancel()
	n.wg.Wait()
	n.logger.Info("data_syncer node stopped")
}

func (n *Node[T, Constraint]) Status() (status rbft.NodeStatus) {
	n.chainConfig.lock.RLock()
	defer n.chainConfig.lock.RUnlock()
	status.H = n.chainConfig.H
	status.EpochInfo = n.chainConfig.epochInfo.Clone()
	switch {
	case n.statusMgr.In(InEpochSyncing):
		status.Status = rbft.InEpochSyncing
	case n.statusMgr.In(StateTransferring):
		status.Status = rbft.StateTransferring
	case n.statusMgr.In(InSyncState):
		status.Status = rbft.InSyncState
	case n.statusMgr.In(Pending):
		status.Status = rbft.Pending
	default:
		status.Status = rbft.Normal
	}
	return
}

func (n *Node[T, Constraint]) getCurrentStatus() status.StatusType {
	switch {
	case n.statusMgr.In(InEpochSyncing):
		return InEpochSyncing
	case n.statusMgr.In(StateTransferring):
		return StateTransferring
	case n.statusMgr.In(InSyncState):
		return InSyncState
	case n.statusMgr.In(InCommit):
		return InCommit
	case n.statusMgr.In(Normal):
		return Normal
	default:
		return Pending
	}
}

func (n *Node[T, Constraint]) GetLowWatermark() uint64 {
	n.chainConfig.lock.RLock()
	defer n.chainConfig.lock.RUnlock()
	return n.chainConfig.H
}

func (n *Node[T, Constraint]) listenEvent() {
	n.wg.Add(1)
	defer n.wg.Done()
	for {
		select {
		case <-n.ctx.Done():
			n.logger.Info("txpool stopped")
			return
		case next := <-n.recvCh:
			nexts := make([]*localEvent, 0)
			for {
				select {
				case <-n.ctx.Done():
					n.logger.Info("txpool stopped")
					return
				default:
				}
				events := n.processEvent(next)
				nexts = append(nexts, events...)
				if len(nexts) == 0 {
					break
				}
				next = nexts[0]
				nexts = nexts[1:]
			}
		}
	}
}

func (n *Node[T, Constraint]) isConsensusMsgWhiteList(msg *consensus.ConsensusMessage) bool {
	switch msg.Type {
	case consensus.Type_PRE_PREPARE:
		return true
	case consensus.Type_SIGNED_CHECKPOINT:
		return true
	default:
		return false
	}
}

func (n *Node[T, Constraint]) consensusMessageFilter(msg *consensus.ConsensusMessage) *localEvent {
	if valid := lo.ContainsBy(common.DataSyncerPipeName, func(item string) bool {
		val, ok := consensus.Type_name[int32(msg.Type)]
		if !ok {
			return false
		}
		return strings.Contains(val, item)
	}); !valid {
		n.logger.Warningf("unknown consensus message type: %s from %d", msg.Type, msg.From)
		return nil
	}

	if n.statusMgr.In(InEpochSyncing) {
		n.logger.Debugf("Replica %d is in epoch syncing status, reject consensus messages", n.chainState.SelfNodeInfo.ID)
		return nil
	}

	if msg.Epoch != n.chainConfig.epochInfo.Epoch && !n.isConsensusMsgWhiteList(msg) {
		return n.checkEpoch(msg)
	}
	start := time.Now()
	next := n.dispatchConsensusMsg(msg)
	traceProcessEvent(fmt.Sprintf("consensusMessage_%s", consensus.Type_name[int32(msg.Type)]), time.Since(start))
	return next
}

// when we are in abnormal or there are some requests in process, we don't need to sync state,
// we only need to sync state when primary is sending null request which means system is in
// normal status and there are no requests in process.
func (n *Node[T, Constraint]) trySyncState() {
	if !n.statusMgr.In(NeedSyncState) {
		n.logger.Infof("Replica %d need to start sync state progress after %v", n.chainState.SelfNodeInfo.ID, n.config.SyncStateRestartTimeout)
		if err := n.timeMgr.RestartTimer(syncStateRestart); err != nil {
			n.logger.Errorf("failed to restart timer: %s", err)
			return
		}
		n.statusMgr.On(NeedSyncState)
	}
}

func (n *Node[T, Constraint]) existSyncState() {
	n.statusMgr.Off(NeedSyncState)
	n.timeMgr.StopTimer(syncStateRestart)
}

func (n *Node[T, Constraint]) dispatchConsensusMsg(msg *consensus.ConsensusMessage) *localEvent {
	var err error
	defer func() {
		if err != nil {
			n.logger.Errorf("failed to handle consensus message: %s", err)
		}
	}()
	switch msg.Type {
	case consensus.Type_NULL_REQUEST:
		if n.statusMgr.InOne(StateTransferring, InSyncState) {
			n.logger.Debugf("Replica %d is in syncing status, ignore it...", n.chainState.SelfNodeInfo.ID)
			return nil
		}
		n.trySyncState()
	case consensus.Type_PRE_PREPARE:
		if err = n.handlePrePrepare(msg); err != nil {
			return nil
		}
	case consensus.Type_SIGNED_CHECKPOINT:
		sckpt := &consensus.SignedCheckpoint{}
		if err = sckpt.UnmarshalVT(msg.Payload); err != nil {
			return nil
		}
		return n.handleSignedCheckpoint(sckpt, msg.Epoch)

	case consensus.Type_SYNC_STATE_RESPONSE:
		stateResp := &consensus.SyncStateResponse{}
		if err = stateResp.UnmarshalVT(msg.Payload); err != nil {
			return nil
		}
		return n.recvSyncStateResponse(stateResp)

	case consensus.Type_FETCH_MISSING_RESPONSE:
		resp := &consensus.FetchMissingResponse{}
		if err = resp.UnmarshalVT(msg.Payload); err != nil {
			return nil
		}
		n.timeMgr.StopTimer(fetchMissingTxsResp)
		return n.recvFetchMissingResponse(resp)
	}
	return nil
}

func (n *Node[T, Constraint]) processEvent(event *localEvent) []*localEvent {
	nextEvent := make([]*localEvent, 0)
	start := time.Now()
	if event.EventType != eventType_consensusMessage {
		n.logger.Debugf("receive event: %s", eventTypes[event.EventType])
	}
	defer func() {
		traceProcessEvent(eventTypes[event.EventType], time.Since(start))
		if event.EventType != eventType_consensusMessage {
			n.logger.Debugf("end process event: %s, cost: %s", eventTypes[event.EventType], time.Since(start))
		}
	}()
	switch event.EventType {
	case eventType_consensusMessage:
		ev, ok := event.Event.(*consensus.ConsensusMessage)
		if !ok {
			n.logger.Errorf("invalid event type: %v", event.Event)
			return nil
		}
		next := n.consensusMessageFilter(ev)
		if next != nil {
			nextEvent = append(nextEvent, next)
		}
		if ev.Type != consensus.Type_NULL_REQUEST {
			n.logger.Debugf("process consensus message: %s from %d, cost: %s", consensus.Type_name[int32(ev.Type)], ev.From, time.Since(start))
		}
	case eventType_commitToExecutor:
		if n.statusMgr.InOne(StateTransferring, InSyncState, InCommit) {
			n.logger.Infof("current status is %s, ignore commit event", statusTypes[n.getCurrentStatus()])
			return nil
		}

		if n.statusMgr.In(NeedFetchMissingTxs) {
			n.missingTxsInFetchingLock.RLock()
			missing := n.missingBatchesInFetching
			n.missingTxsInFetchingLock.RUnlock()
			if missing != nil {
				n.logger.Infof("Replica %d is in fetching missing txs, ignore commit event", n.chainState.SelfNodeInfo.ID)
				return nil
			}
			n.statusMgr.Off(NeedFetchMissingTxs)
		}

		n.logger.Infof("commit to executor, height: %d", n.lastCommitHeight+1)
		n.statusMgr.On(InCommit)
		next, sckptList, err := n.findNextCommit()
		if err != nil {
			n.logger.Error(err)
			n.statusMgr.Off(InCommit)
			nextEvent = append(nextEvent, n.genSyncBlockEvent(sckptList))
			return nextEvent
		}

		if next != nil {
			n.existSyncState()
			n.logger.Debugf("commit height: %d, proposer %d", next.height, next.proposer)
			n.stack.Execute(next.txs, next.localList, next.height, next.timestamp, next.proposer)
			n.increaseCommitHeight()
		} else {
			n.statusMgr.Off(InCommit)
		}

		n.missingTxsInFetchingLock.RLock()
		if n.missingBatchesInFetching != nil {
			n.logger.Infof("found missingBatchesInFetching, start fetch missing txs in height:%d", n.missingBatchesInFetching.request.SequenceNumber)
			n.statusMgr.On(NeedFetchMissingTxs)
		}
		n.missingTxsInFetchingLock.RUnlock()

	case eventType_syncBlock:
		ckptList, ok := event.Event.([]*consensus.SignedCheckpoint)
		if !ok {
			n.logger.Errorf("invalid event type: %v", event.Event)
			return nil
		}
		targetHeight := ckptList[0].GetCheckpoint().GetExecuteState().GetHeight()
		targetDigest := ckptList[0].GetCheckpoint().GetExecuteState().GetDigest()
		targetBatchDigest := ckptList[0].GetCheckpoint().GetExecuteState().GetBatchDigest()

		n.updateHighStateTarget(&rbfttypes.MetaState{Height: targetHeight, Digest: targetDigest, BatchDigest: targetBatchDigest}, ckptList)
		n.tryStateTransfer()

	case eventType_epochSync:
		proof, ok := event.Event.(*consensus.EpochChangeProof)
		if !ok {
			n.logger.Errorf("invalid event type: %v", event.Event)
			return nil
		}
		if n.statusMgr.In(InEpochSyncing) {
			n.logger.Info("epoch syncing in progress, ignore epoch sync event")
			return nil
		}
		n.recvEpochChangeProof(proof)

	case eventType_executed:
		state, ok := event.Event.(*rbfttypes.ServiceState)
		if !ok {
			n.logger.Errorf("invalid event type: %v", event.Event)
			return nil
		}
		if n.statusMgr.In(Pending) {
			n.logger.Infof("current status is %s, ignore executed event", statusTypes[n.getCurrentStatus()])
			return nil
		}
		sckptList := n.checkpointCache.getItems(state.MetaState.Height)
		if sckptList == nil {
			n.logger.Debugf("checkpoint not found: %d", state.MetaState.Height)
			return nil
		}
		if sckptList[0].GetCheckpoint().GetExecuteState().GetDigest() != state.MetaState.Digest {
			err := fmt.Errorf("execute state digest mismatch in height %d: local: %s, quorum: %s", state.MetaState.Height, state.MetaState.Digest, sckptList[0].GetCheckpoint().GetExecuteState().GetDigest())
			n.logger.Error(err)
			panic(err)
		}
		var updateEpoch bool
		if state.Epoch != n.chainConfig.epochInfo.Epoch {
			updateEpoch = true
		}
		if err := n.moveWatermark(state.MetaState.Height, updateEpoch); err != nil {
			n.logger.Errorf("move watermark failed: %s", err)
			return nil
		}

		n.statusMgr.Off(InCommit)
		n.logger.Debugf("execute block %d succeed", state.MetaState.Height)
		// checkpointCache may be not commit before update epoch
		nextEvent = append(nextEvent, n.genCommitEvent())

	case eventType_stateUpdated:
		state, ok := event.Event.(*rbfttypes.ServiceSyncState)
		if !ok {
			n.logger.Errorf("invalid event type: %v", event)
			return nil
		}
		if ev := n.recvStateUpdatedEvent(state); ev != nil {
			nextEvent = append(nextEvent, ev)
		}
	}
	return nextEvent
}

func (n *Node[T, Constraint]) recvStateUpdatedEvent(state *rbfttypes.ServiceSyncState) *localEvent {
	if !n.statusMgr.In(StateTransferring) {
		n.logger.Errorf("node is not in state transferring, ignore state updated event")
		return nil
	}

	if n.highStateTarget == nil {
		n.logger.Errorf("Replica %d has no state targets, cannot resume tryStateTransfer yet", n.chainState.SelfNodeInfo.ID)
		return nil
	}

	if state.MetaState.Height > n.highStateTarget.metaState.Height {
		n.logger.Errorf("Replica %d recovered to seqNo %d which is higher than high-target %d",
			n.chainState.SelfNodeInfo.ID, state.MetaState.Height, n.highStateTarget.metaState.Height)
		return nil
	}

	sckptList := n.checkpointCache.getItems(state.MetaState.Height)
	if sckptList == nil {
		n.logger.Debugf("checkpoint not found: %d", state.MetaState.Height)
		return nil
	}
	if sckptList[0].GetCheckpoint().GetExecuteState().GetDigest() != state.MetaState.Digest {
		err := fmt.Errorf("execute state digest mismatch in height %d: local: %s, quorum: %s", state.MetaState.Height, sckptList[0].GetCheckpoint().GetExecuteState().GetDigest(), state.MetaState.Digest)
		n.logger.Error(err)
		panic(err)
	}

	if state.MetaState.Height < n.highStateTarget.metaState.Height {
		if !state.EpochChanged {
			// If state transfer did not complete successfully, or if it did not reach the highest target, try again.
			n.logger.Warningf("Replica %d recovered to seqNo %d but our high-target has moved to %d, "+
				"keep on state transferring", n.chainState.SelfNodeInfo.ID, state.MetaState.Height, n.highStateTarget.metaState.Height)
			n.setCommitHeight(n.highStateTarget.metaState.Height)
			n.statusMgr.Off(StateTransferring)
			n.tryStateTransfer()
		} else {
			n.logger.Debugf("Replica %d state updated, lastExec = %d, seqNo = %d, accept epoch proof for %d", n.chainState.SelfNodeInfo.ID, n.lastCommitHeight, state.MetaState.Height, state.Epoch-1)
			if ec, ok := n.epochProofCache[state.Epoch-1]; ok {
				n.persistEpochQuorumCheckpoint(ec.GetCheckpoint())
			}
		}
		return nil
	}

	n.logger.Debugf("Replica %d state updated, lastExec = %d, seqNo = %d", n.chainState.SelfNodeInfo.ID, n.lastCommitHeight, state.MetaState.Height)
	epochChanged := state.Epoch != n.chainConfig.epochInfo.Epoch
	if state.EpochChanged || epochChanged {
		n.logger.Debugf("Replica %d accept epoch proof for %d", n.chainState.SelfNodeInfo.ID, state.Epoch-1)
		if ec, ok := n.epochProofCache[state.Epoch-1]; ok {
			n.persistEpochQuorumCheckpoint(ec.GetCheckpoint())
		}
	}

	// finished state update
	n.logger.Infof("======== Replica %d finished stateUpdate, height: %d", n.chainState.SelfNodeInfo.ID, state.MetaState.Height)
	n.setCommitHeight(state.MetaState.Height)
	n.missingTxsInFetchingLock.Lock()
	n.missingBatchesInFetching = nil
	n.missingTxsInFetchingLock.Unlock()

	err := n.moveWatermark(state.MetaState.Height, epochChanged)
	if err != nil {
		n.logger.Errorf("move watermark failed: %s", err)
		return nil
	}

	// process epochInfo
	if epochChanged {
		n.logger.Infof("epoch changed to %d", state.Epoch)
		newEpoch, err := n.stack.GetCurrentEpochInfo()
		if err != nil {
			n.logger.Errorf("Replica %d failed to get current epoch from ledger: %v", n.chainState.SelfNodeInfo.ID, err)
			return nil
		}

		// clean cached old epoch proof
		for epoch := range n.epochProofCache {
			if epoch < newEpoch.Epoch {
				delete(n.epochProofCache, epoch)
			}
		}

		n.logger.Info(`
  +==============================================+
  |                                              |
  |             RBFT Start New Epoch             |
  |                                              |
  +==============================================+

`)
		n.statusMgr.Off(InEpochSyncing)
	}

	n.statusMgr.Off(StateTransferring)
	n.logger.Infof("receive state updated event, finished move watermark to height:%d", state.MetaState.Height)

	// if local has no commit event, start syncing state
	if n.checkpointCache.ready.Len() == 0 {
		n.statusMgr.On(InSyncState)
		if err = n.fetchSyncState(); err != nil {
			n.logger.Errorf("sync state failed: %s", err)
			return nil
		}
	} else {
		n.statusMgr.Off(Pending)
		n.statusMgr.On(Normal)
		return n.genCommitEvent()
	}
	return nil
}

func (n *Node[T, Constraint]) checkEpoch(msg *consensus.ConsensusMessage) *localEvent {
	currentEpoch := n.chainConfig.epochInfo.Epoch
	remoteEpoch := msg.Epoch
	if remoteEpoch > currentEpoch {
		n.logger.Debugf("Replica %d received message type %s from %d with larger epoch, "+
			"current epoch %d, remote epoch %d", n.chainState.SelfNodeInfo.ID, consensus.Type_name[int32(msg.Type)], msg.From, currentEpoch, remoteEpoch)
		// first process epoch sync response with higher epoch.
		if msg.Type == consensus.Type_EPOCH_CHANGE_PROOF {
			proof := &consensus.EpochChangeProof{}
			if uErr := proof.UnmarshalVT(msg.Payload); uErr != nil {
				n.logger.Warningf("Unmarshal EpochChangeProof failed: %s", uErr)
				return nil
			}

			return n.processEpochChangeProof(proof)
		}
		req := &consensus.EpochChangeRequest{
			Author:          n.chainState.SelfNodeInfo.ID,
			StartEpoch:      currentEpoch,
			TargetEpoch:     remoteEpoch,
			AuthorP2PNodeId: n.selfP2PNodeID,
		}
		if err := n.fetchEpochChangeProof(req, msg.From); err != nil {
			n.logger.Errorf("fetch epoch change proof failed: %s", err)
			return nil
		}
	} else {
		n.logger.WithFields(logrus.Fields{"msgType": consensus.Type_name[int32(msg.Type)], "from": msg.From, ",currentEpoch": currentEpoch, "remoteEpoch": remoteEpoch}).Debugf("Replica %d received message with lower epoch, just ignore it...", n.chainState.SelfNodeInfo.ID)
	}

	return nil
}

func (n *Node[T, Constraint]) processEpochChangeProof(proof *consensus.EpochChangeProof) *localEvent {
	n.logger.Debugf("Replica %d received epoch change proof from %d", n.chainState.SelfNodeInfo.ID, proof.Author)

	if changeTo := proof.NextEpoch(); changeTo <= n.chainConfig.epochInfo.Epoch {
		// ignore proof old epoch which we have already started
		n.logger.Debugf("reject lower epoch change to %d", changeTo)
		return nil
	}

	if proof.GenesisBlockDigest != n.config.GenesisBlockDigest {
		n.logger.Warningf("Replica %d reject epoch change proof, because self genesis chainConfig is not consistent with most nodes, expected genesis block hash: %s, self genesis block hash: %s",
			n.chainState.SelfNodeInfo.ID, proof.GenesisBlockDigest, n.config.GenesisBlockDigest)
		return nil
	}

	// 1.Verify epoch-change-proof
	err := n.verifyEpochChangeProof(proof)
	if err != nil {
		n.logger.Errorf("failed to verify epoch change proof: %s", err)
		return nil
	}

	return &localEvent{EventType: eventType_epochSync, Event: proof}
}

// ReportExecuted reports to RBFT core that application service has finished height one batch with
// current height batch seqNo and state digest.
// Users can report any necessary extra field optionally.
// NOTE. Users should ReportExecuted directly after start node to help track the initial state.
func (n *Node[T, Constraint]) ReportExecuted(state *rbfttypes.ServiceState) {
	ev := &localEvent{
		EventType: eventType_executed,
		Event:     state,
	}
	n.postEvent(ev)
}

// ReportStateUpdated reports to RBFT core that application service has finished one desired StateUpdate
// request which was triggered by RBFT core before.
// Users must ReportStateUpdated after RBFT core invoked StateUpdate request no matter this request was
// finished successfully or not, otherwise, RBFT core will enter abnormal status infinitely.
func (n *Node[T, Constraint]) ReportStateUpdated(state *rbfttypes.ServiceSyncState) {
	ev := &localEvent{
		EventType: eventType_stateUpdated,
		Event:     state,
	}
	n.postEvent(ev)
}

func (n *Node[T, Constraint]) initSyncState() error {
	if n.statusMgr.In(InSyncState) {
		n.logger.Warningf("Replica %d try to send syncStateRestart, but it's already in sync state", n.chainState.SelfNodeInfo.ID)
		return nil
	}
	n.statusMgr.On(InSyncState)

	n.logger.Infof("Replica %d now init sync state", n.chainState.SelfNodeInfo.ID)

	return n.fetchSyncState()
}

func (n *Node[T, Constraint]) fetchSyncState() error {
	n.logger.Infof("Replica %d start fetch sync state", n.chainState.SelfNodeInfo.ID)
	// broadcast sync state message to others.
	syncStateMsg := &consensus.SyncState{
		AuthorP2PNodeId: n.chainState.SelfNodeInfo.P2PID,
	}
	payload, err := syncStateMsg.MarshalVTStrict()
	if err != nil {
		n.logger.Errorf("ConsensusMessage_SYNC_STATE marshal error: %v", err)
		return err
	}
	msgNonce := n.getMsgNonce(consensus.Type_SYNC_STATE)
	msg := &consensus.ConsensusMessage{
		Type:    consensus.Type_SYNC_STATE,
		From:    n.chainState.SelfNodeInfo.ID,
		Epoch:   n.chainConfig.epochInfo.Epoch,
		Payload: payload,
		Nonce:   msgNonce,
	}

	if err = n.stack.Broadcast(context.TODO(), msg); err != nil {
		return err
	}
	n.updateMsgNonce(consensus.Type_SYNC_STATE)

	return n.timeMgr.RestartTimer(syncStateResp)
}

func (n *Node[T, Constraint]) recvSyncStateResponse(resp *consensus.SyncStateResponse) *localEvent {
	if !n.statusMgr.In(InSyncState) {
		n.logger.Debugf("Replica %d is not in sync state, ignore it...", n.chainState.SelfNodeInfo.ID)
		return nil
	}

	if resp.GetSignedCheckpoint() == nil || resp.GetSignedCheckpoint().GetCheckpoint() == nil {
		n.logger.Errorf("Replica %d reject sync state response with nil checkpoint info", n.chainState.SelfNodeInfo.ID)
		return nil
	}

	// verify signature of remote checkpoint.
	if err := n.verifySignedCheckpoint(resp.GetSignedCheckpoint()); err != nil {
		n.logger.Errorf("Replica %d verify signature of checkpoint from %d error: %s", n.chainState.SelfNodeInfo.ID, resp.ReplicaId, err)
		return nil
	}

	n.logger.Debugf("Replica %d now received sync state response from replica %d: view=%d, checkpoint=%s",
		n.chainState.SelfNodeInfo.ID, resp.ReplicaId, resp.View, resp.GetSignedCheckpoint().GetCheckpoint().Pretty())

	if oldRsp, ok := n.syncRespStore[resp.ReplicaId]; ok {
		if oldRsp.GetSignedCheckpoint().GetCheckpoint().Height() > resp.GetSignedCheckpoint().Height() {
			n.logger.Debugf("Duplicate sync state response, new height=%d is lower than old height=%d, reject it",
				resp.GetSignedCheckpoint().Height(), oldRsp.GetSignedCheckpoint().GetCheckpoint().Height())
			return nil
		}
	}
	n.syncRespStore[resp.ReplicaId] = resp
	var (
		findQuorum  bool
		sckptList   []*consensus.SignedCheckpoint
		quorumState nodeState
	)

	if n.reachQuorum(len(n.syncRespStore)) {
		states := make(wholeStates)
		for _, response := range n.syncRespStore {
			states[response.GetSignedCheckpoint()] = nodeState{
				height: response.GetSignedCheckpoint().GetCheckpoint().Height(),
				digest: response.GetSignedCheckpoint().GetCheckpoint().Digest(),
			}
			quorumState, sckptList, findQuorum = n.compareWholeStates(states)
			if findQuorum {
				break
			}

		}

		if !findQuorum {
			return nil
		}

		// if remote checkpoint is higher than local, generate sync block event
		if quorumState.height > n.lastCommitHeight {
			n.finishQuorumStateResp()
			return n.genSyncBlockEvent(sckptList)
		}

		if meta := n.chainState.ChainMeta; meta.Height == quorumState.height {
			if meta.BlockHash.String() != quorumState.digest {
				panic(fmt.Errorf("local block[height:%d] hash %s not equal to checkpoint digest %s", meta.Height, meta.BlockHash.String(), quorumState.digest))
			}
		}

		n.statusMgr.Off(Pending)
		if !n.statusMgr.In(Normal) {
			n.statusMgr.On(Normal)
		}
		n.finishQuorumStateResp()
		n.logger.Infof("Replica %d has reached state", n.chainState.SelfNodeInfo.ID)
		if !n.statusMgr.In(InCommit) && n.checkpointCache.ready.Len() > 0 {
			return n.genCommitEvent()
		}
	}
	return nil
}

func (n *Node[T, Constraint]) finishQuorumStateResp() {
	n.timeMgr.StopTimer(syncStateResp)
	n.statusMgr.Off(InSyncState)
	if err := n.timeMgr.RestartTimer(syncStateRestart); err != nil {
		n.logger.Errorf("restart sync state timer failed: %s", err)
	}
	// clean sync state response
	n.syncRespStore = make(map[uint64]*consensus.SyncStateResponse)
}

func (n *Node[T, Constraint]) fetchMissingTxs(fetch *consensus.FetchMissingRequest, proposer uint64) error {
	payload, err := fetch.MarshalVTStrict()
	if err != nil {
		n.logger.Errorf("ConsensusMessage_FetchMissingRequest Marshal Error: %s", err)
		return err
	}
	msgNonce := n.getMsgNonce(consensus.Type_FETCH_MISSING_REQUEST)
	consensusMsg := &consensus.ConsensusMessage{
		Type:    consensus.Type_FETCH_MISSING_REQUEST,
		Payload: payload,
		Epoch:   n.chainConfig.epochInfo.Epoch,
		Nonce:   msgNonce,
		From:    n.chainState.SelfNodeInfo.ID,
	}
	n.missingTxsInFetchingLock.Lock()
	if n.missingBatchesInFetching == nil {
		n.logger.Debugf("Replica %d send fetchMissingRequest to %d", n.chainState.SelfNodeInfo.ID, proposer)
		n.missingBatchesInFetching = &wrapFetchMissingRequest{
			request:  fetch,
			proposer: proposer,
		}
	}
	n.missingTxsInFetchingLock.Unlock()

	to, err := n.chainState.GetNodeInfo(proposer)
	if err != nil {
		return errors.Wrapf(err, "replica %d not in nodeIds chainConfig", proposer)
	}
	if err = n.stack.Unicast(context.TODO(), consensusMsg, to.P2PID); err != nil {
		return err
	}
	n.updateMsgNonce(consensus.Type_FETCH_MISSING_REQUEST)
	return nil
}

func (n *Node[T, Constraint]) recvFetchMissingResponse(resp *consensus.FetchMissingResponse) *localEvent {
	n.timeMgr.StopTimer(fetchMissingTxsResp)
	defer func() {
		n.missingTxsInFetchingLock.Lock()
		n.missingBatchesInFetching = nil
		n.missingTxsInFetchingLock.Unlock()
	}()

	n.missingTxsInFetchingLock.RLock()
	request := n.missingBatchesInFetching
	n.missingTxsInFetchingLock.RUnlock()
	if request == nil {
		n.logger.Debugf("Replica %d ignore fetchMissingResponse with batch hash %s", n.chainState.SelfNodeInfo.ID, resp.BatchDigest)
		return nil
	}
	requests, err := n.checkFetchMissingResponse(resp, request)
	if err != nil {
		n.logger.Warningf("Replica %d fetchMissingResponse from node %d failed, need sync state, err: %v", n.chainState.SelfNodeInfo.ID, request.proposer, err)
		if err = n.initSyncState(); err != nil {
			n.logger.Errorf("init sync state failed: %s", err)
			return nil
		}
		return nil
	}

	if err = n.txpool.ReceiveMissingRequests(resp.BatchDigest, requests); err != nil {
		n.logger.Warningf("Replica %d find something wrong with fetchMissingResponse, error: %v", n.chainState.SelfNodeInfo.ID, err)
		// there is something wrong with primary for it propose a transaction with mismatched hash,
		// so that we should send sync state request directly to expect the new checkpoint.
		err = n.initSyncState()
		if err != nil {
			n.logger.Errorf("init sync state failed: %s", err)
			return nil
		}
		return nil
	}

	n.logger.Infof("Replica %d received fetchMissingResponse for view=%d/seqNo=%d/digest=%s", n.chainState.SelfNodeInfo.ID, resp.View, resp.SequenceNumber, resp.BatchDigest)
	return n.genCommitEvent()
}

func (n *Node[T, Constraint]) checkFetchMissingResponse(resp *consensus.FetchMissingResponse, request *wrapFetchMissingRequest) (map[uint64]*T, error) {
	if resp.GetStatus() != consensus.FetchMissingResponse_Success {
		return nil, fmt.Errorf("replica %d received fetchMissingResponse with failed status", n.chainState.SelfNodeInfo.ID)
	}

	if resp.ReplicaId != request.proposer {
		return nil, fmt.Errorf("replica %d received fetchMissingResponse from replica %d which is not "+
			"primary, ignore it", n.chainState.SelfNodeInfo.ID, resp.ReplicaId)
	}
	if resp.SequenceNumber <= n.lastCommitHeight {
		return nil, fmt.Errorf("replica %d ignore fetchMissingResponse with lower seqNo %d than "+
			"lastCommitHeight %d", n.chainState.SelfNodeInfo.ID, resp.SequenceNumber, n.lastCommitHeight)
	}

	if len(resp.MissingRequests) != len(resp.MissingRequestHashes) {
		return nil, fmt.Errorf("replica %d received mismatch length fetchMissingResponse %v", n.chainState.SelfNodeInfo.ID, resp)
	}

	requests := make(map[uint64]*T)
	for i, reqRaw := range resp.MissingRequests {
		var req T
		if err := Constraint(&req).RbftUnmarshal(reqRaw); err != nil {
			return nil, fmt.Errorf("tx unmarshal Error: %s", err)
		}
		requests[i] = &req
	}
	return requests, nil
}

func (n *Node[T, Constraint]) fetchEpochChangeProof(req *consensus.EpochChangeRequest, remoteId uint64) error {
	n.logger.Infof("Replica %d request epoch changes %d to %d from %d", n.chainState.SelfNodeInfo.ID, req.StartEpoch, req.TargetEpoch, req.Author)

	payload, mErr := req.MarshalVTStrict()
	if mErr != nil {
		n.logger.Warningf("Marshal EpochChangeRequest failed: %s", mErr)
		return mErr
	}

	msgNonce := n.getMsgNonce(consensus.Type_EPOCH_CHANGE_REQUEST)
	cum := &consensus.ConsensusMessage{
		Type:    consensus.Type_EPOCH_CHANGE_REQUEST,
		Payload: payload,
		Epoch:   n.chainConfig.epochInfo.Epoch,
		Nonce:   msgNonce,
		From:    n.chainState.SelfNodeInfo.ID,
	}

	to, err := n.chainState.GetNodeInfo(remoteId)
	if err != nil {
		return errors.Wrapf(err, "replica %d not in nodeIds chainConfig", remoteId)
	}
	err = n.stack.Unicast(context.TODO(), cum, to.P2PID)
	if err != nil {
		return err
	}
	n.updateMsgNonce(consensus.Type_EPOCH_CHANGE_REQUEST)
	return nil
}

func (n *Node[T, Constraint]) verifyEpochChangeProof(proof *consensus.EpochChangeProof) error {
	if changeTo := proof.NextEpoch(); changeTo <= n.chainConfig.epochInfo.Epoch {
		// ignore proof old epoch which we have already started
		return fmt.Errorf("replica %d ignore old epoch proof %d", n.chainState.SelfNodeInfo.ID, changeTo)
	}
	// Skip any stale checkpoints in the proof prefix. Note that with
	// the assertion above, we are guaranteed there is at least one
	// non-stale checkpoints in the proof.
	//
	// It's useful to skip these stale checkpoints to better allow for
	// concurrent node requests.
	//
	// For example, suppose the following:
	//
	// 1. My current trusted state is at epoch 5.
	// 2. I make two concurrent requests to two validators A and B, who
	//    live at epochs 9 and 11 respectively.
	//
	// If A's response returns first, I will ratchet my trusted state
	// to epoch 9. When B's response returns, I will still be able to
	// ratchet forward to 11 even though B's EpochChangeProof
	// includes a bunch of stale checkpoints (for epochs 5, 6, 7, 8).
	//
	// Of course, if B's response returns first, we will reject A's
	// response as it's completely stale.
	var (
		skip       int
		startEpoch uint64
	)
	for _, epc := range proof.EpochChanges {
		if epc.GetCheckpoint().Epoch() >= n.chainConfig.epochInfo.Epoch {
			startEpoch = epc.GetCheckpoint().Epoch()
			break
		}
		skip++
	}
	if startEpoch != n.chainConfig.epochInfo.Epoch {
		return fmt.Errorf("invalid epoch change proof with start epoch %d, "+
			"current epoch %d", startEpoch, n.chainConfig.epochInfo.Epoch)
	}
	// skip smaller epoch
	proof.EpochChanges = proof.EpochChanges[skip:]

	if proof.IsEmpty() {
		return errors.New("empty epoch change proof")
	}
	return nil
}

func (n *Node[T, Constraint]) recvEpochChangeProof(proof *consensus.EpochChangeProof) {
	quorumCheckpoint := proof.Last().Checkpoint

	if quorumCheckpoint.Height() <= n.lastCommitHeight {
		n.logger.Warningf("Replica %d ignore handle epoch change proof at height %d, because node had already committed at height %d",
			n.chainState.SelfNodeInfo.ID, quorumCheckpoint.Height(), n.lastCommitHeight)
		return
	}

	var checkpointSet []*consensus.SignedCheckpoint
	for id, sig := range quorumCheckpoint.Signatures {
		signedCheckpoint := &consensus.SignedCheckpoint{
			Checkpoint: quorumCheckpoint.Checkpoint,
			Signature:  sig,
			Author:     id,
		}
		if err := n.verifySignedCheckpoint(signedCheckpoint); err != nil {
			n.logger.Errorf("Replica %d verify checkpoint error: %s", n.chainState.SelfNodeInfo.ID, err)
			return
		}
		checkpointSet = append(checkpointSet, signedCheckpoint)
	}

	n.statusMgr.On(InEpochSyncing)
	for _, ec := range proof.GetEpochChanges() {
		n.epochProofCache[ec.Checkpoint.Epoch()] = ec
	}
	n.logger.Infof("Replica %d try epoch sync to height %d, epoch %d", n.chainState.SelfNodeInfo.ID,
		quorumCheckpoint.Height(), quorumCheckpoint.NextEpoch())

	target := &rbfttypes.MetaState{
		Height: quorumCheckpoint.Height(),
		Digest: quorumCheckpoint.Digest(),
	}
	n.updateHighStateTarget(target, checkpointSet, proof.GetEpochChanges()...) // for new epoch
	n.tryStateTransfer()
}

func (n *Node[T, Constraint]) tryStateTransfer() {
	if n.statusMgr.In(StateTransferring) {
		n.logger.Debugf("Replica %d is currently mid tryStateTransfer, it must wait for this "+
			"tryStateTransfer to complete before initiating a new one", n.chainState.SelfNodeInfo.ID)
		return
	}
	// if high state target is nil, we could not state update
	if n.highStateTarget == nil {
		n.logger.Debugf("Replica %d has no targets to attempt tryStateTransfer to, delaying", n.chainState.SelfNodeInfo.ID)
		return
	}
	n.statusMgr.On(StateTransferring)
	target := n.highStateTarget
	n.logger.Infof("Replica %d try state update to %d", n.chainState.SelfNodeInfo.ID, target.metaState.Height)
	go n.stack.StateUpdate(n.checkpointCache.lastPersistedHeight, target.metaState.Height, target.metaState.Digest, target.checkpointSet, target.epochChanges...)
}

func (n *Node[T, Constraint]) genSyncBlockEvent(sckptList []*consensus.SignedCheckpoint) *localEvent {
	return &localEvent{
		EventType: eventType_syncBlock,
		Event:     sckptList,
	}
}

func (n *Node[T, Constraint]) genCommitEvent() *localEvent {
	return &localEvent{
		EventType: eventType_commitToExecutor,
	}
}

func (n *Node[T, Constraint]) handlePrePrepare(msg *consensus.ConsensusMessage) error {
	if len(n.batchCache) > maxCacheSize {
		n.logger.Warningf("batch cache size %d exceeds max size %d, ignoring it", len(n.batchCache), maxCacheSize)
		return nil
	}

	prePrepare := &consensus.PrePrepare{}

	if err := prePrepare.UnmarshalVT(msg.Payload); err != nil {
		return err
	}

	n.setBachCache(prePrepare.BatchDigest, prePrepare.HashBatch)
	return nil
}

func (n *Node[T, Constraint]) verifySignedCheckpoint(sckpt *consensus.SignedCheckpoint) error {
	return n.stack.Verify(sckpt.GetAuthor(), sckpt.GetSignature(), sckpt.GetCheckpoint().Hash())
}

func (n *Node[T, Constraint]) handleSignedCheckpoint(sckpt *consensus.SignedCheckpoint, remoteEpoch uint64) *localEvent {
	// 1. verify checkpoint
	if err := n.verifySignedCheckpoint(sckpt); err != nil {
		n.logger.Errorf("checkpoint verify failed: %v", err)
		return nil
	}

	// 2. update checkpoint cache
	recvNum := n.checkpointCache.insert(sckpt, false)
	// if reach quorum, notify node to handle quorum checkpoint
	if n.reachQuorum(recvNum) {
		n.logger.Debugf("checkpoint reach quorum, height: %d", sckpt.Height())
		n.checkpointCache.insertReady(sckpt.Height())
		if currentEpoch := n.chainConfig.epochInfo.Epoch; currentEpoch != remoteEpoch {
			n.logger.Warningf("current local epoch %d is not reached %d, replica should wait for next epoch updated", currentEpoch, remoteEpoch)
			return nil
		}
		if !n.statusMgr.In(InCommit) && n.statusMgr.In(Normal) {
			return n.genCommitEvent()
		}
	}

	return nil
}

func (n *Node[T, Constraint]) findNextCommit() (*readyExecute[T, Constraint], []*consensus.SignedCheckpoint, error) {
	height := n.checkpointCache.ready.PopItem()
	// if not found ready checkpoint, return nil
	if height == math.MaxUint64 {
		return nil, nil, nil
	}
	n.chainConfig.lock.RLock()
	defer n.chainConfig.lock.RUnlock()
	if common.NeedChangeEpoch(height-1, *n.chainConfig.epochInfo) {
		n.checkpointCache.insertReady(height)
		n.logger.Warningf("replica %d not reach next epoch: %d, ignore commit height: %d", n.chainState.SelfNodeInfo.ID, n.chainConfig.epochInfo.Epoch+1, height)
		return nil, nil, nil
	}
	sckptList := n.checkpointCache.getItems(height)
	if sckptList == nil {
		n.logger.Warningf("no checkpoint in height: %d", height)
		return nil, nil, nil
	}
	// 1. check if the next commit height match
	if height != n.lastCommitHeight+1 {
		return nil, sckptList, fmt.Errorf("next commit height not match: %d, %d", height, n.lastCommitHeight)
	}

	batchDigest := sckptList[0].GetCheckpoint().GetExecuteState().GetBatchDigest()
	// 2. check if the batch is in batch cache
	txHashList, timeStamp, proposer, loaded := n.getFromBatchCache(batchDigest)
	if !loaded {
		return nil, sckptList, fmt.Errorf("batch not found in batch cache: height: %d", height)
	}

	// 3. get txList from txpool,
	// if missing txs, start fetch missing txs from proposer
	txList, localList, missingTxs, err := n.txpool.GetRequestsByHashList(batchDigest, timeStamp, txHashList, []string{})
	if err != nil {
		n.logger.Warningf("DataSync node %d get error when get txList, err: %v", n.chainState.SelfNodeInfo.ID, err)
		return nil, sckptList, err
	}
	if missingTxs != nil {
		// reInsert ready checkpoint
		n.checkpointCache.ready.PushItem(height)
		fetch := &consensus.FetchMissingRequest{
			View:                 sckptList[0].GetCheckpoint().GetViewChange().GetBasis().GetView(),
			SequenceNumber:       height,
			BatchDigest:          batchDigest,
			MissingRequestHashes: missingTxs,
			ReplicaId:            n.chainState.SelfNodeInfo.ID,
		}
		if err = n.fetchMissingTxs(fetch, proposer); err != nil {
			n.logger.Warningf("Data node %d fetch missing txs error: %v", n.chainState.SelfNodeInfo.ID, err)
			return nil, sckptList, err
		}

		err = n.timeMgr.StartTimer(fetchMissingTxsResp)
		if err != nil {
			panic(fmt.Errorf("DataSync node %d start timer error: %v", n.chainState.SelfNodeInfo.ID, err))
		}
		return nil, nil, nil
	}

	readyCommit := &readyExecute[T, Constraint]{
		txs:       txList,
		localList: localList,
		height:    height,
		timestamp: timeStamp,
		proposer:  proposer,
	}

	return readyCommit, sckptList, nil
}

func (n *Node[T, Constraint]) postEvent(event *localEvent) {
	n.recvCh <- event
}

func (n *Node[T, Constraint]) reachQuorum(count int) bool {
	return uint64(count) >= common.CalQuorum(uint64(len(n.chainState.ValidatorSet)))
}

// setBatchCache stores a batch digest and its corresponding batch.
func (n *Node[T, Constraint]) setBachCache(batchDigest string, batch *consensus.HashBatch) {
	n.batchCache[batchDigest] = batch
}

// getFromBatchCache retrieves data from the batch cache based on the batch digest.
// It returns a list of request hash strings, a timestamp, and a boolean indicating whether the retrieval was successful.
func (n *Node[T, Constraint]) getFromBatchCache(batchDigest string) ([]string, int64, uint64, bool) {
	batch, ok := n.batchCache[batchDigest]
	if !ok {
		// If data does not exist, return nil values and false
		return nil, 0, 0, false
	}

	return batch.GetRequestHashList(), batch.GetTimestamp(), batch.GetProposer(), true
}

// todo: remove from cache after a certain time or number asynchronously?
func (n *Node[T, Constraint]) removeFromBatchCache(batchDigests []string) {
	for _, batchDigest := range batchDigests {
		delete(n.batchCache, batchDigest)
	}
}

func (n *Node[T, Constraint]) increaseCommitHeight() {
	n.lastCommitHeight++
}

func (n *Node[T, Constraint]) setCommitHeight(height uint64) {
	if n.lastCommitHeight >= height {
		n.logger.Warnf("set commit height %d less than last commit height %d", height, n.lastCommitHeight)
		return
	}
	n.lastCommitHeight = height
}

func (n *Node[T, Constraint]) getMsgNonce(typ consensus.Type) int64 {
	nonce, ok := n.msgNonce[typ]
	if !ok {
		nonce = time.Now().UnixNano()
		n.msgNonce[typ] = nonce
	}
	return nonce
}

func (n *Node[T, Constraint]) updateMsgNonce(typ consensus.Type) {
	nonce, ok := n.msgNonce[typ]
	if !ok {
		nonce = time.Now().UnixNano()
		n.msgNonce[typ] = nonce
	} else {
		n.msgNonce[typ] = nonce + 1
	}
}

func (n *Node[T, Constraint]) updateHighStateTarget(target *rbfttypes.MetaState, checkpointSet []*consensus.SignedCheckpoint, epochChanges ...*consensus.EpochChange) {
	if target == nil {
		n.logger.Warningf("Replica %d received a nil target", n.chainState.SelfNodeInfo.ID)
		return
	}

	if n.highStateTarget != nil && n.highStateTarget.metaState.Height >= target.Height {
		n.logger.Infof("Replica %d not updating state target to seqNo %d, has target for seqNo %d",
			n.chainState.SelfNodeInfo.ID, target.Height, n.highStateTarget.metaState.Height)
		return
	}

	if n.statusMgr.In(StateTransferring) {
		n.logger.Infof("Replica %d has found high-target expired while transferring, "+
			"update target to %d", n.chainState.SelfNodeInfo.ID, target.Height)
	} else {
		n.logger.Infof("Replica %d updating state target to seqNo %d digest %s", n.chainState.SelfNodeInfo.ID,
			target.Height, target.Digest)
	}

	n.highStateTarget = &stateUpdateTarget{
		metaState:     target,
		checkpointSet: checkpointSet,
		epochChanges:  epochChanges,
	}
	for _, checkpoint := range checkpointSet {
		n.checkpointCache.insert(checkpoint, true)
	}
}

// persistEpochQuorumCheckpoint persists QuorumCheckpoint or epoch to database
func (n *Node[T, Constraint]) persistEpochQuorumCheckpoint(c *consensus.QuorumCheckpoint) {
	start := time.Now()
	defer func() {
		n.logger.Infof("Persist epoch %d quorum chkpt cost %s", c.Checkpoint.Epoch, time.Since(start))
	}()
	key := fmt.Sprintf("%s%d", rbft.EpochStatePrefix, c.Checkpoint.Epoch)
	raw, err := c.MarshalVTStrict()
	if err != nil {
		n.logger.Errorf("Persist epoch %d quorum chkpt failed with marshal err: %s ", c.Checkpoint.Epoch, err)
		return
	}

	if err = n.stack.StoreEpochState(key, raw); err != nil {
		n.logger.Errorf("Persist epoch %d quorum chkpt failed with err: %s ", c.Checkpoint.Epoch, err)
	}

	// update latest epoch index
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, c.Checkpoint.Epoch)
	indexKey := rbft.EpochIndexKey
	if err = n.stack.StoreEpochState(indexKey, data); err != nil {
		n.logger.Errorf("Persist epoch index %d failed with err: %s ", c.Checkpoint.Epoch, err)
	}
}

// compareWholeStates compares whole networks' current status during sync state
// including :
// 1. view: current view of bft network
// 2. height: current latest blockChain height
// 3. digest: current latest blockChain hash
func (n *Node[T, Constraint]) compareWholeStates(states wholeStates) (nodeState, []*consensus.SignedCheckpoint, bool) {
	// track all replica with same state used to find quorum consistent state
	sameRespRecord := make(map[nodeState][]*consensus.SignedCheckpoint)

	// check if we can find quorum nodeState who have the same view, height and digest, if we can
	// find, which means quorum nodes agree to same state, save to quorumRsp, set canFind to true
	// and update view if needed
	sckptList := make([]*consensus.SignedCheckpoint, 0)
	var quorumState nodeState
	canFind := false

	// find the quorum nodeState
	for key, state := range states {
		sameRespRecord[state] = append(sameRespRecord[state], key)
		if n.reachQuorum(len(sameRespRecord[state])) {
			n.logger.Debugf("Replica %d find quorum states, try to process", n.chainState.SelfNodeInfo.ID)
			quorumState = state
			sckptList = sameRespRecord[state]
			canFind = true
			break
		}
	}
	return quorumState, sckptList, canFind
}
