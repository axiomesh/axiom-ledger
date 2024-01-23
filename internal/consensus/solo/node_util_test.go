package solo

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

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
	txpool2 "github.com/axiomesh/axiom-ledger/internal/txpool"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	poolSize        = 10
	adminBalance    = 100000000000000000
	batchTimeout    = 50 * time.Millisecond
	removeTxTimeout = 1 * time.Second
)

var validTxsCh = make(chan *precheck.ValidTxs, maxChanSize)

func mockTxPool(t *testing.T) txpool.TxPool[types.Transaction, *types.Transaction] {
	logger := log.NewWithModule("consensus")
	logger.Logger.SetLevel(logrus.DebugLevel)
	repoRoot := t.TempDir()
	r, err := repo.Load(repo.DefaultKeyJsonPassword, repoRoot, true)
	require.Nil(t, err)
	txpoolConf := txpool2.Config{
		RepoRoot:            repoRoot,
		EnableLocalsPersist: false,
		IsTimed:             false,
		Logger:              &common.Logger{FieldLogger: logger},
		BatchSize:           r.GenesisConfig.EpochInfo.ConsensusParams.BlockMaxTxNum,
		PoolSize:            poolSize,
		ToleranceRemoveTime: removeTxTimeout,
		GetAccountNonce: func(address string) uint64 {
			return 0
		},
		GetAccountBalance: func(address string) *big.Int {
			maxGasPrice := new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e9))
			return new(big.Int).Mul(big.NewInt(math.MaxInt64), maxGasPrice)
		},
		ChainInfo: &txpool.ChainInfo{Height: 1, GasPrice: new(big.Int).Mul(big.NewInt(5000), big.NewInt(1e9))},
	}

	txpoolInst, err := txpool2.NewTxPool[types.Transaction, *types.Transaction](txpoolConf)
	require.Nil(t, err)
	return txpoolInst
}

func mockSoloNode(t *testing.T, enableTimed bool) (*Node, error) {
	logger := log.NewWithModule("consensus")
	logger.Logger.SetLevel(logrus.DebugLevel)
	repoRoot := t.TempDir()
	r, err := repo.Load(repo.DefaultKeyJsonPassword, repoRoot, true)
	require.Nil(t, err)
	cfg := r.ConsensusConfig

	recvCh := make(chan consensusEvent, maxChanSize)
	mockCtl := gomock.NewController(t)
	mockNetwork := mock_network.NewMockNetwork(mockCtl)
	mockPrecheck := mock_precheck.NewMockMinPreCheck(mockCtl, validTxsCh)

	txpoolConf := txpool2.Config{
		IsTimed:             r.GenesisConfig.EpochInfo.ConsensusParams.EnableTimedGenEmptyBlock,
		Logger:              &common.Logger{FieldLogger: logger},
		BatchSize:           r.GenesisConfig.EpochInfo.ConsensusParams.BlockMaxTxNum,
		PoolSize:            poolSize,
		ToleranceRemoveTime: removeTxTimeout,
		GetAccountNonce: func(address string) uint64 {
			return 0
		},
		GetAccountBalance: func(address string) *big.Int {
			maxGasPrice := new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e9))
			return new(big.Int).Mul(big.NewInt(math.MaxInt64), maxGasPrice)
		},
		ChainInfo: &txpool.ChainInfo{Height: 1, GasPrice: new(big.Int).Mul(big.NewInt(5000), big.NewInt(1e9))},
	}
	var noTxBatchTimeout time.Duration
	if enableTimed {
		txpoolConf.IsTimed = true
		noTxBatchTimeout = 50 * time.Millisecond
	} else {
		txpoolConf.IsTimed = false
		noTxBatchTimeout = cfg.TimedGenBlock.NoTxBatchTimeout.ToDuration()
	}
	txpoolInst, err := txpool2.NewTxPool[types.Transaction, *types.Transaction](txpoolConf)
	require.Nil(t, err)
	ctx, cancel := context.WithCancel(context.Background())

	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	soloNode := &Node{
		config: &common.Config{
			Config:  r.ConsensusConfig,
			PrivKey: s.Sk,
		},
		lastExec:         uint64(0),
		isTimed:          txpoolConf.IsTimed,
		noTxBatchTimeout: noTxBatchTimeout,
		batchTimeout:     cfg.Solo.BatchTimeout.ToDuration(),
		commitC:          make(chan *common.CommitEvent, maxChanSize),
		blockCh:          make(chan *txpool.RequestHashBatch[types.Transaction, *types.Transaction], maxChanSize),
		txpool:           txpoolInst,
		network:          mockNetwork,
		batchDigestM:     make(map[uint64]string),
		checkpoint:       10,
		recvCh:           recvCh,
		logger:           logger,
		ctx:              ctx,
		cancel:           cancel,
		txPreCheck:       mockPrecheck,
	}
	batchTimerMgr := timer.NewTimerManager(logger)
	err = batchTimerMgr.CreateTimer(timer.Batch, batchTimeout, soloNode.handleTimeoutEvent)
	require.Nil(t, err)
	err = batchTimerMgr.CreateTimer(timer.NoTxBatch, noTxBatchTimeout, soloNode.handleTimeoutEvent)
	require.Nil(t, err)
	soloNode.batchMgr = &batchTimerManager{Timer: batchTimerMgr}
	return soloNode, nil
}

func mockAddTx(node *Node, ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case txs := <-node.txPreCheck.CommitValidTxs():
				txs.LocalCheckRespCh <- &common.TxResp{
					Status: true,
				}
				err := node.txpool.AddLocalTx(txs.Txs[0])
				if err != nil {
					txs.LocalPoolRespCh <- &common.TxResp{
						Status:   false,
						ErrorMsg: err.Error(),
					}
				} else {
					txs.LocalPoolRespCh <- &common.TxResp{
						Status: true,
					}
				}
			}
		}
	}()
}
