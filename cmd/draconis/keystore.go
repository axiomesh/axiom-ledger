package main

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/cmd/draconis/common"
	"github.com/axiomesh/axiom-ledger/pkg/crypto"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var p2pKeystorePrivateKeyFlagVar string

func p2pKeystorePrivateKeyFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "p2p-private-key",
		Usage:       "P2P keystore private key(hex string), if not specified, generate a new one",
		Destination: &p2pKeystorePrivateKeyFlagVar,
		EnvVars:     []string{"DRACONIS_P2P_KEYSTORE_PRIVATE_KEY"},
		Required:    false,
	}
}

var consensusKeystorePrivateKeyFlagVar string

func consensusKeystorePrivateKeyFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "consensus-private-key",
		Usage:       "Consensus keystore private key(hex string), if not specified, generate a new one",
		Destination: &consensusKeystorePrivateKeyFlagVar,
		EnvVars:     []string{"DRACONIS_CONSENSUS_KEYSTORE_PRIVATE_KEY"},
		Required:    false,
	}
}

var keystoreOldPasswordFlagVar string
var keystoreNewPasswordFlagVar string

var keystoreCMD = &cli.Command{
	Name:  "keystore",
	Usage: "The keystore manage commands",
	Subcommands: []*cli.Command{
		{
			Name:   "generate",
			Usage:  "Generate all keystore(p2p and consensus)",
			Action: generateKeystore,
			Flags: []cli.Flag{
				common.KeystorePasswordFlag(),
				p2pKeystorePrivateKeyFlag(),
				consensusKeystorePrivateKeyFlag(),
			},
		},
		{
			Name:   "update-password",
			Usage:  "Update keystore password for all keystore(p2p and consensus)",
			Action: updateKeystorePassword,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "old-password",
					Usage:       "Old keystore password",
					EnvVars:     []string{"DRACONIS_KEYSTORE_OLD_PASSWORD"},
					Destination: &keystoreOldPasswordFlagVar,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "new-password",
					Usage:       "New keystore password",
					EnvVars:     []string{"DRACONIS_KEYSTORE_NEW_PASSWORD"},
					Destination: &keystoreNewPasswordFlagVar,
					Required:    false,
				},
			},
		},
		{
			Name:   "p2p-private-key",
			Usage:  "Show p2p keystore private key",
			Action: showP2PKeystorePrivateKey,
			Flags: []cli.Flag{
				common.KeystorePasswordFlag(),
			},
		},
		{
			Name:   "p2p-public-key",
			Usage:  "Show p2p keystore public key",
			Action: showP2PKeystorePublicKey,
		},
		{
			Name:   "p2p-id",
			Usage:  "Show p2p id",
			Action: showP2PID,
		},
		{
			Name:   "consensus-private-key",
			Usage:  "Show consensus keystore private key",
			Action: showConsensusKeystorePrivateKey,
			Flags: []cli.Flag{
				common.KeystorePasswordFlag(),
			},
		},
		{
			Name:   "consensus-public-key",
			Usage:  "Show consensus keystore public key",
			Action: showConsensusKeystorePublicKey,
		},
	},
}

func generateKeystore(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}

	password := common.KeystorePasswordFlagVar
	if !ctx.IsSet(common.KeystorePasswordFlag().Name) {
		password, err = common.EnterPassword(true)
		if err != nil {
			return err
		}
	}

	if password == "" {
		password = repo.DefaultKeystorePassword
		fmt.Println("keystore password is empty, will use default")
	}
	p2pKeystore, err := repo.GenerateP2PKeystore(p, p2pKeystorePrivateKeyFlagVar, password)
	if err != nil {
		return err
	}
	consensusKeystore, err := repo.GenerateConsensusKeystore(p, consensusKeystorePrivateKeyFlagVar, password)
	if err != nil {
		return err
	}

	if fileutil.Exist(p2pKeystore.Path) {
		fmt.Printf("%s already exist\n", p2pKeystore.Path)
		return nil
	}
	if fileutil.Exist(consensusKeystore.Path) {
		fmt.Printf("%s already exist\n", consensusKeystore.Path)
		return nil
	}

	if err := p2pKeystore.Write(); err != nil {
		return err
	}
	fmt.Printf("generate p2p keystore success, path: %s\n", p2pKeystore.Path)
	fmt.Printf("p2p public key: %s\n", p2pKeystore.PublicKey.String())
	fmt.Printf("p2p ID: %s\n", p2pKeystore.P2PID())

	if err := consensusKeystore.Write(); err != nil {
		return err
	}
	fmt.Printf("generate consensus keystore success, path: %s\n", consensusKeystore.Path)
	fmt.Printf("consensus public key: %s\n", consensusKeystore.PublicKey.String())
	return nil
}

