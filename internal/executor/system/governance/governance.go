package governance

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
)

// TODO remove
var (
	ErrMethodName         = errors.New("no this method")
	ErrVoteResult         = errors.New("vote result is invalid")
	ErrProposalType       = errors.New("proposal type is invalid")
	ErrUser               = errors.New("user is invalid")
	ErrUseHasVoted        = errors.New("user has already voted")
	ErrTitle              = errors.New("title is invalid")
	ErrTooLongTitle       = errors.New("title is too long, max is 200 characters")
	ErrDesc               = errors.New("description is invalid")
	ErrTooLongDesc        = errors.New("description is too long, max is 10000 characters")
	ErrBlockNumber        = errors.New("block number is invalid")
	ErrProposalFinished   = errors.New("proposal has already finished")
	ErrBlockNumberOutDate = errors.New("block number is out of date")

	ErrRepeatedRegister = errors.New("repeated register")
	ErrNotFoundProposal = errors.New("not found proposal")
)

const (
	ProposeMethod             = "propose"
	VoteMethod                = "vote"
	GetLatestProposalIDMethod = "getLatestProposalID"
	ProposalKey               = "proposalKey"
	MaxTitleLength            = 200
	MaxDescLength             = 10000
)

type ProposalType uint8

const (
	// CouncilElect is a proposal for elect the council
	CouncilElect ProposalType = iota

	// NodeUpgrade is a proposal for update or upgrade the node
	NodeUpgrade

	// NodeAdd is a proposal for adding a new node
	NodeAdd

	// NodeRemove is a proposal for removing a node
	NodeRemove

	WhiteListProviderAdd

	WhiteListProviderRemove

	GasUpdate

	// RescueState is an emergency proposal that forces a write to a
	// contract's EVM storage slot. It follows the standard
	// propose → vote → execute flow like every other proposal type,
	// with one relaxation: in solo deployments (council size 0 or
	// 1) any address may propose because the operator may not have
	// the admin key available at the moment of the incident. Used
	// to recover from irreversible on-chain state corruption such as
	// a stuck Uniswap pair whose unlocked flag is stuck at 0.
	// MUST stay at the end of the iota block — never insert above.
	RescueState
)

// RescueExtraArgs is the JSON payload for a RescueState proposal.
// Value MUST be a 32-byte hex string (with or without the 0x prefix)
// because EVM storage slots are always exactly 32 bytes wide.
type RescueExtraArgs struct {
	Target string `json:"target"`
	Slot   uint64 `json:"slot"`
	Value  string `json:"value"`
}

// RescueValidWindow is the number of blocks after a rescue proposal
// becomes effective during which the forced write is allowed to run.
// Outside this window the proposal is treated as expired and ignored.
const RescueValidWindow uint64 = 256

type VoteResult uint8

const (
	Pass VoteResult = iota
	Reject
)

type IGovenance interface {
	Propose(proposalType uint8, title, desc string, expiredBlockNumber uint64, extra []byte) error

	Vote(proposalID uint64, voteResult uint8) error

	Proposal(proposalID uint64) (*Proposal, error)

	GetLatestProposalID() uint64
}

type IExecutor interface {
	ProposeCheck(proposalType ProposalType, extra []byte) error
	Execute(proposal *Proposal) error
}

type Governance struct {
	logger logrus.FieldLogger

	// type2Executor is mapping proposal type to executor
	type2Executor map[ProposalType]IExecutor
	// governance records which executor implements IGovernance interface
	// which instance in governance would call self overide method
	governance map[ProposalType]IGovenance

	account                ledger.IAccount
	proposalID             *ProposalID
	addr2NameSystem        *Addr2NameSystem
	notFinishedProposalMgr *NotFinishedProposalMgr

	currentUser   *ethcommon.Address
	currentHeight uint64
	currentLogs   *[]common.Log
	stateLedger   ledger.StateLedger
}

