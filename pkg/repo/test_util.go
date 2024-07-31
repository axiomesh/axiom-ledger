package repo

import (
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/axiomesh/axiom-kit/types"
)

var (
	MockOperatorKeys = []string{
		"b6477143e17f889263044f6cf463dc37177ac4526c4c39a7a344198457024a2f",
		"05c3708d30c2c72c4b36314a41f30073ab18ea226cf8c6b9f566720bfe2e8631",
		"85a94dd51403590d4f149f9230b6f5de3a08e58899dcaf0f77768efb1825e854",
		"72efcf4bb0e8a300d3e47e6a10f630bcd540de933f01ed5380897fc5e10dc95d",
		"5beed386fab613ab22a2c06be42be3cfb5ac27eef0747240f2dd8aa9e8e2a7a1",
	}
	MockOperatorAddrs = []string{
		"0xc7F999b83Af6DF9e67d0a37Ee7e900bF38b3D013",
		"0x79a1215469FaB6f9c63c1816b45183AD3624bE34",
		"0x97c8B516D19edBf575D72a172Af7F418BE498C37",
		"0xc0Ff2e0b3189132D815b8eb325bE17285AC898f8",
		"0xC791c813C59940398dDC0379DCCdb3b1Ad47b705",
	}
	MockConsensusKeys = []string{
		"0x099383c2b41a282936fe9e656467b2ad6ecafd38753eefa080b5a699e3276372",
		"0x5d21b741bd16e05c3a883b09613d36ad152f1586393121d247bdcfef908cce8f",
		"0x42cc8e862b51a1c21a240bb2ae6f2dbad59668d86fe3c45b2e4710eebd2a63fd",
		"0x6e327c2d5a284b89f9c312a02b2714a90b38e721256f9a157f03ec15c1a386a6",
		"0x10bf2dacd178ae68b3cb72b5b85d0e28c7092e665fb05ca2611c351a0aae273f",
	}
	MockConsensusPubKeys = []string{
		"0xac9bb2675ab6b60b1c6d3ed60e95bdabb16517525458d8d50fa1065014184823556b0bd97922fab8c688788006e8b1030cd506d19101522e203769348ea10d21780e5c26a5c03c0cfcb8de23c7cf16d4d384140613bb953d446a26488fbaf6e0",
		"0xa41eb8e086872835b17e323dadd569d98caa8645b694c5a3095a1a0790a4390cb0db7a79af5411328bc17c9fb213d7f407a471c929e8aa2fe33e9e1472adb000c86990dd81078906d2cccd831a7fa4a0772a094e02d58db361162e95ac5e29fa",
		"0xa8c8b2635518df0212e92e3056ffbc3388cdcaf175227c914cdf713419bb25d3ba73e9f4981330aa20b3016ee668c19f0d3408226aeca43261abf01bd17b3cb5992ada1d0b3bb90b28930eed40d95b3e0a72bf6df5a30feb3330a9e7561eb82b",
		"0xa1a0d4cea22621b1b61adf2e050b4f191b6504344271229eb613045a832e92b3ee152d3f32ef7e79408e190c80709b0006ddc3f10cae1da52a98fb27c0d5eaa29b906c416b3b60e8ec09619fd67a3a4fa7b468f3de96acd120fe1c39ff579ea1",
		"0xa3a599ecb9b7dc46e9e6caa19659c4891238fb2f110be0e7bf9943e560b5ff412b665770e8cbfd0e2f35cc0453e1dd261366f71a74d1a3a7aa4acdd7157e215cbf75898324bc101db9d10daf605bdeef229c8e154d919cb41fa96728b2127576",
	}
	MockP2PKeys = []string{
		"0xce374993d8867572a043e443355400ff4628662486d0d6ef9d76bc3c8b2aa8a8",
		"0x43dd946ade57013fd4e7d0f11d84b94e2fda4336829f154ae345be94b0b63616",
		"0x875e5ef34c34e49d35ff5a0f8a53003d8848fc6edd423582c00edc609a1e3239",
		"0xf0aac0c25791d0bd1b96b2ec3c9c25539045cf6cc5cc9ad0f3cb64453d1f38c0",
		"0x052cbbe64f2d02e09c088befdcd0d2940715ccda7e259aa6b00e3e3df74d084e",
	}
	MockP2PPubKeys = []string{
		"0xd4c0ac1567bcb2c855bb1692c09ab2a2e2c84c45376592674530ce95f1fda351",
		"0xeec45cda21da07acb89d3e7db8ce78933773b4b1daf567c2efddc6d5fd687001",
		"0xb20879ca4baa02370d8a5d033be54e49df5163c0c8257cb0d49b38385aa14930",
		"0x98cdfe5dd41fe8d336d45572778ced304f8571e21c822de148b960d34ccca256",
		"0xb3b36df35fb4ded619490fd61ac1d1bf64f6ac9bb1e6cec9890abe38229179bd",
	}
	MockP2PIDs = []string{
		"12D3KooWQ8s11eFGTNGn4rReiCp4J448rWDwbUqxQ1j3NTsWR4YG",
		"12D3KooWRtQs8GswoXGLqDk5YXBgb6QXdnuybgapdWnjf75oRVeG",
		"12D3KooWMoLEaoMnzbiC1PRX84We6XS4dABve8uNUvcUAFqaAiqM",
		"12D3KooWL6rKYzSqxgvkrQvSMcuxV9da3XPgnx2jJ2XiA3E9GZbj",
		"12D3KooWMuqqPQkXcH2yRqKr9tQh2v6w6k1RCC3EzKEuojZt2Fkp",
	}
	MockDefaultIsDataSyncers = []bool{false, false, false, false, true}
	MockDefaultStakeNumbers  = []*types.CoinNumber{types.CoinNumberByAxc(1000), types.CoinNumberByAxc(1000), types.CoinNumberByAxc(1000), types.CoinNumberByAxc(1000), types.CoinNumberByAxc(0)}

	MockDefaultAccountAddrs = []string{
		"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		"0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
		"0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC",
		"0x90F79bf6EB2c4f870365E785982E1f101E93b906",
		"0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65",
		"0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc",
		"0x976EA74026E726554dB657fA54763abd0C3a0aa9",
		"0x14dC79964da2C08b23698B3D3cc7Ca32193d9955",
		"0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f",
		"0xa0Ee7A142d267C1f36714E4a8F75612F20a79720",
		"0xBcd4042DE499D14e55001CcbB24a551F3b954096",
		"0x71bE63f3384f5fb98995898A86B02Fb2426c5788",
		"0xFABB0ac9d68B0B445fB7357272Ff202C5651694a",
		"0x1CBd3b2770909D4e10f157cABC84C7264073C9Ec",
		"0xdF3e18d64BC6A983f673Ab319CCaE4f1a57C7097",
		"0xcd3B766CCDd6AE721141F452C550Ca635964ce71",
		"0x2546BcD3c84621e976D8185a91A922aE77ECEc30",
		"0xbDA5747bFD65F08deb54cb465eB87D40e51B197E",
		"0xdD2FD4581271e230360230F9337D5c0430Bf44C0",
		"0x8626f6940E2eb28930eFb4CeF49B2d1F2C9C1199",
	}
)

