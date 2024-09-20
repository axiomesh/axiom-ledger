package main

import (
	"crypto/ecdsa"
	_ "embed"
	"fmt"
	"github.com/axiomesh/axiom-kit/types"
	"os"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-ledger/cmd/axiom-ledger/common"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var (
	defaultPort = &ClusterNodePort{
		JsonRpc:   8881,
		WebSocket: 9991,
		P2P:       4001,
		PProf:     53121,
		Monitor:   40011,
	}

	defaultCouncilMemberNames = []string{
		"S2luZw==", // base64 encode King
		"UmVk",     // base64 encode Red
		"QXBwbGU=", // base64 encode Apple
		"Q2F0",     // base64 encode Cat
	}

	defaultCouncilMemberKeys = []string{
		"b6477143e17f889263044f6cf463dc37177ac4526c4c39a7a344198457024a2f",
		"05c3708d30c2c72c4b36314a41f30073ab18ea226cf8c6b9f566720bfe2e8631",
		"85a94dd51403590d4f149f9230b6f5de3a08e58899dcaf0f77768efb1825e854",
		"72efcf4bb0e8a300d3e47e6a10f630bcd540de933f01ed5380897fc5e10dc95d",
	}

	defaultCouncilMemberAddrs = []string{
		"0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013",
		"0x79a1215469FaB6f9c63c1816b45183AD3624bE34",
		"0x97c8B516D19edBf575D72a172Af7F418BE498C37",
		"0xc0Ff2e0b3189132D815b8eb325bE17285AC898f8",
	}
)

//go:embed cluster_generate_config_temp.toml
var clusterGenerateConfigTemp string

var clusterGenerateTargetDir string

var clusterGenerateConfigPath string

var clusterGenerateNodeNumber uint64

func clusterGenerateTargetDirFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "target",
		Usage:       "Generate nodes config and keystore to Target dir",
		Destination: &clusterGenerateTargetDir,
		Required:    true,
	}
}

var clusterGenerateForce bool

func clusterGenerateForceFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:        "force",
		Usage:       "Clear existing configuration and overwrite it",
		Destination: &clusterGenerateForce,
		Required:    false,
	}
}

var clusterCMD = &cli.Command{
	Name:  "cluster",
	Usage: "The cluster helper commands(generate cluster nodes config and keystore)",
	Subcommands: []*cli.Command{
		{
			Name:   "generate-default",
			Usage:  "Generate default 4 nodes config and keystore(support use env to set other config)",
			Action: generateDefault,
			Flags: []cli.Flag{
				clusterGenerateTargetDirFlag(),
				clusterGenerateForceFlag(),
				common.KeystorePasswordFlag(),
			},
		},
		{
			Name:   "quick-generate",
			Usage:  "Quick generate nodes config and keystore by specifying the number of nodes(support use env to set other config)",
			Action: quickGenerate,
			Flags: []cli.Flag{
				clusterGenerateTargetDirFlag(),
				clusterGenerateForceFlag(),
				common.KeystorePasswordFlag(),
				&cli.Uint64Flag{
					Name:        "node-number",
					Usage:       "Generate nodes number",
					Destination: &clusterGenerateNodeNumber,
					Required:    true,
				},
			},
		},
		{
			Name:   "generate-by-config",
			Usage:  "Generate nodes config and keystore by generate config file(support use env to set other config)",
			Action: generateByConfig,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "config-path",
					Usage:       "Generate from config file path",
					Destination: &clusterGenerateConfigPath,
					Required:    false,
				},
				clusterGenerateTargetDirFlag(),
				clusterGenerateForceFlag(),
				common.KeystorePasswordFlag(),
			},
		},
		{
			Name:   "show-config-template",
			Usage:  "Show cluster generate config template",
			Action: showConfigTemplate,
		},
	},
}

