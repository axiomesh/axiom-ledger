package repo

import (
	"math/big"
	"time"

	rbft "github.com/axiomesh/axiom-bft"
	network "github.com/axiomesh/axiom-p2p"
	"github.com/samber/lo"
)

const (
	AriesTestnetName    = "aries"
	TaurusTestnetName   = "taurus"
	GeminiTestnetName   = "gemini"
	DraconisTestnetName = "draconis-testnet"
)

var (
	TestNetConfigBuilderMap = map[string]func() *Config{
		AriesTestnetName:    AriesConfig,
		TaurusTestnetName:   TaurusConfig,
		GeminiTestnetName:   GeminiConfig,
		DraconisTestnetName: DraconisTestNetConfig,
	}

	TestNetConsensusConfigBuilderMap = map[string]func() *ConsensusConfig{
		AriesTestnetName:    AriesConsensusConfig,
		TaurusTestnetName:   TaurusConsensusConfig,
		GeminiTestnetName:   GeminiConsensusConfig,
		DraconisTestnetName: DraconisTestNetConsensusConfig,
	}

	TestNetGenesisConfigBuilderMap = map[string]func() *GenesisConfig{
		//AriesTestnetName:  AriesGenesisConfig,
		TaurusTestnetName:   TaurusGenesisConfig,
		GeminiTestnetName:   GeminiGenesisConfig,
		DraconisTestnetName: DraconisTestNetGenesisConfig,
	}
)

func AriesConfig() *Config {
	// nolint
	return &Config{
		Ulimit: 65535,
		Access: Access{
			EnableWhiteList: false,
		},
		Port: Port{
			JsonRpc:   8881,
			WebSocket: 9991,
			P2P:       4001,
			PProf:     53121,
			Monitor:   40011,
		},
		JsonRPC: JsonRPC{
			GasCap:     300000000,
			EVMTimeout: Duration(5 * time.Second),
			ReadLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			WriteLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			RejectTxsIfConsensusAbnormal: false,
		},
		P2P: P2P{
			Security:        P2PSecurityTLS,
			SendTimeout:     Duration(5 * time.Second),
			ReadTimeout:     Duration(5 * time.Second),
			CompressionAlgo: network.SnappyCompression,
			EnableMetrics:   true,
			Pipe: P2PPipe{
				ReceiveMsgCacheSize: 1024,
				BroadcastType:       P2PPipeBroadcastGossip,
				SimpleBroadcast: P2PPipeSimpleBroadcast{
					WorkerCacheSize:        1024,
					WorkerConcurrencyLimit: 20,
				},
				Gossipsub: P2PPipeGossipsub{
					SubBufferSize:          1024,
					PeerOutboundBufferSize: 1024,
					ValidateBufferSize:     1024,
					SeenMessagesTTL:        Duration(120 * time.Second),
					EnableMetrics:          false,
				},
				UnicastReadTimeout:       Duration(5 * time.Second),
				UnicastSendRetryNumber:   5,
				UnicastSendRetryBaseTime: Duration(100 * time.Millisecond),
				FindPeerTimeout:          Duration(10 * time.Second),
				ConnectTimeout:           Duration(1 * time.Second),
			},
		},
		Sync: Sync{
			WaitStatesTimeout:     Duration(30 * time.Second),
			RequesterRetryTimeout: Duration(5 * time.Second),
			TimeoutCountLimit:     uint64(10),
			ConcurrencyLimit:      1000,
		},
		Consensus: Consensus{
			Type:        ConsensusTypeRbft,
			StorageType: ConsensusStorageTypeMinifile,
		},
		Storage: Storage{
			KvType: KVStorageTypeLeveldb,
		},
		Ledger: Ledger{
			ChainLedgerCacheSize: 100,
			// StateLedgerCacheMegabytesLimit: 128,
			StateLedgerAccountCacheSize: 1024,
		},
		Executor: Executor{
			Type:            ExecTypeNative,
			DisableRollback: false,
		},
		PProf: PProf{
			Enable:   true,
			PType:    PprofTypeHTTP,
			Mode:     PprofModeMem,
			Duration: Duration(30 * time.Second),
		},
		Monitor: Monitor{
			Enable: true,
		},
		Log: Log{
			Level:            "info",
			Filename:         "draconis",
			ReportCaller:     false,
			EnableCompress:   false,
			EnableColor:      true,
			DisableTimestamp: false,
			MaxAge:           30,
			MaxSize:          128,
			RotationTime:     Duration(24 * time.Hour),
			Module: LogModule{
				P2P:        "info",
				Consensus:  "debug",
				Executor:   "info",
				Governance: "info",
				API:        "error",
				CoreAPI:    "info",
				Storage:    "info",
				Profile:    "info",
				Finance:    "error",
				BlockSync:  "info",
				APP:        "info",
				Access:     "info",
				TxPool:     "info",
				Epoch:      "info",
			},
		},
	}
}

