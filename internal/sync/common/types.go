package common

import (
	"time"

	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
)

const (
	SyncBlockRequestPipe  = "sync_block_pipe_v1_request"
	SyncBlockResponsePipe = "sync_block_pipe_v1_response"

	SyncChainDataRequestPipe  = "sync_chain_data_pipe_v1_request"
	SyncChainDataResponsePipe = "sync_chain_data_pipe_v1_response"

	MaxRetryCount = 5
)

type SyncMsgType int

const (
	SyncMsgType_InvalidBlock SyncMsgType = iota
	SyncMsgType_TimeoutBlock
	SyncMsgType_ErrorMsg
)

type SyncMode int

const (
	SyncModeFull SyncMode = iota
	SyncModeSnapshot
)

var SyncModeMap = map[SyncMode]string{
	SyncModeFull:     "full",
	SyncModeSnapshot: "snapshot",
}

type SyncParams struct {
	Peers            []*Node
	LatestBlockHash  string
	Quorum           uint64
	CurHeight        uint64
	TargetHeight     uint64
	QuorumCheckpoint *consensus.SignedCheckpoint
	EpochChanges     []*consensus.EpochChange
}

type LocalEvent struct {
	EventType int
	Event     any
}

// event type
const (
	EventType_InvalidMsg = iota
	EventType_GetSyncProgress
)

type GetSyncProgressReq struct {
	Resp chan *SyncProgress
}

type InvalidMsg struct {
	NodeID string
	Height uint64
	ErrMsg error
	Typ    SyncMsgType
}

type WrapperStateResp struct {
	PeerID string
	Hash   string
	Resp   *pb.SyncStateResponse
}

type Chunk struct {
	Time       time.Time
	ChunkSize  uint64
	CheckPoint *pb.CheckpointState
}

type Node struct {
	Id     uint64
	PeerID string
}

type Peer struct {
	Id           uint64
	PeerID       string
	LatestHeight uint64
	TimeoutCount uint64
}

type PrepareData struct {
	Data any
}

type WrapEpochChange struct {
	Epcs  []*consensus.EpochChange
	Error error
}

type SnapCommitData struct {
	Data       []CommitData
	EpochState *consensus.QuorumCheckpoint
}

type SyncMessage interface {
	MarshalVT() (dAtA []byte, err error)
	UnmarshalVT(dAtA []byte) error
	GetHeight() uint64
}

type SyncRequestMessage interface {
	SyncMessage
}

type SyncResponseMessage interface {
	SyncMessage
	GetStatus() pb.Status
	GetError() string
}

type CommitData interface {
	GetParentHash() string
	GetBlock() *types.Block
	GetHash() string
	GetHeight() uint64
	GetEpoch() uint64
}

var CommitDataRequestConstructor = map[SyncMode]func() SyncRequestMessage{
	SyncModeFull: func() SyncRequestMessage {
		return &pb.SyncBlockRequest{}
	},
	SyncModeSnapshot: func() SyncRequestMessage {
		return &pb.SyncChainDataRequest{}
	},
}

var CommitDataResponseConstructor = map[SyncMode]func() SyncResponseMessage{
	SyncModeFull: func() SyncResponseMessage {
		return &pb.SyncBlockResponse{}
	},
	SyncModeSnapshot: func() SyncResponseMessage {
		return &pb.SyncChainDataResponse{}
	},
}

var CommitDataResponseType = map[SyncMode]pb.Message_Type{
	SyncModeFull:     pb.Message_SYNC_BLOCK_RESPONSE,
	SyncModeSnapshot: pb.Message_SYNC_CHAIN_DATA_RESPONSE,
}

type BlockData struct {
	Block        *types.Block
	Receipts     []*types.Receipt
	StateJournal *types.StateJournal
}

func (b *BlockData) GetParentHash() string {
	return b.Block.Header.ParentHash.String()
}

func (b *BlockData) GetHash() string {
	return b.Block.Hash().String()
}

func (b *BlockData) GetHeight() uint64 {
	return b.Block.Height()
}

func (b *BlockData) GetEpoch() uint64 {
	return b.Block.Header.Epoch
}

func (b *BlockData) GetBlock() *types.Block {
	return b.Block
}

type ChainData struct {
	Block    *types.Block
	Receipts []*types.Receipt
}

func (c *ChainData) GetParentHash() string {
	return c.Block.Header.ParentHash.String()
}

func (c *ChainData) GetHash() string {
	return c.Block.Hash().String()
}

func (c *ChainData) GetHeight() uint64 {
	return c.Block.Height()
}

func (c *ChainData) GetEpoch() uint64 {
	return c.Block.Header.Epoch
}

func (c *ChainData) GetBlock() *types.Block {
	return c.Block
}

func (c *Chunk) FillCheckPoint(chunkMaxHeight uint64, checkpoint *pb.CheckpointState) {
	if chunkMaxHeight >= checkpoint.Height {
		c.CheckPoint = checkpoint
	}
}

// SyncProgress gives progress indications when the node is synchronising with other nodes
type SyncProgress struct {
	InSync             bool   `json:"inSync"`
	CatchUp            bool   `json:"catchUp"`
	StartSyncBlock     uint64 `json:"startSyncBlock"`     // Block number where sync began
	CurrentSyncHeight  uint64 `json:"currentSyncHeight"`  // Current block height where sync began
	HighestBlockHeight uint64 `json:"highestBlockHeight"` // Highest block height where persisted in ledger
	TargetHeight       uint64 `json:"targetHeight"`       // Target block height where sync ended
	SyncMode           string `json:"syncMode"`           // Sync mode (full or snapshot)
	Peers              []Node `json:"peers"`              // List of remote peers in sync
}
