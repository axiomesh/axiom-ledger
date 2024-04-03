package txpool

import (
	"math/big"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
)

type transactionStore[T any, Constraint types.TXConstraint[T]] struct {
	logger logrus.FieldLogger

	// track all valid tx hashes cached in txpool
	txHashMap map[string]*txPointer

	// track all valid tx, mapping user' account to all related transactions.
	allTxs map[string]*txSortedMap[T, Constraint]

	// track the commit nonce and pending nonce of each account.
	nonceCache *nonceCache

	// keeps track of "non-ready" txs (txs that can't be included in next block)
	// only used to help remove some txs if pool is full.
	parkingLotIndex *btreeIndex[T, Constraint]

	// keeps track of "ready" txs
	priorityByPrice *priorityQueue[T, Constraint]

	// keeps track of "ready" txs
	priorityByTime *btreeIndex[T, Constraint]

	// cache all the batched txs which haven't executed.
	batchedTxs map[txPointer]bool

	// cache all batches created by current primary in order, removed after they are been executed.
	batchesCache map[string]*txpool.RequestHashBatch[T, Constraint]

	// trace the missing transaction
	missingBatch map[string]map[uint64]string

	// track the parkingLot transaction which is actually not ready.
	parkingLotSize uint64

	// track the priority transaction.
	priorityNonBatchSize uint64

	// localTTLIndex based on the tolerance time to track all the remained txs
	// that generate by itself and rebroadcast to other vps.
	localTTLIndex *btreeIndex[T, Constraint]

	// removeTTLIndex based on the remove tolerance time to track all the remained txs
	// that arrived in txpool and remove these txs from memPoll cache in case these exist too long.
	removeTTLIndex *btreeIndex[T, Constraint]
}

func newTransactionStore[T any, Constraint types.TXConstraint[T]](f GetAccountNonceFunc, logger logrus.FieldLogger) *transactionStore[T, Constraint] {
	nCache := newNonceCache(f)
	return &transactionStore[T, Constraint]{
		logger:               logger,
		priorityNonBatchSize: 0,
		parkingLotSize:       0,
		txHashMap:            make(map[string]*txPointer),
		allTxs:               make(map[string]*txSortedMap[T, Constraint]),
		batchedTxs:           make(map[txPointer]bool),
		missingBatch:         make(map[string]map[uint64]string),
		batchesCache:         make(map[string]*txpool.RequestHashBatch[T, Constraint]),
		parkingLotIndex:      newBtreeIndex[T, Constraint](Ordered),
		priorityByTime:       newBtreeIndex[T, Constraint](Ordered),
		localTTLIndex:        newBtreeIndex[T, Constraint](Rebroadcast),
		removeTTLIndex:       newBtreeIndex[T, Constraint](Remove),
		nonceCache:           nCache,
		priorityByPrice:      newPriorityQueue[T, Constraint](nCache.getCommitNonce, logger),
	}
}

func (txStore *transactionStore[T, Constraint]) insertPoolTxPointer(txHash string, pointer *txPointer) {
	if _, ok := txStore.txHashMap[txHash]; ok {
		txStore.logger.Warningf("tx %s already exists in txpool", txHash)
		return
	}
	txStore.txHashMap[txHash] = pointer
}

func (txStore *transactionStore[T, Constraint]) deletePoolTxPointer(txHash string) {
	if _, ok := txStore.txHashMap[txHash]; ok {
		delete(txStore.txHashMap, txHash)
	}
}

func (txStore *transactionStore[T, Constraint]) insertPoolTx(account string, txItem *internalTransaction[T, Constraint]) {
	nonce := txItem.getNonce()
	txHash := txItem.getHash()
	txList, ok := txStore.allTxs[account]
	if !ok {
		// if this is new account to send tx, create a new txSortedMap
		txStore.allTxs[account] = newTxSortedMap[T, Constraint]()
	}
	txList = txStore.allTxs[account]
	if txList.items[nonce] == nil {
		poolTxNum.Inc()
	} else {
		txStore.logger.Warningf("old tx will be replaced[account: %s, nonce: %d]", account, nonce)
		txStore.deletePoolTx(account, nonce)
	}
	txList.items[nonce] = txItem
	txList.index.insertKey(txItem)
	// if the account is empty, we need to set the empty flag to false because we insert a new tx
	if txList.isEmpty() {
		txList.setNotEmpty()
	}

	// insert tx pointer in txHashMap
	txStore.insertPoolTxPointer(txHash, &txPointer{
		account: account,
		nonce:   nonce,
	})
}

