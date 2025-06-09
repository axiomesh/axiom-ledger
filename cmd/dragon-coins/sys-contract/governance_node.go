package sys_contract

//
//import (
//	"encoding/json"
//
//	"github.com/ethereum/go-ethereum/ethclient"
//	"github.com/urfave/cli/v2"
//
//	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance"
//)
//
//var GovernanceNodeCMDProposeNodeRemoveArgs = struct {
//	NodeIDs cli.Uint64Slice
//}{
//	NodeIDs: cli.Uint64Slice{},
//}
//
//var GovernanceNodeCMD = &cli.Command{
//	Name:  "governance-node",
//	Usage: "The governance node manage commands",
//	Flags: []cli.Flag{
//		rpcFlag,
//	},
//	Subcommands: []*cli.Command{
//		{
//			Name:   "propose-node-remove",
//			Usage:  "Propose node remove",
//			Action: GovernanceActions{}.proposeNodeRemove,
//			Flags: append(GovernanceCMDProposeCommonArgs, []cli.Flag{
//				&cli.Uint64SliceFlag{
//					Name:        "node-ids",
//					Destination: &GovernanceNodeCMDProposeNodeRemoveArgs.NodeIDs,
//					Required:    true,
//				},
//				senderFlag,
//			}...),
//		},
//	},
//}
//
//func (a GovernanceActions) proposeNodeRemove(ctx *cli.Context) error {
//	return a.doPropose(ctx, uint8(governance.NodeRemove), func(client *ethclient.Client) ([]byte, error) {
//		return json.Marshal(governance.NodeRemoveExtraArgs{
//			NodeIDs: GovernanceNodeCMDProposeNodeRemoveArgs.NodeIDs.Value(),
//		})
//	})
//}
