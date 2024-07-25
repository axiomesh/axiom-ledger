package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/storage/blockfile"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var (
	ErrNotFound = errors.New("not found in DB")
)

var _ ChainLedger = (*ChainLedgerImpl)(nil)

type ChainLedgerImpl struct {
	blockchainStore kv.Storage
	bf              blockfile.BlockFile
	repo            *repo.Repo
	chainMeta       *types.ChainMeta
	logger          logrus.FieldLogger

	// block height -> block txs
	blockTxsCache *lru.Cache[uint64, []*types.Transaction]

	// block height -> block receipts
	blockReceiptsCache *lru.Cache[uint64, []*types.Receipt]

	// block height -> block header
	blockHeaderCache *lru.Cache[uint64, *types.BlockHeader]

	// block height -> block extra
	blockExtraCache *lru.Cache[uint64, *types.BlockExtra]
}

func newChainLedger(rep *repo.Repo, bcStorage kv.Storage, bf blockfile.BlockFile) (*ChainLedgerImpl, error) {
	c := &ChainLedgerImpl{
		blockchainStore: bcStorage,
		bf:              bf,
		repo:            rep,
		logger:          loggers.Logger(loggers.Storage),
	}

	var err error
	c.chainMeta, err = c.LoadChainMeta()
	if err != nil {
		return nil, fmt.Errorf("load chain meta failed: %w", err)
	}

	err = c.checkChainMeta()
	if err != nil {
		return nil, fmt.Errorf("check chain meta failed: %w", err)
	}

	c.blockTxsCache, err = lru.New[uint64, []*types.Transaction](rep.Config.Ledger.ChainLedgerCacheSize)
	if err != nil {
		return nil, fmt.Errorf("new block txs cache failed: %w", err)
	}

	c.blockReceiptsCache, err = lru.New[uint64, []*types.Receipt](rep.Config.Ledger.ChainLedgerCacheSize)
	if err != nil {
		return nil, fmt.Errorf("new block receipts cache failed: %w", err)
	}

	c.blockHeaderCache, err = lru.New[uint64, *types.BlockHeader](rep.Config.Ledger.ChainLedgerCacheSize)
	if err != nil {
		return nil, fmt.Errorf("new block header cache failed: %w", err)
	}

	c.blockExtraCache, err = lru.New[uint64, *types.BlockExtra](rep.Config.Ledger.ChainLedgerCacheSize)
	if err != nil {
		return nil, fmt.Errorf("new block extra cache failed: %w", err)
	}

	return c, nil
}

func NewChainLedger(rep *repo.Repo, storageDir string) (*ChainLedgerImpl, error) {
	bcStoragePath := storagemgr.GetLedgerComponentPath(rep, storagemgr.BlockChain)
	if storageDir != "" {
		bcStoragePath = path.Join(storageDir, storagemgr.BlockChain)
	}
	bcStorage, err := storagemgr.OpenWithMetrics(bcStoragePath, storagemgr.BlockChain)
	if err != nil {
		return nil, fmt.Errorf("create blockchain storage: %w", err)
	}

	cachedStateStorage := storagemgr.NewCachedStorage(bcStorage, 128)

	bfStoragePath := storagemgr.GetLedgerComponentPath(rep, storagemgr.Blockfile)
	if storageDir != "" {
		bfStoragePath = path.Join(storageDir, storagemgr.Blockfile)
	}
	bf, err := blockfile.NewBlockFile(bfStoragePath, loggers.Logger(loggers.Storage))
	if err != nil {
		return nil, fmt.Errorf("blockfile initialize: %w", err)
	}

	return newChainLedger(rep, cachedStateStorage, bf)
}

func (l *ChainLedgerImpl) GetBlockNumberByHash(hash *types.Hash) (uint64, error) {
	data := l.blockchainStore.Get(utils.CompositeKey(utils.BlockHashKey, hash.String()))
	if data == nil {
		return 0, ErrNotFound
	}

	height, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, fmt.Errorf("wrong height, %w", err)
	}
	return uint64(height), nil
}

// GetBlock get block with height
func (l *ChainLedgerImpl) GetBlock(height uint64) (*types.Block, error) {
	header, err := l.GetBlockHeader(height)
	if err != nil {
		return nil, err
	}
	txs, err := l.GetBlockTxList(height)
	if err != nil {
		return nil, err
	}
	extra, err := l.GetBlockExtra(height)
	if err != nil {
		return nil, err
	}
	return &types.Block{
		Header:       header,
		Transactions: txs,
		Extra:        extra,
	}, nil
}

