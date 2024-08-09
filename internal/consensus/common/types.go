package common

import (
	"github.com/pkg/errors"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
)

const (
	LocalTxEvent = iota
	RemoteTxEvent
)

const (
	Batch     timer.TimeoutEvent = "Batch"
	NoTxBatch timer.TimeoutEvent = "NoTxBatch"
)

var (
	ErrorPreCheck       = errors.New("precheck failed")
	ErrorAddTxPool      = errors.New("add txpool failed")
	ErrorConsensusStart = errors.New("consensus not start yet")
)

var DataSyncerPipeName = []string{
	"NULL_REQUEST",           // primary heartbeat
	"PRE_PREPARE",            // get batch
	"SIGNED_CHECKPOINT",      // get checkpoint
	"SYNC_STATE_RESPONSE",    // get quorum state
	"FETCH_MISSING_RESPONSE", // get missing txs in local pool
	"EPOCH_CHANGE_PROOF",     // get epoch change for state update
}

var DataSyncerRequestName = []string{
	"SYNC_STATE",            // get quorum state
	"FETCH_MISSING_REQUEST", // get missing txs in local pool
	"EPOCH_CHANGE_REQUEST",  // get epoch change for state update
}

// UncheckedTxEvent represents misc event sent by local modules
type UncheckedTxEvent struct {
	EventType int
	Event     any
}

type TxWithResp struct {
	Tx      *types.Transaction
	CheckCh chan *TxResp
	PoolCh  chan *TxResp
}

type TxResp struct {
	Status   bool
	ErrorMsg string
}

type CommitEvent struct {
	Block                  *types.Block
	StateUpdatedCheckpoint *Checkpoint
	StateJournal           *types.StateJournal
	Receipts               []*types.Receipt
}

type Checkpoint struct {
	Epoch  uint64
	Height uint64
	Digest string
}
