package snap_sync

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	sync2 "sync"
	"time"

	"github.com/Rican7/retry"
	"github.com/Rican7/retry/strategy"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
)

var _ common.ISyncConstructor = (*SnapSync)(nil)

type SnapSync struct {
	lock             sync2.Mutex
	epochStateCache  map[uint64]*pb.QuorumCheckpoint
	recvCommitDataCh chan []common.CommitData
	commitCh         chan any
	network          network.Network
	logger           logrus.FieldLogger

	ctx    context.Context
	cancel context.CancelFunc

	cnf *common.Config
}

func (s *SnapSync) Stop() {
	s.cancel()
}

func (s *SnapSync) Start() {
	go s.listenCommitData()
}

func (s *SnapSync) Mode() common.SyncMode {
	return common.SyncModeSnapshot
}

func (s *SnapSync) listenCommitData() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case data := <-s.recvCommitDataCh:
			epoch := data[len(data)-1].GetEpoch()
			snapData := &common.SnapCommitData{
				Data:       data,
				EpochState: s.epochStateCache[epoch],
			}
			s.commitCh <- snapData
		}
	}
}

func (s *SnapSync) PostCommitData(data []common.CommitData) {
	s.recvCommitDataCh <- data
}

func (s *SnapSync) Commit() chan any {
	return s.commitCh
}

func NewSnapSync(logger logrus.FieldLogger, net network.Network) common.ISyncConstructor {
	ctx, cancel := context.WithCancel(context.Background())
	return &SnapSync{
		commitCh:         make(chan any, 1),
		recvCommitDataCh: make(chan []common.CommitData, 100),
		epochStateCache:  make(map[uint64]*pb.QuorumCheckpoint),
		network:          net,
		logger:           logger,

		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *SnapSync) Prepare(config *common.Config) (*common.PrepareData, error) {
	s.cnf = config
	epcs, err := s.fetchEpochState()
	if err != nil {
		return nil, err
	}

	return &common.PrepareData{Data: epcs}, nil
}

func (s *SnapSync) fetchEpochState() ([]*pb.EpochChange, error) {
	latestPersistEpoch := s.cnf.LatestPersistEpoch
	snapPersistedEpoch := s.cnf.SnapPersistedEpoch

	// fetch epoch state
	// latestPersistEpoch means the local's latest persisted epoch,
	// snapCurrentEpoch means the current epoch in snapshot,
	// the latestPersistEpoch is typically smaller than snapCurrentEpoch, because our ledger behind the height of the snap state,
	// and we need to synchronize to the block height of the snap.
	if err := s.fetchEpochStates(latestPersistEpoch+1, snapPersistedEpoch); err != nil {
		return nil, err
	}

	epc, err := s.fillEpochChanges(s.cnf.StartEpochChangeNum, snapPersistedEpoch)
	if err != nil {
		return nil, err
	}
	return epc, nil
}

func (s *SnapSync) fetchEpochStates(start, end uint64) error {
	if start > end {
		s.logger.WithFields(logrus.Fields{
			"latest": start - 1,
			"end":    end,
		}).Infof("local latest persist epoch %d is same as end epoch %d", start-1, end)
		return nil
	}

	taskNum := int(end - start + 1)
	s.epochStateCache = make(map[uint64]*pb.QuorumCheckpoint)
	wg := sync2.WaitGroup{}
	wg.Add(taskNum)

	errCh := make(chan error, taskNum)
	for i := start; i <= end; i++ {
		go func(epoch uint64) {
			var (
				err  error
				resp *pb.Message
			)
			defer func() {
				errCh <- err
				wg.Done()
			}()
			peer := s.pickPeer(int(epoch))
			req := &pb.FetchEpochStateRequest{
				Epoch: epoch,
			}
			data, err := req.MarshalVT()
			if err != nil {
				s.logger.WithFields(logrus.Fields{
					"peer": peer,
					"err":  err,
				}).Error("Marshal fetch epoch state request failed")
				return
			}

			invalidPeers := make([]*common.Node, 0)
			if err = retry.Retry(func(attempt uint) error {
				s.logger.WithFields(logrus.Fields{
					"peer":  peer.Id,
					"epoch": epoch,
				}).Info("Send fetch epoch state request")
				resp, err = s.network.Send(peer.PeerID, &pb.Message{
					Type: pb.Message_FETCH_EPOCH_STATE_REQUEST,
					Data: data,
				})
				if err != nil {
					s.logger.WithFields(logrus.Fields{
						"peer": peer.Id,
						"err":  err,
					}).Error("Send fetch epoch state request failed")

					invalidPeers = append(invalidPeers, peer)
					var pickErr error
					peer, pickErr = s.pickRandomPeer(invalidPeers...)
					// reset invalidPeers
					if pickErr != nil {
						invalidPeers = make([]*common.Node, 0)
						peer, _ = s.pickRandomPeer()
					}
					return err
				}

				s.logger.WithFields(logrus.Fields{
					"peer":  peer.Id,
					"epoch": epoch,
				}).Info("Receive fetch epoch state response")
				return nil
			}, strategy.Limit(uint(len(s.cnf.Peers))), strategy.Wait(200*time.Millisecond)); err != nil {
				err = fmt.Errorf("retry send fetch epoch state request failed, all peers invalid: %v", s.cnf.Peers)
				s.logger.WithFields(logrus.Fields{
					"peer":  peer.Id,
					"err":   err,
					"epoch": epoch,
				}).Error("Send fetch epoch state request failed")
				return
			}

			epcStateResp := &pb.FetchEpochStateResponse{}
			if err = epcStateResp.UnmarshalVT(resp.Data); err != nil {
				s.logger.WithFields(logrus.Fields{
					"peer": peer.Id,
					"err":  err,
				}).Error("Unmarshal fetch epoch state response failed")
				return
			}
			epochState := &pb.QuorumCheckpoint{}

			if err = epochState.UnmarshalVT(epcStateResp.Data); err != nil {
				s.logger.WithFields(logrus.Fields{
					"peer": peer.Id,
					"err":  err,
				}).Error("Unmarshal epoch state failed")
				return
			}
			s.lock.Lock()
			s.epochStateCache[epochState.Epoch] = epochState
			s.lock.Unlock()
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SnapSync) fillEpochChanges(start, end uint64) ([]*pb.EpochChange, error) {
	epochChanges := make([]*pb.EpochChange, 0)
	for epoch := start; epoch <= end; epoch++ {
		epc, ok := s.epochStateCache[epoch]
		if !ok {
			return nil, fmt.Errorf("epoch %d not found", epoch)
		}
		epochChanges = append(epochChanges, &pb.EpochChange{QuorumCheckpoint: epc})
	}

	return epochChanges, nil
}

func (s *SnapSync) pickPeer(taskNum int) *common.Node {
	index := taskNum % len(s.cnf.Peers)
	return s.cnf.Peers[index]
}

func (s *SnapSync) pickRandomPeer(exceptPeerIds ...*common.Node) (*common.Node, error) {
	if len(exceptPeerIds) > 0 {
		newPeers := lo.Filter(s.cnf.Peers, func(p *common.Node, _ int) bool {
			return !lo.Contains(exceptPeerIds, p)
		})
		if len(newPeers) == 0 {
			return nil, errors.New("all peers are excluded")
		}
		return newPeers[rand.Intn(len(newPeers))], nil
	}
	return s.cnf.Peers[rand.Intn(len(s.cnf.Peers))], nil
}