func (txStore *transactionStore[T, Constraint]) deletePoolTx(account string, nonce uint64) {
	if txList, ok := txStore.allTxs[account]; ok {
		poolTx := txStore.getPoolTxByTxnPointer(account, nonce)
		if poolTx == nil {
			txStore.logger.Warningf("tx [account:%s, nonce:%d] not found in txpool", account, nonce)
			return
		}
		txList.index.removeKey(poolTx)
		delete(txList.items, nonce)
		poolTxNum.Dec()

		if txList.isEmpty() {
			txList.setEmpty()
		}

		// delete tx pointer in txHashMap
		txStore.deletePoolTxPointer(poolTx.getHash())
	}
}

func (txStore *transactionStore[T, Constraint]) removeTxInPool(poolTx *internalTransaction[T, Constraint], enablePricePriority, isPriority bool) {
	txStore.deletePoolTx(poolTx.getAccount(), poolTx.getNonce())
	if !enablePricePriority && isPriority {
		txStore.priorityByTime.removeKey(poolTx)
	}
	txStore.parkingLotIndex.removeKey(poolTx)
	txStore.removeTTLIndex.removeKey(poolTx)
	txStore.localTTLIndex.removeKey(poolTx)
}

func (txStore *transactionStore[T, Constraint]) insertTxInPool(poolTx *internalTransaction[T, Constraint], isLocal bool) {
	txStore.insertPoolTx(poolTx.getAccount(), poolTx)
	if isLocal {
		txStore.localTTLIndex.insertKey(poolTx)
	}
	// record the tx arrived timestamp
	txStore.removeTTLIndex.insertKey(poolTx)
}

// getPoolTxByTxnPointer gets transaction by account address + nonce
func (txStore *transactionStore[T, Constraint]) getPoolTxByTxnPointer(account string, nonce uint64) *internalTransaction[T, Constraint] {
	if list, ok := txStore.allTxs[account]; ok {
		return list.items[nonce]
	}
	return nil
}

func (txStore *transactionStore[T, Constraint]) increaseParkingLotSize(addSize uint64) {
	txStore.parkingLotSize = txStore.parkingLotSize + addSize
	queueTxNum.Set(float64(txStore.parkingLotSize))
}

func (txStore *transactionStore[T, Constraint]) decreaseParkingLotSize(subSize uint64) {
	if txStore.parkingLotSize < subSize {
		txStore.logger.Error("parkingLotSize < subSize,", "parkingLotSize: ", txStore.parkingLotSize, "subSize: ", subSize)
		txStore.parkingLotSize = 0
	}
	txStore.parkingLotSize = txStore.parkingLotSize - subSize
	queueTxNum.Set(float64(txStore.parkingLotSize))
}

// todo: gc the empty account in txpool(delete key)
type txSortedMap[T any, Constraint types.TXConstraint[T]] struct {
	items     map[uint64]*internalTransaction[T, Constraint] // map nonce to transaction
	index     *btreeIndex[T, Constraint]                     // index for items' nonce
	empty     bool                                           // whether the account is empty in pool
	emptyTime int64                                          // the latest timestamp when the account is empty
}

func newTxSortedMap[T any, Constraint types.TXConstraint[T]]() *txSortedMap[T, Constraint] {
	return &txSortedMap[T, Constraint]{
		items: make(map[uint64]*internalTransaction[T, Constraint]),
		index: newBtreeIndex[T, Constraint](SortNonce),
	}
}

func (m *txSortedMap[T, Constraint]) setEmpty() {
	m.empty = true
	m.emptyTime = time.Now().UnixNano()
}

func (m *txSortedMap[T, Constraint]) getEmptyTime() int64 {
	return m.emptyTime
}

func (m *txSortedMap[T, Constraint]) setNotEmpty() {
	m.empty = false
}

func (m *txSortedMap[T, Constraint]) isEmpty() bool {
	return m.empty
}

func (m *txSortedMap[T, Constraint]) checkIfGc(now, cleanTimeout int64) bool {
	return m.isEmpty() && now-m.getEmptyTime() > cleanTimeout
}

