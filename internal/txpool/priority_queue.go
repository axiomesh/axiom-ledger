package txpool

import (
	"container/heap"
	"fmt"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/sirupsen/logrus"
)

type priceQueue[T any, Constraint types.TXConstraint[T]] struct {
	priced        *TxByPriceAndTime[T, Constraint]               // every account exist only lowest nonce transaction in the queue
	dirtyAccounts map[string]*internalTransaction[T, Constraint] // account -> current pending nonce
	logger        logrus.FieldLogger
}

func newPriceQueue[T any, Constraint types.TXConstraint[T]](logger logrus.FieldLogger) *priceQueue[T, Constraint] {
	p := &priceQueue[T, Constraint]{
		priced:        new(TxByPriceAndTime[T, Constraint]),
		dirtyAccounts: make(map[string]*internalTransaction[T, Constraint]),
		logger:        logger,
	}
	*p.priced = make(TxByPriceAndTime[T, Constraint], 0)
	heap.Init(p.priced)
	return p
}

// push if the account is already in the dirtyAccounts, omit it
// otherwise push it to the priced queue
func (p *priceQueue[T, Constraint]) push(tx *internalTransaction[T, Constraint]) {
	from := tx.getAccount()
	oldTx, ok := p.dirtyAccounts[from]
	if ok {
		// if the old tx has smaller nonce, do nothing
		if oldTx.getNonce() < tx.getNonce() {
			return
		}
		// remove the old tx
		p.priced.remove(oldTx)
	}
	p.dirtyAccounts[from] = tx
	p.priced.push(tx)

}

func (p *priceQueue[T, Constraint]) pop() *internalTransaction[T, Constraint] {
	if p.priced.Len() == 0 {
		return nil
	}
	tx, ok := p.priced.pop().(*internalTransaction[T, Constraint])
	if !ok {
		return nil
	}
	delete(p.dirtyAccounts, tx.getAccount())
	return tx
}

func (p *priceQueue[T, Constraint]) peek() *internalTransaction[T, Constraint] {
	if p.priced.Len() == 0 {
		return nil
	}
	return p.priced.peek()
}

func (p *priceQueue[T, Constraint]) remove(tx *internalTransaction[T, Constraint]) bool {
	from := tx.getAccount()
	if old, ok := p.dirtyAccounts[from]; !ok || old.getNonce() != tx.getNonce() {
		return false
	}

	delete(p.dirtyAccounts, from)
	return p.priced.remove(tx)
}

func (p *priceQueue[T, Constraint]) length() int {
	return p.priced.Len()
}

type priorityQueue[T any, Constraint types.TXConstraint[T]] struct {
	// track all priority txs of account in the pool
	accountsM map[string]*accountQueue[T, Constraint]

	//store the tx sorted by gas price(descending order)
	txsByPrice *priceQueue[T, Constraint]

	// track the priority transaction.
	nonBatchSize uint64

	getNonceFn func(from string) uint64

	logger logrus.FieldLogger
}

func newPriorityQueue[T any, Constraint types.TXConstraint[T]](fn func(string) uint64, logger logrus.FieldLogger) *priorityQueue[T, Constraint] {
	return &priorityQueue[T, Constraint]{
		accountsM:    make(map[string]*accountQueue[T, Constraint]),
		txsByPrice:   newPriceQueue[T, Constraint](logger),
		nonBatchSize: 0,
		getNonceFn:   fn,
		logger:       logger,
	}
}
func (p *priorityQueue[T, Constraint]) push(tx *internalTransaction[T, Constraint]) {
	from := tx.getAccount()
	account, ok := p.accountsM[from]
	if !ok {
		p.accountsM[from] = newAccountQueue[T, Constraint](p.getNonceFn(from))
		account = p.accountsM[from]
	}

	if !account.isValidPush(tx.getNonce()) {
		p.logger.Errorf("pushing tx[account: %s, nonce: %d] to priority queue failed, expected nonce: %d", from, tx.getNonce(), account.lastNonce+1)
		return
	}
	if replaced := p.accountsM[from].push(tx); !replaced {
		p.increasePrioritySize()
	}
	p.txsByPrice.push(tx)
}

func (p *priorityQueue[T, Constraint]) pushBack(tx *internalTransaction[T, Constraint]) {
	from := tx.getAccount()
	account, ok := p.accountsM[from]
	if !ok {
		p.accountsM[from] = newAccountQueue[T, Constraint](p.getNonceFn(from))
		account = p.accountsM[from]
	}

	if _, ok = account.items[tx.getNonce()]; !ok {
		account.push(tx)
		p.increasePrioritySize()
	}
	p.txsByPrice.push(tx)
}

