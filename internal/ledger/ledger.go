package ledger

import (
	"fmt"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
)

// ChainLedger handles block, transaction and receipt data.
//
//go:generate mockgen -destination mock_ledger/mock_ledger.go -package mock_ledger -source ledger.go -typed
type ChainLedger interface {
	// GetBlock get block with height
	GetBlock(height uint64) (*types.Block, error)

	GetBlockNumberByHash(hash *types.Hash) (uint64, error)

	GetBlockHeader(height uint64) (*types.BlockHeader, error)

	GetBlockExtra(height uint64) (*types.BlockExtra, error)

	// GetBlockTxHashList get the block tx hash list using block height
	GetBlockTxHashList(height uint64) ([]*types.Hash, error)

	// GetBlockTxList get the block tx hash list using block height
	GetBlockTxList(height uint64) ([]*types.Transaction, error)

	// GetBlockReceipts get the transactions receipts in a block
	GetBlockReceipts(height uint64) ([]*types.Receipt, error)

	// GetTransactionCount get the transaction count in a block
	GetTransactionCount(height uint64) (uint64, error)

	// GetTransaction get the transaction using transaction hash
	GetTransaction(hash *types.Hash) (*types.Transaction, error)

	// GetTransactionMeta get the transaction meta data
	GetTransactionMeta(hash *types.Hash) (*types.TransactionMeta, error)

	// GetReceipt get the transaction receipt
	GetReceipt(hash *types.Hash) (*types.Receipt, error)

	// PersistExecutionResult persist the execution result
	PersistExecutionResult(block *types.Block, receipts []*types.Receipt) error

	BatchPersistExecutionResult(batchBlock []*types.Block, BatchReceipts [][]*types.Receipt) error

	// GetChainMeta get chain meta data
	GetChainMeta() *types.ChainMeta

	// UpdateChainMeta update the chain meta data
	UpdateChainMeta(*types.ChainMeta)

	// LoadChainMeta get chain meta data
	LoadChainMeta() (*types.ChainMeta, error)

	RollbackBlockChain(height uint64) error

	Close()
}

type StateLedger interface {
	StateAccessor

	AddLog(log *types.EvmLog)

	GetLogs(txHash types.Hash, height uint64) []*types.EvmLog

	RollbackState(height uint64, lastStateRoot *types.Hash) error

	PrepareBlock(lastStateRoot *types.Hash, currentExecutingHeight uint64)

	ClearChangerAndRefund()

	// Close release resource
	Close()

	Finalise()

	Version() uint64

	// NewView get a view at specific block. We can enable snapshot if and only if the block were the latest block.
	NewView(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error)

	IterateTrie(snapshotMeta *utils.SnapshotMeta, kv kv.Storage, errC chan error)

	GetTrieSnapshotMeta() (*utils.SnapshotMeta, error)

	VerifyTrie(blockHeader *types.BlockHeader) (bool, error)

	Prove(rootHash common.Hash, key []byte) (*jmt.ProofResult, error)

	GenerateSnapshot(blockHeader *types.BlockHeader, errC chan error)

	GetHistoryRange() (uint64, uint64)

	GetStateDelta(blockNumber uint64) *types.StateJournal

	UpdateChainState(chainState *chainstate.ChainState)

	Archive(blockHeader *types.BlockHeader, stateJournal *types.StateJournal) error
}

// StateAccessor manipulates the state data
type StateAccessor interface {
	// GetOrCreateAccount
	GetOrCreateAccount(*types.Address) IAccount

	// GetAccount
	GetAccount(*types.Address) IAccount

	// GetBalance
	GetBalance(*types.Address) *big.Int

	// SetBalance
	SetBalance(*types.Address, *big.Int)

	// SubBalance
	SubBalance(*types.Address, *big.Int)

	// AddBalance
	AddBalance(*types.Address, *big.Int)

	// GetState
	GetState(*types.Address, []byte) (bool, []byte)

	// SetState
	SetState(*types.Address, []byte, []byte)

	// SetCode
	SetCode(*types.Address, []byte)

	// GetCode
	GetCode(*types.Address) []byte

	// SetNonce
	SetNonce(*types.Address, uint64)

	// GetNonce
	GetNonce(*types.Address) uint64

	// GetCodeHash
	GetCodeHash(*types.Address) *types.Hash

	// GetCodeSize
	GetCodeSize(*types.Address) int

	// AddRefund
	AddRefund(uint64)

	// SubRefund
	SubRefund(uint64)

	// GetRefund
	GetRefund() uint64

	// GetCommittedState
	GetCommittedState(*types.Address, []byte) []byte

	// Commit commits the state data
	Commit() (*types.StateJournal, error)

	// SelfDestruct
	SelfDestruct(*types.Address) bool

	// HasSelfDestructed
	HasSelfDestructed(*types.Address) bool

	// Selfdestruct6780
	Selfdestruct6780(*types.Address)

	// Exist
	Exist(*types.Address) bool

	// Empty
	Empty(*types.Address) bool

	// AddressInAccessList
	AddressInAccessList(types.Address) bool

	// SlotInAccessList
	SlotInAccessList(types.Address, types.Hash) (bool, bool)

	// AddAddressToAccessList
	AddAddressToAccessList(types.Address)

	// AddSlotToAccessList
	AddSlotToAccessList(types.Address, types.Hash)

	// AddPreimage
	AddPreimage(types.Hash, []byte)

	// Set tx context for state db
	SetTxContext(thash *types.Hash, txIndex int)

	// Clear
	Clear()

	// RevertToSnapshot
	RevertToSnapshot(int)

	// Snapshot
	Snapshot() int
}

type IAccount interface {
	fmt.Stringer

	GetAddress() *types.Address

	GetState(key []byte) (bool, []byte)

	GetCommittedState(key []byte) []byte

	SetState(key []byte, value []byte)

	SetCodeAndHash(code []byte)

	Code() []byte

	CodeHash() []byte

	SetNonce(nonce uint64)

	GetNonce() uint64

	GetBalance() *big.Int

	SetBalance(balance *big.Int)

	SubBalance(amount *big.Int)

	AddBalance(amount *big.Int)

	Finalise() [][]byte

	IsEmpty() bool

	SelfDestructed() bool

	SetSelfDestructed(bool)

	GetStorageRoot() common.Hash

	SetCreated(bool)

	IsCreated() bool
}

var _ vm.StateDB = (*EvmStateDBAdaptor)(nil)

// EvmStateDBAdaptor wraps StateLedger with Wrapper mode
type EvmStateDBAdaptor struct {
	StateLedger StateLedger
}