func NewGov(cfg *common.SystemContractConfig) *Governance {
	gov := &Governance{
		logger:        loggers.Logger(loggers.Governance),
		type2Executor: make(map[ProposalType]IExecutor),
		governance:    make(map[ProposalType]IGovenance),
	}

	// register governance execute contract
	councilManager := NewCouncilManager(gov)
	if err := gov.Register(CouncilElect, councilManager); err != nil {
		panic(err)
	}

	nodeManager := NewNodeManager(gov)
	if err := gov.Register(NodeAdd, nodeManager); err != nil {
		panic(err)
	}
	if err := gov.Register(NodeRemove, nodeManager); err != nil {
		panic(err)
	}
	if err := gov.Register(NodeUpgrade, nodeManager); err != nil {
		panic(err)
	}

	gasManager := NewGasManager(gov)
	if err := gov.Register(GasUpdate, gasManager); err != nil {
		panic(err)
	}

	whiteListProviderManager := NewWhiteListProviderManager(gov)
	if err := gov.Register(WhiteListProviderAdd, whiteListProviderManager); err != nil {
		panic(err)
	}
	if err := gov.Register(WhiteListProviderRemove, whiteListProviderManager); err != nil {
		panic(err)
	}

	// Rescue: single-tx forced EVM storage write. Used to recover
	// from irreversible on-chain state corruption, e.g. a stuck
	// Uniswap pair whose unlocked flag is permanently 0. Propose is
	// overridden to skip the vote cycle and execute immediately.
	rescueManager := NewRescueManager(gov)
	if err := gov.Register(RescueState, rescueManager); err != nil {
		panic(err)
	}

	return gov
}

func (g *Governance) Register(proposalType ProposalType, executor IExecutor) error {
	if _, ok := g.type2Executor[proposalType]; ok {
		return ErrRepeatedRegister
	}
	g.type2Executor[proposalType] = executor

	// if executor also implements IGovenance interface, add it to governance
	if gov, ok := executor.(IGovenance); ok {
		g.governance[proposalType] = gov
	}
	return nil
}

func (g *Governance) SetContext(context *common.VMContext) {
	g.currentUser = context.CurrentUser
	g.currentHeight = context.CurrentHeight
	g.currentLogs = context.CurrentLogs
	g.stateLedger = context.StateLedger

	addr := types.NewAddressByStr(common.GovernanceContractAddr)
	g.account = g.stateLedger.GetOrCreateAccount(addr)
	g.proposalID = NewProposalID(g.stateLedger)
	g.addr2NameSystem = NewAddr2NameSystem(g.stateLedger)
	g.notFinishedProposalMgr = NewNotFinishedProposalMgr(g.stateLedger)
}

func (g *Governance) checkAndUpdateState(method string) error {
	notFinishedProposals, err := g.notFinishedProposalMgr.GetProposals()
	if err != nil {
		return err
	}

	for _, notFinishedProposal := range notFinishedProposals {
		if notFinishedProposal.DeadlineBlockNumber <= g.currentHeight {
			// update original proposal status
			proposal, err := g.LoadProposal(notFinishedProposal.ID)
			if err != nil {
				return err
			}

			if proposal.GetStatus() == Approved || proposal.GetStatus() == Rejected {
				// proposal is finnished, no need update
				continue
			}

			// means proposal is out of deadline,status change to rejected
			proposal.SetStatus(Rejected)

			err = g.SaveProposal(proposal)
			if err != nil {
				return err
			}

			// remove proposal from not finished proposals
			if err = g.notFinishedProposalMgr.RemoveProposal(notFinishedProposal.ID); err != nil {
				return err
			}

			// post log
			g.RecordLog(method, proposal)
		}
	}

	return nil
}

func (g *Governance) CheckProposeArgs(user *ethcommon.Address, proposalType ProposalType, title, desc string, expiredBlockNumber uint64, currentHeight uint64) error {
	if user == nil {
		return ErrUser
	}

	_, isVaildProposalType := g.type2Executor[proposalType]
	if !isVaildProposalType {
		return ErrProposalType
	}

	if title == "" || len(title) > MaxTitleLength {
		if title == "" {
			return ErrTitle
		}
		return ErrTooLongTitle
	}

	if desc == "" || len(desc) > MaxDescLength {
		if desc == "" {
			return ErrDesc
		}
		return ErrTooLongDesc
	}

	if expiredBlockNumber == 0 {
		return ErrBlockNumber
	}

	// check out of date block number
	if expiredBlockNumber < currentHeight {
		return ErrBlockNumberOutDate
	}

	return nil
}

