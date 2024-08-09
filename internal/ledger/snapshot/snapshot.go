package snapshot

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type Snapshot struct {
	rep *repo.Repo

	accountSnapshotCache  *storagemgr.CacheWrapper
	contractSnapshotCache *storagemgr.CacheWrapper
	backend               kv.Storage

	lock sync.RWMutex

	logger logrus.FieldLogger
}

var (
	ErrorRollbackToHigherNumber = errors.New("rollback snapshot to higher blockchain height")
	ErrorRollbackTooMuch        = errors.New("rollback snapshot too much block")
)

// maxBatchSize defines the maximum size of the data in single batch write operation, which is 64 MB.
const maxBatchSize = 64 * 1024 * 1024
const MinJournalHeight = 10

// minPreserveJournalNumber defines the minimum number of snapshot journals that ledger would preserve.
var minPreserveJournalNumber = 128

func NewSnapshot(rep *repo.Repo, backend kv.Storage, logger logrus.FieldLogger) *Snapshot {
	return &Snapshot{
		rep:                   rep,
		accountSnapshotCache:  storagemgr.NewCacheWrapper(rep.Config.Snapshot.AccountSnapshotCacheMegabytesLimit, true),
		contractSnapshotCache: storagemgr.NewCacheWrapper(rep.Config.Snapshot.ContractSnapshotCacheMegabytesLimit, true),
		backend:               backend,
		logger:                logger,
	}
}

func (snap *Snapshot) Account(addr *types.Address) (*types.InnerAccount, error) {
	snap.lock.RLock()
	defer snap.lock.RUnlock()

	accountKey := utils.CompositeAccountKey(addr)

	// try in cache first
	if snap.accountSnapshotCache != nil {
		if blob, ok := snap.accountSnapshotCache.Get(accountKey); ok {
			if len(blob) == 0 { // can be both nil and []byte{}
				return nil, nil
			}
			innerAccount := &types.InnerAccount{Balance: big.NewInt(0)}
			if err := innerAccount.Unmarshal(blob); err != nil {
				panic(err)
			}
			return innerAccount, nil
		}
	}

	// try in kv last
	blob := snap.backend.Get(accountKey)
	if snap.accountSnapshotCache != nil {
		snap.accountSnapshotCache.Set(accountKey, blob)
	}
	if len(blob) == 0 { // can be both nil and []byte{}
		return nil, nil
	}
	innerAccount := &types.InnerAccount{Balance: big.NewInt(0)}
	if err := innerAccount.Unmarshal(blob); err != nil {
		panic(err)
	}

	return innerAccount, nil
}

func (snap *Snapshot) Storage(addr *types.Address, key []byte) ([]byte, error) {
	snap.lock.RLock()
	defer snap.lock.RUnlock()

	snapKey := utils.CompositeStorageKey(addr, key)

	if snap.contractSnapshotCache != nil {
		if blob, ok := snap.contractSnapshotCache.Get(snapKey); ok {
			return blob, nil
		}
	}

	blob := snap.backend.Get(snapKey)
	if snap.contractSnapshotCache != nil {
		snap.contractSnapshotCache.Set(snapKey, blob)
	}
	return blob, nil
}

