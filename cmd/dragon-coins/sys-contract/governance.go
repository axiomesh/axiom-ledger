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

	"github.com/axiomesh/axiom-ledger/cmd/dragon-coins/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance/solidity/governance_client"
)

var GovernanceCMDProposeArgs = struct {
	ProposalType uint64
	Title        string
	Desc         string
	BlockNumber  uint64
	Extra        string
}{}

var GovernanceCMDVoteArgs = struct {
	ProposalID uint64
	Pass       bool
}{}

var GovernanceCMDProposeCommonArgs = []cli.Flag{
	&cli.StringFlag{
		Name:        "title",
		Destination: &GovernanceCMDProposeArgs.Title,
		Required:    true,
	},
	&cli.StringFlag{
		Name:        "desc",
		Destination: &GovernanceCMDProposeArgs.Desc,
		Required:    true,
	},
	&cli.Uint64Flag{
		Name:        "block-number",
		Destination: &GovernanceCMDProposeArgs.BlockNumber,
		Required:    true,
	},
}

var GovernanceCMD = &cli.Command{
	Name:  "governance",
	Usage: "The governance manage commands",
	Flags: []cli.Flag{
		rpcFlag,
	},
	Subcommands: []*cli.Command{
		{
			Name:   "propose",
			Usage:  "Propose",
			Action: GovernanceActions{}.propose,
			Flags: append(GovernanceCMDProposeCommonArgs, []cli.Flag{
				&cli.Uint64Flag{
					Name:        "proposal-type",
					Destination: &GovernanceCMDProposeArgs.ProposalType,
					Required:    true,
				},
				&cli.StringFlag{
					Name:        "extra",
					Destination: &GovernanceCMDProposeArgs.Extra,
					Required:    true,
				},
				senderFlag,
			}...),
		},
		{
			Name:   "vote",
			Usage:  "Vote",
			Action: GovernanceActions{}.vote,
			Flags: []cli.Flag{
				senderFlag,
				&cli.Uint64Flag{
					Name:        "proposal-id",
					Destination: &GovernanceCMDVoteArgs.ProposalID,
					Required:    true,
				},
				&cli.BoolFlag{
					Name:        "pass",
					Destination: &GovernanceCMDVoteArgs.Pass,
				},
			},
		},
		{
			Name:   "latest-proposal-id",
			Usage:  "Get latest-proposal-id",
			Action: GovernanceActions{}.getLatestProposalID,
		},
		{
			Name:   "proposal",
			Usage:  "Get proposal",
			Action: GovernanceActions{}.proposal,
			Flags: []cli.Flag{
				&cli.Uint64Flag{
					Name:        "proposal-id",
					Destination: &GovernanceCMDVoteArgs.ProposalID,
					Required:    true,
				},
			},
		},
		{
			Name:   "council-members",
			Usage:  "Get council members",
			Action: GovernanceActions{}.getCouncilMembers,
		},
	},
}

type GovernanceActions struct{}

func (_ GovernanceActions) bindContract(ctx *cli.Context) (*governance_client.BindingContract, *ethclient.Client, error) {
	if rpc == "" {
		rpc = "http://127.0.0.1:8881"
	}
	client, err := ethclient.DialContext(ctx.Context, rpc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "dial rpc failed")
	}

	contract, err := governance_client.NewBindingContract(ethcommon.HexToAddress(syscommon.GovernanceContractAddr), client)
	if err != nil {
		return nil, nil, errors.Wrap(err, "bind governance contract failed")
	}
	return contract, client, nil
}

// function propose(ProposalType proposalType, string calldata  title, string calldata desc, uint64 blockNumber, bytes calldata extra) external;
func (a GovernanceActions) propose(ctx *cli.Context) error {
	return a.doPropose(ctx, uint8(GovernanceCMDProposeArgs.ProposalType), func(client *ethclient.Client) ([]byte, error) {
		return []byte(GovernanceCMDProposeArgs.Extra), nil
	})
}

