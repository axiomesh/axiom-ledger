package sys_contract

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/hexutil"
	"github.com/axiomesh/axiom-ledger/cmd/axiom-ledger/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager_client"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance"
)

var NodeCMDProposeNodeRegisterArgs node_manager_client.NodeMetaData

var NodeCMDJoinCandidateSetArgs = struct {
	NodeID         uint64
	CommissionRate uint64
}{}

var NodeCMDExitArgs = struct {
	NodeID uint64
}{}

var NodeCMDUpdateMetaDataArgs = struct {
	NodeID uint64
	node_manager_client.NodeMetaData
}{}

var NodeCMDUpdateOperatorArgs = struct {
	NodeID   uint64
	Operator string
}{}

var NodeCMDNodeInfoArgs = struct {
	NodeID uint64
}{}

var NodeCMDNodeInfosArgs = struct {
	NodeIDs cli.Uint64Slice
}{}

var NodeCMD = &cli.Command{
	Name:  "node",
	Usage: "The node commands",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "register",
			Usage:  "Propose a register node proposal",
			Action: NodeActions{}.register,
			//Action: GovernanceActions{}.proposeNodeRegister,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "title",
					Destination: &GovernanceCMDProposeArgs.Title,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "desc",
					Destination: &GovernanceCMDProposeArgs.Desc,
					Required:    false,
				},
				&cli.Uint64Flag{
					Name:        "block-number",
					Destination: &GovernanceCMDProposeArgs.BlockNumber,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "node-name",
					Destination: &NodeCMDProposeNodeRegisterArgs.Name,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "node-desc",
					Destination: &NodeCMDProposeNodeRegisterArgs.Desc,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "node-image-url",
					Destination: &NodeCMDProposeNodeRegisterArgs.ImageURL,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "node-website-url",
					Destination: &NodeCMDProposeNodeRegisterArgs.WebsiteURL,
					Required:    false,
				},
				senderFlag,
				common.KeystorePasswordFlag(),
			},
		},
		{
			Name:   "join-candidate-set",
			Usage:  "Join candidate set and create a staking pool",
			Action: NodeActions{}.joinCandidateSet,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "node-id",
					Destination: &NodeCMDJoinCandidateSetArgs.NodeID,
					Required:    true,
				},
				&cli.Uint64Flag{
					Name:        "commission-rate",
					Destination: &NodeCMDJoinCandidateSetArgs.CommissionRate,
					Required:    false,
				},
				senderFlag,
			},
		},
		{
			Name:   "exit",
			Usage:  "Exit from candidate and validator set, will disable staking pool",
			Action: NodeActions{}.exit,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "node-id",
					Destination: &NodeCMDExitArgs.NodeID,
					Required:    true,
				},
				senderFlag,
			},
		},
		{
			Name:   "update-metadata",
			Usage:  "Update node metadata(incremental update)",
			Action: NodeActions{}.updateMetaData,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "node-id",
					Destination: &NodeCMDUpdateMetaDataArgs.NodeID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "new-name",
					Destination: &NodeCMDUpdateMetaDataArgs.Name,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "new-desc",
					Destination: &NodeCMDUpdateMetaDataArgs.Desc,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "new-image-url",
					Destination: &NodeCMDUpdateMetaDataArgs.ImageURL,
					Required:    false,
				},
				&cli.StringFlag{
					Name:        "new-website-url",
					Destination: &NodeCMDUpdateMetaDataArgs.WebsiteURL,
					Required:    false,
				},
				senderFlag,
			},
		},
		{
			Name:   "update-operator",
			Usage:  "Update node operator",
			Action: NodeActions{}.updateOperator,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "node-id",
					Destination: &NodeCMDUpdateOperatorArgs.NodeID,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "new-operator",
					Destination: &NodeCMDUpdateOperatorArgs.Operator,
					Required:    false,
				},
				senderFlag,
			},
		},
		{
			Name:   "info",
			Usage:  "Get node info by node id",
			Action: NodeActions{}.nodeInfo,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "node-id",
					Destination: &NodeCMDNodeInfoArgs.NodeID,
					Required:    true,
				},
			},
		},
		{
			Name:   "infos",
			Usage:  "Batch Get node info by node id list",
			Action: NodeActions{}.nodeInfos,
			Flags: []cli.Flag{
				&cli.Uint64SliceFlag{
					Name:        "node-ids",
					Destination: &NodeCMDNodeInfosArgs.NodeIDs,
					Required:    true,
				},
			},
		},
		{
			Name:   "total-count",
			Usage:  "Get total node count",
			Action: NodeActions{}.totalCount,
		},
		{
			Name:   "active-validator-set",
			Usage:  "Get active validator set",
			Action: NodeActions{}.activeValidatorSet,
		},
		{
			Name:   "data-syncer-set",
			Usage:  "Get data syncer set",
			Action: NodeActions{}.dataSyncerSet,
		},
		{
			Name:   "candidate-set",
			Usage:  "Get candidate set",
			Action: NodeActions{}.candidateSet,
		},
		{
			Name:   "pending-inactive-set",
			Usage:  "Get pending inactive set",
			Action: NodeActions{}.pendingInactiveSet,
		},
		{
			Name:   "exited-set",
			Usage:  "Get exited set",
			Action: NodeActions{}.exitedSet,
		},
	},
}

