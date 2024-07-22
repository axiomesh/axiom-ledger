package eth

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-ledger/api/jsonrpc/namespaces/eth/oracle"
	rpctypes "github.com/axiomesh/axiom-ledger/api/jsonrpc/types"
	"github.com/axiomesh/axiom-ledger/internal/coreapi/api"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

// GasPriceAPI provides an API to get related info
type GasPriceAPI struct {
	ctx    context.Context
	cancel context.CancelFunc
	rep    *repo.Repo
	api    api.CoreAPI
	logger logrus.FieldLogger

	gasPriceOracle *oracle.Oracle
}

func NewGasPriceAPI(rep *repo.Repo, api api.CoreAPI, logger logrus.FieldLogger) *GasPriceAPI {
	ctx, cancel := context.WithCancel(context.Background())
	return &GasPriceAPI{ctx: ctx, cancel: cancel, rep: rep, api: api, logger: logger, gasPriceOracle: oracle.NewOracle(rep, api, logger)}
}

// legacy tx

// GasPrice returns the current gas price based on dynamic adjustment strategy.
func (api *GasPriceAPI) GasPrice(_ context.Context) (ret *hexutil.Big, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	res, err := api.gasPriceOracle.SuggestTipCap()
	if err != nil {
		return nil, err
	}

	minGasPrice := api.api.ChainState().GetCurrentEpochInfo().FinanceParams.MinGasPrice.ToBigInt()
	if minGasPrice.Cmp(res) > 0 {
		res = minGasPrice
	}

	return (*hexutil.Big)(res), nil
}

// eip1559 tx

// MaxPriorityFeePerGas returns a suggestion for a gas tip cap for dynamic transactions.
func (api *GasPriceAPI) MaxPriorityFeePerGas(_ context.Context) (ret *hexutil.Big, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	res, err := api.gasPriceOracle.SuggestTipCap()
	if err != nil {
		return nil, err
	}
	minGasPrice := api.api.ChainState().GetCurrentEpochInfo().FinanceParams.MinGasPrice.ToBigInt()
	if minGasPrice.Cmp(res) > 0 {
		res = minGasPrice
	}

	return (*hexutil.Big)(res), nil
}

// FeeHistory returns the fee market history.
func (api *GasPriceAPI) FeeHistory(_ context.Context, blockCount math.HexOrDecimal64, lastBlock rpctypes.BlockNumber, rewardPercentiles []float64) (ret *oracle.FeeHistoryResult, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	return api.gasPriceOracle.FeeHistory(blockCount, lastBlock, rewardPercentiles)
}
