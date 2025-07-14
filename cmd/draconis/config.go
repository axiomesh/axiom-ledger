package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var configGenerateArgs = struct {
	DefaultNodeIndex int
	Solo             bool
	EpochEnable      bool
	Auth             string
}{}

var writeAccountKeyFileArgs = struct {
	PrivateKey string
}{}

func passwordFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "password",
		Usage:       "Generate or decode p2p.key with password",
		Destination: &configGenerateArgs.Auth,
		Aliases:     []string{"p", "P", "pwd"},
		Required:    false,
	}
}

var configCMD = &cli.Command{
	Name:  "config",
	Usage: "The config manage commands",
	Subcommands: []*cli.Command{
		{
			Name:   "generate",
			Usage:  "Generate default config and node private key(if not exist)",
			Action: generate,
			Flags: []cli.Flag{
				&cli.IntFlag{
					Name:        "default-node-index",
					Usage:       "use default node config by specified index(1,2,3,4), regenerate if not specified",
					Destination: &configGenerateArgs.DefaultNodeIndex,
					Required:    false,
				},
				&cli.BoolFlag{
					Name:        "solo",
					Usage:       "generate solo config if specified",
					Destination: &configGenerateArgs.Solo,
					Required:    false,
				},
				&cli.BoolFlag{
					Name:        "epoch-enable",
					Usage:       "generate epoch wrf enabled config if specified",
					Destination: &configGenerateArgs.EpochEnable,
					Required:    false,
				},
				passwordFlag(),
			},
		},
		{
			Name:   "generate-account-key",
			Usage:  "Generate account private key",
			Action: generateAccountKey,
			Flags: []cli.Flag{
				passwordFlag(),
			},
		},
		{
			Name:   "write-account-key-file",
			Usage:  "Write account private key string to file",
			Action: writeAccountKeyFile,
			Flags: []cli.Flag{
				passwordFlag(),
				&cli.StringFlag{
					Name:        "private-key",
					Usage:       "private key string",
					Destination: &writeAccountKeyFileArgs.PrivateKey,
					Required:    true,
				},
			},
		},
		{
			Name:   "change-key-password",
			Usage:  "Modifying the auth of the p2p.key secret in the system.",
			Action: changeKeyPassword,
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
			Name:   "check",
			Usage:  "Check if the config file is valid",
			Action: check,
		},
	},
}

func generate(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo already exists")
		return nil
	}

	if !fileutil.Exist(p) {
		err = os.MkdirAll(p, 0755)
		if err != nil {
			return err
		}
	}

	r, err := repo.DefaultWithNodeIndex(p, configGenerateArgs.DefaultNodeIndex-1, configGenerateArgs.EpochEnable, configGenerateArgs.Auth)
	if err != nil {
		return err
	}
	if configGenerateArgs.Solo {
		r.Config.Consensus.Type = repo.ConsensusTypeSolo
	}
	if err := r.Flush(); err != nil {
		return err
	}

	r.PrintNodeInfo(func(c string) {
		fmt.Println(c)
	})
	return nil
}

func generateAccountKey(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo not exist")
		return nil
	}

	keyPath := path.Join(p, repo.P2PKeyFileName)
	if fileutil.Exist(keyPath) {
		fmt.Printf("%s exists, do you want to overwrite it? y/n\n", keyPath)
		var choice string
		if _, err := fmt.Scanln(&choice); err != nil {
			return err
		}
		if choice != "y" {
			return errors.New("interrupt by user")
		}
	} else if configGenerateArgs.Auth != "" { // Designed to automate deployment scripts
		addr, err := repo.GenerateP2PKeyJson(configGenerateArgs.Auth, p, nil)
		if err != nil {
			return err
		}
		fmt.Println("generate account-addr:", addr)
		return nil
	}
	fmt.Println("Enter the password to generate the secret key: ")
	var password string
	if _, err := fmt.Scanln(&password); err != nil {
		return err
	}
	if len(password) == 0 {
		return errors.New("password cannot be empty")
	}
	fmt.Println("Enter the password again: ")
	var password2 string
	if _, err := fmt.Scanln(&password2); err != nil {
		return err
	}
	if password != password2 {
		return errors.New("password not match")
	}
	addr, err := repo.GenerateP2PKeyJson(password, p, nil)
	if err != nil {
		return err
	}
	fmt.Println("generate account-addr:", addr)

	return nil
}

