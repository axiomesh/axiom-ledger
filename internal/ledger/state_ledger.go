package ledger

import (
	"errors"
	"fmt"
	"math/big"
	"path"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sirupsen/logrus"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/storage"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/prune"
	"github.com/axiomesh/axiom-ledger/internal/ledger/snapshot"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var _ StateLedger = (*StateLedgerImpl)(nil)

var (
	ErrorRollbackToHigherNumber = errors.New("rollback to higher blockchain height")
)

// maxBatchSize defines the maximum size of the data in single batch write operation, which is 64 MB.
const maxBatchSize = 64 * 1024 * 1024

type revision struct {
	id           int
	changerIndex int
}

type StateLedgerImpl struct {
	logger       logrus.FieldLogger
	accountCache *AccountCache // todo remove this
	accountTrie  *jmt.JMT      // keep track of the latest world state (dirty or committed)

	pruneCache       *prune.PruneCache
	backend          storage.Storage
	accountTrieCache *storagemgr.CacheWrapper
	storageTrieCache *storagemgr.CacheWrapper

	triePreloader *triePreloader
	accounts      map[string]IAccount
	repo          *repo.Repo
	blockHeight   uint64
	thash         *types.Hash
	txIndex       int

	validRevisions []revision
	nextRevisionId int
	changer        *stateChanger

	accessList *AccessList
	preimages  map[types.Hash][]byte
	refund     uint64
	logs       *evmLogs

	snapshot *snapshot.Snapshot

	transientStorage transientStorage

	// enableExpensiveMetric determines if costly metrics gathering is allowed or not.
	// The goal is to separate standard metrics for health monitoring and debug metrics that might impact runtime performance.
	enableExpensiveMetric bool

	getEpochInfoFunc func(epoch uint64) (*rbft.EpochInfo, error)
}

type SnapshotMeta struct {
	BlockHeader *types.BlockHeader
	EpochInfo   *rbft.EpochInfo
	Nodes       *consensus.QuorumValidators
}

// NewView get a view at specific block. We can enable snapshot if and only if the block were the latest block.
func (l *StateLedgerImpl) NewView(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	l.logger.Debugf("[NewView] height: %v, stateRoot: %v", blockHeader.Number, blockHeader.StateRoot)
	if l.repo.Config.Ledger.EnablePrune {
		min, max := l.GetHistoryRange()
		// Skip the range check when the prune journal is empty (e.g. after
		// a manual wipe, or on first startup with EnablePrune=true). An
		// empty journal means "nothing has been pruned yet" and must not
		// block access to any historical block height. Only the lower
		// bound matters once pruning has actually removed state.
		if min != 0 || max != 0 {
			if blockHeader.Number < min {
				return nil, fmt.Errorf("history at target block %v is invalid, the valid range is from %v to %v", blockHeader.Number, min, max)
			}
		}
	}

	lg := &StateLedgerImpl{
		repo:                  l.repo,
		logger:                l.logger,
		backend:               l.backend,
		pruneCache:            l.pruneCache,
		accountTrieCache:      l.accountTrieCache,
		storageTrieCache:      l.storageTrieCache,
		accountCache:          l.accountCache,
		accounts:              make(map[string]IAccount),
		preimages:             make(map[types.Hash][]byte),
		changer:               NewChanger(),
		accessList:            NewAccessList(),
		logs:                  newEvmLogs(),
		enableExpensiveMetric: l.enableExpensiveMetric,
	}
	if enableSnapshot {
		lg.snapshot = l.snapshot
	}
	lg.refreshAccountTrie(blockHeader.StateRoot)
	return lg, nil
}

// NewViewWithoutCache get a view ledger at specific block. We can enable snapshot if and only if the block were the latest block.
func (l *StateLedgerImpl) NewViewWithoutCache(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	l.logger.Debugf("[NewViewWithoutCache] height: %v, stateRoot: %v", blockHeader.Number, blockHeader.StateRoot)
	if l.repo.Config.Ledger.EnablePrune {
		min, max := l.GetHistoryRange()
		// Skip the range check when the prune journal is empty; see
		// NewView for the full rationale.
		if min != 0 || max != 0 {
			if blockHeader.Number < min {
				return nil, fmt.Errorf("history at target block %v is invalid, the valid range is from %v to %v", blockHeader.Number, min, max)
			}
		}
	}

	ac, _ := NewAccountCache(0, true)
	lg := &StateLedgerImpl{
		repo:                  l.repo,
		logger:                l.logger,
		backend:               l.backend,
		pruneCache:            l.pruneCache,
		accountTrieCache:      l.accountTrieCache,
		storageTrieCache:      l.storageTrieCache,
		accountCache:          ac,
		accounts:              make(map[string]IAccount),
		preimages:             make(map[types.Hash][]byte),
		changer:               NewChanger(),
		accessList:            NewAccessList(),
		logs:                  newEvmLogs(),
		enableExpensiveMetric: l.enableExpensiveMetric,
	}
	if enableSnapshot {
		lg.snapshot = l.snapshot
	}
	lg.refreshAccountTrie(blockHeader.StateRoot)
	return lg, nil
}

func (l *StateLedgerImpl) GetHistoryRange() (uint64, uint64) {
	minHeight := uint64(0)
	maxHeight := uint64(0)

	data := l.backend.Get(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr))
	if data != nil {
		minHeight = utils.UnmarshalHeight(data)
	}

	data = l.backend.Get(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr))
	if data != nil {
		maxHeight = utils.UnmarshalHeight(data)
	}

	return minHeight, maxHeight
}

