package solo

import (
	"context"
	"testing"
	"time"

	"github.com/axiomesh/axiom-kit/txpool/mock_txpool"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck/mock_precheck"
	"github.com/axiomesh/axiom-ledger/internal/network/mock_network"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	poolSize         = 10
	batchTimeout     = 50 * time.Millisecond
	noTxBatchTimeout = 100 * time.Millisecond
	removeTxTimeout  = 1 * time.Second
)

var validTxsCh = make(chan *precheck.ValidTxs, maxChanSize)

func mockSoloNode(t *testing.T, enableTimed bool) (*Node, error) {
	logger := log.NewWithModule("consensus")
	logger.Logger.SetLevel(logrus.DebugLevel)
	repoRoot := t.TempDir()
	r, err := repo.Load(repo.DefaultKeyJsonPassword, repoRoot, true)
	require.Nil(t, err)

	recvCh := make(chan consensusEvent, maxChanSize)
	mockCtl := gomock.NewController(t)
	mockNetwork := mock_network.NewMockNetwork(mockCtl)

	ctx, cancel := context.WithCancel(context.Background())

	mockPool := mock_txpool.NewMockMinimalTxPool[types.Transaction, *types.Transaction](500, mockCtl)
	mockPrecheck := mock_precheck.NewMockMinPreCheck(mockCtl, mockPool)

	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	soloNode := &Node{
		config: &common.Config{
			Config:  r.ConsensusConfig,
			PrivKey: s.Sk,
		},
		lastExec:     uint64(0),
		commitC:      make(chan *common.CommitEvent, maxChanSize),
		blockCh:      make(chan *txpool.RequestHashBatch[types.Transaction, *types.Transaction], maxChanSize),
		txpool:       mockPool,
		network:      mockNetwork,
		batchDigestM: make(map[uint64]string),
		recvCh:       recvCh,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		txPreCheck:   mockPrecheck,
		epcCnf: &epochConfig{
			epochPeriod:         r.GenesisConfig.EpochInfo.EpochPeriod,
			startBlock:          r.GenesisConfig.EpochInfo.StartBlock,
			checkpoint:          r.GenesisConfig.EpochInfo.ConsensusParams.CheckpointPeriod,
			enableGenEmptyBlock: enableTimed,
		},
	}
	batchTimerMgr := timer.NewTimerManager(logger)
	err = batchTimerMgr.CreateTimer(timer.Batch, batchTimeout, soloNode.handleTimeoutEvent)
	require.Nil(t, err)
	err = batchTimerMgr.CreateTimer(timer.NoTxBatch, noTxBatchTimeout, soloNode.handleTimeoutEvent)
	require.Nil(t, err)
	soloNode.batchMgr = &batchTimerManager{Timer: batchTimerMgr}
	return soloNode, nil
}
