package app

import (
	"context"
	"fmt"
	"math/big"
	"syscall"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	consensuspb "github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/storage/kv"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/chainstate"
	"github.com/axiomesh/axiom-ledger/internal/consensus"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/executor"
	devexecutor "github.com/axiomesh/axiom-ledger/internal/executor/dev"
	syscommon "github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/framework/solidity/node_manager"
	"github.com/axiomesh/axiom-ledger/internal/genesis"
	"github.com/axiomesh/axiom-ledger/internal/indexer"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/internal/sync"
	synccomm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	txpool2 "github.com/axiomesh/axiom-ledger/internal/txpool"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type AxiomLedger struct {
	Ctx           context.Context
	Cancel        context.CancelFunc
	logger        logrus.FieldLogger
	ChainState    *chainstate.ChainState
	Repo          *repo.Repo
	ViewLedger    *ledger.Ledger
	BlockExecutor executor.Executor
	Consensus     consensus.Consensus
	TxPool        txpool.TxPool[types.Transaction, *types.Transaction]
	Network       network.Network
	Sync          synccomm.Sync
	BloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	Indexer       *indexer.ChainIndexer

	epochStore kv.Storage
	snapMeta   *snapMeta
	StopCh     chan error
}

func NewAxiomLedger(rep *repo.Repo, ctx context.Context, cancel context.CancelFunc) (*AxiomLedger, error) {
	axm, err := NewAxiomLedgerWithoutConsensus(rep, ctx, cancel)
	if err != nil {
		return nil, fmt.Errorf("generate axiom-ledger without consensus failed: %w", err)
	}

	chainMeta := axm.ViewLedger.ChainLedger.GetChainMeta()

	if !rep.StartArgs.ReadonlyMode {
		// new txpool
		poolConf := rep.ConsensusConfig.TxPool
		getNonceFn := func(address *types.Address) uint64 {
			return axm.ViewLedger.NewView().StateLedger.GetNonce(address)
		}
		fn := func(addr string) uint64 {
			return getNonceFn(types.NewAddressByStr(addr))
		}
		getBalanceFn := func(addr string) *big.Int {
			return axm.ViewLedger.NewView().StateLedger.GetBalance(types.NewAddressByStr(addr))
		}

		priceLimit := poolConf.PriceLimit
		// ensure price limit is not less than min gas price
		if axm.ChainState.EpochInfo.FinanceParams.MinGasPrice.ToBigInt().Cmp(priceLimit.ToBigInt()) > 0 {
			priceLimit = axm.ChainState.EpochInfo.FinanceParams.MinGasPrice
		}

		txpoolConf := txpool2.Config{
			Logger:                 loggers.Logger(loggers.TxPool),
			PoolSize:               poolConf.PoolSize,
			ToleranceTime:          poolConf.ToleranceTime.ToDuration(),
			ToleranceRemoveTime:    poolConf.ToleranceRemoveTime.ToDuration(),
			ToleranceNonceGap:      poolConf.ToleranceNonceGap,
			CleanEmptyAccountTime:  poolConf.CleanEmptyAccountTime.ToDuration(),
			GetAccountNonce:        fn,
			GetAccountBalance:      getBalanceFn,
			EnableLocalsPersist:    poolConf.EnableLocalsPersist,
			RepoRoot:               rep.RepoRoot,
			RotateTxLocalsInterval: poolConf.RotateTxLocalsInterval.ToDuration(),
			PriceLimit:             priceLimit.ToBigInt().Uint64(),
			PriceBump:              poolConf.PriceBump,
			GenerateBatchType:      poolConf.GenerateBatchType,
		}
		axm.TxPool, err = txpool2.NewTxPool[types.Transaction, *types.Transaction](txpoolConf, axm.ChainState)
		if err != nil {
			return nil, fmt.Errorf("new txpool failed: %w", err)
		}

		genesisBlockHeader, err := axm.ViewLedger.ChainLedger.GetBlockHeader(axm.Repo.GenesisConfig.EpochInfo.StartBlock)
		if err != nil {
			return nil, fmt.Errorf("get genesis block header failed: %w", err)
		}
		// new consensus
		axm.Consensus, err = consensus.New(
			rep.Config.Consensus.Type,
			common.WithTxPool(axm.TxPool),
			common.WithRepo(rep),
			common.WithGenesisEpochInfo(rep.GenesisConfig.EpochInfo.Clone()),
			common.WithChainState(axm.ChainState),
			common.WithNetwork(axm.Network),
			common.WithLogger(loggers.Logger(loggers.Consensus)),
			common.WithApplied(chainMeta.Height),
			common.WithDigest(chainMeta.BlockHash.String()),
			common.WithGenesisDigest(genesisBlockHeader.Hash().String()),
			common.WithGetBlockHeaderFunc(axm.ViewLedger.ChainLedger.GetBlockHeader),
			common.WithGetAccountBalanceFunc(func(address string) *big.Int {
				return axm.ViewLedger.NewView().StateLedger.GetBalance(types.NewAddressByStr(address))
			}),
			common.WithGetAccountNonceFunc(func(address *types.Address) uint64 {
				return axm.ViewLedger.NewView().StateLedger.GetNonce(address)
			}),
			common.WithBlockSync(axm.Sync),
			common.WithEpochStore(axm.epochStore),
			common.WithNotifyStopCh(func(err error) {
				axm.StopCh <- err
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("initialize consensus failed: %w", err)
		}
	}

	return axm, nil
}

func PrepareAxiomLedger(rep *repo.Repo) error {
	types.InitEIP155Signer(big.NewInt(int64(rep.GenesisConfig.ChainID)))

	if err := storagemgr.Initialize(rep.Config); err != nil {
		return fmt.Errorf("storagemgr initialize: %w", err)
	}
	if err := raiseUlimit(rep.Config.Ulimit); err != nil {
		return fmt.Errorf("raise ulimit: %w", err)
	}
	return nil
}

func NewAxiomLedgerWithoutConsensus(rep *repo.Repo, ctx context.Context, cancel context.CancelFunc) (*AxiomLedger, error) {
	var (
		rwLdg *ledger.Ledger
		err   error
	)
	if err = PrepareAxiomLedger(rep); err != nil {
		return nil, err
	}

	logger := loggers.Logger(loggers.App)

	// 0. load ledger
	var snap *snapMeta
	if rep.StartArgs.SnapshotMode {
		stateLg, err := storagemgr.OpenWithMetrics(storagemgr.GetLedgerComponentPath(rep, storagemgr.Ledger), storagemgr.Ledger)
		if err != nil {
			return nil, err
		}
		snapshotStorage, err := storagemgr.OpenWithMetrics(storagemgr.GetLedgerComponentPath(rep, storagemgr.Snapshot), storagemgr.Snapshot)
		if err != nil {
			return nil, err
		}
		rwLdg, err = ledger.NewLedgerWithStores(rep, nil, stateLg, snapshotStorage, nil)
		if err != nil {
			return nil, err
		}
		snap, err = loadSnapMeta(rwLdg, rep.SyncArgs)
		if err != nil {
			return nil, err
		}
		if snap.snapBlockHeader.Number == 0 {
			return nil, errors.New("cannot start snap mode at block 0")
		}
		// verify whether trie snapshot is legal
		verified, err := rwLdg.StateLedger.VerifyTrie(snap.snapBlockHeader)
		if err != nil {
			return nil, err
		}
		if !verified {
			return nil, errors.New("verify snapshot trie failed")
		}

		rwLdg.SnapMeta.Store(ledger.SnapInfo{Status: true, SnapBlockHeader: snap.snapBlockHeader.Clone()})
	} else {
		rwLdg, err = ledger.NewLedger(rep)
		if err != nil {
			return nil, err
		}
		// init genesis config
		if !genesis.IsInitialized(rwLdg) {
			if err := genesis.Initialize(rep.GenesisConfig, rwLdg); err != nil {
				return nil, errors.Wrapf(err, "failed to initialize genesis")
			}
			logger.WithFields(logrus.Fields{
				"genesis block hash": rwLdg.ChainLedger.GetChainMeta().BlockHash,
			}).Info("	Initialize genesis")
		}
	}

	vl := rwLdg.NewView()
	var net network.Network
	if !rep.StartArgs.ReadonlyMode {
		net, err = network.New(rep, loggers.Logger(loggers.P2P))
		if err != nil {
			return nil, fmt.Errorf("create p2p failed: %w", err)
		}
	}

	var syncMgr *sync.SyncManager
	var epochStore kv.Storage
	if !rep.StartArgs.ReadonlyMode {
		epochStore, err = storagemgr.OpenWithMetrics(storagemgr.GetLedgerComponentPath(rep, storagemgr.Epoch), storagemgr.Epoch)
		if err != nil {
			return nil, err
		}
		syncMgr, err = sync.NewSyncManager(loggers.Logger(loggers.BlockSync), vl.ChainLedger.GetChainMeta, vl.ChainLedger.GetBlock, vl.ChainLedger.GetBlockHeader,
			vl.ChainLedger.GetBlockReceipts, epochStore.Get, net, rep.Config.Sync)
		if err != nil {
			return nil, fmt.Errorf("create block sync: %w", err)
		}
	}

	chainState := chainstate.NewChainState(rep.P2PKeystore.P2PID(), rep.P2PKeystore.PublicKey, rep.ConsensusKeystore.PublicKey, func(nodeID uint64) (*node_manager.NodeInfo, error) {
		lg := vl.NewView()
		nodeManagerContract := framework.NodeManagerBuildConfig.Build(syscommon.NewViewVMContext(lg.StateLedger))
		nodeInfo, err := nodeManagerContract.GetInfo(nodeID)
		if err != nil {
			return nil, err
		}
		return &nodeInfo, nil
	}, func(p2pID string) (uint64, error) {
		lg := vl.NewView()
		nodeManagerContract := framework.NodeManagerBuildConfig.Build(syscommon.NewViewVMContext(lg.StateLedger))
		return nodeManagerContract.GetNodeIDByP2PID(p2pID)
	}, func(epoch uint64) (*types.EpochInfo, error) {
		lg := vl.NewView()
		epochManagerContract := framework.EpochManagerBuildConfig.Build(syscommon.NewViewVMContext(lg.StateLedger))
		epochInfo, err := epochManagerContract.HistoryEpoch(epoch)
		if err != nil {
			return nil, err
		}
		return epochInfo.ToTypesEpoch(), nil
	})

	axm := &AxiomLedger{
		Ctx:        ctx,
		Cancel:     cancel,
		ChainState: chainState,
		Repo:       rep,
		logger:     logger,
		ViewLedger: vl,
		Network:    net,
		Sync:       syncMgr,

		snapMeta:   snap,
		epochStore: epochStore,
		StopCh:     make(chan error, 1),
	}

	// start p2p network
	if repo.SupportMultiNode[axm.Repo.Config.Consensus.Type] && !axm.Repo.StartArgs.ReadonlyMode {
		if err = axm.Network.Start(); err != nil {
			return nil, fmt.Errorf("peer manager start: %w", err)
		}

		latestHeight := axm.ViewLedger.ChainLedger.GetChainMeta().Height
		// if we reached snap block, needn't sync
		// prepare sync
		if axm.Repo.StartArgs.SnapshotMode {
			if latestHeight > axm.snapMeta.snapBlockHeader.Number {
				return nil, fmt.Errorf("local latest block height %d is bigger than snap block height %d", latestHeight, axm.snapMeta.snapBlockHeader.Number)
			}
			if latestHeight < axm.snapMeta.snapBlockHeader.Number {
				start := time.Now()
				axm.logger.WithFields(logrus.Fields{
					"start height":  latestHeight,
					"target height": axm.snapMeta.snapBlockHeader.Number,
				}).Info("start snap sync")

				// 1. prepare snap sync info(including epoch state which will be persisted、last sync checkpoint)
				prepareRes, snapCheckpoint, err := axm.prepareSnapSync(latestHeight)
				if err != nil {
					return nil, fmt.Errorf("prepare sync: %w", err)
				}

				// 2. verify whether trie snapshot is legal (async with snap sync)
				verifiedCh := make(chan bool, 1)
				go func(resultCh chan bool) {
					now := time.Now()
					verified, err := axm.ViewLedger.StateLedger.VerifyTrie(axm.snapMeta.snapBlockHeader)
					if err != nil {
						resultCh <- false
						return
					}
					axm.logger.WithFields(logrus.Fields{
						"cost":   time.Since(now),
						"height": axm.snapMeta.snapBlockHeader.Number,
						"result": verified,
					}).Info("end verify trie snapshot")
					resultCh <- verified
				}(verifiedCh)

				// 3. start chain data sync
				err = axm.startSnapSync(verifiedCh, snapCheckpoint, axm.snapMeta.snapPeers, latestHeight+1, prepareRes.Data.([]*consensuspb.EpochChange))
				if err != nil {
					return nil, fmt.Errorf("snap sync err: %w", err)
				}

				// 4. wait for generating snapshot of target block
				errC := make(chan error)
				go axm.ViewLedger.StateLedger.GenerateSnapshot(axm.snapMeta.snapBlockHeader, errC)
				err = <-errC
				if err != nil {
					return nil, fmt.Errorf("snap-sync generate snapshot failed: %w", err)
				}

				// 5. end snapshot sync, change to full mode
				err = axm.Sync.SwitchMode(synccomm.SyncModeFull)
				if err != nil {
					return nil, fmt.Errorf("switch mode err: %w", err)
				}

				axm.logger.WithFields(logrus.Fields{
					"start height":  latestHeight,
					"target height": axm.snapMeta.snapBlockHeader.Number,
					"duration":      time.Since(start),
				}).Info("end snap sync")
			}
		}

		axm.ViewLedger.SnapMeta.Store(ledger.SnapInfo{Status: false, SnapBlockHeader: nil})
	}

	var txExec executor.Executor
	if rep.Config.Executor.Type == repo.ExecTypeDev {
		txExec, err = devexecutor.New(loggers.Logger(loggers.Executor))
	} else {
		txExec, err = executor.New(rep, rwLdg, axm.ChainState)
	}
	if err != nil {
		return nil, fmt.Errorf("create BlockExecutor: %w", err)
	}
	axm.BlockExecutor = txExec

	if rep.Config.Ledger.EnableIndexer {
		indexLg, err := storagemgr.OpenWithMetrics(storagemgr.GetLedgerComponentPath(rep, storagemgr.Indexer), storagemgr.Indexer)
		if err != nil {
			return nil, err
		}
		indexCachedStorage := storagemgr.NewCachedStorage(indexLg, 128).(*storagemgr.CachedStorage)
		axm.BloomRequests = make(chan chan *bloombits.Retrieval)
		axm.Indexer = indexer.NewBloomIndexer(rwLdg, indexCachedStorage, repo.BloomBitsBlocks, repo.BloomConfirms)
	}

	if rwLdg.ChainLedger.GetChainMeta().Height != 0 {
		genesisCfg, err := genesis.GetGenesisConfig(rwLdg.NewView().StateLedger)
		if err != nil {
			return nil, err
		}
		rep.GenesisConfig = genesisCfg
	}

	if err := axm.initChainState(); err != nil {
		return nil, err
	}
	return axm, nil
}

func (axm *AxiomLedger) Start() error {
	if err := axm.BlockExecutor.Start(); err != nil {
		return fmt.Errorf("block executor start: %w", err)
	}

	if !axm.Repo.StartArgs.ReadonlyMode {
		if _, err := axm.Sync.Prepare(); err != nil {
			return fmt.Errorf("sync prepare: %w", err)
		}
		axm.Sync.Start()

		if err := axm.Consensus.Start(); err != nil {
			return fmt.Errorf("consensus start: %w", err)
		}
	}
	if axm.Repo.Config.Ledger.EnableIndexer {
		axm.Indexer.Start(axm.BlockExecutor)
	}

	axm.start()

	axm.printLogo()

	return nil
}

func (axm *AxiomLedger) Stop() error {
	if axm.Repo.Config.Consensus.Type != repo.ConsensusTypeSolo && !axm.Repo.StartArgs.ReadonlyMode {
		if err := axm.Network.Stop(); err != nil {
			return fmt.Errorf("network stop: %w", err)
		}
	}
	if !axm.Repo.StartArgs.ReadonlyMode {
		axm.Consensus.Stop()
	}
	if err := axm.BlockExecutor.Stop(); err != nil {
		return fmt.Errorf("block executor stop: %w", err)
	}
	axm.Cancel()

	axm.logger.Infof("%s stopped", repo.AppName)

	return nil
}

func (axm *AxiomLedger) initChainState() error {
	lg := axm.ViewLedger.NewView()
	chainMeta := lg.ChainLedger.GetChainMeta()
	nodeManagerContract := framework.NodeManagerBuildConfig.Build(syscommon.NewViewVMContext(lg.StateLedger))
	votingPowers, err := nodeManagerContract.GetActiveValidatorVotingPowers()
	if err != nil {
		return err
	}

	epochManagerContract := framework.EpochManagerBuildConfig.Build(syscommon.NewViewVMContext(lg.StateLedger))
	currentEpoch, err := epochManagerContract.CurrentEpoch()
	if err != nil {
		return err
	}

	if err := axm.ChainState.UpdateByEpochInfo(currentEpoch.ToTypesEpoch(), lo.SliceToMap(votingPowers, func(item node_manager.ConsensusVotingPower) (uint64, int64) {
		return item.NodeID, item.ConsensusVotingPower
	})); err != nil {
		return err
	}
	axm.ChainState.UpdateChainMeta(chainMeta)
	axm.ChainState.TryUpdateSelfNodeInfo()
	axm.ViewLedger.StateLedger.UpdateChainState(axm.ChainState)
	return nil
}

func (axm *AxiomLedger) printLogo() {
	if !axm.Repo.StartArgs.ReadonlyMode {
		for {
			time.Sleep(100 * time.Millisecond)
			err := axm.Consensus.Ready()
			if err == nil {
				break
			}
		}
	}

	axm.logger.WithFields(logrus.Fields{
		"consensus_type": axm.Repo.Config.Consensus.Type,
	}).Info("Consensus is ready")
	fig := figure.NewFigure(repo.AppName, "slant", true)
	axm.logger.WithField(log.OnlyWriteMsgWithoutFormatterField, nil).Infof(`
=========================================================================================
%s
=========================================================================================
`, fig.String())
}

func raiseUlimit(limitNew uint64) error {
	_, err := fdlimit.Raise(limitNew)
	if err != nil {
		return fmt.Errorf("set limit failed: %w", err)
	}

	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return fmt.Errorf("getrlimit error: %w", err)
	}

	if limit.Cur != limitNew && limit.Cur != limit.Max {
		return errors.New("failed to raise ulimit")
	}

	return nil
}