func AriesConsensusConfig() *ConsensusConfig {
	// nolint
	return &ConsensusConfig{
		TimedGenBlock: TimedGenBlock{
			NoTxBatchTimeout: Duration(2 * time.Second),
		},
		Limit: ReceiveMsgLimiter{
			Enable: false,
			Limit:  10000,
			Burst:  10000,
		},
		TxPool: TxPool{
			PoolSize:               50000,
			ToleranceTime:          Duration(5 * time.Minute),
			ToleranceRemoveTime:    Duration(15 * time.Minute),
			CleanEmptyAccountTime:  Duration(10 * time.Minute),
			RotateTxLocalsInterval: Duration(1 * time.Hour),
			ToleranceNonceGap:      1000,
			EnableLocalsPersist:    true,
			PriceLimit:             DefaultMinGasPrice,
			GenerateBatchType:      GenerateBatchByTime,
			PriceBump:              10,
		},
		TxCache: TxCache{
			SetSize:    50,
			SetTimeout: Duration(100 * time.Millisecond),
		},
		Rbft: RBFT{
			EnableMultiPipes:          false,
			EnableMetrics:             true,
			CommittedBlockCacheNumber: 10,
			Timeout: RBFTTimeout{
				NullRequest:      Duration(9 * time.Second),
				Request:          Duration(6 * time.Second),
				ResendViewChange: Duration(10 * time.Second),
				CleanViewChange:  Duration(60 * time.Second),
				NewView:          Duration(8 * time.Second),
				SyncState:        Duration(1 * time.Second),
				SyncStateRestart: Duration(10 * time.Minute),
				FetchCheckpoint:  Duration(5 * time.Second),
				FetchView:        Duration(1 * time.Second),
				BatchTimeout:     Duration(500 * time.Millisecond),
			},
		},
		Solo: Solo{
			BatchTimeout: Duration(500 * time.Millisecond),
		},
	}
}

func AriesGenesisConfig() *GenesisConfig {
	axmBalance, _ := new(big.Int).SetString(DefaultAXMBalance, 10)
	// balance = axmBalance * 10^decimals
	balance := new(big.Int).Mul(axmBalance, big.NewInt(10).Exp(big.NewInt(10), big.NewInt(int64(DefaultDecimals)), nil))
	adminLen := 4
	totalSupply := new(big.Int).Mul(balance, big.NewInt(int64(adminLen)))
	// nolint
	return &GenesisConfig{
		ChainID: 23411,
		Admins: []*Admin{
			{
				Address: "0xecFE18Dc453CCdF96f1b9b58ccb4db3c6115A1D0",
				Weight:  1,
				Name:    "S2luZw==",
			},
			{
				Address: "0x13f30647b99Edeb8CF3725eCd1Eaf545D9283335",
				Weight:  1,
				Name:    "UmVk",
			},
			{
				Address: "0x6cdB717de826334faD8FB0ce0547Bac0230ba5a4",
				Weight:  1,
				Name:    "QXBwbGU=",
			},
			{
				Address: "0xAc7DD5009788f2CB14db8dCd6728d94Cbd4d705e",
				Weight:  1,
				Name:    "Q2F0",
			},
		},
		SmartAccountAdmin: "0xecFE18Dc453CCdF96f1b9b58ccb4db3c6115A1D0",
		NativeToken: &Token{
			Name:        "Axiom",
			Symbol:      "Token",
			Decimals:    DefaultDecimals,
			TotalSupply: totalSupply.String(),
		},
		Accounts: []*Account{},
		EpochInfo: &rbft.EpochInfo{
			Version:     1,
			Epoch:       1,
			EpochPeriod: 100000000,
			StartBlock:  1,
			P2PBootstrapNodeAddresses: []string{
				"/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAm9VjBKpMJyzXUzLCd4wWigkPD9HHUEmg628pPxGhkyoVg",
				"/ip4/127.0.0.1/tcp/4002/p2p/16Uiu2HAmQ2EnGWAeRLNB8ZfiPAQxwofWbCpA7sfQymTGbbe64z4G",
				"/ip4/127.0.0.1/tcp/4003/p2p/16Uiu2HAkwWBiECscWVK3mp3xTUpGdx5qkBs91RbhT2psQAZHkx5i",
				"/ip4/127.0.0.1/tcp/4004/p2p/16Uiu2HAm3ikUE3LjJeatMMgDuV2cAG9da8ZJJFLA8nBy6qcN1MMg",
			},
			ConsensusParams: rbft.ConsensusParams{
				ValidatorElectionType:         rbft.ValidatorElectionTypeWRF,
				ProposerElectionType:          rbft.ProposerElectionTypeAbnormalRotation,
				CheckpointPeriod:              10,
				HighWatermarkCheckpointPeriod: 4,
				MaxValidatorNum:               20,
				BlockMaxTxNum:                 500,
				EnableTimedGenEmptyBlock:      false,
				NotActiveWeight:               1,
				AbnormalNodeExcludeView:       1,
				AgainProposeIntervalBlockInValidatorsNumPercentage: 30,
				ContinuousNullRequestToleranceNumber:               3,
				ReBroadcastToleranceNumber:                         2,
			},
			CandidateSet: []rbft.NodeInfo{},
			ValidatorSet: []rbft.NodeInfo{
				{
					ID:                   1,
					AccountAddress:       "0xecFE18Dc453CCdF96f1b9b58ccb4db3c6115A1D0",
					P2PNodeID:            "16Uiu2HAm9VjBKpMJyzXUzLCd4wWigkPD9HHUEmg628pPxGhkyoVg",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   2,
					AccountAddress:       "0x13f30647b99Edeb8CF3725eCd1Eaf545D9283335",
					P2PNodeID:            "16Uiu2HAmQ2EnGWAeRLNB8ZfiPAQxwofWbCpA7sfQymTGbbe64z4G",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   3,
					AccountAddress:       "0x6cdB717de826334faD8FB0ce0547Bac0230ba5a4",
					P2PNodeID:            "16Uiu2HAkwWBiECscWVK3mp3xTUpGdx5qkBs91RbhT2psQAZHkx5i",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   4,
					AccountAddress:       "0xAc7DD5009788f2CB14db8dCd6728d94Cbd4d705e",
					P2PNodeID:            "16Uiu2HAm3ikUE3LjJeatMMgDuV2cAG9da8ZJJFLA8nBy6qcN1MMg",
					ConsensusVotingPower: 1000,
				},
			},
			FinanceParams: rbft.FinanceParams{
				GasLimit:               0x5f5e100,
				StartGasPriceAvailable: true,
				StartGasPrice:          5000000000000,
				MaxGasPrice:            10000000000000,
				MinGasPrice:            1000000000000,
				GasChangeRateValue:     1250,
				GasChangeRateDecimals:  4,
			},
			MiscParams: rbft.MiscParams{
				TxMaxSize: DefaultTxMaxSize,
			},
		},
	}
}

