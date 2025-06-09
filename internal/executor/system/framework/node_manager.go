package framework

import (
	"math"
	"math/big"
	"sort"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager_client"
	"github.com/axiomesh/axiom-ledger/pkg/crypto"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	maxMetaDataLength = 256

	nextNodeIDStorageKey        = "nextNodeID"
	nodeRegistry                = "nodeRegistry"
	nodeP2PIDIndex              = "nodeP2PIDIndex"
	nodeConsensusPubKeyIndex    = "nodeConsensusPubKeyIndex"
	nodeNameIndex               = "nodeNameIndex"
	nextEpochUpdateNodes        = "nextEpochUpdateNodes"
	nextEpochUpdatedNodeIDSet   = "nextEpochUpdatedNodeIDSet"
	statusPrefix                = "status"
	activeValidatorVotingPowers = "activeValidatorVotingPowers"
)

var (
	ErrNodeNotFound            = errors.New("node not found")
	ErrStatusSetNotFound       = errors.New("status set not found")
	ErrNodeNotFoundInStatusSet = errors.New("node not found in status set")
)

var NodeManagerBuildConfig = &common.SystemContractBuildConfig[*NodeManager]{
	Name:    "framework_node_manager",
	Address: common.NodeManagerContractAddr,
	AbiStr:  node_manager_client.BindingContractMetaData.ABI,
	Constructor: func(systemContractBase common.SystemContractBase) *NodeManager {
		return &NodeManager{
			SystemContractBase: systemContractBase,
		}
	},
}

var _ node_manager.NodeManager = (*NodeManager)(nil)

type NodeManager struct {
	common.SystemContractBase

	nextNodeID *common.VMSlot[uint64]

	// store all registered node info (including exited nodes)
	nodeRegistry *common.VMMap[uint64, node_manager.NodeInfo]

	// p2p id to node id
	nodeP2PIDIndex *common.VMMap[string, uint64]

	// node.ConsensusPubKey -> NodeID
	nodeConsensusPubKeyIndex *common.VMMap[string, uint64]

	// node.MetaData.Name -> NodeID
	nodeNameIndex *common.VMMap[string, uint64]

	// track all updated node id in the next epoch, will clean up in the next epoch
	nextEpochUpdatedNodeIDSet *common.VMSlot[[]uint64]

	// store all new node info in the next epoch, will clean up in the next epoch
	nextEpochUpdateNodes *common.VMMap[uint64, node_manager.NodeInfo]

	// including Candidate and ActiveValidator
	activeValidatorVotingPowers *common.VMSlot[[]node_manager.ConsensusVotingPower]
}

type sortNodeInfo struct {
	stakeValue *big.Int
	node_manager.NodeInfo
}

type sortNodeInfos []sortNodeInfo

func (s sortNodeInfos) Len() int {
	return len(s)
}

func (s sortNodeInfos) Less(i int, j int) bool {
	return s[i].stakeValue.Cmp(s[j].stakeValue) < 0
}

func (s sortNodeInfos) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

