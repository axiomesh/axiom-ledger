package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var getBlocksArgs = struct {
	RpcURL      string
	OutputDir   string
	StartBlock  uint64
	EndBlock    uint64
	Verbose     bool
	Concurrency int
}{
	OutputDir:   "./blocks", // 默认输出目录
	Concurrency: 5,          // 默认并发数
}

var getBlocksCMD = &cli.Command{
	Name:  "get-blocks",
	Usage: "从RPC获取区块并存储到本地文件",
	Description: `
该命令会从指定的RPC端点获取区块数据，提取其中的交易，
然后将交易按区块号存储到本地文件中。

使用示例：
  tx-client get-blocks --rpc-url http://127.0.0.1:8881 --start-block 1 --end-block 100
  tx-client get-blocks -r http://127.0.0.1:8881 -s 1 -e 100 --output-dir ./my-blocks
	`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "rpc-url",
			Aliases:     []string{"r"},
			Usage:       "RPC URL地址 (例如: http://127.0.0.1:8881)",
			Destination: &getBlocksArgs.RpcURL,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "output-dir",
			Aliases:     []string{"o"},
			Usage:       "输出目录路径，用于存储区块文件",
			Destination: &getBlocksArgs.OutputDir,
			Value:       "./blocks",
		},
		&cli.Uint64Flag{
			Name:        "start-block",
			Aliases:     []string{"s"},
			Usage:       "获取开始区块号",
			Destination: &getBlocksArgs.StartBlock,
			Value:       1,
		},
		&cli.Uint64Flag{
			Name:        "end-block",
			Aliases:     []string{"e"},
			Usage:       "获取结束区块号 (0表示获取到最新区块)",
			Destination: &getBlocksArgs.EndBlock,
			Value:       0,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "详细输出模式",
			Destination: &getBlocksArgs.Verbose,
		},
		&cli.IntFlag{
			Name:        "concurrency",
			Aliases:     []string{"c"},
			Usage:       "并发请求数量",
			Destination: &getBlocksArgs.Concurrency,
			Value:       5,
		},
	},
	Action: getBlocks,
}

// BlockResult 表示区块获取结果
type BlockResult struct {
	BlockNumber uint64
	BlockTxs    *BlockTransactions
	Error       error
}

func getBlocks(ctx *cli.Context) error {
	logger := logrus.New()
	if getBlocksArgs.Verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Info("开始获取区块...")

	// 创建输出目录
	if err := os.MkdirAll(getBlocksArgs.OutputDir, 0755); err != nil {
		return errors.Wrap(err, "创建输出目录失败")
	}

	// 连接到RPC
	ethClient, err := ethclient.DialContext(ctx.Context, getBlocksArgs.RpcURL)
	if err != nil {
		return errors.Wrap(err, "连接到RPC失败")
	}
	defer ethClient.Close()

	// 获取最新区块号
	latestBlock, err := ethClient.BlockNumber(ctx.Context)
	if err != nil {
		return errors.Wrap(err, "获取最新区块号失败")
	}

	startBlock := getBlocksArgs.StartBlock
	endBlock := getBlocksArgs.EndBlock
	if endBlock == 0 {
		endBlock = latestBlock
	}

	if startBlock > endBlock {
		return fmt.Errorf("开始区块号(%d)不能大于结束区块号(%d)", startBlock, endBlock)
	}

	totalBlocks := endBlock - startBlock + 1
	logger.Infof("获取区块范围: %d 到 %d (共 %d 个区块)", startBlock, endBlock, totalBlocks)
	logger.Infof("RPC URL: %s", getBlocksArgs.RpcURL)
	logger.Infof("输出目录: %s", getBlocksArgs.OutputDir)
	logger.Infof("并发数: %d", getBlocksArgs.Concurrency)

	// 创建进度条
	bar := progressbar.NewOptions(int(totalBlocks),
		progressbar.OptionSetDescription("获取区块"),
		progressbar.OptionSetWidth(50),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	if !getBlocksArgs.Verbose {
		// 非详细模式下隐藏其他日志输出
		logger.SetLevel(logrus.ErrorLevel)
	}

	// 使用并发获取区块
	results, err := fetchBlocksConcurrently(ctx.Context, ethClient, startBlock, endBlock, getBlocksArgs.Concurrency, bar)
	if err != nil {
		bar.Finish()
		return err
	}

	bar.Finish()
	fmt.Println() // 换行分隔

	// 按顺序保存结果
	savedTxs := 0
	processedBlocks := 0
	failedBlocks := 0

	// 确保结果按区块号排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].BlockNumber < results[j].BlockNumber
	})

	logger.Info("开始保存区块数据...")
	saveBar := progressbar.NewOptions(len(results),
		progressbar.OptionSetDescription("保存文件"),
		progressbar.OptionSetWidth(50),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	for _, result := range results {
		saveBar.Add(1)

		if result.Error != nil {
			if getBlocksArgs.Verbose {
				logger.Errorf("区块 %d 获取失败: %v", result.BlockNumber, result.Error)
			}
			failedBlocks++
			continue
		}

		// 保存到文件
		filename := filepath.Join(getBlocksArgs.OutputDir, fmt.Sprintf("block_%d.json", result.BlockNumber))
		if err := saveBlockTransactions(filename, result.BlockTxs); err != nil {
			if getBlocksArgs.Verbose {
				logger.Errorf("保存区块 %d 失败: %v", result.BlockNumber, err)
			}
			failedBlocks++
			continue
		}

		processedBlocks++
		savedTxs += len(result.BlockTxs.Transactions)

		if getBlocksArgs.Verbose {
			logger.Debugf("区块 %d: 保存 %d 个交易", result.BlockNumber, len(result.BlockTxs.Transactions))
		}
	}

	saveBar.Finish()

	logger.Info("获取区块完成!")
	logger.Infof("处理区块数: %d", processedBlocks)
	logger.Infof("失败区块数: %d", failedBlocks)
	logger.Infof("保存交易数: %d", savedTxs)

	return nil
}

