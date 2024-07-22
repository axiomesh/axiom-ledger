package adaptor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Rican7/retry"
	"github.com/Rican7/retry/strategy"
	"github.com/axiomesh/axiom-kit/types/pb"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/network"
	p2p "github.com/axiomesh/axiom-p2p"
	"github.com/bcds/go-hpc-dagbft/common/config"
	"github.com/bcds/go-hpc-dagbft/common/types"
	"github.com/bcds/go-hpc-dagbft/common/types/messages"
	"github.com/bcds/go-hpc-dagbft/common/types/protos"
	"github.com/bcds/go-hpc-dagbft/common/utils/channel"
	"github.com/bcds/go-hpc-dagbft/common/utils/containers"
	"github.com/bcds/go-hpc-dagbft/common/utils/results"
	"github.com/bcds/go-hpc-dagbft/protocol"
	"github.com/bcds/go-hpc-dagbft/protocol/layer"
	"github.com/gammazero/workerpool"
	"github.com/gogo/protobuf/proto"
	"github.com/samber/lo"
)

const (
	Primary = "primary"
	Worker  = "worker"
)

type (
	Pid  = string
	ID   = uint64
	Port = int
)

type NetworkProtocol interface {
	layer.NetworkFactory
}

type NetworkFactory struct {
	networks map[types.Host]*Network
}

func NewNetworkFactory(cnf *common.Config, ctx context.Context) *NetworkFactory {
	pid := cnf.ChainState.SelfNodeInfo.P2PID
	workers := lo.SliceToMap(cnf.ChainState.SelfNodeInfo.Workers, func(n types.Host) (types.Host, Pid) {
		return n, pid
	})
	signalNet := &Network{
		Network:       cnf.Network,
		localHost:     containers.Pack2(cnf.ChainState.SelfNodeInfo.Primary, cnf.ChainState.SelfNodeInfo.P2PID),
		remotes:       workers,
		networkConfig: *DefaultNetworkConfig(),
		requestCh:     make(chan *wrapRequest, common.MaxChainSize),
		ctx:           ctx,
	}

	signalNet.networkConfig.LocalWorkers = workers
	signalNet.networkConfig.LocalPrimary = signalNet.localHost

	networks := lo.SliceToMap(cnf.ChainState.SelfNodeInfo.Workers, func(n types.Host) (types.Host, *Network) {
		return n, signalNet
	})
	networks[cnf.ChainState.SelfNodeInfo.Primary] = signalNet

	return &NetworkFactory{
		networks: networks,
	}
}

func (n *NetworkFactory) GetNetwork(peer types.Peer) layer.Network {
	return n.networks[peer.Host]
}

type NetworkConfig struct {
	LocalPrimary    containers.Pair[types.Host, Pid]
	LocalWorkers    map[types.Host]Pid
	ConcurrentLimit int
	RetryConfig
}

type RetryConfig struct {
	RetryAttempts int
	RetryTimeout  time.Duration
}

func DefaultNetworkConfig() *NetworkConfig {
	return &NetworkConfig{
		ConcurrentLimit: 10,
		RetryConfig: RetryConfig{
			RetryAttempts: 5,
			RetryTimeout:  500 * time.Millisecond,
		},
	}
}

type wrapRequest struct {
	to     Pid
	index  int
	msg    *pb.Message
	respCh chan<- protocol.MessageResult
}

type Network struct {
	network.Network

	localHost containers.Pair[types.Host, Pid] // dagbft name -> pid

	networkConfig NetworkConfig

	remotes map[types.Host]Pid

	requestCh chan *wrapRequest

	ctx context.Context
}

func (n *Network) Author() types.Host {
	return n.localHost.First
}

func genWrongResponse(err error, index int) protocol.MessageResult {
	return protocol.MessageResult{
		Index:  index,
		Result: results.Error[protocol.Message](err),
	}
}

