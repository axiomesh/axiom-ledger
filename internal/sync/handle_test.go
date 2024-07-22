package sync

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-kit/types/pb"
	network "github.com/axiomesh/axiom-p2p"
)

const (
	defaultLatestHeight = 100
	wrongMsgType        = 1000
	wrongReqData        = "proto: illegal wireType 7"
	wrongBlock          = "block not found"
	wrongEpoch          = "epoch 101 quorum checkpoint not found"
)

func TestHandleState(t *testing.T) {
	t.Logf("Test handle state")
	localId := 0
	remoteId := 1

	n := 4
	syncs, ledgers := newMockBlockSyncs(t, n)
	_, err := syncs[0].Prepare()
	require.Nil(t, err)

	requestHeight := uint64(defaultLatestHeight)
	prepareLedger(t, ledgers, strconv.Itoa(localId), int(requestHeight))
	block100, err := ledgers[strconv.Itoa(remoteId)].GetBlock(requestHeight)
	require.Nil(t, err)

	epoch1 := ledgers[strconv.Itoa(remoteId)].epochStateDb[epochPrefix+strconv.Itoa(1)]
	requestEpoch := epoch1.Epoch()

	var (
		wrongMsgTypeReq             = fmt.Errorf("wrong message type: %v", wrongMsgType)
		wrongSyncStateReqData       = fmt.Errorf("unmarshal sync state request failed: %v", wrongReqData)
		wrongFetchEpochStateReqData = fmt.Errorf("unmarshal fetch epoch state request failed: %v", wrongReqData)
		getBlockFailed              = fmt.Errorf("get block failed: %v", wrongBlock)
		getEpochStateFailed         = fmt.Errorf("get epoch state failed: %v", wrongEpoch)
	)

	testcases := []struct {
		caller         func(s network.Stream, msg *pb.Message)
		expectedReqTyp pb.Message_Type
		req            *pb.Message
		expectedResp   Response
	}{
		{ // case1 : handle sync state success
			caller:         syncs[remoteId].handleSyncState,
			expectedReqTyp: pb.Message_SYNC_STATE_REQUEST,
			req:            genSuccessReqMsg(t, pb.Message_SYNC_STATE_REQUEST, localId, 100),
			expectedResp:   genSuccessRespMsg(t, pb.Message_SYNC_STATE_RESPONSE, block100),
		},

		{ // case2 : handle sync state failed, wrong msg type
			caller:         syncs[remoteId].handleSyncState,
			expectedReqTyp: pb.Message_SYNC_STATE_REQUEST,
			req:            genWrongTypeReqMsg(pb.Message_Type(wrongMsgType)),
			expectedResp:   genWrongRespMsg(pb.Message_SYNC_STATE_RESPONSE, wrongMsgTypeReq.Error()),
		},

		{ // case3 : handle sync state failed, unmarshal sync state request failed
			caller:         syncs[remoteId].handleSyncState,
			expectedReqTyp: pb.Message_SYNC_STATE_REQUEST,
			req:            genWrongDataReqMsg(t, pb.Message_SYNC_STATE_REQUEST, localId),
			expectedResp:   genWrongRespMsg(pb.Message_SYNC_STATE_RESPONSE, wrongSyncStateReqData.Error()),
		},

		{ // case4 : handle  sync state failed, get block failed
			caller:         syncs[remoteId].handleSyncState,
			expectedReqTyp: pb.Message_SYNC_STATE_REQUEST,
			req:            genSuccessReqMsg(t, pb.Message_SYNC_STATE_REQUEST, localId, requestHeight+100),
			expectedResp:   genWrongRespMsg(pb.Message_SYNC_STATE_RESPONSE, getBlockFailed.Error()),
		},

		{ // case5 : handle fetch epoch state success
			caller:         syncs[remoteId].handleFetchEpochState,
			expectedReqTyp: pb.Message_FETCH_EPOCH_STATE_REQUEST,
			req:            genSuccessReqMsg(t, pb.Message_FETCH_EPOCH_STATE_REQUEST, localId, requestEpoch),
			expectedResp:   genSuccessRespMsg(t, pb.Message_FETCH_EPOCH_STATE_RESPONSE, epoch1),
		},

		{ // case6 : handle fetch epoch state failed, wrong msg type
			caller:         syncs[remoteId].handleFetchEpochState,
			expectedReqTyp: pb.Message_FETCH_EPOCH_STATE_REQUEST,
			req:            genWrongTypeReqMsg(wrongMsgType),
			expectedResp:   genWrongRespMsg(pb.Message_FETCH_EPOCH_STATE_RESPONSE, wrongMsgTypeReq.Error()),
		},

		{ // case7 : handle fetch epoch state failed, unmarshal fetch epoch state request failed
			caller:         syncs[remoteId].handleFetchEpochState,
			expectedReqTyp: pb.Message_FETCH_EPOCH_STATE_REQUEST,
			req:            genWrongDataReqMsg(t, pb.Message_FETCH_EPOCH_STATE_REQUEST, localId),
			expectedResp:   genWrongRespMsg(pb.Message_FETCH_EPOCH_STATE_RESPONSE, wrongFetchEpochStateReqData.Error()),
		},

		{ // case8 : handle fetch epoch state failed, get epoch state failed
			caller:         syncs[remoteId].handleFetchEpochState,
			expectedReqTyp: pb.Message_FETCH_EPOCH_STATE_REQUEST,
			req:            genSuccessReqMsg(t, pb.Message_FETCH_EPOCH_STATE_REQUEST, localId, requestEpoch+100),
			expectedResp:   genWrongRespMsg(pb.Message_FETCH_EPOCH_STATE_RESPONSE, getEpochStateFailed.Error()),
		},
	}

	for i, test := range testcases {
		t.Logf("test case %d", i+1)
		test.caller(nil, test.req)
		resp := getResp(ledgers[strconv.Itoa(remoteId)])
		switch test.expectedReqTyp {
		case pb.Message_SYNC_STATE_REQUEST:
			actualResp := &pb.SyncStateResponse{}
			err = actualResp.UnmarshalVT(resp.Data)
			require.Nil(t, err)
			require.True(t, reflect.DeepEqual(test.expectedResp, actualResp))

		case pb.Message_FETCH_EPOCH_STATE_REQUEST:
			actualResp := &pb.FetchEpochStateResponse{}
			err = actualResp.UnmarshalVT(resp.Data)
			require.Nil(t, err)
			require.True(t, reflect.DeepEqual(test.expectedResp, actualResp))
		}
	}
}

