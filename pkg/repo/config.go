package repo

import (
	"encoding/json"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	"github.com/axiomesh/axiom-kit/fileutil"
	network "github.com/axiomesh/axiom-p2p"
)

type Duration time.Duration

func (d *Duration) MarshalText() (text []byte, err error) {
	return []byte(time.Duration(*d).String()), nil
}

func (d *Duration) UnmarshalText(b []byte) error {
	x, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(x)
	return nil
}

func StringToTimeDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(Duration(5)) {
			return data, nil
		}

		d, err := time.ParseDuration(data.(string))
		if err != nil {
			return nil, err
		}
		return Duration(d), nil
	}
}

func (d *Duration) ToDuration() time.Duration {
	return time.Duration(*d)
}

func (d *Duration) String() string {
	return time.Duration(*d).String()
}

type StartArgs struct {
	ReadonlyMode bool `mapstructure:"readonly_mode" toml:"readonly_mode"`
	SnapshotMode bool `mapstructure:"snapshot_mode" toml:"snapshot_mode"`
}

type Config struct {
	Ulimit    uint64    `mapstructure:"ulimit" toml:"ulimit"`
	Port      Port      `mapstructure:"port" toml:"port"`
	JsonRPC   JsonRPC   `mapstructure:"jsonrpc" toml:"jsonrpc"`
	P2P       P2P       `mapstructure:"p2p" toml:"p2p"`
	Sync      Sync      `mapstructure:"sync" toml:"sync"`
	Consensus Consensus `mapstructure:"consensus" toml:"consensus"`
	Storage   Storage   `mapstructure:"storage" toml:"storage"`
	Ledger    Ledger    `mapstructure:"ledger" toml:"ledger"`
	Snapshot  Snapshot  `mapstructure:"snapshot" toml:"snapshot"`
	Executor  Executor  `mapstructure:"executor" toml:"executor"`
	PProf     PProf     `mapstructure:"pprof" toml:"pprof"`
	Monitor   Monitor   `mapstructure:"monitor" toml:"monitor"`
	Log       Log       `mapstructure:"log" toml:"log"`
	Access    Access    `mapstructure:"access" toml:"access"`
}

type Port struct {
	JsonRpc   int64 `mapstructure:"jsonrpc" toml:"jsonrpc"`
	WebSocket int64 `mapstructure:"websocket" toml:"websocket"`
	P2P       int64 `mapstructure:"p2p" toml:"p2p"`
	PProf     int64 `mapstructure:"pprof" toml:"pprof"`
	Monitor   int64 `mapstructure:"monitor" toml:"monitor"`
}

type JsonRPC struct {
	GasCap                       uint64     `mapstructure:"gas_cap" toml:"gas_cap"`
	EVMTimeout                   Duration   `mapstructure:"evm_timeout" toml:"evm_timeout"`
	ReadLimiter                  JLimiter   `mapstructure:"read_limiter" toml:"read_limiter"`
	WriteLimiter                 JLimiter   `mapstructure:"write_limiter" toml:"write_limiter"`
	RejectTxsIfConsensusAbnormal bool       `mapstructure:"reject_txs_if_consensus_abnormal" toml:"reject_txs_if_consensus_abnormal"`
	QueryLimit                   QueryLimit `mapstructure:"query_limit" toml:"query_limit"`
}

type QueryLimit struct {
	GetLogsBlockRangeLimit uint64 `mapstructure:"get_logs_block_range_limit" toml:"get_logs_block_range_limit"`
}

type P2PPipeGossipsub struct {
	DisableCustomMsgIDFn   bool     `mapstructure:"disable_custom_msg_id_fn" toml:"disable_custom_msg_id_fn"`
	SubBufferSize          int      `mapstructure:"sub_buffer_size" toml:"sub_buffer_size"`
	PeerOutboundBufferSize int      `mapstructure:"peer_outbound_buffer_size" toml:"peer_outbound_buffer_size"`
	ValidateBufferSize     int      `mapstructure:"validate_buffer_size" toml:"validate_buffer_size"`
	SeenMessagesTTL        Duration `mapstructure:"seen_messages_ttl" toml:"seen_messages_ttl"`
	EnableMetrics          bool     `mapstructure:"enable_metrics" toml:"enable_metrics"`
}

type P2PPipeSimpleBroadcast struct {
	WorkerCacheSize        int `mapstructure:"worker_cache_size" toml:"worker_cache_size"`
	WorkerConcurrencyLimit int `mapstructure:"worker_concurrency_limit" toml:"worker_concurrency_limit"`
}

