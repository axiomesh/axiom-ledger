#!/bin/bash
set -x

cd scripts/build/node5

# 添加node5质押池
./axiom-ledger node join-candidate-set \
--node-id 5 \
--commission-rate 0 \
--sender ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80

## 添加质押代币
./axiom-ledger staking add-stake \
--pool-id 5 \
--amount 5000000axc \
--sender ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80

./axiom-ledger staking add-stake \
--pool-id 5 \
--amount 5000000axc \
--sender 7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6

## 查看总的代币数量
./axiom-ledger staking pool-info --pool-id 5