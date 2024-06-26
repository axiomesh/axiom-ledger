package app

import (
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/pkg/events"
)

func (axm *AxiomLedger) start() {
	go axm.listenWaitReportBlock()
	go axm.listenWaitExecuteBlock()
}

func (axm *AxiomLedger) listenWaitReportBlock() {
	if axm.Repo.StartArgs.ReadonlyMode {
		return
	}

	blockCh := make(chan events.ExecutedEvent)
	blockSub := axm.BlockExecutor.SubscribeBlockEvent(blockCh)
	defer blockSub.Unsubscribe()

	mockBlockCh := make(chan events.ExecutedEvent)
	mockBlockSub := axm.Consensus.SubscribeMockBlockEvent(mockBlockCh)
	defer mockBlockSub.Unsubscribe()

	for {
		select {
		case <-axm.Ctx.Done():
			return
		case ev := <-blockCh:
			axm.reportBlock(ev, true)
		case ev := <-mockBlockCh:
			axm.reportBlock(ev, false)
		}
	}
}

func (axm *AxiomLedger) reportBlock(ev events.ExecutedEvent, needRemoveTxs bool) {
	axm.Consensus.ReportState(ev.Block.Header.Number, ev.Block.Hash(), ev.TxPointerList, ev.StateUpdatedCheckpoint, needRemoveTxs)

	// update txpool chain info
	newChainInfo := &txpool.ChainInfo{
		Height: ev.Block.Height(),
	}

	if common.NeedChangeEpoch(ev.Block.Height(), axm.ChainState.EpochInfo) {
		newChainInfo.EpochConf = &txpool.EpochConfig{
			EnableGenEmptyBatch: axm.ChainState.EpochInfo.ConsensusParams.EnableTimedGenEmptyBlock,
			BatchSize:           axm.ChainState.EpochInfo.ConsensusParams.BlockMaxTxNum,
		}
	}
	axm.TxPool.UpdateChainInfo(newChainInfo)
}

func (axm *AxiomLedger) listenWaitExecuteBlock() {
	if axm.Repo.StartArgs.ReadonlyMode {
		return
	}
	for {
		select {
		case commitEvent := <-axm.Consensus.Commit():
			axm.logger.WithFields(logrus.Fields{
				"height": commitEvent.Block.Header.Number,
				"count":  len(commitEvent.Block.Transactions),
			}).Info("Generated block")
			axm.BlockExecutor.AsyncExecuteBlock(commitEvent)
		case <-axm.Ctx.Done():
			return
		}
	}
}
