package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	rbft "github.com/axiomesh/axiom-bft"
	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/types"
	consensuscommon "github.com/axiomesh/axiom-ledger/internal/consensus/common"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
)

type snapMeta struct {
	snapBlockHeader  *types.BlockHeader
	snapPersistEpoch uint64
	snapPeers        []*common.Node
}

func loadSnapMeta(lg *ledger.Ledger, args *repo.SyncArgs) (*snapMeta, error) {
	meta, err := lg.StateLedger.GetTrieSnapshotMeta()
	if err != nil {
		return nil, fmt.Errorf("get snapshot meta hash: %w", err)
	}

	snapPersistedEpoch := meta.EpochInfo.Epoch - 1
	if meta.EpochInfo.EpochPeriod+meta.EpochInfo.StartBlock-1 == meta.BlockHeader.Number {
		snapPersistedEpoch = meta.EpochInfo.Epoch
	}

	// if local node is started with specified nodes for synchronization,
	// the specified nodes will be used instead of the snapshot meta
	rawPeers := meta.Nodes
	if args.RemotePeers != nil && len(args.RemotePeers.Validators) != 0 {
		rawPeers = args.RemotePeers
	}
	// flatten peers
	peers := lo.FlatMap(rawPeers.Validators, func(p *consensus.QuorumValidator, _ int) []*common.Node {
		return []*common.Node{
			{
				Id:     p.Id,
				PeerID: p.PeerId,
			},
		}
	})

	return &snapMeta{
		snapBlockHeader:  meta.BlockHeader,
		snapPeers:        peers,
		snapPersistEpoch: snapPersistedEpoch,
	}, nil
}

func (axm *AxiomLedger) prepareSnapSync(latestHeight uint64) (*common.PrepareData, *pb.QuorumCheckpoint, error) {
	// 1. switch to snapshot mode
	err := axm.Sync.SwitchMode(common.SyncModeSnapshot)
	if err != nil {
		return nil, nil, fmt.Errorf("switch mode err: %w", err)
	}

	var startEpcNum uint64 = 1
	if latestHeight != 0 {
		blockHeader, err := axm.ViewLedger.ChainLedger.GetBlockHeader(latestHeight)
		if err != nil {
			return nil, nil, fmt.Errorf("get latest blockHeader err: %w", err)
		}
		blockEpc := blockHeader.Epoch
		epochManagerContract := framework.EpochManagerBuildConfig.Build(syscommon.NewViewVMContext(axm.ViewLedger.NewView().StateLedger))
		info, err := epochManagerContract.HistoryEpoch(blockEpc)
		if err != nil {
			return nil, nil, fmt.Errorf("get epoch info err: %w", err)
		}
		if info.StartBlock+info.EpochPeriod-1 == latestHeight {
			// if the last blockHeader in this epoch had been persisted, start from the next epoch
			startEpcNum = info.Epoch + 1
		} else {
			startEpcNum = info.Epoch
		}
	}

	// 2. fill snap sync config option
	opts := []common.Option{
		common.WithPeers(axm.snapMeta.snapPeers),
		common.WithStartEpochChangeNum(startEpcNum),
		common.WithLatestPersistEpoch(rbft.GetLatestEpochQuorumCheckpoint(axm.epochStore.Get)),
		common.WithSnapCurrentEpoch(axm.snapMeta.snapPersistEpoch),
	}

	// 3. prepare snap sync info
	res, err := axm.Sync.Prepare(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare sync err: %w", err)
	}

	snapCheckpoint := &pb.QuorumCheckpoint{
		Epoch: axm.snapMeta.snapPersistEpoch,
		State: &pb.ExecuteState{
			Height: axm.snapMeta.snapBlockHeader.Number,
			Digest: axm.snapMeta.snapBlockHeader.Hash().String(),
		},
	}

	return res, snapCheckpoint, nil
}

func (axm *AxiomLedger) startSnapSync(verifySnapCh chan bool, ckpt *pb.QuorumCheckpoint, peers []*common.Node, startHeight uint64, epochChanges []*pb.EpochChange) error {
	syncTaskDoneCh := make(chan error, 1)
	targetHeight := ckpt.State.Height
	params := axm.genSnapSyncParams(peers, startHeight, targetHeight, ckpt, epochChanges)
	start := time.Now()
	if err := axm.Sync.StartSync(params, syncTaskDoneCh); err != nil {
		return err
	}

	for {
		select {
		case <-axm.Ctx.Done():
			return nil
		case err := <-syncTaskDoneCh:
			if err != nil {
				return err
			}
		case data := <-axm.Sync.Commit():
			now := time.Now()
			snapData, ok := data.(*common.SnapCommitData)
			if !ok {
				return fmt.Errorf("invalid commit data type: %T", data)
			}

			err := axm.persistChainData(snapData)
			if err != nil {
				return err
			}
			currentHeight := snapData.Data[len(snapData.Data)-1].GetHeight()
			axm.logger.WithFields(logrus.Fields{
				"Height": currentHeight,
				"target": targetHeight,
				"cost":   time.Since(now),
			}).Info("persist chain data task")

			if currentHeight == targetHeight {
				axm.logger.WithFields(logrus.Fields{
					"targetHeight": targetHeight,
					"cost":         time.Since(start),
				}).Info("snap sync task done")

				if !axm.waitVerifySnapTrie(verifySnapCh) {
					return errors.New("verify snap trie failed")
				}
				return nil
			}
		}
	}
}

func (axm *AxiomLedger) waitVerifySnapTrie(verifySnapCh chan bool) bool {
	return <-verifySnapCh
}

func (axm *AxiomLedger) persistChainData(data *common.SnapCommitData) error {
	var batchBlock []*types.Block
	var batchReceipts [][]*types.Receipt
	for _, commitData := range data.Data {
		chainData := commitData.(*common.ChainData)
		batchBlock = append(batchBlock, chainData.Block)
		batchReceipts = append(batchReceipts, chainData.Receipts)
	}
	if len(batchBlock) > 0 {
		if err := axm.ViewLedger.ChainLedger.BatchPersistExecutionResult(batchBlock, batchReceipts); err != nil {
			return err
		}
	}

	if data.EpochState != nil {
		if err := consensuscommon.PersistEpochChange(axm.epochStore, data.EpochState); err != nil {
			return err
		}
	}
	return nil
}

func (axm *AxiomLedger) genSnapSyncParams(peers []*common.Node, startHeight, targetHeight uint64,
	quorumCkpt *pb.QuorumCheckpoint, epochChanges []*pb.EpochChange) *common.SyncParams {
	latestBlockHash := ethcommon.Hash{}.String()
	if axm.ViewLedger.ChainLedger.GetChainMeta().BlockHash != nil {
		latestBlockHash = axm.ViewLedger.ChainLedger.GetChainMeta().BlockHash.String()
	} else {
		startHeight--
	}
	return &common.SyncParams{
		Peers:            peers,
		LatestBlockHash:  latestBlockHash,
		Quorum:           consensuscommon.GetQuorum(axm.Repo.Config.Consensus.Type, len(peers)),
		CurHeight:        startHeight,
		TargetHeight:     targetHeight,
		QuorumCheckpoint: quorumCkpt,
		EpochChanges:     epochChanges,
	}
}
