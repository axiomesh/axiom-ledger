package repo

import (
	"os"
	"path"
	"time"

	"github.com/pkg/errors"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-kit/types"
)

type GenesisNodeMetaData struct {
	Name       string `mapstructure:"name" toml:"name"`
	Desc       string `mapstructure:"desc" toml:"desc"`
	ImageURL   string `mapstructure:"image_url" toml:"image_url"`
	WebsiteURL string `mapstructure:"website_url" toml:"website_url"`
}

type GenesisNodeInfo struct {
	// Use BLS12-381(Use of new consensus algorithm)
	ConsensusPubKey string `mapstructure:"consensus_pubkey" toml:"consensus_pubkey"`

	// Use ed25519(Currently used in consensus and p2p)
	P2PPubKey string `mapstructure:"p2p_pubkey" toml:"p2p_pubkey"`

	// Operator address, with permission to manage node (can update)
	OperatorAddress string `mapstructure:"operator_address" toml:"operator_address"`

	// Meta data (can update)
	MetaData GenesisNodeMetaData `mapstructure:"metadata" toml:"metadata"`

	IsDataSyncer   bool              `mapstructure:"is_data_syncer" toml:"is_data_syncer"`
	StakeNumber    *types.CoinNumber `mapstructure:"stake_number" toml:"stake_number"`
	CommissionRate uint64            `mapstructure:"commission_rate" toml:"commission_rate"`

	Primary string   `mapstructure:"primary" toml:"primary"`
	Workers []string `mapstructure:"workers" toml:"workers"`
}

type GenesisConfig struct {
	ChainID            uint64            `mapstructure:"chainid" toml:"chainid"`
	Timestamp          int64             `mapstructure:"timestamp" toml:"timestamp"`
	Axc                *Token            `mapstructure:"axc" toml:"axc"`
	Incentive          *Incentive        `mapstructure:"incentive" toml:"incentive"`
	CouncilMembers     []*CouncilMember  `mapstructure:"council_members" toml:"council_members"`
	SmartAccountAdmin  string            `mapstructure:"smart_account_admin" toml:"smart_account_admin"`
	WhitelistProviders []string          `mapstructure:"whitelist_providers" toml:"whitelist_providers"`
	EpochInfo          *types.EpochInfo  `mapstructure:"epoch_info" toml:"epoch_info"`
	Nodes              []GenesisNodeInfo `mapstructure:"nodes" toml:"nodes"`
	Accounts           []*Account        `mapstructure:"accounts" toml:"accounts"`
}

type Token struct {
}

type Incentive struct {
	Referral *Referral `mapstructure:"referral" toml:"referral"`
}

type Referral struct {
	AvgBlockReward string `mapstructure:"avg_block_reward" toml:"avg_block_reward"`
	BlockToNone    uint64 `mapstructure:"block_to_none" toml:"block_to_none"`
}

type CouncilMember struct {
	Address string `mapstructure:"address" toml:"address"`
	Weight  uint64 `mapstructure:"weight" toml:"weight"`
	Name    string `mapstructure:"name" toml:"name"`
}

type Account struct {
	Address string            `mapstructure:"address" toml:"address"`
	Balance *types.CoinNumber `mapstructure:"balance" toml:"balance"`
}

func GenesisEpochInfo() *types.EpochInfo {
	return &types.EpochInfo{
		Epoch:       1,
		EpochPeriod: 100,
		StartBlock:  0,
		ConsensusParams: types.ConsensusParams{
			ProposerElectionType:          types.ProposerElectionTypeWRF,
			CheckpointPeriod:              1,
			HighWatermarkCheckpointPeriod: 10,
			MinValidatorNum:               4,
			MaxValidatorNum:               10,
			BlockMaxTxNum:                 500,
			EnableTimedGenEmptyBlock:      false,
			NotActiveWeight:               1,
			AbnormalNodeExcludeView:       10,
			AgainProposeIntervalBlockInValidatorsNumPercentage: 30,
			ContinuousNullRequestToleranceNumber:               3,
			ReBroadcastToleranceNumber:                         2,
		},
		FinanceParams: types.FinanceParams{
			GasLimit:    0x5f5e100,
			MinGasPrice: GetDefaultMinGasPrice(),
		},
		StakeParams: types.StakeParams{
			StakeEnable:           true,
			MaxAddStakeRatio:      10000,
			MaxUnlockStakeRatio:   1000,
			MaxUnlockingRecordNum: 5,
			// 3 days
			UnlockPeriod:                     uint64(3*24*time.Hour) / uint64(time.Second),
			MaxPendingInactiveValidatorRatio: 1000,
			MinDelegateStake:                 types.CoinNumberByAxc(100),
			MinValidatorStake:                types.CoinNumberByAxc(10000000),
			MaxValidatorStake:                types.CoinNumberByAxc(50000000),
			EnablePartialUnlock:              false,
		},
		MiscParams: types.MiscParams{
			TxMaxSize: DefaultTxMaxSize,
		},
	}
}

func DefaultGenesisConfig() *GenesisConfig {
	if testNetGenesisBuilder, ok := TestNetGenesisConfigBuilderMap[BuildNet]; ok {
		return testNetGenesisBuilder()
	}
	return defaultGenesisConfig()
}

func defaultGenesisConfig() *GenesisConfig {
	return &GenesisConfig{
		ChainID:   1356,
		Timestamp: 1704038400,
		Axc:       &Token{},
		Incentive: &Incentive{
			Referral: &Referral{
				AvgBlockReward: "0",
				BlockToNone:    0,
			},
		},
		CouncilMembers:     []*CouncilMember{},
		SmartAccountAdmin:  "0x0000000000000000000000000000000000000000",
		WhitelistProviders: []string{},
		EpochInfo:          GenesisEpochInfo(),
		Accounts:           []*Account{},
		Nodes:              []GenesisNodeInfo{},
	}
}

func LoadGenesisConfig(repoRoot string) (*GenesisConfig, error) {
	genesis, err := func() (*GenesisConfig, error) {
		genesis := DefaultGenesisConfig()
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
			if err := ReadConfigFromFile(cfgPath, genesis); err != nil {
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
