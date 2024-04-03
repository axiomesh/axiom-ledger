package txpool

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axiomesh/axiom-ledger/pkg/repo"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rbft "github.com/axiomesh/axiom-bft"
	log2 "github.com/axiomesh/axiom-kit/log"
	commonpool "github.com/axiomesh/axiom-kit/txpool"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/components/timer"
)

func TestNewTxPool(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		ast.False(pool.IsPoolFull())
	}
}

func TestTxPoolImpl_Start(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		err := pool.Start()
		ast.Nil(err)

		wrongEvent := &types.Block{}
		pool.revCh <- wrongEvent
		pool.Stop()

		ctx, cancel := context.WithCancel(context.Background())

		log := log2.NewWithModule("txpool")
		pool = &txPoolImpl[types.Transaction, *types.Transaction]{
			logger:   log,
			revCh:    make(chan txPoolEvent, maxChanSize),
			timerMgr: timer.NewTimerManager(log),

			ctx:    ctx,
			cancel: cancel,
		}
		err = pool.Start()
		ast.NotNil(err)
		ast.Contains(err.Error(), "timer RemoveTx doesn't exist")
		pool.Stop()

		t.Run("test load tx records failed", func(t *testing.T) {
			wrongPool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)

			wrongPool.txRecords.filePath = fmt.Sprintf("wrong-path-%d", time.Now().Unix())
			err = wrongPool.Start()
			ast.NotNil(err)
			ast.Contains(err.Error(), "no such file or directory")
		})
	}

}

func TestTxPoolImpl_StartWithTxRecordsFile(t *testing.T) {
	ast := assert.New(t)
	t.Parallel()
	t.Run("test load tx records successfully", func(t *testing.T) {
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			recordFile := pool.txRecordsFile
			err := pool.Start()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			lo.ForEach(txs, func(tx *types.Transaction, _ int) {
				err = pool.AddLocalTx(tx)
				ast.Nil(err)
			})
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(10, pool.txStore.localTTLIndex.size())

			pool.Stop()

			// start pool which txRecordsFile is not empty
			pool = mockTxPoolImpl[types.Transaction, *types.Transaction](t)
			pool.txRecords.filePath = recordFile
			ast.Equal(0, len(pool.txStore.txHashMap))
			ast.Equal(0, pool.txStore.localTTLIndex.size())

			// load tx records successfully
			err = pool.Start()
			ast.Nil(err)
			defer pool.Stop()
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(10, pool.txStore.localTTLIndex.size())
		}
	})
	t.Run("test load tx records failed, pool is full", func(t *testing.T) {
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			recordFile := pool.txRecordsFile
			err := pool.Start()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			lo.ForEach(txs, func(tx *types.Transaction, _ int) {
				err = pool.AddLocalTx(tx)
				ast.Nil(err)
			})
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(10, pool.txStore.localTTLIndex.size())

			pool.Stop()

			// start pool which txRecordsFile is not empty
			pool = mockTxPoolImpl[types.Transaction, *types.Transaction](t)
			pool.txRecords.filePath = recordFile
			ast.Equal(0, len(pool.txStore.txHashMap))
			ast.Equal(0, pool.txStore.localTTLIndex.size())

			// decrease pool size
			pool.poolMaxSize = 5
			err = pool.Start()
			ast.Nil(err)
			defer pool.Stop()
			ast.Equal(5, len(pool.txStore.allTxs[from].items))
			ast.Equal(5, len(pool.txStore.txHashMap))
			ast.Equal(5, pool.txStore.localTTLIndex.size())
		}
	})
}

func TestTxPoolImpl_AddLocalTx(t *testing.T) {
	t.Parallel()
	t.Run("nonce is wanted", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			tx := constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(1, len(pool.txStore.allTxs[from].items))
			ast.Equal(1, len(pool.txStore.txHashMap))
			ast.Equal(uint64(0), pool.txStore.txHashMap[tx.RbftGetTxHash()].nonce)
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			ast.Equal(1, getPrioritySize(pool))
			ast.Equal(0, pool.txStore.parkingLotIndex.size())
			ast.Equal(1, pool.txStore.localTTLIndex.size())
			ast.Equal(1, pool.txStore.removeTTLIndex.size())

			poolTx := pool.txStore.allTxs[from].items[0]
			ast.Equal(tx.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash())

			all, err := GetAllTxRecords(pool.txRecordsFile)
			assert.Nil(t, err)
			assert.Equal(t, 1, len(all))
		}
	})

	t.Run("pool is full", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			pool.poolMaxSize = 1
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)
			ast.False(pool.statusMgr.In(PoolFull))

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(tx.RbftGetTxHash(), pool.GetPendingTxByHash(tx.RbftGetTxHash()).RbftGetTxHash())
			ast.True(pool.statusMgr.In(PoolFull))

			tx1 := constructTx(s, 1)
			err = pool.AddLocalTx(tx1)
			ast.NotNil(err)
			ast.Contains(err.Error(), ErrTxPoolFull.Error())
		}
	})

	t.Run("nonce is bigger", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			tx := constructTx(s, 1)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(1, len(pool.txStore.allTxs[from].items))
			ast.Equal(1, len(pool.txStore.txHashMap))
			ast.Equal(uint64(1), pool.txStore.txHashMap[tx.RbftGetTxHash()].nonce)
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize)
			ast.Equal(0, getPrioritySize(pool))
			ast.Equal(1, pool.txStore.parkingLotIndex.size())
			poolTx := pool.txStore.allTxs[from].items[1]
			ast.Equal(tx.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash())

			tx = constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)
			ast.Equal(uint64(2), pool.txStore.priorityNonBatchSize, "we receive wanted nonce")
			ast.Equal(uint64(0), pool.txStore.parkingLotSize, "we receive wanted nonce, tx1 from parking lot move to priority")

			ast.Equal(2, pool.txStore.localTTLIndex.size())
			ast.Equal(2, pool.txStore.removeTTLIndex.size())
			ast.Equal(2, getPrioritySize(pool))
			ast.Equal(1, pool.txStore.parkingLotIndex.size(), "decrease it when we trigger removeBatch")
		}
	})

	t.Run("nonce is lower", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			for _, tx := range txs {
				err = pool.AddLocalTx(tx)
				ast.Nil(err)
			}

			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(uint64(10), pool.txStore.priorityNonBatchSize)
			ast.Equal(10, getPrioritySize(pool))
			ast.Equal(0, pool.txStore.parkingLotIndex.size())

			lowTx, err := types.GenerateTransactionWithSigner(0, to, big.NewInt(0), []byte("test"), s)
			ast.Nil(err)
			err = pool.AddLocalTx(lowTx)
			ast.NotNil(err)
			ast.Contains(err.Error(), ErrBelowPriceBump.Error())
		}
	})

	t.Run("duplicate tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)

			highTx := constructTx(s, 10)
			err = pool.AddLocalTx(highTx)
			ast.Nil(err)

			err = pool.AddLocalTx(highTx)
			ast.NotNil(err)
			ast.Contains(err.Error(), ErrDuplicateTx.Error())
		}
	})

	t.Run("duplicate nonce but not the same tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			from := s.Addr.String()
			ast.Nil(err)

			// insert tx0(belong to priority)
			tx01 := constructTx(s, 0)
			err = pool.AddLocalTx(tx01)
			ast.Nil(err)
			ast.Equal(1, pool.txStore.localTTLIndex.size())
			ast.Equal(1, pool.txStore.removeTTLIndex.size())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			oldPoolTx := pool.txStore.getPoolTxByTxnPointer(tx01.RbftGetFrom(), tx01.RbftGetNonce())
			ast.NotNil(oldPoolTx)

			// replace tx01, but new tx gas price is lower than tx01*(1+priceBump/100)
			lowPriceTx02, err := types.GenerateTransactionWithSigner(0, to, big.NewInt(1000), nil, s)
			ast.Nil(err)

			err = pool.AddLocalTx(lowPriceTx02)
			ast.NotNil(err)
			ast.Contains(err.Error(), ErrBelowPriceBump.Error())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(1, getPrioritySize(pool))
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)

			// replace tx01, replace success
			newTx := constructPoolTxByGas(s, 0, new(big.Int).Mul(oldPoolTx.getGasPrice(), big.NewInt(2)))
			err = pool.AddLocalTx(newTx.rawTx)
			ast.Nil(err)
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			ast.Equal(1, getPrioritySize(pool))
			ast.Equal(1, pool.txStore.removeTTLIndex.size())
			newPoolTx := pool.txStore.getPoolTxByTxnPointer(oldPoolTx.getAccount(), oldPoolTx.getNonce())
			ast.Equal(newTx.getGasPrice(), newPoolTx.getGasPrice())

			// insert tx2(belong to parking lot)
			tx21 := constructTx(s, 2)
			err = pool.AddLocalTx(tx21)
			ast.Nil(err)
			poolTx := pool.txStore.getPoolTxByTxnPointer(from, tx21.RbftGetNonce())
			ast.Equal(tx21.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash(), "tx21 exist in txpool")
			ast.Equal(2, pool.txStore.localTTLIndex.size())
			ast.Equal(2, pool.txStore.removeTTLIndex.size())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize, "tx21 belong to parking lot")

			// replace tx21, but new tx gas price is lower than tx21*(1+priceBump/100)
			lowPriceTx22, err := types.GenerateTransactionWithSigner(2, to, big.NewInt(1000), nil, s)
			err = pool.AddLocalTx(lowPriceTx22)
			ast.NotNil(err)
			ast.Contains(err.Error(), ErrBelowPriceBump.Error())

			// replace tx21, replace success
			newTx = constructPoolTxByGas(s, 2, new(big.Int).Mul(tx21.GetGasPrice(), big.NewInt(2)))
			tx22 := newTx.rawTx
			ast.Equal(tx21.RbftGetNonce(), tx22.RbftGetNonce())

			err = pool.AddLocalTx(tx22)
			ast.Nil(err)
			ast.NotEqual(tx21.RbftGetTxHash(), tx22.RbftGetTxHash())
			poolTx = pool.txStore.getPoolTxByTxnPointer(from, tx22.RbftGetNonce())
			ast.Equal(tx22.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash(), "tx22 replaced tx21")
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize, "tx22 belong to parking lot")
			ast.Equal(2, pool.txStore.localTTLIndex.size())
			ast.Equal(2, pool.txStore.removeTTLIndex.size())
			removeKey := &orderedIndexKey{
				account: poolTx.getAccount(),
				nonce:   poolTx.rawTx.RbftGetNonce(),
				time:    poolTx.arrivedTime,
			}
			v := pool.txStore.removeTTLIndex.data.Get(removeKey)
			ast.NotNil(v)
			localKey := &orderedIndexKey{
				account: poolTx.getAccount(),
				nonce:   poolTx.rawTx.RbftGetNonce(),
				time:    poolTx.getRawTimestamp(),
			}
			v = pool.txStore.localTTLIndex.data.Get(localKey)
			ast.NotNil(v)
		}
	})

	t.Run("nonce is bigger than tolerance, trigger remove high nonce tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.toleranceNonceGap = 1
			err := pool.Start()
			defer pool.Stop()

			s, err := types.GenerateSigner()
			tx0 := constructTx(s, 0)
			err = pool.AddLocalTx(tx0)
			ast.Nil(err)
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize, "tx0 belong to priority non-batch")
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)

			tx2 := constructTx(s, 2)
			err = pool.AddLocalTx(tx2)
			ast.Nil(err)
			ast.Equal(2, len(pool.txStore.allTxs[tx2.RbftGetFrom()].items))
			ast.Equal(2, pool.txStore.localTTLIndex.size())
			ast.Equal(2, pool.txStore.removeTTLIndex.size())
			ast.Equal(1, pool.txStore.parkingLotIndex.size())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize, "tx2 belong to parking lot")

			tx3 := constructTx(s, 3)
			err = pool.AddLocalTx(tx3)
			ast.NotNil(err)
			ast.Nil(pool.GetPendingTxByHash(tx2.RbftGetTxHash()), "tx2 is not exist in txpool")
			ast.Equal(1, pool.txStore.localTTLIndex.size())
			ast.Equal(1, pool.txStore.removeTTLIndex.size())
			ast.Equal(0, pool.txStore.parkingLotIndex.size())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize, "tx2 and tx3 had been removed from parking lot")
		}
	})

	t.Run("gas price too low", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			// set price limit is bigger than tx gas price
			pool.setPriceLimit(tx.RbftGetGasPrice().Uint64() + 1)
			err = pool.AddLocalTx(tx)
			ast.NotNil(err)
			ast.Contains(err.Error(), ErrGasPriceTooLow.Error())
		}
	})
}