func (n *NodeManager) GenesisInit(genesis *repo.GenesisConfig) error {
	var activeValidators []uint64
	var pendingValidators []uint64
	var candidates []uint64
	var dataSyncers []uint64

	var needCreateStakingPoolIDs []uint64

	minValidatorStake := genesis.EpochInfo.StakeParams.MinValidatorStake.ToBigInt()
	maxValidatorStake := genesis.EpochInfo.StakeParams.MaxValidatorStake.ToBigInt()
	// check node info
	nodeInfoMap := make(map[uint64]*node_manager.NodeInfo, len(genesis.Nodes))
	nodeCfgMap := make(map[uint64]*repo.GenesisNodeInfo, len(genesis.Nodes))
	nodeStakeNumberMap := make(map[uint64]*big.Int, len(genesis.Nodes))
	for i, nodeCfg := range genesis.Nodes {
		nodeInfo := &node_manager.NodeInfo{
			ID:              uint64(i + 1),
			ConsensusPubKey: nodeCfg.ConsensusPubKey,
			P2PPubKey:       nodeCfg.P2PPubKey,
			MetaData: node_manager.NodeMetaData{
				Name:       nodeCfg.MetaData.Name,
				Desc:       nodeCfg.MetaData.Desc,
				ImageURL:   nodeCfg.MetaData.ImageURL,
				WebsiteURL: nodeCfg.MetaData.WebsiteURL,
			},
		}

		err := func() error {
			if nodeCfg.IsDataSyncer && nodeCfg.StakeNumber.ToBigInt().Sign() != 0 {
				return errors.Errorf("invalid stake number %s for data syncer, must be 0", nodeCfg.StakeNumber.String())
			}

			if !ethcommon.IsHexAddress(nodeCfg.OperatorAddress) {
				return errors.New("invalid operator address")
			}
			nodeInfo.Operator = ethcommon.HexToAddress(nodeCfg.OperatorAddress)

			if nodeCfg.CommissionRate > CommissionRateDenominator {
				return errors.Errorf("invalid commission rate %d, need <= %d", nodeCfg.CommissionRate, CommissionRateDenominator)
			}

			stakeNumber := nodeCfg.StakeNumber.ToBigInt()
			if stakeNumber.Cmp(maxValidatorStake) > 0 {
				return errors.Errorf("invalid stake number %s, need <= %s", stakeNumber, maxValidatorStake)
			}
			nodeStakeNumberMap[nodeInfo.ID] = stakeNumber

			_, _, p2pID, err := CheckNodeInfo(*nodeInfo)
			if err != nil {
				return err
			}
			nodeInfo.P2PID = p2pID
			return nil
		}()
		if err != nil {
			return errors.Wrapf(err, "failed to check genesis node %d", nodeInfo.ID)
		}

		nodeInfoMap[nodeInfo.ID] = nodeInfo
		nodeCfgMap[nodeInfo.ID] = &nodeCfg
		if nodeCfg.IsDataSyncer {
			dataSyncers = append(dataSyncers, nodeInfo.ID)
		} else {
			if minValidatorStake.Cmp(nodeStakeNumberMap[nodeInfo.ID]) <= 0 {
				pendingValidators = append(pendingValidators, nodeInfo.ID)
			} else {
				candidates = append(candidates, nodeInfo.ID)
			}

			needCreateStakingPoolIDs = append(needCreateStakingPoolIDs, nodeInfo.ID)
		}
	}

	// select active validators
	sort.Slice(pendingValidators, func(i, j int) bool {
		cmpRes := nodeStakeNumberMap[pendingValidators[i]].Cmp(nodeStakeNumberMap[pendingValidators[j]])
		if cmpRes == 0 {
			return nodeInfoMap[pendingValidators[i]].ID < nodeInfoMap[pendingValidators[j]].ID
		}
		return cmpRes == 1
	})

	if uint64(len(pendingValidators)) <= genesis.EpochInfo.ConsensusParams.MaxValidatorNum {
		activeValidators = append(activeValidators, pendingValidators...)
	} else {
		activeValidators = append(activeValidators, pendingValidators[:genesis.EpochInfo.ConsensusParams.MaxValidatorNum]...)
		candidates = append(candidates, pendingValidators[genesis.EpochInfo.ConsensusParams.MaxValidatorNum:]...)
	}

	// sort candidates
	sort.Slice(candidates, func(i, j int) bool {
		cmpRes := nodeStakeNumberMap[candidates[i]].Cmp(nodeStakeNumberMap[candidates[j]])
		if cmpRes == 0 {
			return nodeInfoMap[candidates[i]].ID < nodeInfoMap[candidates[j]].ID
		}
		return cmpRes == 1
	})
	// sort dataSyncers
	sort.Slice(dataSyncers, func(i, j int) bool {
		return dataSyncers[i] < dataSyncers[j]
	})

	// register node info
	registerNodesFn := func(nodeIDs []uint64, nodeStatus types.NodeStatus) error {
		for _, nodeID := range nodeIDs {
			nodeInfo := nodeInfoMap[nodeID]
			nodeInfo.Status = uint8(nodeStatus)
			if _, err := n.register(*nodeInfo, true); err != nil {
				return errors.Wrapf(err, "failed to register node %d", nodeID)
			}
		}
		return nil
	}
	if err := registerNodesFn(activeValidators, types.NodeStatusActive); err != nil {
		return err
	}
	if err := registerNodesFn(candidates, types.NodeStatusCandidate); err != nil {
		return err
	}
	if err := registerNodesFn(dataSyncers, types.NodeStatusDataSyncer); err != nil {
		return err
	}

	// update others
	if err := n.nextNodeID.Put(uint64(len(genesis.Nodes) + 1)); err != nil {
		return err
	}

	if err := n.getStatusSet(types.NodeStatusActive).Put(activeValidators); err != nil {
		return err
	}
	if err := n.getStatusSet(types.NodeStatusCandidate).Put(candidates); err != nil {
		return err
	}
	if err := n.getStatusSet(types.NodeStatusDataSyncer).Put(dataSyncers); err != nil {
		return err
	}

	axcUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	activeValidatorVotingPowers := lo.Map(activeValidators, func(item uint64, index int) node_manager.ConsensusVotingPower {
		stakeNumber := nodeStakeNumberMap[item]
		// convert unit `mol` to `axc`
		standardizedStakeNumber := new(big.Int).Div(stakeNumber, axcUnit)
		return node_manager.ConsensusVotingPower{
			NodeID:               item,
			ConsensusVotingPower: standardizedStakeNumber.Int64(),
		}
	})
	if err := n.activeValidatorVotingPowers.Put(activeValidatorVotingPowers); err != nil {
		return err
	}

	return nil
}

