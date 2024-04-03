package rbft

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/axiomesh/axiom-kit/txpool/mock_txpool"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck/mock_precheck"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/adaptor"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/testutil"
	"github.com/axiomesh/axiom-ledger/internal/consensus/txcache"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var validTxsCh = make(chan *precheck.ValidTxs, 1024)

func MockMinNode(ctrl *gomock.Controller, t *testing.T) *Node {
	err := storagemgr.Initialize(repo.KVStorageTypeLeveldb, repo.KVStorageCacheSize, repo.KVStorageSync, false)
	assert.Nil(t, err)
	mockRbft := rbft.NewMockMinimalNode[types.Transaction, *types.Transaction](ctrl)
	mockRbft.EXPECT().Status().Return(rbft.NodeStatus{
		ID:     uint64(1),
		View:   uint64(1),
		Status: rbft.Normal,
	}).AnyTimes()
	logger := log.NewWithModule("consensus")
	logger.Logger.SetLevel(logrus.DebugLevel)
	consensusConf, pool := testutil.MockConsensusConfig(logger, ctrl, t)

	ctx, cancel := context.WithCancel(context.Background())
	rbftAdaptor, err := adaptor.NewRBFTAdaptor(consensusConf)
	assert.Nil(t, err)
	err = rbftAdaptor.UpdateEpoch()
	assert.Nil(t, err)

	mockPrecheckMgr := mock_precheck.NewMockMinPreCheck(ctrl, pool)

	_, err = generateRbftConfig(consensusConf)
	assert.Nil(t, err)
	node := &Node{
		config:     consensusConf,
		n:          mockRbft,
		stack:      rbftAdaptor,
		logger:     logger,
		network:    consensusConf.Network,
		ctx:        ctx,
		cancel:     cancel,
		txCache:    txcache.NewTxCache(consensusConf.Config.TxCache.SetTimeout.ToDuration(), uint64(consensusConf.Config.TxCache.SetSize), consensusConf.Logger),
		txFeed:     event.Feed{},
		txPreCheck: mockPrecheckMgr,
		txpool:     consensusConf.TxPool,
	}
	return node
}

func TestStart(t *testing.T) {
	testCase := []struct {
		name           string
		setupMocks     func(n *Node, ctrl *gomock.Controller)
		expectedErrMsg string
	}{
		{
			name: "start rbft failed",
			setupMocks: func(n *Node, ctrl *gomock.Controller) {
				r := rbft.NewMockNode[types.Transaction, *types.Transaction](ctrl)
				r.EXPECT().Start().Return(fmt.Errorf("start rbft error")).AnyTimes()
				r.EXPECT().ReportExecuted(gomock.Any()).AnyTimes()
				n.n = r
			},
			expectedErrMsg: "start rbft error",
		},
		{
			name: "start pool failed",
			setupMocks: func(n *Node, ctrl *gomock.Controller) {
				pool := mock_txpool.NewMockTxPool[types.Transaction, *types.Transaction](ctrl)
				pool.EXPECT().Start().Return(fmt.Errorf("start txpool error")).AnyTimes()
				n.txpool = pool

			},
			expectedErrMsg: "start txpool error",
		},

		{
			name: "get pool tx success",
			setupMocks: func(n *Node, ctrl *gomock.Controller) {
				n.txCache.TxSetSize = 4
				s, err := types.GenerateSigner()
				assert.Nil(t, err)
				txs := testutil.ConstructTxs(s, 10)
				data := make([][]byte, 0)
				lo.ForEach(txs, func(tx *types.Transaction, _ int) {
					d, err := tx.RbftMarshal()
					assert.Nil(t, err)
					data = append(data, d)
				})
				pool := mock_txpool.NewMockTxPool[types.Transaction, *types.Transaction](ctrl)
				pool.EXPECT().Start().Return(nil).AnyTimes()
				pool.EXPECT().GetLocalTxs().Return(data).AnyTimes()
				n.txpool = pool

			},
			expectedErrMsg: "",
		},
	}

	for _, tc := range testCase {
		ctrl := gomock.NewController(t)
		n := MockMinNode(ctrl, t)
		tc.setupMocks(n, ctrl)
		err := n.Start()
		if tc.expectedErrMsg == "" {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
			assert.Equal(t, tc.expectedErrMsg, err.Error())
		}
	}
}