func TestTxPoolImpl_AddRemoteTxs(t *testing.T) {
	t.Parallel()
	t.Run("nonce is wanted", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			tx := constructTx(s, 0)
			pool.AddRemoteTxs([]*types.Transaction{tx})
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(tx.RbftGetTxHash(), pool.GetPendingTxByHash(tx.RbftGetTxHash()).RbftGetTxHash())
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(1, len(pool.txStore.allTxs[from].items))
			ast.Equal(1, len(pool.txStore.txHashMap))
			ast.Equal(uint64(0), pool.txStore.txHashMap[tx.RbftGetTxHash()].nonce)
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			ast.Equal(1, getPrioritySize(pool))
			ast.Equal(0, pool.txStore.parkingLotIndex.size())
			poolTx := pool.txStore.allTxs[from].items[0]
			ast.Equal(tx.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash())
		}
	})

	t.Run("pool is full", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			pool.poolMaxSize = 2
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)
			ast.False(pool.statusMgr.In(PoolFull))

			s, err := types.GenerateSigner()
			ast.Nil(err)
			txs := constructTxs(s, 5)
			pool.AddRemoteTxs(txs)
			// because add remote txs is async,
			// so we need to send getMeta event to ensure last event is handled
			ast.Equal(uint64(2), pool.GetMeta(false).TxCount)
			ast.True(pool.statusMgr.In(PoolFull))

			tx6 := constructTx(s, 6)
			pool.AddRemoteTxs([]*types.Transaction{tx6})
			ast.Nil(pool.GetPendingTxByHash(tx6.RbftGetTxHash()), "tx6 is not exit in txpool, because pool is full")
		}
	})

	t.Run("nonce is bigger in same txs", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			// remove tx5
			lackTx := txs[5]
			txs = append(txs[:5], txs[6:]...)

			pool.AddRemoteTxs(txs)
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(txs[0].RbftGetTxHash(), pool.GetPendingTxByHash(txs[0].RbftGetTxHash()).RbftGetTxHash())
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(9, len(pool.txStore.allTxs[from].items))
			ast.Equal(9, len(pool.txStore.txHashMap))
			ast.Equal(5, getPrioritySize(pool), "tx0-tx4 are in priority minNonceQueue")
			ast.Equal(4, pool.txStore.parkingLotIndex.size(), "tx6-tx9 are in parking lot")
			ast.Equal(uint64(5), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(4), pool.txStore.parkingLotSize)
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(9, pool.txStore.removeTTLIndex.size())

			pool.AddRemoteTxs([]*types.Transaction{lackTx})
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(lackTx.RbftGetTxHash(), pool.GetPendingTxByHash(lackTx.RbftGetTxHash()).RbftGetTxHash())
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(10, getPrioritySize(pool), "tx0-tx0 are in priority minNonceQueue")
			ast.Equal(4, pool.txStore.parkingLotIndex.size(), "tx6-tx9 are in parking lot")
			ast.Equal(uint64(10), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize, "tx6-tx9 status is not in parking lot")
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(10, pool.txStore.removeTTLIndex.size())
		}
	})

	t.Run("nonce is bigger in different txs", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			// remove tx0
			lackTx := txs[0]
			txs1 := append(txs[1:5])
			txs2 := append(txs[5:10])

			pool.AddRemoteTxs(txs1)
			// because add remote txs is async,
			// so we need to send getMeta event to ensure last event is handled
			ast.Equal(uint64(0), pool.GetAccountMeta(from, true).PendingNonce)
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(4, len(pool.txStore.allTxs[from].items))
			ast.Equal(4, len(pool.txStore.txHashMap))
			ast.Equal(0, getPrioritySize(pool))
			ast.Equal(4, pool.txStore.parkingLotIndex.size(), "tx1-tx4 are in parking lot")
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(4), pool.txStore.parkingLotSize)
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(4, pool.txStore.removeTTLIndex.size())

			pool.AddRemoteTxs(txs2)
			ast.Equal(uint64(0), pool.GetAccountMeta(from, true).PendingNonce)
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(9, len(pool.txStore.allTxs[from].items))
			ast.Equal(9, len(pool.txStore.txHashMap))
			ast.Equal(0, getPrioritySize(pool))
			ast.Equal(9, pool.txStore.parkingLotIndex.size(), "tx1-tx9 are in parking lot")
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(9), pool.txStore.parkingLotSize)
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(9, pool.txStore.removeTTLIndex.size())

			pool.AddRemoteTxs([]*types.Transaction{lackTx})
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(lackTx.RbftGetTxHash(), pool.GetPendingTxByHash(lackTx.RbftGetTxHash()).RbftGetTxHash())
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(10, getPrioritySize(pool), "tx0-tx9 are in priority minNonceQueue")
			ast.Equal(9, pool.txStore.parkingLotIndex.size(), "tx1-tx9 are in parking lot")
			ast.Equal(uint64(10), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize, "tx6-tx9 status is not in parking lot")
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(10, pool.txStore.removeTTLIndex.size())
		}
	})

	t.Run("nonce is lower", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			pool.AddRemoteTxs(txs)
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(txs[0].RbftGetTxHash(), pool.GetPendingTxByHash(txs[0].RbftGetTxHash()).RbftGetTxHash())

			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(uint64(10), pool.txStore.priorityNonBatchSize)
			ast.Equal(10, getPrioritySize(pool))
			ast.Equal(0, pool.txStore.parkingLotIndex.size())

			lowTx := txs[0]
			pool.AddRemoteTxs([]*types.Transaction{lowTx})
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(txs[0].RbftGetTxHash(), pool.GetPendingTxByHash(txs[0].RbftGetTxHash()).RbftGetTxHash())
			ast.Equal(10, len(pool.txStore.allTxs[from].items), "add remote tx failed because tx nonce is lower")
		}
	})

	t.Run("duplicate tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)
			ast.Equal(1, pool.txStore.localTTLIndex.size())

			pool.AddRemoteTxs([]*types.Transaction{tx})
			ast.Equal(tx.RbftGetTxHash(), pool.GetPendingTxByHash(tx.RbftGetTxHash()).RbftGetTxHash())
			ast.Equal(1, pool.txStore.localTTLIndex.size(), "remote tx not replace it")

			highTx := constructTx(s, 10)
			err = pool.AddLocalTx(highTx)
			ast.Nil(err)
			ast.Equal(2, pool.txStore.localTTLIndex.size())

			pool.AddRemoteTxs([]*types.Transaction{highTx})
			ast.Equal(tx.RbftGetTxHash(), pool.GetPendingTxByHash(tx.RbftGetTxHash()).RbftGetTxHash())
			ast.Equal(2, pool.txStore.localTTLIndex.size(), "remote tx not replace it")
		}
	})

	t.Run("duplicate nonce but not the same tx in different txs", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			from := s.Addr.String()
			ast.Nil(err)
			tx01 := constructTx(s, 0)
			err = pool.AddLocalTx(tx01)
			ast.Nil(err)
			ast.Equal(1, pool.txStore.localTTLIndex.size(), "tx01 exist in txpool")
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			oldPoolTx := pool.txStore.getPoolTxByTxnPointer(from, tx01.RbftGetNonce())

			// replace tx01, but new tx gas price is lower than tx01*(1+priceBump/100)
			time.Sleep(1 * time.Millisecond)
			tx02, err := types.GenerateTransactionWithSigner(0, to, big.NewInt(1000), nil, s)
			ast.Nil(err)
			ast.NotEqual(tx01.RbftGetTxHash(), tx02.RbftGetTxHash())

			pool.AddRemoteTxs([]*types.Transaction{tx02})
			ast.Equal(uint64(1), pool.GetTotalPendingTxCount())
			ast.Nil(pool.GetPendingTxByHash(tx02.RbftGetTxHash()))

			// replace tx01 with tx03 success
			poolTx := constructPoolTxByGas(s, 0, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(2)))
			tx03 := poolTx.rawTx
			ast.NotEqual(tx01.RbftGetTxHash(), tx03.RbftGetTxHash())
			pool.AddRemoteTxs([]*types.Transaction{tx02, tx03})
			ast.Equal(uint64(1), pool.GetTotalPendingTxCount())
			ast.NotNil(pool.GetPendingTxByHash(tx03.RbftGetTxHash()))
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			poolTx = pool.txStore.getPoolTxByTxnPointer(from, tx03.RbftGetNonce())
			ast.True(oldPoolTx.arrivedTime < poolTx.arrivedTime)
			if pool.enablePricePriority {
				ast.Equal(0, tx03.GetGasPrice().Cmp(pool.txStore.priorityByPrice.peek().getGasPrice()))
			} else {
				data := pool.txStore.priorityByTime.data.Get(&orderedIndexKey{time: poolTx.getRawTimestamp(), account: poolTx.getAccount(), nonce: poolTx.getNonce()})
				ast.NotNil(data)
				ast.Equal(1, pool.txStore.priorityByTime.size())
			}

			// add tx21,belong to parking lot
			tx21 := constructTx(s, 2)
			err = pool.AddLocalTx(tx21)
			ast.Nil(err)
			poolTx = pool.txStore.getPoolTxByTxnPointer(from, tx21.RbftGetNonce())
			ast.Equal(tx21.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash(), "tx21 exist in txpool")
			ast.Equal(1, pool.txStore.localTTLIndex.size(), "tx21 exist in txpool, tx0 is belong to remote tx")
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize, "tx21 exist in parking lot")

			time.Sleep(1 * time.Millisecond)
			tx22, err := types.GenerateTransactionWithSigner(2, to, big.NewInt(1000), nil, s)
			ast.Nil(err)
			ast.NotEqual(tx21.RbftGetTxHash(), tx22.RbftGetTxHash())
			ast.Equal(tx21.RbftGetNonce(), tx22.RbftGetNonce())
			poolTx = constructPoolTxByGas(s, 2, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(2)))
			tx23 := poolTx.rawTx

			pool.AddRemoteTxs([]*types.Transaction{tx22, tx23})
			ast.Equal(tx23.RbftGetTxHash(), pool.GetPendingTxByHash(tx23.RbftGetTxHash()).RbftGetTxHash())

			poolTx = pool.txStore.getPoolTxByTxnPointer(from, tx21.RbftGetNonce())
			ast.Equal(tx23.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash(), "tx23 replaced tx21")
			ast.Equal(0, pool.txStore.localTTLIndex.size(), "tx0 and tx2 are belong to remote tx")
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize, "tx22 exist in parking lot")
		}
	})

	t.Run("duplicate nonce but not the same tx in same txs", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx01 := constructTx(s, 0)
			replaceWrongTx0, err := types.GenerateTransactionWithSigner(0, to, big.NewInt(1000), nil, s)
			ast.Nil(err)

			poolTx2 := constructPoolTxByGas(s, 0, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(2)))
			poolTx3 := constructPoolTxByGas(s, 0, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(3)))
			poolTx4 := constructPoolTxByGas(s, 0, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(4)))
			poolTx5 := constructPoolTxByGas(s, 0, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(5)))
			tx02 := poolTx2.rawTx
			tx03 := poolTx3.rawTx
			tx04 := poolTx4.rawTx
			tx05 := poolTx5.rawTx

			txs := []*types.Transaction{tx01, replaceWrongTx0, tx02, tx03, tx04, tx05}
			pool.AddRemoteTxs(txs)
			ast.Equal(uint64(1), pool.GetTotalPendingTxCount())
			ast.NotNil(pool.GetPendingTxByHash(tx05.RbftGetTxHash()), "tx05 replace tx01")
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(1, pool.txStore.removeTTLIndex.size())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			if pool.enablePricePriority {
				ast.Equal(0, tx05.GetGasPrice().Cmp(pool.txStore.priorityByPrice.peek().getGasPrice()))
			} else {
				data := pool.txStore.priorityByTime.data.Get(&orderedIndexKey{time: poolTx5.getRawTimestamp(), account: poolTx5.getAccount(), nonce: poolTx5.getNonce()})
				ast.NotNil(data)
				ast.Equal(1, pool.txStore.priorityByTime.size())
			}

			tx21 := constructTx(s, 2)
			replaceWrongTx2, err := types.GenerateTransactionWithSigner(2, to, big.NewInt(1000), nil, s)
			ast.Nil(err)

			poolTx2 = constructPoolTxByGas(s, 2, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(2)))
			poolTx3 = constructPoolTxByGas(s, 2, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(3)))
			poolTx4 = constructPoolTxByGas(s, 2, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(4)))
			poolTx5 = constructPoolTxByGas(s, 2, new(big.Int).Mul(tx01.RbftGetGasPrice(), big.NewInt(5)))
			tx22 := poolTx2.rawTx
			tx23 := poolTx3.rawTx
			tx24 := poolTx4.rawTx
			tx25 := poolTx5.rawTx
			txs = []*types.Transaction{tx21, replaceWrongTx2, tx22, tx23, tx24, tx25}
			pool.AddRemoteTxs(txs)
			ast.Nil(pool.GetPendingTxByHash(tx21.RbftGetTxHash()), "tx21 not exist in txpool")
			ast.Equal(tx25.RbftGetTxHash(), pool.GetPendingTxByHash(tx25.RbftGetTxHash()).RbftGetTxHash())
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(2, pool.txStore.removeTTLIndex.size())
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize, "tx25 exist in parking lot")
		}
	})

	t.Run("nonce too high in same txs, trigger remove high nonce tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.toleranceNonceGap = 1
			pool.chainInfo.EpochConf.BatchSize = 500 // make sure not to trigger generate batch
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s1, err := types.GenerateSigner()
			ast.Nil(err)
			from1 := s1.Addr.String()
			s2, err := types.GenerateSigner()
			ast.Nil(err)
			from2 := s2.Addr.String()
			s3, err := types.GenerateSigner()
			ast.Nil(err)
			from3 := s3.Addr.String()

			txs1 := constructTxs(s1, 4)
			// lack tx10
			txs1 = txs1[1:]
			txs2 := constructTxs(s2, 4)
			// lack tx20
			txs2 = txs2[1:]
			txs3 := constructTxs(s3, 4)

			// txs include txs1,tx3
			txs := append(txs1, txs3...)

			pool.AddRemoteTxs(txs)
			ast.Equal(0, len(pool.GetAccountMeta(from1, false).SimpleTxs), "from1 trigger remove all high nonce tx, so its tx count should be 0")
			ast.Equal(4, len(pool.GetAccountMeta(from3, false).SimpleTxs))
			ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize, "from3 exist tx0-tx3 in txpool")
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)

			pool.AddRemoteTxs(txs2)
			ast.Equal(0, len(pool.GetAccountMeta(from2, false).SimpleTxs), "from2 trigger remove all high nonce tx, so its tx count should be 0")
			ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize, "from3 exist tx0-tx3 in txpool")
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
		}
	})
}