func (l *StateLedgerImpl) WithGetEpochInfoFunc(f func(lg StateLedger, epoch uint64) (*rbft.EpochInfo, error)) {
	l.getEpochInfoFunc = func(epoch uint64) (*rbft.EpochInfo, error) {
		return f(l, epoch)
	}
}

func (l *StateLedgerImpl) Finalise() {
	for _, account := range l.accounts {
		keys := account.Finalise()

		if l.triePreloader != nil {
			l.triePreloader.preload(common.Hash{}, [][]byte{utils.CompositeAccountKey(account.GetAddress())})
			if len(keys) > 0 {
				l.triePreloader.preload(account.GetStorageRootHash(), keys)
			}
		}
	}

	l.ClearChangerAndRefund()
}

// todo make arguments configurable
func (l *StateLedgerImpl) IterateTrie(blockHeader *types.BlockHeader, nodesId *consensus.QuorumValidators, kv storage.Storage, errC chan error) {
	stateRoot := blockHeader.StateRoot.ETHHash()
	l.logger.Infof("[IterateTrie] blockhash: %v, rootHash: %v", blockHeader.Hash(), stateRoot)

	queue := []common.Hash{stateRoot}
	batch := kv.NewBatch()
	for len(queue) > 0 {
		trieRoot := queue[0]
		iter := jmt.NewIterator(trieRoot, l.backend, l.pruneCache, 100, time.Second)
		l.logger.Debugf("[IterateTrie] trie root=%v", trieRoot)
		go iter.Iterate()

		for {
			node, err := iter.Next()
			if err != nil {
				if err == jmt.ErrorNoMoreData {
					break
				} else {
					errC <- err
					return
				}
			}
			batch.Put(node.RawKey, node.RawValue)
			// data size exceed threshold, flush to disk
			if batch.Size() > maxBatchSize {
				batch.Commit()
				batch.Reset()
				l.logger.Infof("[IterateTrie] write batch periodically")
			}
			if trieRoot == stateRoot && len(node.LeafValue) > 0 {
				// resolve potential contract account
				acc := &types.InnerAccount{Balance: big.NewInt(0)}
				if err := acc.Unmarshal(node.LeafValue); err != nil {
					panic(err)
				}
				if acc.StorageRoot != (common.Hash{}) {
					// set contract code
					codeKey := utils.CompositeCodeKey(types.NewAddress(types.HexToBytes(node.LeafKey)), acc.CodeHash)
					batch.Put(codeKey, l.backend.Get(codeKey))
					// prepare storage trie root
					queue = append(queue, acc.StorageRoot)
				}
			}
		}
		queue = queue[1:]
		batch.Put(trieRoot[:], l.backend.Get(trieRoot[:]))
	}

	blockData, err := blockHeader.Marshal()
	if err != nil {
		errC <- err
		return
	}
	batch.Put([]byte(utils.TrieBlockHeaderKey), blockData)

	epochInfo, err := l.getEpochInfoFunc(blockHeader.Epoch)
	if err != nil {
		l.logger.Errorf("l.getEpochInfoFunc error:%v\n", err.Error())
		errC <- err
	}
	blob, err := epochInfo.Marshal()
	if err != nil {
		errC <- err
		return
	}
	batch.Put([]byte(utils.TrieNodeInfoKey), blob)

	nodesIdBytes, err := nodesId.MarshalVT()
	if err != nil {
		errC <- err
		return
	}
	batch.Put([]byte(utils.TrieNodeIdKey), nodesIdBytes)

	batch.Commit()
	l.logger.Infof("[IterateTrie] iterate trie successfully")

	errC <- nil
}