func (m *txSortedMap[T, Constraint]) filterReady(demandNonce uint64) ([]*internalTransaction[T, Constraint], uint64) {
	var readyTxs []*internalTransaction[T, Constraint]
	if m.index.data.Len() == 0 {
		return nil, demandNonce
	}
	demandKey := makeSortedNonceKey(demandNonce)
	m.index.data.AscendGreaterOrEqual(demandKey, func(i btree.Item) bool {
		nonce := i.(*sortedNonceKey).nonce
		if nonce == demandNonce {
			readyTxs = append(readyTxs, m.items[demandNonce])
			demandNonce++
			return true
		}
		return false
	})

	return readyTxs, demandNonce
}

// forward removes all allTxs from the map with a nonce lower than the
// provided commitNonce.
func (m *txSortedMap[T, Constraint]) forward(commitNonce uint64) []*internalTransaction[T, Constraint] {
	removedTxs := make([]*internalTransaction[T, Constraint], 0)
	commitNonceKey := makeSortedNonceKey(commitNonce)
	m.index.data.AscendLessThan(commitNonceKey, func(i btree.Item) bool {
		// delete tx from map.
		nonce := i.(*sortedNonceKey).nonce
		txItem := m.items[nonce]
		removedTxs = append(removedTxs, txItem)
		return true
	})
	return removedTxs
}

func (m *txSortedMap[T, Constraint]) behind(highestNonce uint64) []*internalTransaction[T, Constraint] {
	removedTxs := make([]*internalTransaction[T, Constraint], 0)
	highestNonceKey := makeSortedNonceKey(highestNonce)
	m.index.data.AscendGreaterOrEqual(highestNonceKey, func(i btree.Item) bool {
		// delete tx from map.
		nonce := i.(*sortedNonceKey).nonce
		txItem := m.items[nonce]
		removedTxs = append(removedTxs, txItem)
		return true
	})
	return removedTxs
}

type nonceCache struct {
	// commitNonces records each account's latest committed nonce in ledger.
	commitNonces map[string]uint64

	// pendingNonces records each account's latest nonce which has been included in
	// priority minNonceQueue. Invariant: pendingNonces[account] >= commitNonces[account]
	pendingNonces map[string]uint64

	pendingMu       sync.RWMutex
	commitMu        sync.Mutex
	getAccountNonce GetAccountNonceFunc
}

func newNonceCache(f GetAccountNonceFunc) *nonceCache {
	return &nonceCache{
		commitNonces:    make(map[string]uint64),
		pendingNonces:   make(map[string]uint64),
		getAccountNonce: f,
	}
}

func (nc *nonceCache) getCommitNonce(account string) uint64 {
	nc.commitMu.Lock()
	defer nc.commitMu.Unlock()

	nonce, ok := nc.commitNonces[account]
	if !ok {
		cn := nc.getAccountNonce(account)
		nc.commitNonces[account] = cn
		return cn
	}
	return nonce
}

func (nc *nonceCache) setCommitNonce(account string, nonce uint64) {
	nc.commitNonces[account] = nonce
}

func (nc *nonceCache) getPendingNonce(account string) uint64 {
	nc.pendingMu.RLock()
	defer nc.pendingMu.RUnlock()
	nonce, ok := nc.pendingNonces[account]
	if !ok {
		// if nonce is unknown, check if there is committed nonce persisted in db
		// cause there are no pending txs in txpool now, pending nonce is equal to committed nonce
		return nc.getCommitNonce(account)
	}
	return nonce
}

func (nc *nonceCache) setPendingNonce(account string, nonce uint64) {
	nc.pendingMu.Lock()
	nc.pendingNonces[account] = nonce
	nc.pendingMu.Unlock()
}

func (tx *internalTransaction[T, Constraint]) getRawTimestamp() int64 {
	return Constraint(tx.rawTx).RbftGetTimeStamp()
}

func (tx *internalTransaction[T, Constraint]) getAccount() string {
	return Constraint(tx.rawTx).RbftGetFrom()
}

func (tx *internalTransaction[T, Constraint]) getNonce() uint64 {
	return Constraint(tx.rawTx).RbftGetNonce()
}

func (tx *internalTransaction[T, Constraint]) getHash() string {
	return Constraint(tx.rawTx).RbftGetTxHash()
}

func (tx *internalTransaction[T, Constraint]) getGasPrice() *big.Int {
	return Constraint(tx.rawTx).RbftGetGasPrice()
}