type P2PPipe struct {
	ReceiveMsgCacheSize      int                    `mapstructure:"receive_msg_cache_size" toml:"receive_msg_cache_size"`
	BroadcastType            string                 `mapstructure:"broadcast_type" toml:"broadcast_type"`
	SimpleBroadcast          P2PPipeSimpleBroadcast `mapstructure:"simple_broadcast" toml:"simple_broadcast"`
	Gossipsub                P2PPipeGossipsub       `mapstructure:"gossipsub" toml:"gossipsub"`
	UnicastReadTimeout       Duration               `mapstructure:"unicast_read_timeout" toml:"unicast_read_timeout"`
	UnicastSendRetryNumber   int                    `mapstructure:"unicast_send_retry_number" toml:"unicast_send_retry_number"`
	UnicastSendRetryBaseTime Duration               `mapstructure:"unicast_send_retry_base_time" toml:"unicast_send_retry_base_time"`
	FindPeerTimeout          Duration               `mapstructure:"find_peer_timeout" toml:"find_peer_timeout"`
	ConnectTimeout           Duration               `mapstructure:"connect_timeout" toml:"connect_timeout"`
}

type P2P struct {
	BootstrapNodeAddresses []string                `mapstructure:"bootstrap_node_addresses" toml:"bootstrap_node_addresses"`
	Security               string                  `mapstructure:"security" toml:"security"`
	SendTimeout            Duration                `mapstructure:"send_timeout" toml:"send_timeout"`
	ReadTimeout            Duration                `mapstructure:"read_timeout" toml:"read_timeout"`
	CompressionAlgo        network.CompressionAlgo `mapstructure:"compression_option" toml:"compression_option"`
	EnableMetrics          bool                    `mapstructure:"enable_metrics" toml:"enable_metrics"`
	Pipe                   P2PPipe                 `mapstructure:"pipe" toml:"pipe"`
}

type Monitor struct {
	Enable          bool `mapstructure:"enable" toml:"enable"`
	EnableExpensive bool `mapstructure:"enable_expensive" toml:"enable_expensive"`
}

type PProf struct {
	Enable   bool     `mapstructure:"enable" toml:"enable"`
	PType    string   `mapstructure:"ptype" toml:"ptype"`
	Mode     string   `mapstructure:"mode" toml:"mode"`
	Duration Duration `mapstructure:"duration" toml:"duration"`
}

type JLimiter struct {
	Interval Duration `mapstructure:"interval" toml:"interval"`
	Quantum  int64    `mapstructure:"quantum" toml:"quantum"`
	Capacity int64    `mapstructure:"capacity" toml:"capacity"`
	Enable   bool     `mapstructure:"enable" toml:"enable"`
}

type Log struct {
	Level            string `mapstructure:"level" toml:"level"`
	Filename         string `mapstructure:"filename" toml:"filename"`
	ReportCaller     bool   `mapstructure:"report_caller" toml:"report_caller"`
	EnableCompress   bool   `mapstructure:"enable_compress" toml:"enable_compress"`
	EnableColor      bool   `mapstructure:"enable_color" toml:"enable_color"`
	DisableTimestamp bool   `mapstructure:"disable_timestamp" toml:"disable_timestamp"`

	// unit: day
	MaxAge uint `mapstructure:"max_age" toml:"max_age"`

	// unit: MB
	MaxSize uint `mapstructure:"max_size" toml:"max_size"`

	RotationTime Duration  `mapstructure:"rotation_time" toml:"rotation_time"`
	Module       LogModule `mapstructure:"module" toml:"module"`
}

type LogModule struct {
	P2P            string `mapstructure:"p2p" toml:"p2p"`
	Consensus      string `mapstructure:"consensus" toml:"consensus"`
	Executor       string `mapstructure:"executor" toml:"executor"`
	Governance     string `mapstructure:"governance" toml:"governance"`
	API            string `mapstructure:"api" toml:"api"`
	APP            string `mapstructure:"app" toml:"app"`
	CoreAPI        string `mapstructure:"coreapi" toml:"coreapi"`
	Storage        string `mapstructure:"storage" toml:"storage"`
	Profile        string `mapstructure:"profile" toml:"profile"`
	Finance        string `mapstructure:"finance" toml:"finance"`
	TxPool         string `mapstructure:"txpool" toml:"txpool"`
	Access         string `mapstructure:"access" toml:"access"`
	BlockSync      string `mapstructure:"blocksync" toml:"blocksync"`
	Epoch          string `mapstructure:"epoch" toml:"epoch"`
	SystemContract string `mapstructure:"system_contract" toml:"system_contract"`
}

