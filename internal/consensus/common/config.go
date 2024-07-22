package common

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type Config struct {
	Repo               *repo.Repo
	ChainState         *chainstate.ChainState
	Logger             logrus.FieldLogger
	GenesisEpochInfo   *types.EpochInfo
	Network            network.Network
	BlockSync          common.Sync
	TxPool             txpool.TxPool[types.Transaction, *types.Transaction]
	Applied            uint64
	Digest             string
	GenesisDigest      string
	GetBlockHeaderFunc func(height uint64) (*types.BlockHeader, error)
	GetAccountBalance  func(address string) *big.Int
	GetAccountNonce    func(address *types.Address) uint64
	NotifyStop         func(err error)
	EpochStore         kv.Storage
}

type Option func(*Config)

func WithRepo(rep *repo.Repo) Option {
	return func(config *Config) {
		config.Repo = rep
	}
}

func WithTxPool(tp txpool.TxPool[types.Transaction, *types.Transaction]) Option {
	return func(config *Config) {
		config.TxPool = tp
	}
}

func WithGenesisEpochInfo(genesisEpochInfo *types.EpochInfo) Option {
	return func(config *Config) {
		config.GenesisEpochInfo = genesisEpochInfo
	}
}

func WithNetwork(net network.Network) Option {
	return func(config *Config) {
		config.Network = net
	}
}

func WithBlockSync(blockSync common.Sync) Option {
	return func(config *Config) {
		config.BlockSync = blockSync
	}
}

func WithChainState(chainState *chainstate.ChainState) Option {
	return func(config *Config) {
		config.ChainState = chainState
	}
}

func WithLogger(logger logrus.FieldLogger) Option {
	return func(config *Config) {
		config.Logger = logger
	}
}

func WithApplied(height uint64) Option {
	return func(config *Config) {
		config.Applied = height
	}
}

func WithDigest(digest string) Option {
	return func(config *Config) {
		config.Digest = digest
	}
}

func WithGenesisDigest(digest string) Option {
	return func(config *Config) {
		config.GenesisDigest = digest
	}
}

func WithGetBlockHeaderFunc(f func(height uint64) (*types.BlockHeader, error)) Option {
	return func(config *Config) {
		config.GetBlockHeaderFunc = f
	}
}

func WithGetAccountBalanceFunc(f func(address string) *big.Int) Option {
	return func(config *Config) {
		config.GetAccountBalance = f
	}
}

func WithGetAccountNonceFunc(f func(address *types.Address) uint64) Option {
	return func(config *Config) {
		config.GetAccountNonce = f
	}
}

func WithEpochStore(epochStore kv.Storage) Option {
	return func(config *Config) {
		config.EpochStore = epochStore
	}
}

func WithNotifyStopCh(f func(err error)) Option {
	return func(config *Config) {
		config.NotifyStop = f
	}
}

func checkConfig(config *Config) error {
	if config.Logger == nil {
		return errors.New("logger is nil")
	}

	return nil
}

func GenerateConfig(opts ...Option) (*Config, error) {
	config := &Config{}
	for _, opt := range opts {
		opt(config)
	}

	if err := checkConfig(config); err != nil {
		return nil, fmt.Errorf("create consensus: %w", err)
	}

	return config, nil
}

type Logger struct {
	logrus.FieldLogger
}

// Trace implements rbft.Logger.
func (lg *Logger) Trace(name string, stage string, content any) {
	lg.Info(name, stage, content)
}

func (lg *Logger) Critical(v ...any) {
	lg.Fatal(v...)
}

func (lg *Logger) Criticalf(format string, v ...any) {
	lg.Fatalf(format, v...)
}

func (lg *Logger) Notice(v ...any) {
	lg.Info(v...)
}

func (lg *Logger) Noticef(format string, v ...any) {
	lg.Infof(format, v...)
}

func GetQuorum(consensusType string, N int) uint64 {
	switch consensusType {
	case repo.ConsensusTypeRbft:
		f := (N - 1) / 3
		return uint64((N + f + 2) / 2)
	case repo.ConsensusTypeSolo, repo.ConsensusTypeSoloDev:
		fallthrough
	default:
		return 0
	}
}