func (l *StateLedgerImpl) GetTrieSnapshotMeta() (*SnapshotMeta, error) {
	rawBlock := l.backend.Get([]byte(utils.TrieBlockHeaderKey))
	rawEpochInfo := l.backend.Get([]byte(utils.TrieNodeInfoKey))
	rawNodes := l.backend.Get([]byte(utils.TrieNodeIdKey))
	if len(rawBlock) == 0 || len(rawEpochInfo) == 0 || len(rawNodes) == 0 {
		return nil, ErrNotFound
	}
	blockHeader := &types.BlockHeader{}
	err := blockHeader.Unmarshal(rawBlock)
	if err != nil {
		return nil, err
	}
	epochInfo := &rbft.EpochInfo{}
	err = epochInfo.Unmarshal(rawEpochInfo)
	if err != nil {
		return nil, err
	}

	nodes := &consensus.QuorumValidators{}
	err = nodes.UnmarshalVT(rawNodes)
	if err != nil {
		return nil, err
	}
	meta := &SnapshotMeta{
		BlockHeader: blockHeader,
		EpochInfo:   epochInfo,
		Nodes:       nodes,
	}
	return meta, nil
}

func (l *StateLedgerImpl) GenerateSnapshot(blockHeader *types.BlockHeader, errC chan error) {
	stateRoot := blockHeader.StateRoot.ETHHash()
	l.logger.Infof("[GenerateSnapshot] blockNum: %v, blockhash: %v, rootHash: %v", blockHeader.Number, blockHeader.Hash(), stateRoot)

	queue := []common.Hash{stateRoot}
	batch := l.snapshot.Batch()
	for len(queue) > 0 {
		trieRoot := queue[0]
		iter := jmt.NewIterator(trieRoot, l.backend, l.pruneCache, 100, time.Second)
		l.logger.Debugf("[GenerateSnapshot] trie root=%v", trieRoot)
		go iter.IterateLeaf()

		for {
			node, err := iter.Next()
			if err != nil {
				if err == jmt.ErrorNoMoreData {
					break
				} else {
					errC <- err
					return
				}
			}
			batch.Put(node.LeafKey, node.LeafValue)
			// data size exceed threshold, flush to disk
			if batch.Size() > maxBatchSize {
				batch.Commit()
				batch.Reset()
				l.logger.Infof("[GenerateSnapshot] write batch periodically")
			}
			if trieRoot == stateRoot && len(node.LeafValue) > 0 {
				// resolve potential contract account
				acc := &types.InnerAccount{Balance: big.NewInt(0)}
				if err := acc.Unmarshal(node.LeafValue); err != nil {
					panic(err)
				}
				if acc.StorageRoot != (common.Hash{}) {
					// prepare storage trie root
					queue = append(queue, acc.StorageRoot)
				}
			}
		}
		queue = queue[1:]
		batch.Put(trieRoot[:], l.backend.Get(trieRoot[:]))
	}
	batch.Commit()
	l.logger.Infof("[GenerateSnapshot] generate snapshot successfully")

	errC <- nil
}

