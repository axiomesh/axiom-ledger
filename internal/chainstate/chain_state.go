package chainstate

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/pkg/crypto"
)

type ExpandedNodeInfo struct {
	types.NodeInfo
	P2PPubKey       *crypto.Ed25519PublicKey
	ConsensusPubKey *crypto.Bls12381PublicKey
}

type ValidatorInfo struct {
	ID                   uint64
	ConsensusVotingPower int64
}

type ChainState struct {
	nodeInfoCacheLock     *sync.RWMutex
	p2pID2NodeIDCacheLock *sync.RWMutex
	epochInfoCacheLock    *sync.RWMutex
	getNodeInfoFn         func(uint64) (*types.NodeInfo, error)
	getNodeIDByP2PIDFn    func(p2pID string) (uint64, error)
	getEpochInfoFn        func(epoch uint64) (*types.EpochInfo, error)
	nodeInfoCache         map[uint64]*ExpandedNodeInfo
	p2pID2NodeIDCache     map[string]uint64
	epochInfoCache        map[uint64]*types.EpochInfo
	selfRegistered        bool

	// states
	EpochInfo *types.EpochInfo

	ChainMeta    *types.ChainMeta
	ValidatorSet []ValidatorInfo
	SelfNodeInfo *ExpandedNodeInfo
	IsSyncing    bool
	IsValidator  bool
	IsDataSyncer bool
}

func NewChainState(p2pID string, p2pPubKey *crypto.Ed25519PublicKey, consensusPubKey *crypto.Bls12381PublicKey, getNodeInfoFn func(uint64) (*types.NodeInfo, error), getNodeIDByP2PIDFn func(p2pID string) (uint64, error), getEpochInfoFn func(epoch uint64) (*types.EpochInfo, error)) *ChainState {
	selfRegistered := false
	selfNodeInfo := &ExpandedNodeInfo{
		NodeInfo: types.NodeInfo{
			P2PID: p2pID,
		},
		P2PPubKey:       p2pPubKey,
		ConsensusPubKey: consensusPubKey,
	}
	selfNodeID, err := getNodeIDByP2PIDFn(p2pID)
	if err == nil {
		nodeInfo, err := getNodeInfoFn(selfNodeID)
		if err == nil {
			selfNodeInfo.NodeInfo = *nodeInfo
			selfRegistered = true
		}
	}

	return &ChainState{
		nodeInfoCacheLock:     &sync.RWMutex{},
		p2pID2NodeIDCacheLock: &sync.RWMutex{},
		epochInfoCacheLock:    &sync.RWMutex{},
		getNodeInfoFn:         getNodeInfoFn,
		getNodeIDByP2PIDFn:    getNodeIDByP2PIDFn,
		getEpochInfoFn:        getEpochInfoFn,
		nodeInfoCache:         make(map[uint64]*ExpandedNodeInfo),
		p2pID2NodeIDCache:     make(map[string]uint64),
		epochInfoCache:        make(map[uint64]*types.EpochInfo),
		selfRegistered:        selfRegistered,
		SelfNodeInfo:          selfNodeInfo,
	}
}

func NewMockChainState(selfNodeID uint64, nodeInfoMap map[uint64]*ExpandedNodeInfo, epochMap map[uint64]*types.EpochInfo) *ChainState {
	var currentEpochID uint64
	var currentEpoch *types.EpochInfo
	for epochID, epochInfo := range epochMap {
		if epochID > currentEpochID {
			currentEpochID = epochID
			currentEpoch = epochInfo
		}
	}

	return &ChainState{
		nodeInfoCacheLock:     &sync.RWMutex{},
		p2pID2NodeIDCacheLock: &sync.RWMutex{},
		epochInfoCacheLock:    &sync.RWMutex{},
		getNodeInfoFn: func(u uint64) (*types.NodeInfo, error) {
			return nil, errors.New("node not found")
		},
		getNodeIDByP2PIDFn: func(p2pID string) (uint64, error) {
			return 0, errors.New("node not found")
		},
		getEpochInfoFn: func(epoch uint64) (*types.EpochInfo, error) {
			return nil, errors.New("epoch not found")
		},
		nodeInfoCache: nodeInfoMap,
		p2pID2NodeIDCache: lo.MapEntries(nodeInfoMap, func(key uint64, value *ExpandedNodeInfo) (string, uint64) {
			return value.NodeInfo.P2PID, key
		}),
		epochInfoCache: epochMap,
		EpochInfo:      currentEpoch,
		SelfNodeInfo:   nodeInfoMap[selfNodeID],
	}
}

