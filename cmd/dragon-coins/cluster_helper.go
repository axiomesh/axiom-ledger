package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type ClusterCouncilMember struct {
	Address string `mapstructure:"address" toml:"address" json:"address"`
	Weight  uint64 `mapstructure:"weight" toml:"weight" json:"weight"`
	Name    string `mapstructure:"name" toml:"name" json:"name"`
}

type ClusterAccount struct {
	Address string            `mapstructure:"address" toml:"address" json:"address"`
	Balance *types.CoinNumber `mapstructure:"balance" toml:"balance" json:"balance"`
}

type ClusterNodePort struct {
	JsonRpc   int64 `mapstructure:"jsonrpc" toml:"jsonrpc" json:"jsonrpc"`
	WebSocket int64 `mapstructure:"websocket" toml:"websocket" json:"websocket"`
	P2P       int64 `mapstructure:"p2p" toml:"p2p" json:"p2p"`
	PProf     int64 `mapstructure:"pprof" toml:"pprof" json:"pprof"`
	Monitor   int64 `mapstructure:"monitor" toml:"monitor" json:"monitor"`
}

type ClusterNode struct {
	Name                string            `mapstructure:"name" toml:"name" json:"name"`
	Desc                string            `mapstructure:"desc" toml:"desc" json:"desc"`
	P2PPrivateKey       string            `mapstructure:"p2p_private_key" toml:"p2p_private_key" json:"p2p_private_key"`
	ConsensusPrivateKey string            `mapstructure:"consensus_private_key" toml:"consensus_private_key" json:"consensus_private_key"`
	OperatorAddress     string            `mapstructure:"operator_address" toml:"operator_address" json:"operator_address"`
	IsDataSyncer        bool              `mapstructure:"is_data_syncer" toml:"is_data_syncer" json:"is_data_syncer"`
	StakeNumber         *types.CoinNumber `mapstructure:"stake_number" toml:"stake_number" json:"stake_number"`
	CommissionRate      uint64            `mapstructure:"commission_rate" toml:"commission_rate" json:"commission_rate"`
	IP                  string            `mapstructure:"ip" toml:"ip" json:"ip"`
	Port                ClusterNodePort   `mapstructure:"port" toml:"port" json:"port"`
}

type ClusterGenerateConfig struct {
	DefaultPort               *ClusterNodePort        `mapstructure:"default_port" toml:"default_port" json:"default_port"`
	EnablePortAutoIncrease    bool                    `mapstructure:"enable_port_auto_increase" toml:"enable_port_auto_increase" json:"enable_port_auto_increase"`
	MintForOperatorCoinAmount *types.CoinNumber       `mapstructure:"mint_for_operator_coin_amount" toml:"mint_for_operator_coin_amount" json:"mint_for_operator_coin_amount"`
	CouncilMembers            []*ClusterCouncilMember `mapstructure:"council_members" toml:"council_members" json:"council_members"`
	Nodes                     []*ClusterNode          `mapstructure:"nodes" toml:"nodes" json:"nodes"`
	Accounts                  []*ClusterAccount       `mapstructure:"accounts" toml:"accounts" json:"accounts"`
}

type ClusterGeneratorHelper struct {
	// inputs
	cfg *ClusterGenerateConfig

	keystorePassword string
	targetDir        string
	force            bool

	// intermediate
	genesisCfgTemplate *repo.GenesisConfig

	generalCfgTemplate   *repo.Config
	consensusCfgTemplate *repo.ConsensusConfig
	nodeRepos            map[uint64]*repo.Repo
	operatorKeyMap       map[uint64]*ecdsa.PrivateKey
	p2pAddresses         []string
}

func NewClusterGeneratorHelper(cfg *ClusterGenerateConfig, keystorePassword string, targetDir string, force bool, operatorKeyMap map[uint64]*ecdsa.PrivateKey) *ClusterGeneratorHelper {
	if operatorKeyMap == nil {
		operatorKeyMap = make(map[uint64]*ecdsa.PrivateKey)
	}
	return &ClusterGeneratorHelper{
		cfg:              cfg,
		keystorePassword: keystorePassword,
		targetDir:        targetDir,
		force:            force,
		nodeRepos:        make(map[uint64]*repo.Repo),
		operatorKeyMap:   operatorKeyMap,
	}
}

