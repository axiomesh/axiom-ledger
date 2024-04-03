package rbft

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Rican7/retry"
	"github.com/Rican7/retry/strategy"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	rbfttypes "github.com/axiomesh/axiom-bft/types"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/adaptor"
	"github.com/axiomesh/axiom-ledger/internal/consensus/txcache"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/pkg/events"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	p2p "github.com/axiomesh/axiom-p2p"
)

const (
	consensusMsgPipeIDPrefix = "consensus_msg_pipe_v1_"
	txsBroadcastMsgPipeID    = "txs_broadcast_msg_pipe_v1"
)

func init() {
	repo.Register(repo.ConsensusTypeRbft, true)
}

type Node struct {
	config                 *common.Config
	txpool                 txpool.TxPool[types.Transaction, *types.Transaction]
	n                      rbft.Node[types.Transaction, *types.Transaction]
	stack                  *adaptor.RBFTAdaptor
	logger                 logrus.FieldLogger
	network                network.Network
	consensusMsgPipes      map[int32]p2p.Pipe
	consensusGlobalMsgPipe p2p.Pipe
	txsBroadcastMsgPipe    p2p.Pipe
	receiveMsgLimiter      *rate.Limiter
	started                atomic.Bool

	ctx        context.Context
	cancel     context.CancelFunc
	txCache    *txcache.TxCache
	txPreCheck precheck.PreCheck

	txFeed event.Feed
}

