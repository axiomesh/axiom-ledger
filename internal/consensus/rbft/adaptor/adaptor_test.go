package adaptor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/axiomesh/axiom-bft/common/consensus"
	rbfttypes "github.com/axiomesh/axiom-bft/types"
	"github.com/axiomesh/axiom-kit/log"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/consensus/common"
	"github.com/axiomesh/axiom-ledger/internal/consensus/rbft/testutil"
	network "github.com/axiomesh/axiom-p2p"
)

func mockAdaptor(ctrl *gomock.Controller, t *testing.T) *RBFTAdaptor {
	logger := log.NewWithModule("consensus")
	cfg, _ := testutil.MockConsensusConfig(logger, ctrl, t)
	stack, err := NewRBFTAdaptor(cfg)
	assert.Nil(t, err)

	consensusMsgPipes := make(map[int32]network.Pipe, len(consensus.Type_name))
	for id, name := range consensus.Type_name {
		msgPipe, err := stack.config.Network.CreatePipe(context.Background(), "test_pipe_"+name)
		assert.Nil(t, err)
		consensusMsgPipes[id] = msgPipe
	}

	stack.SetMsgPipes(consensusMsgPipes)
	err = stack.UpdateEpoch()
	assert.Nil(t, err)
	return stack
}

func mockAdaptorWithStorageType(ctrl *gomock.Controller, t *testing.T, typ string) *RBFTAdaptor {
	logger := log.NewWithModule("consensus")
	cfg, _ := testutil.MockConsensusConfig(logger, ctrl, t)
	cfg.Repo.Config.Consensus.StorageType = typ
	stack, err := NewRBFTAdaptor(cfg)
	assert.Nil(t, err)

	consensusMsgPipes := make(map[int32]network.Pipe, len(consensus.Type_name))
	for id, name := range consensus.Type_name {
		msgPipe, err := stack.config.Network.CreatePipe(context.Background(), "test_pipe_"+name)
		assert.Nil(t, err)
		consensusMsgPipes[id] = msgPipe
	}

	stack.SetMsgPipes(consensusMsgPipes)
	err = stack.UpdateEpoch()
	assert.Nil(t, err)
	return stack
}

func TestSignAndVerify(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	msgSign, err := adaptor.Sign([]byte("test sign"))
	ast.Nil(err)

	err = adaptor.Verify(1, msgSign, []byte("test sign"))
	ast.Nil(err)

	err = adaptor.Verify(2, msgSign, []byte("test sign"))
	ast.Error(err)

	err = adaptor.Verify(1, msgSign, []byte("wrong sign"))
	ast.Error(err)

	wrongSign := msgSign
	wrongSign[0] = 255 - wrongSign[0]
	err = adaptor.Verify(1, wrongSign, []byte("test sign"))
	ast.Error(err)
}

func TestExecute(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	txs := make([]*types.Transaction, 0)
	tx := &types.Transaction{
		Inner: &types.DynamicFeeTx{
			Nonce: 0,
		},
		Time: time.Time{},
	}

	txs = append(txs, tx, tx)
	adaptor.Execute(txs, []bool{true}, uint64(2), time.Now().UnixNano(), 0)
	ready := <-adaptor.ReadyC
	ast.Equal(uint64(2), ready.Height)
}

func TestStateUpdate(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	block2 := testutil.ConstructBlock("block2", uint64(2))
	testutil.SetMockBlockLedger(block2, false)
	defer testutil.ResetMockBlockLedger()

	quorumCkpt := &consensus.SignedCheckpoint{
		Checkpoint: &consensus.Checkpoint{
			ExecuteState: &consensus.Checkpoint_ExecuteState{
				Height: block2.Height(),
				Digest: block2.Hash().String(),
			},
		},
	}
	adaptor.StateUpdate(0, block2.Header.Number, block2.Hash().String(), []*consensus.SignedCheckpoint{quorumCkpt})

	targetB := <-adaptor.BlockC
	ast.Equal(uint64(2), targetB.Block.Header.Number)

	block3 := testutil.ConstructBlock("block3", uint64(3))
	testutil.SetMockBlockLedger(block3, false)

	ckp := &consensus.Checkpoint{
		ExecuteState: &consensus.Checkpoint_ExecuteState{
			Height: block3.Height(),
			Digest: block3.Hash().String(),
		},
	}
	signCkp := &consensus.SignedCheckpoint{
		Checkpoint: ckp,
	}

	t.Run("StateUpdate with receive stop signal", func(t *testing.T) {
		block4 := testutil.ConstructBlock("block4", uint64(4))
		testutil.SetMockBlockLedger(block4, false)

		block5 := testutil.ConstructBlock("block5", uint64(5))
		testutil.SetMockBlockLedger(block5, false)

		adaptor.Cancel()
		time.Sleep(100 * time.Millisecond)
		adaptor.StateUpdate(0, block5.Header.Number, block5.Hash().String(),
			[]*consensus.SignedCheckpoint{signCkp})
	})
}

