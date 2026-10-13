package governance

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
)

var (
	ErrRescueInvalidArgs      = errors.New("rescue extra args invalid")
	ErrRescueInvalidTarget    = errors.New("rescue target address is zero")
	ErrRescueInvalidValue     = errors.New("rescue value must be 32 bytes hex")
	ErrRescueSystemContract   = errors.New("refuse to write system contract storage")
	ErrRescueNotYetEffective  = errors.New("rescue proposal not yet effective")
	ErrRescueExpired          = errors.New("rescue proposal expired")
	ErrRescueMismatchExecutor = errors.New("rescue proposal must be executed by RescueManager")
)

// rescueForbiddenTargets is the set of addresses that a RescueState
// proposal MUST NOT touch. A malformed or malicious proposal must never
// be able to overwrite governance itself, the whitelist, the epoch
// manager, or any system paymaster / contract registry.
var rescueForbiddenTargets = map[string]bool{
	strings.ToLower(common.GovernanceContractAddr):          true,
	strings.ToLower(common.EpochManagerContractAddr):        true,
	strings.ToLower(common.WhiteListContractAddr):           true,
	strings.ToLower(common.AXMContractAddr):                 true,
	strings.ToLower(common.AXCContractAddr):                 true,
	strings.ToLower(common.EntryPointContractAddr):          true,
	strings.ToLower(common.AccountFactoryContractAddr):      true,
	strings.ToLower(common.VerifyingPaymasterContractAddr):  true,
	strings.ToLower(common.TokenPaymasterContractAddr):      true,
	strings.ToLower(common.ProposalIDContractAddr):          true,
	strings.ToLower(common.NotFinishedProposalContractAddr): true,
	strings.ToLower(common.Addr2NameContractAddr):           true,
}

// RescueManager is a governance executor that performs a forced EVM
// storage write. It is the only path by which an off-chain operator
// can directly modify another contract's storage slot via the
// governance pipeline (and therefore via consensus).
//
// RescueState proposals follow the standard propose → vote → execute
// flow: RescueManager.Propose is the IGovenance.Propose override
// (relaxing the council check for solo deployments, see below) and
// RescueManager.Vote delegates to Governance.voteCore which runs the
// same Pass/Reject tally, status calculation and Approved → Execute
// handoff as every other proposal type. The forced write itself
// happens in RescueManager.Execute, invoked from voteCore when the
// proposal reaches Approved status.
//
// Council gating is conditional: in solo deployments (council size
// 0 or 1, including chains with no council initialised yet) any
// address may propose, because the operator may not have the admin
// key available at the moment of the incident. In multi-admin
// deployments (council size > 1) the proposer MUST be on the
// council, otherwise a leaked single key could rewrite any
// non-system contract's storage.
type RescueManager struct {
	gov *Governance
}

func NewRescueManager(gov *Governance) *RescueManager {
	return &RescueManager{gov: gov}
}

// Propose is the IGovenance.Propose override for RescueState. It runs
// the same validation as the default path (state check, basic args,
// payload check, council membership) and then delegates the
// proposal-bookkeeping to Governance.CreateProposal so the new
// proposal lands in the notFinishedProposalMgr with Status=Voting.
// The actual storage write is deferred to RescueManager.Execute,
// which runs from Governance.voteCore when the proposal reaches
// Approved status — same-tx as the final vote.
func (rm *RescueManager) Propose(pType uint8, title, desc string, blockNumber uint64, extra []byte) error {
	if ProposalType(pType) != RescueState {
		return ErrRescueMismatchExecutor
	}

	if err := rm.gov.checkAndUpdateState(ProposeMethod); err != nil {
		return err
	}

	if err := rm.gov.CheckProposeArgs(rm.gov.currentUser, RescueState, title, desc, blockNumber, rm.gov.currentHeight); err != nil {
		return err
	}

	if err := rm.ProposeCheck(RescueState, extra); err != nil {
		return err
	}

	// Council gating is relaxed in solo deployments. If the council
	// has zero or one member (single-validator, single-admin, or
	// pre-init chains with no council yet), any address is allowed
	// to propose rescue — the operator may not have the admin key
	// handy during the very incident that needs rescuing. For a
	// multi-admin deployment (council.Members > 1) the proposer MUST
	// still be on the council, otherwise a leaked single key could
	// rewrite any non-system contract's storage.
	isCouncilMember, council := CheckInCouncil(rm.gov.account, rm.gov.currentUser.String())
	if council == nil {
		// no council initialised at all — treat as solo, force empty
		// struct so the proposal bookkeeping below still works.
		council = &Council{}
	}
	if !isCouncilMember && len(council.Members) > 1 {
		return ErrNotFoundCouncilMember
	}

	// Hand off to the default CreateProposal so the proposal is
	// saved, registered with notFinishedProposalMgr (with the
	// proposer's pass-vote already counted), and logged exactly the
	// same way as every other proposal type. Execution is deferred
	// to RescueManager.Execute via Governance.voteCore.
	if err := rm.gov.CreateProposal(council, RescueState, title, desc, blockNumber, extra, true); err != nil {
		return err
	}

	// Solo short-circuit: the proposer's default pass-vote is
	// already in the tally. For council size 0 (no council) or 1
	// (single admin) this is enough to satisfy SimpleStrategy, so
	// we flip the proposal to Approved and run the forced write in
	// the same transaction — preserving the original single-tx
	// rescue behaviour for solo / single-admin deployments. For a
	// multi-admin deployment the proposer's vote is not enough and
	// the proposal stays in Voting, waiting for more council votes
	// through the standard RescueManager.Vote → voteCore path.
	id := rm.gov.GetLatestProposalID()
	proposal, err := rm.gov.Proposal(id)
	if err != nil {
		return err
	}
	if CalcProposalStatus(proposal.Strategy, proposal.TotalVotes,
		uint64(len(proposal.PassVotes)), uint64(len(proposal.RejectVotes))) != Approved {
		return nil
	}
	proposal.Status = Approved
	proposal.EffectiveBlockNumber = rm.gov.currentHeight
	if err := rm.gov.SaveProposal(proposal); err != nil {
		return err
	}
	if err := rm.gov.notFinishedProposalMgr.RemoveProposal(proposal.ID); err != nil {
		return err
	}
	rm.gov.RecordLog(VoteMethod, proposal)
	return rm.Execute(proposal)
}