func writeAccountKeyFile(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo not exist")
		return nil
	}

	privateKeyStr := strings.TrimPrefix(writeAccountKeyFileArgs.PrivateKey, "0x")
	sk, err := ethcrypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return errors.Wrap(err, "invalid private key")
	}
	keyPath := path.Join(p, repo.P2PKeyFileName)
	if fileutil.Exist(keyPath) {
		fmt.Printf("%s exists, do you want to overwrite it? y/n\n", keyPath)
		var choice string
		if _, err := fmt.Scanln(&choice); err != nil {
			return err
		}
		if choice != "y" {
			return errors.New("interrupt by user")
		}
	} else if configGenerateArgs.Auth != "" { // Designed to automate deployment scripts
		addr, err := repo.GenerateP2PKeyJson(configGenerateArgs.Auth, p, sk)
		if err != nil {
			return err
		}
		fmt.Println("write account-addr:", addr)
		return nil
	}

	fmt.Println("Enter the password to generate the secret key: ")
	var password string
	if _, err := fmt.Scanln(&password); err != nil {
		return err
	}
	if len(password) == 0 {
		return errors.New("password cannot be empty")
	}
	fmt.Println("Enter the password again: ")
	var password2 string
	if _, err := fmt.Scanln(&password2); err != nil {
		return err
	}
	if password != password2 {
		return errors.New("password not match")
	}
	addr, err := repo.GenerateP2PKeyJson(password, p, sk)
	if err != nil {
		return err
	}
	fmt.Println("write account-addr:", addr)

	return nil
}

func changeKeyPassword(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	keyPath := path.Join(p, repo.P2PKeyFileName)
	if !fileutil.Exist(keyPath) {
		fmt.Printf("%s not exist \n", keyPath)
		return nil
	}
	fmt.Println("Please enter the old password: ")
	var oldPassword string
	if _, err := fmt.Scanln(&oldPassword); err != nil {
		return err
	}
	key, err := repo.FromKeyJson(oldPassword, keyPath)
	if err != nil {
		return err
	}
	fmt.Println("Please enter the new password: ")
	var newPassword string
	if _, err := fmt.Scanln(&newPassword); err != nil {
		return err
	}
	fmt.Println("Please enter the new password again: ")
	var newPassword2 string
	if _, err := fmt.Scanln(&newPassword2); err != nil {
		return err
	}
	if newPassword != newPassword2 {
		return errors.New("new password not match")
	}
	if err := repo.ChangeAuthOfKeyJson(oldPassword, newPassword, keyPath); err != nil {
		return err
	}
	fmt.Printf("Change password of [%s] success!\n", ethcrypto.PubkeyToAddress(key.PublicKey).String())
	return nil
}

func nodeInfo(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo not exist")
		return nil
	}

	r, err := repo.Load(configGenerateArgs.Auth, p, false)
	if err != nil {
		return err
	}

	r.PrintNodeInfo(func(c string) {
		fmt.Println(c)
	})
	return nil
}

func show(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo not exist")
		return nil
	}

	r, err := repo.Load(configGenerateArgs.Auth, p, false)
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
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo not exist")
		return nil
	}

	r, err := repo.Load(configGenerateArgs.Auth, p, false)
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

func check(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis repo not exist")
		return nil
	}

	_, err = repo.Load(configGenerateArgs.Auth, p, false)
	if err != nil {
		fmt.Println("config file format error, please check:", err)
		os.Exit(1)
		return nil
	}

	return nil
}

func getRootPath(ctx *cli.Context) (string, error) {
	p := ctx.String("repo")

	var err error
	if p == "" {
		p, err = repo.LoadRepoRootFromEnv(p)
		if err != nil {
			return "", err
		}
	}
	return p, nil
}