func TaurusConfig() *Config {
	// nolint
	return &Config{
		Ulimit: 65535,
		Access: Access{
			EnableWhiteList: false,
		},
		Port: Port{
			JsonRpc:   8881,
			WebSocket: 9991,
			P2P:       4001,
			PProf:     53121,
			Monitor:   40011,
		},
		JsonRPC: JsonRPC{
			GasCap:     300000000,
			EVMTimeout: Duration(5 * time.Second),
			ReadLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			WriteLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			RejectTxsIfConsensusAbnormal: false,
		},
		P2P: P2P{
			BootstrapNodeAddresses: []string{
				"/ip4/18.162.168.102/tcp/4001/p2p/16Uiu2HAmUM4Po1HP2UF8QaCzQBe4qCPiXFZaxcZDFCo4SXSU2aqB",
				"/ip4/16.162.105.179/tcp/4001/p2p/16Uiu2HAmS8wnz2DWcNKiT9kiEQHpE1xaD6p42mvKE7mDKrFGo3QW",
				"/ip4/43.198.79.250/tcp/4001/p2p/16Uiu2HAm4pPKpHEPZWNga6VKCtT8YQMwZb7bu8Hj9QtwNgoy5ajN",
				"/ip4/18.162.169.230/tcp/4001/p2p/16Uiu2HAmMF4Xcnh4VTBLGVRwTeHVCb5z8vpHKWS8hE5Q5FQNDTSe",
			},
			Security:        P2PSecurityTLS,
			SendTimeout:     Duration(5 * time.Second),
			ReadTimeout:     Duration(5 * time.Second),
			CompressionAlgo: network.SnappyCompression,
			EnableMetrics:   true,
			Pipe: P2PPipe{
				ReceiveMsgCacheSize: 1024,
				BroadcastType:       P2PPipeBroadcastGossip,
				SimpleBroadcast: P2PPipeSimpleBroadcast{
					WorkerCacheSize:        1024,
					WorkerConcurrencyLimit: 20,
				},
				Gossipsub: P2PPipeGossipsub{
					SubBufferSize:          1024,
					PeerOutboundBufferSize: 1024,
					ValidateBufferSize:     1024,
					SeenMessagesTTL:        Duration(120 * time.Second),
					EnableMetrics:          false,
				},
				UnicastReadTimeout:       Duration(5 * time.Second),
				UnicastSendRetryNumber:   5,
				UnicastSendRetryBaseTime: Duration(100 * time.Millisecond),
				FindPeerTimeout:          Duration(10 * time.Second),
				ConnectTimeout:           Duration(1 * time.Second),
			},
		},
		Sync: Sync{
			WaitStatesTimeout:     Duration(30 * time.Second),
			RequesterRetryTimeout: Duration(5 * time.Second),
			TimeoutCountLimit:     uint64(10),
			ConcurrencyLimit:      1000,
		},
		Consensus: Consensus{
			Type:        ConsensusTypeRbft,
			StorageType: ConsensusStorageTypeMinifile,
		},
		Storage: Storage{
			KvType:      KVStorageTypePebble,
			KvCacheSize: 128,
			Sync:        true,
		},
		Ledger: Ledger{
			ChainLedgerCacheSize: 100,
			// StateLedgerCacheMegabytesLimit: 128,
			StateLedgerAccountCacheSize: 1024,
		},
		Executor: Executor{
			Type:            ExecTypeNative,
			DisableRollback: false,
		},
		PProf: PProf{
			Enable:   true,
			PType:    PprofTypeHTTP,
			Mode:     PprofModeMem,
			Duration: Duration(30 * time.Second),
		},
		Monitor: Monitor{
			Enable:          true,
			EnableExpensive: true,
		},
		Log: Log{
			Level:            "info",
			Filename:         "draconis",
			ReportCaller:     false,
			EnableCompress:   false,
			EnableColor:      true,
			DisableTimestamp: false,
			MaxAge:           30,
			MaxSize:          128,
			RotationTime:     Duration(24 * time.Hour),
			Module: LogModule{
				P2P:        "info",
				Consensus:  "debug",
				Executor:   "info",
				Governance: "info",
				API:        "info",
				CoreAPI:    "info",
				Storage:    "info",
				Profile:    "info",
				Finance:    "error",
				BlockSync:  "info",
				APP:        "info",
				Access:     "info",
				TxPool:     "info",
				Epoch:      "info",
			},
		},
	}
}