func (h *ClusterGeneratorHelper) Generate() error {
	if !fileutil.Exist(clusterGenerateTargetDir) {
		if err := os.Mkdir(clusterGenerateTargetDir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create target dir %s", h.targetDir)
		}
	}

	if h.keystorePassword == "" {
		h.keystorePassword = repo.DefaultKeystorePassword
		fmt.Println("keystore password is empty, will use default")
	}

	if h.cfg.DefaultPort == nil {
		h.cfg.DefaultPort = defaultPort
	} else {
		if h.cfg.DefaultPort.JsonRpc == 0 {
			h.cfg.DefaultPort.JsonRpc = 8881
		}
		if h.cfg.DefaultPort.WebSocket == 0 {
			h.cfg.DefaultPort.WebSocket = 9991
		}
		if h.cfg.DefaultPort.P2P == 0 {
			h.cfg.DefaultPort.P2P = 4001
		}
		if h.cfg.DefaultPort.PProf == 0 {
			h.cfg.DefaultPort.PProf = 53121
		}
		if h.cfg.DefaultPort.Monitor == 0 {
			h.cfg.DefaultPort.Monitor = 40011
		}
	}

	h.genesisCfgTemplate = repo.DefaultGenesisConfig()
	h.genesisCfgTemplate.Timestamp = time.Now().Unix()
	if err := repo.ReadConfigFromEnv(h.genesisCfgTemplate); err != nil {
		return errors.Wrap(err, "failed to read genesis config from env")
	}
	h.generalCfgTemplate = repo.DefaultConfig()
	if err := repo.ReadConfigFromEnv(h.generalCfgTemplate); err != nil {
		return errors.Wrap(err, "failed to read general config from env")
	}

	h.consensusCfgTemplate = repo.DefaultConsensusConfig()
	if err := repo.ReadConfigFromEnv(h.consensusCfgTemplate); err != nil {
		return errors.Wrap(err, "failed to read consensus config from env")
	}

	for i := range h.cfg.Nodes {
		nodeID := uint64(i + 1)
		nodeRepoPath := h.nodeRepoRoot(nodeID)
		if fileutil.Exist(nodeRepoPath) {
			if !h.force {
				return errors.Errorf("node%d already exists, if you want to override it, use --force", nodeID)
			}
		} else {
			if err := os.MkdirAll(nodeRepoPath, 0755); err != nil {
				return errors.Wrapf(err, "failed to create node%d repo", nodeID)
			}
		}

		h.nodeRepos[nodeID] = &repo.Repo{
			RepoRoot: nodeRepoPath,
		}
	}

	if err := h.populateGenesisConfig(); err != nil {
		return errors.Wrap(err, "failed to populate genesis config")
	}

	for i, node := range h.cfg.Nodes {
		if err := h.populateNodeRepo(uint64(i+1), node); err != nil {
			return errors.Wrapf(err, "failed to generate node%d repo", i)
		}
	}

	for id := range h.cfg.Nodes {
		nodeID := uint64(id + 1)
		nodeRepo := h.nodeRepos[nodeID]
		nodeRepo.Config.P2P.BootstrapNodeAddresses = h.p2pAddresses
		if err := nodeRepo.Flush(); err != nil {
			return errors.Wrapf(err, "failed to flush node%d repo", nodeID)
		}
		if err := nodeRepo.P2PKeystore.Write(); err != nil {
			return errors.Wrapf(err, "failed to write node%d p2p keystore", nodeID)
		}
		if err := nodeRepo.ConsensusKeystore.Write(); err != nil {
			return errors.Wrapf(err, "failed to write node%d consensus keystore", nodeID)
		}
		if operatorKey := h.operatorKeyMap[nodeID]; operatorKey != nil {
			if err := os.WriteFile(filepath.Join(nodeRepo.RepoRoot, "operator.key"), []byte(hex.EncodeToString(ethcrypto.FromECDSA(operatorKey))), 0755); err != nil {
				return errors.Wrapf(err, "failed to write node%d operator key", nodeID)
			}
		}

		envFile := fmt.Sprintf("DRAGON_COINS_KEYSTORE_PASSWORD=%s\n", h.keystorePassword)
		if err := os.WriteFile(filepath.Join(nodeRepo.RepoRoot, ".env"), []byte(envFile), 0755); err != nil {
			return errors.Wrapf(err, "failed to write node%d .env", nodeID)
		}

		fmt.Printf("node%d repo generated\n", nodeID)
		nodeRepo.PrintNodeInfo(func(c string) {
			fmt.Println(c)
		})
		fmt.Printf("operator-address: %s\n", h.cfg.Nodes[nodeID-1].OperatorAddress)
		fmt.Println()
	}

	return nil
}

