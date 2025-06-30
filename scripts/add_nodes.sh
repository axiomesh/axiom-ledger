#!/bin/bash
set -x

N=$1

APP_NAME=draconis

RPC=http://localhost:8881

cd build && rm -rf node$N && cp -r node4 node$N

cd node$N && rm -rf storage logs *.json

# port
new_jsonrpc=888$N
new_websocket=999$N
new_p2p=400$N
new_pprof=5312$N
new_monitor=4001$N

# incentive_address
new_address="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"


# 文件路径
file_path="config.toml"

# 修改port信息
sed -i '' "s/^  jsonrpc = .*/  jsonrpc = $new_jsonrpc/" $file_path
sed -i '' "s/^  websocket = .*/  websocket = $new_websocket/" $file_path
sed -i '' "s/^  p2p = .*/  p2p = $new_p2p/" $file_path
sed -i '' "s/^  pprof = .*/  pprof = $new_pprof/" $file_path
sed -i '' "s/^  monitor = .*/  monitor = $new_monitor/" $file_path


# 修改incentive_address
sed -i '' "s/^  incentive_address = .*/  incentive_address = '$new_address'/" $file_path

operator_path="operator.key"
operator_key="ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

echo "$operator_key" > $operator_path

## generate key
./$APP_NAME keystore generate

### step2: 发起新增节点交易
./$APP_NAME node --rpc $RPC register --node-name node$N --node-desc sync_node$N --sender $operator_key
