package adaptor

import (
	"fmt"
	"sync"
	"time"

	"github.com/Rican7/retry"
	"github.com/Rican7/retry/strategy"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/txpool"
	kittypes "github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	synccomm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	"github.com/axiomesh/axiom-ledger/pkg/crypto"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	"github.com/bcds/go-hpc-dagbft/common/config"
	"github.com/bcds/go-hpc-dagbft/common/types"
	"github.com/bcds/go-hpc-dagbft/common/types/events"
	"github.com/bcds/go-hpc-dagbft/common/types/protos"
	"github.com/bcds/go-hpc-dagbft/common/utils/channel"
	"github.com/bcds/go-hpc-dagbft/common/utils/containers"
	"github.com/bcds/go-hpc-dagbft/protocol"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

var _ layer.Ledger = (*BlockChain)(nil)

type LedgerConfig struct {
	ChainState *chainstate.ChainState

	GetBlockHeaderFunc func(height uint64) (*kittypes.BlockHeader, error)
}

type BlockChain struct {
	epochStore kv.Storage
	sync       synccomm.Sync
	crypto     layer.Crypto
	stores     layer.StorageFactory
	txPool     txpool.TxPool[kittypes.Transaction, *kittypes.Transaction]

	ledgerConfig      *LedgerConfig
	narwhalConfig     config.Configs
	logger            logrus.FieldLogger
	closeCh           chan bool
	decisionCh        chan containers.Pair[*types.QuorumCheckpoint, chan<- *events.StableCommittedEvent]
	updatingCh        chan containers.Tuple[*types.QuorumCheckpoint, []*types.QuorumCheckpoint, chan<- *events.StateUpdatedEvent]
	executingCh       chan containers.Tuple[types.Height, *types.ConsensusOutput, chan<- *events.ExecutedEvent]
	waitEpochChangeCh chan struct{}

	sendReadyC          chan *Ready
	sendToExecuteCh     chan *common.CommitEvent
	notifyStateUpdateCh chan<- containers.Tuple[types.Height, *types.QuorumCheckpoint, chan<- *events.StateUpdatedEvent]
}

func NewBlockchain(config *common.Config, readyC chan *Ready, epochStore kv.Storage, closeCh chan bool) (*BlockChain, error) {
	var (
		err   error
		store layer.Storage
	)
	storeBuilder := func(e types.Epoch, name string) layer.Storage {
		storePath := repo.GetStoragePath(config.Repo.RepoRoot, storagemgr.Consensus, name)

		switch config.Repo.Config.Consensus.StorageType {
		case repo.ConsensusStorageTypeMinifile:
			store, err = common.OpenMinifile(storePath)
		case repo.ConsensusStorageTypeRosedb:
			store, err = common.OpenRosedb(storePath)
		case repo.ConsensusStorageTypePebble:
			store, err = common.OpenPebbleDb(storePath)
		}
		if err != nil {
			panic(err)
		}
		return store
	}
	stores := NewFactory(config.ChainState.GetCurrentEpochInfo().Epoch, storeBuilder)

	var priv crypto.PrivateKey
	if config.Repo.Config.Consensus.UseBlsKey {
		priv = config.Repo.ConsensusKeystore.PrivateKey
	} else {
		priv = config.Repo.P2PKeystore.PrivateKey
	}

	crp, err := NewCrypto(priv, config)
	if err != nil {
		return nil, err
	}

	return &BlockChain{
		stores:     stores,
		crypto:     crp,
		epochStore: epochStore,
		txPool:     config.TxPool,
		sync:       config.BlockSync,
		logger:     config.Logger,
		// init all channel
		closeCh:             closeCh,
		sendReadyC:          readyC,
		decisionCh:          make(chan containers.Pair[*types.QuorumCheckpoint, chan<- *events.StableCommittedEvent]),
		updatingCh:          make(chan containers.Tuple[*types.QuorumCheckpoint, []*types.QuorumCheckpoint, chan<- *events.StateUpdatedEvent]),
		executingCh:         make(chan containers.Tuple[types.Height, *types.ConsensusOutput, chan<- *events.ExecutedEvent]),
		sendToExecuteCh:     make(chan *common.CommitEvent),
		notifyStateUpdateCh: make(chan containers.Tuple[types.Height, *types.QuorumCheckpoint, chan<- *events.StateUpdatedEvent]),
	}, nil
}

func (b *BlockChain) listenChainEvent() {
	for {
		select {
		case <-b.closeCh:
			return
		case execution := <-b.executingCh:
			stateHeight, output, evCh := execution.Unpack()
			b.asyncExecute(output, stateHeight, evCh)
		case update := <-b.updatingCh:
			target, changes, evCh := update.Unpack()
			if err := b.update(target, changes); err != nil {
				panic(fmt.Errorf("failed to update: %w", err))
			}
			stateResult := containers.Pack3(target.Height(), target, evCh)
			channel.SafeSend(b.notifyStateUpdateCh, stateResult, b.closeCh)

		case decision := <-b.decisionCh:
			checkpoint, evCh := decision.Unpack()

			b.checkpoint(checkpoint)

			stableCommitted := &events.StableCommittedEvent{
				Checkpoint: checkpoint,
			}
			channel.SafeSend(evCh, stableCommitted, b.closeCh)
		}
	}
}

func (b *BlockChain) GetLedgerState(height *types.Height) (*types.ExecuteState, error) {
	if height != nil {
		header, err := b.ledgerConfig.GetBlockHeaderFunc(*height)
		if err != nil {
			return nil, err
		}

		return &types.ExecuteState{
			ExecuteState: protos.ExecuteState{
				Height:    *height,
				StateRoot: header.Hash().String(),
			},
			Reconfigured: common.NeedChangeEpoch(*height, b.ledgerConfig.ChainState.GetCurrentEpochInfo()),
		}, nil
	}
	return nil, fmt.Errorf("height is nil")
}

// todo: add workers in Chain state, support dynamic adjustment of worker parameters.
func (b *BlockChain) GetLedgerValidators() (protocol.Validators, error) {
	var validators []*protos.Validator
	validators = lo.Map(b.ledgerConfig.ChainState.ValidatorSet, func(item chainstate.ValidatorInfo, index int) *protos.Validator {
		nodeInfo, err := b.ledgerConfig.ChainState.GetNodeInfo(item.ID)
		if err != nil {
			b.logger.Warningf("failed to get node info: %v", err)
			return nil
		}
		pubBytes, err := nodeInfo.ConsensusPubKey.Marshal()
		if err != nil {
			b.logger.Warningf("failed to marshal pub key: %v", err)
			return nil
		}
		return &protos.Validator{
			Hostname:    nodeInfo.Primary,
			PubKey:      pubBytes,
			ValidatorId: uint32(item.ID),
			VotePower:   uint64(item.ConsensusVotingPower),
			Workers:     nodeInfo.Workers,
		}
	})
	return validators, nil
}

// todo(lrx): modify this function when the ledger version is upgraded
func (b *BlockChain) GetLedgerAlgoVersion() (string, error) {
	return "DagBFT@1.0", nil
}

func (b *BlockChain) Execute(output *types.ConsensusOutput, height types.Height, eventCh chan<- *events.ExecutedEvent) error {
	b.logger.Infof("Execute Output %b at height %b, batches %b, txs: %b",
		output.CommitInfo.CommitSequence, height, output.BatchCount(), output.TransactionCount())

	channel.SafeSend(b.executingCh, containers.Pack3(height, output, eventCh), b.closeCh)
	if common.NeedChangeEpoch(height, b.ledgerConfig.ChainState.GetCurrentEpochInfo()) {
		select {
		case <-b.waitEpochChangeCh:
			return nil
		case <-b.closeCh:
			return nil
		//todo: configurable timeout
		case <-time.After(60 * time.Second):
			return fmt.Errorf("wait epoch change timeout")
		}
	}

	return nil
}

func (b *BlockChain) asyncExecute(output *types.ConsensusOutput, height types.Height, executedCh chan<- *events.ExecutedEvent) {
	validTxs := b.filterValidTxs(output.Batches)
	b.logger.Debugf("valid txs in execute: %b", len(validTxs))
	ready := &Ready{
		Txs:            validTxs,
		Height:         height,
		Timestamp:      output.Timestamp(),
		ProposerNodeID: uint64(output.CommitInfo.Leader.GetHeader().GetAuthorId()),
		ExecutedCh:     executedCh,
		CommitState: &types.CommitState{
			CommitState: protos.CommitState{
				Sequence:     output.CommitSequence(),
				CommitDigest: output.CommitInfo.Digest().String(),
			},
		},
	}
	if common.NeedChangeEpoch(height, b.ledgerConfig.ChainState.GetCurrentEpochInfo()) {
		ready.EpochChangedCh = b.waitEpochChangeCh
	}
	channel.SafeSend(b.sendReadyC, ready, b.closeCh)
}

func (b *BlockChain) filterValidTxs(batches [][]*types.Batch) []*kittypes.Transaction {
	validTxs := make([]*kittypes.Transaction, 0)
	seenHashes := make(map[string]struct{})
	flattenBatches := lo.Flatten(batches)
	lo.ForEach(flattenBatches, func(batch *types.Batch, index int) {
		lo.ForEach(batch.Transactions, func(data []byte, index int) {
			tx := &kittypes.Transaction{}
			if err := tx.Unmarshal(data); err != nil {
				b.logger.Errorf("failed to unmarshal tx: %v", err)
				return
			}

			if _, ok := seenHashes[tx.GetHash().String()]; ok {
				b.logger.Debugf("duplicated tx in different batches: %s", tx.GetHash().String())
				return
			}
			seenHashes[tx.GetHash().String()] = struct{}{}
			validTxs = append(validTxs, tx)
		})
	})
	return validTxs
}

func (b *BlockChain) Checkpoint(checkpoint *types.QuorumCheckpoint, eventCh chan<- *events.StableCommittedEvent) error {
	b.logger.Infof("Checkpoint at %b,%b", checkpoint.Checkpoint().CommitState().CommitSequence(), checkpoint.Checkpoint().ExecuteState().StateHeight())

	channel.SafeSend(b.decisionCh, containers.Pack2(checkpoint, eventCh), b.closeCh)
	return nil
}

func (b *BlockChain) checkpoint(checkpoint *types.QuorumCheckpoint) {
	// Update active validators and crypto validatorVerifier to the new epoch
	if checkpoint.Checkpoint().EndsEpoch() {
		//todo: update validators and crypto validatorVerifier
		epochChange, err := checkpoint.Marshal()
		if err != nil {
			b.logger.Errorf("failed to marshal checkpoint: %v", err)
			return
		}
		if err = common.PersistEpochChange(b.epochStore, checkpoint.Epoch(), epochChange); err != nil {
			b.logger.Errorf("failed to persist epoch change: %v", err)
			return
		}
	}
	b.ledgerConfig.ChainState.UpdateCheckpoint(&pb.QuorumCheckpoint{
		Epoch: checkpoint.Epoch(),
		State: &pb.ExecuteState{
			Height: checkpoint.Checkpoint().ExecuteState().StateHeight(),
			Digest: checkpoint.Checkpoint().ExecuteState().StateRoot().String(),
		},
	})
}

func (b *BlockChain) StateUpdate(checkpoint *types.QuorumCheckpoint, eventCh chan<- *events.StateUpdatedEvent, epochChanges ...*types.QuorumCheckpoint) error {
	b.logger.Infof("Update State to sequence: %b, height: %b", checkpoint.Checkpoint().CommitState().CommitSequence(), checkpoint.Checkpoint().ExecuteState().StateHeight())

	channel.SafeSend(b.updatingCh, containers.Pack3(checkpoint, epochChanges, eventCh), b.closeCh)
	return nil
}

func (b *BlockChain) update(checkpoint *types.QuorumCheckpoint, epochChanges []*types.QuorumCheckpoint) error {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		lo.ForEach(epochChanges, func(epoch *types.QuorumCheckpoint, index int) {
			if epoch != nil {
				data, err := epoch.Marshal()
				if err != nil {
					b.logger.Errorf("marshal epoch change failed: %v", err)
					return
				}
				if err = common.PersistEpochChange(b.epochStore, epoch.Epoch(), data); err != nil {
					b.logger.Errorf("persist epoch change failed: %v", err)
					return
				}
			}
		})
	}()

	localHeight := b.ledgerConfig.ChainState.GetCurrentCheckpointState().Height
	latestBlockHash := b.ledgerConfig.ChainState.GetCurrentCheckpointState().Digest
	targetHeight := checkpoint.Checkpoint().ExecuteState().StateHeight()
	peerM := b.getRemotePeers(epochChanges)

	syncTaskDoneCh := make(chan error, 1)
	if err := retry.Retry(func(attempt uint) error {
		params := &synccomm.SyncParams{
			Peers:           peerM,
			LatestBlockHash: latestBlockHash,
			// ensure sync remote count including at least one correct node
			Quorum:       common.CalFaulty(uint64(len(peerM) + 1)),
			CurHeight:    localHeight + 1,
			TargetHeight: targetHeight,
			QuorumCheckpoint: &pb.QuorumCheckpoint{
				Epoch: checkpoint.Epoch(),
				State: &pb.ExecuteState{
					Height: checkpoint.Checkpoint().ExecuteState().StateHeight(),
					Digest: checkpoint.Checkpoint().ExecuteState().StateRoot().String(),
				},
			},
			EpochChanges: lo.Map(epochChanges, func(epoch *types.QuorumCheckpoint, index int) *pb.EpochChange {
				return &pb.EpochChange{
					QuorumCheckpoint: &pb.QuorumCheckpoint{
						Epoch: epoch.Epoch(),
						State: &pb.ExecuteState{
							Height: epoch.Checkpoint().ExecuteState().StateHeight(),
							Digest: epoch.Checkpoint().ExecuteState().StateRoot().String(),
						},
					},
				}
			}),
		}
		err := b.sync.StartSync(params, syncTaskDoneCh)
		if err != nil {
			b.logger.Infof("start sync failed[local:%b, target:%b]: %s", localHeight, targetHeight, err)
			return err
		}
		return nil
	}, strategy.Limit(5), strategy.Wait(500*time.Microsecond)); err != nil {
		panic(fmt.Errorf("retry start sync failed: %v", err))
	}

	wg.Wait()
	var stateUpdatedCheckpoint *common.Checkpoint
	// wait for the sync to finish
	for {
		select {
		case <-b.closeCh:
			b.logger.Info("state update is canceled!!!!!!")
			return nil
		case syncErr := <-syncTaskDoneCh:
			if syncErr != nil {
				return syncErr
			}
		case data := <-b.sync.Commit():
			endSync := false
			blockCache, ok := data.([]synccomm.CommitData)
			if !ok {
				panic("state update failed: invalid commit data")
			}

			b.logger.Infof("fetch chunk: start: %b, end: %b", blockCache[0].GetHeight(), blockCache[len(blockCache)-1].GetHeight())

			for _, commitData := range blockCache {
				// if the block is the target block, we should resign the stateUpdatedCheckpoint in CommitEvent
				// and send the quitSync signal to sync module
				if commitData.GetHeight() == targetHeight {
					stateUpdatedCheckpoint = &common.Checkpoint{
						Epoch:  checkpoint.Epoch(),
						Height: checkpoint.Height(),
						Digest: checkpoint.Checkpoint().ExecuteState().StateRoot().String(),
					}
					endSync = true
				}
				block, ok := commitData.(*synccomm.BlockData)
				if !ok {
					panic("state update failed: invalid commit data")
				}
				commitEvent := &common.CommitEvent{
					Block:                  block.Block,
					StateUpdatedCheckpoint: stateUpdatedCheckpoint,
				}
				channel.SafeSend(b.sendToExecuteCh, commitEvent, b.closeCh)

				if endSync {
					b.logger.Infof("State update finished, target height: %b", targetHeight)
					return nil
				}
			}
		}
	}
}

