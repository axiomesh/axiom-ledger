package ledger

import (
	"github.com/axiomesh/axiom-kit/types"
)

type ArchiveStateChange interface {
	// revert undoes the state changes by this entry
	Revert(ledger *ArchiveStateLedger)

	// dirted returns the address modified by this state entry
	dirtied() *types.Address
}

type ArchiveStateChanger struct {
	changes []ArchiveStateChange
	dirties map[types.Address]int // dirty address and the number of changes
}

func NewChanger() *ArchiveStateChanger {
	return &ArchiveStateChanger{
		dirties: make(map[types.Address]int),
	}
}

func (s *ArchiveStateChanger) Append(change ArchiveStateChange) {
	s.changes = append(s.changes, change)
	if addr := change.dirtied(); addr != nil {
		s.dirties[*addr]++
	}
}

func (s *ArchiveStateChanger) Revert(ledger *ArchiveStateLedger, snapshot int) {
	for i := len(s.changes) - 1; i >= snapshot; i-- {
		s.changes[i].Revert(ledger)

		if addr := s.changes[i].dirtied(); addr != nil {
			if s.dirties[*addr]--; s.dirties[*addr] == 0 {
				delete(s.dirties, *addr)
			}
		}
	}

	s.changes = s.changes[:snapshot]
}

func (s *ArchiveStateChanger) dirty(addr types.Address) {
	s.dirties[addr]++
}

func (s *ArchiveStateChanger) length() int {
	return len(s.changes)
}

func (s *ArchiveStateChanger) Reset() {
	s.changes = []ArchiveStateChange{}
	s.dirties = make(map[types.Address]int)
}

type (

	//RustResetObjectChange struct {
	//	prev IAccount
	//}
	//
	//RustRefundChange struct {
	//	prev uint64
	//}

	ArchiveAddLogChange struct {
		txHash *types.Hash
	}

	//RustAddPreimageChange struct {
	//	hash types.Hash
	//}
	//
	ArchiveAccessListAddAccountChange struct {
		address *types.Address
	}

	ArchiveAccessListAddSlotChange struct {
		address *types.Address
		slot    *types.Hash
	}
	//
	//RustTransientStorageChange struct {
	//	account       *types.Address
	//	key, prevalue []byte
	//}
)

//func (ch RustCreateObjectChange) Revert(l *ArchiveStateLedger) {
//	delete(l.Accounts, ch.account.String())
//}
//
//func (ch RustCreateObjectChange) dirtied() *types.Address {
//	return ch.account
//}

//	func (ch RustRefundChange) Revert(l *ArchiveStateLedger) {
//		l.Refund = ch.prev
//	}
//
//	func (ch RustRefundChange) dirtied() *types.Address {
//		return nil
//	}
//
//	func (ch RustAddPreimageChange) Revert(l *ArchiveStateLedger) {
//		delete(l.Preimages, ch.hash)
//	}
//
//	func (ch RustAddPreimageChange) dirtied() *types.Address {
//		return nil
//	}
func (ch ArchiveAccessListAddAccountChange) Revert(l *ArchiveStateLedger) {
	l.AccessList.DeleteAddress(*ch.address)
}

func (ch ArchiveAccessListAddAccountChange) dirtied() *types.Address {
	return nil
}

func (ch ArchiveAccessListAddSlotChange) Revert(l *ArchiveStateLedger) {
	l.AccessList.DeleteSlot(*ch.address, *ch.slot)
}

func (ch ArchiveAccessListAddSlotChange) dirtied() *types.Address {
	return nil
}

func (ch ArchiveAddLogChange) Revert(l *ArchiveStateLedger) {
	logs := l.Logs.logs[*ch.txHash]
	if len(logs) == 1 {
		delete(l.Logs.logs, *ch.txHash)
	} else {
		l.Logs.logs[*ch.txHash] = logs[:len(logs)-1]
	}
	l.Logs.logSize--
}

func (ch ArchiveAddLogChange) dirtied() *types.Address {
	return nil
}

//func (ch RustTransientStorageChange) Revert(l *ArchiveStateLedger) {
//	l.setTransientState(*ch.account, ch.key, ch.prevalue)
//}
//
//func (ch RustTransientStorageChange) dirtied() *types.Address {
//	return nil
//}
