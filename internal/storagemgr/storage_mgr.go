package storagemgr

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	pebbledb "github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/prometheus/common/model"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/storage/kv/leveldb"
	"github.com/axiomesh/axiom-kit/storage/kv/pebble"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	BlockChain      = "blockchain"
	Ledger          = "ledger"
	Indexer         = "indexer"
	Snapshot        = "snapshot"
	Blockfile       = "blockfile"
	Consensus       = "consensus"
	Epoch           = "epoch"
	TxPool          = "txpool"
	TrieIndexer     = "trie_indexer"
	ArchiveHistory  = "archive_history"
	ArchiveSnapshot = "archive_snapshot"
	ArchiveJournal  = "archive_journal"
)

var globalStorageMgr = &storageMgr{
	storageBuilderMap: make(map[string]func(p string, metricsPrefixName string) (kv.Storage, error)),
	storages:          make(map[string]kv.Storage),
	lock:              new(sync.Mutex),
}

func init() {
	memoryBuilder := func(p string, metricsPrefixName string) (kv.Storage, error) {
		return kv.NewMemory(), nil
	}

	// only for test
	globalStorageMgr.storageBuilderMap[repo.KVStorageTypeLeveldb] = memoryBuilder
	globalStorageMgr.storageBuilderMap[repo.KVStorageTypePebble] = memoryBuilder
	globalStorageMgr.storageBuilderMap[""] = memoryBuilder
}

type storageMgr struct {
	storageBuilderMap map[string]func(p string, metricsPrefixName string) (kv.Storage, error)
	storages          map[string]kv.Storage
	defaultKVType     string
	lock              *sync.Mutex
}

var defaultPebbleOptions = &pebbledb.Options{
	// MemTableStopWritesThreshold is max number of the existent MemTables(including the frozen one).
	// This manner is the same with leveldb, including a frozen memory table and another live one.
	MemTableStopWritesThreshold: 2,

	// The default compaction concurrency(1 thread)
	MaxConcurrentCompactions: func() int { return runtime.NumCPU() },

	// Per-level options. Options for at least one level must be specified. The
	// options for the last level are used for all subsequent levels.
	// This option is the same with Ethereum.
	Levels: []pebbledb.LevelOptions{
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		{TargetFileSize: 2 * 1024 * 1024, BlockSize: 32 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
	},
}

func (m *storageMgr) open(typ string, p string, metricsPrefixName string) (kv.Storage, error) {
	builder, ok := m.storageBuilderMap[typ]
	if !ok {
		return nil, fmt.Errorf("unknow kv type %s, expect leveldb or pebble", typ)
	}
	return builder(p, metricsPrefixName)
}

func Initialize(repoConfig *repo.Config) error {
	storageConfig := repoConfig.Storage
	globalStorageMgr.storageBuilderMap[repo.KVStorageTypeLeveldb] = func(p string, _ string) (kv.Storage, error) {
		return leveldb.New(p, nil)
	}
	globalStorageMgr.storageBuilderMap[repo.KVStorageTypePebble] = func(p string, metricsPrefixName string) (kv.Storage, error) {
		defaultPebbleOptions.Cache = pebbledb.NewCache(storageConfig.KVCacheSize * 1024 * 1024)
		defaultPebbleOptions.MemTableSize = uint64(storageConfig.Pebble.MemTableSize * 1024 * 1024) // The size of single memory table
		defaultPebbleOptions.MemTableStopWritesThreshold = storageConfig.Pebble.MemTableStopWritesThreshold
		defaultPebbleOptions.MaxOpenFiles = storageConfig.Pebble.MaxOpenFiles
		defaultPebbleOptions.L0CompactionFileThreshold = storageConfig.Pebble.L0CompactionFileThreshold
		defaultPebbleOptions.LBaseMaxBytes = storageConfig.Pebble.LBaseMaxSize * 1024 * 1024
		namespace := "axiom_ledger"
		subsystem := "ledger"
		var metricOpts []pebble.MetricsOption
		if repoConfig.Monitor.Enable && model.IsValidMetricName(model.LabelValue(metricsPrefixName)) {
			metricOpts = append(metricOpts,
				pebble.WithDiskSizeGauge(namespace, subsystem, metricsPrefixName),
				pebble.WithDiskWriteThroughput(namespace, subsystem, metricsPrefixName),
				pebble.WithWalWriteThroughput(namespace, subsystem, metricsPrefixName),
				pebble.WithEffectiveWriteThroughput(namespace, subsystem, metricsPrefixName))
		}
		return pebble.New(p, defaultPebbleOptions, &pebbledb.WriteOptions{Sync: storageConfig.Sync}, loggers.Logger(loggers.Storage), metricOpts...)
	}
	_, ok := globalStorageMgr.storageBuilderMap[storageConfig.KvType]
	if !ok {
		return fmt.Errorf("unknow kv type %s, expect leveldb or pebble", storageConfig.KvType)
	}
	globalStorageMgr.defaultKVType = storageConfig.KvType
	return nil
}

func Open(p string) (kv.Storage, error) {
	return OpenSpecifyType(globalStorageMgr.defaultKVType, p, "")
}

func OpenWithMetrics(p string, uniqueMetricsPrefixName string) (kv.Storage, error) {
	if uniqueMetricsPrefixName != "" && !model.IsValidMetricName(model.LabelValue(uniqueMetricsPrefixName)) {
		return nil, fmt.Errorf("%q is not a valid metric name", uniqueMetricsPrefixName)
	}
	return OpenSpecifyType(globalStorageMgr.defaultKVType, p, uniqueMetricsPrefixName)
}

func OpenSpecifyType(typ string, p string, metricName string) (kv.Storage, error) {
	globalStorageMgr.lock.Lock()
	defer globalStorageMgr.lock.Unlock()
	s, ok := globalStorageMgr.storages[p]
	if !ok {
		var err error
		s, err = globalStorageMgr.open(typ, p, metricName)
		if err != nil {
			return nil, err
		}
		globalStorageMgr.storages[p] = s
	}
	return s, nil
}

func GetLedgerComponentPath(rep *repo.Repo, component string) string {
	if rep.Config != nil && rep.Config.Ledger.Path.EnableConfigPath {
		switch component {
		case BlockChain:
			return rep.Config.Ledger.Path.ChainLedgerPath
		case Blockfile:
			return rep.Config.Ledger.Path.BlockfilePath
		case Ledger:
			return rep.Config.Ledger.Path.StateLedgerPath
		case ArchiveSnapshot:
			return rep.Config.Ledger.Path.ArchiveSnapshotPath
		case ArchiveHistory:
			return rep.Config.Ledger.Path.ArchiveHistoryPath
		case ArchiveJournal:
			return rep.Config.Ledger.Path.ArchiveJournalPath
		}
	}
	return filepath.Join(rep.RepoRoot, "storage", component)
}
