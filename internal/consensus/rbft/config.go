package rbft

import (
	"time"

	"go.opentelemetry.io/otel/trace"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/metrics/disabled"
	"github.com/axiomesh/axiom-bft/common/metrics/prometheus"
	rbfttypes "github.com/axiomesh/axiom-bft/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
)

func defaultRbftConfig() rbft.Config {
	return rbft.Config{
		LastServiceState: &rbfttypes.ServiceState{
			MetaState: &rbfttypes.MetaState{
				Height: 0,
				Digest: "",
			},
			Epoch: 0,
		},
		SetSize:                   1000,
		BatchTimeout:              200 * time.Millisecond,
		RequestTimeout:            6 * time.Second,
		NullRequestTimeout:        9 * time.Second,
		VcResendTimeout:           10 * time.Second,
		CleanVCTimeout:            60 * time.Second,
		NewViewTimeout:            8 * time.Second,
		SyncStateTimeout:          1 * time.Second,
		SyncStateRestartTimeout:   10 * time.Second,
		FetchCheckpointTimeout:    5 * time.Second,
		FetchViewTimeout:          1 * time.Second,
		CheckPoolTimeout:          3 * time.Minute,
		FlowControl:               false,
		FlowControlMaxMem:         0,
		MetricsProv:               &disabled.Provider{},
		Tracer:                    trace.NewNoopTracerProvider().Tracer("axiom-ledger"),
		DelFlag:                   make(chan bool, 10),
		Logger:                    nil,
		NoTxBatchTimeout:          0,
		CheckPoolRemoveTimeout:    15 * time.Minute,
		CommittedBlockCacheNumber: 10,
	}
}

func generateRbftConfig(config *common.Config) (rbft.Config, error) {
	readConfig := config.Repo.ConsensusConfig

	currentEpoch := config.ChainState.GetCurrentEpochInfo()

	defaultConfig := defaultRbftConfig()
	defaultConfig.GenesisEpochInfo = config.GenesisEpochInfo
	defaultConfig.SelfP2PNodeID = config.Network.PeerID()
	lastServiceState := &rbfttypes.ServiceState{
		MetaState: &rbfttypes.MetaState{
			Height: config.Applied,
			Digest: config.Digest,
		},
		Epoch: currentEpoch.Epoch,
	}
	// if lastHeight is less than the start block of current epoch, it means the new epoch is not started yet
	// for ex: epoch height is 100, current epoch is 2, last height is 100, start block is 101
	// if we persist the last height: 100, it means the new epoch had modified to 2
	// but the last block which height=100 is belonged to the old epoch 1
	if lastServiceState.MetaState.Height < currentEpoch.StartBlock {
		lastServiceState.Epoch = currentEpoch.Epoch - 1
	}
	defaultConfig.LastServiceState = lastServiceState
	defaultConfig.GenesisBlockDigest = config.GenesisDigest
	defaultConfig.Logger = &common.Logger{FieldLogger: config.Logger}

	if readConfig.Rbft.Timeout.Request > 0 {
		defaultConfig.RequestTimeout = readConfig.Rbft.Timeout.Request.ToDuration()
	}
	if readConfig.Rbft.Timeout.NullRequest > 0 {
		defaultConfig.NullRequestTimeout = readConfig.Rbft.Timeout.NullRequest.ToDuration()
	}
	if readConfig.Rbft.Timeout.ResendViewChange > 0 {
		defaultConfig.VcResendTimeout = readConfig.Rbft.Timeout.ResendViewChange.ToDuration()
	}
	if readConfig.Rbft.Timeout.CleanViewChange > 0 {
		defaultConfig.CleanVCTimeout = readConfig.Rbft.Timeout.CleanViewChange.ToDuration()
	}
	if readConfig.Rbft.Timeout.NewView > 0 {
		defaultConfig.NewViewTimeout = readConfig.Rbft.Timeout.NewView.ToDuration()
	}
	if readConfig.Rbft.Timeout.SyncState > 0 {
		defaultConfig.SyncStateTimeout = readConfig.Rbft.Timeout.SyncState.ToDuration()
	}
	if readConfig.Rbft.Timeout.SyncStateRestart > 0 {
		defaultConfig.SyncStateRestartTimeout = readConfig.Rbft.Timeout.SyncStateRestart.ToDuration()
	}
	if readConfig.Rbft.Timeout.FetchCheckpoint > 0 {
		defaultConfig.FetchCheckpointTimeout = readConfig.Rbft.Timeout.FetchCheckpoint.ToDuration()
	}
	if readConfig.Rbft.Timeout.FetchView > 0 {
		defaultConfig.FetchViewTimeout = readConfig.Rbft.Timeout.FetchView.ToDuration()
	}
	if readConfig.Rbft.Timeout.BatchTimeout > 0 {
		defaultConfig.BatchTimeout = readConfig.Rbft.Timeout.BatchTimeout.ToDuration()
	}
	if readConfig.TxPool.ToleranceTime > 0 {
		defaultConfig.CheckPoolTimeout = readConfig.TxPool.ToleranceTime.ToDuration()
	}
	if readConfig.Rbft.EnableMetrics {
		defaultConfig.MetricsProv = &prometheus.Provider{
			Name: "rbft",
		}
	}

	if readConfig.Rbft.CommittedBlockCacheNumber > 0 {
		defaultConfig.CommittedBlockCacheNumber = readConfig.Rbft.CommittedBlockCacheNumber
	}

	return defaultConfig, nil
}
