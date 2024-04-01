package saccount

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/saccount/interfaces"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/internal/ledger/mock_ledger"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"go.uber.org/mock/gomock"
)

const (
	salt           = 88899
	accountBalance = 3000000000000000000
)

func initEntryPoint(t *testing.T) (*EntryPoint, *SmartAccountFactory) {
	mockCtl := gomock.NewController(t)
	stateLedger := mock_ledger.NewMockStateLedger(mockCtl)

	accountMap := make(map[string]*ledger.SimpleAccount)

	stateLedger.EXPECT().GetOrCreateAccount(gomock.Any()).DoAndReturn(func(address *types.Address) ledger.IAccount {
		account, ok := accountMap[address.String()]
		if !ok {
			acc := ledger.NewMockAccount(2, address)
			acc.SetBalance(big.NewInt(accountBalance))
			accountMap[address.String()] = acc
			return acc
		}
		return account
	}).AnyTimes()
	stateLedger.EXPECT().GetBalance(gomock.Any()).DoAndReturn(func(address *types.Address) *big.Int {
		account := accountMap[address.String()]
		return account.GetBalance()
	}).AnyTimes()
	stateLedger.EXPECT().SubBalance(gomock.Any(), gomock.Any()).DoAndReturn(func(address *types.Address, value *big.Int) {
		account := accountMap[address.String()]
		account.SubBalance(value)
	}).AnyTimes()
	stateLedger.EXPECT().AddBalance(gomock.Any(), gomock.Any()).DoAndReturn(func(address *types.Address, value *big.Int) {
		account := accountMap[address.String()]
		account.AddBalance(value)
	}).AnyTimes()

	stateLedger.EXPECT().AddLog(gomock.Any()).AnyTimes()
	stateLedger.EXPECT().GetNonce(gomock.Any()).Return(0).AnyTimes()
	stateLedger.EXPECT().Snapshot().AnyTimes()
	stateLedger.EXPECT().Commit().Return(types.NewHash([]byte("")), nil).AnyTimes()
	stateLedger.EXPECT().Clear().AnyTimes()
	stateLedger.EXPECT().GetNonce(gomock.Any()).Return(uint64(0)).AnyTimes()
	stateLedger.EXPECT().SetNonce(gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().Finalise().AnyTimes()
	stateLedger.EXPECT().Snapshot().Return(0).AnyTimes()
	stateLedger.EXPECT().RevertToSnapshot(0).AnyTimes()
	stateLedger.EXPECT().SetTxContext(gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().GetLogs(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	stateLedger.EXPECT().PrepareBlock(gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().SetBalance(gomock.Any(), gomock.Any()).AnyTimes()
	stateLedger.EXPECT().AddLog(gomock.Any()).AnyTimes()
	stateLedger.EXPECT().GetCodeHash(gomock.Any()).AnyTimes()
	stateLedger.EXPECT().Exist(gomock.Any()).AnyTimes()
	stateLedger.EXPECT().GetRefund().AnyTimes()
	stateLedger.EXPECT().GetCode(gomock.Any()).AnyTimes()

	coinbase := "0xed17543171C1459714cdC6519b58fFcC29A3C3c9"
	blkCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash:     nil,
		Coinbase:    ethcommon.HexToAddress(coinbase),
		BlockNumber: new(big.Int).SetUint64(0),
		Time:        uint64(time.Now().Unix()),
		Difficulty:  big.NewInt(0x2000),
		BaseFee:     big.NewInt(0),
		GasLimit:    0x2fefd8,
		Random:      &ethcommon.Hash{},
	}

	evm := vm.NewEVM(blkCtx, vm.TxContext{}, &ledger.EvmStateDBAdaptor{
		StateLedger: stateLedger,
	}, &params.ChainConfig{
		ChainID: big.NewInt(1356),
	}, vm.Config{})

	entryPoint := NewEntryPoint(&common.SystemContractConfig{})
	context := &common.VMContext{
		CurrentEVM:    evm,
		CurrentHeight: 1,
		CurrentLogs:   new([]common.Log),
		CurrentUser:   nil,
		StateLedger:   stateLedger,
	}
	entryPoint.SetContext(context)

	accountFactory := NewSmartAccountFactory(&common.SystemContractConfig{entryPoint.logger}, entryPoint)
	entryPointAddr := entryPoint.selfAddress().ETHAddress()
	context.CurrentUser = &entryPointAddr
	accountFactory.SetContext(context)
	return entryPoint, accountFactory
}

func buildUserOp(t *testing.T, entryPoint *EntryPoint, accountFactory *SmartAccountFactory) (*interfaces.UserOperation, *ecdsa.PrivateKey) {
	sk, _ := crypto.GenerateKey()
	owner := crypto.PubkeyToAddress(sk.PublicKey)
	initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
	callData := ethcommon.Hex2Bytes("b61d27f6000000000000000000000000ed17543171c1459714cdc6519b58ffcc29a3c3c9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000044a9059cbb000000000000000000000000c7f999b83af6df9e67d0a37ee7e900bf38b3d013000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000")
	// replace owner
	copy(initCode[20+4+(32-20):], owner.Bytes())

	senderAddr := accountFactory.GetAddress(owner, big.NewInt(salt))
	userOp := &interfaces.UserOperation{
		Sender:               senderAddr,
		Nonce:                big.NewInt(0),
		InitCode:             initCode,
		CallData:             callData,
		CallGasLimit:         big.NewInt(0),
		VerificationGasLimit: big.NewInt(9000000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(2000000000000),
		MaxPriorityFeePerGas: big.NewInt(2000000000000),
		PaymasterAndData:     nil,
	}
	userOpHash := entryPoint.GetUserOpHash(*userOp)
	hash := accounts.TextHash(userOpHash)
	sig, err := crypto.Sign(hash, sk)
	assert.Nil(t, err)
	userOp.Signature = sig
	return userOp, sk
}

func TestEntryPoint_SimulateHandleOp(t *testing.T) {
	entryPoint, accountFactory := initEntryPoint(t)
	userOp, sk := buildUserOp(t, entryPoint, accountFactory)

	err := entryPoint.SimulateHandleOp(*userOp, ethcommon.Address{}, nil)
	executeResultErr, ok := err.(*common.RevertError)
	assert.True(t, ok, "can't convert err to RevertError")
	assert.Equal(t, executeResultErr.GetError(), vm.ErrExecutionReverted)
	assert.Contains(t, executeResultErr.Error(), "AA21 didn't pay prefund")

	userOp.InitCode = nil
	userOp.VerificationGasLimit = big.NewInt(90000)
	userOpHash := entryPoint.GetUserOpHash(*userOp)
	hash := accounts.TextHash(userOpHash)
	sig, err := crypto.Sign(hash, sk)
	assert.Nil(t, err)
	userOp.Signature = sig

	err = entryPoint.SimulateHandleOp(*userOp, ethcommon.Address{}, nil)
	executeResultErr, ok = err.(*common.RevertError)
	assert.True(t, ok, "can't convert err to RevertError")
	assert.Equal(t, executeResultErr.GetError(), vm.ErrExecutionReverted)
	assert.Contains(t, executeResultErr.Error(), "error ExecutionResult")
}

func TestEntryPoint_HandleOp_Error(t *testing.T) {
	entryPoint, accountFactory := initEntryPoint(t)
	userOp, sk := buildUserOp(t, entryPoint, accountFactory)

	testcases := []struct {
		BeforeRun  func(userOp *interfaces.UserOperation)
		IsSkipSign bool
		ErrMsg     string
	}{
		{
			BeforeRun: func(userOp *interfaces.UserOperation) {},
			ErrMsg:    "AA21 didn't pay prefund",
		},
		{
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
			},
			ErrMsg: "AA10 sender already constructed",
		},
		{
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
			},
			IsSkipSign: true,
			ErrMsg:     "signature error",
		},
		{
			// not set paymaster signature
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				verifyingPaymaster := NewVerifyingPaymaster(entryPoint)
				paymasterPriv, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(paymasterPriv.PublicKey)
				InitializeVerifyingPaymaster(entryPoint.stateLedger, owner)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				verifyingPaymaster.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				paymasterAndData := ethcommon.HexToAddress(common.VerifyingPaymasterContractAddr).Bytes()
				arg := abi.Arguments{
					{Name: "validUntil", Type: common.UInt48Type},
					{Name: "validAfter", Type: common.UInt48Type},
				}
				validUntil := big.NewInt(time.Now().Add(10 * time.Second).Unix())
				validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
				timeData, _ := arg.Pack(validUntil, validAfter)
				paymasterAndData = append(paymasterAndData, timeData...)
				userOp.PaymasterAndData = paymasterAndData
			},
			ErrMsg: "invalid signature length in paymasterAndData",
		},
		{
			// wrong paymaster signature
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				verifyingPaymaster := NewVerifyingPaymaster(entryPoint)
				paymasterPriv, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(paymasterPriv.PublicKey)
				InitializeVerifyingPaymaster(entryPoint.stateLedger, owner)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				verifyingPaymaster.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				paymasterAndData := ethcommon.HexToAddress(common.VerifyingPaymasterContractAddr).Bytes()
				arg := abi.Arguments{
					{Name: "validUntil", Type: common.UInt48Type},
					{Name: "validAfter", Type: common.UInt48Type},
				}
				validUntil := big.NewInt(time.Now().Add(10 * time.Second).Unix())
				validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
				timeData, _ := arg.Pack(validUntil, validAfter)
				paymasterAndData = append(paymasterAndData, timeData...)
				hash := verifyingPaymaster.getHash(*userOp, validUntil, validAfter)
				// error signature format
				ethHash := accounts.TextHash(hash)
				sig, _ := crypto.Sign(ethHash, sk)
				userOp.PaymasterAndData = append(paymasterAndData, sig...)
			},
			ErrMsg: "AA34 signature error",
		},
		{
			// not enough token balance to pay gas
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				tokenPaymaster := NewTokenPaymaster(entryPoint)
				paymasterPriv, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(paymasterPriv.PublicKey)
				InitializeTokenPaymaster(entryPoint.stateLedger, owner)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				tokenPaymaster.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				paymasterAndData := ethcommon.HexToAddress(common.TokenPaymasterContractAddr).Bytes()
				tokenAddr := ethcommon.HexToAddress(common.AXCContractAddr).Bytes()
				userOp.PaymasterAndData = append(paymasterAndData, tokenAddr...)
			},
			ErrMsg: "not enough token balance",
		},
		{
			// use session key
			// out of session key spending limit
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
				sessionPriv, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sessionPriv.PublicKey)
				userOp.CallData = append([]byte{}, setSessionSig...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(owner.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(big.NewInt(100000).Bytes(), 32)...)
				validUntil := big.NewInt(time.Now().Add(10 * time.Second).Unix())
				validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validAfter.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validUntil.Bytes(), 32)...)
			},
			ErrMsg: "spent amount exceeds session spending limit",
		},
		{
			// use session key
			// out of date
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
				sessionPriv, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sessionPriv.PublicKey)
				userOp.CallData = append([]byte{}, setSessionSig...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(owner.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(big.NewInt(3000000000000000000).Bytes(), 32)...)
				validUntil := big.NewInt(time.Now().Add(-time.Second).Unix())
				validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validAfter.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validUntil.Bytes(), 32)...)
			},
			ErrMsg: "session valid time is out of date",
		},
		{
			// execute batch error
			BeforeRun: func(userOp *interfaces.UserOperation) {
				userOp.VerificationGasLimit = big.NewInt(90000)
				userOp.InitCode = nil
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
				sessionPriv, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sessionPriv.PublicKey)
				userOp.CallData = append([]byte{}, executeBatchSig...)
				// error arguments
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(owner.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(big.NewInt(100000).Bytes(), 32)...)
			},
			ErrMsg: "unpack error",
		},
	}

	for _, testcase := range testcases {
		tmpUserOp := *userOp

		testcase.BeforeRun(&tmpUserOp)

		if !testcase.IsSkipSign {
			userOpHash := entryPoint.GetUserOpHash(tmpUserOp)
			hash := accounts.TextHash(userOpHash)
			sig, err := crypto.Sign(hash, sk)
			assert.Nil(t, err)
			tmpUserOp.Signature = sig
		}

		err := entryPoint.HandleOps([]interfaces.UserOperation{tmpUserOp}, ethcommon.Address{})
		executeResultErr, ok := err.(*common.RevertError)
		assert.True(t, ok, "can't convert err to RevertError")
		assert.Equal(t, executeResultErr.GetError(), vm.ErrExecutionReverted)
		assert.Contains(t, executeResultErr.Error(), testcase.ErrMsg)
	}
}

