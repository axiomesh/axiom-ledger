package executor

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system"
	sys_common "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/finance"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/events"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	blockChanNumber = 1024
)

var _ Executor = (*BlockExecutor)(nil)

// BlockExecutor executes block from consensus
type BlockExecutor struct {
	ledger             *ledger.Ledger
	logger             logrus.FieldLogger
	blockC             chan *common.CommitEvent
	gas                *finance.Gas
	incentive          *finance.Incentive
	cumulativeGasUsed  uint64
	currentHeight      uint64
	currentBlockHash   *types.Hash
	blockFeed          event.Feed
	blockFeedForRemote event.Feed
	logsFeed           event.Feed
	ctx                context.Context
	cancel             context.CancelFunc

	evm         *vm.EVM
	evmChainCfg *params.ChainConfig
	gasLimit    uint64
	rep         *repo.Repo
	lock        *sync.Mutex

	nvm sys_common.VirtualMachine

	epochExchange   bool
	afterBlockHooks []func(block *types.Block)
}

// New creates executor instance
func New(rep *repo.Repo, ledger *ledger.Ledger) (*BlockExecutor, error) {
	ctx, cancel := context.WithCancel(context.Background())

	blockExecutor := &BlockExecutor{
		ledger:            ledger,
		logger:            loggers.Logger(loggers.Executor),
		ctx:               ctx,
		cancel:            cancel,
		blockC:            make(chan *common.CommitEvent, blockChanNumber),
		gas:               finance.NewGas(rep),
		cumulativeGasUsed: 0,
		currentHeight:     ledger.ChainLedger.GetChainMeta().Height,
		currentBlockHash:  ledger.ChainLedger.GetChainMeta().BlockHash,
		evmChainCfg:       newEVMChainCfg(rep.GenesisConfig),
		rep:               rep,
		gasLimit:          rep.GenesisConfig.EpochInfo.FinanceParams.GasLimit,
		lock:              &sync.Mutex{},
	}

	blockExecutor.evm = newEvm(1, uint64(0), blockExecutor.evmChainCfg, blockExecutor.ledger.StateLedger, blockExecutor.ledger.ChainLedger, "")

	// initialize native vm
	blockExecutor.nvm = system.New()
	var err error
	blockExecutor.incentive, err = finance.NewIncentive(rep.GenesisConfig, blockExecutor.nvm)

	blockExecutor.afterBlockHooks = []func(block *types.Block){
		blockExecutor.updateEpochInfo,
		//blockExecutor.updateMiningInfo,
	}

	return blockExecutor, err
}

// Start starts executor
func (exec *BlockExecutor) Start() error {
	go exec.listenExecuteEvent()

	exec.logger.WithFields(logrus.Fields{
		"height": exec.currentHeight,
		"hash":   exec.currentBlockHash.String(),
	}).Infof("BlockExecutor started")

	return nil
}

// Stop stops executor
func (exec *BlockExecutor) Stop() error {
	exec.cancel()

	exec.logger.Info("BlockExecutor stopped")

	return nil
}

// ExecuteBlock executes block from consensus
func (exec *BlockExecutor) ExecuteBlock(block *common.CommitEvent) {
	exec.processExecuteEvent(block)
}

func (exec *BlockExecutor) AsyncExecuteBlock(block *common.CommitEvent) {
	exec.blockC <- block
}

// SubscribeBlockEvent registers a subscription of ExecutedEvent.
func (exec *BlockExecutor) SubscribeBlockEvent(ch chan<- events.ExecutedEvent) event.Subscription {
	return exec.blockFeed.Subscribe(ch)
}

// SubscribeBlockEventForRemote registers a subscription of ExecutedEvent.
func (exec *BlockExecutor) SubscribeBlockEventForRemote(ch chan<- events.ExecutedEvent) event.Subscription {
	return exec.blockFeedForRemote.Subscribe(ch)
}

func (exec *BlockExecutor) SubscribeLogsEvent(ch chan<- []*types.EvmLog) event.Subscription {
	return exec.logsFeed.Subscribe(ch)
}

func (exec *BlockExecutor) listenExecuteEvent() {
	for {
		select {
		case <-exec.ctx.Done():
			close(exec.blockC)
			return
		case commitEvent := <-exec.blockC:
			exec.processExecuteEvent(commitEvent)
		}
	}
}

func newEVMChainCfg(genesisConfig *repo.GenesisConfig) *params.ChainConfig {
	shanghaiTime := uint64(0)
	CancunTime := uint64(0)
	PragueTime := uint64(0)

	return &params.ChainConfig{
		ChainID:                 big.NewInt(int64(genesisConfig.ChainID)),
		HomesteadBlock:          big.NewInt(0),
		EIP150Block:             big.NewInt(0),
		EIP155Block:             big.NewInt(0),
		EIP158Block:             big.NewInt(0),
		ByzantiumBlock:          big.NewInt(0),
		ConstantinopleBlock:     big.NewInt(0),
		PetersburgBlock:         big.NewInt(0),
		IstanbulBlock:           big.NewInt(0),
		MuirGlacierBlock:        big.NewInt(0),
		BerlinBlock:             big.NewInt(0),
		LondonBlock:             big.NewInt(0),
		ArrowGlacierBlock:       big.NewInt(0),
		MergeNetsplitBlock:      big.NewInt(0),
		TerminalTotalDifficulty: big.NewInt(0),
		ShanghaiTime:            &shanghaiTime,
		CancunTime:              &CancunTime,
		PragueTime:              &PragueTime,
	}
}
