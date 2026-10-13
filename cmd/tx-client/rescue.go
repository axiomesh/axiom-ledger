package main

// rescue.go — 单笔交易 emergency EVM storage write
//
// 用途：通过 governance.propose(RescueState=7) 强制写 pair 合约任意 storage slot。
// 配合 RescueManager.Propose override（见 internal/executor/system/governance/rescue_manager.go），
// 提案在同笔 tx 内自动 Approved 并执行 Execute，不需走 vote 流程。
//
// 用法：
//   tx-client rescue \
//     --rpc-url  http://127.0.0.1:8881 \
//     --private-key 0x... \
//     --target 0xPAIR \
//     --slot   12 \
//     --value  0x0000000000000000000000000000000000000000000000000000000000000001

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// governanceRescueABI is the minimal subset of the Governance system
// contract ABI that the rescue subcommand needs: propose(...) and
// getLatestProposalID(). Inlined so this file has no dependency on
// project-internal abigen bindings.
const governanceRescueABI = `[
  {"inputs":[],"name":"getLatestProposalID","outputs":[{"internalType":"uint64","name":"","type":"uint64"}],"stateMutability":"view","type":"function"},
  {"inputs":[{"internalType":"enumProposalType","name":"proposalType","type":"uint8"},{"internalType":"string","name":"title","type":"string"},{"internalType":"string","name":"desc","type":"string"},{"internalType":"uint64","name":"blockNumber","type":"uint64"},{"internalType":"bytes","name":"extra","type":"bytes"}],"name":"propose","outputs":[],"stateMutability":"nonpayable","type":"function"}
]`

// RescueExtraArgs is the JSON payload the on-chain RescueManager
// expects. MUST stay in sync with internal/executor/system/governance/rescue_manager.go.
type RescueExtraArgs struct {
	Target string `json:"target"`
	Slot   uint64 `json:"slot"`
	Value  string `json:"value"`
}

const (
	// rescueState is the proposal type number registered in
	// internal/executor/system/governance (last entry of the iota
	// block). MUST stay in sync with that file.
	rescueState uint8 = 7
)

var rescueArgs = struct {
	RpcURL          string
	ContractAddress string
	PrivateKey      string
	Target          string
	Slot            uint64
	Value           string
	Title           string
	Description     string
	Duration        uint64
	GasPrice        uint64
	GasLimit        uint64
	Verbose         bool
}{
	RpcURL:          "http://127.0.0.1:8881",
	ContractAddress: "0x0000000000000000000000000000000000001001", // 默认治理合约地址
	PrivateKey:      "",
	Target:          "",
	Slot:            0,
	Value:           "",
	Title:           "rescue",
	Description:     "rescue forced storage write",
	Duration:        1024,           // blocks, relative; will be added to current head
	GasPrice:        10000000000000, // 10 Gwei
	GasLimit:        500000,
	Verbose:         false,
}