func (n *Network) listenRequestToSubmit() error {
	wp := workerpool.New(n.networkConfig.ConcurrentLimit)
	for {
		select {
		case <-n.ctx.Done():
			return nil
		case req := <-n.requestCh:
			wp.Submit(func() {
				var result protocol.MessageResult
				if err := retry.Retry(func(attempt uint) error {
					responseMsg, err := n.Send(req.to, req.msg)
					if err != nil {
						if strings.Contains(err.Error(), p2p.WaitMsgTimeout.Error()) {
							return err
						} else {
							result = genWrongResponse(err, req.index)
							return nil
						}
					}

					resp, err := decodeMessageResult(responseMsg, true)
					if err != nil {
						result = genWrongResponse(err, req.index)
						return nil
					}

					result = protocol.MessageResult{
						Index:  req.index,
						Result: results.OK(resp),
					}
					return nil
				}, strategy.Limit(uint(n.networkConfig.RetryTimeout)), strategy.Wait(200*time.Millisecond)); err != nil {
					result = genWrongResponse(err, req.index)
				}

				req.respCh <- result
			})
		}
	}
}
func decodeMessageResult(msg *pb.Message, isPrimary bool) (protocol.Message, error) {
	// epoch message
	switch msg.Type {
	case pb.Message_EPOCH_REQUEST_RESPONSE:
		var resp protos.EpochChangeResponse
		err := resp.Unmarshal(msg.Data)
		if err != nil {
			return nil, err
		}
	case pb.Message_EPOCH_PROOF_RESPONSE:
		return &messages.NullMessage{}, nil
	}
	if isPrimary {
		switch msg.Type {
		case pb.Message_REQUEST_VOTE_RESPONSE:
			var resp protos.RequestVoteResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.RequestVoteResponse{RequestVoteResponse: resp}, nil
		case pb.Message_SEND_CERTIFICATE_RESPONSE:
			var resp protos.SendCertificateResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.SendCertificateResponse{SendCertificateResponse: resp}, nil
		case pb.Message_FETCH_CERTIFICATES_RESPONSE:
			var resp protos.FetchCertificatesResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.FetchCertificatesResponse{FetchCertificatesResponse: resp}, nil
		case pb.Message_FETCH_COMMIT_SUB_DAG_RESPONSE:
			var resp protos.FetchCommitSummaryResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.FetchCommitSummaryResponse{FetchCommitSummaryResponse: resp}, nil
		case pb.Message_WORKER_OUR_BATCH_RESPONSE:
			return &messages.NullMessage{}, nil
		case pb.Message_WORKER_OTHERS_BATCH_RESPONSE:
			return &messages.NullMessage{}, nil
		case pb.Message_SEND_CHECKPOINT_RESPONSE:
			return &messages.NullMessage{}, nil
		case pb.Message_FETCH_CHECKPOINT_RESPONSE:
			var resp protos.SignedCheckpoint
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.FetchCheckpointResponse{SignedCheckpoint: resp}, nil
		case pb.Message_FETCH_QUORUM_CHECKPOINT_RESPONSE:
			var resp protos.QuorumCheckpoint
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.FetchQuorumCheckpointResponse{QuorumCheckpoint: resp}, nil
		}
	} else {
		// worker message
		switch msg.Type {
		case pb.Message_NEW_BATCH_RESPONSE:
			return &messages.NullMessage{}, nil
		case pb.Message_WORKER_SYNCHRONIZE_RESPONSE:
			return &messages.NullMessage{}, nil
		case pb.Message_FETCH_BATCHES_RESPONSE:
			var resp protos.FetchBatchesResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.FetchBatchesResponse{FetchBatchesResponse: resp}, nil
		case pb.Message_REQUEST_BATCH_RESPONSE:
			var resp protos.RequestBatchResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.RequestBatchResponse{RequestBatchResponse: resp}, nil
		case pb.Message_REQUEST_BATCHES_RESPONSE:
			var resp protos.RequestBatchesResponse
			err := resp.Unmarshal(msg.Data)
			if err != nil {
				return nil, err
			}
			return &messages.RequestBatchesResponse{RequestBatchesResponse: resp}, nil
		case pb.Message_PRIMARY_STATE_RESPONSE:
			return &messages.NullMessage{}, nil
		}
	}

	return nil, fmt.Errorf("wrong message type: %v", msg.Type)
}

