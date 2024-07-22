package eth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethhexutil "github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/api/jsonrpc/namespaces/eth/tracers"
	rpctypes "github.com/axiomesh/axiom-ledger/api/jsonrpc/types"
	"github.com/axiomesh/axiom-ledger/internal/coreapi/api"
	"github.com/axiomesh/axiom-ledger/internal/executor"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/utils"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

// BlockChain API provides an API for accessing blockchain data
type BlockChainAPI struct {
	ctx    context.Context
	cancel context.CancelFunc
	rep    *repo.Repo
	api    api.CoreAPI
	logger logrus.FieldLogger
}

func NewBlockChainAPI(rep *repo.Repo, api api.CoreAPI, logger logrus.FieldLogger) *BlockChainAPI {
	ctx, cancel := context.WithCancel(context.Background())
	return &BlockChainAPI{ctx: ctx, cancel: cancel, rep: rep, api: api, logger: logger}
}

// ChainId returns the chain's identifier in hex format
func (api *BlockChainAPI) ChainId() (ret ethhexutil.Uint, err error) { // nolint
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debug("eth_chainId")
	return ethhexutil.Uint(api.rep.GenesisConfig.ChainID), nil
}

// BlockNumber returns the current block number.
func (api *BlockChainAPI) BlockNumber() (ret ethhexutil.Uint64, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debug("eth_blockNumber")
	meta, err := api.api.Chain().Meta()
	if err != nil {
		return 0, err
	}

	return ethhexutil.Uint64(meta.Height), nil
}

// GetBalance returns the provided account's balance, blockNum is ignored.
func (api *BlockChainAPI) GetBalance(address common.Address, blockNrOrHash *rpctypes.BlockNumberOrHash) (ret *ethhexutil.Big, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_getBalance, address: %s, block number : %d", address.String())

	stateLedger, err := getStateLedgerAt(api.api, blockNrOrHash)
	if err != nil {
		return nil, err
	}

	balance := stateLedger.GetBalance(types.NewAddress(address.Bytes()))
	api.logger.Debugf("balance: %d", balance)

	return (*ethhexutil.Big)(balance), nil
}

type AccountResult struct {
	Address      common.Address    `json:"address"`
	AccountProof []string          `json:"accountProof"`
	Balance      *ethhexutil.Big   `json:"balance"`
	CodeHash     common.Hash       `json:"codeHash"`
	Nonce        ethhexutil.Uint64 `json:"nonce"`
	StorageHash  common.Hash       `json:"storageHash"`
	StorageProof []StorageResult   `json:"storageProof"`
}

type StorageResult struct {
	Key   string          `json:"key"`
	Value *ethhexutil.Big `json:"value"`
	Proof []string        `json:"proof"`
}

