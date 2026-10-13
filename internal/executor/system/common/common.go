package common

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtype "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sirupsen/logrus"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/ledger"
)

const (
	// ZeroAddress is a special address, no one has control
	ZeroAddress = "0x0000000000000000000000000000000000000000"

	// SystemContractStartAddr is the start address of system contract
	// the address range is 0x1000-0xffff, start from 1000, avoid conflicts with precompiled contracts
	SystemContractStartAddr = "0x0000000000000000000000000000000000001000"

	// ProposalIDContractAddr is the contract to used to generate the proposal ID
	ProposalIDContractAddr = "0x0000000000000000000000000000000000001000"
	GovernanceContractAddr = "0x0000000000000000000000000000000000001001"

	// AXMContractAddr is the contract to used to manager native token info
	AXMContractAddr = "0x0000000000000000000000000000000000001002"

	// Addr2NameContractAddr for unique name mapping to address
	Addr2NameContractAddr           = "0x0000000000000000000000000000000000001003"
	WhiteListContractAddr           = "0x0000000000000000000000000000000000001004"
	NotFinishedProposalContractAddr = "0x0000000000000000000000000000000000001005"

	// EpochManagerContractAddr is the contract to used to manager chain epoch info
	EpochManagerContractAddr = "0x0000000000000000000000000000000000001006"

	// AXCContractAddr is the system contract for axc
	AXCContractAddr = "0x0000000000000000000000000000000000001007"

	// Smart account contract
	// EntryPointContractAddr is the address of entry point system contract
	EntryPointContractAddr = "0x0000000000000000000000000000000000001008"
	// AccountFactoryContractAddr is the address of account factory system contract
	AccountFactoryContractAddr = "0x0000000000000000000000000000000000001009"
	// VerifyingPaymasterContractAddr is the address of verifying paymaster system contract
	VerifyingPaymasterContractAddr = "0x000000000000000000000000000000000000100a"
	// TokenPaymasterContractAddr is the address of token paymaster system contract
	TokenPaymasterContractAddr = "0x000000000000000000000000000000000000100b"

	// SystemContractEndAddr is the end address of system contract
	SystemContractEndAddr = "0x000000000000000000000000000000000000ffff"
)

var (
	BoolType, _         = abi.NewType("bool", "", nil)
	BigIntType, _       = abi.NewType("uint256", "", nil)
	UInt64Type, _       = abi.NewType("uint64", "", nil)
	UInt48Type, _       = abi.NewType("uint48", "", nil)
	StringType, _       = abi.NewType("string", "", nil)
	AddressType, _      = abi.NewType("address", "", nil)
	BytesType, _        = abi.NewType("bytes", "", nil)
	Bytes32Type, _      = abi.NewType("bytes32", "", nil)
	AddressSliceType, _ = abi.NewType("address[]", "", nil)
	BytesSliceType, _   = abi.NewType("bytes[]", "", nil)
)

type SystemContractConfig struct {
	Logger logrus.FieldLogger
}

type VirtualMachine interface {
	vm.PrecompiledContract

	// View return a view system contract
	View() VirtualMachine

	// GetContractInstance return the contract instance by given address
	GetContractInstance(addr *types.Address) SystemContract
}

type VMContext struct {
	StateLedger   ledger.StateLedger
	CurrentHeight uint64
	CurrentLogs   *[]Log
	CurrentUser   *ethcommon.Address
	CurrentEVM    *vm.EVM
}

// SystemContract must be implemented by all system contract
type SystemContract interface {
	SetContext(*VMContext)
}

func IsInSlice[T ~uint8 | ~string](value T, slice []T) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}

	return false
}