func (h *ClusterGeneratorHelper) populateGenesisConfig() error {
	// populate Accounts
	if len(lo.UniqBy(h.cfg.Accounts, func(c *ClusterAccount) string { return c.Address })) != len(h.cfg.Accounts) {
		return errors.New("duplicated account address")
	}

	if len(h.cfg.Accounts) != 0 {
		h.genesisCfgTemplate.Accounts = []*repo.Account{}
		for i, account := range h.cfg.Accounts {
			if !ethcommon.IsHexAddress(account.Address) {
				return errors.Errorf("account %d address is invalid: %s", i, account.Address)
			}

			h.genesisCfgTemplate.Accounts = append(h.genesisCfgTemplate.Accounts, &repo.Account{
				Address: account.Address,
				Balance: account.Balance,
			})
		}
	} else {
		h.genesisCfgTemplate.Accounts = lo.Map(append(defaultCouncilMemberAddrs, repo.MockDefaultAccountAddrs...), func(addr string, idx int) *repo.Account {
			return &repo.Account{
				Address: addr,
				Balance: repo.GetDefaultAccountBalance(),
			}
		})
	}

	// populate CouncilMembers
	if len(lo.UniqBy(h.cfg.CouncilMembers, func(c *ClusterCouncilMember) string { return c.Address })) != len(h.cfg.CouncilMembers) {
		return errors.New("duplicated council member address")
	}
	if len(h.cfg.CouncilMembers) != 0 {
		h.genesisCfgTemplate.CouncilMembers = []*repo.CouncilMember{}
		for i, councilMember := range h.cfg.CouncilMembers {
			if !ethcommon.IsHexAddress(councilMember.Address) {
				return errors.Errorf("council member %d address is invalid: %s", i, councilMember.Address)
			}
			if councilMember.Weight == 0 {
				return errors.Errorf("council member %d weight cannot be 0", i)
			}
			h.genesisCfgTemplate.CouncilMembers = append(h.genesisCfgTemplate.CouncilMembers, &repo.CouncilMember{
				Address: councilMember.Address,
				Weight:  councilMember.Weight,
				Name:    councilMember.Name,
			})
		}
	} else {
		h.genesisCfgTemplate.CouncilMembers = lo.Map(defaultCouncilMemberAddrs, func(item string, idx int) *repo.CouncilMember {
			return &repo.CouncilMember{
				Address: item,
				Weight:  1,
				Name:    defaultCouncilMemberNames[idx],
			}
		})
	}

	var err error
	// populate Nodes
	nodeP2PKeyDuplicateRecord := make(map[string]bool)
	nodeConsensusKeyDuplicateRecord := make(map[string]bool)
	for i, nodeConfig := range h.cfg.Nodes {
		nodeID := uint64(i + 1)
		nodeRepo := h.nodeRepos[nodeID]

		nodeRepo.P2PKeystore, err = repo.GenerateP2PKeystore(nodeRepo.RepoRoot, nodeConfig.P2PPrivateKey, h.keystorePassword)
		if err != nil {
			return errors.Errorf("failed to generate p2p keystore for node %d: %v", nodeID, err)
		}

		nodeRepo.ConsensusKeystore, err = repo.GenerateConsensusKeystore(nodeRepo.RepoRoot, nodeConfig.ConsensusPrivateKey, h.keystorePassword)
		if err != nil {
			return errors.Errorf("failed to generate consensus keystore for node %d: %v", nodeID, err)
		}

		if nodeConfig.CommissionRate > framework.CommissionRateDenominator {
			return errors.Errorf("invalid commission rate %d, need <= %d", nodeConfig.CommissionRate, framework.CommissionRateDenominator)
		}

		if nodeConfig.OperatorAddress == "" {
			sk, err := ethcrypto.GenerateKey()
			if err != nil {
				return errors.Errorf("failed to generate operator key for node %d: %v", nodeID, err)
			}
			h.operatorKeyMap[nodeID] = sk
			nodeConfig.OperatorAddress = ethcrypto.PubkeyToAddress(sk.PublicKey).String()
		} else {
			if !ethcommon.IsHexAddress(nodeConfig.OperatorAddress) {
				return errors.Errorf("invalid operator address %s for node %d: %v", nodeConfig.OperatorAddress, nodeID, err)
			}
		}

		if nodeP2PKeyDuplicateRecord[nodeRepo.P2PKeystore.PublicKey.String()] {
			return errors.Errorf("duplicated p2p private key for node %d: %s", nodeID, nodeConfig.P2PPrivateKey)
		}
		nodeP2PKeyDuplicateRecord[nodeRepo.P2PKeystore.PublicKey.String()] = true

		if nodeConsensusKeyDuplicateRecord[nodeRepo.ConsensusKeystore.PublicKey.String()] {
			return errors.Errorf("duplicated consensus private key for node %d: %s", nodeID, nodeConfig.ConsensusPrivateKey)
		}
		nodeConsensusKeyDuplicateRecord[nodeRepo.ConsensusKeystore.PublicKey.String()] = true

		if nodeConfig.StakeNumber == nil {
			nodeConfig.StakeNumber = h.genesisCfgTemplate.EpochInfo.StakeParams.MinValidatorStake
		}

		if nodeConfig.Name == "" {
			nodeConfig.Name = fmt.Sprintf("node%d", nodeID)
		}
		if nodeConfig.Desc == "" {
			nodeConfig.Desc = nodeConfig.Name
		}

		operatorAddress := ethcommon.HexToAddress(nodeConfig.OperatorAddress).String()

		h.genesisCfgTemplate.Nodes = append(h.genesisCfgTemplate.Nodes, repo.GenesisNodeInfo{
			ConsensusPubKey: nodeRepo.ConsensusKeystore.PublicKey.String(),
			P2PPubKey:       nodeRepo.P2PKeystore.PublicKey.String(),
			OperatorAddress: operatorAddress,
			MetaData: repo.GenesisNodeMetaData{
				Name: nodeConfig.Name,
				Desc: nodeConfig.Desc,
			},
			IsDataSyncer:   nodeConfig.IsDataSyncer,
			StakeNumber:    nodeConfig.StakeNumber,
			CommissionRate: nodeConfig.CommissionRate,
		})

		if h.cfg.MintForOperatorCoinAmount != nil {
			h.genesisCfgTemplate.Accounts = append(h.genesisCfgTemplate.Accounts, &repo.Account{
				Address: operatorAddress,
				Balance: h.cfg.MintForOperatorCoinAmount,
			})
		}

		if h.genesisCfgTemplate.SmartAccountAdmin != "" {
			h.genesisCfgTemplate.SmartAccountAdmin = nodeConfig.OperatorAddress
		}
	}

	return nil
}

