package sys_contract

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-ledger/cmd/draconis/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/liquid_staking_token_client"
)

var StakingLSTCMDBalanceOfArgs = struct {
	Owner string
}{}

var StakingLSTCMDOwnerOfArgs = struct {
	LSTID string
}{}

var StakingLSTCMDSafeTransferFromArgs = struct {
	From  string
	To    string
	LSTID string
}{}

var StakingLSTCMDTransferFromArgs = struct {
	From  string
	To    string
	LSTID string
}{}

var StakingLSTCMDApproveArgs = struct {
	To    string
	LSTID string
}{}

var StakingLSTCMDSetApprovalForAllArgs = struct {
	Operator string
	Approved bool
}{}

var StakingLSTCMDGetApprovedArgs = struct {
	LSTID string
}{}

var StakingLSTCMDIsApprovedForAllArgs = struct {
	Owner    string
	Operator string
}{}

var StakingLSTCMDInfoArgs = struct {
	LSTID string
}{}

var StakingLSTCMDLockedRewardArgs = struct {
	LSTID string
}{}

var StakingLSTCMDUnlockingCoinArgs = struct {
	LSTID string
}{}

var StakingLSTCMDUnlockedCoinArgs = struct {
	LSTID string
}{}

var StakingLSTCMDLockedCoinArgs = struct {
	LSTID string
}{}

var StakingLSTCMDTotalCoinArgs = struct {
	LSTID string
}{}

var StakingLSTCMD = &cli.Command{
	Name:  "staking-lst",
	Usage: "The staking lst manage commands",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "balance-of",
			Usage:  "Get balance of owner",
			Action: StakingLSTActions{}.info,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "owner",
					Destination: &StakingLSTCMDBalanceOfArgs.Owner,
					Required:    true,
				},
			},
		},
		{
			Name:   "owner-of",
			Usage:  "Get lst owner",
			Action: StakingLSTActions{}.ownerOf,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDOwnerOfArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "safeTransferFrom",
			Usage:  "Safe transfer from",
			Action: StakingLSTActions{}.safeTransferFrom,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDSafeTransferFromArgs.LSTID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "from",
					Destination: &StakingLSTCMDSafeTransferFromArgs.From,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "to",
					Destination: &StakingLSTCMDSafeTransferFromArgs.To,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "transferFrom",
			Usage:  "Transfer from",
			Action: StakingLSTActions{}.transferFrom,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDTransferFromArgs.LSTID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "from",
					Destination: &StakingLSTCMDTransferFromArgs.From,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "to",
					Destination: &StakingLSTCMDTransferFromArgs.To,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "approve",
			Usage:  "Approve",
			Action: StakingLSTActions{}.approve,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "to",
					Destination: &StakingLSTCMDApproveArgs.To,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDApproveArgs.LSTID,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "set-approval-for-all",
			Usage:  "Set Approval for all",
			Action: StakingLSTActions{}.setApprovalForAll,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "operator",
					Destination: &StakingLSTCMDSetApprovalForAllArgs.Operator,
					Required:    true,
				},
				&cli.BoolFlag{
					Name:        "approved",
					Destination: &StakingLSTCMDSetApprovalForAllArgs.Approved,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "get-approved",
			Usage:  "Get lst approved",
			Action: StakingLSTActions{}.getApproved,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDGetApprovedArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "is-approved-for-all",
			Usage:  "Get is approved for all",
			Action: StakingLSTActions{}.isApprovedForAll,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "owner",
					Destination: &StakingLSTCMDIsApprovedForAllArgs.Owner,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "operator",
					Destination: &StakingLSTCMDIsApprovedForAllArgs.Operator,
					Required:    true,
				},
			},
		},
		{
			Name:   "info",
			Usage:  "Get lst info",
			Action: StakingLSTActions{}.info,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDInfoArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "locked-reward",
			Usage:  "Get lst locked reward",
			Action: StakingLSTActions{}.lockedReward,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDLockedRewardArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "unlocking-coin",
			Usage:  "Get lst unlocking coin(can withdraw until unlock time arrived)",
			Action: StakingLSTActions{}.unlockingCoin,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDUnlockingCoinArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "unlocked-coin",
			Usage:  "Get lst unlocked coin(can withdraw directly)",
			Action: StakingLSTActions{}.unlockedCoin,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDUnlockedCoinArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "locked-coin",
			Usage:  "Get lst locked coin(principal + reward)",
			Action: StakingLSTActions{}.lockedCoin,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDLockedCoinArgs.LSTID,
					Required:    true,
				},
			},
		},
		{
			Name:   "total-coin",
			Usage:  "Get lst total coin(principal + reward + unlocked + unlocking)",
			Action: StakingLSTActions{}.totalCoin,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingLSTCMDTotalCoinArgs.LSTID,
					Required:    true,
				},
			},
		},
	},
}

type StakingLSTActions struct {
}

func (_ StakingLSTActions) bindingContract(ctx *cli.Context) (*liquid_staking_token_client.BindingContract, *ethclient.Client, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "dial rpc failed")
	}

	contract, err := liquid_staking_token_client.NewBindingContract(ethcommon.HexToAddress(syscommon.LiquidStakingTokenContractAddr), client)
	if err != nil {
		return nil, nil, errors.Wrap(err, "bind staking contract failed")
	}
	return contract, client, nil
}