func (l *StateLedgerImpl) VerifyTrie(blockHeader *types.BlockHeader) (bool, error) {
	l.logger.Infof("[VerifyTrie] start verifying blockNumber: %v, rootHash: %v", blockHeader.Number, blockHeader.StateRoot.String())
	defer l.logger.Infof("[VerifyTrie] finish VerifyTrie")
	return jmt.VerifyTrie(blockHeader.StateRoot.ETHHash(), l.backend, l.pruneCache)
}

func (l *StateLedgerImpl) Prove(rootHash common.Hash, key []byte) (*jmt.ProofResult, error) {
	var trie *jmt.JMT
	if rootHash == (common.Hash{}) {
		trie = l.accountTrie
		return trie.Prove(key)
	}
	trie, err := jmt.New(rootHash, l.backend, nil, l.pruneCache, l.logger)
	if err != nil {
		return nil, err
	}
	return trie.Prove(key)
}

func newStateLedger(rep *repo.Repo, stateStorage, snapshotStorage storage.Storage) (StateLedger, error) {
	stateCachedStorage := storagemgr.NewCachedStorage(stateStorage, 128).(*storagemgr.CachedStorage)
	accountTrieCache := storagemgr.NewCacheWrapper(rep.Config.Ledger.StateLedgerAccountTrieCacheMegabytesLimit, true)
	storageTrieCache := storagemgr.NewCacheWrapper(rep.Config.Ledger.StateLedgerStorageTrieCacheMegabytesLimit, true)

	accountCache, err := NewAccountCache(rep.Config.Ledger.StateLedgerAccountCacheSize, false)
	if err != nil {
		return nil, err
	}
	accountCache.SetEnableExpensiveMetric(rep.Config.Monitor.EnableExpensive)

	ledger := &StateLedgerImpl{
		repo:                  rep,
		logger:                loggers.Logger(loggers.Storage),
		backend:               stateCachedStorage,
		accountTrieCache:      accountTrieCache,
		storageTrieCache:      storageTrieCache,
		pruneCache:            prune.NewPruneCache(rep, stateCachedStorage, accountTrieCache, storageTrieCache, loggers.Logger(loggers.Storage)),
		accountCache:          accountCache,
		accounts:              make(map[string]IAccount),
		preimages:             make(map[types.Hash][]byte),
		changer:               NewChanger(),
		accessList:            NewAccessList(),
		logs:                  newEvmLogs(),
		enableExpensiveMetric: rep.Config.Monitor.EnableExpensive,
	}

	if snapshotStorage != nil {
		ledger.snapshot = snapshot.NewSnapshot(rep, snapshotStorage, ledger.logger)
	}

	ledger.refreshAccountTrie(nil)

	return ledger, nil
}

// NewStateLedger create a new ledger instance
func NewStateLedger(rep *repo.Repo, storageDir string) (StateLedger, error) {
	stateStoragePath := repo.GetStoragePath(rep.RepoRoot, storagemgr.Ledger)
	if storageDir != "" {
		stateStoragePath = path.Join(storageDir, storagemgr.Ledger)
	}
	stateStorage, err := storagemgr.OpenWithMetrics(stateStoragePath, storagemgr.Ledger)
	if err != nil {
		return nil, fmt.Errorf("create stateDB: %w", err)
	}

	snapshotStoragePath := repo.GetStoragePath(rep.RepoRoot, storagemgr.Snapshot)
	if storageDir != "" {
		snapshotStoragePath = path.Join(storageDir, storagemgr.Snapshot)
	}
	snapshotStorage, err := storagemgr.OpenWithMetrics(snapshotStoragePath, storagemgr.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("create snapshot storage: %w", err)
	}

	return newStateLedger(rep, stateStorage, snapshotStorage)
}

func (l *StateLedgerImpl) SetTxContext(thash *types.Hash, ti int) {
	l.thash = thash
	l.txIndex = ti
}

// Close close the ledger instance
func (l *StateLedgerImpl) Close() {
	_ = l.backend.Close()
	l.triePreloader.close()
}
