package repo

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type Repo struct {
	RepoRoot        string
	Config          *Config
	ConsensusConfig *ConsensusConfig
	GenesisConfig   *GenesisConfig

	ConsensusKeystore *ConsensusKeystore
	P2PKeystore       *P2PKeystore
	StartArgs         *StartArgs
	SyncArgs          *SyncArgs
}

func (r *Repo) PrintNodeInfo(writer func(c string)) {
	writer(fmt.Sprintf("Repo-root: %s", r.RepoRoot))
	writer(fmt.Sprintf("consensus-pubkey: %s", r.ConsensusKeystore.PublicKey.String()))
	writer(fmt.Sprintf("p2p-pubkey: %s", r.P2PKeystore.PublicKey.String()))
	writer(fmt.Sprintf("p2p-id: %s", r.P2PKeystore.P2PID()))

	localIP, err := getLocalIP()
	if err != nil {
		localIP = "127.0.0.1"
	}
	writer(fmt.Sprintf("p2p-addr: /ip4/%s/tcp/%d/p2p/%s", localIP, r.Config.Port.P2P, r.P2PKeystore.P2PID()))
}

func (r *Repo) Flush() error {
	if err := writeConfigWithEnv(path.Join(r.RepoRoot, CfgFileName), r.Config); err != nil {
		return errors.Wrap(err, "failed to write config")
	}
	if err := writeConfigWithEnv(path.Join(r.RepoRoot, consensusCfgFileName), r.ConsensusConfig); err != nil {
		return errors.Wrap(err, "failed to write consensus config")
	}
	if err := writeConfigWithEnv(path.Join(r.RepoRoot, genesisCfgFileName), r.GenesisConfig); err != nil {
		return errors.Wrap(err, "failed to write genesis config")
	}
	return nil
}

func (r *Repo) ReadKeystore() error {
	var err error
	r.ConsensusKeystore, err = ReadConsensusKeystore(r.RepoRoot)
	if err != nil {
		return errors.Wrap(err, "failed to read consensus keystore")
	}

	r.P2PKeystore, err = ReadP2PKeystore(r.RepoRoot)
	if err != nil {
		return errors.Wrap(err, "failed to read p2p keystore")
	}
	return nil
}

func (r *Repo) DecryptKeystore(password string) error {
	if err := r.ConsensusKeystore.DecryptPrivateKey(password); err != nil {
		return errors.Wrap(err, "failed to decrypt consensus private key")
	}
	if err := r.P2PKeystore.DecryptPrivateKey(password); err != nil {
		return errors.Wrap(err, "failed to decrypt p2p private key")
	}

	return nil
}

func writeConfigWithEnv(cfgPath string, config any) error {
	// read from environment
	if err := ReadConfigFromEnv(config); err != nil {
		return errors.Wrapf(err, "failed to read cfg from environment")
	}
	// write back environment variables first
	if err := writeConfig(cfgPath, config); err != nil {
		return err
	}
	return nil
}

