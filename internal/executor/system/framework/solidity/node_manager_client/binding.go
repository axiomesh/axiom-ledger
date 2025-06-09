// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package node_manager_client

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

// ConsensusVotingPower is an auto generated low-level Go binding around an user-defined struct.
type ConsensusVotingPower struct {
	NodeID               uint64
	ConsensusVotingPower int64
}

// NodeInfo is an auto generated low-level Go binding around an user-defined struct.
type NodeInfo struct {
	ID              uint64
	ConsensusPubKey string
	P2PPubKey       string
	P2PID           string
	Operator        common.Address
	MetaData        NodeMetaData
	Status          uint8
}

// NodeMetaData is an auto generated low-level Go binding around an user-defined struct.
type NodeMetaData struct {
	Name       string
	Desc       string
	ImageURL   string
	WebsiteURL string
}

// BindingContractMetaData contains all meta data concerning the BindingContract contract.
var BindingContractMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint8\",\"name\":\"status\",\"type\":\"uint8\"}],\"name\":\"IncorrectStatus\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"curNum\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"minNum\",\"type\":\"uint256\"}],\"name\":\"NotEnoughValidator\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"PendingInactiveSetIsFull\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"}],\"name\":\"Exit\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"commissionRate\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"operatorLiquidStakingTokenID\",\"type\":\"uint256\"}],\"name\":\"JoinedCandidateSet\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"indexed\":false,\"internalType\":\"structNodeInfo\",\"name\":\"info\",\"type\":\"tuple\"}],\"name\":\"Register\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"indexed\":false,\"internalType\":\"structNodeMetaData\",\"name\":\"metaData\",\"type\":\"tuple\"}],\"name\":\"UpdateMetaData\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"newOperator\",\"type\":\"address\"}],\"name\":\"UpdateOperator\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"}],\"name\":\"exit\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getActiveValidatorIDSet\",\"outputs\":[{\"internalType\":\"uint64[]\",\"name\":\"ids\",\"type\":\"uint64[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getActiveValidatorSet\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo[]\",\"name\":\"info\",\"type\":\"tuple[]\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"NodeID\",\"type\":\"uint64\"},{\"internalType\":\"int64\",\"name\":\"ConsensusVotingPower\",\"type\":\"int64\"}],\"internalType\":\"structConsensusVotingPower[]\",\"name\":\"votingPowers\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getCandidateIDSet\",\"outputs\":[{\"internalType\":\"uint64[]\",\"name\":\"ids\",\"type\":\"uint64[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getCandidateSet\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo[]\",\"name\":\"infos\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getDataSyncerIDSet\",\"outputs\":[{\"internalType\":\"uint64[]\",\"name\":\"ids\",\"type\":\"uint64[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getDataSyncerSet\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo[]\",\"name\":\"infos\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getExitedIDSet\",\"outputs\":[{\"internalType\":\"uint64[]\",\"name\":\"ids\",\"type\":\"uint64[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getExitedSet\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo[]\",\"name\":\"infos\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"}],\"name\":\"getInfo\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo\",\"name\":\"info\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"consensusPubKey\",\"type\":\"string\"}],\"name\":\"getInfoByConsensusPubKey\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo\",\"name\":\"info\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64[]\",\"name\":\"nodeIDs\",\"type\":\"uint64[]\"}],\"name\":\"getInfos\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo[]\",\"name\":\"info\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getPendingInactiveIDSet\",\"outputs\":[{\"internalType\":\"uint64[]\",\"name\":\"ids\",\"type\":\"uint64[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getPendingInactiveSet\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"ConsensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"P2PID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"Operator\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"internalType\":\"structNodeMetaData\",\"name\":\"MetaData\",\"type\":\"tuple\"},{\"internalType\":\"enumStatus\",\"name\":\"Status\",\"type\":\"uint8\"}],\"internalType\":\"structNodeInfo[]\",\"name\":\"infos\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getTotalCount\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"commissionRate\",\"type\":\"uint64\"}],\"name\":\"joinCandidateSet\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"consensusPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"p2pPubKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"p2pID\",\"type\":\"string\"},{\"internalType\":\"address\",\"name\":\"newOperator\",\"type\":\"address\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"website\",\"type\":\"string\"}],\"name\":\"register\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"imageURL\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"websiteURL\",\"type\":\"string\"}],\"name\":\"updateMetaData\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"nodeID\",\"type\":\"uint64\"},{\"internalType\":\"address\",\"name\":\"newOperator\",\"type\":\"address\"}],\"name\":\"updateOperator\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// BindingContractABI is the input ABI used to generate the binding from.
// Deprecated: Use BindingContractMetaData.ABI instead.
var BindingContractABI = BindingContractMetaData.ABI

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

