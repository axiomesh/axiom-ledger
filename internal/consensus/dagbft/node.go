package dagbft

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/dagbft/adaptor"
	"github.com/axiomesh/axiom-ledger/internal/consensus/precheck"
	"github.com/axiomesh/axiom-ledger/pkg/events"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	dagbft "github.com/bcds/go-hpc-dagbft"
	dagtypes "github.com/bcds/go-hpc-dagbft/common/types"
	dagevents "github.com/bcds/go-hpc-dagbft/common/types/events"
	"github.com/bcds/go-hpc-dagbft/common/types/protos"
	"github.com/bcds/go-hpc-dagbft/common/utils/channel"
	"github.com/bcds/go-hpc-dagbft/common/utils/containers"
	"github.com/bcds/go-hpc-dagbft/test/mock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var _ consensus.Consensus = (*Node)(nil)

func init() {
	repo.Register(repo.ConsensusTypeDagBft, true)
}

type Node struct {
	nodeConfig *Config
	stack      *adaptor.DagBFTAdaptor
	txpool     txpool.TxPool[types.Transaction, *types.Transaction]
	engine     dagbft.DagBFT
	txPreCheck precheck.PreCheck

	primary dagtypes.Host

	workers           map[dagtypes.Host]bool
	readyC            chan *adaptor.Ready
	blockC            chan *common.CommitEvent
	executedMap       map[uint64]containers.Tuple[*dagtypes.CommitState, chan<- *dagevents.ExecutedEvent, chan<- struct{}]
	updatedResult     containers.Tuple[dagtypes.Height, *dagtypes.QuorumCheckpoint, chan<- *dagevents.StateUpdatedEvent]
	recvStateUpdateCh chan containers.Tuple[dagtypes.Height, *dagtypes.QuorumCheckpoint, chan<- *dagevents.StateUpdatedEvent]
	handleBatchCh     chan batchType

	wg      *sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	closeCh chan bool
	crashCh chan bool

	logger        logrus.FieldLogger
	txFeed        event.Feed
	mockBlockFeed event.Feed
}

func NewNode(config *common.Config) (*Node, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	crashCh := make(chan bool)
	closeCh := make(chan bool)
	dagConfig, err := GenerateDagBftConfig(config)

	networkFactory := adaptor.NewNetworkFactory(config, ctx)
	readyC := make(chan *adaptor.Ready)
	dagAdaptor, err := adaptor.NewAdaptor(config, dagConfig.NetworkConfig, readyC, dagConfig.DAGConfigs, ctx, closeCh)
	if err != nil {
		return nil, err
	}

	engine := dagbft.New(networkFactory, dagAdaptor.GetLedger(), dagConfig.Logger, &mock.Provider{}, crashCh)

	if err != nil {
		cancel()
		return nil, err
	}

	return &Node{
		txpool:  config.TxPool,
		engine:  engine,
		readyC:  readyC,
		logger:  config.Logger,
		stack:   dagAdaptor,
		ctx:     ctx,
		cancel:  cancel,
		closeCh: closeCh,
		crashCh: crashCh,

		executedMap: make(map[uint64]containers.Tuple[*dagtypes.CommitState, chan<- *dagevents.ExecutedEvent, chan<- struct{}]),
	}, nil
}

func (n *Node) Start() error {
	n.txpool.Init(txpool.ConsensusConfig{
		SelfID: n.nodeConfig.ChainState.SelfNodeInfo.ID,
		NotifyGenerateBatchFn: func(typ int) {
			channel.SafeSend(n.handleBatchCh, batchType(typ), n.closeCh)
		},
		NotifyFindNextBatchFn: func(completionMissingBatchHashes ...string) {
			n.logger.Errorf("cannot reach notifyFindNextBatchFn in dagbft consensus")
			return
		},
	})
	go func() {
		select {
		case <-n.closeCh:
			return

		case crash := <-n.crashCh:
			if crash {
				n.logger.Errorf("consensus crashed,, see details in node log")
				n.Stop()
			}

		case typ := <-n.handleBatchCh:
			batch, err := n.txpool.GenerateRequestBatch(int(typ))
		}
	}()

	n.stack.Start()
	err := n.engine.Start()
	if err != nil {
		return err
	}

	return nil
}

