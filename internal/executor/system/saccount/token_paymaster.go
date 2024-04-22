package saccount

import (
	"fmt"
	"math/big"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/common"
	"github.com/axiomesh/axiom-ledger/internal/executor/system/saccount/interfaces"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sirupsen/logrus"
)

const (
	tokenOwnerKey  = "token_owner_key"
	tokenOracleKey = "token_oracle_key"
)

var (
	balanceOfSig          = crypto.Keccak256([]byte("balanceOf(address)"))[:4]
	transferFromSig       = crypto.Keccak256([]byte("transferFrom(address,address,uint256)"))[:4]
	getTokenValueOfAxcSig = crypto.Keccak256([]byte("getTokenValueOfAXC(address,uint256)"))[:4]
	decimalsSig           = crypto.Keccak256([]byte("decimals()"))[:4]

	contextArg = abi.Arguments{
		{Name: "account", Type: common.AddressType},
		{Name: "token", Type: common.AddressType},
		{Name: "gasPriceUserOp", Type: common.BigIntType},
		{Name: "maxTokenCost", Type: common.BigIntType},
		{Name: "maxCost", Type: common.BigIntType},
	}
)

var _ interfaces.IPaymaster = (*TokenPaymaster)(nil)

type TokenPaymaster struct {
	entryPoint interfaces.IEntryPoint
	selfAddr   *types.Address
	logger     logrus.FieldLogger

	account ledger.IAccount

	// context fields
	currentUser *ethcommon.Address

	currentLogs *[]common.Log
	stateLedger ledger.StateLedger
	currentEVM  *vm.EVM
}

func NewTokenPaymaster(entryPoint interfaces.IEntryPoint) *TokenPaymaster {
	return &TokenPaymaster{
		entryPoint: entryPoint,
		selfAddr:   types.NewAddressByStr(common.TokenPaymasterContractAddr),
		logger:     loggers.Logger(loggers.SystemContract),
	}
}

func InitializeTokenPaymaster(stateLedger ledger.StateLedger, owner ethcommon.Address) {
	account := stateLedger.GetOrCreateAccount(types.NewAddressByStr(common.TokenPaymasterContractAddr))
	account.SetState([]byte(tokenOwnerKey), owner.Bytes())
}

func (tp *TokenPaymaster) GetOwner() ethcommon.Address {
	isExist, ownerBytes := tp.account.GetState([]byte(tokenOwnerKey))
	if isExist {
		return ethcommon.BytesToAddress(ownerBytes)
	}
	return ethcommon.Address{}
}

func (tp *TokenPaymaster) SetContext(context *common.VMContext) {
	tp.currentUser = context.CurrentUser
	tp.currentLogs = context.CurrentLogs
	tp.stateLedger = context.StateLedger
	tp.currentEVM = context.CurrentEVM

	tp.account = tp.stateLedger.GetOrCreateAccount(tp.selfAddress())
}

func (tp *TokenPaymaster) selfAddress() *types.Address {
	return tp.selfAddr
}

func (tp *TokenPaymaster) AddToken(token, tokenPriceOracle ethcommon.Address) error {
	if tp.currentUser.Hex() != tp.GetOwner().Hex() {
		return common.NewRevertStringError("only owner can add token")
	}

	key := append([]byte(tokenOracleKey), token.Bytes()...)
	if isExist, _ := tp.account.GetState(key); isExist {
		return common.NewRevertStringError("token already set")
	}

	tp.account.SetState(key, tokenPriceOracle.Bytes())
	return nil
}

func (tp *TokenPaymaster) GetToken(token ethcommon.Address) ethcommon.Address {
	key := append([]byte(tokenOracleKey), token.Bytes()...)
	if isExist, oracleBytes := tp.account.GetState(key); isExist {
		return ethcommon.BytesToAddress(oracleBytes)
	}

	return ethcommon.Address{}
}

