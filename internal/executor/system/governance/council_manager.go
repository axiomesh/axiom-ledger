package governance

import (
	"encoding/json"
	"errors"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	vm "github.com/axiomesh/eth-kit/evm"
)

var (
	ErrCouncilNumber            = errors.New("council members total count can't bigger than candidates count")
	ErrMinCouncilMembersCount   = errors.New("council members count can't less than 4")
	ErrRepeatedAddress          = errors.New("council member address repeated")
	ErrRepeatedName             = errors.New("council member name repeated")
	ErrNotFoundCouncilMember    = errors.New("council member is not found")
	ErrNotFoundCouncil          = errors.New("council is not found")
	ErrCouncilExtraArgs         = errors.New("unmarshal council extra arguments error")
	ErrNotFoundCouncilProposal  = errors.New("council proposal not found for the id")
	ErrExistNotFinishedProposal = errors.New("exist not finished proposal, must finished all proposal then propose council proposal")
	ErrDeadlineBlockNumber      = errors.New("can't vote, proposal is out of deadline block number")
)

const (
	// CouncilProposalKey is key for CouncilProposal storage
	CouncilProposalKey = "councilProposalKey"

	// CouncilKey is key for council storage
	CouncilKey = "councilKey"

	// MinCouncilMembersCount is min council members count
	MinCouncilMembersCount = 4
)

// CouncilExtraArgs is council proposal extra arguments
type CouncilExtraArgs struct {
	Candidates []*CouncilMember
}

// CouncilProposalArgs is council proposal arguments
type CouncilProposalArgs struct {
	BaseProposalArgs
	CouncilExtraArgs
}

// CouncilProposal is storage of council proposal
type CouncilProposal struct {
	BaseProposal
	Candidates []*CouncilMember
}

// Council is storage of council
type Council struct {
	Members []*CouncilMember
}

type CouncilMember struct {
	Address string
	Weight  uint64
	Name    string
}

// CouncilVoteArgs is council vote arguments
type CouncilVoteArgs struct {
	BaseVoteArgs
}

var _ common.SystemContract = (*CouncilManager)(nil)

type CouncilManager struct {
	gov *Governance

	account                ledger.IAccount
	stateLedger            ledger.StateLedger
	currentLog             *common.Log
	proposalID             *ProposalID
	addr2NameSystem        *Addr2NameSystem
	lastHeight             uint64
	notFinishedProposalMgr *NotFinishedProposalMgr
}

func NewCouncilManager(cfg *common.SystemContractConfig) *CouncilManager {
	gov, err := NewGov([]ProposalType{CouncilElect}, cfg.Logger)
	if err != nil {
		panic(err)
	}

	return &CouncilManager{
		gov: gov,
	}
}

func (cm *CouncilManager) Reset(lastHeight uint64, stateLedger ledger.StateLedger) {
	addr := types.NewAddressByStr(common.CouncilManagerContractAddr)
	cm.account = stateLedger.GetOrCreateAccount(addr)
	cm.stateLedger = stateLedger
	cm.currentLog = &common.Log{
		Address: addr,
	}
	cm.proposalID = NewProposalID(stateLedger)
	cm.addr2NameSystem = NewAddr2NameSystem(stateLedger)
	cm.notFinishedProposalMgr = NewNotFinishedProposalMgr(stateLedger)

	// check and update
	cm.checkAndUpdateState(lastHeight)
	cm.lastHeight = lastHeight
}

func (cm *CouncilManager) Run(msg *vm.Message) (result *vm.ExecutionResult, err error) {
	defer cm.gov.SaveLog(cm.stateLedger, cm.currentLog)

	// parse method and arguments from msg payload
	args, err := cm.gov.GetArgs(msg)
	if err != nil {
		return nil, err
	}

	switch v := args.(type) {
	case *ProposalArgs:
		councilArgs := &CouncilProposalArgs{
			BaseProposalArgs: v.BaseProposalArgs,
		}

		extraArgs := &CouncilExtraArgs{}
		if err = json.Unmarshal(v.Extra, extraArgs); err != nil {
			return nil, ErrCouncilExtraArgs
		}

		councilArgs.CouncilExtraArgs = *extraArgs

		result, err = cm.propose(msg.From, councilArgs)
	case *VoteArgs:
		voteArgs := &CouncilVoteArgs{
			BaseVoteArgs: v.BaseVoteArgs,
		}

		result, err = cm.vote(msg.From, voteArgs)
	case *GetProposalArgs:
		result, err = cm.getProposal(v.ProposalID)
	default:
		return nil, errors.New("unknown proposal args")
	}

	if result != nil {
		usedGas := common.CalculateDynamicGas(msg.Data)
		result.UsedGas = usedGas
	}
	return result, err
}

