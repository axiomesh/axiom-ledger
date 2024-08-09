package prune

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

// PruneCache enables trie node caches, so that every trie read op will happen in trie cache first,
// which avoids frequent reading from disk.
// By using PruneCache, we also implement trie pruning schema, which reduces state storage size that a full node must hold.
type PruneCache struct {
	rep           *repo.Repo
	ledgerStorage kv.Storage
	states        *states

	prunner *prunner

	logger logrus.FieldLogger
}

type states struct {
	diffs     []*diff
	lock      sync.RWMutex
	allKeyMap map[string]struct{}
}

type diff struct {
	height uint64

	accountDiff map[string]types.Node
	storageDiff map[string]types.Node

	ledgerStorage kv.Storage
}

const (
	TypeAccount = 1
	TypeStorage = 2
)

var (
	ErrorRollbackToHigherNumber = errors.New("rollback PruneCache to higher blockchain height")
	ErrorRollbackTooMuch        = errors.New("rollback PruneCache too much block")
)

func (s *states) rebuildAllKeyMap() {
	s.allKeyMap = make(map[string]struct{})
	if len(s.diffs) > 0 {
		for _, diff := range s.diffs {
			for k := range diff.accountDiff {
				s.allKeyMap[k] = struct{}{}
			}
			for k := range diff.storageDiff {
				s.allKeyMap[k] = struct{}{}
			}
		}
	}
}

func NewPruneCache(rep *repo.Repo, ledgerStorage kv.Storage, accountTrieCache *storagemgr.CacheWrapper, storageTrieCache *storagemgr.CacheWrapper, logger logrus.FieldLogger) *PruneCache {
	tc := &PruneCache{
		rep:           rep,
		ledgerStorage: ledgerStorage,
		states:        &states{diffs: make([]*diff, 0), allKeyMap: make(map[string]struct{})},
		logger:        logger,
	}

	p := NewPrunner(rep, ledgerStorage, accountTrieCache, storageTrieCache, tc.states, logger)
	tc.prunner = p
	go p.pruning()
	return tc
}

func (tc *PruneCache) addNewDiff(batch kv.Batch, height uint64, ledgerStorage kv.Storage, stateJournal *types.StateJournal, persist bool) {
	l := &diff{
		height:        height,
		ledgerStorage: ledgerStorage,
		accountDiff:   make(map[string]types.Node),
		storageDiff:   make(map[string]types.Node),
	}
	if persist {
		batch.Put(utils.CompositeKey(utils.PruneJournalKey, height), stateJournal.Encode())
		batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr), utils.MarshalUint64(height))
		if height == 0 {
			batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr), utils.MarshalUint64(height))
		}
	}

	for _, journal := range stateJournal.TrieJournal {
		batch.Put(journal.RootHash[:], journal.RootNodeKey.Encode())
		for k := range journal.PruneSet {
			if journal.Type == TypeAccount {
				l.accountDiff[k] = nil
			} else {
				l.storageDiff[k] = nil
			}
			tc.states.allKeyMap[k] = struct{}{}
		}
		for k, v := range journal.DirtySet {
			if journal.Type == TypeAccount {
				l.accountDiff[k] = v
			} else {
				l.storageDiff[k] = v
			}
			tc.states.allKeyMap[k] = struct{}{}
		}
	}
	tc.states.diffs = append(tc.states.diffs, l)
}

func (tc *PruneCache) Update(batch kv.Batch, height uint64, trieJournals *types.StateJournal) {
	tc.states.lock.Lock()
	defer tc.states.lock.Unlock()

	tc.logger.Debugf("[PruneCache-Update] update trie cache at height: %v", height)

	tc.addNewDiff(batch, height, tc.ledgerStorage, trieJournals, true)
}

func (tc *PruneCache) Enable() bool {
	return tc != nil
}

func (tc *PruneCache) Get(version uint64, key []byte) (types.Node, bool) {
	tc.states.lock.RLock()
	defer tc.states.lock.RUnlock()

	if len(tc.states.diffs) == 0 {
		return nil, false
	}

	k := string(key)

	if _, ok := tc.states.allKeyMap[k]; !ok {
		return nil, false
	}

	for i := len(tc.states.diffs) - 1; i >= 0; i-- {
		if tc.states.diffs[i].height > version {
			continue
		}
		// the origin trie node may be recycled later, so we must deep-copy it here.
		if v, ok := tc.states.diffs[i].accountDiff[k]; ok {
			if v == nil {
				return v, ok
			}
			return v.Copy(), true
		}
		if v, ok := tc.states.diffs[i].storageDiff[k]; ok {
			if v == nil {
				return v, ok
			}
			return v.Copy(), true
		}
	}

	return nil, false
}

// Rollback rebuilds pruneCache from pruneJournal at target height.
func (tc *PruneCache) Rollback(height uint64, persist bool) error {
	tc.states.lock.Lock()
	defer tc.states.lock.Unlock()

	minHeight, maxHeight := tc.GetRange()

	tc.logger.Infof("[PruneCache-Rollback] minHeight=%v, maxHeight=%v, targetHeight=%v", minHeight, maxHeight, height)

	if maxHeight < height {
		return ErrorRollbackToHigherNumber
	}

	if minHeight > height {
		return ErrorRollbackTooMuch
	}

	tc.states.diffs = make([]*diff, 0)
	tc.states.allKeyMap = make(map[string]struct{}, 0)

	batch := tc.ledgerStorage.NewBatch()
	for i := minHeight; i <= height; i++ {
		trieJournal := tc.getStateJournal(i)
		tc.logger.Debugf("[PruneCache-Rollback] apply trie journal of height=%v, trieJournal=%v", i, trieJournal)
		if trieJournal == nil {
			tc.logger.Warnf("[PruneCache-Rollback] trie journal is empty at height: %v", i)
			continue
		}
		tc.addNewDiff(batch, i, tc.ledgerStorage, trieJournal, false)
	}
	batch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr), utils.MarshalUint64(height))

	for i := height + 1; i <= maxHeight; i++ {
		batch.Delete(utils.CompositeKey(utils.PruneJournalKey, i))
	}

	if persist {
		batch.Commit()
	}
	tc.states.rebuildAllKeyMap()

	return nil
}

func (tc *PruneCache) GetRange() (uint64, uint64) {
	minHeight := uint64(0)
	maxHeight := uint64(0)

	data := tc.ledgerStorage.Get(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr))
	if data != nil {
		minHeight = utils.UnmarshalUint64(data)
	}

	data = tc.ledgerStorage.Get(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr))
	if data != nil {
		maxHeight = utils.UnmarshalUint64(data)
	}

	return minHeight, maxHeight
}

func (tc *PruneCache) getStateJournal(height uint64) *types.StateJournal {
	data := tc.ledgerStorage.Get(utils.CompositeKey(utils.PruneJournalKey, height))
	if data == nil {
		return nil
	}

	res := &types.StateJournal{}
	err := res.Decode(data)
	if err != nil {
		panic(err)
	}

	return res
}

// for debug
func (dl *diff) String() string {
	res := strings.Builder{}
	res.WriteString("Version[")
	res.WriteString(strconv.Itoa(int(dl.height)))
	res.WriteString("], \nAccountDiff[\n")
	res.WriteString(fmt.Sprintf("journal=%v\n", dl.accountDiff))
	res.WriteString("], \nStorageDiff[\n")
	res.WriteString(fmt.Sprintf("journal=%v\n", dl.storageDiff))
	res.WriteString("]")
	return res.String()
}
