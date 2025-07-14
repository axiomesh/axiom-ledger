# Axiom-Ledger Node Deployment

# Generating Deployment Package

Navigate to the axiom-ledger repository and execute the packaging script

```bash
git clone git@github.com:axiomesh/draconis.git
cd draconis && make package version=dev
```

The deployment package will be generated in the project directory as: axiom-ledger-dev.tar.gz

# Node Configuration Generation

Upload the deployment package to the server and execute the following commands to generate the node's configuration files and private keys:

1. Extract the deployment package

```bash
mkdir node && cd node
cp draconis-dev.tar.gz ./
tar -zxvf draconis-dev.tar.gz
```

2. Initialize the node

Generate configuration files and node keys (randomly generated)

```bash
./init.sh
```

After execution, the node's account address and p2p-id will be printed. You can also view them again using `./axiom-ledger config node-info`.

The node's account address and p2p-id will be used in the genesis configuration.

# Modifying Node Configuration

Configuration items are located in the config.toml file in the deployment path.

## Modifying Port Configuration

Refer to the configuration file documentation. If there are port conflicts, modify the port configuration in the config.toml file.

## Modifying Genesis Configuration

**Must be modified before startup; modifications after block generation will not take effect**

1. Modify the number of validator nodes, default is 4, change it to the actual number of deployed nodes

`genesis.epoch_info.consensus_params.max_validator_num`

2. Modify the validator node list

The ID is the node identifier, an integer type, and must not conflict between nodes. Account_address and p2p_node_id are generated during node initialization. Consensus_voting_power represents the probability of this node to produce a block.

Modify the validator node list to correspond to the configurations of multiple nodes generated earlier.

```toml
[[genesis.epoch_info.validator_set]]
    id = 1
Account_address = '0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013'
    p2p_node_id = '16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF'
    consensus_voting_power = 1000
```

3. Modify the p2p bootstrap node address list

The configuration item is `genesis.epoch_info.p2p_bootstrap_node_addresses`.

Change the addresses to the p2p addresses of the actual deployed nodes (modify IP, p2p port, and p2p-id).

**The p2p address format is: /ip4/${IP}/tcp/${P2P_PORT}/p2p/${P2P_ID}**

```toml
p2p_bootstrap_node_addresses = [
  '/ip4/127.0.0.1/tcp/4001/p2p/16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF',
  '/ip4/127.0.0.1/tcp/4002/p2p/16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK'
]
```

4. Modify the list of accounts with tokens in the genesis

The configuration item is `genesis.accounts`

```toml
accounts = [
    '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266',
    '0x70997970C51812dc3A010C7d01b50e0d17dc79C8',
    '0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC',
    '0x90F79bf6EB2c4f870365E785982E1f101E93b906',
    '0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65',
    '0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc',
    '0x976EA74026E726554dB657fA54763abd0C3a0aa9',
    '0x14dC79964da2C08b23698B3D3cc7Ca32193d9955',
    '0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f',
    '0xa0Ee7A142d267C1f36714E4a8F75612F20a79720',
    '0xBcd4042DE499D14e55001CcbB24a551F3b954096',
    '0x71bE63f3384f5fb98995898A86B02Fb2426c5788',
    '0xFABB0ac9d68B0B445fB7357272Ff202C5651694a',
    '0x1CBd3b2770909D4e10f157cABC84C7264073C9Ec',
    '0xdF3e18d64BC6A983f673Ab319CCaE4f1a57C7097',
    '0xcd3B766CCDd6AE721141F452C550Ca635964ce71',
    '0x2546BcD3c84621e976D8185a91A922aE77ECEc30',
    '0xbDA5747bFD65F08deb54cb465eB87D40e51B197E',
    '0xdD2FD4581271e230360230F9337D5c0430Bf44C0',
    '0x8626f6940E2eb28930eFb4CeF49B2d1F2C9C1199'
]
```

Default accounts and corresponding private keys:

