package governance

import (
	"encoding/json"
	"math/big"

	"github.com/pkg/errors"

	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var _ ProposalHandler = (*GasManager)(nil)

var (
	ErrExistNotFinishedGasProposal     = errors.New("exist not gas finished proposal")
	ErrExistNotFinishedCouncilProposal = errors.New("exist not finished council proposal")
	ErrRepeatedGasInfo                 = errors.New("repeated gas info")
)

type GasExtraArgs struct {
	MinGasPrice uint64 // mol
}

type GasManager struct {
	gov *Governance
	DefaultProposalPermissionManager
}

func NewGasManager(gov *Governance) *GasManager {
	return &GasManager{
		gov:                              gov,
		DefaultProposalPermissionManager: NewDefaultProposalPermissionManager(gov),
	}
}

func (gm *GasManager) SetContext(ctx *common.VMContext) {}

func (gm *GasManager) GenesisInit(genesis *repo.GenesisConfig) error {
	return nil
}

func (gm *GasManager) ProposeArgsCheck(proposalType ProposalType, title, desc string, blockNumber uint64, extra []byte) error {
	_, err := gm.getGasProposalExtraArgs(extra)
	if err != nil {
		return err
	}

	// Check for outstanding council proposals and gas proposals
	if _, err := gm.checkNotFinishedProposal(); err != nil {
		return err
	}
	return nil
}

func (gm *GasManager) VotePassExecute(proposal *Proposal) error {
	extraArgs, err := gm.getGasProposalExtraArgs(proposal.Extra)
	if err != nil {
		return err
	}

	epochManagerContract := framework.EpochManagerBuildConfig.Build(gm.gov.CrossCallSystemContractContext())
	nextEpochInfo, err := epochManagerContract.NextEpoch()
	if err != nil {
		return err
	}
	nextEpochInfo.FinanceParams.MinGasPrice = new(big.Int).SetUint64(extraArgs.MinGasPrice)

	if err := epochManagerContract.UpdateNextEpoch(nextEpochInfo); err != nil {
		return err
	}

	return nil
}

func (gm *GasManager) checkNotFinishedProposal() (bool, error) {
	_, notFinishedProposals, err := gm.gov.notFinishedProposals.Get()
	if err != nil {
		return false, err
	}
	for _, notFinishedProposal := range notFinishedProposals {
		if notFinishedProposal.Type == CouncilElect {
			return false, ErrExistNotFinishedCouncilProposal
		}
		if notFinishedProposal.Type == GasUpdate {
			return false, ErrExistNotFinishedGasProposal
		}
	}
	return true, nil
}

func (gm *GasManager) getGasProposalExtraArgs(extra []byte) (*GasExtraArgs, error) {
	extraArgs := &GasExtraArgs{}
	if err := json.Unmarshal(extra, extraArgs); err != nil {
		return nil, errors.Wrap(err, "unmarshal gas extra arguments error")
	}

	// Check whether the gas proposal and GetNextEpochInfo are consistent
	epochManagerContract := framework.EpochManagerBuildConfig.Build(gm.gov.CrossCallSystemContractContext())
	nextEpochInfo, err := epochManagerContract.NextEpoch()
	if err != nil {
		return nil, err
	}
	financeParams := nextEpochInfo.FinanceParams
	if financeParams.MinGasPrice.Cmp(new(big.Int).SetUint64(extraArgs.MinGasPrice)) == 0 {
		return nil, ErrRepeatedGasInfo
	}

	return extraArgs, nil
}

func (gm *GasManager) CleanProposal(proposal *Proposal) error {
	return nil
}