func (n *Node) Stop() {
	n.engine.Stop()
	close(n.closeCh)
	n.cancel()
	n.logger.Info("dagbft consensus stopped")
}

func (n *Node) Prepare(tx *types.Transaction) error {
	defer n.txFeed.Send([]*types.Transaction{tx})

	txWithResp := &common.TxWithResp{
		Tx:      tx,
		CheckCh: make(chan *common.TxResp, 1),
		PoolCh:  make(chan *common.TxResp, 1),
	}
	ev := &common.UncheckedTxEvent{
		EventType: common.LocalTxEvent,
		Event:     txWithResp,
	}
	n.txPreCheck.PostUncheckedTxEvent(ev)
	precheckResp := <-txWithResp.CheckCh
	if !precheckResp.Status {
		return errors.Wrap(common.ErrorPreCheck, precheckResp.ErrorMsg)
	}

	resp := <-txWithResp.PoolCh
	if !resp.Status {
		return errors.Wrap(common.ErrorAddTxPool, resp.ErrorMsg)
	}

	return nil
}

func (n *Node) Commit() chan *common.CommitEvent {
	return n.blockC
}

func (n *Node) Step(_ []byte) error {
	return nil
}

func (n *Node) Ready() error {
	status := n.engine.ReadStatus()
	if status.Primary.Status.String() == "Normal" {
		return nil
	}
	return fmt.Errorf("%s", status.Primary.Status.String())
}

func (n *Node) ReportState(height uint64, blockHash *types.Hash, txHashList []*events.TxPointer, stateUpdatedCheckpoint *common.Checkpoint, needRemoveTxs bool) {
	reconfigured := common.NeedChangeEpoch(height, n.nodeConfig.ChainState.GetCurrentEpochInfo())
	if executed, ok := n.executedMap[height]; ok {
		commitState, respCh, waitCh := executed.Unpack()
		respCh <- &dagevents.ExecutedEvent{
			CommitState: commitState,
			ExecutedState: &dagtypes.ExecuteState{
				ExecuteState: protos.ExecuteState{
					Height:    height,
					StateRoot: blockHash.String(),
				},
				Reconfigured: reconfigured,
			},
		}
		// if reconfigured, notify epoch changed to consensus
		if reconfigured {
			waitCh <- struct{}{}
		}
	}

	// if state updated, notify state updated to consensus
	if stateUpdatedHeight, quorumCkpt, respCh := n.updatedResult.Unpack(); stateUpdatedHeight == height {
		channel.SafeSend(respCh, &dagevents.StateUpdatedEvent{Checkpoint: quorumCkpt, Updated: true}, n.closeCh)
	}
}

func (n *Node) Quorum(N uint64) uint64 {
	return common.CalQuorum(N)
}

func (n *Node) GetLowWatermark() uint64 {
	checkpoint := n.nodeConfig.ChainState.GetCurrentCheckpointState()
	return checkpoint.GetHeight()
}

func (n *Node) SubscribeTxEvent(events chan<- []*types.Transaction) event.Subscription {
	return n.txFeed.Subscribe(events)
}

func (n *Node) SubscribeMockBlockEvent(ch chan<- events.ExecutedEvent) event.Subscription {
	return n.mockBlockFeed.Subscribe(ch)
}

func (n *Node) IsPrimary() bool {
	return len(n.primary) > 0
}

func (n *Node) IsWorker() bool {
	return len(n.workers) > 0
}

func (n *Node) GetAPI() API {
	return API{n}
}

type API struct {
	*Node
}

type PrimaryAPI struct {
	*adaptor.DagBFTAdaptor
	API
}

func (n *Node) listenConsensusEngineEvent() {
	for {
		select {
		case <-n.closeCh:
			return
		case res := <-n.recvStateUpdateCh:
			n.updatedResult = res
		case r := <-n.readyC:
			block := &types.Block{
				Header: &types.BlockHeader{
					Number:         r.Height,
					Timestamp:      r.Timestamp / int64(time.Second),
					ProposerNodeID: r.ProposerNodeID,
				},
				Transactions: r.Txs,
			}
			commitEvent := &common.CommitEvent{
				Block: block,
			}
			channel.SafeSend(n.blockC, commitEvent, n.closeCh)
			n.executedMap[r.Height] = containers.Pack3(r.CommitState, r.ExecutedCh, r.EpochChangedCh)
		}
	}
}
