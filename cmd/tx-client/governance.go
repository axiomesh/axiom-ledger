package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var ProposalTypeMap = map[uint8]string{
	uint8(governance.Voting):   "投票中",
	uint8(governance.Approved): "已通过",
	uint8(governance.Rejected): "已拒绝",
}

// NodeInfo 节点信息结构
type NodeInfo struct {
	NodeId  string `json:"NodeId"`  // P2P节点ID
	Address string `json:"Address"` // 账户地址
	Name    string `json:"Name"`    // 节点名称
}

// ProposalArgs 提案参数结构
type ProposalArgs struct {
	Nodes []NodeInfo `json:"Nodes"`
}

// ProposalInfo 提案信息结构
type ProposalInfo struct {
	ID     uint64 `json:"ID"`
	Type   int    `json:"Type"`
	Status int    `json:"Status"`
}

var governanceArgs = struct {
	RpcURL          string
	ContractAddress string
	PrivateKey      string
	ProposalType    uint64
	Title           string
	Description     string
	Duration        uint64
	NodeConfigFile  string
	ProposalID      uint64
	VoteOption      uint64
	GasPrice        uint64
	GasLimit        uint64
	Verbose         bool
}{
	RpcURL:          "http://127.0.0.1:8881",
	ContractAddress: "0x0000000000000000000000000000000000001001", // 默认治理合约地址
	PrivateKey:      "",
	ProposalType:    uint64(governance.NodeAdd),
	Title:           "节点管理提案",
	Description:     "节点管理提案描述",
	Duration:        1000000,
	NodeConfigFile:  "./nodes.json",
	ProposalID:      0,
	VoteOption:      0,              // 0: 赞成, 1: 反对
	GasPrice:        10000000000000, // 10 Gwei
	GasLimit:        300000,
	Verbose:         false,
}

var proposeCMD = &cli.Command{
	Name:  "propose",
	Usage: "发起节点管理提案",
	Description: `
该命令用于发起节点管理提案，包括添加节点和删除节点。

使用示例：
  tx-client propose --private-key 0x123... --type 2 --title "添加节点" --nodes nodes.json
  tx-client propose --private-key 0x123... --type 3 --title "删除节点" --nodes nodes.json
	`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "rpc-url",
			Aliases:     []string{"r"},
			Usage:       "RPC URL地址",
			Destination: &governanceArgs.RpcURL,
			Value:       "http://127.0.0.1:8881",
		},
		&cli.StringFlag{
			Name:        "contract",
			Usage:       "治理合约地址",
			Destination: &governanceArgs.ContractAddress,
			Value:       "0x0000000000000000000000000000000000001001",
		},
		&cli.StringFlag{
			Name:        "private-key",
			Aliases:     []string{"k"},
			Usage:       "私钥（用于签名交易）",
			Destination: &governanceArgs.PrivateKey,
			Required:    true,
		},
		&cli.Uint64Flag{
			Name:        "type",
			Usage:       "提案类型 (2: 添加节点, 3: 删除节点)",
			Destination: &governanceArgs.ProposalType,
			Value:       2,
		},
		&cli.StringFlag{
			Name:        "title",
			Usage:       "提案标题",
			Destination: &governanceArgs.Title,
			Value:       "节点管理提案",
		},
		&cli.StringFlag{
			Name:        "description",
			Usage:       "提案描述",
			Destination: &governanceArgs.Description,
			Value:       "节点管理提案描述",
		},
		&cli.Uint64Flag{
			Name:        "duration",
			Usage:       "提案持续时间（区块数）",
			Destination: &governanceArgs.Duration,
			Value:       1000000,
		},
		&cli.StringFlag{
			Name:        "nodes",
			Usage:       "节点配置文件路径",
			Destination: &governanceArgs.NodeConfigFile,
			Value:       "./nodes.json",
		},
		&cli.Uint64Flag{
			Name:        "gas-price",
			Usage:       "Gas价格（wei）",
			Destination: &governanceArgs.GasPrice,
			Value:       10000000000000,
		},
		&cli.Uint64Flag{
			Name:        "gas-limit",
			Usage:       "Gas限制",
			Destination: &governanceArgs.GasLimit,
			Value:       300000,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "详细输出模式",
			Destination: &governanceArgs.Verbose,
		},
	},
	Action: proposeNodes,
}