func TestInit(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	node := MockMinNode(ctrl, t)

	node.config.Config.Rbft.EnableMultiPipes = false
	err := node.initConsensusMsgPipes()
	ast.Nil(err)

	node.config.Config.Rbft.EnableMultiPipes = true
	err = node.initConsensusMsgPipes()
	ast.Nil(err)
}

func TestNewNode(t *testing.T) {
	testCase := []struct {
		name           string
		setupMocks     func(consensusConf *common.Config, ctrl *gomock.Controller)
		expectedErrMsg string
	}{
		{
			name: "new node success",
			setupMocks: func(consensusConf *common.Config, ctrl *gomock.Controller) {
				// 设置mock和所需的条件
				// 例如，storagemgr初始化，logger等
			},
			expectedErrMsg: "",
		},
		{
			name: "invalid config",
			setupMocks: func(consensusConf *common.Config, ctrl *gomock.Controller) {
				// illegal config
				consensusConf.GetCurrentEpochInfoFromEpochMgrContractFunc = func() (*rbft.EpochInfo, error) {
					return nil, fmt.Errorf("get epoch info error")
				}
			},
			expectedErrMsg: "get epoch info error",
		},
		{
			name: "new adaptor err",
			setupMocks: func(consensusConf *common.Config, ctrl *gomock.Controller) {
				// illegal storage type
				consensusConf.ConsensusStorageType = "invalidType"
			},
			expectedErrMsg: "unsupported consensus storage type",
		},
		{
			name: "new rbft err",
			setupMocks: func(consensusConf *common.Config, ctrl *gomock.Controller) {
				// illegal genesis epoch
				consensusConf.GenesisEpochInfo.Epoch = 100
			},
			expectedErrMsg: "genesis epoch and start_block must be 1",
		},

		{
			name: "enable p2p limit",
			setupMocks: func(consensusConf *common.Config, ctrl *gomock.Controller) {
				// illegal genesis epoch
				consensusConf.Config.Limit.Enable = true
				consensusConf.Config.Limit.Limit = 100
				consensusConf.Config.Limit.Burst = 100
			},
			expectedErrMsg: "",
		},
	}

	for _, tc := range testCase {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ast := assert.New(t)
		err := storagemgr.Initialize(repo.KVStorageTypeLeveldb, repo.KVStorageCacheSize, repo.KVStorageSync, false)
		ast.Nil(err)

		logger := log.NewWithModule("consensus")
		consensusConf, _ := testutil.MockConsensusConfig(logger, ctrl, t)
		tc.setupMocks(consensusConf, ctrl)
		node, err := NewNode(consensusConf)

		if tc.expectedErrMsg != "" {
			ast.NotNil(err)
			ast.True(strings.Contains(err.Error(), tc.expectedErrMsg))
		} else {
			ast.Nil(err)
			ast.NotNil(node)
			node.Stop()
		}
	}
}
func TestPrepare(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	node := MockMinNode(ctrl, t)
	mockRbft := rbft.NewMockMinimalNode[types.Transaction, *types.Transaction](ctrl)
	node.n = mockRbft

	txSubscribeCh := make(chan []*types.Transaction, 1)
	sub := node.SubscribeTxEvent(txSubscribeCh)
	defer sub.Unsubscribe()

	sk, err := crypto.GenerateKey()
	ast.Nil(err)

	toAddr := crypto.PubkeyToAddress(sk.PublicKey)
	tx1, singer, err := types.GenerateTransactionAndSigner(uint64(0), types.NewAddressByStr(toAddr.String()), big.NewInt(0), []byte("hello"))
	ast.Nil(err)

	// before node started, return error
	err = node.Prepare(tx1)
	ast.NotNil(err)
	ast.Contains(err.Error(), common.ErrorConsensusStart.Error())
	<-txSubscribeCh

	err = node.Start()
	ast.Nil(err)

	mockRbft.EXPECT().Status().Return(rbft.NodeStatus{
		Status: rbft.InViewChange,
	}).Times(1)
	err = node.Ready()
	ast.Error(err)

	mockRbft.EXPECT().Status().Return(rbft.NodeStatus{
		Status: rbft.Normal,
	}).AnyTimes()
	err = node.Ready()
	ast.Nil(err)

	txCache := make(map[string]*types.Transaction)
	nonceCache := make(map[string]uint64)
	node.n.(*rbft.MockNode[types.Transaction, *types.Transaction]).EXPECT().Propose(gomock.Any(), gomock.Any()).Do(func(requests []*types.Transaction, local bool) error {
		for _, tx := range requests {
			txCache[tx.RbftGetTxHash()] = tx
			if _, ok := nonceCache[tx.GetFrom().String()]; !ok {
				nonceCache[tx.GetFrom().String()] = tx.GetNonce()
			} else if nonceCache[tx.GetFrom().String()] < tx.GetNonce() {
				nonceCache[tx.GetFrom().String()] = tx.GetNonce()
			}
		}
		return nil
	}).Return(nil).AnyTimes()

	err = node.Prepare(tx1)
	ast.Nil(err)
	<-txSubscribeCh
	tx2, err := types.GenerateTransactionWithSigner(uint64(1), types.NewAddressByStr(toAddr.String()), big.NewInt(0), []byte("hello"), singer)
	ast.Nil(err)
	err = node.Prepare(tx2)
	ast.Nil(err)
	<-txSubscribeCh

	t.Run("GetLowWatermark", func(t *testing.T) {
		node.n.(*rbft.MockNode[types.Transaction, *types.Transaction]).EXPECT().GetLowWatermark().DoAndReturn(func() uint64 {
			return 1
		}).AnyTimes()
		lowWatermark := node.GetLowWatermark()
		ast.Equal(uint64(1), lowWatermark)
	})

	t.Run("prepare tx failed", func(t *testing.T) {
		wrongPrecheckMgr := mock_precheck.NewMockPreCheck(ctrl)
		wrongPrecheckMgr.EXPECT().Start().AnyTimes()
		wrongPrecheckMgr.EXPECT().PostUncheckedTxEvent(gomock.Any()).Do(func(ev *common.UncheckedTxEvent) {
			txWithResp := ev.Event.(*common.TxWithResp)
			txWithResp.CheckCh <- &common.TxResp{
				Status:   false,
				ErrorMsg: "check error",
			}
		}).Times(1)

		node.txPreCheck = wrongPrecheckMgr

		err = node.Prepare(tx1)
		<-txSubscribeCh
		ast.NotNil(err)
		ast.Contains(err.Error(), "check error")

		wrongPrecheckMgr.EXPECT().PostUncheckedTxEvent(gomock.Any()).Do(func(ev *common.UncheckedTxEvent) {
			txWithResp := ev.Event.(*common.TxWithResp)
			txWithResp.CheckCh <- &common.TxResp{
				Status: true,
			}
			txWithResp.PoolCh <- &common.TxResp{
				Status:   false,
				ErrorMsg: "add pool error",
			}
		}).Times(1)

		err = node.Prepare(tx1)
		<-txSubscribeCh
		ast.NotNil(err)
		ast.Contains(err.Error(), "add pool error")
	})
}

