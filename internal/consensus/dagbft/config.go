package dagbft

import (
	"math/big"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/dagbft/adaptor"
	dagconfig "github.com/bcds/go-hpc-dagbft/common/config"
	dagtypes "github.com/bcds/go-hpc-dagbft/common/types"
	"github.com/bcds/go-hpc-dagbft/common/utils/containers"
	"github.com/bcds/go-hpc-dagbft/protocol/observer"
	"github.com/bcds/go-hpc-dagbft/test/mock"
	"github.com/samber/lo"
)

type Config struct {
	dagconfig.DAGConfigs
	// Logger is the logger used to record logger in DagBFT.
	Logger *common.Logger

	// MetricsProv is the metrics Provider used to generate metrics instance.
	MetricsProv observer.MetricProvider

	NetworkConfig *adaptor.NetworkConfig

	ChainState *chainstate.ChainState

	GetBlockHeaderFunc func(height uint64) (*types.BlockHeader, error)
	GetAccountBalance  func(address string) *big.Int
	GetAccountNonce    func(address *types.Address) uint64
}

// todo(lrx): common metrics for consensus
// todo(lrx): add primary and workers
func GenerateDagBftConfig(config *common.Config) (*Config, error) {
	defaultConfig := &Config{DAGConfigs: dagconfig.DefaultDAGConfigs}
	defaultConfig.Logger = &common.Logger{FieldLogger: config.Logger}
	defaultConfig.MetricsProv = &mock.Provider{}
	defaultConfig.GetBlockHeaderFunc = config.GetBlockHeaderFunc
	defaultConfig.GetAccountBalance = config.GetAccountBalance
	defaultConfig.GetAccountNonce = config.GetAccountNonce
	defaultConfig.ChainState = config.ChainState
	defaultConfig.NetworkConfig = adaptor.DefaultNetworkConfig()
	defaultConfig.NetworkConfig.LocalPrimary = containers.Pack2(config.ChainState.SelfNodeInfo.Primary, config.ChainState.SelfNodeInfo.P2PID)

	defaultConfig.NetworkConfig.LocalWorkers = lo.SliceToMap(config.ChainState.SelfNodeInfo.Workers, func(n dagtypes.Host) (dagtypes.Host, adaptor.Pid) {
		return n, config.ChainState.SelfNodeInfo.P2PID
	})
	return defaultConfig, nil
}