var voteCMD = &cli.Command{
	Name:  "vote",
	Usage: "对提案进行投票",
	Description: `
该命令用于对节点管理提案进行投票。

使用示例：
  tx-client vote --private-key 0x123... --proposal-id 1 --option 0
  tx-client vote --private-key 0x123... --proposal-id 1 --option 1
	`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "rpc-url",
			Aliases:     []string{"r"},
			Usage:       "RPC URL地址",
			Destination: &governanceArgs.RpcURL,
			Value:       "http://127.0.0.1:8881",
		},
		&cli.StringFlag{
			Name:        "contract",
			Usage:       "治理合约地址",
			Destination: &governanceArgs.ContractAddress,
			Value:       "0x0000000000000000000000000000000000001001",
		},
		&cli.StringFlag{
			Name:        "private-key",
			Aliases:     []string{"k"},
			Usage:       "私钥（用于签名交易）",
			Destination: &governanceArgs.PrivateKey,
			Required:    true,
		},
		&cli.Uint64Flag{
			Name:        "proposal-id",
			Usage:       "提案ID",
			Destination: &governanceArgs.ProposalID,
			Required:    true,
		},
		&cli.Uint64Flag{
			Name:        "option",
			Usage:       "投票选项 (0: 赞成, 1: 反对)",
			Destination: &governanceArgs.VoteOption,
			Value:       0,
		},
		&cli.Uint64Flag{
			Name:        "gas-price",
			Usage:       "Gas价格（wei）",
			Destination: &governanceArgs.GasPrice,
			Value:       10000000000000,
		},
		&cli.Uint64Flag{
			Name:        "gas-limit",
			Usage:       "Gas限制",
			Destination: &governanceArgs.GasLimit,
			Value:       300000,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "详细输出模式",
			Destination: &governanceArgs.Verbose,
		},
	},
	Action: voteProposal,
}

var queryProposalCMD = &cli.Command{
	Name:  "query-proposal",
	Usage: "查询提案信息",
	Description: `
该命令用于查询提案的详细信息。

使用示例：
  tx-client query-proposal --proposal-id 1
	`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "rpc-url",
			Aliases:     []string{"r"},
			Usage:       "RPC URL地址",
			Destination: &governanceArgs.RpcURL,
			Value:       "http://127.0.0.1:8881",
		},
		&cli.StringFlag{
			Name:        "contract",
			Usage:       "治理合约地址",
			Destination: &governanceArgs.ContractAddress,
			Value:       "0x0000000000000000000000000000000000001001",
		},
		&cli.Uint64Flag{
			Name:        "proposal-id",
			Usage:       "提案ID",
			Destination: &governanceArgs.ProposalID,
			Required:    true,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "详细输出模式",
			Destination: &governanceArgs.Verbose,
		},
	},
	Action: queryProposal,
}