func updateKeystorePassword(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}
	p2pKeystorePath := filepath.Join(p, repo.P2PKeystoreFileName)
	if !fileutil.Exist(p2pKeystorePath) {
		fmt.Printf("%s not exist\n", p2pKeystorePath)
		return nil
	}
	consensusKeystorePath := filepath.Join(p, repo.ConsensusKeystoreFileName)
	if !fileutil.Exist(consensusKeystorePath) {
		fmt.Printf("%s not exist\n", consensusKeystorePath)
		return nil
	}

	p2pKeystoreInfo, err := crypto.ReadKeystoreInfo(p2pKeystorePath)
	if err != nil {
		return err
	}
	consensusKeystoreInfo, err := crypto.ReadKeystoreInfo(consensusKeystorePath)
	if err != nil {
		return err
	}

	fmt.Println("input old password:")
	oldPassword := keystoreOldPasswordFlagVar
	if !ctx.IsSet("old-password") {
		oldPassword, err = common.EnterPassword(false)
		if err != nil {
			return err
		}
	}
	fmt.Println("input new password:")
	newPassword := keystoreNewPasswordFlagVar
	if !ctx.IsSet("new-password") {
		newPassword, err = common.EnterPassword(false)
		if err != nil {
			return err
		}
	}

	if err := p2pKeystoreInfo.UpdatePassword(oldPassword, newPassword); err != nil {
		return errors.Wrap(err, "failed to update p2p keystore password")
	}
	if err := consensusKeystoreInfo.UpdatePassword(oldPassword, newPassword); err != nil {
		return errors.Wrap(err, "failed to update consensus keystore password")
	}

	if err := crypto.WriteKeystoreInfo(p2pKeystorePath, p2pKeystoreInfo); err != nil {
		return err
	}
	if err := crypto.WriteKeystoreInfo(consensusKeystorePath, consensusKeystoreInfo); err != nil {
		return err
	}
	fmt.Println("update keystore password success")
	return nil
}

func showP2PKeystorePrivateKey(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}
	p2pKeystore, err := repo.ReadP2PKeystore(p)
	if err != nil {
		return err
	}
	password := common.KeystorePasswordFlagVar
	if !ctx.IsSet(common.KeystorePasswordFlag().Name) {
		password, err = common.EnterPassword(false)
		if err != nil {
			return err
		}
	}
	if password == "" {
		password = repo.DefaultKeystorePassword
		fmt.Println("keystore password is empty, will use default")
	}
	if err := p2pKeystore.DecryptPrivateKey(password); err != nil {
		return errors.Wrap(err, "failed to decrypt p2p keystore private key, please check password")
	}

	fmt.Println(p2pKeystore.PrivateKey.String())
	return nil
}

func showP2PKeystorePublicKey(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}
	p2pKeystore, err := repo.ReadP2PKeystore(p)
	if err != nil {
		return err
	}
	fmt.Println(p2pKeystore.PublicKey.String())
	return nil
}

func showP2PID(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}
	p2pKeystore, err := repo.ReadP2PKeystore(p)
	if err != nil {
		return err
	}
	fmt.Println(p2pKeystore.P2PID())
	return nil
}

func showConsensusKeystorePrivateKey(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}
	consensusKeystore, err := repo.ReadConsensusKeystore(p)
	if err != nil {
		return err
	}
	password := common.KeystorePasswordFlagVar
	if !ctx.IsSet(common.KeystorePasswordFlag().Name) {
		password, err = common.EnterPassword(false)
		if err != nil {
			return err
		}
	}
	if password == "" {
		password = repo.DefaultKeystorePassword
		fmt.Println("keystore password is empty, will use default")
	}
	if err := consensusKeystore.DecryptPrivateKey(password); err != nil {
		return errors.Wrap(err, "failed to decrypt consensus keystore private key, please check password")
	}

	fmt.Println(consensusKeystore.PrivateKey.String())
	return nil
}

func showConsensusKeystorePublicKey(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger repo not exist")
		return nil
	}
	consensusKeystore, err := repo.ReadConsensusKeystore(p)
	if err != nil {
		return err
	}
	fmt.Println(consensusKeystore.PublicKey.String())
	return nil
}
