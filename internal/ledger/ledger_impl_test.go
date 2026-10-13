package ledger

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethhexutil "github.com/ethereum/go-ethereum/common/hexutil"
	etherTypes "github.com/ethereum/go-ethereum/core/types"
	crypto1 "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/storage"
	"github.com/axiomesh/axiom-kit/storage/blockfile"
	"github.com/axiomesh/axiom-kit/storage/leveldb"
	"github.com/axiomesh/axiom-kit/storage/pebble"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/snapshot"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

func TestNew001(t *testing.T) {
	repoRoot := t.TempDir()

	lBlockStorage, err := leveldb.New(filepath.Join(repoRoot, "lStorage"), nil)
	assert.Nil(t, err)
	lStateStorage, err := leveldb.New(filepath.Join(repoRoot, "lLedger"), nil)
	assert.Nil(t, err)
	lSnapshotStorage, err := leveldb.New(filepath.Join(repoRoot, "lSnapshot"), nil)
	assert.Nil(t, err)
	pBlockStorage, err := pebble.New(filepath.Join(repoRoot, "pStorage"), nil, nil, logrus.New())
	assert.Nil(t, err)
	pStateStorage, err := pebble.New(filepath.Join(repoRoot, "pLedger"), nil, nil, logrus.New())
	assert.Nil(t, err)
	pSnapshotStorage, err := pebble.New(filepath.Join(repoRoot, "pSnapshot"), nil, nil, logrus.New())
	assert.Nil(t, err)

	testcase := map[string]struct {
		blockStorage    storage.Storage
		stateStorage    storage.Storage
		snapshotStorage storage.Storage
	}{
		"leveldb": {blockStorage: lBlockStorage, stateStorage: lStateStorage, snapshotStorage: lSnapshotStorage},
		"pebble":  {blockStorage: pBlockStorage, stateStorage: pStateStorage, snapshotStorage: pSnapshotStorage},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			logger := log.NewWithModule("account_test")
			addr := types.NewAddress(LeftPadBytes([]byte{100}, 20))
			blockFile, err := blockfile.NewBlockFile(filepath.Join(repoRoot, name), logger)
			assert.Nil(t, err)
			l, err := NewLedgerWithStores(createMockRepo(t), tc.blockStorage, tc.stateStorage, tc.snapshotStorage, blockFile)
			require.Nil(t, err)
			require.NotNil(t, l)
			sl := l.StateLedger.(*StateLedgerImpl)

			sl.blockHeight = 1
			sl.SetNonce(addr, 1)
			rootHash1, err := sl.Commit()
			require.Nil(t, err)

			sl.blockHeight = 2
			rootHash2, err := sl.Commit()
			require.Nil(t, err)

			sl.blockHeight = 3
			sl.SetNonce(addr, 3)
			rootHash3, err := sl.Commit()
			require.Nil(t, err)

			assert.Equal(t, rootHash1, rootHash2)
			assert.NotEqual(t, rootHash1, rootHash3)

			l.Close()
		})
	}
}

func TestNew002(t *testing.T) {
	repoRoot := t.TempDir()

	lBlockStorage, err := leveldb.New(filepath.Join(repoRoot, "lStorage"), nil)
	assert.Nil(t, err)
	lStateStorage, err := leveldb.New(filepath.Join(repoRoot, "lLedger"), nil)
	assert.Nil(t, err)
	lSnapshotStorage, err := leveldb.New(filepath.Join(repoRoot, "lSnapshot"), nil)
	assert.Nil(t, err)
	pBlockStorage, err := pebble.New(filepath.Join(repoRoot, "pStorage"), nil, nil, logrus.New())
	assert.Nil(t, err)
	pStateStorage, err := pebble.New(filepath.Join(repoRoot, "pLedger"), nil, nil, logrus.New())
	assert.Nil(t, err)
	pSnapshotStorage, err := pebble.New(filepath.Join(repoRoot, "pSnapshot"), nil, nil, logrus.New())
	assert.Nil(t, err)

	testcase := map[string]struct {
		blockStorage    storage.Storage
		stateStorage    storage.Storage
		snapshotStorage storage.Storage
	}{
		"leveldb": {blockStorage: lBlockStorage, stateStorage: lStateStorage, snapshotStorage: lSnapshotStorage},
		"pebble":  {blockStorage: pBlockStorage, stateStorage: pStateStorage, snapshotStorage: pSnapshotStorage},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			tc.blockStorage.Put([]byte(utils.ChainMetaKey), []byte{1})
			logger := log.NewWithModule("account_test")
			blockFile, err := blockfile.NewBlockFile(filepath.Join(repoRoot, name), logger)
			assert.Nil(t, err)
			l, err := NewLedgerWithStores(createMockRepo(t), tc.blockStorage, tc.stateStorage, tc.snapshotStorage, blockFile)
			require.NotNil(t, err)
			require.Nil(t, l)
		})
	}
}

func TestNew003(t *testing.T) {
	repoRoot := t.TempDir()

	lBlockStorage, err := leveldb.New(filepath.Join(repoRoot, "lStorage"), nil)
	assert.Nil(t, err)
	lStateStorage, err := leveldb.New(filepath.Join(repoRoot, "lLedger"), nil)
	assert.Nil(t, err)
	lSnapshotStorage, err := leveldb.New(filepath.Join(repoRoot, "lSnapshot"), nil)
	assert.Nil(t, err)
	pBlockStorage, err := pebble.New(filepath.Join(repoRoot, "pStorage"), nil, nil, logrus.New())
	assert.Nil(t, err)
	pStateStorage, err := pebble.New(filepath.Join(repoRoot, "pLedger"), nil, nil, logrus.New())
	assert.Nil(t, err)
	pSnapshotStorage, err := pebble.New(filepath.Join(repoRoot, "pSnapshot"), nil, nil, logrus.New())
	assert.Nil(t, err)

	testcase := map[string]struct {
		blockStorage    storage.Storage
		stateStorage    storage.Storage
		snapshotStorage storage.Storage
	}{
		"leveldb": {blockStorage: lBlockStorage, stateStorage: lStateStorage, snapshotStorage: lSnapshotStorage},
		"pebble":  {blockStorage: pBlockStorage, stateStorage: pStateStorage, snapshotStorage: pSnapshotStorage},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			kvdb := tc.stateStorage

			logger := log.NewWithModule("account_test")
			blockFile, err := blockfile.NewBlockFile(filepath.Join(repoRoot, name), logger)
			assert.Nil(t, err)

			l, err := NewLedgerWithStores(createMockRepo(t), tc.blockStorage, kvdb, tc.snapshotStorage, blockFile)
			require.Nil(t, err)
			require.NotNil(t, l)

			l, err = NewLedgerWithStores(createMockRepo(t), tc.blockStorage, kvdb, nil, blockFile)
			require.Nil(t, err)
			require.NotNil(t, l)

			rep := createMockRepo(t)
			rep.Config.Ledger.StateLedgerAccountCacheSize = -1

			l, err = NewLedgerWithStores(rep, tc.blockStorage, kvdb, tc.snapshotStorage, blockFile)
			require.NotNil(t, err)
			require.Nil(t, l)

			l, err = NewLedgerWithStores(rep, tc.blockStorage, kvdb, nil, blockFile)
			require.NotNil(t, err)
			require.Nil(t, l)

			rep.Config.Ledger.ChainLedgerCacheSize = -1
		})
	}
}

func TestChainLedger_PersistBlockData(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			ledger.StateLedger.(*StateLedgerImpl).blockHeight = 1

			// create an account
			account := types.NewAddress(LeftPadBytes([]byte{100}, 20))

			ledger.StateLedger.SetState(account, []byte("a"), []byte("b"))
			stateRoot, err := ledger.StateLedger.Commit()
			assert.Nil(t, err)
			ledger.PersistBlockData(genBlockData(1, stateRoot))
		})
	}
}

func TestChainLedger_Commit(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			lg, repoRoot := initLedger(t, "", tc.kvType)
			sl := lg.StateLedger.(*StateLedgerImpl)

			// create an account
			account := types.NewAddress(LeftPadBytes([]byte{100}, 20))

			sl.blockHeight = 1
			sl.SetState(account, []byte("a"), []byte("b"))
			sl.Finalise()
			stateRoot1, err := sl.Commit()
			assert.NotNil(t, stateRoot1)
			assert.Nil(t, err)
			sl.GetCommittedState(account, []byte("a"))
			isSuicide := sl.HasSuicide(account)
			assert.Equal(t, isSuicide, false)
			assert.Equal(t, uint64(1), sl.Version())

			sl.blockHeight = 2
			sl.Finalise()
			stateRoot2, err := sl.Commit()
			assert.Nil(t, err)
			assert.Equal(t, uint64(2), sl.Version())
			assert.Equal(t, stateRoot1, stateRoot2)

			sl.SetState(account, []byte("a"), []byte("3"))
			sl.SetState(account, []byte("a"), []byte("2"))
			sl.blockHeight = 3
			sl.Finalise()
			stateRoot3, err := sl.Commit()
			assert.Nil(t, err)
			assert.Equal(t, uint64(3), lg.StateLedger.Version())
			assert.NotEqual(t, stateRoot1, stateRoot3)

			lg.StateLedger.SetBalance(account, new(big.Int).SetInt64(100))
			sl.blockHeight = 4
			stateRoot4, err := sl.Commit()
			assert.Nil(t, err)
			assert.Equal(t, uint64(4), lg.StateLedger.Version())
			assert.NotEqual(t, stateRoot3, stateRoot4)

			code := RightPadBytes([]byte{100}, 100)
			lg.StateLedger.SetCode(account, code)
			lg.StateLedger.SetState(account, []byte("b"), []byte("3"))
			lg.StateLedger.SetState(account, []byte("c"), []byte("2"))
			sl.blockHeight = 5
			sl.Finalise()
			stateRoot5, err := sl.Commit()
			assert.Nil(t, err)
			assert.Equal(t, uint64(5), lg.StateLedger.Version())
			assert.NotEqual(t, stateRoot4, stateRoot5)

			minHeight, maxHeight := sl.snapshot.GetJournalRange()
			journal5 := sl.snapshot.GetBlockJournal(maxHeight)
			assert.Equal(t, uint64(1), minHeight)
			assert.Equal(t, uint64(5), maxHeight)
			assert.Equal(t, 1, len(journal5.Journals))
			entry := journal5.Journals[0]
			assert.Equal(t, account.String(), entry.Address.String())
			assert.True(t, entry.AccountChanged)
			assert.Equal(t, uint64(100), entry.PrevAccount.Balance.Uint64())
			assert.Equal(t, uint64(0), entry.PrevAccount.Nonce)
			assert.Nil(t, entry.PrevAccount.CodeHash)
			assert.Equal(t, 2, len(entry.PrevStates))
			assert.Nil(t, entry.PrevStates[hex.EncodeToString([]byte("b"))])
			assert.Nil(t, entry.PrevStates[hex.EncodeToString([]byte("c"))])
			isExist := sl.Exist(account)
			assert.True(t, isExist)
			isEmpty := sl.Empty(account)
			assert.False(t, isEmpty)
			err = sl.snapshot.RemoveJournalsBeforeBlock(10)
			assert.NotNil(t, err)
			err = sl.snapshot.RemoveJournalsBeforeBlock(0)
			assert.Nil(t, err)

			// Extra Test
			hash := types.NewHashByStr("0xe9FC370DD36C9BD5f67cCfbc031C909F53A3d8bC7084C01362c55f2D42bA841c")
			revid := lg.StateLedger.(*StateLedgerImpl).Snapshot()
			lg.StateLedger.(*StateLedgerImpl).AddLog(&types.EvmLog{
				TransactionHash: hash,
			})
			lg.StateLedger.(*StateLedgerImpl).GetLogs(*hash, 1)
			lg.StateLedger.(*StateLedgerImpl).Logs()
			lg.StateLedger.(*StateLedgerImpl).GetCodeHash(account)
			lg.StateLedger.(*StateLedgerImpl).GetCodeSize(account)
			currentAccount := lg.StateLedger.(*StateLedgerImpl).GetAccount(account)
			lg.StateLedger.(*StateLedgerImpl).setAccount(currentAccount)
			lg.StateLedger.(*StateLedgerImpl).AddBalance(account, big.NewInt(1))
			lg.StateLedger.(*StateLedgerImpl).SubBalance(account, big.NewInt(1))
			lg.StateLedger.(*StateLedgerImpl).SetNonce(account, 1)
			lg.StateLedger.(*StateLedgerImpl).AddRefund(1)
			refund := lg.StateLedger.(*StateLedgerImpl).GetRefund()
			assert.Equal(t, refund, uint64(1))
			lg.StateLedger.(*StateLedgerImpl).SubRefund(1)
			refund = lg.StateLedger.(*StateLedgerImpl).GetRefund()
			assert.Equal(t, refund, uint64(0))
			lg.StateLedger.(*StateLedgerImpl).AddAddressToAccessList(*account)
			isInAddressList := lg.StateLedger.(*StateLedgerImpl).AddressInAccessList(*account)
			assert.Equal(t, isInAddressList, true)
			lg.StateLedger.(*StateLedgerImpl).AddSlotToAccessList(*account, *hash)
			isInSlotAddressList, _ := lg.StateLedger.(*StateLedgerImpl).SlotInAccessList(*account, *hash)
			assert.Equal(t, isInSlotAddressList, true)
			lg.StateLedger.(*StateLedgerImpl).AddPreimage(*hash, []byte("11"))
			lg.StateLedger.(*StateLedgerImpl).PrepareAccessList(*account, account, []types.Address{}, AccessTupleList{})
			lg.StateLedger.(*StateLedgerImpl).Suicide(account)
			lg.StateLedger.(*StateLedgerImpl).RevertToSnapshot(revid)
			lg.StateLedger.(*StateLedgerImpl).ClearChangerAndRefund()

			lg.ChainLedger.(*ChainLedgerImpl).CloseBlockfile()

			// load ChainLedgerImpl from db, rollback to height 0 since no chain meta stored
			ldg, _ := initLedger(t, repoRoot, tc.kvType)

			ok, _ := ldg.StateLedger.GetState(account, []byte("a"))
			assert.False(t, ok)

			ok, _ = ldg.StateLedger.GetState(account, []byte("b"))
			assert.False(t, ok)

			ok, _ = ldg.StateLedger.GetState(account, []byte("c"))
			assert.False(t, ok)

			assert.Equal(t, uint64(0), ldg.StateLedger.GetBalance(account).Uint64())
			assert.Equal(t, []byte(nil), ldg.StateLedger.GetCode(account))

			ver := ldg.StateLedger.Version()
			assert.Equal(t, uint64(0), ver)
			err = lg.StateLedger.(*StateLedgerImpl).snapshot.RemoveJournalsBeforeBlock(4)
			assert.NotNil(t, err)
		})
	}
}

