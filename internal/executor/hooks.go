package executor

import (
	"encoding/binary"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/base"
)

func (exec *BlockExecutor) updateEpochInfo(block *types.Block) {
	// check need turn into NewEpoch
	epochInfo := exec.rep.EpochInfo
	if block.Header.Number == (epochInfo.StartBlock + epochInfo.EpochPeriod - 1) {
		var seed []byte
		seed = append(seed, []byte(exec.currentBlockHash.String())...)
		seed = append(seed, []byte(block.Header.ProposerAccount)...)
		seed = binary.BigEndian.AppendUint64(seed, block.Header.Number)
		seed = binary.BigEndian.AppendUint64(seed, block.Header.Epoch)
		seed = binary.BigEndian.AppendUint64(seed, uint64(block.Header.Timestamp))
		for _, tx := range block.Transactions {
			seed = append(seed, []byte(tx.GetHash().String())...)
		}

		newEpoch, err := base.TurnIntoNewEpoch(seed, exec.ledger.StateLedger)
		if err != nil {
			panic(err)
		}
		exec.rep.EpochInfo = newEpoch
		exec.epochExchange = true
		exec.logger.WithFields(logrus.Fields{
			"height":                block.Header.Number,
			"new_epoch":             newEpoch.Epoch,
			"new_epoch_start_block": newEpoch.StartBlock,
		}).Info("Turn into new epoch")
	}
}

//func (exec *BlockExecutor) updateMiningInfo(block *types.Block) {
//	// calculate mining rewards and transfer the mining reward
//	receiver := types.NewAddressByStr(block.Header.ProposerAccount).ETHAddress()
//	if err := exec.incentive.SetMiningRewards(receiver, exec.ledger.StateLedger,
//		exec.currentHeight); err != nil {
//		exec.logger.WithFields(logrus.Fields{
//			"height": block.Height(),
//			"err":    err.Error(),
//		}).Errorf("set mining rewards error")
//		// not panic the error, since there is a chance that the balance is not enough
//		// panic(err)
//	}
//}
