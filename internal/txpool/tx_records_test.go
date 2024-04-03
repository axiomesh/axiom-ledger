package txpool

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/axiomesh/axiom-kit/types"
)

func TestTxRecords(t *testing.T) {
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	pool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)
	assert.Nil(t, err)
	records := pool.txRecords
	defer func() {
		cleanupErr := os.Remove(records.filePath)
		if cleanupErr != nil {
			t.Errorf("Failed to clean up file: %v", cleanupErr)
		}
	}()

	// test rotate
	err = records.rotate(pool.txStore.allTxs)
	assert.Nil(t, err)

	// test insert
	tx := constructTx(s, 0)
	err = records.insert(tx)
	assert.Nil(t, err)

	// test get
	all, err := GetAllTxRecords(records.filePath)
	assert.Nil(t, err)
	marshal, err := tx.RbftMarshal()
	assert.Nil(t, err)
	assert.True(t, bytes.Equal(all[0], marshal))

	tx2 := &types.Transaction{}
	err = tx2.RbftUnmarshal(all[0])
	assert.Nil(t, err)
	assert.Equal(t, tx.RbftGetTxHash(), tx2.RbftGetTxHash())

	// test load
	input, err := os.Open(records.filePath)
	assert.Nil(t, err)
	defer func() {
		err = input.Close()
		assert.Nil(t, err)
	}()
	taskDoneCh := make(chan struct{}, 1)
	batchCh := records.load(input, taskDoneCh)
	batch := <-batchCh
	assert.Equal(t, 1, len(batch))
	assert.Equal(t, tx.RbftGetTxHash(), batch[0].RbftGetTxHash())
	<-taskDoneCh
}

func TestDevNull(t *testing.T) {
	devNull := devNull{}
	n, err := devNull.Write([]byte("test"))
	assert.Nil(t, err)
	assert.Equal(t, len("test"), n)

	err = devNull.Close()
	assert.Nil(t, err)
}

func TestTxRecords_load(t *testing.T) {
	// test not exist file
	pool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)
	records := pool.txRecords

	taskDoneCh := make(chan struct{}, 1)

	records.load(nil, taskDoneCh)
	<-taskDoneCh

	// test wrong read
	err := pool.Start()
	assert.Nil(t, err)
	defer pool.Stop()

	_, err = records.writer.Write([]byte{1, 0, 0, 0, 0, 0, 0, 0})
	assert.Nil(t, err)
	input, err := os.Open(records.filePath)
	assert.Nil(t, err)

	records.load(input, taskDoneCh)
	<-taskDoneCh
	err = input.Close()
	assert.Nil(t, err)

	// test wrong tx
	err = records.rotate(nil) // reset pb file
	assert.Nil(t, err)
	_, err = records.writer.Write([]byte{1, 0, 0, 0, 0, 0, 0, 0, 1})
	assert.Nil(t, err)
	input, err = os.Open(records.filePath)
	assert.Nil(t, err)
	records.load(input, taskDoneCh)
	<-taskDoneCh
	err = input.Close()
	assert.Nil(t, err)

	// test 1000+ txs load
	err = records.rotate(nil) // reset pb file
	assert.Nil(t, err)
	s, err := types.GenerateSigner()
	txs := constructTxs(s, TxRecordsBatchSize+1)
	for _, tx := range txs {
		err = records.insert(tx)
		assert.Nil(t, err)
	}
	input, err = os.Open(records.filePath)
	assert.Nil(t, err)
	batchCh := records.load(input, taskDoneCh)
	batch := <-batchCh
	assert.Equal(t, TxRecordsBatchSize, len(batch))
	batch = <-batchCh
	assert.Equal(t, 1, len(batch))
	assert.True(t, batch[0].RbftGetTimeStamp() > txs[len(txs)-1].RbftGetTimeStamp(), "load tx will reset tx time")
	<-taskDoneCh
	err = input.Close()
	assert.Nil(t, err)
}

func TestTxRecords_LoadMoreThanOneBatch(t *testing.T) {
	// init pool and txs
	pool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)
	err := pool.Start() // used for init txrecord
	assert.Nil(t, err)
	records := pool.txRecords
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	txs := constructTxs(s, TxRecordsBatchSize+1)
	for _, tx := range txs {
		err = records.insert(tx)
		assert.Nil(t, err)
	}

	// restart pool
	pool.Stop()
	pool.ctx, pool.cancel = context.WithCancel(context.Background())

	err = pool.Start() // used for test txrecord load
	assert.Nil(t, err)

	defer pool.Stop()

	meta := pool.GetMeta(false)
	assert.Equal(t, uint64(TxRecordsBatchSize+1), meta.TxCount)
	assert.Equal(t, TxRecordsBatchSize+1, pool.txStore.localTTLIndex.size(), "all tx should be in localTTLIndex")
	assert.Equal(t, TxRecordsBatchSize+1, getPrioritySize(pool))
	assert.Equal(t, uint64(TxRecordsBatchSize+1), pool.txStore.priorityNonBatchSize)
}

func TestTxRecords_Rotate(t *testing.T) {
	// init pool and txs
	pool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)
	_ = pool.Start()
	defer pool.Stop()
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	tx1 := constructTx(s, 0)
	_, err = pool.addTx(tx1, true)
	assert.Nil(t, err)
	tx2 := constructTx(s, 1)
	_, err = pool.addTx(tx2, true)
	s2, err := types.GenerateSigner()
	assert.Nil(t, err)
	tx3 := constructTx(s2, 0)
	_, err = pool.addTx(tx3, false)

	// test rotate for locals and unlocals
	txs := pool.txStore.allTxs
	err = pool.txRecords.rotate(txs)
	assert.Nil(t, err)
}

func TestTxRecords_RotateMoreThanOneBatch(t *testing.T) {
	// init pool and txs
	pool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)
	err := pool.Start()
	assert.Nil(t, err)
	defer pool.Stop()
	s, err := types.GenerateSigner()
	txs := constructTxs(s, TxRecordsBatchSize+1)
	for _, tx := range txs {
		_, err = pool.addTx(tx, true)
		assert.Nil(t, err)
	}

	err = os.Remove(pool.txRecords.filePath)
	err = pool.txRecords.rotate(pool.txStore.allTxs)
	records, err := GetAllTxRecords(pool.txRecords.filePath)
	assert.Nil(t, err)
	assert.True(t, len(records) == TxRecordsBatchSize+1)
}

func TestTxRecords_Insert(t *testing.T) {
	// init pool and txs
	pool := mockTxPoolImpl[types.Transaction, *types.Transaction](t)
	records := pool.txRecords

	// test insert no writer
	s, err := types.GenerateSigner()
	assert.Nil(t, err)
	tx1 := constructTx(s, 0)
	err = records.insert(tx1)
	assert.NotNil(t, err) // if not start pool first, records writer is null
}
