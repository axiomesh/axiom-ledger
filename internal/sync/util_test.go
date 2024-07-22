package sync

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/axiomesh/axiom-ledger/internal/components"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/testutil"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/ledger/mock_ledger"
	"github.com/axiomesh/axiom-ledger/internal/network/mock_network"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	network "github.com/axiomesh/axiom-p2p"
)

const (
	wrongTypeSendSyncState = iota
	wrongTypeSendSyncBlockRequest
	wrongTypeSendSyncBlockResponse
	wrongTypeSendStream
	latencyTypeSendState
)

const waitCaseTimeout = 5 * time.Second

const (
	epochPrefix = "epoch." + rbft.EpochStatePrefix
	epochPeriod = 100
)

type mockLedger struct {
	*mock_ledger.MockChainLedger
	blockDb      map[uint64]*types.Block
	chainMeta    *types.ChainMeta
	receiptsDb   map[uint64][]*types.Receipt
	epochStateDb map[string]*pb.EpochChange

	stateResponse      chan *pb.Message
	epochStateResponse chan *pb.Message
}

func genReceipts(block *types.Block) []*types.Receipt {
	receipts := make([]*types.Receipt, 0)
	for _, tx := range block.Transactions {
		receipts = append(receipts, &types.Receipt{
			Status:            types.ReceiptSUCCESS,
			TxHash:            tx.GetHash(),
			EffectiveGasPrice: &big.Int{},
		})
	}
	return receipts
}

func newMockMinLedger(t *testing.T, genesisBlock *types.Block) *mockLedger {
	genesis := genesisBlock.Clone()
	mockLg := &mockLedger{
		blockDb: map[uint64]*types.Block{
			genesis.Height(): genesis,
		},
		receiptsDb:         make(map[uint64][]*types.Receipt),
		epochStateDb:       make(map[string]*pb.EpochChange),
		stateResponse:      make(chan *pb.Message, 1),
		epochStateResponse: make(chan *pb.Message, 1),
		chainMeta: &types.ChainMeta{
			Height:    genesis.Height(),
			BlockHash: genesis.Hash(),
		},
	}
	ctrl := gomock.NewController(t)
	mockLg.MockChainLedger = mock_ledger.NewMockChainLedger(ctrl)

	mockLg.EXPECT().GetBlock(gomock.Any()).DoAndReturn(func(height uint64) (*types.Block, error) {
		if mockLg.blockDb[height] == nil {
			return nil, errors.New("block not found")
		}
		return mockLg.blockDb[height], nil
	}).AnyTimes()
	mockLg.EXPECT().GetBlockHeader(gomock.Any()).DoAndReturn(func(height uint64) (*types.BlockHeader, error) {
		if mockLg.blockDb[height] == nil {
			return nil, errors.New("block not found")
		}
		return mockLg.blockDb[height].Header, nil
	}).AnyTimes()

	mockLg.EXPECT().GetChainMeta().DoAndReturn(func() *types.ChainMeta {
		return mockLg.chainMeta
	}).AnyTimes()

	mockLg.EXPECT().GetBlockReceipts(gomock.Any()).DoAndReturn(func(height uint64) ([]*types.Receipt, error) {
		if mockLg.receiptsDb[height] == nil {
			return nil, fmt.Errorf("receipts not found:[height:%d]", height)
		}
		return mockLg.receiptsDb[height], nil
	}).AnyTimes()

	mockLg.EXPECT().PersistExecutionResult(gomock.Any(), gomock.Any()).DoAndReturn(func(block *types.Block, receipts []*types.Receipt) error {
		h := block.Height()
		mockLg.blockDb[h] = block
		if mockLg.chainMeta.Height <= h {
			mockLg.chainMeta.Height = h
			mockLg.chainMeta.BlockHash = block.Hash()
		}
		mockLg.receiptsDb[h] = receipts

		if h%epochPeriod == 0 {
			epoch := h / epochPeriod
			key := fmt.Sprintf(epochPrefix+"%d", epoch)
			mockLg.epochStateDb[key] = &pb.EpochChange{
				QuorumCheckpoint: &pb.QuorumCheckpoint{
					Epoch: h / epochPeriod,
					State: &pb.ExecuteState{
						Height: h,
						Digest: block.Hash().String(),
					},
				},
			}
		}
		return nil
	}).AnyTimes()
	return mockLg
}