func TestTxPoolImpl_AddRebroadcastTxs(t *testing.T) {
	t.Parallel()
	t.Run("nonce is wanted", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			tx := constructTx(s, 0)
			pool.AddRebroadcastTxs([]*types.Transaction{tx})
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(tx.RbftGetTxHash(), pool.GetPendingTxByHash(tx.RbftGetTxHash()).RbftGetTxHash())
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(1, len(pool.txStore.allTxs[from].items))
			ast.Equal(1, len(pool.txStore.txHashMap))
			ast.Equal(uint64(0), pool.txStore.txHashMap[tx.RbftGetTxHash()].nonce)
			ast.Equal(uint64(1), pool.txStore.priorityNonBatchSize)
			ast.Equal(1, getPrioritySize(pool))
			ast.Equal(0, pool.txStore.parkingLotIndex.size())
			poolTx := pool.txStore.allTxs[from].items[0]
			ast.Equal(tx.RbftGetTxHash(), poolTx.rawTx.RbftGetTxHash())
		}
	})

	t.Run("pool is full", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			pool.poolMaxSize = 1
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)
			ast.False(pool.statusMgr.In(PoolFull))

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			pool.AddRebroadcastTxs([]*types.Transaction{tx})
			// because add remote txs is async,
			// so we need to send getPendingTxByHash event to ensure last event is handled
			ast.Equal(tx.RbftGetTxHash(), pool.GetPendingTxByHash(tx.RbftGetTxHash()).RbftGetTxHash())
			ast.True(pool.statusMgr.In(PoolFull))

			tx1 := constructTx(s, 1)
			pool.AddRebroadcastTxs([]*types.Transaction{tx1})
			ast.NotNil(pool.GetPendingTxByHash(tx1.RbftGetTxHash()), "tx1 is added to pool even if pool is full")
			actualPoolSize := pool.GetTotalPendingTxCount()
			ast.True(actualPoolSize > pool.poolMaxSize, "pool actual size is bigger than poolMaxSize")
		}
	})
}

