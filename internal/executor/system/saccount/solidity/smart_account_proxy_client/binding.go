// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package smart_account_proxy_client

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// PassKey is an auto generated low-level Go binding around an user-defined struct.
type PassKey struct {
	PublicKey []byte
	Algo      uint8
}

// SessionKey is an auto generated low-level Go binding around an user-defined struct.
type SessionKey struct {
	Addr          common.Address
	SpendingLimit *big.Int
	SpentAmount   *big.Int
	ValidUntil    uint64
	ValidAfter    uint64
}

// BindingContractMetaData contains all meta data concerning the BindingContract contract.
var BindingContractMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"getGuardian\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"getOwner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"getPasskeys\",\"outputs\":[{\"components\":[{\"internalType\":\"bytes\",\"name\":\"publicKey\",\"type\":\"bytes\"},{\"internalType\":\"uint8\",\"name\":\"algo\",\"type\":\"uint8\"}],\"internalType\":\"structPassKey[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"getSessions\",\"outputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"addr\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"spendingLimit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"spentAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint64\",\"name\":\"validUntil\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"validAfter\",\"type\":\"uint64\"}],\"internalType\":\"structSessionKey[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"getStatus\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
	Bin: "0x6080604052348015600e575f5ffd5b5061058f8061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610055575f3560e01c806313625e2e1461005957806330ccebb514610089578063b301d902146100b9578063bc6f15ea146100e9578063fa54416114610119575b5f5ffd5b610073600480360381019061006e91906101c7565b610149565b6040516100809190610349565b60405180910390f35b6100a3600480360381019061009e91906101c7565b610150565b6040516100b09190610378565b60405180910390f35b6100d360048036038101906100ce91906101c7565b610156565b6040516100e091906103a0565b60405180910390f35b61010360048036038101906100fe91906101c7565b61015c565b6040516101109190610539565b60405180910390f35b610133600480360381019061012e91906101c7565b610163565b60405161014091906103a0565b60405180910390f35b6060919050565b5f919050565b5f919050565b6060919050565b5f919050565b5f5ffd5b5f73ffffffffffffffffffffffffffffffffffffffff82169050919050565b5f6101968261016d565b9050919050565b6101a68161018c565b81146101b0575f5ffd5b50565b5f813590506101c18161019d565b92915050565b5f602082840312156101dc576101db610169565b5b5f6101e9848285016101b3565b91505092915050565b5f81519050919050565b5f82825260208201905092915050565b5f819050602082019050919050565b6102248161018c565b82525050565b5f819050919050565b61023c8161022a565b82525050565b5f67ffffffffffffffff82169050919050565b61025e81610242565b82525050565b60a082015f8201516102785f85018261021b565b50602082015161028b6020850182610233565b50604082015161029e6040850182610233565b5060608201516102b16060850182610255565b5060808201516102c46080850182610255565b50505050565b5f6102d58383610264565b60a08301905092915050565b5f602082019050919050565b5f6102f7826101f2565b61030181856101fc565b935061030c8361020c565b805f5b8381101561033c57815161032388826102ca565b975061032e836102e1565b92505060018101905061030f565b5085935050505092915050565b5f6020820190508181035f83015261036181846102ed565b905092915050565b61037281610242565b82525050565b5f60208201905061038b5f830184610369565b92915050565b61039a8161018c565b82525050565b5f6020820190506103b35f830184610391565b92915050565b5f81519050919050565b5f82825260208201905092915050565b5f819050602082019050919050565b5f81519050919050565b5f82825260208201905092915050565b8281835e5f83830152505050565b5f601f19601f8301169050919050565b5f610424826103e2565b61042e81856103ec565b935061043e8185602086016103fc565b6104478161040a565b840191505092915050565b5f60ff82169050919050565b61046781610452565b82525050565b5f604083015f8301518482035f860152610487828261041a565b915050602083015161049c602086018261045e565b508091505092915050565b5f6104b2838361046d565b905092915050565b5f602082019050919050565b5f6104d0826103b9565b6104da81856103c3565b9350836020820285016104ec856103d3565b805f5b85811015610527578484038952815161050885826104a7565b9450610513836104ba565b925060208a019950506001810190506104ef565b50829750879550505050505092915050565b5f6020820190508181035f83015261055181846104c6565b90509291505056fea2646970667358221220d7f80e1c6a701a8cb66be1ae01840ca420fa5be66fd7907bdafd45d799fb9ea364736f6c634300081e0033",
}

// BindingContractABI is the input ABI used to generate the binding from.
// Deprecated: Use BindingContractMetaData.ABI instead.
var BindingContractABI = BindingContractMetaData.ABI

// BindingContractBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use BindingContractMetaData.Bin instead.
var BindingContractBin = BindingContractMetaData.Bin