func MockRepo(t testing.TB) *Repo {
	return MockRepoWithNodeID(t, 4, 1, MockDefaultIsDataSyncers, MockDefaultStakeNumbers)
}

func MockRepoWithNodeID(t testing.TB, nodeCount int, nodeID uint64, isDataSyncers []bool, stakeNumbers []*types.CoinNumber) *Repo {
	require.True(t, nodeCount <= len(MockConsensusKeys), "nodeCount must be less than or equal to the number of MockConsensusKeys")
	require.True(t, int(nodeID) <= nodeCount, "nodeID must be less than or equal to nodeCount")

	repoRoot := t.TempDir()
	consensusKeystore, err := GenerateConsensusKeystore(repoRoot, MockConsensusKeys[nodeID-1], DefaultKeystorePassword)
	require.Nil(t, err)
	p2pKeystore, err := GenerateP2PKeystore(repoRoot, MockP2PKeys[nodeID-1], DefaultKeystorePassword)
	require.Nil(t, err)
	rep := &Repo{
		RepoRoot:          repoRoot,
		Config:            defaultConfig(),
		ConsensusConfig:   defaultConsensusConfig(),
		GenesisConfig:     defaultGenesisConfig(),
		ConsensusKeystore: consensusKeystore,
		P2PKeystore:       p2pKeystore,
		StartArgs:         &StartArgs{},
	}
	rep.Config.Ledger.EnablePreload = true
	rep.GenesisConfig.EpochInfo.StakeParams.MinValidatorStake = types.CoinNumberByAxc(1)

	rep.GenesisConfig.Nodes = lo.Map(MockConsensusPubKeys[0:nodeCount], func(key string, index int) GenesisNodeInfo {
		return GenesisNodeInfo{
			ConsensusPubKey: key,
			P2PPubKey:       MockP2PPubKeys[index],
			OperatorAddress: MockOperatorAddrs[index],
			IsDataSyncer:    isDataSyncers[index],
			MetaData: GenesisNodeMetaData{
				Name: "node" + strconv.Itoa(index+1),
				Desc: "node" + strconv.Itoa(index+1),
			},
			StakeNumber:    stakeNumbers[index],
			CommissionRate: 0,
		}
	})

	rep.GenesisConfig.Accounts = lo.Map(rep.GenesisConfig.Nodes, func(item GenesisNodeInfo, index int) *Account {
		return &Account{
			Address: item.OperatorAddress,
			Balance: types.CoinNumberByAxc(10000000),
		}
	})

	for _, addr := range MockDefaultAccountAddrs {
		rep.GenesisConfig.Accounts = append(rep.GenesisConfig.Accounts, &Account{
			Address: addr,
			Balance: types.CoinNumberByAxc(1000_0000),
		})
	}

	rep.GenesisConfig.SmartAccountAdmin = rep.GenesisConfig.Nodes[0].OperatorAddress

	return rep
}
