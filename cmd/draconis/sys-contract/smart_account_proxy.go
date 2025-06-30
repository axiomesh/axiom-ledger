package sys_contract

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/saccount/solidity/smart_account_proxy_client"
)

var AccountProxyCMDCommonArgs []cli.Flag

var AccountAddr string

var SmartAccountProxyCMD = &cli.Command{
	Name:  "smart-account-proxy",
	Usage: "smart account proxy query smart account",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "get-owner",
			Usage:  "get owner of smart account",
			Action: SmartAccountProxyActions{}.getOwner,
			Flags: append(AccountProxyCMDCommonArgs, []cli.Flag{
				&cli.StringFlag{
					Name:        "addr",
					Usage:       "smart account address",
					Destination: &AccountAddr,
					Required:    true,
				},
				senderFlag,
			}...),
		},
		{
			Name:   "get-guardian",
			Usage:  "get guardian of smart account",
			Action: SmartAccountProxyActions{}.getGuardian,
			Flags: append(AccountProxyCMDCommonArgs, []cli.Flag{
				&cli.StringFlag{
					Name:        "addr",
					Usage:       "smart account address",
					Destination: &AccountAddr,
					Required:    true,
				},
				senderFlag,
			}...),
		},
		{
			Name:   "get-status",
			Usage:  "get lock status time of smart account",
			Action: SmartAccountProxyActions{}.getStatus,
			Flags: append(AccountProxyCMDCommonArgs, []cli.Flag{
				&cli.StringFlag{
					Name:        "addr",
					Usage:       "smart account address",
					Destination: &AccountAddr,
					Required:    true,
				},
				senderFlag,
			}...),
		},
		{
			Name:   "get-passkeys",
			Usage:  "get passkeys of smart account",
			Action: SmartAccountProxyActions{}.getPasskeys,
			Flags: append(AccountProxyCMDCommonArgs, []cli.Flag{
				&cli.StringFlag{
					Name:        "addr",
					Usage:       "smart account address",
					Destination: &AccountAddr,
					Required:    true,
				},
				senderFlag,
			}...),
		},
		{
			Name:   "get-sessions",
			Usage:  "get sessions of smart account",
			Action: SmartAccountProxyActions{}.getSessions,
			Flags: append(AccountProxyCMDCommonArgs, []cli.Flag{
				&cli.StringFlag{
					Name:        "addr",
					Usage:       "smart account address",
					Destination: &AccountAddr,
					Required:    true,
				},
				senderFlag,
			}...),
		},
	},
}

type SmartAccountProxyActions struct{}

// nolint
func (_ SmartAccountProxyActions) bindContract(ctx *cli.Context) (*smart_account_proxy_client.BindingContract, *ethclient.Client, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "dial rpc failed")
	}

	contract, err := smart_account_proxy_client.NewBindingContract(ethcommon.HexToAddress(syscommon.AccountProxyContractAddr), client)
	if err != nil {
		return nil, nil, errors.Wrap(err, "bind governance contract failed")
	}
	return contract, client, nil
}

func (a SmartAccountProxyActions) getOwner(ctx *cli.Context) error {
	contract, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	owner, err := contract.GetOwner(&bind.CallOpts{Context: ctx.Context}, ethcommon.HexToAddress(AccountAddr))
	if err != nil {
		return err
	}

	fmt.Printf("owner: %s\n", owner.Hex())

	return nil
}

func (a SmartAccountProxyActions) getGuardian(ctx *cli.Context) error {
	contract, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	guardian, err := contract.GetGuardian(&bind.CallOpts{Context: ctx.Context}, ethcommon.HexToAddress(AccountAddr))
	if err != nil {
		return err
	}

	fmt.Printf("guardian: %s\n", guardian.Hex())
	return nil
}

func (a SmartAccountProxyActions) getStatus(ctx *cli.Context) error {
	contract, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	status, err := contract.GetStatus(&bind.CallOpts{Context: ctx.Context}, ethcommon.HexToAddress(AccountAddr))
	if err != nil {
		return err
	}

	fmt.Printf("lock status time: %s\n", time.Unix(int64(status), 0).Format("2006-01-02 15:04:05"))
	return nil
}

func (a SmartAccountProxyActions) getPasskeys(ctx *cli.Context) error {
	contract, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	passkeyList, err := contract.GetPasskeys(&bind.CallOpts{Context: ctx.Context}, ethcommon.HexToAddress(AccountAddr))
	if err != nil {
		return err
	}

	fmt.Printf("passkey list: %+v\n", passkeyList)
	return nil
}

func (a SmartAccountProxyActions) getSessions(ctx *cli.Context) error {
	contract, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	sessionList, err := contract.GetSessions(&bind.CallOpts{Context: ctx.Context}, ethcommon.HexToAddress(AccountAddr))
	if err != nil {
		return err
	}

	fmt.Printf("session: %+v\n", sessionList)
	return nil
}
