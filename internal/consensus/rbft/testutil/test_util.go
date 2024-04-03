package testutil

import (
	"encoding/hex"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/txpool/mock_txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/network/mock_network"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	sync_comm "github.com/axiomesh/axiom-ledger/internal/sync/common"
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
		Timestamp:  time.Now().Unix(),
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

	N := 3
	f := (N - 1) / 3
	mock.EXPECT().CountConnectedValidators().Return(uint64((N + f + 2) / 2)).AnyTimes()
	mock.EXPECT().PeerID().Return(selfAddr).AnyTimes()
	return mock
}

func MockMiniBlockSync(ctrl *gomock.Controller) *mock_sync.MockSync {
	blockCacheChan = make(chan any, 1024)
	mock := mock_sync.NewMockSync(ctrl)
	mock.EXPECT().StartSync(gomock.Any(), gomock.Any()).DoAndReturn(
		func(params *sync_comm.SyncParams, syncTaskDoneCh chan error) error {
			blockCache := make([]sync_comm.CommitData, 0)
			for i := params.CurHeight; i <= params.TargetHeight; i++ {
				block, err := getRemoteMockBlockLedger(i)
				if err != nil {
					return err
				}
				data := &sync_comm.BlockData{
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
	s, err := types.GenerateSigner()
	assert.Nil(t, err)

	genesisEpochInfo := repo.GenesisEpochInfo(false)
	rep := t.TempDir()

	epochStore, err := storagemgr.Open(repo.GetStoragePath(rep, storagemgr.Epoch))
	require.Nil(t, err)
	conf := &common.Config{
		RepoRoot:             rep,
		Config:               repo.DefaultConsensusConfig(),
		Logger:               logger,
		ConsensusType:        "",
		ConsensusStorageType: repo.ConsensusStorageTypeMinifile,
		PrivKey:              s.Sk,
		GenesisEpochInfo:     genesisEpochInfo,
		Applied:              0,
		Digest:               "",
		GetEpochInfoFromEpochMgrContractFunc: func(epoch uint64) (*rbft.EpochInfo, error) {
			return genesisEpochInfo, nil
		},
		GetChainMetaFunc: GetChainMetaFunc,
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
		GetCurrentEpochInfoFromEpochMgrContractFunc: func() (*rbft.EpochInfo, error) {
			return genesisEpochInfo, nil
		},

		EpochStore: epochStore,
	}

	p2pID, err := repo.KeyToNodeID(s.Sk)
	assert.Nil(t, err)

	mockNetwork := MockMiniNetwork(ctrl, p2pID)
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