// function vote(uint64 proposalID, VoteResult voteResult) external;
func (a GovernanceActions) vote(ctx *cli.Context) error {
	gov, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		var voteResult uint8
		if !GovernanceCMDVoteArgs.Pass {
			voteResult = 1
		}
		return gov.Vote(opts, GovernanceCMDVoteArgs.ProposalID, voteResult)
	}, func(receipt *types.Receipt) error {
		if len(receipt.Logs) == 0 {
			return errors.New("no log")
		}
		voteEvent, err := gov.ParseVote(*receipt.Logs[len(receipt.Logs)-1])
		if err != nil {
			return errors.Wrap(err, "parse propose failed")
		}
		votedProposal := &governance_client.Proposal{}
		if err := json.Unmarshal(voteEvent.Proposal, votedProposal); err != nil {
			return err
		}
		switch governance.ProposalStatus(votedProposal.Status) {
		case governance.Voting:
			fmt.Printf("proposal status is voting, pass votes: %v, reject votes: %v, total votes: %v\n", len(votedProposal.PassVotes), len(votedProposal.RejectVotes), votedProposal.TotalVotes)
		case governance.Approved:
			fmt.Printf("proposal status is approved, pass votes: %v, reject votes: %v, total votes: %v\n", len(votedProposal.PassVotes), len(votedProposal.RejectVotes), votedProposal.TotalVotes)
			if votedProposal.ExecuteSuccess {
				fmt.Println("proposal execute success")
			} else {
				fmt.Println("proposal execute failed:", votedProposal.ExecuteFailedMsg)
			}
		case governance.Rejected:
			fmt.Printf("proposal status is rejected, pass votes: %v, reject votes: %v, total votes: %v\n", len(votedProposal.PassVotes), len(votedProposal.RejectVotes), votedProposal.TotalVotes)
		}
		return nil
	})
}

func (a GovernanceActions) getLatestProposalID(ctx *cli.Context) error {
	gov, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	latestProposalID, err := gov.GetLatestProposalID(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	fmt.Println("latest proposal id: ", latestProposalID)
	return nil
}

func (a GovernanceActions) getCouncilMembers(ctx *cli.Context) error {
	gov, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	councilMembers, err := gov.GetCouncilMembers(&bind.CallOpts{Context: ctx.Context})
	if err != nil {
		return err
	}
	fmt.Println("council members: ")
	if err := common.Pretty(councilMembers); err != nil {
		return err
	}
	return nil
}

// function proposal(uint64 proposalID) external view returns (Proposal calldata proposal);
func (a GovernanceActions) proposal(ctx *cli.Context) error {
	gov, _, err := a.bindContract(ctx)
	if err != nil {
		return err
	}
	proposalInfo, err := gov.Proposal(&bind.CallOpts{Context: ctx.Context}, GovernanceCMDVoteArgs.ProposalID)
	if err != nil {
		return err
	}
	extra := proposalInfo.Extra
	proposalInfo.Extra = nil
	fmt.Println("proposal info: ")
	if err := common.Pretty(proposalInfo); err != nil {
		return err
	}
	fmt.Println("proposal extra: ")
	fmt.Println(string(extra))
	return nil
}

// function propose(ProposalType proposalType, string calldata  title, string calldata desc, uint64 blockNumber, bytes calldata extra) external;
func (a GovernanceActions) doPropose(ctx *cli.Context, proposalType uint8, extraBuilder func(client *ethclient.Client) ([]byte, error)) error {
	gov, client, err := a.bindContract(ctx)
	if err != nil {
		return err
	}

	return SendAndWaitTx(ctx, client, func(client *ethclient.Client, opts *bind.TransactOpts) (*types.Transaction, error) {
		extra, err := extraBuilder(client)
		if err != nil {
			return nil, err
		}
		return gov.Propose(opts, proposalType, GovernanceCMDProposeArgs.Title, GovernanceCMDProposeArgs.Desc, GovernanceCMDProposeArgs.BlockNumber, extra)
	}, func(receipt *types.Receipt) error {
		if len(receipt.Logs) == 0 {
			return errors.New("no log")
		}
		proposeEvent, err := gov.ParsePropose(*receipt.Logs[len(receipt.Logs)-1])
		if err != nil {
			return errors.Wrap(err, "parse propose failed")
		}
		fmt.Println("proposal id: ", proposeEvent.ProposalID)
		return nil
	})
}