func generateHash(blockHashStr string) *types.Hash {
	from := make([]byte, 0)
	strLen := len(blockHashStr)
	for i := 0; i < 32; i++ {
		from = append(from, blockHashStr[i%strLen])
	}
	fromStr := hex.EncodeToString(from)
	return types.NewHashByStr(fromStr)
}

func ConstructBlock(height uint64, parentHash *types.Hash, txs ...*types.Transaction) *types.Block {
	blockHashStr := "block" + strconv.FormatUint(height, 10)
	from := make([]byte, 0)
	strLen := len(blockHashStr)
	for i := 0; i < 32; i++ {
		from = append(from, blockHashStr[i%strLen])
	}
	header := &types.BlockHeader{
		Number:     height,
		ParentHash: parentHash,
		Timestamp:  time.Now().Unix(),
		Epoch:      ((height - 1) / epochPeriod) + 1,
	}
	if len(txs) == 0 {
		txs = make([]*types.Transaction, 0)
		header.TxRoot = &types.Hash{}
	} else {
		root, err := components.CalcTxsMerkleRoot(txs)
		if err != nil {
			panic(err)
		}
		header.TxRoot = root
	}
	return &types.Block{
		Header:       header,
		Transactions: txs,
		Extra:        &types.BlockExtra{}}
}

func ConstructBlocks(count int, parentHash *types.Hash) []*types.Block {
	blocks := make([]*types.Block, 0)
	parent := parentHash
	for i := 2; i <= count; i++ {
		blocks = append(blocks, ConstructBlock(uint64(i), parent))
		parent = blocks[len(blocks)-1].Hash()
	}
	return blocks
}

func (net *mockMiniNetwork) newMockBlockRequestPipe(nets map[string]*mockMiniNetwork, mode common.SyncMode, ctrl *gomock.Controller, localId string, wrongPipeId ...int) network.Pipe {
	var wrongRemoteId string
	if len(wrongPipeId) > 0 {
		if len(wrongPipeId) != 2 {
			panic("wrong pipe id must be 2, local id + remote id")
		}
		if strconv.Itoa(wrongPipeId[0]) == localId {
			wrongRemoteId = strconv.Itoa(wrongPipeId[1])
		}
	}
	mockPipe := mock_network.NewMockPipe(ctrl)
	logger := logrus.New()
	fp := fmt.Sprintf("recv_block_req-node%s.log", localId)

	// show package path
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	path := filepath.Join(filepath.Dir(pwd), fp)
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	logger.SetOutput(file)
	mockPipe.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, to string, data []byte) error {
			switch mode {
			case common.SyncModeFull:
				msg := &pb.SyncBlockRequest{}
				if err := msg.UnmarshalVT(data); err != nil {
					return fmt.Errorf("unmarshal message failed: %w", err)
				}

				if wrongRemoteId != "" && wrongRemoteId == to {
					return fmt.Errorf("send remote peer err: %s", to)
				}

				toNet := nets[to]
				logger.WithFields(logrus.Fields{
					"to":        to,
					"toNetNode": toNet.Node,
					"height":    msg.Height,
				}).Info("recv block req")
				toNet.blockReqPipeDb <- &network.PipeMsg{
					From: localId,
					Data: data,
				}
			case common.SyncModeSnapshot:
				msg := &pb.SyncChainDataRequest{}
				if err := msg.UnmarshalVT(data); err != nil {
					return fmt.Errorf("unmarshal message failed: %w", err)
				}

				if wrongRemoteId != "" && wrongRemoteId == to {
					return fmt.Errorf("send remote peer err: %s", to)
				}

				nets[to].chainDataReqPipeDb <- &network.PipeMsg{
					From: localId,
					Data: data,
				}
			}
			return nil
		}).AnyTimes()

	var ch chan *network.PipeMsg
	switch mode {
	case common.SyncModeFull:
		ch = net.blockReqPipeDb
	case common.SyncModeSnapshot:
		ch = net.chainDataReqPipeDb
	}
	mockPipe.EXPECT().Receive(gomock.Any()).DoAndReturn(
		func(ctx context.Context) *network.PipeMsg {
			for {
				select {
				case <-ctx.Done():
					return nil
				case msg := <-ch:
					return msg
				}
			}
		}).AnyTimes()

	return mockPipe
}