func (n *NodeManager) SetContext(ctx *common.VMContext) {
	n.SystemContractBase.SetContext(ctx)

	n.nextNodeID = common.NewVMSlot[uint64](n.StateAccount, nextNodeIDStorageKey)
	n.nodeRegistry = common.NewVMMap[uint64, node_manager.NodeInfo](n.StateAccount, nodeRegistry, func(key uint64) string {
		return strconv.FormatUint(key, 10)
	})
	n.nodeP2PIDIndex = common.NewVMMap[string, uint64](n.StateAccount, nodeP2PIDIndex, func(key string) string {
		return key
	})
	n.nodeConsensusPubKeyIndex = common.NewVMMap[string, uint64](n.StateAccount, nodeConsensusPubKeyIndex, func(key string) string {
		return key
	})
	n.nodeNameIndex = common.NewVMMap[string, uint64](n.StateAccount, nodeNameIndex, func(key string) string {
		return key
	})

	n.nextEpochUpdateNodes = common.NewVMMap[uint64, node_manager.NodeInfo](n.StateAccount, nextEpochUpdateNodes, func(key uint64) string {
		return strconv.FormatUint(key, 10)
	})
	n.nextEpochUpdatedNodeIDSet = common.NewVMSlot[[]uint64](n.StateAccount, nextEpochUpdatedNodeIDSet)
	n.activeValidatorVotingPowers = common.NewVMSlot[[]node_manager.ConsensusVotingPower](n.StateAccount, activeValidatorVotingPowers)
}

func (n *NodeManager) Register(consensusPubKey string, p2pPubKey string, p2pID string, newOperator ethcommon.Address,
	name string, desc string, imageURL string, website string) (uint64, error) {
	status := uint8(types.NodeStatusDataSyncer)
	info := node_manager.NodeInfo{
		ConsensusPubKey: consensusPubKey,
		P2PPubKey:       p2pPubKey,
		P2PID:           p2pID,
		Operator:        newOperator,
		MetaData: node_manager.NodeMetaData{
			Name:       name,
			Desc:       desc,
			ImageURL:   imageURL,
			WebsiteURL: website,
		},
		Status: status,
	}
	id, err := n.register(info, false)
	if err != nil {
		return id, errors.Wrapf(err, "failed to register node with p2p id %s", info.P2PID)
	}
	info.ID = id
	n.EmitEvent(&node_manager.EventRegister{NodeID: info.ID, Info: info})
	return id, nil
}

func (n *NodeManager) register(info node_manager.NodeInfo, isGenesisInit bool) (id uint64, err error) {
	info = StandardizationNodeInfo(info)
	if n.nodeP2PIDIndex.Has(info.P2PID) {
		return 0, errors.Errorf("p2p id %s is already in use", info.P2PID)
	}
	if n.nodeConsensusPubKeyIndex.Has(info.ConsensusPubKey) {
		return 0, errors.Errorf("consensus public key %s is already in use", info.ConsensusPubKey)
	}

	if n.nodeNameIndex.Has(info.MetaData.Name) {
		return 0, errors.Errorf("name %s is already in use", info.MetaData.Name)
	}

	if !isGenesisInit {
		exist, id, err := n.nextNodeID.Get()
		if err != nil {
			return 0, err
		}
		if !exist {
			id = 1
		}
		info.ID = id
		info.Status = uint8(types.NodeStatusDataSyncer)

		if err = n.nextNodeID.Put(id + 1); err != nil {
			return 0, err
		}

		statusIDSet := n.getStatusSet(types.NodeStatusDataSyncer)
		isExist, set, err := statusIDSet.Get()
		if err != nil {
			return 0, err
		}
		if !isExist {
			set = []uint64{}
		}
		if err = statusIDSet.Put(append(set, id)); err != nil {
			return 0, err
		}
	}

	if err := n.nodeRegistry.Put(info.ID, info); err != nil {
		return 0, err
	}
	if err := n.nodeP2PIDIndex.Put(info.P2PID, info.ID); err != nil {
		return 0, err
	}
	if err := n.nodeConsensusPubKeyIndex.Put(info.ConsensusPubKey, info.ID); err != nil {
		return 0, err
	}
	if err := n.nodeNameIndex.Put(info.MetaData.Name, info.ID); err != nil {
		return 0, err
	}
	return info.ID, nil
}

