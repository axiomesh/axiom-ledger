package txpool

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/btree"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/fileutil"
	commonpool "github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/components"
	"github.com/axiomesh/axiom-ledger/internal/components/status"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var _ commonpool.TxPool[types.Transaction, *types.Transaction] = (*txPoolImpl[types.Transaction, *types.Transaction])(nil)

var (
	ErrTxPoolFull     = errors.New("tx pool full")
	ErrNonceTooLow    = errors.New("nonce too low")
	ErrNonceTooHigh   = errors.New("nonce too high")
	ErrDuplicateTx    = errors.New("duplicate tx")
	ErrGasPriceTooLow = errors.New("gas price too low")
	ErrBelowPriceBump = errors.New("replace old tx err, gas price is below price bump")
)

// txPoolImpl contains all currently known transactions.
type txPoolImpl[T any, Constraint types.TXConstraint[T]] struct {
	logger                 logrus.FieldLogger
	chainState             *chainstate.ChainState
	selfID                 uint64
	txStore                *transactionStore[T, Constraint] // store all transaction info
	toleranceNonceGap      uint64
	toleranceTime          time.Duration
	toleranceRemoveTime    time.Duration
	cleanEmptyAccountTime  time.Duration
	rotateTxLocalsInterval time.Duration
	poolMaxSize            uint64
	priceLimit             atomic.Pointer[big.Int] // Minimum gas price to enforce for acceptance into the pool
	PriceBump              uint64                  // Minimum price bump percentage to replace an already existing transaction (nonce)
	enableLocalsPersist    bool
	txRecordsFile          string
	enablePricePriority    bool

	getAccountNonce       GetAccountNonceFunc
	getAccountBalance     GetAccountBalanceFunc
	notifyGenerateBatch   bool
	notifyGenerateBatchFn func(typ int)
	notifyFindNextBatchFn func(completionMissingBatchHashes ...string) // notify consensus that it can find next batch

	timerMgr  timer.Timer
	statusMgr *status.StatusMgr
	started   atomic.Bool
	txRecords *txRecords[T, Constraint]

	revCh  chan txPoolEvent
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewTxPool[T any, Constraint types.TXConstraint[T]](config Config, chainState *chainstate.ChainState) (commonpool.TxPool[T, Constraint], error) {
	return newTxPoolImpl[T, Constraint](config, chainState)
}

func (p *txPoolImpl[T, Constraint]) Start() error {
	if p.started.Load() {
		return errors.New("txpool already started")
	}
	go p.listenEvent()

	err := p.timerMgr.StartTimer(RemoveTx)
	if err != nil {
		return err
	}
	err = p.timerMgr.StartTimer(CleanEmptyAccount)
	if err != nil {
		return err
	}
	if p.enableLocalsPersist {
		err = p.timerMgr.StartTimer(RotateTxLocals)
		if err != nil {
			return err
		}
	}

	p.started.Store(true)
	p.logger.Info("txpool started")
	return nil
}

func (p *txPoolImpl[T, Constraint]) processRecords() error {
	taskDoneCh := make(chan struct{}, 1)
	defer close(taskDoneCh)

	input, err := os.Open(p.txRecords.filePath)
	if err != nil {
		p.logger.Errorf("Failed to open tx records file: %v", err)
		return err
	}
	defer input.Close()

	now := time.Now()
	txsCh := p.txRecords.load(input, taskDoneCh)
	totalInsertCount := 0
	for {
		select {
		case txs, ok := <-txsCh:
			if !ok {
				return nil
			}
			totalInsertCount += p.processRecordsTask(txs)
		case <-taskDoneCh:
			close(txsCh)
			for txs := range txsCh {
				totalInsertCount += p.processRecordsTask(txs)
			}

			p.logger.WithFields(logrus.Fields{
				"add_num": totalInsertCount,
				"cost":    time.Since(now),
			}).Info("End add record txs successfully")
			return nil
		}
	}
}

func (p *txPoolImpl[T, Constraint]) processRecordsTask(txs []*T) int {
	start := time.Now()
	req := &reqLocalRecordTx[T, Constraint]{
		txs: txs,
		ch:  make(chan int, 1),
	}
	p.handleLocalRecordTx(req)
	insertCount := <-req.ch

	if p.checkPoolFull() {
		p.setFull()
	}

	p.logger.WithFields(logrus.Fields{
		"add_num": insertCount,
		"cost":    time.Since(start),
	}).Info("Add record txs successfully")

	return insertCount
}

func (p *txPoolImpl[T, Constraint]) listenEvent() {
	p.wg.Add(1)
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			p.logger.Info("txpool stopped")
			return
		case next := <-p.revCh:
			nexts := make([]txPoolEvent, 0)
			for {
				select {
				case <-p.ctx.Done():
					p.logger.Info("txpool stopped")
					return
				default:
				}
				events := p.processEvent(next)
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

func (p *txPoolImpl[T, Constraint]) processEvent(event txPoolEvent) []txPoolEvent {
	switch e := event.(type) {
	case *addTxsEvent:
		return p.dispatchAddTxsEvent(e)
	case *removeTxsEvent:
		p.dispatchRemoveTxsEvent(e)
		return nil
	case *batchEvent:
		return p.dispatchBatchEvent(e)
	case *poolInfoEvent:
		p.dispatchPoolInfoEvent(e)
		return nil
	case *consensusEvent:
		p.dispatchConsensusEvent(e)
		return nil
	case *localEvent:
		p.dispatchLocalEvent(e)
	default:
		p.logger.Warning("unknown event type", e)
	}

	return nil
}

func (p *txPoolImpl[T, Constraint]) updateValidTxs(validTxs *[]*T, tx *T, replaced bool) {
	*validTxs = append(*validTxs, tx)
	if replaced {
		for index := 0; index < len(*validTxs); index++ {
			if Constraint((*validTxs)[index]).RbftGetFrom() == Constraint(tx).RbftGetFrom() &&
				Constraint((*validTxs)[index]).RbftGetNonce() == Constraint(tx).RbftGetNonce() {
				// if old tx is replaced, remove it
				(*validTxs)[index] = (*validTxs)[len(*validTxs)-1]
				// remove old tx
				*validTxs = (*validTxs)[:len(*validTxs)-1]
				break
			}
		}
	}
}

func (p *txPoolImpl[T, Constraint]) dispatchAddTxsEvent(event *addTxsEvent) []txPoolEvent {
	if event.EventType != localTxEvent && event.EventType != remoteTxsEvent {
		p.logger.Debugf("start dispatch add txs event:%s", addTxsEventToStr[event.EventType])
	}
	var nextEvents []txPoolEvent

	start := time.Now()
	metricsPrefix := "addTxs_"
	defer func() {
		traceProcessEvent(fmt.Sprintf("%s%s", metricsPrefix, addTxsEventToStr[event.EventType]), time.Since(start))
	}()
	switch event.EventType {
	case localTxEvent:
		req := event.Event.(*reqLocalTx[T, Constraint])
		if p.statusMgr.In(PoolFull) {
			traceRejectTx(ErrTxPoolFull.Error())
			req.errCh <- ErrTxPoolFull
			return nil
		}

		_, err := p.addTx(req.tx, true)
		// trigger remove all high nonce txs
		if errors.Is(err, ErrNonceTooHigh) {
			removeEvent := p.genHighNonceEvent(req.tx)
			nextEvents = append(nextEvents, removeEvent)
		}

		if err == nil {
			if p.enableLocalsPersist && p.txRecordsFile != "" {
				now := time.Now()
				err = p.txRecords.insert(req.tx)
				tracePersistRecords(time.Since(now))
			}

			// if it is a valid tx, check if it should notify consensus
			p.postConsensusSignal([]*T{req.tx})
		}
		defer func() {
			req.errCh <- err
			if err != nil {
				traceRejectTx(err.Error())
			}
		}()

	case remoteTxsEvent, reBroadcastTxsEvent:
		nonceTooHighAccounts := make(map[string]bool)
		req := event.Event.(*reqRemoteTxs[T, Constraint])
		txs := req.txs
		validTxs := make([]*T, 0)

		traceRejectTxs := func(txs []*T) {
			for i := 0; i < len(txs); i++ {
				traceRejectTx(ErrTxPoolFull.Error())
			}
		}

		overSpaceTxs := make([]*T, 0)
		defer func() {
			traceRejectTxs(overSpaceTxs)
		}()

		// rebroadcast txs should not be rejected
		if event.EventType == remoteTxsEvent {
			// if tx pool is full, reject the left txs
			if len(p.txStore.txHashMap)+len(txs) > int(p.poolMaxSize) {
				if len(p.txStore.txHashMap) < int(p.poolMaxSize) {
					remainSpace := int(p.poolMaxSize) - len(p.txStore.txHashMap)
					overSpaceTxs = txs[remainSpace:]
					txs = txs[:remainSpace]
				}
			}
			if p.statusMgr.In(PoolFull) {
				overSpaceTxs = txs
				return nil
			}
		}

		lo.ForEach(txs, func(tx *T, i int) {
			replaced, err := p.addTx(tx, false)

			// trigger remove all high nonce txs, ensure this event is only triggered once
			if errors.Is(err, ErrNonceTooHigh) && !nonceTooHighAccounts[Constraint(txs[i]).RbftGetFrom()] {
				removeEvent := p.genHighNonceEvent(txs[i])
				nextEvents = append(nextEvents, removeEvent)
				nonceTooHighAccounts[Constraint(txs[i]).RbftGetFrom()] = true
			}
			if err != nil {
				traceRejectTx(err.Error())
			} else {
				p.updateValidTxs(&validTxs, tx, replaced)
			}
		})

		p.postConsensusSignal(validTxs)

	case missingTxsEvent:
		// NOTICE!!! this event will ignore the pool full status
		req := event.Event.(*reqMissingTxs[T, Constraint])
		validTxs, err := p.validateReceiveMissingRequests(req.batchHash, req.txs)
		if err == nil {
			lo.ForEach(validTxs, func(tx *T, _ int) {
				p.replaceTx(tx, false)
			})
			delete(p.txStore.missingBatch, req.batchHash)
		} else {
			p.logger.WithFields(logrus.Fields{"batchHash": req.batchHash, "err": err}).Warning("receive missing txs failed")
		}
		defer func() {
			req.errCh <- err
		}()

	case localRecordTxEvent:
		req := event.Event.(*reqLocalRecordTx[T, Constraint])
		p.handleLocalRecordTx(req)
	default:
		p.logger.Errorf("unknown addTxs event type: %d", event.EventType)
	}

	// check if pool is full
	if p.checkPoolFull() {
		p.setFull()
	}

	return nextEvents
}

func (p *txPoolImpl[T, Constraint]) handleLocalRecordTx(req *reqLocalRecordTx[T, Constraint]) {
	txs := req.txs
	validTxs := make([]*T, 0)

	traceRejectTxs := func(txs []*T) {
		for i := 0; i < len(txs); i++ {
			traceRejectTx(ErrTxPoolFull.Error())
		}
	}
	overSpaceTxs := make([]*T, 0)
	defer func() {
		if len(overSpaceTxs) > 0 {
			traceRejectTxs(overSpaceTxs)
		}
		req.ch <- len(validTxs)
	}()

	// if tx pool is full, reject the left txs
	if len(p.txStore.txHashMap)+len(txs) > int(p.poolMaxSize) {
		if len(p.txStore.txHashMap) < int(p.poolMaxSize) {
			remainSpace := int(p.poolMaxSize) - len(p.txStore.txHashMap)
			overSpaceTxs = txs[remainSpace:]
			txs = txs[:remainSpace]
		}
	}
	if p.statusMgr.In(PoolFull) {
		overSpaceTxs = txs
		return
	}

	lo.ForEach(txs, func(tx *T, i int) {
		replaced, err := p.addTx(tx, true)
		// omit add record txs error
		if err == nil {
			p.updateValidTxs(&validTxs, tx, replaced)
		}
	})
}

func (p *txPoolImpl[T, Constraint]) postConsensusSignal(validTxs []*T) {
	// when primary generate batch, reset notifyGenerateBatch flag
	if p.txStore.priorityNonBatchSize >= p.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum && !p.notifyGenerateBatch {
		p.logger.Infof("notify generate batch")
		p.notifyGenerateBatchFn(commonpool.GenBatchSizeEvent)
		p.notifyGenerateBatch = true
	}

	// notify find next batch for replica
	completionMissingBatchHashes := p.checkIfCompleteMissingBatch(validTxs)
	if len(completionMissingBatchHashes) != 0 {
		p.logger.Infof("notify find next batch")
		p.notifyFindNextBatchFn(completionMissingBatchHashes...)
	}
}

func (p *txPoolImpl[T, Constraint]) checkIfCompleteMissingBatch(txs []*T) []string {
	completionMissingBatchHashes := make([]string, 0)
	// for replica, add validTxs to nonBatchedTxs and check if the missing transactions and batches are fetched
	lo.ForEach(txs, func(tx *T, _ int) {
		for batchHash, missingTxs := range p.txStore.missingBatch {
			for i, missingTxHash := range missingTxs {
				// we have received a tx which we are fetching.
				if Constraint(tx).RbftGetTxHash() == missingTxHash {
					delete(missingTxs, i)
				}
			}

			// we receive all missing txs
			if len(missingTxs) == 0 {
				delete(p.txStore.missingBatch, batchHash)
				completionMissingBatchHashes = append(completionMissingBatchHashes, batchHash)
			}
		}
	})
	return completionMissingBatchHashes
}

func (p *txPoolImpl[T, Constraint]) genHighNonceEvent(tx *T) *removeTxsEvent {
	account := Constraint(tx).RbftGetFrom()
	removeEvent := &removeTxsEvent{
		EventType: highNonceTxsEvent,
		Event: &reqHighNonceTxs{
			account:   account,
			highNonce: p.txStore.nonceCache.getPendingNonce(account),
		},
	}
	return removeEvent
}

func (p *txPoolImpl[T, Constraint]) dispatchRemoveTxsEvent(event *removeTxsEvent) {
	p.logger.Debugf("start dispatch remove txs event:%s", removeTxsEventToStr[event.EventType])
	metricsPrefix := "removeTxs_"
	start := time.Now()
	defer func() {
		traceProcessEvent(fmt.Sprintf("%s%s", metricsPrefix, removeTxsEventToStr[event.EventType]), time.Since(start))
		// p.logger.WithFields(logrus.Fields{"cost": time.Since(start)}).Debugf(
		//	"end dispatch remove txs event:%s", removeTxsEventToStr[event.EventType])
	}()
	var (
		removeCount int
		err         error
	)
	switch event.EventType {
	case highNonceTxsEvent:
		req := event.Event.(*reqHighNonceTxs)
		removeCount, err = p.removeHighNonceTxsByAccount(req.account, req.highNonce)
		if err != nil {
			p.logger.Warningf("remove high nonce txs by account failed: %s", err)
		}
		if removeCount > 0 {
			p.logger.Debugf("successfully remove high nonce txs by account: %s, count: %d", req.account, removeCount)
			traceRemovedTx("highNonce", removeCount)
		}
	case timeoutTxsEvent:
		removeCount = p.handleRemoveTimeoutTxs()
		if removeCount > 0 {
			p.logger.Infof("Successful remove timeout txs, count: %d", removeCount)
			traceRemovedTx("timeout", removeCount)
		}
	case committedTxsEvent:
		removeCount = p.handleRemoveStateUpdatingTxs(event.Event.(*reqRemoveCommittedTxs).txPointerList)
		if removeCount > 0 {
			p.logger.Infof("Successfully remove committed txs, count: %d", removeCount)
			traceRemovedTx("committed", removeCount)
		}
	case batchedTxsEvent:
		removeCount = p.handleRemoveBatches(event.Event.(*reqRemoveBatchedTxs).batchHashList)
		if removeCount > 0 {
			p.logger.Infof("Successfully remove batched txs, count: %d", removeCount)
			traceRemovedTx("batched", removeCount)
		}
	case invalidTxsEvent:
		removeCount = p.handleRemoveInvalidTxs(event.Event.(*reqRemoveInvalidTxs[T, Constraint]).removeTxs)
		if removeCount > 0 {
			p.logger.Infof("Successfully remove gas too low txs, count: %d", removeCount)
			traceRemovedTx("invalid", removeCount)
		}
	default:
		p.logger.Warningf("unknown removeTxs event type: %d", event.EventType)
	}
	if !p.checkPoolFull() {
		p.setNotFull()
	}
}

// remove invalid txs(invalid signature or gasPrice too low)
func (p *txPoolImpl[T, Constraint]) handleRemoveInvalidTxs(removeTxsM map[string]*internalTransaction[T, Constraint]) int {
	updateAccounts := make(map[string]uint64)
	removeCount := 0
	removePriorityCount := 0
	for account, revertTx := range removeTxsM {
		if revertTx == nil {
			continue
		}
		revertNonce := revertTx.getNonce()

		if list, ok := p.txStore.allTxs[account]; ok {
			// 1. remove invalid tx from pool store
			if err := p.cleanTxsByAccount(account, list, []*internalTransaction[T, Constraint]{revertTx}, true); err != nil {
				p.logger.Errorf("cleanTxsByAccount failed: %s", err)
			} else {
				removeCount++
				if !p.enablePricePriority {
					removePriorityCount++
				}
			}
		} else {
			p.logger.Errorf("account %s not found in pool", account)
			continue
		}

		if p.enablePricePriority {
			removePriorityTxs := p.txStore.priorityByPrice.removeTxBehindNonce(revertTx)
			removePriorityCount += len(removePriorityTxs)
		} else {
			// 2. because we remove low nonce txs from priority minNonceQueue, we need to remove txs which bigger than revertNonce from priority minNonceQueue
			removePriorityTxs := make([]*internalTransaction[T, Constraint], 0)
			p.txStore.priorityByTime.data.Ascend(func(a btree.Item) bool {
				tx := a.(*orderedIndexKey)
				if tx.account == account && tx.nonce > revertNonce {
					poolTx := p.txStore.getPoolTxByTxnPointer(tx.account, tx.nonce)
					removePriorityTxs = append(removePriorityTxs, poolTx)
				}
				return true
			})
			if err := p.txStore.priorityByTime.removeBatchKeys(account, removePriorityTxs); err != nil {
				p.logger.Errorf("removeBatchKeys failed: %s", err)
			}
			removePriorityCount += len(removePriorityTxs)
		}

		// 3. because we remove GasTooLow txs from priority minNonceQueue, we need to revert the pending nonce
		p.revertPendingNonce(&txPointer{account: account, nonce: revertNonce}, updateAccounts)
	}

	// 4. decrease nonBatchSize, if removePriorityCount > nonBatchSize, set nonBatchSize = 0
	if p.txStore.priorityNonBatchSize < uint64(removePriorityCount) {
		p.logger.Errorf("decrease nonBatchSize error, want decrease to %d, actual size %d", removePriorityCount, p.txStore.priorityNonBatchSize)
		p.setPriorityNonBatchSize(0)
	} else {
		p.decreasePriorityNonBatchSize(uint64(removePriorityCount))
	}

	for account, pendingNonce := range updateAccounts {
		p.logger.Debugf("Account %s revert it's pendingNonce to %d", account, pendingNonce)
	}
	return removeCount
}

func (p *txPoolImpl[T, Constraint]) dispatchBatchEvent(event *batchEvent) []txPoolEvent {
	// p.logger.Debugf("start dispatch batch event:%s", batchEventToStr[event.EventType])
	start := time.Now()
	metricsPrefix := "batch_"
	defer func() {
		traceProcessEvent(fmt.Sprintf("%s%s", metricsPrefix, batchEventToStr[event.EventType]), time.Since(start))
		// p.logger.WithFields(logrus.Fields{"cost": time.Since(start)}).Debugf("end dispatch batch event:%d", event.EventType)
	}()
	switch event.EventType {
	case commonpool.ReplyBatchSignalEvent:
		if p.notifyGenerateBatch {
			p.notifyGenerateBatch = false
		}
	case commonpool.GenBatchTimeoutEvent, commonpool.GenBatchFirstEvent, commonpool.GenBatchSizeEvent, commonpool.GenBatchNoTxTimeoutEvent:
		// it means primary receive the notify signal, and trigger the generate batch size event
		// we need reset the notify flag

		// if receive GenBatchFirstEvent, it means the new primary is elected, and it could generate batch,
		// so we need reset the notify flag in case of the new primary's txpool exist many txs

		removeEvent, err := p.handleGenBatchRequest(event)
		if err != nil {
			p.logger.Warning(err)
		}
		if len(removeEvent) > 0 {
			return removeEvent
		}
	case commonpool.ReConstructBatchEvent:
		req := event.Event.(*reqReConstructBatch[T, Constraint])
		deDuplicateTxHashes, err := p.handleReConstructBatchByOrder(req.oldBatch)
		if err != nil {
			req.respCh <- &respReConstructBatch{err: err}
		} else {
			req.respCh <- &respReConstructBatch{
				deDuplicateTxHashes: deDuplicateTxHashes,
			}
		}
		return nil
	case commonpool.GetTxsForGenBatchEvent:
		req := event.Event.(*reqGetTxsForGenBatch[T, Constraint])
		txs, localList, missingTxsHash, err := p.handleGetRequestsByHashList(req.batchHash, req.timestamp, req.hashList, req.deDuplicateTxHashes)
		if err != nil {
			req.respCh <- &respGetTxsForGenBatch[T, Constraint]{err: err}
		} else {
			resp := &respGetTxsForGenBatch[T, Constraint]{
				txs:            txs,
				localList:      localList,
				missingTxsHash: missingTxsHash,
			}
			req.respCh <- resp
		}
		return nil
	default:
		p.logger.Warningf("unknown generate batch event type: %s", batchEventToStr[event.EventType])
		return nil
	}

	return nil
}

func (p *txPoolImpl[T, Constraint]) handleGenBatchRequest(event *batchEvent) ([]txPoolEvent, error) {
	req := event.Event.(*reqGenBatch[T, Constraint])
	removeTxs, batch, err := p.handleGenerateRequestBatch(event.EventType)
	poolEvent := make([]txPoolEvent, 0)
	if len(removeTxs) > 0 {
		removeInvalidEvent := &removeTxsEvent{
			EventType: invalidTxsEvent,
			Event: &reqRemoveInvalidTxs[T, Constraint]{
				removeTxs: removeTxs,
			},
		}
		poolEvent = append(poolEvent, removeInvalidEvent)
	}
	respBatch := &respGenBatch[T, Constraint]{}
	if err != nil {
		respBatch.err = err
		req.respCh <- respBatch
		return poolEvent, err
	}
	respBatch.resp = batch
	req.respCh <- respBatch

	return poolEvent, nil
}

func (p *txPoolImpl[T, Constraint]) dispatchPoolInfoEvent(event *poolInfoEvent) {
	// p.logger.Debugf("start dispatch get pool info event:%s", poolInfoEventToStr[event.EventType])
	metricsPrefix := "poolInfo_"
	start := time.Now()
	defer func() {
		// p.logger.WithFields(logrus.Fields{"cost": time.Since(start)}).Debugf(
		//	"end dispatch get pool info event:%s", poolInfoEventToStr[event.EventType])
		traceProcessEvent(fmt.Sprintf("%s%s", metricsPrefix, poolInfoEventToStr[event.EventType]), time.Since(start))
	}()
	switch event.EventType {
	case reqPendingTxCountEvent:
		req := event.Event.(*reqPendingTxCountMsg)
		req.ch <- p.handleGetTotalPendingTxCount()
	case reqNonceEvent:
		req := event.Event.(*reqNonceMsg)
		req.ch <- p.handleGetPendingTxCountByAccount(req.account)
	case reqTxEvent:
		req := event.Event.(*reqTxMsg[T, Constraint])
		req.ch <- p.handleGetPendingTxByHash(req.hash)
	case reqAccountMetaEvent:
		req := event.Event.(*reqAccountPoolMetaMsg[T, Constraint])
		req.ch <- p.handleGetAccountMeta(req.account, req.full)
	case reqPoolMetaEvent:
		req := event.Event.(*reqPoolMetaMsg[T, Constraint])
		req.ch <- p.handleGetMeta(req.full)
	}
}

func (p *txPoolImpl[T, Constraint]) dispatchConsensusEvent(event *consensusEvent) {
	// p.logger.Debugf("start dispatch consensus event:%s", consensusEventToStr[event.EventType])
	start := time.Now()
	metricsPrefix := "consensus_"
	defer func() {
		traceProcessEvent(fmt.Sprintf("%s%s", metricsPrefix, consensusEventToStr[event.EventType]), time.Since(start))
		// p.logger.WithFields(logrus.Fields{"cost": time.Since(start)}).Debugf(
		//	"end dispatch consensus event:%s", consensusEventToStr[event.EventType])
	}()

	switch event.EventType {
	case SendMissingTxsEvent:
		req := event.Event.(*reqSendMissingTxs[T, Constraint])
		txs, err := p.handleSendMissingRequests(req.batchHash, req.missingHashList)
		req.respCh <- &respSendMissingTxs[T, Constraint]{
			resp: txs,
			err:  err,
		}
	case FilterReBroadcastTxsEvent:
		req := event.Event.(*reqFilterReBroadcastTxs[T, Constraint])
		txs := p.handleFilterOutOfDateRequests(req.timeout)
		req.respCh <- txs
	case RestoreOneBatchEvent:
		req := event.Event.(*reqRestoreOneBatch)
		err := p.handleRestoreOneBatch(req.batchHash)
		req.errCh <- err
	case RestoreAllBatchedEvent:
		p.handleRestorePool()
	}
}

func (p *txPoolImpl[T, Constraint]) dispatchLocalEvent(event *localEvent) {
	// p.logger.Debugf("start dispatch local event:%s", localEventToStr[event.EventType])
	start := time.Now()
	metricsPrefix := "localEvent_"
	defer func() {
		traceProcessEvent(fmt.Sprintf("%s%s", metricsPrefix, localEventToStr[event.EventType]), time.Since(start))
		// p.logger.WithFields(logrus.Fields{"cost": time.Since(start)}).Debugf(
		//	"end dispatch local event:%s", localEventToStr[event.EventType])
	}()
	switch event.EventType {
	case gcAccountEvent:
		count := p.handleGcAccountEvent()
		if count > 0 {
			p.logger.Debugf("handle gc account event, count: %d", count)
		}
	case rotateTxLocalsEvent:
		err := p.handleRotateTxLocalsEvent()
		if err != nil {
			p.logger.Errorf("handle rotate tx locals event failed: %s", err)
		}
		p.logger.Debugf("handle rotate tx locals event")
	}
}

func (p *txPoolImpl[T, Constraint]) handleGcAccountEvent() int {
	dirtyAccount := make([]string, 0)
	now := time.Now().UnixNano()
	for account, list := range p.txStore.allTxs {
		if list.checkIfGc(now, p.cleanEmptyAccountTime.Nanoseconds()) {
			dirtyAccount = append(dirtyAccount, account)
		}
	}

	lo.ForEach(dirtyAccount, func(account string, _ int) {
		p.cleanAccountInCache(account)
	})
	return len(dirtyAccount)
}

func (p *txPoolImpl[T, Constraint]) cleanAccountInCache(account string) {
	delete(p.txStore.allTxs, account)
	delete(p.txStore.nonceCache.commitNonces, account)
	delete(p.txStore.nonceCache.pendingNonces, account)
}

func (p *txPoolImpl[T, Constraint]) postEvent(event txPoolEvent) {
	p.revCh <- event
}

func (p *txPoolImpl[T, Constraint]) AddLocalTx(tx *T) error {
	req := &reqLocalTx[T, Constraint]{
		tx:    tx,
		errCh: make(chan error),
	}

	ev := &addTxsEvent{
		EventType: localTxEvent,
		Event:     req,
	}
	p.postEvent(ev)

	return <-req.errCh
}

func (p *txPoolImpl[T, Constraint]) addLocalRecordTx(txs []*T) int {
	req := &reqLocalRecordTx[T, Constraint]{
		txs: txs,
		ch:  make(chan int),
	}

	ev := &addTxsEvent{
		EventType: localRecordTxEvent,
		Event:     req,
	}
	p.postEvent(ev)

	return <-req.ch
}

func (p *txPoolImpl[T, Constraint]) AddRemoteTxs(txs []*T) {
	req := &reqRemoteTxs[T, Constraint]{
		txs: txs,
	}

	ev := &addTxsEvent{
		EventType: remoteTxsEvent,
		Event:     req,
	}

	p.postEvent(ev)
}

func (p *txPoolImpl[T, Constraint]) AddRebroadcastTxs(txs []*T) {
	req := &reqRemoteTxs[T, Constraint]{
		txs: txs,
	}

	ev := &addTxsEvent{
		EventType: reBroadcastTxsEvent,
		Event:     req,
	}

	p.postEvent(ev)
}

func (p *txPoolImpl[T, Constraint]) addTx(tx *T, local bool) (bool, error) {
	txAccount := Constraint(tx).RbftGetFrom()
	txHash := Constraint(tx).RbftGetTxHash()
	txNonce := Constraint(tx).RbftGetNonce()
	gasPrice := Constraint(tx).RbftGetGasPrice()
	var (
		needReplace bool
		replaced    bool
	)

	currentSeqNo := p.txStore.nonceCache.getPendingNonce(txAccount)

	// 1. validate tx
	err := p.validateTx(txHash, txNonce, currentSeqNo, gasPrice)
	if errors.Is(err, ErrNonceTooLow) || err == nil {
		// if exist old tx with same nonce, replace it
		if p.txStore.allTxs[txAccount] != nil {
			oldTx, exist := p.txStore.allTxs[txAccount].items[txNonce]
			if exist {
				if p.isValidPriceBump(oldTx.getGasPrice(), gasPrice) {
					p.logger.Warningf("Receive duplicate nonce transaction [account: %s, nonce: %d, hash: %s],"+
						" will replace old tx[hash: %s]", txAccount, txNonce, txHash, oldTx.getHash())
					needReplace = true
					err = nil
				} else {
					err = ErrBelowPriceBump
				}
			}
		}
	}
	if err != nil {
		traceRejectTx(err.Error())
		return false, err
	}

	if needReplace {
		replaced = p.replaceTx(tx, local)
	} else {
		// 3. insert new tx into txHashMap、allTxs、localTTLIndex、removeTTLIndex
		now := time.Now().UnixNano()
		txItem := &internalTransaction[T, Constraint]{
			rawTx:       tx,
			local:       local,
			lifeTime:    Constraint(tx).RbftGetTimeStamp(),
			arrivedTime: now,
		}
		p.txStore.insertTxInPool(txItem, local)

		// 4. update pending or queue
		// if we receive valid tx which we wanted, update pendingNonce、priorityByTime or parkingLotIndex
		p.processDirtyAccount(txAccount, txItem, txNonce == currentSeqNo)
	}

	return replaced, nil
}

func (p *txPoolImpl[T, Constraint]) validateTx(txHash string, txNonce, currentSeqNo uint64, gasPrice *big.Int) error {
	// 1. reject duplicate tx
	if pointer := p.txStore.txHashMap[txHash]; pointer != nil {
		return ErrDuplicateTx
	}

	// 2. reject nonce too high tx, trigger remove all high nonce txs of account outbound
	if txNonce > currentSeqNo+p.toleranceNonceGap {
		return errors.Wrapf(ErrNonceTooHigh, "txNonce: %d, expect nonce: %d, toleranceNonceGap: %d", txNonce, currentSeqNo, p.toleranceNonceGap)
	}

	// 3. reject nonce too low tx(except gas price bump tx)
	if txNonce < currentSeqNo {
		return errors.Wrapf(ErrNonceTooLow, "txNonce: %d, expect nonce: %d", txNonce, currentSeqNo)
	}

	// 4. reject tx with gas price lower than price limit
	if gasPrice.Cmp(p.getPriceLimit()) < 0 {
		return errors.Wrapf(ErrGasPriceTooLow, "tx gas price %s is lower than price limit: %s", gasPrice.String(), p.priceLimit.Load().String())
	}

	return nil
}

func (p *txPoolImpl[T, Constraint]) handleRemoveTimeoutTxs() int {
	now := time.Now().UnixNano()
	removedTxs := make(map[string][]*internalTransaction[T, Constraint])
	existTxs := make(map[string]map[uint64]bool)
	var (
		index      int
		readyCount int
	)
	clearedAccount := make(map[string]bool)
	p.txStore.removeTTLIndex.data.Ascend(func(a btree.Item) bool {
		index++
		removeKey := a.(*orderedIndexKey)
		txAccount := removeKey.account
		txNonce := removeKey.nonce
		poolTx := p.txStore.getPoolTxByTxnPointer(txAccount, txNonce)
		if poolTx == nil {
			p.logger.Errorf("Get nil poolTx from txStore:[account:%s, nonce:%d]", txAccount, txNonce)
			return true
		}
		if now-poolTx.arrivedTime > p.toleranceRemoveTime.Nanoseconds() {
			// for those batched txs, we don't need to removedTxs temporarily.
			pointer := &txPointer{account: txAccount, nonce: txNonce}
			if _, ok := p.txStore.batchedTxs[*pointer]; ok {
				return true
			}

			orderedKey := &orderedIndexKey{time: poolTx.getRawTimestamp(), account: poolTx.getAccount(), nonce: poolTx.getNonce()}
			commitNonce := p.txStore.nonceCache.getCommitNonce(txAccount)

			// for priority txs, we don't need to removedTxs temporarily except for the lower commit nonce txs
			if p.enablePricePriority && !clearedAccount[txAccount] {
				// commit nonce equal to the committed tx count in ledger
				var cleanNonce uint64
				if commitNonce > 0 {
					cleanNonce = commitNonce - 1
				}
				removed := p.txStore.priorityByPrice.removeTxBeforeNonce(txAccount, cleanNonce)
				removedTxs[txAccount] = removed
				clearedAccount[txAccount] = true
				readyCount += len(removed)
			} else {
				if tx := p.txStore.priorityByTime.data.Get(orderedKey); tx != nil {
					if txNonce < commitNonce {
						if p.fillRemoveTxs(orderedKey, poolTx, removedTxs, existTxs) {
							readyCount++
						}
					}
					return true
				}
			}
			// if removed priority txs, we need to decrease priorityNonBatchSize
			p.decreasePriorityNonBatchSize(uint64(readyCount))

			deleteTx := func(index *btreeIndex[T, Constraint]) bool {
				if tx := index.data.Get(orderedKey); tx != nil {
					p.fillRemoveTxs(orderedKey, poolTx, removedTxs, existTxs)
					return true
				}
				return false
			}

			return deleteTx(p.txStore.parkingLotIndex)
		}
		return false
	})
	for account, txs := range removedTxs {
		if list, ok := p.txStore.allTxs[account]; ok {
			// remove index from removedTxs
			_ = p.cleanTxsByAccount(account, list, txs, readyCount > 0)
		}
	}

	return len(removedTxs)
}

// GetUncommittedTransactions returns the uncommitted transactions.
// not used
func (p *txPoolImpl[T, Constraint]) GetUncommittedTransactions(_ uint64) []*T {
	return []*T{}
}

func (p *txPoolImpl[T, Constraint]) Stop() {
	p.timerMgr.Stop()
	p.cancel()
	p.wg.Wait()
	if p.txRecords != nil {
		if err := p.txRecords.close(); err != nil {
			p.logger.Errorf("Failed to close txRecords: %v", err)
		}
	}
	p.started.Store(false)
	p.logger.Infof("TxPool stopped!!!")
}

func (p *txPoolImpl[T, Constraint]) setPriceLimit(price uint64) {
	p.priceLimit.Store(new(big.Int).SetUint64(price))
}

func (p *txPoolImpl[T, Constraint]) getPriceLimit() *big.Int {
	return p.priceLimit.Load()
}

// newTxPoolImpl returns the txpool instance.
func newTxPoolImpl[T any, Constraint types.TXConstraint[T]](config Config, chainState *chainstate.ChainState) (*txPoolImpl[T, Constraint], error) {
	// check config parameters
	config.sanitize()
	ctx, cancel := context.WithCancel(context.Background())

	txpoolImp := &txPoolImpl[T, Constraint]{
		logger:            config.Logger,
		chainState:        chainState,
		getAccountNonce:   config.GetAccountNonce,
		getAccountBalance: config.GetAccountBalance,
		revCh:             make(chan txPoolEvent, maxChanSize),

		toleranceTime:          config.ToleranceTime,
		toleranceNonceGap:      config.ToleranceNonceGap,
		toleranceRemoveTime:    config.ToleranceRemoveTime,
		cleanEmptyAccountTime:  config.CleanEmptyAccountTime,
		poolMaxSize:            config.PoolSize,
		rotateTxLocalsInterval: config.RotateTxLocalsInterval,
		PriceBump:              config.PriceBump,

		statusMgr: status.NewStatusMgr(),

		ctx:    ctx,
		cancel: cancel,
	}

	txpoolImp.txStore = newTransactionStore[T, Constraint](config.GetAccountNonce, config.Logger)

	txpoolImp.enableLocalsPersist = config.EnableLocalsPersist
	txpoolImp.txRecordsFile = path.Join(repo.GetStoragePath(config.RepoRoot, storagemgr.TxPool), TxRecordsFile)
	if txpoolImp.enableLocalsPersist {
		txpoolImp.txRecords = newTxRecords[T, Constraint](txpoolImp.txRecordsFile, config.Logger)
	}
	if config.GenerateBatchType == repo.GenerateBatchByGasPrice {
		txpoolImp.enablePricePriority = true
	}

	// init timer for remove tx
	txpoolImp.timerMgr = timer.NewTimerManager(txpoolImp.logger)
	err := txpoolImp.timerMgr.CreateTimer(RemoveTx, txpoolImp.toleranceRemoveTime, txpoolImp.handleRemoveTimeout)
	if err != nil {
		return nil, err
	}
	err = txpoolImp.timerMgr.CreateTimer(CleanEmptyAccount, txpoolImp.cleanEmptyAccountTime, txpoolImp.handleRemoveTimeout)
	if err != nil {
		return nil, err
	}
	if txpoolImp.enableLocalsPersist {
		if !fileutil.ExistDir(path.Dir(txpoolImp.txRecordsFile)) {
			err = os.MkdirAll(filepath.Dir(txpoolImp.txRecordsFile), 0755)
			if err != nil {
				return nil, err
			}
		}

		if !fileutil.Exist(txpoolImp.txRecordsFile) {
			_, err = os.Create(txpoolImp.txRecordsFile)
			if err != nil {
				return nil, err
			}
		}
		err = txpoolImp.timerMgr.CreateTimer(RotateTxLocals, txpoolImp.rotateTxLocalsInterval, txpoolImp.handleRemoveTimeout)
		if err != nil {
			return nil, err
		}
	}

	txpoolImp.setPriceLimit(config.PriceLimit)

	txpoolImp.logger.Infof("TxPool pool size = %d", txpoolImp.poolMaxSize)
	txpoolImp.logger.Infof("TxPool batch size = %d", txpoolImp.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum)
	txpoolImp.logger.Infof("TxPool enable generate empty batch = %v", txpoolImp.chainState.EpochInfo.ConsensusParams.EnableTimedGenEmptyBlock)
	txpoolImp.logger.Infof("TxPool tolerance time = %v", txpoolImp.toleranceTime)
	txpoolImp.logger.Infof("TxPool tolerance remove time = %v", txpoolImp.toleranceRemoveTime)
	txpoolImp.logger.Infof("TxPool tolerance nonce gap = %d", txpoolImp.toleranceNonceGap)
	txpoolImp.logger.Infof("TxPool clean empty account time = %v", txpoolImp.cleanEmptyAccountTime)
	txpoolImp.logger.Infof("TxPool rotate tx locals interval = %v", txpoolImp.rotateTxLocalsInterval)
	txpoolImp.logger.Infof("TxPool enable locals persist = %v", txpoolImp.enableLocalsPersist)
	txpoolImp.logger.Infof("TxPool tx records file = %s", txpoolImp.txRecordsFile)
	txpoolImp.logger.Infof("TxPool price limit = %v, priceBump = %v", txpoolImp.getPriceLimit(), txpoolImp.PriceBump)
	txpoolImp.logger.Infof("TxPool enable price priority = %v", txpoolImp.enablePricePriority)
	return txpoolImp, nil
}

func (p *txPoolImpl[T, Constraint]) ReplyBatchSignal() {
	ev := &batchEvent{
		EventType: commonpool.ReplyBatchSignalEvent,
	}
	p.postEvent(ev)
}

func (p *txPoolImpl[T, Constraint]) Init(conf commonpool.ConsensusConfig) {
	p.selfID = conf.SelfID
	p.notifyGenerateBatchFn = conf.NotifyGenerateBatchFn
	p.notifyFindNextBatchFn = conf.NotifyFindNextBatchFn

	if p.enableLocalsPersist {
		if err := p.processRecords(); err != nil {
			p.logger.Errorf("Failed to process records: %v", err)
		}
		if err := p.txRecords.rotate(p.txStore.allTxs); err != nil {
			p.logger.Errorf("Failed to rotate records: %v", err)
		}
	}

}

// GenerateRequestBatch generates a transaction batch and post it
// to outside if there are transactions in common_pool.
func (p *txPoolImpl[T, Constraint]) GenerateRequestBatch(typ int) (*commonpool.RequestHashBatch[T, Constraint], error) {
	return p.generateRequestBatch(typ)
}

// GenerateRequestBatch generates a transaction batch and post it
// to outside if there are transactions in common_pool.
func (p *txPoolImpl[T, Constraint]) generateRequestBatch(typ int) (*commonpool.RequestHashBatch[T, Constraint], error) {
	req := &reqGenBatch[T, Constraint]{
		respCh: make(chan *respGenBatch[T, Constraint]),
	}
	if typ != commonpool.GenBatchSizeEvent && typ != commonpool.GenBatchTimeoutEvent &&
		typ != commonpool.GenBatchNoTxTimeoutEvent && typ != commonpool.GenBatchFirstEvent {
		err := fmt.Errorf("invalid batch type %d", typ)
		return nil, err
	}
	ev := &batchEvent{
		EventType: typ,
		Event:     req,
	}
	p.postEvent(ev)

	resp := <-req.respCh
	if resp.err != nil {
		return nil, resp.err
	}

	return resp.resp, nil
}

func (p *txPoolImpl[T, Constraint]) validateTxData(tx *T) error {
	minGasPrice := p.chainState.EpochInfo.FinanceParams.MinGasPrice.ToBigInt()

	if !p.enablePricePriority {
		if Constraint(tx).RbftGetGasPrice().Cmp(minGasPrice) < 0 {
			return fmt.Errorf("tx gas price is lower than chain gas price, tx gas price = %s, chain gas price = %s", Constraint(tx).RbftGetGasPrice().String(), minGasPrice.String())
		}
	}

	return components.VerifyInsufficientBalance[T, Constraint](tx, p.getAccountBalance)
}

func (p *txPoolImpl[T, Constraint]) popExecutableTxs(size uint64, batch *commonpool.RequestHashBatch[T, Constraint]) map[string]*internalTransaction[T, Constraint] {
	if p.enablePricePriority {
		return p.popExecutableTxsByPrice(size, batch)
	}
	return p.popExecutableTxsByTime(size, batch)
}

func (p *txPoolImpl[T, Constraint]) popExecutableTxsByPrice(size uint64, batch *commonpool.RequestHashBatch[T, Constraint]) map[string]*internalTransaction[T, Constraint] {
	currentSize := uint64(0)
	removeInvalidTxs := make(map[string]*internalTransaction[T, Constraint])
	for p.txStore.priorityByPrice.txsByPrice.length() > 0 && currentSize < size {
		poolTx := p.txStore.priorityByPrice.peek()
		from := poolTx.getAccount()
		// validate tx Data:
		// 1. tx gas price should be bigger than chain gas price(omit in gas price priority mode)
		// 2. tx balance should be enough
		// if validation failed, put tx into removeInvalidTxs, trigger remove txs event from txpool
		if err := p.validateTxData(poolTx.rawTx); err != nil {
			p.logger.WithFields(logrus.Fields{
				"account": poolTx.getAccount(),
				"nonce":   poolTx.getNonce(),
				"err":     err,
			}).Warning("validate tx data failed")
			if _, ok := removeInvalidTxs[from]; !ok {
				removeInvalidTxs[from] = poolTx
				p.txStore.priorityByPrice.txsByPrice.pop()
			}
			continue
		}

		p.txStore.priorityByPrice.pop()
		batch.FillBatchItem(poolTx.rawTx, poolTx.local)
		p.txStore.batchedTxs[txPointer{account: poolTx.getAccount(), nonce: poolTx.getNonce()}] = true
		currentSize++
	}
	return removeInvalidTxs
}

func (p *txPoolImpl[T, Constraint]) popExecutableTxsByTime(batchSize uint64, txBatch *commonpool.RequestHashBatch[T, Constraint]) map[string]*internalTransaction[T, Constraint] {
	skippedTxs := make(map[txPointer]*internalTransaction[T, Constraint])
	removeInvalidTxs := make(map[string]*internalTransaction[T, Constraint])
	p.txStore.priorityByTime.data.Ascend(func(a btree.Item) bool {
		tx := a.(*orderedIndexKey)
		ptr := txPointer{account: tx.account, nonce: tx.nonce}
		poolTx := p.txStore.getPoolTxByTxnPointer(tx.account, tx.nonce)

		// if tx after removed tx's nonce, omit it(it will be removed in next loop)
		if _, ok := removeInvalidTxs[tx.account]; ok {
			return true
		}
		// validate tx Data:
		// 1. tx gas price should be bigger than chain gas price
		// 2. tx balance should be enough
		// if validation failed, put tx into removeInvalidTxs, trigger remove txs event from txpool
		if err := p.validateTxData(poolTx.rawTx); err != nil {
			p.logger.WithFields(logrus.Fields{
				"account": tx.account,
				"nonce":   tx.nonce,
				"err":     err,
			}).Warning("validate tx data failed")
			removeInvalidTxs[tx.account] = poolTx

			return true
		}
		// if tx has existed in bathedTxs
		if _, ok := p.txStore.batchedTxs[ptr]; ok {
			return true
		}
		txSeq := tx.nonce
		// p.logger.Debugf("txpool txNonce:%s-%d", tx.account, tx.nonce)
		commitNonce := p.txStore.nonceCache.getCommitNonce(tx.account)
		// p.logger.Debugf("ledger txNonce:%s-%d", tx.account, commitNonce)
		var seenPrevious bool
		if txSeq >= 1 {
			_, seenPrevious = p.txStore.batchedTxs[txPointer{account: tx.account, nonce: txSeq - 1}]
		}
		// include transaction if it's "next" for given account, or
		// we've already sent its ancestor to Consensus

		// commitNonce is the nonce of last committed tx for given account,
		if seenPrevious || (txSeq == commitNonce) {
			p.txStore.batchedTxs[ptr] = true
			txBatch.FillBatchItem(poolTx.rawTx, poolTx.local)
			if txBatch.BatchItemSize() == batchSize {
				return false
			}

			// check if we can now include some txs that were skipped before for given account
			skippedTxn := txPointer{account: tx.account, nonce: tx.nonce + 1}
			for {
				skippedPoolTx, ok := skippedTxs[skippedTxn]
				if !ok {
					break
				}
				p.txStore.batchedTxs[skippedTxn] = true
				txBatch.FillBatchItem(skippedPoolTx.rawTx, skippedPoolTx.local)
				if txBatch.BatchItemSize() == batchSize {
					return false
				}
				skippedTxn.nonce++
			}
		} else {
			skippedTxs[ptr] = poolTx
		}
		return true
	})
	return removeInvalidTxs
}

// handleGenerateRequestBatch fetches next block of transactions for consensus,
// batchedTx are all txs sent to consensus but were not committed yet, txpool should filter out such txs.
func (p *txPoolImpl[T, Constraint]) handleGenerateRequestBatch(typ int) (
	map[string]*internalTransaction[T, Constraint], *commonpool.RequestHashBatch[T, Constraint], error) {
	switch typ {
	case commonpool.GenBatchSizeEvent, commonpool.GenBatchFirstEvent:
		if p.txStore.priorityNonBatchSize < p.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum {
			return nil, nil, fmt.Errorf("actual batch size %d is smaller than %d, ignore generate batch",
				p.txStore.priorityNonBatchSize, p.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum)
		}
	case commonpool.GenBatchTimeoutEvent:
		if !p.hasPendingRequestInPool() {
			return nil, nil, errors.New("there is no pending tx, ignore generate batch")
		}
	case commonpool.GenBatchNoTxTimeoutEvent:
		if p.hasPendingRequestInPool() {
			return nil, nil, errors.New("there is pending tx, ignore generate no tx batch")
		}
		if !p.chainState.EpochInfo.ConsensusParams.EnableTimedGenEmptyBlock {
			err := errors.New("not supported generate no tx batch")
			p.logger.Warning(err)
			return nil, nil, err
		}
	}

	txBatch := &commonpool.RequestHashBatch[T, Constraint]{
		TxHashList: make([]string, 0),
		TxList:     make([]*T, 0),
		LocalList:  make([]bool, 0),
	}

	// txs has lower nonce will be observed first in priority index iterator.
	p.logger.Debugf("Length of non-batched transactions: %d", p.txStore.priorityNonBatchSize)
	var batchSize uint64
	if p.txStore.priorityNonBatchSize > p.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum {
		batchSize = p.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum
	} else {
		batchSize = p.txStore.priorityNonBatchSize
	}

	// get executable txs
	removeInvalidTxs := p.popExecutableTxs(batchSize, txBatch)

	if !p.chainState.EpochInfo.ConsensusParams.EnableTimedGenEmptyBlock && txBatch.BatchItemSize() == 0 && len(removeInvalidTxs) == 0 && p.hasPendingRequestInPool() {
		err := fmt.Errorf("===== Note!!! Primary generate a batch with 0 txs, "+
			"but PriorityNonBatchSize is %d, we need reset PriorityNonBatchSize", p.txStore.priorityNonBatchSize)
		p.logger.Warning(err.Error())
		p.setPriorityNonBatchSize(0)
		return removeInvalidTxs, nil, err
	}

	if typ != commonpool.GenBatchNoTxTimeoutEvent && txBatch.BatchItemSize() == 0 {
		return removeInvalidTxs, nil, errors.New("there is no valid tx to generate batch")
	}
	txBatch.Timestamp = time.Now().UnixNano()
	batchHash := txBatch.GenerateBatchHash()
	txBatch.BatchHash = batchHash
	p.txStore.batchesCache[batchHash] = txBatch

	// reset PriorityNonBatchSize
	if p.txStore.priorityNonBatchSize <= txBatch.BatchItemSize() {
		p.setPriorityNonBatchSize(0)
	} else {
		p.decreasePriorityNonBatchSize(txBatch.BatchItemSize())
	}
	p.logger.Debugf("Primary generate a batch with %d txs, which hash is %s, and now there are %d "+
		"pending txs and %d batches in txPool, pending Status: %v", txBatch.BatchItemSize(),
		batchHash, p.txStore.priorityNonBatchSize, len(p.txStore.batchesCache), p.hasPendingRequestInPool())
	return removeInvalidTxs, txBatch, nil
}

// RestoreOneBatch moves one batch from batchStore.
func (p *txPoolImpl[T, Constraint]) RestoreOneBatch(batchHash string) error {
	req := &reqRestoreOneBatch{
		batchHash: batchHash,
		errCh:     make(chan error),
	}
	ev := &consensusEvent{
		EventType: RestoreOneBatchEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.errCh
}

func (p *txPoolImpl[T, Constraint]) handleRestoreOneBatch(batchHash string) error {
	batch, ok := p.txStore.batchesCache[batchHash]
	if !ok {
		return errors.New("can't find batch from batchesCache")
	}

	if err := p.putBackBatchedTxs(batch); err != nil {
		return err
	}

	p.logger.Debugf("Restore one batch, which hash is %s, now there are %d non-batched txs, "+
		"%d batches in txPool", batchHash, p.txStore.priorityNonBatchSize, len(p.txStore.batchesCache))
	return nil
}

func (p *txPoolImpl[T, Constraint]) RemoveBatches(batchHashList []string) {
	req := &reqRemoveBatchedTxs{batchHashList: batchHashList}
	ev := &removeTxsEvent{
		EventType: batchedTxsEvent,
		Event:     req,
	}
	p.postEvent(ev)
}

// RemoveBatches removes several batches by given digests of
// transaction batches from the pool(batchedTxs).
func (p *txPoolImpl[T, Constraint]) handleRemoveBatches(batchHashList []string) int {
	priorityLen := p.txStore.priorityByTime.size()
	if p.enablePricePriority {
		priorityLen = int(p.txStore.priorityByPrice.size())
	}
	// update current cached commit nonce for account
	p.logger.Debugf("RemoveBatches: batch len:%d", len(batchHashList))
	p.logger.Debugf("Before RemoveBatches in txpool, and now there are %d non-batched txs, %d batches, "+
		"priority len: %d, parkingLot len: %d, parkingLot size len: %d, batchedTx len: %d, txHashMap len: %d", p.txStore.priorityNonBatchSize,
		len(p.txStore.batchesCache), priorityLen, p.txStore.parkingLotIndex.size(), p.txStore.parkingLotSize,
		len(p.txStore.batchedTxs), len(p.txStore.txHashMap))
	var count int
	updateAccounts := make(map[string]uint64)
	for _, batchHash := range batchHashList {
		batch, ok := p.txStore.batchesCache[batchHash]
		if !ok {
			p.logger.Debugf("Cannot find batch %s in txpool batchedCache which may have been "+
				"discard when ReConstructBatchByOrder", batchHash)
			continue
		}
		delete(p.txStore.batchesCache, batchHash)
		dirtyAccounts := make(map[string]bool)
		for _, txHash := range batch.TxHashList {
			pointer, ok := p.txStore.txHashMap[txHash]
			// if we are not found the tx in txHashMap, it means the tx has been removed
			if !ok {
				continue
			}
			p.updateNonceCache(pointer, updateAccounts)
			delete(p.txStore.batchedTxs, *pointer)
			dirtyAccounts[pointer.account] = true
			count++
		}
		// clean related txs info in cache
		for account := range dirtyAccounts {
			if err := p.cleanTxsBeforeCommitNonce(account, p.txStore.nonceCache.getCommitNonce(account)); err != nil {
				p.logger.Errorf("cleanTxsBeforeCommitNonce error: %v", err)
			}
		}
	}

	var readyNum uint64
	if p.enablePricePriority {
		readyNum = p.txStore.priorityByPrice.size()
	} else {
		readyNum = uint64(p.txStore.priorityByTime.size())
	}
	// set nonBatchSize to min(nonBatchedTxs, readyNum),
	if p.txStore.priorityNonBatchSize > readyNum {
		p.logger.Debugf("Set nonBatchSize from %d to the size of priorityByTime %d", p.txStore.priorityNonBatchSize, readyNum)
		p.setPriorityNonBatchSize(readyNum)
	}
	for account, pendingNonce := range updateAccounts {
		p.logger.Debugf("Account %s update its pendingNonce to %d by commitNonce", account, pendingNonce)
	}
	priorityLen = p.txStore.priorityByTime.size()
	if p.enablePricePriority {
		priorityLen = int(p.txStore.priorityByPrice.size())
	}
	p.logger.Infof("After Removes batches in txpool, and now there are %d non-batched txs, %d batches, "+
		"priority len: %d, parkingLot len: %d, parkingLot size len: %d, batchedTx len: %d, txHashMap len: %d", p.txStore.priorityNonBatchSize,
		len(p.txStore.batchesCache), priorityLen, p.txStore.parkingLotIndex.size(), p.txStore.parkingLotSize,
		len(p.txStore.batchedTxs), len(p.txStore.txHashMap))
	return count
}

func (p *txPoolImpl[T, Constraint]) cleanTxsBeforeCommitNonce(account string, commitNonce uint64) error {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if p.enablePricePriority {
			p.txStore.priorityByPrice.removeTxBeforeNonce(account, commitNonce-1)
		}
	}()

	var outErr error
	go func() {
		defer wg.Done()
		// clean related txs info in cache
		if list, ok := p.txStore.allTxs[account]; ok {
			// remove all previous seq number txs for this account.
			removedTxs := list.forward(commitNonce)
			// remove index smaller than commitNonce delete index.
			if err := p.cleanTxsByAccount(account, list, removedTxs, true); err != nil {
				outErr = err
			}
		}
	}()
	wg.Wait()
	return outErr
}

func (p *txPoolImpl[T, Constraint]) cleanTxsByAccount(account string, list *txSortedMap[T, Constraint], removedTxs []*internalTransaction[T, Constraint], clearPriority bool) error {
	var failed atomic.Bool
	var wg sync.WaitGroup
	wg.Add(5)
	go func(txs []*internalTransaction[T, Constraint], pendingNonce uint64) {
		defer wg.Done()
		lo.ForEach(txs, func(poolTx *internalTransaction[T, Constraint], _ int) {
			nonce := poolTx.getNonce()
			// tx which removed is belonged to parkingLot, we should decrease parkingLotSize
			if nonce > pendingNonce {
				p.txStore.decreaseParkingLotSize(1)
			}
			delete(p.txStore.batchedTxs, txPointer{account: account, nonce: poolTx.getNonce()})
			p.txStore.deletePoolTx(account, nonce)
		})
	}(removedTxs, p.txStore.nonceCache.getPendingNonce(account))
	go func(txs []*internalTransaction[T, Constraint]) {
		defer wg.Done()
		if err := p.txStore.parkingLotIndex.removeBatchKeys(account, txs); err != nil {
			p.logger.Errorf("remove parkingLotIndex error: %v", err)
			failed.Store(true)
		}
	}(removedTxs)
	go func(txs []*internalTransaction[T, Constraint]) {
		defer wg.Done()
		if err := p.txStore.localTTLIndex.removeBatchKeys(account, txs); err != nil {
			p.logger.Errorf("remove localTTLIndex error: %v", err)
			failed.Store(true)
		}
	}(removedTxs)
	go func(txs []*internalTransaction[T, Constraint]) {
		defer wg.Done()
		if err := p.txStore.removeTTLIndex.removeBatchKeys(account, txs); err != nil {
			p.logger.Errorf("remove removeTTLIndex error: %v", err)
			failed.Store(true)
		}
	}(removedTxs)

	go func(txs []*internalTransaction[T, Constraint]) {
		defer wg.Done()
		if clearPriority && !p.enablePricePriority {
			if err := p.txStore.priorityByTime.removeBatchKeys(account, txs); err != nil {
				p.logger.Errorf("remove priorityByTime error: %v", err)
				failed.Store(true)
			}
		}
	}(removedTxs)

	wg.Wait()

	if failed.Load() {
		return errors.New("failed to remove txs")
	}

	if len(list.items) == 0 {
		list.setEmpty()
	}

	return nil
}

func (p *txPoolImpl[T, Constraint]) revertPendingNonce(pointer *txPointer, updateAccounts map[string]uint64) {
	pendingNonce := p.txStore.nonceCache.getPendingNonce(pointer.account)
	// because we remove the tx from the txpool, so we need revert the pendingNonce to pointer.nonce
	// it means we want next nonce is pointer.nonce
	if pendingNonce > pointer.nonce {
		p.txStore.nonceCache.setPendingNonce(pointer.account, pointer.nonce)
		updateAccounts[pointer.account] = pointer.nonce
	}
}

func (p *txPoolImpl[T, Constraint]) updateNonceCache(pointer *txPointer, updateAccounts map[string]uint64) {
	preCommitNonce := p.txStore.nonceCache.getCommitNonce(pointer.account)
	// next wanted nonce
	newCommitNonce := pointer.nonce + 1
	if preCommitNonce < newCommitNonce {
		p.txStore.nonceCache.setCommitNonce(pointer.account, newCommitNonce)
		// Note!!! updating pendingNonce to commitNonce for the restart node
		pendingNonce, ok := p.txStore.nonceCache.pendingNonces[pointer.account]
		if !ok || pendingNonce < newCommitNonce {
			updateAccounts[pointer.account] = newCommitNonce
			p.txStore.nonceCache.setPendingNonce(pointer.account, newCommitNonce)
			if p.enablePricePriority {
				p.txStore.priorityByPrice.updateAccountNonce(pointer.account, newCommitNonce)
			}

		}
	}
}

func (p *txPoolImpl[T, Constraint]) RemoveStateUpdatingTxs(txPointerList []*commonpool.WrapperTxPointer) {
	req := &reqRemoveCommittedTxs{txPointerList: txPointerList}
	ev := &removeTxsEvent{
		EventType: committedTxsEvent,
		Event:     req,
	}
	p.postEvent(ev)
}

func (p *txPoolImpl[T, Constraint]) handleRemoveStateUpdatingTxs(txPointerList []*commonpool.WrapperTxPointer) int {
	p.logger.Debugf("start RemoveStateUpdatingTxs, len:%d", len(txPointerList))
	removeCount := 0
	dirtyAccounts := make(map[string]bool)
	updateAccounts := make(map[string]uint64)
	removeTxs := make(map[string][]*internalTransaction[T, Constraint])
	maxPriorityNonce := make(map[string]uint64)
	lo.ForEach(txPointerList, func(wrapperPointer *commonpool.WrapperTxPointer, _ int) {
		txHash := wrapperPointer.TxHash
		if pointer, ok := p.txStore.txHashMap[txHash]; ok {
			poolTx := p.txStore.getPoolTxByTxnPointer(pointer.account, pointer.nonce)
			if poolTx == nil {
				p.logger.Errorf("pool tx %s not found in txpool, but exists in txHashMap", txHash)
				return
			}
			if removeTxs[pointer.account] == nil {
				removeTxs[pointer.account] = make([]*internalTransaction[T, Constraint], 0)
			}
			// record dirty accounts and removeTxs
			removeTxs[pointer.account] = append(removeTxs[pointer.account], poolTx)
			dirtyAccounts[pointer.account] = true

			if p.enablePricePriority {
				old, exist := maxPriorityNonce[pointer.account]
				if !exist || exist && old < pointer.nonce {
					maxPriorityNonce[pointer.account] = pointer.nonce
				}
			}
		}
	})

	if p.enablePricePriority && len(maxPriorityNonce) > 0 {
		for from, nonce := range maxPriorityNonce {
			p.txStore.priorityByPrice.removeTxBeforeNonce(from, nonce)
		}
	}

	for account := range dirtyAccounts {
		if list, ok := p.txStore.allTxs[account]; ok {
			if err := p.cleanTxsByAccount(account, list, removeTxs[account], true); err != nil {
				p.logger.Errorf("cleanTxsByAccount error: %v", err)
			} else {
				removeCount += len(removeTxs[account])
			}
		}
	}

	// update nonce because we had persist these txs
	lo.ForEach(txPointerList, func(wrapperPointer *commonpool.WrapperTxPointer, _ int) {
		p.updateNonceCache(&txPointer{account: wrapperPointer.Account, nonce: wrapperPointer.Nonce}, updateAccounts)
	})

	readyNum := uint64(p.txStore.priorityByTime.size())
	if p.enablePricePriority {
		readyNum = p.txStore.priorityByPrice.size()
	}
	// set nonBatchSize to min(nonBatchedTxs, readyNum),
	if p.txStore.priorityNonBatchSize > readyNum {
		p.logger.Infof("Set nonBatchSize from %d to the size of priority %d", p.txStore.priorityNonBatchSize, readyNum)
		p.setPriorityNonBatchSize(readyNum)
	}

	for account, pendingNonce := range updateAccounts {
		p.logger.Debugf("Account %s update its pendingNonce to %d by commitNonce", account, pendingNonce)
	}

	if removeCount > 0 {
		p.logger.Infof("finish RemoveStateUpdatingTxs, len:%d, removeCount:%d", len(txPointerList), removeCount)
		traceRemovedTx("RemoveStateUpdatingTxs", removeCount)
	}
	return removeCount
}

// GetRequestsByHashList returns the transaction list corresponding to the given hash list.
// When replicas receive hashList from primary, they need to generate a totally same
// batch to primary generated one. deDuplicateTxHashes specifies some txs which should
// be excluded from duplicate rules.
//  1. If this batch has been batched, just return its transactions without error.
//  2. If we have checked this batch and found we were missing some transactions, just
//     return the same missingTxsHash as before without error.
//  3. If one transaction in hashList has been batched before in another batch,
//     return ErrDuplicateTx
//  4. If we miss some transactions, we need to fetch these transactions from primary,
//     and return missingTxsHash without error
//  5. If this node get all transactions from pool, generate a batch and return its
//     transactions without error
func (p *txPoolImpl[T, Constraint]) GetRequestsByHashList(batchHash string, timestamp int64, hashList []string,
	deDuplicateTxHashes []string) (txs []*T, localList []bool, missingTxsHash map[uint64]string, err error) {
	req := &reqGetTxsForGenBatch[T, Constraint]{
		batchHash:           batchHash,
		timestamp:           timestamp,
		hashList:            hashList,
		deDuplicateTxHashes: deDuplicateTxHashes,
		respCh:              make(chan *respGetTxsForGenBatch[T, Constraint]),
	}

	ev := &batchEvent{
		EventType: commonpool.GetTxsForGenBatchEvent,
		Event:     req,
	}
	p.postEvent(ev)

	resp := <-req.respCh
	if resp.err != nil {
		err = resp.err
		return
	}
	txs = resp.txs
	localList = resp.localList
	missingTxsHash = resp.missingTxsHash
	return
}

func (p *txPoolImpl[T, Constraint]) handleGetRequestsByHashList(batchHash string, timestamp int64, hashList []string,
	deDuplicateTxHashes []string) ([]*T, []bool, map[uint64]string, error) {
	var (
		txs            []*T
		localList      []bool
		missingTxsHash map[uint64]string
		err            error
	)
	if batch, ok := p.txStore.batchesCache[batchHash]; ok {
		// If replica already has this batch, directly return tx list
		p.logger.Debugf("Batch %s is already in batchesCache", batchHash)
		return batch.TxList, batch.LocalList, nil, nil
	}

	// If we have checked this batch and found we miss some transactions,
	// just return the same missingTxsHash as before
	if missingBatch, ok := p.txStore.missingBatch[batchHash]; ok {
		p.logger.Debugf("GetRequestsByHashList failed, find batch %s in missingBatch store", batchHash)
		return nil, nil, missingBatch, nil
	}

	deDuplicateMap := make(map[string]bool)
	for _, duplicateHash := range deDuplicateTxHashes {
		deDuplicateMap[duplicateHash] = true
	}

	missingTxsHash = make(map[uint64]string)
	var hasMissing bool
	for index, txHash := range hashList {
		pointer := p.txStore.txHashMap[txHash]
		if pointer == nil {
			p.logger.Debugf("Can't find tx by hash: %s from txpool", txHash)
			missingTxsHash[uint64(index)] = txHash
			hasMissing = true
			continue
		}
		if deDuplicateMap[txHash] {
			// ignore deDuplicate txs for duplicate rule
			p.logger.Warningf("Ignore de-duplicate tx %s when create same batch", txHash)
		} else {
			if _, ok := p.txStore.batchedTxs[*pointer]; ok {
				// If this transaction has been batched, return ErrDuplicateTx
				p.logger.Warningf("Duplicate transaction in getTxsByHashList with "+
					"hash: %s, batch batchHash: %s", txHash, batchHash)
				err = errors.New("duplicate transaction")
				return nil, nil, nil, err
			}
		}
		poolTx := p.txStore.getPoolTxByTxnPointer(pointer.account, pointer.nonce)

		if !hasMissing {
			txs = append(txs, poolTx.rawTx)
			localList = append(localList, poolTx.local)
		}
	}

	if len(missingTxsHash) != 0 {
		// clone to avoid concurrent read and write problems with consensus and txpool
		p.txStore.missingBatch[batchHash] = lo.MapEntries(missingTxsHash, func(k uint64, v string) (uint64, string) {
			return k, v
		})
		return nil, nil, missingTxsHash, nil
	}
	for _, txHash := range hashList {
		pointer := p.txStore.txHashMap[txHash]
		p.txStore.batchedTxs[*pointer] = true
	}
	// store the batch to cache
	batch := &commonpool.RequestHashBatch[T, Constraint]{
		BatchHash:  batchHash,
		TxList:     txs,
		TxHashList: hashList,
		LocalList:  localList,
		Timestamp:  timestamp,
	}
	p.txStore.batchesCache[batchHash] = batch
	if p.txStore.priorityNonBatchSize <= uint64(len(hashList)) {
		p.setPriorityNonBatchSize(0)
	} else {
		p.decreasePriorityNonBatchSize(uint64(len(hashList)))
	}
	missingTxsHash = nil
	p.logger.Debugf("Replica generate a batch, which digest is %s, and now there are %d "+
		"non-batched txs and %d batches in txpool", batchHash, p.txStore.priorityNonBatchSize, len(p.txStore.batchesCache))
	return txs, localList, missingTxsHash, nil
}

func (p *txPoolImpl[T, Constraint]) HasPendingRequestInPool() bool {
	return p.hasPendingRequestInPool()
}

func (p *txPoolImpl[T, Constraint]) hasPendingRequestInPool() bool {
	return p.statusMgr.In(HasPendingRequest)
}

func (p *txPoolImpl[T, Constraint]) checkPendingRequestInPool() bool {
	return p.txStore.priorityNonBatchSize > 0
}

func (p *txPoolImpl[T, Constraint]) PendingRequestsNumberIsReady() bool {
	return p.statusMgr.In(ReadyGenerateBatch)
}

func (p *txPoolImpl[T, Constraint]) checkPendingRequestsNumberIsReady() bool {
	return p.txStore.priorityNonBatchSize >= p.chainState.EpochInfo.ConsensusParams.BlockMaxTxNum
}

func (p *txPoolImpl[T, Constraint]) ReceiveMissingRequests(batchHash string, txs map[uint64]*T) error {
	req := &reqMissingTxs[T, Constraint]{
		batchHash: batchHash,
		txs:       txs,
		errCh:     make(chan error),
	}

	ev := &addTxsEvent{
		EventType: missingTxsEvent,
		Event:     req,
	}

	p.postEvent(ev)
	return <-req.errCh
}

func (p *txPoolImpl[T, Constraint]) validateReceiveMissingRequests(batchHash string, txs map[uint64]*T) ([]*T, error) {
	p.logger.Debugf("Replica received %d missingTxs, batch hash: %s", len(txs), batchHash)
	if _, ok := p.txStore.missingBatch[batchHash]; !ok {
		p.logger.Debugf("Can't find batch %s from missingBatch", batchHash)
		return nil, nil
	}

	targetBatch := p.txStore.missingBatch[batchHash]
	validTxs := make([]*T, 0)
	for index, missingTxHash := range targetBatch {
		txsHash := Constraint(txs[index]).RbftGetTxHash()
		if txsHash != missingTxHash {
			return nil, errors.New("find a hash mismatch tx")
		}
		validTxs = append(validTxs, txs[index])
	}

	return validTxs, nil
}

func (p *txPoolImpl[T, Constraint]) isValidPriceBump(oldGasPrice, newGasPrice *big.Int) bool {
	// thresholdGasPrice = oldGasPrice  * (100 + priceBump) / 100
	a := big.NewInt(100 + int64(p.PriceBump))
	validPrice := new(big.Int).Mul(a, oldGasPrice)
	newPrice := new(big.Int).Mul(newGasPrice, big.NewInt(100))
	return newPrice.Cmp(validPrice) >= 0
}

func (p *txPoolImpl[T, Constraint]) replaceTx(tx *T, local bool) bool {
	account := Constraint(tx).RbftGetFrom()
	txNonce := Constraint(tx).RbftGetNonce()
	pendingNonce := p.txStore.nonceCache.getPendingNonce(account)
	var replaced bool

	oldPoolTx := p.txStore.getPoolTxByTxnPointer(account, txNonce)
	if oldPoolTx != nil {
		p.txStore.removeTxInPool(oldPoolTx, p.enablePricePriority, true)
		traceRemovedTx("primary_replace_old", 1)
		replaced = true
	}

	// update txPointer in allTxs
	now := time.Now().UnixNano()
	newPoolTx := &internalTransaction[T, Constraint]{
		local:       false,
		rawTx:       tx,
		lifeTime:    Constraint(tx).RbftGetTimeStamp(),
		arrivedTime: now,
	}

	// 1. insert new tx to pool
	p.txStore.insertTxInPool(newPoolTx, local)

	// if tx is the pending tx, update pending nonce
	if txNonce == pendingNonce {
		p.processDirtyAccount(account, newPoolTx, true)
	}

	// 2. replace old tx in priority queue or parking lot
	if txNonce < pendingNonce {
		if p.enablePricePriority {
			p.txStore.priorityByPrice.replaceTx(newPoolTx)
		} else {
			p.logger.WithFields(logrus.Fields{"txNonce": txNonce, "account": account}).Info("replace old tx in priority queue")
			p.txStore.priorityByTime.insertKey(newPoolTx)
		}
		if !replaced {
			p.increasePriorityNonBatchSize(1)
		}
	} else if txNonce > pendingNonce {
		// insert new tx to parking lot
		p.txStore.parkingLotIndex.insertKey(newPoolTx)
		// replace old tx in parking lot is not needed to update parking lot size
		if !replaced {
			p.txStore.increaseParkingLotSize(1)
		}
	}

	return replaced
}

func (p *txPoolImpl[T, Constraint]) SendMissingRequests(batchHash string, missingHashList map[uint64]string) (txs map[uint64]*T, err error) {
	req := &reqSendMissingTxs[T, Constraint]{
		batchHash:       batchHash,
		missingHashList: missingHashList,
		respCh:          make(chan *respSendMissingTxs[T, Constraint]),
	}
	ev := &consensusEvent{
		EventType: SendMissingTxsEvent,
		Event:     req,
	}
	p.postEvent(ev)
	resp := <-req.respCh
	if resp.err != nil {
		return nil, resp.err
	}
	txs = resp.resp
	return
}

func (p *txPoolImpl[T, Constraint]) handleSendMissingRequests(batchHash string, missingHashList map[uint64]string) (txs map[uint64]*T, err error) {
	for _, txHash := range missingHashList {
		if pointer := p.txStore.txHashMap[txHash]; pointer == nil {
			return nil, fmt.Errorf("transaction %s doesn't exist in txHashMap", txHash)
		}
	}
	var targetBatch *commonpool.RequestHashBatch[T, Constraint]
	var ok bool
	if targetBatch, ok = p.txStore.batchesCache[batchHash]; !ok {
		return nil, fmt.Errorf("batch %s doesn't exist in batchedCache", batchHash)
	}
	targetBatchLen := uint64(len(targetBatch.TxList))
	txs = make(map[uint64]*T)
	for index, txHash := range missingHashList {
		if index >= targetBatchLen || targetBatch.TxHashList[index] != txHash {
			return nil, fmt.Errorf("find invalid transaction, index: %d, targetHash: %s", index, txHash)
		}
		txs[index] = targetBatch.TxList[index]
	}
	return
}

// ReConstructBatchByOrder reconstruct batch from empty txPool by order, must be called after RestorePool.
func (p *txPoolImpl[T, Constraint]) ReConstructBatchByOrder(oldBatch *commonpool.RequestHashBatch[T, Constraint]) (
	deDuplicateTxHashes []string, err error) {
	req := &reqReConstructBatch[T, Constraint]{
		oldBatch: oldBatch,
		respCh:   make(chan *respReConstructBatch),
	}
	ev := &batchEvent{
		EventType: commonpool.ReConstructBatchEvent,
		Event:     req,
	}
	p.postEvent(ev)
	resp := <-req.respCh
	return resp.deDuplicateTxHashes, resp.err
}

func (p *txPoolImpl[T, Constraint]) handleReConstructBatchByOrder(oldBatch *commonpool.RequestHashBatch[T, Constraint]) ([]string, error) {
	// check if there exists duplicate batch hash.
	if _, ok := p.txStore.batchesCache[oldBatch.BatchHash]; ok {
		p.logger.Warningf("When re-construct batch, batch %s already exists", oldBatch.BatchHash)
		err := errors.New("invalid batch: batch already exists")
		return nil, err
	}

	// TxPointerList has to match TxList by size and content
	if len(oldBatch.TxHashList) != len(oldBatch.TxList) {
		p.logger.Warningf("Batch is invalid because TxPointerList and TxList have different lengths.")
		err := errors.New("invalid batch: TxPointerList and TxList have different lengths")
		return nil, err
	}

	for i, tx := range oldBatch.TxList {
		txHash := Constraint(tx).RbftGetTxHash()
		if txHash != oldBatch.TxHashList[i] {
			p.logger.Warningf("Batch is invalid because the hash %s in txHashList does not match "+
				"the calculated hash %s of the corresponding transaction.", oldBatch.TxHashList[i], txHash)
			err := errors.New("invalid batch: hash of transaction does not match")
			return nil, err
		}
	}

	localList := make([]bool, len(oldBatch.TxHashList))

	lo.ForEach(oldBatch.TxHashList, func(_ string, index int) {
		localList[index] = false
	})

	batch := &commonpool.RequestHashBatch[T, Constraint]{
		TxHashList: oldBatch.TxHashList,
		TxList:     oldBatch.TxList,
		LocalList:  localList,
		Timestamp:  oldBatch.Timestamp,
	}
	// The given batch hash should match with the calculated batch hash.
	batch.BatchHash = batch.GenerateBatchHash()
	if batch.BatchHash != oldBatch.BatchHash {
		p.logger.Warningf("The given batch hash %s does not match with the "+
			"calculated batch hash %s.", oldBatch.BatchHash, batch.BatchHash)
		err := errors.New("invalid batch: batch hash does not match")
		return nil, err
	}

	deDuplicateTxHashes := make([]string, 0)

	// There may be some duplicate transactions which are batched in different batches during vc, for those txs,
	// we only accept them in the first batch containing them and de-duplicate them in following batches.
	for _, tx := range oldBatch.TxList {
		ptr := &txPointer{
			account: Constraint(tx).RbftGetFrom(),
			nonce:   Constraint(tx).RbftGetNonce(),
		}
		txHash := Constraint(tx).RbftGetTxHash()
		if _, ok := p.txStore.batchedTxs[*ptr]; ok {
			p.logger.Warningf("De-duplicate tx %s when re-construct batch by order", txHash)
			deDuplicateTxHashes = append(deDuplicateTxHashes, txHash)
		} else {
			p.txStore.batchedTxs[*ptr] = true
		}
	}
	p.logger.Debugf("ReConstructBatchByOrder batch %s into batchedCache", oldBatch.BatchHash)
	p.txStore.batchesCache[batch.BatchHash] = batch
	return deDuplicateTxHashes, nil
}

func (p *txPoolImpl[T, Constraint]) RestorePool() {
	ev := &consensusEvent{
		EventType: RestoreAllBatchedEvent,
	}
	p.postEvent(ev)
}

// RestorePool move all batched txs back to non-batched tx which should
// only be used after abnormal recovery.
func (p *txPoolImpl[T, Constraint]) handleRestorePool() {
	priorityLen := p.txStore.priorityByTime.size()
	if p.enablePricePriority {
		priorityLen = int(p.txStore.priorityByPrice.size())
	}
	p.logger.Infof("Before restore pool, there are %d non-batched txs, %d batches, "+
		"priority len: %d, parkingLot len: %d, parkingLot size len: %d, batchedTx len: %d, txHashMap len: %d, local txs: %d", p.txStore.priorityNonBatchSize,
		len(p.txStore.batchesCache), priorityLen, p.txStore.parkingLotIndex.size(), p.txStore.parkingLotSize,
		len(p.txStore.batchedTxs), len(p.txStore.txHashMap), p.txStore.localTTLIndex.size())

	for _, batch := range p.txStore.batchesCache {
		if err := p.putBackBatchedTxs(batch); err != nil {
			p.logger.Errorf("Failed to put back batched[batchHash: %s]: %s, just remove the batch from cache", batch.BatchHash, err)
			delete(p.txStore.batchesCache, batch.BatchHash)
			continue
		}
	}

	priorityLen = p.txStore.priorityByTime.size()
	if p.enablePricePriority {
		priorityLen = int(p.txStore.priorityByPrice.size())
	}
	// clear missingTxs after abnormal.
	p.txStore.missingBatch = make(map[string]map[uint64]string)
	p.txStore.batchedTxs = make(map[txPointer]bool)
	p.logger.Infof("After restore pool, there are %d non-batched txs, %d batches, "+
		"priority len: %d, parkingLot len: %d, parkingLot size len: %d, batchedTx len: %d, txHashMap len: %d, local txs: %d", p.txStore.priorityNonBatchSize,
		len(p.txStore.batchesCache), priorityLen, p.txStore.parkingLotIndex.size(), p.txStore.parkingLotSize,
		len(p.txStore.batchedTxs), len(p.txStore.txHashMap), p.txStore.localTTLIndex.size())
}

func (p *txPoolImpl[T, Constraint]) putBackBatchedTxs(batch *commonpool.RequestHashBatch[T, Constraint]) error {
	// remove from batchedTxs and batchStore
	p.logger.Infof("put back batched txs: %s", batch.BatchHash)
	putBachTxPtrs := make(map[txPointer]*internalTransaction[T, Constraint], 0)

	// basic check
	for i := len(batch.TxList) - 1; i >= 0; i-- {
		tx := batch.TxList[i]
		hash := Constraint(tx).RbftGetTxHash()
		ptr := p.txStore.txHashMap[hash]
		if ptr == nil {
			return fmt.Errorf("can't find tx from txHashMap:[txHash:%s]", hash)
		}

		if !p.txStore.batchedTxs[*ptr] {
			return fmt.Errorf("can't find tx from batchedTxs:[txHash:%s]", hash)
		}

		poolTx := p.txStore.getPoolTxByTxnPointer(ptr.account, ptr.nonce)
		if poolTx == nil {
			return fmt.Errorf("can't find tx from pool:[txHash:%s, account:%s, nonce:%d]", hash, ptr.account, ptr.nonce)
		}
		// check if the given tx exist in priority
		putBachTxPtrs[*ptr] = poolTx
		if !p.enablePricePriority {
			key := &orderedIndexKey{time: poolTx.getRawTimestamp(), account: ptr.account, nonce: ptr.nonce}
			if poolTransaction := p.txStore.priorityByTime.data.Get(key); poolTransaction == nil {
				return fmt.Errorf("can't find tx from priorityByTime:[txHash:%s]", hash)
			}
		}
	}

	// 1. delete tx from batchedTxs list
	for ptr, pTx := range putBachTxPtrs {
		if p.enablePricePriority {
			p.txStore.priorityByPrice.pushBack(pTx)
		}
		delete(p.txStore.batchedTxs, ptr)
	}

	// 2. increase nonBatchSize
	p.increasePriorityNonBatchSize(uint64(len(batch.TxList)))
	// 3. remove batchCache
	delete(p.txStore.batchesCache, batch.BatchHash)
	return nil
}

// FilterOutOfDateRequests get the remained local txs in TTLIndex and broadcast to other vp peers by tolerance time.
func (p *txPoolImpl[T, Constraint]) FilterOutOfDateRequests(timeout bool) []*T {
	req := &reqFilterReBroadcastTxs[T, Constraint]{
		timeout: timeout,
		respCh:  make(chan []*T),
	}
	ev := &consensusEvent{
		EventType: FilterReBroadcastTxsEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.respCh
}

func (p *txPoolImpl[T, Constraint]) handleFilterOutOfDateRequests(timeout bool) []*T {
	now := time.Now().UnixNano()
	var forward []*internalTransaction[T, Constraint]
	p.txStore.localTTLIndex.data.Ascend(func(a btree.Item) bool {
		orderedKey := a.(*orderedIndexKey)
		poolTx := p.txStore.getPoolTxByTxnPointer(orderedKey.account, orderedKey.nonce)
		if poolTx == nil {
			p.logger.Error("Get nil poolTx from txStore")
			return true
		}

		if !timeout || now-poolTx.lifeTime > p.toleranceTime.Nanoseconds() {
			if p.enablePricePriority {
				account := p.txStore.priorityByPrice.accountsM[orderedKey.account]
				if account != nil {
					if orderedKey.nonce >= account.minNonceQueue.PeekItem() {
						if reBroadTx := account.items[orderedKey.nonce]; reBroadTx != nil {
							forward = append(forward, reBroadTx)
						}
					}
				}
			} else {
				priorityKey := &orderedIndexKey{time: poolTx.getRawTimestamp(), account: poolTx.getAccount(), nonce: poolTx.getNonce()}
				// for those priority txs, we need rebroadcast
				if tx := p.txStore.priorityByTime.data.Get(priorityKey); tx != nil {
					forward = append(forward, poolTx)
				}
			}
		} else {
			return false
		}
		return true
	})
	result := make([]*T, len(forward))
	// update pool tx's timestamp to now for next forwarding check.
	for i, poolTx := range forward {
		// update localTTLIndex
		p.txStore.localTTLIndex.updateIndex(poolTx, now)
		result[i] = poolTx.rawTx
		// update txpool tx in allTxs
		poolTx.lifeTime = now
	}
	return result
}

func (p *txPoolImpl[T, Constraint]) fillRemoveTxs(orderedKey *orderedIndexKey, poolTx *internalTransaction[T, Constraint],
	removedTxs map[string][]*internalTransaction[T, Constraint], existTxs map[string]map[uint64]bool) bool {
	if _, ok := removedTxs[orderedKey.account]; !ok {
		removedTxs[orderedKey.account] = make([]*internalTransaction[T, Constraint], 0)
	}
	// record need removedTxs and the count
	if existTxs[orderedKey.account] == nil {
		existTxs[orderedKey.account] = make(map[uint64]bool)
	}
	// omit the same tx nonce
	if existTxs[orderedKey.account][orderedKey.nonce] {
		return false
	}
	removedTxs[orderedKey.account] = append(removedTxs[orderedKey.account], poolTx)
	existTxs[orderedKey.account][orderedKey.nonce] = true
	return true
}

func (p *txPoolImpl[T, Constraint]) removeHighNonceTxsByAccount(account string, nonce uint64) (int, error) {
	var removeTxs []*internalTransaction[T, Constraint]
	if list, ok := p.txStore.allTxs[account]; ok {
		removeTxs = list.behind(nonce)
		// remove high nonce txs which exist in parkingLotIndex(not exist in priority), so we need not clean priority
		if err := p.cleanTxsByAccount(account, list, removeTxs, false); err != nil {
			return 0, err
		}
	}

	return len(removeTxs), nil
}

// =============================================================================
// internal methods
// =============================================================================
func (p *txPoolImpl[T, Constraint]) processDirtyAccount(account string, tx *internalTransaction[T, Constraint], pending bool) {
	if list, ok := p.txStore.allTxs[account]; ok {
		// search for related sequential txs in allTxs
		// and add these txs into priorityByTime and parkingLotIndex.
		if pending {
			pendingNonce := p.txStore.nonceCache.getPendingNonce(account)
			readyTxs, nextDemandNonce := list.filterReady(pendingNonce)
			p.txStore.nonceCache.setPendingNonce(account, nextDemandNonce)

			// insert ready txs into priority
			for _, poolTx := range readyTxs {
				if p.enablePricePriority {
					p.txStore.priorityByPrice.push(poolTx)
				} else {
					p.txStore.priorityByTime.insertKey(poolTx)
				}
			}

			// update priorityNonBatchSize
			p.increasePriorityNonBatchSize(uint64(len(readyTxs)))

			// because we insert ready tx, we should decrease the ParkingLotSize by length which covert non-ready txs to ready txs
			nonReady2ReadyCnt := len(readyTxs) - 1
			// update parkingLotSize
			p.txStore.decreaseParkingLotSize(uint64(nonReady2ReadyCnt))
		} else {
			// if not pending, we should insert tx to the parkingLotIndex
			p.txStore.parkingLotIndex.insertKey(tx)
			p.txStore.increaseParkingLotSize(1)
		}
	}
}

func (p *txPoolImpl[T, Constraint]) increasePriorityNonBatchSize(addSize uint64) {
	p.txStore.priorityNonBatchSize = p.txStore.priorityNonBatchSize + addSize
	if p.checkPendingRequestInPool() {
		p.setHasPendingRequest()
	}
	if p.checkPendingRequestsNumberIsReady() {
		p.setReady()
	}
	readyTxNum.Set(float64(p.txStore.priorityNonBatchSize))
}

func (p *txPoolImpl[T, Constraint]) decreasePriorityNonBatchSize(subSize uint64) {
	if p.txStore.priorityNonBatchSize < subSize {
		p.logger.Error("nonBatchSize < subSize", "nonBatchSize: ", p.txStore.priorityNonBatchSize, "subSize: ", subSize)
		p.txStore.priorityNonBatchSize = 0
	}
	p.txStore.priorityNonBatchSize = p.txStore.priorityNonBatchSize - subSize
	if !p.checkPendingRequestInPool() {
		p.setNoPendingRequest()
	}
	if !p.checkPendingRequestsNumberIsReady() {
		p.setNotReady()
	}
	readyTxNum.Set(float64(p.txStore.priorityNonBatchSize))
}

func (p *txPoolImpl[T, Constraint]) setPriorityNonBatchSize(txnSize uint64) {
	p.txStore.priorityNonBatchSize = txnSize
	if p.checkPendingRequestInPool() {
		p.setHasPendingRequest()
	} else {
		p.setNoPendingRequest()
	}

	if p.checkPendingRequestsNumberIsReady() {
		p.setReady()
	} else {
		p.setNotReady()
	}
	readyTxNum.Set(float64(p.txStore.priorityNonBatchSize))
}

func (p *txPoolImpl[T, Constraint]) handleRotateTxLocalsEvent() error {
	return p.txRecords.rotate(p.txStore.allTxs)
}

func (p *txPoolImpl[T, Constraint]) GetLocalTxs() [][]byte {
	var res [][]byte
	for _, txs := range p.txStore.allTxs {
		for _, item := range txs.items {
			if item.local {
				marshal, err := Constraint(item.rawTx).RbftMarshal()
				if err != nil {
					p.logger.Error("GetLocalTxs: failed to marshal local tx, hash is ", item.getHash())
				}
				res = append(res, marshal)
			}
		}
	}
	return res
}