func TestEntryPoint_HandleOp(t *testing.T) {
	entryPoint, accountFactory := initEntryPoint(t)
	userOp, sk := buildUserOp(t, entryPoint, accountFactory)

	guardianPriv, _ := crypto.GenerateKey()
	guardian := crypto.PubkeyToAddress(guardianPriv.PublicKey)
	newOwnerPriv, _ := crypto.GenerateKey()
	newOwner := crypto.PubkeyToAddress(newOwnerPriv.PublicKey)

	testcases := []struct {
		BeforeRun func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey
		AfterRun  func(userOp *interfaces.UserOperation)
	}{
		{
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				return nil
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				balance := entryPoint.stateLedger.GetBalance(types.NewAddress(userOp.Sender.Bytes()))
				assert.Greater(t, balance.Uint64(), uint64(0))
				assert.Greater(t, uint64(accountBalance), balance.Uint64())
			},
		},
		{
			// set verifying paymaster
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				userOp.InitCode = initCode
				userOp.Sender = accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				verifyingPaymaster := NewVerifyingPaymaster(entryPoint)
				paymasterPriv, _ := crypto.GenerateKey()
				paymasterOwner := crypto.PubkeyToAddress(paymasterPriv.PublicKey)
				InitializeVerifyingPaymaster(entryPoint.stateLedger, paymasterOwner)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				verifyingPaymaster.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				paymasterAndData := ethcommon.HexToAddress(common.VerifyingPaymasterContractAddr).Bytes()
				arg := abi.Arguments{
					{Name: "validUntil", Type: common.UInt48Type},
					{Name: "validAfter", Type: common.UInt48Type},
				}
				validUntil := big.NewInt(time.Now().Add(10 * time.Second).Unix())
				validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
				timeData, _ := arg.Pack(validUntil, validAfter)
				paymasterAndData = append(paymasterAndData, timeData...)
				hash := verifyingPaymaster.getHash(*userOp, validUntil, validAfter)
				ethHash := accounts.TextHash(hash)
				sig, _ := crypto.Sign(ethHash, paymasterPriv)
				userOp.PaymasterAndData = append(paymasterAndData, sig...)

				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				balance := entryPoint.stateLedger.GetBalance(types.NewAddress(userOp.Sender.Bytes()))
				assert.Equal(t, balance.Uint64(), uint64(accountBalance))
				t.Logf("after run, sender %s balance: %s", userOp.Sender.Hex(), balance.Text(10))

				balance = entryPoint.stateLedger.GetBalance(types.NewAddressByStr(common.VerifyingPaymasterContractAddr))
				assert.Greater(t, balance.Uint64(), uint64(0))
				assert.Greater(t, uint64(accountBalance), balance.Uint64())
				t.Logf("after run, verifying paymaster balance: %s", balance.Text(10))
			},
		},
		{
			// set session key
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				userOp.InitCode = initCode
				userOp.Sender = accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				sessionPriv, _ := crypto.GenerateKey()
				owner = crypto.PubkeyToAddress(sessionPriv.PublicKey)
				userOp.CallData = append([]byte{}, setSessionSig...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(owner.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(big.NewInt(1000000000000000000).Bytes(), 32)...)
				validUntil := big.NewInt(time.Now().Add(10 * time.Second).Unix())
				validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validAfter.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validUntil.Bytes(), 32)...)

				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				balance := entryPoint.stateLedger.GetBalance(types.NewAddress(userOp.Sender.Bytes()))
				assert.Greater(t, balance.Uint64(), uint64(0))
				assert.Greater(t, uint64(accountBalance), balance.Uint64())

				// get session
				sa := NewSmartAccount(entryPoint.logger, entryPoint)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				sa.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				sa.InitializeOrLoad(userOp.Sender, ethcommon.Address{}, ethcommon.Address{})
				session := sa.getSession()
				t.Logf("after run, session: %v", session)
				assert.NotNil(t, session)
				assert.Greater(t, session.SpentAmount.Uint64(), uint64(0))
			},
		},
		{
			// initialize smart account with guardian
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				// append guardian
				initCode = append(initCode, ethcommon.LeftPadBytes(guardian.Bytes(), 32)...)
				userOp.InitCode = initCode
				senderAddr := accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Sender = senderAddr
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				sa := NewSmartAccount(entryPoint.logger, entryPoint)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				sa.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				sa.InitializeOrLoad(userOp.Sender, ethcommon.Address{}, ethcommon.Address{})

				g := sa.getGuardian()
				assert.Equal(t, g, guardian)
			},
		},
		{
			// set guardian
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				userOp.InitCode = initCode
				senderAddr := accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Sender = senderAddr
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				userOp.CallData = append([]byte{}, setGuardianSig...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(userOp.Sender.Bytes(), 32)...)
				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				sa := NewSmartAccount(entryPoint.logger, entryPoint)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				sa.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				sa.InitializeOrLoad(userOp.Sender, ethcommon.Address{}, ethcommon.Address{})

				g := sa.getGuardian()
				assert.Equal(t, g, userOp.Sender)
			},
		},
		{
			// reset owner
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				userOp.InitCode = initCode
				senderAddr := accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Sender = senderAddr
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				userOp.CallData = append([]byte{}, resetOwnerSig...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(newOwner.Bytes(), 32)...)
				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				sa := NewSmartAccount(entryPoint.logger, entryPoint)
				entryPointAddr := entryPoint.selfAddress().ETHAddress()
				sa.SetContext(&common.VMContext{
					CurrentEVM:  entryPoint.currentEVM,
					StateLedger: entryPoint.stateLedger,
					CurrentUser: &entryPointAddr,
					CurrentLogs: entryPoint.currentLogs,
				})
				sa.InitializeOrLoad(userOp.Sender, ethcommon.Address{}, ethcommon.Address{})

				owner := sa.getOwner()
				assert.Equal(t, owner, newOwner)
			},
		},
		{
			// call smart account execute
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				userOp.InitCode = initCode
				senderAddr := accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Sender = senderAddr
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				userOp.CallData = append([]byte{}, executeSig...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(entryPoint.selfAddr.Bytes(), 32)...)
				userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(big.NewInt(100000).Bytes(), 32)...)
				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
				balance := entryPoint.stateLedger.GetBalance(entryPoint.selfAddress())
				assert.Greater(t, balance.Uint64(), uint64(100000))
			},
		},
		{
			// call smart account execute batch
			BeforeRun: func(userOp *interfaces.UserOperation) *ecdsa.PrivateKey {
				userOp.VerificationGasLimit = big.NewInt(90000)
				sk, _ := crypto.GenerateKey()
				owner := crypto.PubkeyToAddress(sk.PublicKey)
				initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
				// replace owner
				copy(initCode[20+4+(32-20):], owner.Bytes())
				userOp.InitCode = initCode
				senderAddr := accountFactory.GetAddress(owner, big.NewInt(salt))
				userOp.Sender = senderAddr
				userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

				userOp.CallData = append([]byte{}, executeBatchSig...)
				data, err := executeBatchMethod.Pack([]ethcommon.Address{guardian, newOwner}, [][]byte{nil, nil})
				assert.Nil(t, err)
				userOp.CallData = append(userOp.CallData, data...)
				return sk
			},
			AfterRun: func(userOp *interfaces.UserOperation) {
			},
		},
	}

	beneficiaryPriv, _ := crypto.GenerateKey()
	beneficiary := crypto.PubkeyToAddress(beneficiaryPriv.PublicKey)
	for _, testcase := range testcases {
		tmpUserOp := *userOp

		priv := testcase.BeforeRun(&tmpUserOp)

		if priv == nil {
			priv = sk
		}

		userOpHash := entryPoint.GetUserOpHash(tmpUserOp)
		hash := accounts.TextHash(userOpHash)
		sig, err := crypto.Sign(hash, priv)
		assert.Nil(t, err)
		tmpUserOp.Signature = sig

		err = entryPoint.HandleOps([]interfaces.UserOperation{tmpUserOp}, beneficiary)
		assert.Nil(t, err)
		testcase.AfterRun(&tmpUserOp)
	}
}