func (a StakingLSTActions) balanceOf(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	if !ethcommon.IsHexAddress(StakingLSTCMDBalanceOfArgs.Owner) {
		return errors.New("invalid owner address")
	}
	owner := ethcommon.HexToAddress(StakingLSTCMDBalanceOfArgs.Owner)

	res, err := lstContract.BalanceOf(&bind.CallOpts{Context: ctx.Context}, owner)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) ownerOf(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDOwnerOfArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDOwnerOfArgs.LSTID)
	}

	res, err := lstContract.OwnerOf(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) safeTransferFrom(ctx *cli.Context) error {
	lstContract, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDSafeTransferFromArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDSafeTransferFromArgs.LSTID)
	}
	if !ethcommon.IsHexAddress(StakingLSTCMDSafeTransferFromArgs.From) {
		return errors.New("invalid from address")
	}
	from := ethcommon.HexToAddress(StakingLSTCMDSafeTransferFromArgs.From)
	if !ethcommon.IsHexAddress(StakingLSTCMDSafeTransferFromArgs.To) {
		return errors.New("invalid to address")
	}
	to := ethcommon.HexToAddress(StakingLSTCMDSafeTransferFromArgs.To)

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return lstContract.SafeTransferFrom(opts, from, to, lstID)
	}, nil)
}

func (a StakingLSTActions) transferFrom(ctx *cli.Context) error {
	lstContract, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDTransferFromArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDTransferFromArgs.LSTID)
	}
	if !ethcommon.IsHexAddress(StakingLSTCMDTransferFromArgs.From) {
		return errors.New("invalid from address")
	}
	from := ethcommon.HexToAddress(StakingLSTCMDTransferFromArgs.From)
	if !ethcommon.IsHexAddress(StakingLSTCMDTransferFromArgs.To) {
		return errors.New("invalid to address")
	}
	to := ethcommon.HexToAddress(StakingLSTCMDTransferFromArgs.To)

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return lstContract.SafeTransferFrom(opts, from, to, lstID)
	}, nil)
}

func (a StakingLSTActions) approve(ctx *cli.Context) error {
	lstContract, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDApproveArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDApproveArgs.LSTID)
	}
	if !ethcommon.IsHexAddress(StakingLSTCMDApproveArgs.To) {
		return errors.New("invalid to address")
	}
	to := ethcommon.HexToAddress(StakingLSTCMDApproveArgs.To)

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return lstContract.Approve(opts, to, lstID)
	}, nil)
}

func (a StakingLSTActions) setApprovalForAll(ctx *cli.Context) error {
	lstContract, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}

	if !ethcommon.IsHexAddress(StakingLSTCMDSetApprovalForAllArgs.Operator) {
		return errors.New("invalid operator address")
	}
	operator := ethcommon.HexToAddress(StakingLSTCMDSetApprovalForAllArgs.Operator)

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return lstContract.SetApprovalForAll(opts, operator, StakingLSTCMDSetApprovalForAllArgs.Approved)
	}, nil)
}

func (a StakingLSTActions) getApproved(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDOwnerOfArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDOwnerOfArgs.LSTID)
	}

	res, err := lstContract.GetApproved(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) isApprovedForAll(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	if !ethcommon.IsHexAddress(StakingLSTCMDIsApprovedForAllArgs.Owner) {
		return errors.New("invalid owner address")
	}
	owner := ethcommon.HexToAddress(StakingLSTCMDIsApprovedForAllArgs.Owner)
	if !ethcommon.IsHexAddress(StakingLSTCMDIsApprovedForAllArgs.Operator) {
		return errors.New("invalid operator address")
	}
	operator := ethcommon.HexToAddress(StakingLSTCMDIsApprovedForAllArgs.Operator)

	res, err := lstContract.IsApprovedForAll(&bind.CallOpts{Context: ctx.Context}, owner, operator)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) info(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDInfoArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDInfoArgs.LSTID)
	}

	res, err := lstContract.GetInfo(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) lockedReward(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDLockedRewardArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDLockedRewardArgs.LSTID)
	}

	res, err := lstContract.GetLockedReward(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) unlockingCoin(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDUnlockingCoinArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDUnlockingCoinArgs.LSTID)
	}

	res, err := lstContract.GetUnlockingCoin(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) unlockedCoin(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDUnlockedCoinArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDUnlockedCoinArgs.LSTID)
	}

	res, err := lstContract.GetUnlockedCoin(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) lockedCoin(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDLockedCoinArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDLockedCoinArgs.LSTID)
	}

	res, err := lstContract.GetLockedCoin(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingLSTActions) totalCoin(ctx *cli.Context) error {
	lstContract, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	lstID, ok := new(big.Int).SetString(StakingLSTCMDTotalCoinArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingLSTCMDTotalCoinArgs.LSTID)
	}

	res, err := lstContract.GetTotalCoin(&bind.CallOpts{Context: ctx.Context}, lstID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}
