package repo

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/mitchellh/go-homedir"
	"github.com/mitchellh/mapstructure"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	rbft "github.com/axiomesh/axiom-bft"
)

type Repo struct {
	RepoRoot        string
	Config          *Config
	ConsensusConfig *ConsensusConfig
	GenesisConfig   *GenesisConfig
	P2PAddress      string
	P2PKey          *ecdsa.PrivateKey
	P2PID           string

	// TODO: Move to epoch manager service
	// Track current epoch info, will be updated bt executor
	EpochInfo *rbft.EpochInfo

	StartArgs *StartArgs
}

func (r *Repo) PrintNodeInfo(writer func(c string)) {
	writer(fmt.Sprintf("%s-repo: %s", AppName, r.RepoRoot))
	writer(fmt.Sprintf("node-key-addr: %s", r.P2PAddress))
	writer(fmt.Sprintf("p2p-id: %s", r.P2PID))
	writer(fmt.Sprintf("p2p-addr: /ip4/0.0.0.0/tcp/%d/p2p/%s", r.Config.Port.P2P, r.P2PID))
}

type signerOpts struct {
}

func (*signerOpts) HashFunc() crypto.Hash {
	return crypto.SHA3_256
}

var signOpt = &signerOpts{}

func (r *Repo) P2PKeySign(data []byte) ([]byte, error) {
	return r.P2PKey.Sign(rand.Reader, data, signOpt)
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

func writeConfigWithEnv(cfgPath string, config any) error {
	if err := writeConfig(cfgPath, config); err != nil {
		return err
	}
	// write back environment variables first
	// TODO: wait viper support read from environment variables
	if err := readConfigFromFile(cfgPath, config); err != nil {
		return errors.Wrapf(err, "failed to read cfg from environment")
	}
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
	return DefaultWithNodeIndex(repoRoot, 0, false, DefaultKeyJsonPassword)
}

func DefaultWithNodeIndex(repoRoot string, nodeIndex int, epochEnable bool, auth string) (*Repo, error) {
	var p2pKey *ecdsa.PrivateKey
	var err error
	var p2pAddress string
	if nodeIndex < 0 || nodeIndex > len(DefaultNodeKeys)-1 {
		p2pKey, err = fromP2PKeyJson(auth, repoRoot)
		if err != nil {
			return nil, err
		}
		nodeIndex = 0
	} else {
		p2pKey, err = ParseKey([]byte(DefaultNodeKeys[nodeIndex]))
		_, err = GenerateP2PKeyJson(auth, repoRoot, p2pKey)
		if err != nil {
			return nil, err
		}
	}
	p2pAddress = ethcrypto.PubkeyToAddress(p2pKey.PublicKey).String()

	id, err := KeyToNodeID(p2pKey)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	cfg.Port.P2P = int64(4001 + nodeIndex)

	genesisCfg := DefaultGenesisConfig(epochEnable)

	return &Repo{
		RepoRoot:        repoRoot,
		Config:          cfg,
		ConsensusConfig: DefaultConsensusConfig(),
		GenesisConfig:   genesisCfg,
		P2PAddress:      p2pAddress,
		P2PKey:          p2pKey,
		P2PID:           id,
		EpochInfo:       genesisCfg.EpochInfo,
		StartArgs:       &StartArgs{ReadonlyMode: false, SnapshotMode: false},
	}, nil
}

// Load config from the repo, which is automatically initialized when the repo is empty
func Load(auth string, repoRoot string, needAuth bool) (*Repo, error) {
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

	var p2pKey *ecdsa.PrivateKey
	var p2pAddress string
	var p2pId string
	if needAuth {
		p2pKey, err = fromP2PKeyJson(auth, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to load node p2pKey: %w", err)
		}
		p2pAddress = ethcrypto.PubkeyToAddress(p2pKey.PublicKey).String()
		p2pId, err = KeyToNodeID(p2pKey)
		if err != nil {
			return nil, err
		}
	} else {
		customKeyJson, err := GetCustomKeyJson(path.Join(repoRoot, P2PKeyFileName))
		if err != nil {
			return nil, err
		}
		p2pId = customKeyJson.NodeP2PId
		p2pAddress = ethcommon.HexToAddress(customKeyJson.Address).String()
	}

	repo := &Repo{
		RepoRoot:        repoRoot,
		Config:          cfg,
		ConsensusConfig: consensusCfg,
		GenesisConfig:   genesisCfg,
		P2PAddress:      p2pAddress,
		P2PKey:          p2pKey, // maybe nil
		P2PID:           p2pId,
		EpochInfo:       genesisCfg.EpochInfo,
		StartArgs:       &StartArgs{ReadonlyMode: false, SnapshotMode: false},
	}

	return repo, nil
}

func GetStoragePath(repoRoot string, subPath ...string) string {
	p := filepath.Join(repoRoot, "storage")
	for _, s := range subPath {
		p = filepath.Join(p, s)
	}

	return p
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

func readConfigFromFile(cfgFilePath string, config any) error {
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

	return readConfig(vp, config)
}

func readConfig(vp *viper.Viper, config any) error {
	vp.AutomaticEnv()
	if _, ok := config.(*GenesisConfig); ok {
		vp.SetEnvPrefix("DRACONIS_GENESIS")
	} else if _, ok := config.(*ConsensusConfig); ok {
		vp.SetEnvPrefix("DRACONIS_CONSENSUS")
	} else {
		vp.SetEnvPrefix("DRACONIS")
	}
	replacer := strings.NewReplacer(".", "_")
	vp.SetEnvKeyReplacer(replacer)

	err := vp.ReadInConfig()
	if err != nil {
		return err
	}

	if err := vp.Unmarshal(config, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		StringToTimeDurationHookFunc(),
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
