package sys_contract

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/cmd/draconis/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/staking_manager_client"
)

var StakingCMDAddStakeArgs = struct {
	PoolID uint64
	Owner  string
	Amount string
}{}

var StakingCMDUnlockStakeArgs = struct {
	LSTID  string
	Amount string
}{}

var StakingCMDWithdrawArgs = struct {
	LSTID     string
	Recipient string
	Amount    string
}{}

var StakingCMDUpdatePoolCommissionRateArgs = struct {
	PoolID            uint64
	NewCommissionRate uint64
}{}

var StakingCMDPoolInfoArgs = struct {
	PoolID uint64
}{}

var StakingCMDPoolHistoryLSTRateArgs = struct {
	PoolID uint64
	Epoch  uint64
}{}

var StakingCMD = &cli.Command{
	Name:  "staking",
	Usage: "The staking manage commands",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "add-stake",
			Usage:  "Add stake to a staking pool",
			Action: StakingActions{}.addStake,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "pool-id",
					Destination: &StakingCMDAddStakeArgs.PoolID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "owner",
					Destination: &StakingCMDAddStakeArgs.Owner,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "amount",
					Destination: &StakingCMDAddStakeArgs.Amount,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "unlock-stake",
			Usage:  "Unlock stake from a lst",
			Action: StakingActions{}.unlockStake,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingCMDUnlockStakeArgs.LSTID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "amount",
					Destination: &StakingCMDUnlockStakeArgs.Amount,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "withdraw",
			Usage:  "Withdraw from a lst",
			Action: StakingActions{}.withdraw,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "lst-id",
					Destination: &StakingCMDWithdrawArgs.LSTID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "recipient",
					Destination: &StakingCMDWithdrawArgs.Recipient,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "amount",
					Destination: &StakingCMDWithdrawArgs.Amount,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "update-pool-commission-rate",
			Usage:  "Update pool commission rate(must be called by node operator)",
			Action: StakingActions{}.updatePoolCommissionRate,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "pool-id",
					Destination: &StakingCMDUpdatePoolCommissionRateArgs.PoolID,
					Required:    true,
				},
				&cli.Uint64Flag{
					Name:        "new-commission-rate",
					Usage:       "new commission rate, range: 0-10000",
					Destination: &StakingCMDUpdatePoolCommissionRateArgs.NewCommissionRate,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "pool-info",
			Usage:  "Get pool info",
			Action: StakingActions{}.poolInfo,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "pool-id",
					Destination: &StakingCMDPoolInfoArgs.PoolID,
					Required:    true,
				},
			},
		},
		{
			Name:   "pool-history-lst-rate",
			Usage:  "Get pool history lst rate",
			Action: StakingActions{}.poolHistoryLSTRate,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "pool-id",
					Destination: &StakingCMDPoolHistoryLSTRateArgs.PoolID,
					Required:    true,
				},
				&cli.Uint64Flag{
					Name:        "epoch",
					Destination: &StakingCMDPoolHistoryLSTRateArgs.Epoch,
					Required:    true,
				},
			},
		},
	},
}

type StakingActions struct {
}

func (_ StakingActions) bindingContract(ctx *cli.Context) (*staking_manager_client.BindingContract, *ethclient.Client, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "dial rpc failed")
	}

	contract, err := staking_manager_client.NewBindingContract(ethcommon.HexToAddress(syscommon.StakingManagerContractAddr), client)
	if err != nil {
		return nil, nil, errors.Wrap(err, "bind staking contract failed")
	}
	return contract, client, nil
}