func (net *mockMiniNetwork) newMockBlockResponsePipe(nets map[string]*mockMiniNetwork, mode common.SyncMode, ctrl *gomock.Controller, localId string, wrongPid ...int) network.Pipe {
	var wrongSendBlockResponse bool
	if len(wrongPid) > 0 {
		if len(wrongPid) != 2 {
			panic("wrong pipe id must be 2, local id + remote id")
		}
		if strconv.Itoa(wrongPid[1]) == localId {
			wrongSendBlockResponse = true
		}
	}
	mockPipe := mock_network.NewMockPipe(ctrl)
	if wrongSendBlockResponse {
		mockPipe.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("send commitData response error")).AnyTimes()
	} else {
		mockPipe.EXPECT().Send(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, to string, data []byte) error {
				msg := &pb.Message{}
				if err := msg.UnmarshalVT(data); err != nil {
					return fmt.Errorf("unmarshal message failed: %w", err)
				}
				resp := &network.PipeMsg{
					From: localId,
					Data: data,
				}
				switch msg.Type {
				case pb.Message_SYNC_BLOCK_RESPONSE:
					nets[to].blockRespPipeDb <- resp
				case pb.Message_SYNC_CHAIN_DATA_RESPONSE:
					nets[to].chainDataRespPipeDb <- resp
				default:
					return fmt.Errorf("invalid message type: %v", msg.Type)
				}
				return nil
			}).AnyTimes()
	}

	var ch chan *network.PipeMsg
	switch mode {
	case common.SyncModeFull:
		ch = net.blockRespPipeDb
	case common.SyncModeSnapshot:
		ch = net.chainDataRespPipeDb
	}
	mockPipe.EXPECT().Receive(gomock.Any()).DoAndReturn(
		func(ctx context.Context) *network.PipeMsg {
			for {
				select {
				case <-ctx.Done():
					return nil
				case msg := <-ch:
					return msg
				}
			}
		}).AnyTimes()

	return mockPipe
}

type pipeMsgIndexM struct {
	lock           sync.RWMutex
	blockReqCache  map[string]chan *network.PipeMsg
	blockRespCache map[string]chan *network.PipeMsg

	chainDataReqCache  map[string]chan *network.PipeMsg
	chainDataRespCache map[string]chan *network.PipeMsg
}

type mockMiniNetwork struct {
	*mock_network.MockNetwork
	Node                string
	Ctl                 *gomock.Controller
	blockReqPipeDb      chan *network.PipeMsg
	blockRespPipeDb     chan *network.PipeMsg
	chainDataReqPipeDb  chan *network.PipeMsg
	chainDataRespPipeDb chan *network.PipeMsg
}

func newMockMiniNetworks(n int) map[string]*mockMiniNetwork {
	nets := make(map[string]*mockMiniNetwork)
	for i := 0; i < n; i++ {
		mock := &mockMiniNetwork{
			blockReqPipeDb:      make(chan *network.PipeMsg, 1000),
			blockRespPipeDb:     make(chan *network.PipeMsg, 1000),
			chainDataReqPipeDb:  make(chan *network.PipeMsg, 1000),
			chainDataRespPipeDb: make(chan *network.PipeMsg, 1000),
		}
		nets[strconv.Itoa(i)] = mock
	}
	return nets
}

