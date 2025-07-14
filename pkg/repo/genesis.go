package repo

import (
	"math/big"
	"os"
	"path"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/fileutil"
)

type GenesisConfig struct {
	ChainID                uint64          `mapstructure:"chainid" toml:"chainid"`
	Timestamp              int64           `mapstructure:"timestamp" toml:"timestamp"`
	NativeToken            *Token          `mapstructure:"nativeToken" toml:"nativeToken"`
	Axc                    *Token          `mapstructure:"axc" toml:"axc"`
	Incentive              *Incentive      `mapstructure:"incentive" toml:"incentive"`
	Balance                string          `mapstructure:"balance" toml:"balance"`
	Admins                 []*Admin        `mapstructure:"admins" toml:"admins"`
	SmartAccountAdmin      string          `mapstructure:"smart_account_admin" toml:"smart_account_admin"`
	InitWhiteListProviders []string        `mapstructure:"init_white_list_providers" toml:"init_white_list_providers"`
	Accounts               []*Account      `mapstructure:"accounts" toml:"accounts"`
	EpochInfo              *rbft.EpochInfo `mapstructure:"epoch_info" toml:"epoch_info"`
	NodeNames              []*NodeName     `mapstructure:"node_names" toml:"node_names"`
}

type Account struct {
	Address string `mapstructure:"address" toml:"address"`
	Balance string `mapstructure:"balance" toml:"balance"`
}

type Token struct {
	Name        string `mapstructure:"name" toml:"name"`
	Symbol      string `mapstructure:"symbol" toml:"symbol"`
	Decimals    uint8  `mapstructure:"decimals" toml:"decimals"`
	TotalSupply string `mapstructure:"total_supply" toml:"total_supply"`
}

type Incentive struct {
	Mining          *Mining          `mapstructure:"mining" toml:"mining"`
	UserAcquisition *UserAcquisition `mapstructure:"user_acquisition" toml:"user_acquisition"`
	Distributions   []*Distribution  `mapstructure:"distributions" toml:"distributions"`
}

type Mining struct {
	BlockNumToHalf uint64 `mapstructure:"block_num_to_half" toml:"block_num_to_half"`
	BlockNumToNone uint64 `mapstructure:"block_num_to_none" toml:"block_num_to_none"`
	TotalAmount    string `mapstructure:"total_amount" toml:"total_amount"`
}

type UserAcquisition struct {
	AvgBlockReward string `mapstructure:"avg_block_reward" toml:"avg_block_reward"`
	BlockToNone    uint64 `mapstructure:"block_to_none" toml:"block_to_none"`
}

type Distribution struct {
	Name         string  `mapstructure:"name" toml:"name"`
	Addr         string  `mapstructure:"addr" toml:"addr"`
	Percentage   float64 `mapstructure:"percentage" toml:"percentage"`
	InitEmission float64 `mapstructure:"init_emission" toml:"init_emission"`
	Locked       bool    `mapstructure:"locked" toml:"locked"`
}

type Admin struct {
	Address string `mapstructure:"address" toml:"address"`
	Weight  uint64 `mapstructure:"weight" toml:"weight"`
	Name    string `mapstructure:"name" toml:"name"`
}

type NodeName struct {
	ID   uint64 `mapstructure:"id" toml:"id"`
	Name string `mapstructure:"name" toml:"name"`
}

func GenesisEpochInfo(epochEnable bool) *rbft.EpochInfo {
	var candidateSet, validatorSet []rbft.NodeInfo
	if epochEnable {
		candidateSet = lo.Map(DefaultNodeAddrs[4:], func(item string, idx int) rbft.NodeInfo {
			idx += 4
			return rbft.NodeInfo{
				ID:                   uint64(idx + 1),
				AccountAddress:       DefaultNodeAddrs[idx],
				P2PNodeID:            defaultNodeIDs[idx],
				ConsensusVotingPower: int64(len(DefaultNodeAddrs)-idx) * 1000,
			}
		})
		validatorSet = lo.Map(DefaultNodeAddrs[0:4], func(item string, idx int) rbft.NodeInfo {
			return rbft.NodeInfo{
				ID:                   uint64(idx + 1),
				AccountAddress:       DefaultNodeAddrs[idx],
				P2PNodeID:            defaultNodeIDs[idx],
				ConsensusVotingPower: int64(len(DefaultNodeAddrs)-idx) * 1000,
			}
		})
	} else {
		validatorSet = lo.Map(DefaultNodeAddrs[0:4], func(item string, idx int) rbft.NodeInfo {
			return rbft.NodeInfo{
				ID:                   uint64(idx + 1),
				AccountAddress:       DefaultNodeAddrs[idx],
				P2PNodeID:            defaultNodeIDs[idx],
				ConsensusVotingPower: 1000,
			}
		})
	}

	return &rbft.EpochInfo{
		Version:     1,
		Epoch:       1,
		EpochPeriod: 10,
		StartBlock:  1,
		ConsensusParams: rbft.ConsensusParams{
			ProposerElectionType:          rbft.ProposerElectionTypeWRF,
			ValidatorElectionType:         rbft.ValidatorElectionTypeWRF,
			CheckpointPeriod:              1,
			HighWatermarkCheckpointPeriod: 10,
			MaxValidatorNum:               4,
			BlockMaxTxNum:                 500,
			EnableTimedGenEmptyBlock:      false,
			NotActiveWeight:               1,
			AbnormalNodeExcludeView:       10,
			AgainProposeIntervalBlockInValidatorsNumPercentage: 30,
			ContinuousNullRequestToleranceNumber:               3,
			ReBroadcastToleranceNumber:                         2,
		},
		CandidateSet: candidateSet,
		ValidatorSet: validatorSet,
		FinanceParams: rbft.FinanceParams{
			GasLimit:               0x5f5e100,
			StartGasPriceAvailable: true,
			StartGasPrice:          DefaultStartGasPrice,
			MaxGasPrice:            DefaultMaxGasPrice,
			MinGasPrice:            DefaultMinGasPrice,
			GasChangeRateValue:     1250,
			GasChangeRateDecimals:  4,
		},
		MiscParams: rbft.MiscParams{
			TxMaxSize: DefaultTxMaxSize,
		},
	}
}

