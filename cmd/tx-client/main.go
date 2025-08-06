package main

import (
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()
	app.Name = "tx-client"
	app.Usage = "交易重放客户端 - 从RPC获取区块并重放交易到区块链"
	app.Version = "1.0.0"
	app.Compiled = time.Now()

	app.Commands = []*cli.Command{
		getBlocksCMD,
		replayCMD,
		proposeCMD,
		voteCMD,
		queryProposalCMD,
		{
			Name:    "version",
			Aliases: []string{"v"},
			Usage:   "显示版本信息",
			Action: func(ctx *cli.Context) error {
				fmt.Printf("tx-client version: %s\n", app.Version)
				fmt.Printf("Build date: %s\n", app.Compiled.Format("2006-01-02 15:04:05"))
				return nil
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
 