// TurnIntoNewEpoch turn into new epoch, should be called after stakeManager epoch change
// since the active stake value will impact on the sort
func (n *NodeManager) TurnIntoNewEpoch(_ *types.EpochInfo, newEpoch *types.EpochInfo) error {
	// process node leave first
	exitedNodes, err := n.processNodeLeave()
	if err != nil {
		return err
	}
	// sort all candidates and validators
	nodesIDs, err := n.getConsensusCandidateNodeIDs()
	if err != nil {
		return err
	}
	// exclude exited nodes
	exitedNodeMap := lo.SliceToMap(exitedNodes, func(nodeID uint64) (uint64, struct{}) { return nodeID, struct{}{} })
	nodesIDs = lo.Reject(nodesIDs, func(item uint64, index int) bool {
		_, ok := exitedNodeMap[item]
		return ok
	})
	nodeInfos, err := n.GetInfos(nodesIDs)
	if err != nil {
		return err
	}
	sortInfos := make(sortNodeInfos, len(nodeInfos))
	stakeManager := StakingManagerBuildConfig.Build(n.CrossCallSystemContractContext())
	for i, info := range nodeInfos {
		stakeValue, err := stakeManager.PoolActiveStake(info.ID)
		if err != nil {
			return err
		}
		sortInfos[i] = sortNodeInfo{
			stakeValue: stakeValue,
			NodeInfo:   info,
		}
	}
	sort.Sort(sortInfos)
	maxValidator := newEpoch.ConsensusParams.MaxValidatorNum
	minValidator := newEpoch.ConsensusParams.MinValidatorNum
	minStakeValue := newEpoch.StakeParams.MinValidatorStake
	var validatorsIDs, candidatesIDs []uint64
	var validators, candidates []sortNodeInfo
	// traverse from highest to lowest
	for i := len(sortInfos) - 1; i >= 0; i-- {
		// only add to validator if it has enough stake and validator set is not full
		if sortInfos[i].stakeValue.Cmp(minStakeValue.ToBigInt()) >= 0 && uint64(len(validators)) < maxValidator {
			// put node into validator nodes
			validators = append(validators, sortInfos[i])
			// record relative node id
			validatorsIDs = append(validatorsIDs, sortInfos[i].ID)
		} else {
			// put node into candidate nodes
			candidates = append(candidates, sortInfos[i])
			// record relative node id
			candidatesIDs = append(candidatesIDs, sortInfos[i].ID)
		}
	}
	// revert if not enough validators
	if uint64(len(validators)) < minValidator {
		return errors.New("not enough validators")
	}

	// update role set
	if err = n.getStatusSet(types.NodeStatusCandidate).Put(candidatesIDs); err != nil {
		return err
	}
	if err = n.getStatusSet(types.NodeStatusActive).Put(validatorsIDs); err != nil {
		return err
	}

	// update activeValidatorVotingPowers
	axcUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	activeValidatorVotingPowers := lo.Map(validators, func(item sortNodeInfo, index int) node_manager.ConsensusVotingPower {
		// convert unit `mol` to `axc`
		standardizedStakeNumber := new(big.Int).Div(item.stakeValue, axcUnit)
		return node_manager.ConsensusVotingPower{
			NodeID:               item.ID,
			ConsensusVotingPower: standardizedStakeNumber.Int64(),
		}
	})
	if err := n.activeValidatorVotingPowers.Put(activeValidatorVotingPowers); err != nil {
		return err
	}

	// update node role if need
	for _, candidate := range candidates {
		if candidate.Status != uint8(types.NodeStatusCandidate) {
			candidate.Status = uint8(types.NodeStatusCandidate)
			if err := n.nodeRegistry.Put(candidate.ID, candidate.NodeInfo); err != nil {
				return err
			}
		}
	}
	for _, validator := range validators {
		if validator.Status != uint8(types.NodeStatusActive) {
			validator.Status = uint8(types.NodeStatusActive)
			if err := n.nodeRegistry.Put(validator.ID, validator.NodeInfo); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *NodeManager) processNodeLeave() ([]uint64, error) {
	pendingInactiveSlot := n.getStatusSet(types.NodeStatusPendingInactive)
	// get pending inactive sets
	isExist, nodeIDs, err := pendingInactiveSlot.Get()
	if err != nil {
		return nil, err
	}
	if !isExist {
		return []uint64{}, nil
	}
	for _, id := range nodeIDs {
		// get the node
		info, err := n.GetInfo(id)
		if err != nil {
			return nil, err
		}
		if err = n.operatorTransferStatus(types.NodeStatusPendingInactive, types.NodeStatusExited, &info); err != nil {
			return nil, err
		}
	}
	return nodeIDs, nil
}

func (n *NodeManager) getConsensusCandidateNodeIDs() ([]uint64, error) {
	isExist, candidateIDs, err := n.getStatusSet(types.NodeStatusCandidate).Get()
	if err != nil {
		return nil, err
	}
	if !isExist {
		candidateIDs = []uint64{}
	}
	isExist, validateIDs, err := n.getStatusSet(types.NodeStatusActive).Get()
	if err != nil {
		return nil, err
	}
	if !isExist {
		validateIDs = []uint64{}
	}
	nodeIDs := append(candidateIDs, validateIDs...)
	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})
	return nodeIDs, nil
}

