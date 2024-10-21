package executor

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/axiomesh/axiom-ledger/internal/components"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
	synccomm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	consensuscommon "github.com/axiomesh/axiom-ledger/internal/consensus/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/events"
)

type InvalidReason string

type BlockWrapper struct {
	block     *types.Block
	invalidTx map[int]InvalidReason
}

func (exec *BlockExecutor) applyTransactions(txs []*types.Transaction, height uint64) []*types.Receipt {
	receipts := make([]*types.Receipt, 0, len(txs))

	for i, tx := range txs {
		receipts = append(receipts, exec.applyTransaction(i, tx, height))
	}

	exec.logger.Debugf("executor executed %d txs", len(txs))

	return receipts
}

func (exec *BlockExecutor) executeInDiffMode(commitEvent *consensuscommon.CommitEvent) (*ledger.BlockData, *types.StateJournal, error) {
	current := time.Now()
	block := commitEvent.Block.Clone()
	receipts := commitEvent.SyncMeta.Receipts
	stateJournal := commitEvent.SyncMeta.StateJournal

	// 1. update state ledger from diff
	if err := exec.ledger.StateLedger.ApplyStateJournal(block.Height(), stateJournal); err != nil {
		return nil, nil, err
	}
	txCount := len(commitEvent.Block.Transactions)
	exec.ledger.StateLedger.SetTxContext(commitEvent.Block.Transactions[txCount-1].GetHash(), txCount-1)

	// 2. get txs hash
	txHashList := make([]*types.Hash, 0)
	for _, tx := range block.Transactions {
		txHashList = append(txHashList, tx.GetHash())
	}

	exec.logger.WithFields(logrus.Fields{
		"hash":             block.Hash().String(),
		"height":           block.Header.Number,
		"epoch":            block.Header.Epoch,
		"coinbase":         syscommon.StakingManagerContractAddr,
		"proposer_node_id": block.Header.ProposerNodeID,
		"gas_used":         block.Header.GasUsed,
		"parent_hash":      block.Header.ParentHash.String(),
		"tx_root":          block.Header.TxRoot.String(),
		"receipt_root":     block.Header.ReceiptRoot.String(),
		"state_root":       block.Header.StateRoot.String(),
	}).Info("[Execute-Block-Diff] Block meta")

	exec.logger.WithFields(logrus.Fields{
		"height": block.Header.Number,
		"count":  len(block.Transactions),
		"elapse": time.Since(current),
	}).Info("[Execute-Block-Diff] Executed block")

	calcBlockSize.Observe(float64(block.Size()))
	executeBlockDuration.Observe(float64(time.Since(current)) / float64(time.Second))

	return &ledger.BlockData{
		Block:      block,
		Receipts:   receipts,
		TxHashList: txHashList,
	}, stateJournal, nil
}

