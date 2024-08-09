package prune

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type Archiver struct {
	rep *repo.Repo

	chainState                 *chainstate.ChainState
	archiveHistoryBackend      kv.Storage
	archiveJournalBackend      kv.Storage
	archiveTrieSnapshotBackend kv.Storage

	lastArchiveBlock              uint64
	archiveTrieSnapshotPath       string
	archiveTrieSnapshotOriginPath string

	ledgerBackend   kv.Storage
	snapshotBackend kv.Storage

	logger logrus.FieldLogger
}

type ArchiveArgs struct {
	ArchiveHistoryStorage kv.Storage
	ArchiveJournalStorage kv.Storage
}

func NewArchiver(rep *repo.Repo, archiveArgs *ArchiveArgs, logger logrus.FieldLogger) *Archiver {
	snapshotPath := storagemgr.GetLedgerComponentPath(rep, storagemgr.ArchiveSnapshot)
	snapshotOriginPath := filepath.Join(snapshotPath, "origin")
	archiveSnapshotStorage, err := storagemgr.Open(snapshotOriginPath)
	if err != nil {
		panic(err)
	}
	archiver := &Archiver{
		rep:                           rep,
		archiveTrieSnapshotBackend:    archiveSnapshotStorage,
		archiveJournalBackend:         archiveArgs.ArchiveJournalStorage,
		archiveHistoryBackend:         archiveArgs.ArchiveHistoryStorage,
		logger:                        logger,
		archiveTrieSnapshotPath:       snapshotPath,
		archiveTrieSnapshotOriginPath: snapshotOriginPath,
	}
	if data := archiver.archiveTrieSnapshotBackend.Get(utils.CompositeKey(utils.ArchiveKey, utils.MaxHeightStr)); data != nil {
		archiver.lastArchiveBlock = utils.UnmarshalUint64(data)
	}
	return archiver
}

func (archiver *Archiver) Archive(blockHeader *types.BlockHeader, stateJournal *types.StateJournal) error {
	if archiver.chainState != nil && !archiver.chainState.IsDataSyncer {
		return nil
	}

	cur := time.Now()
	var wg sync.WaitGroup

	// archive journal data
	wg.Add(1)
	go func() {
		defer wg.Done()
		journalBatch := archiver.archiveJournalBackend.NewBatch()
		journalBatch.Put(utils.CompositeKey(utils.PruneJournalKey, blockHeader.Number), stateJournal.Encode())
		journalBatch.Commit()
	}()

	// archive history data
	wg.Add(1)
	go func() {
		defer wg.Done()
		historyBatch := archiver.archiveHistoryBackend.NewBatch()
		for _, journal := range stateJournal.TrieJournal {
			historyBatch.Put(journal.RootHash[:], journal.RootNodeKey.Encode())
			for k, v := range journal.DirtySet {
				historyBatch.Put([]byte(k), v.Encode())
			}
		}
		for k, v := range stateJournal.CodeJournal {
			historyBatch.Put([]byte(k), v)
		}
		historyBatch.Commit()
	}()

	// update trie snapshot data
	wg.Add(1)
	go func() {
		defer wg.Done()
		snapshotBatch := archiver.archiveTrieSnapshotBackend.NewBatch()
		for _, journal := range stateJournal.TrieJournal {
			snapshotBatch.Put(journal.RootHash[:], journal.RootNodeKey.Encode())
			for k, v := range journal.DirtySet {
				snapshotBatch.Put([]byte(k), v.Encode())
			}
			for k := range journal.PruneSet {
				snapshotBatch.Delete([]byte(k))
			}
		}
		for k, v := range stateJournal.CodeJournal {
			snapshotBatch.Put([]byte(k), v)
		}
		snapshotBatch.Commit()
	}()

	wg.Wait()

	if blockHeader.Number-archiver.lastArchiveBlock < uint64(archiver.rep.Config.Ledger.ArchiveBlockNum) {
		return nil
	}

	// archive snapshot data
	snapshotBatch := archiver.archiveTrieSnapshotBackend.NewBatch()
	epochInfo, err := archiver.chainState.GetEpochInfo(blockHeader.Epoch)
	if err != nil {
		return fmt.Errorf("get epoch info failed: %w", err)
	}
	snapshotMeta := &utils.SnapshotMeta{
		BlockHeader: blockHeader,
		EpochInfo:   epochInfo,
	}
	snapshotMetaBytes, err := snapshotMeta.Marshal()
	if err != nil {
		return fmt.Errorf("marshal snapshotMeta failed: %w", err)
	}
	snapshotBatch.Put([]byte(utils.SnapshotMetaKey), snapshotMetaBytes)
	snapshotBatch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MinHeightStr), utils.MarshalUint64(snapshotMeta.BlockHeader.Number))
	snapshotBatch.Put(utils.CompositeKey(utils.PruneJournalKey, utils.MaxHeightStr), utils.MarshalUint64(snapshotMeta.BlockHeader.Number))
	snapshotBatch.Put(utils.CompositeKey(utils.ArchiveKey, utils.MaxHeightStr), utils.MarshalUint64(snapshotMeta.BlockHeader.Number))
	snapshotBatch.Commit()

	if err := archiver.archiveTrieSnapshotBackend.Close(); err != nil {
		return errors.Errorf("archive snapshot error: %v", err)
	}
	snapshotTargetPath := filepath.Join(archiver.archiveTrieSnapshotPath, fmt.Sprintf("snapshot-%v-%v", blockHeader.Number, time.Now().Format("2006-01-02T15-04-05")))
	if err := os.MkdirAll(snapshotTargetPath, os.ModePerm); err != nil {
		return errors.Errorf("mkdir snapshot archive dir error: %v", err.Error())
	}
	if err := copyDir(archiver.archiveTrieSnapshotOriginPath, snapshotTargetPath); err != nil {
		return errors.Errorf("copy archived snapshot error: %v", err)
	}
	originSnapshotStorage, err := storagemgr.Open(archiver.archiveTrieSnapshotOriginPath)
	if err != nil {
		return errors.Errorf("reopen snapshot storage error: %v", err)
	}

	archiver.archiveTrieSnapshotBackend = originSnapshotStorage
	archiver.lastArchiveBlock = blockHeader.Number

	archiver.logger.Infof("[Archive] archive history at height: %v, time: %v", blockHeader.Number, time.Since(cur))
	return nil
}

func (archiver *Archiver) UpdateChainState(chainState *chainstate.ChainState) {
	archiver.chainState = chainState
}

func (archiver *Archiver) GetHistoryBackend() kv.Storage {
	return archiver.archiveHistoryBackend
}

func (archiver *Archiver) GetStateJournal(height uint64) *types.StateJournal {
	data := archiver.archiveJournalBackend.Get(utils.CompositeKey(utils.PruneJournalKey, height))
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

// todo confirm archiver may need rollback

func copyDir(src, dest string) error {
	files, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, file := range files {
		srcPath := filepath.Join(src, file.Name())
		destPath := filepath.Join(dest, file.Name())

		if file.IsDir() {
			if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
				return errors.Errorf("mkdir %s dir error: %v", destPath, err.Error())
			}
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = srcFile.Close()
	}()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() {
		_ = destFile.Close()
	}()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}