func TestChainLedger_EVMAccessor(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)

			hash := common.HexToHash("0xe9FC370DD36C9BD5f67cCfbc031C909F53A3d8bC7084C01362c55f2D42bA841c")
			// create an account
			account := common.BytesToAddress(LeftPadBytes([]byte{100}, 20))

			evmStateDB := &EvmStateDBAdaptor{StateLedger: ledger.StateLedger}

			evmStateDB.CreateAccount(account)
			evmStateDB.AddBalance(account, big.NewInt(2))
			balance := evmStateDB.GetBalance(account)
			assert.Equal(t, balance, big.NewInt(2))
			evmStateDB.SubBalance(account, big.NewInt(1))
			balance = evmStateDB.GetBalance(account)
			assert.Equal(t, balance, big.NewInt(1))
			evmStateDB.SetNonce(account, 10)
			nonce := evmStateDB.GetNonce(account)
			assert.Equal(t, nonce, uint64(10))
			evmStateDB.GetCodeHash(account)
			evmStateDB.SetCode(account, []byte("1111"))
			code := evmStateDB.GetCode(account)
			assert.Equal(t, code, []byte("1111"))
			codeSize := evmStateDB.GetCodeSize(account)
			assert.Equal(t, codeSize, 4)
			evmStateDB.AddRefund(2)
			refund := evmStateDB.GetRefund()
			assert.Equal(t, refund, uint64(2))
			evmStateDB.SubRefund(1)
			refund = evmStateDB.GetRefund()
			assert.Equal(t, refund, uint64(1))
			evmStateDB.GetCommittedState(account, hash)
			evmStateDB.SetState(account, hash, hash)
			value := evmStateDB.GetState(account, hash)
			assert.Equal(t, value, hash)
			evmStateDB.Suicide(account)
			isSuicide := evmStateDB.HasSuicided(account)
			assert.Equal(t, isSuicide, true)
			isExist := evmStateDB.Exist(account)
			assert.Equal(t, isExist, true)
			isEmpty := evmStateDB.Empty(account)
			assert.Equal(t, isEmpty, false)
			evmStateDB.PrepareEVMAccessList(account, &account, []common.Address{}, etherTypes.AccessList{})
			evmStateDB.AddAddressToAccessList(account)
			isIn := evmStateDB.AddressInAccessList(account)
			assert.Equal(t, isIn, true)
			evmStateDB.AddSlotToAccessList(account, hash)
			isSlotIn, _ := evmStateDB.SlotInAccessList(account, hash)
			assert.Equal(t, isSlotIn, true)
			evmStateDB.AddPreimage(hash, []byte("1111"))
			// ledger.StateLedgerImpl.(*SimpleLedger).PrepareEVM(hash, 1)
			evmStateDB.StateDB()
			ledger.StateLedger.SetTxContext(types.NewHash(hash.Bytes()), 1)
			evmStateDB.AddLog(&etherTypes.Log{})

			addr := common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
			impl := evmStateDB.StateLedger.(*StateLedgerImpl)
			impl.transientStorage = newTransientStorage()
			evmStateDB.SetTransientState(addr, hash, hash)
			evmStateDB.GetTransientState(addr, hash)
			_ = evmStateDB.StateLedger.(*StateLedgerImpl).transientStorage.Copy()
			evmStateDB.Prepare(params.Rules{IsBerlin: true}, addr, addr, &addr, []common.Address{addr}, nil)
		})
	}
}

func TestChainLedger_Rollback(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, repoRoot := initLedger(t, "", tc.kvType)
			stateLedger := ledger.StateLedger.(*StateLedgerImpl)

			// create an addr0
			addr0 := types.NewAddress(LeftPadBytes([]byte{100}, 20))
			addr1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))

			ledger.StateLedger.PrepareBlock(nil, 1)
			ledger.StateLedger.SetBalance(addr0, new(big.Int).SetInt64(1))
			stateLedger.Finalise()
			stateRoot1, err := stateLedger.Commit()
			assert.Nil(t, err)
			assert.NotNil(t, stateRoot1)
			ledger.PersistBlockData(genBlockData(1, stateRoot1))

			ledger.StateLedger.PrepareBlock(stateRoot1, 2)
			ledger.StateLedger.SetBalance(addr0, new(big.Int).SetInt64(2))
			ledger.StateLedger.SetState(addr0, []byte("a"), []byte("2"))

			code := sha256.Sum256([]byte("code"))
			ret := crypto1.Keccak256Hash(code[:])
			codeHash := ret.Bytes()
			ledger.StateLedger.SetCode(addr0, code[:])
			ledger.StateLedger.Finalise()

			stateRoot2, err := stateLedger.Commit()
			assert.Nil(t, err)
			ledger.PersistBlockData(genBlockData(2, stateRoot2))

			ledger.StateLedger.PrepareBlock(stateRoot2, 3)
			account0 := ledger.StateLedger.GetAccount(addr0)
			assert.Equal(t, uint64(2), account0.GetBalance().Uint64())

			ledger.StateLedger.SetBalance(addr1, new(big.Int).SetInt64(3))
			ledger.StateLedger.SetBalance(addr0, new(big.Int).SetInt64(4))
			ledger.StateLedger.SetState(addr0, []byte("a"), []byte("3"))
			ledger.StateLedger.SetState(addr0, []byte("b"), []byte("4"))

			code1 := sha256.Sum256([]byte("code1"))
			ret1 := crypto1.Keccak256Hash(code1[:])
			codeHash1 := ret1.Bytes()
			ledger.StateLedger.SetCode(addr0, code1[:])
			ledger.StateLedger.Finalise()

			stateRoot3, err := stateLedger.Commit()
			assert.Nil(t, err)
			ledger.PersistBlockData(genBlockData(3, stateRoot3))

			block, err := ledger.ChainLedger.GetBlock(3)
			assert.Nil(t, err)
			assert.NotNil(t, block)
			assert.Equal(t, uint64(3), ledger.ChainLedger.GetChainMeta().Height)

			ledger.StateLedger.(*StateLedgerImpl).accountCache.codeCache.Remove(addr0.String())
			account0 = ledger.StateLedger.GetAccount(addr0)
			assert.Equal(t, uint64(4), account0.GetBalance().Uint64())
			assert.Equal(t, code1[:], account0.Code())

			err = ledger.Rollback(4)
			assert.True(t, errors.Is(err, ErrNotFound))

			_, err = ledger.ChainLedger.GetBlockHeader(3)
			assert.Nil(t, err)

			_, err = ledger.ChainLedger.GetBlockHeader(100)
			assert.NotNil(t, err)

			num, err := ledger.ChainLedger.GetTransactionCount(0)
			assert.NotNil(t, err)
			num, err = ledger.ChainLedger.GetTransactionCount(3)
			assert.Nil(t, err)
			assert.NotNil(t, num)

			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.BlockTxSetKey, 3), []byte("???"))
			num, err = ledger.ChainLedger.GetTransactionCount(3)
			assert.NotNil(t, err)
			assert.Contains(t, err.Error(), "unmarshal tx hash data error")
			assert.Equal(t, num, uint64(0))

			meta, err := ledger.ChainLedger.LoadChainMeta()
			assert.Nil(t, err)
			assert.NotNil(t, meta)
			assert.Equal(t, uint64(3), meta.Height)

			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put([]byte(utils.ChainMetaKey), []byte("nil"))
			meta2, err := ledger.ChainLedger.LoadChainMeta()
			assert.Contains(t, err.Error(), "unmarshal chain meta")
			assert.Nil(t, meta2)

			err = ledger.Rollback(3)
			assert.Nil(t, err)
			block3, err := ledger.ChainLedger.GetBlock(3)
			assert.Nil(t, err)
			assert.NotNil(t, block3)
			assert.Equal(t, stateRoot3, block3.Header.StateRoot)
			assert.Equal(t, uint64(3), ledger.ChainLedger.GetChainMeta().Height)
			assert.Equal(t, codeHash1, account0.CodeHash())
			assert.Equal(t, code1[:], account0.Code())

			err = ledger.Rollback(2)
			assert.Nil(t, err)
			block, err = ledger.ChainLedger.GetBlock(3)
			assert.Equal(t, ErrNotFound, err)
			assert.Nil(t, block)
			block2, err := ledger.ChainLedger.GetBlock(2)
			assert.Nil(t, err)
			assert.NotNil(t, block2)
			assert.Equal(t, uint64(2), ledger.ChainLedger.GetChainMeta().Height)
			assert.Equal(t, stateRoot2.String(), block2.Header.StateRoot.String())

			account0 = ledger.StateLedger.GetAccount(addr0)
			assert.Equal(t, uint64(2), account0.GetBalance().Uint64())
			assert.Equal(t, uint64(0), account0.GetNonce())
			assert.Equal(t, codeHash[:], account0.CodeHash())
			assert.Equal(t, code[:], account0.Code())
			ok, val := account0.GetState([]byte("a"))
			assert.True(t, ok)
			assert.Equal(t, []byte("2"), val)

			account1 := ledger.StateLedger.GetAccount(addr1)
			assert.Nil(t, account1)

			ledger.ChainLedger.GetChainMeta()
			ledger.ChainLedger.(*ChainLedgerImpl).CloseBlockfile()

			ledger, _ = initLedger(t, repoRoot, tc.kvType)

			err = ledger.Rollback(1)
			assert.Nil(t, err)
			err = ledger.Rollback(0)
			assert.Nil(t, err)

			err = ledger.Rollback(100)
			assert.NotNil(t, err)
		})
	}
}

func TestChainLedger_GetAccount(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			stateLedger := ledger.StateLedger.(*StateLedgerImpl)

			addr := types.NewAddress(LeftPadBytes([]byte{1}, 20))
			code := LeftPadBytes([]byte{1}, 120)
			key0 := []byte{100, 100}
			key1 := []byte{100, 101}

			account := ledger.StateLedger.GetOrCreateAccount(addr)
			account.SetBalance(new(big.Int).SetInt64(1))
			account.SetNonce(2)
			account.SetCodeAndHash(code)

			account.SetState(key0, key1)
			account.SetState(key1, key0)

			stateLedger.blockHeight = 1
			stateLedger.Finalise()
			stateRoot, err := stateLedger.Commit()
			assert.Nil(t, err)
			assert.NotNil(t, stateRoot)

			account1 := ledger.StateLedger.GetAccount(addr)

			assert.Equal(t, account.GetBalance(), ledger.StateLedger.GetBalance(addr))
			assert.Equal(t, account.GetBalance(), account1.GetBalance())
			assert.Equal(t, account.GetNonce(), account1.GetNonce())
			assert.Equal(t, account.CodeHash(), account1.CodeHash())
			assert.Equal(t, account.Code(), account1.Code())
			ok0, val0 := account.GetState(key0)
			ok1, val1 := account.GetState(key1)
			assert.Equal(t, ok0, ok1)
			assert.Equal(t, val0, key1)
			assert.Equal(t, val1, key0)

			key2 := []byte{100, 102}
			val2 := []byte{111}
			ledger.StateLedger.SetState(addr, key0, val0)
			ledger.StateLedger.SetState(addr, key2, val2)
			ledger.StateLedger.SetState(addr, key0, val1)
			stateLedger.blockHeight = 2
			stateLedger.Finalise()
			stateRoot, err = stateLedger.Commit()
			assert.Nil(t, err)
			assert.NotNil(t, stateRoot)

			ledger.StateLedger.SetState(addr, key0, val0)
			ledger.StateLedger.SetState(addr, key0, val1)
			ledger.StateLedger.SetState(addr, key2, nil)
			stateLedger.blockHeight = 3
			stateLedger.Finalise()
			stateRoot, err = stateLedger.Commit()
			assert.Nil(t, err)
			assert.NotNil(t, stateRoot)

			ok, val := ledger.StateLedger.GetState(addr, key0)
			assert.True(t, ok)
			assert.Equal(t, val1, val)

			ok, val2 = ledger.StateLedger.GetState(addr, key2)
			assert.False(t, ok)
			assert.Nil(t, val2)
		})
	}
}

func TestChainLedger_GetCode(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			stateLedger := ledger.StateLedger.(*StateLedgerImpl)

			addr := types.NewAddress(LeftPadBytes([]byte{1}, 20))
			code := LeftPadBytes([]byte{10}, 120)

			code0 := ledger.StateLedger.GetCode(addr)
			assert.Nil(t, code0)

			ledger.StateLedger.SetCode(addr, code)

			stateLedger.blockHeight = 1
			stateRoot, err := stateLedger.Commit()
			assert.Nil(t, err)
			assert.NotNil(t, stateRoot)

			vals := ledger.StateLedger.GetCode(addr)
			assert.Equal(t, code, vals)

			cache, _ := NewAccountCache(1, true)
			acc := &SimpleAccount{
				logger:                loggers.Logger(loggers.Storage),
				Addr:                  addr,
				backend:               stateLedger.backend,
				cache:                 cache,
				enableExpensiveMetric: true,
			}
			assert.Equal(t, common.Hash{}, acc.GetStorageRoot())

			acc.originCode = code
			assert.Equal(t, code, acc.Code())
			acc.originCode = nil

			acc.dirtyAccount = &types.InnerAccount{CodeHash: crypto1.Keccak256Hash(code).Bytes()}
			assert.Equal(t, code, acc.Code())
		})
	}
}

func TestChainLedger_AddState(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)

			account := types.NewAddress(LeftPadBytes([]byte{100}, 20))
			key0 := "100"
			value0 := []byte{100}
			ledger.StateLedger.(*StateLedgerImpl).blockHeight = 1
			ledger.StateLedger.SetState(account, []byte(key0), value0)
			ledger.StateLedger.Finalise()
			rootHash, err := ledger.StateLedger.Commit()
			assert.Nil(t, err)

			ledger.PersistBlockData(genBlockData(1, rootHash))
			require.Equal(t, uint64(1), ledger.StateLedger.Version())

			ok, val := ledger.StateLedger.GetState(account, []byte(key0))
			assert.True(t, ok)
			assert.Equal(t, value0, val)

			key1 := "101"
			value0 = []byte{99}
			value1 := []byte{101}
			ledger.StateLedger.(*StateLedgerImpl).blockHeight = 2
			ledger.StateLedger.SetState(account, []byte(key0), value0)
			ledger.StateLedger.SetState(account, []byte(key1), value1)
			ledger.StateLedger.Finalise()
			rootHash, err = ledger.StateLedger.Commit()
			assert.Nil(t, err)

			ledger.PersistBlockData(genBlockData(2, rootHash))
			require.Equal(t, uint64(2), ledger.StateLedger.Version())

			ok, val = ledger.StateLedger.GetState(account, []byte(key0))
			assert.True(t, ok)
			assert.Equal(t, value0, val)

			ok, val = ledger.StateLedger.GetState(account, []byte(key1))
			assert.True(t, ok)
			assert.Equal(t, value1, val)
		})
	}
}