// proposeNodes 发起节点管理提案
func proposeNodes(ctx *cli.Context) error {
	logger := logrus.New()
	if governanceArgs.Verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Info("开始发起节点管理提案...")

	// 连接到RPC
	ethClient, err := ethclient.DialContext(ctx.Context, governanceArgs.RpcURL)
	if err != nil {
		return errors.Wrap(err, "连接到RPC失败")
	}
	defer ethClient.Close()

	// 解析私钥
	privateKeyStr := governanceArgs.PrivateKey
	if strings.HasPrefix(privateKeyStr, "0x") {
		privateKeyStr = privateKeyStr[2:]
	}

	// 确保私钥长度为64个字符（32字节）
	if len(privateKeyStr) != 64 {
		return errors.Wrap(err, "私钥长度不正确，需要64个十六进制字符")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return errors.Wrap(err, "解析私钥失败")
	}

	// 获取账户地址
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("无法获取公钥")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)
	logger.Infof("使用账户地址: %s", address.Hex())

	// 获取nonce
	nonce, err := ethClient.PendingNonceAt(ctx.Context, address)
	if err != nil {
		return errors.Wrap(err, "获取nonce失败")
	}

	// 获取链ID
	chainID, err := ethClient.ChainID(ctx.Context)
	if err != nil {
		return errors.Wrap(err, "获取链ID失败")
	}

	// 创建合约实例
	contractAddress := common.HexToAddress(governanceArgs.ContractAddress)
	contract, err := NewGovernance(contractAddress, ethClient)
	if err != nil {
		return errors.Wrap(err, "创建合约实例失败")
	}

	// 读取节点配置文件
	nodeArgs, err := loadNodeConfig(governanceArgs.NodeConfigFile)
	if err != nil {
		return errors.Wrap(err, "加载节点配置失败")
	}

	// 序列化节点参数
	argsBytes, err := json.Marshal(nodeArgs)
	if err != nil {
		return errors.Wrap(err, "序列化节点参数失败")
	}

	// 创建交易选项
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return errors.Wrap(err, "创建交易选项失败")
	}
	auth.GasPrice = big.NewInt(int64(governanceArgs.GasPrice))
	auth.GasLimit = governanceArgs.GasLimit
	auth.Nonce = big.NewInt(int64(nonce))
	// 调用合约的Propose方法
	logger.Infof("发送提案交易...")
	tx, err := contract.Propose(auth, uint8(governanceArgs.ProposalType), governanceArgs.Title, governanceArgs.Description, governanceArgs.Duration, argsBytes)
	if err != nil {
		return errors.Wrap(err, "调用合约Propose方法失败")
	}

	logger.Infof("提案交易已发送，交易哈希: %s", tx.Hash().Hex())

	// 等待交易确认
	logger.Info("等待交易确认...")
	receipt, err := waitForTransaction(ctx.Context, ethClient, tx.Hash())
	if err != nil {
		return errors.Wrap(err, "等待交易确认失败")
	}

	if receipt.Status == 1 {
		logger.Info("提案交易执行成功!")

		// 通过getLatestProposalID获取提案ID
		latestProposalID, err := contract.GetLatestProposalID(&bind.CallOpts{})
		if err != nil {
			logger.Warnf("获取最新提案ID失败: %v", err)
		} else {
			logger.Infof("✅ 提案创建成功! 提案ID: %d", latestProposalID)
		}

		// 解析事件日志获取提案ID
		if len(receipt.Logs) > 0 {
			logger.Infof("交易日志数量: %d", len(receipt.Logs))
			// 这里可以进一步解析事件日志来获取提案ID
			// 由于事件结构未知，暂时只显示基本信息
		}
	} else {
		return errors.New("提案交易执行失败")
	}

	return nil
}

