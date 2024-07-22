package snap_sync

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/axiomesh/axiom-bft/common/consensus"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/network"
	"github.com/axiomesh/axiom-ledger/internal/network/mock_network"
	"github.com/axiomesh/axiom-ledger/internal/sync/common"
)

func TestSnapSync_Mode(t *testing.T) {
	sync := mockSnapSync(nil)
	sync.Start()
	defer sync.Stop()
	require.Equal(t, common.SyncModeSnapshot, sync.Mode())
}

func TestSnapSync_Commit(t *testing.T) {
	commitData := []common.CommitData{
		&common.ChainData{Block: &types.Block{Header: &types.BlockHeader{Number: 1, Epoch: 1}}},
		&common.ChainData{Block: &types.Block{Header: &types.BlockHeader{Number: 2, Epoch: 1}}},
	}

	sync := mockSnapSync(nil)
	sync.Start()
	defer sync.Stop()

	epochStates := make(map[uint64]*pb.QuorumCheckpoint)
	epochStates[1] = &pb.QuorumCheckpoint{
		Epoch: uint64(0),
		State: &pb.ExecuteState{
			Height: 2,
			Digest: "test2",
		},
	}
	sync.(*SnapSync).epochStateCache = epochStates

	go func() {
		sync.(*SnapSync).listenCommitData()
	}()
	sync.PostCommitData(commitData)

	res := <-sync.Commit()
	require.Equal(t, "test2", res.(*common.SnapCommitData).EpochState.GetState().GetDigest())
}

func TestSnapSync_Prepare(t *testing.T) {
	peersSet := []*common.Node{
		{
			Id:     1,
			PeerID: "peer1",
		},
		{
			Id:     2,
			PeerID: "peer2",
		},
		{
			Id:     3,
			PeerID: "peer3",
		},
	}

	epcStates := make(map[string]map[uint64]*pb.QuorumCheckpoint)
	epcStates[peersSet[0].PeerID] = make(map[uint64]*pb.QuorumCheckpoint)
	epcStates[peersSet[0].PeerID][1] = &pb.QuorumCheckpoint{
		Epoch: 1,
		State: &pb.ExecuteState{
			Height: 100,
			Digest: "test100",
		},
	}
	epcStates[peersSet[0].PeerID][2] = &pb.QuorumCheckpoint{
		Epoch: 2,
		State: &pb.ExecuteState{
			Height: 200,
			Digest: "test200",
		},
	}

	t.Run("test fetch epoch state failed", func(t *testing.T) {
		latestBlock := &types.Block{
			Header: &types.BlockHeader{
				Number: 133,
				Epoch:  2,
			},
		}
		conf := &common.Config{
			Peers:               peersSet,
			StartEpochChangeNum: latestBlock.Header.Epoch,
			SnapPersistedEpoch:  1,
			LatestPersistEpoch:  0,
		}
		sync := mockSnapSync(mockNetwork(t, nil))
		defer sync.Stop()
		data, err := sync.Prepare(conf)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "all peers invalid")
		require.Nil(t, data)
	})
	t.Run("test fill epoch change failed", func(t *testing.T) {
		latestBlock := &types.Block{
			Header: &types.BlockHeader{
				Number: 133,
				Epoch:  1,
			},
		}
		conf := &common.Config{
			Peers:               peersSet,
			StartEpochChangeNum: latestBlock.Header.Epoch,
			SnapPersistedEpoch:  2,
			LatestPersistEpoch:  2,
		}
		sync := mockSnapSync(mockNetwork(t, epcStates))
		defer sync.Stop()
		data, err := sync.Prepare(conf)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "epoch 1 not found")
		require.Nil(t, data)
	})

	t.Run("test snap sync need not prepare epoch change", func(t *testing.T) {
		latestBlock := &types.Block{
			Header: &types.BlockHeader{
				Number: 133,
				Epoch:  2,
			},
		}
		conf := &common.Config{
			Peers:               peersSet,
			StartEpochChangeNum: latestBlock.Header.Epoch,
			SnapPersistedEpoch:  1,
			LatestPersistEpoch:  1,
		}
		sync := mockSnapSync(mockNetwork(t, epcStates))
		defer sync.Stop()
		data, err := sync.Prepare(conf)
		require.Nil(t, err)
		require.Equal(t, 0, len(data.Data.([]*consensus.EpochChange)))
	})

	t.Run("test snap sync need prepare epoch change", func(t *testing.T) {
		// test snap sync need prepare epoch change
		latestBlock := &types.Block{
			Header: &types.BlockHeader{
				Number: 1,
				Epoch:  1,
			},
		}
		conf := &common.Config{
			Peers:               peersSet,
			StartEpochChangeNum: latestBlock.Header.Epoch,
			SnapPersistedEpoch:  2,
			LatestPersistEpoch:  0,
		}
		sync := mockSnapSync(mockNetwork(t, epcStates))
		defer sync.Stop()

		data, err := sync.Prepare(conf)
		require.Nil(t, err)
		require.Equal(t, data.Data.([]*consensus.EpochChange)[0].Checkpoint.Digest(), "test100")
		require.Equal(t, data.Data.([]*consensus.EpochChange)[1].Checkpoint.Digest(), "test200")
	})

	t.Run("test latestBlock EpochHeight is bigger than snapPersistedEpoch", func(t *testing.T) {
		sync := mockSnapSync(mockNetwork(t, epcStates))
		defer sync.Stop()

		latestBlock := &types.Block{
			Header: &types.BlockHeader{
				Number: 1,
				Epoch:  1,
			},
		}
		conf := &common.Config{
			Peers:               peersSet,
			StartEpochChangeNum: latestBlock.Header.Epoch,
			SnapPersistedEpoch:  2,
			LatestPersistEpoch:  0,
		}
		data, err := sync.Prepare(conf)
		require.Nil(t, err)
		require.Equal(t, data.Data.([]*consensus.EpochChange)[0].Checkpoint.Digest(), "test100")
		require.Equal(t, data.Data.([]*consensus.EpochChange)[1].Checkpoint.Digest(), "test200")
	})
}

func mockNetwork(t *testing.T, mockEpcStates map[string]map[uint64]*pb.QuorumCheckpoint) network.Network {
	ctrl := gomock.NewController(t)
	net := mock_network.NewMockNetwork(ctrl)
	net.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(func(peerId string, msg *pb.Message) (*pb.Message, error) {
		req := &pb.FetchEpochStateRequest{}
		err := req.UnmarshalVT(msg.GetData())
		require.Nil(t, err)

		if states, ok := mockEpcStates[peerId]; ok {
			epcState, ok := states[req.Epoch]
			if !ok {
				return nil, fmt.Errorf("no state for epoch %d", req.Epoch)
			}
			epcStateBytes, err := epcState.MarshalVT()
			require.Nil(t, err)
			resp := &pb.FetchEpochStateResponse{
				Data:   epcStateBytes,
				Status: pb.Status_SUCCESS,
			}
			respBytes, err := resp.MarshalVT()
			require.Nil(t, err)
			return &pb.Message{
				Type: pb.Message_FETCH_EPOCH_STATE_RESPONSE,
				Data: respBytes,
			}, nil
		}
		return nil, fmt.Errorf("no state for peer %s", peerId)
	}).AnyTimes()
	return net
}

func mockSnapSync(network network.Network) common.ISyncConstructor {
	logger := log.NewWithModule("snap_sync_test")
	logger.Logger.SetLevel(logrus.DebugLevel)

	return NewSnapSync(logger, network)
}

func stopSnapSync(t *testing.T, cancel context.CancelFunc) {
	cancel()
}