func TaurusConsensusConfig() *ConsensusConfig {
	// nolint
	return &ConsensusConfig{
		TimedGenBlock: TimedGenBlock{
			NoTxBatchTimeout: Duration(2 * time.Second),
		},
		Limit: ReceiveMsgLimiter{
			Enable: false,
			Limit:  10000,
			Burst:  10000,
		},
		TxPool: TxPool{
			PoolSize:               50000,
			ToleranceTime:          Duration(5 * time.Minute),
			ToleranceRemoveTime:    Duration(15 * time.Minute),
			CleanEmptyAccountTime:  Duration(10 * time.Minute),
			ToleranceNonceGap:      1000,
			EnableLocalsPersist:    true,
			RotateTxLocalsInterval: Duration(10 * time.Hour),
			PriceLimit:             DefaultMinGasPrice,
			PriceBump:              10,
			GenerateBatchType:      GenerateBatchByTime,
		},
		TxCache: TxCache{
			SetSize:    50,
			SetTimeout: Duration(100 * time.Millisecond),
		},
		Rbft: RBFT{
			EnableMultiPipes:          false,
			EnableMetrics:             true,
			CommittedBlockCacheNumber: 10,
			Timeout: RBFTTimeout{
				NullRequest:      Duration(3 * time.Second),
				Request:          Duration(2 * time.Second),
				ResendViewChange: Duration(10 * time.Second),
				CleanViewChange:  Duration(60 * time.Second),
				NewView:          Duration(8 * time.Second),
				SyncState:        Duration(1 * time.Second),
				SyncStateRestart: Duration(10 * time.Minute),
				FetchCheckpoint:  Duration(5 * time.Second),
				FetchView:        Duration(1 * time.Second),
				BatchTimeout:     Duration(500 * time.Millisecond),
			},
		},
		Solo: Solo{
			BatchTimeout: Duration(500 * time.Millisecond),
		},
	}
}

func TaurusGenesisConfig() *GenesisConfig {
	// nolint
	return &GenesisConfig{
		ChainID: 23412,
		Admins: []*Admin{
			{
				Address: "0x83Db4fA2CbB682753C94ca8A809a4a321aA36e1b",
				Weight:  1,
				Name:    "S2luZw==",
			},
			{
				Address: "0xf19C1808C42e7513157be618BD801Bbda72031ee",
				Weight:  1,
				Name:    "UmVk",
			},
			{
				Address: "0x91C8a7e7184a5D77ADC1e98C0522a9046dFda4B2",
				Weight:  1,
				Name:    "QXBwbGU=",
			},
			{
				Address: "0xe73eA9cA988eEA7a17bd402E27C8fABa5879A36F",
				Weight:  1,
				Name:    "Q2F0",
			},
		},
		SmartAccountAdmin:      "0x83Db4fA2CbB682753C94ca8A809a4a321aA36e1b",
		InitWhiteListProviders: []string{},
		Accounts:               []*Account{},
		EpochInfo: &rbft.EpochInfo{
			Version:                   1,
			Epoch:                     1,
			EpochPeriod:               1800,
			StartBlock:                1,
			P2PBootstrapNodeAddresses: []string{},
			ConsensusParams: rbft.ConsensusParams{
				ValidatorElectionType:         rbft.ValidatorElectionTypeWRF,
				ProposerElectionType:          rbft.ProposerElectionTypeWRF,
				CheckpointPeriod:              1,
				HighWatermarkCheckpointPeriod: 10,
				MaxValidatorNum:               8,
				BlockMaxTxNum:                 500,
				EnableTimedGenEmptyBlock:      false,
				NotActiveWeight:               1,
				AbnormalNodeExcludeView:       100,
				AgainProposeIntervalBlockInValidatorsNumPercentage: 30,
				ContinuousNullRequestToleranceNumber:               3,
				ReBroadcastToleranceNumber:                         2,
			},
			FinanceParams: rbft.FinanceParams{
				GasLimit:               100000000,
				StartGasPriceAvailable: true,
				StartGasPrice:          5000000000000,
				MaxGasPrice:            10000000000000,
				MinGasPrice:            1000000000000,
				GasChangeRateValue:     1250,
				GasChangeRateDecimals:  4,
			},
			MiscParams: rbft.MiscParams{
				TxMaxSize: DefaultTxMaxSize,
			},
			ValidatorSet: []rbft.NodeInfo{
				{
					ID:                   1,
					AccountAddress:       "0x83Db4fA2CbB682753C94ca8A809a4a321aA36e1b",
					P2PNodeID:            "16Uiu2HAmUM4Po1HP2UF8QaCzQBe4qCPiXFZaxcZDFCo4SXSU2aqB",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   2,
					AccountAddress:       "0xf19C1808C42e7513157be618BD801Bbda72031ee",
					P2PNodeID:            "16Uiu2HAmS8wnz2DWcNKiT9kiEQHpE1xaD6p42mvKE7mDKrFGo3QW",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   3,
					AccountAddress:       "0x91C8a7e7184a5D77ADC1e98C0522a9046dFda4B2",
					P2PNodeID:            "16Uiu2HAm4pPKpHEPZWNga6VKCtT8YQMwZb7bu8Hj9QtwNgoy5ajN",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   4,
					AccountAddress:       "0xe73eA9cA988eEA7a17bd402E27C8fABa5879A36F",
					P2PNodeID:            "16Uiu2HAmMF4Xcnh4VTBLGVRwTeHVCb5z8vpHKWS8hE5Q5FQNDTSe",
					ConsensusVotingPower: 1000,
				},
			},
			CandidateSet:  []rbft.NodeInfo{},
			DataSyncerSet: []rbft.NodeInfo{},
		},
		NodeNames: []*NodeName{
			{
				ID:   1,
				Name: "S2luZw==",
			},
			{
				ID:   2,
				Name: "UmVk",
			},
			{
				ID:   3,
				Name: "QXBwbGU=",
			},
			{
				ID:   4,
				Name: "Q2F0",
			},
		},
	}
}