type Access struct {
	EnableWhiteList bool `mapstructure:"enable_white_list" toml:"enable_white_list"`
}

type Sync struct {
	WaitStatesTimeout     Duration `mapstructure:"wait_states_timeout" toml:"wait_states_timeout"`
	RequesterRetryTimeout Duration `mapstructure:"requester_retry_timeout" toml:"requester_retry_timeout"`
	TimeoutCountLimit     uint64   `mapstructure:"timeout_count_limit" toml:"timeout_count_limit"`
	ConcurrencyLimit      uint64   `mapstructure:"concurrency_limit" toml:"concurrency_limit"`
}

type Consensus struct {
	Type        string `mapstructure:"type" toml:"type"`
	StorageType string `mapstructure:"storage_type" toml:"storage_type"`
}

type Storage struct {
	KvType      string `mapstructure:"kv_type" toml:"kv_type"`
	KvCacheSize int    `mapstructure:"kv_cache_size" toml:"kv_cache_size"`
	Sync        bool   `mapstructure:"sync" toml:"sync"`
}

type Ledger struct {
	ChainLedgerCacheSize                      int  `mapstructure:"chain_ledger_cache_size" toml:"chain_ledger_cache_size"`
	StateLedgerAccountTrieCacheMegabytesLimit int  `mapstructure:"state_ledger_account_trie_cache_megabytes_limit" toml:"state_ledger_account_trie_cache_megabytes_limit"`
	StateLedgerStorageTrieCacheMegabytesLimit int  `mapstructure:"state_ledger_storage_trie_cache_megabytes_limit" toml:"state_ledger_storage_trie_cache_megabytes_limit"`
	StateLedgerAccountCacheSize               int  `mapstructure:"state_ledger_account_cache_size" toml:"state_ledger_account_cache_size"`
	EnablePrune                               bool `mapstructure:"enable_prune" toml:"enable_prune"`
	StateLedgerReservedHistoryBlockNum        int  `mapstructure:"state_ledger_reserved_history_block_num" toml:"state_ledger_reserved_history_block_num"`
}

type Snapshot struct {
	AccountSnapshotCacheMegabytesLimit  int `mapstructure:"account_snapshot_cache_megabytes_limit" toml:"account_snapshot_cache_megabytes_limit"`
	ContractSnapshotCacheMegabytesLimit int `mapstructure:"contract_snapshot_cache_megabytes_limit" toml:"contract_snapshot_cache_megabytes_limit"`
}

type Executor struct {
	Type            string `mapstructure:"type" toml:"type"`
	DisableRollback bool   `mapstructure:"disable_rollback" toml:"disable_rollback"`
}

var SupportMultiNode = make(map[string]bool)
var registrationMutex sync.Mutex

func Register(consensusType string, isSupported bool) {
	registrationMutex.Lock()
	defer registrationMutex.Unlock()
	SupportMultiNode[consensusType] = isSupported
}

func (c *Config) Bytes() ([]byte, error) {
	ret, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func DefaultConfig() *Config {
	if testNetConfigBuilder, ok := TestNetConfigBuilderMap[BuildNet]; ok {
		return testNetConfigBuilder()
	}
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
				Enable:   false,
			},
			WriteLimiter: JLimiter{
				Interval: 50,
				Quantum:  500,
				Capacity: 10000,
				Enable:   false,
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
				"/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
				"/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
				"/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmTwEET536QC9MZmYFp1NUshjRuaq5YSH1sLjW65WasvRk",
				"/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmQBFTnRr84M3xNhi3EcWmgZnnBsDgewk4sNtpA3smBsHJ",
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
			MaxAge:           30,
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

func LoadConfig(repoRoot string) (*Config, error) {
	cfg, err := func() (*Config, error) {
		cfg := DefaultConfig()
		cfgPath := path.Join(repoRoot, CfgFileName)
		existConfig := fileutil.Exist(cfgPath)
		if !existConfig {
			err := os.MkdirAll(repoRoot, 0755)
			if err != nil {
				return nil, errors.Wrap(err, "failed to build default config")
			}

			if err := writeConfigWithEnv(cfgPath, cfg); err != nil {
				return nil, errors.Wrap(err, "failed to build default config")
			}
		} else {
			if err := CheckWritable(repoRoot); err != nil {
				return nil, err
			}
			if err := readConfigFromFile(cfgPath, cfg); err != nil {
				return nil, err
			}
		}

		return cfg, nil
	}()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}
	return cfg, nil
}
