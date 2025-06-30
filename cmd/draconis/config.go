package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/cmd/draconis/common"
	"github.com/axiomesh/axiom-ledger/internal/genesis"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var configCMD = &cli.Command{
	Name:  "config",
	Usage: "The config manage commands",
	Subcommands: []*cli.Command{
		{
			Name:   "generate",
			Usage:  "Generate default config",
			Action: generate,
		},
		{
			Name:   "node-info",
			Usage:  "show node info",
			Action: nodeInfo,
		},
		{
			Name:   "show",
			Usage:  "Show the complete config processed by the environment variable",
			Action: show,
		},
		{
			Name:   "show-consensus",
			Usage:  "Show the complete consensus config processed by the environment variable",
			Action: showConsensus,
		},
		{
			Name:   "show-genesis",
			Usage:  "Show the complete genesis config processed by the environment variable",
			Action: showGenesis,
		},
		{
			Name:   "show-genesis-from-ledger",
			Usage:  "Show the complete genesis config from ledger",
			Action: showGenesisFromLedger,
		},
		{
			Name:   "check",
			Usage:  "Check if the config file is valid",
			Action: check,
		},
	},
}

func generate(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo already exists")
		return nil
	}

	if !fileutil.Exist(p) {
		err = os.MkdirAll(p, 0755)
		if err != nil {
			return err
		}
	}

	r, err := repo.Default(p)
	if err != nil {
		return err
	}
	if err := r.Flush(); err != nil {
		return err
	}
	fmt.Printf("config successfully generated in %s\n", p)
	return nil
}

func nodeInfo(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}

	r, err := repo.Load(p)
	if err != nil {
		return err
	}
	if err := r.ReadKeystore(); err != nil {
		return err
	}

	r.PrintNodeInfo(func(c string) {
		fmt.Println(c)
	})
	return nil
}

func show(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}

	r, err := repo.Load(p)
	if err != nil {
		return err
	}
	str, err := repo.MarshalConfig(r.Config)
	if err != nil {
		return err
	}
	fmt.Println(str)
	return nil
}

func showConsensus(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}

	r, err := repo.Load(p)
	if err != nil {
		return err
	}
	str, err := repo.MarshalConfig(r.ConsensusConfig)
	if err != nil {
		return err
	}
	fmt.Println(str)
	return nil
}

func showGenesis(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}

	r, err := repo.Load(p)
	if err != nil {
		return err
	}
	str, err := repo.MarshalConfig(r.GenesisConfig)
	if err != nil {
		return err
	}
	fmt.Println(str)
	return nil
}

func showGenesisFromLedger(ctx *cli.Context) error {
	r, err := common.PrepareRepo(ctx)
	if err != nil {
		return err
	}

	stateLedger, err := ledger.NewStateLedger(r, "")
	if err != nil {
		return fmt.Errorf("init state ledger failed: %w", err)
	}

	genesisConfig, err := genesis.GetGenesisConfig(stateLedger)
	if err != nil {
		return err
	}
	if genesisConfig == nil {
		return errors.New("genesis config not exist")
	}

	fmt.Println(genesisConfig)
	return nil
}

func check(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}

	_, err = repo.Load(p)
	if err != nil {
		fmt.Println("config file format error, please check:", err)
		os.Exit(1)
		return nil
	}

	return nil
}
