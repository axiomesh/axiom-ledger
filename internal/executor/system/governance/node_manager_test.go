package governance

import (
	"encoding/json"
	"fmt"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/base"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/mock_ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

func TestName(t *testing.T) {
	args := &NodeExtraArgs{
		Nodes: []*NodeMember{
			{
				NodeId:  "16Uiu2HAm2HeK145KTfLaURhcoxBUMZ1PfhVnLRfnmE8qncvXWoZj",
				Address: "0xd0091F6D0b39B9E9D2E9051fA46d13B63b8C7B18",
				Name:    "node5",
				ID:      5,
			},
		},
	}

	data, _ := json.Marshal(args)
	fmt.Println(ethcommon.Bytes2Hex(data))

}
func PrepareNodeManager(t *testing.T) (*Governance, *mock_ledger.MockStateLedger, *rbft.EpochInfo) {
	logger := logrus.New()
	gov := NewGov(&common.SystemContractConfig{
		Logger: logger,
	})

	mockCtl := gomock.NewController(t)
	stateLedger := mock_ledger.NewMockStateLedger(mockCtl)

	account := ledger.NewMockAccount(2, types.NewAddressByStr(common.GovernanceContractAddr))

	stateLedger.EXPECT().GetOrCreateAccount(gomock.Any()).Return(account).AnyTimes()
	stateLedger.EXPECT().AddLog(gomock.Any()).AnyTimes()
	stateLedger.EXPECT().SetBalance(gomock.Any(), gomock.Any()).AnyTimes()

	err := InitCouncilMembers(stateLedger, []*repo.Admin{
		{
			Address: admin1,
			Weight:  1,
			Name:    "111",
		},
		{
			Address: admin2,
			Weight:  1,
			Name:    "222",
		},
		{
			Address: admin3,
			Weight:  1,
			Name:    "333",
		},
		{
			Address: admin4,
			Weight:  1,
			Name:    "444",
		},
	})
	assert.Nil(t, err)

	g := repo.GenesisEpochInfo(true)
	g.EpochPeriod = 100
	g.StartBlock = 1
	err = base.InitEpochInfo(stateLedger, g)
	assert.Nil(t, err)

	err = InitNodeMembers(stateLedger, []*repo.NodeName{
		{
			ID:   1,
			Name: "111",
		},
	}, &rbft.EpochInfo{
		ValidatorSet: []rbft.NodeInfo{
			{
				ID:             1,
				AccountAddress: admin1,
				P2PNodeID:      "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
			},
		},
	})

	return gov, stateLedger, g
}

func TestNodeManager_RunForPropose(t *testing.T) {
	gov, stateLedger, _ := PrepareNodeManager(t)

	testcases := []struct {
		Caller string
		Type   ProposalType
		Data   *NodeExtraArgs
		Err    error
		HasErr bool
	}{
		{
			Caller: admin1,
			Type:   NodeAdd,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin2,
						Name:    "222",
					},
				},
			},
			HasErr: false,
		},
		{
			Caller: "0x1000000000000000000000000000000000000000",
			Type:   NodeAdd,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: ErrNotFoundCouncilMember,
		},
		{
			Caller: admin1,
			Type:   NodeAdd,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: ErrRepeatedNodeID,
		},
		{
			Caller: admin1,
			Type:   NodeAdd,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin1,
						Name:    "111",
					},
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: ErrRepeatedNodeAddress,
		},
		{
			Caller: admin1,
			Type:   NodeAdd,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin2,
						Name:    "111",
					},
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin2,
						Name:    "111",
					},
				},
			},
			Err: ErrRepeatedNodeName,
		},
		{
			Caller: admin1,
			Type:   NodeRemove,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: nil,
		},
		{
			Caller: "0x1000000000000000000000000000000000000000",
			Type:   NodeRemove,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: ErrNotFoundCouncilMember,
		},
		{
			Caller: admin1,
			Type:   NodeRemove,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: ErrRepeatedNodeID,
		},
		{
			Caller: admin1,
			Type:   NodeRemove,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin1,
						Name:    "111",
					},
					{
						NodeId:  "16Uiu2HAmRypzJbdbUNYsCV2VVgv9UryYS5d7wejTJXT73mNLJ8AK",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			Err: ErrNotFoundNodeID,
		},
		{
			Caller: admin1,
			Type:   NodeRemove,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin2,
						Name:    "111",
					},
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin2,
						Name:    "111",
					},
				},
			},
			Err: ErrNotFoundNodeAddress,
		},
		{
			Caller: admin1,
			Type:   NodeRemove,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "222",
					},
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "222",
					},
				},
			},
			Err: ErrNotFoundNodeName,
		},
	}

	for _, test := range testcases {
		addr := types.NewAddressByStr(test.Caller).ETHAddress()
		logs := make([]common.Log, 0)
		gov.SetContext(&common.VMContext{
			CurrentUser:   &addr,
			CurrentHeight: 1,
			CurrentLogs:   &logs,
			StateLedger:   stateLedger,
		})

		data, err := json.Marshal(test.Data)
		assert.Nil(t, err)

		if test.Data == nil {
			data = []byte("")
		}

		err = gov.Propose(uint8(test.Type), "test", "test desc", 100, data)
		if test.Err != nil {
			assert.Equal(t, test.Err, err)
		} else {
			if test.HasErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		}
	}
}

