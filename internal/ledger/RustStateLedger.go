package ledger

/*
#cgo LDFLAGS: "/Users/koi/Documents/dev/project/forestore/target/release/libforestore.a"  -ldl -lm
#include "/Users/koi/Documents/dev/project/forestore/src/c_ffi/forestore.h"
*/
import "C"
import (
	"fmt"
	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	ethcommon "github.com/ethereum/go-ethereum/common"
	etherTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru/v2"
	"math/big"
	"os"
	"path"
	"sort"
	"unsafe"
)

type RustStateLedger struct {
	repo       *repo.Repo
	stateDBPtr *C.struct_EvmStateDB

	stateDBViewPtr *C.struct_EvmStateDB
	blockHeight    uint64
	thash          *types.Hash
	txIndex        int

	validRevisions []revision
	nextRevisionId int
	changer        *RustStateChanger

	Accounts map[string]IAccount
	Refund   uint64
	Logs     *evmLogs

	Preimages map[types.Hash][]byte

	transientStorage transientStorage
	AccessList       *AccessList
	codeCache        *lru.Cache[ethcommon.Address, []byte]
} /**/

func NewRustStateLedger(rep *repo.Repo, height uint64) *RustStateLedger {
	dir := repo.GetStoragePath(rep.RepoRoot, storagemgr.Rust_Ledger)
	logDir := path.Join(dir, "logs")
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		panic(err)
	}
	C.set_up_default_logger(C.CString(logDir))

	version := height
	initialVersion := rep.GenesisConfig.EpochInfo.StartBlock
	if initialVersion != 0 && initialVersion != 1 {
		panic("Genesis start block num should be 0 or 1")
	}
	C.rollback_evm_state_db(C.CString(dir), C.uint64_t(version))
	cOpts := C.CEvmStateDBOptions{genesis_version: C.uint64_t(initialVersion), snapshot_rewrite_interval: C.uint64_t(rep.Config.Forestore.SnapshotRewriteInterval)}
	stateDbPtr := C.new_evm_state_db(C.CString(dir), cOpts)
	stateDBViewPtr := C.evm_state_db_view(stateDbPtr)
	codeCache, _ := lru.New[ethcommon.Address, []byte](1000)
	return &RustStateLedger{
		repo:           rep,
		stateDBPtr:     stateDbPtr,
		stateDBViewPtr: stateDBViewPtr,
		Accounts:       make(map[string]IAccount),
		Logs:           newEvmLogs(),

		Preimages:        make(map[types.Hash][]byte),
		transientStorage: newTransientStorage(),
		changer:          NewChanger(),

		AccessList: NewAccessList(),
		codeCache:  codeCache,
	}
}

func (r *RustStateLedger) GetOrCreateAccount(address *types.Address) IAccount {
	addr := address.String()

	value, ok := r.Accounts[addr]
	if ok {
		return value
	}
	account := NewRustAccount(r.stateDBPtr, address.ETHAddress(), r.changer, r.codeCache)
	r.Accounts[addr] = account
	return account
}

func (r *RustStateLedger) GetAccount(address *types.Address) IAccount {
	addr := address.String()

	value, ok := r.Accounts[addr]
	if ok {
		return value
	}
	cAddress := convertToCAddress(address.ETHAddress())
	exist := bool(C.exist(r.stateDBPtr, cAddress))
	if exist {
		account := NewRustAccount(r.stateDBPtr, address.ETHAddress(), r.changer, r.codeCache)
		r.Accounts[addr] = account
		return account
	} else {
		return nil
	}
}

func (r *RustStateLedger) GetBalance(address *types.Address) *big.Int {
	account := r.GetOrCreateAccount(address)
	return account.GetBalance()
}

func (r *RustStateLedger) SetBalance(address *types.Address, balance *big.Int) {
	account := r.GetOrCreateAccount(address)
	account.SetBalance(balance)
}

func (r *RustStateLedger) SubBalance(address *types.Address, balance *big.Int) {
	account := r.GetOrCreateAccount(address)
	account.SubBalance(balance)
}

func (r *RustStateLedger) AddBalance(address *types.Address, balance *big.Int) {
	account := r.GetOrCreateAccount(address)
	account.AddBalance(balance)
}

func (r *RustStateLedger) GetState(address *types.Address, bytes []byte) (bool, []byte) {
	account := r.GetOrCreateAccount(address)
	return account.GetState(bytes)
}

func (r *RustStateLedger) GetBit256State(address *types.Address, bytes []byte) ethcommon.Hash {
	account := r.GetOrCreateAccount(address)
	return account.(*RustAccount).GetBit256State(bytes)
}

