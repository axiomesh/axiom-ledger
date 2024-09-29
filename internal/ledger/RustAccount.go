package ledger

/*
#cgo LDFLAGS: /Users/koi/Documents/dev/project/forestore/target/release/libforestore.a  -ldl -lm
#include "/Users/koi/Documents/dev/project/forestore/src/c_ffi/forestore.h"
*/
import "C"
import (
	"crypto/sha256"
	"fmt"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/holiman/uint256"
	"math/big"
	"runtime"
	"unsafe"
)

type RustAccount struct {
	stateDBPtr *C.struct_EvmStateDB
	cAddress   C.CAddress
	ethAddress common.Address
	Addr       *types.Address
	created    bool
	changer    *RustStateChanger
	codeCache  *lru.Cache[common.Address, []byte]
}

func NewRustAccount(stateDBPtr *C.struct_EvmStateDB, address common.Address, changer *RustStateChanger, codeCache *lru.Cache[common.Address, []byte]) *RustAccount {
	return &RustAccount{
		stateDBPtr: stateDBPtr,
		ethAddress: address,
		Addr:       types.NewAddress(address.Bytes()),
		cAddress:   convertToCAddress(address),
		changer:    changer,
		codeCache:  codeCache,
	}
}

func cu256ToUInt256(cu256 C.CU256) *uint256.Int {
	return &uint256.Int{uint64(cu256.low1), uint64(cu256.low2), uint64(cu256.high1), uint64(cu256.high2)}
}

func convertToCU256(u *uint256.Int) C.CU256 {
	return C.CU256{low1: C.uint64_t(u[0]), low2: C.uint64_t(u[1]), high1: C.uint64_t(u[2]), high2: C.uint64_t(u[3])}
}

func convertToCAddress(address common.Address) C.CAddress {
	cAddress := C.CAddress{}
	addrArray := (*[20]C.uint8_t)(unsafe.Pointer(&cAddress.address[0]))
	copy((*[20]uint8)(unsafe.Pointer(addrArray))[:], address[:])
	return cAddress
}

func convertToCH256(hash common.Hash) C.CH256 {
	ch256 := C.CH256{}
	bytesArray := (*[32]C.uint8_t)(unsafe.Pointer(&ch256.bytes[0]))
	copy((*[32]uint8)(unsafe.Pointer(bytesArray))[:], hash[:])
	return ch256
}

func hashKey(key []byte) common.Hash {
	keyHash := sha256.Sum256(key)
	return keyHash
}

func (o *RustAccount) String() string {
	return fmt.Sprintf("{account: %v, code length: %v}", o.ethAddress, len(o.Code()))
}

func (o *RustAccount) initStorageTrie() {
}

func (o *RustAccount) GetAddress() *types.Address {
	return o.Addr
}

// GetState Get state from local cache, if not found, then get it from DB
func (o *RustAccount) GetState(k []byte) (bool, []byte) {
	key := hashKey(k)
	var length C.uintptr_t
	ptr := C.get_any_size_state(o.stateDBPtr, o.cAddress, convertToCH256(key), &length)
	defer C.deallocate_memory(ptr, length)
	valueLen := int(length)
	goSlice := C.GoBytes(unsafe.Pointer(ptr), C.int(length))
	return valueLen != 0, goSlice
}

func (o *RustAccount) GetBit256State(key []byte) common.Hash {
	hash_key := hashKey(key)
	cH256 := C.get_state(o.stateDBPtr, o.cAddress, convertToCH256(hash_key))
	goHash := *(*common.Hash)(unsafe.Pointer(&cH256.bytes))
	return goHash
}

func (o *RustAccount) GetCommittedState(k []byte) []byte {
	key := hashKey(k)
	var length C.uintptr_t
	ptr := C.get_any_size_committed_state(o.stateDBPtr, o.cAddress, convertToCH256(key), &length)
	defer C.deallocate_memory(ptr, length)
	goSlice := C.GoBytes(unsafe.Pointer(ptr), C.int(length))
	return goSlice
}