func initMockMiniNetwork(t *testing.T, ledgers map[string]*mockLedger, nets map[string]*mockMiniNetwork, ctrl *gomock.Controller, localId string, wrong ...int) {
	mock := nets[localId]
	mock.MockNetwork = mock_network.NewMockNetwork(ctrl)
	mock.Ctl = ctrl
	mock.Node = localId
	var (
		wrongSendStream        bool
		latencySendState       bool
		latencyRemoteId        int
		wrongSendStateRequest  bool
		wrongSendBlockRequest  bool
		wrongSendBlockResponse bool
	)

	if len(wrong) > 0 {
		if len(wrong) != 3 {
			panic("wrong pipe id must be 3, wrong type + local id + remote id")
		}
		switch wrong[0] {
		case wrongTypeSendSyncState:
			if strconv.Itoa(wrong[1]) == localId {
				wrongSendStateRequest = true
			}
		case wrongTypeSendStream:
			if strconv.Itoa(wrong[2]) == localId {
				wrongSendStream = true
			}
		case latencyTypeSendState:
			if strconv.Itoa(wrong[1]) == localId {
				latencySendState = true
				latencyRemoteId = wrong[2]
			}
		case wrongTypeSendSyncBlockRequest:
			wrong = wrong[1:]
			wrongSendBlockRequest = true
		case wrongTypeSendSyncBlockResponse:
			wrong = wrong[1:]
			wrongSendBlockResponse = true
		}
	}

	mock.EXPECT().RegisterMsgHandler(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mock.EXPECT().CreatePipe(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, pipeID string) (network.Pipe, error) {
			switch pipeID {
			case common.SyncBlockRequestPipe, common.SyncChainDataRequestPipe:
				mode := common.SyncModeFull
				if pipeID == common.SyncChainDataRequestPipe {
					mode = common.SyncModeSnapshot
				}
				if !wrongSendBlockRequest {
					return mock.newMockBlockRequestPipe(nets, mode, ctrl, localId), nil
				}
				return mock.newMockBlockRequestPipe(nets, mode, ctrl, localId, wrong...), nil
			case common.SyncBlockResponsePipe, common.SyncChainDataResponsePipe:
				mode := common.SyncModeFull
				if pipeID == common.SyncChainDataResponsePipe {
					mode = common.SyncModeSnapshot
				}
				if !wrongSendBlockResponse {
					return mock.newMockBlockResponsePipe(nets, mode, ctrl, localId), nil
				}
				return mock.newMockBlockResponsePipe(nets, mode, ctrl, localId, wrong...), nil
			default:
				return nil, fmt.Errorf("invalid pipe id: %s", pipeID)
			}
		}).AnyTimes()

	mock.EXPECT().PeerID().Return(localId).AnyTimes()

	if wrongSendStateRequest {
		mock.EXPECT().Send(gomock.Any(), gomock.Any()).Return(nil, errors.New("send error")).AnyTimes()
	} else {
		mock.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(
			func(to string, msg *pb.Message) (*pb.Message, error) {
				if msg.Type == pb.Message_FETCH_EPOCH_STATE_REQUEST {
					req := &pb.FetchEpochStateRequest{}
					if err := req.UnmarshalVT(msg.Data); err != nil {
						return nil, fmt.Errorf("unmarshal fetch epoch state request failed: %w", err)
					}

					key := fmt.Sprintf(epochPrefix+"%d", req.Epoch)
					eps, ok := ledgers[to].epochStateDb[key]
					if !ok {
						return nil, fmt.Errorf("epoch %d not found", req.Epoch)
					}

					v, err := eps.MarshalVT()
					require.Nil(t, err)
					epsResp := &pb.FetchEpochStateResponse{
						Data: v,
					}
					data, err := epsResp.MarshalVT()
					require.Nil(t, err)
					resp := &pb.Message{From: to, Type: pb.Message_FETCH_EPOCH_STATE_RESPONSE, Data: data}
					return resp, nil
				}

				req := &pb.SyncStateRequest{}
				if err := req.UnmarshalVT(msg.Data); err != nil {
					return nil, fmt.Errorf("unmarshal sync state request failed: %w", err)
				}

				block, err := ledgers[to].GetBlock(req.Height)
				if err != nil {
					return nil, fmt.Errorf("get block with height %d failed: %w", req.Height, err)
				}

				stateResp := &pb.SyncStateResponse{
					Status: pb.Status_SUCCESS,
					CheckpointState: &pb.CheckpointState{
						Height:       block.Height(),
						Digest:       block.Hash().String(),
						LatestHeight: ledgers[to].GetChainMeta().Height,
					},
				}

				data, err := stateResp.MarshalVT()
				if err != nil {
					return nil, fmt.Errorf("marshal sync state response failed: %w", err)
				}
				resp := &pb.Message{From: to, Type: pb.Message_SYNC_STATE_RESPONSE, Data: data}

				if latencySendState && to == strconv.Itoa(latencyRemoteId) {
					time.Sleep(200 * time.Millisecond)
				}
				return resp, nil
			}).AnyTimes()
	}

	mock.EXPECT().SendWithStream(gomock.Any(), gomock.Any()).DoAndReturn(func(stream network.Stream, msg *pb.Message) error {
		if wrongSendStream {
			return errors.New("send stream error")
		}
		if msg.Type == pb.Message_FETCH_EPOCH_STATE_RESPONSE {
			resp := &pb.FetchEpochStateResponse{}
			if err := resp.UnmarshalVT(msg.Data); err != nil {
				return fmt.Errorf("unmarshal fetch epoch state response failed: %w", err)
			}
			ledgers[msg.From].epochStateResponse <- msg
			return nil
		}

		resp := &pb.SyncStateResponse{}
		if err := resp.UnmarshalVT(msg.Data); err != nil {
			return fmt.Errorf("unmarshal sync state response failed: %w", err)
		}
		ledgers[msg.From].stateResponse <- msg
		return nil
	}).AnyTimes()

	mock.EXPECT().GetConnectedPeers(gomock.Any()).DoAndReturn(func(peers []string) []string {
		return peers
	}).AnyTimes()
}