func dispatchMsgToType(msg protocol.Message) pb.Message_Type {
	switch msg.(type) {
	// primary
	case *messages.RequestVoteRequest:
		return pb.Message_REQUEST_VOTE
	case *messages.SendCertificateRequest:
		return pb.Message_SEND_CERTIFICATE
	case *messages.FetchCertificatesRequest:
		return pb.Message_FETCH_CERTIFICATES
	case *messages.FetchCommitSummaryRequest:
		return pb.Message_FETCH_COMMIT_SUB_DAG
	case *messages.WorkerOurBatchMsg:
		return pb.Message_WORKER_OUR_BATCH
	case *messages.WorkerOthersBatchMsg:
		return pb.Message_WORKER_OTHERS_BATCH
	case *messages.SendCheckpointRequest:
		return pb.Message_SEND_CHECKPOINT
	case *messages.FetchCheckpointRequest:
		return pb.Message_FETCH_CHECKPOINT
	case *messages.FetchQuorumCheckpointRequest:
		return pb.Message_FETCH_QUORUM_CHECKPOINT
	// worker
	case *messages.WorkerBatchMsg:
		return pb.Message_NEW_BATCH
	case *messages.WorkerSynchronizeMsg:
		return pb.Message_WORKER_SYNCHRONIZE
	case *messages.FetchBatchesRequest:
		return pb.Message_FETCH_BATCHES
	case *messages.RequestBatchRequest:
		return pb.Message_REQUEST_BATCH
	case *messages.RequestBatchesRequest:
		return pb.Message_REQUEST_BATCHES
	case *messages.PrimaryStateMsg:
		return pb.Message_PRIMARY_STATE
	// epoch
	case *messages.EpochChangeRequest:
		return pb.Message_EPOCH_REQUEST
	case *messages.SendEpochChangeProof:
		return pb.Message_EPOCH_PROOF
	}

	return pb.Message_UNKNOWN
}

func (n *Network) Unicast(req protocol.Message, peer types.Peer, retry config.RetryConfig) (<-chan protocol.MessageResult, chan bool) {
	respCh := make(chan protocol.MessageResult, 1)
	cancel := make(chan bool)
	n.unicast(req, peer, retry, respCh, cancel)
	return respCh, cancel
}

func (n *Network) unicast(request protocol.Message, peer types.Peer, retryConf config.RetryConfig, respCh chan protocol.MessageResult, cancelCh chan bool) {
	var (
		enc   []byte
		err   error
		index = 0
	)

	if m, ok := request.(proto.Marshaler); ok {
		enc, err = m.Marshal()
	} else {
		enc, err = json.Marshal(m)
	}

	if err != nil {
		failedResp := protocol.MessageResult{
			Index:  0,
			Result: results.Error[protocol.Message](err),
		}

		channel.SafeSend(respCh, failedResp, cancelCh)
		return
	}

	pid, ok := n.remotes[peer.Host]
	if !ok {
		failedResp := protocol.MessageResult{
			Index:  0,
			Result: results.Error[protocol.Message](fmt.Errorf("peer not found: %s", peer.Host)),
		}

		channel.SafeSend(respCh, failedResp, cancelCh)
		return
	}

	netMsg := &pb.Message{
		From: n.Author(),
		Data: enc,
		Type: dispatchMsgToType(request),
	}

	wr := &wrapRequest{to: pid, index: index, msg: netMsg, respCh: respCh}
	channel.SafeSend(n.requestCh, wr, cancelCh)
	return
}

func (n *Network) Broadcast(m protocol.Message, peers []types.Peer, retry config.RetryConfig) (<-chan protocol.MessageResult, []chan bool) {
	cancels := channel.MakeChannels[bool](len(peers))
	responses := make(chan protocol.MessageResult, len(peers))
	lo.ForEach(peers, func(peer types.Peer, index int) {
		n.unicast(m, peer, retry, responses, cancels[index])
	})
	return responses, cancels
}
