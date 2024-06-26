package txpool

import (
	"fmt"

	"github.com/google/btree"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-kit/types"
)

const (
	Ordered = iota
	SortNonce
	Remove
	Rebroadcast

	btreeDegree = 10
)

// the key of priorityByTime and parkingLotIndex.
type orderedIndexKey struct {
	time    int64
	account string
	nonce   uint64
}

// Less should guarantee item can be cast into orderedIndexKey.
func (oik *orderedIndexKey) Less(than btree.Item) bool {
	other := than.(*orderedIndexKey)
	if oik.time != other.time {
		return oik.time < other.time
	}
	if oik.account != other.account {
		return oik.account < other.account
	}
	return oik.nonce < other.nonce
}

type sortedNonceKey struct {
	nonce uint64
}

// Less should guarantee item can be cast into sortedNonceKey.
func (snk *sortedNonceKey) Less(item btree.Item) bool {
	dst, _ := item.(*sortedNonceKey)
	return snk.nonce < dst.nonce
}

func makeOrderedIndexKey(timestamp int64, account string, nonce uint64) *orderedIndexKey {
	return &orderedIndexKey{
		account: account,
		nonce:   nonce,
		time:    timestamp,
	}
}

func makeSortedNonceKey(nonce uint64) *sortedNonceKey {
	return &sortedNonceKey{
		nonce: nonce,
	}
}

type btreeIndex[T any, Constraint types.TXConstraint[T]] struct {
	data *btree.BTree
	typ  int
}

func newBtreeIndex[T any, Constraint types.TXConstraint[T]](typ int) *btreeIndex[T, Constraint] {
	return &btreeIndex[T, Constraint]{
		data: btree.New(btreeDegree),
		typ:  typ,
	}
}

func (idx *btreeIndex[T, Constraint]) getTimestamp(poolTx *internalTransaction[T, Constraint]) int64 {
	switch idx.typ {
	case Ordered:
		return poolTx.getRawTimestamp()
	case Rebroadcast:
		return poolTx.lifeTime
	case Remove:
		return poolTx.arrivedTime
	}
	return 0
}

func (idx *btreeIndex[T, Constraint]) insertKey(poolTx *internalTransaction[T, Constraint]) bool {
	switch idx.typ {
	case SortNonce:
		if old := idx.data.ReplaceOrInsert(makeSortedNonceKey(poolTx.getNonce())); old != nil {
			return true
		}

	case Ordered, Rebroadcast, Remove:
		if old := idx.data.ReplaceOrInsert(makeOrderedIndexKey(idx.getTimestamp(poolTx), poolTx.getAccount(), poolTx.getNonce())); old != nil {
			return true
		}
	}

	return false
}

func (idx *btreeIndex[T, Constraint]) removeKey(poolTx *internalTransaction[T, Constraint]) bool {
	switch idx.typ {
	case SortNonce:
		if old := idx.data.Delete(makeSortedNonceKey(poolTx.getNonce())); old != nil {
			return true
		}
	case Ordered, Rebroadcast, Remove:
		if old := idx.data.Delete(makeOrderedIndexKey(idx.getTimestamp(poolTx), poolTx.getAccount(), poolTx.getNonce())); old != nil {
			return true
		}
	}

	return false
}

func (idx *btreeIndex[T, Constraint]) removeBatchKeys(account string, txs []*internalTransaction[T, Constraint]) error {
	var err error
	lo.ForEach(txs, func(poolTx *internalTransaction[T, Constraint], _ int) {
		if poolTx.getAccount() != account {
			err = fmt.Errorf("account %s is not equal to %s", poolTx.getAccount(), account)
			return
		}
	})
	if err != nil {
		return err
	}
	lo.ForEach(txs, func(tx *internalTransaction[T, Constraint], _ int) {
		idx.removeKey(tx)
	})
	return nil
}

// size returns the size of the index
func (idx *btreeIndex[T, Constraint]) size() int {
	return idx.data.Len()
}

func (idx *btreeIndex[T, Constraint]) updateIndex(oldPoolTx *internalTransaction[T, Constraint], newTimestamp int64) {
	oldOrderedKey := &orderedIndexKey{time: idx.getTimestamp(oldPoolTx), account: oldPoolTx.getAccount(), nonce: oldPoolTx.getNonce()}
	newOrderedKey := &orderedIndexKey{time: newTimestamp, account: oldPoolTx.getAccount(), nonce: oldPoolTx.getNonce()}
	idx.data.Delete(oldOrderedKey)
	idx.data.ReplaceOrInsert(newOrderedKey)
}