func (cm *CouncilManager) propose(addr ethcommon.Address, args *CouncilProposalArgs) (*vm.ExecutionResult, error) {
	cm.gov.logger.Debugf("Propose council election, addr: %s, args: %+v", addr.String(), args)
	for i, candidate := range args.Candidates {
		cm.gov.logger.Debugf("candidate %d: %+v", i, *candidate)
	}
	baseProposal, err := cm.gov.Propose(&addr, ProposalType(args.ProposalType), args.Title, args.Desc, args.BlockNumber, cm.lastHeight, false)
	if err != nil {
		return nil, err
	}

	// check proposal council member num
	if len(args.Candidates) < MinCouncilMembersCount {
		return nil, ErrMinCouncilMembersCount
	}

	// check proposal candidates has repeated address
	if len(lo.Uniq[string](lo.Map[*CouncilMember, string](args.Candidates, func(item *CouncilMember, index int) string {
		return item.Address
	}))) != len(args.Candidates) {
		return nil, ErrRepeatedAddress
	}

	// set proposal id
	proposal := &CouncilProposal{
		BaseProposal: *baseProposal,
	}

	isExist, council := CheckInCouncil(cm.account, addr.String())
	if !isExist {
		return nil, ErrNotFoundCouncilMember
	}

	if !checkAddr2Name(args.Candidates) {
		return nil, ErrRepeatedName
	}

	if !cm.checkFinishedAllProposal() {
		return nil, ErrExistNotFinishedProposal
	}

	id, err := cm.proposalID.GetAndAddID()
	if err != nil {
		return nil, err
	}
	proposal.ID = id

	proposal.TotalVotes = lo.Sum[uint64](lo.Map[*CouncilMember, uint64](council.Members, func(item *CouncilMember, index int) uint64 {
		return item.Weight
	}))
	proposal.Candidates = args.Candidates

	b, err := cm.saveProposal(proposal)
	if err != nil {
		return nil, err
	}

	// propose generate not finished proposal
	if err = cm.notFinishedProposalMgr.SetProposal(&NotFinishedProposal{
		ID:                  proposal.ID,
		DeadlineBlockNumber: proposal.BlockNumber,
		ContractAddr:        common.CouncilManagerContractAddr,
	}); err != nil {
		return nil, err
	}

	returnData, err := cm.gov.PackOutputArgs(ProposeMethod, id)

	// record log
	cm.gov.RecordLog(cm.currentLog, ProposeMethod, &proposal.BaseProposal, b)

	return &vm.ExecutionResult{
		ReturnData: returnData,
		Err:        err,
	}, nil
}

// Vote a proposal, return vote status
func (cm *CouncilManager) vote(user ethcommon.Address, voteArgs *CouncilVoteArgs) (*vm.ExecutionResult, error) {
	cm.gov.logger.Debugf("Vote council election, addr: %s, args: %+v", user.String(), voteArgs)
	result := &vm.ExecutionResult{}

	// check user can vote
	isExist, _ := CheckInCouncil(cm.account, user.String())
	if !isExist {
		return nil, ErrNotFoundCouncilMember
	}

	// get proposal
	proposal, err := cm.loadProposal(voteArgs.ProposalId)
	if err != nil {
		result.Err = err
		return result, nil
	}

	res := VoteResult(voteArgs.VoteResult)
	proposalStatus, err := cm.gov.Vote(&user, &proposal.BaseProposal, res)
	if err != nil {
		return nil, err
	}
	proposal.Status = proposalStatus

	b, err := cm.saveProposal(proposal)
	if err != nil {
		return nil, err
	}

	// update not finished proposal
	if proposal.Status == Approved || proposal.Status == Rejected {
		if err := cm.notFinishedProposalMgr.RemoveProposal(proposal.ID); err != nil {
			return nil, err
		}
	}

	// if proposal is approved, update the council members
	if proposal.Status == Approved {
		council := &Council{
			Members: proposal.Candidates,
		}

		// save council
		cb, err := json.Marshal(council)
		if err != nil {
			return nil, err
		}
		cm.account.SetState([]byte(CouncilKey), cb)

		// set name when proposal approved
		setName(cm.addr2NameSystem, council.Members)

		for i, member := range council.Members {
			cm.gov.logger.Debugf("after vote, now council member %d, %+v", i, *member)
		}
	}

	cm.gov.RecordLog(cm.currentLog, VoteMethod, &proposal.BaseProposal, b)

	return result, nil
}

func (cm *CouncilManager) loadProposal(id uint64) (*CouncilProposal, error) {
	return loadCouncilProposal(cm.stateLedger, id)
}