func generateDefault(ctx *cli.Context) error {
	if clusterGenerateTargetDir == "" {
		return errors.New("target dir is empty")
	}

	defaultCouncilMemberKeyMap := make(map[uint64]*ecdsa.PrivateKey)
	for i, keyStr := range defaultCouncilMemberKeys {
		key, err := ethcrypto.HexToECDSA(keyStr)
		if err != nil {
			return errors.Wrapf(err, "failed to parse private key %s", keyStr)
		}
		defaultCouncilMemberKeyMap[uint64(i+1)] = key
	}
	helper := NewClusterGeneratorHelper(&ClusterGenerateConfig{
		DefaultPort:               defaultPort,
		EnablePortAutoIncrease:    true,
		MintForOperatorCoinAmount: repo.GetDefaultAccountBalance(),
		Accounts: lo.Map(append(defaultCouncilMemberAddrs, repo.MockDefaultAccountAddrs...), func(addr string, idx int) *ClusterAccount {
			balance := repo.GetDefaultAccountBalance()
			if addr == "0x70997970C51812dc3A010C7d01b50e0d17dc79C8" {
				balance = types.CoinNumberByAxc(18446744073709551615)
			}
			return &ClusterAccount{
				Address: addr,
				Balance: balance,
			}
		}),
		CouncilMembers: lo.Map(defaultCouncilMemberAddrs, func(item string, idx int) *ClusterCouncilMember {
			return &ClusterCouncilMember{
				Address: item,
				Weight:  1,
				Name:    defaultCouncilMemberNames[idx],
			}
		}),
		Nodes: []*ClusterNode{
			{
				Name:                "node1",
				Desc:                "node1",
				P2PPrivateKey:       "0xce374993d8867572a043e443355400ff4628662486d0d6ef9d76bc3c8b2aa8a8",
				ConsensusPrivateKey: "0x099383c2b41a282936fe9e656467b2ad6ecafd38753eefa080b5a699e3276372",
				OperatorAddress:     defaultCouncilMemberAddrs[0],
				IsDataSyncer:        false,
				StakeNumber:         repo.GetDefaultAccountBalance(),
				IP:                  "127.0.0.1",
				Port:                ClusterNodePort{},
			},
			{
				P2PPrivateKey:       "0x43dd946ade57013fd4e7d0f11d84b94e2fda4336829f154ae345be94b0b63616",
				ConsensusPrivateKey: "0x5d21b741bd16e05c3a883b09613d36ad152f1586393121d247bdcfef908cce8f",
				OperatorAddress:     defaultCouncilMemberAddrs[1],
				IsDataSyncer:        false,
				StakeNumber:         repo.GetDefaultAccountBalance(),
				IP:                  "127.0.0.1",
				Port:                ClusterNodePort{},
			},
			{
				P2PPrivateKey:       "0x875e5ef34c34e49d35ff5a0f8a53003d8848fc6edd423582c00edc609a1e3239",
				ConsensusPrivateKey: "0x42cc8e862b51a1c21a240bb2ae6f2dbad59668d86fe3c45b2e4710eebd2a63fd",
				OperatorAddress:     defaultCouncilMemberAddrs[2],
				IsDataSyncer:        false,
				StakeNumber:         repo.GetDefaultAccountBalance(),
				IP:                  "127.0.0.1",
				Port:                ClusterNodePort{},
			},
			{
				P2PPrivateKey:       "0xf0aac0c25791d0bd1b96b2ec3c9c25539045cf6cc5cc9ad0f3cb64453d1f38c0",
				ConsensusPrivateKey: "0x6e327c2d5a284b89f9c312a02b2714a90b38e721256f9a157f03ec15c1a386a6",
				OperatorAddress:     defaultCouncilMemberAddrs[3],
				IsDataSyncer:        false,
				StakeNumber:         repo.GetDefaultAccountBalance(),
				IP:                  "127.0.0.1",
				Port:                ClusterNodePort{},
			},
		},
	}, common.KeystorePasswordFlagVar, clusterGenerateTargetDir, clusterGenerateForce, defaultCouncilMemberKeyMap)

	return helper.Generate()
}

func quickGenerate(ctx *cli.Context) error {
	if clusterGenerateTargetDir == "" {
		return errors.New("target dir is empty")
	}

	if clusterGenerateNodeNumber == 0 {
		return errors.New("node number is zero")
	}

	helper := NewClusterGeneratorHelper(&ClusterGenerateConfig{
		EnablePortAutoIncrease:    true,
		MintForOperatorCoinAmount: repo.GetDefaultAccountBalance(),
		Nodes: lo.RepeatBy(int(clusterGenerateNodeNumber), func(index int) *ClusterNode {
			return &ClusterNode{}
		}),
	}, common.KeystorePasswordFlagVar, clusterGenerateTargetDir, clusterGenerateForce, nil)

	return helper.Generate()
}

func generateByConfig(ctx *cli.Context) error {
	if clusterGenerateTargetDir == "" {
		return errors.New("target dir is empty")
	}
	if clusterGenerateConfigPath == "" {
		return errors.New("config path is empty")
	}
	cfgContent, err := os.ReadFile(clusterGenerateConfigPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read config from %s", clusterGenerateConfigPath)
	}

	var cfg ClusterGenerateConfig
	if err := toml.Unmarshal(cfgContent, &cfg); err != nil {
		return errors.Wrapf(err, "failed to unmarshal config from %s", clusterGenerateConfigPath)
	}

	helper := NewClusterGeneratorHelper(&cfg, common.KeystorePasswordFlagVar, clusterGenerateTargetDir, clusterGenerateForce, nil)
	return helper.Generate()
}

func showConfigTemplate(ctx *cli.Context) error {
	fmt.Println(clusterGenerateConfigTemp)
	return nil
}