func TestEntryPoint_HandleOps_SessionKey(t *testing.T) {
	entryPoint, accountFactory := initEntryPoint(t)
	userOp, _ := buildUserOp(t, entryPoint, accountFactory)
	beneficiaryPriv, _ := crypto.GenerateKey()
	beneficiary := crypto.PubkeyToAddress(beneficiaryPriv.PublicKey)

	userOp.VerificationGasLimit = big.NewInt(90000)
	sk, _ := crypto.GenerateKey()
	owner := crypto.PubkeyToAddress(sk.PublicKey)
	initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
	// replace owner
	copy(initCode[20+4+(32-20):], owner.Bytes())
	userOp.InitCode = initCode
	userOp.Sender = accountFactory.GetAddress(owner, big.NewInt(salt))
	userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

	sessionPriv, _ := crypto.GenerateKey()
	sessionOwner := crypto.PubkeyToAddress(sessionPriv.PublicKey)
	userOp.CallData = append([]byte{}, setSessionSig...)
	userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(sessionOwner.Bytes(), 32)...)
	userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(big.NewInt(1000000000000000000).Bytes(), 32)...)
	validUntil := big.NewInt(time.Now().Add(10 * time.Second).Unix())
	validAfter := big.NewInt(time.Now().Add(-time.Second).Unix())
	userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validAfter.Bytes(), 32)...)
	userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(validUntil.Bytes(), 32)...)

	userOpHash := entryPoint.GetUserOpHash(*userOp)
	hash := accounts.TextHash(userOpHash)
	sig, err := crypto.Sign(hash, sk)
	assert.Nil(t, err)
	userOp.Signature = sig

	// use session key to sign
	sessionUserOp := *userOp
	sessionUserOp.VerificationGasLimit = big.NewInt(90000)
	sessionUserOp.InitCode = nil
	sessionUserOp.Nonce = new(big.Int).Add(userOp.Nonce, big.NewInt(1))
	sessionUserOp.CallData = append([]byte{}, executeSig...)
	sessionUserOp.CallData = append(sessionUserOp.CallData, ethcommon.LeftPadBytes(entryPoint.selfAddr.Bytes(), 32)...)
	sessionUserOp.CallData = append(sessionUserOp.CallData, ethcommon.LeftPadBytes(big.NewInt(100000).Bytes(), 32)...)
	userOpHash = entryPoint.GetUserOpHash(sessionUserOp)
	hash = accounts.TextHash(userOpHash)
	sessionKeySig, err := crypto.Sign(hash, sessionPriv)
	assert.Nil(t, err)
	sessionUserOp.Signature = sessionKeySig

	err = entryPoint.HandleOps([]interfaces.UserOperation{*userOp}, beneficiary)
	assert.Nil(t, err)

	// after set session key effect, use session key to sign
	err = entryPoint.HandleOps([]interfaces.UserOperation{sessionUserOp}, beneficiary)
	assert.Nil(t, err)
}