// fetchBlocksConcurrently 并发获取区块数据
func fetchBlocksConcurrently(ctx context.Context, client *ethclient.Client, startBlock, endBlock uint64, concurrency int, bar *progressbar.ProgressBar) ([]BlockResult, error) {
	totalBlocks := endBlock - startBlock + 1

	// 创建工作任务channel
	jobs := make(chan uint64, totalBlocks)
	results := make(chan BlockResult, totalBlocks)

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			blockWorker(ctx, client, jobs, results)
		}()
	}

	// 发送工作任务
	go func() {
		defer close(jobs)
		for blockNum := startBlock; blockNum <= endBlock; blockNum++ {
			select {
			case jobs <- blockNum:
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待所有工作完成并收集结果
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果并更新进度条
	var blockResults []BlockResult
	for result := range results {
		blockResults = append(blockResults, result)
		bar.Add(1)
	}

	return blockResults, nil
}

// blockWorker 区块获取工作协程
func blockWorker(ctx context.Context, client *ethclient.Client, jobs <-chan uint64, results chan<- BlockResult) {
	for blockNum := range jobs {
		result := fetchSingleBlock(ctx, client, blockNum)
		results <- result

		// 添加小延迟避免过于频繁的请求
		time.Sleep(10 * time.Millisecond)
	}
}

// fetchSingleBlock 获取单个区块
func fetchSingleBlock(ctx context.Context, client *ethclient.Client, blockNum uint64) BlockResult {
	// 获取区块数据
	block, err := client.BlockByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		return BlockResult{
			BlockNumber: blockNum,
			Error:       err,
		}
	}

	// 提取交易数据
	var txData []TransactionData
	for _, tx := range block.Transactions() {
		// 获取交易的原始数据
		rawTx, err := tx.MarshalBinary()
		if err != nil {
			// 跳过无法编码的交易，但不影响整个区块
			continue
		}

		txData = append(txData, TransactionData{
			Hash: tx.Hash().String(),
			Raw:  fmt.Sprintf("0x%x", rawTx),
		})
	}

	// 创建区块交易数据
	blockTxs := &BlockTransactions{
		BlockNumber:  blockNum,
		BlockHash:    block.Hash().String(),
		Transactions: txData,
		Timestamp:    block.Time(),
	}

	return BlockResult{
		BlockNumber: blockNum,
		BlockTxs:    blockTxs,
		Error:       nil,
	}
}

// saveBlockTransactions 保存区块交易到文件
func saveBlockTransactions(filename string, blockTxs *BlockTransactions) error {
	data, err := json.MarshalIndent(blockTxs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}