func genGenesisBlock() *types.Block {
	header := &types.BlockHeader{
		Number:      1,
		ReceiptRoot: &types.Hash{},
		StateRoot:   &types.Hash{},
		TxRoot:      &types.Hash{},
		Bloom:       new(types.Bloom),
		ParentHash:  types.NewHashByStr("0x00"),
		Timestamp:   time.Now().Unix(),
	}

	genesisBlock := &types.Block{
		Header:       header,
		Transactions: []*types.Transaction{},
	}
	return genesisBlock
}

func newMockBlockSyncs(t *testing.T, n int, wrongPipeId ...int) ([]*SyncManager, map[string]*mockLedger) {
	ctrl := gomock.NewController(t)
	syncs := make([]*SyncManager, 0)
	genesis := genGenesisBlock()

	ledgers := make(map[string]*mockLedger)
	// 1. prepare all ledgers
	for i := 0; i < n; i++ {
		lg := newMockMinLedger(t, genesis)
		localId := strconv.Itoa(i)
		ledgers[localId] = lg
	}
	nets := newMockMiniNetworks(n)

	// 2. prepare all syncs
	for i := 0; i < n; i++ {
		localId := strconv.Itoa(i)
		logger := log.NewWithModule("sync" + strconv.Itoa(i))
		logger.Logger.SetLevel(logrus.DebugLevel)
		initMockMiniNetwork(t, ledgers, nets, ctrl, localId, wrongPipeId...)

		getBlockFn := func(height uint64) (*types.Block, error) {
			return ledgers[localId].GetBlock(height)
		}
		getBlockHeaderFn := func(height uint64) (*types.BlockHeader, error) {
			return ledgers[localId].GetBlockHeader(height)
		}
		getChainMetaFn := func() *types.ChainMeta {
			return ledgers[localId].GetChainMeta()
		}

		conf := repo.Sync{
			RequesterRetryTimeout: repo.Duration(1 * time.Second),
			TimeoutCountLimit:     5,
			ConcurrencyLimit:      10,
			WaitStatesTimeout:     repo.Duration(100 * time.Second),
			MaxChunkSize:          100,
		}

		getReceiptsFn := func(height uint64) ([]*types.Receipt, error) {
			return ledgers[localId].GetBlockReceipts(height)
		}

		getEpochStateFn := func(key []byte) []byte {
			epochState := ledgers[localId].epochStateDb[string(key)]
			val, err := epochState.MarshalVT()
			require.Nil(t, err)
			return val
		}

		blockSync, err := NewSyncManager(logger, getChainMetaFn, getBlockFn, getBlockHeaderFn, getReceiptsFn, getEpochStateFn, nets[strconv.Itoa(i)], conf)
		require.Nil(t, err)
		syncs = append(syncs, blockSync)
	}

	return syncs, ledgers
}

