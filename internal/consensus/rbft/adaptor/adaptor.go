package adaptor

import (
	"context"

	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	synccomm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	p2p "github.com/axiomesh/axiom-p2p"
)

var _ rbft.ExternalStack[types.Transaction, *types.Transaction] = (*RBFTAdaptor)(nil)
var _ rbft.Storage = (*RBFTAdaptor)(nil)
var _ rbft.Network = (*RBFTAdaptor)(nil)
var _ rbft.Crypto = (*RBFTAdaptor)(nil)
var _ rbft.ServiceOutbound[types.Transaction, *types.Transaction] = (*RBFTAdaptor)(nil)
var _ rbft.EpochService = (*RBFTAdaptor)(nil)
var _ rbft.Ledger = (*RBFTAdaptor)(nil)

type RBFTAdaptor struct {
	epochStore        kv.Storage
	store             rbft.Storage
	network           network.Network
	msgPipes          map[int32]p2p.Pipe
	ReadyC            chan *Ready
	BlockC            chan *common.CommitEvent
	logger            logrus.FieldLogger
	StateUpdating     bool
	StateUpdateHeight uint64

	currentSyncHeight uint64
	Cancel            context.CancelFunc
	config            *common.Config
	EpochInfo         *types.EpochInfo

	sync     synccomm.Sync
	quitSync chan struct{}
	ctx      context.Context

	MockBlockFeed event.Feed
}

type Ready struct {
	Txs             []*types.Transaction
	LocalList       []bool
	BatchDigest     string
	Height          uint64
	Timestamp       int64
	ProposerAccount string
	ProposerNodeID  uint64
}

func NewRBFTAdaptor(config *common.Config) (*RBFTAdaptor, error) {
	var err error
	storePath := storagemgr.GetLedgerComponentPath(config.Repo, storagemgr.Consensus)
	var store rbft.Storage
	switch config.Repo.Config.Consensus.StorageType {
	case repo.ConsensusStorageTypeMinifile:
		store, err = OpenMinifile(storePath)
	case repo.ConsensusStorageTypeRosedb:
		store, err = OpenRosedb(storePath)
	default:
		return nil, errors.Errorf("unsupported consensus storage type: %s", config.Repo.Config.Consensus.StorageType)
	}
	if err != nil {
		return nil, errors.Errorf("open consensus storage %s failed: %v", storePath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stack := &RBFTAdaptor{
		epochStore: config.EpochStore,
		store:      store,
		network:    config.Network,
		ReadyC:     make(chan *Ready, 1024),
		BlockC:     make(chan *common.CommitEvent, 1024),
		quitSync:   make(chan struct{}, 1),
		logger:     config.Logger,
		config:     config,

		sync: config.BlockSync,

		ctx:    ctx,
		Cancel: cancel,
	}

	return stack, nil
}

func (a *RBFTAdaptor) UpdateEpoch() error {
	a.EpochInfo = a.config.ChainState.EpochInfo
	return nil
}

func (a *RBFTAdaptor) SetMsgPipes(msgPipes map[int32]p2p.Pipe) {
	a.msgPipes = msgPipes
}