func (h *ClusterGeneratorHelper) nodeRepoRoot(nodeID uint64) string {
	return filepath.Join(h.targetDir, fmt.Sprintf("node%d", nodeID))
}

func (h *ClusterGeneratorHelper) populateNodeRepo(nodeID uint64, nodeConfig *ClusterNode) error {
	nodeRepo := h.nodeRepos[nodeID]
	nodeRepo.GenesisConfig = &repo.GenesisConfig{}
	nodeRepo.Config = &repo.Config{}
	nodeRepo.ConsensusConfig = &repo.ConsensusConfig{}

	if err := copier.Copy(nodeRepo.GenesisConfig, h.genesisCfgTemplate); err != nil {
		return errors.Wrap(err, "failed to copy genesis config")
	}
	if err := copier.Copy(nodeRepo.Config, h.generalCfgTemplate); err != nil {
		return errors.Wrap(err, "failed to copy general config")
	}
	if err := copier.Copy(nodeRepo.ConsensusConfig, h.consensusCfgTemplate); err != nil {
		return errors.Wrap(err, "failed to copy consensus config")
	}

	if nodeConfig.IP == "" {
		nodeConfig.IP = "127.0.0.1"
	}

	if nodeConfig.Port.JsonRpc == 0 {
		nodeConfig.Port.JsonRpc = h.cfg.DefaultPort.JsonRpc
		if h.cfg.EnablePortAutoIncrease {
			nodeConfig.Port.JsonRpc += int64(nodeID) - 1
		}
	}
	nodeRepo.Config.Port.JsonRpc = nodeConfig.Port.JsonRpc

	if nodeConfig.Port.WebSocket == 0 {
		nodeConfig.Port.WebSocket = h.cfg.DefaultPort.WebSocket
		if h.cfg.EnablePortAutoIncrease {
			nodeConfig.Port.WebSocket += int64(nodeID) - 1
		}
	}
	nodeRepo.Config.Port.WebSocket = nodeConfig.Port.WebSocket

	if nodeConfig.Port.P2P == 0 {
		nodeConfig.Port.P2P = h.cfg.DefaultPort.P2P
		if h.cfg.EnablePortAutoIncrease {
			nodeConfig.Port.P2P += int64(nodeID) - 1
		}
	}
	nodeRepo.Config.Port.P2P = nodeConfig.Port.P2P

	if nodeConfig.Port.PProf == 0 {
		nodeConfig.Port.PProf = h.cfg.DefaultPort.PProf
		if h.cfg.EnablePortAutoIncrease {
			nodeConfig.Port.PProf += int64(nodeID) - 1
		}
	}
	nodeRepo.Config.Port.PProf = nodeConfig.Port.PProf

	if nodeConfig.Port.Monitor == 0 {
		nodeConfig.Port.Monitor = h.cfg.DefaultPort.Monitor
		if h.cfg.EnablePortAutoIncrease {
			nodeConfig.Port.Monitor += int64(nodeID) - 1
		}
	}
	nodeRepo.Config.Port.Monitor = nodeConfig.Port.Monitor

	nodeRepo.Config.Node.IncentiveAddress = nodeConfig.OperatorAddress

	h.p2pAddresses = append(h.p2pAddresses, fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", nodeConfig.IP, nodeRepo.Config.Port.P2P, nodeRepo.P2PKeystore.P2PID()))
	return nil
}
