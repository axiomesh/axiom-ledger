package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
)

var replayArgs = struct {
	InputDir   string
	RpcURL     string
	StartBlock uint64
	EndBlock   uint64
	Interval   time.Duration
	DryRun     bool
	Verbose    bool
}{
	InputDir: "./blocks",             // 默认输入目录
	Interval: 501 * time.Millisecond, // 默认501ms间隔
}

var replayCMD = &cli.Command{
	Name:  "replay",
	Usage: "从本地文件重放交易到区块链",
	Description: `
该命令会从本地文件中读取交易数据，
然后按照原始顺序重新发送到指定的区块链RPC端点。

使用示例：
  tx-client replay --rpc-url http://127.0.0.1:8881
  tx-client replay -r http://127.0.0.1:8881 --start-block 1 --end-block 100
  tx-client replay -r http://127.0.0.1:8881 --dry-run --verbose
	`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "input-dir",
			Aliases:     []string{"i"},
			Usage:       "输入目录路径，包含区块文件",
			Destination: &replayArgs.InputDir,
			Value:       "./blocks",
		},
		&cli.StringFlag{
			Name:        "rpc-url",
			Aliases:     []string{"r"},
			Usage:       "RPC URL地址 (例如: http://127.0.0.1:8881)",
			Destination: &replayArgs.RpcURL,
			Required:    true,
		},
		&cli.Uint64Flag{
			Name:        "start-block",
			Aliases:     []string{"s"},
			Usage:       "重放开始区块号 (默认: 1)",
			Destination: &replayArgs.StartBlock,
			Value:       1,
		},
		&cli.Uint64Flag{
			Name:        "end-block",
			Aliases:     []string{"e"},
			Usage:       "重放结束区块号 (0表示重放所有可用区块)",
			Destination: &replayArgs.EndBlock,
			Value:       0,
		},
		&cli.DurationFlag{
			Name:        "interval",
			Usage:       "发送交易间隔时间 (默认: 501ms)",
			Destination: &replayArgs.Interval,
			Value:       501 * time.Millisecond,
		},
		&cli.BoolFlag{
			Name:        "dry-run",
			Usage:       "演习模式 - 只显示交易而不实际发送",
			Destination: &replayArgs.DryRun,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "详细输出模式",
			Destination: &replayArgs.Verbose,
		},
	},
	Action: replayTransactions,
}