type NodeActions struct{}

func (a GovernanceActions) proposeNodeRegister(ctx *cli.Context) error {
	r, err := common.PrepareRepoWithKeystore(ctx)
	if err != nil {
		return err
	}

	return a.doPropose(ctx, uint8(governance.NodeRegister), func(client *ethclient.Client) ([]byte, error) {
		if GovernanceCMDProposeArgs.Title == "" {
			GovernanceCMDProposeArgs.Title = fmt.Sprintf("register node[%s]", NodeCMDProposeNodeRegisterArgs.Name)
		}
		if GovernanceCMDProposeArgs.Desc == "" {
			GovernanceCMDProposeArgs.Desc = fmt.Sprintf("register node[%s]: %s", NodeCMDProposeNodeRegisterArgs.Name, NodeCMDProposeNodeRegisterArgs.Desc)
		}
		if GovernanceCMDProposeArgs.BlockNumber == 0 {
			currentBlockNumber, err := client.BlockNumber(ctx.Context)
			if err != nil {
				return nil, err
			}
			epochManager, err := EpochActions{}.bindContract(ctx)
			if err != nil {
				return nil, err
			}
			epochInfo, err := epochManager.CurrentEpoch(&bind.CallOpts{Context: ctx.Context})
			if err != nil {
				return nil, errors.Wrap(err, "get current epoch failed")
			}
			GovernanceCMDProposeArgs.BlockNumber = currentBlockNumber + epochInfo.EpochPeriod
		}

		signStruct := governance.NodeRegisterExtraArgsSignStruct{
			ConsensusPubKey: r.ConsensusKeystore.PublicKey.String(),
			P2PPubKey:       r.P2PKeystore.PublicKey.String(),
			MetaData: node_manager.NodeMetaData{
				Name:       NodeCMDProposeNodeRegisterArgs.Name,
				Desc:       NodeCMDProposeNodeRegisterArgs.Desc,
				ImageURL:   NodeCMDProposeNodeRegisterArgs.ImageURL,
				WebsiteURL: NodeCMDProposeNodeRegisterArgs.WebsiteURL,
			},
		}
		signStructRaw, err := json.Marshal(signStruct)
		if err != nil {
			return nil, err
		}
		consensusPrivateKeySignature, err := r.ConsensusKeystore.PrivateKey.Sign(signStructRaw)
		if err != nil {
			return nil, err
		}
		p2pPrivateKeySignature, err := r.P2PKeystore.PrivateKey.Sign(signStructRaw)
		if err != nil {
			return nil, err
		}
		extra, err := json.Marshal(governance.NodeRegisterExtraArgs{
			ConsensusPubKey:              signStruct.ConsensusPubKey,
			P2PPubKey:                    signStruct.P2PPubKey,
			MetaData:                     signStruct.MetaData,
			ConsensusPrivateKeySignature: hexutil.Encode(consensusPrivateKeySignature),
			P2PPrivateKeySignature:       hexutil.Encode(p2pPrivateKeySignature),
		})
		if err != nil {
			return nil, err
		}
		return extra, nil
	})
}

func (_ NodeActions) bindContract(ctx *cli.Context) (*node_manager_client.BindingContract, *ethclient.Client, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "dial rpc failed")
	}

	contract, err := node_manager_client.NewBindingContract(ethcommon.HexToAddress(syscommon.NodeManagerContractAddr), client)
	if err != nil {
		return nil, nil, errors.Wrap(err, "bind node contract failed")
	}
	return contract, client, nil
}

// function register(NodeInfo memory info) external;
func (a NodeActions) register(ctx *cli.Context) error {
	nodeManager, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	r, err := common.PrepareRepoWithKeystore(ctx)
	if err != nil {
		return err
	}

	opa, err := GetSenderAddress()
	if err != nil {
		return errors.Wrap(err, "get operator address failed")
	}

	infoArgs := node_manager_client.NodeInfo{
		ConsensusPubKey: r.ConsensusKeystore.PublicKey.String(),
		P2PPubKey:       r.P2PKeystore.PublicKey.String(),
		P2PID:           r.P2PKeystore.P2PID(),
		Operator:        opa,
		MetaData:        NodeCMDProposeNodeRegisterArgs,
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		return nodeManager.Register(opts, infoArgs)
	}, nil)
}