func (l *ChainLedgerImpl) GetBlockHeader(height uint64) (*types.BlockHeader, error) {
	if height > l.chainMeta.Height || l.chainMeta.BlockHash == nil {
		return nil, ErrNotFound
	}

	blockHeader, ok := l.blockHeaderCache.Get(height)
	if !ok {
		data, err := l.bf.Get(blockfile.BlockFileHeaderTable, height)
		if err != nil {
			return nil, fmt.Errorf("get block header with height %d from blockfile failed: %w", height, err)
		}
		blockHeader = &types.BlockHeader{}
		if err := blockHeader.Unmarshal(data); err != nil {
			return nil, fmt.Errorf("unmarshal block header error: %w", err)
		}
		l.blockHeaderCache.Add(height, blockHeader)
	}

	return blockHeader, nil
}

func (l *ChainLedgerImpl) GetBlockExtra(height uint64) (*types.BlockExtra, error) {
	if height > l.chainMeta.Height || l.chainMeta.BlockHash == nil {
		return nil, ErrNotFound
	}

	blockExtra, ok := l.blockExtraCache.Get(height)
	if !ok {
		data, err := l.bf.Get(blockfile.BlockFileExtraTable, height)
		if err != nil {
			return nil, fmt.Errorf("get block extra with height %d from blockfile failed: %w", height, err)
		}
		blockExtra = &types.BlockExtra{}
		if err := blockExtra.Unmarshal(data); err != nil {
			return nil, fmt.Errorf("unmarshal block extra error: %w", err)
		}
		l.blockExtraCache.Add(height, blockExtra)
	}

	return blockExtra, nil
}

func (l *ChainLedgerImpl) GetBlockTxHashList(height uint64) ([]*types.Hash, error) {
	if height > l.chainMeta.Height || l.chainMeta.BlockHash == nil {
		return nil, ErrNotFound
	}

	txHashesData := l.blockchainStore.Get(utils.CompositeKey(utils.BlockTxSetKey, height))
	if txHashesData == nil {
		return nil, errors.New("cannot get tx hashes of block")
	}
	txHashes := make([]*types.Hash, 0)
	if err := json.Unmarshal(txHashesData, &txHashes); err != nil {
		return nil, fmt.Errorf("unmarshal tx hash data error: %w", err)
	}
	return txHashes, nil
}

func (l *ChainLedgerImpl) GetBlockTxList(height uint64) ([]*types.Transaction, error) {
	if height > l.chainMeta.Height || l.chainMeta.BlockHash == nil {
		return nil, ErrNotFound
	}

	txs, ok := l.blockTxsCache.Get(height)
	if !ok {
		txsBytes, err := l.bf.Get(blockfile.BlockFileTXsTable, height)
		if err != nil {
			return nil, fmt.Errorf("get transactions with height %d from blockfile failed: %w", height, err)
		}
		txs, err = types.UnmarshalTransactions(txsBytes)
		if err != nil {
			return nil, fmt.Errorf("unmarshal txs bytes error: %w", err)
		}
		l.blockTxsCache.Add(height, txs)
	}
	return txs, nil
}

// GetTransaction get the transaction using transaction hash
func (l *ChainLedgerImpl) GetTransaction(hash *types.Hash) (*types.Transaction, error) {
	start := time.Now()
	metaBytes := l.blockchainStore.Get(utils.CompositeKey(utils.TransactionMetaKey, hash.String()))
	getTransactionDuration.Observe(float64(time.Since(start)) / float64(time.Second))
	getTransactionCounter.Inc()
	if metaBytes == nil {
		return nil, ErrNotFound
	}
	meta := &types.TransactionMeta{}
	if err := meta.Unmarshal(metaBytes); err != nil {
		return nil, fmt.Errorf("unmarshal transaction meta bytes error: %w", err)
	}

	var txs []*types.Transaction
	txs, ok := l.blockTxsCache.Get(meta.BlockHeight)
	if !ok {
		txsBytes, err := l.bf.Get(blockfile.BlockFileTXsTable, meta.BlockHeight)
		if err != nil {
			return nil, fmt.Errorf("get transactions with height %d from blockfile failed: %w", meta.BlockHeight, err)
		}

		return types.UnmarshalTransactionWithIndex(txsBytes, meta.Index)
	}

	return txs[meta.Index], nil
}

