package utils

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/axiomesh/axiom-bft/common/consensus"
	"strconv"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
)

const (
	BlockHashKey       = "block-hash-"
	BlockTxSetKey      = "block-tx-set-"
	TransactionMetaKey = "tx-meta-"
	ChainMetaKey       = "chain-meta"
	PruneJournalKey    = "prune-nodeInfo-"
	SnapshotKey        = "snap-"
	SnapshotMetaKey    = "snap-meta"
	RollbackBlockKey   = "rollback-block"
	RollbackStateKey   = "rollback-state"
	TrieNodeIndexKey   = "tni-"
	ArchiveKey         = "archive-"
)

const (
	MinHeightStr = "minHeight"
	MaxHeightStr = "maxHeight"
)

var keyCache = storagemgr.NewCacheWrapper(64, false)

func CompositeKey(prefix string, value any) []byte {
	return append([]byte(prefix), []byte(fmt.Sprintf("%v", value))...)
}

func CompositeAccountKey(addr *types.Address) []byte {
	k := addr.Bytes()
	if res, ok := keyCache.Get(k); ok {
		return res
	}
	res := hexutil.EncodeToNibbles(addr.String())
	keyCache.Set(k, res)
	return res
}

func CompositeStorageKey(addr *types.Address, key []byte) []byte {
	k := append(addr.Bytes(), key...)
	if res, ok := keyCache.Get(k); ok {
		return res
	}
	keyHash := sha256.Sum256(k)
	res := hexutil.EncodeToNibbles(types.NewHash(keyHash[:]).String())
	keyCache.Set(k, res)
	return res
}

func CompositeCodeKey(addr *types.Address, codeHash []byte) []byte {
	return append(addr.Bytes(), codeHash...)
}

func MarshalUint64(data uint64) []byte {
	return []byte(strconv.FormatUint(data, 10))
}

func UnmarshalUint64(data []byte) uint64 {
	res, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		panic(err)
	}

	return res
}

type SnapshotMeta struct {
	BlockHeader *types.BlockHeader
	EpochInfo   *types.EpochInfo
	Nodes       *consensus.QuorumValidators
}

type snapshotMetaMarshalHelper struct {
	BlockHeader []byte `json:"block_header"`
	EpochInfo   []byte `json:"epoch_info"`
	Nodes       []byte `json:"nodes"`
}

func (m *SnapshotMeta) Marshal() ([]byte, error) {
	blockHeader, err := m.BlockHeader.Marshal()
	if err != nil {
		return nil, err
	}
	epochInfo, err := m.EpochInfo.Marshal()
	if err != nil {
		return nil, err
	}
	nodes, err := m.Nodes.MarshalVTStrict()
	if err != nil {
		return nil, err
	}

	return json.Marshal(&snapshotMetaMarshalHelper{
		BlockHeader: blockHeader,
		EpochInfo:   epochInfo,
		Nodes:       nodes,
	})
}

func (m *SnapshotMeta) Unmarshal(data []byte) error {
	var helper snapshotMetaMarshalHelper
	if err := json.Unmarshal(data, &helper); err != nil {
		return err
	}

	blockHeader := &types.BlockHeader{}
	err := blockHeader.Unmarshal(helper.BlockHeader)
	if err != nil {
		return err
	}
	epochInfo := &types.EpochInfo{}
	err = epochInfo.Unmarshal(helper.EpochInfo)
	if err != nil {
		return err
	}

	nodes := &consensus.QuorumValidators{}
	err = nodes.UnmarshalVT(helper.Nodes)
	if err != nil {
		return err
	}

	m.BlockHeader = blockHeader
	m.EpochInfo = epochInfo
	m.Nodes = nodes

	return nil
}
