package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"syscall"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/sirupsen/logrus"

	rbft "github.com/axiomesh/axiom-bft"
	consensus2 "github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/storage"
	"github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/api/jsonrpc"
	"github.com/axiomesh/axiom-ledger/internal/consensus"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/executor"
	devexecutor "github.com/axiomesh/axiom-ledger/internal/executor/dev"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/base"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/genesis"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/internal/sync"
	sync_comm "github.com/axiomesh/axiom-ledger/internal/sync/common"
	txpool2 "github.com/axiomesh/axiom-ledger/internal/txpool"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/profile"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

type AxiomLedger struct {
	Ctx           context.Context
	Cancel        context.CancelFunc
	Repo          *repo.Repo
	logger        logrus.FieldLogger
	ViewLedger    *ledger.Ledger
	BlockExecutor executor.Executor
	Consensus     consensus.Consensus
	TxPool        txpool.TxPool[types.Transaction, *types.Transaction]
	Network       network.Network
	Sync          sync_comm.Sync
	Monitor       *profile.Monitor
	Pprof         *profile.Pprof
	LoggerWrapper *loggers.LoggerWrapper
	Jsonrpc       *jsonrpc.ChainBrokerService

	epochStore storage.Storage
	snapMeta   *snapMeta
}

func NewAxiomLedger(rep *repo.Repo, ctx context.Context, cancel context.CancelFunc) (*AxiomLedger, error) {
	axm, err := NewAxiomLedgerWithoutConsensus(rep, ctx, cancel)
	if err != nil {
		return nil, fmt.Errorf("generate draconis without consensus failed: %w", err)
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
		epcCnf := &txpool.EpochConfig{
			BatchSize:           rep.EpochInfo.ConsensusParams.BlockMaxTxNum,
			EnableGenEmptyBatch: rep.EpochInfo.ConsensusParams.EnableTimedGenEmptyBlock,
		}
		chainInfo := &txpool.ChainInfo{
			Height:    chainMeta.Height,
			GasPrice:  chainMeta.GasPrice,
			EpochConf: epcCnf,
		}

		priceLimit := poolConf.PriceLimit
		// ensure price limit is not less than min gas price
		if rep.EpochInfo.FinanceParams.MinGasPrice > priceLimit {
			priceLimit = rep.EpochInfo.FinanceParams.MinGasPrice
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
			ChainInfo:              chainInfo,
			PriceLimit:             priceLimit,
			PriceBump:              poolConf.PriceBump,
			GenerateBatchType:      poolConf.GenerateBatchType,
		}
		axm.TxPool, err = txpool2.NewTxPool[types.Transaction, *types.Transaction](txpoolConf)
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
			common.WithConfig(rep.RepoRoot, rep.ConsensusConfig),
			common.WithGenesisEpochInfo(rep.GenesisConfig.EpochInfo.Clone()),
			common.WithConsensusType(rep.Config.Consensus.Type),
			common.WithConsensusStorageType(rep.Config.Consensus.StorageType),
			common.WithPrivKey(rep.P2PKey),
			common.WithNetwork(axm.Network),
			common.WithLogger(loggers.Logger(loggers.Consensus)),
			common.WithApplied(chainMeta.Height),
			common.WithDigest(chainMeta.BlockHash.String()),
			common.WithGenesisDigest(genesisBlockHeader.Hash().String()),
			common.WithGetChainMetaFunc(axm.ViewLedger.ChainLedger.GetChainMeta),
			common.WithGetBlockHeaderFunc(axm.ViewLedger.ChainLedger.GetBlockHeader),
			common.WithGetAccountBalanceFunc(func(address string) *big.Int {
				return axm.ViewLedger.NewView().StateLedger.GetBalance(types.NewAddressByStr(address))
			}),
			common.WithGetAccountNonceFunc(func(address *types.Address) uint64 {
				return axm.ViewLedger.NewView().StateLedger.GetNonce(address)
			}),
			common.WithGetEpochInfoFromEpochMgrContractFunc(func(epoch uint64) (*rbft.EpochInfo, error) {
				return base.GetEpochInfo(axm.ViewLedger.NewView().StateLedger, epoch)
			}),
			common.WithGetCurrentEpochInfoFromEpochMgrContractFunc(func() (*rbft.EpochInfo, error) {
				return base.GetCurrentEpochInfo(axm.ViewLedger.NewView().StateLedger)
			}),
			common.WithBlockSync(axm.Sync),
			common.WithEpochStore(axm.epochStore),
		)
		if err != nil {
			return nil, fmt.Errorf("initialize consensus failed: %w", err)
		}
	}

	return axm, nil
}

