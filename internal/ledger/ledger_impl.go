package ledger

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/axiomesh/axiom-kit/storage/blockfile"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type Ledger struct {
	ChainLedger ChainLedger
	StateLedger StateLedger
	SnapMeta    atomic.Value
}

type BlockData struct {
	Block      *types.Block
	Receipts   []*types.Receipt
	TxHashList []*types.Hash
}

type SnapInfo struct {
	Status          bool
	SnapBlockHeader *types.BlockHeader
}

func NewLedgerWithStores(repo *repo.Repo, blockchainStore kv.Storage, ldb, snapshot kv.Storage, bf blockfile.BlockFile) (*Ledger, error) {
	var err error
	ledger := &Ledger{}
	if blockchainStore != nil || bf != nil {
		ledger.ChainLedger, err = newChainLedger(repo, blockchainStore, bf)
		if err != nil {
			return nil, fmt.Errorf("init chain ledger failed: %w", err)
		}
	} else {
		ledger.ChainLedger, err = NewChainLedger(repo, "")
		if err != nil {
			return nil, fmt.Errorf("init chain ledger failed: %w", err)
		}
	}
	//if ldb != nil {
	//	ledger.StateLedger, err = newStateLedger(repo, ldb, snapshot)
	//	if err != nil {
	//		return nil, fmt.Errorf("init state ledger failed: %w", err)
	//	}
	//} else {
	//	ledger.StateLedger, err = NewStateLedger(repo, "")
	//	if err != nil {
	//		return nil, fmt.Errorf("init state ledger failed: %w", err)
	//	}
	//}
	meta := ledger.ChainLedger.GetChainMeta()
	ledger.StateLedger = NewRustStateLedger(repo, meta.Height)

	if err := ledger.Rollback(meta.Height); err != nil {
		return nil, fmt.Errorf("rollback ledger to height %d failed: %w", meta.Height, err)
	}

	return ledger, nil
}

func NewMemory(repo *repo.Repo) (*Ledger, error) {
	return NewLedgerWithStores(repo, kv.NewMemory(), kv.NewMemory(), kv.NewMemory(), blockfile.NewMemory())
}

func NewLedger(rep *repo.Repo) (*Ledger, error) {
	return NewLedgerWithStores(rep, nil, nil, nil, nil)
}

// PersistBlockData persists block data
func (l *Ledger) PersistBlockData(blockData *BlockData) {
	current := time.Now()
	block := blockData.Block
	receipts := blockData.Receipts

	if err := l.ChainLedger.PersistExecutionResult(block, receipts); err != nil {
		panic(err)
	}
	getTransactionCounter.Set(0)
	persistBlockDuration.Observe(float64(time.Since(current)) / float64(time.Second))
	blockHeightMetric.Set(float64(block.Header.Number))
}

// Rollback rollback ledger to history version
func (l *Ledger) Rollback(height uint64) error {
	if l.ChainLedger.GetChainMeta().BlockHash == nil {
		return nil
	}
	blockHeader, err := l.ChainLedger.GetBlockHeader(height)
	if err != nil {
		return fmt.Errorf("rollback state to height %d failed: %w", height, err)
	}

	if err := l.StateLedger.RollbackState(height, blockHeader.StateRoot); err != nil {
		return fmt.Errorf("rollback state to height %d failed: %w", height, err)
	}

	if err := l.ChainLedger.RollbackBlockChain(height); err != nil {
		return fmt.Errorf("rollback block to height %d failed: %w", height, err)
	}

	blockHeightMetric.Set(float64(height))
	return nil
}

func (l *Ledger) Close() {
	l.ChainLedger.Close()
	l.StateLedger.Close()
}

// NewView load the latest state ledger and chain ledger by default
func (l *Ledger) NewView() *Ledger {
	snap := l.SnapMeta.Load()
	if snap != nil && snap.(SnapInfo).Status {
		snapBlockHeader := snap.(SnapInfo).SnapBlockHeader
		var sm atomic.Value
		sm.Store(snap)

		sl, err := l.StateLedger.NewView(snapBlockHeader, true)
		if err != nil {
			panic(err)
		}

		return &Ledger{
			ChainLedger: l.ChainLedger,
			StateLedger: sl,
			SnapMeta:    sm,
		}
	}

	meta := l.ChainLedger.GetChainMeta()
	block, err := l.ChainLedger.GetBlockHeader(meta.Height)
	if err != nil {
		panic(err)
	}
	sl, err := l.StateLedger.NewView(block, true)
	if err != nil {
		panic(err)
	}
	return &Ledger{
		ChainLedger: l.ChainLedger,
		StateLedger: sl,
	}
}
