package network

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	network "github.com/axiomesh/axiom-p2p"
)

const (
	protocolID string = "/draconis/1.0.0" // magic protocol
)

// ony used for mock
type Pipe interface {
	fmt.Stringer
	Send(ctx context.Context, to string, data []byte) error
	Broadcast(ctx context.Context, targets []string, data []byte) error
	Receive(ctx context.Context) *network.PipeMsg
}

//go:generate mockgen -destination mock_network/mock_network.go -package mock_network -source network.go -typed
type Network interface {
	network.PipeManager

	Start() error

	Stop() error

	// Send sends message waiting response
	Send(string, *pb.Message) (*pb.Message, error)

	// SendWithStream sends message using existed stream
	SendWithStream(network.Stream, *pb.Message) error

	PeerID() string

	// CountConnectedValidators counts connected validator numbers
	CountConnectedValidators() uint64

	// RegisterMsgHandler registers one message type handler
	RegisterMsgHandler(messageType pb.Message_Type, handler func(network.Stream, *pb.Message)) error

	// RegisterMultiMsgHandler registers multi message type handler
	RegisterMultiMsgHandler(messageTypes []pb.Message_Type, handler func(network.Stream, *pb.Message)) error
}

var _ Network = (*networkImpl)(nil)

type networkImpl struct {
	repo   *repo.Repo
	ledger *ledger.Ledger
	p2p    network.Network
	logger logrus.FieldLogger
	ctx    context.Context
	cancel context.CancelFunc
	gater  connmgr.ConnectionGater
	network.PipeManager

	msgHandlers sync.Map // map[pb.Message_Type]MessageHandler
}

func New(repoConfig *repo.Repo, logger logrus.FieldLogger, ledger *ledger.Ledger) (Network, error) {
	return newNetworkImpl(repoConfig, logger, ledger)
}

func newNetworkImpl(repoConfig *repo.Repo, logger logrus.FieldLogger, ledger *ledger.Ledger) (*networkImpl, error) {
	ctx, cancel := context.WithCancel(context.Background())
	swarm := &networkImpl{repo: repoConfig, logger: logger, ledger: ledger, ctx: ctx, cancel: cancel}
	if err := swarm.init(); err != nil {
		return nil, err
	}

	return swarm, nil
}

