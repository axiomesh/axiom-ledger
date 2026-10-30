# 节点链数据恢复手册

本文档描述当 axiom-ledger 节点因为**硬件故障（主板崩溃、掉电、磁盘损坏）**导致链数据不一致时，
如何诊断和恢复。

两个恢复工具都是**独立二进制**，不包含在 `draconis` 里，可以单独编译、单独分发到故障节点上执行：

| 工具 | 用途 | 源码 |
|---|---|---|
| `chainfix` | 诊断 / 修复 chain meta 与 blockfile 的高度、哈希不一致 | `cmd/chainfix` |
| `walfix` | 修复共识 WAL segment 尾部被截断的问题 | `cmd/walfix` |

> **为什么必须独立**：故障节点往往连 `draconis` 都启动不了。恢复工具必须能在节点**完全停止**的状态下，
> 独立读写 `storage/` 目录里的 pebble / blockfile 数据。把它们编进 `draconis` 会引入对节点初始化流程的
> 依赖，反而在故障时不可用。

---

## 目录

- [构建](#构建)
- [数据布局](#数据布局)
- [故障模式一：chain meta 与 blockfile 高度不一致](#故障模式一chain-meta-与-blockfile-高度不一致)
- [故障模式二：共识 WAL 损坏](#故障模式二共识-wal-损坏)
- [完整恢复流程](#完整恢复流程)
- [工具命令参考](#工具命令参考)
- [已知陷阱](#已知陷阱)
- [故障复盘：2026-10-30 la757](#故障复盘2026-10-30-la757)

---

## 构建

```bash
# 只构建恢复工具
make recovery-tools-static

# 分别构建
make chainfix-static
make walfix-static
```

产物：

```
./chainfix          # 静态二进制
./walfix
./bin/chainfix
./bin/walfix
```

两个工具都是 `CGO_ENABLED=0` + `-extldflags=-static` 编译的**纯静态二进制**，可以 `scp` 到任何
Linux 机器上直接跑，不需要目标机有 glibc 或 Go 环境。

分发：

```bash
scp bin/chainfix root@<node>:/root/chainfix-static
scp bin/walfix  root@<node>:/root/walfix-static
```

---

## 数据布局

节点的所有链数据都在 `<repoRoot>/storage/` 下，分成**四个互相独立的存储**：

```
storage/
├── blockchain/     pebble   chain meta（高度、块哈希、gas price）
├── ledger/         pebble   状态 trie + prune journal
├── snapshot/       pebble   账户/存储快照 + snapshot journal
├── blockfile/      裸文件   块数据（4 张表）
│   ├── header.ridx / header.NNNN.rdat
│   ├── transactions.ridx / transactions.NNNN.rdat
│   ├── extra.ridx / extra.NNNN.rdat
│   └── receipts.ridx / receipts.NNNN.rdat
├── consensus/      rosedb   共识 WAL（rbft 临时状态）
└── epoch/          pebble   epoch 信息
```

### 关键事实

**1. chain meta 是 chain 的"权威指针"**

它只存三样东西（protobuf `pb.ChainMeta`）：

| 字段 | 编号 | 内容 |
|---|---|---|
| `height` | 1 | varint |
| `block_hash` | 2 | 32 字节 |
| `gas_price` | 3 | `big.Int.Bytes()`（大端，无前导零） |

实际存储形态（`1e9` gas price）：

```
08 b49bed08          field 1: height = 18566580
12 20 <32 字节哈希>   field 2: block_hash
1a 04 3b9aca00       field 3: gas_price = 1e9
```

**2. blockfile 的四张表必须等长**

axiom-kit 的 `BlockFile.repair()` 会取所有表的**最小条目数**，并把每张表截断到该长度：

```go
min := uint64(math.MaxUint64)
for _, table := range bf.tables {
    if min > items { min = items }
}
for _, table := range bf.tables {
    table.truncate(min)      // ← 全部截到最短的那张
}
```

所以**任何一张表的 `.ridx` 丢失或变短，整个 blockfile 都会被截断**。

> ⚠️ 这就是为什么 `chainfix` 的备份必须用 `cp` 而不是 `mv`/`rename`：
> 节点用 `O_CREATE` 打开索引文件，文件不存在就会创建一个**空的**，
> 然后 `repair()` 把整个 blockfile 截成 0。

**3. blockfile 索引的映射关系**

`.ridx` 的每个条目是 6 字节：`uint16 filenum` + `uint32 offset`。

- 条目 0 是哨兵，指向第一个数据文件的开头
- 条目 N 的 offset = **第 N 个块负载的结束位置**
- 条目 N-1 的 offset = 第 N 个块负载的起始位置

所以读第 N 个块的负载需要索引里至少有 N+1 个条目。

**4. 块哈希不是 header 字节的 sha256**

axiom-kit 的 `BlockHeader.CalculateHash()` 会**重建**一个 header，只复制除 `Bloom` 外的所有字段，
然后对重建后的序列化结果做 sha256：

```go
blockheader := &BlockHeader{
    Number: h.Number, StateRoot: h.StateRoot, ..., Extra: h.Extra,
    // ← Bloom 没有被复制！
}
raw, _ := blockheader.Marshal()
data := sha256.Sum256(raw)
```

而 blockfile 存的是**含 bloom 的原始 header**。所以正确算法是：

```
block_hash = sha256( strip_field_9(marshal(header)) )
```

`chainfix` 的 `stripProtoField()` 就是干这个的。**用错了这个算法，chain meta 里的哈希永远对不上，
节点也无法同步**（因为 `executor.currentBlockHash` 来自 chain meta）。

**5. gas price 存的是 `big.Int.Bytes()`**

不是 protobuf varint。`chainfix` 从 header 的 field 10（`int64 gas_price`）读出 varint 值后，
转成 `big.Int.Bytes()` 再写入 chain meta。

**6. 状态 trie 的 root 由 chain meta 指向的块决定**

启动时：

```go
meta := ledger.ChainLedger.GetChainMeta()
ledger.Rollback(meta.Height)         // → RollbackState(height, stateRoot)
```

`RollbackState` 会：

```go
l.Clear()
l.accountCache.clear()
l.snapshot.Rollback(height)          // 回退 snapshot journal
l.pruneCache.Rollback(height)        // 重建 prune cache
l.refreshAccountTrie(stateRoot)      // ← trie 从 state root 重新加载
```

`stateRoot` 来自 **block header**，不是 chain meta。所以只要 blockfile 里那个高度的 header 是对的，
state 就一定会挂在正确的根上。

**7. snapshot 是独立存储，有自己的 rollback**

`snapshot` 在 `storage/snapshot`，是**独立的 pebble 库**。节点启动时独立回退它：

```go
if maxHeight < height  { return ErrorRollbackToHigherNumber }
if minHeight > height  { return ErrorRollbackTooMuch }
if maxHeight == height { return nil }   // ← 提前返回，不做任何 revert
```

> ⚠️ **绝对不要手工改 snapshot journal 的 `maxHeight`**。改了会让上面的 `maxHeight == height`
> 提前返回，journal 里的 revert 不再执行，snapshot 里就留着**被丢弃的那些块的状态**。
> 而 `GetAccount` 是**优先读 snapshot** 的：
>
> ```go
> // try getting account from snapshot first
> if l.snapshot != nil {
>     if innerAccount, err := l.snapshot.Account(address); err == nil {
>         return account      // ← 返回未来的状态！
>     }
> }
> ```
>
> 结果是读到的余额 / nonce 是未来的，状态直接错。**正确做法是让节点自己回退 snapshot。**

---

## 故障模式一：chain meta 与 blockfile 高度不一致

### 症状

节点启动时 panic：

```
panic: illegal chain meta!
```

来源 `internal/ledger/chain_ledger.go` 的 `checkChainMeta()`：

```go
if l.chainMeta.Height > bfHeight {
    panic("illegal chain meta!")
}
```

### 为什么会发生

`doBatchPersistExecutionResult()` 写数据时，**blockfile 先写，chain meta 后写**，
两者不在同一个原子事务里：

```go
l.bf.BatchAppendBlock(...)        // 1. 先写 blockfile
l.persistChainMeta(batcher, meta)  // 2. 再写 chain meta
batcher.Commit()                  // 3. 最后提交 pebble batch
```

崩溃发生在 1 和 3 之间时：

- blockfile 只到 N（缺了 N+1 ... N+k 的负载）
- chain meta 已经写了 N+k

→ 高度对不上 → panic。

**反过来**（chain meta 落后）不会 panic，`checkChainMeta()` 会自动用 blockfile 的高度重建 chain meta：

```go
if l.chainMeta.Height < bfHeight {
    l.chainMeta.Height = bfHeight
    // 用 blockfile 里那个块的 header 重建 hash 和 gas price
}
```

### 诊断

```bash
./chainfix-static ./ diagnose
```

输出会给出：

- chain meta 的高度 / 哈希 / gas price
- 四张 `.ridx` 的条目数（**必须等长**）
- 每个 `.rdat` 的实际大小 vs 索引声称的 offset
  （"index survived, data did not" = 索引还在但负载丢了）
- prune journal / snapshot journal 的范围
- header 索引的高度连续性

### 判断该回滚到哪里

**目标高度 = blockfile 能提供的最大高度**。

```bash
./chainfix-static ./ index
```

`effective height` 是**四张表的最小条目数减 1**——这就是节点实际能看到的高度。

### 关键一步：确认数据没分叉

回滚会把 chain meta 的哈希**重写**成 blockfile 里的哈希。如果网络已经分叉，
重写后节点会拿着错误的哈希去同步，症状是**永远追不上**或**被 peer 拒绝**。

所以回滚**之前**必须交叉验证：

```bash
# 本地：从 blockfile 算出某个高度的哈希
./chainfix-static ./ hashes 18566578 18566580

# peer：从 RPC 拿同一个高度的哈希
curl -s -X POST -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x11B4DB4",false],"id":1}' \
  http://localhost:8881
```

两边必须**完全一致**。

### 回滚

```bash
# 1. 先干跑，看清楚要做什么
./chainfix-static ./ rollback 18566580

# 2. 备份整个 storage 目录
cp -a storage /root/storage.bak.$(date +%s)

# 3. 执行
./chainfix-static ./ rollback 18566580 --apply

# 4. 验证
./chainfix-static ./ diagnose
```

`rollback` 会做三件事：

1. **截断超过目标高度的 `.ridx`**（先 `cp` 备份成 `.ridx.bak`，再**原地** truncate）
2. **重写 chain meta**：高度 = 目标高度，哈希 = blockfile 里的真实哈希，gas price = header 里的值
   （旧值备份到 `chain-meta-backup`）
3. **裁剪 prune journal**：删掉高于目标高度的条目，`maxHeight` 设为目标高度

`rollback` 还会做**两个安全检查**：

- **state root 预检**：目标高度的 state root 是否还在 ledger KV 里。
  不在 → **拒绝执行**（exit 1）。因为节点打不开那个高度的状态，回滚了也起不来。
- **snapshot journal 报告**：只报告范围，**不修改**（原因见上面第 7 条）。

### 恢复后

节点启动时会自动：

- `RollbackState(18566580, stateRoot)` → trie 挂回正确的根
- `snapshot.Rollback(18566580)` → 用 journal 回退 snapshot
- `pruneCache.Rollback(18566580)` → 重建 prune cache
- 从 `chain.Height + 1 = 18566581` 开始向 peer 同步

---

## 故障模式二：共识 WAL 损坏

### 症状

节点启动失败：

```
ERRO Startup failed [app] [err:init draconis failed: initialize consensus failed: invalid crc, the data may be corrupted]
```

或者 panic：

```
panic: runtime error: index out of range [1] with length 1
goroutine 1 [running]:
github.com/rosedblabs/wal.(*Reader).CurrentSegmentId(...)
	/home/liweixia/go/pkg/mod/github.com/rosedblabs/wal@v1.3.3/wal.go:288
github.com/rosedblabs/rosedb/v2.(*DB).loadIndexFromWAL(...)
	/home/liweixia/go/pkg/mod/github.com/rosedblabs/rosedb/v2@v2.3.3/db.go:570
```

### 为什么会发生

共识状态存在 `storage/consensus/`（rosedb），底层是 `rosedblabs/wal`。

WAL 的 chunk 格式：

```
0..3  crc32(header[4:] + payload)
4..5  payload 长度 (little endian uint16)
6     chunk 类型
```

数据按 32 KB 的 block 追加。崩溃时正在写的那个 chunk 只写了一半，
读取时 CRC 校验失败 → `ErrInvalidCRC`。

### 诊断

```bash
ls -la ./storage/consensus/
```

典型输出：

```
-rw-r--r--  1 root root     574 Oct 27 14:57 000000001.HINT
-rw-r--r--  1 root root      11 Oct 27 14:57 000000001.MERGEFIN
-rw-r--r--  1 root root  15022 Oct 27 14:57 000000001.SEG
-rw-r--r--  1 root root 1794048 Oct 27 15:01 000011346.SEG   ← 崩溃时正在写的
```

**时间戳明显晚于其他文件的，就是损坏的那个。**

```bash
./walfix-static ./storage/consensus
```

输出：

```
last segment: 000011346.SEG
  size:            1794048 bytes
  valid prefix:    1794048 bytes (4 blocks)
  ! 11 bytes of a partially written chunk at the tail
```

- `valid prefix < size` → 尾部有半个 chunk，**可以精确修复**
- `valid prefix == size` → 损坏在中间，walfix 无能为力 → 走下面的"整体清空"方案

### 修复

```bash
# 1. 停节点 + 备份
systemctl stop draconis
cp -a ./storage/consensus /root/consensus.bak.$(date +%s)

# 2. 干跑
./walfix-static ./storage/consensus

# 3. 执行
./walfix-static ./storage/consensus --apply
```

`walfix` 做的事：

1. 逐 chunk 校验 CRC，把 segment **截断到最后一个合法 chunk 的边界**。
   **坏 chunk 之前的数据全部保留**（实测：4000/4000 key 无丢失）。
2. 如果 WAL 只剩 **一个** segment，补一个空的。
   原因见下面的"已知陷阱"。

### 彻底清空（最后的手段）

共识状态全部可以从链上重建。启动时：

```go
common.WithApplied(chainMeta.Height),           // ← 来自 chain meta
common.WithDigest(chainMeta.BlockHash.String()),
```

所以：

```bash
systemctl stop draconis
cp -a ./storage/consensus /root/consensus.bak.$(date +%s)
rm -f ./storage/consensus/*.SEG ./storage/consensus/*.HINT ./storage/consensus/*.MERGEFIN
```

> ⚠️ 会丢失未提交的共识消息 / epoch 视图状态，节点需要走一轮 view change 重新稳定。

---

## 完整恢复流程

### 场景：节点因为主板崩溃起不来

```
                    ┌──────────────────────────────┐
                    │  systemctl stop draconis      │
                    └──────────────┬───────────────┘
                                   │
                    ┌──────────────▼───────────────┐
                    │ 备份 storage/（整个目录）      │
                    └──────────────┬───────────────┘
                                   │
              ┌────────────────────▼────────────────────┐
              │ ./chainfix-static ./ diagnose            │
              │ 判断是哪种故障                            │
              └───┬──────────────────────────────┬───────┘
                  │                              │
      chain meta > blockfile              invalid crc / WAL panic
                  │                              │
    ┌─────────────▼─────────────┐    ┌───────────▼────────────┐
    │ 交叉验证哈希（本地 vs peer）│    │ ./walfix-static <dir>  │
    │ chainfix hashes           │    │ 干跑，看 valid prefix   │
    └─────────────┬─────────────┘    └───────────┬────────────┘
                  │                              │
                  │                    ┌─────────▼─────────┐
                  │                    │ prefix < size ?    │
                  │                    └──┬─────────────┬──┘
                  │                     是 │             │ 否
                  │              ┌────────▼──────┐  ┌───▼─────────────┐
                  │              │ walfix --apply│  │ 清空 consensus  │
                  │              └───────────────┘  └─────────────────┘
                  │
    ┌─────────────▼──────────────┐
    │ chainfix rollback --apply  │
    │ （含 state root 预检）       │
    └─────────────┬──────────────┘
                  │
    ┌─────────────▼──────────────┐
    │ 确认 draconis 二进制版本     │
    │ 见"已知陷阱"第 3 条         │
    └─────────────┬──────────────┘
                  │
    ┌─────────────▼──────────────┐
    │ systemctl start draconis    │
    │ 观察日志，追赶链             │
    └─────────────────────────────┘
```

### 检查清单

```bash
# 0. 停节点
systemctl stop draconis

# 1. 备份（必做）
cp -a storage /root/storage.bak.$(date +%s)
ls -ld /root/storage.bak.*

# 2. 诊断
./chainfix-static ./ diagnose

# 3. 交叉验证哈希（关键！确认没分叉）
./chainfix-static ./ hashes <from> <to>
# 对比 peer 的 eth_getBlockByNumber

# 4. 回滚（先干跑）
./chainfix-static ./ rollback <height>

# 5. 回滚（执行）
./chainfix-static ./ rollback <height> --apply

# 6. 验证
./chainfix-static ./ diagnose      # 期望 verdict: OK

# 7. 检查二进制版本
./draconis 2>&1 | head -3

# 8. 启动
systemctl start draconis
tail -f logs/draconis.log
```

### 启动后确认

```bash
./chainfix-static ./ diagnose | head -20
```

期望：

```
--- chain meta (pebble) ---
  height:     18566604
  blockHash:  0x...
  gasPrice:   0x3b9aca00

--- verdict ---
  OK: chain meta height matches the blockfile height.
```

日志里应能看到连续出块：

```
[Execute-Block] Persisted block [executor] [height:18566604]
```

---

## 工具命令参考

### chainfix

```
usage: chainfix <repoRoot> <command> [flags]

read-only commands:
  diagnose            full report: chain meta, blockfile indexes, data files,
                      prune journal, and a verdict on what is broken
  meta                print the raw chain meta and its decoded fields
  index               print per-schema .ridx entry counts and the gaps between
                      them (the blockfile height is the minimum across schemas)
  verify              walk the header index, read the block height out of the
                      payload, and report whether the heights are contiguous
  scan-tail           walk the header payload past the end of the index and
                      report the blocks that are present but not indexed
  state-height        report the height of the state trie and compare it with
                      the blockfile and the chain meta
  hashes <from> <to>  print the block hash the blockfile implies for a range
                      of heights, so the data can be checked against what the
                      other nodes report
  compare <dir>       compare this node's .ridx indexes against another
                      node's blockfile directory and report where they differ

repair commands (all require --apply, all take a backup first):
  adopt-index <dir>   copy the .ridx indexes from another node's blockfile
                      directory into this node, keeping only the entries that
                      this node's .rdat files can actually serve
  rollback <height>   move the chain back to a height the blockfile can
                      serve: truncate the indexes, rewrite the chain meta
                      with the real block hash and gas price, and trim the
                      prune journal
  set-meta <height>   write an explicit chain meta height (and optionally the
                      block hash and gas price)
  prune-max <height>  set the prune journal maxHeight
  backup-meta         copy the current chain meta aside (no --apply needed)
  restore-meta        restore the chain meta from the backup

helper command (run on the healthy node):
  slice-index <dir>   write the tail of this node's .ridx indexes into <dir>
                      so it can be shipped to a peer that lost its trailing
                      entries. Combine with --from to cut only the last few
```

> **所有会修改数据的命令都必须显式加 `--apply`**，否则只干跑并打印将要做什么。

### walfix

```
usage: walfix <consensusDir> [--apply]
```

---

## 已知陷阱

### 1. 备份 `.ridx` 必须用 `cp`，不能用 `mv`

节点用 `O_CREATE` 打开索引文件。如果索引文件**不存在**：

1. 节点创建一个**空的** `.ridx`
2. `BlockFile.repair()` 取各表最小条目数 = 0
3. **把整个 blockfile 截断成 0** → 1856 万个块全部销毁

所以顺序必须是：**先 `cp` 备份 → 再原地 truncate**，文件必须始终在原位。

### 2. WAL 只剩一个 segment 会导致 panic

`rosedblabs/wal` 的 `Reader.CurrentSegmentId()` **没有边界检查**：

```go
func (r *Reader) CurrentSegmentId() SegmentID {
	return r.segmentReaders[r.currentReader].segment.id   // ← 可能越界
}
```

而 `loadIndexFromWAL` 在 `mergeFinSegmentId = 1` 时会 skip 到末尾：

```
轮次 0: currentReader=0, segmentReaders=[1.SEG] → id=1, 1<=1 → skip → currentReader=1
轮次 1: currentReader=1, segmentReaders 长度只有 1 → segmentReaders[1] → panic
```

（`Next()` 有 `if r.currentReader >= len(...)` 保护，`CurrentSegmentId()` 没有——这是 upstream 的 bug。）

所以 WAL 必须保持**至少两个 segment**。`walfix` 会在只剩一个时补一个空的。

### 3. 必须确认 draconis 二进制版本

日志第一行会打印版本：

```
Draconis version: v0.0.0-release-2024-04-01-rc1-draconis-v2-c6ef540c
```

尾部的 commit hash 要包含需要的修复。旧版本有两处会 panic：

| 位置 | 触发条件 |
|---|---|
| `NewView` 范围检查 | `blockHeader.Number > max`（`max` 是 prune journal 顶，不是链顶） |
| `CalNextGasPrice` | 链上 gas price 超出 `[min, max]` 时 |

### 4. 不要手工改 snapshot journal

见[数据布局第 7 条](#数据布局)。改了会让 snapshot 留着未来的状态。

### 5. 恢复后追赶会很慢

节点刚启动时，账户 trie cache / storage trie cache / accountCache **全是空的**，
每个块都要从磁盘重新加载历史账户和 storage slot。这是**追赶阶段的最坏情况**，
速度会随 cache 预热而回升。

如果确实太慢：

- **先查磁盘健康**（主板刚崩溃过，磁盘可能有问题）：
  ```bash
  dmesg -T | grep -iE "error|failed|timeout|reset|ata|scsi" | tail -30
  iostat -x 1 5
  ```
- **加大 cache**（安全）：
  ```toml
  [storage]
  kv_cache_size = 512                    # 默认 128

  [ledger]
  state_ledger_account_trie_cache_megabytes_limit = 512   # 默认 128
  state_ledger_storage_trie_cache_megabytes_limit = 512   # 默认 128
  state_ledger_account_cache_size = 8192                 # 默认 1024

  [snapshot]
  account_snapshot_cache_megabytes_limit = 512           # 默认 128
  contract_snapshot_cache_megabytes_limit = 512          # 默认 128
  ```
- **关闭 fsync**（最快，但有风险）：
  ```toml
  [storage]
  sync = false
  ```
  > ⚠️ 掉电 / 崩溃可能丢最近几个块。**这正是本次故障的成因类别，慎用。**

---

## 故障复盘：2026-10-30 la757

### 现象

```
panic: illegal chain meta!
```

chain meta 高度 18566590 > blockfile 高度 18566580，缺 10 个块。

### 处理过程

1. `chainfix diagnose` 确认：blockfile 负载完好，只是 chain meta 超前
2. `chainfix hashes` 交叉验证 → **发现哈希算法错误**
   - axiom-kit 的 `CalculateHash()` 不把 `Bloom` 计入哈希
   - 修正为 `sha256(strip_field_9(marshal(header)))`
   - 与 peer 哈希完全一致，确认**数据没有分叉**
3. `chainfix rollback 18566580 --apply`
   - 首次执行因 `mv` 备份导致 `receipts.ridx` 消失 → **改为 `cp` 备份 + 原地 truncate**
   - 加了 state root 预检（目标高度的 state root 必须在 ledger KV 里）
   - 加了 snapshot journal 报告（**不修改**）
4. 启动 → `invalid crc` → 共识 WAL 尾部截断
   - `walfix` 精确截断到最后一个合法 chunk，**数据零丢失**
5. 启动成功，从 18566581 开始追赶，最终追平。

### 暴露的问题（已修复）

| 问题 | 修复 |
|---|---|
| `chainfix` 备份用 `mv` 导致索引消失 | 改为 `cp` + 原地 truncate |
| `chainfix` 会修改 snapshot journal | 改为只报告 |
| `chainfix` 没有 state root 预检 | 新增预检，缺失则拒绝执行 |
| `chainfix` 缺索引文件时静默跳过 | 改为明确报错并退出 |
| 没有 WAL 修复工具 | 新增 `cmd/walfix` |

### 顺带修复的生产 bug

见 `git log fe578bd2`：

- `internal/finance/gas.go`：`CalNextGasPrice` 把越界的父 gas price **clamp** 到 `[min, max]`，
  而不是直接返回 `ErrGasOutOfRange`（恢复后 chain meta 短暂不一致时不会 panic）
- `internal/ledger/prune/prune_cache.go`：`Rollback` 处理 `EnablePrune=false` 和
  `maxHeight < target` 两种边界情况
- `internal/ledger/state_ledger.go`：`NewView` 的历史范围检查只校验下界
  （`max` 是 journal 顶，不是链顶，用它做上界是错的）
