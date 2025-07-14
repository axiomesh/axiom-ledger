package repo

const (
	AppName = "Draconis"

	// CfgFileName is the default config name
	CfgFileName = "config.toml"

	consensusCfgFileName = "consensus.toml"

	genesisCfgFileName = "genesis.toml"

	// defaultRepoRoot is the path to the default config dir location.
	defaultRepoRoot = "~/.draconis"

	// rootPathEnvVar is the environment variable used to change the path root.
	rootPathEnvVar = "DRACONIS_PATH"

	P2PKeyFileName = "p2p.key"

	// NodeP2PIdName is the name of the custom field in custom key json named p2p.key
	NodeP2PIdName = "node_p2p_id"

	DefaultKeyJsonPassword = "2025@draconis"

	pidFileName = "running.pid"

	LogsDirName = "logs"
)

const (
	ConsensusTypeSolo    = "solo"
	ConsensusTypeRbft    = "rbft"
	ConsensusTypeSoloDev = "solo_dev"

	ConsensusStorageTypeMinifile = "minifile"
	ConsensusStorageTypeRosedb   = "rosedb"

	KVStorageTypeLeveldb = "leveldb"
	KVStorageTypePebble  = "pebble"
	KVStorageCacheSize   = 16
	KVStorageSync        = true

	P2PSecurityTLS   = "tls"
	P2PSecurityNoise = "noise"

	PprofModeMem     = "mem"
	PprofModeCpu     = "cpu"
	PprofTypeHTTP    = "http"
	PprofTypeRuntime = "runtime"

	P2PPipeBroadcastSimple = "simple"
	P2PPipeBroadcastGossip = "gossip"

	ExecTypeNative = "native"
	ExecTypeDev    = "dev"

	// txSlotSize is used to calculate how many data slots a single transaction
	// takes up based on its size. The slots are used as DoS protection, ensuring
	// that validating a new transaction remains a constant operation (in reality
	// O(maxslots), where max slots are 4 currently).
	txSlotSize = 32 * 1024

	// DefaultTxMaxSize is the maximum size a single transaction can have. This field has
	// non-trivial consequences: larger transactions are significantly harder and
	// more expensive to propagate; larger transactions also take more resources
	// to validate whether they fit into the pool or not.
	DefaultTxMaxSize = 4 * txSlotSize // 128KB

	DefaultAXMBalance = "1000000000000000000000000000" // one billion AXM

	DefaultDecimals = 18

	DefaultAXCTotalSupply = "2000000000000000000000000000"

	DefaultStartGasPrice = 5000000000000
	DefaultMaxGasPrice   = 10000000000000
	DefaultMinGasPrice   = 1000_000_000_000
)

const (
	GenerateBatchByTime     = "fifo" // default
	GenerateBatchByGasPrice = "price_priority"
)

var (
	DefaultAdminNames = []string{
		"bm9kZTE=", // base64 encode node1
		"bm9kZTI=", // base64 encode node2
		"bm9kZTM=", // base64 encode node3
		"bm9kZTQ=", // base64 encode node4
	}

	DefaultNodeNames = []string{
		"bm9kZTE=", // base64 encode node1
		"bm9kZTI=", // base64 encode node2
		"bm9kZTM=", // base64 encode node3
		"bm9kZTQ=", // base64 encode node4

		// data syncer
		"bm9kZTU=", // base64 encode node5
	}

	DefaultNodeKeys = []string{
		"b6477143e17f889263044f6cf463dc37177ac4526c4c39a7a344198457024a2f",
		"05c3708d30c2c72c4b36314a41f30073ab18ea226cf8c6b9f566720bfe2e8631",
		"85a94dd51403590d4f149f9230b6f5de3a08e58899dcaf0f77768efb1825e854",
		"72efcf4bb0e8a300d3e47e6a10f630bcd540de933f01ed5380897fc5e10dc95d",

		// data syncer
		"06bf783a69c860a2ab33fe2f99fed38d14bbdba7ef2295bbcb5a073e6c8847ec",
	}

	DefaultNodeAddrs = []string{
		"0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013",
		"0x79a1215469FaB6f9c63c1816b45183AD3624bE34",
		"0x97c8B516D19edBf575D72a172Af7F418BE498C37",
		"0xc0Ff2e0b3189132D815b8eb325bE17285AC898f8",

		// data syncer
		"0xd0091F6D0b39B9E9D2E9051fA46d13B63b8C7B18",
	}

	defaultNodeIDs = []string{
		"16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
		"16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
		"16Uiu2HAmTwEET536QC9MZmYFp1NUshjRuaq5YSH1sLjW65WasvRk",
		"16Uiu2HAmQBFTnRr84M3xNhi3EcWmgZnnBsDgewk4sNtpA3smBsHJ",

		// data syncer
		"16Uiu2HAm2HeK145KTfLaURhcoxBUMZ1PfhVnLRfnmE8qncvXWoZj",
	}

	DefaultAXCDistribution = []Distribution{
		{
			Name:         "Community",
			Addr:         "0xf16F8B02df2Dd7c4043C41F3f1EBB17f15358888",
			Percentage:   0.44,
			InitEmission: 0,
			Locked:       true,
		},
		{
			Name:         "Advisor",
			Addr:         "0x6F878f6355646240878bc732e91d875DE8649999",
			Percentage:   0.05,
			InitEmission: 0,
			Locked:       true,
		},
		{
			Name:         "Investor",
			Addr:         "0x56ee19704930abbdd1ABb11D6B6fE4b6b3B76666",
			Percentage:   0.05,
			InitEmission: 0,
			Locked:       true,
		},
		{
			Name:         "Contributors",
			Addr:         "0xF0aD804C24caE19F5Ab061dDDfa843B3A5976666",
			Percentage:   0.2,
			InitEmission: 0,
			Locked:       true,
		},
		{
			Name:         "Foundation",
			Addr:         "0x150776D3268c0eAEdAB7d880fd929fe1c5666666",
			Percentage:   0.15,
			InitEmission: 0,
			Locked:       true,
		},
		{
			Name:         "Mining",
			Addr:         "0xE7aEe2a87E7d5129Bd2fBf12fAfF7534Ed146666",
			Percentage:   0.02,
			InitEmission: 0,
			Locked:       false,
		},
		{
			Name:         "UserAcquisition",
			Addr:         "0xe829F053f2DA7E97dC4A7E40E5B29370B7669999",
			Percentage:   0.04,
			InitEmission: 0,
			Locked:       false,
		},
	}

	DefaultAXCDistributionPrivateKey = map[string]string{
		"Community":       "c1e0c69234f8d95c150d31c861951675608cc3a3a8941466911f9b9901d4295c",
		"Advisor":         "cf3dda5b1d28ea25149e74a53b9442b79238944e45ff4a592e84aa9467e7bb28",
		"Investor":        "8a8f960fea458305ddb84c71fceeec3484adcc807e1989934069c93c946bdaac",
		"Contributors":    "4a4d0a01f7fbe7a61b132564fa785bf32c04983828dfd581e7a73b51ad8d9d7b",
		"Foundation":      "5b11af4b31ea70763486004cf0f3e0521da98d2c95cce4013a207287f8aeabc6",
		"Mining":          "1d7a8c36d0931b47ada09215eb33b79d8c5dc877ca8eb4891b97bc8691061abc",
		"UserAcquisition": "ea464b9759a95a7b3b7d66cf16205d733fb3c73d9c666217e676568db68da375",
	}
)