// DeployBindingContract deploys a new Ethereum contract, binding an instance of BindingContract to it.
func DeployBindingContract(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *BindingContract, error) {
	parsed, err := BindingContractMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(BindingContractBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &BindingContract{BindingContractCaller: BindingContractCaller{contract: contract}, BindingContractTransactor: BindingContractTransactor{contract: contract}, BindingContractFilterer: BindingContractFilterer{contract: contract}}, nil
}

// BindingContract is an auto generated Go binding around an Ethereum contract.
type BindingContract struct {
	BindingContractCaller     // Read-only binding to the contract
	BindingContractTransactor // Write-only binding to the contract
	BindingContractFilterer   // Log filterer for contract events
}

// BindingContractCaller is an auto generated read-only Go binding around an Ethereum contract.
type BindingContractCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BindingContractTransactor is an auto generated write-only Go binding around an Ethereum contract.
type BindingContractTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BindingContractFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type BindingContractFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BindingContractSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type BindingContractSession struct {
	Contract     *BindingContract  // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// BindingContractCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type BindingContractCallerSession struct {
	Contract *BindingContractCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts          // Call options to use throughout this session
}

// BindingContractTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type BindingContractTransactorSession struct {
	Contract     *BindingContractTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts          // Transaction auth options to use throughout this session
}

// BindingContractRaw is an auto generated low-level Go binding around an Ethereum contract.
type BindingContractRaw struct {
	Contract *BindingContract // Generic contract binding to access the raw methods on
}

// BindingContractCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type BindingContractCallerRaw struct {
	Contract *BindingContractCaller // Generic read-only contract binding to access the raw methods on
}

// BindingContractTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type BindingContractTransactorRaw struct {
	Contract *BindingContractTransactor // Generic write-only contract binding to access the raw methods on
}