func GeminiConfig() *Config {
	// nolint
	return &Config{
		Ulimit: 65535,
		Port: Port{
			JsonRpc:   8881,
			WebSocket: 9991,
			P2P:       4001,
			PProf:     53121,
			Monitor:   40011,
		},
		JsonRPC: JsonRPC{
			GasCap:     300000000,
			EVMTimeout: Duration(5 * time.Second),
			ReadLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			WriteLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			RejectTxsIfConsensusAbnormal: false,
			QueryLimit: QueryLimit{
				GetLogsBlockRangeLimit: 2000,
			},
		},
		P2P: P2P{
			Security:        P2PSecurityTLS,
			SendTimeout:     Duration(5 * time.Second),
			ReadTimeout:     Duration(5 * time.Second),
			CompressionAlgo: network.SnappyCompression,
			EnableMetrics:   true,
			Pipe: P2PPipe{
				ReceiveMsgCacheSize: 10240,
				BroadcastType:       P2PPipeBroadcastGossip,
				SimpleBroadcast: P2PPipeSimpleBroadcast{
					WorkerCacheSize:        1024,
					WorkerConcurrencyLimit: 20,
				},
				Gossipsub: P2PPipeGossipsub{
					SubBufferSize:          10240,
					PeerOutboundBufferSize: 10240,
					ValidateBufferSize:     10240,
					SeenMessagesTTL:        Duration(120 * time.Second),
					EnableMetrics:          true,
				},
				UnicastReadTimeout:       Duration(5 * time.Second),
				UnicastSendRetryNumber:   5,
				UnicastSendRetryBaseTime: Duration(100 * time.Millisecond),
				FindPeerTimeout:          Duration(10 * time.Second),
				ConnectTimeout:           Duration(1 * time.Second),
			},
		},
		Sync: Sync{
			WaitStatesTimeout:     Duration(30 * time.Second),
			RequesterRetryTimeout: Duration(5 * time.Second),
			TimeoutCountLimit:     uint64(10),
			ConcurrencyLimit:      1000,
		},
		Consensus: Consensus{
			Type:        ConsensusTypeRbft,
			StorageType: ConsensusStorageTypeMinifile,
		},
		Storage: Storage{
			KvType:      KVStorageTypePebble,
			KvCacheSize: 128,
			Sync:        true,
		},
		Ledger: Ledger{
			ChainLedgerCacheSize:                      100,
			StateLedgerAccountTrieCacheMegabytesLimit: 128,
			StateLedgerStorageTrieCacheMegabytesLimit: 128,
			StateLedgerAccountCacheSize:               1024,
			EnablePrune:                               true,
			StateLedgerReservedHistoryBlockNum:        256,
		},
		Snapshot: Snapshot{
			AccountSnapshotCacheMegabytesLimit:  128,
			ContractSnapshotCacheMegabytesLimit: 128,
		},
		Executor: Executor{
			Type:            ExecTypeNative,
			DisableRollback: false,
		},
		PProf: PProf{
			Enable:   true,
			PType:    PprofTypeHTTP,
			Mode:     PprofModeMem,
			Duration: Duration(30 * time.Second),
		},
		Monitor: Monitor{
			Enable:          true,
			EnableExpensive: true,
		},
		Log: Log{
			Level:            "info",
			Filename:         "draconis",
			ReportCaller:     false,
			EnableCompress:   false,
			EnableColor:      true,
			DisableTimestamp: false,
			MaxAge:           10,
			MaxSize:          128,
			RotationTime:     Duration(24 * time.Hour),
			Module: LogModule{
				P2P:            "info",
				Consensus:      "debug",
				Executor:       "info",
				Governance:     "info",
				API:            "info",
				CoreAPI:        "info",
				Storage:        "info",
				Profile:        "info",
				Finance:        "error",
				BlockSync:      "info",
				APP:            "info",
				Access:         "info",
				TxPool:         "info",
				Epoch:          "info",
				SystemContract: "info",
			},
		},
		Access: Access{
			EnableWhiteList: false,
		},
	}
}

func GeminiConsensusConfig() *ConsensusConfig {
	// nolint
	return &ConsensusConfig{
		TimedGenBlock: TimedGenBlock{
			NoTxBatchTimeout: Duration(2 * time.Second),
		},
		Limit: ReceiveMsgLimiter{
			Enable: false,
			Limit:  10000,
			Burst:  10000,
		},
		TxPool: TxPool{
			PoolSize:               50000,
			ToleranceTime:          Duration(5 * time.Minute),
			ToleranceRemoveTime:    Duration(15 * time.Minute),
			CleanEmptyAccountTime:  Duration(10 * time.Minute),
			RotateTxLocalsInterval: Duration(1 * time.Hour),
			ToleranceNonceGap:      1000,
			EnableLocalsPersist:    true,
			PriceLimit:             1000_000_000_000,
			PriceBump:              10,
			GenerateBatchType:      GenerateBatchByTime,
		},
		TxCache: TxCache{
			SetSize:    50,
			SetTimeout: Duration(100 * time.Millisecond),
		},
		Rbft: RBFT{
			EnableMultiPipes:          false,
			EnableMetrics:             true,
			CommittedBlockCacheNumber: 10,
			Timeout: RBFTTimeout{
				NullRequest:      Duration(3 * time.Second),
				Request:          Duration(2 * time.Second),
				ResendViewChange: Duration(10 * time.Second),
				CleanViewChange:  Duration(60 * time.Second),
				NewView:          Duration(8 * time.Second),
				SyncState:        Duration(1 * time.Second),
				SyncStateRestart: Duration(10 * time.Minute),
				FetchCheckpoint:  Duration(5 * time.Second),
				FetchView:        Duration(1 * time.Second),
				BatchTimeout:     Duration(500 * time.Millisecond),
			},
		},
		Solo: Solo{
			BatchTimeout: Duration(500 * time.Millisecond),
		},
	}
}