func (exec *BlockExecutor) executeInFullMode(block *types.Block) (*ledger.BlockData, *types.StateJournal, error) {
	current := time.Now()
	receipts := make([]*types.Receipt, 0)
	receipts = exec.applyTransactions(block.Transactions, block.Height())

	totalGasFee := new(big.Int)
	for i, receipt := range receipts {
		receipt.EffectiveGasPrice = block.Transactions[i].Inner.EffectiveGasPrice(big.NewInt(0))
		txGasFee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
		totalGasFee = totalGasFee.Add(totalGasFee, txGasFee)
	}
	block.Header.TotalGasFee = totalGasFee
	block.Header.GasFeeReward = totalGasFee
	block.Header.GasPrice = 0
	for _, hook := range exec.afterBlockHooks {
		if err := hook.Func(block); err != nil {
			exec.logger.WithFields(logrus.Fields{
				"height": block.Height(),
				"err":    err.Error(),
				"hook":   hook.Name,
			}).Panic("Execute afterBlock hook failed")
			return nil, nil, err
		}
	}

	exec.ledger.StateLedger.Finalise()

	applyTxsDuration.Observe(float64(time.Since(current)) / float64(time.Second))
	exec.logger.WithFields(logrus.Fields{
		"time":  time.Since(current),
		"count": len(block.Transactions),
	}).Info("[Execute-Block] Apply transactions elapsed")

	calcMerkleStart := time.Now()
	txRoot, err := components.CalcTxsMerkleRoot(block.Transactions)
	if err != nil {
		return nil, nil, err
	}

	receiptRoot, err := components.CalcReceiptMerkleRoot(receipts)
	if err != nil {
		return nil, nil, err
	}

	calcMerkleDuration.Observe(float64(time.Since(calcMerkleStart)) / float64(time.Second))

	block.Header.TxRoot = txRoot
	block.Header.ReceiptRoot = receiptRoot
	block.Header.ParentHash = exec.currentBlockHash

	stateJournal, err := exec.ledger.StateLedger.Commit()
	if err != nil {
		return nil, nil, fmt.Errorf("commit stateLedger failed: %w", err)
	}
	block.Header.StateRoot = stateJournal.RootHash
	block.Header.GasUsed = exec.cumulativeGasUsed

	block.Header.CalculateHash()

	exec.logger.WithFields(logrus.Fields{
		"hash":             block.Hash().String(),
		"height":           block.Header.Number,
		"epoch":            block.Header.Epoch,
		"coinbase":         syscommon.StakingManagerContractAddr,
		"proposer_node_id": block.Header.ProposerNodeID,
		"gas_used":         block.Header.GasUsed,
		"parent_hash":      block.Header.ParentHash.String(),
		"tx_root":          block.Header.TxRoot.String(),
		"receipt_root":     block.Header.ReceiptRoot.String(),
		"state_root":       block.Header.StateRoot.String(),
	}).Info("[Execute-Block] Block meta")

	calcBlockSize.Observe(float64(block.Size()))
	executeBlockDuration.Observe(float64(time.Since(current)) / float64(time.Second))

	exec.updateLogsBlockHash(receipts, block.Hash())
	block.Header.Bloom = ledger.CreateBloom(receipts)

	txHashList := make([]*types.Hash, 0)
	for _, tx := range block.Transactions {
		txHashList = append(txHashList, tx.GetHash())
	}
	exec.logger.WithFields(logrus.Fields{
		"height": block.Header.Number,
		"count":  len(block.Transactions),
		"elapse": time.Since(current),
	}).Info("[Execute-Block] Executed block")

	return &ledger.BlockData{
		Block:      block,
		Receipts:   receipts,
		TxHashList: txHashList,
	}, stateJournal, nil
}

func (exec *BlockExecutor) updateChainState(updateEpoch bool) error {
	exec.chainState.UpdateChainMeta(exec.ledger.ChainLedger.GetChainMeta())
	exec.chainState.TryUpdateSelfNodeInfo()
	if updateEpoch {
		epochManagerContract := framework.EpochManagerBuildConfig.Build(syscommon.NewVMContextByExecutor(exec.ledger.StateLedger))
		info, err := epochManagerContract.CurrentEpoch()
		if err != nil {
			return err
		}
		newEpoch := info.ToTypesEpoch()
		nodeManagerContract := framework.NodeManagerBuildConfig.Build(syscommon.NewVMContextByExecutor(exec.ledger.StateLedger))
		votingPowers, err := nodeManagerContract.GetActiveValidatorVotingPowers()
		if err != nil {
			return err
		}
		if err = exec.chainState.UpdateByEpochInfo(newEpoch, lo.SliceToMap(votingPowers, func(item node_manager.ConsensusVotingPower) (uint64, int64) {
			return item.NodeID, item.ConsensusVotingPower
		})); err != nil {
			return err
		}
	}
	return nil
}