func (n *NodeManager) checkPermission(nodeID uint64) (*node_manager.NodeInfo, error) {
	nodeInfo, err := n.GetInfo(nodeID)
	if err != nil {
		return nil, err
	}

	if !n.Ctx.CallFromSystem && n.Ctx.From != nodeInfo.Operator {
		return nil, errors.New("no permission")
	}
	return &nodeInfo, nil
}

func (n *NodeManager) JoinCandidateSet(nodeID uint64, commissionRate uint64) error {
	nodeInfo, err := n.checkPermission(nodeID)
	if err != nil {
		return err
	}
	if err = n.operatorTransferStatus(types.NodeStatusDataSyncer, types.NodeStatusCandidate, nodeInfo); err != nil {
		return err
	}

	lstID, err := StakingManagerBuildConfig.Build(n.CrossCallSystemContractContext()).CreatePool(nodeID, commissionRate)
	if err != nil {
		return errors.Wrapf(err, "failed to create staking pool for node %d", nodeID)
	}

	n.EmitEvent(&node_manager.EventJoinedCandidateSet{
		NodeID:                       nodeID,
		CommissionRate:               commissionRate,
		OperatorLiquidStakingTokenID: lstID,
	})

	return nil
}

func (n *NodeManager) Exit(nodeID uint64) error {
	nodeInfo, err := n.checkPermission(nodeID)
	if err != nil {
		return err
	}
	switch types.NodeStatus(nodeInfo.Status) {
	case types.NodeStatusActive:
		// get active set
		activeSet, err := n.getStatusSet(types.NodeStatusActive).MustGet()
		if err != nil {
			return err
		}
		// get pending inactive set
		isExist, pendingInactiveSet, err := n.getStatusSet(types.NodeStatusPendingInactive).Get()
		if err != nil {
			return err
		}
		if !isExist {
			pendingInactiveSet = []uint64{}
		}
		// get epoch info
		epochInfo, err := EpochManagerBuildConfig.Build(n.CrossCallSystemContractContext()).CurrentEpoch()
		if err != nil {
			return err
		}
		// calculate the max pending inactive number
		maxPendingInactiveNumberFloat, _ := new(big.Float).Mul(
			big.NewFloat(float64(len(activeSet))),
			new(big.Float).Quo(
				new(big.Float).SetUint64(epochInfo.StakeParams.MaxPendingInactiveValidatorRatio),
				big.NewFloat(CommissionRateDenominator),
			)).Float64()
		maxPendingInactiveNumber := uint64(math.Floor(maxPendingInactiveNumberFloat))
		if maxPendingInactiveNumber == 0 {
			maxPendingInactiveNumber = 1
		}
		// check if the pending inactive set still has sparse space to leave
		if uint64(len(pendingInactiveSet)) >= maxPendingInactiveNumber {
			return n.Revert(&node_manager.ErrorPendingInactiveSetIsFull{})
		}
		if uint64(len(activeSet))-1 < epochInfo.ConsensusParams.MinValidatorNum {
			return n.Revert(&node_manager.ErrorNotEnoughValidator{
				CurNum: big.NewInt(int64(len(activeSet))),
				MinNum: big.NewInt(int64(epochInfo.ConsensusParams.MinValidatorNum)),
			})
		}
		// transfer the status
		if err = n.operatorTransferStatus(types.NodeStatusActive, types.NodeStatusPendingInactive, nodeInfo); err != nil {
			return err
		}

		validatorIDSet, err := n.GetActiveValidatorIDSet()
		if err != nil {
			return err
		}
		pendingInactiveIDSet, err := n.GetPendingInactiveIDSet()
		if err != nil {
			return err
		}
		// check remain validators
		stakingManager := StakingManagerBuildConfig.Build(n.CrossCallSystemContractContext())
		if err := stakingManager.checkNextEpochValidatorActiveStake(epochInfo, len(validatorIDSet), map[uint64]struct{}{}, lo.SliceToMap(pendingInactiveIDSet, func(item uint64) (uint64, struct{}) {
			return item, struct{}{}
		})); err != nil {
			return err
		}

		n.EmitEvent(&node_manager.EventExit{NodeID: nodeID})
		return nil
	case types.NodeStatusCandidate, types.NodeStatusDataSyncer:
		if err = n.operatorTransferStatus(types.NodeStatus(nodeInfo.Status), types.NodeStatusExited, nodeInfo); err != nil {
			return err
		}
		n.EmitEvent(&node_manager.EventExit{NodeID: nodeID})
		return nil
	default:
		return n.Revert(&node_manager.ErrorIncorrectStatus{Status: nodeInfo.Status})
	}
}

