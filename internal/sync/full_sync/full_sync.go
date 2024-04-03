package full_sync

import (
	"context"

	"github.com/axiomesh/axiom-ledger/internal/sync/common"
)

var _ common.ISyncConstructor = (*FullSync)(nil)

type FullSync struct {
	recvCommitDataCh chan []common.CommitData
	commitCh         chan any
	ctx              context.Context
	cancel           context.CancelFunc
}

func (s *FullSync) Start() {
	go s.listenCommitData()
}

func (s *FullSync) Stop() {
	s.cancel()
}

func (s *FullSync) Mode() common.SyncMode {
	return common.SyncModeFull
}

func (s *FullSync) PostCommitData(data []common.CommitData) {
	s.recvCommitDataCh <- data
}

func (s *FullSync) Commit() chan any {
	return s.commitCh
}

func NewFullSync() common.ISyncConstructor {
	ctx, cancel := context.WithCancel(context.Background())
	return &FullSync{
		recvCommitDataCh: make(chan []common.CommitData, 1),
		commitCh:         make(chan any, 1),
		ctx:              ctx,
		cancel:           cancel,
	}
}

func (s *FullSync) Prepare(_ *common.Config) (*common.PrepareData, error) {
	return nil, nil
}

func (s *FullSync) listenCommitData() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case data := <-s.recvCommitDataCh:
			s.commitCh <- data
		}
	}
}
