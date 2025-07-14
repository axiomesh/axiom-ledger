package main

import (
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

func main() {
	app := cli.NewApp()
	app.Name = repo.AppName
	app.Usage = "A leading inter-blockchain platform"
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
		epochCMD,
		txpoolCMD,
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
				passwordFlag(),
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