```plaintext
Account #0: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
Private Key: 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
    
Account #1: 0x70997970C51812dc3A010C7d01b50e0d17dc79C8
Private Key: 0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d
    
Account #2: 0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC
Private Key: 0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a
    
Account #3: 0x90F79bf6EB2c4f870365E785982E1f101E93b906
Private Key: 0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6
    
Account #4: 0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65
Private Key: 0x47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a
    
Account #5: 0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc
Private Key: 0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba
    
Account #6: 0x976EA74026E726554dB657fA54763abd0C3a0aa9
Private Key: 0x92db14e403b83dfe3df233f83dfa3a0d7096f21ca9b0d6d6b8d88b2b4ec1564e
    
Account #7: 0x14dC79964da2C08b23698B3D3cc7Ca32193d9955
Private Key: 0x4bbbf85ce3377467afe5d46f804f221813b2bb87f24d81f60f1fcdbf7cbf4356
    
Account #8: 0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f
Private Key: 0xdbda1821b80551c9d65939329250298aa3472ba22feea921c0cf5d620ea67b97
    
Account #9: 0xa0Ee7A142d267C1f36714E4a8F75612F20a79720
Private Key: 0x2a871d0798f97d79848a013d4936a73bf4cc922c825d33c1cf7073dff6d409c6
    
Account #10: 0xBcd4042DE499D14e55001CcbB24a551F3b954096
Private Key: 0xf214f2b2cd398c806f84e317254e0f0b801d0643303237d97a22a48e01628897
    
Account #11: 0x71bE63f3384f5fb98995898A86B02Fb2426c5788
Private Key: 0x701b615bbdfb9de65240bc28bd21bbc0d996645a3dd57e7b12bc2bdf6f192c82
    
Account #12: 0xFABB0ac9d68B0B445fB7357272Ff202C5651694a
Private Key: 0xa267530f49f8280200edf313ee7af6b827f2a8bce2897751d06a843f644967b1
    
Account #13: 0x1CBd3b2770909D4e10f157cABC84C7264073C9Ec
Private Key: 0x47c99abed3324a2707c28affff1267e45918ec8c3f20b8aa892e8b065d2942dd
    
Account #14: 0xdF3e18d64BC6A983f673Ab319CCaE4f1a57C7097
Private Key: 0xc526ee95bf44d8fc405a158bb884d9d1238d99f0612e9f33d006bb0789009aaa
    
Account #15: 0xcd3B766CCDd6AE721141F452C550Ca635964ce71
Private Key: 0x8166f546bab6da521a8369cab06c5d2b9e46670292d85c875ee9ec20e84ffb61
    
Account #16: 0x2546BcD3c84621e976D8185a91A922aE77ECEc30
Private Key: 0xea6c44ac03bff858b476bba40716402b03e41b8e97e276d1baec7c37d42484a0
    
Account #17: 0xbDA5747bFD65F08deb54cb465eB87D40e51B197E
Private Key: 0x689af8efa8c651a91ad287602527f3af2fe9f6501a7ac4b061667b5a93e037fd
    
Account #18: 0xdD2FD4581271e230360230F9337D5c0430Bf44C0
Private Key: 0xde9be858da4a475276426320d5e9262ecfc3ba460bfac56360bfa6c4c28b4ee0
    
Account #19: 0x8626f6940E2eb28930eFb4CeF49B2d1F2C9C1199
Private Key: 0xdf57089febbacf7ba0bc227dafbffa9fc08a93fdc68e1e42411a14efcf23656
```

# Starting All Nodes

Execute the start.sh script in the deployment path of each node.

Other scripts:

- Start node: `./start.sh`
- Restart node: `./restart.sh`
- Stop node: `./stop.sh`
- Check if node is running: `./status.sh`
- Check node version: `./version.sh`

# Updating Binary

**Note: Do not directly replace axiom-ledger in the deployment package; this is just a script. The actual binary is located in tools/bin/axiom-ledger within the deployment package.**

Enter the deployment directory and execute the binary update script, providing the new binary path:

```bash
./update_binary.sh ~/new_axiom-ledger
```