func TestNodeManager_RunForNodeUpgradePropose(t *testing.T) {
	gov, stateLedger, _ := PrepareNodeManager(t)

	testcases := []struct {
		Caller string
		Data   *NodeExtraArgs
		Err    error
		HasErr bool
	}{
		{
			Caller: admin1,
			Data: &NodeExtraArgs{
				Nodes: []*NodeMember{
					{
						NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
						Address: admin1,
						Name:    "111",
					},
				},
			},
			HasErr: false,
		},
	}

	for _, test := range testcases {
		addr := types.NewAddressByStr(test.Caller).ETHAddress()
		logs := make([]common.Log, 0)
		gov.SetContext(&common.VMContext{
			CurrentUser:   &addr,
			CurrentHeight: 1,
			CurrentLogs:   &logs,
			StateLedger:   stateLedger,
		})

		data, err := json.Marshal(test.Data)
		assert.Nil(t, err)

		if test.Data == nil {
			data = []byte("")
		}

		err = gov.Propose(uint8(NodeUpgrade), "test", "test desc", 100, data)
		if test.Err != nil {
			assert.Equal(t, test.Err, err)
		} else {
			if test.HasErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		}
	}
}

func TestNodeManager_GetNodeMembers(t *testing.T) {
	mockCtl := gomock.NewController(t)
	stateLedger := mock_ledger.NewMockStateLedger(mockCtl)

	account := ledger.NewMockAccount(1, types.NewAddressByStr(common.GovernanceContractAddr))

	stateLedger.EXPECT().GetOrCreateAccount(gomock.Any()).Return(account).AnyTimes()
	stateLedger.EXPECT().SetBalance(gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().AddLog(gomock.Any()).AnyTimes()

	err := InitCouncilMembers(stateLedger, []*repo.Admin{
		{
			Address: admin1,
			Weight:  1,
			Name:    "111",
		},
		{
			Address: admin2,
			Weight:  1,
			Name:    "222",
		},
		{
			Address: admin3,
			Weight:  1,
			Name:    "333",
		},
		{
			Address: admin4,
			Weight:  1,
			Name:    "444",
		},
	})
	assert.Nil(t, err)
	err = InitNodeMembers(stateLedger, []*repo.NodeName{
		{
			ID:   1,
			Name: "111",
		},
	}, &rbft.EpochInfo{
		ValidatorSet: []rbft.NodeInfo{
			{
				ID:             1,
				AccountAddress: admin1,
				P2PNodeID:      "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
			},
		},
	})

	members, err := GetNodeMembers(stateLedger)
	assert.Nil(t, err)
	t.Log("GetNodeMembers-members:", members)
}

func TestNodeManager_RunForAddVote(t *testing.T) {
	gov, stateLedger, g := PrepareNodeManager(t)

	// propose
	addr := types.NewAddressByStr(admin1)
	ethaddr := addr.ETHAddress()

	logs := make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})

	arg := NodeExtraArgs{
		Nodes: []*NodeMember{
			{
				Name:    "222",
				NodeId:  "16Uiu2HAmTMVkvoGdwjHkqSEhdSM5P7L8ronFfnDePhmQSN6CvR8m",
				Address: admin2,
			},
		},
	}
	data, err := json.Marshal(arg)
	assert.Nil(t, err)

	err = gov.Propose(uint8(NodeAdd), "test", "test desc", 100, data)
	assert.Nil(t, err)

	g.DataSyncerSet = append(g.DataSyncerSet, rbft.NodeInfo{
		ID:                   9,
		AccountAddress:       "0x88E9A1cE92b4D6e4d860CFBB5bB7aC44d9b548f8",
		P2PNodeID:            "16Uiu2HAkwmNbfH8ZBdnYhygUHyG5mSWrWTEra3gwHWt9dGTUSRVV",
		ConsensusVotingPower: 100,
	})
	err = base.InitEpochInfo(stateLedger, g)
	assert.Nil(t, err)

	testcases := []struct {
		Caller     string
		ProposalID uint64
		Res        VoteResult
		Err        error
		HasErr     bool
	}{
		{
			Caller:     admin1,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        Pass,
			Err:        ErrUseHasVoted,
		},
		{
			Caller:     admin2,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        Pass,
			HasErr:     false,
		},
		{
			Caller:     admin2,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        Pass,
			Err:        ErrUseHasVoted,
		},
		{
			Caller:     "0x1000000000000000000000000000000000000000",
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        Pass,
			Err:        ErrNotFoundCouncilMember,
		},
	}

	for _, test := range testcases {
		logs = make([]common.Log, 0)
		addr := types.NewAddressByStr(test.Caller).ETHAddress()
		gov.SetContext(&common.VMContext{
			StateLedger:   stateLedger,
			CurrentHeight: 1,
			CurrentLogs:   &logs,
			CurrentUser:   &addr,
		})

		err := gov.Vote(test.ProposalID, uint8(test.Res))
		if test.Err != nil {
			assert.Equal(t, test.Err, err)
		} else {
			if test.HasErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		}
	}
}

