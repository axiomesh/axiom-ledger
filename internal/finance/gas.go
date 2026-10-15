package finance

import (
	"math"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type Gas struct {
	repo   *repo.Repo
	logger logrus.FieldLogger
}

func NewGas(repo *repo.Repo) *Gas {
	logger := loggers.Logger(loggers.Finance)
	return &Gas{repo: repo, logger: logger}
}

// CalNextGasPrice returns the current block gas price, based on the formula:
//
// G_c = G_p * (1 + (0.5 * (MaxBatchSize - TxsCount) / MaxBatchSize))
//
// if G_c <= minGasPrice, G_c = minGasPrice
//
// if G_c >= maxGasPrice, G_c = maxGasPrice
func (gas *Gas) CalNextGasPrice(parentGasPrice uint64, txs int) (uint64, error) {
	currentEpoch := gas.repo.EpochInfo
	max := currentEpoch.FinanceParams.MaxGasPrice
	min := currentEpoch.FinanceParams.MinGasPrice
	if parentGasPrice < min || parentGasPrice > max {
		// Out-of-range parent gas price is treated as a recoverable condition
		// (e.g. legacy chain-meta with 0, or a corrupt meta where a hash was
		// written into the gas-price field). Clamp to the configured bounds
		// instead of aborting block processing.
		gas.logger.Warningf("parent gas price %d out of range [%d, %d], clamping", parentGasPrice, min, max)
		if parentGasPrice < min {
			parentGasPrice = min
		} else {
			parentGasPrice = max
		}
	}
	total := int(gas.repo.EpochInfo.ConsensusParams.BlockMaxTxNum)
	if txs > total {
		return 0, ErrTxsOutOfRange
	}
	percentage := 2 * float64(txs-total/2) / float64(total) * float64(currentEpoch.FinanceParams.GasChangeRateValue) / math.Pow10(int(currentEpoch.FinanceParams.GasChangeRateDecimals))
	currentPrice := uint64(float64(parentGasPrice) * (1 + percentage))
	if currentPrice > max {
		gas.logger.Warningf("gas price is touching ceiling, current price is %d, max is %d", currentPrice, max)
		currentPrice = max
	}
	if currentPrice < min {
		gas.logger.Warningf("gas price is touching floor, current price is %d, min is %d", currentPrice, min)
		currentPrice = min
	}
	return currentPrice, nil
}