func NewNode(config *common.Config) (*Node, error) {
	rbftConfig, err := generateRbftConfig(config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	rbftAdaptor, err := adaptor.NewRBFTAdaptor(config)
	if err != nil {
		cancel()
		return nil, err
	}

	n, err := rbft.NewNode[types.Transaction, *types.Transaction](rbftConfig, rbftAdaptor, config.TxPool)
	if err != nil {
		cancel()
		return nil, err
	}

	var receiveMsgLimiter *rate.Limiter
	if config.Config.Limit.Enable {
		receiveMsgLimiter = rate.NewLimiter(rate.Limit(config.Config.Limit.Limit), int(config.Config.Limit.Burst))
	}

	return &Node{
		config:            config,
		n:                 n,
		logger:            config.Logger,
		stack:             rbftAdaptor,
		receiveMsgLimiter: receiveMsgLimiter,
		ctx:               ctx,
		cancel:            cancel,
		txCache:           txcache.NewTxCache(config.Config.TxCache.SetTimeout.ToDuration(), uint64(config.Config.TxCache.SetSize), config.Logger),
		network:           config.Network,
		txPreCheck:        precheck.NewTxPreCheckMgr(ctx, config),
		txpool:            config.TxPool,
	}, nil
}

func (n *Node) initConsensusMsgPipes() error {
	n.consensusMsgPipes = make(map[int32]p2p.Pipe, len(consensus.Type_name))
	if n.config.Config.Rbft.EnableMultiPipes {
		for id, name := range consensus.Type_name {
			msgPipe, err := n.network.CreatePipe(n.ctx, consensusMsgPipeIDPrefix+name)
			if err != nil {
				return err
			}
			n.consensusMsgPipes[id] = msgPipe
		}
	} else {
		globalMsgPipe, err := n.network.CreatePipe(n.ctx, consensusMsgPipeIDPrefix+"global")
		if err != nil {
			return err
		}
		n.consensusGlobalMsgPipe = globalMsgPipe
	}

	n.stack.SetMsgPipes(n.consensusMsgPipes, n.consensusGlobalMsgPipe)
	return nil
}

func (n *Node) Start() error {
	err := n.stack.UpdateEpoch()
	if err != nil {
		return err
	}

	n.n.ReportExecuted(&rbfttypes.ServiceState{
		MetaState: &rbfttypes.MetaState{
			Height: n.config.Applied,
			Digest: n.config.Digest,
		},
		Epoch: n.stack.EpochInfo.Epoch,
	})

	if err := n.initConsensusMsgPipes(); err != nil {
		return err
	}

	txsBroadcastMsgPipe, err := n.network.CreatePipe(n.ctx, txsBroadcastMsgPipeID)
	if err != nil {
		return err
	}
	n.txsBroadcastMsgPipe = txsBroadcastMsgPipe

	if err := retry.Retry(func(attempt uint) error {
		err = n.checkQuorum()
		if err != nil {
			return err
		}
		return nil
	},
		strategy.Wait(1*time.Second),
	); err != nil {
		n.logger.Error(err)
	}

	go n.txPreCheck.Start()
	go n.txCache.ListenEvent()

	go n.listenNewTxToSubmit()
	go n.listenExecutedBlockToReport()
	go n.listenBatchMemTxsToBroadcast()
	go n.listenConsensusMsg()
	go n.listenTxsBroadcastMsg()

	// start consensus engine
	if err = n.n.Start(); err != nil {
		return err
	}

	// start txpool engine
	if err = n.txpool.Start(); err != nil {
		return err
	}

	if txs := n.txpool.GetLocalTxs(); len(txs) > 0 {
		for len(txs) > 0 {
			minBatch := func(a, b int) int {
				if a < b {
					return a
				}
				return b
			}
			length := minBatch(len(txs), int(n.txCache.TxSetSize))
			data := txs[:length]
			err = n.broadcastTxs(data)
			if err != nil {
				return err
			}
			txs = txs[length:]
		}
	}

	n.started.Store(true)
	n.logger.Info("=====Consensus started=========")
	return nil
}

func (n *Node) listenConsensusMsg() {
	for _, pipe := range n.consensusMsgPipes {
		pipe := pipe
		go func() {
			for {
				msg := pipe.Receive(n.ctx)
				if msg == nil {
					return
				}

				if err := n.Step(msg.Data); err != nil {
					n.logger.WithFields(logrus.Fields{"pipe": pipe.String(), "err": err, "from": msg.From}).Warn("Process consensus message failed")
					continue
				}
			}
		}()
	}

	if n.consensusGlobalMsgPipe != nil {
		go func() {
			for {
				msg := n.consensusGlobalMsgPipe.Receive(n.ctx)
				if msg == nil {
					return
				}

				if err := n.Step(msg.Data); err != nil {
					n.logger.WithFields(logrus.Fields{"pipe": n.consensusGlobalMsgPipe.String(), "err": err, "from": msg.From}).Warn("Process consensus message failed")
					continue
				}
			}
		}()
	}
}

func (n *Node) listenTxsBroadcastMsg() {
	for {
		msg := n.txsBroadcastMsgPipe.Receive(n.ctx)
		if msg == nil {
			return
		}

		if n.receiveMsgLimiter != nil && !n.receiveMsgLimiter.Allow() {
			// rate limit exceeded, refuse to process the message
			n.logger.Warn("Node received too many PUSH_TXS messages. Rate limiting in effect")
			continue
		}

		tx := &pb.BytesSlice{}
		if err := tx.UnmarshalVT(msg.Data); err != nil {
			n.logger.WithField("err", err).Warn("Unmarshal txs message failed")
			continue
		}
		n.submitTxsFromRemote(tx.Slice)
	}
}

func (n *Node) listenNewTxToSubmit() {
	for {
		select {
		case <-n.ctx.Done():
			return

		case txWithResp := <-n.txCache.TxRespC:
			ev := &common.UncheckedTxEvent{
				EventType: common.LocalTxEvent,
				Event:     txWithResp,
			}
			n.txPreCheck.PostUncheckedTxEvent(ev)
		}
	}
}

func (n *Node) listenExecutedBlockToReport() {
	for {
		select {
		case r := <-n.stack.ReadyC:
			block := &types.Block{
				Header: &types.BlockHeader{
					Epoch:           n.stack.EpochInfo.Epoch,
					Number:          r.Height,
					Timestamp:       r.Timestamp / int64(time.Second),
					ProposerAccount: r.ProposerAccount,
					ProposerNodeID:  r.ProposerNodeID,
				},
				Transactions: r.Txs,
			}
			commitEvent := &common.CommitEvent{
				Block: block,
			}
			n.stack.PostCommitEvent(commitEvent)
		case <-n.ctx.Done():
			return
		}
	}
}

func (n *Node) broadcastTxs(txSetData [][]byte) error {
	msg := &pb.BytesSlice{
		Slice: txSetData,
	}
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}

	return n.txsBroadcastMsgPipe.Broadcast(context.TODO(), lo.Map(lo.Flatten([][]rbft.NodeInfo{n.stack.EpochInfo.ValidatorSet, n.stack.EpochInfo.CandidateSet}), func(item rbft.NodeInfo, index int) string {
		return item.P2PNodeID
	}), data)
}