func (n *NodeManager) UpdateMetaData(nodeID uint64, name string, desc string, imageURL string, websiteURL string) error {
	metaData := node_manager.NodeMetaData{
		Name:       name,
		Desc:       desc,
		ImageURL:   imageURL,
		WebsiteURL: websiteURL,
	}
	nodeInfo, err := n.checkPermission(nodeID)
	if err != nil {
		return err
	}
	oldName := nodeInfo.MetaData.Name
	if metaData.Name == "" {
		return errors.New("name cannot be empty")
	}

	if n.nodeNameIndex.Has(metaData.Name) && metaData.Name != oldName {
		return errors.Errorf("name %s is already in use", metaData.Name)
	}

	nodeInfo.MetaData = metaData
	if err = n.nodeRegistry.Put(nodeID, *nodeInfo); err != nil {
		return err
	}

	// rebuild name index
	if metaData.Name != oldName {
		if err := n.nodeNameIndex.Delete(oldName); err != nil {
			return err
		}
		if err := n.nodeNameIndex.Put(nodeInfo.MetaData.Name, nodeID); err != nil {
			return err
		}
	}

	n.EmitEvent(&node_manager.EventUpdateMetaData{NodeID: nodeID, MetaData: metaData})
	return nil
}

func (n *NodeManager) UpdateOperator(nodeID uint64, newOperator ethcommon.Address) error {
	nodeInfo, err := n.checkPermission(nodeID)
	if err != nil {
		return err
	}
	if common.IsZeroAddress(newOperator) {
		return errors.New("new operator cannot be zero address")
	}
	nodeInfo.Operator = newOperator
	if err = n.nodeRegistry.Put(nodeID, *nodeInfo); err != nil {
		return err
	}
	n.EmitEvent(&node_manager.EventUpdateOperator{NodeID: nodeID, NewOperator: newOperator})
	return nil
}

func (n *NodeManager) GetInfoByConsensusPubKey(consensusPubKey string) (node_manager.NodeInfo, error) {
	isExist, nodeID, err := n.nodeConsensusPubKeyIndex.Get(consensusPubKey)
	if err != nil {
		return node_manager.NodeInfo{}, err
	}
	if !isExist {
		return node_manager.NodeInfo{}, ErrNodeNotFound
	}

	return n.GetInfo(nodeID)
}

func (n *NodeManager) GetInfo(nodeID uint64) (info node_manager.NodeInfo, err error) {
	isExist, nodeInfo, err := n.nodeRegistry.Get(nodeID)
	if err != nil {
		return node_manager.NodeInfo{}, err
	}
	if !isExist {
		return node_manager.NodeInfo{}, ErrNodeNotFound
	}
	return nodeInfo, nil
}

func (n *NodeManager) GetNodeIDByP2PID(p2pID string) (uint64, error) {
	isExist, nodeID, err := n.nodeP2PIDIndex.Get(p2pID)
	if err != nil {
		return 0, err
	}
	if !isExist {
		return 0, ErrNodeNotFound
	}
	return nodeID, nil
}

func (n *NodeManager) ExistNodeByP2PID(p2pID string) bool {
	return n.nodeP2PIDIndex.Has(p2pID)
}

func (n *NodeManager) ExistNodeByConsensusPubKey(consensusPubKey string) bool {
	return n.nodeConsensusPubKeyIndex.Has(hexutil.Encode(hexutil.Decode(consensusPubKey)))
}

func (n *NodeManager) ExistNodeByName(nodeName string) bool {
	return n.nodeNameIndex.Has(nodeName)
}

func (n *NodeManager) GetTotalCount() (uint64, error) {
	_, id, err := n.nextNodeID.Get()
	if err != nil {
		return 0, err
	}
	return id - 1, nil
}

