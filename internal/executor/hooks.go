package executor

import (
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
)

func (exec *BlockExecutor) tryTurnIntoNewEpoch(block *types.Block) error {
	// check need turn into NewEpoch
	epochInfo := exec.chainState.GetCurrentEpochInfo()
	if block.Header.Number == (epochInfo.StartBlock + epochInfo.EpochPeriod - 1) {
		epochManagerContract := framework.EpochManagerBuildConfig.Build(syscommon.NewVMContextByExecutor(exec.ledger.StateLedger))
		nodeManagerContract := framework.NodeManagerBuildConfig.Build(syscommon.NewVMContextByExecutor(exec.ledger.StateLedger))
		stakingManagerContract := framework.StakingManagerBuildConfig.Build(syscommon.NewVMContextByExecutor(exec.ledger.StateLedger))

		newEpoch, err := epochManagerContract.TurnIntoNewEpoch()
		if err != nil {
			return err
		}
		typesNewEpoch := newEpoch.ToTypesEpoch()

		if err := stakingManagerContract.TurnIntoNewEpoch(&epochInfo, typesNewEpoch); err != nil {
			return err
		}
		if err := nodeManagerContract.TurnIntoNewEpoch(&epochInfo, typesNewEpoch); err != nil {
			return err
		}

		votingPowers, err := nodeManagerContract.GetActiveValidatorVotingPowers()
		if err != nil {
			return err
		}

		if err := exec.chainState.UpdateByEpochInfo(typesNewEpoch, lo.SliceToMap(votingPowers, func(item node_manager.ConsensusVotingPower) (uint64, int64) {
			return item.NodeID, item.ConsensusVotingPower
		})); err != nil {
			return err
		}

		exec.logger.WithFields(logrus.Fields{
			"height":                block.Header.Number,
			"new_epoch":             newEpoch.Epoch,
			"new_epoch_start_block": newEpoch.StartBlock,
		}).Info("Turn into new epoch")
	}
	return nil
}

func (exec *BlockExecutor) recordReward(block *types.Block) error {
	// calculate mining rewards and transfer the mining reward
	stakingManager := framework.StakingManagerBuildConfig.Build(syscommon.NewVMContextByExecutor(exec.ledger.StateLedger))
	stakeReword, err := stakingManager.RecordReward(block.Header.ProposerNodeID, block.Header.GasFeeReward)
	if err != nil {
		return err
	}
	exec.logger.Debugf("RecordReward to node %d, gas reward: %s, stake reward: %s", block.Header.ProposerNodeID, block.Header.GasFeeReward, stakeReword)

	exec.logger.WithFields(logrus.Fields{
		"node":         block.Header.Number,
		"gas_reward":   block.Header.GasFeeReward,
		"stake_reward": stakeReword,
	}).Info("Record block reward")
	return nil
}