// GetProof returns the Merkle-proof for a given account and optionally some storage keys.
func (api *BlockChainAPI) GetProof(address common.Address, storageKeys []string, blockNrOrHash *rpctypes.BlockNumberOrHash) (ret *AccountResult, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	stateLedger, err := getStateLedgerAt(api.api, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	addr := types.NewAddress(address.Bytes())

	// construct account proof
	acc := stateLedger.GetOrCreateAccount(addr)
	rawAccountProof, err := stateLedger.Prove(common.Hash{}, utils.CompositeAccountKey(addr))
	if err != nil {
		return nil, err
	}
	var accountProof []string
	for _, proof := range rawAccountProof.Proof {
		accountProof = append(accountProof, base64.StdEncoding.EncodeToString(proof))
	}
	ret = &AccountResult{
		Address:      address,
		Nonce:        (ethhexutil.Uint64)(acc.GetNonce()),
		Balance:      (*ethhexutil.Big)(acc.GetBalance()),
		CodeHash:     common.BytesToHash(acc.CodeHash()),
		StorageHash:  acc.GetStorageRoot(),
		AccountProof: accountProof,
	}

	// construct storage proof
	keys := make([]common.Hash, len(storageKeys))
	for i, hexKey := range storageKeys {
		var err error
		keys[i], err = hexutil.DecodeHash(hexKey)
		if err != nil {
			return nil, err
		}
	}
	for _, key := range keys {
		hashKey := crypto.Keccak256(key.Bytes())
		rawStorageProof, err := stateLedger.Prove(acc.GetStorageRoot(), utils.CompositeStorageKey(addr, hashKey))
		if err != nil {
			return nil, err
		}
		var storageProof []string
		for _, proof := range rawStorageProof.Proof {
			storageProof = append(storageProof, base64.StdEncoding.EncodeToString(proof))
		}
		evmStateDB := &ledger.EvmStateDBAdaptor{StateLedger: stateLedger}
		storageResult := StorageResult{
			Key:   hexutil.Encode(key[:]),
			Value: (*ethhexutil.Big)(evmStateDB.GetState(addr.ETHAddress(), common.BytesToHash(hashKey)).Big()),
			Proof: storageProof,
		}
		ret.StorageProof = append(ret.StorageProof, storageResult)
	}

	return ret, nil
}

// GetBlockByNumber returns the block identified by number.
func (api *BlockChainAPI) GetBlockByNumber(blockNum rpctypes.BlockNumber, fullTx bool) (ret map[string]any, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_getBlockByNumber, number: %d, full: %v", blockNum, fullTx)

	if blockNum == rpctypes.PendingBlockNumber || blockNum == rpctypes.LatestBlockNumber {
		meta, err := api.api.Chain().Meta()
		if err != nil {
			return nil, err
		}
		blockNum = rpctypes.BlockNumber(meta.Height)
	}

	blockHeader, err := api.api.Broker().GetBlockHeaderByNumber(uint64(blockNum))
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return formatBlock(api.api, api.api.ChainState().GetCurrentEpochInfo(), blockHeader, fullTx)
}

// GetBlockByHash returns the block identified by hash.
func (api *BlockChainAPI) GetBlockByHash(hash common.Hash, fullTx bool) (ret map[string]any, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_getBlockByHash, hash: %s, full: %v", hash.String(), fullTx)

	blockHeader, err := api.api.Broker().GetBlockHeaderByHash(types.NewHash(hash.Bytes()))
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return formatBlock(api.api, api.api.ChainState().GetCurrentEpochInfo(), blockHeader, fullTx)
}

// GetCode returns the contract code at the given address, blockNum is ignored.
func (api *BlockChainAPI) GetCode(address common.Address, blockNrOrHash *rpctypes.BlockNumberOrHash) (ret ethhexutil.Bytes, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_getCode, address: %s", address.String())

	stateLedger, err := getStateLedgerAt(api.api, blockNrOrHash)
	if err != nil {
		return nil, err
	}

	code := stateLedger.GetCode(types.NewAddress(address.Bytes()))

	return code, nil
}

// GetStorageAt returns the contract storage at the given address and key, blockNum is ignored.
func (api *BlockChainAPI) GetStorageAt(address common.Address, key string, blockNrOrHash *rpctypes.BlockNumberOrHash) (ret ethhexutil.Bytes, err error) {
	defer func(start time.Time) {
		invokeReadOnlyDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_getStorageAt, address: %s, key: %s", address, key)

	stateLedger, err := getStateLedgerAt(api.api, blockNrOrHash)
	if err != nil {
		return nil, err
	}

	hash, err := hexutil.DecodeHash(key)
	if err != nil {
		return nil, err
	}

	ok, val := stateLedger.GetState(types.NewAddress(address.Bytes()), hash.Bytes())
	if !ok {
		return nil, nil
	}

	return val, nil
}

// Call performs a raw contract call.
func (api *BlockChainAPI) Call(args types.CallArgs, blockNrOrHash *rpctypes.BlockNumberOrHash, overrides *tracers.StateOverride, blockOverrides *tracers.BlockOverrides) (ret ethhexutil.Bytes, err error) {
	defer func(start time.Time) {
		invokeCallContractDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_call, args: %v, state overrides: %v, block overrides: %v", args, overrides, blockOverrides)

	receipt, err := DoCall(api.ctx, blockNrOrHash, overrides, blockOverrides, api.api, args, api.rep.Config.JsonRPC.EVMTimeout.ToDuration(), api.rep.Config.JsonRPC.GasCap, api.logger)
	if err != nil {
		return nil, err
	}
	api.logger.Debugf("receipt: %v", receipt)
	if len(receipt.Revert()) > 0 {
		return nil, newRevertError(receipt.Revert())
	}

	return receipt.Return(), receipt.Err
}

// DoCall todo call with historical ledger
func DoCall(ctx context.Context, blockNrOrHash *rpctypes.BlockNumberOrHash, overrides *tracers.StateOverride, blockOverrides *tracers.BlockOverrides, api api.CoreAPI, args types.CallArgs, timeout time.Duration, globalGasCap uint64, logger logrus.FieldLogger) (*core.ExecutionResult, error) {
	defer func(start time.Time) { logger.Debug("Executing EVM call finished", "runtime", time.Since(start)) }(time.Now())

	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// GET EVM Instance
	msg, err := executor.CallArgsToMessage(&args, globalGasCap, big.NewInt(0))
	if err != nil {
		return nil, err
	}

	// use copy state ledger to call
	stateLedger, err := getStateLedgerAt(api, blockNrOrHash)
	if err != nil {
		return nil, err
	}
	stateLedger.SetTxContext(types.NewHash([]byte("mockTx")), 0)
	if err := overrides.Apply(stateLedger); err != nil {
		return nil, err
	}

	evm, err := api.Broker().GetEvm(msg, &vm.Config{NoBaseFee: true})
	if err != nil {
		return nil, errors.New("error get evm")
	}
	blockCtx := evm.Context
	if blockOverrides != nil {
		blockOverrides.Apply(&blockCtx)
		evm.Context = blockCtx
	}

	go func() {
		<-ctx.Done()
		evm.Cancel()
	}()

	txContext := core.NewEVMTxContext(msg)
	evm.Reset(txContext, &ledger.EvmStateDBAdaptor{StateLedger: stateLedger})
	gp := new(core.GasPool).AddGas(math.MaxUint64)
	result, err := core.ApplyMessage(evm, msg, gp)

	// If the timer caused an abort, return an appropriate error message
	if evm.Cancelled() {
		return nil, fmt.Errorf("execution aborted (timeout = %v)", timeout)
	}
	if err != nil {
		// logger.Errorf("err: %w (supplied gas %d)", err, msg.GasLimit)
		return result, err
	}

	return result, nil
}

// EstimateGas returns an estimate of gas usage for the given smart contract call.
// It adds 2,000 gas to the returned value instead of using the gas adjustment
// param from the SDK.
func (api *BlockChainAPI) EstimateGas(args types.CallArgs, blockNrOrHash *rpctypes.BlockNumberOrHash) (ret ethhexutil.Uint64, err error) {
	defer func(start time.Time) {
		invokeCallContractDuration.Observe(time.Since(start).Seconds())
		queryTotalCounter.Inc()
		if err != nil {
			queryFailedCounter.Inc()
		}
	}(time.Now())

	api.logger.Debugf("eth_estimateGas, args: %s", args)

	// Determine the highest gas limit can be used during the estimation.
	// if args.Gas == nil || uint64(*args.Gas) < params.TxGas {
	// 	// Retrieve the block to act as the gas ceiling
	// 	args.Gas = (*ethhexutil.Uint64)(&api.rep.GasLimit)
	// }
	// Determine the lowest and highest possible gas limits to binary search in between
	var (
		lo  uint64 = params.TxGas - 1
		hi  uint64
		cap uint64
	)
	if args.Gas != nil && uint64(*args.Gas) >= params.TxGas {
		hi = uint64(*args.Gas)
	} else {
		// todo : api need get current epochInfo
		hi = api.rep.GenesisConfig.EpochInfo.FinanceParams.GasLimit
	}

	var feeCap *big.Int
	if args.GasPrice != nil && (args.MaxFeePerGas != nil || args.MaxPriorityFeePerGas != nil) {
		return 0, errors.New("both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified")
	} else if args.GasPrice != nil {
		feeCap = args.GasPrice.ToInt()
	} else if args.MaxFeePerGas != nil {
		feeCap = args.MaxFeePerGas.ToInt()
	} else {
		feeCap = common.Big0
	}
	if feeCap.BitLen() != 0 {
		stateLedger, err := getStateLedgerAt(api.api, blockNrOrHash)
		if err != nil {
			return 0, err
		}
		balance := stateLedger.GetBalance(types.NewAddress(args.From.Bytes()))
		api.logger.Debugf("balance: %d", balance)
		available := new(big.Int).Set(balance)
		if args.Value != nil {
			if args.Value.ToInt().Cmp(available) >= 0 {
				return 0, core.ErrInsufficientFundsForTransfer
			}
			available.Sub(available, args.Value.ToInt())
		}
		allowance := new(big.Int).Div(available, feeCap)
		if allowance.IsUint64() && hi > allowance.Uint64() {
			transfer := args.Value
			if transfer == nil {
				transfer = new(ethhexutil.Big)
			}
			api.logger.Warn("Gas estimation capped by limited funds", "original", hi, "balance", balance,
				"sent", transfer.ToInt(), "maxFeePerGas", feeCap, "fundable", allowance)
			hi = allowance.Uint64()
		}
	}

	gasCap := api.rep.Config.JsonRPC.GasCap
	if gasCap != 0 && hi > gasCap {
		api.logger.Warn("Caller gas above allowance, capping", "requested", hi, "cap", gasCap)
		hi = gasCap
	}

	cap = hi

	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(gas uint64) (bool, *core.ExecutionResult, error) {
		args.Gas = (*ethhexutil.Uint64)(&gas)

		t := hexutil.Uint64(time.Now().Unix())
		blockOveride := tracers.BlockOverrides{
			Time: &t,
		}
		result, err := DoCall(api.ctx, blockNrOrHash, nil, &blockOveride, api.api, args, api.rep.Config.JsonRPC.EVMTimeout.ToDuration(), api.rep.Config.JsonRPC.GasCap, api.logger)
		if err != nil {
			if errors.Is(err, core.ErrIntrinsicGas) {
				return true, nil, nil // Special case, raise gas limit
			}
			return true, nil, err
		}
		return result.Failed(), result, nil
	}

	// Execute the binary search and hone in on an executable gas limit
	for lo+1 < hi {
		mid := (hi + lo) / 2
		failed, _, err := executable(mid)
		if err != nil {
			return 0, err
		}
		if failed {
			lo = mid
		} else {
			hi = mid
		}
	}

	// Reject the transaction as invalid if it still fails at the highest allowance
	if hi == cap {
		failed, ret, err := executable(hi)
		if err != nil {
			return 0, err
		}
		if failed {
			if ret != nil && ret.Err != vm.ErrOutOfGas {
				if len(ret.Revert()) > 0 {
					return 0, newRevertError(ret.Revert())
				}
				return 0, ret.Err
			}
			return 0, fmt.Errorf("gas required exceeds allowance (%d)", cap)
		}
	}
	return ethhexutil.Uint64(hi), nil
}

// accessListResult returns an optional accesslist
// It's the result of the `debug_createAccessList` RPC call.
// It contains an error if the transaction itself failed.
type accessListResult struct {
	Accesslist *ethtypes.AccessList `json:"accessList"`
	Error      string               `json:"error,omitempty"`
	GasUsed    ethhexutil.Uint64    `json:"gasUsed"`
}

func (api *BlockChainAPI) CreateAccessList(args types.CallArgs, blockNrOrHash *rpctypes.BlockNumberOrHash) (*accessListResult, error) {
	return nil, ErrNotSupportApiError
}

// FormatBlock creates an ethereum block from a tendermint header and ethereum-formatted
// transactions.
func formatBlock(api api.CoreAPI, epochInfo types.EpochInfo, blockHeader *types.BlockHeader, fullTx bool) (map[string]any, error) {
	var err error
	var transactions []any
	if fullTx {
		txList, err := api.Broker().GetBlockTxList(blockHeader.Number)
		if err != nil {
			return nil, err
		}
		transactions = make([]any, len(txList))
		for i, tx := range txList {
			transactions[i] = NewRPCTransaction(tx, common.BytesToHash(blockHeader.Hash().Bytes()), blockHeader.Number, uint64(i))
		}
	} else {
		txHashList, err := api.Broker().GetBlockTxHashList(blockHeader.Number)
		if err != nil {
			return nil, err
		}
		transactions = make([]any, len(txHashList))
		for i, txHash := range txHashList {
			transactions[i] = txHash.ETHHash()
		}
	}

	blockExtra, err := api.Broker().GetBlockExtra(blockHeader.Number)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"number":           (*ethhexutil.Big)(big.NewInt(int64(blockHeader.Number))),
		"hash":             blockHeader.Hash().ETHHash(),
		"baseFeePerGas":    ethhexutil.Uint64(0),
		"parentHash":       blockHeader.ParentHash.ETHHash(),
		"nonce":            ethtypes.BlockNonce{}, // PoW specific
		"logsBloom":        blockHeader.Bloom.ETHBloom(),
		"transactionsRoot": blockHeader.TxRoot.ETHHash(),
		"stateRoot":        blockHeader.StateRoot.ETHHash(),
		"miner":            syscommon.StakingManagerContractAddr,
		"extraData":        ethhexutil.Bytes([]byte{}),
		"size":             ethhexutil.Uint64(blockExtra.Size),
		"gasLimit":         ethhexutil.Uint64(epochInfo.FinanceParams.GasLimit), // Static gas limit
		"gasUsed":          ethhexutil.Uint64(blockHeader.GasUsed),
		"timestamp":        ethhexutil.Uint64(blockHeader.Timestamp),
		"transactions":     transactions,
		"receiptsRoot":     blockHeader.ReceiptRoot.ETHHash(),
		"sha3Uncles":       ethtypes.EmptyUncleHash, // No uncles in rbft
		"uncles":           []string{},
		"mixHash":          common.Hash{},
		"difficulty":       (*ethhexutil.Big)(big.NewInt(0)),
	}, nil
}

// revertError is an API error that encompassas an EVM revertal with JSON error
// code and a binary data blob.
type revertError struct {
	error
	reason string // revert reason hex encoded
}

// ErrorCode returns the JSON error code for a revertal.
// See: https://github.com/ethereum/wiki/wiki/JSON-RPC-Error-Codes-Improvement-Proposal
func (e *revertError) ErrorCode() int {
	return 3
}

// ErrorData returns the hex encoded revert reason.
func (e *revertError) ErrorData() any {
	return e.reason
}

func newRevertError(data []byte) *revertError {
	reason, errUnpack := abi.UnpackRevert(data)
	err := errors.New("execution reverted")
	if errUnpack == nil {
		err = fmt.Errorf("execution reverted: %v", reason)
	}
	return &revertError{
		error:  err,
		reason: ethhexutil.Encode(data),
	}
}
