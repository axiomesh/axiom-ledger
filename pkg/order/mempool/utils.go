package mempool

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/meshplus/bitxhub-model/pb"
	raftproto "github.com/meshplus/bitxhub/pkg/order/etcdraft/proto"

	cmap "github.com/orcaman/concurrent-map"
)

func (mpi *mempoolImpl) getBatchSeqNo() uint64 {
	return atomic.LoadUint64(&mpi.batchSeqNo)
}

func (mpi *mempoolImpl) increaseBatchSeqNo() {
	atomic.AddUint64(&mpi.batchSeqNo, 1)
}

func (mpi *mempoolImpl) msgToConsensusPbMsg(data []byte, tyr raftproto.RaftMessage_Type) *pb.Message {
	rm := &raftproto.RaftMessage{
		Type:   tyr,
		FromId: mpi.localID,
		Data:   data,
	}
	cmData, err := rm.Marshal()
	if err != nil {
		return nil
	}
	msg := &pb.Message{
		Type: pb.Message_CONSENSUS,
		Data: cmData,
	}
	return msg
}

func newSubscribe() *subscribeEvent {
	return &subscribeEvent{
		txForwardC:           make(chan *TxSlice),
		localMissingTxnEvent: make(chan *LocalMissingTxnEvent),
		fetchTxnRequestC:     make(chan *FetchTxnRequest),
		updateLeaderC:        make(chan uint64),
		fetchTxnResponseC:    make(chan *FetchTxnResponse),
		commitTxnC:           make(chan *raftproto.Ready),
		getBlockC:            make(chan *constructBatchEvent),
		pendingNonceC:        make(chan *getNonceRequest),
	}
}

// TODO (YH): restore commitNonce and pendingNonce from db.
func newNonceCache() *nonceCache {
	return &nonceCache{
		commitNonces:  make(map[string]uint64),
		pendingNonces: make(map[string]uint64),
	}
}

func (mpi *mempoolImpl) poolIsFull() bool {
	return atomic.LoadInt32(&mpi.txStore.poolSize) >= DefaultPoolSize
}

func (mpi *mempoolImpl) isLeader() bool {
	return mpi.leader == mpi.localID
}

func (mpi *mempoolImpl) isBatchTimerActive() bool {
	return !mpi.batchTimerMgr.isActive.IsEmpty()
}

// startBatchTimer starts the batch timer and reset the batchTimerActive to true.
func (mpi *mempoolImpl) startBatchTimer(reason string) {
	// stop old timer
	mpi.stopBatchTimer(StopReason3)
	mpi.logger.Debugf("Start batch timer, reason: %s", reason)
	timestamp := time.Now().UnixNano()
	key := strconv.FormatInt(timestamp, 10)
	mpi.batchTimerMgr.isActive.Set(key, true)

	time.AfterFunc(mpi.batchTimerMgr.timeout, func() {
		if mpi.batchTimerMgr.isActive.Has(key) {
			mpi.batchTimerMgr.timeoutEventC <- true
		}
	})
}

// stopBatchTimer stops the batch timer and reset the batchTimerActive to false.
func (mpi *mempoolImpl) stopBatchTimer(reason string) {
	if mpi.batchTimerMgr.isActive.IsEmpty() {
		return
	}
	mpi.logger.Debugf("Stop batch timer, reason: %s", reason)
	mpi.batchTimerMgr.isActive = cmap.New()
}

// newTimer news a timer with default timeout.
func newTimer(d time.Duration) *timerManager {
	return &timerManager{
		timeout:       d,
		isActive:      cmap.New(),
		timeoutEventC: make(chan bool),
	}
}

func getAccount(tx *pb.Transaction) (string, error) {
	if tx.IBTP != nil {
		account := fmt.Sprintf("%s-%s-%d", tx.IBTP.From, tx.IBTP.To, tx.IBTP.Category())
		return account, nil
	}
	return tx.From.String(), nil
}