// Vote is the IGovenance.Vote override for RescueState. It delegates
// the full vote bookkeeping (state check, council check, tally,
// status calculation, terminal-state cleanup and Approved → Execute
// handoff) to Governance.voteCore so rescue proposals follow the
// exact same flow as every other proposal type. We intentionally do
// NOT call rm.gov.Vote here — that would recurse through the
// override dispatch.
func (rm *RescueManager) Vote(proposalID uint64, voteRes uint8) error {
	return rm.gov.voteCore(proposalID, voteRes)
}

// Proposal delegates to the default governance storage.
func (rm *RescueManager) Proposal(proposalID uint64) (*Proposal, error) {
	return rm.gov.Proposal(proposalID)
}

// GetLatestProposalID delegates to the default governance counter.
func (rm *RescueManager) GetLatestProposalID() uint64 {
	return rm.gov.GetLatestProposalID()
}

// ProposeCheck runs at propose time. It validates the JSON payload and
// refuses proposals that target system contracts or supply a malformed
// value.
func (rm *RescueManager) ProposeCheck(proposalType ProposalType, extra []byte) error {
	if proposalType != RescueState {
		return ErrProposalType
	}
	args, err := rm.parseExtra(extra)
	if err != nil {
		return err
	}
	if args.Target == common.ZeroAddress {
		return ErrRescueInvalidTarget
	}
	if args.Value == "" {
		return ErrRescueInvalidValue
	}
	if rescueForbiddenTargets[strings.ToLower(args.Target)] {
		return ErrRescueSystemContract
	}
	// Value must decode to exactly 32 bytes (EVM storage slot width).
	valueBytes, err := hex.DecodeString(strings.TrimPrefix(args.Value, "0x"))
	if err != nil || len(valueBytes) != 32 {
		return ErrRescueInvalidValue
	}
	return nil
}

// Execute runs the forced EVM storage write. It is invoked from
// RescueManager.Propose (the IGovenance override) in the same
// transaction as the propose call, so the rescue is a single-tx
// operation: no separate vote is required. The write goes through
// stateLedger.SetState so every validator reaches the same state root.
func (rm *RescueManager) Execute(proposal *Proposal) error {
	if proposal.Type != RescueState {
		return ErrRescueMismatchExecutor
	}
	args, err := rm.parseExtra(proposal.Extra)
	if err != nil {
		rm.gov.logger.WithError(err).Error("[RESCUE] parse extra failed")
		return err
	}

	// Bound the validity window: the proposal must be run shortly after
	// it becomes effective, otherwise the write is considered stale.
	if rm.gov.currentHeight < proposal.EffectiveBlockNumber {
		return ErrRescueNotYetEffective
	}
	if rm.gov.currentHeight > proposal.EffectiveBlockNumber+RescueValidWindow {
		return ErrRescueExpired
	}

	valueBytes, err := hex.DecodeString(strings.TrimPrefix(args.Value, "0x"))
	if err != nil || len(valueBytes) != 32 {
		rm.gov.logger.WithFields(logrus.Fields{
			"target": args.Target,
			"slot":   args.Slot,
			"value":  args.Value,
		}).Error("[RESCUE] invalid value at execute time")
		return ErrRescueInvalidValue
	}

	target := types.NewAddressByStr(args.Target)
	slotBytes := slotToBytes(args.Slot)

	// The consensus-critical write. All validators run this and the
	// resulting state root must match, so any divergence aborts the
	// block.
	rm.gov.stateLedger.SetState(target, slotBytes, valueBytes)

	rm.gov.logger.WithFields(logrus.Fields{
		"proposal_id":      proposal.ID,
		"effective_height": proposal.EffectiveBlockNumber,
		"current_height":   rm.gov.currentHeight,
		"target":           args.Target,
		"slot":             args.Slot,
		"value":            args.Value,
		"proposer":         proposal.Proposer,
	}).Warn("[RESCUE] forced storage write applied via governance")

	return nil
}

func (rm *RescueManager) parseExtra(extra []byte) (*RescueExtraArgs, error) {
	args := &RescueExtraArgs{}
	if err := json.Unmarshal(extra, args); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRescueInvalidArgs, err.Error())
	}
	return args, nil
}

// slotToBytes encodes a uint64 storage slot as a 32-byte big-endian
// word — the canonical form used by go-ethereum's state accessors.
// MUST always be 32 bytes: a uint64 only uses the trailing 8 bytes,
// but EVM storage keys are fixed-width, so we left-pad with zeros.
// (big.Int.Bytes() drops leading zeros and returns []byte{} for 0,
// which is why we can't just call it directly.)
func slotToBytes(slot uint64) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint64(b[24:], slot)
	return b
}