func PrepareAxiomLedger(rep *repo.Repo) error {
	types.InitEIP155Signer(big.NewInt(int64(rep.GenesisConfig.ChainID)))

	if err := storagemgr.Initialize(rep.Config.Storage.KvType, rep.Config.Storage.KvCacheSize, rep.Config.Storage.Sync, rep.Config.Monitor.Enable); err != nil {
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
		stateLg, err := storagemgr.OpenWithMetrics(repo.GetStoragePath(rep.RepoRoot, storagemgr.Ledger), storagemgr.Ledger)
		if err != nil {
			return nil, err
		}
		rwLdg, err = ledger.NewLedgerWithStores(rep, nil, stateLg, nil, nil)
		if err != nil {
			return nil, err
		}
		snap, err = loadSnapMeta(rwLdg)
		if err != nil {
			return nil, err
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
		if rwLdg.ChainLedger.GetChainMeta().Height == 0 {
			if err := genesis.Initialize(rep.GenesisConfig, rwLdg); err != nil {
				return nil, err
			}
			logger.WithFields(logrus.Fields{
				"genesis block hash": rwLdg.ChainLedger.GetChainMeta().BlockHash,
			}).Info("Initialize genesis")
		}
	}
	if err != nil {
		return nil, fmt.Errorf("create RW ledger: %w", err)
	}

	vl := rwLdg.NewView()
	var net network.Network
	if !rep.StartArgs.ReadonlyMode {
		net, err = network.New(rep, loggers.Logger(loggers.P2P), vl)
		if err != nil {
			return nil, fmt.Errorf("create peer manager: %w", err)
		}
	}

	var syncMgr *sync.SyncManager
	var epochStore storage.Storage
	if !rep.StartArgs.ReadonlyMode {
		epochStore, err = storagemgr.OpenWithMetrics(repo.GetStoragePath(rep.RepoRoot, storagemgr.Epoch), storagemgr.Epoch)
		if err != nil {
			return nil, err
		}
		syncMgr, err = sync.NewSyncManager(loggers.Logger(loggers.BlockSync), vl.ChainLedger.GetChainMeta, vl.ChainLedger.GetBlock, vl.ChainLedger.GetBlockHeader,
			vl.ChainLedger.GetBlockReceipts, epochStore.Get, net, rep.Config.Sync)
		if err != nil {
			return nil, fmt.Errorf("create block sync: %w", err)
		}
	}

	axm := &AxiomLedger{
		Ctx:        ctx,
		Cancel:     cancel,
		Repo:       rep,
		logger:     logger,
		ViewLedger: vl,
		Network:    net,
		Sync:       syncMgr,

		snapMeta:   snap,
		epochStore: epochStore,
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
				err = axm.startSnapSync(verifiedCh, snapCheckpoint, axm.snapMeta.snapPeers, latestHeight+1, prepareRes.Data.([]*consensus2.EpochChange))
				if err != nil {
					return nil, fmt.Errorf("snap sync err: %w", err)
				}

				// 4. end snapshot sync, change to full mode
				err = axm.Sync.SwitchMode(sync_comm.SyncModeFull)
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
		txExec, err = executor.New(rep, rwLdg)
	}
	if err != nil {
		return nil, fmt.Errorf("create BlockExecutor: %w", err)
	}
	axm.BlockExecutor = txExec

	if rwLdg.ChainLedger.GetChainMeta().Height != 0 {
		genesisCfg, err := genesis.GetGenesisConfig(rwLdg.NewViewWithoutCache())
		if err != nil {
			return nil, err
		}
		rep.GenesisConfig = genesisCfg
	}

	// read current epoch info from ledger
	axm.Repo.EpochInfo, err = base.GetCurrentEpochInfo(axm.ViewLedger.StateLedger)
	if err != nil {
		return nil, err
	}
	vl.WithGetEpochInfoFunc(base.GetEpochInfo)
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
