# Development Tips

# Configuration Loading Logic

The sequence for loading node configurations is as follows:

1. First, construct a Config instance using the default hardcoded configurations in the code.
2. Read the configurations from config.toml and load them into the Config instance.
3. Scan the environment variables and load the read variables into the Config instance.

Therefore, the priority and sequence of loading node configurations are opposite: **Environment Variables > Configuration File > Hardcoded Default Configurations in the Code**. The higher priority will override the lower priority.

**Note: Before executing config generate for node initialization, if environment variables for configuration items are set, the values from the environment variables will be used as the configuration values in the generated config.toml.**

Correspondence of configuration items and environment variables: The names of the configuration items in the configuration file are converted to all uppercase and connected with underscores, with the prefix DRACONIS_ (for genesis.toml it is DRACONIS_GENESIS_; for consensus.toml it is DRACONIS_CONSENSUS_).

For example, if the configuration item for the JSON-RPC port is port.jsonrpc, the corresponding environment variable is DRACONIS_PORT_JSONRPC.

# Node Process Management

Nodes started with make cluster and make solo, as well as nodes deployed manually through make package, come with default node process management scripts. For specific usage, refer to the document ["node deploy Document"](./node-deploy.md).

**Note: When upgrading the binary, it is essential to use the update_binary.sh script in the deployment path, as the axiom-ledger file in the deployment path is not the binary but a proxy script (automatically setting the repo path and loading environment variables from .env.sh). The actual node binary is located in tools/bin/axiom-ledger within the deployment path.**

# Rapid Deployment of Multiple Nodes on a Single Machine (4 Nodes or 8 Nodes)

You can quickly generate configuration files for 4/8 nodes using the following script (the script needs to be placed in the root directory of the axiom-ledger code repository, and make build should be executed beforehand).

Create a .env.sh file to store the environment variables to be changed for all node configurations. When generating node configurations, the values corresponding to the environment variables will be used first.

```bash
#! /bin/bash
set -e
```

Create a generate_nodes.sh script (in the same directory as .env.sh script). You can modify the script to avoid port conflicts.

```bash
#!/usr/bin/env bash

set -e

CURRENT_PATH=$(
  cd $(dirname ${BASH_SOURCE[0]})
  pwd
)
source ${CURRENT_PATH}/.env.sh
BUILD_PATH=${CURRENT_PATH}/nodes

# 4 or 8, when it is 8, it includes 4 candidate nodes
N=4

echo "===> Generating $N nodes configuration on ${BUILD_PATH}"
for ((i = 1; i < N + 1; i = i + 1)); do
  root=${BUILD_PATH}/node${i}
  echo ""
  echo "generate node${i} config"
  mkdir -p ${root}
  rm -rf ${root}/*
  rm -f ${root}/.env.sh
  cp -rf ${CURRENT_PATH}/scripts/package/* ${root}/
  cp -f ${CURRENT_PATH}/bin/draconis ${root}/tools/bin/
  echo "export DRACONIS_PORT_JSONRPC=888${i}" >>${root}/.env.sh
  echo "export DRACONIS_PORT_WEBSOCKET=999${i}" >>${root}/.env.sh
  echo "export DRACONIS_PORT_P2P=400${i}" >>${root}/.env.sh
  echo "export DRACONIS_PORT_PPROF=5312${i}" >>${root}/.env.sh
  echo "export DRACONIS_PORT_MONITOR=4001${i}" >>${root}/.env.sh
  # For 8 nodes, epoch needs to be enabled
  # ${root}/draconis config generate --default-node-index ${i} --epoch-enable
  # For 4 nodes
  ${root}/draconis config generate --default-node-index ${i}
done
```

Run generate_nodes.sh, which will generate the node deployment package in the nodes folder in the same directory as the script. Execute the start.sh script in each node's deployment package to start the nodes in the background.

# Rapid Deployment of Multiple Nodes on Multiple Machines (4 Nodes or 8 Nodes)

Refer to the rapid deployment of multiple nodes on a single machine, but add a list of seed node addresses to the .env.sh file and modify the IP to the actual IP of the deployed machines.

For 4-node configuration:

```bash
export DRACONIS_GENESIS_EPOCH_INFO_P2P_BOOTSTRAP_NODE_ADDRESSES="\
/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF;\
/ip4/127.0.0.1/tcp/4002/p2p/16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK;\
/ip4/127.0.0.1/tcp/4003/p2p/16Uiu2HAmTwEET536QC9MZmYFp1NUshjRuaq5YSH1sLjW65WasvRk;\
/ip4/127.0.0.1/tcp/4004/p2p/16Uiu2HAmQBFTnRr84M3xNhi3EcWmgZnnBsDgewk4sNtpA3smBsHJ;\
"
```

For 8-node configuration:

```bash
export DRACONIS_GENESIS_EPOCH_INFO_P2P_BOOTSTRAP_NODE_ADDRESSES="\
/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF;\
/ip4/127.0.0.1/tcp/4002/p2p/16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK;\
/ip4/127.0.0.1/tcp/4003/p2p/16Uiu2HAmTwEET536QC9MZmYFp1NUshjRuaq5YSH1sLjW65WasvRk;\
/ip4/127.0.0.1/tcp/4004/p2p/16Uiu2HAmQBFTnRr84M3xNhi3EcWmgZnnBsDgewk4sNtpA3smBsHJ;\
/ip4/127.0.0.1/tcp/4005/p2p/16Uiu2HAm2HeK145KTfLaURhcoxBUMZ1PfhVnLRfnmE8qncvXWoZj;\
/ip4/127.0.0.1/tcp/4006/p2p/16Uiu2HAm2CVtLveAtroaN7pcR8U2saBKjwYqRAikMSwxqdoYMxtv;\
/ip4/127.0.0.1/tcp/4007/p2p/16Uiu2HAmQv3m5SSyYAoafKmYbTbGmXBaS4DXHXR9wxWKQ9xLzC3n;\
/ip4/127.0.0.1/tcp/4008/p2p/16Uiu2HAkx1o5fzWLdAobanvE6vqbf1XSbDSgCnid3AoqDGQYFVxo;\
"
```

Then run generate_nodes.sh, which will generate the node deployment package in the nodes folder in the same directory as the script. Upload the deployment package to the respective servers and execute the start.sh script in the deployment package to start the nodes.

# Debugging Techniques

## Ledger Debugging

Currently, tools for rolling back and replaying blocks in the ledger are provided. Therefore, if there are inconsistencies in the ledger or other ledger-related bugs, these tools can be used along with the ledger logs for debugging.

For instance, if there is a ledger fork bug and two nodes fork at block 1000:

On each node's deployment path, perform the following actions:

1. Roll back to the previous block height (this will not directly modify the original storage folder but will replay to the specified block height in a new folder named storage-rollback-${block_number})

```bash
./draconis ledger simple-rollback --target-block-number 999
```

The storage at block height 999 is located in the storage-rollback-999 folder within the deployment package.

2. Replay the transactions of block 1000 on the storage at block height 999 (this command will output debug logs of the ledger for the repeated transactions to a file without timestamps)

```bash
export DRACONIS_LOG_MODULE_STORAGE=debug && export DRACONIS_LOG_DISABLE_TIMESTAMP=true && ./draconis ledger simple-sync --source-storage ./storage --target-storage ./storage-rollback-999 --target-block-number 1000 -f > ./block-1000.log
```

Finally, compare the ledger logs of the problematic block replayed on both nodes to identify any inconsistencies.

## Consensus Debugging

The consensus module itself processes transactions and consensus messages in a serial manner (similar to a state machine). Currently, the only way to debug it is to compare the logs of multiple nodes.

**Note: During development and testing, make sure to set the log level of the consensus to debug (default is info), and synchronize the clocks of the machines during deployment.**

## Unrecoverable Node Panic and Restart

When a node experiences a panic and cannot recover after restart (due to consensus and block execution reasons), the node can be started in read-only mode (without starting the p2p and consensus modules) using the readonly parameter. Subsequently, the block and transaction data of the node can be queried through jsonrpc for debugging.

Enter the node's deployment directory and execute the following command to start the node in read-only mode:

```bash
cd ${deploy_path}
./draconis start --readonly
```