func GeminiGenesisConfig() *GenesisConfig {
	// nolint
	return &GenesisConfig{
		ChainID: 23413,
		Admins: []*Admin{
			{
				Address: "0xB37cF4c055A454EEc7d7e141eA03fFFaa979b27E",
				Weight:  1,
				Name:    "S2luZw==",
			},
			{
				Address: "0xeF95F6fDF32250553bD6fb90c4c86DC8Ff1707fd",
				Weight:  1,
				Name:    "UmVk",
			},
			{
				Address: "0x204B723dCBdFF60d32EA4888D618Dd1E256d1a93",
				Weight:  1,
				Name:    "QXBwbGU=",
			},
			{
				Address: "0xEDbc7AE5605BA4ACc53085C4094648e8DB2172b5",
				Weight:  1,
				Name:    "Q2F0",
			},
		},
		SmartAccountAdmin:      "0x83Db4fA2CbB682753C94ca8A809a4a321aA36e1b",
		InitWhiteListProviders: []string{},
		Accounts:               []*Account{},
		EpochInfo: &rbft.EpochInfo{
			Version:                   1,
			Epoch:                     1,
			EpochPeriod:               1800,
			StartBlock:                1,
			P2PBootstrapNodeAddresses: []string{},
			ConsensusParams: rbft.ConsensusParams{
				ValidatorElectionType:         rbft.ValidatorElectionTypeWRF,
				ProposerElectionType:          rbft.ProposerElectionTypeWRF,
				CheckpointPeriod:              1,
				HighWatermarkCheckpointPeriod: 10,
				MaxValidatorNum:               8,
				BlockMaxTxNum:                 500,
				EnableTimedGenEmptyBlock:      false,
				NotActiveWeight:               1,
				AbnormalNodeExcludeView:       100,
				AgainProposeIntervalBlockInValidatorsNumPercentage: 30,
				ContinuousNullRequestToleranceNumber:               3,
				ReBroadcastToleranceNumber:                         2,
			},
			FinanceParams: rbft.FinanceParams{
				GasLimit:               100000000,
				StartGasPriceAvailable: true,
				StartGasPrice:          5000000000000,
				MaxGasPrice:            10000000000000,
				MinGasPrice:            1000000000000,
				GasChangeRateValue:     1250,
				GasChangeRateDecimals:  4,
			},
			MiscParams: rbft.MiscParams{
				TxMaxSize: DefaultTxMaxSize,
			},
			ValidatorSet: []rbft.NodeInfo{
				{
					ID:                   1,
					AccountAddress:       "0xB37cF4c055A454EEc7d7e141eA03fFFaa979b27E",
					P2PNodeID:            "16Uiu2HAmDXvQN7tu9ufpoCfEbjF34ZH7XeghJdqwJHfd8comLPyj",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   2,
					AccountAddress:       "0xeF95F6fDF32250553bD6fb90c4c86DC8Ff1707fd",
					P2PNodeID:            "16Uiu2HAmL1xr5VjULUPobWyFsUqnx4Cw8Fw3zHPxaahQqc1LSmtA",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   3,
					AccountAddress:       "0x204B723dCBdFF60d32EA4888D618Dd1E256d1a93",
					P2PNodeID:            "16Uiu2HAmRXGApoLQ49tn3wJH52ywhNUoGgxZsnT676k6FpZWw117",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   4,
					AccountAddress:       "0xEDbc7AE5605BA4ACc53085C4094648e8DB2172b5",
					P2PNodeID:            "16Uiu2HAmAnxJJoXBKJXg9rSp7LdAPcW83rTU7nuvb9gCShPwS8qb",
					ConsensusVotingPower: 1000,
				},
			},
			CandidateSet: []rbft.NodeInfo{},
			DataSyncerSet: []rbft.NodeInfo{
				{
					ID:                   5,
					AccountAddress:       "0x967BD1E74829866d7b84b22b61a033d2Aa12422e",
					P2PNodeID:            "16Uiu2HAmF4DcwTqdBo4RscLcPS7VwvoGs18sXmcV3pWiqHZA9UD4",
					ConsensusVotingPower: 0,
				},
			},
		},
		NodeNames: []*NodeName{
			{
				ID:   1,
				Name: "S2luZw==",
			},
			{
				ID:   2,
				Name: "UmVk",
			},
			{
				ID:   3,
				Name: "QXBwbGU=",
			},
			{
				ID:   4,
				Name: "Q2F0",
			},
			{
				ID:   5,
				Name: "bm9kZTU=",
			},
		},
	}
}

