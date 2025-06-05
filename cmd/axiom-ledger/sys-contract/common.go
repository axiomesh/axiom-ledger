package sys_contract

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var rpc = "http://127.0.0.1:8881"

var rpcFlag = &cli.StringFlag{
	Name:        "rpc",
	Aliases:     []string{"r"},
	Destination: &rpc,
	Usage:       "rpc server addr",
	Required:    false,
	DefaultText: "http://127.0.0.1:8881",
}

var sender = "b6477143e17f889263044f6cf463dc37177ac4526c4c39a7a344198457024a2f"

var senderFlag = &cli.StringFlag{
	Name:        "sender",
	Aliases:     []string{"s"},
	Destination: &sender,
	Usage:       "sender private key",
	Required:    false,
	DefaultText: "b6477143e17f889263044f6cf463dc37177ac4526c4c39a7a344198457024a2f",
}

func SendAndWaitTx(ctx *cli.Context, client *ethclient.Client, txBuilder func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error), logsParser func(receipt *types.Receipt) error) error {
	chainID, err := client.ChainID(ctx.Context)
	if err != nil {
		return errors.Wrap(err, "get chain id failed")
	}
	signer := types.NewCancunSigner(chainID)

	sk, err := ethcrypto.HexToECDSA(strings.TrimPrefix(sender, "0x"))
	if err != nil {
		return errors.Wrap(err, "decode sender private key error")
	}

	tx, err := txBuilder(client, &bind.TransactOpts{
		From: ethcrypto.PubkeyToAddress(sk.PublicKey),
		Signer: func(address ethcommon.Address, transaction *types.Transaction) (*types.Transaction, error) {
			return types.SignTx(transaction, signer, sk)
		},
		Context: ctx.Context,
		NoSend:  true,
	})
	if err != nil {
		return errors.Wrap(err, "build tx failed")
	}
	fmt.Printf("build tx hash: %s\n", tx.Hash().Hex())

	if err := client.SendTransaction(ctx.Context, tx); err != nil {
		return errors.Wrap(err, "send tx failed")
	}
	fmt.Println("wait tx confirmed...")
	receipt, err := WaitTxConfirmed(ctx.Context, client, tx.Hash())
	if err != nil {
		return errors.Wrap(err, "wait tx confirmed failed")
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return errors.New("tx confirmed, but execution failed")
	}
	fmt.Println("tx confirmed")
	if logsParser != nil {
		if err := logsParser(receipt); err != nil {
			return errors.Wrap(err, "parse logs failed")
		}
	}
	return nil
}

func WaitTxConfirmed(ctx context.Context, client *ethclient.Client, txHash ethcommon.Hash) (*types.Receipt, error) {
	queryTicker := time.NewTicker(1 * time.Second)
	defer queryTicker.Stop()

	for {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}

// GetSenderAddress returns the sender address from the private key.
func GetSenderAddress() (ethcommon.Address, error) {
	sk, err := ethcrypto.HexToECDSA(strings.TrimPrefix(sender, "0x"))
	if err != nil {
		return ethcommon.Address{}, errors.Wrap(err, "decode sender private key error")
	}
	return ethcrypto.PubkeyToAddress(sk.PublicKey), nil
}