func TestTxPoolImpl_ReceiveMissingRequests(t *testing.T) {
	prepareBatch := func(s *types.Signer) (string, map[uint64]*types.Transaction) {
		txs := constructTxs(s, 4)
		txHashList := make([]string, len(txs))
		txsM := make(map[uint64]*types.Transaction)

		lo.ForEach(txs, func(tx *types.Transaction, index int) {
			txHashList[index] = tx.RbftGetTxHash()
			txsM[uint64(index)] = tx
		})
		newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
			TxList:     txs,
			LocalList:  []bool{true, true, true, true},
			TxHashList: txHashList,
			Timestamp:  time.Now().UnixNano(),
		}

		batchDigest := newBatch.GenerateBatchHash()
		return batchDigest, txsM
	}

	t.Parallel()
	t.Run("handle missing requests", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure not notify generate batch
			pool.chainInfo.EpochConf.BatchSize = 500
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)

			batchDigest, txsM := prepareBatch(s)
			err = pool.ReceiveMissingRequests(batchDigest, txsM)
			ast.Nil(err)
			ast.Equal(0, len(pool.txStore.txHashMap), "missingBatch is nil")

			missingHashList := make(map[uint64]string)
			missingHashList[uint64(0)] = txsM[uint64(0)].RbftGetTxHash()
			missingHashList[uint64(1)] = txsM[uint64(1)].RbftGetTxHash()
			missingHashList[uint64(2)] = txsM[uint64(2)].RbftGetTxHash()
			missingHashList[uint64(3)] = "wrong_hash3"
			pool.txStore.missingBatch[batchDigest] = missingHashList
			err = pool.ReceiveMissingRequests(batchDigest, txsM)
			ast.NotNil(err, "find a hash mismatch tx")
			ast.Equal(1, len(pool.txStore.missingBatch))
			// insert right txHash
			missingHashList[uint64(3)] = txsM[uint64(3)].RbftGetTxHash()
			pool.txStore.missingBatch[batchDigest] = missingHashList
			err = pool.ReceiveMissingRequests(batchDigest, txsM)
			ast.Nil(err)
			ast.Equal(0, len(pool.txStore.missingBatch), "missingBatch had been removed")
		}
	})

	t.Run("trigger notify findNextBatch, before receive missing requests from primary, receive from addNewRequests", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure not notify generate batch
			pool.chainInfo.EpochConf.BatchSize = 500
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)

			notifySignalCh := make(chan string, 1)
			pool.notifyFindNextBatchFn = func(hashes ...string) {
				notifySignalCh <- hashes[0]
			}
			s, err = types.GenerateSigner()
			ast.Nil(err)

			batchDigest, txsM := prepareBatch(s)
			missingHashList := make(map[uint64]string)

			txs := lo.MapToSlice(txsM, func(k uint64, v *types.Transaction) *types.Transaction {
				return v
			})

			missingHashList = lo.MapEntries(txsM, func(k uint64, v *types.Transaction) (uint64, string) {
				return k, v.RbftGetTxHash()
			})

			pool.txStore.missingBatch[batchDigest] = missingHashList
			pool.AddRemoteTxs(txs)
			ast.NotNil(pool.GetPendingTxByHash(txs[0].RbftGetTxHash()))
			completedDigest := <-notifySignalCh
			ast.Equal(batchDigest, completedDigest)
			ast.Equal(0, len(pool.txStore.missingBatch), "missingBatch had been removed")
		}
	})

	t.Run("receive missing requests from primary, trigger replace tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure not notify generate batch
			pool.chainInfo.EpochConf.BatchSize = 500
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)

			oldTx0, err := types.GenerateTransactionWithSigner(0, to, big.NewInt(1000), nil, s)
			ast.Nil(err)

			// insert oldTx0
			err = pool.AddLocalTx(oldTx0)
			ast.Nil(err)
			ast.NotNil(pool.GetPendingTxByHash(oldTx0.RbftGetTxHash()))

			batchDigest, txsM := prepareBatch(s)
			missingHashList := lo.MapEntries(txsM, func(k uint64, v *types.Transaction) (uint64, string) {
				return k, v.RbftGetTxHash()
			})
			pool.txStore.missingBatch[batchDigest] = missingHashList

			err = pool.ReceiveMissingRequests(batchDigest, txsM)
			ast.Nil(err)
			ast.Nil(pool.GetPendingTxByHash(oldTx0.RbftGetTxHash()))
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(0, len(pool.txStore.missingBatch), "missingBatch had been removed")
		}
	})

	t.Run("receive missing requests from primary, but pool full", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure not notify generate batch
			pool.chainInfo.EpochConf.BatchSize = 500
			pool.poolMaxSize = 1
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)

			tx, err := types.GenerateTransactionWithSigner(0, to, big.NewInt(1000), nil, s)
			ast.Nil(err)

			// insert tx to trigger pool full
			err = pool.AddLocalTx(tx)
			ast.Nil(err)
			ast.NotNil(pool.GetPendingTxByHash(tx.RbftGetTxHash()))
			ast.True(pool.IsPoolFull())

			batchDigest, txsM := prepareBatch(s)
			missingHashList := lo.MapEntries(txsM, func(k uint64, v *types.Transaction) (uint64, string) {
				return k, v.RbftGetTxHash()
			})
			pool.txStore.missingBatch[batchDigest] = missingHashList

			err = pool.ReceiveMissingRequests(batchDigest, txsM)
			ast.Nil(err, "receive missing requests from primary, ignore pool full status")
		}
	})

	t.Run("receive missing requests from primary, insert parkinglot txs", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure not notify generate batch
			pool.chainInfo.EpochConf.BatchSize = 500
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)

			oldTx1, err := types.GenerateTransactionWithSigner(1, to, big.NewInt(1000), nil, s)
			ast.Nil(err)

			// insert oldTx1
			err = pool.AddLocalTx(oldTx1)
			ast.Nil(err)
			ast.Equal(uint64(1), pool.txStore.parkingLotSize)

			batchDigest, txsM := prepareBatch(s)
			missingHashList := lo.MapEntries(txsM, func(k uint64, v *types.Transaction) (uint64, string) {
				return k, v.RbftGetTxHash()
			})

			pool.txStore.missingBatch[batchDigest] = missingHashList

			err = pool.ReceiveMissingRequests(batchDigest, txsM)
			ast.Nil(err)
			ast.Nil(pool.GetPendingTxByHash(oldTx1.RbftGetTxHash()))
			ast.Equal(0, pool.txStore.localTTLIndex.size())
			ast.Equal(uint64(0), pool.txStore.parkingLotSize)
			ast.Equal(0, len(pool.txStore.missingBatch), "missingBatch had been removed")
		}
	})
}

func TestTxPoolImpl_AddLocalRecordTxs(t *testing.T) {
	t.Parallel()
	t.Run("add local record txs success", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 10)
			count := pool.addLocalRecordTx(txs)
			ast.Equal(len(txs), count)
			ast.NotNil(pool.txStore.allTxs[from])
			ast.Equal(10, len(pool.txStore.allTxs[from].items))
			ast.Equal(10, len(pool.txStore.txHashMap))
			ast.Equal(uint64(10), pool.txStore.priorityNonBatchSize)
			ast.Equal(10, getPrioritySize(pool))
			ast.Equal(0, pool.txStore.parkingLotIndex.size())
			ast.Equal(10, pool.txStore.localTTLIndex.size())
		}
	})
}

