package txpool

import (
	"github.com/google/btree"
	"github.com/samber/lo"

	commonpool "github.com/axiomesh/axiom-kit/txpool"
)

func (p *txPoolImpl[T, Constraint]) IsStarted() bool {
	return p.started.Load()
}

func (p *txPoolImpl[T, Constraint]) GetChainInfo() *commonpool.ChainInfo {
	req := &reqChainInfoMsg{
		ch: make(chan *commonpool.ChainInfo),
	}
	ev := &poolInfoEvent{
		EventType: reqChainInfoEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.ch
}

func (p *txPoolImpl[T, Constraint]) UpdateChainInfo(chainInfo *commonpool.ChainInfo) {
	req := &updateChainInfoMsg{
		chainInfo: chainInfo,
	}
	ev := &poolInfoEvent{
		EventType: updateChainInfoEvent,
		Event:     req,
	}
	p.postEvent(ev)
}

func (p *txPoolImpl[T, Constraint]) GetTotalPendingTxCount() uint64 {
	req := &reqPendingTxCountMsg{ch: make(chan uint64)}
	ev := &poolInfoEvent{
		EventType: reqPendingTxCountEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.ch
}

func (p *txPoolImpl[T, Constraint]) handleGetTotalPendingTxCount() uint64 {
	return uint64(len(p.txStore.txHashMap))
}

// GetPendingTxCountByAccount returns the latest pending nonce of the account in txpool
func (p *txPoolImpl[T, Constraint]) GetPendingTxCountByAccount(account string) uint64 {
	req := &reqNonceMsg{
		account: account,
		ch:      make(chan uint64),
	}
	ev := &poolInfoEvent{
		EventType: reqNonceEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.ch
}

// GetPendingTxCountByAccount returns the latest pending nonce of the account in txpool
func (p *txPoolImpl[T, Constraint]) handleGetPendingTxCountByAccount(account string) uint64 {
	return p.txStore.nonceCache.getPendingNonce(account)
}

func (p *txPoolImpl[T, Constraint]) GetPendingTxByHash(hash string) *T {
	req := &reqTxMsg[T, Constraint]{
		hash: hash,
		ch:   make(chan *T),
	}
	ev := &poolInfoEvent{
		EventType: reqTxEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.ch
}

func (p *txPoolImpl[T, Constraint]) handleGetPendingTxByHash(hash string) *T {
	key, ok := p.txStore.txHashMap[hash]
	if !ok {
		return nil
	}

	txMap, ok := p.txStore.allTxs[key.account]
	if !ok {
		return nil
	}

	item, ok := txMap.items[key.nonce]
	if !ok {
		return nil
	}

	return item.rawTx
}

func (p *txPoolImpl[T, Constraint]) GetAccountMeta(account string, full bool) *commonpool.AccountMeta[T, Constraint] {
	req := &reqAccountPoolMetaMsg[T, Constraint]{
		account: account,
		full:    full,
		ch:      make(chan *commonpool.AccountMeta[T, Constraint]),
	}
	ev := &poolInfoEvent{
		EventType: reqAccountMetaEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.ch
}

func (p *txPoolImpl[T, Constraint]) handleGetAccountMeta(account string, full bool) *commonpool.AccountMeta[T, Constraint] {
	if p.txStore.allTxs[account] == nil {
		return &commonpool.AccountMeta[T, Constraint]{
			CommitNonce:  p.txStore.nonceCache.getCommitNonce(account),
			PendingNonce: p.txStore.nonceCache.getPendingNonce(account),
			TxCount:      0,
			Txs:          []*commonpool.TxInfo[T, Constraint]{},
			SimpleTxs:    []*commonpool.TxSimpleInfo{},
		}
	}

	fullTxs := make([]*internalTransaction[T, Constraint], 0)
	p.txStore.allTxs[account].index.data.Ascend(func(i btree.Item) bool {
		nonce := i.(*sortedNonceKey).nonce
		item, ok := p.txStore.allTxs[account].items[nonce]
		if !ok {
			p.logger.Errorf("Get nil tx from allTxs items")
			return true
		}
		fullTxs = append(fullTxs, item)
		return true
	})

	res := &commonpool.AccountMeta[T, Constraint]{
		CommitNonce:  p.txStore.nonceCache.getCommitNonce(account),
		PendingNonce: p.txStore.nonceCache.getPendingNonce(account),
		TxCount:      uint64(len(fullTxs)),
	}

	if full {
		res.Txs = make([]*commonpool.TxInfo[T, Constraint], 0, len(fullTxs))
		for _, tx := range fullTxs {
			res.Txs = append(res.Txs, &commonpool.TxInfo[T, Constraint]{
				Tx:          tx.rawTx,
				Local:       tx.local,
				LifeTime:    tx.lifeTime,
				ArrivedTime: tx.arrivedTime,
			})
		}
	} else {
		res.SimpleTxs = make([]*commonpool.TxSimpleInfo, 0, len(fullTxs))
		for _, tx := range fullTxs {
			c := Constraint(tx.rawTx)
			res.SimpleTxs = append(res.SimpleTxs, &commonpool.TxSimpleInfo{
				Hash:        c.RbftGetTxHash(),
				Nonce:       c.RbftGetNonce(),
				Size:        c.RbftGetSize(),
				Local:       tx.local,
				LifeTime:    tx.lifeTime,
				ArrivedTime: tx.arrivedTime,
			})
		}
	}

	return res
}

func (p *txPoolImpl[T, Constraint]) GetMeta(full bool) *commonpool.Meta[T, Constraint] {
	req := &reqPoolMetaMsg[T, Constraint]{
		full: full,
		ch:   make(chan *commonpool.Meta[T, Constraint]),
	}
	ev := &poolInfoEvent{
		EventType: reqPoolMetaEvent,
		Event:     req,
	}
	p.postEvent(ev)
	return <-req.ch
}

func (p *txPoolImpl[T, Constraint]) handleGetMeta(full bool) *commonpool.Meta[T, Constraint] {
	res := &commonpool.Meta[T, Constraint]{
		TxCountLimit:    p.poolMaxSize,
		TxCount:         uint64(len(p.txStore.txHashMap)),
		ReadyTxCount:    p.txStore.priorityNonBatchSize,
		NotReadyTxCount: p.txStore.parkingLotSize,
		Batches:         make(map[string]*commonpool.BatchSimpleInfo, len(p.txStore.batchesCache)),
		MissingBatchTxs: make(map[string]map[uint64]string, len(p.txStore.missingBatch)),
		Accounts:        make(map[string]*commonpool.AccountMeta[T, Constraint], len(p.txStore.allTxs)),
	}
	for _, batch := range p.txStore.batchesCache {
		txs := make([]*commonpool.TxSimpleInfo, 0, len(batch.TxHashList))
		for _, txHash := range batch.TxHashList {
			txIdx := p.txStore.txHashMap[txHash]
			tx := p.txStore.allTxs[txIdx.account].items[txIdx.nonce]
			c := Constraint(tx.rawTx)
			txs = append(txs, &commonpool.TxSimpleInfo{
				Hash:        c.RbftGetTxHash(),
				Nonce:       c.RbftGetNonce(),
				Size:        c.RbftGetSize(),
				Local:       tx.local,
				LifeTime:    tx.lifeTime,
				ArrivedTime: tx.arrivedTime,
			})
		}
		res.Batches[batch.BatchHash] = &commonpool.BatchSimpleInfo{
			TxCount:   uint64(len(batch.TxHashList)),
			Txs:       txs,
			Timestamp: batch.Timestamp,
		}
	}
	for h, b := range p.txStore.missingBatch {
		res.MissingBatchTxs[h] = lo.MapEntries(b, func(key uint64, value string) (uint64, string) {
			return key, value
		})
	}
	for addr, val := range p.txStore.allTxs {
		// if account is not exist any txs in txStore, omit it
		if !val.empty {
			res.Accounts[addr] = p.handleGetAccountMeta(addr, full)
		}
	}

	return res
}

// IsPoolFull checks if txPool is full which means if number of all cached txs
// has exceeded the limited txSize.
func (p *txPoolImpl[T, Constraint]) IsPoolFull() bool {
	return p.statusMgr.In(PoolFull)
}

func (p *txPoolImpl[T, Constraint]) checkPoolFull() bool {
	return uint64(len(p.txStore.txHashMap)) >= p.poolMaxSize
}
