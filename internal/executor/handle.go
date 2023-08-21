package executor

import (
	"bytes"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/cbergoon/merkletree"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom/internal/executor/system"
	"github.com/axiomesh/axiom/internal/ledger"
	"github.com/axiomesh/axiom/pkg/model/events"
	"github.com/axiomesh/eth-kit/adaptor"
	ethvm "github.com/axiomesh/eth-kit/evm"
	ethledger "github.com/axiomesh/eth-kit/ledger"
)

const (
	GasNormalTx = 21000
	GasFailedTx = 21000
	GasBVMTx    = 21000 * 10
)

type InvalidReason string

type BlockWrapper struct {
	block     *types.Block
	invalidTx map[int]InvalidReason
}

func (exec *BlockExecutor) applyTransactions(txs []*types.Transaction, invalidTxs map[int]InvalidReason) []*types.Receipt {
	receipts := make([]*types.Receipt, 0, len(txs))

	for i, tx := range txs {
		receipts = append(receipts, exec.applyTransaction(i, tx, invalidTxs[i]))
	}

	exec.logger.Debugf("executor executed %d txs", len(txs))

	return receipts
}

func (exec *BlockExecutor) processExecuteEvent(blockWrapper *BlockWrapper) *ledger.BlockData {
	var txHashList []*types.Hash
	current := time.Now()
	block := blockWrapper.block

	// check executor handle the right block
	if block.BlockHeader.Number != exec.currentHeight+1 {
		exec.logger.WithFields(logrus.Fields{"block height": block.BlockHeader.Number,
			"matchedHeight": exec.currentHeight + 1}).Warning("current block height is not matched")
		return nil
	}

	for _, tx := range block.Transactions {
		txHashList = append(txHashList, tx.GetHash())
	}

	// TODO: CHANGE COINBASE ADDRESSS
	exec.evm = newEvm(block.Height(), uint64(block.BlockHeader.Timestamp), exec.evmChainCfg, exec.ledger.StateLedger, exec.ledger.ChainLedger, exec.admins[0])
	exec.ledger.PrepareBlock(block.BlockHash, block.Height())
	receipts := exec.applyTransactions(block.Transactions, blockWrapper.invalidTx)
	applyTxsDuration.Observe(float64(time.Since(current)) / float64(time.Second))
	exec.logger.WithFields(logrus.Fields{
		"time":  time.Since(current),
		"count": len(block.Transactions),
	}).Debug("Apply transactions elapsed")

	calcMerkleStart := time.Now()
	txRoot, err := exec.buildTxMerkleTree(block.Transactions)
	if err != nil {
		panic(err)
	}

	receiptRoot, err := exec.calcReceiptMerkleRoot(receipts)
	if err != nil {
		panic(err)
	}

	calcMerkleDuration.Observe(float64(time.Since(calcMerkleStart)) / float64(time.Second))

	block.BlockHeader.TxRoot = txRoot
	block.BlockHeader.ReceiptRoot = receiptRoot
	block.BlockHeader.ParentHash = exec.currentBlockHash
	block.BlockHeader.Bloom = ledger.CreateBloom(receipts)
	gasPrice, err := exec.GasPrice()
	if err != nil {
		panic(err)
	}
	block.BlockHeader.GasPrice = gasPrice.Int64()

	accounts, journalHash := exec.ledger.FlushDirtyData()

	block.BlockHeader.StateRoot = journalHash
	block.BlockHash = block.Hash()

	exec.logger.WithFields(logrus.Fields{
		"tx_root":      block.BlockHeader.TxRoot.String(),
		"receipt_root": block.BlockHeader.ReceiptRoot.String(),
		"state_root":   block.BlockHeader.StateRoot.String(),
	}).Debug("block meta")
	calcBlockSize.Observe(float64(block.Size()))
	executeBlockDuration.Observe(float64(time.Since(current)) / float64(time.Second))

	exec.getLogsForReceipt(receipts, block.Height(), block.BlockHash)

	data := &ledger.BlockData{
		Block:      block,
		Receipts:   receipts,
		Accounts:   accounts,
		TxHashList: txHashList,
	}

	exec.logger.WithFields(logrus.Fields{
		"height": blockWrapper.block.BlockHeader.Number,
		"count":  len(blockWrapper.block.Transactions),
		"elapse": time.Since(current),
	}).Info("Executed block")

	now := time.Now()
	exec.ledger.PersistBlockData(data)
	exec.postBlockEvent(data.Block, data.TxHashList)
	exec.postLogsEvent(data.Receipts)
	exec.logger.WithFields(logrus.Fields{
		"gasPrice": data.Block.BlockHeader.GasPrice,
		"height":   data.Block.BlockHeader.Number,
		"hash":     data.Block.BlockHash.String(),
		"count":    len(data.Block.Transactions),
		"elapse":   time.Since(now),
	}).Info("Persisted block")

	exec.currentHeight = block.BlockHeader.Number
	exec.currentBlockHash = block.BlockHash
	exec.clear()

	return nil
}