func (n *Node) listenBatchMemTxsToBroadcast() {
	for {
		select {
		case txSet := <-n.txCache.TxSetC:
			var requests [][]byte
			for _, tx := range txSet {
				raw, err := tx.RbftMarshal()
				if err != nil {
					n.logger.Error(err)
					continue
				}
				requests = append(requests, raw)
			}

			// broadcast to other node
			err := n.broadcastTxs(requests)
			if err != nil {
				n.logger.Errorf("failed to broadcast txpool txs: %v", err)
			}

		case <-n.ctx.Done():
			return
		}
	}
}

func (n *Node) Stop() {
	n.stack.Cancel()
	n.cancel()
	if n.txCache.CloseC != nil {
		close(n.txCache.CloseC)
	}
	n.n.Stop()
	n.logger.Info("=====Consensus stopped=========")
}

func (n *Node) Prepare(tx *types.Transaction) error {
	defer n.txFeed.Send([]*types.Transaction{tx})
	if !n.started.Load() {
		return common.ErrorConsensusStart
	}

	txWithResp := &common.TxWithResp{
		Tx:      tx,
		CheckCh: make(chan *common.TxResp, 1),
		PoolCh:  make(chan *common.TxResp, 1),
	}
	n.txCache.TxRespC <- txWithResp
	precheckResp := <-txWithResp.CheckCh
	if !precheckResp.Status {
		return errors.Wrap(common.ErrorPreCheck, precheckResp.ErrorMsg)
	}

	resp := <-txWithResp.PoolCh
	if !resp.Status {
		return errors.Wrap(common.ErrorAddTxPool, resp.ErrorMsg)
	}

	// make sure that tx is prechecked and add LocalTxPool successfully
	n.txCache.RecvTxC <- tx
	return nil
}

func (n *Node) submitTxsFromRemote(txs [][]byte) {
	var requests []*types.Transaction
	for _, item := range txs {
		tx := &types.Transaction{}
		if err := tx.RbftUnmarshal(item); err != nil {
			n.logger.Error(err)
			continue
		}
		requests = append(requests, tx)
	}

	n.txFeed.Send(requests)
	ev := &common.UncheckedTxEvent{
		EventType: common.RemoteTxEvent,
		Event:     requests,
	}
	n.txPreCheck.PostUncheckedTxEvent(ev)
}

func (n *Node) Commit() chan *common.CommitEvent {
	return n.stack.GetCommitChannel()
}

func (n *Node) Step(msg []byte) error {
	m := &consensus.ConsensusMessage{}
	if err := m.UnmarshalVT(msg); err != nil {
		return err
	}
	n.n.Step(context.Background(), m)

	return nil
}

func (n *Node) Ready() error {
	status := n.n.Status().Status
	isNormal := status == rbft.Normal
	if !isNormal {
		return fmt.Errorf("%s", status2String(status))
	}
	return nil
}

func (n *Node) GetLowWatermark() uint64 {
	return n.n.GetLowWatermark()
}