// function joinCandidateSet(uint64 nodeID, uint64 commissionRate) external;
func (a NodeActions) joinCandidateSet(ctx *cli.Context) error {
	nodeManager, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		return nodeManager.JoinCandidateSet(opts, NodeCMDJoinCandidateSetArgs.NodeID, NodeCMDJoinCandidateSetArgs.CommissionRate)
	}, nil)
}

// function exit(uint64 nodeID) external;
func (a NodeActions) exit(ctx *cli.Context) error {
	nodeManager, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		return nodeManager.Exit(opts, NodeCMDExitArgs.NodeID)
	}, nil)
}

// function updateMetaData(uint64 nodeID, NodeMetaData memory metaData) external;
func (a NodeActions) updateMetaData(ctx *cli.Context) error {
	nodeManager, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	oldInfo, err := nodeManager.GetInfo(&bind.CallOpts{Context: ctx.Context}, NodeCMDUpdateMetaDataArgs.NodeID)
	if err != nil {
		return err
	}
	metaData := oldInfo.MetaData
	updated := false
	if metaData.Name != NodeCMDUpdateMetaDataArgs.Name && NodeCMDUpdateMetaDataArgs.Name != "" {
		metaData.Name = NodeCMDUpdateMetaDataArgs.Name
		updated = true
	}
	if metaData.Desc != NodeCMDUpdateMetaDataArgs.Desc && NodeCMDUpdateMetaDataArgs.Desc != "" {
		metaData.Desc = NodeCMDUpdateMetaDataArgs.Desc
		updated = true
	}
	if metaData.ImageURL != NodeCMDUpdateMetaDataArgs.ImageURL && NodeCMDUpdateMetaDataArgs.ImageURL != "" {
		metaData.ImageURL = NodeCMDUpdateMetaDataArgs.ImageURL
		updated = true
	}
	if metaData.WebsiteURL != NodeCMDUpdateMetaDataArgs.WebsiteURL && NodeCMDUpdateMetaDataArgs.WebsiteURL != "" {
		metaData.WebsiteURL = NodeCMDUpdateMetaDataArgs.WebsiteURL
		updated = true
	}
	if !updated {
		return errors.New("nothing to update")
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		return nodeManager.UpdateMetaData(opts, NodeCMDUpdateMetaDataArgs.NodeID, metaData.Name, metaData.Desc, metaData.ImageURL, metaData.WebsiteURL)
	}, nil)
}

// function updateOperator(uint64 nodeID, address newOperator) external;
func (a NodeActions) updateOperator(ctx *cli.Context) error {
	nodeManager, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	if !ethcommon.IsHexAddress(NodeCMDUpdateOperatorArgs.Operator) {
		return errors.New("invalid operator address")
	}
	operator := ethcommon.HexToAddress(NodeCMDUpdateOperatorArgs.Operator)
	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		return nodeManager.UpdateOperator(opts, NodeCMDUpdateOperatorArgs.NodeID, operator)
	}, nil)
}

// function getNodeInfo(uint64 nodeID) external view returns (NodeInfo memory info);
func (a NodeActions) nodeInfo(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetInfo(&bind.CallOpts{Context: ctx.Context}, NodeCMDNodeInfoArgs.NodeID)
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

// function getTotalNodeCount() external view returns (uint64);
func (a NodeActions) totalCount(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetTotalCount(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

// function getNodeInfos(uint64[] memory nodeIDs) external view returns (NodeInfo[] memory info);
func (a NodeActions) nodeInfos(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetInfos(&bind.CallOpts{Context: ctx.Context}, NodeCMDNodeInfosArgs.NodeIDs.Value())
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

type NodeInfoWithVotingPower struct {
	node_manager_client.NodeInfo
	VotingPower int64
}

// function getActiveValidatorSet() external view returns (NodeInfo[] memory info, ConsensusVotingPower[] memory votingPowers);
func (a NodeActions) activeValidatorSet(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetActiveValidatorSet(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	infos := make([]NodeInfoWithVotingPower, len(res.Info))
	for i := range res.Info {
		infos[i].NodeInfo = res.Info[i]
		infos[i].VotingPower = res.VotingPowers[i].ConsensusVotingPower
	}

	return common.Pretty(infos)
}

// function getDataSyncerSet() external view returns (NodeInfo[] memory infos);
func (a NodeActions) dataSyncerSet(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetDataSyncerSet(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

// function getCandidateSet() external view returns (NodeInfo[] memory infos);
func (a NodeActions) candidateSet(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetCandidateSet(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

// function getPendingInactiveSet() external view returns (NodeInfo[] memory infos);
func (a NodeActions) pendingInactiveSet(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetPendingInactiveSet(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	return common.Pretty(res)
}

// function getExitedSet() external view returns (NodeInfo[] memory infos);
func (a NodeActions) exitedSet(ctx *cli.Context) error {
	nodeManager, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	res, err := nodeManager.GetExitedSet(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	return common.Pretty(res)
}