func (exec *BlockExecutor) processExecuteEvent(commitEvent *consensuscommon.CommitEvent) {
	block := commitEvent.Block.Clone()

	var updateEpoch bool
	if block.Header.Number == (exec.chainState.EpochInfo.StartBlock + exec.chainState.EpochInfo.EpochPeriod - 1) {
		updateEpoch = true
	}

	// check executor handle the right block
	if block.Header.Number != exec.currentHeight+1 {
		if block.Header.Number <= exec.currentHeight {
			exec.logger.WithFields(logrus.Fields{"block height": block.Header.Number,
				"expectHeight": exec.currentHeight + 1}).Warning("current block height is not matched, will ignore it...")
			return
		} else {
			panic(fmt.Sprintf("block height %d is not matched the current expect height %d", block.Header.Number, exec.currentHeight+1))
		}
	}

	exec.cumulativeGasUsed = 0
	exec.evm = newEvm(block.Height(), uint64(block.Header.Timestamp), exec.evmChainCfg, exec.ledger.StateLedger, exec.ledger.ChainLedger, syscommon.StakingManagerContractAddr)
	// get last block's stateRoot to init the latest world state trie
	parentBlockHeader, err := exec.ledger.ChainLedger.GetBlockHeader(block.Height() - 1)
	if err != nil {
		exec.logger.WithFields(logrus.Fields{
			"height": block.Height() - 1,
			"err":    err.Error(),
		}).Panic("Get last block from ledger error")
		return
	}
	exec.ledger.StateLedger.PrepareBlock(parentBlockHeader.StateRoot, block.Height())

	var (
		data         *ledger.BlockData
		stateJournal *types.StateJournal
	)

	if commitEvent.SyncMeta.Mode == synccomm.SyncModeDiff {
		data, stateJournal, err = exec.executeInDiffMode(commitEvent)
	} else {
		data, stateJournal, err = exec.executeInFullMode(block)
	}
	if err != nil {
		panic(fmt.Errorf("execute block failed: %w", err))
	}

	now := time.Now()
	exec.ledger.PersistBlockData(data)

	// metrics for cal tx tps
	txCounter.Add(float64(len(data.Block.Transactions)))
	if block.Header.ProposerNodeID == exec.chainState.SelfNodeInfo.ID {
		proposedBlockCounter.Inc()
	}

	exec.logger.WithFields(logrus.Fields{
		"height": data.Block.Header.Number,
		"hash":   data.Block.Hash().String(),
		"count":  len(data.Block.Transactions),
		"elapse": time.Since(now),
	}).Info("[Execute-Block] Persisted block")

	// only archive node will archive data
	if err = exec.ledger.StateLedger.Archive(data.Block.Header, stateJournal); err != nil {
		panic(err)
	}

	exec.currentHeight = block.Header.Number
	exec.currentBlockHash = block.Hash()

	if err = exec.updateChainState(updateEpoch); err != nil {
		panic(fmt.Errorf("update chain state failed: %w", err))
	}
	exec.ledger.StateLedger.UpdateChainState(exec.chainState)

	txHashList := make([]string, len(data.Block.Transactions))
	lo.ForEach(data.Block.Transactions, func(item *types.Transaction, index int) {
		txHashList[index] = item.RbftGetTxHash()
	})

	// validate local checkpoint with quorum checkpoint
	if commitEvent.SyncMeta.QuorumStateUpdatedCheckpoint != nil {
		if block.Header.Number != commitEvent.SyncMeta.QuorumStateUpdatedCheckpoint.Height {
			panic(fmt.Errorf("local checkpoint height %d not match quorum checkpoint height %d",
				block.Header.Number, commitEvent.SyncMeta.QuorumStateUpdatedCheckpoint.Height))
		}
		if exec.currentBlockHash.String() != commitEvent.SyncMeta.QuorumStateUpdatedCheckpoint.Digest {
			panic(fmt.Errorf("local checkpoint %s not match quorum checkpoint %s in height %d",
				exec.currentBlockHash.String(), commitEvent.SyncMeta.QuorumStateUpdatedCheckpoint.Digest, block.Header.Number))
		}
	}
	exec.postBlockEvent(data.Block, txHashList)
	exec.postLogsEvent(data.Receipts)
	exec.clear()
	types.RecycleStateJournal(stateJournal)
}

func (exec *BlockExecutor) postBlockEvent(block *types.Block, txHashList []string) {
	exec.blockFeed.Send(events.ExecutedEvent{
		Block:      block,
		TxHashList: txHashList,
	})
	exec.blockFeedForRemote.Send(events.ExecutedEvent{
		Block:      block,
		TxHashList: txHashList,
	})
}

func (exec *BlockExecutor) postLogsEvent(receipts []*types.Receipt) {
	logs := make([]*types.EvmLog, 0)
	for _, receipt := range receipts {
		logs = append(logs, receipt.EvmLogs...)
	}

	exec.logsFeed.Send(logs)
}