// voteProposal 对提案进行投票
func voteProposal(ctx *cli.Context) error {
	logger := logrus.New()
	if governanceArgs.Verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Info("开始对提案进行投票...")

	// 连接到RPC
	ethClient, err := ethclient.DialContext(ctx.Context, governanceArgs.RpcURL)
	if err != nil {
		return errors.Wrap(err, "连接到RPC失败")
	}
	defer ethClient.Close()

	// 解析私钥
	privateKeyStr := governanceArgs.PrivateKey
	if strings.HasPrefix(privateKeyStr, "0x") {
		privateKeyStr = privateKeyStr[2:]
	}

	// 确保私钥长度为64个字符（32字节）
	if len(privateKeyStr) != 64 {
		return errors.Wrap(err, "私钥长度不正确，需要64个十六进制字符")
	}

	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return errors.Wrap(err, "解析私钥失败")
	}

	// 获取账户地址
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("无法获取公钥")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)
	logger.Infof("使用账户地址: %s", address.Hex())

	// 获取链ID
	chainID, err := ethClient.ChainID(ctx.Context)
	if err != nil {
		return errors.Wrap(err, "获取链ID失败")
	}

	// 创建合约实例
	contractAddress := common.HexToAddress(governanceArgs.ContractAddress)
	contract, err := NewGovernance(contractAddress, ethClient)
	if err != nil {
		return errors.Wrap(err, "创建合约实例失败")
	}

	// 创建交易选项
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return errors.Wrap(err, "创建交易选项失败")
	}
	auth.GasPrice = big.NewInt(int64(governanceArgs.GasPrice))
	auth.GasLimit = uint64(governanceArgs.GasLimit)

	// 调用合约的Vote方法
	logger.Infof("发送投票交易...")
	tx, err := contract.Vote(auth, uint64(governanceArgs.ProposalID), uint8(governanceArgs.VoteOption))
	if err != nil {
		return errors.Wrap(err, "调用合约Vote方法失败")
	}

	logger.Infof("投票交易已发送，交易哈希: %s", tx.Hash().Hex())

	// 等待交易确认
	logger.Info("等待交易确认...")
	receipt, err := waitForTransaction(ctx.Context, ethClient, tx.Hash())
	if err != nil {
		return errors.Wrap(err, "等待交易确认失败")
	}

	if receipt.Status == 1 {
		logger.Info("投票交易执行成功!")
	} else {
		return errors.New("投票交易执行失败")
	}

	return nil
}

// queryProposal 查询提案信息
func queryProposal(ctx *cli.Context) error {
	logger := logrus.New()
	if governanceArgs.Verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	logger.Info("开始查询提案信息...")

	// 连接到RPC
	ethClient, err := ethclient.DialContext(ctx.Context, governanceArgs.RpcURL)
	if err != nil {
		return errors.Wrap(err, "连接到RPC失败")
	}
	defer ethClient.Close()

	// 创建合约实例
	contractAddress := common.HexToAddress(governanceArgs.ContractAddress)
	contract, err := NewGovernance(contractAddress, ethClient)
	if err != nil {
		return errors.Wrap(err, "创建合约实例失败")
	}

	// 调用合约的Proposal方法
	proposal, err := contract.Proposal(&bind.CallOpts{}, uint64(governanceArgs.ProposalID))
	if err != nil {
		return errors.Wrap(err, "调用合约Proposal方法失败")
	}

	// 显示提案信息
	logger.Info("提案信息:")
	logger.Infof("  提案ID: %d", proposal.ID)
	logger.Infof("  提案类型: %d", proposal.Type)
	logger.Infof("  提案策略: %d", proposal.Strategy)
	logger.Infof("  提案人: %s", proposal.Proposer)
	logger.Infof("  标题: %s", proposal.Title)
	logger.Infof("  描述: %s", proposal.Desc)
	logger.Infof("  过期区块数量: %d", proposal.BlockNumber)
	logger.Infof("  总投票数: %d", proposal.TotalVotes)
	logger.Infof("  赞成票: %v", proposal.PassVotes)
	logger.Infof("  反对票: %v", proposal.RejectVotes)
	logger.Infof("  状态: %s", ProposalTypeMap[proposal.Status])
	logger.Infof("  提案创建区块号: %d", proposal.CreatedBlockNumber)
	logger.Infof("  提案通过区块号: %d", proposal.EffectiveBlockNumber)

	return nil
}

// loadNodeConfig 加载节点配置文件
func loadNodeConfig(filename string) (*ProposalArgs, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrap(err, "读取节点配置文件失败")
	}

	var args ProposalArgs
	err = json.Unmarshal(data, &args)
	if err != nil {
		return nil, errors.Wrap(err, "解析节点配置文件失败")
	}

	return &args, nil
}

// waitForTransaction 等待交易确认
func waitForTransaction(ctx context.Context, client *ethclient.Client, hash common.Hash) (*types.Receipt, error) {
	for {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err == nil {
			return receipt, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			continue
		}
	}
}