func DraconisTestNetConfig() *Config {
	// nolint
	return &Config{
		Ulimit: 65535,
		Port: Port{
			JsonRpc:   8881,
			WebSocket: 9091,
			P2P:       4001,
			PProf:     53121,
			Monitor:   40011,
		},
		JsonRPC: JsonRPC{
			GasCap:     300000000,
			EVMTimeout: Duration(5 * time.Second),
			ReadLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			WriteLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   true,
			},
			RejectTxsIfConsensusAbnormal: false,
			QueryLimit: QueryLimit{
				GetLogsBlockRangeLimit: 2000,
			},
		},
		P2P: P2P{
			Security:        P2PSecurityTLS,
			SendTimeout:     Duration(5 * time.Second),
			ReadTimeout:     Duration(5 * time.Second),
			CompressionAlgo: network.SnappyCompression,
			EnableMetrics:   true,
			BootstrapNodeAddresses: []string{
				"/ip4/44.204.228.37/tcp/4001/p2p/16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
				"/ip4/44.201.186.3/tcp/4001/p2p/16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
				"/ip4/3.80.140.180/tcp/4001/p2p/16Uiu2HAmTwEET536QC9MZmYFp1NUshjRuaq5YSH1sLjW65WasvRk",
				"/ip4/44.211.74.132/tcp/4001/p2p/16Uiu2HAmQBFTnRr84M3xNhi3EcWmgZnnBsDgewk4sNtpA3smBsHJ",
			},
			Pipe: P2PPipe{
				ReceiveMsgCacheSize: 10240,
				BroadcastType:       P2PPipeBroadcastGossip,
				SimpleBroadcast: P2PPipeSimpleBroadcast{
					WorkerCacheSize:        1024,
					WorkerConcurrencyLimit: 20,
				},
				Gossipsub: P2PPipeGossipsub{
					SubBufferSize:          10240,
					PeerOutboundBufferSize: 10240,
					ValidateBufferSize:     10240,
					SeenMessagesTTL:        Duration(120 * time.Second),
					EnableMetrics:          true,
				},
				UnicastReadTimeout:       Duration(5 * time.Second),
				UnicastSendRetryNumber:   5,
				UnicastSendRetryBaseTime: Duration(100 * time.Millisecond),
				FindPeerTimeout:          Duration(10 * time.Second),
				ConnectTimeout:           Duration(1 * time.Second),
			},
		},
		Sync: Sync{
			WaitStatesTimeout:     Duration(30 * time.Second),
			RequesterRetryTimeout: Duration(5 * time.Second),
			TimeoutCountLimit:     uint64(10),
			ConcurrencyLimit:      100,
		},
		Consensus: Consensus{
			Type:        ConsensusTypeRbft,
			StorageType: ConsensusStorageTypeRosedb,
		},
		Storage: Storage{
			KvType:      KVStorageTypePebble,
			KvCacheSize: 128,
			Sync:        true,
		},
		Ledger: Ledger{
			ChainLedgerCacheSize:                      100,
			StateLedgerAccountTrieCacheMegabytesLimit: 128,
			StateLedgerStorageTrieCacheMegabytesLimit: 128,
			StateLedgerAccountCacheSize:               1024,
			EnablePrune:                               true,
			StateLedgerReservedHistoryBlockNum:        256,
		},
		Snapshot: Snapshot{
			AccountSnapshotCacheMegabytesLimit:  128,
			ContractSnapshotCacheMegabytesLimit: 128,
		},
		Executor: Executor{
			Type:            ExecTypeNative,
			DisableRollback: false,
		},
		PProf: PProf{
			Enable:   false,
			PType:    PprofTypeHTTP,
			Mode:     PprofModeMem,
			Duration: Duration(30 * time.Second),
		},
		Monitor: Monitor{
			Enable:          false,
			EnableExpensive: false,
		},
		Log: Log{
			Level:            "info",
			Filename:         "draconis",
			ReportCaller:     false,
			EnableCompress:   false,
			EnableColor:      true,
			DisableTimestamp: false,
			MaxAge:           10,
			MaxSize:          128,
			RotationTime:     Duration(24 * time.Hour),
			Module: LogModule{
				P2P:            "info",
				Consensus:      "info",
				Executor:       "info",
				Governance:     "info",
				API:            "info",
				CoreAPI:        "info",
				Storage:        "info",
				Profile:        "info",
				Finance:        "error",
				BlockSync:      "info",
				APP:            "info",
				Access:         "info",
				TxPool:         "info",
				Epoch:          "info",
				SystemContract: "info",
			},
		},
		Access: Access{
			EnableWhiteList: false,
		},
	}
}

func DraconisTestNetConsensusConfig() *ConsensusConfig {
	// nolint
	return &ConsensusConfig{
		TimedGenBlock: TimedGenBlock{
			NoTxBatchTimeout: Duration(2 * time.Second),
		},
		Limit: ReceiveMsgLimiter{
			Enable: false,
			Limit:  10000,
			Burst:  10000,
		},
		TxPool: TxPool{
			PoolSize:               50000,
			ToleranceTime:          Duration(5 * time.Minute),
			ToleranceRemoveTime:    Duration(15 * time.Minute),
			CleanEmptyAccountTime:  Duration(10 * time.Minute),
			RotateTxLocalsInterval: Duration(1 * time.Hour),
			ToleranceNonceGap:      1000,
			EnableLocalsPersist:    true,
			PriceLimit:             1000_000_000_000,
			PriceBump:              10,
			GenerateBatchType:      GenerateBatchByTime,
		},
		TxCache: TxCache{
			SetSize:    50,
			SetTimeout: Duration(100 * time.Millisecond),
		},
		Rbft: RBFT{
			EnableMultiPipes:          false,
			EnableMetrics:             true,
			CommittedBlockCacheNumber: 10,
			Timeout: RBFTTimeout{
				NullRequest:      Duration(3 * time.Second),
				Request:          Duration(2 * time.Second),
				ResendViewChange: Duration(10 * time.Second),
				CleanViewChange:  Duration(60 * time.Second),
				NewView:          Duration(8 * time.Second),
				SyncState:        Duration(1 * time.Second),
				SyncStateRestart: Duration(10 * time.Minute),
				FetchCheckpoint:  Duration(5 * time.Second),
				FetchView:        Duration(1 * time.Second),
				BatchTimeout:     Duration(500 * time.Millisecond),
			},
		},
		Solo: Solo{
			BatchTimeout: Duration(500 * time.Millisecond),
		},
	}
}

