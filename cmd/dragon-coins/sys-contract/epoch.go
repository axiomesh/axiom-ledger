package sys_contract

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-ledger/cmd/dragon-coins/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/epoch_manager_client"
)

var EpochCMDHistoryEpochArgs = struct {
	Epoch uint64
}{}

var EpochCMD = &cli.Command{
	Name:  "epoch",
	Usage: "The epoch manage commands",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "current",
			Usage:  "Get current epoch info",
			Action: EpochActions{}.getCurrentEpoch,
		},
		{
			Name:   "next",
			Usage:  "Get next epoch info",
			Action: EpochActions{}.getNextEpoch,
		},
		{
			Name:  "history",
			Usage: "Get history epoch info",
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "epoch",
					Aliases:     []string{"e"},
					Destination: &EpochCMDHistoryEpochArgs.Epoch,
					Usage:       "history epoch number",
					DefaultText: "1",
					Required:    true,
				},
			},
			Action: EpochActions{}.getHistoryEpoch,
		},
	},
}

type EpochActions struct{}

func (a EpochActions) bindContract(ctx *cli.Context) (*epoch_manager_client.BindingContract, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, errors.Wrap(err, "dial rpc failed")
	}

	contract, err := epoch_manager_client.NewBindingContract(ethcommon.HexToAddress(syscommon.EpochManagerContractAddr), client)
	if err != nil {
		return nil, errors.Wrap(err, "bind epoch manager contract failed")
	}
	return contract, nil
}

func (a EpochActions) getCurrentEpoch(ctx *cli.Context) error {
	epochManager, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	epochInfo, err := epochManager.CurrentEpoch(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return errors.Wrap(err, "get current epoch failed")
	}
	return common.Pretty(epochInfo)
}

func (a EpochActions) getNextEpoch(ctx *cli.Context) error {
	epochManager, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	epochInfo, err := epochManager.NextEpoch(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return errors.Wrap(err, "get next epoch failed")
	}
	return common.Pretty(epochInfo)
}

func (a EpochActions) getHistoryEpoch(ctx *cli.Context) error {
	if !ctx.IsSet("epoch") {
		EpochCMDHistoryEpochArgs.Epoch = 1
	}

	epochManager, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	epochInfo, err := epochManager.HistoryEpoch(&bind.CallOpts{Context: ctx.Context}, EpochCMDHistoryEpochArgs.Epoch)
	if err != nil {
		return errors.Wrap(err, "get history epoch failed")
	}
	return common.Pretty(epochInfo)
}