func TestEntryPoint_HandleOps_Recovery(t *testing.T) {
	// use guardian to recovery owner
	entryPoint, accountFactory := initEntryPoint(t)
	userOp, _ := buildUserOp(t, entryPoint, accountFactory)
	beneficiaryPriv, _ := crypto.GenerateKey()
	beneficiary := crypto.PubkeyToAddress(beneficiaryPriv.PublicKey)

	userOp.VerificationGasLimit = big.NewInt(90000)
	sk, _ := crypto.GenerateKey()
	owner := crypto.PubkeyToAddress(sk.PublicKey)
	initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
	// replace owner
	copy(initCode[20+4+(32-20):], owner.Bytes())
	userOp.InitCode = initCode
	userOp.Sender = accountFactory.GetAddress(owner, big.NewInt(salt))
	userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))

	// set guardian
	guardianPriv, _ := crypto.GenerateKey()
	guardianOwner := crypto.PubkeyToAddress(guardianPriv.PublicKey)
	userOp.CallData = append([]byte{}, setGuardianSig...)
	userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(guardianOwner.Bytes(), 32)...)

	userOpHash := entryPoint.GetUserOpHash(*userOp)
	hash := accounts.TextHash(userOpHash)
	sig, err := crypto.Sign(hash, sk)
	assert.Nil(t, err)
	userOp.Signature = sig

	err = entryPoint.HandleOps([]interfaces.UserOperation{*userOp}, beneficiary)
	assert.Nil(t, err)

	// use guardian to sign to recovery owner
	userOp.InitCode = nil
	userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
	newOwnerPriv, _ := crypto.GenerateKey()
	newOwner := crypto.PubkeyToAddress(newOwnerPriv.PublicKey)
	// call HandleAccountRecovery, directly set new owner
	userOp.CallData = append([]byte{}, newOwner.Bytes()...)

	// guardian sign
	userOpHash = entryPoint.GetUserOpHash(*userOp)
	hash = accounts.TextHash(userOpHash)
	sig, err = crypto.Sign(hash, guardianPriv)
	assert.Nil(t, err)
	userOp.Signature = sig

	// tmp set lock time
	LockedTime = 2 * time.Second
	err = entryPoint.HandleAccountRecovery([]interfaces.UserOperation{*userOp}, beneficiary)
	assert.Nil(t, err)

	// use new owner would failed
	userOp.InitCode = nil
	userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
	guardianPriv, _ = crypto.GenerateKey()
	guardianOwner = crypto.PubkeyToAddress(guardianPriv.PublicKey)
	userOp.CallData = append([]byte{}, setGuardianSig...)
	userOp.CallData = append(userOp.CallData, ethcommon.LeftPadBytes(guardianOwner.Bytes(), 32)...)
	userOpHash = entryPoint.GetUserOpHash(*userOp)
	hash = accounts.TextHash(userOpHash)
	sig, err = crypto.Sign(hash, newOwnerPriv)
	assert.Nil(t, err)
	userOp.Signature = sig
	err = entryPoint.HandleOps([]interfaces.UserOperation{*userOp}, beneficiary)
	assert.Contains(t, err.Error(), "signature error")

	// after lock time, new owner would be success
	time.Sleep(LockedTime + time.Second)
	// update evm block time
	entryPoint.currentEVM.Context.Time = uint64(time.Now().Unix())
	userOp.Nonce = entryPoint.GetNonce(userOp.Sender, big.NewInt(salt))
	userOpHash = entryPoint.GetUserOpHash(*userOp)
	hash = accounts.TextHash(userOpHash)
	sig, err = crypto.Sign(hash, newOwnerPriv)
	assert.Nil(t, err)
	userOp.Signature = sig
	err = entryPoint.HandleOps([]interfaces.UserOperation{*userOp}, beneficiary)
	assert.Nil(t, err)
}