func (o *RustAccount) GetBit256CommittedState(k []byte) common.Hash {
	key := hashKey(k)
	cH256 := C.get_committed_state(o.stateDBPtr, o.cAddress, convertToCH256(key))
	goHash := *(*common.Hash)(unsafe.Pointer(&cH256.bytes))
	return goHash
}

// SetState Set account state
func (o *RustAccount) SetState(k []byte, value []byte) {
	key := hashKey(k)
	valuePtr := (*C.uchar)(unsafe.Pointer(&value[0]))
	valueLen := C.uintptr_t(len(value))
	C.set_any_size_state(o.stateDBPtr, o.cAddress, convertToCH256(key), valuePtr, valueLen)
	runtime.KeepAlive(value)
}

func (o *RustAccount) SetBit256State(k []byte, value common.Hash) {
	key := hashKey(k)
	C.set_state(o.stateDBPtr, o.cAddress, convertToCH256(key), convertToCH256(value))
}

// SetCodeAndHash Set the contract code and hash
func (o *RustAccount) SetCodeAndHash(code []byte) {
	o.codeCache.Add(o.ethAddress, code)
	codePtr := (*C.uchar)(unsafe.Pointer(&code[0]))
	codeLen := C.uintptr_t(len(code))
	C.set_code(o.stateDBPtr, o.cAddress, codePtr, codeLen)
}

// Code return the contract code
func (o *RustAccount) Code() []byte {
	if v, ok := o.codeCache.Get(o.ethAddress); ok {
		return v
	}
	var length C.uintptr_t
	ptr := C.get_code(o.stateDBPtr, o.cAddress, &length)
	defer C.deallocate_memory(ptr, length)
	goSlice := C.GoBytes(unsafe.Pointer(ptr), C.int(length))

	o.codeCache.Add(o.ethAddress, goSlice)
	return goSlice
}

func (o *RustAccount) CodeHash() []byte {
	c_hash := C.get_code_hash(o.stateDBPtr, o.cAddress)
	goHash := *(*common.Hash)(unsafe.Pointer(&c_hash.bytes))
	return goHash[:]
}

// SetNonce Set the nonce which indicates the contract number
func (o *RustAccount) SetNonce(nonce uint64) {
	C.set_nonce(o.stateDBPtr, o.cAddress, convertToCU256(uint256.NewInt(nonce)))
}

// GetNonce Get the nonce from user account
func (o *RustAccount) GetNonce() uint64 {
	nonce := C.get_nonce(o.stateDBPtr, o.cAddress)
	return uint64(nonce.low1)
}

// GetBalance Get the balance from the account
func (o *RustAccount) GetBalance() *big.Int {
	balance := C.get_balance(o.stateDBPtr, o.cAddress)
	return cu256ToUInt256(balance).ToBig()
}

// SetBalance Set the balance to the account
func (o *RustAccount) SetBalance(balance *big.Int) {
	u, _ := uint256.FromBig(balance)
	C.set_balance(o.stateDBPtr, o.cAddress, convertToCU256(u))

}

func (o *RustAccount) SubBalance(amount *big.Int) {
	u, _ := uint256.FromBig(amount)
	C.sub_balance(o.stateDBPtr, o.cAddress, convertToCU256(u))

}

func (o *RustAccount) AddBalance(amount *big.Int) {
	u, _ := uint256.FromBig(amount)
	C.add_balance(o.stateDBPtr, o.cAddress, convertToCU256(u))

}

// Finalise moves all dirty states into the pending states.
// Return all dirty state keys
func (o *RustAccount) Finalise() [][]byte {
	return nil
}

func (o *RustAccount) SetSelfDestructed(selfDestructed bool) {
	C.self_destruct(o.stateDBPtr, o.cAddress)
}

func (o *RustAccount) IsEmpty() bool {
	return o.GetBalance().Sign() == 0 && o.GetNonce() == 0 && o.Code() == nil && !o.SelfDestructed()
}

func (o *RustAccount) SelfDestructed() bool {
	return bool(C.has_self_destructed(o.stateDBPtr, o.cAddress))
}

func (o *RustAccount) GetStorageRoot() common.Hash {
	panic("not support")
}

func (o *RustAccount) IsCreated() bool {
	return o.created
}

func (o *RustAccount) SetCreated(created bool) {
	o.created = created
}
