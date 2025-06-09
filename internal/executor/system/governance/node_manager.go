package governance

//
//import (
//	"encoding/json"
//	"fmt"
//	"strconv"
//
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager_client"
//	ethcommon "github.com/ethereum/go-ethereum/common"
//	"github.com/pkg/errors"
//	"github.com/samber/lo"
//
//	"github.com/axiomesh/axiom-kit/hexutil"
//	"github.com/axiomesh/axiom-kit/types"
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
//	"github.com/axiomesh/axiom-ledger/pkg/repo"
//)
//
//const (
//	nodeManagerNamespace = "nodeManager"
//
//	pendingNameIndexStorageKey            = "pendingNameIndex"
//	pendingP2PIDIndexStorageKey           = "pendingP2PIDIndex"
//	pendingConsensusPubKeyIndexStorageKey = "pendingConsensusPubKeyIndex"
//
//	pendingRemoveNodeIDIndexStorageKey = "pendingRemoveNodeIDIndex"
//)
//
//var _ ProposalHandler = (*NodeManager)(nil)
//
//type NodeRegisterExtraArgsSignStruct struct {
//	ConsensusPubKey string                    `json:"consensus_pub_key"`
//	P2PPubKey       string                    `json:"p2p_pub_key"`
//	MetaData        node_manager.NodeMetaData `json:"meta_data"`
//}
//
//type NodeRegisterExtraArgs struct {
//	ConsensusPubKey              string                           `json:"consensus_pub_key"`
//	P2PPubKey                    string                           `json:"p2p_pub_key"`
//	MetaData                     node_manager_client.NodeMetaData `json:"meta_data"`
//	ConsensusPrivateKeySignature string                           `json:"consensus_private_key_signature"`
//	P2PPrivateKeySignature       string                           `json:"p2p_private_key_signature"`
//}
//
//type NodeRemoveExtraArgs struct {
//	NodeIDs []uint64 `json:"node_ids"`
//}
//
//type NodeUpgradeExtraArgs struct {
//	DownloadUrls []string `json:"download_urls"`
//	CheckHash    string   `json:"check_hash"`
//}
//
//type NodeManager struct {
//	gov *Governance
//	DefaultProposalPermissionManager
//
//	pendingNameIndex            *common.VMMap[string, struct{}]
//	pendingP2PIDIndex           *common.VMMap[string, struct{}]
//	pendingConsensusPubKeyIndex *common.VMMap[string, struct{}]
//
//	pendingRemoveNodeIDIndex *common.VMMap[uint64, struct{}]
//}
//
//func NewNodeManager(gov *Governance) *NodeManager {
//	return &NodeManager{
//		gov:                              gov,
//		DefaultProposalPermissionManager: NewDefaultProposalPermissionManager(gov),
//	}
//}
//
//func (nm *NodeManager) GenesisInit(genesis *repo.GenesisConfig) error {
//	return nil
//}
//
//func (nm *NodeManager) SetContext(ctx *common.VMContext) {
//	nm.pendingNameIndex = common.NewVMMap[string, struct{}](nm.gov.StateAccount, fmt.Sprintf("%s_%s", nodeManagerNamespace, pendingNameIndexStorageKey), func(key string) string {
//		return key
//	})
//	nm.pendingP2PIDIndex = common.NewVMMap[string, struct{}](nm.gov.StateAccount, fmt.Sprintf("%s_%s", nodeManagerNamespace, pendingP2PIDIndexStorageKey), func(key string) string {
//		return key
//	})
//	nm.pendingConsensusPubKeyIndex = common.NewVMMap[string, struct{}](nm.gov.StateAccount, fmt.Sprintf("%s_%s", nodeManagerNamespace, pendingConsensusPubKeyIndexStorageKey), func(key string) string {
//		return key
//	})
//	nm.pendingRemoveNodeIDIndex = common.NewVMMap[uint64, struct{}](nm.gov.StateAccount, fmt.Sprintf("%s_%s", nodeManagerNamespace, pendingRemoveNodeIDIndexStorageKey), func(key uint64) string {
//		return strconv.FormatUint(key, 10)
//	})
//}
//
//func (nm *NodeManager) ProposePermissionCheck(proposalType ProposalType, user ethcommon.Address) (has bool, err error) {
//	switch proposalType {
//	case NodeRegister:
//		// any node can register
//		return true, nil
//	case NodeRemove:
//		return nm.gov.isCouncilMember(user)
//	case NodeUpgrade:
//		return nm.gov.isCouncilMember(user)
//	default:
//		return false, errors.Errorf("unknown proposal type %d", proposalType)
//	}
//}
//
//func (nm *NodeManager) ProposeArgsCheck(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
//	switch proposalType {
//	case NodeRegister:
//		return nm.registerProposeArgsCheck(proposalType, title, desc, blockNumber, extra)
//	case NodeRemove:
//		return nm.removeProposeArgsCheck(proposalType, title, desc, blockNumber, extra)
//	case NodeUpgrade:
//		return nm.upgradeProposeArgsCheck(proposalType, title, desc, blockNumber, extra)
//	default:
//		return errors.Errorf("unknown proposal type %d", proposalType)
//	}
//}
//
////func (nm *NodeManager) VotePassExecute(proposal *Proposal) error {
////	switch proposal.Type {
////	case NodeRegister:
////		return nm.executeRegister(proposal)
////	case NodeRemove:
////		return nm.executeRemove(proposal)
////	case NodeUpgrade:
////		return nm.executeUpgrade(proposal)
////	default:
////		return errors.Errorf("unknown proposal type %d", proposal.Type)
////	}
////}
//
////func (nm *NodeManager) registerProposeArgsCheck(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
////	nodeExtraArgs := &NodeRegisterExtraArgs{}
////	if err := json.Unmarshal(extra, nodeExtraArgs); err != nil {
////		return errors.Wrap(err, "unmarshal node register extra arguments error")
////	}
////
////	consensusPubKey, p2pPubKey, p2pID, err := framework.CheckNodeInfo(node_manager.NodeInfo{
////		ConsensusPubKey: nodeExtraArgs.ConsensusPubKey,
////		P2PPubKey:       nodeExtraArgs.P2PPubKey,
////		Operator:        nm.gov.Ctx.From,
////		MetaData:        nodeExtraArgs.MetaData,
////	})
////	if err != nil {
////		return errors.Wrap(err, "check node info error")
////	}
////
////	// verify signature
////	nodeRegisterExtraArgsSignStruct := &NodeRegisterExtraArgsSignStruct{
////		ConsensusPubKey: nodeExtraArgs.ConsensusPubKey,
////		P2PPubKey:       nodeExtraArgs.P2PPubKey,
////		MetaData:        nodeExtraArgs.MetaData,
////	}
////	nodeRegisterExtraArgsSignStructBytes, err := json.Marshal(nodeRegisterExtraArgsSignStruct)
////	if err != nil {
////		return errors.Wrap(err, "failed to marshal node register extra args sign struct")
////	}
////
////	if !p2pPubKey.Verify(nodeRegisterExtraArgsSignStructBytes, hexutil.Decode(nodeExtraArgs.P2PPrivateKeySignature)) {
////		return errors.New("failed to verify p2p private key signature")
////	}
////	if !consensusPubKey.Verify(nodeRegisterExtraArgsSignStructBytes, hexutil.Decode(nodeExtraArgs.ConsensusPrivateKeySignature)) {
////		return errors.New("failed to verify consensus private key signature")
////	}
////
////	nodeManagerContract := framework.NodeManagerBuildConfig.Build(nm.gov.CrossCallSystemContractContext())
////
////	// check  unique index
////	if nodeManagerContract.ExistNodeByConsensusPubKey(nodeExtraArgs.ConsensusPubKey) {
////		return errors.Errorf("consensus public key already registered: %s", nodeExtraArgs.ConsensusPubKey)
////	}
////	if nodeManagerContract.ExistNodeByP2PID(p2pID) {
////		return errors.Errorf("p2p public key already registered: %s", nodeExtraArgs.P2PPubKey)
////	}
////	if nodeManagerContract.ExistNodeByName(nodeExtraArgs.MetaData.Name) {
////		return errors.Errorf("name already registered: %s", nodeExtraArgs.ConsensusPubKey)
////	}
////
////	if nm.pendingNameIndex.Has(nodeExtraArgs.MetaData.Name) {
////		return errors.Errorf("name already registered: %s", nodeExtraArgs.MetaData.Name)
////	}
////	if nm.pendingP2PIDIndex.Has(p2pID) {
////		return errors.Errorf("p2p public key already registered: %s", nodeExtraArgs.P2PPubKey)
////	}
////	if nm.pendingConsensusPubKeyIndex.Has(nodeExtraArgs.ConsensusPubKey) {
////		return errors.Errorf("consensus public key already registered: %s", nodeExtraArgs.ConsensusPubKey)
////	}
////
////	// add pending index
////	if err := nm.pendingNameIndex.Put(nodeExtraArgs.MetaData.Name, struct{}{}); err != nil {
////		return err
////	}
////	if err := nm.pendingP2PIDIndex.Put(p2pID, struct{}{}); err != nil {
////		return err
////	}
////	if err := nm.pendingConsensusPubKeyIndex.Put(nodeExtraArgs.ConsensusPubKey, struct{}{}); err != nil {
////		return err
////	}
////
////	// 10axc / 1000gmol
////	// Increase gas consumption to avoid attacks
////	nm.gov.Ctx.SetGasCost(10000000)
////	return nil
////}
////
////func (nm *NodeManager) removeProposeArgsCheck(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
////	nodeExtraArgs := &NodeRemoveExtraArgs{}
////	if err := json.Unmarshal(extra, nodeExtraArgs); err != nil {
////		return errors.Wrap(err, "unmarshal node remove extra arguments error")
////	}
////
////	nodeManagerContract := framework.NodeManagerBuildConfig.Build(nm.gov.CrossCallSystemContractContext())
////
////	for _, nodeID := range nodeExtraArgs.NodeIDs {
////		nodeInfo, err := nodeManagerContract.GetInfo(nodeID)
////		if err != nil {
////			return errors.Wrapf(err, "failed to get node info %d", nodeID)
////		}
////		if nodeInfo.Status == uint8(types.NodeStatusExited) {
////			return errors.Errorf("node already exited: %d[%s]", nodeID, nodeInfo.MetaData.Name)
////		}
////
////		if nm.pendingRemoveNodeIDIndex.Has(nodeID) {
////			return errors.Errorf("remove node already exist in other proposal: %d[%s]", nodeID, nodeInfo.MetaData.Name)
////		}
////		if err := nm.pendingRemoveNodeIDIndex.Put(nodeID, struct{}{}); err != nil {
////			return err
////		}
////	}
////
////	return nil
////}
//
//func (nm *NodeManager) upgradeProposeArgsCheck(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
//	upgradeExtraArgs := &NodeUpgradeExtraArgs{}
//	if err := json.Unmarshal(extra, upgradeExtraArgs); err != nil {
//		return errors.Wrap(err, "unmarshal node upgrade extra arguments error")
//	}
//
//	// check proposal has repeated download url
//	if len(lo.Uniq[string](upgradeExtraArgs.DownloadUrls)) != len(upgradeExtraArgs.DownloadUrls) {
//		return errors.New("repeated download url")
//	}
//
//	return nil
//}
//
////func (nm *NodeManager) executeRegister(proposal *Proposal) error {
////	nodeExtraArgs := &NodeRegisterExtraArgs{}
////	if err := json.Unmarshal(proposal.Extra, nodeExtraArgs); err != nil {
////		return errors.Wrap(err, "unmarshal node register extra arguments error")
////	}
////
////	_, _, p2pID, err := framework.CheckNodeInfo(node_manager.NodeInfo{
////		ConsensusPubKey: nodeExtraArgs.ConsensusPubKey,
////		P2PPubKey:       nodeExtraArgs.P2PPubKey,
////		Operator:        nm.gov.Ctx.From,
////		MetaData:        nodeExtraArgs.MetaData,
////	})
////	if err != nil {
////		return errors.Wrap(err, "check node info error")
////	}
////
////	nodeManagerContract := framework.NodeManagerBuildConfig.Build(nm.gov.CrossCallSystemContractContext())
////	_, err = nodeManagerContract.Register(node_manager_client.NodeInfo{
////		ConsensusPubKey: nodeExtraArgs.ConsensusPubKey,
////		P2PPubKey:       nodeExtraArgs.P2PPubKey,
////		P2PID:           p2pID,
////		Operator:        ethcommon.HexToAddress(proposal.Proposer),
////		MetaData:        nodeExtraArgs.MetaData,
////		Status:          uint8(types.NodeStatusDataSyncer),
////	})
////	if err != nil {
////		return err
////	}
////	return nil
////}
//
//func (nm *NodeManager) executeRemove(proposal *Proposal) error {
//	nodeExtraArgs := &NodeRemoveExtraArgs{}
//	if err := json.Unmarshal(proposal.Extra, nodeExtraArgs); err != nil {
//		return errors.Wrap(err, "unmarshal node remove extra arguments error")
//	}
//
//	ctx, snapshot := nm.gov.CrossCallSystemContractContextWithSnapshot()
//	nodeManagerContract := framework.NodeManagerBuildConfig.Build(ctx)
//	err := func() error {
//		for _, nodeID := range nodeExtraArgs.NodeIDs {
//			if err := nodeManagerContract.Exit(nodeID); err != nil {
//				return err
//			}
//		}
//		return nil
//	}()
//	if err != nil {
//		// revert all changes
//		ctx.StateLedger.RevertToSnapshot(snapshot)
//		return err
//	}
//
//	return nil
//}
//
//func (nm *NodeManager) executeUpgrade(proposal *Proposal) error {
//	return nil
//}
//
////func (nm *NodeManager) CleanProposal(proposal *Proposal) error {
////	switch proposal.Type {
////	case NodeRegister:
////		nodeExtraArgs := &NodeRegisterExtraArgs{}
////		if err := json.Unmarshal(proposal.Extra, nodeExtraArgs); err != nil {
////			return errors.Wrap(err, "unmarshal node register extra arguments error")
////		}
////		_, _, p2pID, err := framework.CheckNodeInfo(node_manager.NodeInfo{
////			ConsensusPubKey: nodeExtraArgs.ConsensusPubKey,
////			P2PPubKey:       nodeExtraArgs.P2PPubKey,
////			Operator:        nm.gov.Ctx.From,
////			MetaData:        nodeExtraArgs.MetaData,
////		})
////		if err != nil {
////			return errors.Wrap(err, "check node info error")
////		}
////		// clean pending index
////		if err := nm.pendingNameIndex.Delete(nodeExtraArgs.MetaData.Name); err != nil {
////			return err
////		}
////		if err := nm.pendingP2PIDIndex.Delete(p2pID); err != nil {
////			return err
////		}
////		if err := nm.pendingConsensusPubKeyIndex.Delete(nodeExtraArgs.ConsensusPubKey); err != nil {
////			return err
////		}
////	case NodeRemove:
////		nodeExtraArgs := &NodeRemoveExtraArgs{}
////		if err := json.Unmarshal(proposal.Extra, nodeExtraArgs); err != nil {
////			return errors.Wrap(err, "unmarshal node register extra arguments error")
////		}
////		// clean pending index
////		for _, nodeID := range nodeExtraArgs.NodeIDs {
////			if err := nm.pendingRemoveNodeIDIndex.Delete(nodeID); err != nil {
////				return err
////			}
////		}
////	default:
////		return nil
////	}
////	return nil
////}