func stopSyncs(syncs []*SyncManager) {
	for _, s := range syncs {
		s.Stop()
	}
}

func prepareBlockSyncs(t *testing.T, epochInterval int, local, count, txCount int, begin, end uint64) ([]*SyncManager, []*pb.EpochChange, []*common.Node) {
	syncs := make([]*SyncManager, count)
	var (
		epochChanges       []*pb.EpochChange
		needGenEpochChange bool
		ledgers            = make(map[string]*mockLedger)
	)

	remainder := (int(begin) + epochInterval) % epochInterval
	// prepare epoch change
	if begin+uint64(epochInterval)-uint64(remainder) < end {
		needGenEpochChange = true
	}

	genesis := genGenesisBlock()

	beforeBeginBlocks := make([]*types.Block, 0)
	parent := genesis.Hash()
	s, err := types.GenerateSigner()
	require.Nil(t, err)
	txs := testutil.ConstructTxs(s, int(txCount))
	if begin > 1 {
		for j := 2; j < int(begin); j++ {
			b := ConstructBlock(uint64(j), parent, txs...)
			beforeBeginBlocks = append(beforeBeginBlocks, b)
			parent = b.Hash()
		}
	}

	blockCache := make([]*types.Block, 0)
	parentHash := genesis.Hash()
	if len(beforeBeginBlocks) > 0 {
		parentHash = beforeBeginBlocks[len(beforeBeginBlocks)-1].Hash()
	}
	for j := begin; j <= end; j++ {
		block := ConstructBlock(j, parentHash, txs...)
		blockCache = append(blockCache, block)
		parentHash = block.Hash()
	}

	for i := 0; i < count; i++ {
		lg := newMockMinLedger(t, genesis)
		localId := strconv.Itoa(i)
		lo.ForEach(beforeBeginBlocks, func(block *types.Block, _ int) {
			err := lg.PersistExecutionResult(block, genReceipts(block))
			require.Nil(t, err)
		})

		if i != local {
			lo.ForEach(blockCache, func(block *types.Block, _ int) {
				err := lg.PersistExecutionResult(block, genReceipts(block))
				require.Nil(t, err)
			})

			if needGenEpochChange && len(epochChanges) == 0 {
				nextEpochHeight := begin + uint64(epochInterval) - uint64(remainder)
				nextEpochHash := lg.blockDb[nextEpochHeight].Hash().String()
				for nextEpochHeight <= end {
					nextEpochHash = lg.blockDb[nextEpochHeight].Hash().String()
					epochChanges = append(epochChanges, prepareEpochChange(nextEpochHeight, nextEpochHash))
					nextEpochHeight += uint64(epochInterval)
				}
			}
		}
		ledgers[localId] = lg
	}

	nets := make(map[string]*mock_network.MiniNetwork)
	peers := make([]*common.Node, count)
	for i := 0; i < count; i++ {
		peers[i] = &common.Node{Id: uint64(i), PeerID: strconv.Itoa(i)}
	}
	peerIds := lo.FlatMap(peers, func(peer *common.Node, _ int) []string {
		return []string{peer.PeerID}
	})

	manager := network.GenMockHostManager(peerIds)
	lo.ForEach(peerIds, func(peer string, i int) {
		net, err := mock_network.NewMiniNetwork(uint64(i), peer, manager)
		require.Nil(t, err)

		err = net.Start()
		require.Nil(t, err)

		nets[peer] = net
	})

	for localId := range ledgers {
		lg := ledgers[localId]
		fmt.Printf("%s: %T", localId, lg)
		logger := log.NewWithModule("sync" + localId)
		// dismiss log print
		discard := io.Discard
		logger.Logger.SetOutput(discard)
		getChainMetaFn := func() *types.ChainMeta {
			return lg.GetChainMeta()
		}
		getBlockFn := func(height uint64) (*types.Block, error) {
			return lg.GetBlock(height)
		}
		getBlockHeaderFn := func(height uint64) (*types.BlockHeader, error) {
			return lg.GetBlockHeader(height)
		}
		getReceiptsFn := func(height uint64) ([]*types.Receipt, error) {
			return lg.GetBlockReceipts(height)
		}

		getEpochStateFn := func(key []byte) []byte {
			epochState := lg.epochStateDb[string(key)]
			val, err := epochState.MarshalVT()
			require.Nil(t, err)
			return val
		}
		conf := repo.Sync{
			RequesterRetryTimeout: repo.Duration(5 * time.Second),
			TimeoutCountLimit:     5,
			ConcurrencyLimit:      100,
			WaitStatesTimeout:     repo.Duration(30 * time.Second),
			FullValidation:        true,
		}

		blockSync, err := NewSyncManager(logger, getChainMetaFn, getBlockFn, getBlockHeaderFn, getReceiptsFn, getEpochStateFn, nets[localId], conf)
		require.Nil(t, err)
		localIndex, err := strconv.Atoi(localId)
		require.Nil(t, err)
		syncs[localIndex] = blockSync
	}

	return syncs, epochChanges, peers
}