var rescueCMD = &cli.Command{
	Name:      "rescue",
	Usage:     "单笔交易 emergency EVM storage write（治理 rescue 提案，自动 approve + execute）",
	ArgsUsage: "--target 0xPAIR --slot N --value 0x...32bytes",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "rpc-url",
			Usage:       "axiom-ledger JSON-RPC URL",
			Value:       rescueArgs.RpcURL,
			Destination: &rescueArgs.RpcURL,
		},
		&cli.StringFlag{
			Name:        "contract",
			Usage:       "governance system contract address",
			Value:       rescueArgs.ContractAddress,
			Destination: &rescueArgs.ContractAddress,
		},
		&cli.StringFlag{
			Name:        "private-key",
			Usage:       "operator 私钥 (0x-prefixed 64 hex chars)",
			Required:    true,
			Destination: &rescueArgs.PrivateKey,
		},
		&cli.StringFlag{
			Name:        "target",
			Usage:       "目标合约地址 (0x...)",
			Required:    true,
			Destination: &rescueArgs.Target,
		},
		&cli.Uint64Flag{
			Name:        "slot",
			Usage:       "要写的 storage slot 编号",
			Required:    true,
			Destination: &rescueArgs.Slot,
		},
		&cli.StringFlag{
			Name:        "value",
			Usage:       "新值 (0x-prefixed 64 hex chars = 32 bytes)",
			Required:    true,
			Destination: &rescueArgs.Value,
		},
		&cli.StringFlag{
			Name:        "title",
			Usage:       "proposal title",
			Value:       rescueArgs.Title,
			Destination: &rescueArgs.Title,
		},
		&cli.StringFlag{
			Name:        "description",
			Usage:       "proposal description",
			Value:       rescueArgs.Description,
			Destination: &rescueArgs.Description,
		},
		&cli.Uint64Flag{
			Name:        "duration",
			Usage:       "提案有效窗口（块数，会加上当前 head）",
			Value:       rescueArgs.Duration,
			Destination: &rescueArgs.Duration,
		},
		&cli.Uint64Flag{
			Name:        "gas-price",
			Usage:       "gas price (wei)",
			Value:       rescueArgs.GasPrice,
			Destination: &rescueArgs.GasPrice,
		},
		&cli.Uint64Flag{
			Name:        "gas-limit",
			Usage:       "gas limit",
			Value:       rescueArgs.GasLimit,
			Destination: &rescueArgs.GasLimit,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "enable debug logging",
			Value:       rescueArgs.Verbose,
			Destination: &rescueArgs.Verbose,
		},
	},
	Action: func(ctx *cli.Context) error {
		log := logrus.New()
		log.SetLevel(logrus.InfoLevel)
		if rescueArgs.Verbose {
			log.SetLevel(logrus.DebugLevel)
		}
		return runRescue(rescueArgs.RpcURL, rescueArgs.ContractAddress, rescueArgs.PrivateKey,
			rescueArgs.Target, rescueArgs.Slot, rescueArgs.Value,
			rescueArgs.Title, rescueArgs.Description, rescueArgs.Duration,
			rescueArgs.GasPrice, rescueArgs.GasLimit, log)
	},
}

