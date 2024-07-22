package solo

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/gogo/protobuf/sortkeys"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/pkg/events"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

func init() {
	repo.Register(repo.ConsensusTypeSolo, false)
}

type Node struct {
	config       *common.Config
	commitC      chan *common.CommitEvent                                             // block channel
	logger       logrus.FieldLogger                                                   // logger
	txpool       txpool.TxPool[types.Transaction, *types.Transaction]                 // transaction pool
	batchDigestM map[uint64]string                                                    // mapping blockHeight to batch digest
	recvCh       chan consensusEvent                                                  // receive message from consensus engine
	blockCh      chan *txpool.RequestHashBatch[types.Transaction, *types.Transaction] // receive batch from txpool
	batchMgr     *batchTimerManager
	lastExec     uint64          // the index of the last-applied block
	network      network.Network // network manager
	txPreCheck   precheck.PreCheck
	started      atomic.Bool
	epcCnf       *epochConfig

	ctx    context.Context
	cancel context.CancelFunc
	sync.RWMutex
	txFeed        event.Feed
	mockBlockFeed event.Feed
}

func NewNode(config *common.Config) (*Node, error) {
	currentEpoch := config.ChainState.GetCurrentEpochInfo()

	epochConf := &epochConfig{
		epochPeriod:         currentEpoch.EpochPeriod,
		startBlock:          currentEpoch.StartBlock,
		checkpoint:          currentEpoch.ConsensusParams.CheckpointPeriod,
		enableGenEmptyBlock: currentEpoch.ConsensusParams.EnableTimedGenEmptyBlock,
	}

	// init batch timer manager
	recvCh := make(chan consensusEvent, maxChanSize)

	ctx, cancel := context.WithCancel(context.Background())
	soloNode := &Node{
		config:       config,
		blockCh:      make(chan *txpool.RequestHashBatch[types.Transaction, *types.Transaction], maxChanSize),
		commitC:      make(chan *common.CommitEvent, maxChanSize),
		batchDigestM: make(map[uint64]string),
		recvCh:       recvCh,
		lastExec:     config.Applied,
		txpool:       config.TxPool,
		network:      config.Network,
		ctx:          ctx,
		cancel:       cancel,
		txPreCheck:   precheck.NewTxPreCheckMgr(ctx, config),
		epcCnf:       epochConf,
		logger:       config.Logger,
	}
	batchTimerMgr := &batchTimerManager{Timer: timer.NewTimerManager(config.Logger)}

	err := batchTimerMgr.CreateTimer(common.Batch, config.Repo.ConsensusConfig.Solo.BatchTimeout.ToDuration(), soloNode.handleTimeoutEvent)
	if err != nil {
		return nil, err
	}
	err = batchTimerMgr.CreateTimer(common.NoTxBatch, config.Repo.ConsensusConfig.TimedGenBlock.NoTxBatchTimeout.ToDuration(), soloNode.handleTimeoutEvent)
	if err != nil {
		return nil, err
	}
	soloNode.batchMgr = batchTimerMgr
	soloNode.logger.Infof("SOLO lastExec = %d", soloNode.lastExec)
	soloNode.logger.Infof("SOLO epoch period = %d", soloNode.epcCnf.epochPeriod)
	soloNode.logger.Infof("SOLO checkpoint period = %d", soloNode.epcCnf.checkpoint)
	soloNode.logger.Infof("SOLO enable gen empty block = %t", soloNode.epcCnf.enableGenEmptyBlock)
	soloNode.logger.Infof("SOLO no-tx batch timeout = %v", config.Repo.ConsensusConfig.TimedGenBlock.NoTxBatchTimeout.ToDuration())
	soloNode.logger.Infof("SOLO batch timeout = %v", config.Repo.ConsensusConfig.Solo.BatchTimeout.ToDuration())
	soloNode.logger.Infof("SOLO batch size = %d", config.GenesisEpochInfo.ConsensusParams.BlockMaxTxNum)
	soloNode.logger.Infof("SOLO pool size = %d", config.Repo.ConsensusConfig.TxPool.PoolSize)
	soloNode.logger.Infof("SOLO tolerance time = %v", config.Repo.ConsensusConfig.TxPool.ToleranceTime.ToDuration())
	soloNode.logger.Infof("SOLO tolerance remove time = %v", config.Repo.ConsensusConfig.TxPool.ToleranceRemoveTime.ToDuration())
	soloNode.logger.Infof("SOLO tolerance nonce gap = %d", config.Repo.ConsensusConfig.TxPool.ToleranceNonceGap)
	return soloNode, nil
}