func TestEntryPoint_call(t *testing.T) {
	entryPoint, _ := initEntryPoint(t)
	// test call method
	to := ethcommon.HexToAddress(common.AccountFactoryContractAddr)
	res, usedGas, err := call(entryPoint.stateLedger, entryPoint.currentEVM, big.NewInt(300000), types.NewAddressByStr(common.EntryPointContractAddr), &to, []byte(""))
	assert.Nil(t, err)
	assert.Nil(t, res)
	assert.EqualValues(t, usedGas, 300000)

	_, usedGas, err = callWithValue(entryPoint.stateLedger, entryPoint.currentEVM, big.NewInt(300000), big.NewInt(10000), types.NewAddressByStr(common.EntryPointContractAddr), &to, []byte(""))
	assert.Nil(t, err)
	assert.EqualValues(t, usedGas, 300000)
}

func TestEntryPoint_GetUserOpHash(t *testing.T) {
	entryPoint, _ := initEntryPoint(t)
	dstUserOpHash := "c02fe0170a5402c0d08502e2140d43803ce1672500b9100ac95aea6e7e2bd508"
	sk, err := crypto.HexToECDSA("8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba")
	assert.Nil(t, err)
	owner := crypto.PubkeyToAddress(sk.PublicKey)

	t.Logf("owner addr: %s", owner.String())

	initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
	// replace owner
	copy(initCode[20+4+(32-20):], owner.Bytes())
	t.Logf("initCode addr: %v", initCode)
	callData := ethcommon.Hex2Bytes("b61d27f60000000000000000000000008464135c8f25da09e49bc8782676a84730c318bc000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000044a9059cbb000000000000000000000000c7f999b83af6df9e67d0a37ee7e900bf38b3d013000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000")

	senderAddr, _, err := entryPoint.createSender(big.NewInt(10000), initCode)
	assert.Nil(t, err)
	t.Logf("sender addr: %s", senderAddr.String())

	userOp := &interfaces.UserOperation{
		Sender:               senderAddr,
		Nonce:                big.NewInt(0),
		InitCode:             initCode,
		CallData:             callData,
		CallGasLimit:         big.NewInt(12100),
		VerificationGasLimit: big.NewInt(21971),
		PreVerificationGas:   big.NewInt(50579),
		MaxFeePerGas:         big.NewInt(5875226332498),
		MaxPriorityFeePerGas: big.NewInt(0),
		PaymasterAndData:     nil,
		Signature:            ethcommon.Hex2Bytes("70aae8a31ee824b05e449e082f9fcf848218fe4563988d426e72d4675d1179b9735c026f847eb22e6b0f1a8dc81825678f383b07189c5df6a4a513a972ad71191c"),
	}

	userOpHash := entryPoint.GetUserOpHash(*userOp)
	t.Logf("op hash: %x", userOpHash)
	t.Logf("chain id: %s", entryPoint.currentEVM.ChainConfig().ChainID.Text(10))
	assert.Equal(t, ethcommon.Hex2Bytes(dstUserOpHash), userOpHash)

	hash := accounts.TextHash(userOpHash)
	// 0xc911ab31b05daa531d9ef77fc2670e246a9489610e48e88d8296604c26dcd5c4
	t.Logf("eth hash: %x", hash)
	sig, err := crypto.Sign(hash, sk)
	assert.Nil(t, err)
	// js return r|s|v, v only 1 byte
	// golang return rid, v = rid +27
	msgSig := make([]byte, 65)
	copy(msgSig, sig)
	msgSig[64] += 27
	assert.Equal(t, msgSig, userOp.Signature)

	recoveredPub, err := crypto.Ecrecover(hash, sig)
	assert.Nil(t, err)
	pubKey, _ := crypto.UnmarshalPubkey(recoveredPub)
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)
	assert.Equal(t, owner, recoveredAddr)
}

