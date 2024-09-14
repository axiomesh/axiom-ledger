package ledger

import (
	"github.com/ethereum/go-ethereum/common"
	etherTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"

	"github.com/axiomesh/axiom-kit/intutil"
	"github.com/axiomesh/axiom-kit/types"
)

func (esa EvmStateDBAdaptor) CreateAccount(address common.Address) {
	esa.StateLedger.GetOrCreateAccount(types.NewAddress(address.Bytes()))
}

func (esa EvmStateDBAdaptor) SubBalance(addr common.Address, amount *uint256.Int) {
	esa.StateLedger.SubBalance(types.NewAddress(addr.Bytes()), intutil.Uint256ToBigInt(amount))
}

func (esa EvmStateDBAdaptor) AddBalance(addr common.Address, amount *uint256.Int) {
	esa.StateLedger.AddBalance(types.NewAddress(addr.Bytes()), intutil.Uint256ToBigInt(amount))
}

func (esa EvmStateDBAdaptor) GetBalance(addr common.Address) *uint256.Int {
	bigIntBalance := esa.StateLedger.GetBalance(types.NewAddress(addr.Bytes()))
	uintBalance, _ := intutil.BigIntToUint256(bigIntBalance)
	return uintBalance
}

func (esa EvmStateDBAdaptor) GetNonce(addr common.Address) uint64 {
	return esa.StateLedger.GetNonce(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) SetNonce(addr common.Address, nonce uint64) {
	esa.StateLedger.SetNonce(types.NewAddress(addr.Bytes()), nonce)
}

func (esa EvmStateDBAdaptor) GetCodeHash(addr common.Address) common.Hash {
	return common.BytesToHash(esa.StateLedger.GetCodeHash(types.NewAddress(addr.Bytes())).Bytes())
}

func (esa EvmStateDBAdaptor) GetCode(addr common.Address) []byte {
	return esa.StateLedger.GetCode(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) SetCode(addr common.Address, code []byte) {
	esa.StateLedger.SetCode(types.NewAddress(addr.Bytes()), code)
}

func (esa EvmStateDBAdaptor) GetCodeSize(addr common.Address) int {
	return esa.StateLedger.GetCodeSize(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) AddRefund(gas uint64) {
	esa.StateLedger.AddRefund(gas)
}

func (esa EvmStateDBAdaptor) SubRefund(gas uint64) {
	esa.StateLedger.SubRefund(gas)
}

func (esa EvmStateDBAdaptor) GetRefund() uint64 {
	return esa.StateLedger.GetRefund()
}

func (esa EvmStateDBAdaptor) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	ret := esa.StateLedger.(*RustStateLedger).GetBit256CommittedState(types.NewAddress(addr.Bytes()), hash.Bytes())
	return ret
}

func (esa EvmStateDBAdaptor) GetState(addr common.Address, hash common.Hash) common.Hash {
	return esa.StateLedger.(*RustStateLedger).GetBit256State(types.NewAddress(addr.Bytes()), hash.Bytes())
}

func (esa EvmStateDBAdaptor) SetState(addr common.Address, key, value common.Hash) {
	esa.StateLedger.SetState(types.NewAddress(addr.Bytes()), key.Bytes(), value.Bytes())
}

func (esa EvmStateDBAdaptor) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	impl := esa.StateLedger.(*StateLedgerImpl)
	if impl != nil {
		return impl.transientStorage.Get(*types.NewAddress(addr.Bytes()), key)
	}
	return common.Hash{}
}

func (esa EvmStateDBAdaptor) SetTransientState(addr common.Address, key, value common.Hash) {
	prev := esa.GetTransientState(addr, key)
	if prev == value {
		return
	}
	if impl := esa.StateLedger.(*RustStateLedger); impl != nil {
		impl.setTransientState(*types.NewAddress(addr.Bytes()), key, value)
	}
}

func (esa EvmStateDBAdaptor) Exist(addr common.Address) bool {
	return esa.StateLedger.Exist(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) Empty(addr common.Address) bool {
	return esa.StateLedger.Empty(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) AddressInAccessList(addr common.Address) bool {
	return esa.StateLedger.AddressInAccessList(*types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) SlotInAccessList(addr common.Address, slot common.Hash) (addressOk bool, slotOk bool) {
	return esa.StateLedger.SlotInAccessList(*types.NewAddress(addr.Bytes()), *types.NewHash(slot.Bytes()))
}

func (esa EvmStateDBAdaptor) AddAddressToAccessList(addr common.Address) {
	esa.StateLedger.AddAddressToAccessList(*types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	esa.StateLedger.AddSlotToAccessList(*types.NewAddress(addr.Bytes()), *types.NewHash(slot.Bytes()))
}

func (esa EvmStateDBAdaptor) Prepare(rules params.Rules, sender, coinbase common.Address, dst *common.Address, precompiles []common.Address, list etherTypes.AccessList) {
	esa.StateLedger.(*RustStateLedger).Prepare(rules, sender, coinbase, dst, precompiles, list)
	// l.logs.thash = types.NewHash(hash.Bytes())
	// l.logs.txIndex = index
	l, ok := esa.StateLedger.(*StateLedgerImpl)
	if !ok {
		return
	}
	l.accessList = NewAccessList()
	if rules.IsBerlin {
		// Clear out any leftover from previous executions
		al := NewAccessList()
		l.accessList = al

		al.AddAddress(*types.NewAddress(sender.Bytes()))
		if dst != nil {
			al.AddAddress(*types.NewAddress(dst.Bytes()))
			// If it's a create-tx, the destination will be added inside evm.create
		}
		for _, addr := range precompiles {
			al.AddAddress(*types.NewAddress(addr.Bytes()))
		}
		for _, el := range list {
			al.AddAddress(*types.NewAddress(el.Address.Bytes()))
			for _, key := range el.StorageKeys {
				al.AddSlot(*types.NewAddress(el.Address.Bytes()), *types.NewHash(key.Bytes()))
			}
		}
		// if rules.IsShanghai { // EIP-3651: warm coinbase
		// 	al.AddAddress(coinbase)
		// }
	}
	// Reset transient storage at the beginning of transaction execution
	l.transientStorage = newTransientStorage()
}

func (esa EvmStateDBAdaptor) RevertToSnapshot(revid int) {
	esa.StateLedger.RevertToSnapshot(revid)
}

func (esa EvmStateDBAdaptor) Snapshot() int {
	return esa.StateLedger.Snapshot()
}

func (esa EvmStateDBAdaptor) AddLog(log *etherTypes.Log) {
	var topics []*types.Hash
	for _, topic := range log.Topics {
		topics = append(topics, types.NewHash(topic.Bytes()))
	}
	logs := &types.EvmLog{
		Address:     types.NewAddress(log.Address.Bytes()),
		Topics:      topics,
		Data:        log.Data,
		BlockNumber: log.BlockNumber,
		BlockHash:   types.NewHash(log.BlockHash.Bytes()),
		LogIndex:    uint64(log.Index),
		Removed:     log.Removed,
	}
	esa.StateLedger.AddLog(logs)
}

func (esa EvmStateDBAdaptor) AddPreimage(hash common.Hash, data []byte) {
	esa.StateLedger.AddPreimage(*types.NewHash(hash.Bytes()), data)
}

func (esa EvmStateDBAdaptor) StateDB() vm.StateDB {
	return esa
}

func (esa EvmStateDBAdaptor) PrepareEVMAccessList(sender common.Address, dest *common.Address, preEVMcompiles []common.Address, txEVMAccesses etherTypes.AccessList) {
	l, ok := esa.StateLedger.(*StateLedgerImpl)
	if !ok {
		return
	}
	var precompiles []types.Address
	for _, compile := range preEVMcompiles {
		precompiles = append(precompiles, *types.NewAddress(compile.Bytes()))
	}
	var txAccesses AccessTupleList
	for _, list := range txEVMAccesses {
		var storageKeys []types.Hash
		for _, keys := range list.StorageKeys {
			storageKeys = append(storageKeys, *types.NewHash(keys.Bytes()))
		}
		txAccesses = append(txAccesses, AccessTuple{Address: *types.NewAddress(list.Address.Bytes()), StorageKeys: storageKeys})
	}

	l.PrepareAccessList(*types.NewAddress(sender.Bytes()), types.NewAddress(dest.Bytes()), precompiles, txAccesses)
}

func (esa EvmStateDBAdaptor) SelfDestruct(addr common.Address) {
	esa.StateLedger.SelfDestruct(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) HasSelfDestructed(addr common.Address) bool {
	return esa.StateLedger.HasSelfDestructed(types.NewAddress(addr.Bytes()))
}

func (esa EvmStateDBAdaptor) Selfdestruct6780(addr common.Address) {
	esa.StateLedger.Selfdestruct6780(types.NewAddress(addr.Bytes()))
}

type evmLogs struct {
	Logs    map[types.Hash][]*types.EvmLog
	LogSize uint
}

func newEvmLogs() *evmLogs {
	return &evmLogs{
		Logs: make(map[types.Hash][]*types.EvmLog),
	}
}

type EvmReceipts []*types.Receipt

func CreateBloom(receipts EvmReceipts) *types.Bloom {
	var bin types.Bloom
	for _, receipt := range receipts {
		for _, log := range receipt.EvmLogs {
			bin.Add(log.Address.Bytes())
			for _, b := range log.Topics {
				bin.Add(b.Bytes())
			}
		}
	}
	return &bin
}
