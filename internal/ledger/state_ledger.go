package ledger

import (
	"errors"
	"fmt"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	archive "github.com/axiomesh/axiom-ledger/internal/ledger/archive"
	"math/big"
	"path"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/jmt"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/prune"
	"github.com/axiomesh/axiom-ledger/internal/ledger/snapshot"
	"github.com/axiomesh/axiom-ledger/internal/ledger/trie_indexer"
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
	logger      logrus.FieldLogger
	accountTrie *jmt.JMT // keep track of the latest world state (dirty or committed)

	pruneCache       *prune.PruneCache
	trieIndexer      *trie_indexer.TrieIndexer
	archiver         *archive.Archiver
	backend          kv.Storage
	accountTrieCache *storagemgr.CacheWrapper
	storageTrieCache *storagemgr.CacheWrapper

	chainState *chainstate.ChainState

	triePreloader *triePreloaderManager
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
}

// NewView get a view at specific block. We can enable snapshot if and only if the block were the latest block.
func (l *StateLedgerImpl) NewView(blockHeader *types.BlockHeader, enableSnapshot bool) (StateLedger, error) {
	l.logger.Debugf("[NewView] height: %v, stateRoot: %v", blockHeader.Number, blockHeader.StateRoot)

	// if current node is an archive node, and request query historical state, then use archived history ledger as backend.
	if l.chainState != nil && (!enableSnapshot && l.chainState.IsDataSyncer) {
		lg := &StateLedgerImpl{
			repo:        l.repo,
			logger:      l.logger,
			backend:     l.archiver.GetHistoryBackend(),
			archiver:    l.archiver,
			accounts:    make(map[string]IAccount),
			preimages:   make(map[types.Hash][]byte),
			changer:     newChanger(),
			accessList:  NewAccessList(),
			logs:        newEvmLogs(),
			blockHeight: blockHeader.Number,
		}
		lg.refreshAccountTrie(blockHeader.StateRoot)
		return lg, nil
	}

	minHeight, maxHeight := l.GetHistoryRange()
	if blockHeader.Number < minHeight || blockHeader.Number > maxHeight {
		return nil, fmt.Errorf("history at target block %v is invalid, the valid range is from %v to %v", blockHeader.Number, minHeight, maxHeight)
	}

	lg := &StateLedgerImpl{
		repo:             l.repo,
		logger:           l.logger,
		backend:          l.backend,
		pruneCache:       l.pruneCache,
		accountTrieCache: l.accountTrieCache,
		storageTrieCache: l.storageTrieCache,
		trieIndexer:      l.trieIndexer,
		archiver:         l.archiver,
		accounts:         make(map[string]IAccount),
		preimages:        make(map[types.Hash][]byte),
		changer:          newChanger(),
		accessList:       NewAccessList(),
		logs:             newEvmLogs(),
		blockHeight:      blockHeader.Number,
	}
	if enableSnapshot {
		lg.snapshot = l.snapshot
	}
	lg.refreshAccountTrie(blockHeader.StateRoot)
	return lg, nil
}

func (l *StateLedgerImpl) GetHistoryRange() (uint64, uint64) {
	return l.pruneCache.GetRange()
}

func (l *StateLedgerImpl) GetStateJournal(blockNumber uint64) *types.StateJournal {
	if !l.chainState.IsDataSyncer {
		return nil
	}

	return l.archiver.GetStateJournal(blockNumber)
}

func (l *StateLedgerImpl) UpdateChainState(chainState *chainstate.ChainState) {
	l.archiver.UpdateChainState(chainState)
	l.chainState = chainState
}

func (l *StateLedgerImpl) Archive(blockHeader *types.BlockHeader, stateJournal *types.StateJournal) error {
	return l.archiver.Archive(blockHeader, stateJournal)
}

func (l *StateLedgerImpl) Finalise() {
	for _, account := range l.accounts {
		keys := account.Finalise()

		if l.triePreloader != nil && len(keys) > 0 && l.repo.Config.Ledger.EnablePreload {
			l.triePreloader.preload(account.GetStorageRoot(), keys)
		}
		account.SetCreated(false)
	}

	l.ClearChangerAndRefund()
}

// IterateTrie iterate the whole account trie and all contract storage tries of target block, and store them in kv.
func (l *StateLedgerImpl) IterateTrie(snapshotMeta *utils.SnapshotMeta, kv kv.Storage, errC chan error) {
	stateRoot := snapshotMeta.BlockHeader.StateRoot.ETHHash()
	l.logger.Infof("[IterateTrie] blockhash: %v, rootHash: %v", snapshotMeta.BlockHeader.Hash(), stateRoot)
	batch := kv.NewBatch()

	// in validate node, we should rebuild prune cache before iterate trie
	if l.pruneCache != nil {
		if err := l.pruneCache.Rollback(snapshotMeta.BlockHeader.Number, false); err != nil {
			errC <- err
		}
	}
	batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr), utils.MarshalUint64(snapshotMeta.BlockHeader.Number))
	batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr), utils.MarshalUint64(snapshotMeta.BlockHeader.Number))

	queue := []common.Hash{stateRoot}
	for len(queue) > 0 {
		trieRoot := queue[0]
		iter := jmt.NewIterator(trieRoot, l.backend, l.pruneCache, 10000, 300*time.Second)
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
		l.logger.Infof("[IterateTrie] trieRoot=%v, rootNodeKey from kv=%v", trieRoot, l.backend.Get(trieRoot[:]))
		batch.Put(trieRoot[:], l.backend.Get(trieRoot[:]))
	}

	snapshotMetaBytes, err := snapshotMeta.Marshal()
	if err != nil {
		errC <- err
		return
	}
	batch.Put([]byte(utils.SnapshotMetaKey), snapshotMetaBytes)

	batch.Commit()
	l.logger.Infof("[IterateTrie] iterate trie successfully")

	errC <- nil
}