func DefaultGenesisConfig(epochEnable bool) *GenesisConfig {
	if testNetGenesisBuilder, ok := TestNetGenesisConfigBuilderMap[BuildNet]; ok {
		return testNetGenesisBuilder()
	}
	axmBalance, _ := new(big.Int).SetString(DefaultAXMBalance, 10)
	adminLen := 4
	accountAddrs := []string{
		"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		"0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
		"0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC",
		"0x90F79bf6EB2c4f870365E785982E1f101E93b906",
		"0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65",
		"0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc",
		"0x976EA74026E726554dB657fA54763abd0C3a0aa9",
		"0x14dC79964da2C08b23698B3D3cc7Ca32193d9955",
		"0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f",
		"0xa0Ee7A142d267C1f36714E4a8F75612F20a79720",
		"0xBcd4042DE499D14e55001CcbB24a551F3b954096",
		"0x71bE63f3384f5fb98995898A86B02Fb2426c5788",
		"0xFABB0ac9d68B0B445fB7357272Ff202C5651694a",
		"0x1CBd3b2770909D4e10f157cABC84C7264073C9Ec",
		"0xdF3e18d64BC6A983f673Ab319CCaE4f1a57C7097",
		"0xcd3B766CCDd6AE721141F452C550Ca635964ce71",
		"0x2546BcD3c84621e976D8185a91A922aE77ECEc30",
		"0xbDA5747bFD65F08deb54cb465eB87D40e51B197E",
		"0xdD2FD4581271e230360230F9337D5c0430Bf44C0",
		"0x8626f6940E2eb28930eFb4CeF49B2d1F2C9C1199",
	}

	adminAddrs := DefaultNodeAddrs[0:adminLen]
	// adminAddrs + accountAddrs
	allAccounts := append(adminAddrs, accountAddrs...)

	accounts := lo.Map(allAccounts, func(addr string, idx int) *Account {
		return &Account{
			Address: addr,
			Balance: axmBalance.String(),
		}
	})

	totalSupply := new(big.Int).Mul(axmBalance, big.NewInt(int64(len(allAccounts))))
	return &GenesisConfig{
		ChainID:   1356,
		Timestamp: 1704038400,
		Admins: lo.Map(DefaultNodeAddrs[0:adminLen], func(item string, idx int) *Admin {
			return &Admin{
				Address: item,
				Weight:  1,
				Name:    DefaultAdminNames[idx],
			}
		}),
		SmartAccountAdmin: DefaultNodeAddrs[0],
		NativeToken: &Token{
			Name:        "Axiom",
			Symbol:      "AXM",
			Decimals:    DefaultDecimals,
			TotalSupply: totalSupply.String(),
		},
		Axc: &Token{
			Name:        "Axiomesh Credit",
			Symbol:      "axc",
			Decimals:    DefaultDecimals,
			TotalSupply: DefaultAXCTotalSupply,
		},
		Incentive: &Incentive{
			Mining: &Mining{
				BlockNumToHalf: 126144000,
				BlockNumToNone: 630720001,
				TotalAmount:    "40000000000000000000000000",
			},
			UserAcquisition: &UserAcquisition{
				AvgBlockReward: "126000000000000000",
				BlockToNone:    315360000,
			},
			Distributions: lo.Map(DefaultAXCDistribution, func(item Distribution, _ int) *Distribution {
				return &Distribution{
					Name:         item.Name,
					Addr:         item.Addr,
					Percentage:   item.Percentage,
					InitEmission: item.InitEmission,
					Locked:       item.Locked,
				}
			}),
		},
		NodeNames:              GenesisNodeNameInfo(epochEnable),
		InitWhiteListProviders: DefaultNodeAddrs,
		Accounts:               accounts,
		EpochInfo:              GenesisEpochInfo(epochEnable),
	}
}

func LoadGenesisConfig(repoRoot string) (*GenesisConfig, error) {
	genesis, err := func() (*GenesisConfig, error) {
		genesis := DefaultGenesisConfig(false)
		cfgPath := path.Join(repoRoot, genesisCfgFileName)
		existConfig := fileutil.Exist(cfgPath)
		if !existConfig {
			err := os.MkdirAll(repoRoot, 0755)
			if err != nil {
				return nil, errors.Wrap(err, "failed to build default config")
			}

			if err := writeConfigWithEnv(cfgPath, genesis); err != nil {
				return nil, errors.Wrap(err, "failed to build default genesis config")
			}
		} else {
			if err := CheckWritable(repoRoot); err != nil {
				return nil, err
			}
			if err := readConfigFromFile(cfgPath, genesis); err != nil {
				return nil, err
			}
		}

		return genesis, nil
	}()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load genesis config")
	}
	return genesis, nil
}

func GenesisNodeNameInfo(epochEnable bool) []*NodeName {
	var nodes []*NodeName
	var sliceLength int
	if epochEnable {
		sliceLength = len(DefaultNodeAddrs)
	} else {
		sliceLength = 4
	}
	nodes = lo.Map(DefaultNodeAddrs[:sliceLength], func(item string, idx int) *NodeName {
		return &NodeName{
			Name: DefaultNodeNames[idx],
			ID:   uint64(idx + 1),
		}
	})
	return nodes
}