// function addStake(uint64 poolID, address owner, uint256 amount) external payable;
func (a StakingActions) addStake(ctx *cli.Context) error {
	stakingManager, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	amount, err := types.ParseCoinNumber(StakingCMDAddStakeArgs.Amount)
	if err != nil {
		return err
	}
	amountBigInt := amount.ToBigInt()
	if amountBigInt.Sign() == 0 {
		return errors.New("amount is zero")
	}
	if StakingCMDAddStakeArgs.Owner != "" {
		if !ethcommon.IsHexAddress(StakingCMDAddStakeArgs.Owner) {
			return errors.New("invalid owner address")
		}
	}
	owner := ethcommon.HexToAddress(StakingCMDAddStakeArgs.Owner)
	zero := ethcommon.Address{}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		if zero == owner {
			owner = opts.From
		}
		opts.Value = new(big.Int).Set(amountBigInt)
		return stakingManager.AddStake(opts, StakingCMDAddStakeArgs.PoolID, owner, amountBigInt)
	}, func(receipt *ethtypes.Receipt) error {
		if len(receipt.Logs) == 0 {
			return errors.New("no log")
		}
		addStakeEvent, err := stakingManager.ParseAddStake(*receipt.Logs[len(receipt.Logs)-1])
		if err != nil {
			return errors.Wrap(err, "parse log failed")
		}
		fmt.Println("add stake success, lst id:", addStakeEvent.LiquidStakingTokenID)
		return nil
	})
}

// function unlock(uint256 liquidStakingTokenID, uint256 amount) external;
func (a StakingActions) unlockStake(ctx *cli.Context) error {
	stakingManager, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	amount, err := types.ParseCoinNumber(StakingCMDUnlockStakeArgs.Amount)
	if err != nil {
		return err
	}
	amountBigInt := amount.ToBigInt()
	if amountBigInt.Sign() == 0 {
		return errors.New("amount is zero")
	}
	lstID, ok := new(big.Int).SetString(StakingCMDUnlockStakeArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingCMDUnlockStakeArgs.LSTID)
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return stakingManager.Unlock(opts, lstID, amountBigInt)
	}, func(receipt *ethtypes.Receipt) error {
		if len(receipt.Logs) == 0 {
			return errors.New("no log")
		}
		unlockStakeEvent, err := stakingManager.ParseUnlock(*receipt.Logs[len(receipt.Logs)-1])
		if err != nil {
			return errors.Wrap(err, "parse log failed")
		}
		unlockTime := time.Unix(int64(unlockStakeEvent.UnlockTimestamp), 0)
		fmt.Println("unlock stake success, unlock time:", unlockTime.Format("2006-01-02 15:04:05"))
		return nil
	})
}

// function withdraw(uint256 liquidStakingTokenID, address recipient, uint256 amount) external;
func (a StakingActions) withdraw(ctx *cli.Context) error {
	stakingManager, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	amount, err := types.ParseCoinNumber(StakingCMDWithdrawArgs.Amount)
	if err != nil {
		return err
	}
	amountBigInt := amount.ToBigInt()
	if amountBigInt.Sign() == 0 {
		return errors.New("amount is zero")
	}
	lstID, ok := new(big.Int).SetString(StakingCMDWithdrawArgs.LSTID, 10)
	if !ok {
		return errors.Errorf("invalid lst id: %s", StakingCMDWithdrawArgs.LSTID)
	}
	if StakingCMDWithdrawArgs.Recipient != "" {
		if !ethcommon.IsHexAddress(StakingCMDWithdrawArgs.Recipient) {
			return errors.New("invalid recipient address")
		}
	}
	recipient := ethcommon.HexToAddress(StakingCMDWithdrawArgs.Recipient)
	zero := ethcommon.Address{}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		if zero == recipient {
			recipient = opts.From
		}
		return stakingManager.Withdraw(opts, lstID, recipient, amountBigInt)
	}, nil)
}

func (a StakingActions) updatePoolCommissionRate(ctx *cli.Context) error {
	stakingManager, client, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return stakingManager.UpdatePoolCommissionRate(opts, StakingCMDUpdatePoolCommissionRateArgs.PoolID, StakingCMDUpdatePoolCommissionRateArgs.NewCommissionRate)
	}, nil)
}

func (a StakingActions) poolInfo(ctx *cli.Context) error {
	stakingManager, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	res, err := stakingManager.GetPoolInfo(&bind.CallOpts{Context: ctx.Context}, StakingCMDPoolInfoArgs.PoolID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

func (a StakingActions) poolHistoryLSTRate(ctx *cli.Context) error {
	stakingManager, _, err := a.bindingContract(ctx)
	if err != nil {
		return err
	}
	res, err := stakingManager.GetPoolHistoryLiquidStakingTokenRate(&bind.CallOpts{Context: ctx.Context}, StakingCMDPoolHistoryLSTRateArgs.PoolID, StakingCMDPoolHistoryLSTRateArgs.Epoch)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}
