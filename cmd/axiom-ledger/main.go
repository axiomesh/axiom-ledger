package main

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/cmd/axiom-ledger/common"
	sys_contract "github.com/axiomesh/axiom-ledger/cmd/axiom-ledger/sys-contract"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

func main() {
	loadEnvFile()

	app := cli.NewApp()
	app.Name = repo.AppName
	app.Usage = "A blockchain infrastructure with high scalability, privacy, security and composability"
	app.Compiled = time.Now()

	cli.VersionPrinter = func(c *cli.Context) {
		printVersion(func(c string) {
			fmt.Println(c)
		})
	}

	// global flags
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Usage: "Work path",
		},
	}

	app.Commands = []*cli.Command{
		configCMD,
		ledgerCMD,
		txpoolCMD,
		keystoreCMD,
		clusterCMD,
		sys_contract.EpochCMD,
		sys_contract.GovernanceCMD,
		sys_contract.GovernanceNodeCMD,
		sys_contract.NodeCMD,
		sys_contract.StakingCMD,
		sys_contract.StakingLSTCMD,
		sys_contract.SmartAccountProxyCMD,
		{
			Name:   "start",
			Usage:  "Start a long-running daemon process",
			Action: start,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:        "readonly",
					Aliases:     []string{"r"},
					Usage:       "enable readonly mode(disable consensus and network), only support read api",
					Destination: &startArgs.Readonly,
					Required:    false,
				},
				&cli.BoolFlag{
					Name:        "snapshot",
					Aliases:     []string{"s"},
					Usage:       "enable snapshot mode(sync by snapshot), state had be updated in snapshot's height",
					Destination: &startArgs.Snapshot,
					Required:    false,
				},
				&cli.BoolFlag{
					Name:        "archive",
					Aliases:     []string{"a"},
					Usage:       "enable new archive node skip executing txs and apply state journal directly",
					Destination: &startArgs.Archive,
					Required:    false,
				},
				&cli.StringSliceFlag{
					Name:        "peers",
					Aliases:     []string{"p"},
					Usage:       "peers address, format: <id1>:<pid1>,<id2>:<pid2>,...",
					Value:       &cli.StringSlice{},
					Destination: &syncPeerArgs.remotePeers,
					Required:    false,
				},
				common.KeystorePasswordFlag(),
			},
		},
		{
			Name:    "version",
			Aliases: []string{"v"},
			Usage:   "Show code version",
			Action: func(ctx *cli.Context) error {
				printVersion(func(c string) {
					fmt.Println(c)
				})
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func loadEnvFile() {
	envFile := os.Getenv("AXIOM_LEDGER_ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}
	if fileutil.Exist(envFile) {
		if err := godotenv.Load(envFile); err != nil {
			fmt.Printf("load env file %s failed: %s\n", envFile, err)
			return
		}
	}
}