func (l *StateLedgerImpl) GetTrieSnapshotMeta() (*utils.SnapshotMeta, error) {
	raw := l.backend.Get([]byte(utils.SnapshotMetaKey))
	if len(raw) == 0 {
		return nil, ErrNotFound
	}

	snapshotMeta := &utils.SnapshotMeta{}
	if err := snapshotMeta.Unmarshal(raw); err != nil {
		return nil, err
	}
	return snapshotMeta, nil
}

// GenerateSnapshot generate the snapshot by iterating state trie leaves.
func (l *StateLedgerImpl) GenerateSnapshot(blockHeader *types.BlockHeader, errC chan error) {
	stateRoot := blockHeader.StateRoot.ETHHash()
	l.logger.Infof("[GenerateSnapshot] blockNum: %v, blockhash: %v, rootHash: %v", blockHeader.Number, blockHeader.Hash(), stateRoot)

	// make sure snapshot is empty before generate it
	minHeight, maxHeight := l.snapshot.GetJournalRange()
	if l.Version() == 0 || minHeight != 0 || maxHeight != 0 {
		l.logger.Infof("[GenerateSnapshot] snapshot is non-empty, finish generating")
		errC <- nil
		return
	}

	//  rebuild prune cache before iterate trie
	if err := l.pruneCache.Rollback(blockHeader.Number, false); err != nil {
		errC <- err
		return
	}

	queue := []common.Hash{stateRoot}
	batch := l.snapshot.Batch()
	for len(queue) > 0 {
		trieRoot := queue[0]
		iter := jmt.NewIterator(trieRoot, l.backend, l.pruneCache, 10000, 300*time.Second)
		l.logger.Infof("[GenerateSnapshot] trie root=%v", trieRoot)
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
	}
	batch.Put(utils.CompositeKey(utils.SnapshotKey, utils.MinHeightStr), utils.MarshalUint64(blockHeader.Number))
	batch.Put(utils.CompositeKey(utils.SnapshotKey, utils.MaxHeightStr), utils.MarshalUint64(blockHeader.Number))
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

func newStateLedger(rep *repo.Repo, stateStorage, snapshotStorage kv.Storage) (StateLedger, error) {
	stateCachedStorage := storagemgr.NewCachedStorage(stateStorage, 128).(*storagemgr.CachedStorage)
	accountTrieCache := storagemgr.NewCacheWrapper(rep.Config.Ledger.StateLedgerAccountTrieCacheMegabytesLimit, true)
	storageTrieCache := storagemgr.NewCacheWrapper(rep.Config.Ledger.StateLedgerStorageTrieCacheMegabytesLimit, true)

	trieIndexerKv, err := storagemgr.OpenWithMetrics(storagemgr.GetLedgerComponentPath(rep, storagemgr.TrieIndexer), storagemgr.TrieIndexer)
	if err != nil {
		return nil, err
	}

	// init archive path
	archiveHistoryStorage, err := storagemgr.Open(storagemgr.GetLedgerComponentPath(rep, storagemgr.ArchiveHistory))
	if err != nil {
		return nil, err
	}
	archiveJournalStorage, err := storagemgr.Open(storagemgr.GetLedgerComponentPath(rep, storagemgr.ArchiveJournal))
	if err != nil {
		return nil, err
	}
	archiveArgs := &archive.ArchiveArgs{
		ArchiveHistoryStorage: archiveHistoryStorage,
		ArchiveJournalStorage: archiveJournalStorage,
	}

	ledger := &StateLedgerImpl{
		repo:             rep,
		logger:           loggers.Logger(loggers.Storage),
		backend:          stateCachedStorage,
		accountTrieCache: accountTrieCache,
		storageTrieCache: storageTrieCache,
		pruneCache:       prune.NewPruneCache(rep, stateCachedStorage, accountTrieCache, storageTrieCache, loggers.Logger(loggers.Storage)),
		trieIndexer:      trie_indexer.NewTrieIndexer(rep, trieIndexerKv, loggers.Logger(loggers.Storage)),
		archiver:         archive.NewArchiver(rep, archiveArgs, loggers.Logger(loggers.Storage)),
		accounts:         make(map[string]IAccount),
		preimages:        make(map[types.Hash][]byte),
		changer:          newChanger(),
		accessList:       NewAccessList(),
		logs:             newEvmLogs(),
	}

	if snapshotStorage != nil {
		ledger.snapshot = snapshot.NewSnapshot(rep, snapshotStorage, ledger.logger)
	}

	ledger.refreshAccountTrie(nil)

	return ledger, nil
}

// NewStateLedger create a new ledger instance
func NewStateLedger(rep *repo.Repo, storageDir string) (StateLedger, error) {
	stateStoragePath := storagemgr.GetLedgerComponentPath(rep, storagemgr.Ledger)
	if storageDir != "" {
		stateStoragePath = path.Join(storageDir, storagemgr.Ledger)
	}
	stateStorage, err := storagemgr.OpenWithMetrics(stateStoragePath, storagemgr.Ledger)
	if err != nil {
		return nil, fmt.Errorf("create stateDB: %w", err)
	}

	snapshotStoragePath := storagemgr.GetLedgerComponentPath(rep, storagemgr.Snapshot)
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