func (l *ChainLedgerImpl) GetTransactionCount(height uint64) (uint64, error) {
	txHashesData := l.blockchainStore.Get(utils.CompositeKey(utils.BlockTxSetKey, height))
	if txHashesData == nil {
		return 0, errors.New("cannot get tx hashes of block")
	}
	txHashes := make([]types.Hash, 0)
	if err := json.Unmarshal(txHashesData, &txHashes); err != nil {
		return 0, fmt.Errorf("unmarshal tx hash data error: %w", err)
	}

	return uint64(len(txHashes)), nil
}

// GetTransactionMeta get the transaction meta data
func (l *ChainLedgerImpl) GetTransactionMeta(hash *types.Hash) (*types.TransactionMeta, error) {
	data := l.blockchainStore.Get(utils.CompositeKey(utils.TransactionMetaKey, hash.String()))
	if data == nil {
		return nil, ErrNotFound
	}

	meta := &types.TransactionMeta{}
	if err := meta.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("unmarshal transaction meta error: %w", err)
	}

	return meta, nil
}

// GetReceipt get the transaction receipt
func (l *ChainLedgerImpl) GetReceipt(hash *types.Hash) (*types.Receipt, error) {
	metaBytes := l.blockchainStore.Get(utils.CompositeKey(utils.TransactionMetaKey, hash.String()))
	if metaBytes == nil {
		return nil, ErrNotFound
	}
	meta := &types.TransactionMeta{}
	if err := meta.Unmarshal(metaBytes); err != nil {
		return nil, fmt.Errorf("unmarshal transaction meta bytes error: %w", err)
	}

	var rs []*types.Receipt
	rs, ok := l.blockReceiptsCache.Get(meta.BlockHeight)
	if !ok {
		rsBytes, err := l.bf.Get(blockfile.BlockFileReceiptsTable, meta.BlockHeight)
		if err != nil {
			return nil, fmt.Errorf("get receipts with height %d from blockfile failed: %w", meta.BlockHeight, err)
		}

		return types.UnmarshalReceiptWithIndex(rsBytes, meta.Index)
	}

	return rs[meta.Index], nil
}

func (l *ChainLedgerImpl) GetBlockReceipts(height uint64) ([]*types.Receipt, error) {
	var rs []*types.Receipt
	rs, ok := l.blockReceiptsCache.Get(height)
	if !ok {
		rsBytes, err := l.bf.Get(blockfile.BlockFileReceiptsTable, height)
		if err != nil {
			return nil, fmt.Errorf("get receipts with height %d from blockfile failed: %w", height, err)
		}

		return types.UnmarshalReceipts(rsBytes)
	}
	return rs, nil
}

// PersistExecutionResult persist the execution result
func (l *ChainLedgerImpl) PersistExecutionResult(block *types.Block, receipts []*types.Receipt) error {
	if block == nil {
		return errors.New("empty persist block data")
	}
	return l.doBatchPersistExecutionResult([]*types.Block{block}, [][]*types.Receipt{receipts})
}

func (l *ChainLedgerImpl) BatchPersistExecutionResult(batchBlock []*types.Block, BatchReceipts [][]*types.Receipt) error {
	if len(batchBlock) == 0 {
		return errors.New("empty batch persist block data")
	}

	if len(batchBlock) != len(BatchReceipts) {
		return errors.New("batch persist execution result param's length not match")
	}
	return l.doBatchPersistExecutionResult(batchBlock, BatchReceipts)
}