func (exec *BlockExecutor) applyTransaction(i int, tx *types.Transaction, height uint64) *types.Receipt {

	exec.ledger.StateLedger.(*ledger.ArchiveStateLedger).PrepareTranct()

	defer func() {
		exec.ledger.StateLedger.SetNonce(tx.GetFrom(), tx.GetNonce()+1)
		exec.ledger.StateLedger.Finalise()
	}()

	exec.ledger.StateLedger.SetTxContext(tx.GetHash(), i)

	receipt := &types.Receipt{
		TxHash: tx.GetHash(),
	}

	var result *core.ExecutionResult
	var err error

	msg := TransactionToMessage(tx)

	statedb := exec.ledger.StateLedger
	evmStateDB := &ledger.EvmStateDBAdaptor{StateLedger: statedb}
	// TODO: Move to system contract
	snapshot := statedb.Snapshot()

	// execute evm
	gp := new(core.GasPool).AddGas(exec.gasLimit)
	txContext := core.NewEVMTxContext(msg)
	exec.evm.Reset(txContext, evmStateDB)
	exec.logger.Debugf("evm apply message, msg gas limit: %d, gas price: %s", msg.GasLimit, msg.GasPrice.Text(10))
	result, err = core.ApplyMessage(exec.evm, msg, gp)
	if err != nil {
		exec.logger.Errorf("apply tx failed: %s", err.Error())
		statedb.RevertToSnapshot(snapshot)
		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(err.Error())
		return receipt
	}
	if err == nil {
		exec.ledger.StateLedger.(*ledger.ArchiveStateLedger).FinalizeTransact()
	} else {
		exec.ledger.StateLedger.(*ledger.ArchiveStateLedger).RollbackTransact()
	}
	if result.Failed() {
		if len(result.Revert()) > 0 {
			reason, errUnpack := abi.UnpackRevert(result.Revert())
			if errUnpack == nil {
				exec.logger.Warnf("execute tx failed: %s: %s", result.Err.Error(), reason)
			} else {
				exec.logger.Warnf("execute tx failed: %s", result.Err.Error())
			}
		} else {
			exec.logger.Warnf("execute tx failed: %s", result.Err.Error())
		}

		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(result.Err.Error())
		if strings.HasPrefix(result.Err.Error(), vm.ErrExecutionReverted.Error()) {
			receipt.Ret = append(receipt.Ret, common.CopyBytes(result.ReturnData)...)
		}
	} else {
		receipt.Status = types.ReceiptSUCCESS
		receipt.Ret = result.Return()
	}

	receipt.TxHash = tx.GetHash()
	receipt.GasUsed = result.UsedGas
	if msg.To == nil || bytes.Equal(msg.To.Bytes(), common.Address{}.Bytes()) {
		receipt.ContractAddress = types.NewAddress(crypto.CreateAddress(exec.evm.TxContext.Origin, tx.GetNonce()).Bytes())
	}
	receipt.EvmLogs = exec.ledger.StateLedger.GetLogs(*receipt.TxHash, height)
	receipt.Bloom = ledger.CreateBloom(ledger.EvmReceipts{receipt})
	exec.cumulativeGasUsed += receipt.GasUsed
	receipt.CumulativeGasUsed = exec.cumulativeGasUsed

	return receipt
}

func (exec *BlockExecutor) clear() {
	exec.ledger.StateLedger.Clear()
}

func getBlockHashFunc(chainLedger ledger.ChainLedger) vm.GetHashFunc {
	return func(n uint64) common.Hash {
		blockHeader, err := chainLedger.GetBlockHeader(n)
		if err != nil {
			return common.Hash{}
		}
		return common.BytesToHash(blockHeader.Hash().Bytes())
	}
}

func newEvm(number uint64, timestamp uint64, chainCfg *params.ChainConfig, db ledger.StateLedger, chainLedger ledger.ChainLedger, coinbase string) *vm.EVM {
	if coinbase == "" {
		coinbase = syscommon.ZeroAddress
	}

	blkCtx := NewEVMBlockContextAdaptor(number, timestamp, coinbase, getBlockHashFunc(chainLedger))

	return vm.NewEVM(blkCtx, vm.TxContext{}, &ledger.EvmStateDBAdaptor{
		StateLedger: db,
	}, chainCfg, vm.Config{})
}

func (exec *BlockExecutor) NewEvmWithViewLedger(txCtx vm.TxContext, vmConfig vm.Config) (*vm.EVM, error) {
	if vmConfig.NoBaseFee && txCtx.GasPrice == nil {
		txCtx.GasPrice = big.NewInt(0)
	}
	var blkCtx vm.BlockContext
	meta := exec.ledger.ChainLedger.GetChainMeta()
	blockHeader, err := exec.ledger.ChainLedger.GetBlockHeader(meta.Height)
	if err != nil {
		return nil, err
	}

	evmLg := &ledger.EvmStateDBAdaptor{
		StateLedger: exec.ledger.NewView().StateLedger,
	}
	blkCtx = NewEVMBlockContextAdaptor(meta.Height, uint64(blockHeader.Timestamp), syscommon.StakingManagerContractAddr, getBlockHashFunc(exec.ledger.ChainLedger))
	return vm.NewEVM(blkCtx, txCtx, evmLg, exec.evmChainCfg, vmConfig), nil
}

func (exec *BlockExecutor) GetChainConfig() *params.ChainConfig {
	return exec.evmChainCfg
}

func (exec *BlockExecutor) updateLogsBlockHash(receipts []*types.Receipt, hash *types.Hash) {
	for _, receipt := range receipts {
		for _, log := range receipt.EvmLogs {
			log.BlockHash = hash
		}
	}
}