func TestNodeManager_RunForAddVote_Approved(t *testing.T) {
	gov, stateLedger, _ := PrepareNodeManager(t)

	// propose
	addr := types.NewAddressByStr(admin1)
	ethaddr := addr.ETHAddress()

	logs := make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})

	arg := NodeExtraArgs{
		Nodes: []*NodeMember{
			{
				Name:    "222",
				NodeId:  "16Uiu2HAmTMVkvoGdwjHkqSEhdSM5P7L8ronFfnDePhmQSN6CvR8m",
				Address: admin2,
			},
		},
	}
	data, err := json.Marshal(arg)
	assert.Nil(t, err)

	err = gov.Propose(uint8(NodeAdd), "test", "test desc", 100, data)
	assert.Nil(t, err)

	testcases := []struct {
		Caller     string
		ProposalID uint64
		Res        VoteResult
	}{
		{
			Caller:     admin2,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        Pass,
		},
		{
			Caller:     admin3,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        Pass,
		},
	}

	for _, test := range testcases {
		logs = make([]common.Log, 0)
		addr := types.NewAddressByStr(test.Caller).ETHAddress()
		gov.SetContext(&common.VMContext{
			StateLedger:   stateLedger,
			CurrentHeight: 1,
			CurrentLogs:   &logs,
			CurrentUser:   &addr,
		})

		err := gov.Vote(test.ProposalID, uint8(test.Res))
		assert.Nil(t, err)
	}

	nodes, err := GetNodeMembers(stateLedger)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(nodes))
}

func TestNodeManager_RunForRemoveVote_Approved(t *testing.T) {
	gov, stateLedger, g := PrepareNodeManager(t)

	dataSyncSet := rbft.NodeInfo{
		ID:                   9,
		AccountAddress:       admin4,
		P2PNodeID:            "16Uiu2HAkwmNbfH8ZBdnYhygUHyG5mSWrWTEra3gwHWt9dGTUSRVV",
		ConsensusVotingPower: 100,
	}
	g.DataSyncerSet = append(g.DataSyncerSet, dataSyncSet)
	err := base.InitEpochInfo(stateLedger, g)
	assert.Nil(t, err)

	err = InitNodeMembers(stateLedger, []*repo.NodeName{
		{
			ID:   1,
			Name: "111",
		},
		{
			ID:   9,
			Name: "444",
		},
	}, &rbft.EpochInfo{
		ValidatorSet: []rbft.NodeInfo{
			{
				ID:             1,
				AccountAddress: admin1,
				P2PNodeID:      "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
			},
		},
		DataSyncerSet: []rbft.NodeInfo{dataSyncSet},
	})
	// propose
	addr := types.NewAddressByStr(admin1)
	ethaddr := addr.ETHAddress()

	logs := make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})

	err = gov.Propose(uint8(NodeRemove), "test title", "test desc", 100, generateNodeRemoveProposeData(t, NodeExtraArgs{
		Nodes: []*NodeMember{
			{
				ID:      9,
				NodeId:  "16Uiu2HAkwmNbfH8ZBdnYhygUHyG5mSWrWTEra3gwHWt9dGTUSRVV",
				Address: admin4,
				Name:    "444",
			},
		},
	}))
	assert.Nil(t, err)

	ethaddr = types.NewAddressByStr(admin2).ETHAddress()
	logs = make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})
	err = gov.Vote(gov.proposalID.GetID()-1, uint8(Pass))
	assert.Nil(t, err)

	ethaddr = types.NewAddressByStr(admin3).ETHAddress()
	logs = make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})
	err = gov.Vote(gov.proposalID.GetID()-1, uint8(Pass))
	assert.Nil(t, err)

	nodes, err := GetNodeMembers(stateLedger)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nodes))
}