func TestTxPoolImpl_GenerateRequestBatch(t *testing.T) {
	t.Parallel()
	t.Run("generate wrong batch event", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			wrongTyp := 1000
			_, err = pool.GenerateRequestBatch(wrongTyp)
			ast.NotNil(err)
		}
	})

	t.Run("generate batch size event", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			ch := make(chan int, 1)
			pool.notifyGenerateBatchFn = func(typ int) {
				ch <- typ
			}
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			txs := constructTxs(s, 8)
			// remove tx1
			txs = append(txs[:1], txs[2:]...)
			pool.AddRemoteTxs(txs)
			tx1, err := types.GenerateTransactionWithSigner(1, to, big.NewInt(1000), nil, s)
			ast.Nil(err)
			err = pool.AddLocalTx(tx1)
			ast.Nil(err)
			typ := <-ch
			ast.Equal(commonpool.GenBatchSizeEvent, typ)
			batch, err := pool.GenerateRequestBatch(typ)
			ast.Nil(err)
			ast.NotNil(batch)
			txHashList := batch.TxHashList
			ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize)
			ast.Equal(1, len(pool.txStore.batchesCache))
			lo.ForEach(pool.GetMeta(true).Batches[batch.BatchHash].Txs, func(tx *commonpool.TxSimpleInfo, index int) {
				ast.Equal(txHashList[index], tx.Hash)
			})
		}
	})

	t.Run("generate batch size event which is less than batchSize", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			txs := constructTxs(s, 3)
			pool.AddRemoteTxs(txs)
			_, err = pool.GenerateRequestBatch(commonpool.GenBatchSizeEvent)
			ast.NotNil(err)
			ast.Contains(err.Error(), "ignore generate batch")
		}
	})

	t.Run("generate batch timeout event", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)

			batchTimer := timer.NewTimerManager(pool.logger)
			ch := make(chan *commonpool.RequestHashBatch[types.Transaction, *types.Transaction], 1)
			handler := func(name timer.TimeoutEvent) {
				batch, err := pool.generateRequestBatch(commonpool.GenBatchTimeoutEvent)
				ast.Nil(err)
				ch <- batch
			}
			err = batchTimer.CreateTimer(timer.Batch, 10*time.Millisecond, handler)
			ast.Nil(err)
			err = batchTimer.StartTimer(timer.Batch)
			ast.Nil(err)

			batch := <-ch
			ast.Equal(1, len(batch.TxList))
			ast.Equal(tx.RbftGetTxHash(), batch.TxHashList[0])
		}
	})

	t.Run("generate batch timeout event which tx pool is empty", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			batchTimer := timer.NewTimerManager(pool.logger)
			ch := make(chan *commonpool.RequestHashBatch[types.Transaction, *types.Transaction], 1)
			handler := func(name timer.TimeoutEvent) {
				batch, err := pool.generateRequestBatch(commonpool.GenBatchTimeoutEvent)
				ast.NotNil(err)
				ast.Contains(err.Error(), "there is no pending tx, ignore generate batch")
				ch <- batch
			}
			err = batchTimer.CreateTimer(timer.Batch, 10*time.Millisecond, handler)
			ast.Nil(err)
			err = batchTimer.StartTimer(timer.Batch)
			ast.Nil(err)

			batch := <-ch
			ast.Nil(batch)
		}
	})

	t.Run("generate no-tx batch timeout event which tx pool is not empty", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			tx := constructTx(s, 0)
			err = pool.AddLocalTx(tx)
			ast.Nil(err)

			batch, err := pool.GenerateRequestBatch(commonpool.GenBatchNoTxTimeoutEvent)
			ast.NotNil(err)
			ast.Contains(err.Error(), "there is pending tx, ignore generate no tx batch")
			ast.Nil(batch)
		}
	})

	t.Run("generate no-tx batch timeout event which not support no-tx", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			_, err = pool.GenerateRequestBatch(commonpool.GenBatchNoTxTimeoutEvent)
			ast.NotNil(err)
			ast.Contains(err.Error(), "not supported generate no tx batch")
		}
	})

	t.Run("generate no-tx batch timeout event", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.chainInfo.EpochConf.BatchSize = 4
			pool.chainInfo.EpochConf.EnableGenEmptyBatch = true
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			noTxBatchTimer := timer.NewTimerManager(pool.logger)
			ch := make(chan *commonpool.RequestHashBatch[types.Transaction, *types.Transaction], 1)
			handler := func(name timer.TimeoutEvent) {
				batch, err := pool.generateRequestBatch(commonpool.GenBatchNoTxTimeoutEvent)
				ast.Nil(err)
				ch <- batch
			}
			err = noTxBatchTimer.CreateTimer(timer.NoTxBatch, 10*time.Millisecond, handler)
			ast.Nil(err)
			err = noTxBatchTimer.StartTimer(timer.NoTxBatch)
			ast.Nil(err)

			batch := <-ch
			ast.NotNil(batch)
			ast.Equal(0, len(batch.TxList))
		}
	})

	t.Run("generate batch size event, but validate tx gasPrice failed", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// disable this case in price priority mode
			if pool.enablePricePriority {
				return
			}
			pool.chainInfo.EpochConf.BatchSize = 4
			ch := make(chan int, 1)
			pool.notifyGenerateBatchFn = func(typ int) {
				ch <- typ
			}
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 8)
			// remove tx0
			lackTxs := txs[1:]
			pool.AddRemoteTxs(lackTxs)
			ast.Equal(uint64(7), pool.GetTotalPendingTxCount())
			ast.Equal(uint64(0), pool.GetAccountMeta(from, false).PendingNonce)
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize, "lack of tx0")

			// ensure txGasPrice is less than chainGasPrice
			txGasPrice := new(big.Int).Sub(pool.chainInfo.GasPrice, big.NewInt(1))
			tx1, err := types.GenerateTransactionWithGasPrice(0, defaultGasLimit, txGasPrice, s)
			ast.Nil(err)
			err = pool.AddLocalTx(tx1)
			ast.Nil(err)
			ast.Equal(uint64(8), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(8), pool.GetTotalPendingTxCount())
			ast.Equal(uint64(8), pool.GetAccountMeta(from, false).PendingNonce)

			// trigger generate batch with GenBatchSizeEvent
			typ := <-ch
			ast.Equal(commonpool.GenBatchSizeEvent, typ)
			batch, err := pool.GenerateRequestBatch(typ)
			ast.NotNil(err)
			ast.Contains(err.Error(), "there is no valid tx to generate batch")
			ast.Nil(batch)
			ast.Equal(uint64(7), pool.GetTotalPendingTxCount())
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(0), pool.GetAccountMeta(from, false).PendingNonce, "remove tx0, revert pendingNonce")

			s2, err := types.GenerateSigner()
			ast.Nil(err)
			from2 := s2.Addr.String()
			txs = constructTxs(s2, 8)

			// remove tx2
			lackTxs = append(txs[:2], txs[3:]...)
			pool.AddRemoteTxs(lackTxs)
			ast.Equal(uint64(2), pool.GetAccountMeta(from2, false).PendingNonce)
			ast.Equal(uint64(2), pool.txStore.priorityNonBatchSize, "lack of tx2, tx0 and tx1 match generated batch")

			// ensure verifying insufficient balance failed
			tx2, err := types.GenerateTransactionWithGasPrice(2, math.MaxInt64, pool.chainInfo.GasPrice, s2)
			ast.Nil(err)
			err = pool.AddLocalTx(tx2)
			ast.Nil(err)
			// trigger generate batch with GenBatchSizeEvent
			typ = <-ch
			ast.Equal(commonpool.GenBatchSizeEvent, typ)
			batch, err = pool.GenerateRequestBatch(typ)
			ast.Nil(err)
			ast.NotNil(batch)
			ast.Equal(uint64(2), batch.BatchItemSize(), "including tx0, tx1")
			ast.Equal(uint64(14), pool.GetTotalPendingTxCount(), "remove tx2")
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
			ast.Equal(uint64(2), pool.GetAccountMeta(from2, false).PendingNonce)
		}
	})

	t.Run("generate batch size event, but account balance not enough", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure account balance not enough
			pool.getAccountBalance = func(address string) *big.Int {
				return big.NewInt(0)
			}

			pool.chainInfo.EpochConf.BatchSize = 4
			ch := make(chan int, 1)
			pool.notifyGenerateBatchFn = func(typ int) {
				ch <- typ
			}
			err := pool.Start()
			defer pool.Stop()
			ast.Nil(err)

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			txs := constructTxs(s, 3)

			pool.AddRemoteTxs(txs)
			ast.Equal(uint64(3), pool.GetTotalPendingTxCount())
			ast.Equal(uint64(3), pool.txStore.priorityNonBatchSize)
			ast.Equal(3, getPrioritySize(pool))

			tx4 := constructTx(s, 3)
			err = pool.AddLocalTx(tx4)
			ast.Nil(err)
			// trigger generate batch with GenBatchSizeEvent
			typ := <-ch
			ast.Equal(commonpool.GenBatchSizeEvent, typ)
			batch, err := pool.GenerateRequestBatch(typ)
			ast.NotNil(err)
			ast.Contains(err.Error(), "there is no valid tx to generate batch")
			ast.Nil(batch)
			ast.Equal(uint64(3), pool.GetTotalPendingTxCount(), "remove tx0, revert pendingNonce to 0")
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
			ast.Equal(0, getPrioritySize(pool))
			ast.Equal(uint64(0), pool.GetAccountMeta(from, false).PendingNonce, "remove tx0, revert pendingNonce")
		}

	})
}

func TestTxPoolImpl_ReConstructBatchByOrder(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		err := pool.Start()
		ast.Nil(err)
		defer pool.Stop()

		s, err := types.GenerateSigner()
		ast.Nil(err)

		txs := constructTxs(s, 4)
		txHashList := make([]string, len(txs))
		txsM := make(map[uint64]*types.Transaction)

		lo.ForEach(txs, func(tx *types.Transaction, index int) {
			txHashList[index] = tx.RbftGetTxHash()
			txsM[uint64(index)] = tx
		})
		newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
			TxList:     txs,
			LocalList:  []bool{true, true, true, true},
			TxHashList: txHashList,
			Timestamp:  time.Now().UnixNano(),
		}

		batchDigest := newBatch.GenerateBatchHash()
		newBatch.BatchHash = batchDigest

		pool.txStore.batchesCache[batchDigest] = newBatch

		duHash, err := pool.ReConstructBatchByOrder(newBatch)
		ast.NotNil(err)
		ast.Contains(err.Error(), "batch already exist")
		ast.Equal(0, len(duHash))

		illegalBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
			TxList:     txs,
			LocalList:  []bool{true, true, true, true},
			TxHashList: nil,
			Timestamp:  time.Now().UnixNano(),
		}

		_, err = pool.ReConstructBatchByOrder(illegalBatch)
		ast.NotNil(err)
		ast.Contains(err.Error(), "TxPointerList and TxList have different lengths")

		illegalTxHashList := make([]string, len(txs))
		lo.ForEach(txHashList, func(tx string, index int) {
			if index == 1 {
				illegalTxHashList[index] = "invalid"
			} else {
				illegalTxHashList[index] = tx
			}
		})
		illegalBatch.TxHashList = illegalTxHashList

		_, err = pool.ReConstructBatchByOrder(illegalBatch)
		ast.NotNil(err)
		ast.Contains(err.Error(), "hash of transaction does not match")

		illegalBatch.TxHashList = txHashList
		illegalBatch.BatchHash = "invalid"
		_, err = pool.ReConstructBatchByOrder(illegalBatch)
		ast.NotNil(err)
		ast.Contains(err.Error(), "batch hash does not match")

		pool.txStore.batchesCache = make(map[string]*commonpool.RequestHashBatch[types.Transaction, *types.Transaction])
		duTxs := txs[:2]
		pointer0 := &txPointer{
			account: duTxs[0].RbftGetFrom(),
			nonce:   duTxs[0].RbftGetNonce(),
		}
		pointer1 := &txPointer{
			account: duTxs[1].RbftGetFrom(),
			nonce:   duTxs[1].RbftGetNonce(),
		}
		pool.txStore.batchedTxs[*pointer0] = true
		pool.txStore.batchedTxs[*pointer1] = true

		dupTxHashes, err := pool.ReConstructBatchByOrder(newBatch)
		ast.Nil(err)
		ast.Equal(2, len(dupTxHashes))
		ast.Equal(duTxs[0].RbftGetTxHash(), dupTxHashes[0])
		ast.Equal(duTxs[1].RbftGetTxHash(), dupTxHashes[1])
	}
}

