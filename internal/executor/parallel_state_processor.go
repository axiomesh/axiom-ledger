// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package executor

import (
	"context"
	"math/big"
	"strings"

	"github.com/axiomesh/axiom-kit/types"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/blockstm"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/sirupsen/logrus"
)

var log = loggers.Logger(loggers.Executor)

type ParallelEVMConfig struct {
	Enable               bool
	SpeculativeProcesses int
}

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type ParallelStateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	logger logrus.FieldLogger

	bc *types.Block // Canonical block chain
	//engine consensus.Engine // Consensus engine used for block rewards

	parallelSpeculativeProcesses int // Number of parallel speculative processes
	gasLimit                     uint64
}

// NewParallelStateProcessor initialises a new StateProcessor.
func NewParallelStateProcessor(config *params.ChainConfig, bc *types.Block, parallelSpeculativeProcesses int, gasLimit uint64) *ParallelStateProcessor {
	return &ParallelStateProcessor{
		config:                       config,
		bc:                           bc,
		parallelSpeculativeProcesses: parallelSpeculativeProcesses,
		gasLimit:                     gasLimit,
		logger:                       loggers.Logger(loggers.Executor),
	}
}

type ExecutionTask struct {
	logger logrus.FieldLogger

	msg    core.Message
	config *params.ChainConfig

	gasLimit                   uint64
	blockNumber                *big.Int
	blockHash                  types.Hash
	tx                         *types.Transaction
	index                      int
	statedb                    ledger.StateLedger // State database that stores the modified values after tx execution.
	cleanStateDB               ledger.StateLedger // A clean copy of the initial statedb. It should not be modified.
	finalStateDB               ledger.StateLedger // The final statedb.
	header                     *types.BlockHeader
	blockChain                 *types.Block
	evmConfig                  vm.Config
	result                     *core.ExecutionResult
	resultErr                  error
	shouldDelayFeeCal          *bool
	shouldRerunWithoutFeeDelay bool
	sender                     types.Address
	totalUsedGas               *uint64
	receipts                   *ledger.EvmReceipts
	allLogs                    *[]*types.EvmLog

	// length of dependencies          -> 2 + k (k = a whole number)
	// first 2 element in dependencies -> transaction index, and flag representing if delay is allowed or not
	//                                       (0 -> delay is not allowed, 1 -> delay is allowed)
	// next k elements in dependencies -> transaction indexes on which transaction i is dependent on
	dependencies []int
	coinbase     types.Address
	blockContext vm.BlockContext

	sp *StateDbPool
}

func (task *ExecutionTask) Execute(mvh *blockstm.MVHashMap, incarnation int) (err error) {
	task.statedb = task.cleanStateDB.Copy()

	//task.statedb = task.sp.GetStateDb(task.cleanStateDB)

	task.statedb.SetTxContext(task.tx.GetHash(), task.index)
	task.statedb.SetMVHashmap(mvh)
	task.statedb.SetIncarnation(incarnation)
	evmStateDB := &ledger.EvmStateDBAdaptor{StateLedger: task.statedb}

	evm := vm.NewEVM(task.blockContext, vm.TxContext{}, evmStateDB, task.config, task.evmConfig)

	// Create a new context to be used in the EVM environment.
	txContext := core.NewEVMTxContext(&task.msg)
	evm.Reset(txContext, evmStateDB)

	defer func() {
		if r := recover(); r != nil {
			// In some pre-matured executions, EVM will panic. Recover from panic and retry the execution.
			log.Debug("Recovered from EVM failure.", "Error:", r)

			err = blockstm.ErrExecAbortError{Dependency: task.statedb.DepTxIndex()}

			return
		}
	}()

	// Apply the transaction to the current state (included in the env).
	if *task.shouldDelayFeeCal {
		//currentFromStart := time.Now()
		task.result, task.resultErr = core.ApplyMessageNoFeeBurnOrTip(evm, task.msg, new(core.GasPool).AddGas(task.gasLimit))
		if task.result == nil || task.resultErr != nil {
			return blockstm.ErrExecAbortError{Dependency: task.statedb.DepTxIndex(), OriginError: task.resultErr}
		}
		//evmExecuteEachDuration.Observe(float64(time.Since(currentFromStart)) / float64(time.Second))

		reads := task.statedb.MVReadMap()

		if _, ok := reads[blockstm.NewSubpathKey(types.NewAddress(task.blockContext.Coinbase.Bytes()), ledger.BalancePath)]; ok {
			log.Info("Coinbase is in MVReadMap", "address", task.blockContext.Coinbase)

			task.shouldRerunWithoutFeeDelay = true
		}

	} else {
		task.result, task.resultErr = core.ApplyMessage(evm, &task.msg, new(core.GasPool).AddGas(task.gasLimit))
	}

	if task.statedb.HadInvalidRead() || task.resultErr != nil {
		err = blockstm.ErrExecAbortError{Dependency: task.statedb.DepTxIndex(), OriginError: err}
		return
	}

	task.statedb.Finalise()
	return
}