// GetActiveValidatorIDSet is a free data retrieval call binding the contract method 0xe4eee358.
//
// Solidity: function getActiveValidatorIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCaller) GetActiveValidatorIDSet(opts *bind.CallOpts) ([]uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getActiveValidatorIDSet")

	if err != nil {
		return *new([]uint64), err
	}

	out0 := *abi.ConvertType(out[0], new([]uint64)).(*[]uint64)

	return out0, err

}

// GetActiveValidatorIDSet is a free data retrieval call binding the contract method 0xe4eee358.
//
// Solidity: function getActiveValidatorIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractSession) GetActiveValidatorIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetActiveValidatorIDSet(&_BindingContract.CallOpts)
}

// GetActiveValidatorIDSet is a free data retrieval call binding the contract method 0xe4eee358.
//
// Solidity: function getActiveValidatorIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCallerSession) GetActiveValidatorIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetActiveValidatorIDSet(&_BindingContract.CallOpts)
}

// GetActiveValidatorSet is a free data retrieval call binding the contract method 0x24408a68.
//
// Solidity: function getActiveValidatorSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info, (uint64,int64)[] votingPowers)
func (_BindingContract *BindingContractCaller) GetActiveValidatorSet(opts *bind.CallOpts) (struct {
	Info         []NodeInfo
	VotingPowers []ConsensusVotingPower
}, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getActiveValidatorSet")

	outstruct := new(struct {
		Info         []NodeInfo
		VotingPowers []ConsensusVotingPower
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Info = *abi.ConvertType(out[0], new([]NodeInfo)).(*[]NodeInfo)
	outstruct.VotingPowers = *abi.ConvertType(out[1], new([]ConsensusVotingPower)).(*[]ConsensusVotingPower)

	return *outstruct, err

}

// GetActiveValidatorSet is a free data retrieval call binding the contract method 0x24408a68.
//
// Solidity: function getActiveValidatorSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info, (uint64,int64)[] votingPowers)
func (_BindingContract *BindingContractSession) GetActiveValidatorSet() (struct {
	Info         []NodeInfo
	VotingPowers []ConsensusVotingPower
}, error) {
	return _BindingContract.Contract.GetActiveValidatorSet(&_BindingContract.CallOpts)
}

// GetActiveValidatorSet is a free data retrieval call binding the contract method 0x24408a68.
//
// Solidity: function getActiveValidatorSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info, (uint64,int64)[] votingPowers)
func (_BindingContract *BindingContractCallerSession) GetActiveValidatorSet() (struct {
	Info         []NodeInfo
	VotingPowers []ConsensusVotingPower
}, error) {
	return _BindingContract.Contract.GetActiveValidatorSet(&_BindingContract.CallOpts)
}

// GetCandidateIDSet is a free data retrieval call binding the contract method 0x3817e9e7.
//
// Solidity: function getCandidateIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCaller) GetCandidateIDSet(opts *bind.CallOpts) ([]uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getCandidateIDSet")

	if err != nil {
		return *new([]uint64), err
	}

	out0 := *abi.ConvertType(out[0], new([]uint64)).(*[]uint64)

	return out0, err

}

// GetCandidateIDSet is a free data retrieval call binding the contract method 0x3817e9e7.
//
// Solidity: function getCandidateIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractSession) GetCandidateIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetCandidateIDSet(&_BindingContract.CallOpts)
}

// GetCandidateIDSet is a free data retrieval call binding the contract method 0x3817e9e7.
//
// Solidity: function getCandidateIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCallerSession) GetCandidateIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetCandidateIDSet(&_BindingContract.CallOpts)
}

// GetCandidateSet is a free data retrieval call binding the contract method 0x9c137646.
//
// Solidity: function getCandidateSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCaller) GetCandidateSet(opts *bind.CallOpts) ([]NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getCandidateSet")

	if err != nil {
		return *new([]NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]NodeInfo)).(*[]NodeInfo)

	return out0, err

}

// GetCandidateSet is a free data retrieval call binding the contract method 0x9c137646.
//
// Solidity: function getCandidateSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractSession) GetCandidateSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetCandidateSet(&_BindingContract.CallOpts)
}

// GetCandidateSet is a free data retrieval call binding the contract method 0x9c137646.
//
// Solidity: function getCandidateSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCallerSession) GetCandidateSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetCandidateSet(&_BindingContract.CallOpts)
}

// GetDataSyncerIDSet is a free data retrieval call binding the contract method 0xee299445.
//
// Solidity: function getDataSyncerIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCaller) GetDataSyncerIDSet(opts *bind.CallOpts) ([]uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getDataSyncerIDSet")

	if err != nil {
		return *new([]uint64), err
	}

	out0 := *abi.ConvertType(out[0], new([]uint64)).(*[]uint64)

	return out0, err

}

// GetDataSyncerIDSet is a free data retrieval call binding the contract method 0xee299445.
//
// Solidity: function getDataSyncerIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractSession) GetDataSyncerIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetDataSyncerIDSet(&_BindingContract.CallOpts)
}