func TestTxPoolImpl_GetRequestsByHashList(t *testing.T) {
	t.Parallel()

	t.Run("exist batch or missingTxs", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)

			txs := constructTxs(s, 4)
			txHashList := make([]string, len(txs))
			txsM := make(map[uint64]*types.Transaction)

			lo.ForEach(txs, func(tx *types.Transaction, index int) {
				txHashList[index] = tx.RbftGetTxHash()
				txsM[uint64(index)] = tx
			})
			newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
				TxList:     txs,
				LocalList:  []bool{true, true, true, true},
				TxHashList: txHashList,
				Timestamp:  time.Now().UnixNano(),
			}

			batchDigest := newBatch.GenerateBatchHash()
			newBatch.BatchHash = batchDigest

			pool.txStore.batchesCache[batchDigest] = newBatch
			getTxs, localList, missingTxsHash, err := pool.GetRequestsByHashList(batchDigest, newBatch.Timestamp, newBatch.TxHashList, nil)
			ast.Nil(err)
			ast.Equal(4, len(getTxs))
			ast.Equal(4, len(localList))
			ast.Equal(0, len(missingTxsHash))

			pool.txStore.batchesCache = make(map[string]*commonpool.RequestHashBatch[types.Transaction, *types.Transaction])
			expectMissingTxsHash := make(map[uint64]string)
			expectMissingTxsHash[0] = txs[0].RbftGetTxHash()
			pool.txStore.missingBatch[batchDigest] = expectMissingTxsHash

			getTxs, localList, missingTxsHash, err = pool.GetRequestsByHashList(batchDigest, newBatch.Timestamp, newBatch.TxHashList, nil)
			ast.Nil(err)
			ast.Equal(0, len(getTxs))
			ast.Equal(0, len(localList))
			ast.Equal(1, len(missingTxsHash))
			ast.Equal(txs[0].RbftGetTxHash(), missingTxsHash[0])
		}
	})

	t.Run("doesn't exist tx in txpool", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)

			txs := constructTxs(s, 4)
			txHashList := make([]string, len(txs))
			txsM := make(map[uint64]*types.Transaction)

			lo.ForEach(txs, func(tx *types.Transaction, index int) {
				txHashList[index] = tx.RbftGetTxHash()
				txsM[uint64(index)] = tx
			})
			newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
				TxList:     txs,
				LocalList:  []bool{true, true, true, true},
				TxHashList: txHashList,
				Timestamp:  time.Now().UnixNano(),
			}

			batchDigest := newBatch.GenerateBatchHash()
			newBatch.BatchHash = batchDigest

			getTxs, localList, missingTxsHash, err := pool.GetRequestsByHashList(batchDigest, newBatch.Timestamp, newBatch.TxHashList, nil)
			ast.Nil(err)
			ast.Equal(0, len(getTxs))
			ast.Equal(0, len(localList))
			ast.Equal(4, len(missingTxsHash))
		}
	})

	t.Run("exist duplicate tx in txpool", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)

			txs := constructTxs(s, 4)
			txHashList := make([]string, len(txs))
			txsM := make(map[uint64]*types.Transaction)

			lo.ForEach(txs, func(tx *types.Transaction, index int) {
				txHashList[index] = tx.RbftGetTxHash()
				txsM[uint64(index)] = tx
			})
			newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
				TxList:     txs,
				LocalList:  []bool{true, true, true, true},
				TxHashList: txHashList,
				Timestamp:  time.Now().UnixNano(),
			}

			batchDigest := newBatch.GenerateBatchHash()
			newBatch.BatchHash = batchDigest

			pool.AddRemoteTxs(txs)
			dupTx := txs[0]
			pointer := &txPointer{
				account: dupTx.RbftGetFrom(),
				nonce:   dupTx.RbftGetNonce(),
			}
			pool.txStore.batchedTxs[*pointer] = true
			_, _, _, err = pool.GetRequestsByHashList(batchDigest, newBatch.Timestamp, newBatch.TxHashList, nil)
			ast.NotNil(err)
			ast.Contains(err.Error(), "duplicate transaction")

			ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize)
			getTxs, localList, missing, err := pool.GetRequestsByHashList(batchDigest, newBatch.Timestamp, newBatch.TxHashList, []string{dupTx.RbftGetTxHash()})
			ast.Nil(err)
			ast.Equal(0, len(missing))
			ast.Equal(4, len(getTxs))
			ast.Equal(4, len(localList))
			ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)

			lo.ForEach(localList, func(local bool, index int) {
				ast.False(local)
			})
		}
	})
}

func TestTxPoolImpl_SendMissingRequests(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		err := pool.Start()
		ast.Nil(err)
		defer pool.Stop()
		s, err := types.GenerateSigner()
		ast.Nil(err)

		txs := constructTxs(s, 4)
		txHashList := make([]string, len(txs))
		txsM := make(map[uint64]*types.Transaction)

		lo.ForEach(txs, func(tx *types.Transaction, index int) {
			txHashList[index] = tx.RbftGetTxHash()
			txsM[uint64(index)] = tx
		})
		newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
			TxList:     txs,
			LocalList:  []bool{true, true, true, true},
			TxHashList: txHashList,
			Timestamp:  time.Now().UnixNano(),
		}

		batchDigest := newBatch.GenerateBatchHash()
		newBatch.BatchHash = batchDigest

		missHashList := make(map[uint64]string)
		missHashList[uint64(0)] = txHashList[0]
		_, err = pool.SendMissingRequests(batchDigest, missHashList)
		ast.NotNil(err)
		ast.Contains(err.Error(), "doesn't exist in txHashMap")

		pool.AddRemoteTxs(txs)
		_, err = pool.SendMissingRequests(batchDigest, missHashList)
		ast.NotNil(err)
		ast.Contains(err.Error(), "doesn't exist in batchedCache")

		pool.txStore.batchesCache[batchDigest] = newBatch

		illegalMissHashList := make(map[uint64]string)
		illegalMissHashList[uint64(4)] = txHashList[0]

		_, err = pool.SendMissingRequests(batchDigest, illegalMissHashList)
		ast.NotNil(err)
		ast.Contains(err.Error(), "find invalid transaction")

		getTxs, err := pool.SendMissingRequests(batchDigest, missHashList)
		ast.Nil(err)
		ast.Equal(1, len(getTxs))
		ast.Equal(txs[0].RbftGetTxHash(), getTxs[0].RbftGetTxHash())
	}
}

func TestTxPoolImpl_FilterOutOfDateRequests(t *testing.T) {
	t.Parallel()
	t.Run("filter out of date requests with timeout", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			pool.toleranceTime = 1 * time.Millisecond
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			tx0 := constructTx(s, 0)
			tx1 := constructTx(s, 1)
			tx3 := constructTx(s, 3) // lack tx2, so tx3 is not ready
			txs := []*types.Transaction{tx0, tx1, tx3}
			lo.ForEach(txs, func(tx *types.Transaction, _ int) {
				err = pool.AddLocalTx(tx)
				ast.Nil(err)
			})
			ast.Equal(uint64(3), pool.GetTotalPendingTxCount())
			ast.Equal(uint64(2), pool.txStore.priorityNonBatchSize)
			ast.Equal(2, getPrioritySize(pool))
			ast.Equal(1, pool.txStore.parkingLotIndex.size())
			poolTx := pool.txStore.getPoolTxByTxnPointer(from, 0)
			ast.NotNil(poolTx)
			firstTime := poolTx.lifeTime

			// trigger update lifeTime
			time.Sleep(2 * time.Millisecond)
			rebroadcastTxs := pool.FilterOutOfDateRequests(true)
			ast.Equal(2, len(rebroadcastTxs), "rebroadcast tx1, tx2")
			ast.Equal(tx0.RbftGetTxHash(), rebroadcastTxs[0].RbftGetTxHash())
			ast.Equal(tx1.RbftGetTxHash(), rebroadcastTxs[1].RbftGetTxHash())
			ast.True(poolTx.lifeTime > firstTime)
		}
	})

	t.Run("filter out of date requests without timeout", func(t *testing.T) {
		ast := assert.New(t)
		testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
			"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
			"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
		}

		for _, tc := range testcase {
			pool := tc
			// ensure that not triggering timeout
			pool.toleranceTime = 100 * time.Second
			err := pool.Start()
			ast.Nil(err)
			defer pool.Stop()

			s, err := types.GenerateSigner()
			ast.Nil(err)
			from := s.Addr.String()
			tx0 := constructTx(s, 0)
			err = pool.AddLocalTx(tx0)
			ast.Nil(err)
			poolTx := pool.txStore.getPoolTxByTxnPointer(from, 0)
			ast.NotNil(poolTx)
			firstTime := poolTx.lifeTime

			txs := pool.FilterOutOfDateRequests(true)
			ast.Equal(0, len(txs))

			txs = pool.FilterOutOfDateRequests(false)
			ast.Equal(1, len(txs))
			ast.True(poolTx.lifeTime > firstTime)
		}
	})
}

