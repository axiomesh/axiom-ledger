package ledger

import (
	"math/big"
	"sync"

	"github.com/axiomesh/axiom-kit/types"
)

type stateChange interface {
	// revert undoes the state changes by this entry
	revert(*StateLedgerImpl)

	// dirted returns the address modified by this state entry
	dirtied() *types.Address
}

type stateChanger struct {
	changes []stateChange
	dirties map[types.Address]int // dirty address and the number of changes

	lock sync.RWMutex
}

func NewChanger() *stateChanger {
	return &stateChanger{
		dirties: make(map[types.Address]int),
	}
}

func (s *stateChanger) append(change stateChange) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.changes = append(s.changes, change)
	if addr := change.dirtied(); addr != nil {
		s.dirties[*addr]++
	}
}

func (s *stateChanger) revert(ledger *StateLedgerImpl, snapshot int) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for i := len(s.changes) - 1; i >= snapshot; i-- {
		s.changes[i].revert(ledger)

		if addr := s.changes[i].dirtied(); addr != nil {
			if s.dirties[*addr]--; s.dirties[*addr] == 0 {
				delete(s.dirties, *addr)
			}
		}
	}

	s.changes = s.changes[:snapshot]
}

func (s *stateChanger) dirty(addr types.Address) {
	s.dirties[addr]++
}

func (s *stateChanger) length() int {
	return len(s.changes)
}

type (
	createObjectChange struct {
		account *types.Address
	}

	resetObjectChange struct {
		prev IAccount
	}

	suicideChange struct {
		account     *types.Address
		prev        bool
		prevbalance *big.Int
	}

	balanceChange struct {
		account *types.Address
		prev    *big.Int
	}

	nonceChange struct {
		account *types.Address
		prev    uint64
	}

	storageChange struct {
		account       *types.Address
		key, prevalue []byte
	}

	codeChange struct {
		account  *types.Address
		prevcode []byte
	}

	refundChange struct {
		prev uint64
	}

	addLogChange struct {
		txHash *types.Hash
	}

	addPreimageChange struct {
		hash types.Hash
	}

	touchChange struct {
		account *types.Address
	}

	accessListAddAccountChange struct {
		address *types.Address
	}

	accessListAddSlotChange struct {
		address *types.Address
		slot    *types.Hash
	}

	transientStorageChange struct {
		account       *types.Address
		key, prevalue []byte
	}
)

func (ch createObjectChange) revert(l *StateLedgerImpl) {
	delete(l.accounts, ch.account.String())
}

func (ch createObjectChange) dirtied() *types.Address {
	return ch.account
}

// nolint
func (ch resetObjectChange) revert(l *StateLedgerImpl) {
	l.setAccount(ch.prev)
}

// nolint
func (ch resetObjectChange) dirtied() *types.Address {
	return nil
}

func (ch suicideChange) revert(l *StateLedgerImpl) {
	acc := l.GetOrCreateAccount(ch.account)
	account := acc.(*SimpleAccount)
	account.suicided = ch.prev
	account.setBalance(ch.prevbalance)
}

func (ch suicideChange) dirtied() *types.Address {
	return ch.account
}

// nolint
func (ch touchChange) revert(l *StateLedgerImpl) {}

func (ch touchChange) dirtied() *types.Address {
	return ch.account
}

func (ch balanceChange) revert(l *StateLedgerImpl) {
	l.GetOrCreateAccount(ch.account).(*SimpleAccount).setBalance(ch.prev)
}

func (ch balanceChange) dirtied() *types.Address {
	return ch.account
}

func (ch nonceChange) revert(l *StateLedgerImpl) {
	l.GetOrCreateAccount(ch.account).(*SimpleAccount).setNonce(ch.prev)
}

func (ch nonceChange) dirtied() *types.Address {
	return ch.account
}

func (ch codeChange) revert(l *StateLedgerImpl) {
	l.GetOrCreateAccount(ch.account).(*SimpleAccount).setCodeAndHash(ch.prevcode)
}

func (ch codeChange) dirtied() *types.Address {
	return ch.account
}

func (ch storageChange) revert(l *StateLedgerImpl) {
	l.GetOrCreateAccount(ch.account).(*SimpleAccount).setState(ch.key, ch.prevalue)
}

func (ch storageChange) dirtied() *types.Address {
	return ch.account
}

func (ch refundChange) revert(l *StateLedgerImpl) {
	l.refund = ch.prev
}

func (ch refundChange) dirtied() *types.Address {
	return nil
}

func (ch addPreimageChange) revert(l *StateLedgerImpl) {
	delete(l.preimages, ch.hash)
}

func (ch addPreimageChange) dirtied() *types.Address {
	return nil
}

func (ch accessListAddAccountChange) revert(l *StateLedgerImpl) {
	l.accessList.DeleteAddress(*ch.address)
}

func (ch accessListAddAccountChange) dirtied() *types.Address {
	return nil
}

func (ch accessListAddSlotChange) revert(l *StateLedgerImpl) {
	l.accessList.DeleteSlot(*ch.address, *ch.slot)
}

func (ch accessListAddSlotChange) dirtied() *types.Address {
	return nil
}

func (ch addLogChange) revert(l *StateLedgerImpl) {
	logs := l.logs.logs[*ch.txHash]
	if len(logs) == 1 {
		delete(l.logs.logs, *ch.txHash)
	} else {
		l.logs.logs[*ch.txHash] = logs[:len(logs)-1]
	}
	l.logs.logSize--
}

func (ch addLogChange) dirtied() *types.Address {
	return nil
}

func (ch transientStorageChange) revert(l *StateLedgerImpl) {
	l.setTransientState(*ch.account, ch.key, ch.prevalue)
}

func (ch transientStorageChange) dirtied() *types.Address {
	return nil
}

// clear empties the changer's internal state in place, so that any
// SimpleAccount which was created with this changer keeps a valid
// pointer and the next EVM transaction can still append to the same
// journal. Calling clear instead of swapping in a fresh *stateChanger
// is what makes RevertToSnapshot correct after ClearChangerAndRefund:
// previously the old l.changer was orphaned, the EVM's storage writes
// were tracked in the old pointer, and a subsequent revert looked at
// the empty new pointer and silently did nothing — leaking the
// failed-transaction state into the next block.
//
// This is called by StateLedgerImpl.ClearChangerAndRefund at the end
// of every transaction (defer in BlockExecutor.applyTransaction) and
// at the end of every block (StateLedgerImpl.Finalise). Both call sites
// require the same *stateChanger instance to be retained for the
// lifetime of any in-flight EVM execution so that a failed transaction
// can still rewind its journal correctly.
func (s *stateChanger) clear() {
	s.lock.Lock()
	defer s.lock.Unlock()
	// s.changes[:0] keeps the underlying array alive so the next batch
	// of changes can reuse the allocated capacity instead of forcing a
	// fresh allocation. dirties, being a map, has to be re-cleared by
	// deleting keys; we deliberately do NOT allocate a new map here so
	// the existing bucket backing stays in place across transactions.
	s.changes = s.changes[:0]
	for k := range s.dirties {
		delete(s.dirties, k)
	}
}
