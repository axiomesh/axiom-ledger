package chainstate

import (
	"sync"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
	"github.com/axiomesh/axiom-ledger/pkg/crypto"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

func NewMockChainState(genesisConfig *repo.GenesisConfig, epochMap map[uint64]*types.EpochInfo) *ChainState {
	return NewMockChainStateWithNodeID(genesisConfig, epochMap, 1)
}

func NewMockChainStateWithNodeID(genesisConfig *repo.GenesisConfig, epochMap map[uint64]*types.EpochInfo, selfNodeID uint64) *ChainState {
	nodeInfoMap := map[uint64]*ExpandedNodeInfo{}
	var validatorSet []ValidatorInfo
	for i, nodeInfo := range genesisConfig.Nodes {
		nodeID := uint64(i + 1)

		consensusPubKey := &crypto.Bls12381PublicKey{}
		if err := consensusPubKey.Unmarshal(hexutil.Decode(nodeInfo.ConsensusPubKey)); err != nil {
			panic(errors.Wrap(err, "failed to unmarshal consensus public key"))
		}

		p2pPubKey := &crypto.Ed25519PublicKey{}
		if err := p2pPubKey.Unmarshal(hexutil.Decode(nodeInfo.P2PPubKey)); err != nil {
			panic(errors.Wrap(err, "failed to unmarshal p2p public key"))
		}
		p2pID, err := repo.P2PPubKeyToID(p2pPubKey)
		if err != nil {
			panic(errors.Wrap(err, "failed to calculate p2p id from p2p public key"))
		}

		nodeInfoMap[nodeID] = &ExpandedNodeInfo{
			NodeInfo: node_manager.NodeInfo{
				ID:              nodeID,
				ConsensusPubKey: nodeInfo.ConsensusPubKey,
				P2PPubKey:       nodeInfo.P2PPubKey,
				P2PID:           p2pID,
				Operator:        ethcommon.HexToAddress(nodeInfo.OperatorAddress),
				MetaData: node_manager.NodeMetaData{
					Name:       nodeInfo.MetaData.Name,
					Desc:       nodeInfo.MetaData.Desc,
					ImageURL:   nodeInfo.MetaData.ImageURL,
					WebsiteURL: nodeInfo.MetaData.WebsiteURL,
				},
				Status: uint8(types.NodeStatusActive),
			},
			P2PPubKey:       p2pPubKey,
			ConsensusPubKey: consensusPubKey,
		}
		if !nodeInfo.IsDataSyncer {
			validatorSet = append(validatorSet, ValidatorInfo{ID: nodeID, ConsensusVotingPower: 1000})
		}
	}
	if epochMap == nil {
		epochMap = map[uint64]*types.EpochInfo{}
	}
	if _, ok := epochMap[genesisConfig.EpochInfo.Epoch]; !ok {
		epochMap[genesisConfig.EpochInfo.Epoch] = genesisConfig.EpochInfo
	}
	selfNodeInfo := nodeInfoMap[selfNodeID]
	isDataSyncer := genesisConfig.Nodes[selfNodeID-1].IsDataSyncer
	return &ChainState{
		nodeInfoCacheLock:     sync.RWMutex{},
		p2pID2NodeIDCacheLock: sync.RWMutex{},
		epochInfoCacheLock:    sync.RWMutex{},
		getNodeInfoFn: func(u uint64) (*node_manager.NodeInfo, error) {
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
		selfRegistered: true,
		EpochInfo:      genesisConfig.EpochInfo,
		ChainMeta: &types.ChainMeta{
			Height:    0,
			BlockHash: nil,
		},
		ValidatorSet: validatorSet,
		SelfNodeInfo: selfNodeInfo,
		IsDataSyncer: isDataSyncer,
		IsValidator:  !isDataSyncer,
	}
}
