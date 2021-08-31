package repo

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/ethereum/go-ethereum/event"
	libp2pcert "github.com/meshplus/go-libp2p-cert"
)

type Repo struct {
	Config           *Config
	NetworkConfig    *NetworkConfig
	Key              *Key
	Certs            *libp2pcert.Certs
	ConfigChangeFeed event.Feed
}

func (r *Repo) SubscribeConfigChange(ch chan *Repo) event.Subscription {
	return r.ConfigChangeFeed.Subscribe(ch)
}

func Load(repoRoot string, passwd string, configPath string, networkPath string) (*Repo, error) {
	bViper := viper.New()
	nViper := viper.New()
	config, err := UnmarshalConfig(bViper, repoRoot, configPath)
	if err != nil {
		return nil, err
	}

	if err := checkConfig(config); err != nil {
		return nil, err
	}

	var networkConfig *NetworkConfig
	if len(networkPath) == 0 {
		networkConfig, err = loadNetworkConfig(nViper, repoRoot, config.Genesis)
	} else {
		networkConfig, err = loadNetworkConfig(nViper, networkPath, config.Genesis)
		cmd := exec.Command("cp", filepath.Join(networkPath, "network.toml"), filepath.Join(repoRoot, "network.toml"))
		err := cmd.Run()
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, fmt.Errorf("load network config: %w", err)
	}

	certs, err := libp2pcert.LoadCerts(repoRoot, config.NodeCertPath, config.AgencyCertPath, config.CACertPath)
	if err != nil {
		return nil, err
	}

	key, err := loadPrivKey(repoRoot, passwd)
	if err != nil {
		return nil, fmt.Errorf("load private key: %w", err)
	}

	repo := &Repo{
		Config:        config,
		NetworkConfig: networkConfig,
		Key:           key,
		Certs:         certs,
	}

	// watch bitxhub.toml on changed
	WatchBitxhubConfig(bViper, &repo.ConfigChangeFeed)

	// watch network.toml on changed
	WatchNetworkConfig(nViper, &repo.ConfigChangeFeed, &NetworkConfig{Genesis: config.Genesis})

	return repo, nil
}

func checkConfig(config *Config) error {
	// check genesis admin info
	hasSuperAdmin := false
	for _, admin := range config.Genesis.Admins {
		if admin.Weight == SuperAdminWeight {
			hasSuperAdmin = true
		} else if admin.Weight != NormalAdminWeight {
			return fmt.Errorf("Illegal admin weight in genesis config!")
		}
	}

	if !hasSuperAdmin {
		return fmt.Errorf("Set up at least one super administrator in genesis config!")
	}
	return nil
}

func GetAPI(repoRoot string) (string, error) {
	data, err := ioutil.ReadFile(filepath.Join(repoRoot, APIName))
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func GetKeyPath(repoRoot string) string {
	return filepath.Join(repoRoot, KeyName)
}

func GetStoragePath(repoRoot string, subPath ...string) string {
	p := filepath.Join(repoRoot, "storage")
	for _, s := range subPath {
		p = filepath.Join(p, s)
	}

	return p
}
