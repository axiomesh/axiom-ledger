package solo

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

func defaultTimedConfig() TimedGenBlock {
	return TimedGenBlock{
		Enable:       true,
		BlockTimeout: 2 * time.Second,
	}
}

func generateSoloConfig(repoRoot string) (time.Duration, MempoolConfig, TimedGenBlock, error) {
	readConfig, err := readConfig(repoRoot)
	if err != nil {
		return 0, MempoolConfig{}, TimedGenBlock{}, fmt.Errorf("read solo config error: %w", err)
	}
	mempoolConf := MempoolConfig{
		BatchSize:      readConfig.SOLO.MempoolConfig.BatchSize,
		PoolSize:       readConfig.SOLO.MempoolConfig.PoolSize,
		TxSliceSize:    readConfig.SOLO.MempoolConfig.TxSliceSize,
		TxSliceTimeout: readConfig.SOLO.MempoolConfig.TxSliceTimeout,
	}

	timedGenBlock := readConfig.TimedGenBlock
	return readConfig.SOLO.BatchTimeout, mempoolConf, timedGenBlock, nil
}

func readConfig(repoRoot string) (*SOLOConfig, error) {
	v := viper.New()
	v.SetConfigFile(filepath.Join(repoRoot, "order.toml"))
	v.SetConfigType("toml")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("readInConfig error: %w", err)
	}

	config := &SOLOConfig{
		TimedGenBlock: defaultTimedConfig(),
	}

	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}

	if err := checkConfig(config); err != nil {
		return nil, fmt.Errorf("check config failed: %w", err)
	}
	return config, nil
}

func checkConfig(config *SOLOConfig) error {
	if config.TimedGenBlock.BlockTimeout.Nanoseconds() <= 0 {
		return fmt.Errorf("Illegal parameter, blockTimeout must be a positive number. ")
	}
	return nil
}
