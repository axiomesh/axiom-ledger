package governance

import (
	"encoding/json"
	"errors"
	"sort"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/samber/lo"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/base"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	// NodeMembersKey is key for node member storage
	NodeMembersKey = "nodeMembersKey"
)

var (
	ErrNodeNumber              = errors.New("node members total count can't bigger than candidates count")
	ErrNotFoundNodeID          = errors.New("node id is not found")
	ErrNotFoundNodeName        = errors.New("node name is not found")
	ErrNotFoundNodeAddress     = errors.New("address is not found")
	ErrNodeExtraArgs           = errors.New("unmarshal node extra arguments error")
	ErrNodeProposalNumberLimit = errors.New("node proposal number limit, only allow one node proposal")
	ErrNotFoundNodeProposal    = errors.New("node proposal not found for the id")
	ErrUnKnownProposalArgs     = errors.New("unknown proposal args")
	ErrRepeatedNodeID          = errors.New("repeated node id")
	ErrRepeatedNodeName        = errors.New("repeated node name")
	ErrRepeatedNodeAddress     = errors.New("repeated address")
	ErrUpgradeExtraArgs        = errors.New("unmarshal node upgrade extra arguments error")
	ErrRepeatedDownloadUrl     = errors.New("repeated download url")
)

// NodeExtraArgs is Node proposal extra arguments
type NodeExtraArgs struct {
	Nodes []*NodeMember
}

type Node struct {
	Members []*NodeMember
}

type NodeMember struct {
	Name    string `mapstructure:"name" toml:"name"`
	NodeId  string `mapstructure:"node_id" toml:"node_id"`
	Address string `mapstructure:"address" toml:"address"`
	ID      uint64 `mapstructure:"id" toml:"id"`
}

type UpgradeExtraArgs struct {
	DownloadUrls []string
	CheckHash    string
}

type NodeManager struct {
	gov *Governance
}

func NewNodeManager(gov *Governance) *NodeManager {
	return &NodeManager{
		gov: gov,
	}
}

func (nm *NodeManager) getNodeExtraArgs(extra []byte) (*NodeExtraArgs, error) {
	extraArgs := &NodeExtraArgs{}
	if err := json.Unmarshal(extra, extraArgs); err != nil {
		return nil, ErrNodeExtraArgs
	}

	return extraArgs, nil
}

func (nm *NodeManager) getUpgradeExtraArgs(extra []byte) (*UpgradeExtraArgs, error) {
	extraArgs := &UpgradeExtraArgs{}
	if err := json.Unmarshal(extra, extraArgs); err != nil {
		return nil, ErrUpgradeExtraArgs
	}

	return extraArgs, nil
}

func (nm *NodeManager) ProposeCheck(proposalType ProposalType, extra []byte) error {
	return nil
}

func (nm *NodeManager) Execute(proposal *Proposal) error {
	return nil
}

func (nm *NodeManager) Propose(pType uint8, title, desc string, blockNumber uint64, extra []byte) error {
	// check and update state
	if err := nm.gov.checkAndUpdateState(ProposeMethod); err != nil {
		return err
	}

	proposalType := ProposalType(pType)
	if proposalType == NodeAdd || proposalType == NodeRemove {
		return nm.proposeNodeAddRemove(proposalType, title, desc, blockNumber, extra)
	}

	return nm.proposeUpgrade(proposalType, title, desc, blockNumber, extra)
}

func (nm *NodeManager) proposeNodeAddRemove(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
	addr := nm.gov.currentUser
	if err := nm.gov.CheckProposeArgs(addr, proposalType, title, desc, blockNumber, nm.gov.currentHeight); err != nil {
		return err
	}

	nodeExtraArgs, err := nm.getNodeExtraArgs(extra)
	if err != nil {
		return err
	}

	members, err := GetNodeMembers(nm.gov.stateLedger)
	if err != nil {
		return err
	}
	// store node information in members into map
	memberNameMap := make(map[string]*NodeMember)
	for _, member := range members {
		key := member.Name
		memberNameMap[key] = member
	}
	memberAddressMap := make(map[string]*NodeMember)
	for _, member := range members {
		key := member.Address
		memberAddressMap[key] = member
	}
	memberNodeIdMap := make(map[string]*NodeMember)
	for _, member := range members {
		key := member.NodeId
		memberNodeIdMap[key] = member
	}

	isExist, council := CheckInCouncil(nm.gov.account, addr.String())
	isPassVoteIncludeUser := true
	if proposalType == NodeAdd {
		// check addr if is exist in council
		if !isExist {
			return ErrNotFoundCouncilMember
		}
		for _, node := range nodeExtraArgs.Nodes {
			if _, found := memberNodeIdMap[node.NodeId]; found {
				return ErrRepeatedNodeID
			}
			if _, found := memberAddressMap[node.Address]; found {
				return ErrRepeatedNodeAddress
			}
			if _, found := memberNameMap[node.Name]; found {
				return ErrRepeatedNodeName
			}
		}
	} else {
		// check whether the from address and node address are consistent
		for _, node := range nodeExtraArgs.Nodes {
			if addr.String() != node.Address {
				// If the addresses are inconsistent, verify whether the address that initiated the proposal is a member of the committee.
				if !isExist {
					return ErrNotFoundCouncilMember
				}
			} else if !isExist {
				// not council member can't vote
				isPassVoteIncludeUser = false
			}

			if _, found := memberNodeIdMap[node.NodeId]; !found {
				return ErrNotFoundNodeID
			}
			if _, found := memberAddressMap[node.Address]; !found {
				return ErrNotFoundNodeAddress
			}
			if _, found := memberNameMap[node.Name]; !found {
				return ErrNotFoundNodeName
			}
		}
	}

	// check proposal has repeated nodes
	if len(lo.Uniq[string](lo.Map[*NodeMember, string](nodeExtraArgs.Nodes, func(item *NodeMember, index int) string {
		return item.NodeId
	}))) != len(nodeExtraArgs.Nodes) {
		return ErrRepeatedNodeID
	}

	return nm.gov.CreateProposal(council, proposalType, title, desc, blockNumber, extra, isPassVoteIncludeUser)
}