func RemoveFirstMatchStrInSlice(slice []string, val string) []string {
	for i, v := range slice {
		if v == val {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

type Log struct {
	Address *types.Address
	Topics  []*types.Hash
	Data    []byte
	Removed bool
}

func CalculateDynamicGas(bytes []byte) uint64 {
	gas, _ := core.IntrinsicGas(bytes, []ethtype.AccessTuple{}, false, true, true, true)
	return gas
}

// Used for record evm log
func Bool2Bytes(b bool) []byte {
	if b {
		return []byte{1}
	}

	return []byte{0}
}

var revertSelector = crypto.Keccak256([]byte("Error(string)"))[:4]

type RevertError struct {
	err error
	// data is encoded reverted reason, or result
	data []byte
	// reverted result
	str string
}

func NewRevertStringError(data string) error {
	packed, err := (abi.Arguments{{Type: StringType}}).Pack(data)
	if err != nil {
		return err
	}
	return &RevertError{
		err:  vm.ErrExecutionReverted,
		data: append(revertSelector, packed...),
		str:  data,
	}
}

func NewRevertError(name string, inputs abi.Arguments, args []any) error {
	abiErr := abi.NewError(name, inputs)
	selector := ethcommon.CopyBytes(abiErr.ID.Bytes()[:4])
	packed, err := inputs.Pack(args...)
	if err != nil {
		return err
	}

	return &RevertError{
		err:  vm.ErrExecutionReverted,
		data: append(selector, packed...),
		str:  fmt.Sprintf("%s, args: %v", abiErr.String(), args),
	}
}

func (e *RevertError) Error() string {
	return fmt.Sprintf("%s errdata %s", e.err.Error(), e.str)
}

func (e *RevertError) GetError() error {
	return e.err
}

func (e *RevertError) Data() []byte {
	return e.data
}

// DecodeRevert tries to turn a raw EVM revert payload into a human
// readable string. It understands the two Solidity builtins:
//
//   - 0x08c379a0 = Error(string) (require/assert with reason)
//   - 0x4e487b71 = Panic(uint256)  (assert, division by zero, etc.)
//
// Anything else is reported as a custom error with its 4-byte selector.
// Returns an empty string when the payload is too short to decode.
//
// This complements go-ethereum's abi.UnpackRevert, which silently falls
// back to an error for any selector that is not Error(string) or
// Panic(uint256) — making it hard to see custom errors or unknown
// selectors in logs. The eth_call API path uses this helper for that
// reason.
func DecodeRevert(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	selector := binary.BigEndian.Uint32(data[:4])
	switch selector {
	case 0x08c379a0:
		// Error(string): offset(32) + length(32) + bytes
		if len(data) < 4+64 {
			return "Error(string) <malformed payload>"
		}
		strOffset := binary.BigEndian.Uint64(data[4:36])
		if strOffset != 32 {
			return "Error(string) <unexpected offset>"
		}
		strLen := binary.BigEndian.Uint64(data[4+strOffset : 4+strOffset+32])
		if uint64(len(data)) < 4+strOffset+32+strLen {
			return "Error(string) <truncated>"
		}
		return fmt.Sprintf("Error(string) %q", string(data[4+strOffset+32:4+strOffset+32+strLen]))
	case 0x4e487b71:
		// Panic(uint256)
		if len(data) < 4+32 {
			return "Panic(uint256) <malformed payload>"
		}
		code := new(big.Int).SetBytes(data[4:36])
		return fmt.Sprintf("Panic(uint256) 0x%x (%s)", code.Uint64(), PanicCodeToString(code.Uint64()))
	default:
		return fmt.Sprintf("custom error, selector=0x%08x", selector)
	}
}

// PanicCodeToString maps the well-known Solidity Panic(uint256) codes
// to short descriptions, so a log line reads as "Panic(0x11) arithmetic
// overflow or underflow" instead of just "Panic(0x11)".
func PanicCodeToString(code uint64) string {
	switch code {
	case 0x01:
		return "assertion failed"
	case 0x11:
		return "arithmetic overflow or underflow"
	case 0x12:
		return "division or modulo by zero"
	case 0x21:
		return "conversion to enum out of bounds"
	case 0x22:
		return "storage encoding error"
	case 0x31:
		return "empty array pop"
	case 0x32:
		return "array index out of bounds"
	case 0x41:
		return "memory allocation too large"
	case 0x51:
		return "called an uninitialized internal function"
	default:
		return "unknown panic code"
	}
}