func replayTransactions(ctx *cli.Context) error {
	logger := logrus.New()
	if replayArgs.Verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Info("开始交易重放...")

	// 检查输入目录是否存在
	if !fileutil.Exist(replayArgs.InputDir) {
		return errors.New("输入目录不存在")
	}

	// 获取可用的区块文件
	blockFiles, err := getBlockFiles(replayArgs.InputDir)
	if err != nil {
		return errors.Wrap(err, "获取区块文件失败")
	}

	if len(blockFiles) == 0 {
		return errors.New("输入目录中没有找到任何区块文件")
	}

	// 过滤区块范围
	startBlock := replayArgs.StartBlock
	endBlock := replayArgs.EndBlock
	if endBlock == 0 {
		endBlock = getMaxBlockNumber(blockFiles)
	}

	if startBlock > endBlock {
		return fmt.Errorf("开始区块号(%d)不能大于结束区块号(%d)", startBlock, endBlock)
	}

	logger.Infof("重放区块范围: %d 到 %d", startBlock, endBlock)
	logger.Infof("输入目录: %s", replayArgs.InputDir)
	logger.Infof("RPC URL: %s", replayArgs.RpcURL)

	// 如果不是演习模式，初始化以太坊客户端
	var ethClient *ethclient.Client
	if !replayArgs.DryRun {
		ethClient, err = ethclient.DialContext(ctx.Context, replayArgs.RpcURL)
		if err != nil {
			return errors.Wrap(err, "连接到RPC失败")
		}
		defer ethClient.Close()

		// 测试连接
		chainID, err := ethClient.ChainID(ctx.Context)
		if err != nil {
			return errors.Wrap(err, "获取链ID失败")
		}
		logger.Infof("连接到链ID: %d", chainID)
	}

	// 开始重放交易
	totalTxs := 0
	sentTxs := 0
	failedTxs := 0
	processedBlocks := 0

	for blockNum := startBlock; blockNum <= endBlock; blockNum++ {
		filename := filepath.Join(replayArgs.InputDir, fmt.Sprintf("block_%d.json", blockNum))
		if !fileutil.Exist(filename) {
			logger.Warnf("区块 %d 文件不存在，跳过", blockNum)
			continue
		}

		logger.Infof("处理区块 %d", blockNum)

		// 从文件加载区块交易
		blockTxs, err := loadBlockTransactions(filename)
		if err != nil {
			logger.Errorf("加载区块 %d 失败: %v", blockNum, err)
			continue
		}

		// 处理每个交易
		txCount := len(blockTxs.Transactions)
		if txCount == 0 {
			logger.Infof("区块 %d 没有交易", blockNum)
			continue
		}

		logger.Infof("区块 %d 包含 %d 个交易", blockNum, txCount)
		totalTxs += txCount

		for i, txData := range blockTxs.Transactions {
			if replayArgs.Verbose {
				logger.Debugf("处理交易 %d/%d (区块 %d): %s", i+1, txCount, blockNum, txData.Hash)
			}

			if replayArgs.DryRun {
				logger.Infof("演习模式 - 交易: %s", txData.Hash)
				sentTxs++
			} else {
				// 解码原始交易数据
				ethTx, err := decodeRawTransaction(txData.Raw)
				if err != nil {
					logger.Errorf("解码交易 %s 失败: %v", txData.Hash, err)
					failedTxs++
					continue
				}

				// 发送交易
				err = ethClient.SendTransaction(ctx.Context, ethTx)
				if err != nil {
					logger.Errorf("发送交易 %s 失败: %v", txData.Hash, err)
					failedTxs++
					continue
				}

				if replayArgs.Verbose {
					logger.Debugf("成功发送交易: %s", ethTx.Hash().String())
				}
				sentTxs++
			}

			// 交易间隔
			if replayArgs.Interval > 0 {
				time.Sleep(replayArgs.Interval)
			}
		}

		processedBlocks++
	}

	logger.Info("重放完成!")
	logger.Infof("处理区块数: %d", processedBlocks)
	logger.Infof("总交易数: %d", totalTxs)
	logger.Infof("成功发送: %d", sentTxs)
	logger.Infof("失败: %d", failedTxs)

	return nil
}

// loadBlockTransactions 从文件加载区块交易
func loadBlockTransactions(filename string) (*BlockTransactions, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var blockTxs BlockTransactions
	if err := json.Unmarshal(data, &blockTxs); err != nil {
		return nil, err
	}

	return &blockTxs, nil
}

// getBlockFiles 获取目录中的区块文件列表
func getBlockFiles(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var blockFiles []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) == ".json" &&
			len(file.Name()) > 6 &&
			file.Name()[:6] == "block_" {
			blockFiles = append(blockFiles, file.Name())
		}
	}

	return blockFiles, nil
}

// getMaxBlockNumber 获取区块文件中的最大区块号
func getMaxBlockNumber(blockFiles []string) uint64 {
	var maxBlockNum uint64 = 0
	for _, filename := range blockFiles {
		// 提取文件名中的区块号
		// 格式: block_123.json
		if len(filename) > 6 && filename[:6] == "block_" {
			blockNumStr := filename[6 : len(filename)-5] // 去掉 "block_" 和 ".json"
			if blockNum, err := strconv.ParseUint(blockNumStr, 10, 64); err == nil {
				if blockNum > maxBlockNum {
					maxBlockNum = blockNum
				}
			}
		}
	}
	return maxBlockNum
}

// decodeRawTransaction 解码原始交易数据
func decodeRawTransaction(rawHex string) (*types.Transaction, error) {
	// 移除0x前缀
	if len(rawHex) >= 2 && rawHex[:2] == "0x" {
		rawHex = rawHex[2:]
	}

	// 十六进制解码
	rawBytes, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("解码十六进制字符串失败: %w", err)
	}

	// 解码交易
	tx := &types.Transaction{}
	if err := tx.UnmarshalBinary(rawBytes); err != nil {
		return nil, fmt.Errorf("反序列化交易失败: %w", err)
	}

	return tx, nil
}