func (nm *NodeManager) proposeUpgrade(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
	addr := nm.gov.currentUser
	if err := nm.gov.CheckProposeArgs(addr, proposalType, title, desc, blockNumber, nm.gov.currentHeight); err != nil {
		return err
	}

	upgradeExtraArgs, err := nm.getUpgradeExtraArgs(extra)
	if err != nil {
		return err
	}

	// check proposal has repeated download url
	if len(lo.Uniq[string](upgradeExtraArgs.DownloadUrls)) != len(upgradeExtraArgs.DownloadUrls) {
		return ErrRepeatedDownloadUrl
	}

	// check addr if is exist in council
	isExist, council := CheckInCouncil(nm.gov.account, addr.String())
	if !isExist {
		return ErrNotFoundCouncilMember
	}

	return nm.gov.CreateProposal(council, proposalType, title, desc, blockNumber, extra, true)
}

// Vote a proposal, return vote status
func (nm *NodeManager) Vote(proposalID uint64, voteRes uint8) error {
	// check and update state
	if err := nm.gov.checkAndUpdateState(VoteMethod); err != nil {
		return err
	}

	voteResult := VoteResult(voteRes)
	proposal, err := nm.gov.LoadProposal(proposalID)
	if err != nil {
		return err
	}

	if proposal.Type == NodeUpgrade {
		return nm.voteUpgrade(nm.gov.currentUser, proposal, voteResult)
	}

	return nm.voteNodeAddRemove(nm.gov.currentUser, proposal, voteResult)
}

func (nm *NodeManager) Proposal(proposalID uint64) (*Proposal, error) {
	return nm.gov.Proposal(proposalID)
}

// GetLatestProposalID return current proposal lastest id
func (nm *NodeManager) GetLatestProposalID() uint64 {
	return nm.gov.GetLatestProposalID()
}

func (nm *NodeManager) voteNodeAddRemove(user *ethcommon.Address, proposal *Proposal, voteResult VoteResult) error {
	if _, err := nm.gov.CheckBeforeVote(user, proposal, voteResult); err != nil {
		return err
	}

	// check user can vote
	isExist, _ := CheckInCouncil(nm.gov.account, user.String())
	if !isExist {
		return ErrNotFoundCouncilMember
	}

	switch voteResult {
	case Pass:
		proposal.PassVotes = append(proposal.PassVotes, user.String())
	case Reject:
		proposal.RejectVotes = append(proposal.RejectVotes, user.String())
	}
	proposal.Status = CalcProposalStatus(proposal.Strategy, proposal.TotalVotes, uint64(len(proposal.PassVotes)), uint64(len(proposal.RejectVotes)))

	isProposalSaved := false
	// update not finished proposal
	if proposal.Status == Approved || proposal.Status == Rejected {
		proposal.EffectiveBlockNumber = nm.gov.currentHeight
		if err := nm.gov.SaveProposal(proposal); err != nil {
			return err
		}
		isProposalSaved = true

		if err := nm.gov.notFinishedProposalMgr.RemoveProposal(proposal.ID); err != nil {
			return err
		}
	}

	if !isProposalSaved {
		if err := nm.gov.SaveProposal(proposal); err != nil {
			return err
		}
	}

	// if proposal is approved, update the node members
	if proposal.Status == Approved {
		members, err := GetNodeMembers(nm.gov.stateLedger)
		if err != nil {
			return err
		}

		nodeExtraArgs, err := nm.getNodeExtraArgs(proposal.Extra)
		if err != nil {
			return err
		}

		if proposal.Type == NodeAdd {
			for _, node := range nodeExtraArgs.Nodes {
				newNodeID, err := base.AddNode(nm.gov.stateLedger, rbft.NodeInfo{
					AccountAddress:       node.Address,
					P2PNodeID:            node.NodeId,
					ConsensusVotingPower: 1000,
				})

				if err != nil {
					return err
				}
				node.ID = newNodeID
			}
			members = append(members, nodeExtraArgs.Nodes...)
		}

		if proposal.Type == NodeRemove {
			// https://github.com/samber/lo
			// Use the Associate method to create a map with the node's NodeId as the key and the NodeMember object as the value
			nodeIdToNodeMap := lo.Associate(nodeExtraArgs.Nodes, func(node *NodeMember) (string, *NodeMember) {
				return node.NodeId, node
			})

			// The members slice is updated to filteredMembers, which does not contain members with the same NodeId as proposalNodes
			filteredMembers := lo.Reject(members, func(member *NodeMember, _ int) bool {
				_, exists := nodeIdToNodeMap[member.NodeId]
				return exists
			})

			for _, node := range nodeExtraArgs.Nodes {
				err = base.RemoveNodeByP2PNodeID(nm.gov.stateLedger, node.NodeId)
				if err != nil {
					return err
				}
			}
			members = filteredMembers
		}

		cb, err := json.Marshal(members)
		if err != nil {
			return err
		}
		nm.gov.account.SetState([]byte(NodeMembersKey), cb)
	}

	// record log
	nm.gov.RecordLog(VoteMethod, proposal)

	// vote not return value
	return nil
}