// NewBindingContract creates a new instance of BindingContract, bound to a specific deployed contract.
func NewBindingContract(address common.Address, backend bind.ContractBackend) (*BindingContract, error) {
	contract, err := bindBindingContract(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &BindingContract{BindingContractCaller: BindingContractCaller{contract: contract}, BindingContractTransactor: BindingContractTransactor{contract: contract}, BindingContractFilterer: BindingContractFilterer{contract: contract}}, nil
}

// NewBindingContractCaller creates a new read-only instance of BindingContract, bound to a specific deployed contract.
func NewBindingContractCaller(address common.Address, caller bind.ContractCaller) (*BindingContractCaller, error) {
	contract, err := bindBindingContract(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &BindingContractCaller{contract: contract}, nil
}

// NewBindingContractTransactor creates a new write-only instance of BindingContract, bound to a specific deployed contract.
func NewBindingContractTransactor(address common.Address, transactor bind.ContractTransactor) (*BindingContractTransactor, error) {
	contract, err := bindBindingContract(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &BindingContractTransactor{contract: contract}, nil
}

// NewBindingContractFilterer creates a new log filterer instance of BindingContract, bound to a specific deployed contract.
func NewBindingContractFilterer(address common.Address, filterer bind.ContractFilterer) (*BindingContractFilterer, error) {
	contract, err := bindBindingContract(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &BindingContractFilterer{contract: contract}, nil
}

// bindBindingContract binds a generic wrapper to an already deployed contract.
func bindBindingContract(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := BindingContractMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_BindingContract *BindingContractRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _BindingContract.Contract.BindingContractCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_BindingContract *BindingContractRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _BindingContract.Contract.BindingContractTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_BindingContract *BindingContractRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _BindingContract.Contract.BindingContractTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_BindingContract *BindingContractCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _BindingContract.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_BindingContract *BindingContractTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _BindingContract.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_BindingContract *BindingContractTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _BindingContract.Contract.contract.Transact(opts, method, params...)
}

// GetGuardian is a free data retrieval call binding the contract method 0xb301d902.
//
// Solidity: function getGuardian(address ) view returns(address)
func (_BindingContract *BindingContractCaller) GetGuardian(opts *bind.CallOpts, arg0 common.Address) (common.Address, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getGuardian", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetGuardian is a free data retrieval call binding the contract method 0xb301d902.
//
// Solidity: function getGuardian(address ) view returns(address)
func (_BindingContract *BindingContractSession) GetGuardian(arg0 common.Address) (common.Address, error) {
	return _BindingContract.Contract.GetGuardian(&_BindingContract.CallOpts, arg0)
}

// GetGuardian is a free data retrieval call binding the contract method 0xb301d902.
//
// Solidity: function getGuardian(address ) view returns(address)
func (_BindingContract *BindingContractCallerSession) GetGuardian(arg0 common.Address) (common.Address, error) {
	return _BindingContract.Contract.GetGuardian(&_BindingContract.CallOpts, arg0)
}

// GetOwner is a free data retrieval call binding the contract method 0xfa544161.
//
// Solidity: function getOwner(address ) view returns(address)
func (_BindingContract *BindingContractCaller) GetOwner(opts *bind.CallOpts, arg0 common.Address) (common.Address, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getOwner", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetOwner is a free data retrieval call binding the contract method 0xfa544161.
//
// Solidity: function getOwner(address ) view returns(address)
func (_BindingContract *BindingContractSession) GetOwner(arg0 common.Address) (common.Address, error) {
	return _BindingContract.Contract.GetOwner(&_BindingContract.CallOpts, arg0)
}

// GetOwner is a free data retrieval call binding the contract method 0xfa544161.
//
// Solidity: function getOwner(address ) view returns(address)
func (_BindingContract *BindingContractCallerSession) GetOwner(arg0 common.Address) (common.Address, error) {
	return _BindingContract.Contract.GetOwner(&_BindingContract.CallOpts, arg0)
}

// GetPasskeys is a free data retrieval call binding the contract method 0xbc6f15ea.
//
// Solidity: function getPasskeys(address ) view returns((bytes,uint8)[])
func (_BindingContract *BindingContractCaller) GetPasskeys(opts *bind.CallOpts, arg0 common.Address) ([]PassKey, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getPasskeys", arg0)

	if err != nil {
		return *new([]PassKey), err
	}

	out0 := *abi.ConvertType(out[0], new([]PassKey)).(*[]PassKey)

	return out0, err

}

// GetPasskeys is a free data retrieval call binding the contract method 0xbc6f15ea.
//
// Solidity: function getPasskeys(address ) view returns((bytes,uint8)[])
func (_BindingContract *BindingContractSession) GetPasskeys(arg0 common.Address) ([]PassKey, error) {
	return _BindingContract.Contract.GetPasskeys(&_BindingContract.CallOpts, arg0)
}

// GetPasskeys is a free data retrieval call binding the contract method 0xbc6f15ea.
//
// Solidity: function getPasskeys(address ) view returns((bytes,uint8)[])
func (_BindingContract *BindingContractCallerSession) GetPasskeys(arg0 common.Address) ([]PassKey, error) {
	return _BindingContract.Contract.GetPasskeys(&_BindingContract.CallOpts, arg0)
}

// GetSessions is a free data retrieval call binding the contract method 0x13625e2e.
//
// Solidity: function getSessions(address ) view returns((address,uint256,uint256,uint64,uint64)[])
func (_BindingContract *BindingContractCaller) GetSessions(opts *bind.CallOpts, arg0 common.Address) ([]SessionKey, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getSessions", arg0)

	if err != nil {
		return *new([]SessionKey), err
	}

	out0 := *abi.ConvertType(out[0], new([]SessionKey)).(*[]SessionKey)

	return out0, err

}

// GetSessions is a free data retrieval call binding the contract method 0x13625e2e.
//
// Solidity: function getSessions(address ) view returns((address,uint256,uint256,uint64,uint64)[])
func (_BindingContract *BindingContractSession) GetSessions(arg0 common.Address) ([]SessionKey, error) {
	return _BindingContract.Contract.GetSessions(&_BindingContract.CallOpts, arg0)
}

// GetSessions is a free data retrieval call binding the contract method 0x13625e2e.
//
// Solidity: function getSessions(address ) view returns((address,uint256,uint256,uint64,uint64)[])
func (_BindingContract *BindingContractCallerSession) GetSessions(arg0 common.Address) ([]SessionKey, error) {
	return _BindingContract.Contract.GetSessions(&_BindingContract.CallOpts, arg0)
}

// GetStatus is a free data retrieval call binding the contract method 0x30ccebb5.
//
// Solidity: function getStatus(address ) view returns(uint64)
func (_BindingContract *BindingContractCaller) GetStatus(opts *bind.CallOpts, arg0 common.Address) (uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getStatus", arg0)

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetStatus is a free data retrieval call binding the contract method 0x30ccebb5.
//
// Solidity: function getStatus(address ) view returns(uint64)
func (_BindingContract *BindingContractSession) GetStatus(arg0 common.Address) (uint64, error) {
	return _BindingContract.Contract.GetStatus(&_BindingContract.CallOpts, arg0)
}

// GetStatus is a free data retrieval call binding the contract method 0x30ccebb5.
//
// Solidity: function getStatus(address ) view returns(uint64)
func (_BindingContract *BindingContractCallerSession) GetStatus(arg0 common.Address) (uint64, error) {
	return _BindingContract.Contract.GetStatus(&_BindingContract.CallOpts, arg0)
}
