package executor

import (
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/pkg/events"
)

type Executor interface {
	Start() error

	Stop() error

	AsyncExecuteBlock(commitEvent *common.CommitEvent)

	ExecuteBlock(commitEvent *common.CommitEvent)

	CurrentHeader() *types.BlockHeader

	SubscribeBlockEvent(chan<- events.ExecutedEvent) event.Subscription

	SubscribeBlockEventForRemote(chan<- events.ExecutedEvent) event.Subscription

	SubscribeLogsEvent(chan<- []*types.EvmLog) event.Subscription

	NewEvmWithViewLedger(txCtx vm.TxContext, vmConfig vm.Config) (*vm.EVM, error)

	GetChainConfig() *params.ChainConfig
}