func TestTxPoolImpl_RestoreOneBatch(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		pool.chainInfo.EpochConf.BatchSize = 100
		err := pool.Start()
		ast.Nil(err)
		defer pool.Stop()

		err = pool.RestoreOneBatch("wrong batch")
		ast.NotNil(err)
		ast.Contains(err.Error(), "can't find batch from batchesCache")

		s, err := types.GenerateSigner()
		ast.Nil(err)
		from := s.Addr.String()
		txs := constructTxs(s, 4)
		txHashList := make([]string, len(txs))
		txsM := make(map[uint64]*types.Transaction)

		lo.ForEach(txs, func(tx *types.Transaction, index int) {
			txHashList[index] = tx.RbftGetTxHash()
			txsM[uint64(index)] = tx
		})
		newBatch := &commonpool.RequestHashBatch[types.Transaction, *types.Transaction]{
			TxList:     txs,
			LocalList:  []bool{true, true, true, true},
			TxHashList: txHashList,
			Timestamp:  time.Now().UnixNano(),
		}

		batchDigest := newBatch.GenerateBatchHash()
		newBatch.BatchHash = batchDigest

		pool.txStore.batchesCache[batchDigest] = newBatch

		err = pool.RestoreOneBatch(batchDigest)
		ast.NotNil(err)
		ast.Contains(err.Error(), "can't find tx from txHashMap")

		pool.AddRemoteTxs(txs)
		ast.NotNil(pool.GetPendingTxByHash(txHashList[0]))

		err = pool.RestoreOneBatch(batchDigest)
		ast.NotNil(err)
		ast.Contains(err.Error(), "can't find tx from batchedTxs")

		ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize)
		batch, err := pool.GenerateRequestBatch(commonpool.GenBatchTimeoutEvent)
		ast.Nil(err)
		ast.NotNil(batch)
		ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)

		if !pool.enablePricePriority {
			removeTx := pool.txStore.getPoolTxByTxnPointer(from, 0)
			pool.txStore.priorityByTime.removeKey(removeTx)
			err = pool.RestoreOneBatch(batchDigest)
			ast.NotNil(err)
			ast.Contains(err.Error(), "can't find tx from priorityByTime")
			pool.txStore.priorityByTime.insertKey(removeTx)
		} else {
			err = pool.RestoreOneBatch(batch.BatchHash)
			ast.Nil(err)
			ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize)
			ast.Equal(pool.txStore.priorityByPrice.accountsM[from].lastNonce, uint64(3))
			ast.Equal(pool.txStore.priorityByPrice.size(), uint64(4))
		}

	}
}

func TestTxPoolImpl_RestorePool(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		err := pool.Start()
		ast.Nil(err)
		defer pool.Stop()

		s, err := types.GenerateSigner()
		ast.Nil(err)
		txs := constructTxs(s, 3)
		pool.AddRemoteTxs(txs)
		tx3 := constructTx(s, 3)
		err = pool.AddLocalTx(tx3)
		ast.Nil(err)

		batch, err := pool.GenerateRequestBatch(commonpool.GenBatchTimeoutEvent)
		ast.Nil(err)
		ast.Equal(4, len(batch.TxList))
		ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
		ast.Equal(4, len(pool.txStore.batchedTxs))
		ast.Equal(1, len(pool.txStore.batchesCache))
		ast.Equal(uint64(4), pool.txStore.nonceCache.pendingNonces[tx3.RbftGetFrom()])
		ast.Equal(batch.Timestamp, pool.txStore.batchesCache[batch.BatchHash].Timestamp)

		pool.RestorePool()
		ast.NotNil(pool.GetPendingTxByHash(tx3.RbftGetTxHash()))
		ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize)
		ast.Equal(0, len(pool.txStore.batchedTxs))
		ast.Equal(0, len(pool.txStore.batchesCache))
		ast.Equal(uint64(4), pool.txStore.nonceCache.pendingNonces[tx3.RbftGetFrom()])
	}
}

func TestTxPoolImpl_GetInfo(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		err := pool.Start()
		ast.Nil(err)
		defer pool.Stop()

		s, err := types.GenerateSigner()
		ast.Nil(err)
		from := s.Addr.String()

		txs := constructTxs(s, 4)
		pool.AddRemoteTxs(txs)
		t.Run("getmeta", func(t *testing.T) {
			meta := pool.GetMeta(false)
			ast.NotNil(meta)
			ast.Equal(1, len(meta.Accounts))
			ast.NotNil(meta.Accounts[from])
			ast.Equal(uint64(4), meta.Accounts[from].PendingNonce)
			ast.Equal(uint64(0), meta.Accounts[from].CommitNonce)
			ast.Equal(uint64(4), meta.Accounts[from].TxCount)
			ast.Equal(4, len(meta.Accounts[from].SimpleTxs))
			ast.Equal(0, len(meta.Accounts[from].Txs))

			meta = pool.GetMeta(true)
			ast.Equal(0, len(meta.Accounts[from].SimpleTxs))
			ast.Equal(4, len(meta.Accounts[from].Txs))
		})

		t.Run("getAccountMeta", func(t *testing.T) {
			emptyS, sErr := types.GenerateSigner()
			ast.Nil(sErr)
			emptyFrom := emptyS.Addr.String()
			accountMeta := pool.GetAccountMeta(emptyFrom, false)
			ast.NotNil(accountMeta)
			ast.Equal(uint64(0), accountMeta.PendingNonce)
			ast.Equal(uint64(0), accountMeta.CommitNonce)
			ast.Equal(uint64(0), accountMeta.TxCount)
			ast.Equal(0, len(accountMeta.SimpleTxs))
			ast.Equal(0, len(accountMeta.Txs))

			accountMeta = pool.GetAccountMeta(from, false)
			ast.NotNil(accountMeta)
			ast.Equal(uint64(4), accountMeta.PendingNonce)
			ast.Equal(uint64(0), accountMeta.CommitNonce)
			ast.Equal(uint64(4), accountMeta.TxCount)
			ast.Equal(4, len(accountMeta.SimpleTxs))
			ast.Equal(0, len(accountMeta.Txs))

			accountMeta = pool.GetAccountMeta(from, true)
			ast.NotNil(accountMeta)
			ast.Equal(0, len(accountMeta.SimpleTxs))
			ast.Equal(4, len(accountMeta.Txs))
		})

		t.Run("getTx", func(t *testing.T) {
			tx := pool.GetPendingTxByHash(txs[1].RbftGetTxHash())
			ast.Equal(txs[1].RbftGetTxHash(), tx.RbftGetTxHash())
		})

		t.Run("getNonce", func(t *testing.T) {
			nonce := pool.GetPendingTxCountByAccount(from)
			ast.Equal(uint64(4), nonce)
		})

		t.Run("get total tx count", func(t *testing.T) {
			count := pool.GetTotalPendingTxCount()
			ast.Equal(uint64(4), count)
		})

		t.Run("get chainInfo", func(t *testing.T) {
			info := pool.GetChainInfo()
			ast.Equal(pool.chainInfo.Height, info.Height)
			ast.Equal(pool.chainInfo.GasPrice.String(), info.GasPrice.String())

			newChainInfo := &commonpool.ChainInfo{
				Height:   pool.chainInfo.Height + 1,
				GasPrice: new(big.Int).Mul(pool.chainInfo.GasPrice, big.NewInt(2)),
			}
			pool.UpdateChainInfo(newChainInfo)
			info = pool.GetChainInfo()
			ast.Equal(newChainInfo.Height, info.Height)
			ast.Equal(newChainInfo.GasPrice.String(), info.GasPrice.String())
		})
	}

}

func TestTxPoolImpl_RemoveBatches(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		pool.chainInfo.EpochConf.BatchSize = 4
		err := pool.Start()
		defer pool.Stop()
		ast.Nil(err)

		pool.RemoveBatches([]string{"wrong batch"})

		s, err := types.GenerateSigner()
		ast.Nil(err)
		from := s.Addr.String()
		txs := constructTxs(s, 4)
		pool.AddRemoteTxs(txs)
		batch, err := pool.GenerateRequestBatch(commonpool.GenBatchTimeoutEvent)
		ast.Nil(err)
		ast.Equal(4, len(pool.txStore.txHashMap))
		ast.Equal(4, len(batch.TxList))
		ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
		ast.Equal(4, len(pool.txStore.batchedTxs))
		ast.NotNil(pool.txStore.batchesCache[batch.BatchHash])
		ast.Equal(uint64(4), pool.txStore.nonceCache.pendingNonces[from])
		ast.Equal(uint64(0), pool.txStore.nonceCache.commitNonces[from])

		pool.RemoveBatches([]string{batch.BatchHash})
		ast.Equal(uint64(4), pool.GetPendingTxCountByAccount(from))
		ast.Equal(0, len(pool.txStore.txHashMap))
		ast.Equal(uint64(4), pool.txStore.nonceCache.commitNonces[from])
	}
}