// GetDataSyncerIDSet is a free data retrieval call binding the contract method 0xee299445.
//
// Solidity: function getDataSyncerIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCallerSession) GetDataSyncerIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetDataSyncerIDSet(&_BindingContract.CallOpts)
}

// GetDataSyncerSet is a free data retrieval call binding the contract method 0x39937e75.
//
// Solidity: function getDataSyncerSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCaller) GetDataSyncerSet(opts *bind.CallOpts) ([]NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getDataSyncerSet")

	if err != nil {
		return *new([]NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]NodeInfo)).(*[]NodeInfo)

	return out0, err

}

// GetDataSyncerSet is a free data retrieval call binding the contract method 0x39937e75.
//
// Solidity: function getDataSyncerSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractSession) GetDataSyncerSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetDataSyncerSet(&_BindingContract.CallOpts)
}

// GetDataSyncerSet is a free data retrieval call binding the contract method 0x39937e75.
//
// Solidity: function getDataSyncerSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCallerSession) GetDataSyncerSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetDataSyncerSet(&_BindingContract.CallOpts)
}

// GetExitedIDSet is a free data retrieval call binding the contract method 0x711049f1.
//
// Solidity: function getExitedIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCaller) GetExitedIDSet(opts *bind.CallOpts) ([]uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getExitedIDSet")

	if err != nil {
		return *new([]uint64), err
	}

	out0 := *abi.ConvertType(out[0], new([]uint64)).(*[]uint64)

	return out0, err

}

// GetExitedIDSet is a free data retrieval call binding the contract method 0x711049f1.
//
// Solidity: function getExitedIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractSession) GetExitedIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetExitedIDSet(&_BindingContract.CallOpts)
}

// GetExitedIDSet is a free data retrieval call binding the contract method 0x711049f1.
//
// Solidity: function getExitedIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCallerSession) GetExitedIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetExitedIDSet(&_BindingContract.CallOpts)
}

// GetExitedSet is a free data retrieval call binding the contract method 0x4c434612.
//
// Solidity: function getExitedSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCaller) GetExitedSet(opts *bind.CallOpts) ([]NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getExitedSet")

	if err != nil {
		return *new([]NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]NodeInfo)).(*[]NodeInfo)

	return out0, err

}

// GetExitedSet is a free data retrieval call binding the contract method 0x4c434612.
//
// Solidity: function getExitedSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractSession) GetExitedSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetExitedSet(&_BindingContract.CallOpts)
}

// GetExitedSet is a free data retrieval call binding the contract method 0x4c434612.
//
// Solidity: function getExitedSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCallerSession) GetExitedSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetExitedSet(&_BindingContract.CallOpts)
}

// GetInfo is a free data retrieval call binding the contract method 0x1ba603d6.
//
// Solidity: function getInfo(uint64 nodeID) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractCaller) GetInfo(opts *bind.CallOpts, nodeID uint64) (NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getInfo", nodeID)

	if err != nil {
		return *new(NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new(NodeInfo)).(*NodeInfo)

	return out0, err

}

// GetInfo is a free data retrieval call binding the contract method 0x1ba603d6.
//
// Solidity: function getInfo(uint64 nodeID) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractSession) GetInfo(nodeID uint64) (NodeInfo, error) {
	return _BindingContract.Contract.GetInfo(&_BindingContract.CallOpts, nodeID)
}

// GetInfo is a free data retrieval call binding the contract method 0x1ba603d6.
//
// Solidity: function getInfo(uint64 nodeID) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractCallerSession) GetInfo(nodeID uint64) (NodeInfo, error) {
	return _BindingContract.Contract.GetInfo(&_BindingContract.CallOpts, nodeID)
}

// GetInfoByConsensusPubKey is a free data retrieval call binding the contract method 0x706c5cac.
//
// Solidity: function getInfoByConsensusPubKey(string consensusPubKey) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractCaller) GetInfoByConsensusPubKey(opts *bind.CallOpts, consensusPubKey string) (NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getInfoByConsensusPubKey", consensusPubKey)

	if err != nil {
		return *new(NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new(NodeInfo)).(*NodeInfo)

	return out0, err

}

// GetInfoByConsensusPubKey is a free data retrieval call binding the contract method 0x706c5cac.
//
// Solidity: function getInfoByConsensusPubKey(string consensusPubKey) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractSession) GetInfoByConsensusPubKey(consensusPubKey string) (NodeInfo, error) {
	return _BindingContract.Contract.GetInfoByConsensusPubKey(&_BindingContract.CallOpts, consensusPubKey)
}

// GetInfoByConsensusPubKey is a free data retrieval call binding the contract method 0x706c5cac.
//
// Solidity: function getInfoByConsensusPubKey(string consensusPubKey) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractCallerSession) GetInfoByConsensusPubKey(consensusPubKey string) (NodeInfo, error) {
	return _BindingContract.Contract.GetInfoByConsensusPubKey(&_BindingContract.CallOpts, consensusPubKey)
}