func (r *RustStateLedger) SetState(address *types.Address, key []byte, value []byte) {
	account := r.GetOrCreateAccount(address)
	account.SetState(key, value)
}

func (r *RustStateLedger) SetBit256State(address *types.Address, key []byte, value ethcommon.Hash) {
	account := r.GetOrCreateAccount(address)
	account.(*RustAccount).SetBit256State(key, value)
}

func (r *RustStateLedger) SetCode(address *types.Address, bytes []byte) {
	account := r.GetOrCreateAccount(address)
	account.SetCodeAndHash(bytes)
}

func (r *RustStateLedger) GetCode(address *types.Address) []byte {
	account := r.GetOrCreateAccount(address)
	return account.Code()
}

func (r *RustStateLedger) SetNonce(address *types.Address, u uint64) {
	account := r.GetOrCreateAccount(address)
	account.SetNonce(u)
}

func (r *RustStateLedger) GetNonce(address *types.Address) uint64 {
	account := r.GetOrCreateAccount(address)
	return account.GetNonce()
}

func (r *RustStateLedger) GetCodeHash(address *types.Address) *types.Hash {
	account := r.GetOrCreateAccount(address)
	return types.NewHash(account.CodeHash())
}

func (r *RustStateLedger) GetCodeSize(address *types.Address) int {
	account := r.GetOrCreateAccount(address)
	return len(account.Code())
}

func (r *RustStateLedger) AddRefund(gas uint64) {
	C.add_refund(r.stateDBPtr, C.uint64_t(gas))
}

func (r *RustStateLedger) SubRefund(gas uint64) {
	C.sub_refund(r.stateDBPtr, C.uint64_t(gas))
}

func (r *RustStateLedger) GetRefund() uint64 {
	return r.Refund
}

func (r *RustStateLedger) GetCommittedState(address *types.Address, bytes []byte) []byte {
	account := r.GetOrCreateAccount(address)
	return account.GetCommittedState(bytes)
}

func (r *RustStateLedger) GetBit256CommittedState(address *types.Address, bytes []byte) ethcommon.Hash {
	account := r.GetOrCreateAccount(address)
	return account.(*RustAccount).GetBit256CommittedState(bytes)
}

func (r *RustStateLedger) Commit() (*types.Hash, error) {
	committedRes := C.commit(r.stateDBPtr)
	rootHash := *(*[32]byte)(unsafe.Pointer(&committedRes.root_hash.bytes))
	committed_height := committedRes.committed_version
	if r.blockHeight != uint64(committed_height) {
		panic("committed height not match")
	}
	r.Accounts = make(map[string]IAccount)
	return types.NewHash(rootHash[:]), nil
}

func (r *RustStateLedger) SelfDestruct(address *types.Address) bool {
	account := r.GetOrCreateAccount(address)
	account.SetSelfDestructed(true)
	account.SetBalance(new(big.Int))
	return true
}

func (r *RustStateLedger) HasSelfDestructed(address *types.Address) bool {
	account := r.GetAccount(address)
	if account != nil {
		return account.SelfDestructed()
	}
	return false
}

func (r *RustStateLedger) Selfdestruct6780(address *types.Address) {
	account := r.GetAccount(address)
	if account == nil {
		return
	}

	if account.IsCreated() {
		r.SelfDestruct(address)
	}
}

func (r *RustStateLedger) Exist(address *types.Address) bool {
	exist := !r.GetOrCreateAccount(address).IsEmpty()
	return exist
}

func (r *RustStateLedger) Empty(address *types.Address) bool {
	empty := r.GetOrCreateAccount(address).IsEmpty()
	return empty
}

func (r *RustStateLedger) AddressInAccessList(addr types.Address) bool {
	return r.AccessList.ContainsAddress(addr)
}

func (r *RustStateLedger) SlotInAccessList(addr types.Address, slot types.Hash) (bool, bool) {
	return r.AccessList.Contains(addr, slot)
}

func (r *RustStateLedger) AddAddressToAccessList(addr types.Address) {
	if r.AccessList.AddAddress(addr) {
		r.changer.Append(RustAccessListAddAccountChange{address: &addr})
	}
}

func (r *RustStateLedger) AddSlotToAccessList(addr types.Address, slot types.Hash) {
	addrMod, slotMod := r.AccessList.AddSlot(addr, slot)
	if addrMod {
		r.changer.Append(RustAccessListAddAccountChange{address: &addr})
	}
	if slotMod {
		r.changer.Append(RustAccessListAddSlotChange{
			address: &addr,
			slot:    &slot,
		})
	}
}