// PostOp implements interfaces.IPaymaster.
func (tp *TokenPaymaster) PostOp(mode interfaces.PostOpMode, context []byte, actualGasCost *big.Int) error {
	if tp.currentUser.Hex() != common.EntryPointContractAddr {
		return common.NewRevertStringError("only entrypoint can call post op")
	}

	account, tokenAddr, _, maxTokenCost, maxCost, err := decodeContext(context)
	if err != nil {
		return common.NewRevertStringError(fmt.Sprintf("token paymaster: decode context failed: %s", err.Error()))
	}

	if maxCost.Sign() == 0 {
		// max cost is 0, no need to transfer token
		return nil
	}

	// use same conversion rate as used for validation.
	actualTokenCost := new(big.Int).Div(new(big.Int).Mul(actualGasCost, maxTokenCost), maxCost)
	// transfer token as gas fee from account to paymaster
	return tp.transferFrom(account, tokenAddr, actualTokenCost)
}

// ValidatePaymasterUserOp implements interfaces.IPaymaster.
func (tp *TokenPaymaster) ValidatePaymasterUserOp(userOp interfaces.UserOperation, userOpHash []byte, maxCost *big.Int) (context []byte, validationData *big.Int, err error) {
	if tp.currentUser.Hex() != common.EntryPointContractAddr {
		return nil, nil, common.NewRevertStringError("only entrypoint can call validate paymaster user op")
	}

	context, validation, err := tp.validatePaymasterUserOp(userOp, userOpHash, maxCost)
	return context, big.NewInt(int64(validation.SigValidation)), err
}

// nolint
func (tp *TokenPaymaster) validatePaymasterUserOp(userOp interfaces.UserOperation, userOpHash []byte, maxCost *big.Int) (context []byte, validation *interfaces.Validation, err error) {
	validation = &interfaces.Validation{SigValidation: interfaces.SigValidationFailed}
	paymasterAndData := userOp.PaymasterAndData
	if len(paymasterAndData) < 20+20 {
		return nil, validation, common.NewRevertStringError("token paymaster: paymasterAndData must specify token")
	}

	tokenAddr := ethcommon.BytesToAddress(paymasterAndData[20:40])
	maxTokenCost, err := tp.getTokenValueOfAxc(tokenAddr, maxCost)
	if err != nil {
		return nil, validation, common.NewRevertStringError(fmt.Sprintf("token paymaster: get token %s value failed: %s", tokenAddr.String(), err.Error()))
	}
	balance, err := tp.getTokenBalance(userOp.Sender, tokenAddr)
	if err != nil {
		return nil, validation, common.NewRevertStringError(fmt.Sprintf("token paymaster: get token balance failed: %s", err.Error()))
	}
	if balance.Cmp(maxTokenCost) < 0 {
		return nil, validation, common.NewRevertStringError(fmt.Sprintf("token paymaster: not enough token balance, balance: %s, maxTokenCost: %s", balance.String(), maxTokenCost.String()))
	}
	context, err = encodeContext(userOp.Sender, tokenAddr, interfaces.GetGasPrice(&userOp), maxTokenCost, maxCost)
	if err != nil {
		return nil, validation, common.NewRevertStringError(fmt.Sprintf("token paymaster: encode context failed: %s", err.Error()))
	}
	tp.logger.Infof("token paymaster: validate paymaster user op success, sender: %s, token addr: %s, maxTokenCost: %s, maxCost: %s, balance: %s", userOp.Sender.String(), tokenAddr.String(), maxTokenCost.String(), maxCost.String(), balance.String())
	validation.SigValidation = interfaces.SigValidationSucceeded
	return context, validation, err
}