func TestGetBlockHeaderAndExtra(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetBlockHeader(1)
			assert.True(t, errors.Is(err, ErrNotFound))
			_, err = ledger.ChainLedger.GetBlockExtra(1)
			assert.True(t, errors.Is(err, ErrNotFound))

			ledger, _ = initLedger(t, "", tc.kvType)
			stateLedger := ledger.StateLedger.(*StateLedgerImpl)

			// create an addr0
			addr0 := types.NewAddress(LeftPadBytes([]byte{100}, 20))

			ledger.StateLedger.PrepareBlock(nil, 1)
			ledger.StateLedger.SetBalance(addr0, new(big.Int).SetInt64(1))
			stateLedger.Finalise()
			stateRoot1, err := stateLedger.Commit()
			assert.Nil(t, err)
			assert.NotNil(t, stateRoot1)
			ledger.PersistBlockData(genBlockData(1, stateRoot1))

			ledger.StateLedger.PrepareBlock(stateRoot1, 2)
			ledger.StateLedger.SetBalance(addr0, new(big.Int).SetInt64(2))
			ledger.StateLedger.SetState(addr0, []byte("a"), []byte("2"))

			code := sha256.Sum256([]byte("code"))
			ledger.StateLedger.SetCode(addr0, code[:])
			ledger.StateLedger.Finalise()

			stateRoot2, err := stateLedger.Commit()
			assert.Nil(t, err)
			ledger.PersistBlockData(genBlockData(2, stateRoot2))

			blockHeader, err := ledger.ChainLedger.GetBlockHeader(2)
			assert.Nil(t, err)
			assert.NotNil(t, blockHeader)
			assert.Equal(t, uint64(2), ledger.ChainLedger.GetChainMeta().Height)
			assert.Equal(t, uint64(2), blockHeader.Number)

			blockExtra, err := ledger.ChainLedger.GetBlockExtra(2)
			assert.Nil(t, err)
			assert.NotNil(t, blockExtra)
			assert.NotZero(t, blockExtra.Size)

			number, err := ledger.ChainLedger.GetBlockNumberByHash(blockHeader.Hash())
			assert.Nil(t, err)
			assert.Equal(t, blockHeader.Number, number)
		})
	}
}

func TestGetBlockTxHashListByNumber(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetBlockTxHashList(1)
			assert.NotNil(t, err)
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.BlockTxSetKey, 1), []byte("1"))
			_, err = ledger.ChainLedger.GetBlockTxHashList(1)
			assert.NotNil(t, err)

			txHashes := []*types.Hash{
				{},
			}
			txHashesRaw, err := json.Marshal(txHashes)
			require.Nil(t, err)
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.BlockTxSetKey, 2), txHashesRaw)
			ledger.ChainLedger.(*ChainLedgerImpl).chainMeta.Height = 2
			txHashesRes, err := ledger.ChainLedger.GetBlockTxHashList(2)
			assert.Nil(t, err)
			assert.Equal(t, len(txHashes), len(txHashesRes))
			assert.Equal(t, txHashes[0].String(), txHashesRes[0].String())
		})
	}
}

func TestGetBlockTxListByNumber(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetBlockTxList(1)
			assert.NotNil(t, err)
			err = ledger.ChainLedger.(*ChainLedgerImpl).bf.AppendBlock(0, []byte("1"), []byte("1"), []byte("1"), []byte("1"), []byte("1"))
			require.Nil(t, err)
			_, err = ledger.ChainLedger.GetBlockTxList(1)
			assert.NotNil(t, err)
		})
	}
}

func TestGetTransaction(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.True(t, errors.Is(err, ErrNotFound))
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.TransactionMetaKey, types.NewHash([]byte("1")).String()), []byte("1"))
			_, err = ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
			err = ledger.ChainLedger.(*ChainLedgerImpl).bf.AppendBlock(0, []byte("1"), []byte("1"), []byte("1"), []byte("1"), []byte("1"))
			require.Nil(t, err)
			_, err = ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
		})
	}
}

func TestGetTransaction1(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.True(t, errors.Is(err, ErrNotFound))
			meta := types.TransactionMeta{
				BlockHeight: 0,
			}
			metaBytes, err := meta.Marshal()
			require.Nil(t, err)
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.TransactionMetaKey, types.NewHash([]byte("1")).String()), metaBytes)
			_, err = ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
			err = ledger.ChainLedger.(*ChainLedgerImpl).bf.AppendBlock(0, []byte("1"), []byte("1"), []byte("1"), []byte("1"), []byte("1"))
			require.Nil(t, err)
			_, err = ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
		})
	}
}

func TestGetTransactionMeta(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetTransactionMeta(types.NewHash([]byte("1")))
			assert.True(t, errors.Is(err, ErrNotFound))
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.TransactionMetaKey, types.NewHash([]byte("1")).String()), []byte("1"))
			_, err = ledger.ChainLedger.GetTransactionMeta(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
			err = ledger.ChainLedger.(*ChainLedgerImpl).bf.AppendBlock(0, []byte("1"), []byte("1"), []byte("1"), []byte("1"), []byte("1"))
			require.Nil(t, err)
			_, err = ledger.ChainLedger.GetTransactionMeta(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
		})
	}
}

func TestGetReceipt(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetReceipt(types.NewHash([]byte("1")))
			assert.True(t, errors.Is(err, ErrNotFound))
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.TransactionMetaKey, types.NewHash([]byte("1")).String()), []byte("0"))
			_, err = ledger.ChainLedger.GetReceipt(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
			err = ledger.ChainLedger.(*ChainLedgerImpl).bf.AppendBlock(0, []byte("1"), []byte("1"), []byte("1"), []byte("1"), []byte("1"))
			require.Nil(t, err)
			_, err = ledger.ChainLedger.GetReceipt(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
			_, err = ledger.ChainLedger.GetBlockReceipts(0)
			assert.Contains(t, err.Error(), "get receipts with height 0 from blockfile failed")
			_, err = ledger.ChainLedger.GetBlockReceipts(1)
			assert.Contains(t, err.Error(), "EOF")
		})
	}
}

func TestGetReceipt1(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			_, err := ledger.ChainLedger.GetTransaction(types.NewHash([]byte("1")))
			assert.True(t, errors.Is(err, ErrNotFound))
			meta := types.TransactionMeta{
				BlockHeight: 0,
			}
			metaBytes, err := meta.Marshal()
			require.Nil(t, err)
			ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.Put(utils.CompositeKey(utils.TransactionMetaKey, types.NewHash([]byte("1")).String()), metaBytes)
			_, err = ledger.ChainLedger.GetReceipt(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
			err = ledger.ChainLedger.(*ChainLedgerImpl).bf.AppendBlock(0, []byte("1"), []byte("1"), []byte("1"), []byte("1"), []byte("1"))
			require.Nil(t, err)
			_, err = ledger.ChainLedger.GetReceipt(types.NewHash([]byte("1")))
			assert.NotNil(t, err)
		})
	}
}

func TestPrepare(t *testing.T) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}
	for name, tc := range testcase {
		t.Run(name, func(t *testing.T) {
			ledger, _ := initLedger(t, "", tc.kvType)
			batch := ledger.ChainLedger.(*ChainLedgerImpl).blockchainStore.NewBatch()
			var transactions []*types.Transaction
			transaction, err := types.GenerateEmptyTransactionAndSigner()
			require.Nil(t, err)
			transactions = append(transactions, transaction)
			block := &types.Block{
				Header: &types.BlockHeader{
					Number: uint64(0),
				},
				Transactions: transactions,
			}
			_, _, err = ledger.ChainLedger.(*ChainLedgerImpl).prepareBlock(batch, block)
			require.Nil(t, err)
			var receipts []*types.Receipt
			receipt := &types.Receipt{
				TxHash: types.NewHash([]byte("1")),
			}
			receipts = append(receipts, receipt)
			_, err = ledger.ChainLedger.(*ChainLedgerImpl).prepareReceipts(batch, block, receipts)
			require.Nil(t, err)
			_, err = ledger.ChainLedger.(*ChainLedgerImpl).prepareTransactions(batch, block)
			require.Nil(t, err)

			bloomRes := CreateBloom(receipts)
			require.NotNil(t, bloomRes)
		})
	}
}

// =========================== Test History Ledger ===========================

func TestStateLedger_EOAHistory(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))
	account3 := types.NewAddress(LeftPadBytes([]byte{103}, 20))

	// set EOA account data in block 1
	// account1: balance=101, nonce=0
	// account2: balance=201, nonce=0
	// account3: balance=301, nonce=0
	sl.blockHeight = 1
	sl.SetBalance(account1, new(big.Int).SetInt64(101))
	sl.SetBalance(account2, new(big.Int).SetInt64(201))
	sl.SetBalance(account3, new(big.Int).SetInt64(301))
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	isSuicide := sl.HasSuicide(account1)
	assert.Equal(t, isSuicide, false)
	assert.Equal(t, uint64(1), sl.Version())

	// set EOA account data in block 2
	// account1: balance=102, nonce=12
	// account2: balance=201, nonce=22
	// account3: balance=302, nonce=32
	sl.blockHeight = 2
	sl.SetBalance(account1, new(big.Int).SetInt64(102))
	sl.SetBalance(account3, new(big.Int).SetInt64(302))
	sl.SetNonce(account1, 12)
	sl.SetNonce(account2, 22)
	sl.SetNonce(account3, 32)
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set EOA account data in block 3
	// account1: balance=103, nonce=13
	// account2: balance=203, nonce=23
	// account3: balance=302, nonce=32
	sl.blockHeight = 3
	sl.SetBalance(account1, new(big.Int).SetInt64(103))
	sl.SetBalance(account2, new(big.Int).SetInt64(203))
	sl.SetNonce(account1, 13)
	sl.SetNonce(account2, 23)
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// set EOA account data in block 4 (same with block 3)
	// account1: balance=103, nonce=13
	// account2: balance=203, nonce=23
	// account3: balance=302, nonce=32
	sl.blockHeight = 4
	stateRoot4, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(4), sl.Version())
	assert.Equal(t, stateRoot3, stateRoot4)

	// set EOA account data in block 5
	// account1: balance=103, nonce=15
	// account2: balance=203, nonce=25
	// account3: balance=305, nonce=35
	sl.blockHeight = 5
	sl.SetBalance(account1, new(big.Int).SetInt64(103))
	sl.SetBalance(account2, new(big.Int).SetInt64(203))
	sl.SetBalance(account3, new(big.Int).SetInt64(305))
	sl.SetNonce(account1, 15)
	sl.SetNonce(account2, 25)
	sl.SetNonce(account3, 35)
	stateRoot5, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(5), sl.Version())
	assert.NotEqual(t, stateRoot4, stateRoot5)

	// check state ledger in block 1
	block1 := &types.Block{
		Header: &types.BlockHeader{
			Number:    1,
			StateRoot: stateRoot1,
		},
	}
	lg1, _ := sl.NewView(block1.Header, false)
	lg1.(*StateLedgerImpl).accountCache.clear()
	assert.Equal(t, uint64(101), lg1.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(201), lg1.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(301), lg1.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(0), lg1.GetNonce(account1))
	assert.Equal(t, uint64(0), lg1.GetNonce(account2))
	assert.Equal(t, uint64(0), lg1.GetNonce(account3))

	// check state ledger in block 2
	block2 := &types.Block{
		Header: &types.BlockHeader{
			Number:    2,
			StateRoot: stateRoot2,
		},
	}
	lg2, _ := sl.NewViewWithoutCache(block2.Header, false)
	assert.Equal(t, uint64(102), lg2.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(201), lg2.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(302), lg2.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(12), lg2.GetNonce(account1))
	assert.Equal(t, uint64(22), lg2.GetNonce(account2))
	assert.Equal(t, uint64(32), lg2.GetNonce(account3))

	// check state ledger in block 3
	block3 := &types.Block{
		Header: &types.BlockHeader{
			Number:    3,
			StateRoot: stateRoot3,
		},
	}
	lg3, _ := sl.NewViewWithoutCache(block3.Header, false)
	assert.Equal(t, uint64(103), lg3.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(203), lg3.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(302), lg3.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(13), lg3.GetNonce(account1))
	assert.Equal(t, uint64(23), lg3.GetNonce(account2))
	assert.Equal(t, uint64(32), lg3.GetNonce(account3))

	// check state ledger in block 4
	block4 := &types.Block{
		Header: &types.BlockHeader{
			Number:    4,
			StateRoot: stateRoot4,
		},
	}
	lg4, _ := sl.NewViewWithoutCache(block4.Header, false)
	assert.Equal(t, uint64(103), lg4.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(203), lg4.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(302), lg4.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(13), lg4.GetNonce(account1))
	assert.Equal(t, uint64(23), lg4.GetNonce(account2))
	assert.Equal(t, uint64(32), lg4.GetNonce(account3))

	// check state ledger in block 5
	block5 := &types.Block{
		Header: &types.BlockHeader{
			Number:    5,
			StateRoot: stateRoot5,
		},
	}
	lg5, _ := sl.NewView(block5.Header, true)
	lg5.(*StateLedgerImpl).accountCache.clear()
	assert.Equal(t, uint64(103), lg5.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(203), lg5.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(305), lg5.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(15), lg5.GetNonce(account1))
	assert.Equal(t, uint64(25), lg5.GetNonce(account2))
	assert.Equal(t, uint64(35), lg5.GetNonce(account3))
}