func (g *Governance) Propose(pType uint8, title, desc string, blockNumber uint64, extra []byte) error {
	proposalType := ProposalType(pType)
	_, ok := g.type2Executor[proposalType]
	if !ok {
		return ErrProposalType
	}

	gov, ok := g.governance[proposalType]
	if ok {
		// call overide function
		return gov.Propose(pType, title, desc, blockNumber, extra)
	}

	// check and update state
	if err := g.checkAndUpdateState(ProposeMethod); err != nil {
		return err
	}

	if err := g.CheckProposeArgs(g.currentUser, proposalType, title, desc, blockNumber, g.currentHeight); err != nil {
		return err
	}

	executor, ok := g.type2Executor[proposalType]
	if !ok {
		return ErrProposalType
	}
	if err := executor.ProposeCheck(proposalType, extra); err != nil {
		return err
	}

	// check proposer is council member
	isCouncilMember, council := CheckInCouncil(g.account, g.currentUser.String())
	if !isCouncilMember {
		return ErrNotFoundCouncilMember
	}

	return g.CreateProposal(council, proposalType, title, desc, blockNumber, extra, true)
}

func (g *Governance) CreateProposal(council *Council, proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte, includeUser bool) error {
	proposal := &Proposal{
		Type:               proposalType,
		Strategy:           NowProposalStrategy,
		Proposer:           g.currentUser.String(),
		Title:              title,
		Desc:               desc,
		BlockNumber:        blockNumber,
		Status:             Voting,
		Extra:              extra,
		CreatedBlockNumber: g.currentHeight,
	}

	id, err := g.proposalID.GetAndAddID()
	if err != nil {
		return err
	}
	proposal.ID = id
	proposal.TotalVotes = lo.Sum[uint64](lo.Map[CouncilMember, uint64](council.Members, func(item CouncilMember, index int) uint64 {
		return item.Weight
	}))
	// proposer vote pass by default
	if includeUser {
		proposal.PassVotes = []string{g.currentUser.String()}
	}

	if err := g.SaveProposal(proposal); err != nil {
		return err
	}

	// propose generate not finished proposal
	if err = g.notFinishedProposalMgr.SetProposal(&NotFinishedProposal{
		ID:                  proposal.ID,
		DeadlineBlockNumber: proposal.BlockNumber,
		Type:                proposal.Type,
	}); err != nil {
		return err
	}

	// record log
	g.RecordLog(ProposeMethod, proposal)

	return nil
}

func (g *Governance) CheckBeforeVote(user *ethcommon.Address, proposal *Proposal, voteResult VoteResult) (bool, error) {
	if user == nil {
		return false, ErrUser
	}

	if voteResult != Pass && voteResult != Reject {
		return false, ErrVoteResult
	}

	// check if user has voted
	if common.IsInSlice[string](user.String(), proposal.PassVotes) || common.IsInSlice[string](user.String(), proposal.RejectVotes) {
		return false, ErrUseHasVoted
	}

	// check proposal status
	if proposal.Status == Approved || proposal.Status == Rejected {
		return false, ErrProposalFinished
	}

	return true, nil
}

// Vote a proposal, return vote status
func (g *Governance) Vote(proposalID uint64, voteRes uint8) error {
	proposal, err := g.LoadProposal(proposalID)
	if err != nil {
		return err
	}

	proposalType := proposal.Type
	if _, ok := g.type2Executor[proposalType]; !ok {
		return ErrProposalType
	}

	// Dispatch to per-type override if registered. The override is
	// expected to call g.voteCore to perform the actual bookkeeping
	// (it must not loop back through g.Vote).
	if gov, ok := g.governance[proposalType]; ok {
		return gov.Vote(proposalID, voteRes)
	}

	return g.voteCore(proposalID, voteRes)
}