func (nm *NodeManager) voteUpgrade(user *ethcommon.Address, proposal *Proposal, voteResult VoteResult) error {
	if _, err := nm.gov.CheckBeforeVote(nm.gov.currentUser, proposal, voteResult); err != nil {
		return err
	}

	// check user can vote
	isExist, _ := CheckInCouncil(nm.gov.account, user.String())
	if !isExist {
		return ErrNotFoundCouncilMember
	}

	switch voteResult {
	case Pass:
		proposal.PassVotes = append(proposal.PassVotes, nm.gov.currentUser.String())
	case Reject:
		proposal.RejectVotes = append(proposal.RejectVotes, nm.gov.currentUser.String())
	}
	proposal.Status = CalcProposalStatus(proposal.Strategy, proposal.TotalVotes, uint64(len(proposal.PassVotes)), uint64(len(proposal.RejectVotes)))

	isProposalSaved := false
	// update not finished proposal
	if proposal.Status == Approved || proposal.Status == Rejected {
		proposal.EffectiveBlockNumber = nm.gov.currentHeight
		if err := nm.gov.SaveProposal(proposal); err != nil {
			return err
		}
		isProposalSaved = true

		if err := nm.gov.notFinishedProposalMgr.RemoveProposal(proposal.ID); err != nil {
			return err
		}
	}

	if !isProposalSaved {
		if err := nm.gov.SaveProposal(proposal); err != nil {
			return err
		}
	}

	// record log
	// if approved, guardian sync log, then update node and restart
	nm.gov.RecordLog(VoteMethod, proposal)

	// vote not return value
	return nil
}

func InitNodeMembers(lg ledger.StateLedger, members []*repo.NodeName, epochInfo *rbft.EpochInfo) error {
	// read member config, write to ViewLedger
	nodeMembers := mergeData(members, epochInfo)
	c, err := json.Marshal(nodeMembers)
	if err != nil {
		return err
	}
	account := lg.GetOrCreateAccount(types.NewAddressByStr(common.GovernanceContractAddr))
	account.SetState([]byte(NodeMembersKey), c)
	return nil
}

func mergeData(members []*repo.NodeName, epochInfo *rbft.EpochInfo) []*NodeMember {
	var result []*NodeMember
	nodeMap := make(map[uint64]*repo.NodeName)
	for _, node := range members {
		nodeMap[node.ID] = node
	}

	// traverse epochInfo and merge data
	fillNodeInfo := func(nodes []rbft.NodeInfo) {
		for _, nodeInfo := range nodes {
			nodeMember := NodeMember{
				NodeId:  nodeInfo.P2PNodeID,
				Address: nodeInfo.AccountAddress,
				ID:      nodeInfo.ID,
			}
			if member, ok := nodeMap[nodeInfo.ID]; ok {
				nodeMember.Name = member.Name
			}
			result = append(result, &nodeMember)
		}
	}
	fillNodeInfo(epochInfo.ValidatorSet)
	fillNodeInfo(epochInfo.CandidateSet)
	fillNodeInfo(epochInfo.DataSyncerSet)
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func GetNodeMembers(lg ledger.StateLedger) ([]*NodeMember, error) {
	account := lg.GetOrCreateAccount(types.NewAddressByStr(common.GovernanceContractAddr))
	success, data := account.GetState([]byte(NodeMembersKey))
	if success {
		var members []*NodeMember
		if err := json.Unmarshal(data, &members); err != nil {
			return nil, err
		}
		return members, nil
	}
	return nil, errors.New("node member should be initialized in genesis")
}