// getTokenValueOfAxc translate the give axc value to token value
func (tp *TokenPaymaster) getTokenValueOfAxc(token ethcommon.Address, value *big.Int) (*big.Int, error) {
	key := append([]byte(tokenOracleKey), token.Bytes()...)
	if isExist, oracleBytes := tp.account.GetState(key); isExist {
		oracleAddr := ethcommon.BytesToAddress(oracleBytes)
		callData := append(getTokenValueOfAxcSig, ethcommon.LeftPadBytes(token.Bytes(), 32)...)
		callData = append(callData, ethcommon.LeftPadBytes(value.Bytes(), 32)...)
		// return value of token is 10 decimals
		tokenValueRes, _, err := call(tp.stateLedger, tp.currentEVM, big.NewInt(MaxCallGasLimit), tp.selfAddress(), &oracleAddr, callData)
		if err != nil {
			return nil, err
		}
		tokenValue := new(big.Int).SetBytes(tokenValueRes)

		// get token decimals
		callData = decimalsSig
		tokenDecimalsRes, _, err := call(tp.stateLedger, tp.currentEVM, big.NewInt(MaxCallGasLimit), tp.selfAddress(), &token, callData)
		if err != nil {
			return nil, err
		}
		tokenDecimals := new(big.Int).SetBytes(tokenDecimalsRes)

		// tokenValue / 10^10 * 10^tokenDecimals = tokenValue * 10^(tokenDecimals - 10)
		if tokenDecimals.Cmp(big.NewInt(10)) >= 0 {
			return new(big.Int).Mul(tokenValue, new(big.Int).Exp(big.NewInt(10), new(big.Int).Sub(tokenDecimals, big.NewInt(10)), nil)), nil
		}
		// tokenValue * 10^(tokenDecimals - 10) = tokenValue / 10^(10 - tokenDecimals)
		return new(big.Int).Div(tokenValue, new(big.Int).Exp(big.NewInt(10), new(big.Int).Sub(big.NewInt(10), tokenDecimals), nil)), nil
	}

	return big.NewInt(0), common.NewRevertStringError(fmt.Sprintf("token paymaster: token %s not set oracle", token.String()))
}

func (tp *TokenPaymaster) getTokenBalance(account, token ethcommon.Address) (*big.Int, error) {
	callData := append(balanceOfSig, ethcommon.LeftPadBytes(account.Bytes(), 32)...)
	result, _, err := call(tp.stateLedger, tp.currentEVM, big.NewInt(MaxCallGasLimit), tp.selfAddress(), &token, callData)
	if err != nil {
		return nil, err
	}

	return new(big.Int).SetBytes(result), nil
}

func (tp *TokenPaymaster) transferFrom(account, token ethcommon.Address, tokenValue *big.Int) error {
	callData := append(transferFromSig, ethcommon.LeftPadBytes(account.Bytes(), 32)...)
	callData = append(callData, ethcommon.LeftPadBytes(tp.selfAddress().Bytes(), 32)...)
	callData = append(callData, ethcommon.LeftPadBytes(tokenValue.Bytes(), 32)...)
	_, _, err := call(tp.stateLedger, tp.currentEVM, big.NewInt(MaxCallGasLimit), tp.selfAddress(), &token, callData)
	if err != nil {
		return common.NewRevertStringError(fmt.Sprintf("token paymaster: call transferFrom failed: %s", err.Error()))
	}

	return nil
}

func encodeContext(account, token ethcommon.Address, gasPriceUserOp, maxTokenCost, maxCost *big.Int) ([]byte, error) {
	return contextArg.Pack(account, token, gasPriceUserOp, maxTokenCost, maxCost)
}

func decodeContext(context []byte) (account, token ethcommon.Address, gasPriceUserOp, maxTokenCost, maxCost *big.Int, err error) {
	args, err := contextArg.Unpack(context)
	if err != nil {
		return ethcommon.Address{}, ethcommon.Address{}, nil, nil, nil, err
	}

	return args[0].(ethcommon.Address), args[1].(ethcommon.Address), args[2].(*big.Int), args[3].(*big.Int), args[4].(*big.Int), nil
}