// todo batch update snapshot of serveral blocks
func (snap *Snapshot) Update(height uint64, journal *types.SnapshotJournal, destructs map[string]struct{}, accounts map[string][]byte, storage map[string]map[string][]byte) (int, error) {
	snap.lock.Lock()
	defer snap.lock.Unlock()

	snap.logger.Infof("[Snapshot-Update] update snapshot at height:%v", height)

	batch := snap.backend.NewBatch()

	for addr := range destructs {
		accountKey := utils.CompositeAccountKey(types.NewAddressByStr(addr))
		batch.Delete(accountKey)
		snap.accountSnapshotCache.Del(accountKey)
	}

	for addr, acc := range accounts {
		accountKey := utils.CompositeAccountKey(types.NewAddressByStr(addr))
		snap.accountSnapshotCache.Set(accountKey, acc)
		batch.Put(accountKey, acc)
	}

	for rawAddr, slots := range storage {
		addr := types.NewAddressByStr(rawAddr)
		for slot, blob := range slots {
			storageKey := utils.CompositeStorageKey(addr, []byte(slot))
			snap.contractSnapshotCache.Set(storageKey, blob)
			batch.Put(storageKey, blob)
		}
	}

	data, err := journal.Encode()
	if err != nil {
		return 0, fmt.Errorf("marshal snapshot journal error: %w", err)
	}

	batch.Put(utils.CompositeKey(utils.SnapshotKey, height), data)
	batch.Put(utils.CompositeKey(utils.SnapshotKey, utils.MaxHeightStr), utils.MarshalUint64(height))
	if height == 0 {
		batch.Put(utils.CompositeKey(utils.SnapshotKey, utils.MinHeightStr), utils.MarshalUint64(height))
	}

	if height > MinJournalHeight {
		minHeight, _ := snap.GetJournalRange()

		if height > minHeight && int(height-minHeight) >= minPreserveJournalNumber {
			for i := minHeight; i < height; i++ {
				batch.Delete(utils.CompositeKey(utils.SnapshotKey, i))
			}
			batch.Put(utils.CompositeKey(utils.SnapshotKey, utils.MinHeightStr), utils.MarshalUint64(height))

			snap.logger.Infof("[removeJournalsBeforeBlock] remove snapshot journals before block %v", height)
		}
	}

	batch.Commit()
	return batch.Size(), nil
}

// Rollback removes snapshot journals whose block number < height
func (snap *Snapshot) Rollback(height uint64) error {
	snap.lock.Lock()
	defer snap.lock.Unlock()

	if snap.contractSnapshotCache != nil {
		snap.contractSnapshotCache.Reset()
	}

	if snap.accountSnapshotCache != nil {
		snap.accountSnapshotCache.Reset()
	}

	minHeight, maxHeight := snap.GetJournalRange()
	snap.logger.Infof("[Snapshot-Rollback] minHeight=%v,maxHeight=%v,height=%v", minHeight, maxHeight, height)

	// empty snapshot
	if minHeight == maxHeight && maxHeight == 0 {
		return nil
	}

	if maxHeight < height {
		return ErrorRollbackToHigherNumber
	}

	if minHeight > height {
		return ErrorRollbackTooMuch
	}

	if maxHeight == height {
		return nil
	}

	batch := snap.backend.NewBatch()
	for i := maxHeight; i > height; i-- {
		snap.logger.Infof("[Snapshot-Rollback] try executing snapshot journal of height %v", i)
		blockJournal := snap.GetBlockJournal(i)
		if blockJournal == nil {
			snap.logger.Warnf("[Snapshot-Rollback] snapshot journal is empty at height: %v", i)
		} else {
			for _, entry := range blockJournal.Journals {
				snap.logger.Debugf("[Snapshot-Rollback] execute entry: %v", entry.String())
				revertJournal(entry, batch)
			}
		}
		batch.Delete(utils.CompositeKey(utils.SnapshotKey, i))
		batch.Put(utils.CompositeKey(utils.SnapshotKey, utils.MaxHeightStr), utils.MarshalUint64(i-1))
		if batch.Size() > maxBatchSize {
			batch.Commit()
			batch.Reset()
			snap.logger.Infof("[Snapshot-Rollback] write batch periodically")
		}
	}
	batch.Commit()

	return nil
}

func (snap *Snapshot) ExportMetrics() {
	if snap == nil {
		return
	}
	accountCacheMetrics := snap.accountSnapshotCache.ExportMetrics()
	snapshotAccountCacheMissCounterPerBlock.Set(float64(accountCacheMetrics.CacheMissCounter))
	snapshotAccountCacheHitCounterPerBlock.Set(float64(accountCacheMetrics.CacheHitCounter))
	snapshotAccountCacheSize.Set(float64(accountCacheMetrics.CacheSize / 1024 / 1024))
}

func (snap *Snapshot) ResetMetrics() {
	if snap == nil {
		return
	}
	snap.accountSnapshotCache.ResetCounterMetrics()
	snap.contractSnapshotCache.ResetCounterMetrics()
}

// Batch provides the ability to write snapshot directly
func (snap *Snapshot) Batch() kv.Batch {
	snap.lock.Lock()
	defer snap.lock.Unlock()

	// reset snapshot cache to ensure data consistency
	if snap.contractSnapshotCache != nil {
		snap.contractSnapshotCache.Reset()
	}
	if snap.accountSnapshotCache != nil {
		snap.accountSnapshotCache.Reset()
	}

	return snap.backend.NewBatch()
}