func (n *Node) ReportState(height uint64, blockHash *types.Hash, txPointerList []*events.TxPointer, ckp *consensus.Checkpoint, needRemoveTxs bool) {
	// need update cached epoch info
	epochInfo := n.stack.EpochInfo
	epochChanged := false
	if common.NeedChangeEpoch(height, epochInfo) {
		n.txPreCheck.UpdateEpochInfo(epochInfo)
		err := n.stack.UpdateEpoch()
		if err != nil {
			panic(err)
		}
		epochChanged = true
		n.logger.Debugf("Finished execute block %d, update epoch to %d", height, n.stack.EpochInfo.Epoch)
	}
	currentEpoch := n.stack.EpochInfo.Epoch

	// skip the block before the target height
	if n.stack.StateUpdating {
		if epochChanged || n.stack.StateUpdateHeight == height {
			// when we end state updating, we need to verify the checkpoint from quorum nodes
			if n.stack.StateUpdateHeight == height && ckp != nil {
				if err := n.verifyStateUpdatedCheckpoint(ckp); err != nil {
					n.logger.WithField("err", err).Errorf("verify state updated checkpoint failed")
					panic(err)
				}
			}
			// notify consensus update epoch and accept epoch proof
			state := &rbfttypes.ServiceSyncState{
				ServiceState: rbfttypes.ServiceState{
					MetaState: &rbfttypes.MetaState{
						Height: height,
						Digest: blockHash.String(),
					},
					Epoch: currentEpoch,
				},
				EpochChanged: epochChanged,
			}
			n.n.ReportStateUpdated(state)
		}

		if n.stack.StateUpdateHeight == height {
			n.stack.StateUpdating = false
		}

		if needRemoveTxs {
			// notify tx pool remove these committed tx
			committedTxHashList := make([]*txpool.WrapperTxPointer, len(txPointerList))
			lo.ForEach(txPointerList, func(item *events.TxPointer, index int) {
				committedTxHashList[index] = &txpool.WrapperTxPointer{
					TxHash:  item.Hash.String(),
					Account: item.Account,
					Nonce:   item.Nonce,
				}
			})
			n.txpool.RemoveStateUpdatingTxs(committedTxHashList)
		}
		return
	}

	state := &rbfttypes.ServiceState{
		MetaState: &rbfttypes.MetaState{
			Height: height,
			Digest: blockHash.String(),
		},
		Epoch: currentEpoch,
	}
	n.n.ReportExecuted(state)

	if n.stack.StateUpdateHeight == height {
		n.stack.StateUpdating = false
	}
}

func (n *Node) SubscribeMockBlockEvent(ch chan<- events.ExecutedEvent) event.Subscription {
	return n.stack.MockBlockFeed.Subscribe(ch)
}

func (n *Node) verifyStateUpdatedCheckpoint(checkpoint *consensus.Checkpoint) error {
	height := checkpoint.Height()
	localBlockHeader, err := n.config.GetBlockHeaderFunc(height)
	if err != nil || localBlockHeader == nil {
		return fmt.Errorf("get local block header failed: %w", err)
	}
	if localBlockHeader.Hash().String() != checkpoint.GetExecuteState().GetDigest() {
		return fmt.Errorf("local block [hash %s, height: %d] not equal to checkpoint digest %s",
			localBlockHeader.Hash().String(), height, checkpoint.GetExecuteState().GetDigest())
	}
	return nil
}

func (n *Node) Quorum(totalNum uint64) uint64 {
	// N := uint64(len(n.stack.EpochInfo.ValidatorSet))
	return adaptor.CalQuorum(totalNum)
}

func (n *Node) checkQuorum() error {
	totalNum := uint64(len(n.stack.EpochInfo.ValidatorSet))
	n.logger.Infof("=======Quorum = %d, connected validators = %d", n.Quorum(totalNum), n.network.CountConnectedValidators()+1)
	if n.network.CountConnectedValidators()+1 < n.Quorum(totalNum) {
		return errors.New("the number of connected validators don't reach Quorum")
	}
	return nil
}

func (n *Node) SubscribeTxEvent(events chan<- []*types.Transaction) event.Subscription {
	return n.txFeed.Subscribe(events)
}

// status2String returns a long description of SystemStatus
func status2String(status rbft.StatusType) string {
	switch status {
	case rbft.Normal:
		return "Normal"
	case rbft.InConfChange:
		return "system is in conf change"
	case rbft.InViewChange:
		return "system is in view change"
	case rbft.InRecovery:
		return "system is in recovery"
	case rbft.StateTransferring:
		return "system is in state update"
	case rbft.Pending:
		return "system is in pending state"
	case rbft.Stopped:
		return "system is stopped"
	default:
		return fmt.Sprintf("Unknown status: %d", status)
	}
}