func TestStop(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	node := MockMinNode(ctrl, t)

	// test start
	err := node.Start()
	ast.Nil(err)
	ast.Nil(node.checkQuorum())

	now := time.Now()
	node.stack.ReadyC <- &adaptor.Ready{
		Height:    uint64(2),
		Timestamp: now.UnixNano(),
	}
	block := <-node.Commit()
	ast.Equal(uint64(2), block.Block.Height())
	ast.Equal(now.Unix(), block.Block.Header.Timestamp, "convert nano to second")

	// test stop
	node.Stop()
	time.Sleep(1 * time.Second)
	_, ok := <-node.txCache.CloseC
	ast.Equal(false, ok)
}

func TestReadConfig(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	logger := log.NewWithModule("consensus")
	cnf, _ := testutil.MockConsensusConfig(logger, ctrl, t)
	rbftConf, err := generateRbftConfig(cnf)
	assert.Nil(t, err)

	rbftConf.Logger.Notice()
	rbftConf.Logger.Noticef("test notice")
	ast.Equal(1000, rbftConf.SetSize)
	ast.Equal(500*time.Millisecond, rbftConf.BatchTimeout)
	ast.Equal(5*time.Minute, rbftConf.CheckPoolTimeout)
}

func TestStep(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	node := MockMinNode(ctrl, t)
	err := node.Step([]byte("test"))
	ast.NotNil(err)
	msg := &consensus.ConsensusMessage{}
	msgBytes, _ := msg.MarshalVT()
	err = node.Step(msgBytes)
	ast.Nil(err)
}

