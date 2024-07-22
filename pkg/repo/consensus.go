package repo

import (
	"path"
	"time"

	"github.com/pkg/errors"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-kit/types"
)

const (
	GenerateBatchByTime     = "fifo" // default
	GenerateBatchByGasPrice = "price_priority"
)

type ReceiveMsgLimiter struct {
	Enable bool  `mapstructure:"enable" toml:"enable"`
	Limit  int64 `mapstructure:"limit" toml:"limit"`
	Burst  int64 `mapstructure:"burst" toml:"burst"`
}

type ConsensusConfig struct {
	TimedGenBlock TimedGenBlock     `mapstructure:"timed_gen_block" toml:"timed_gen_block"`
	Limit         ReceiveMsgLimiter `mapstructure:"limit" toml:"limit"`
	TxPool        TxPool            `mapstructure:"tx_pool" toml:"tx_pool"`
	TxCache       TxCache           `mapstructure:"tx_cache" toml:"tx_cache"`
	Rbft          RBFT              `mapstructure:"rbft" toml:"rbft"`
	Solo          Solo              `mapstructure:"solo" toml:"solo"`
	DagBft        DagBft            `mapstructure:"dag_bft" toml:"dag_bft"`
}

type TimedGenBlock struct {
	NoTxBatchTimeout Duration `mapstructure:"no_tx_batch_timeout" toml:"no_tx_batch_timeout"`
}

type TxPool struct {
	PoolSize               uint64            `mapstructure:"pool_size" toml:"pool_size"`
	ToleranceTime          Duration          `mapstructure:"tolerance_time" toml:"tolerance_time"`
	ToleranceRemoveTime    Duration          `mapstructure:"tolerance_remove_time" toml:"tolerance_remove_time"`
	CleanEmptyAccountTime  Duration          `mapstructure:"clean_empty_account_time" toml:"clean_empty_account_time"`
	ToleranceNonceGap      uint64            `mapstructure:"tolerance_nonce_gap" toml:"tolerance_nonce_gap"`
	EnableLocalsPersist    bool              `mapstructure:"enable_locals_persist" toml:"enable_locals_persist"`
	RotateTxLocalsInterval Duration          `mapstructure:"rotate_tx_locals_interval" toml:"rotate_tx_locals_interval"`
	PriceLimit             *types.CoinNumber `mapstructure:"price_limit" toml:"price_limit"`
	PriceBump              uint64            `mapstructure:"price_bump" toml:"price_bump"`
	GenerateBatchType      string            `mapstructure:"generate_batch_type" toml:"generate_batch_type"`
}

type TxCache struct {
	SetSize    int      `mapstructure:"set_size" toml:"set_size"`
	SetTimeout Duration `mapstructure:"set_timeout" toml:"set_timeout"`
}

type RBFT struct {
	EnableMetrics             bool        `mapstructure:"enable_metrics" toml:"enable_metrics"`
	CommittedBlockCacheNumber uint64      `mapstructure:"committed_block_cache_number" toml:"committed_block_cache_number"`
	Timeout                   RBFTTimeout `mapstructure:"timeout" toml:"timeout"`
}

type RBFTTimeout struct {
	NullRequest      Duration `mapstructure:"null_request" toml:"null_request"`
	Request          Duration `mapstructure:"request" toml:"request"`
	ResendViewChange Duration `mapstructure:"resend_viewchange" toml:"resend_viewchange"`
	CleanViewChange  Duration `mapstructure:"clean_viewchange" toml:"clean_viewchange"`
	NewView          Duration `mapstructure:"new_view" toml:"new_view"`
	SyncState        Duration `mapstructure:"sync_state" toml:"sync_state"`
	SyncStateRestart Duration `mapstructure:"sync_state_restart" toml:"sync_state_restart"`
	FetchCheckpoint  Duration `mapstructure:"fetch_checkpoint" toml:"fetch_checkpoint"`
	FetchView        Duration `mapstructure:"fetch_view" toml:"fetch_view"`
	BatchTimeout     Duration `mapstructure:"batch_timeout" toml:"batch_timeout"`
}

type Solo struct {
	BatchTimeout Duration `mapstructure:"batch_timeout" toml:"batch_timeout"`
}