func TestEntryPoint_createSender(t *testing.T) {
	entryPoint, _ := initEntryPoint(t)
	initCode := ethcommon.Hex2Bytes("00000000000000000000000000000000000010095fbfb9cf0000000000000000000000009965507d1a55bcc2695c58ba16fb37d819b0a4dc0000000000000000000000000000000000000000000000000000000000015b43")
	addr, _, err := entryPoint.createSender(big.NewInt(10000), initCode)
	assert.Nil(t, err)

	assert.NotNil(t, addr)
	assert.NotEqual(t, addr, ethcommon.Address{})

	addr2, _, err := entryPoint.createSender(big.NewInt(10000), initCode)
	assert.Nil(t, err)
	assert.Equal(t, addr, addr2)
}

func TestEntryPoint_ExecutionResult(t *testing.T) {
	err := interfaces.ExecutionResult(big.NewInt(1000), big.NewInt(999), big.NewInt(0), big.NewInt(0), false, nil)
	_, ok := err.(*common.RevertError)
	assert.True(t, ok)
	t.Logf("err is : %s", err)
}

func TestEntryPoint_userOpEvent(t *testing.T) {
	userOpEvent := abi.NewEvent("UserOperationEvent", "UserOperationEvent", false, abi.Arguments{
		{Name: "userOpHash", Type: common.Bytes32Type, Indexed: true},
		{Name: "sender", Type: common.AddressType, Indexed: true},
		{Name: "paymaster", Type: common.AddressType, Indexed: true},
		{Name: "nonce", Type: common.BigIntType},
		{Name: "success", Type: common.BoolType},
		{Name: "actualGasCost", Type: common.BigIntType},
		{Name: "actualGasUsed", Type: common.BigIntType},
	})

	assert.Equal(t, "0x49628fd1471006c1482da88028e9ce4dbb080b815c9b0344d39e5a8e6ec1419f", types.NewHash(userOpEvent.ID.Bytes()).String())
}