func TestReportState(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	node := MockMinNode(ctrl, t)

	block := testutil.ConstructBlock("blockHash", uint64(20))
	node.stack.StateUpdating = true
	node.stack.StateUpdateHeight = 20
	node.ReportState(uint64(10), block.Hash(), nil, nil, false)
	ast.Equal(true, node.stack.StateUpdating)

	node.ReportState(uint64(20), block.Hash(), nil, nil, false)
	ast.Equal(false, node.stack.StateUpdating)

	node.ReportState(uint64(21), block.Hash(), nil, nil, false)
	ast.Equal(false, node.stack.StateUpdating)

	t.Run("ReportStateUpdating with checkpoint", func(t *testing.T) {
		node.stack.StateUpdating = true
		node.stack.StateUpdateHeight = 30
		block30 := testutil.ConstructBlock("blockHash", uint64(30))
		testutil.SetMockBlockLedger(block30, true)
		defer testutil.ResetMockBlockLedger()

		ckp := &consensus.Checkpoint{
			ExecuteState: &consensus.Checkpoint_ExecuteState{
				Height: 30,
				Digest: block30.Hash().String(),
			},
		}
		node.ReportState(uint64(30), block.Hash(), nil, ckp, false)
		ast.Equal(false, node.stack.StateUpdating)
	})
}

func TestQuorum(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)
	node := MockMinNode(ctrl, t)
	node.stack.EpochInfo.ValidatorSet = []rbft.NodeInfo{}
	node.stack.EpochInfo.ValidatorSet = append(node.stack.EpochInfo.ValidatorSet, rbft.NodeInfo{ID: 1})
	node.stack.EpochInfo.ValidatorSet = append(node.stack.EpochInfo.ValidatorSet, rbft.NodeInfo{ID: 2})
	node.stack.EpochInfo.ValidatorSet = append(node.stack.EpochInfo.ValidatorSet, rbft.NodeInfo{ID: 3})
	node.stack.EpochInfo.ValidatorSet = append(node.stack.EpochInfo.ValidatorSet, rbft.NodeInfo{ID: 4})

	// N = 3f + 1, f=1
	quorum := node.Quorum(uint64(len(node.stack.EpochInfo.ValidatorSet)))
	ast.Equal(uint64(3), quorum)

	node.stack.EpochInfo.ValidatorSet = append(node.stack.EpochInfo.ValidatorSet, rbft.NodeInfo{ID: 5})
	// N = 3f + 2, f=1
	quorum = node.Quorum(uint64(len(node.stack.EpochInfo.ValidatorSet)))
	ast.Equal(uint64(4), quorum)

	node.stack.EpochInfo.ValidatorSet = append(node.stack.EpochInfo.ValidatorSet, rbft.NodeInfo{ID: 6})
	// N = 3f + 3, f=1
	quorum = node.Quorum(uint64(len(node.stack.EpochInfo.ValidatorSet)))
	ast.Equal(uint64(4), quorum)
}

func TestStatus2String(t *testing.T) {
	ast := assert.New(t)

	assertMapping := map[rbft.StatusType]string{
		rbft.Normal: "Normal",

		rbft.InConfChange:      "system is in conf change",
		rbft.InViewChange:      "system is in view change",
		rbft.InRecovery:        "system is in recovery",
		rbft.StateTransferring: "system is in state update",
		rbft.Pending:           "system is in pending state",
		rbft.Stopped:           "system is stopped",
		1000:                   "Unknown status: 1000",
	}

	for status, assertStatusStr := range assertMapping {
		statusStr := status2String(status)
		ast.Equal(assertStatusStr, statusStr)
	}
}
