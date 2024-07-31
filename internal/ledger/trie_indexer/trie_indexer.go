package trie_indexer

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

// TrieIndexer is used for fast accessing the latest state trie, only enabled in validator.
type TrieIndexer struct {
	rep *repo.Repo

	backend kv.Storage

	logger logrus.FieldLogger
}

// todo(zqr): verify archive node compatibility, disable trie indexer in archive node in current version.
func NewTrieIndexer(rep *repo.Repo, backend kv.Storage, logger logrus.FieldLogger) *TrieIndexer {
	return &TrieIndexer{
		rep:     rep,
		backend: backend,
		logger:  logger,
	}
}

// todo(zqr): consider using journal to better support ledger rollback
// todo(zqr): batch write after several blocks
func (indexer *TrieIndexer) Update(height uint64, stateDelta *types.StateJournal) {
	current := time.Now()
	batch := indexer.backend.NewBatch()

	for _, tj := range stateDelta.TrieJournal {
		for rawNk, node := range tj.DirtySet {
			nk := types.DecodeNodeKey([]byte(rawNk))
			if node.Type() == types.TypeLeafNode {
				k := node.(*types.LeafNode).Key[:]
				//batch.Put(utils.CompositeKey(utils.TrieNodeIndexKey, node.(*types.LeafNode).Key[:]), utils.MarshalUint64(uint64(len(nk.Path))))
				indexer.logger.Debugf("[TrieIndexes-Update] update trie leaf node key %v with length %v", k, len(nk.Path))
				batch.Put(k, utils.MarshalUint64(uint64(len(nk.Path))))
			}
			batch.Put(nk.GetIndex(), utils.MarshalUint64(nk.Version))
		}
	}

	batch.Commit()

	indexer.logger.Infof("[TrieIndexer-Update] update trie indexes at height: %v, write size (bytes): %v, time: %v", height, batch.Size(), time.Since(current))
}

// GetTrieIndexes returns all trie node indexes of target merkle path.
// todo(zqr): use cache to reduce goroutine number
func (indexer *TrieIndexer) GetTrieIndexes(height uint64, typ []byte, key []byte) [][]byte {
	// check validity, must be contract storage slot (256 bit) or account address (160 bit)
	if len(typ) != 64 && len(typ) != 40 {
		return nil
	}

	// get the number of nodes in target merkle path
	rawLen := indexer.backend.Get(utils.CompositeKey(utils.TrieNodeIndexKey, key))
	pathLen := uint64(0)
	if len(rawLen) != 0 {
		pathLen = utils.UnmarshalUint64(rawLen)
	}
	indexer.logger.Debugf("[GetTrieIndexes] get trie node key %v with length %v", key, pathLen)

	res := make([][]byte, 0)
	nks := make([]*types.NodeKey, 0)

	// get each node's index of target merkle path
	var wg sync.WaitGroup
	for i := uint64(0); i <= pathLen; i++ {
		nk := &types.NodeKey{
			Type: typ,
			Path: key[:i],
		}
		nks = append(nks, nk)
		wg.Add(1)
		go func() {
			defer wg.Done()
			rawVersion := indexer.backend.Get(nk.GetIndex())
			if len(rawVersion) == 0 {
				nk.Type = []byte{}
				return
			}
			nk.Version = utils.UnmarshalUint64(rawVersion)
		}()
	}
	wg.Wait()

	for _, nk := range nks {
		// in case of ledger rollback
		if len(nk.Type) == 0 || nk.Version > height {
			continue
		}
		res = append(res, nk.Encode())
	}
	return res
}