func (r *RustStateLedger) Prepare(rules params.Rules, sender, coinbase ethcommon.Address, dst *ethcommon.Address, precompiles []ethcommon.Address, list etherTypes.AccessList) {
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

func (r *RustStateLedger) AddPreimage(address types.Hash, preimage []byte) {
	preimagePtr := (*C.uchar)(unsafe.Pointer(&preimage[0]))
	preimageLen := C.uintptr_t(len(preimage))
	C.add_preimage(r.stateDBPtr, convertToCH256(address.ETHHash()), preimagePtr, preimageLen)
}

func (r *RustStateLedger) SetTxContext(thash *types.Hash, ti int) {
	r.thash = thash
	r.txIndex = ti

}

func (r *RustStateLedger) Clear() {
	r.Accounts = make(map[string]IAccount)
}

func (r *RustStateLedger) RevertToSnapshot(snapshot int) {
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

func (r *RustStateLedger) Snapshot() int {
	id := r.nextRevisionId
	r.nextRevisionId++
	r.validRevisions = append(r.validRevisions, revision{id: id, changerIndex: r.changer.length()})
	C.snapshot(r.stateDBPtr)
	return id
}

func (r *RustStateLedger) setTransientState(addr types.Address, key, value ethcommon.Hash) {
	C.set_transient_state(r.stateDBPtr, convertToCAddress(addr.ETHAddress()), convertToCH256(key), convertToCH256(value))
}

func (r *RustStateLedger) AddLog(log *types.EvmLog) {
	if log.TransactionHash == nil {
		log.TransactionHash = r.thash
	}

	log.TransactionIndex = uint64(r.txIndex)
	r.changer.Append(RustAddLogChange{txHash: log.TransactionHash})
	log.LogIndex = uint64(r.Logs.LogSize)
	if _, ok := r.Logs.Logs[*log.TransactionHash]; !ok {
		r.Logs.Logs[*log.TransactionHash] = make([]*types.EvmLog, 0)
	}

	r.Logs.Logs[*log.TransactionHash] = append(r.Logs.Logs[*log.TransactionHash], log)
	r.Logs.LogSize++
}

func (r *RustStateLedger) GetLogs(txHash types.Hash, height uint64) []*types.EvmLog {
	logs := r.Logs.Logs[txHash]
	for _, l := range logs {
		l.BlockNumber = height
	}
	return logs
}

func (r *RustStateLedger) RollbackState(height uint64, lastStateRoot *types.Hash) error {
	// TODO implement me
	//panic("implement me")
	return nil
}

func (r *RustStateLedger) PrepareBlock(lastStateRoot *types.Hash, currentExecutingHeight uint64) {
	r.Logs = newEvmLogs()
	// TODO lastStateRoot relate logic
	r.blockHeight = currentExecutingHeight
}

func (r *RustStateLedger) PrepareTranct() {
	C.prepare_transact(r.stateDBPtr)
}

func (r *RustStateLedger) FinalizeTransact() {
	C.finalize_transact(r.stateDBPtr)
}

func (r *RustStateLedger) RollbackTransact() {
	C.rollback_transact(r.stateDBPtr)
}

func (r *RustStateLedger) ClearChangerAndRefund() {
	r.Refund = 0
	r.changer.Reset()
	r.validRevisions = r.validRevisions[:0]
	r.nextRevisionId = 0
}

func (r *RustStateLedger) Close() {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) Finalise() {
	r.ClearChangerAndRefund()
}

func (r *RustStateLedger) Version() uint64 {
	return r.blockHeight
}

func (r *RustStateLedger) NewView(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	return &RustStateLedger{
		repo:           r.repo,
		stateDBPtr:     r.stateDBViewPtr,
		stateDBViewPtr: r.stateDBViewPtr,
		Accounts:       make(map[string]IAccount),
		Logs:           newEvmLogs(),
		blockHeight:    blockHeader.Number,
	}, nil
}

func (r *RustStateLedger) IterateTrie(snapshotMeta *SnapshotMeta, kv kv.Storage, errC chan error) {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) GetTrieSnapshotMeta() (*SnapshotMeta, error) {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) VerifyTrie(blockHeader *types.BlockHeader) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) Prove(rootHash ethcommon.Hash, key []byte) (*jmt.ProofResult, error) {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) GenerateSnapshot(blockHeader *types.BlockHeader, errC chan error) {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) GetHistoryRange() (uint64, uint64) {
	//TODO implement me
	panic("implement me")
}

func (r *RustStateLedger) CurrentBlockHeight() uint64 {
	return r.blockHeight
}

func (r *RustStateLedger) GetStateDelta(blockNumber uint64) *types.StateDelta {
	//TODO implement me
	panic("implement me")
}

var _ StateLedger = (*RustStateLedger)(nil)
