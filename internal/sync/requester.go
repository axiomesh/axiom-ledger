package sync

import (
	"context"
	"time"

	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
	network "github.com/axiomesh/axiom-p2p"
)

type requester struct {
	mode        common.SyncMode
	peerID      string
	blockHeight uint64

	quitCh          chan struct{}
	retryCh         chan string             // retry peerID
	invalidCh       chan *common.LocalEvent // a channel which send invalid msg to syncer
	gotBlock        bool
	gotCommitDataCh chan struct{}

	commitDataRequestPipe network.Pipe
	commitData            common.CommitData
	inRetryStatus         bool

	ctx context.Context
}

func newRequester(mode common.SyncMode, ctx context.Context, peerID string, height uint64, invalidCh chan *common.LocalEvent, pipe network.Pipe) *requester {
	return &requester{
		mode:                  mode,
		peerID:                peerID,
		blockHeight:           height,
		invalidCh:             invalidCh,
		quitCh:                make(chan struct{}, 1),
		retryCh:               make(chan string, 1),
		gotCommitDataCh:       make(chan struct{}, 1),
		commitDataRequestPipe: pipe,
		inRetryStatus:         false,
		ctx:                   ctx,
	}
}

func (r *requester) start(requestRetryTimeout time.Duration) {
	go r.requestRoutine(requestRetryTimeout)
}

// Responsible for making more requests as necessary
// Returns only when a commitData is found (e.g. AddBlock() is called)
func (r *requester) requestRoutine(requestRetryTimeout time.Duration) {
OUTER_LOOP:
	for {
		r.gotBlock = false
		r.inRetryStatus = false
		ticker := time.NewTicker(requestRetryTimeout)
		// Send request and wait

		var (
			data []byte
			err  error
		)
		switch r.mode {
		case common.SyncModeFull:
			req := &pb.SyncBlockRequest{Height: r.blockHeight}
			data, err = req.MarshalVT()
		case common.SyncModeSnapshot:
			req := &pb.SyncChainDataRequest{Height: r.blockHeight}
			data, err = req.MarshalVT()
		}

		if err != nil {
			r.postInvalidMsg(&common.InvalidMsg{NodeID: r.peerID, Height: r.blockHeight, ErrMsg: err, Typ: common.SyncMsgType_ErrorMsg})
			r.inRetryStatus = true
			goto WAIT_LOOP
		}
		if err = r.commitDataRequestPipe.Send(r.ctx, r.peerID, data); err != nil {
			r.postInvalidMsg(&common.InvalidMsg{NodeID: r.peerID, Height: r.blockHeight, ErrMsg: err, Typ: common.SyncMsgType_ErrorMsg})
			r.inRetryStatus = true
		}

	WAIT_LOOP:
		for {
			select {
			case <-r.quitCh:
				ticker.Stop()
				return
			case <-r.ctx.Done():
				return
			case newPeer := <-r.retryCh:
				// Retry the new peer
				r.peerID = newPeer
				continue OUTER_LOOP
			case <-ticker.C:
				// ensure last retry signal has been processed
				if r.gotBlock || r.inRetryStatus {
					continue WAIT_LOOP
				}
				// Timeout
				r.postInvalidMsg(&common.InvalidMsg{NodeID: r.peerID, Height: r.blockHeight, Typ: common.SyncMsgType_TimeoutBlock})
				r.inRetryStatus = true
			case <-r.gotCommitDataCh:
				// We got a commitData!
				// Continue the for-loop and wait til Quit.
				r.gotBlock = true
				continue WAIT_LOOP
			}
		}
	}
}

func (r *requester) setCommitData(data common.CommitData) {
	r.commitData = data

	select {
	case r.gotCommitDataCh <- struct{}{}:
	default:
	}
}

func (r *requester) clearBlock() {
	r.commitData = nil
}

func (r *requester) postInvalidMsg(msg *common.InvalidMsg) {
	ev := &common.LocalEvent{
		EventType: common.EventType_InvalidMsg,
		Event:     msg,
	}
	r.invalidCh <- ev
}
