package ledger

import (
	lru "github.com/hashicorp/golang-lru/v2"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	etherTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"

	"github.com/axiomesh/axiom-kit/types"
)

func TestEvmStateDBAdaptor_TestSnapshot(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)
	evmStateDB := EvmStateDBAdaptor{StateLedger: sl}
	// create an account
	account := types.NewAddress(LeftPadBytes([]byte{110}, 20))
	// change the balance
	input := uint256.NewInt(1000)
	evmStateDB.AddBalance(account.ETHAddress(), input)
	// keep the snapshot
	ss := evmStateDB.Snapshot()
	// change the balance twice
	evmStateDB.AddBalance(account.ETHAddress(), input)
	// revert the snapshot
	evmStateDB.RevertToSnapshot(ss)
	// check the balance
	balance := evmStateDB.GetBalance(account.ETHAddress())
	assert.Equal(t, input, balance)
}

func TestEvmStateDBAdaptor_PrepareEVMAccessList(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)
	evmStateDB := EvmStateDBAdaptor{StateLedger: sl}
	account := common.BytesToAddress(LeftPadBytes([]byte{111}, 20))
	at1 := etherTypes.AccessTuple{
		Address:     account,
		StorageKeys: []common.Hash{common.BytesToHash([]byte{1})},
	}
	evmStateDB.PrepareEVMAccessList(account, &account, []common.Address{account}, etherTypes.AccessList{at1})
	isIn := evmStateDB.AddressInAccessList(account)
	assert.Equal(t, true, isIn)
}

func TestEvmStateDBAdaptor_Selfdestruct6780(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)
	evmStateDB := EvmStateDBAdaptor{StateLedger: sl}
	address := common.BytesToAddress(LeftPadBytes([]byte{111}, 20))
	// case for non-existent account
	evmStateDB.Selfdestruct6780(address)

	// case for existing account
	account := sl.GetOrCreateAccount(types.NewAddress(address.Bytes()))
	account.AddBalance(big.NewInt(1000))
	evmStateDB.Selfdestruct6780(address)
	iAccount := sl.GetAccount(types.NewAddress(address.Bytes()))
	assert.Equal(t, iAccount.GetBalance(), big.NewInt(0))
	sa := iAccount.(*SimpleAccount)
	assert.Equal(t, sa.selfDestructed, true)
}

func TestDeleteCreateRevert(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)
	addr := types.NewAddress(LeftPadBytes([]byte{100}, 20))
	sl.blockHeight = 1
	sl.Finalise()
	_, err := sl.Commit()
	assert.Nil(t, err)

	code := RightPadBytes([]byte{100}, 100)
	lg.StateLedger.SetCode(addr, code)
	sl.blockHeight = 2
	sl.Finalise()
	_, err = sl.Commit()
	assert.Nil(t, err)

	acc := sl.GetAccount(addr)
	assert.NotNil(t, acc)
	// Simulate self-destructing in one transaction, then create-reverting in another
	evmLg := &EvmStateDBAdaptor{StateLedger: lg.StateLedger}
	evmLg.SelfDestruct(addr.ETHAddress())
	sl.Finalise()

	id := evmLg.Snapshot()
	evmLg.AddBalance(addr.ETHAddress(), uint256.NewInt(1))
	evmLg.RevertToSnapshot(id)
	sl.blockHeight = 3
	sl.Finalise()
	_, err = sl.Commit()
	assert.Nil(t, err)
	t.Logf("account == nil %v", sl.GetAccount(addr) == nil)
	t.Logf("self-destructed has finished %v", sl.HasSelfDestructed(addr))
	assert.True(t, sl.GetAccount(addr) == nil)
	assert.Equal(t, sl.HasSelfDestructed(addr), false)
}
func TestName(t *testing.T) {
	l, err := lru.New[uint64, []byte](1)
	if err != nil {
		panic(err)
	}
	l.Add(1, []byte("hello"))
	value, ok := l.Get(1)
	println(string(value))
	println(ok)
}