type DagBft struct {
	MinHeaderBatchesSize           uint64   `mapstructure:"min_header_batches_size" toml:"min_header_batches_size"`
	MaxHeaderBatchesSize           uint64   `mapstructure:"max_header_batches_size" toml:"max_header_batches_size"`
	MaxHeaderDelay                 Duration `mapstructure:"max_header_delay" toml:"max_header_delay"`
	MinHeaderDelay                 Duration `mapstructure:"min_header_delay" toml:"min_header_delay"`
	HeaderResentDelay              Duration `mapstructure:"header_resent_delay" toml:"header_resent_delay"`
	GcRoundDepth                   uint64   `mapstructure:"gc_round_depth" toml:"gc_round_depth"`
	ExpiredRoundDepth              uint64   `mapstructure:"expired_round_depth" toml:"expired_round_depth"`
	SealingBatchLimit              uint64   `mapstructure:"sealing_batch_limit" toml:"sealing_batch_limit"`
	MaxBatchCount                  uint64   `mapstructure:"max_batch_count" toml:"max_batch_count"`
	MaxBatchSize                   uint64   `mapstructure:"max_batch_size" toml:"max_batch_size"`
	MaxBatchDelay                  Duration `mapstructure:"max_batch_delay" toml:"max_batch_delay"`
	SyncRetryDelay                 Duration `mapstructure:"sync_retry_delay" toml:"sync_retry_delay"`
	SyncRetryNodes                 uint64   `mapstructure:"sync_retry_nodes" toml:"sync_retry_nodes"`
	HeartbeatsTimeout              Duration `mapstructure:"heartbeats_timeout" toml:"heartbeats_timeout"`
	EnableFastSyncRecovery         bool     `mapstructure:"enable_fast_sync_recovery" toml:"enable_fast_sync_recovery"`
	EnableLeaderReputation         bool     `mapstructure:"enable_leader_reputation" toml:"enable_leader_reputation"`
	AllowInconsistentExecuteResult bool     `mapstructure:"allow_inconsistent_execute_result" toml:"allow_inconsistent_execute_result"`
}

func DefaultConsensusConfig() *ConsensusConfig {
	if testNetConsensusConfigBuilder, ok := TestNetConsensusConfigBuilderMap[BuildNet]; ok {
		return testNetConsensusConfigBuilder()
	}
	return defaultConsensusConfig()
}

func defaultConsensusConfig() *ConsensusConfig {
	// nolint
	return &ConsensusConfig{
		TimedGenBlock: TimedGenBlock{
			NoTxBatchTimeout: Duration(2 * time.Second),
		},
		Limit: ReceiveMsgLimiter{
			Enable: false,
			Limit:  10000,
			Burst:  10000,
		},
		TxPool: TxPool{
			PoolSize:               50000,
			ToleranceTime:          Duration(5 * time.Minute),
			ToleranceRemoveTime:    Duration(15 * time.Minute),
			CleanEmptyAccountTime:  Duration(10 * time.Minute),
			RotateTxLocalsInterval: Duration(1 * time.Hour),
			ToleranceNonceGap:      1000,
			EnableLocalsPersist:    true,
			PriceLimit:             GetDefaultMinGasPrice(),
			PriceBump:              10,
			GenerateBatchType:      GenerateBatchByTime,
		},
		TxCache: TxCache{
			SetSize:    50,
			SetTimeout: Duration(100 * time.Millisecond),
		},
		Rbft: RBFT{
			EnableMetrics:             true,
			CommittedBlockCacheNumber: 10,
			Timeout: RBFTTimeout{
				NullRequest:      Duration(3 * time.Second),
				Request:          Duration(2 * time.Second),
				ResendViewChange: Duration(10 * time.Second),
				CleanViewChange:  Duration(60 * time.Second),
				NewView:          Duration(8 * time.Second),
				SyncState:        Duration(1 * time.Second),
				SyncStateRestart: Duration(10 * time.Minute),
				FetchCheckpoint:  Duration(5 * time.Second),
				FetchView:        Duration(1 * time.Second),
				BatchTimeout:     Duration(500 * time.Millisecond),
			},
		},
		Solo: Solo{
			BatchTimeout: Duration(500 * time.Millisecond),
		},
		DagBft: DagBft{
			MinHeaderBatchesSize:           10,
			MaxHeaderBatchesSize:           100,
			MaxHeaderDelay:                 Duration(200000000),
			MinHeaderDelay:                 Duration(50000000),
			HeaderResentDelay:              Duration(-1), // todo(lrx): not sure
			GcRoundDepth:                   50,
			ExpiredRoundDepth:              100,
			SealingBatchLimit:              100,
			MaxBatchCount:                  100,
			MaxBatchSize:                   500000,
			MaxBatchDelay:                  Duration(100000000),
			SyncRetryDelay:                 Duration(5000000000),
			SyncRetryNodes:                 3,
			HeartbeatsTimeout:              Duration(1000000000),
			EnableFastSyncRecovery:         true,
			EnableLeaderReputation:         true,
			AllowInconsistentExecuteResult: false,
		},
	}
}

func LoadConsensusConfig(repoRoot string) (*ConsensusConfig, error) {
	cfg, err := func() (*ConsensusConfig, error) {
		cfg := DefaultConsensusConfig()
		cfgPath := path.Join(repoRoot, consensusCfgFileName)
		existConfig := fileutil.Exist(cfgPath)
		if !existConfig {
			if err := writeConfigWithEnv(cfgPath, cfg); err != nil {
				return nil, errors.Wrap(err, "failed to build default consensus config")
			}
		} else {
			if err := ReadConfigFromFile(cfgPath, cfg); err != nil {
				return nil, err
			}
		}
		return cfg, nil
	}()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load consensus config")
	}
	return cfg, nil
}
