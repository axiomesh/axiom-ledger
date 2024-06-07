package system

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/assert"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/governance"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/saccount"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/saccount/interfaces"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

const (
	admin1 = "0x1210000000000000000000000000000000000000"
	admin2 = "0x1220000000000000000000000000000000000000"
	admin3 = "0x1230000000000000000000000000000000000000"
	admin4 = "0x1240000000000000000000000000000000000000"
)

func TestContractRun(t *testing.T) {
	rep := repo.MockRepo(t)
	lg, err := ledger.NewMemory(rep)
	assert.Nil(t, err)

	nvm := New()
	err = nvm.GenesisInit(rep.GenesisConfig, lg.StateLedger)
	assert.Nil(t, err)

	from := types.NewAddressByStr(admin1).ETHAddress()
	to := types.NewAddressByStr(common.GovernanceContractAddr).ETHAddress()
	data := hexutil.Bytes(generateVoteData(t, 1, governance.Pass))
	getIDData := hexutil.Bytes(generateGetLatestProposalIDData(t))
	getUserOpHash := hexutil.Bytes(generateGetUserOpHashData(t))
	entrypointAddr := types.NewAddressByStr(common.EntryPointContractAddr).ETHAddress()

	coinbase := "0xed17543171C1459714cdC6519b58fFcC29A3C3c9"
	blkCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash:     nil,
		Coinbase:    ethcommon.HexToAddress(coinbase),
		BlockNumber: new(big.Int).SetUint64(1),
		Time:        uint64(time.Now().Unix()),
		Difficulty:  big.NewInt(0x2000),
		BaseFee:     big.NewInt(0),
		GasLimit:    0x2fefd8,
		Random:      &ethcommon.Hash{},
	}

	shanghaiTime := uint64(0)
	CancunTime := uint64(0)
	PragueTime := uint64(0)

	evm := vm.NewEVM(blkCtx, vm.TxContext{
		Origin:   entrypointAddr,
		GasPrice: big.NewInt(100000000000),
	}, &ledger.EvmStateDBAdaptor{
		StateLedger: lg.StateLedger,
	}, &params.ChainConfig{
		ChainID:                 big.NewInt(1356),
		HomesteadBlock:          big.NewInt(0),
		EIP150Block:             big.NewInt(0),
		EIP155Block:             big.NewInt(0),
		EIP158Block:             big.NewInt(0),
		ByzantiumBlock:          big.NewInt(0),
		ConstantinopleBlock:     big.NewInt(0),
		PetersburgBlock:         big.NewInt(0),
		IstanbulBlock:           big.NewInt(0),
		MuirGlacierBlock:        big.NewInt(0),
		BerlinBlock:             big.NewInt(0),
		LondonBlock:             big.NewInt(0),
		ArrowGlacierBlock:       big.NewInt(0),
		MergeNetsplitBlock:      big.NewInt(0),
		TerminalTotalDifficulty: big.NewInt(0),
		ShanghaiTime:            &shanghaiTime,
		CancunTime:              &CancunTime,
		PragueTime:              &PragueTime,
	}, vm.Config{})

	testcases := []struct {
		Message       *core.Message
		IsExpectedErr bool
	}{
		{
			Message: &core.Message{
				From: from,
				To:   nil,
				Data: data,
			},
			IsExpectedErr: true,
		},
		{
			Message: &core.Message{
				From: from,
				To:   &to,
				Data: data,
			},
			IsExpectedErr: true,
		},
		{
			Message: &core.Message{
				From: from,
				To:   &to,
				Data: hexutil.Bytes("error data"), //fallback
			},
			IsExpectedErr: false,
		},
		{
			Message: &core.Message{
				From: from,
				To:   &to,
				Data: crypto.Keccak256([]byte("vote(uint64,uint8)")),
			},
			IsExpectedErr: true,
		},
		{
			Message: &core.Message{
				From: from,
				To:   &to,
				Data: getIDData,
			},
			IsExpectedErr: false,
		},
		{
			Message: &core.Message{
				From: from,
				To:   &from,
				Data: data,
			},
			IsExpectedErr: true,
		},
		{
			Message: &core.Message{
				From: from,
				To:   &entrypointAddr,
				Data: getUserOpHash,
			},
			IsExpectedErr: false,
		},
	}
	for i, testcase := range testcases {
		t.Run(fmt.Sprintf("testcase %d", i), func(t *testing.T) {
			args := &vm.StatefulArgs{
				From: testcase.Message.From,
				To:   testcase.Message.To,
				EVM:  evm,
			}
			_, err := nvm.Run(testcase.Message.Data, args)
			if testcase.IsExpectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestNativeVM_PackOutputArgs(t *testing.T) {
	nvm := New()

	data := "0x8e604d15a98b7f944a25c1d8e463c1583fb7a24bf222ad76ac3aae3079b135b2"
	hash := crypto.Keccak256Hash([]byte(data))

	outputs, err := nvm.packOutputArgs(saccount.EntryPointBuildConfig.StaticConfig(), "getUserOpHash", hash)
	assert.Nil(t, err)
	assert.NotNil(t, outputs)
}

func generateVoteData(t *testing.T, proposalID uint64, voteResult governance.VoteResult) []byte {
	gabi := governance.BuildConfig.StaticConfig().GetAbi()

	data, err := gabi.Pack("vote", proposalID, voteResult)
	assert.Nil(t, err)

	return data
}

func generateGetUserOpHashData(t *testing.T) []byte {
	gabi := saccount.EntryPointBuildConfig.StaticConfig().GetAbi()

	data, err := gabi.Pack("getUserOpHash", interfaces.UserOperation{
		Sender:               types.NewAddressByStr(admin1).ETHAddress(),
		Nonce:                big.NewInt(0),
		CallGasLimit:         big.NewInt(100000),
		VerificationGasLimit: big.NewInt(10000),
		PreVerificationGas:   big.NewInt(10000),
		MaxFeePerGas:         big.NewInt(10000),
		MaxPriorityFeePerGas: big.NewInt(10000),
	})
	assert.Nil(t, err)

	return data
}

func generateGetLatestProposalIDData(t *testing.T) []byte {
	gabi := governance.BuildConfig.StaticConfig().GetAbi()

	data, err := gabi.Pack(governance.GetLatestProposalIDMethod)
	assert.Nil(t, err)

	return data
}

func TestNativeVM_RequiredGas(t *testing.T) {
	nvm := New()
	gas := nvm.RequiredGas([]byte{1})
	assert.Equal(t, uint64(RunSystemContractGas), gas)
}

func Test_convertInputArgs(t *testing.T) {
	op := struct {
		Sender               ethcommon.Address `json:"sender"`
		Nonce                *big.Int          `json:"nonce"`
		InitCode             []byte            `json:"initCode"`
		CallData             []byte            `json:"callData"`
		CallGasLimit         *big.Int          `json:"callGasLimit"`
		VerificationGasLimit *big.Int          `json:"verificationGasLimit"`
		PreVerificationGas   *big.Int          `json:"preVerificationGas"`
		MaxFeePerGas         *big.Int          `json:"maxFeePerGas"`
		MaxPriorityFeePerGas *big.Int          `json:"maxPriorityFeePerGas"`
		PaymasterAndData     []byte            `json:"paymasterAndData"`
		Signature            []byte            `json:"signature"`
	}{
		Sender:               ethcommon.HexToAddress("0x9624CE13b5E63cfAb1369b7f832B36128242454E"),
		Nonce:                big.NewInt(0),
		InitCode:             []byte("0x00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43"),
		CallData:             []byte("0xb61d27f60000000000000000000000008464135c8f25da09e49bc8782676a84730c318bc000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000044a9059cbb000000000000000000000000c7f999b83af6df9e67d0a37ee7e900bf38b3d013000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000"),
		CallGasLimit:         big.NewInt(0),
		VerificationGasLimit: big.NewInt(10000000),
		PreVerificationGas:   big.NewInt(10000000),
		MaxFeePerGas:         big.NewInt(10000000),
		MaxPriorityFeePerGas: big.NewInt(10000000),
		PaymasterAndData:     []byte{},
		Signature:            []byte("0x70aae8a31ee824b05e449e082f9fcf848218fe4563988d426e72d4675d1179b9735c026f847eb22e6b0f1a8dc81825678f383b07189c5df6a4a513a972ad71191"),
	}

	rep := repo.MockRepo(t)
	lg, err := ledger.NewMemory(rep)
	assert.Nil(t, err)

	// call entrypoint SimulateHandleOp
	entrypoint := saccount.EntryPointBuildConfig.Build(common.NewViewVMContext(lg.StateLedger))
	method := reflect.ValueOf(entrypoint).MethodByName("SimulateHandleOp")
	var inputs []reflect.Value
	inputs = append(inputs, reflect.ValueOf(op))
	inputs = append(inputs, reflect.ValueOf(ethcommon.Address{}))
	inputs = append(inputs, reflect.ValueOf([]byte("")))
	outputs := convertInputArgs(method, inputs)
	assert.Equal(t, len(inputs), len(outputs))

	ops := []struct {
		Sender               ethcommon.Address `json:"sender"`
		Nonce                *big.Int          `json:"nonce"`
		InitCode             []byte            `json:"initCode"`
		CallData             []byte            `json:"callData"`
		CallGasLimit         *big.Int          `json:"callGasLimit"`
		VerificationGasLimit *big.Int          `json:"verificationGasLimit"`
		PreVerificationGas   *big.Int          `json:"preVerificationGas"`
		MaxFeePerGas         *big.Int          `json:"maxFeePerGas"`
		MaxPriorityFeePerGas *big.Int          `json:"maxPriorityFeePerGas"`
		PaymasterAndData     []byte            `json:"paymasterAndData"`
		Signature            []byte            `json:"signature"`
	}{
		{
			Sender:               ethcommon.HexToAddress("0x9624CE13b5E63cfAb1369b7f832B36128242454E"),
			Nonce:                big.NewInt(0),
			InitCode:             []byte("0x00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43"),
			CallData:             []byte("0xb61d27f60000000000000000000000008464135c8f25da09e49bc8782676a84730c318bc000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000044a9059cbb000000000000000000000000c7f999b83af6df9e67d0a37ee7e900bf38b3d013000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000"),
			CallGasLimit:         big.NewInt(0),
			VerificationGasLimit: big.NewInt(10000000),
			PreVerificationGas:   big.NewInt(10000000),
			MaxFeePerGas:         big.NewInt(10000000),
			MaxPriorityFeePerGas: big.NewInt(10000000),
			PaymasterAndData:     []byte{},
			Signature:            []byte("0x70aae8a31ee824b05e449e082f9fcf848218fe4563988d426e72d4675d1179b9735c026f847eb22e6b0f1a8dc81825678f383b07189c5df6a4a513a972ad71191"),
		},
	}

	// call entrypoint HandleOps
	entrypoint = saccount.EntryPointBuildConfig.Build(common.NewViewVMContext(lg.StateLedger))
	method = reflect.ValueOf(entrypoint).MethodByName("HandleOps")
	var inputArgs []reflect.Value
	inputArgs = append(inputArgs, reflect.ValueOf(ops))
	inputArgs = append(inputArgs, reflect.ValueOf(ethcommon.Address{}))

	outputs = convertInputArgs(method, inputs)
	assert.Equal(t, len(inputs), len(outputs))
}