func (n *Node) GetLowWatermark() uint64 {
	req := &getLowWatermarkReq{
		Resp: make(chan uint64),
	}
	n.postMsg(req)
	return <-req.Resp
}

func (n *Node) Start() error {
	n.txpool.Init(txpool.ConsensusConfig{
		NotifyGenerateBatchFn: n.notifyGenerateBatch,
	})
	err := n.txpool.Start()
	if err != nil {
		return err
	}
	err = n.batchMgr.StartTimer(common.Batch)
	if err != nil {
		return err
	}

	if n.epcCnf.enableGenEmptyBlock && !n.batchMgr.IsTimerActive(common.NoTxBatch) {
		err = n.batchMgr.StartTimer(common.NoTxBatch)
		if err != nil {
			return err
		}
	}
	n.txPreCheck.Start()
	go n.listenEvent()
	n.started.Store(true)
	n.logger.Info("Consensus started")
	return nil
}

func (n *Node) Stop() {
	n.cancel()
	n.logger.Info("Consensus stopped")
}

func (n *Node) Prepare(tx *types.Transaction) error {
	defer n.txFeed.Send([]*types.Transaction{tx})
	if err := n.Ready(); err != nil {
		return fmt.Errorf("node get ready failed: %w", err)
	}
	txWithResp := &common.TxWithResp{
		Tx:      tx,
		CheckCh: make(chan *common.TxResp, 1),
		PoolCh:  make(chan *common.TxResp, 1),
	}
	n.postMsg(txWithResp)
	resp := <-txWithResp.CheckCh
	if !resp.Status {
		return errors.Wrap(common.ErrorPreCheck, resp.ErrorMsg)
	}

	resp = <-txWithResp.PoolCh
	if !resp.Status {
		return errors.Wrap(common.ErrorAddTxPool, resp.ErrorMsg)
	}
	return nil
}

func (n *Node) Commit() chan *common.CommitEvent {
	return n.commitC
}

func (n *Node) Step([]byte) error {
	return nil
}

func (n *Node) Ready() error {
	if !n.started.Load() {
		return common.ErrorConsensusStart
	}
	return nil
}

func (n *Node) ReportState(height uint64, blockHash *types.Hash, txPointerList []*events.TxPointer, _ *common.Checkpoint, _ bool) {
	txHashList := make([]*types.Hash, len(txPointerList))
	lo.ForEach(txPointerList, func(item *events.TxPointer, i int) {
		txHashList[i] = item.Hash
	})
	epochChanged := false
	if common.NeedChangeEpoch(height, types.EpochInfo{StartBlock: n.epcCnf.startBlock, EpochPeriod: n.epcCnf.epochPeriod}) {
		epochChanged = true
	}
	state := &chainState{
		Height:       height,
		BlockHash:    blockHash,
		TxHashList:   txHashList,
		EpochChanged: epochChanged,
	}
	n.postMsg(state)
}

func (n *Node) Quorum(_ uint64) uint64 {
	return 1
}

func (n *Node) SubscribeTxEvent(events chan<- []*types.Transaction) event.Subscription {
	return n.txFeed.Subscribe(events)
}

func (n *Node) SubscribeMockBlockEvent(ch chan<- events.ExecutedEvent) event.Subscription {
	return n.mockBlockFeed.Subscribe(ch)
}

func (n *Node) SubmitTxsFromRemote(_ [][]byte) error {
	return nil
}

