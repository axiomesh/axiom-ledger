# tx-client

交易重放客户端 - 从RPC获取区块并重放交易到区块链，支持节点治理功能

## 功能特性

- **区块获取**: 从RPC端点获取区块数据并保存到本地文件
- **交易重放**: 从本地文件读取交易并按序重放到区块链
- **节点治理**: 支持发起节点管理提案和投票功能
- **并发处理**: 支持多线程并发获取区块数据
- **进度显示**: 实时显示操作进度
- **详细日志**: 支持详细输出模式

## 安装

### 从源码编译

```bash
# 克隆项目
git clone <repository-url>
cd axiom-ledger

# 编译tx-client
make build-tx-client

# 或者直接使用go build
go build -o bin/tx-client ./cmd/tx-client
```

### 验证安装

```bash
./bin/tx-client version
```

## 使用方法

### 1. 获取区块数据

从RPC端点获取指定范围的区块数据并保存到本地文件。

```bash
# 基本用法
./bin/tx-client get-blocks --rpc-url http://127.0.0.1:8881 --start-block 1 --end-block 100

# 指定输出目录和并发数
./bin/tx-client get-blocks \
  --rpc-url http://127.0.0.1:8881 \
  --start-block 1 \
  --end-block 1000 \
  --output-dir ./blocks \
  --concurrency 5 \
  --verbose

# 获取最新区块
./bin/tx-client get-blocks --rpc-url http://127.0.0.1:8881 --start-block 1
```

**参数说明:**
- `--rpc-url`: RPC端点地址
- `--start-block`: 开始区块号
- `--end-block`: 结束区块号（0表示获取到最新区块）
- `--output-dir`: 输出目录路径
- `--concurrency`: 并发数（默认5）
- `--verbose`: 详细输出模式

### 2. 重放交易

从本地文件读取交易数据并重放到区块链。

```bash
# 基本用法
./bin/tx-client replay --rpc-url http://127.0.0.1:8881

# 指定区块范围
./bin/tx-client replay \
  --rpc-url http://127.0.0.1:8881 \
  --start-block 1 \
  --end-block 100 \
  --interval 100ms

# 演习模式（不实际发送交易）
./bin/tx-client replay \
  --rpc-url http://127.0.0.1:8881 \
  --dry-run \
  --verbose
```

**参数说明:**
- `--rpc-url`: RPC端点地址
- `--input-dir`: 输入目录路径（包含区块文件）
- `--start-block`: 重放开始区块号
- `--end-block`: 重放结束区块号（0表示重放所有可用区块）
- `--interval`: 发送交易间隔时间
- `--dry-run`: 演习模式，不实际发送交易
- `--verbose`: 详细输出模式

### 3. 节点治理功能

#### 发起提案

```bash
# 添加节点提案
./bin/tx-client propose \
  --private-key 0x1234567890abcdef... \
  --type 1 \
  --title "添加新节点" \
  --description "添加节点6到网络" \
  --nodes nodes.json \
  --rpc-url http://127.0.0.1:8881

# 删除节点提案
./bin/tx-client propose \
  --private-key 0x1234567890abcdef... \
  --type 2 \
  --title "删除节点" \
  --description "删除故障节点" \
  --nodes nodes.json \
  --rpc-url http://127.0.0.1:8881
```

#### 投票

```bash
# 赞成投票
./bin/tx-client vote \
  --private-key 0x1234567890abcdef... \
  --proposal-id 1 \
  --option 0 \
  --rpc-url http://127.0.0.1:8881

# 反对投票
./bin/tx-client vote \
  --private-key 0x1234567890abcdef... \
  --proposal-id 1 \
  --option 1 \
  --rpc-url http://127.0.0.1:8881
```

#### 查询提案

```bash
# 查询提案信息
./bin/tx-client query-proposal \
  --proposal-id 1 \
  --rpc-url http://127.0.0.1:8881
```

**治理功能参数说明:**
- `--private-key`: 私钥（用于签名交易）
- `--contract`: 治理合约地址
- `--type`: 提案类型（1: 添加节点, 2: 删除节点）
- `--title`: 提案标题
- `--description`: 提案描述
- `--duration`: 提案持续时间（区块数）
- `--nodes`: 节点配置文件路径
- `--proposal-id`: 提案ID
- `--option`: 投票选项（0: 赞成, 1: 反对）
- `--gas-price`: Gas价格（wei）
- `--gas-limit`: Gas限制

### 节点配置文件格式

创建 `nodes.json` 文件，格式如下：