func prepareEpochChange(height uint64, hash string) *pb.EpochChange {
	return &pb.EpochChange{
		QuorumCheckpoint: &pb.QuorumCheckpoint{
			State: &pb.ExecuteState{
				Height: height,
				Digest: hash,
			},
		},
	}
}

func genSyncParams(peers []*common.Node, latestBlockHash string, quorum uint64, curHeight, targetHeight uint64,
	quorumCkpt *pb.QuorumCheckpoint, epochChanges ...*pb.EpochChange) *common.SyncParams {
	return &common.SyncParams{
		Peers:            peers,
		LatestBlockHash:  latestBlockHash,
		Quorum:           quorum,
		CurHeight:        curHeight,
		TargetHeight:     targetHeight,
		QuorumCheckpoint: quorumCkpt,
		EpochChanges:     epochChanges,
	}
}

func prepareLedger(t *testing.T, ledgers map[string]*mockLedger, exceptId string, endHeight int) {
	blocks := ConstructBlocks(endHeight, genGenesisBlock().Hash())
	for id, lg := range ledgers {
		if id == exceptId {
			continue
		}
		lo.ForEach(blocks, func(block *types.Block, _ int) {
			err := lg.PersistExecutionResult(block, genReceipts(block))
			require.Nil(t, err)
		})
	}
}

func waitSyncTaskDone(ch chan error) error {
	for {
		select {
		case <-time.After(waitCaseTimeout):
			return errors.New("sync task Done timeout")
		case err := <-ch:
			return err
		}
	}
}

func waitCommitData(t *testing.T, ch chan any, handler func(*testing.T, any)) {
	for {
		select {
		case <-time.After(waitCaseTimeout):
			t.Errorf("commit data timeout")
			return
		case data := <-ch:
			handler(t, data)
			return
		}
	}
}