func writeConfig(cfgPath string, config any) error {
	raw, err := MarshalConfig(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(cfgPath, []byte(raw), 0755); err != nil {
		return err
	}

	return nil
}

func MarshalConfig(config any) (string, error) {
	buf := bytes.NewBuffer([]byte{})
	e := toml.NewEncoder(buf)
	e.SetIndentTables(true)
	e.SetArraysMultiline(true)
	err := e.Encode(config)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Default(repoRoot string) (*Repo, error) {
	return &Repo{
		RepoRoot:        repoRoot,
		Config:          DefaultConfig(),
		ConsensusConfig: DefaultConsensusConfig(),
		GenesisConfig:   DefaultGenesisConfig(),
		StartArgs:       &StartArgs{ReadonlyMode: false, SnapshotMode: false},
	}, nil
}

// Load config from the repo, which is automatically initialized when the repo is empty
func Load(repoRoot string) (*Repo, error) {
	repoRoot, err := LoadRepoRootFromEnv(repoRoot)
	if err != nil {
		return nil, err
	}

	cfg, err := LoadConfig(repoRoot)
	if err != nil {
		return nil, err
	}

	consensusCfg, err := LoadConsensusConfig(repoRoot)
	if err != nil {
		return nil, err
	}

	genesisCfg, err := LoadGenesisConfig(repoRoot)
	if err != nil {
		return nil, err
	}

	repo := &Repo{
		RepoRoot:        repoRoot,
		Config:          cfg,
		ConsensusConfig: consensusCfg,
		GenesisConfig:   genesisCfg,
		StartArgs:       &StartArgs{ReadonlyMode: false, SnapshotMode: false},
	}

	return repo, nil
}

func LoadRepoRootFromEnv(repoRoot string) (string, error) {
	if repoRoot != "" {
		return repoRoot, nil
	}
	repoRoot = os.Getenv(rootPathEnvVar)
	var err error
	if len(repoRoot) == 0 {
		repoRoot, err = homedir.Expand(defaultRepoRoot)
	}
	return repoRoot, err
}

func ReadConfigFromEnv(config any) error {
	vp := viper.New()
	return readConfig(vp, config, false)
}

func ReadConfigFromFile(cfgFilePath string, config any) error {
	vp := viper.New()
	vp.SetConfigFile(cfgFilePath)
	vp.SetConfigType("toml")

	// only check types, viper does not have a strong type checking
	raw, err := os.ReadFile(cfgFilePath)
	if err != nil {
		return err
	}
	decoder := toml.NewDecoder(bytes.NewBuffer(raw))
	checker := reflect.New(reflect.TypeOf(config).Elem())
	if err := decoder.Decode(checker.Interface()); err != nil {
		var decodeError *toml.DecodeError
		if errors.As(err, &decodeError) {
			return errors.Errorf("check config formater failed from %s:\n%s", cfgFilePath, decodeError.String())
		}

		return errors.Wrapf(err, "check config formater failed from %s", cfgFilePath)
	}

	return readConfig(vp, config, true)
}

func readConfig(vp *viper.Viper, config any, fromFile bool) error {
	// not use viper 1.18.2(it not support only read env without file default)
	vp.AutomaticEnv()
	envPrefix := "AXIOM_LEDGER"
	switch config.(type) {
	case *GenesisConfig:
		envPrefix += "_GENESIS"
	case *ConsensusConfig:
		envPrefix += "_CONSENSUS"
	}
	vp.SetEnvPrefix(envPrefix)
	replacer := strings.NewReplacer(".", "_")
	vp.SetEnvKeyReplacer(replacer)

	if fromFile {
		err := vp.ReadInConfig()
		if err != nil {
			return err
		}
	}

	if err := vp.Unmarshal(config, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		StringToTimeDurationHookFunc(),
		StringToCoinNumberHookFunc(),
		func(
			f reflect.Kind,
			t reflect.Kind,
			data any) (any, error) {
			if f != reflect.String || t != reflect.Slice {
				return data, nil
			}

			raw := data.(string)
			if raw == "" {
				return []string{}, nil
			}
			raw = strings.TrimPrefix(raw, ";")
			raw = strings.TrimSuffix(raw, ";")

			return strings.Split(raw, ";"), nil
		},
	))); err != nil {
		return err
	}

	return nil
}

func WritePid(rootPath string) error {
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)
	if err := os.WriteFile(filepath.Join(rootPath, pidFileName), []byte(pidStr), 0755); err != nil {
		return errors.Wrap(err, "failed to write pid file")
	}
	return nil
}

func RemovePID(rootPath string) error {
	return os.Remove(filepath.Join(rootPath, pidFileName))
}

func CheckWritable(dir string) error {
	_, err := os.Stat(dir)
	if err == nil {
		// dir exists, make sure we can write to it
		testfile := filepath.Join(dir, "test")
		fi, err := os.Create(testfile)
		if err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("%s is not writeable by the current user", dir)
			}
			return fmt.Errorf("unexpected error while checking writeablility of repo root: %s", err)
		}
		_ = fi.Close()
		return os.Remove(testfile)
	}

	if os.IsNotExist(err) {
		// dir doesn't exist, check that we can create it
		return os.Mkdir(dir, 0775)
	}

	if os.IsPermission(err) {
		return fmt.Errorf("cannot write to %s, incorrect permissions", err)
	}

	return err
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		ipNet, isIpNet := addr.(*net.IPNet)
		if isIpNet && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", errors.New("not found local ip")
}