func (exec *BlockExecutor) listenPreExecuteEvent() {
	for {
		select {
		case commitEvent := <-exec.preBlockC:
			now := time.Now()
			blockWrapper := exec.verifySign(commitEvent)
			exec.logger.WithFields(logrus.Fields{
				"height": commitEvent.Block.BlockHeader.Number,
				"count":  len(commitEvent.Block.Transactions),
				"elapse": time.Since(now),
			}).Debug("Verified signature")
			exec.blockC <- blockWrapper
		case <-exec.ctx.Done():
			return
		}
	}
}

func (exec *BlockExecutor) buildTxMerkleTree(txs []*types.Transaction) (*types.Hash, error) {
	hash, err := calcMerkleRoot(lo.Map(txs, func(item *types.Transaction, index int) merkletree.Content {
		return item.GetHash()
	}))
	if err != nil {
		return nil, err
	}

	return hash, nil
}

func (exec *BlockExecutor) verifySign(commitEvent *types.CommitEvent) *BlockWrapper {
	blockWrapper := &BlockWrapper{
		block:     commitEvent.Block,
		invalidTx: make(map[int]InvalidReason),
	}

	if commitEvent.Block.BlockHeader.Number == 1 {
		return blockWrapper
	}

	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
	)
	txs := commitEvent.Block.Transactions
	txsLen := len(commitEvent.LocalList)
	wg.Add(len(txs))
	for i, tx := range txs {
		// if the tx is received from api, we will pass the verify.
		if txsLen > i && commitEvent.LocalList[i] {
			wg.Done()
			continue
		}
		go func(i int, tx *types.Transaction) {
			defer wg.Done()
			err := tx.VerifySignature()
			if err != nil {
				mutex.Lock()
				defer mutex.Unlock()
				blockWrapper.invalidTx[i] = InvalidReason(err.Error())
			}
		}(i, tx)
	}
	wg.Wait()

	return blockWrapper
}

func (exec *BlockExecutor) postBlockEvent(block *types.Block, txHashList []*types.Hash) {
	go exec.blockFeed.Send(events.ExecutedEvent{
		Block:      block,
		TxHashList: txHashList,
	})
	go exec.blockFeedForRemote.Send(events.ExecutedEvent{
		Block:      block,
		TxHashList: txHashList,
	})
}

func (exec *BlockExecutor) postLogsEvent(receipts []*types.Receipt) {
	go func() {
		logs := make([]*types.EvmLog, 0)
		for _, receipt := range receipts {
			logs = append(logs, receipt.EvmLogs...)
		}

		exec.logsFeed.Send(logs)
	}()
}

// TODO: process invalidReason
func (exec *BlockExecutor) applyTransaction(i int, tx *types.Transaction, _ InvalidReason) *types.Receipt {
	defer func() {
		exec.ledger.SetNonce(tx.GetFrom(), tx.GetNonce()+1)
		exec.ledger.Finalise(true)
	}()

	exec.ledger.SetTxContext(tx.GetHash(), i)
	receipt := exec.applyEthTransaction(i, tx)
	exec.payGasFee(tx, receipt.GasUsed)

	return receipt
}

