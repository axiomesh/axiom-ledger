package main

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/sol/binding"
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

var historyEpochArgs = struct {
	Epoch uint64
}{}

var epochCMD = &cli.Command{
	Name:  "epoch",
	Usage: "The epoch manage commands",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "current",
			Usage:  "Get current epoch info",
			Action: getCurrentEpoch,
		},
		{
			Name:   "next",
			Usage:  "Get next epoch info",
			Action: getNextEpoch,
		},
		{
			Name:  "history",
			Usage: "Get history epoch info",
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "epoch",
					Aliases:     []string{"e"},
					Destination: &historyEpochArgs.Epoch,
					Usage:       "history epoch number",
					DefaultText: "1",
					Required:    true,
				},
			},
			Action: getHistoryEpoch,
		},
	},
}

func bindEpochManagerContract(ctx *cli.Context) (*binding.EpochManager, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, errors.Wrap(err, "dial rpc failed")
	}

	epochManager, err := binding.NewEpochManager(ethcommon.HexToAddress(common.EpochManagerContractAddr), client)
	if err != nil {
		return nil, errors.Wrap(err, "bind epoch manager contract failed")
	}
	return epochManager, nil
}

func getCurrentEpoch(ctx *cli.Context) error {
	epochManager, err := bindEpochManagerContract(ctx)
	if err != nil {
		return err
	}
	epochInfo, err := epochManager.CurrentEpoch(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return errors.Wrap(err, "get current epoch failed")
	}
	return pretty(epochInfo)
}

func getNextEpoch(ctx *cli.Context) error {
	epochManager, err := bindEpochManagerContract(ctx)
	if err != nil {
		return err
	}
	epochInfo, err := epochManager.NextEpoch(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return errors.Wrap(err, "get next epoch failed")
	}
	return pretty(epochInfo)
}

func getHistoryEpoch(ctx *cli.Context) error {
	if !ctx.IsSet("epoch") {
		historyEpochArgs.Epoch = 1
	}

	epochManager, err := bindEpochManagerContract(ctx)
	if err != nil {
		return err
	}
	epochInfo, err := epochManager.HistoryEpoch(&bind.CallOpts{Context: ctx.Context}, historyEpochArgs.Epoch)
	if err != nil {
		return errors.Wrap(err, "get history epoch failed")
	}
	return pretty(epochInfo)
}
