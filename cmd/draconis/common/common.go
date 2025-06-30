package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-ledger/internal/app"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var KeystorePasswordFlagVar string

func KeystorePasswordFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "password",
		Usage:       "Keystore password",
		EnvVars:     []string{"DRACONIS_KEYSTORE_PASSWORD"},
		Destination: &KeystorePasswordFlagVar,
		Aliases:     []string{"pwd"},
		Required:    false,
	}
}

func EnterPassword(needConfirm bool) (string, error) {
	var password string
	fmt.Println("enter a password for keystore(will use default if input empty):")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", errors.Wrap(err, "can not read password")
	}
	text := string(passwordBytes)
	password = strings.ReplaceAll(text, "\n", "")
	if needConfirm {
		fmt.Println("please re-enter password for keystore:")
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", errors.Wrap(err, "can not read password")
		}
		confirmedPassword := strings.ReplaceAll(string(passwordBytes), "\n", "")
		if password != confirmedPassword {
			fmt.Println("passwords did not match, please try again")
			return EnterPassword(true)
		}
	}
	return password, nil
}

func Pretty(d any) error {
	res, err := json.MarshalIndent(d, "", "\t")
	if err != nil {
		return err
	}
	fmt.Println(string(res))
	return nil
}

func GetRootPath(ctx *cli.Context) (string, error) {
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

func PrepareRepo(ctx *cli.Context) (*repo.Repo, error) {
	p, err := GetRootPath(ctx)
	if err != nil {
		return nil, err
	}
	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		return nil, errors.New("axiom-ledger repo not exist")
	}

	r, err := repo.Load(p)
	if err != nil {
		return nil, err
	}

	// close monitor in offline mode
	r.Config.Monitor.Enable = false

	fmt.Printf("%s-repo: %s\n", repo.AppName, r.RepoRoot)

	if err := loggers.Initialize(ctx.Context, r, false); err != nil {
		return nil, err
	}

	if err := app.PrepareAxiomLedger(r); err != nil {
		return nil, fmt.Errorf("prepare axiom-ledger failed: %w", err)
	}
	return r, nil
}

func PrepareRepoWithKeystore(ctx *cli.Context) (*repo.Repo, error) {
	r, err := PrepareRepo(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.ReadKeystore(); err != nil {
		return nil, err
	}

	password := KeystorePasswordFlagVar
	if !ctx.IsSet(KeystorePasswordFlag().Name) {
		password, err = EnterPassword(false)
		if err != nil {
			return nil, err
		}
	}
	if password == "" {
		password = repo.DefaultKeystorePassword
		fmt.Println("keystore password is empty, will use default")
	}
	if err := r.DecryptKeystore(password); err != nil {
		return nil, err
	}
	return r, nil
}

func WaitUserConfirm() error {
	var choice string
	if _, err := fmt.Scanln(&choice); err != nil {
		return err
	}
	if choice != "y" {
		return errors.New("interrupt by user")
	}
	return nil
}

func DecodePeers(peersStr []string) (*consensus.QuorumValidators, error) {
	var peers []*consensus.QuorumValidator
	if len(peersStr) == 0 {
		return nil, errors.New("peers cannot be empty")
	}
	for _, p := range peersStr {
		// spilt id and peerId by :
		data := strings.Split(p, ":")
		if len(data) != 2 {
			return nil, fmt.Errorf("invalid peer: %s, should be id:peerId", p)
		}
		id, err := strconv.ParseUint(data[0], 10, 64)
		if err != nil {
			return nil, err
		}
		peers = append(peers, &consensus.QuorumValidator{
			Id:     id,
			PeerId: data[1],
		})
	}

	return &consensus.QuorumValidators{Validators: peers}, nil
}