// GetInfos is a free data retrieval call binding the contract method 0x84e7ee6e.
//
// Solidity: function getInfos(uint64[] nodeIDs) view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info)
func (_BindingContract *BindingContractCaller) GetInfos(opts *bind.CallOpts, nodeIDs []uint64) ([]NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getInfos", nodeIDs)

	if err != nil {
		return *new([]NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]NodeInfo)).(*[]NodeInfo)

	return out0, err

}

// GetInfos is a free data retrieval call binding the contract method 0x84e7ee6e.
//
// Solidity: function getInfos(uint64[] nodeIDs) view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info)
func (_BindingContract *BindingContractSession) GetInfos(nodeIDs []uint64) ([]NodeInfo, error) {
	return _BindingContract.Contract.GetInfos(&_BindingContract.CallOpts, nodeIDs)
}

// GetInfos is a free data retrieval call binding the contract method 0x84e7ee6e.
//
// Solidity: function getInfos(uint64[] nodeIDs) view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info)
func (_BindingContract *BindingContractCallerSession) GetInfos(nodeIDs []uint64) ([]NodeInfo, error) {
	return _BindingContract.Contract.GetInfos(&_BindingContract.CallOpts, nodeIDs)
}

// GetPendingInactiveIDSet is a free data retrieval call binding the contract method 0x23bed95e.
//
// Solidity: function getPendingInactiveIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCaller) GetPendingInactiveIDSet(opts *bind.CallOpts) ([]uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getPendingInactiveIDSet")

	if err != nil {
		return *new([]uint64), err
	}

	out0 := *abi.ConvertType(out[0], new([]uint64)).(*[]uint64)

	return out0, err

}

// GetPendingInactiveIDSet is a free data retrieval call binding the contract method 0x23bed95e.
//
// Solidity: function getPendingInactiveIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractSession) GetPendingInactiveIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetPendingInactiveIDSet(&_BindingContract.CallOpts)
}

// GetPendingInactiveIDSet is a free data retrieval call binding the contract method 0x23bed95e.
//
// Solidity: function getPendingInactiveIDSet() view returns(uint64[] ids)
func (_BindingContract *BindingContractCallerSession) GetPendingInactiveIDSet() ([]uint64, error) {
	return _BindingContract.Contract.GetPendingInactiveIDSet(&_BindingContract.CallOpts)
}

// GetPendingInactiveSet is a free data retrieval call binding the contract method 0xcea3bc64.
//
// Solidity: function getPendingInactiveSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCaller) GetPendingInactiveSet(opts *bind.CallOpts) ([]NodeInfo, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getPendingInactiveSet")

	if err != nil {
		return *new([]NodeInfo), err
	}

	out0 := *abi.ConvertType(out[0], new([]NodeInfo)).(*[]NodeInfo)

	return out0, err

}

// GetPendingInactiveSet is a free data retrieval call binding the contract method 0xcea3bc64.
//
// Solidity: function getPendingInactiveSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractSession) GetPendingInactiveSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetPendingInactiveSet(&_BindingContract.CallOpts)
}

// GetPendingInactiveSet is a free data retrieval call binding the contract method 0xcea3bc64.
//
// Solidity: function getPendingInactiveSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
func (_BindingContract *BindingContractCallerSession) GetPendingInactiveSet() ([]NodeInfo, error) {
	return _BindingContract.Contract.GetPendingInactiveSet(&_BindingContract.CallOpts)
}