func (n *Node) listenEvent() {
	for {
		select {
		case <-n.ctx.Done():
			n.logger.Info("----- Exit listen event -----")
			return

		case ev := <-n.recvCh:
			switch e := ev.(type) {
			// handle report state
			case *chainState:
				if e.Height%n.epcCnf.checkpoint == 0 {
					n.logger.WithFields(logrus.Fields{
						"height": e.Height,
						"hash":   e.BlockHash.String(),
					}).Info("Report checkpoint")

					digestList := make([]string, len(n.batchDigestM))
					// flatten batchDigestM{<height:> <digest>} to []digest, sort by height
					heightList := make([]uint64, 0)
					for h := range n.batchDigestM {
						// remove batches which is less than current state height
						if h <= e.Height {
							heightList = append(heightList, h)
						}
					}
					sortkeys.Uint64s(heightList)
					lo.ForEach(heightList, func(height uint64, index int) {
						digestList[index] = n.batchDigestM[height]
						delete(n.batchDigestM, height)
					})
					n.logger.Debug("RemoveBatches", len(digestList), digestList)
					n.txpool.RemoveBatches(digestList)
				}

				if e.EpochChanged {
					currentEpoch := n.config.ChainState.GetCurrentEpochInfo()

					n.epcCnf.startBlock = currentEpoch.StartBlock
					n.epcCnf.epochPeriod = currentEpoch.EpochPeriod
					n.epcCnf.enableGenEmptyBlock = currentEpoch.ConsensusParams.EnableTimedGenEmptyBlock
					n.epcCnf.checkpoint = currentEpoch.ConsensusParams.CheckpointPeriod

					if n.epcCnf.enableGenEmptyBlock && !n.batchMgr.IsTimerActive(common.NoTxBatch) {
						err := n.batchMgr.StartTimer(common.NoTxBatch)
						if err != nil {
							n.logger.WithFields(logrus.Fields{
								"error":  err.Error(),
								"height": e.Height,
							}).Error("Start timer failed")
						}
					}
					n.logger.WithFields(logrus.Fields{
						"epoch":               currentEpoch.Epoch,
						"start":               currentEpoch.StartBlock,
						"period":              currentEpoch.EpochPeriod,
						"batchSize":           currentEpoch.ConsensusParams.BlockMaxTxNum,
						"checkpoint":          n.epcCnf.checkpoint,
						"enableGenEmptyBlock": n.epcCnf.enableGenEmptyBlock,
					}).Info("Report epoch changed")
				}

			// receive tx from api
			case *common.TxWithResp:
				unCheckedEv := &common.UncheckedTxEvent{
					EventType: common.LocalTxEvent,
					Event:     e,
				}
				n.txPreCheck.PostUncheckedTxEvent(unCheckedEv)

			// handle timeout event
			case timer.TimeoutEvent:
				if err := n.processBatchTimeout(e); err != nil {
					n.logger.Errorf("Process batch timeout failed: %v", err)
				}

			case *getLowWatermarkReq:
				e.Resp <- n.lastExec
			case *genBatchReq:
				n.batchMgr.StopTimer(common.Batch)
				n.batchMgr.StopTimer(common.NoTxBatch)
				batch, err := n.txpool.GenerateRequestBatch(e.typ)
				if err != nil {
					n.logger.Errorf("Generate batch failed: %v", err)
				} else if batch != nil {
					n.generateBlock(batch)
					// start no-tx batch timer when this node handle the last transaction
					if n.epcCnf.enableGenEmptyBlock && !n.txpool.HasPendingRequestInPool() {
						if err = n.batchMgr.RestartTimer(common.NoTxBatch); err != nil {
							n.logger.Errorf("restart no-tx batch timeout failed: %v", err)
						}
					}
				}
				if err = n.batchMgr.RestartTimer(common.Batch); err != nil {
					n.logger.Errorf("restart batch timeout failed: %v", err)
				}
			}
		}
	}
}