//func TestStateUpdateWithEpochChange(t *testing.T) {
//	ast := assert.New(t)
//	ctrl := gomock.NewController(t)
//
//	adaptor := mockAdaptor(ctrl, t)
//	block2 := testutil.ConstructBlock("block2", uint64(2))
//	testutil.SetMockBlockLedger(block2, false)
//	defer testutil.ResetMockBlockLedger()
//
//	block3 := testutil.ConstructBlock("block3", uint64(3))
//	testutil.SetMockBlockLedger(block3, false)
//
//	ckp := &consensus.Checkpoint{
//		ExecuteState: &consensus.Checkpoint_ExecuteState{
//			Height: block3.Height(),
//			Digest: block3.Hash().String(),
//		},
//	}
//	signCkp := &consensus.SignedCheckpoint{
//		Checkpoint: ckp,
//	}
//
//	peerSet := []*consensus.QuorumValidator{
//		{
//			Id:     1,
//			PeerId: "1",
//		},
//		{
//			Id:     2,
//			PeerId: "2",
//		},
//		{
//			Id:     3,
//			PeerId: "3",
//		},
//		{
//			Id:     4,
//			PeerId: "5",
//		},
//	}
//
//	// add self to validators
//	peerSet = append(peerSet, &consensus.QuorumValidator{
//		Id:     uint64(0),
//		PeerId: adaptor.network.PeerID(),
//	})
//
//	epochChange := &pb.EpochChange{
//		Checkpoint: &pb.QuorumCheckpoint{Checkpoint: ckp},
//		Validators: &consensus.QuorumValidators{Validators: peerSet},
//	}
//
//	adaptor.StateUpdate(0, block3.Header.Number, block3.Hash().String(),
//		[]*consensus.SignedCheckpoint{signCkp}, epochChange)
//
//	target2 := <-adaptor.BlockC
//	ast.Equal(uint64(2), target2.Block.Header.Number)
//	ast.Equal(block2.Hash().String(), target2.Block.Hash().String())
//
//	target3 := <-adaptor.BlockC
//	ast.Equal(uint64(3), target3.Block.Header.Number)
//	ast.Equal(block3.Hash().String(), target3.Block.Hash().String())
//}

func TestStateUpdateWithRollback(t *testing.T) {
	testutil.ResetMockBlockLedger()
	testutil.ResetMockChainMeta()

	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	adaptor.config.ChainState.ChainMeta = testutil.GetChainMetaFunc()
	block2 := testutil.ConstructBlock("block2", uint64(2))
	testutil.SetMockBlockLedger(block2, false)
	defer testutil.ResetMockBlockLedger()

	block3 := testutil.ConstructBlock("block3", uint64(3))
	testutil.SetMockBlockLedger(block3, false)

	ckp := &consensus.Checkpoint{
		ExecuteState: &consensus.Checkpoint_ExecuteState{
			Height: block3.Height(),
			Digest: block3.Hash().String(),
		},
	}
	signCkp := &consensus.SignedCheckpoint{
		Checkpoint: ckp,
	}

	block4 := testutil.ConstructBlock("block4", uint64(4))
	adaptor.config.ChainState.ChainMeta = &types.ChainMeta{Height: uint64(4), BlockHash: block4.Hash()}
	defer testutil.ResetMockChainMeta()

	testutil.SetMockBlockLedger(block3, true)
	defer testutil.ResetMockBlockLedger()
	adaptor.StateUpdate(2, block3.Header.Number, block3.Hash().String(),
		[]*consensus.SignedCheckpoint{signCkp})

	wrongBlock3 := testutil.ConstructBlock("wrong_block3", uint64(3))
	testutil.SetMockBlockLedger(wrongBlock3, true)
	defer testutil.ResetMockBlockLedger()

	adaptor.StateUpdate(2, block3.Header.Number, block3.Hash().String(),
		[]*consensus.SignedCheckpoint{signCkp})

	target := <-adaptor.BlockC
	ast.Equal(uint64(3), target.Block.Header.Number, "low watermark is 2, we should rollback to 2, and then sync to 3")
	ast.Equal(block3.Hash().String(), target.Block.Hash().String())
}

// refactor this unit test
func TestNetwork(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	msg := &consensus.ConsensusMessage{}
	err := adaptor.Unicast(context.Background(), msg, "1")
	ast.Nil(err)
	err = adaptor.Broadcast(context.Background(), msg)
	ast.Nil(err)

	msg = &consensus.ConsensusMessage{}
	err = adaptor.Unicast(context.Background(), msg, "1")
	ast.Nil(err)

	err = adaptor.Unicast(context.Background(), &consensus.ConsensusMessage{Type: consensus.Type(-1)}, "1")
	ast.Error(err)

	err = adaptor.Broadcast(context.Background(), msg)
	ast.Nil(err)

	err = adaptor.Broadcast(context.Background(), &consensus.ConsensusMessage{Type: consensus.Type(-1)})
	ast.Error(err)

	adaptor.SendFilterEvent(rbfttypes.InformTypeFilterFinishRecovery)
}

func TestEpochService(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	e, err := adaptor.GetEpochInfo(1)
	ast.Nil(err)
	ast.Equal(uint64(1), e.Epoch)

	e, err = adaptor.GetCurrentEpochInfo()
	ast.Nil(err)
	ast.Equal(uint64(1), e.Epoch)
}

func TestRBFTAdaptor_PostCommitEvent(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	adaptor := mockAdaptor(ctrl, t)
	commitC := adaptor.GetCommitChannel()
	adaptor.PostCommitEvent(&common.CommitEvent{
		Block: &types.Block{
			Header: &types.BlockHeader{
				Number: 1,
			},
		},
	})
	commitEvent := <-commitC
	ast.Equal(uint64(1), commitEvent.Block.Header.Number)
}

func TestLedger(t *testing.T) {
	ast := assert.New(t)
	ctrl := gomock.NewController(t)

	testutil.ResetMockBlockLedger()
	block := testutil.ConstructBlock("block1", uint64(1))
	testutil.SetMockBlockLedger(block, true)
	adaptor := mockAdaptor(ctrl, t)
	meta, err := adaptor.GetBlockMeta(1)
	ast.Nil(err)
	ast.NotNil(meta)
	ast.EqualValues(block.Height(), meta.BlockNum)
	ast.EqualValues(block.Hash().String(), meta.BlockHash)
	ast.EqualValues(block.Header.ProposerNodeID, meta.ProcessorNodeID)

	_, err = adaptor.GetBlockMeta(2)
	ast.Error(err)
}