// GetTotalCount is a free data retrieval call binding the contract method 0x56d42bb3.
//
// Solidity: function getTotalCount() view returns(uint64)
func (_BindingContract *BindingContractCaller) GetTotalCount(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _BindingContract.contract.Call(opts, &out, "getTotalCount")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetTotalCount is a free data retrieval call binding the contract method 0x56d42bb3.
//
// Solidity: function getTotalCount() view returns(uint64)
func (_BindingContract *BindingContractSession) GetTotalCount() (uint64, error) {
	return _BindingContract.Contract.GetTotalCount(&_BindingContract.CallOpts)
}

// GetTotalCount is a free data retrieval call binding the contract method 0x56d42bb3.
//
// Solidity: function getTotalCount() view returns(uint64)
func (_BindingContract *BindingContractCallerSession) GetTotalCount() (uint64, error) {
	return _BindingContract.Contract.GetTotalCount(&_BindingContract.CallOpts)
}

// Exit is a paid mutator transaction binding the contract method 0x0f143c6a.
//
// Solidity: function exit(uint64 nodeID) returns()
func (_BindingContract *BindingContractTransactor) Exit(opts *bind.TransactOpts, nodeID uint64) (*types.Transaction, error) {
	return _BindingContract.contract.Transact(opts, "exit", nodeID)
}

// Exit is a paid mutator transaction binding the contract method 0x0f143c6a.
//
// Solidity: function exit(uint64 nodeID) returns()
func (_BindingContract *BindingContractSession) Exit(nodeID uint64) (*types.Transaction, error) {
	return _BindingContract.Contract.Exit(&_BindingContract.TransactOpts, nodeID)
}

// Exit is a paid mutator transaction binding the contract method 0x0f143c6a.
//
// Solidity: function exit(uint64 nodeID) returns()
func (_BindingContract *BindingContractTransactorSession) Exit(nodeID uint64) (*types.Transaction, error) {
	return _BindingContract.Contract.Exit(&_BindingContract.TransactOpts, nodeID)
}

// JoinCandidateSet is a paid mutator transaction binding the contract method 0xece07dd6.
//
// Solidity: function joinCandidateSet(uint64 nodeID, uint64 commissionRate) returns()
func (_BindingContract *BindingContractTransactor) JoinCandidateSet(opts *bind.TransactOpts, nodeID uint64, commissionRate uint64) (*types.Transaction, error) {
	return _BindingContract.contract.Transact(opts, "joinCandidateSet", nodeID, commissionRate)
}

// JoinCandidateSet is a paid mutator transaction binding the contract method 0xece07dd6.
//
// Solidity: function joinCandidateSet(uint64 nodeID, uint64 commissionRate) returns()
func (_BindingContract *BindingContractSession) JoinCandidateSet(nodeID uint64, commissionRate uint64) (*types.Transaction, error) {
	return _BindingContract.Contract.JoinCandidateSet(&_BindingContract.TransactOpts, nodeID, commissionRate)
}

// JoinCandidateSet is a paid mutator transaction binding the contract method 0xece07dd6.
//
// Solidity: function joinCandidateSet(uint64 nodeID, uint64 commissionRate) returns()
func (_BindingContract *BindingContractTransactorSession) JoinCandidateSet(nodeID uint64, commissionRate uint64) (*types.Transaction, error) {
	return _BindingContract.Contract.JoinCandidateSet(&_BindingContract.TransactOpts, nodeID, commissionRate)
}

// Register is a paid mutator transaction binding the contract method 0x0faaa240.
//
// Solidity: function register(string consensusPubKey, string p2pPubKey, string p2pID, address newOperator, string name, string desc, string imageURL, string website) returns(uint64 nodeID)
func (_BindingContract *BindingContractTransactor) Register(opts *bind.TransactOpts, consensusPubKey string, p2pPubKey string, p2pID string, newOperator common.Address, name string, desc string, imageURL string, website string) (*types.Transaction, error) {
	return _BindingContract.contract.Transact(opts, "register", consensusPubKey, p2pPubKey, p2pID, newOperator, name, desc, imageURL, website)
}

// Register is a paid mutator transaction binding the contract method 0x0faaa240.
//
// Solidity: function register(string consensusPubKey, string p2pPubKey, string p2pID, address newOperator, string name, string desc, string imageURL, string website) returns(uint64 nodeID)
func (_BindingContract *BindingContractSession) Register(consensusPubKey string, p2pPubKey string, p2pID string, newOperator common.Address, name string, desc string, imageURL string, website string) (*types.Transaction, error) {
	return _BindingContract.Contract.Register(&_BindingContract.TransactOpts, consensusPubKey, p2pPubKey, p2pID, newOperator, name, desc, imageURL, website)
}

// Register is a paid mutator transaction binding the contract method 0x0faaa240.
//
// Solidity: function register(string consensusPubKey, string p2pPubKey, string p2pID, address newOperator, string name, string desc, string imageURL, string website) returns(uint64 nodeID)
func (_BindingContract *BindingContractTransactorSession) Register(consensusPubKey string, p2pPubKey string, p2pID string, newOperator common.Address, name string, desc string, imageURL string, website string) (*types.Transaction, error) {
	return _BindingContract.Contract.Register(&_BindingContract.TransactOpts, consensusPubKey, p2pPubKey, p2pID, newOperator, name, desc, imageURL, website)
}

// UpdateMetaData is a paid mutator transaction binding the contract method 0x37aa97c4.
//
// Solidity: function updateMetaData(uint64 nodeID, string name, string desc, string imageURL, string websiteURL) returns()
func (_BindingContract *BindingContractTransactor) UpdateMetaData(opts *bind.TransactOpts, nodeID uint64, name string, desc string, imageURL string, websiteURL string) (*types.Transaction, error) {
	return _BindingContract.contract.Transact(opts, "updateMetaData", nodeID, name, desc, imageURL, websiteURL)
}

// UpdateMetaData is a paid mutator transaction binding the contract method 0x37aa97c4.
//
// Solidity: function updateMetaData(uint64 nodeID, string name, string desc, string imageURL, string websiteURL) returns()
func (_BindingContract *BindingContractSession) UpdateMetaData(nodeID uint64, name string, desc string, imageURL string, websiteURL string) (*types.Transaction, error) {
	return _BindingContract.Contract.UpdateMetaData(&_BindingContract.TransactOpts, nodeID, name, desc, imageURL, websiteURL)
}

// UpdateMetaData is a paid mutator transaction binding the contract method 0x37aa97c4.
//
// Solidity: function updateMetaData(uint64 nodeID, string name, string desc, string imageURL, string websiteURL) returns()
func (_BindingContract *BindingContractTransactorSession) UpdateMetaData(nodeID uint64, name string, desc string, imageURL string, websiteURL string) (*types.Transaction, error) {
	return _BindingContract.Contract.UpdateMetaData(&_BindingContract.TransactOpts, nodeID, name, desc, imageURL, websiteURL)
}

// UpdateOperator is a paid mutator transaction binding the contract method 0xb016e7d6.
//
// Solidity: function updateOperator(uint64 nodeID, address newOperator) returns()
func (_BindingContract *BindingContractTransactor) UpdateOperator(opts *bind.TransactOpts, nodeID uint64, newOperator common.Address) (*types.Transaction, error) {
	return _BindingContract.contract.Transact(opts, "updateOperator", nodeID, newOperator)
}

// UpdateOperator is a paid mutator transaction binding the contract method 0xb016e7d6.
//
// Solidity: function updateOperator(uint64 nodeID, address newOperator) returns()
func (_BindingContract *BindingContractSession) UpdateOperator(nodeID uint64, newOperator common.Address) (*types.Transaction, error) {
	return _BindingContract.Contract.UpdateOperator(&_BindingContract.TransactOpts, nodeID, newOperator)
}

// UpdateOperator is a paid mutator transaction binding the contract method 0xb016e7d6.
//
// Solidity: function updateOperator(uint64 nodeID, address newOperator) returns()
func (_BindingContract *BindingContractTransactorSession) UpdateOperator(nodeID uint64, newOperator common.Address) (*types.Transaction, error) {
	return _BindingContract.Contract.UpdateOperator(&_BindingContract.TransactOpts, nodeID, newOperator)
}

// BindingContractExitIterator is returned from FilterExit and is used to iterate over the raw logs and unpacked data for Exit events raised by the BindingContract contract.
type BindingContractExitIterator struct {
	Event *BindingContractExit // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BindingContractExitIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BindingContractExit)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BindingContractExit)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BindingContractExitIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BindingContractExitIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BindingContractExit represents a Exit event raised by the BindingContract contract.
type BindingContractExit struct {
	NodeID uint64
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterExit is a free log retrieval operation binding the contract event 0x15ba9e702a5964fb3133af391a524b76ea8687b7f40d5c6d73f7d61878189d78.
//
// Solidity: event Exit(uint64 indexed nodeID)
func (_BindingContract *BindingContractFilterer) FilterExit(opts *bind.FilterOpts, nodeID []uint64) (*BindingContractExitIterator, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.FilterLogs(opts, "Exit", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return &BindingContractExitIterator{contract: _BindingContract.contract, event: "Exit", logs: logs, sub: sub}, nil
}

// WatchExit is a free log subscription operation binding the contract event 0x15ba9e702a5964fb3133af391a524b76ea8687b7f40d5c6d73f7d61878189d78.
//
// Solidity: event Exit(uint64 indexed nodeID)
func (_BindingContract *BindingContractFilterer) WatchExit(opts *bind.WatchOpts, sink chan<- *BindingContractExit, nodeID []uint64) (event.Subscription, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.WatchLogs(opts, "Exit", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BindingContractExit)
				if err := _BindingContract.contract.UnpackLog(event, "Exit", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseExit is a log parse operation binding the contract event 0x15ba9e702a5964fb3133af391a524b76ea8687b7f40d5c6d73f7d61878189d78.
//
// Solidity: event Exit(uint64 indexed nodeID)
func (_BindingContract *BindingContractFilterer) ParseExit(log types.Log) (*BindingContractExit, error) {
	event := new(BindingContractExit)
	if err := _BindingContract.contract.UnpackLog(event, "Exit", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BindingContractJoinedCandidateSetIterator is returned from FilterJoinedCandidateSet and is used to iterate over the raw logs and unpacked data for JoinedCandidateSet events raised by the BindingContract contract.
type BindingContractJoinedCandidateSetIterator struct {
	Event *BindingContractJoinedCandidateSet // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BindingContractJoinedCandidateSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BindingContractJoinedCandidateSet)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BindingContractJoinedCandidateSet)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BindingContractJoinedCandidateSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BindingContractJoinedCandidateSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BindingContractJoinedCandidateSet represents a JoinedCandidateSet event raised by the BindingContract contract.
type BindingContractJoinedCandidateSet struct {
	NodeID                       uint64
	CommissionRate               uint64
	OperatorLiquidStakingTokenID *big.Int
	Raw                          types.Log // Blockchain specific contextual infos
}

// FilterJoinedCandidateSet is a free log retrieval operation binding the contract event 0xb780ea5f0172f0d1e42f012be8f3f29f10bbacc69fdb43a64e41336e8320bd81.
//
// Solidity: event JoinedCandidateSet(uint64 indexed nodeID, uint64 commissionRate, uint256 operatorLiquidStakingTokenID)
func (_BindingContract *BindingContractFilterer) FilterJoinedCandidateSet(opts *bind.FilterOpts, nodeID []uint64) (*BindingContractJoinedCandidateSetIterator, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.FilterLogs(opts, "JoinedCandidateSet", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return &BindingContractJoinedCandidateSetIterator{contract: _BindingContract.contract, event: "JoinedCandidateSet", logs: logs, sub: sub}, nil
}

// WatchJoinedCandidateSet is a free log subscription operation binding the contract event 0xb780ea5f0172f0d1e42f012be8f3f29f10bbacc69fdb43a64e41336e8320bd81.
//
// Solidity: event JoinedCandidateSet(uint64 indexed nodeID, uint64 commissionRate, uint256 operatorLiquidStakingTokenID)
func (_BindingContract *BindingContractFilterer) WatchJoinedCandidateSet(opts *bind.WatchOpts, sink chan<- *BindingContractJoinedCandidateSet, nodeID []uint64) (event.Subscription, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.WatchLogs(opts, "JoinedCandidateSet", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BindingContractJoinedCandidateSet)
				if err := _BindingContract.contract.UnpackLog(event, "JoinedCandidateSet", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseJoinedCandidateSet is a log parse operation binding the contract event 0xb780ea5f0172f0d1e42f012be8f3f29f10bbacc69fdb43a64e41336e8320bd81.
//
// Solidity: event JoinedCandidateSet(uint64 indexed nodeID, uint64 commissionRate, uint256 operatorLiquidStakingTokenID)
func (_BindingContract *BindingContractFilterer) ParseJoinedCandidateSet(log types.Log) (*BindingContractJoinedCandidateSet, error) {
	event := new(BindingContractJoinedCandidateSet)
	if err := _BindingContract.contract.UnpackLog(event, "JoinedCandidateSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BindingContractRegisterIterator is returned from FilterRegister and is used to iterate over the raw logs and unpacked data for Register events raised by the BindingContract contract.
type BindingContractRegisterIterator struct {
	Event *BindingContractRegister // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BindingContractRegisterIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BindingContractRegister)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BindingContractRegister)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BindingContractRegisterIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BindingContractRegisterIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BindingContractRegister represents a Register event raised by the BindingContract contract.
type BindingContractRegister struct {
	NodeID uint64
	Info   NodeInfo
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRegister is a free log retrieval operation binding the contract event 0x39d7e313eafd5ac66e5491ea78903da2cce5ef4cfee38cd00335bfe099317094.
//
// Solidity: event Register(uint64 indexed nodeID, (uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractFilterer) FilterRegister(opts *bind.FilterOpts, nodeID []uint64) (*BindingContractRegisterIterator, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.FilterLogs(opts, "Register", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return &BindingContractRegisterIterator{contract: _BindingContract.contract, event: "Register", logs: logs, sub: sub}, nil
}

// WatchRegister is a free log subscription operation binding the contract event 0x39d7e313eafd5ac66e5491ea78903da2cce5ef4cfee38cd00335bfe099317094.
//
// Solidity: event Register(uint64 indexed nodeID, (uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractFilterer) WatchRegister(opts *bind.WatchOpts, sink chan<- *BindingContractRegister, nodeID []uint64) (event.Subscription, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.WatchLogs(opts, "Register", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BindingContractRegister)
				if err := _BindingContract.contract.UnpackLog(event, "Register", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRegister is a log parse operation binding the contract event 0x39d7e313eafd5ac66e5491ea78903da2cce5ef4cfee38cd00335bfe099317094.
//
// Solidity: event Register(uint64 indexed nodeID, (uint64,string,string,string,address,(string,string,string,string),uint8) info)
func (_BindingContract *BindingContractFilterer) ParseRegister(log types.Log) (*BindingContractRegister, error) {
	event := new(BindingContractRegister)
	if err := _BindingContract.contract.UnpackLog(event, "Register", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BindingContractUpdateMetaDataIterator is returned from FilterUpdateMetaData and is used to iterate over the raw logs and unpacked data for UpdateMetaData events raised by the BindingContract contract.
type BindingContractUpdateMetaDataIterator struct {
	Event *BindingContractUpdateMetaData // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BindingContractUpdateMetaDataIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BindingContractUpdateMetaData)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BindingContractUpdateMetaData)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BindingContractUpdateMetaDataIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BindingContractUpdateMetaDataIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BindingContractUpdateMetaData represents a UpdateMetaData event raised by the BindingContract contract.
type BindingContractUpdateMetaData struct {
	NodeID   uint64
	MetaData NodeMetaData
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterUpdateMetaData is a free log retrieval operation binding the contract event 0x808f1ed07a386a6f6e717905ba5391e9fa1b8bbad1da2374216aa7a1bbd85a15.
//
// Solidity: event UpdateMetaData(uint64 indexed nodeID, (string,string,string,string) metaData)
func (_BindingContract *BindingContractFilterer) FilterUpdateMetaData(opts *bind.FilterOpts, nodeID []uint64) (*BindingContractUpdateMetaDataIterator, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.FilterLogs(opts, "UpdateMetaData", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return &BindingContractUpdateMetaDataIterator{contract: _BindingContract.contract, event: "UpdateMetaData", logs: logs, sub: sub}, nil
}

// WatchUpdateMetaData is a free log subscription operation binding the contract event 0x808f1ed07a386a6f6e717905ba5391e9fa1b8bbad1da2374216aa7a1bbd85a15.
//
// Solidity: event UpdateMetaData(uint64 indexed nodeID, (string,string,string,string) metaData)
func (_BindingContract *BindingContractFilterer) WatchUpdateMetaData(opts *bind.WatchOpts, sink chan<- *BindingContractUpdateMetaData, nodeID []uint64) (event.Subscription, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.WatchLogs(opts, "UpdateMetaData", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BindingContractUpdateMetaData)
				if err := _BindingContract.contract.UnpackLog(event, "UpdateMetaData", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUpdateMetaData is a log parse operation binding the contract event 0x808f1ed07a386a6f6e717905ba5391e9fa1b8bbad1da2374216aa7a1bbd85a15.
//
// Solidity: event UpdateMetaData(uint64 indexed nodeID, (string,string,string,string) metaData)
func (_BindingContract *BindingContractFilterer) ParseUpdateMetaData(log types.Log) (*BindingContractUpdateMetaData, error) {
	event := new(BindingContractUpdateMetaData)
	if err := _BindingContract.contract.UnpackLog(event, "UpdateMetaData", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BindingContractUpdateOperatorIterator is returned from FilterUpdateOperator and is used to iterate over the raw logs and unpacked data for UpdateOperator events raised by the BindingContract contract.
type BindingContractUpdateOperatorIterator struct {
	Event *BindingContractUpdateOperator // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BindingContractUpdateOperatorIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BindingContractUpdateOperator)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BindingContractUpdateOperator)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BindingContractUpdateOperatorIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BindingContractUpdateOperatorIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BindingContractUpdateOperator represents a UpdateOperator event raised by the BindingContract contract.
type BindingContractUpdateOperator struct {
	NodeID      uint64
	NewOperator common.Address
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterUpdateOperator is a free log retrieval operation binding the contract event 0xf44e07f847b3e109d399de191e51da03076eb693a80f1e8af6c80aa3aa5caa74.
//
// Solidity: event UpdateOperator(uint64 indexed nodeID, address newOperator)
func (_BindingContract *BindingContractFilterer) FilterUpdateOperator(opts *bind.FilterOpts, nodeID []uint64) (*BindingContractUpdateOperatorIterator, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.FilterLogs(opts, "UpdateOperator", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return &BindingContractUpdateOperatorIterator{contract: _BindingContract.contract, event: "UpdateOperator", logs: logs, sub: sub}, nil
}

// WatchUpdateOperator is a free log subscription operation binding the contract event 0xf44e07f847b3e109d399de191e51da03076eb693a80f1e8af6c80aa3aa5caa74.
//
// Solidity: event UpdateOperator(uint64 indexed nodeID, address newOperator)
func (_BindingContract *BindingContractFilterer) WatchUpdateOperator(opts *bind.WatchOpts, sink chan<- *BindingContractUpdateOperator, nodeID []uint64) (event.Subscription, error) {

	var nodeIDRule []interface{}
	for _, nodeIDItem := range nodeID {
		nodeIDRule = append(nodeIDRule, nodeIDItem)
	}

	logs, sub, err := _BindingContract.contract.WatchLogs(opts, "UpdateOperator", nodeIDRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BindingContractUpdateOperator)
				if err := _BindingContract.contract.UnpackLog(event, "UpdateOperator", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUpdateOperator is a log parse operation binding the contract event 0xf44e07f847b3e109d399de191e51da03076eb693a80f1e8af6c80aa3aa5caa74.
//
// Solidity: event UpdateOperator(uint64 indexed nodeID, address newOperator)
func (_BindingContract *BindingContractFilterer) ParseUpdateOperator(log types.Log) (*BindingContractUpdateOperator, error) {
	event := new(BindingContractUpdateOperator)
	if err := _BindingContract.contract.UnpackLog(event, "UpdateOperator", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