func (cm *CouncilManager) saveProposal(proposal *CouncilProposal) ([]byte, error) {
	return saveCouncilProposal(cm.stateLedger, proposal)
}

// getProposal view proposal details
func (cm *CouncilManager) getProposal(proposalID uint64) (*vm.ExecutionResult, error) {
	result := &vm.ExecutionResult{}

	isExist, b := cm.account.GetState([]byte(fmt.Sprintf("%s%d", CouncilProposalKey, proposalID)))
	if isExist {
		packed, err := cm.gov.PackOutputArgs(ProposalMethod, b)
		if err != nil {
			return nil, err
		}
		result.ReturnData = packed
		return result, nil
	}
	return nil, ErrNotFoundCouncilProposal
}

func (cm *CouncilManager) EstimateGas(callArgs *types.CallArgs) (uint64, error) {
	_, err := cm.gov.GetArgs(&vm.Message{Data: *callArgs.Data})
	if err != nil {
		return 0, err
	}

	return common.CalculateDynamicGas(*callArgs.Data), nil
}

func (cm *CouncilManager) checkAndUpdateState(lastHeight uint64) {
	if err := CheckAndUpdateState(lastHeight, cm.stateLedger); err != nil {
		cm.gov.logger.Errorf("check and update state error: %s", err)
	}
}

func InitCouncilMembers(lg ledger.StateLedger, admins []*repo.Admin, initBlance string) error {
	addr2NameSystem := NewAddr2NameSystem(lg)

	council := &Council{}
	for _, admin := range admins {
		council.Members = append(council.Members, &CouncilMember{
			Address: admin.Address,
			Weight:  admin.Weight,
			Name:    admin.Name,
		})

		// set name
		addr2NameSystem.SetName(admin.Address, admin.Name)
	}

	account := lg.GetOrCreateAccount(types.NewAddressByStr(common.CouncilManagerContractAddr))
	b, err := json.Marshal(council)
	if err != nil {
		return err
	}
	account.SetState([]byte(CouncilKey), b)
	return nil
}

func (cm *CouncilManager) checkFinishedAllProposal() bool {
	notFinishedProposals, err := GetNotFinishedProposals(cm.stateLedger)
	if err != nil {
		cm.gov.logger.Errorf("get not finished proposals error: %s", err)
		return false
	}

	return len(notFinishedProposals) == 0
}

func CheckInCouncil(account ledger.IAccount, addr string) (bool, *Council) {
	// check council if is exist
	council := &Council{}
	council, isExistCouncil := GetCouncil(account)
	if !isExistCouncil {
		return false, nil
	}

	// check addr if is exist in council
	isExist := common.IsInSlice[string](addr, lo.Map[*CouncilMember, string](council.Members, func(item *CouncilMember, index int) string {
		return item.Address
	}))
	if !isExist {
		return false, nil
	}

	return true, council
}

func GetCouncil(account ledger.IAccount) (*Council, bool) {
	// Check if the council exists in the account's state
	isExist, data := account.GetState([]byte(CouncilKey))
	if !isExist {
		return nil, false
	}

	// Unmarshal the data into a Council object
	council := &Council{}
	if err := json.Unmarshal(data, council); err != nil {
		return nil, false
	}

	return council, true
}

func checkAddr2Name(members []*CouncilMember) bool {
	// repeated name return false
	return len(lo.Uniq[string](lo.Map[*CouncilMember, string](members, func(item *CouncilMember, index int) string {
		return item.Name
	}))) == len(members)
}

func setName(addr2NameSystem *Addr2NameSystem, members []*CouncilMember) {
	for _, member := range members {
		addr2NameSystem.SetName(member.Address, member.Name)
	}
}

func loadCouncilProposal(stateLedger ledger.StateLedger, id uint64) (*CouncilProposal, error) {
	addr := types.NewAddressByStr(common.CouncilManagerContractAddr)
	account := stateLedger.GetOrCreateAccount(addr)

	isExist, b := account.GetState([]byte(fmt.Sprintf("%s%d", CouncilProposalKey, id)))
	if isExist {
		proposal := &CouncilProposal{}
		if err := json.Unmarshal(b, proposal); err != nil {
			return nil, err
		}
		return proposal, nil
	}

	return nil, ErrNotFoundCouncilProposal
}

func saveCouncilProposal(stateLedger ledger.StateLedger, proposal ProposalObject) ([]byte, error) {
	addr := types.NewAddressByStr(common.CouncilManagerContractAddr)
	account := stateLedger.GetOrCreateAccount(addr)

	b, err := json.Marshal(proposal)
	if err != nil {
		return nil, err
	}
	// save proposal
	account.SetState([]byte(fmt.Sprintf("%s%d", CouncilProposalKey, proposal.GetID())), b)

	return b, nil
}