func (task *ExecutionTask) MVReadList() []blockstm.ReadDescriptor {
	return task.statedb.MVReadList()
}

func (task *ExecutionTask) MVWriteList() []blockstm.WriteDescriptor {
	return task.statedb.MVWriteList()
}

func (task *ExecutionTask) MVFullWriteList() []blockstm.WriteDescriptor {
	return task.statedb.MVFullWriteList()
}

func (task *ExecutionTask) ReleaseStateDb() {
	task.sp.PutBackToPool(task.statedb)
}
func (task *ExecutionTask) Sender() types.Address {
	return task.sender
}

func (task *ExecutionTask) Hash() types.Hash {
	return *task.tx.GetHash()
}

func (task *ExecutionTask) Dependencies() []int {
	return task.dependencies
}

func (task *ExecutionTask) Settle() {
	if task.resultErr != nil {
		task.logger.Error("Error while executing transaction", "Error:", task.resultErr)
		receipt := &types.Receipt{
			TxHash: task.tx.GetHash(),
		}
		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(task.resultErr.Error())
		*task.receipts = append(*task.receipts, receipt)
		return
	}

	task.finalStateDB.SetTxContext(task.tx.GetHash(), task.index)
	task.finalStateDB.ApplyMVWriteSet(task.statedb.MVFullWriteList())

	for _, l := range task.statedb.GetLogs(*task.tx.GetHash(), task.blockNumber.Uint64()) {
		task.finalStateDB.AddLog(l)
	}

	// if *task.shouldDelayFeeCal {
	// 	task.finalStateDB.AddBalance(&task.coinbase, task.result.FeeTipped)
	// }

	for k, v := range task.statedb.Preimages() {
		task.finalStateDB.AddPreimage(k, v)
	}

	// Update the state with pending changes.
	//var root []byte

	// if task.config.IsByzantium(task.blockNumber) {
	// 	task.finalStateDB.Finalise()
	// } else {
	// 	//root = task.finalStateDB.IntermediateRoot(task.config.IsEIP158(task.blockNumber)).Bytes()
	// }

	*task.totalUsedGas += task.result.UsedGas

	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &types.Receipt{CumulativeGasUsed: *task.totalUsedGas}
	if task.result.Failed() {
		receipt.Status = types.ReceiptFAILED
	} else {
		receipt.Status = types.ReceiptSUCCESS
	}

	receipt.TxHash = task.tx.GetHash()
	receipt.GasUsed = task.result.UsedGas
	receipt.Ret = task.result.Return()

	// If the transaction created a contract, store the creation address in the receipt.
	if task.msg.To == nil {
		receipt.ContractAddress = types.NewAddress(crypto.CreateAddress(task.msg.From, task.tx.GetNonce()).Bytes())
	}

	//same as serial-execute
	if task.result.Failed() {
		if len(task.result.Revert()) > 0 {
			reason, errUnpack := abi.UnpackRevert(task.result.Revert())
			if errUnpack == nil {
				task.logger.Warnf("execute tx failed: %s: %s", task.result.Err.Error(), reason)
			} else {
				task.logger.Warnf("execute tx failed: %s", task.result.Err.Error())
			}
		} else {
			task.logger.Warnf("execute tx failed: %s", task.result.Err.Error())
		}

		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(task.result.Err.Error())
		if strings.HasPrefix(task.result.Err.Error(), vm.ErrExecutionReverted.Error()) {
			receipt.Ret = append(receipt.Ret, common.CopyBytes(task.result.ReturnData)...)
		}
	} else {
		receipt.Status = types.ReceiptSUCCESS
		receipt.Ret = task.result.Return()
	}

	// Set the receipt logs and create the bloom filter.
	receipt.EvmLogs = task.finalStateDB.GetLogs(*task.tx.GetHash(), task.blockNumber.Uint64())
	receipt.Bloom = ledger.CreateBloom(ledger.EvmReceipts{receipt})

	*task.receipts = append(*task.receipts, receipt)
	*task.allLogs = append(*task.allLogs, receipt.EvmLogs...)
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *ParallelStateProcessor) Process(block *types.Block, ledgerNow *ledger.Ledger, sp *StateDbPool, cfg vm.Config, interruptCtx context.Context) (ledger.EvmReceipts, []*types.EvmLog, uint64, error) {
	var (
		receipts    ledger.EvmReceipts
		header      = block.Header
		blockHash   = block.Hash()
		blockNumber = block.Height()
		allLogs     []*types.EvmLog
		usedGas     = new(uint64)
		metadata    bool
	)

	tasks := make([]blockstm.ExecTask, 0, len(block.Transactions))
	shouldDelayFeeCal := true
	coinbase := syscommon.StakingManagerContractAddr
	deps := make(map[int][]int)
	blockContext := NewEVMBlockContextAdaptor(block.Height(), uint64(block.Header.Timestamp), syscommon.StakingManagerContractAddr, getBlockHashFunc(ledgerNow.ChainLedger))
	cleansdb := ledgerNow.StateLedger.Copy()

	for i, tx := range block.Transactions {
		msg := TransactionToMessage(tx)
		if msg.From.Hex() == coinbase {
			shouldDelayFeeCal = false
		}

		task := &ExecutionTask{
			logger:            loggers.Logger(loggers.Executor),
			msg:               *msg,
			config:            p.config,
			gasLimit:          p.gasLimit,
			blockNumber:       big.NewInt(int64(blockNumber)),
			blockHash:         *blockHash,
			tx:                tx,
			index:             i,
			cleanStateDB:      cleansdb,
			finalStateDB:      ledgerNow.StateLedger,
			blockChain:        p.bc,
			header:            header,
			evmConfig:         cfg,
			shouldDelayFeeCal: &shouldDelayFeeCal,
			sender:            *types.NewAddress(msg.From.Bytes()),
			totalUsedGas:      usedGas,
			receipts:          &receipts,
			allLogs:           &allLogs,
			dependencies:      deps[i],
			coinbase:          *types.NewAddressByStr(coinbase),
			blockContext:      blockContext,
			sp:                sp,
		}

		tasks = append(tasks, task)
	}

	profile := false
	p.logger.Info("blockstm.ExecuteParallel")
	_, err := blockstm.ExecuteParallel(tasks, profile, metadata, p.parallelSpeculativeProcesses, interruptCtx)
	feeTippedAll := big.NewInt(0)
	for _, task := range tasks {
		executionTask := task.(*ExecutionTask)
		if executionTask.resultErr == nil && *executionTask.shouldDelayFeeCal {
			feeTippedAll.Add(feeTippedAll, executionTask.result.FeeTipped)
		}
		executionTask.ReleaseStateDb()
	}
	p.logger.Info("fee tipped:", feeTippedAll)
	ledgerNow.StateLedger.AddBalance(types.NewAddressByStr(coinbase), feeTippedAll)
	if err != nil {
		return nil, nil, 0, err
	}
	return receipts, allLogs, *usedGas, nil
}