func (l *ChainLedgerImpl) doBatchPersistExecutionResult(batchBlock []*types.Block, batchReceipts [][]*types.Receipt) error {
	current := time.Now()
	if len(batchBlock) == 0 {
		return errors.New("empty doBatch persist block data")
	}

	if len(batchBlock) != len(batchReceipts) {
		return errors.New("doBatch persist execution result param's length not match")
	}

	batcher := l.blockchainStore.NewBatch()
	var listOfHash, listOfHeader, listOfExtra, listOfReceipts, listOfTransactions [][]byte
	for i, block := range batchBlock {
		listOfHash = append(listOfHash, block.Hash().Bytes())

		receipts := batchReceipts[i]
		rs, err := l.prepareReceipts(batcher, block, receipts)
		if err != nil {
			return fmt.Errorf("preapare receipts failed: %w", err)
		}
		listOfReceipts = append(listOfReceipts, rs)
		ts, err := l.prepareTransactions(batcher, block)
		if err != nil {
			return fmt.Errorf("prepare transactions failed: %w", err)
		}
		listOfTransactions = append(listOfTransactions, ts)

		h, e, err := l.prepareBlock(batcher, block)
		if err != nil {
			return fmt.Errorf("prepare block failed: %w", err)
		}
		listOfHeader = append(listOfHeader, h)
		listOfExtra = append(listOfExtra, e)
	}

	lastBlock := batchBlock[len(batchBlock)-1]
	beginBlock := batchBlock[0]

	if err := l.bf.BatchAppendBlock(beginBlock.Header.Number, listOfHash, listOfHeader, listOfExtra, listOfReceipts, listOfTransactions); err != nil {
		return fmt.Errorf("append block with height %d to blockfile failed: %w", lastBlock.Header.Number, err)
	}

	meta := &types.ChainMeta{
		Height:    lastBlock.Header.Number,
		BlockHash: lastBlock.Hash(),
	}

	l.logger.WithFields(logrus.Fields{
		"Height":    meta.Height,
		"BlockHash": meta.BlockHash,
	}).Debug("prepare chain meta")

	if err := l.persistChainMeta(batcher, meta); err != nil {
		return fmt.Errorf("persist chain meta failed: %w", err)
	}

	batcher.Commit()
	for i, block := range batchBlock {
		if len(block.Transactions) > 0 {
			l.blockTxsCache.Add(block.Header.Number, block.Transactions)
		}
		l.blockReceiptsCache.Add(block.Header.Number, batchReceipts[i])
		l.blockHeaderCache.Add(block.Header.Number, block.Header)
		l.blockExtraCache.Add(block.Header.Number, block.Extra)
	}

	l.UpdateChainMeta(meta)

	l.logger.WithField("time", time.Since(current)).Debug("persist execution result elapsed")
	return nil
}

// UpdateChainMeta update the chain meta data
func (l *ChainLedgerImpl) UpdateChainMeta(meta *types.ChainMeta) {
	l.chainMeta.Height = meta.Height
	l.chainMeta.BlockHash = meta.BlockHash
}

// GetChainMeta get chain meta data
func (l *ChainLedgerImpl) GetChainMeta() *types.ChainMeta {
	return &types.ChainMeta{
		Height:    l.chainMeta.Height,
		BlockHash: l.chainMeta.BlockHash,
	}
}

// LoadChainMeta load chain meta data
func (l *ChainLedgerImpl) LoadChainMeta() (*types.ChainMeta, error) {
	ok := l.blockchainStore.Has([]byte(utils.ChainMetaKey))

	chain := &types.ChainMeta{}
	if ok {
		body := l.blockchainStore.Get([]byte(utils.ChainMetaKey))
		if err := chain.Unmarshal(body); err != nil {
			return nil, fmt.Errorf("unmarshal chain meta: %w", err)
		}
	}

	return chain, nil
}

func (l *ChainLedgerImpl) prepareReceipts(_ kv.Batch, _ *types.Block, receipts []*types.Receipt) ([]byte, error) {
	return types.MarshalReceipts(receipts)
}

func (l *ChainLedgerImpl) prepareTransactions(batcher kv.Batch, block *types.Block) ([]byte, error) {
	for i, tx := range block.Transactions {
		meta := &types.TransactionMeta{
			BlockHeight: block.Header.Number,
			BlockHash:   block.Hash(),
			Index:       uint64(i),
		}

		metaBytes, err := meta.Marshal()
		if err != nil {
			return nil, fmt.Errorf("marshal tx meta error: %s", err)
		}

		batcher.Put(utils.CompositeKey(utils.TransactionMetaKey, tx.GetHash().String()), metaBytes)
	}

	return types.MarshalTransactions(block.Transactions)
}