func TestTxPoolImpl_RemoveStateUpdatingTxs(t *testing.T) {
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		pool.chainInfo.EpochConf.BatchSize = 100
		err := pool.Start()
		defer pool.Stop()
		ast.Nil(err)

		s, err := types.GenerateSigner()
		ast.Nil(err)
		from := s.Addr.String()
		txs := constructTxs(s, 4)
		pool.AddRemoteTxs(txs)
		ast.Equal(uint64(4), pool.GetPendingTxCountByAccount(from))
		ast.Equal(4, len(pool.txStore.txHashMap))
		ast.Equal(uint64(4), pool.txStore.priorityNonBatchSize)
		ast.Equal(uint64(0), pool.txStore.nonceCache.commitNonces[from])

		txPointerList := make([]*commonpool.WrapperTxPointer, len(txs))
		lo.ForEach(txs, func(tx *types.Transaction, index int) {
			txPointerList[index] = &commonpool.WrapperTxPointer{
				TxHash:  tx.RbftGetTxHash(),
				Account: tx.RbftGetFrom(),
				Nonce:   tx.RbftGetNonce(),
			}
		})
		pool.RemoveStateUpdatingTxs(txPointerList)
		ast.Equal(uint64(4), pool.GetPendingTxCountByAccount(from))
		ast.Equal(0, len(pool.txStore.txHashMap))
		ast.Equal(uint64(0), pool.txStore.priorityNonBatchSize)
		ast.Equal(uint64(4), pool.txStore.nonceCache.commitNonces[from])

		// lack nonce
		s2, err := types.GenerateSigner()
		ast.Nil(err)
		from2 := s2.Addr.String()
		txs2 := constructTxs(s2, 4)
		// lack nonce 0
		lackTxs := txs2[1:]
		pool.AddRemoteTxs(lackTxs)
		ast.Equal(uint64(0), pool.GetPendingTxCountByAccount(from2))
		txPointerList = make([]*commonpool.WrapperTxPointer, len(txs2))
		lo.ForEach(txs2, func(tx *types.Transaction, index int) {
			txPointerList[index] = &commonpool.WrapperTxPointer{
				TxHash:  tx.RbftGetTxHash(),
				Account: tx.RbftGetFrom(),
				Nonce:   tx.RbftGetNonce(),
			}
		})
		pool.RemoveStateUpdatingTxs(txPointerList)
		ast.Equal(uint64(4), pool.GetPendingTxCountByAccount(from2))
		ast.Equal(0, len(pool.txStore.txHashMap))
		ast.Equal(uint64(4), pool.txStore.nonceCache.commitNonces[from2])
	}
}

func TestTxPoolImpl_GetLocalTxs(t *testing.T) {
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	tx := constructTx(s, 0)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		err = pool.Start()
		assert.Nil(t, err)
		defer pool.Stop()
		_, err = pool.addTx(tx, true)
		localTxs := pool.GetLocalTxs()
		tx2 := &types.Transaction{}
		err = tx2.RbftUnmarshal(localTxs[0])
		assert.Nil(t, err)
		assert.Equal(t, tx.RbftGetTxHash(), tx2.RbftGetTxHash())
		assert.Equal(t, tx.RbftGetNonce(), tx2.RbftGetNonce())
		assert.Equal(t, tx.RbftGetFrom(), tx2.RbftGetFrom())
	}
}

func TestTxPoolImpl_UpdateChainInfo(t *testing.T) {
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}
	for _, tc := range testcase {
		pool := tc
		oldChainInfo := pool.chainInfo.Clone()
		assert.NotNil(t, oldChainInfo)
		err := pool.Start()
		assert.Nil(t, err)
		defer pool.Stop()

		newChainInfo := &commonpool.ChainInfo{
			Height:   oldChainInfo.Height + 1,
			GasPrice: big.NewInt(100),
		}
		pool.UpdateChainInfo(newChainInfo)
		// post get pool info event to ensure chain info is updated
		_ = pool.GetMeta(false)
		newChainInfo = pool.chainInfo
		assert.NotNil(t, newChainInfo)
		assert.Equal(t, oldChainInfo.Height+1, newChainInfo.Height)
		assert.Equal(t, big.NewInt(100), newChainInfo.GasPrice)
	}

}

// nolint
func TestTPSWithLocalTx(t *testing.T) {
	t.Skip()
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		pool.logger.(*logrus.Entry).Logger.SetLevel(logrus.ErrorLevel)
		pool.chainInfo.EpochConf.BatchSize = 500

		batchesCache := make(chan *commonpool.RequestHashBatch[types.Transaction, *types.Transaction], 10240)
		pool.notifyGenerateBatchFn = func(typ int) {
			go func() {
				batch, err := pool.GenerateRequestBatch(typ)
				ast.Nil(err)
				ast.NotNil(batch)
				batchesCache <- batch
			}()
		}
		err := pool.Start()
		defer pool.Stop()
		ast.Nil(err)

		round := 5000
		thread := 1
		total := round * thread

		endCh := make(chan int64, 1)
		go listenBatchCache(total, endCh, batchesCache, pool, t)

		wg := new(sync.WaitGroup)
		start := time.Now().UnixNano()
		for i := 0; i < thread; i++ {
			wg.Add(1)
			s, err := types.GenerateSigner()
			ast.Nil(err)
			go func(i int, wg *sync.WaitGroup, s *types.Signer) {
				defer wg.Done()
				for j := 0; j < round; j++ {
					tx := constructTx(s, uint64(j))
					err = pool.AddLocalTx(tx)
					ast.Nil(err)
				}
			}(i, wg, s)
		}
		wg.Wait()

		end := <-endCh
		dur := end - start
		fmt.Printf("=========================duration: %v\n", time.Duration(dur).Seconds())
		fmt.Printf("=========================tps: %v\n", float64(total)/time.Duration(dur).Seconds())
	}
}

// nolint
func TestTPSWithRemoteTxs(t *testing.T) {
	t.Skip()
	ast := assert.New(t)
	testcase := map[string]*txPoolImpl[types.Transaction, *types.Transaction]{
		"price_priority": mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByGasPrice),
		"time":           mockTxPoolImplWithTyp[types.Transaction, *types.Transaction](t, repo.GenerateBatchByTime),
	}

	for _, tc := range testcase {
		pool := tc
		// ignore log
		pool.logger.(*logrus.Entry).Logger.SetLevel(logrus.ErrorLevel)

		pool.chainInfo.EpochConf.BatchSize = 500
		pool.toleranceNonceGap = 100000
		round := 10000
		thread := 10
		total := round * thread
		pool.poolMaxSize = uint64(total)

		batchesCache := make(chan *commonpool.RequestHashBatch[types.Transaction, *types.Transaction], 10240)
		pool.notifyGenerateBatchFn = func(typ int) {
			go func() {
				batch, err := pool.GenerateRequestBatch(typ)
				ast.Nil(err)
				ast.NotNil(batch)
				batchesCache <- batch
			}()
		}
		err := pool.Start()
		defer pool.Stop()
		ast.Nil(err)

		endCh := make(chan int64, 1)
		txC := &txCache{
			RecvTxC: make(chan *types.Transaction, 10240),
			TxSetC:  make(chan []*types.Transaction, 10240),
			closeC:  make(chan struct{}),
			txSet:   make([]*types.Transaction, 0),
		}
		go listenBatchCache(total, endCh, batchesCache, pool, t)
		go txC.listenTxCache()
		// !!!NOTICE!!! if modify thread, need to modify postTxs' sleep timeout
		go txC.listenPostTxs(pool)

		start := time.Now().UnixNano()
		go prepareTx(thread, round, txC, t)

		end := <-endCh
		dur := end - start
		fmt.Printf("=========================duration: %v\n", time.Duration(dur).Seconds())
		fmt.Printf("=========================tps: %v\n", float64(total)/time.Duration(dur).Seconds())
	}
}

func prepareTx(thread, round int, tc *txCache, t *testing.T) {
	wg := new(sync.WaitGroup)
	for i := 0; i < thread; i++ {
		wg.Add(1)
		s, err := types.GenerateSigner()
		require.Nil(t, err)
		go func(i int, wg *sync.WaitGroup, s *types.Signer) {
			defer wg.Done()
			for j := 0; j < round; j++ {
				tx := constructTx(s, uint64(j))
				tc.RecvTxC <- tx
			}
		}(i, wg, s)
	}
	wg.Wait()
}

type txCache struct {
	TxSetC  chan []*types.Transaction
	RecvTxC chan *types.Transaction
	closeC  chan struct{}
	txSet   []*types.Transaction
}

func (tc *txCache) listenTxCache() {
	for {
		select {
		case <-tc.closeC:
			return
		case tx := <-tc.RecvTxC:
			tc.txSet = append(tc.txSet, tx)
			if uint64(len(tc.txSet)) >= 50 {
				dst := make([]*types.Transaction, len(tc.txSet))
				copy(dst, tc.txSet)
				tc.TxSetC <- dst
				tc.txSet = make([]*types.Transaction, 0)
			}
		}
	}
}

func (tc *txCache) listenPostTxs(pool *txPoolImpl[types.Transaction, *types.Transaction]) {
	for {
		select {
		case <-tc.closeC:
			return
		case txs := <-tc.TxSetC:
			pool.AddRemoteTxs(txs)
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func listenBatchCache(total int, endCh chan int64, cacheCh chan *commonpool.RequestHashBatch[types.Transaction, *types.Transaction],
	pool *txPoolImpl[types.Transaction, *types.Transaction], t *testing.T) {
	seqNo := uint64(0)
	for {
		select {
		case batch := <-cacheCh:
			now := time.Now()
			txList, localList, missingTxs, err := pool.GetRequestsByHashList(batch.BatchHash, batch.Timestamp, batch.TxHashList, nil)
			require.Nil(t, err)
			require.Equal(t, len(batch.TxHashList), len(txList))
			require.Equal(t, len(batch.TxHashList), len(localList))
			require.Equal(t, 0, len(missingTxs))

			newBatch := &rbft.RequestBatch[types.Transaction, *types.Transaction]{
				RequestHashList: batch.TxHashList,
				RequestList:     txList,
				Timestamp:       batch.Timestamp,
				SeqNo:           atomic.AddUint64(&seqNo, 1),
				LocalList:       localList,
				BatchHash:       batch.BatchHash,
				Proposer:        1,
			}
			fmt.Printf("GetRequestsByHashList: %d, cost: %v\n", newBatch.SeqNo, time.Since(now))

			pool.RemoveBatches([]string{newBatch.BatchHash})
			if newBatch.SeqNo >= uint64(total)/pool.chainInfo.EpochConf.BatchSize {
				end := newBatch.Timestamp
				endCh <- end
			}
			fmt.Printf("remove batch: %d, cost: %v\n", newBatch.SeqNo, time.Since(now))
		}
	}
}