```json
{
  "Nodes": [
    {
      "NodeId": "16Uiu2HAmEFqQMC6247sZNmrcjGzRqAdbWb6tZn3wuxcrpGMd4NjA",
      "Address": "0x99Dce8548A011C3D508a65c6020EE64365c4D3b9",
      "Name": "node6"
    }
  ]
}
```

- `NodeId`: P2P节点ID
- `Address`: 账户地址
- `Name`: 节点名称

## 文件格式

### 区块文件格式

每个区块保存为独立的JSON文件，文件名格式：`block_{区块号}.json`

```json
{
  "block_number": 1,
  "block_hash": "0x1234567890abcdef...",
  "transactions": [
    {
      "hash": "0xabcdef1234567890...",
      "raw": "0xf869808609184e72a0008398968094a2f28344131970356c4a112d1e634e51589aa57c80808301433ca035995b37b9dde4d0909d0e9f7c1eb797991ead4fbd991c373f3033045a566e34a024ddf3cbadca0ddfad10ff8b41b1f9f0f95ca69baaa2f55e39f08f491bf0957e"
    }
  ],
  "timestamp": 1640995200
}
```

## 使用场景

### 1. 数据备份和恢复
```bash
# 备份区块数据
./bin/tx-client get-blocks --rpc-url http://node1:8881 --start-block 1 --end-block 10000

# 恢复交易到新节点
./bin/tx-client replay --rpc-url http://node2:8881 --start-block 1 --end-block 10000
```

### 2. 网络测试
```bash
# 获取测试网络的区块数据
./bin/tx-client get-blocks --rpc-url http://testnet:8881 --start-block 1 --end-block 1000

# 重放到本地测试网络
./bin/tx-client replay --rpc-url http://localhost:8881 --dry-run --verbose
```

### 3. 节点治理
```bash
# 添加新节点到网络
./bin/tx-client propose --private-key 0x123... --type 1 --nodes new_nodes.json

# 对提案进行投票
./bin/tx-client vote --private-key 0x456... --proposal-id 1 --option 0
```

## 故障排除

### 常见问题

1. **连接RPC失败**
   ```
   Error: 连接到RPC失败: dial tcp 127.0.0.1:8881: connect: connection refused
   ```
   解决方案：检查RPC端点是否正常运行，确认端口和地址正确。

2. **交易发送失败**
   ```
   Error: 发送交易失败: insufficient funds for gas * price + value
   ```
   解决方案：确保账户有足够的余额支付Gas费用。

3. **文件权限错误**
   ```
   Error: 创建输出目录失败: permission denied
   ```
   解决方案：检查目录权限，确保有写入权限。

4. **私钥格式错误**
   ```
   Error: 解析私钥失败: invalid hex string
   ```
   解决方案：确保私钥是有效的十六进制字符串，可以包含或不包含0x前缀。

### 调试模式

使用 `--verbose` 参数启用详细输出模式，获取更多调试信息：

```bash
./bin/tx-client get-blocks --rpc-url http://127.0.0.1:8881 --verbose
./bin/tx-client replay --rpc-url http://127.0.0.1:8881 --verbose
./bin/tx-client propose --private-key 0x123... --verbose
```

## 性能优化

### 1. 并发设置
- 根据网络带宽和RPC端点性能调整并发数
- 默认并发数为5，可根据实际情况调整

### 2. 文件描述符限制
- 大量并发操作可能需要提高文件描述符限制
- Linux系统：`ulimit -n 65536`
- macOS系统：`ulimit -n 65536`

### 3. 内存使用
- 大范围区块获取会占用较多内存
- 建议分批处理大量区块数据

## 安全注意事项

1. **私钥安全**
   - 不要在命令行中直接使用私钥
   - 考虑使用环境变量或配置文件
   - 生产环境中使用硬件钱包或安全的密钥管理

2. **网络安全**
   - 使用HTTPS连接RPC端点
   - 在可信网络环境中使用

3. **数据验证**
   - 重放前验证交易数据的完整性
   - 使用演习模式测试交易

## 开发

### 项目结构
```
cmd/tx-client/
├── main.go           # 主程序入口
├── common.go         # 共享数据结构
├── getblocks.go      # 区块获取功能
├── replay.go         # 交易重放功能
├── governance.go     # 治理功能
├── replay_test.go    # 测试文件
└── README.md         # 文档
```

### 构建测试版本
```bash
go build -race -o bin/tx-client-test ./cmd/tx-client
```

### 运行测试
```bash
go test ./cmd/tx-client
```

## 许可证

本项目采用 [MIT License](LICENSE) 许可证。 