func (p *priorityQueue[T, Constraint]) pop() *internalTransaction[T, Constraint] {
	if p.txsByPrice.length() == 0 {
		return nil
	}
	tx := p.txsByPrice.pop()
	if p.accountsM[tx.getAccount()].pop().getNonce() != tx.getNonce() {
		panic(fmt.Errorf("pop tx from priority queue err: nonce not match"))
	}

	from := tx.getAccount()
	lastNonce := tx.getNonce()
	p.shift(from, lastNonce)
	p.decreasePrioritySize()
	return tx
}

func (p *priorityQueue[T, Constraint]) peek() *internalTransaction[T, Constraint] {
	if p.txsByPrice.length() == 0 {
		return nil
	}
	return p.txsByPrice.peek()
}

func (p *priorityQueue[T, Constraint]) shift(from string, lastNonce uint64) {
	account, ok := p.accountsM[from]
	if !ok {
		return
	}

	nonce := account.minNonceQueue.peek()
	if lastNonce+1 != nonce {
		return
	}

	if _, ok := p.txsByPrice.dirtyAccounts[from]; ok {
		panic(fmt.Errorf("shift tx to txsByPrice err: dirty account is not empty"))
	}
	p.txsByPrice.push(account.items[nonce])
}

func (p *priorityQueue[T, Constraint]) increasePrioritySize() {
	p.nonBatchSize++
}

func (p *priorityQueue[T, Constraint]) decreasePrioritySize() {
	p.nonBatchSize--
}

func (p *priorityQueue[T, Constraint]) setPrioritySize(size uint64) {
	p.nonBatchSize = size
}

func (p *priorityQueue[T, Constraint]) updateAccountNonce(from string, nonce uint64) {
	if account, ok := p.accountsM[from]; ok {
		account.updateLastNonce(nonce)
	}
}

func (p *priorityQueue[T, Constraint]) replaceTx(tx *internalTransaction[T, Constraint]) bool {
	from := tx.getAccount()
	account, ok := p.accountsM[from]
	if !ok {
		return false
	}

	oldTx := account.items[tx.getNonce()]

	if replaced := account.push(tx); replaced {
		if p.txsByPrice.remove(oldTx) {
			p.txsByPrice.push(tx)
			return true
		}
	}

	return false
}

func (p *priorityQueue[T, Constraint]) removeTxBehindNonce(tx *internalTransaction[T, Constraint]) []*internalTransaction[T, Constraint] {
	from := tx.getAccount()
	account, ok := p.accountsM[from]
	if !ok {
		return nil
	}

	p.txsByPrice.remove(tx)
	if removeTxs, ok := account.removeBehind(tx.getNonce()); ok {
		oldSize := p.nonBatchSize
		p.setPrioritySize(oldSize - uint64(len(removeTxs)))
		return removeTxs
	}

	return nil
}

// remove txs which lower than given nonce(including the given nonce)
func (p *priorityQueue[T, Constraint]) removeTxBeforeNonce(from string, nonce uint64) []*internalTransaction[T, Constraint] {
	account, ok := p.accountsM[from]
	if !ok {
		p.logger.Debugf("account %s not found in priority queue", from)
		return nil
	}
	minNonceTx := account.peek()
	if minNonceTx == nil {
		p.logger.Debugf("account %s has no tx", from)
		return nil
	}

	if nonce < minNonceTx.getNonce() {
		p.logger.Debugf("account %s min nonce[%d] is bigger than %d", from, minNonceTx.getNonce(), nonce)
		return nil
	}

	p.txsByPrice.remove(minNonceTx)

	p.logger.Debugf("start to remove txs before nonce %d from account %s, local min nonce is %d", nonce, from, minNonceTx.getNonce())
	removeTxs := account.prune(nonce)
	p.logger.Debugf("remove %d txs from account %s", len(removeTxs), from)
	oldSize := p.nonBatchSize
	p.setPrioritySize(oldSize - uint64(len(removeTxs)))
	if shiftTx := account.peek(); shiftTx != nil {
		p.txsByPrice.push(shiftTx)
	}
	return removeTxs
}

func (p *priorityQueue[T, Constraint]) size() uint64 {
	return p.nonBatchSize
}
