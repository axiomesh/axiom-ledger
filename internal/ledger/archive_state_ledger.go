package ledger

/*
#cgo LDFLAGS: /Users/koi/Documents/dev/project/forestore/target/release/libforestore.a  -ldl -lm
#include "/Users/zhangqirui/workplace/forestore/src/c_ffi/forestore.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	ethcommon "github.com/ethereum/go-ethereum/common"
	etherTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"math/big"
	"os"
	"path"
	"sort"
	"unsafe"
)

type ArchiveStateLedger struct {
	repo       *repo.Repo
	stateDBPtr *C.struct_EvmStateDB

	blockHeight uint64
	thash       *types.Hash
	txIndex     int

	validRevisions []revision
	nextRevisionId int
	changer        *ArchiveStateChanger

	Accounts map[string]IAccount
	Refund   uint64
	Logs     *evmLogs

	Preimages map[types.Hash][]byte

	transientStorage transientStorage
	AccessList       *AccessList
}

func NewArchiveStateLedger(rep *repo.Repo, height uint64) *ArchiveStateLedger {
	dir := path.Join(rep.RepoRoot, storagemgr.ArchiveRustLedger)
	log_dir := path.Join(rep.RepoRoot, storagemgr.ArchiveRustLedger, "logs")
	err := os.MkdirAll(log_dir, 0755)
	if err != nil {
		panic(err)
	}
	C.set_up_default_logger(C.CString(log_dir))

	version := height
	initialVersion := rep.GenesisConfig.EpochInfo.StartBlock
	if initialVersion != 0 && initialVersion != 1 {
		panic("Genesis start block num should be 0 or 1")
	}
	C.rollback_evm_state_db(C.CString(dir), C.uint64_t(version))

	stateDbPtr := C.new_evm_state_db(C.CString(dir), C.uint64_t(initialVersion))
	fmt.Println("stateDbPtr", unsafe.Pointer(stateDbPtr))
	//stateDBViewPtr := C.evm_state_db_view(stateDbPtr)
	return &ArchiveStateLedger{
		repo:             rep,
		stateDBPtr:       stateDbPtr,
		Accounts:         make(map[string]IAccount),
		Logs:             newEvmLogs(),
		Preimages:        make(map[types.Hash][]byte),
		transientStorage: newTransientStorage(),
		changer:          NewChanger(),

		AccessList: NewAccessList(),
	}
}

func (r *ArchiveStateLedger) GetOrCreateAccount(address *types.Address) IAccount {
	addr := address.String()

	value, ok := r.Accounts[addr]
	if ok {
		return value
	}
	account := NewArchiveAccount(r.stateDBPtr, address.ETHAddress(), r.changer)
	r.Accounts[addr] = account
	return account
}

// todo get from historical trie
func (r *ArchiveStateLedger) GetAccount(address *types.Address) IAccount {
	addr := address.String()

	value, ok := r.Accounts[addr]
	if ok {
		return value
	}
	cAddress := convertToCAddress(address.ETHAddress())
	exist := bool(C.exist(r.stateDBPtr, cAddress))
	if exist {
		account := NewArchiveAccount(r.stateDBPtr, address.ETHAddress(), r.changer)
		r.Accounts[addr] = account
		return account
	} else {
		return nil
	}
}

func (r *ArchiveStateLedger) GetBalance(address *types.Address) *big.Int {
	account := r.GetOrCreateAccount(address)
	return account.GetBalance()
}

func (r *ArchiveStateLedger) SetBalance(address *types.Address, balance *big.Int) {
	account := r.GetOrCreateAccount(address)
	account.SetBalance(balance)
}

func (r *ArchiveStateLedger) SubBalance(address *types.Address, balance *big.Int) {
	account := r.GetOrCreateAccount(address)
	account.SubBalance(balance)
}

func (r *ArchiveStateLedger) AddBalance(address *types.Address, balance *big.Int) {
	account := r.GetOrCreateAccount(address)
	account.AddBalance(balance)
}

func (r *ArchiveStateLedger) GetState(address *types.Address, bytes []byte) (bool, []byte) {
	account := r.GetOrCreateAccount(address)
	return account.GetState(bytes)
}

func (r *ArchiveStateLedger) GetBit256State(address *types.Address, bytes []byte) ethcommon.Hash {
	account := r.GetOrCreateAccount(address)
	return account.(*ArchiveAccount).GetBit256State(bytes)
}

func (r *ArchiveStateLedger) SetState(address *types.Address, key []byte, value []byte) {
	account := r.GetOrCreateAccount(address)
	account.SetState(key, value)
}

func (r *ArchiveStateLedger) SetBit256State(address *types.Address, key []byte, value ethcommon.Hash) {
	account := r.GetOrCreateAccount(address)
	account.(*ArchiveAccount).SetBit256State(key, value)
}

func (r *ArchiveStateLedger) SetCode(address *types.Address, bytes []byte) {
	account := r.GetOrCreateAccount(address)
	account.SetCodeAndHash(bytes)
}

func (r *ArchiveStateLedger) GetCode(address *types.Address) []byte {
	account := r.GetOrCreateAccount(address)
	return account.Code()
}

func (r *ArchiveStateLedger) SetNonce(address *types.Address, u uint64) {
	account := r.GetOrCreateAccount(address)
	account.SetNonce(u)
}

func (r *ArchiveStateLedger) GetNonce(address *types.Address) uint64 {
	account := r.GetOrCreateAccount(address)
	return account.GetNonce()
}

func (r *ArchiveStateLedger) GetCodeHash(address *types.Address) *types.Hash {
	account := r.GetOrCreateAccount(address)
	return types.NewHash(account.CodeHash())
}

func (r *ArchiveStateLedger) GetCodeSize(address *types.Address) int {
	account := r.GetOrCreateAccount(address)
	return len(account.Code())
}

func (r *ArchiveStateLedger) AddRefund(gas uint64) {
	C.add_refund(r.stateDBPtr, C.uint64_t(gas))
}

func (r *ArchiveStateLedger) SubRefund(gas uint64) {
	C.sub_refund(r.stateDBPtr, C.uint64_t(gas))
}

func (r *ArchiveStateLedger) GetRefund() uint64 {
	return r.Refund
}

func (r *ArchiveStateLedger) GetCommittedState(address *types.Address, bytes []byte) []byte {
	account := r.GetOrCreateAccount(address)
	return account.GetCommittedState(bytes)
}

func (r *ArchiveStateLedger) GetBit256CommittedState(address *types.Address, bytes []byte) ethcommon.Hash {
	account := r.GetOrCreateAccount(address)
	return account.(*ArchiveAccount).GetBit256CommittedState(bytes)
}

func (r *ArchiveStateLedger) Commit() (*types.StateJournal, error) {
	panic("archive state ledger not support commit operation")
}

func (r *ArchiveStateLedger) SelfDestruct(address *types.Address) bool {
	account := r.GetOrCreateAccount(address)
	account.SetSelfDestructed(true)
	account.SetBalance(new(big.Int))
	return true
}

func (r *ArchiveStateLedger) HasSelfDestructed(address *types.Address) bool {
	account := r.GetAccount(address)
	if account != nil {
		return account.SelfDestructed()
	}
	return false
}

func (r *ArchiveStateLedger) Selfdestruct6780(address *types.Address) {
	account := r.GetAccount(address)
	if account == nil {
		return
	}

	if account.IsCreated() {
		r.SelfDestruct(address)
	}
}

func (r *ArchiveStateLedger) Exist(address *types.Address) bool {
	exist := !r.GetOrCreateAccount(address).IsEmpty()
	return exist
}

func (r *ArchiveStateLedger) Empty(address *types.Address) bool {
	empty := r.GetOrCreateAccount(address).IsEmpty()
	return empty
}

func (r *ArchiveStateLedger) AddressInAccessList(addr types.Address) bool {
	return r.AccessList.ContainsAddress(addr)
}

func (r *ArchiveStateLedger) SlotInAccessList(addr types.Address, slot types.Hash) (bool, bool) {
	return r.AccessList.Contains(addr, slot)
}

func (r *ArchiveStateLedger) AddAddressToAccessList(addr types.Address) {
	if r.AccessList.AddAddress(addr) {
		r.changer.Append(ArchiveAccessListAddAccountChange{address: &addr})
	}
}

func (r *ArchiveStateLedger) AddSlotToAccessList(addr types.Address, slot types.Hash) {
	addrMod, slotMod := r.AccessList.AddSlot(addr, slot)
	if addrMod {
		r.changer.Append(ArchiveAccessListAddAccountChange{address: &addr})
	}
	if slotMod {
		r.changer.Append(ArchiveAccessListAddSlotChange{
			address: &addr,
			slot:    &slot,
		})
	}
}

func (r *ArchiveStateLedger) Prepare(rules params.Rules, sender, coinbase ethcommon.Address, dst *ethcommon.Address, precompiles []ethcommon.Address, list etherTypes.AccessList) {
	r.AccessList = NewAccessList()
	if rules.IsBerlin {
		// Clear out any leftover from previous executions
		al := NewAccessList()
		r.AccessList = al

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
	r.transientStorage = newTransientStorage()
}

func (r *ArchiveStateLedger) AddPreimage(address types.Hash, preimage []byte) {
	preimagePtr := (*C.uchar)(unsafe.Pointer(&preimage[0]))
	preimageLen := C.uintptr_t(len(preimage))
	C.add_preimage(r.stateDBPtr, convertToCH256(address.ETHHash()), preimagePtr, preimageLen)
}

func (r *ArchiveStateLedger) SetTxContext(thash *types.Hash, ti int) {
	r.thash = thash
	r.txIndex = ti

}

func (r *ArchiveStateLedger) Clear() {
	r.Accounts = make(map[string]IAccount)
}

func (r *ArchiveStateLedger) RevertToSnapshot(snapshot int) {
	idx := sort.Search(len(r.validRevisions), func(i int) bool {
		return r.validRevisions[i].id >= snapshot
	})
	if idx == len(r.validRevisions) || r.validRevisions[idx].id != snapshot {
		panic(fmt.Errorf("revision id %v cannod be reverted", snapshot))
	}
	snap := r.validRevisions[idx].changerIndex

	r.changer.Revert(r, snap)
	r.validRevisions = r.validRevisions[:idx]
	C.revert_to_snapshot(r.stateDBPtr, C.uintptr_t(snapshot))
}

func (r *ArchiveStateLedger) Snapshot() int {
	id := r.nextRevisionId
	r.nextRevisionId++
	r.validRevisions = append(r.validRevisions, revision{id: id, changerIndex: r.changer.length()})
	C.snapshot(r.stateDBPtr)
	return id
}

func (r *ArchiveStateLedger) setTransientState(addr types.Address, key, value ethcommon.Hash) {
	C.set_transient_state(r.stateDBPtr, convertToCAddress(addr.ETHAddress()), convertToCH256(key), convertToCH256(value))
}

func (r *ArchiveStateLedger) AddLog(log *types.EvmLog) {
	if log.TransactionHash == nil {
		log.TransactionHash = r.thash
	}

	log.TransactionIndex = uint64(r.txIndex)
	r.changer.Append(ArchiveAddLogChange{txHash: log.TransactionHash})
	log.LogIndex = uint64(r.Logs.logSize)
	if _, ok := r.Logs.logs[*log.TransactionHash]; !ok {
		r.Logs.logs[*log.TransactionHash] = make([]*types.EvmLog, 0)
	}

	r.Logs.logs[*log.TransactionHash] = append(r.Logs.logs[*log.TransactionHash], log)
	r.Logs.logSize++
}

func (r *ArchiveStateLedger) GetLogs(txHash types.Hash, height uint64) []*types.EvmLog {
	logs := r.Logs.logs[txHash]
	for _, l := range logs {
		l.BlockNumber = height
	}
	return logs
}

func (r *ArchiveStateLedger) RollbackState(height uint64, lastStateRoot *types.Hash) error {
	// TODO implement me
	//panic("implement me")
	return nil
}

func (r *ArchiveStateLedger) PrepareBlock(lastStateRoot *types.Hash, currentExecutingHeight uint64) {
	r.Logs = newEvmLogs()
	// TODO lastStateRoot relate logic
	r.blockHeight = currentExecutingHeight
}

func (r *ArchiveStateLedger) PrepareTranct() {
	C.prepare_transact(r.stateDBPtr)
}

func (r *ArchiveStateLedger) FinalizeTransact() {
	C.finalize_transact(r.stateDBPtr)
}

func (r *ArchiveStateLedger) RollbackTransact() {
	C.rollback_transact(r.stateDBPtr)
}

func (r *ArchiveStateLedger) ClearChangerAndRefund() {
	r.Refund = 0
	r.changer.Reset()
	r.validRevisions = r.validRevisions[:0]
	r.nextRevisionId = 0
}

func (r *ArchiveStateLedger) Close() {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) Finalise() {
	r.ClearChangerAndRefund()
}

func (r *ArchiveStateLedger) Version() uint64 {
	return r.blockHeight
}

func (r *ArchiveStateLedger) NewView(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	return &ArchiveStateLedger{
		repo: r.repo,
		//stateDBPtr:     r.stateDBViewPtr,
		//stateDBViewPtr: r.stateDBViewPtr,
		Accounts:    make(map[string]IAccount),
		Logs:        newEvmLogs(),
		blockHeight: blockHeader.Number,
	}, nil
}

func (r *ArchiveStateLedger) IterateTrie(snapshotMeta *utils.SnapshotMeta, kv kv.Storage, errC chan error) {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) GetTrieSnapshotMeta() (*utils.SnapshotMeta, error) {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) VerifyTrie(blockHeader *types.BlockHeader) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) Prove(rootHash ethcommon.Hash, key []byte) (*jmt.ProofResult, error) {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) GenerateSnapshot(blockHeader *types.BlockHeader, errC chan error) {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) GetHistoryRange() (uint64, uint64) {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) GetStateJournal(blockNumber uint64) *types.StateJournal {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) UpdateChainState(chainState *chainstate.ChainState) {

}

func (r *ArchiveStateLedger) Archive(blockHeader *types.BlockHeader, stateJournal *types.StateJournal) error {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) ApplyStateJournal(blockNumber uint64, stateJournal *types.StateJournal) error {
	//TODO implement me
	panic("implement me")
}

func (r *ArchiveStateLedger) ExportArchivedSnapshot(targetFilePath string) error {
	//TODO implement me
	panic("implement me")
}

var _ StateLedger = (*ArchiveStateLedger)(nil)