func getResp(ledger *mockLedger) *pb.Message {
	for {
		select {
		case resp := <-ledger.stateResponse:
			return resp
		case resp := <-ledger.epochStateResponse:
			return resp
		}
	}
}

func genSuccessReqMsg(t *testing.T, typ pb.Message_Type, localId int, input uint64) *pb.Message {
	var (
		reqData []byte
		err     error
	)
	switch typ {
	case pb.Message_SYNC_STATE_REQUEST:
		req := &pb.SyncStateRequest{
			Height: input,
		}
		reqData, err = req.MarshalVT()
		require.Nil(t, err)

	case pb.Message_FETCH_EPOCH_STATE_REQUEST:
		req := &pb.FetchEpochStateRequest{
			Epoch: input,
		}
		reqData, err = req.MarshalVT()
		require.Nil(t, err)
	}

	msg := &pb.Message{
		From: strconv.Itoa(localId),
		Type: typ,
		Data: reqData,
	}
	return msg
}

func genSuccessRespMsg(t *testing.T, typ pb.Message_Type, input any) Response {
	var (
		resp Response
	)
	switch typ {
	case pb.Message_SYNC_STATE_RESPONSE:
		block := input.(*types.Block)
		resp = &pb.SyncStateResponse{
			Status: pb.Status_SUCCESS,
			CheckpointState: &pb.CheckpointState{
				Height:       block.Height(),
				Digest:       block.Hash().String(),
				LatestHeight: block.Height(),
			},
		}

	case pb.Message_FETCH_EPOCH_STATE_RESPONSE:
		eps := input.(*pb.QuorumCheckpoint)
		epochState, err := eps.MarshalVT()
		require.Nil(t, err)
		resp = &pb.FetchEpochStateResponse{
			Status: pb.Status_SUCCESS,
			Data:   epochState,
		}
	}

	return resp
}

func genWrongRespMsg(typ pb.Message_Type, err string) Response {
	var (
		resp Response
	)
	switch typ {
	case pb.Message_SYNC_STATE_RESPONSE:
		resp = &pb.SyncStateResponse{
			Status: pb.Status_ERROR,
			Error:  err,
		}
		if strings.Contains(err, wrongBlock) {
			resp.(*pb.SyncStateResponse).CheckpointState = &pb.CheckpointState{
				LatestHeight: uint64(defaultLatestHeight),
			}
		}

	case pb.Message_FETCH_EPOCH_STATE_RESPONSE:
		resp = &pb.FetchEpochStateResponse{
			Status: pb.Status_ERROR,
			Error:  err,
		}
	}

	return resp
}

func genWrongTypeReqMsg(wrongTyp pb.Message_Type) *pb.Message {
	return &pb.Message{
		Type: wrongTyp,
		Data: []byte{},
	}
}

func genWrongDataReqMsg(t *testing.T, typ pb.Message_Type, localId int) *pb.Message {
	msg := genSuccessReqMsg(t, typ, localId, 1)
	msg.Data = []byte("wrong data")

	return msg
}