func (n *Node) processBatchTimeout(e timer.TimeoutEvent) error {
	switch e {
	case common.Batch:
		n.batchMgr.StopTimer(common.Batch)
		defer func() {
			if err := n.batchMgr.RestartTimer(common.Batch); err != nil {
				n.logger.Errorf("restart batch timeout failed: %v", err)
			}
		}()
		if n.txpool.HasPendingRequestInPool() {
			n.batchMgr.StopTimer(common.NoTxBatch)
			defer func() {
				// if generate last batch, restart no-tx batch timeout to ensure next empty-block will be generated after no-tx batch timeout
				if n.txpool.HasPendingRequestInPool() {
					if n.epcCnf.enableGenEmptyBlock {
						if err := n.batchMgr.RestartTimer(common.NoTxBatch); err != nil {
							n.logger.Errorf("restart no-tx batch timeout failed: %v", err)
						}
					}
				}
			}()
			batch, err := n.txpool.GenerateRequestBatch(txpool.GenBatchTimeoutEvent)
			if err != nil {
				return err
			}
			if batch != nil {
				now := time.Now().UnixNano()
				if n.batchMgr.lastBatchTime != 0 {
					interval := time.Duration(now - n.batchMgr.lastBatchTime).Seconds()
					batchInterval.WithLabelValues("timeout").Observe(interval)
					if n.batchMgr.minTimeoutBatchTime == 0 || interval < n.batchMgr.minTimeoutBatchTime {
						n.logger.Debugf("update min timeoutBatch Time[height:%d, interval:%f, lastBatchTime:%v]",
							n.lastExec+1, interval, time.Unix(0, n.batchMgr.lastBatchTime))
						minBatchIntervalDuration.WithLabelValues("timeout").Set(interval)
						n.batchMgr.minTimeoutBatchTime = interval
					}
				}
				n.batchMgr.lastBatchTime = now
				n.generateBlock(batch)
				n.logger.Debugf("batch timeout, post proposal: [batchHash: %s]", batch.BatchHash)
			}
		}
	case common.NoTxBatch:
		n.batchMgr.StopTimer(common.NoTxBatch)
		defer func() {
			if n.epcCnf.enableGenEmptyBlock {
				if err := n.batchMgr.RestartTimer(common.NoTxBatch); err != nil {
					n.logger.Errorf("restart no-tx batch timeout failed: %v", err)
				}
			}
		}()
		if n.txpool.HasPendingRequestInPool() {
			n.logger.Debugf("TxPool is not empty, skip handle the no-tx batch timer event")
			return nil
		}

		batch, err := n.txpool.GenerateRequestBatch(txpool.GenBatchNoTxTimeoutEvent)
		if err != nil {
			return err
		}
		if batch != nil {
			n.logger.Debug("Prepare create empty block")
			now := time.Now().UnixNano()
			if n.batchMgr.lastBatchTime != 0 {
				interval := time.Duration(now - n.batchMgr.lastBatchTime).Seconds()
				batchInterval.WithLabelValues("timeout_no_tx").Observe(interval)
				if n.batchMgr.minNoTxTimeoutBatchTime == 0 || interval < n.batchMgr.minNoTxTimeoutBatchTime {
					n.logger.Debugf("update min noTxTimeoutBatch Time[height:%d, interval:%f, lastBatchTime:%v]",
						n.lastExec+1, interval, time.Unix(0, n.batchMgr.lastBatchTime))
					minBatchIntervalDuration.WithLabelValues("timeout_no_tx").Set(interval)
					n.batchMgr.minNoTxTimeoutBatchTime = interval
				}
			}
			n.batchMgr.lastBatchTime = now

			n.generateBlock(batch)
			n.logger.Debugf("batch no-tx timeout, post proposal: %v", batch)
		}
	}
	return nil
}

// Schedule to collect txs to the listenReadyBlock channel
func (n *Node) generateBlock(batch *txpool.RequestHashBatch[types.Transaction, *types.Transaction]) {
	n.logger.WithFields(logrus.Fields{
		"batch_hash": batch.BatchHash,
		"tx_count":   len(batch.TxList),
	}).Debugf("Receive proposal from txpool")

	// genesis block
	nextBlock := n.lastExec + 1
	if n.config.ChainState.ChainMeta.BlockHash == nil {
		nextBlock = 0
	}

	block := &types.Block{
		Header: &types.BlockHeader{
			Number:         nextBlock,
			Timestamp:      batch.Timestamp / int64(time.Second),
			ProposerNodeID: 1,
		},
		Transactions: batch.TxList,
	}
	localList := make([]bool, len(batch.TxList))
	for i := 0; i < len(batch.TxList); i++ {
		localList[i] = true
	}
	executeEvent := &common.CommitEvent{
		Block: block,
	}
	n.batchDigestM[block.Height()] = batch.BatchHash
	n.lastExec = nextBlock
	n.commitC <- executeEvent
	n.logger.Infof("======== Call execute, height=%d", n.lastExec)
}

func (n *Node) notifyGenerateBatch(typ int) {
	req := &genBatchReq{typ: typ}
	n.postMsg(req)
}

func (n *Node) postMsg(ev consensusEvent) {
	n.recvCh <- ev
}

func (n *Node) handleTimeoutEvent(typ timer.TimeoutEvent) {
	switch typ {
	case common.Batch:
		n.postMsg(common.Batch)
	case common.NoTxBatch:
		n.postMsg(common.NoTxBatch)
	default:
		n.logger.Errorf("receive wrong timeout event type: %s", typ)
	}
}
