package governance

import (
	"encoding/json"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance/solidity/governance"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance/solidity/governance_client"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var (
	ErrVoteResult         = errors.New("vote result is invalid")
	ErrProposalType       = errors.New("proposal type is invalid")
	ErrUseHasVoted        = errors.New("user has already voted")
	ErrTitle              = errors.New("title is invalid")
	ErrTooLongTitle       = errors.New("title is too long, max is 200 characters")
	ErrDesc               = errors.New("description is invalid")
	ErrTooLongDesc        = errors.New("description is too long, max is 10000 characters")
	ErrBlockNumber        = errors.New("block number is invalid")
	ErrProposalFinished   = errors.New("proposal has already finished")
	ErrBlockNumberOutDate = errors.New("block number is out of date")

	ErrRepeatedRegister = errors.New("repeated proposal handler register")
	ErrNotFoundProposal = errors.New("not found")
)

const (
	nextProposalIDStorageKey       = "nextProposalID"
	proposalsStorageKey            = "proposals"
	notFinishedProposalsStorageKey = "notFinishedProposals"
)

const (
	ProposeEvent              = "Propose"
	VoteEvent                 = "Vote"
	GetLatestProposalIDMethod = "getLatestProposalID"
	MaxTitleLength            = 200
	MaxDescLength             = 10000
)

type ProposalStatus uint8

const (
	Voting ProposalStatus = iota
	Approved
	Rejected
)

type ProposalType uint8

const (
	// CouncilElect is a proposal for elect the council
	CouncilElect ProposalType = iota

	// NodeUpgrade is a proposal for update or upgrade the node
	NodeUpgrade

	// NodeRegister is a proposal for adding a new node
	NodeRegister

	// NodeRemove is a proposal for removing a node
	NodeRemove

	// WhitelistProviderAdd is a proposal for adding a new white list provider
	WhitelistProviderAdd

	// WhitelistProviderRemove is a proposal for removing a white list provider
	WhitelistProviderRemove

	// GasUpdate is a proposal for updating gas price
	GasUpdate

	// ChainParamUpgrade is a proposal for upgrading the chain param
	ChainParamUpgrade
)

type Proposal struct {
	ID          uint64
	Type        ProposalType
	Strategy    ProposalStrategy
	Proposer    string
	Title       string
	Desc        string
	BlockNumber uint64

	// totalVotes is total votes for this proposal
	// attention: some users may not vote for this proposal
	TotalVotes uint64

	// passVotes record user address for passed vote
	PassVotes []string

	RejectVotes []string
	Status      ProposalStatus

	// Extra information for some special proposal
	Extra []byte

	// CreatedBlockNumber is block number when the proposal has be created
	CreatedBlockNumber uint64

	// EffectiveBlockNumber is block number when the proposal has be take effect
	EffectiveBlockNumber uint64

	ExecuteSuccess   bool
	ExecuteFailedMsg string
}

type NotFinishedProposal struct {
	ID                  uint64
	DeadlineBlockNumber uint64
	Type                ProposalType
}

type VoteResult uint8

const (
	Pass VoteResult = iota
	Reject
)

var BuildConfig = &common.SystemContractBuildConfig[*Governance]{
	Name:    "governance",
	Address: common.GovernanceContractAddr,
	AbiStr:  governance_client.BindingContractMetaData.ABI,
	Constructor: func(systemContractBase common.SystemContractBase) *Governance {
		gov := &Governance{
			SystemContractBase: systemContractBase,
		}
		gov.Init()
		return gov
	},
}

type ProposalExecutor interface {
	ProposeArgsCheck(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error

	VotePassExecute(proposal *Proposal) error

	CleanProposal(proposal *Proposal) error
}

type ProposalPermissionManager interface {
	ProposePermissionCheck(proposalType ProposalType, user ethcommon.Address) (has bool, err error)

	TotalVotes(proposalType ProposalType) (uint64, error)

	UpdateVoteStatus(proposal *Proposal) error

	VotePermissionCheck(proposalType ProposalType, user ethcommon.Address) (has bool, err error)
}

type ProposalHandler interface {
	GenesisInit(genesis *repo.GenesisConfig) error
	SetContext(ctx *common.VMContext)
	ProposalExecutor
	ProposalPermissionManager
}

type Governance struct {
	common.SystemContractBase

	// proposal type -> proposal handler
	proposalHandlerMap map[ProposalType]ProposalHandler

	council              *common.VMSlot[Council]
	nextProposalID       *common.VMSlot[uint64]
	proposals            *common.VMMap[uint64, Proposal]
	notFinishedProposals *common.VMSlot[[]NotFinishedProposal]
}

func (g *Governance) GenesisInit(genesis *repo.GenesisConfig) error {
	if err := g.nextProposalID.Put(1); err != nil {
		return err
	}
	for _, handler := range g.proposalHandlerMap {
		if err := handler.GenesisInit(genesis); err != nil {
			return errors.Wrapf(err, "failed to genesis init handler %d", handler)
		}
	}
	return nil
}

func (g *Governance) SetContext(ctx *common.VMContext) {
	g.SystemContractBase.SetContext(ctx)

	g.council = common.NewVMSlot[Council](g.StateAccount, councilStorageKey)
	g.nextProposalID = common.NewVMSlot[uint64](g.StateAccount, nextProposalIDStorageKey)
	g.proposals = common.NewVMMap[uint64, Proposal](g.StateAccount, proposalsStorageKey, func(key uint64) string {
		return strconv.FormatUint(key, 10)
	})
	g.notFinishedProposals = common.NewVMSlot[[]NotFinishedProposal](g.StateAccount, notFinishedProposalsStorageKey)
}

func (g *Governance) Init() {
	g.proposalHandlerMap = make(map[ProposalType]ProposalHandler)

	// register governance proposal handler
	councilManager := NewCouncilManager(g)
	if err := g.registerHandler(CouncilElect, councilManager); err != nil {
		panic(err)
	}

	//nodeManager := NewNodeManager(g)
	//if err := g.registerHandler(NodeRegister, nodeManager); err != nil {
	//	panic(err)
	//}
	//if err := g.registerHandler(NodeRemove, nodeManager); err != nil {
	//	panic(err)
	//}
	//if err := g.registerHandler(NodeUpgrade, nodeManager); err != nil {
	//	panic(err)
	//}

	gasManager := NewGasManager(g)
	if err := g.registerHandler(GasUpdate, gasManager); err != nil {
		panic(err)
	}

	chainParamManager := NewChainParamManager(g)
	if err := g.registerHandler(ChainParamUpgrade, chainParamManager); err != nil {
		panic(err)
	}

	whiteListProviderManager := NewWhiteListProviderManager(g)
	if err := g.registerHandler(WhitelistProviderAdd, whiteListProviderManager); err != nil {
		panic(err)
	}
	if err := g.registerHandler(WhitelistProviderRemove, whiteListProviderManager); err != nil {
		panic(err)
	}
}

func (g *Governance) registerHandler(proposalType ProposalType, handler ProposalHandler) error {
	if _, ok := g.proposalHandlerMap[proposalType]; ok {
		return ErrRepeatedRegister
	}
	g.proposalHandlerMap[proposalType] = handler
	return nil
}

func (g *Governance) getHandler(proposalType ProposalType) (ProposalHandler, error) {
	handler, ok := g.proposalHandlerMap[proposalType]
	if !ok {
		return nil, ErrProposalType
	}
	handler.SetContext(g.Ctx)
	return handler, nil
}

func (g *Governance) checkAndUpdateState(method string) error {
	exist, notFinishedProposals, err := g.notFinishedProposals.Get()
	if err != nil {
		return err
	}
	if !exist {
		return nil
	}

	for _, notFinishedProposal := range notFinishedProposals {
		if notFinishedProposal.DeadlineBlockNumber <= g.Ctx.BlockNumber {
			// update original proposal status
			proposalExist, proposal, err := g.proposals.Get(notFinishedProposal.ID)
			if err != nil {
				return err
			}
			if !proposalExist {
				return ErrNotFoundProposal
			}

			if proposal.Status == Approved || proposal.Status == Rejected {
				// proposal is finnished, no need update
				continue
			}

			// means proposal is out of deadline,status change to rejected
			proposal.Status = Rejected

			if err = g.proposals.Put(notFinishedProposal.ID, proposal); err != nil {
				return err
			}

			// remove proposal from not finished proposals
			if err = g.removeNotFinishedProposal(notFinishedProposal.ID); err != nil {
				return err
			}

			handler, err := g.getHandler(proposal.Type)
			if err != nil {
				return err
			}
			if err = handler.CleanProposal(&proposal); err != nil {
				return errors.Wrapf(err, "failed to clean proposal, id: %d, type: %d", proposal.ID, proposal.Type)
			}

			// todo: refactor
			switch method {
			case ProposeEvent:
				g.EmitProposeEvent(&proposal)
			case VoteEvent:
				g.EmitVoteEvent(&proposal)
			default:
				return errors.New("unknown method")
			}
		}
	}

	return nil
}

func (g *Governance) CheckProposeArgs(proposalType ProposalType, title, desc string, expiredBlockNumber uint64, currentHeight uint64) error {
	_, isVaildProposalType := g.proposalHandlerMap[proposalType]
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
	handler, err := g.getHandler(proposalType)
	if err != nil {
		return err
	}

	// check and update state
	if err := g.checkAndUpdateState(ProposeEvent); err != nil {
		return err
	}

	if err := g.CheckProposeArgs(proposalType, title, desc, blockNumber, g.Ctx.BlockNumber); err != nil {
		return err
	}

	if err := handler.ProposeArgsCheck(proposalType, title, desc, blockNumber, extra); err != nil {
		return errors.Wrapf(err, "failed to check propose args for proposal type %d", pType)
	}

	hasPermission, err := handler.ProposePermissionCheck(proposalType, g.Ctx.From)
	if err != nil {
		return errors.Wrapf(err, "failed to check propose permission for proposal type %d", pType)
	}
	if !hasPermission {
		return errors.Errorf("no permission for propose proposal[%d]", pType)
	}

	totalVotes, err := handler.TotalVotes(proposalType)
	if err != nil {
		return errors.Wrapf(err, "failed to get total votes for proposal type %d", pType)
	}

	proposerHasVotePermission, err := handler.VotePermissionCheck(proposalType, g.Ctx.From)
	if err != nil {
		return errors.Wrapf(err, "failed to check vote permission for proposal type %d", pType)
	}

	return g.createProposal(totalVotes, proposalType, title, desc, blockNumber, extra, proposerHasVotePermission)
}

func (g *Governance) createProposal(totalVotes uint64, proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte, includeUser bool) error {
	proposal := &Proposal{
		Type:               proposalType,
		Strategy:           NowProposalStrategy,
		Proposer:           g.Ctx.From.String(),
		Title:              title,
		Desc:               desc,
		BlockNumber:        blockNumber,
		Status:             Voting,
		Extra:              extra,
		CreatedBlockNumber: g.Ctx.BlockNumber,
	}

	id, err := g.nextProposalID.MustGet()
	if err != nil {
		return err
	}
	if err := g.nextProposalID.Put(id + 1); err != nil {
		return err
	}
	proposal.ID = id
	proposal.TotalVotes = totalVotes
	// proposer vote pass by default
	if includeUser {
		proposal.PassVotes = []string{proposal.Proposer}
	}

	if err = g.proposals.Put(proposal.ID, *proposal); err != nil {
		return err
	}

	// propose generate not finished proposal
	if err = g.addNotFinishedProposal(&NotFinishedProposal{
		ID:                  proposal.ID,
		DeadlineBlockNumber: proposal.BlockNumber,
		Type:                proposal.Type,
	}); err != nil {
		return err
	}

	g.EmitProposeEvent(proposal)

	return nil
}

func (g *Governance) checkBeforeVote(user ethcommon.Address, proposal *Proposal, voteResult VoteResult) (bool, error) {
	if voteResult != Pass && voteResult != Reject {
		return false, ErrVoteResult
	}

	// check if user has voted
	if lo.Contains(proposal.PassVotes, user.String()) || lo.Contains(proposal.RejectVotes, user.String()) {
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
	voteResult := VoteResult(voteRes)
	proposalExist, proposal, err := g.proposals.Get(proposalID)
	if err != nil {
		return err
	}
	if !proposalExist {
		return ErrNotFoundProposal
	}

	handler, err := g.getHandler(proposal.Type)
	if err != nil {
		return err
	}

	// check and update state
	if err := g.checkAndUpdateState(VoteEvent); err != nil {
		return err
	}

	if _, err := g.checkBeforeVote(g.Ctx.From, &proposal, voteResult); err != nil {
		return err
	}

	hasPermission, err := handler.VotePermissionCheck(proposal.Type, g.Ctx.From)
	if err != nil {
		return errors.Wrapf(err, "failed to check vote permission for proposal type %d", proposal.Type)
	}
	if !hasPermission {
		return errors.Errorf("no permission for vote proposal[%d]", proposal.Type)
	}

	switch voteResult {
	case Pass:
		proposal.PassVotes = append(proposal.PassVotes, g.Ctx.From.String())
	case Reject:
		proposal.RejectVotes = append(proposal.RejectVotes, g.Ctx.From.String())
	}
	if err = handler.UpdateVoteStatus(&proposal); err != nil {
		return errors.Wrapf(err, "failed to update vote status for proposal type %d", proposal.Type)
	}

	isProposalSaved := false
	// update not finished proposal
	if proposal.Status == Approved || proposal.Status == Rejected {
		proposal.EffectiveBlockNumber = g.Ctx.BlockNumber
		if err = g.proposals.Put(proposal.ID, proposal); err != nil {
			return err
		}
		isProposalSaved = true

		if err := g.removeNotFinishedProposal(proposal.ID); err != nil {
			return err
		}
	}

	if !isProposalSaved {
		if err = g.proposals.Put(proposal.ID, proposal); err != nil {
			return err
		}
	}

	// if proposal approved, then execute
	if proposal.Status == Approved {
		if err := handler.VotePassExecute(&proposal); err != nil {
			proposal.ExecuteSuccess = false
			proposal.ExecuteFailedMsg = err.Error()
		} else {
			proposal.ExecuteSuccess = true
		}

		if err = g.proposals.Put(proposal.ID, proposal); err != nil {
			return err
		}
	}
	if err = handler.CleanProposal(&proposal); err != nil {
		return errors.Wrapf(err, "failed to clean proposal, id: %d, type: %d", proposal.ID, proposal.Type)
	}

	g.EmitVoteEvent(&proposal)

	return nil
}

// GetLatestProposalID return current proposal lastest id
func (g *Governance) GetLatestProposalID() (uint64, error) {
	nextID, err := g.nextProposalID.MustGet()
	if err != nil {
		return 0, err
	}

	return nextID - 1, nil
}

func (g *Governance) Proposal(proposalID uint64) (*Proposal, error) {
	proposalExist, proposal, err := g.proposals.Get(proposalID)
	if err != nil {
		return nil, err
	}
	if !proposalExist {
		return nil, ErrNotFoundProposal
	}
	return &proposal, nil
}

func (g *Governance) GetCouncilMembers() ([]governance.CouncilMember, error) {
	council, err := g.council.MustGet()
	if err != nil {
		return nil, err
	}

	return lo.Map(council.Members, func(item CouncilMember, index int) governance.CouncilMember {
		return governance.CouncilMember{
			Addr:   ethcommon.HexToAddress(item.Address),
			Weight: item.Weight,
			Name:   item.Name,
		}
	}), nil
}

func (g *Governance) GetNotFinishedProposalIDs() ([]uint64, error) {
	_, notFinishedProposals, err := g.notFinishedProposals.Get()
	if err != nil {
		return nil, err
	}
	return lo.Map(notFinishedProposals, func(item NotFinishedProposal, index int) uint64 {
		return item.ID
	}), nil
}

func (g *Governance) checkFinishedAllProposal() (bool, error) {
	exist, notFinishedProposals, err := g.notFinishedProposals.Get()
	if err != nil {
		return false, err
	}
	if !exist {
		return true, nil
	}

	return len(notFinishedProposals) == 0, nil
}

func (g *Governance) isCouncilMember(user ethcommon.Address) (bool, error) {
	council, err := g.council.MustGet()
	if err != nil {
		return false, err
	}

	return lo.ContainsBy(council.Members, func(item CouncilMember) bool {
		return item.Address == user.String()
	}), nil
}

func (g *Governance) EmitProposeEvent(proposal *Proposal) {
	data, err := json.Marshal(proposal)
	if err != nil {
		panic(err)
	}
	g.EmitEvent(&governance.EventPropose{
		ProposalID:   proposal.ID,
		ProposalType: uint8(proposal.Type),
		Proposer:     ethcommon.HexToAddress(proposal.Proposer),
		Proposal:     data,
	})
}

func (g *Governance) EmitVoteEvent(proposal *Proposal) {
	data, err := json.Marshal(proposal)
	if err != nil {
		panic(err)
	}
	g.EmitEvent(&governance.EventVote{
		ProposalID:   proposal.ID,
		ProposalType: uint8(proposal.Type),
		Proposer:     ethcommon.HexToAddress(proposal.Proposer),
		Proposal:     data,
	})
}

func (g *Governance) addNotFinishedProposal(proposal *NotFinishedProposal) error {
	exist, proposals, err := g.notFinishedProposals.Get()
	if err != nil {
		return err
	}
	if !exist {
		proposals = make([]NotFinishedProposal, 0)
	}

	proposals = lo.UniqBy(append(proposals, *proposal), func(item NotFinishedProposal) uint64 {
		return item.ID
	})

	if err := g.notFinishedProposals.Put(proposals); err != nil {
		return err
	}
	return nil
}

func (g *Governance) removeNotFinishedProposal(id uint64) error {
	exist, proposals, err := g.notFinishedProposals.Get()
	if err != nil {
		return err
	}
	if !exist {
		return ErrNotFoundProposal
	}

	newProposals := lo.Filter[NotFinishedProposal](proposals, func(item NotFinishedProposal, index int) bool {
		return item.ID != id
	})

	if len(newProposals) == len(proposals) {
		return ErrNotFoundProposal
	}

	if err := g.notFinishedProposals.Put(newProposals); err != nil {
		return err
	}
	return nil
}