func GetDeps(txDependency [][]uint64) map[int][]int {
	deps := make(map[int][]int)
	for i := 0; i <= len(txDependency)-1; i++ {
		deps[i] = []int{}
		for j := 0; j <= len(txDependency[i])-1; j++ {
			deps[i] = append(deps[i], int(txDependency[i][j]))
		}
	}

	return deps
}

func GetDepsByBlock(block *types.Block) map[int][]int {
	type NonceIndexMap struct {
		Nonce uint64
		Index int
	}
	deps := make(map[int][]int)

	nonceMap := make(map[string]*NonceIndexMap)
	txs := block.Transactions
	for i, tx := range txs {
		deps[i] = []int{}
		addr := tx.GetFrom().String()
		if nonceMap[addr] == nil {
			nonceMap[addr] = &NonceIndexMap{
				Nonce: tx.GetNonce(),
				Index: i,
			}
		} else {
			deps[i] = append(deps[i], nonceMap[addr].Index)
			nonceMap[addr].Nonce = tx.GetNonce()
			nonceMap[addr].Index = i
		}
	}
	return deps
}

// returns true if dependencies are correct
func VerifyDeps(deps map[int][]int) bool {
	// number of transactions in the block
	n := len(deps)

	// Handle out-of-range and circular dependency problem
	for i := 0; i <= n-1; i++ {
		val := deps[i]
		for _, depTx := range val {
			if depTx >= n || depTx >= i {
				return false
			}
		}
	}

	return true
}