func (l *ChainLedgerImpl) prepareBlock(batcher kv.Batch, block *types.Block) (header []byte, extra []byte, err error) {
	if block.Extra == nil {
		block.Extra = &types.BlockExtra{}
	}
	block.Extra.Size = block.CalculateSize()
	header, err = block.Header.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal block header error: %w", err)
	}
	extra, err = block.Extra.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal block extra error: %w", err)
	}

	height := block.Header.Number

	var txHashes []*types.Hash
	for _, tx := range block.Transactions {
		txHashes = append(txHashes, tx.GetHash())
	}

	data, err := json.Marshal(txHashes)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal tx hash error: %w", err)
	}

	batcher.Put(utils.CompositeKey(utils.BlockTxSetKey, height), data)

	hash := block.Hash().String()
	batcher.Put(utils.CompositeKey(utils.BlockHashKey, hash), []byte(fmt.Sprintf("%d", height)))

	return header, extra, nil
}

func (l *ChainLedgerImpl) persistChainMeta(batcher kv.Batch, meta *types.ChainMeta) error {
	data, err := meta.Marshal()
	if err != nil {
		return fmt.Errorf("marshal chain meta error: %w", err)
	}

	batcher.Put([]byte(utils.ChainMetaKey), data)

	return nil
}

func (l *ChainLedgerImpl) removeChainDataOnBlock(batch kv.Batch, height uint64) error {
	block, err := l.GetBlock(height)
	if err != nil {
		return fmt.Errorf("get block with height %d failed: %w", height, err)
	}

	if err := l.bf.TruncateBlocks(height - 1); err != nil {
		return fmt.Errorf("truncate blocks failed: %w", err)
	}

	batch.Delete(utils.CompositeKey(utils.BlockTxSetKey, height))
	batch.Delete(utils.CompositeKey(utils.BlockHashKey, block.Hash().String()))

	for _, tx := range block.Transactions {
		batch.Delete(utils.CompositeKey(utils.TransactionMetaKey, tx.GetHash().String()))
	}

	l.blockTxsCache.Remove(height)
	l.blockReceiptsCache.Remove(height)
	l.blockHeaderCache.Remove(height)
	l.blockExtraCache.Remove(height)
	return nil
}

func (l *ChainLedgerImpl) RollbackBlockChain(height uint64) error {
	meta := l.GetChainMeta()

	if meta.Height < height {
		return ErrorRollbackToHigherNumber
	}

	if meta.Height == height || meta.BlockHash == nil {
		return nil
	}

	batch := l.blockchainStore.NewBatch()

	for i := meta.Height; i > height; i-- {
		err := l.removeChainDataOnBlock(batch, i)
		if err != nil {
			return fmt.Errorf("remove chain data on block %d failed: %w", i, err)
		}
		if batch.Size() > maxBatchSize {
			batch.Commit()
			batch.Reset()
			l.logger.Infof("[RollbackBlockChain] write batch periodically")
		}
	}

	block, err := l.GetBlock(height)
	if err != nil {
		return fmt.Errorf("get block with height %d failed: %w", height, err)
	}
	meta = &types.ChainMeta{
		Height:    block.Header.Number,
		BlockHash: block.Hash(),
	}

	if err := l.persistChainMeta(batch, meta); err != nil {
		return fmt.Errorf("persist chain meta failed: %w", err)
	}

	batch.Commit()

	l.UpdateChainMeta(meta)

	return nil
}

func (l *ChainLedgerImpl) checkChainMeta() error {
	bfNextBlockNumber := l.bf.NextBlockNumber()
	// no block
	if bfNextBlockNumber == 0 {
		if l.chainMeta.BlockHash != nil {
			panic("illegal chain meta!")
		}
		return nil
	}
	if l.chainMeta.Height < bfNextBlockNumber-1 {
		l.chainMeta.Height = bfNextBlockNumber - 1
		batcher := l.blockchainStore.NewBatch()

		currentBlock, err := l.GetBlock(l.chainMeta.Height)
		if err != nil {
			return fmt.Errorf("get blockfile block: %w", err)
		}

		_, err = l.prepareTransactions(batcher, currentBlock)
		if err != nil {
			return err
		}
		_, _, err = l.prepareBlock(batcher, currentBlock)
		if err != nil {
			return err
		}

		l.chainMeta.BlockHash = currentBlock.Hash()
		if err := l.persistChainMeta(batcher, l.chainMeta); err != nil {
			return fmt.Errorf("update chain meta: %w", err)
		}

		batcher.Commit()
	}

	return nil
}

func (l *ChainLedgerImpl) Close() {
	_ = l.blockchainStore.Close()
	_ = l.bf.Close()
}

func (l *ChainLedgerImpl) CloseBlockfile() {
	_ = l.bf.Close()
}