func TestStateLedger_ContractStateHistory(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))
	account3 := types.NewAddress(LeftPadBytes([]byte{103}, 20))

	// set contract account data in block 1
	// account1: key1=val101, key2=val102
	// account3: key1=val301, key2=val302
	sl.blockHeight = 1
	sl.SetState(account1, []byte("key1"), []byte("val101"))
	sl.SetState(account1, []byte("key2"), []byte("val102"))
	sl.SetState(account3, []byte("key1"), []byte("val301"))
	sl.SetState(account3, []byte("key2"), []byte("val302"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	isSuicide := sl.HasSuicide(account1)
	assert.Equal(t, isSuicide, false)
	assert.Equal(t, uint64(1), sl.Version())

	// set contract account data in block 2
	// account1: key1=val1011, key2=val102
	// account2: key1=val201, key2=val202
	// account3: key1=val3011, key2=val3021
	sl.blockHeight = 2
	sl.SetState(account1, []byte("key1"), []byte("val1011"))
	sl.SetState(account2, []byte("key1"), []byte("val201"))
	sl.SetState(account2, []byte("key2"), []byte("val202"))
	sl.SetState(account3, []byte("key1"), []byte("val3011"))
	sl.SetState(account3, []byte("key2"), []byte("val3021"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set contract account data in block 3
	// account1: key1=val1013, key2=val102
	// account2: key1=val2011, key2=nil
	// account3: key1=nil, key2=nil
	sl.blockHeight = 3
	sl.SetState(account1, []byte("key1"), []byte("val1013"))
	sl.SetState(account2, []byte("key1"), []byte("val2011"))
	sl.SetState(account2, []byte("key2"), nil)
	sl.SetState(account3, []byte("key1"), nil)
	sl.SetState(account3, []byte("key2"), nil)
	sl.Finalise()
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// set contract account data in block 4 (same with block 3)
	// account1: key1=val1013, key2=val102
	// account2: key1=val2011, key2=nil
	// account3: key1=nil, key2=nil
	sl.blockHeight = 4
	sl.Finalise()
	stateRoot4, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(4), sl.Version())
	assert.Equal(t, stateRoot3, stateRoot4)

	// set contract account data in block 5
	// account1: key1=val1015, key2=val1025
	// account2: key1=val2015, key2=val2025
	// account3: key1=val3015, key2=val3025
	sl.blockHeight = 5
	sl.SetState(account1, []byte("key1"), []byte("val1015"))
	sl.SetState(account1, []byte("key2"), []byte("val1025"))
	sl.SetState(account2, []byte("key1"), []byte("val2015"))
	sl.SetState(account2, []byte("key2"), []byte("val2025"))
	sl.SetState(account3, []byte("key1"), []byte("val3015"))
	sl.SetState(account3, []byte("key2"), []byte("val3025"))
	sl.Finalise()
	stateRoot5, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(5), sl.Version())
	assert.NotEqual(t, stateRoot4, stateRoot5)

	// check state ledger in block 1
	block1 := &types.Block{
		Header: &types.BlockHeader{
			Number:    1,
			StateRoot: stateRoot1,
		},
	}
	lg1, _ := sl.NewViewWithoutCache(block1.Header, false)
	exist, a1k1 := lg1.GetState(account1, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val101"), a1k1)
	exist, a1k2 := lg1.GetState(account1, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val102"), a1k2)
	exist, _ = lg1.GetState(account2, []byte("key1"))
	assert.False(t, exist)
	exist, a3k1 := lg1.GetState(account3, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val301"), a3k1)
	exist, a3k2 := lg1.GetState(account3, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val302"), a3k2)

	// check state ledger in block 2
	block2 := &types.Block{
		Header: &types.BlockHeader{
			Number:    2,
			StateRoot: stateRoot2,
		},
	}
	lg2, _ := sl.NewViewWithoutCache(block2.Header, false)
	exist, a1k1 = lg2.GetState(account1, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1011"), a1k1)
	exist, a1k2 = lg2.GetState(account1, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val102"), a1k2)
	exist, a2k1 := lg2.GetState(account2, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val201"), a2k1)
	exist, a2k2 := lg2.GetState(account2, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val202"), a2k2)
	exist, a3k1 = lg2.GetState(account3, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val3011"), a3k1)
	exist, a3k2 = lg2.GetState(account3, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val3021"), a3k2)

	// check state ledger in block 3
	block3 := &types.Block{
		Header: &types.BlockHeader{
			Number:    3,
			StateRoot: stateRoot3,
		},
	}
	lg3, _ := sl.NewViewWithoutCache(block3.Header, false)
	exist, a1k1 = lg3.GetState(account1, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1013"), a1k1)
	exist, a1k2 = lg3.GetState(account1, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val102"), a1k2)
	exist, a2k1 = lg3.GetState(account2, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val2011"), a2k1)
	exist, a2k2 = lg3.GetState(account2, []byte("key2"))
	assert.False(t, exist)
	exist, a3k1 = lg3.GetState(account3, []byte("key1"))
	assert.False(t, exist)
	exist, a3k2 = lg3.GetState(account3, []byte("key2"))
	assert.False(t, exist)

	// check state ledger in block 4
	block4 := &types.Block{
		Header: &types.BlockHeader{
			Number:    4,
			StateRoot: stateRoot4,
		},
	}
	lg4, _ := sl.NewViewWithoutCache(block4.Header, false)
	exist, a1k1 = lg4.GetState(account1, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1013"), a1k1)
	exist, a1k2 = lg4.GetState(account1, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val102"), a1k2)
	exist, a2k1 = lg4.GetState(account2, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val2011"), a2k1)
	exist, a2k2 = lg4.GetState(account2, []byte("key2"))
	assert.False(t, exist)
	exist, a3k1 = lg4.GetState(account3, []byte("key1"))
	assert.False(t, exist)
	exist, a3k2 = lg4.GetState(account3, []byte("key2"))
	assert.False(t, exist)

	// check state ledger in block 5
	block5 := &types.Block{
		Header: &types.BlockHeader{
			Number:    5,
			StateRoot: stateRoot5,
		},
	}
	lg5, _ := sl.NewViewWithoutCache(block5.Header, false)
	exist, a1k1 = lg5.GetState(account1, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1015"), a1k1)
	exist, a1k2 = lg5.GetState(account1, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1025"), a1k2)
	exist, a2k1 = lg5.GetState(account2, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val2015"), a2k1)
	exist, a2k2 = lg5.GetState(account2, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val2025"), a2k2)
	exist, a3k1 = lg5.GetState(account3, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val3015"), a3k1)
	exist, a3k2 = lg5.GetState(account3, []byte("key2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val3025"), a3k2)
}

func TestStateLedger_ContractCodeHistory(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	eoaAccount := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	contractAccount := types.NewAddress(LeftPadBytes([]byte{102}, 20))

	// set account data in block 1
	// account1: nonce=1
	// account2: key1=val1,code=code1
	sl.blockHeight = 1
	sl.SetNonce(eoaAccount, 1)
	code1 := sha256.Sum256([]byte("code1"))
	sl.SetState(contractAccount, []byte("key1"), []byte("val1"))
	sl.SetCode(contractAccount, code1[:])
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), sl.Version())

	// set account data in block 2
	// account1: nonce=1
	// account2: key1=val1,code=code2
	sl.blockHeight = 2
	code2 := sha256.Sum256([]byte("code2"))
	sl.SetCode(contractAccount, code2[:])
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.NotNil(t, stateRoot2)
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set account data in block 3
	// account1: nonce=2
	// account2: key1=val1,code=code2
	sl.blockHeight = 3
	sl.SetNonce(eoaAccount, 2)
	sl.Finalise()
	stateRoot3, err := sl.Commit()
	assert.NotNil(t, stateRoot3)
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// check state ledger in block 1
	block1 := &types.Block{
		Header: &types.BlockHeader{
			Number:    1,
			StateRoot: stateRoot1,
		},
	}
	lg1, _ := sl.NewViewWithoutCache(block1.Header, false)
	assert.Equal(t, uint64(1), lg1.GetNonce(eoaAccount))
	exist, a2k1 := lg1.GetState(contractAccount, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1"), a2k1)
	assert.Equal(t, code1[:], lg1.GetCode(contractAccount))

	// check state ledger in block 2
	block2 := &types.Block{
		Header: &types.BlockHeader{
			Number:    2,
			StateRoot: stateRoot2,
		},
	}
	lg2, _ := sl.NewViewWithoutCache(block2.Header, false)
	assert.Equal(t, uint64(1), lg2.GetNonce(eoaAccount))
	exist, a2k1 = lg2.GetState(contractAccount, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1"), a2k1)
	assert.Equal(t, code2[:], lg2.GetCode(contractAccount))

	// check state ledger in block 3
	block3 := &types.Block{
		Header: &types.BlockHeader{
			Number:    3,
			StateRoot: stateRoot3,
		},
	}
	lg3, _ := sl.NewViewWithoutCache(block3.Header, false)
	assert.Equal(t, uint64(2), lg3.GetNonce(eoaAccount))
	exist, a2k1 = lg3.GetState(contractAccount, []byte("key1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("val1"), a2k1)
	assert.Equal(t, code2[:], lg3.GetCode(contractAccount))
}

func TestStateLedger_RollbackToAccountHistoryVersion(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))
	account3 := types.NewAddress(LeftPadBytes([]byte{103}, 20))

	// set EOA account data in block 1
	// account1: balance=101, nonce=0
	// account2: balance=201, nonce=0
	// account3: balance=301, nonce=0
	sl.blockHeight = 1
	sl.SetBalance(account1, new(big.Int).SetInt64(101))
	sl.SetBalance(account2, new(big.Int).SetInt64(201))
	sl.SetNonce(account1, 1)
	sl.SetNonce(account2, 2)
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(1, stateRoot1))
	isSuicide := sl.HasSuicide(account1)
	assert.Equal(t, isSuicide, false)
	assert.Equal(t, uint64(1), sl.Version())

	// set EOA account data in block 2
	// account1: balance=102, nonce=12
	// account2: balance=201, nonce=22
	// account3: balance=302, nonce=32
	sl.blockHeight = 2
	sl.SetBalance(account1, new(big.Int).SetInt64(102))
	sl.SetBalance(account3, new(big.Int).SetInt64(302))
	sl.SetNonce(account1, 12)
	sl.SetNonce(account2, 22)
	sl.SetNonce(account3, 32)
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2))
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set EOA account data in block 3
	// account1: balance=103, nonce=13
	// account2: balance=203, nonce=23
	// account3: balance=302, nonce=32
	sl.blockHeight = 3
	sl.SetBalance(account1, new(big.Int).SetInt64(103))
	sl.SetBalance(account2, new(big.Int).SetInt64(203))
	sl.SetNonce(account1, 13)
	sl.SetNonce(account2, 23)
	sl.Finalise()
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(3, stateRoot3))
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// revert from block 3 to block 2
	err = lg.Rollback(2)
	assert.Nil(t, err)

	// check state ledger in block 2
	lg = lg.NewViewWithoutCache()
	assert.Equal(t, uint64(102), lg.StateLedger.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(201), lg.StateLedger.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(302), lg.StateLedger.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(12), lg.StateLedger.GetNonce(account1))
	assert.Equal(t, uint64(22), lg.StateLedger.GetNonce(account2))
	assert.Equal(t, uint64(32), lg.StateLedger.GetNonce(account3))

	// revert from block 2 to block 1
	err = lg.Rollback(1)
	assert.Nil(t, err)

	// check state ledger in block 1
	assert.Equal(t, uint64(101), lg.StateLedger.GetBalance(account1).Uint64())
	assert.Equal(t, uint64(201), lg.StateLedger.GetBalance(account2).Uint64())
	assert.Equal(t, uint64(0), lg.StateLedger.GetBalance(account3).Uint64())
	assert.Equal(t, uint64(1), lg.StateLedger.GetNonce(account1))
	assert.Equal(t, uint64(2), lg.StateLedger.GetNonce(account2))
	assert.Equal(t, uint64(0), lg.StateLedger.GetNonce(account3))

	// revert from block 1 to block 3
	err = lg.Rollback(3)
	assert.NotNil(t, err)
}

func TestStateLedger_RollbackToContractHistoryVersion(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))
	account3 := types.NewAddress(LeftPadBytes([]byte{103}, 20))

	// set contract account data in block 1
	// account1: k1=v1
	// account2: k2=v2
	sl.blockHeight = 1
	sl.SetState(account1, []byte("k1"), []byte("v1"))
	sl.SetState(account2, []byte("k2"), []byte("v2"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(1, stateRoot1))
	assert.Equal(t, uint64(1), sl.Version())

	// set contract account data in block 2
	// account1: k1=v1, k11=v11
	// account2: k2=v22
	// account3: k3=v3
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k11"), []byte("v11"))
	sl.SetState(account2, []byte("k2"), []byte("v22"))
	sl.SetState(account3, []byte("k3"), []byte("v3"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2))
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set contract account data in block 3
	// account1: k1=v111, k11=nil
	// account2: k2=v222
	// account3: k3=v33
	sl.blockHeight = 3
	sl.SetState(account1, []byte("k1"), []byte("v111"))
	sl.SetState(account1, []byte("k11"), nil)
	sl.SetState(account2, []byte("k2"), []byte("v222"))
	sl.SetState(account3, []byte("k3"), []byte("v33"))
	sl.Finalise()
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(3, stateRoot3))
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// set contract account data in block 4
	// account1: k1=v111, k11=v1111
	// account2: k2=v222
	// account3: k3=v33
	sl.blockHeight = 4
	sl.SetState(account1, []byte("k11"), []byte("v1111"))
	sl.Finalise()
	stateRoot4, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(4, stateRoot3))
	assert.Equal(t, uint64(4), sl.Version())
	assert.NotEqual(t, stateRoot4, stateRoot3)

	// check state ledger in block 4
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v := lg.StateLedger.GetState(account1, []byte("k11"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1111"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v111"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v222"), v)
	// verify acc3
	exist, v = lg.StateLedger.GetState(account3, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v33"), v)

	// revert from block 4 to block 3
	err = lg.Rollback(3)
	assert.Nil(t, err)
	// check state ledger in block 3
	// account1: k1=v111, k11=nil
	// account2: k2=v222
	// account3: k3=v33
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k11"))
	assert.False(t, exist)
	assert.Equal(t, []byte(nil), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v111"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v222"), v)
	// verify acc3
	exist, v = lg.StateLedger.GetState(account3, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v33"), v)

	// revert from block 3 to block 2
	err = lg.Rollback(2)
	assert.Nil(t, err)
	// check state ledger in block 2
	// account1: k1=v1, k11=v11
	// account2: k2=v22
	// account3: k3=v3
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k11"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v11"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)
	// verify acc3
	exist, v = lg.StateLedger.GetState(account3, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v3"), v)

	// revert from block 2 to block 1
	err = lg.Rollback(1)
	assert.Nil(t, err)
	// check state ledger in block 1
	// account1: k1=v1
	// account2: k2=v2
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k11"))
	assert.False(t, exist)
	assert.Equal(t, []byte(nil), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v2"), v)
	// verify acc3
	exist, v = lg.StateLedger.GetState(account3, []byte("k3"))
	assert.False(t, exist)
	assert.Equal(t, []byte(nil), v)
}

func TestStateLedger_ReplayAfterAccountRollback(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))

	// set contract account data in block 1
	// account1: k1=v1,k2=v2
	// account2: k2=v2
	sl.blockHeight = 1
	sl.SetState(account1, []byte("k1"), []byte("v1"))
	sl.SetState(account1, []byte("k2"), []byte("v2"))
	sl.SetState(account2, []byte("k2"), []byte("v2"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(1, stateRoot1))
	assert.Equal(t, uint64(1), sl.Version())

	// set contract account data in block 2
	// account1: k1=v1, k2=v22, k3=v3
	// account2: k2=v22
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v22"))
	sl.SetState(account1, []byte("k3"), []byte("v3"))
	sl.SetState(account2, []byte("k2"), []byte("v22"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2))
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// check state ledger in block 2
	// account1: k1=v1, k2=v22, k3=v3
	// account2: k2=v22
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v := lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v3"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)

	// revert from block 2 to block 1
	err = lg.Rollback(1)
	sl = lg.StateLedger.(*StateLedgerImpl)
	assert.Nil(t, err)
	// replay block 2 with the same state
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v22"))
	sl.SetState(account2, []byte("k2"), []byte("v22"))
	sl.SetState(account1, []byte("k3"), []byte("v3"))
	sl.Finalise()
	stateRoot2AfterReplay1, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2AfterReplay1))
	assert.Equal(t, stateRoot2AfterReplay1, stateRoot2)

	// check state ledger in block 2 after replay
	// account1: k1=v1, k2=v22, k3=v3
	// account2: k2=v22
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v3"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)

	// revert from block 2 to block 1
	err = lg.Rollback(1)
	assert.Nil(t, err)
	// replay block 2 with different state
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v222"))
	sl.SetState(account2, []byte("k2"), []byte("v222"))
	sl.SetState(account1, []byte("k3"), []byte("v33"))
	sl.Finalise()
	stateRoot2AfterReplay2, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2AfterReplay2))
	assert.NotEqual(t, stateRoot2AfterReplay2, stateRoot2)

	// check state ledger in block 2 after second replay
	// account1: k1=v1, k2=v222, k3=v33
	// account2: k2=v222
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v222"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v33"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v222"), v)
}

func TestStateLedger_ReplayAfterStateRollback(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))

	// set contract account data in block 1
	// account1: k1=v1,k2=v2
	// account2: k2=v2
	sl.blockHeight = 1
	sl.SetState(account1, []byte("k1"), []byte("v1"))
	sl.SetState(account1, []byte("k2"), []byte("v2"))
	sl.SetState(account2, []byte("k2"), []byte("v2"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(1, stateRoot1))
	assert.Equal(t, uint64(1), sl.Version())

	// set contract account data in block 2
	// account1: k1=v1, k2=v22, k3=v3
	// account2: k2=v22
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v22"))
	sl.SetState(account1, []byte("k3"), []byte("v3"))
	sl.SetState(account2, []byte("k2"), []byte("v22"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2))
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// check state ledger in block 2
	// account1: k1=v1, k2=v22, k3=v3
	// account2: k2=v22
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v := lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v3"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)

	// revert from block 2 to block 1
	err = lg.Rollback(1)
	assert.Nil(t, err)
	// replay block 2 with the same state
	sl = lg.StateLedger.(*StateLedgerImpl)
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v22"))
	sl.SetState(account2, []byte("k2"), []byte("v22"))
	sl.SetState(account1, []byte("k3"), []byte("v3"))
	sl.Finalise()
	stateRoot2AfterReplay1, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2AfterReplay1))
	assert.Equal(t, stateRoot2AfterReplay1, stateRoot2)

	// check state ledger in block 2 after replay
	// account1: k1=v1, k2=v22, k3=v3
	// account2: k2=v22
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v3"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v22"), v)

	// revert from block 2 to block 1
	err = lg.Rollback(1)
	assert.Nil(t, err)
	// replay block 2 with different state
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v222"))
	sl.SetState(account2, []byte("k2"), []byte("v222"))
	sl.SetState(account1, []byte("k3"), []byte("v33"))
	sl.Finalise()
	stateRoot2AfterReplay2, err := sl.Commit()
	assert.Nil(t, err)
	lg.PersistBlockData(genBlockData(2, stateRoot2AfterReplay2))
	assert.NotEqual(t, stateRoot2AfterReplay2, stateRoot2)

	// check state ledger in block 2 after second replay
	// account1: k1=v1, k2=v222, k3=v33
	// account2: k2=v222
	lg = lg.NewViewWithoutCache()
	// verify acc1
	exist, v = lg.StateLedger.GetState(account1, []byte("k1"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v1"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v222"), v)
	exist, v = lg.StateLedger.GetState(account1, []byte("k3"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v33"), v)
	// verify acc2
	exist, v = lg.StateLedger.GetState(account2, []byte("k2"))
	assert.True(t, exist)
	assert.Equal(t, []byte("v222"), v)
}

func TestStateLedger_AccountSuicide(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account := types.NewAddress(LeftPadBytes([]byte{100}, 20))

	sl.blockHeight = 1
	sl.SetState(account, []byte("a"), []byte("b"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	assert.Equal(t, sl.HasSuicide(account), false)
	assert.Equal(t, uint64(1), sl.Version())

	code := RightPadBytes([]byte{100}, 100)
	lg.StateLedger.SetCode(account, code)
	lg.StateLedger.SetState(account, []byte("b"), []byte("3"))
	sl.blockHeight = 2
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), lg.StateLedger.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	exist, va := sl.GetState(account, []byte("a"))
	assert.True(t, exist)
	assert.Equal(t, []byte("b"), va)

	exist, vb := sl.GetState(account, []byte("b"))
	assert.True(t, exist)
	assert.Equal(t, []byte("3"), vb)

	acc := sl.GetAccount(account)
	assert.NotNil(t, acc)

	// suicide account
	evmLg := &EvmStateDBAdaptor{StateLedger: lg.StateLedger}
	evmLg.Suicide(account.ETHAddress())
	sl.blockHeight = 3
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), lg.StateLedger.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	acc = sl.GetAccount(account)
	assert.Nil(t, acc)
}

// =========================== Test Trie Snapshot ===========================

func TestStateLedger_IterateEOATrie(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddressByStr("dac17f958d2ee523a2206206994597c13d831ec7")
	account2 := types.NewAddressByStr("0000000000000000000000000000000000000007")
	account3 := types.NewAddressByStr("9000000000000000000000000000000000000000")

	// set EOA account data in block 1
	// account1: balance=101, nonce=0
	// account2: balance=201, nonce=0
	// account3: balance=301, nonce=0
	sl.blockHeight = 1
	sl.SetBalance(account1, new(big.Int).SetInt64(101))
	sl.SetBalance(account2, new(big.Int).SetInt64(201))
	sl.SetBalance(account3, new(big.Int).SetInt64(301))
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	isSuicide := sl.HasSuicide(account1)
	assert.Equal(t, isSuicide, false)
	assert.Equal(t, uint64(1), sl.Version())

	// set EOA account data in block 2
	// account1: balance=102, nonce=12
	// account2: balance=201, nonce=22
	// account3: balance=302, nonce=32
	sl.blockHeight = 2
	sl.SetBalance(account1, new(big.Int).SetInt64(102))
	sl.SetBalance(account3, new(big.Int).SetInt64(302))
	sl.SetNonce(account1, 12)
	sl.SetNonce(account2, 22)
	sl.SetNonce(account3, 32)
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set EOA account data in block 3
	// account1: balance=103, nonce=13
	// account2: balance=203, nonce=23
	// account3: balance=302, nonce=32
	sl.blockHeight = 3
	sl.SetBalance(account1, new(big.Int).SetInt64(103))
	sl.SetBalance(account2, new(big.Int).SetInt64(203))
	sl.SetNonce(account1, 13)
	sl.SetNonce(account2, 23)
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// set EOA account data in block 4 (same with block 3)
	// account1: balance=103, nonce=13
	// account2: balance=203, nonce=23
	// account3: balance=302, nonce=32
	sl.blockHeight = 4
	stateRoot4, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(4), sl.Version())
	assert.Equal(t, stateRoot3, stateRoot4)

	// set EOA account data in block 5
	// account1: balance=103, nonce=15
	// account2: balance=203, nonce=25
	// account3: balance=305, nonce=35
	sl.blockHeight = 5
	sl.SetBalance(account1, new(big.Int).SetInt64(103))
	sl.SetBalance(account2, new(big.Int).SetInt64(203))
	sl.SetBalance(account3, new(big.Int).SetInt64(305))
	sl.SetNonce(account1, 15)
	sl.SetNonce(account2, 25)
	sl.SetNonce(account3, 35)
	stateRoot5, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(5), sl.Version())
	assert.NotEqual(t, stateRoot4, stateRoot5)

	t.Run("test iterate and verify trie of block 1", func(t *testing.T) {
		// iterate trie of block 1
		block1 := &types.Block{
			Header: &types.BlockHeader{
				Number:    1,
				StateRoot: stateRoot1,
				Epoch:     1,
			},
		}
		s1 := initKVStorage(createMockRepo(t).RepoRoot)
		errC1 := make(chan error)
		go sl.IterateTrie(block1.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s1, errC1)
		err, ok := <-errC1
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 1
		sl1, err := sl.NewViewWithoutCache(block1.Header, false)
		assert.Nil(t, err)
		sl1.(*StateLedgerImpl).backend = s1
		sl1.(*StateLedgerImpl).refreshAccountTrie(block1.Header.StateRoot)
		verify, err := sl1.VerifyTrie(block1.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		assert.Equal(t, uint64(101), sl1.GetBalance(account1).Uint64())
		assert.Equal(t, uint64(201), sl1.GetBalance(account2).Uint64())
		assert.Equal(t, uint64(301), sl1.GetBalance(account3).Uint64())
		assert.Equal(t, uint64(0), sl1.GetNonce(account1))
		assert.Equal(t, uint64(0), sl1.GetNonce(account2))
		assert.Equal(t, uint64(0), sl1.GetNonce(account3))
		meta, err := sl1.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block1.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
		assert.Equal(t, block1.Header.Epoch, meta.EpochInfo.Epoch)
		assert.Equal(t, "P2PNodeID-1", meta.EpochInfo.ValidatorSet[0].P2PNodeID)
		assert.Equal(t, "P2PNodeID-1", meta.Nodes.Validators[0].PeerId)
	})

	t.Run("test iterate and verify trie of block 2", func(t *testing.T) {
		// iterate trie of block 2
		block2 := &types.Block{
			Header: &types.BlockHeader{
				Number:    2,
				StateRoot: stateRoot2,
			},
		}
		s2 := initKVStorage(createMockRepo(t).RepoRoot)
		errC2 := make(chan error)
		go sl.IterateTrie(block2.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s2, errC2)
		err, ok := <-errC2
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 2
		sl2, _ := sl.NewViewWithoutCache(block2.Header, false)
		sl2.(*StateLedgerImpl).backend = s2
		sl2.(*StateLedgerImpl).refreshAccountTrie(block2.Header.StateRoot)
		verify, err := sl2.VerifyTrie(block2.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		assert.Equal(t, uint64(102), sl2.GetBalance(account1).Uint64())
		assert.Equal(t, uint64(201), sl2.GetBalance(account2).Uint64())
		assert.Equal(t, uint64(302), sl2.GetBalance(account3).Uint64())
		assert.Equal(t, uint64(12), sl2.GetNonce(account1))
		assert.Equal(t, uint64(22), sl2.GetNonce(account2))
		assert.Equal(t, uint64(32), sl2.GetNonce(account3))
		meta, err := sl2.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block2.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})

	t.Run("test iterate and verify trie of block 3", func(t *testing.T) {
		// iterate trie of block 3
		block3 := &types.Block{
			Header: &types.BlockHeader{
				Number:    3,
				StateRoot: stateRoot3,
			},
		}
		s3 := initKVStorage(createMockRepo(t).RepoRoot)
		errC3 := make(chan error)
		go sl.IterateTrie(block3.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s3, errC3)
		err, ok := <-errC3
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 3
		sl3, _ := sl.NewViewWithoutCache(block3.Header, false)
		sl3.(*StateLedgerImpl).backend = s3
		verify, err := sl3.VerifyTrie(block3.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		assert.Equal(t, uint64(103), sl3.GetBalance(account1).Uint64())
		assert.Equal(t, uint64(203), sl3.GetBalance(account2).Uint64())
		assert.Equal(t, uint64(302), sl3.GetBalance(account3).Uint64())
		assert.Equal(t, uint64(13), sl3.GetNonce(account1))
		assert.Equal(t, uint64(23), sl3.GetNonce(account2))
		assert.Equal(t, uint64(32), sl3.GetNonce(account3))
		meta, err := sl3.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block3.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
		// change block3 stateRoot
		block3.Header.StateRoot = types.NewHashByStr("0x1234")
		verify, err = sl3.VerifyTrie(block3.Header)
		assert.False(t, verify)
		assert.NotNil(t, err)
	})

	t.Run("test iterate and verify trie of block 4", func(t *testing.T) {
		// iterate trie of block 4
		block4 := &types.Block{
			Header: &types.BlockHeader{
				Number:    4,
				StateRoot: stateRoot4,
			},
		}
		s4 := initKVStorage(createMockRepo(t).RepoRoot)
		errC4 := make(chan error)
		go sl.IterateTrie(block4.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s4, errC4)
		err, ok := <-errC4
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 4
		sl4, _ := sl.NewViewWithoutCache(block4.Header, false)
		sl4.(*StateLedgerImpl).backend = s4
		sl4.(*StateLedgerImpl).refreshAccountTrie(block4.Header.StateRoot)
		verify, err := sl4.VerifyTrie(block4.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		assert.Equal(t, uint64(103), sl4.GetBalance(account1).Uint64())
		assert.Equal(t, uint64(203), sl4.GetBalance(account2).Uint64())
		assert.Equal(t, uint64(302), sl4.GetBalance(account3).Uint64())
		assert.Equal(t, uint64(13), sl4.GetNonce(account1))
		assert.Equal(t, uint64(23), sl4.GetNonce(account2))
		assert.Equal(t, uint64(32), sl4.GetNonce(account3))
		meta, err := sl4.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block4.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})

	t.Run("test iterate and verify trie of block 5", func(t *testing.T) {
		// iterate trie of block 5
		block5 := &types.Block{
			Header: &types.BlockHeader{
				Number:    5,
				StateRoot: stateRoot5,
			},
		}
		s5 := initKVStorage(createMockRepo(t).RepoRoot)
		errC5 := make(chan error)
		go sl.IterateTrie(block5.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s5, errC5)
		err, ok := <-errC5
		assert.True(t, ok)
		assert.Nil(t, err)
		// check state ledger in block 5
		sl5, _ := sl.NewViewWithoutCache(block5.Header, false)
		sl5.(*StateLedgerImpl).backend = s5
		sl5.(*StateLedgerImpl).refreshAccountTrie(block5.Header.StateRoot)
		verify, err := sl5.VerifyTrie(block5.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		assert.Equal(t, uint64(103), sl5.GetBalance(account1).Uint64())
		assert.Equal(t, uint64(203), sl5.GetBalance(account2).Uint64())
		assert.Equal(t, uint64(305), sl5.GetBalance(account3).Uint64())
		assert.Equal(t, uint64(15), sl5.GetNonce(account1))
		assert.Equal(t, uint64(25), sl5.GetNonce(account2))
		assert.Equal(t, uint64(35), sl5.GetNonce(account3))
		meta, err := sl5.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block5.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})

	t.Run("test iterate and verify trie error", func(t *testing.T) {
		// iterate trie of block 5
		block5 := &types.Block{
			Header: &types.BlockHeader{
				Number:    5,
				StateRoot: stateRoot5,
			},
		}
		s5 := initKVStorage(createMockRepo(t).RepoRoot)
		errC5 := make(chan error)
		sl.getEpochInfoFunc = func(epoch uint64) (*rbft.EpochInfo, error) {
			return nil, errors.Errorf("getEpochInfoFunc error test")
		}
		go sl.IterateTrie(block5.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s5, errC5)
		err, ok := <-errC5
		assert.True(t, ok)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "getEpochInfoFunc error test")
	})
}

func TestStateLedger_IterateStorageTrie(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddressByStr("dac17f958d2ee523a2206206994597c13d831ec7")
	account2 := types.NewAddressByStr("0000000000000000000000000000000000000007")
	account3 := types.NewAddressByStr("9000000000000000000000000000000000000000")

	// set storage account data in block 1
	// account1: k1=v1
	// account2: k1=v1
	// account3: k1=v1
	sl.blockHeight = 1
	sl.SetState(account1, []byte("k1"), []byte("v1"))
	sl.SetCode(account1, []byte("code1"))
	sl.SetState(account2, []byte("k1"), []byte("v1"))
	sl.SetState(account3, []byte("k1"), []byte("v1"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	isSuicide := sl.HasSuicide(account1)
	assert.Equal(t, isSuicide, false)
	assert.Equal(t, uint64(1), sl.Version())

	// set storage account data in block 2
	// account1: k1=v1, k2=v2
	// account2: k1=v1, k2=v22
	// account3: k1=v1
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v2"))
	sl.SetState(account2, []byte("k2"), []byte("v22"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(2), sl.Version())
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set storage account data in block 3
	// account1: k1=v11, k2=v2
	// account2: k1=v22, k2=v22
	// account3: k1=v1, k2=v22
	sl.blockHeight = 3
	sl.SetState(account1, []byte("k1"), []byte("v11"))
	sl.SetState(account2, []byte("k1"), []byte("v22"))
	sl.SetState(account3, []byte("k2"), []byte("v22"))
	sl.Finalise()
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), sl.Version())
	assert.NotEqual(t, stateRoot2, stateRoot3)

	// set storage account data in block 4 (same with block 3)
	// account1: k1=v11, k2=v2
	// account2: k1=v22, k2=v22
	// account3: k1=v1, k2=v22
	sl.blockHeight = 4
	sl.Finalise()
	stateRoot4, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(4), sl.Version())
	assert.Equal(t, stateRoot3, stateRoot4)

	// set storage account data in block 5
	// account1: k1=v11, k2=v22
	// account2: k1=v11, k2=v22
	// account3: k1=v11, k2=v22
	sl.blockHeight = 5
	sl.SetState(account1, []byte("k2"), []byte("v22"))
	sl.SetState(account2, []byte("k1"), []byte("v11"))
	sl.SetState(account3, []byte("k1"), []byte("v11"))
	sl.SetCode(account1, []byte("code2"))
	sl.Finalise()
	stateRoot5, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(5), sl.Version())
	assert.NotEqual(t, stateRoot4, stateRoot5)

	t.Run("test iterate and verify trie of block 1", func(t *testing.T) {
		// iterate trie of block 1
		block1 := &types.Block{
			Header: &types.BlockHeader{
				Number:    1,
				StateRoot: stateRoot1,
				Epoch:     1,
			},
		}
		s1 := initKVStorage(createMockRepo(t).RepoRoot)
		errC1 := make(chan error)
		go sl.IterateTrie(block1.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s1, errC1)
		err, ok := <-errC1
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 1
		sl1, _ := sl.NewViewWithoutCache(block1.Header, false)
		sl1.(*StateLedgerImpl).backend = s1
		sl1.(*StateLedgerImpl).refreshAccountTrie(block1.Header.StateRoot)
		verify, err := sl1.VerifyTrie(block1.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		exist, val := sl1.GetState(account1, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		val = sl1.GetCode(account1)
		assert.True(t, exist)
		assert.Equal(t, []byte("code1"), val)
		exist, val = sl1.GetState(account2, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		exist, val = sl1.GetState(account3, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		meta, err := sl1.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block1.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
		assert.Equal(t, block1.Header.Epoch, meta.EpochInfo.Epoch)
		assert.Equal(t, "P2PNodeID-1", meta.EpochInfo.ValidatorSet[0].P2PNodeID)
		assert.Equal(t, "P2PNodeID-1", meta.Nodes.Validators[0].PeerId)
	})

	t.Run("test iterate and verify trie of block 2", func(t *testing.T) {
		// iterate trie of block 2
		block2 := &types.Block{
			Header: &types.BlockHeader{
				Number:    2,
				StateRoot: stateRoot2,
			},
		}
		s2 := initKVStorage(createMockRepo(t).RepoRoot)
		errC2 := make(chan error)
		go sl.IterateTrie(block2.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s2, errC2)
		err, ok := <-errC2
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 2
		sl2, _ := sl.NewViewWithoutCache(block2.Header, false)
		sl2.(*StateLedgerImpl).backend = s2
		sl2.(*StateLedgerImpl).refreshAccountTrie(block2.Header.StateRoot)
		verify, err := sl2.VerifyTrie(block2.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		exist, val := sl2.GetState(account1, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		exist, val = sl2.GetState(account1, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v2"), val)
		exist, val = sl2.GetState(account2, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		exist, val = sl2.GetState(account2, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl2.GetState(account3, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		meta, err := sl2.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block2.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})

	t.Run("test iterate and verify trie of block 3", func(t *testing.T) {
		// iterate trie of block 3
		block3 := &types.Block{
			Header: &types.BlockHeader{
				Number:    3,
				StateRoot: stateRoot3,
			},
		}
		s3 := initKVStorage(createMockRepo(t).RepoRoot)
		errC3 := make(chan error)
		go sl.IterateTrie(block3.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s3, errC3)
		err, ok := <-errC3
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 3
		sl3, _ := sl.NewViewWithoutCache(block3.Header, false)
		sl3.(*StateLedgerImpl).backend = s3
		verify, err := sl3.VerifyTrie(block3.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		exist, val := sl3.GetState(account1, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v11"), val)
		exist, val = sl3.GetState(account1, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v2"), val)
		exist, val = sl3.GetState(account2, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl3.GetState(account2, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl3.GetState(account3, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		exist, val = sl3.GetState(account3, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		meta, err := sl3.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block3.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})

	t.Run("test iterate and verify trie of block 4", func(t *testing.T) {
		// iterate trie of block 4
		block4 := &types.Block{
			Header: &types.BlockHeader{
				Number:    4,
				StateRoot: stateRoot4,
			},
		}
		s4 := initKVStorage(createMockRepo(t).RepoRoot)
		errC4 := make(chan error)
		go sl.IterateTrie(block4.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s4, errC4)
		err, ok := <-errC4
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 4
		sl4, _ := sl.NewViewWithoutCache(block4.Header, false)
		sl4.(*StateLedgerImpl).backend = s4
		sl4.(*StateLedgerImpl).refreshAccountTrie(block4.Header.StateRoot)
		verify, err := sl4.VerifyTrie(block4.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		exist, val := sl4.GetState(account1, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v11"), val)
		exist, val = sl4.GetState(account1, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v2"), val)
		exist, val = sl4.GetState(account2, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl4.GetState(account2, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl4.GetState(account3, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v1"), val)
		exist, val = sl4.GetState(account3, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		meta, err := sl4.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block4.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})

	t.Run("test iterate and verify trie of block 5", func(t *testing.T) {
		// iterate trie of block 5
		block5 := &types.Block{
			Header: &types.BlockHeader{
				Number:    5,
				StateRoot: stateRoot5,
			},
		}
		s5 := initKVStorage(createMockRepo(t).RepoRoot)
		errC5 := make(chan error)
		go sl.IterateTrie(block5.Header, &consensus.QuorumValidators{Validators: []*consensus.QuorumValidator{{Id: 1, PeerId: "P2PNodeID-1"}}}, s5, errC5)
		err, ok := <-errC5
		assert.True(t, ok)
		assert.Nil(t, err)

		// check state ledger in block 5
		sl5, _ := sl.NewViewWithoutCache(block5.Header, false)
		sl5.(*StateLedgerImpl).backend = s5
		sl5.(*StateLedgerImpl).refreshAccountTrie(block5.Header.StateRoot)
		verify, err := sl5.VerifyTrie(block5.Header)
		assert.True(t, verify)
		assert.Nil(t, err)
		exist, val := sl5.GetState(account1, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v11"), val)
		exist, val = sl5.GetState(account1, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl5.GetState(account2, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v11"), val)
		exist, val = sl5.GetState(account2, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		exist, val = sl5.GetState(account3, []byte("k1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v11"), val)
		exist, val = sl5.GetState(account3, []byte("k2"))
		assert.True(t, exist)
		assert.Equal(t, []byte("v22"), val)
		val = sl5.GetCode(account1)
		assert.True(t, exist)
		assert.Equal(t, []byte("code2"), val)
		meta, err := sl5.(*StateLedgerImpl).GetTrieSnapshotMeta()
		assert.Nil(t, err)
		assert.Equal(t, block5.Header.StateRoot.String(), meta.BlockHeader.StateRoot.String())
	})
}

func TestStateLedger_GetTrieSnapshotMeta(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	meta, err := sl.GetTrieSnapshotMeta()
	require.Nil(t, meta)
	require.Equal(t, err, ErrNotFound)

	sl.backend.Put([]byte(utils.TrieBlockHeaderKey), []byte{1})
	meta, err = sl.GetTrieSnapshotMeta()
	require.Nil(t, meta)
	require.NotNil(t, err)

	sl.backend.Put([]byte(utils.TrieNodeInfoKey), []byte{1})
	meta, err = sl.GetTrieSnapshotMeta()
	require.Nil(t, meta)
	require.NotNil(t, err)

	blockHeader := &types.BlockHeader{
		Number: 1,
		Epoch:  1,
	}
	blockHeaderData, err := blockHeader.Marshal()
	require.Nil(t, err)
	require.NotNil(t, blockHeaderData)
	sl.backend.Put([]byte(utils.TrieBlockHeaderKey), blockHeaderData)
	meta, err = sl.GetTrieSnapshotMeta()
	require.Nil(t, meta)
	require.NotNil(t, err)

	epochInfo := &rbft.EpochInfo{
		Epoch: 1,
		ValidatorSet: []rbft.NodeInfo{
			{
				P2PNodeID: "P2PNodeID-1",
			},
		},
	}
	epochData, err := epochInfo.Marshal()
	require.Nil(t, err)
	require.NotNil(t, epochData)
	sl.backend.Put([]byte(utils.TrieNodeInfoKey), epochData)

	sl.backend.Put([]byte(utils.TrieNodeIdKey), []byte{1})
	meta, err = sl.GetTrieSnapshotMeta()
	require.Nil(t, meta)
	require.NotNil(t, err)

	nodesId := &consensus.QuorumValidators{
		Validators: []*consensus.QuorumValidator{
			{
				Id:     1,
				PeerId: "P2PNodeID-1",
			},
			{
				Id:     2,
				PeerId: "P2PNodeID-2",
			},
		},
	}

	nodesIdData, err := nodesId.MarshalVT()
	require.Nil(t, err)
	sl.backend.Put([]byte(utils.TrieNodeIdKey), nodesIdData)

	meta, err = sl.GetTrieSnapshotMeta()
	require.Nil(t, err)
	require.NotNil(t, meta)
	require.Equal(t, blockHeader.Epoch, meta.BlockHeader.Epoch)
	require.Equal(t, epochInfo.Epoch, meta.EpochInfo.Epoch)
	require.Equal(t, nodesId.Validators[0].Id, meta.Nodes.Validators[0].Id)
}

func TestStateLedger_GenerateSnapshotFromTrie(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddressByStr("dac17f958d2ee523a2206206994597c13d831ec7")
	account2 := types.NewAddressByStr("0000000000000000000000000000000000000007")
	account3 := types.NewAddressByStr("9000000000000000000000000000000000000000")

	// set storage account data in block 1
	// account1: k1=v1
	// account2: balance=1, nonce=2
	sl.blockHeight = 1
	sl.SetState(account1, []byte("k1"), []byte("v1"))
	sl.SetCode(account1, []byte("code1"))
	sl.SetBalance(account2, big.NewInt(1))
	sl.SetNonce(account2, 2)
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)

	// set storage account data in block 2
	// account1: k1=v1, k2=v2
	// account2: balance=1, nonce=2
	// account3: balance=3, nonce=4
	sl.blockHeight = 2
	sl.SetState(account1, []byte("k2"), []byte("v2"))
	sl.SetBalance(account3, big.NewInt(3))
	sl.SetNonce(account3, 4)
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.NotEqual(t, stateRoot1, stateRoot2)

	// set storage account data in block 3 (same with block 2)
	// account1: k1=v1, k2=v2
	// account2: balance=1, nonce=2
	// account3: balance=3, nonce=4
	sl.blockHeight = 3
	sl.Finalise()
	stateRoot3, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(3), sl.Version())
	assert.Equal(t, stateRoot2, stateRoot3)

	// set storage account data in block 4
	// account1: k1=v1, k2=v22
	// account2: balance=1, nonce=5
	// account3: balance=6, nonce=7
	sl.blockHeight = 4
	sl.SetState(account1, []byte("k2"), []byte("v22"))
	sl.SetNonce(account2, 5)
	sl.SetBalance(account3, big.NewInt(6))
	sl.SetNonce(account3, 7)
	sl.Finalise()
	stateRoot4, err := sl.Commit()
	assert.Nil(t, err)
	assert.Equal(t, uint64(4), sl.Version())
	assert.NotEqual(t, stateRoot3, stateRoot4)

	t.Run("test iterate and verify trie of block 1", func(t *testing.T) {
		// iterate trie of block 1
		block1 := &types.Block{
			Header: &types.BlockHeader{
				Number:    1,
				StateRoot: stateRoot1,
			},
		}
		snap := newSnapshot(createMockRepo(t), t.TempDir())
		sl.snapshot = snap
		errC1 := make(chan error)
		go sl.GenerateSnapshot(block1.Header, errC1)
		err, ok := <-errC1
		assert.True(t, ok)
		assert.Nil(t, err)

		// check snapshot in block 1
		val, err := snap.Storage(account1, []byte("k1"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v1"))
		acc, err := snap.Account(account2)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(2))
		assert.Equal(t, acc.Balance.Uint64(), uint64(1))
	})

	t.Run("test iterate and verify trie of block 2", func(t *testing.T) {
		// iterate trie of block 2
		block2 := &types.Block{
			Header: &types.BlockHeader{
				Number:    2,
				StateRoot: stateRoot2,
			},
		}
		snap := newSnapshot(createMockRepo(t), t.TempDir())
		sl.snapshot = snap
		errC := make(chan error)
		go sl.GenerateSnapshot(block2.Header, errC)
		err, ok := <-errC
		assert.True(t, ok)
		assert.Nil(t, err)

		// check snapshot in block 2
		val, err := snap.Storage(account1, []byte("k1"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v1"))
		val, err = snap.Storage(account1, []byte("k2"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v2"))
		acc, err := snap.Account(account2)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(2))
		assert.Equal(t, acc.Balance.Uint64(), uint64(1))
		acc, err = snap.Account(account3)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(4))
		assert.Equal(t, acc.Balance.Uint64(), uint64(3))
	})

	t.Run("test iterate and verify trie of block 3", func(t *testing.T) {
		// iterate trie of block 3
		block3 := &types.Block{
			Header: &types.BlockHeader{
				Number:    3,
				StateRoot: stateRoot3,
			},
		}
		snap := newSnapshot(createMockRepo(t), t.TempDir())
		sl.snapshot = snap
		errC := make(chan error)
		go sl.GenerateSnapshot(block3.Header, errC)
		err, ok := <-errC
		assert.True(t, ok)
		assert.Nil(t, err)

		// check snapshot in block 3
		val, err := snap.Storage(account1, []byte("k1"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v1"))
		val, err = snap.Storage(account1, []byte("k2"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v2"))
		acc, err := snap.Account(account2)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(2))
		assert.Equal(t, acc.Balance.Uint64(), uint64(1))
		acc, err = snap.Account(account3)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(4))
		assert.Equal(t, acc.Balance.Uint64(), uint64(3))
	})

	t.Run("test iterate and verify trie of block 4", func(t *testing.T) {
		// iterate trie of block 4
		block4 := &types.Block{
			Header: &types.BlockHeader{
				Number:    4,
				StateRoot: stateRoot4,
			},
		}
		snap := newSnapshot(createMockRepo(t), t.TempDir())
		sl.snapshot = snap
		errC := make(chan error)
		go sl.GenerateSnapshot(block4.Header, errC)
		err, ok := <-errC
		assert.True(t, ok)
		assert.Nil(t, err)

		// check snapshot in block 4
		val, err := snap.Storage(account1, []byte("k1"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v1"))
		val, err = snap.Storage(account1, []byte("k2"))
		assert.Nil(t, err)
		assert.Equal(t, val, []byte("v22"))
		acc, err := snap.Account(account2)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(5))
		assert.Equal(t, acc.Balance.Uint64(), uint64(1))
		acc, err = snap.Account(account3)
		assert.Nil(t, err)
		assert.Equal(t, acc.Nonce, uint64(7))
		assert.Equal(t, acc.Balance.Uint64(), uint64(6))
	})
}

// =========================== Test Trie Proof ===========================

func TestStateLedger_TrieProof(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))
	account3 := types.NewAddress(LeftPadBytes([]byte{103}, 20))

	// set account data in block 1
	// account1: balance=101, nonce=0
	// account2: balance=201, nonce=0
	// account3: key1=val1, code=code1
	sl.blockHeight = 1
	sl.SetBalance(account1, new(big.Int).SetInt64(101))
	sl.SetBalance(account2, new(big.Int).SetInt64(201))
	sl.SetCode(account3, []byte("code1"))
	sl.SetState(account3, []byte("key1"), []byte("val1"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), sl.Version())

	// set account data in block 2
	// account1: balance=102, nonce=12
	// account2: balance=201, nonce=22
	// account3: key1=val2, code=code1
	sl.blockHeight = 2
	sl.SetBalance(account1, new(big.Int).SetInt64(102))
	sl.SetBalance(account3, new(big.Int).SetInt64(302))
	sl.SetNonce(account1, 12)
	sl.SetNonce(account2, 22)
	sl.SetState(account3, []byte("key1"), []byte("val2"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.NotEqual(t, stateRoot1, stateRoot2)

	t.Run("prove block1", func(t *testing.T) {
		// check state ledger in block 1
		block1 := &types.Block{
			Header: &types.BlockHeader{
				Number:    1,
				StateRoot: stateRoot1,
			},
		}
		lg1, _ := sl.NewViewWithoutCache(block1.Header, false)
		acc3 := lg1.GetOrCreateAccount(account3)
		assert.NotNil(t, acc3)
		assert.NotEqual(t, "", acc3.String())
		// prove account1 in block1
		account1Proof, err := lg1.Prove(stateRoot1.ETHHash(), utils.CompositeAccountKey(account1))
		assert.Nil(t, err)
		verify, err := jmt.VerifyProof(stateRoot1.ETHHash(), account1Proof)
		assert.Nil(t, err)
		assert.True(t, verify)
		verify, err = jmt.VerifyProof(stateRoot2.ETHHash(), account1Proof)
		assert.Nil(t, err)
		assert.False(t, verify)
		// prove account3 in block1
		account3Proof, err := lg1.Prove(stateRoot1.ETHHash(), utils.CompositeAccountKey(account3))
		assert.Nil(t, err)
		verify, err = jmt.VerifyProof(stateRoot1.ETHHash(), account3Proof)
		assert.Nil(t, err)
		assert.True(t, verify)
		// prove account3's storage in block1
		account3StorageProof, err := lg1.Prove(acc3.GetStorageRootHash(), utils.CompositeStorageKey(account3, []byte("key1")))
		assert.Nil(t, err)
		assert.True(t, bytes.Equal(utils.CompositeStorageKey(account3, []byte("key1")), account3StorageProof.Key))
		assert.Equal(t, []byte("val1"), account3StorageProof.Value)
		verify, err = jmt.VerifyProof(acc3.GetStorageRootHash(), account3StorageProof)
		assert.Nil(t, err)
		assert.True(t, verify)
		verify, err = jmt.VerifyProof(stateRoot1.ETHHash(), account3StorageProof)
		assert.Nil(t, err)
		assert.False(t, verify)
	})

	t.Run("prove block2", func(t *testing.T) {
		// check state ledger in block 2
		block2 := &types.Block{
			Header: &types.BlockHeader{
				Number:    2,
				StateRoot: stateRoot2,
			},
		}
		lg2, _ := sl.NewViewWithoutCache(block2.Header, false)
		acc3 := lg2.GetOrCreateAccount(account3)
		assert.NotNil(t, acc3)
		// prove account1 in block2
		account1Proof, err := lg2.Prove(stateRoot2.ETHHash(), utils.CompositeAccountKey(account1))
		assert.Nil(t, err)
		verify, err := jmt.VerifyProof(stateRoot2.ETHHash(), account1Proof)
		assert.Nil(t, err)
		assert.True(t, verify)
		// prove account3 in block2
		account3Proof, err := lg2.Prove(stateRoot2.ETHHash(), utils.CompositeAccountKey(account3))
		assert.Nil(t, err)
		verify, err = jmt.VerifyProof(stateRoot2.ETHHash(), account3Proof)
		assert.Nil(t, err)
		assert.True(t, verify)
		// prove account3's storage in block2
		account3StorageProof, err := lg2.Prove(acc3.GetStorageRootHash(), utils.CompositeStorageKey(account3, []byte("key1")))
		assert.Nil(t, err)
		assert.True(t, bytes.Equal(utils.CompositeStorageKey(account3, []byte("key1")), account3StorageProof.Key))
		assert.Equal(t, []byte("val2"), account3StorageProof.Value)
		verify, err = jmt.VerifyProof(acc3.GetStorageRootHash(), account3StorageProof)
		assert.Nil(t, err)
		assert.True(t, verify)
		verify, err = jmt.VerifyProof(stateRoot2.ETHHash(), account3StorageProof)
		assert.Nil(t, err)
		assert.False(t, verify)
	})
}

func TestStateLedger_RPCGetProof(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	account1 := types.NewAddress(LeftPadBytes([]byte{101}, 20))
	account2 := types.NewAddress(LeftPadBytes([]byte{102}, 20))
	account3 := types.NewAddress(LeftPadBytes([]byte{103}, 20))

	// set EOA account data in block 1
	// account1: balance=101, nonce=0
	// account2: balance=201, nonce=0
	// account3: key1=val1, code=code1
	sl.blockHeight = 1
	sl.SetBalance(account1, new(big.Int).SetInt64(101))
	sl.SetNonce(account1, 0)
	sl.SetBalance(account2, new(big.Int).SetInt64(201))
	sl.SetCode(account3, []byte("code1"))
	rawKey1, err := hexutil.DecodeHash("0x1")
	assert.Nil(t, err)
	key1 := crypto1.Keccak256Hash(rawKey1[:])
	sl.SetState(account3, key1[:], []byte("val1"))
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), sl.Version())

	// set EOA account data in block 2
	// account1: balance=102, nonce=12
	// account2: balance=201, nonce=22
	// account3: key1=val2, code=code1
	sl.blockHeight = 2
	sl.SetBalance(account1, new(big.Int).SetInt64(102))
	sl.SetBalance(account3, new(big.Int).SetInt64(302))
	sl.SetNonce(account1, 12)
	sl.SetNonce(account2, 22)
	sl.SetState(account3, key1[:], []byte("val2"))
	sl.Finalise()
	stateRoot2, err := sl.Commit()
	assert.Nil(t, err)
	assert.NotEqual(t, stateRoot1, stateRoot2)

	t.Run("prove block1", func(t *testing.T) {
		block1 := &types.Block{
			Header: &types.BlockHeader{
				Number:    1,
				StateRoot: stateRoot1,
			},
		}
		lg1, _ := sl.NewViewWithoutCache(block1.Header, false)
		acc3 := lg1.GetOrCreateAccount(account3)
		assert.NotNil(t, acc3)

		// prove account1 (EOA) in block1, check validity of account1 GetProof result
		account1Proof, err := lg1.Prove(stateRoot1.ETHHash(), utils.CompositeAccountKey(account1))
		assert.Nil(t, err)
		account1RpcProof, err := mockGetProof(account1.ETHAddress(), nil, lg1)
		assert.True(t, equalProof(account1Proof, convertAccountProof(account1RpcProof)))
		verify, err := jmt.VerifyProof(stateRoot1.ETHHash(), account1Proof)
		assert.Nil(t, err)
		assert.True(t, verify)

		// prove account3 (contract) in block1, check validity of account3 GetProof result
		account3Proof, err := lg1.Prove(stateRoot1.ETHHash(), utils.CompositeAccountKey(account3))
		assert.Nil(t, err)
		account3RpcProof, err := mockGetProof(account3.ETHAddress(), nil, lg1)
		assert.True(t, equalProof(account3Proof, convertAccountProof(account3RpcProof)))
		verify, err = jmt.VerifyProof(stateRoot1.ETHHash(), convertAccountProof(account3RpcProof))
		assert.Nil(t, err)
		assert.True(t, verify)

		// prove account3's storage in block1
		account3StorageProof, err := lg1.Prove(acc3.GetStorageRoot(), utils.CompositeStorageKey(account3, key1[:]))
		assert.Nil(t, err)
		account3Key1RpcProof, err := mockGetProof(account3.ETHAddress(), []string{"0x1"}, lg1)
		assert.Nil(t, err)
		assert.True(t, equalProof(account3StorageProof, convertStorageProof(account3Key1RpcProof.Address.Hash(), &account3Key1RpcProof.StorageProof[0])))
		verify, err = jmt.VerifyProof(acc3.GetStorageRoot(), convertStorageProof(account3Key1RpcProof.Address.Hash(), &account3Key1RpcProof.StorageProof[0]))
		assert.Nil(t, err)
		assert.True(t, verify)
	})

	t.Run("prove block2", func(t *testing.T) {
		block2 := &types.Block{
			Header: &types.BlockHeader{
				Number:    2,
				StateRoot: stateRoot2,
			},
		}
		lg2, _ := sl.NewViewWithoutCache(block2.Header, false)
		acc3 := lg2.GetOrCreateAccount(account3)
		assert.NotNil(t, acc3)

		// prove account1 (EOA) in block2, check validity of account1 GetProof result
		account1Proof, err := lg2.Prove(stateRoot2.ETHHash(), utils.CompositeAccountKey(account1))
		assert.Nil(t, err)
		account1RpcProof, err := mockGetProof(account1.ETHAddress(), nil, lg2)
		assert.True(t, equalProof(account1Proof, convertAccountProof(account1RpcProof)))
		verify, err := jmt.VerifyProof(stateRoot2.ETHHash(), account1Proof)
		assert.Nil(t, err)
		assert.True(t, verify)
		// tamper with proof
		account1RpcProof.AccountProof = account1RpcProof.AccountProof[1:]
		verify, err = jmt.VerifyProof(stateRoot2.ETHHash(), convertAccountProof(account1RpcProof))
		assert.Nil(t, err)
		assert.False(t, verify)

		// prove account3 (contract) in block2, check validity of account3 GetProof result
		account3Proof, err := lg2.Prove(stateRoot2.ETHHash(), utils.CompositeAccountKey(account3))
		assert.Nil(t, err)
		account3RpcProof, err := mockGetProof(account3.ETHAddress(), nil, lg2)
		assert.True(t, equalProof(account3Proof, convertAccountProof(account3RpcProof)))
		verify, err = jmt.VerifyProof(stateRoot2.ETHHash(), convertAccountProof(account3RpcProof))
		assert.Nil(t, err)
		assert.True(t, verify)
		// tamper with proof
		account3RpcProof.AccountProof[1] = "bad proof"
		verify, err = jmt.VerifyProof(stateRoot2.ETHHash(), convertAccountProof(account3RpcProof))
		assert.NotNil(t, err)
		assert.False(t, verify)

		// prove account3's storage in block2
		account3StorageProof, err := lg2.Prove(acc3.GetStorageRoot(), utils.CompositeStorageKey(account3, key1[:]))
		assert.Nil(t, err)
		account3Key1RpcProof, err := mockGetProof(account3.ETHAddress(), []string{"0x1"}, lg2)
		assert.Nil(t, err)
		assert.True(t, equalProof(account3StorageProof, convertStorageProof(account3Key1RpcProof.Address.Hash(), &account3Key1RpcProof.StorageProof[0])))
		verify, err = jmt.VerifyProof(acc3.GetStorageRoot(), convertStorageProof(account3Key1RpcProof.Address.Hash(), &account3Key1RpcProof.StorageProof[0]))
		assert.Nil(t, err)
		assert.True(t, verify)
	})
}

func TestLedger_NewView(t *testing.T) {
	lg, _ := initLedger(t, "", "pebble")
	sl := lg.StateLedger.(*StateLedgerImpl)

	// create an account
	contractAccount := types.NewAddress(LeftPadBytes([]byte{102}, 20))

	// set account data in block 1
	sl.blockHeight = 1
	code1 := sha256.Sum256([]byte("code1"))
	sl.SetState(contractAccount, []byte("key1"), []byte("val1"))
	sl.SetCode(contractAccount, code1[:])
	sl.Finalise()
	stateRoot1, err := sl.Commit()
	assert.NotNil(t, stateRoot1)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), sl.Version())

	t.Run("can't find block", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				assert.NotNil(t, r)
				assert.Contains(t, fmt.Sprintf("%v", r), "out of bounds")
			}
		}()
		lg.NewView().StateLedger.NewViewWithoutCache(nil, false)
	})

	t.Run("new view ledger success", func(t *testing.T) {
		// check state ledger in block 1
		block1 := &types.Block{
			Header: &types.BlockHeader{
				Number:    1,
				StateRoot: stateRoot1,
			},
		}
		err = lg.ChainLedger.PersistExecutionResult(block1, []*types.Receipt{})
		assert.Nil(t, err)
		lg.ChainLedger.UpdateChainMeta(&types.ChainMeta{Height: 1})
		lg1, _ := lg.NewView().StateLedger.NewViewWithoutCache(block1.Header, false)
		exist, a2k1 := lg1.GetState(contractAccount, []byte("key1"))
		assert.True(t, exist)
		assert.Equal(t, []byte("val1"), a2k1)
		assert.Equal(t, code1[:], lg1.GetCode(contractAccount))
	})
}

type mockAccountResult struct {
	Address      common.Address      `json:"address"`
	AccountProof []string            `json:"accountProof"`
	Balance      *ethhexutil.Big     `json:"balance"`
	CodeHash     common.Hash         `json:"codeHash"`
	Nonce        ethhexutil.Uint64   `json:"nonce"`
	StorageHash  common.Hash         `json:"storageHash"`
	StorageProof []mockStorageResult `json:"storageProof"`
}

type mockStorageResult struct {
	Key   string          `json:"key"`
	Value *ethhexutil.Big `json:"value"`
	Proof []string        `json:"proof"`
}

func convertAccountProof(proof *mockAccountResult) *jmt.ProofResult {
	res := &jmt.ProofResult{}
	res.Key = utils.CompositeAccountKey(types.NewAddress(proof.Address.Bytes()))
	acc := &types.InnerAccount{
		Nonce:       uint64(proof.Nonce),
		Balance:     (proof.Balance).ToInt(),
		CodeHash:    proof.CodeHash[:],
		StorageRoot: proof.StorageHash,
	}
	if proof.CodeHash == (common.Hash{}) {
		acc.CodeHash = nil
	}
	blob, err := acc.Marshal()
	if err != nil {
		return nil
	}

	res.Value = blob
	for _, item := range proof.AccountProof {
		p, err := base64.StdEncoding.DecodeString(item)
		if err != nil {
			return nil
		}
		res.Proof = append(res.Proof, p)
	}
	return res
}

func convertStorageProof(acc common.Hash, proof *mockStorageResult) *jmt.ProofResult {
	res := &jmt.ProofResult{}
	res.Key = utils.CompositeStorageKey(types.NewAddress(acc.Bytes()), crypto1.Keccak256(hexutil.Decode(proof.Key)))
	res.Value = proof.Value.ToInt().Bytes()
	for _, item := range proof.Proof {
		p, err := base64.StdEncoding.DecodeString(item)
		if err != nil {
			return nil
		}
		res.Proof = append(res.Proof, p)
	}
	return res
}

func equalProof(proof1 *jmt.ProofResult, proof2 *jmt.ProofResult) bool {
	if !slices.Equal(proof1.Key, proof2.Key) || !slices.Equal(proof1.Value, proof2.Value) {
		return false
	}
	if len(proof1.Proof) != len(proof2.Proof) {
		return false
	}
	for i := range proof1.Proof {
		if !slices.Equal(proof1.Proof[i], proof2.Proof[i]) {
			return false
		}
	}
	return true
}

func mockGetProof(address common.Address, storageKeys []string, stateLedger StateLedger) (ret *mockAccountResult, err error) {
	addr := types.NewAddress(address.Bytes())

	// construct account proof
	acc := stateLedger.GetOrCreateAccount(addr)
	rawAccountProof, err := stateLedger.Prove(common.Hash{}, utils.CompositeAccountKey(addr))
	if err != nil {
		return nil, err
	}

	var accountProof []string
	for _, proof := range rawAccountProof.Proof {
		accountProof = append(accountProof, base64.StdEncoding.EncodeToString(proof))
	}
	ret = &mockAccountResult{
		Address:      address,
		Nonce:        (ethhexutil.Uint64)(acc.GetNonce()),
		Balance:      (*ethhexutil.Big)(acc.GetBalance()),
		CodeHash:     common.BytesToHash(acc.CodeHash()),
		StorageHash:  acc.GetStorageRoot(),
		AccountProof: accountProof,
	}

	// construct storage proof
	keys := make([]common.Hash, len(storageKeys))
	for i, hexKey := range storageKeys {
		var err error
		keys[i], err = hexutil.DecodeHash(hexKey)
		if err != nil {
			return nil, err
		}
	}
	for _, key := range keys {
		hashKey := crypto1.Keccak256(key.Bytes())
		rawStorageProof, err := stateLedger.Prove(acc.GetStorageRoot(), utils.CompositeStorageKey(addr, hashKey))
		if err != nil {
			return nil, err
		}
		var storageProof []string
		for _, proof := range rawStorageProof.Proof {
			storageProof = append(storageProof, base64.StdEncoding.EncodeToString(proof))
		}
		evmStateDB := &EvmStateDBAdaptor{StateLedger: stateLedger}
		storageResult := mockStorageResult{
			Key:   hexutil.Encode(key[:]),
			Value: (*ethhexutil.Big)(evmStateDB.GetState(addr.ETHAddress(), common.BytesToHash(hashKey)).Big()),
			Proof: storageProof,
		}
		ret.StorageProof = append(ret.StorageProof, storageResult)
	}

	return ret, nil
}

func genBlockData(height uint64, stateRoot *types.Hash) *BlockData {
	return &BlockData{
		Block: &types.Block{
			Header: &types.BlockHeader{
				Number:    height,
				StateRoot: stateRoot,
			},
			Transactions: []*types.Transaction{},
		},
		Receipts: nil,
	}
}

func createMockRepo(t *testing.T) *repo.Repo {
	r, err := repo.Default(t.TempDir())
	require.Nil(t, err)
	return r
}

func initLedger(t *testing.T, repoRoot string, kv string) (*Ledger, string) {
	rep := createMockRepo(t)
	if repoRoot != "" {
		rep.RepoRoot = repoRoot
	}

	err := storagemgr.Initialize(kv, repo.KVStorageCacheSize, repo.KVStorageSync, false)
	require.Nil(t, err)
	rep.Config.Monitor.EnableExpensive = true
	l, err := NewLedger(rep)
	require.Nil(t, err)
	l.WithGetEpochInfoFunc(func(lg StateLedger, epoch uint64) (*rbft.EpochInfo, error) {
		return &rbft.EpochInfo{
			Epoch: epoch,
			ValidatorSet: []rbft.NodeInfo{
				{
					P2PNodeID: "P2PNodeID-1",
				},
			},
		}, nil
	})

	return l, rep.RepoRoot
}

// LeftPadBytes zero-pads slice to the left up to length l.
func LeftPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l)
	copy(padded[l-len(slice):], slice)

	return padded
}

func RightPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l)
	copy(padded, slice)

	return padded
}

func BenchmarkStateLedgerWrite(b *testing.B) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		b.Run(name, func(b *testing.B) {
			r, _ := repo.Default(b.TempDir())
			storagemgr.Initialize(tc.kvType, 256, repo.KVStorageSync, false)
			l, _ := NewLedger(r)
			benchStateLedgerWrite(b, l.StateLedger)
		})
	}
}

func BenchmarkStateLedgerRead(b *testing.B) {
	testcase := map[string]struct {
		kvType string
	}{
		"leveldb": {kvType: "leveldb"},
		"pebble":  {kvType: "pebble"},
	}

	for name, tc := range testcase {
		b.Run(name, func(b *testing.B) {
			r, _ := repo.Default(b.TempDir())
			storagemgr.Initialize(tc.kvType, 256, repo.KVStorageSync, false)
			l, _ := NewLedger(r)
			benchStateLedgerRead(b, l.StateLedger)
		})
	}
}

func benchStateLedgerWrite(b *testing.B, sl StateLedger) {
	var (
		keys, vals = makeDataset(5_000_000, 32, 32, false)
	)

	b.Run("Write", func(b *testing.B) {
		stateLedger := sl.(*StateLedgerImpl)
		addr := types.NewAddress(LeftPadBytes([]byte{1}, 20))
		for i := 0; i < len(keys); i++ {
			stateLedger.SetState(addr, keys[i], vals[i])
		}
		b.ResetTimer()
		b.ReportAllocs()
		stateLedger.blockHeight = 1
		stateLedger.Finalise()
		stateLedger.Commit()
	})
}

func benchStateLedgerRead(b *testing.B, sl StateLedger) {
	var (
		keys, vals = makeDataset(10_000_000, 32, 32, false)
	)
	stateLedger := sl.(*StateLedgerImpl)
	addr := types.NewAddress(LeftPadBytes([]byte{1}, 20))
	for i := 0; i < len(keys); i++ {
		stateLedger.SetState(addr, keys[i], vals[i])
	}
	stateLedger.blockHeight = 1
	stateLedger.Finalise()
	stateLedger.Commit()

	b.Run("Read", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < len(keys); i++ {
			stateLedger.GetState(addr, keys[i])
		}
	})
}

func makeDataset(size, ksize, vsize int, order bool) ([][]byte, [][]byte) {
	var keys [][]byte
	var vals [][]byte
	for i := 0; i < size; i++ {
		keys = append(keys, randBytes(ksize))
		vals = append(vals, randBytes(vsize))
	}

	// order generated slice according to bytes order
	if order {
		sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i], keys[j]) < 0 })
	}
	return keys, vals
}

// randomHash generates a random blob of data and returns it as a hash.
func randBytes(len int) []byte {
	buf := make([]byte, len)
	if n, err := rand.Read(buf); n != len || err != nil {
		panic(err)
	}
	return buf
}

func initKVStorage(path string) storage.Storage {
	dir, _ := os.MkdirTemp(path, "")
	s, _ := pebble.New(dir, nil, nil, logrus.New())
	return s
}

func newSnapshot(rep *repo.Repo, path string) *snapshot.Snapshot {
	dir, _ := os.MkdirTemp(path, "")
	s, _ := pebble.New(dir, nil, nil, logrus.New())
	return snapshot.NewSnapshot(rep, s, log.NewWithModule("snapshot_test"))
}

// TestStateLedger_RevertStorageAfterFinalise exercises two transactions in the
// same block. Finalise must keep cached accounts attached to the active journal,
// so a failed second transaction restores the first transaction's storage.
func TestStateLedger_RevertStorageAfterFinalise(t *testing.T) {
	for _, kvType := range []string{"leveldb", "pebble"} {
		t.Run(kvType, func(t *testing.T) {
			lg, _ := initLedger(t, "", kvType)
			t.Cleanup(lg.Close)
			sl := lg.StateLedger.(*StateLedgerImpl)
			sl.blockHeight = 1

			pair := types.NewAddressByStr("0x3a1092655b3a9436253c813bcf02e445e3340333")
			lockSlot := common.HexToHash("0x0c").Bytes()
			unlocked := common.HexToHash("0x01").Bytes()
			locked := common.Hash{}.Bytes()

			// Transaction 1 succeeds. Keep the account cached in this block;
			// committing a block here would clear it and hide the bug.
			sl.SetNonce(pair, 1) // A deployed contract is not an empty account.
			sl.SetState(pair, lockSlot, unlocked)
			sl.Finalise()
			ok, value := sl.GetState(pair, lockSlot)
			require.True(t, ok)
			require.Equal(t, unlocked, value)

			// Transaction 2 takes the lock, then fails. This is the same
			// Snapshot/RevertToSnapshot boundary used by the EVM on failure.
			snapshot := sl.Snapshot()
			sl.SetState(pair, lockSlot, locked)
			ok, value = sl.GetState(pair, lockSlot)
			require.True(t, ok)
			require.Equal(t, locked, value)
			sl.RevertToSnapshot(snapshot)

			ok, value = sl.GetState(pair, lockSlot)
			assert.True(t, ok)
			assert.Equal(t, unlocked, value, "failed transaction must restore the lock after a previous Finalise")

			// A failed transaction is still finalised by the executor. Its
			// intermediate lock must not enter the block's pending state.
			sl.Finalise()
			assert.Equal(t, unlocked, sl.GetCommittedState(pair, lockSlot), "Finalise must not retain the failed transaction's lock")
		})
	}
}