func (exec *BlockExecutor) applyEthTransaction(_ int, tx *types.Transaction) *types.Receipt {
	receipt := &types.Receipt{
		Version: tx.GetVersion(),
		TxHash:  tx.GetHash(),
	}

	var result *ethvm.ExecutionResult
	var err error

	msg := adaptor.TransactionToMessage(tx)
	msg.GasPrice, err = exec.getCurrentGasPrice()
	if err != nil {
		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(err.Error())
		return receipt
	}

	exec.logger.Debugf("tx gas: %v", tx.GetGas())
	exec.logger.Debugf("tx gas price: %v", tx.GetGasPrice())
	statedb := exec.ledger.StateLedger
	snapshot := statedb.Snapshot()
	contract, ok := system.GetSystemContract(tx.GetTo())
	if ok {
		// execute built contract
		contract.Reset(statedb)
		result, err = contract.Run(msg)
	} else {
		// execute evm
		gp := new(ethvm.GasPool).AddGas(exec.gasLimit)
		txContext := ethvm.NewEVMTxContext(msg)
		exec.evm.Reset(txContext, exec.ledger.StateLedger)
		exec.logger.Debugf("msg gas: %v", msg.GasPrice)
		result, err = ethvm.ApplyMessage(exec.evm, msg, gp)
	}

	if err != nil {
		exec.logger.Errorf("apply msg failed: %s", err.Error())
		statedb.RevertToSnapshot(snapshot)
		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(err.Error())
		exec.ledger.Finalise(true)
		return receipt
	}
	if result.Failed() {
		exec.logger.Warnf("execute tx failed: %s", result.Err.Error())
		receipt.Status = types.ReceiptFAILED
		receipt.Ret = []byte(result.Err.Error())
		if strings.HasPrefix(result.Err.Error(), ethvm.ErrExecutionReverted.Error()) {
			receipt.Ret = append(receipt.Ret, common.CopyBytes(result.ReturnData)...)
		}
	} else {
		receipt.Status = types.ReceiptSUCCESS
		receipt.Ret = result.Return()
	}

	receipt.TxHash = tx.GetHash()
	receipt.GasUsed = result.UsedGas
	exec.ledger.Finalise(true)
	if msg.To == nil || bytes.Equal(msg.To.Bytes(), common.Address{}.Bytes()) {
		receipt.ContractAddress = types.NewAddress(crypto.CreateAddress(exec.evm.TxContext.Origin, tx.GetNonce()).Bytes())
	}

	// receipt.EvmLogs = exec.ledger.GetLogs(*tx.GetHash(), height, blockHash)
	receipt.Bloom = ledger.CreateBloom(ledger.EvmReceipts{receipt})
	return receipt
}

func (exec *BlockExecutor) clear() {
	exec.ledger.Clear()
}

func (exec *BlockExecutor) calcReceiptMerkleRoot(receipts []*types.Receipt) (*types.Hash, error) {
	current := time.Now()

	receiptHashes := make([]merkletree.Content, 0, len(receipts))
	for _, receipt := range receipts {
		receiptHashes = append(receiptHashes, receipt.Hash())
	}
	receiptRoot, err := calcMerkleRoot(receiptHashes)
	if err != nil {
		return nil, err
	}

	exec.logger.WithField("time", time.Since(current)).Debug("Calculate receipt merkle roots")

	return receiptRoot, nil
}

func calcMerkleRoot(contents []merkletree.Content) (*types.Hash, error) {
	if len(contents) == 0 {
		return &types.Hash{}, nil
	}

	tree, err := merkletree.NewTree(contents)
	if err != nil {
		return nil, err
	}

	return types.NewHash(tree.MerkleRoot()), nil
}

func newEvm(number uint64, timestamp uint64, chainCfg *params.ChainConfig, db ethledger.StateLedger, chainLedger ethledger.ChainLedger, admin string) *ethvm.EVM {
	blkCtx := ethvm.NewEVMBlockContext(number, timestamp, db, chainLedger, admin)

	return ethvm.NewEVM(blkCtx, ethvm.TxContext{}, db, chainCfg, ethvm.Config{})
}

func (exec *BlockExecutor) GetEvm(txCtx ethvm.TxContext, vmConfig ethvm.Config) (*ethvm.EVM, error) {
	var blkCtx ethvm.BlockContext
	meta := exec.ledger.GetChainMeta()
	block, err := exec.ledger.GetBlock(meta.Height)
	if err != nil {
		exec.logger.Errorf("fail to get block at %d: %v", meta.Height, err.Error())
		return nil, err
	}

	blkCtx = ethvm.NewEVMBlockContext(meta.Height, uint64(block.BlockHeader.Timestamp), exec.ledger.StateLedger, exec.ledger.ChainLedger, exec.admins[0])
	return ethvm.NewEVM(blkCtx, txCtx, exec.ledger.StateLedger, exec.evmChainCfg, vmConfig), nil
}

// getCurrentGasPrice returns the current block's gas price, which is
// stored in the last block's blockheader
func (exec *BlockExecutor) getCurrentGasPrice() (*big.Int, error) {
	latestHeight := exec.ledger.ChainLedger.GetChainMeta().Height
	block, err := exec.ledger.GetBlock(latestHeight)
	if err != nil {
		return nil, err
	}
	return big.NewInt(block.BlockHeader.GasPrice), nil
}

// payGasFee share the revenue to nodes, now it is empty
// leave the function for the future use.
func (exec *BlockExecutor) payGasFee(tx *types.Transaction, gasUsed uint64) {
}

func (exec *BlockExecutor) getLogsForReceipt(receipts []*types.Receipt, height uint64, hash *types.Hash) {
	for _, receipt := range receipts {
		receipt.EvmLogs = exec.ledger.GetLogs(*receipt.TxHash, height, hash)
	}
}