// voteCore performs the standard vote bookkeeping after a proposal
// has been type-checked and dispatched. It is the single source of
// truth for the Pass/Reject tally, status calculation, terminal-state
// cleanup, and Approved → executor.Execute handoff. Both the default
// Governance.Vote path and per-type IGovenance overrides (e.g.
// RescueManager.Vote) call into this so the bookkeeping stays
// consistent across proposal types.
func (g *Governance) voteCore(proposalID uint64, voteRes uint8) error {
	voteResult := VoteResult(voteRes)
	proposal, err := g.LoadProposal(proposalID)
	if err != nil {
		return err
	}

	// check and update state
	if err := g.checkAndUpdateState(VoteMethod); err != nil {
		return err
	}

	if _, err := g.CheckBeforeVote(g.currentUser, proposal, voteResult); err != nil {
		return err
	}

	// check user can vote
	isExist, _ := CheckInCouncil(g.account, g.currentUser.String())
	if !isExist {
		return ErrNotFoundCouncilMember
	}

	switch voteResult {
	case Pass:
		proposal.PassVotes = append(proposal.PassVotes, g.currentUser.String())
	case Reject:
		proposal.RejectVotes = append(proposal.RejectVotes, g.currentUser.String())
	}
	proposal.Status = CalcProposalStatus(proposal.Strategy, proposal.TotalVotes, uint64(len(proposal.PassVotes)), uint64(len(proposal.RejectVotes)))

	isProposalSaved := false
	// update not finished proposal
	if proposal.Status == Approved || proposal.Status == Rejected {
		proposal.EffectiveBlockNumber = g.currentHeight
		if err := g.SaveProposal(proposal); err != nil {
			return err
		}
		isProposalSaved = true

		if err := g.notFinishedProposalMgr.RemoveProposal(proposal.ID); err != nil {
			return err
		}
	}

	if !isProposalSaved {
		if err := g.SaveProposal(proposal); err != nil {
			return err
		}
	}

	// if proposal approved, then execute
	if proposal.Status == Approved {
		executor, ok := g.type2Executor[proposal.Type]
		if !ok {
			return ErrProposalType
		}
		if err := executor.Execute(proposal); err != nil {
			return err
		}
	}

	g.RecordLog(VoteMethod, proposal)

	return nil
}

// GetLatestProposalID return current proposal lastest id
func (g *Governance) GetLatestProposalID() uint64 {
	return g.proposalID.GetID() - 1
}

func (g *Governance) Proposal(proposalID uint64) (*Proposal, error) {
	return g.LoadProposal(proposalID)
}

func (g *Governance) SaveProposal(proposal *Proposal) error {
	b, err := json.Marshal(proposal)
	if err != nil {
		return err
	}

	g.account.SetState([]byte(fmt.Sprintf("%s%d", ProposalKey, proposal.GetID())), b)
	return nil
}

func (g *Governance) LoadProposal(proposalID uint64) (*Proposal, error) {
	isExist, b := g.account.GetState([]byte(fmt.Sprintf("%s%d", ProposalKey, proposalID)))
	if !isExist {
		return nil, ErrNotFoundProposal
	}

	proposal := &Proposal{}
	if err := json.Unmarshal(b, proposal); err != nil {
		return nil, err
	}
	return proposal, nil
}

// RecordLog record execution log for governance
func (g *Governance) RecordLog(method string, proposal *Proposal) {
	// set method signature, proposal id, proposal type, proposer as log topic for index
	idhash := make([]byte, 8)
	binary.BigEndian.PutUint64(idhash, proposal.ID)
	typeHash := make([]byte, 2)
	binary.BigEndian.PutUint16(typeHash, uint16(proposal.Type))

	data, _ := json.Marshal(proposal)

	currentLog := common.Log{
		Address: types.NewAddressByStr(common.GovernanceContractAddr),
	}
	// topic add method signature, id, proposal type, proposer
	currentLog.Topics = append(currentLog.Topics, types.NewHashByStr(method),
		types.NewHash(idhash), types.NewHash(typeHash), types.NewHash([]byte(proposal.Proposer)))
	currentLog.Data = data
	currentLog.Removed = false

	*g.currentLogs = append(*g.currentLogs, currentLog)
}