func DraconisTestNetGenesisConfig() *GenesisConfig {

	// 500 million DRS
	defaultDRSTotalSupply := "500000000000000000000000000"
	totalSupply, _ := new(big.Int).SetString(defaultDRSTotalSupply, 10)
	accountBalance := new(big.Int).Div(totalSupply, new(big.Int).SetUint64(5))

	// nolint
	return &GenesisConfig{
		ChainID: 1356,
		Admins: []*Admin{
			{
				Address: "0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013",
				Weight:  1,
				Name:    "bm9kZTE=",
			},
			{
				Address: "0x79a1215469FaB6f9c63c1816b45183AD3624bE34",
				Weight:  1,
				Name:    "bm9kZTI=",
			},
			{
				Address: "0x97c8B516D19edBf575D72a172Af7F418BE498C37",
				Weight:  1,
				Name:    "bm9kZTM=",
			},
			{
				Address: "0xc0Ff2e0b3189132D815b8eb325bE17285AC898f8",
				Weight:  1,
				Name:    "bm9kZTQ=",
			},
		},
		SmartAccountAdmin:      "0x83Db4fA2CbB682753C94ca8A809a4a321aA36e1b",
		InitWhiteListProviders: []string{},
		NativeToken: &Token{
			Name:        "DraconisCoins",
			Symbol:      "DRS",
			Decimals:    18,
			TotalSupply: totalSupply.String(),
		},
		Axc: &Token{
			Name:        "not support",
			Symbol:      "AXC",
			Decimals:    18,
			TotalSupply: "0",
		},
		Accounts: []*Account{
			{
				Address: "0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013",
				Balance: accountBalance.String(),
			},
			{
				Address: "0x79a1215469FaB6f9c63c1816b45183AD3624bE34",
				Balance: accountBalance.String(),
			},
			{
				Address: "0x97c8B516D19edBf575D72a172Af7F418BE498C37",
				Balance: accountBalance.String(),
			},
			{
				Address: "0xc0Ff2e0b3189132D815b8eb325bE17285AC898f8",
				Balance: accountBalance.String(),
			},
			{
				Address: "0xd0091F6D0b39B9E9D2E9051fA46d13B63b8C7B18",
				Balance: accountBalance.String(),
			},
		},
		EpochInfo: &rbft.EpochInfo{
			Version:                   1,
			Epoch:                     1,
			EpochPeriod:               10,
			StartBlock:                1,
			P2PBootstrapNodeAddresses: []string{},
			ConsensusParams: rbft.ConsensusParams{
				ValidatorElectionType:         rbft.ValidatorElectionTypeWRF,
				ProposerElectionType:          rbft.ProposerElectionTypeWRF,
				CheckpointPeriod:              1,
				HighWatermarkCheckpointPeriod: 10,
				MaxValidatorNum:               8,
				BlockMaxTxNum:                 500,
				EnableTimedGenEmptyBlock:      false,
				NotActiveWeight:               1,
				AbnormalNodeExcludeView:       100,
				AgainProposeIntervalBlockInValidatorsNumPercentage: 30,
				ContinuousNullRequestToleranceNumber:               3,
				ReBroadcastToleranceNumber:                         2,
			},
			FinanceParams: rbft.FinanceParams{
				GasLimit:               100000000,
				StartGasPriceAvailable: true,
				StartGasPrice:          1000000000000,
				MaxGasPrice:            10000000000000,
				MinGasPrice:            1000000000000,
				GasChangeRateValue:     1250,
				GasChangeRateDecimals:  4,
			},

			MiscParams: rbft.MiscParams{
				TxMaxSize: DefaultTxMaxSize,
			},
			ValidatorSet: []rbft.NodeInfo{
				{
					ID:                   1,
					AccountAddress:       "0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013",
					P2PNodeID:            "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   2,
					AccountAddress:       "0x79a1215469FaB6f9c63c1816b45183AD3624bE34",
					P2PNodeID:            "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   3,
					AccountAddress:       "0x97c8B516D19edBf575D72a172Af7F418BE498C37",
					P2PNodeID:            "16Uiu2HAmTwEET536QC9MZmYFp1NUshjRuaq5YSH1sLjW65WasvRk",
					ConsensusVotingPower: 1000,
				},
				{
					ID:                   4,
					AccountAddress:       "0xc0Ff2e0b3189132D815b8eb325bE17285AC898f8",
					P2PNodeID:            "16Uiu2HAmQBFTnRr84M3xNhi3EcWmgZnnBsDgewk4sNtpA3smBsHJ",
					ConsensusVotingPower: 1000,
				},
			},
			CandidateSet: []rbft.NodeInfo{},
			DataSyncerSet: []rbft.NodeInfo{
				{
					ID:                   5,
					AccountAddress:       "0xd0091F6D0b39B9E9D2E9051fA46d13B63b8C7B18",
					P2PNodeID:            "16Uiu2HAm2HeK145KTfLaURhcoxBUMZ1PfhVnLRfnmE8qncvXWoZj",
					ConsensusVotingPower: 0,
				},
			},
		},

		NodeNames: []*NodeName{
			{
				ID:   1,
				Name: "bm9kZTE=",
			},
			{
				ID:   2,
				Name: "bm9kZTI=",
			},
			{
				ID:   3,
				Name: "bm9kZTM=",
			},
			{
				ID:   4,
				Name: "bm9kZTQ=",
			},
			{
				ID:   5,
				Name: "bm9kZTU=",
			},
		},

		Incentive: &Incentive{
			Mining: &Mining{
				BlockNumToHalf: 126144000,
				BlockNumToNone: 0,
				TotalAmount:    "40000000000000000000000000",
			},
			UserAcquisition: &UserAcquisition{
				AvgBlockReward: "126000000000000000",
				BlockToNone:    0,
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
	}
}
