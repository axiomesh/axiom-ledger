package peermgr

import (
	"github.com/ethereum/go-ethereum/event"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	network "github.com/axiomesh/axiom-p2p"
)

// TODO: refactor
type OrderMessageEvent struct {
	IsTxsFromRemote bool
	Data            []byte
	Txs             [][]byte
}

type KeyType any

type BasicPeerManager interface {
	// Start
	Start() error

	// Stop
	Stop() error

	// AsyncSend sends message to peer with peer info.
	AsyncSend(KeyType, *pb.Message) error

	// Send sends message waiting response
	Send(KeyType, *pb.Message) (*pb.Message, error)

	// CountConnectedPeers counts connected peer numbers
	CountConnectedPeers() uint64

	// Peers return all peers including local peer.
	Peers() []peer.AddrInfo
}

type OrderPeerManager interface {
	BasicPeerManager

	// SubscribeOrderMessage
	SubscribeOrderMessage(ch chan<- OrderMessageEvent) event.Subscription

	// AddNode adds a vp peer.
	AddNode(newNodeID uint64, vpInfo *types.VpInfo)

	// DelNode deletes a vp peer.
	DelNode(delID uint64)

	// UpdateRouter update the local router to quorum router.
	UpdateRouter(vpInfos map[uint64]*types.VpInfo, isNew bool) bool

	// Broadcast message to all node
	Broadcast(*pb.Message) error

	// Disconnect disconnect with all vp peers.
	Disconnect(vpInfos map[uint64]*types.VpInfo)

	// OrderPeers return all OrderPeers include account and id.
	OrderPeers() map[uint64]*types.VpInfo
}

//go:generate mockgen -destination mock_peermgr/mock_peermgr.go -package mock_peermgr -source peermgr.go
type PeerManager interface {
	OrderPeerManager

	// SendWithStream sends message using existed stream
	SendWithStream(network.Stream, *pb.Message) error

	// ReConfig
	ReConfig(config any) error
}