func (b *BlockChain) getRemotePeers(epochChanges []*types.QuorumCheckpoint) []*synccomm.Node {
	peersM := make(map[uint64]*synccomm.Node)

	// get the validator set of the current local epoch
	for _, validatorInfo := range b.ledgerConfig.ChainState.ValidatorSet {
		v, err := b.ledgerConfig.ChainState.GetNodeInfo(validatorInfo.ID)
		if err == nil {
			if v.NodeInfo.P2PID != b.ledgerConfig.ChainState.SelfNodeInfo.P2PID {
				peersM[validatorInfo.ID] = &synccomm.Node{Id: validatorInfo.ID, PeerID: v.P2PID}
			}
		}
	}

	// get the validator set of the remote latest epoch
	if len(epochChanges) != 0 {
		lo.ForEach(epochChanges[len(epochChanges)-1].Validators(), func(v *protos.Validator, _ int) {
			if _, ok := peersM[uint64(v.ValidatorId)]; !ok && uint64(v.GetValidatorId()) != b.ledgerConfig.ChainState.SelfNodeInfo.ID {
				info, err := b.ledgerConfig.ChainState.GetNodeInfo(uint64(v.ValidatorId))
				if err != nil {
					return
				}
				peersM[uint64(v.ValidatorId)] = &synccomm.Node{Id: uint64(v.ValidatorId), PeerID: info.P2PID}
			}
		})
	}
	// flatten peersM
	return lo.Values(peersM)
}

func (b *BlockChain) GetEpoch() types.Epoch {
	return b.ledgerConfig.ChainState.GetCurrentEpochInfo().Epoch
}

func (b *BlockChain) GetEpochCrypto() layer.Crypto {
	return b.crypto
}

func (b *BlockChain) GetEpochConfig(epoch types.Epoch) config.Configs {
	return b.narwhalConfig
}

func (b *BlockChain) GetEpochStorage(epoch types.Epoch) layer.StorageFactory {
	return b.stores
}

func (b *BlockChain) GetEpochCheckpoint(epoch *types.Epoch) *types.QuorumCheckpoint {
	raw, err := common.GetEpochChange(b.epochStore, *epoch)
	if err != nil {
		b.logger.Errorf("failed to read epoch %d quorum chkpt: %v", epoch, err)
		return nil
	}
	cp := &types.QuorumCheckpoint{}
	if err = cp.Unmarshal(raw); err != nil {
		b.logger.Errorf("failed to unmarshal epoch %d quorum chkpt: %v", epoch, err)
		return nil
	}
	return cp
}