func (n *NodeManager) GetInfos(nodeIDs []uint64) (infos []node_manager.NodeInfo, err error) {
	infos = []node_manager.NodeInfo{}
	for _, id := range nodeIDs {
		info, err := n.GetInfo(id)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (n *NodeManager) GetActiveValidatorSet() (infos []node_manager.NodeInfo, votingPowers []node_manager.ConsensusVotingPower, err error) {
	isExists, innerVotingPowers, err := n.activeValidatorVotingPowers.Get()
	if err != nil {
		return nil, nil, err
	}
	if !isExists {
		return []node_manager.NodeInfo{}, []node_manager.ConsensusVotingPower{}, nil
	}

	for i := 0; i < len(innerVotingPowers); i++ {
		info, err := n.GetInfo(innerVotingPowers[i].NodeID)
		if err != nil {
			return nil, nil, err
		}

		infos = append(infos, info)
		votingPowers = append(votingPowers, node_manager.ConsensusVotingPower{
			NodeID:               innerVotingPowers[i].NodeID,
			ConsensusVotingPower: innerVotingPowers[i].ConsensusVotingPower,
		})
	}
	return infos, votingPowers, nil
}

func (n *NodeManager) GetDataSyncerSet() (infos []node_manager.NodeInfo, err error) {
	nodes, err := n.GetDataSyncerIDSet()
	if err != nil {
		return nil, err
	}
	return n.GetInfos(nodes)
}

func (n *NodeManager) GetCandidateSet() (infos []node_manager.NodeInfo, err error) {
	nodes, err := n.GetCandidateIDSet()
	if err != nil {
		return nil, err
	}
	return n.GetInfos(nodes)
}

func (n *NodeManager) GetPendingInactiveSet() (infos []node_manager.NodeInfo, err error) {
	nodes, err := n.GetPendingInactiveIDSet()
	if err != nil {
		return nil, err
	}
	return n.GetInfos(nodes)
}

func (n *NodeManager) GetExitedSet() (infos []node_manager.NodeInfo, err error) {
	nodes, err := n.GetExitedIDSet()
	if err != nil {
		return nil, err
	}
	return n.GetInfos(nodes)
}

func (n *NodeManager) GetActiveValidatorVotingPowers() (votingPowers []node_manager.ConsensusVotingPower, err error) {
	votingPowers, err = n.activeValidatorVotingPowers.MustGet()
	if err != nil {
		return nil, err
	}
	return votingPowers, nil
}

func (n *NodeManager) GetActiveValidatorIDSet() ([]uint64, error) {
	isExists, innerVotingPowers, err := n.activeValidatorVotingPowers.Get()
	if err != nil {
		return nil, err
	}
	if !isExists {
		return []uint64{}, nil
	}
	return lo.Map(innerVotingPowers, func(item node_manager.ConsensusVotingPower, index int) uint64 { return item.NodeID }), nil
}

func (n *NodeManager) GetCandidateIDSet() ([]uint64, error) {
	isExists, nodes, err := n.getStatusSet(types.NodeStatusCandidate).Get()
	if err != nil {
		return nil, err
	}
	if !isExists {
		return []uint64{}, nil
	}
	return nodes, nil
}

func (n *NodeManager) GetDataSyncerIDSet() ([]uint64, error) {
	isExists, nodes, err := n.getStatusSet(types.NodeStatusDataSyncer).Get()
	if err != nil {
		return nil, err
	}
	if !isExists {
		return []uint64{}, nil
	}
	return nodes, nil
}

func (n *NodeManager) GetExitedIDSet() ([]uint64, error) {
	isExists, nodes, err := n.getStatusSet(types.NodeStatusExited).Get()
	if err != nil {
		return nil, err
	}
	if !isExists {
		return []uint64{}, nil
	}
	return nodes, nil
}

func (n *NodeManager) GetPendingInactiveIDSet() ([]uint64, error) {
	isExists, nodes, err := n.getStatusSet(types.NodeStatusPendingInactive).Get()
	if err != nil {
		return nil, err
	}
	if !isExists {
		return []uint64{}, nil
	}
	return nodes, nil
}

func (n *NodeManager) operatorTransferStatus(from, to types.NodeStatus, nodeInfo *node_manager.NodeInfo) error {
	if from == to {
		return errors.New("from and to status cannot be the same")
	}
	// check status
	if nodeInfo.Status != uint8(from) {
		return n.Revert(&node_manager.ErrorIncorrectStatus{Status: uint8(from)})
	}
	// update status
	nodeInfo.Status = uint8(to)
	// update nodeInfo
	if err := n.nodeRegistry.Put(nodeInfo.ID, *nodeInfo); err != nil {
		return err
	}
	// get from status set, e.g. candidate
	fromSlot := n.getStatusSet(from)
	fromSets, err := fromSlot.MustGet()
	if err != nil {
		return err
	}
	// find node from the set and remove the node
	index := -1
	for i, id := range fromSets {
		if id == nodeInfo.ID {
			index = i
			break
		}
	}
	if index == -1 {
		return ErrNodeNotFoundInStatusSet
	}
	fromSets = append(fromSets[:index], fromSets[index+1:]...)
	// update the set
	if err = fromSlot.Put(fromSets); err != nil {
		return err
	}
	// get to status set, e.g. pending inactive
	toSlot := n.getStatusSet(to)
	isExist, toSets, err := toSlot.Get()
	if err != nil {
		return err
	}
	if !isExist {
		toSets = []uint64{}
	}
	if err = toSlot.Put(append(toSets, nodeInfo.ID)); err != nil {
		return err
	}

	// if transfer status from active and candidate state, should disable the pool first
	if to == types.NodeStatusExited && (from == types.NodeStatusPendingInactive || from == types.NodeStatusCandidate) {
		// get staking manager
		stakingManager := StakingManagerBuildConfig.Build(n.CrossCallSystemContractContext())
		return stakingManager.DisablePool(nodeInfo.ID)
	}
	return nil
}

func (n *NodeManager) getStatusSet(status types.NodeStatus) *common.VMSlot[[]uint64] {
	return common.NewVMSlot[[]uint64](n.StateAccount, statusPrefix+strconv.Itoa(int(status)))
}

func (n *NodeManager) TestPutNodeInfo(nodeInfo *node_manager.NodeInfo) (err error) {
	return n.nodeRegistry.Put(nodeInfo.ID, *nodeInfo)
}

func StandardizationNodeInfo(info node_manager.NodeInfo) node_manager.NodeInfo {
	return node_manager.NodeInfo{
		ID:              info.ID,
		ConsensusPubKey: hexutil.Encode(hexutil.Decode(info.ConsensusPubKey)),
		P2PPubKey:       hexutil.Encode(hexutil.Decode(info.P2PPubKey)),
		P2PID:           info.P2PID,
		Operator:        info.Operator,
		MetaData: node_manager.NodeMetaData{
			Name:       info.MetaData.Name,
			Desc:       info.MetaData.Desc,
			ImageURL:   info.MetaData.ImageURL,
			WebsiteURL: info.MetaData.WebsiteURL,
		},
		Status: info.Status,
	}
}

func CheckNodeInfo(nodeInfo node_manager.NodeInfo) (consensusPubKey *crypto.Bls12381PublicKey, p2pPubKey *crypto.Ed25519PublicKey, p2pID string, err error) {
	if len(nodeInfo.MetaData.Name) > maxMetaDataLength {
		return nil, nil, "", errors.Errorf("name length %d exceeds max length %d", len(nodeInfo.MetaData.Name), maxMetaDataLength)
	}
	if len(nodeInfo.MetaData.Desc) > maxMetaDataLength {
		return nil, nil, "", errors.Errorf("desc length %d exceeds max length %d", len(nodeInfo.MetaData.Desc), maxMetaDataLength)
	}
	if len(nodeInfo.MetaData.ImageURL) > maxMetaDataLength {
		return nil, nil, "", errors.Errorf("image_url length %d exceeds max length %d", len(nodeInfo.MetaData.ImageURL), maxMetaDataLength)
	}
	if len(nodeInfo.MetaData.WebsiteURL) > maxMetaDataLength {
		return nil, nil, "", errors.Errorf("website_url length %d exceeds max length %d", len(nodeInfo.MetaData.WebsiteURL), maxMetaDataLength)
	}

	if nodeInfo.MetaData.Name == "" {
		return nil, nil, "", errors.New("name cannot be empty")
	}

	consensusPubKey = &crypto.Bls12381PublicKey{}
	if err := consensusPubKey.Unmarshal(hexutil.Decode(nodeInfo.ConsensusPubKey)); err != nil {
		return nil, nil, "", errors.Wrap(err, "failed to unmarshal consensus public key")
	}

	p2pPubKey = &crypto.Ed25519PublicKey{}
	if err := p2pPubKey.Unmarshal(hexutil.Decode(nodeInfo.P2PPubKey)); err != nil {
		return nil, nil, "", errors.Wrap(err, "failed to unmarshal p2p public key")
	}
	p2pID, err = repo.P2PPubKeyToID(p2pPubKey)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "failed to calculate p2p id from p2p public key")
	}

	return consensusPubKey, p2pPubKey, p2pID, nil
}
