package testutil

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/txpool/mock_txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/network/mock_network"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	synccomm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/internal/sync/common/mock_sync"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var (
	mockBlockLedger      = make(map[uint64]*types.Block)
	mockLocalBlockLedger = make(map[uint64]*types.Block)
	mockChainMeta        *types.ChainMeta
	blockCacheChan       = make(chan any, 1024)
)

func SetMockChainMeta(chainMeta *types.ChainMeta) {
	mockChainMeta = chainMeta
}

func ResetMockChainMeta() {
	block := ConstructBlock("block1", uint64(1))
	mockChainMeta = &types.ChainMeta{Height: uint64(1), BlockHash: block.Hash()}
}

func SetMockBlockLedger(block *types.Block, local bool) {
	if local {
		mockLocalBlockLedger[block.Height()] = block
	} else {
		mockBlockLedger[block.Height()] = block
	}
}

func getRemoteMockBlockLedger(height uint64) (*types.Block, error) {
	if block, ok := mockBlockLedger[height]; ok {
		return block, nil
	}
	return nil, errors.New("block not found")
}

func ResetMockBlockLedger() {
	mockBlockLedger = make(map[uint64]*types.Block)
	mockLocalBlockLedger = make(map[uint64]*types.Block)
}

func ConstructBlock(blockHashStr string, height uint64) *types.Block {
	from := make([]byte, 0)
	strLen := len(blockHashStr)
	for i := 0; i < 32; i++ {
		from = append(from, blockHashStr[i%strLen])
	}
	fromStr := hex.EncodeToString(from)
	blockHash := types.NewHashByStr(fromStr)
	header := &types.BlockHeader{
		Number:     height,
		ParentHash: blockHash,
	}
	return &types.Block{
		Header:       header,
		Transactions: []*types.Transaction{},
	}
}

func MockMiniNetwork(ctrl *gomock.Controller, selfAddr string) *mock_network.MockNetwork {
	mock := mock_network.NewMockNetwork(ctrl)
	mockPipe := mock_network.NewMockPipe(ctrl)
	mockPipe.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockPipe.EXPECT().Broadcast(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockPipe.EXPECT().Receive(gomock.Any()).Return(nil).AnyTimes()

	mock.EXPECT().CreatePipe(gomock.Any(), gomock.Any()).Return(mockPipe, nil).AnyTimes()
	mock.EXPECT().PeerID().Return(selfAddr).AnyTimes()
	return mock
}

func MockMiniBlockSync(ctrl *gomock.Controller) *mock_sync.MockSync {
	blockCacheChan = make(chan any, 1024)
	mock := mock_sync.NewMockSync(ctrl)
	mock.EXPECT().StartSync(gomock.Any(), gomock.Any()).DoAndReturn(
		func(params *synccomm.SyncParams, syncTaskDoneCh chan error) error {
			blockCache := make([]synccomm.CommitData, 0)
			for i := params.CurHeight; i <= params.TargetHeight; i++ {
				block, err := getRemoteMockBlockLedger(i)
				if err != nil {
					return err
				}
				data := &synccomm.BlockData{
					Block: block,
				}
				blockCache = append(blockCache, data)
			}
			blockCacheChan <- blockCache
			return nil
		}).AnyTimes()

	mock.EXPECT().Commit().Return(blockCacheChan).AnyTimes()
	return mock
}

func MockConsensusConfig(logger logrus.FieldLogger, ctrl *gomock.Controller, t *testing.T) (*common.Config, txpool.TxPool[types.Transaction, *types.Transaction]) {
	rep := repo.MockRepo(t)

	epochStore, err := storagemgr.Open(storagemgr.GetLedgerComponentPath(rep, storagemgr.Epoch))
	require.Nil(t, err)

	chainState := chainstate.NewMockChainState(rep.GenesisConfig, map[uint64]*types.EpochInfo{
		1: rep.GenesisConfig.EpochInfo.Clone(),
		2: rep.GenesisConfig.EpochInfo.Clone(),
	})
	chainState.ChainMeta = GetChainMetaFunc()
	chainState.IsDataSyncer = false
	conf := &common.Config{
		Repo:             repo.MockRepo(t),
		Logger:           logger,
		GenesisEpochInfo: rep.GenesisConfig.EpochInfo,
		Applied:          0,
		Digest:           "",
		ChainState:       chainState,
		GetBlockHeaderFunc: func(height uint64) (*types.BlockHeader, error) {
			if block, ok := mockLocalBlockLedger[height]; ok {
				return block.Header, nil
			} else {
				return nil, errors.New("block not found")
			}
		},
		GetAccountNonce: func(address *types.Address) uint64 {
			return 0
		},
		EpochStore: epochStore,
	}

	mockNetwork := MockMiniNetwork(ctrl, rep.P2PKeystore.P2PID())
	conf.Network = mockNetwork

	mockBlockSync := MockMiniBlockSync(ctrl)
	conf.BlockSync = mockBlockSync

	mockTxpool := mock_txpool.NewMockMinimalTxPool[types.Transaction, *types.Transaction](500, ctrl)
	conf.TxPool = mockTxpool

	return conf, mockTxpool
}

func GetChainMetaFunc() *types.ChainMeta {
	if mockChainMeta == nil {
		ResetMockChainMeta()
	}
	return mockChainMeta
}

func ConstructTxs(s *types.Signer, count int) []*types.Transaction {
	txs := make([]*types.Transaction, count)
	for i := 0; i < count; i++ {
		tx, err := types.GenerateTransactionWithSigner(uint64(i), types.NewAddressByStr("0xdAC17F958D2ee523a2206206994597C13D831ec7"), big.NewInt(0), nil, s)
		if err != nil {
			panic(err)
		}
		txs[i] = tx
	}
	return txs
}