func runRescue(rpcURL, contractHex, privKeyHex, target string, slot uint64, valueHex,
	title, desc string, duration, gasPrice, gasLimit uint64, log *logrus.Logger) error {

	logf := func(level logrus.Level, msg string, args ...interface{}) {
		log.Logf(level, msg, args...)
	}

	// 1) validate value (32-byte hex, exactly)
	raw, err := hex.DecodeString(strings.TrimPrefix(valueHex, "0x"))
	if err != nil {
		return errors.Wrap(err, "value must be hex")
	}
	if len(raw) != 32 {
		return errors.Errorf("value must decode to exactly 32 bytes, got %d", len(raw))
	}
	canonValue := valueHex
	if !strings.HasPrefix(canonValue, "0x") {
		canonValue = "0x" + canonValue
	}

	// 2) build extra = JSON of RescueExtraArgs
	extra, err := json.Marshal(RescueExtraArgs{
		Target: target,
		Slot:   slot,
		Value:  canonValue,
	})
	if err != nil {
		return errors.Wrap(err, "encode extra args")
	}

	// 3) dial RPC
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return errors.Wrap(err, "dial rpc")
	}
	defer client.Close()

	// 4) parse private key
	rawPK := strings.TrimPrefix(privKeyHex, "0x")
	if len(rawPK) != 64 {
		return errors.New("private key must be 64 hex chars")
	}
	pk, err := crypto.HexToECDSA(rawPK)
	if err != nil {
		return errors.Wrap(err, "parse private key")
	}
	operator := crypto.PubkeyToAddress(*pk.Public().(*ecdsa.PublicKey))
	logf(logrus.InfoLevel, "operator: %s", operator.Hex())

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return errors.Wrap(err, "get chain id")
	}

	// 5) bind governance contract from inline ABI
	parsedABI, err := abi.JSON(strings.NewReader(governanceRescueABI))
	if err != nil {
		return errors.Wrap(err, "parse inline governance ABI")
	}
	contractAddr := common.HexToAddress(contractHex)
	bound := bind.NewBoundContract(contractAddr, parsedABI, client, client, nil)

	// 6) fetch nonce + current head. The on-chain propose() takes
	//    an ABSOLUTE block number as the deadline, not a relative
	//    offset, so we MUST convert user-supplied duration into
	//    `head + duration` before signing — otherwise a long-running
	//    chain (head > 1e7) makes any small default duration
	//    immediately out of date and CheckProposeArgs reverts with
	//    ErrBlockNumberOutDate.
	nonce, err := client.PendingNonceAt(ctx, operator)
	if err != nil {
		return errors.Wrap(err, "get nonce")
	}
	head, err := client.BlockNumber(ctx)
	if err != nil {
		return errors.Wrap(err, "get head block number")
	}
	deadline := head + duration
	logf(logrus.InfoLevel, "head=%d duration=%d -> deadline=%d", head, duration, deadline)

	auth, err := bind.NewKeyedTransactorWithChainID(pk, chainID)
	if err != nil {
		return errors.Wrap(err, "build transactor")
	}
	auth.Nonce = new(big.Int).SetUint64(nonce)
	auth.GasPrice = new(big.Int).SetUint64(gasPrice)
	auth.GasLimit = gasLimit

	// 7) send propose tx
	logf(logrus.InfoLevel, "sending governance.propose(RescueState=%d, slot=%d)", rescueState, slot)
	tx, err := bound.Transact(auth, "propose", uint8(rescueState), title, desc, deadline, extra)
	if err != nil {
		return errors.Wrap(err, "send propose tx")
	}
	logf(logrus.InfoLevel, "tx hash: %s", tx.Hash().Hex())

	// 8) wait for it to be mined
	receipt, err := waitRescueMined(ctx, client, tx.Hash(), 60*time.Second, log)
	if err != nil {
		return err
	}
	logf(logrus.InfoLevel, "mined in block %d (status=%d)", receipt.BlockNumber.Uint64(), receipt.Status)
	if receipt.Status != 1 {
		return errors.Errorf("propose tx reverted (block %d)", receipt.BlockNumber.Uint64())
	}

	// 9) read back the latest proposal id so the operator sees
	//    the proposal number on the line. We pack the calldata
	//    ourselves and call via ethclient because BoundContract does
	//    not expose a generic CallContract helper.
	callData, err := parsedABI.Pack("getLatestProposalID")
	if err != nil {
		logf(logrus.WarnLevel, "pack getLatestProposalID: %v", err)
	} else {
		raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &contractAddr, Data: callData}, nil)
		if err != nil {
			logf(logrus.WarnLevel, "call getLatestProposalID: %v", err)
		} else {
			unpacked, err := parsedABI.Unpack("getLatestProposalID", raw)
			if err != nil || len(unpacked) == 0 {
				logf(logrus.WarnLevel, "unpack latest proposal id: %v", err)
			} else if v, ok := unpacked[0].(*big.Int); ok {
				logf(logrus.InfoLevel, "rescue proposal id: %s", v.String())
			}
		}
	}

	// The on-chain override has already run RescueManager.Execute in
	// the same block as propose. If we got here without an error from
	// Transact and the receipt status is 1, the forced write went
	// through. There is no separate "vote" step.
	logf(logrus.InfoLevel, "rescue complete — verify with eth_getStorageAt and eth_call(sync)")
	fmt.Println("done.")
	return nil
}

func waitRescueMined(ctx context.Context, client *ethclient.Client, hash common.Hash, timeout time.Duration, log *logrus.Logger) (*types.Receipt, error) {
	deadline := time.Now().Add(timeout)
	for {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err == nil {
			return receipt, nil
		}
		if !errors.Is(err, ethereum.NotFound) {
			// transient error; keep polling until timeout
			log.Debugf("waiting for receipt of %s: %v", hash.Hex(), err)
		}
		if time.Now().After(deadline) {
			return nil, errors.Wrapf(err, "timed out waiting for receipt of %s", hash.Hex())
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
