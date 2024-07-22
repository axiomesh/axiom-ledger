package adaptor

import (
	"context"

	"github.com/axiomesh/axiom-kit/storage/kv"
	kittypes "github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/bcds/go-hpc-dagbft/common/config"
	"github.com/bcds/go-hpc-dagbft/common/types"
	"github.com/bcds/go-hpc-dagbft/common/types/events"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

type Ready struct {
	Txs            []*kittypes.Transaction
	Height         uint64
	Timestamp      int64
	ProposerNodeID uint64
	ExecutedCh     chan<- *events.ExecutedEvent
	CommitState    *types.CommitState
	EpochChangedCh chan<- struct{}
}

type DagBFTAdaptor struct {
	epochStore    kv.Storage
	narwhalConfig config.Configs
	ledgerConfig  *LedgerConfig
	Chain         *BlockChain

	logger logrus.FieldLogger

	ctx context.Context

	crashCh chan bool
}

func NewAdaptor(cnf *common.Config, networkConfig *NetworkConfig, readyC chan *Ready, dagConfig config.DAGConfigs, ctx context.Context, closeCh chan bool) (*DagBFTAdaptor, error) {
	bc, err := NewBlockchain(cnf, readyC, cnf.EpochStore, closeCh)
	if err != nil {
		return nil, err
	}

	d := &DagBFTAdaptor{
		epochStore: cnf.EpochStore,
		narwhalConfig: config.Configs{
			PrimaryNode: networkConfig.LocalPrimary.First,
			WorkerNodes: lo.MapValues(networkConfig.LocalWorkers, func(value Pid, key types.Host) bool { return true }),
			DAGConfigs:  dagConfig,
		},
		ledgerConfig: &LedgerConfig{
			ChainState:         cnf.ChainState,
			GetBlockHeaderFunc: cnf.GetBlockHeaderFunc,
		},
		Chain:  bc,
		logger: cnf.Logger,

		ctx: ctx,
	}
	return d, nil
}

func (d *DagBFTAdaptor) Start() {
	go d.Chain.listenChainEvent()
}

func (d *DagBFTAdaptor) GetLedger() layer.Ledger {
	return d.Chain
}