func (c *ChainState) UpdateChainMeta(chainMeta *types.ChainMeta) {
	c.ChainMeta = chainMeta
}

func (c *ChainState) UpdateByEpochInfo(epochInfo *types.EpochInfo, validatorSet map[uint64]int64) error {
	c.EpochInfo = epochInfo
	c.ValidatorSet = lo.MapToSlice(validatorSet, func(item uint64, key int64) ValidatorInfo {
		return ValidatorInfo{
			ID:                   item,
			ConsensusVotingPower: key,
		}
	})
	_, c.IsValidator = validatorSet[c.SelfNodeInfo.ID]
	return nil
}

func (c *ChainState) TryUpdateSelfNodeInfo() {
	if c.selfRegistered {
		return
	}
	selfNodeID, err := c.getNodeIDByP2PIDFn(c.SelfNodeInfo.P2PID)
	if err == nil {
		nodeInfo, err := c.getNodeInfoFn(selfNodeID)
		if err == nil {
			c.SelfNodeInfo.NodeInfo = *nodeInfo
			c.selfRegistered = true
		}
	}
}

func (c *ChainState) GetNodeInfo(nodeID uint64) (*ExpandedNodeInfo, error) {
	c.nodeInfoCacheLock.Lock()
	defer c.nodeInfoCacheLock.Unlock()
	expandedNodeInfo, ok := c.nodeInfoCache[nodeID]
	if ok {
		return expandedNodeInfo, nil
	}

	info, err := c.getNodeInfoFn(nodeID)
	if err != nil {
		return nil, err
	}
	p2pPubKey := crypto.Ed25519PublicKey{}
	if err := p2pPubKey.Unmarshal(hexutil.Decode(info.P2PPubKey)); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal p2p pub key for node %d", nodeID)
	}
	consensusPubKey := crypto.Bls12381PublicKey{}
	if err := consensusPubKey.Unmarshal(hexutil.Decode(info.ConsensusPubKey)); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal consensus pub key for node %d", nodeID)
	}
	expandedNodeInfo = &ExpandedNodeInfo{
		NodeInfo:        *info,
		P2PPubKey:       &p2pPubKey,
		ConsensusPubKey: &consensusPubKey,
	}
	c.nodeInfoCache[nodeID] = expandedNodeInfo
	c.p2pID2NodeIDCache[info.P2PID] = nodeID
	return expandedNodeInfo, nil
}

func (c *ChainState) GetNodeIDByP2PID(p2pID string) (uint64, error) {
	c.p2pID2NodeIDCacheLock.Lock()
	defer c.p2pID2NodeIDCacheLock.Unlock()
	nodeID, ok := c.p2pID2NodeIDCache[p2pID]
	if ok {
		return nodeID, nil
	}
	var err error
	nodeID, err = c.getNodeIDByP2PIDFn(p2pID)
	if err != nil {
		return 0, err
	}

	c.p2pID2NodeIDCache[p2pID] = nodeID
	return nodeID, nil
}

func (c *ChainState) GetEpochInfo(epoch uint64) (*types.EpochInfo, error) {
	c.epochInfoCacheLock.Lock()
	defer c.epochInfoCacheLock.Unlock()
	epochInfo, ok := c.epochInfoCache[epoch]
	if ok {
		return nil, nil
	}
	var err error
	epochInfo, err = c.getEpochInfoFn(epoch)
	if err != nil {
		return nil, err
	}

	c.epochInfoCache[epoch] = epochInfo
	return epochInfo, nil
}