func TestNodeManager_RunForRemoveVote(t *testing.T) {
	gov, stateLedger, _ := PrepareNodeManager(t)

	// propose
	addr := types.NewAddressByStr(admin1)
	ethaddr := addr.ETHAddress()

	logs := make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})

	err := gov.Propose(uint8(NodeRemove), "test title", "test desc", 100, generateNodeRemoveProposeData(t, NodeExtraArgs{
		Nodes: []*NodeMember{
			{
				NodeId:  "16Uiu2HAmJ38LwfY6pfgDWNvk3ypjcpEMSePNTE6Ma2NCLqjbZJSF",
				Address: admin1,
				Name:    "111",
			},
		},
	}))
	assert.Nil(t, err)

	testcases := []struct {
		Caller     string
		ProposalID uint64
		Res        uint8
		Err        error
		HasErr     bool
	}{
		{
			Caller:     admin2,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        uint8(Pass),
			HasErr:     false,
		},
		{
			Caller:     admin1,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        uint8(Pass),
			Err:        ErrUseHasVoted,
		},
		{
			Caller:     "0x1000000000000000000000000000000000000000",
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        uint8(Pass),
			Err:        ErrNotFoundCouncilMember,
		},
	}

	for _, test := range testcases {
		ethaddr = types.NewAddressByStr(test.Caller).ETHAddress()
		logs := make([]common.Log, 0)
		gov.SetContext(&common.VMContext{
			StateLedger:   stateLedger,
			CurrentHeight: 1,
			CurrentUser:   &ethaddr,
			CurrentLogs:   &logs,
		})

		err := gov.Vote(test.ProposalID, uint8(test.Res))
		if test.Err != nil {
			assert.Equal(t, test.Err, err)
		} else {
			if test.HasErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		}
	}
}

func TestNodeManager_RunForUpgradeVote(t *testing.T) {
	gov, stateLedger, _ := PrepareNodeManager(t)

	// propose
	addr := types.NewAddressByStr(admin1)
	ethaddr := addr.ETHAddress()

	logs := make([]common.Log, 0)
	gov.SetContext(&common.VMContext{
		StateLedger:   stateLedger,
		CurrentHeight: 1,
		CurrentUser:   &ethaddr,
		CurrentLogs:   &logs,
	})
	err := gov.Propose(uint8(NodeUpgrade), "test title", "test desc", 100, generateNodeUpgradeProposeData(t, UpgradeExtraArgs{
		DownloadUrls: []string{"http://127.0.0.1:9000"},
		CheckHash:    "hash",
	}))
	assert.Nil(t, err)

	testcases := []struct {
		Caller     string
		ProposalID uint64
		Res        uint8
		Err        error
		HasErr     bool
	}{
		{
			Caller:     admin2,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        uint8(Pass),
			HasErr:     false,
		},
		{
			Caller:     admin1,
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        uint8(Pass),
			Err:        ErrUseHasVoted,
		},
		{
			Caller:     "0x1000000000000000000000000000000000000000",
			ProposalID: gov.proposalID.GetID() - 1,
			Res:        uint8(Pass),
			Err:        ErrNotFoundCouncilMember,
		},
	}

	for _, test := range testcases {
		ethaddr = types.NewAddressByStr(test.Caller).ETHAddress()
		logs := make([]common.Log, 0)
		gov.SetContext(&common.VMContext{
			StateLedger:   stateLedger,
			CurrentHeight: 1,
			CurrentUser:   &ethaddr,
			CurrentLogs:   &logs,
		})

		err := gov.Vote(test.ProposalID, uint8(test.Res))
		if test.Err != nil {
			assert.Equal(t, test.Err, err)
		} else {
			if test.HasErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		}
	}
}

func generateNodeRemoveProposeData(t *testing.T, extraArgs NodeExtraArgs) []byte {
	data, err := json.Marshal(extraArgs)
	assert.Nil(t, err)
	return data
}

func generateNodeUpgradeProposeData(t *testing.T, extraArgs UpgradeExtraArgs) []byte {
	data, err := json.Marshal(extraArgs)
	assert.Nil(t, err)
	return data
}