func (swarm *networkImpl) init() error {
	// init peers with ips and hosts
	bootstrap := make([]string, 0)
	for _, a := range swarm.repo.Config.P2P.BootstrapNodeAddresses {
		if !strings.Contains(a, swarm.repo.P2PID) {
			bootstrap = append(bootstrap, a)
		}
	}

	var securityType network.SecurityType
	switch swarm.repo.Config.P2P.Security {
	case repo.P2PSecurityTLS:
		securityType = network.SecurityTLS
	case repo.P2PSecurityNoise:
		securityType = network.SecurityNoise
	default:
		securityType = network.SecurityNoise
	}

	var pipeBroadcastType network.PipeBroadcastType
	switch swarm.repo.Config.P2P.Pipe.BroadcastType {
	case repo.P2PPipeBroadcastSimple:
		pipeBroadcastType = network.PipeBroadcastSimple
	case repo.P2PPipeBroadcastGossip:
		pipeBroadcastType = network.PipeBroadcastGossip
	default:
		return fmt.Errorf("unsupported p2p pipe broadcast type: %v", swarm.repo.Config.P2P.Pipe.BroadcastType)
	}

	libp2pKey, err := repo.Libp2pKeyFromECDSAKey(swarm.repo.P2PKey)
	if err != nil {
		return fmt.Errorf("failed to convert ecdsa p2pKey: %w", err)
	}
	protocolIDWithVersion := fmt.Sprintf("%s-%x", protocolID, sha256.Sum256([]byte(repo.BuildVersionSecret)))

	gater := newConnectionGater(swarm.logger, swarm.ledger)
	opts := []network.Option{
		network.WithLocalAddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", swarm.repo.Config.Port.P2P)),
		network.WithPrivateKey(libp2pKey),
		network.WithProtocolID(protocolIDWithVersion),
		network.WithLogger(swarm.logger),
		network.WithTimeout(10*time.Second, swarm.repo.Config.P2P.SendTimeout.ToDuration(), swarm.repo.Config.P2P.ReadTimeout.ToDuration()),
		network.WithSecurity(securityType),
		network.WithPipe(network.PipeConfig{
			BroadcastType:       pipeBroadcastType,
			ReceiveMsgCacheSize: swarm.repo.Config.P2P.Pipe.ReceiveMsgCacheSize,
			SimpleBroadcast: network.PipeSimpleConfig{
				WorkerCacheSize:        swarm.repo.Config.P2P.Pipe.SimpleBroadcast.WorkerCacheSize,
				WorkerConcurrencyLimit: swarm.repo.Config.P2P.Pipe.SimpleBroadcast.WorkerConcurrencyLimit,
			},
			Gossipsub: network.PipeGossipsubConfig{
				SubBufferSize:          swarm.repo.Config.P2P.Pipe.Gossipsub.SubBufferSize,
				PeerOutboundBufferSize: swarm.repo.Config.P2P.Pipe.Gossipsub.PeerOutboundBufferSize,
				ValidateBufferSize:     swarm.repo.Config.P2P.Pipe.Gossipsub.ValidateBufferSize,
				SeenMessagesTTL:        swarm.repo.Config.P2P.Pipe.Gossipsub.SeenMessagesTTL.ToDuration(),
				EnableMetrics:          swarm.repo.Config.P2P.Pipe.Gossipsub.EnableMetrics,
			},
			UnicastReadTimeout:       swarm.repo.Config.P2P.Pipe.UnicastReadTimeout.ToDuration(),
			UnicastSendRetryNumber:   swarm.repo.Config.P2P.Pipe.UnicastSendRetryNumber,
			UnicastSendRetryBaseTime: swarm.repo.Config.P2P.Pipe.UnicastSendRetryBaseTime.ToDuration(),
			FindPeerTimeout:          swarm.repo.Config.P2P.Pipe.FindPeerTimeout.ToDuration(),
			ConnectTimeout:           swarm.repo.Config.P2P.Pipe.ConnectTimeout.ToDuration(),
		}),
		network.WithBootstrap(bootstrap),
		network.WithConnectionGater(gater),
		network.WithCompressionOption(swarm.repo.Config.P2P.CompressionAlgo),
		network.WithMetrics(swarm.repo.Config.P2P.EnableMetrics),
	}

	p2p, err := network.New(swarm.ctx, opts...)
	if err != nil {
		return fmt.Errorf("create p2p: %w", err)
	}
	swarm.p2p = p2p
	swarm.gater = gater
	swarm.PipeManager = p2p
	return nil
}

func (swarm *networkImpl) Start() error {
	swarm.p2p.SetMessageHandler(swarm.handleMessage)

	if err := swarm.p2p.Start(); err != nil {
		return fmt.Errorf("start p2p failed: %w", err)
	}

	return nil
}

func (swarm *networkImpl) Stop() error {
	swarm.cancel()
	return swarm.p2p.Stop()
}

func (swarm *networkImpl) SendWithStream(s network.Stream, msg *pb.Message) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return fmt.Errorf("marshal message error: %w", err)
	}

	return s.AsyncSend(data)
}

func (swarm *networkImpl) Send(to string, msg *pb.Message) (*pb.Message, error) {
	data, err := msg.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("marshal message error: %w", err)
	}

	ret, err := swarm.p2p.Send(to, data)
	if err != nil {
		return nil, fmt.Errorf("sync send: %w", err)
	}

	m := &pb.Message{}
	if err := m.UnmarshalVT(ret); err != nil {
		return nil, fmt.Errorf("unmarshal message error: %w", err)
	}

	return m, nil
}

func (swarm *networkImpl) CountConnectedValidators() uint64 {
	var cnt uint64
	validatorSet := swarm.repo.EpochInfo.ValidatorSet
	for _, n := range validatorSet {
		if swarm.p2p.IsConnected(n.P2PNodeID) {
			cnt++
		}
	}
	return cnt
}

func (swarm *networkImpl) PeerID() string {
	return swarm.p2p.PeerID()
}

func (swarm *networkImpl) RegisterMsgHandler(messageType pb.Message_Type, handler func(network.Stream, *pb.Message)) error {
	if handler == nil {
		return errors.New("register msg handler: empty handler")
	}

	for msgType := range pb.Message_Type_name {
		if msgType == int32(messageType) {
			swarm.msgHandlers.Store(messageType, handler)
			return nil
		}
	}

	return errors.New("register msg handler: invalid message type")
}

func (swarm *networkImpl) RegisterMultiMsgHandler(messageTypes []pb.Message_Type, handler func(network.Stream, *pb.Message)) error {
	for _, typ := range messageTypes {
		if err := swarm.RegisterMsgHandler(typ, handler); err != nil {
			return err
		}
	}

	return nil
}
