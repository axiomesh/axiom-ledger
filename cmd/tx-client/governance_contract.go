// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

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

// Proposal is an auto generated low-level Go binding around an user-defined struct.
type Proposal struct {
	ID                   uint64
	Type                 uint8
	Strategy             uint8
	Proposer             string
	Title                string
	Desc                 string
	BlockNumber          uint64
	TotalVotes           uint64
	PassVotes            []string
	RejectVotes          []string
	Status               uint8
	Extra                []byte
	CreatedBlockNumber   uint64
	EffectiveBlockNumber uint64
}

// GovernanceMetaData contains all meta data concerning the Governance contract.
var GovernanceMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"getLatestProposalID\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"\",\"type\":\"uint64\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"proposalID\",\"type\":\"uint64\"}],\"name\":\"proposal\",\"outputs\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"ID\",\"type\":\"uint64\"},{\"internalType\":\"enumProposalType\",\"name\":\"Type\",\"type\":\"uint8\"},{\"internalType\":\"enumProposalStrategy\",\"name\":\"Strategy\",\"type\":\"uint8\"},{\"internalType\":\"string\",\"name\":\"Proposer\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"Title\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"Desc\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"BlockNumber\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"TotalVotes\",\"type\":\"uint64\"},{\"internalType\":\"string[]\",\"name\":\"PassVotes\",\"type\":\"string[]\"},{\"internalType\":\"string[]\",\"name\":\"RejectVotes\",\"type\":\"string[]\"},{\"internalType\":\"enumProposalStatus\",\"name\":\"Status\",\"type\":\"uint8\"},{\"internalType\":\"bytes\",\"name\":\"Extra\",\"type\":\"bytes\"},{\"internalType\":\"uint64\",\"name\":\"CreatedBlockNumber\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"EffectiveBlockNumber\",\"type\":\"uint64\"}],\"internalType\":\"structProposal\",\"name\":\"proposal\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumProposalType\",\"name\":\"proposalType\",\"type\":\"uint8\"},{\"internalType\":\"string\",\"name\":\"title\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"desc\",\"type\":\"string\"},{\"internalType\":\"uint64\",\"name\":\"blockNumber\",\"type\":\"uint64\"},{\"internalType\":\"bytes\",\"name\":\"extra\",\"type\":\"bytes\"}],\"name\":\"propose\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"proposalID\",\"type\":\"uint64\"},{\"internalType\":\"enumVoteResult\",\"name\":\"voteResult\",\"type\":\"uint8\"}],\"name\":\"vote\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// GovernanceABI is the input ABI used to generate the binding from.
// Deprecated: Use GovernanceMetaData.ABI instead.
var GovernanceABI = GovernanceMetaData.ABI

// Governance is an auto generated Go binding around an Ethereum contract.
type Governance struct {
	GovernanceCaller     // Read-only binding to the contract
	GovernanceTransactor // Write-only binding to the contract
	GovernanceFilterer   // Log filterer for contract events
}

// GovernanceCaller is an auto generated read-only Go binding around an Ethereum contract.
type GovernanceCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GovernanceTransactor is an auto generated write-only Go binding around an Ethereum contract.
type GovernanceTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GovernanceFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type GovernanceFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GovernanceSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type GovernanceSession struct {
	Contract     *Governance       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// GovernanceCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type GovernanceCallerSession struct {
	Contract *GovernanceCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// GovernanceTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type GovernanceTransactorSession struct {
	Contract     *GovernanceTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// GovernanceRaw is an auto generated low-level Go binding around an Ethereum contract.
type GovernanceRaw struct {
	Contract *Governance // Generic contract binding to access the raw methods on
}

// GovernanceCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type GovernanceCallerRaw struct {
	Contract *GovernanceCaller // Generic read-only contract binding to access the raw methods on
}

// GovernanceTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type GovernanceTransactorRaw struct {
	Contract *GovernanceTransactor // Generic write-only contract binding to access the raw methods on
}

// NewGovernance creates a new instance of Governance, bound to a specific deployed contract.
func NewGovernance(address common.Address, backend bind.ContractBackend) (*Governance, error) {
	contract, err := bindGovernance(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Governance{GovernanceCaller: GovernanceCaller{contract: contract}, GovernanceTransactor: GovernanceTransactor{contract: contract}, GovernanceFilterer: GovernanceFilterer{contract: contract}}, nil
}

// NewGovernanceCaller creates a new read-only instance of Governance, bound to a specific deployed contract.
func NewGovernanceCaller(address common.Address, caller bind.ContractCaller) (*GovernanceCaller, error) {
	contract, err := bindGovernance(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &GovernanceCaller{contract: contract}, nil
}

// NewGovernanceTransactor creates a new write-only instance of Governance, bound to a specific deployed contract.
func NewGovernanceTransactor(address common.Address, transactor bind.ContractTransactor) (*GovernanceTransactor, error) {
	contract, err := bindGovernance(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &GovernanceTransactor{contract: contract}, nil
}

// NewGovernanceFilterer creates a new log filterer instance of Governance, bound to a specific deployed contract.
func NewGovernanceFilterer(address common.Address, filterer bind.ContractFilterer) (*GovernanceFilterer, error) {
	contract, err := bindGovernance(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &GovernanceFilterer{contract: contract}, nil
}

// bindGovernance binds a generic wrapper to an already deployed contract.
func bindGovernance(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := GovernanceMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Governance *GovernanceRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Governance.Contract.GovernanceCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Governance *GovernanceRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Governance.Contract.GovernanceTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Governance *GovernanceRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Governance.Contract.GovernanceTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Governance *GovernanceCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Governance.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Governance *GovernanceTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Governance.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Governance *GovernanceTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Governance.Contract.contract.Transact(opts, method, params...)
}

// GetLatestProposalID is a free data retrieval call binding the contract method 0x6785bd6e.
//
// Solidity: function getLatestProposalID() view returns(uint64)
func (_Governance *GovernanceCaller) GetLatestProposalID(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _Governance.contract.Call(opts, &out, "getLatestProposalID")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetLatestProposalID is a free data retrieval call binding the contract method 0x6785bd6e.
//
// Solidity: function getLatestProposalID() view returns(uint64)
func (_Governance *GovernanceSession) GetLatestProposalID() (uint64, error) {
	return _Governance.Contract.GetLatestProposalID(&_Governance.CallOpts)
}

// GetLatestProposalID is a free data retrieval call binding the contract method 0x6785bd6e.
//
// Solidity: function getLatestProposalID() view returns(uint64)
func (_Governance *GovernanceCallerSession) GetLatestProposalID() (uint64, error) {
	return _Governance.Contract.GetLatestProposalID(&_Governance.CallOpts)
}

// Proposal is a free data retrieval call binding the contract method 0x7afa0aa3.
//
// Solidity: function proposal(uint64 proposalID) view returns((uint64,uint8,uint8,string,string,string,uint64,uint64,string[],string[],uint8,bytes,uint64,uint64) proposal)
func (_Governance *GovernanceCaller) Proposal(opts *bind.CallOpts, proposalID uint64) (Proposal, error) {
	var out []interface{}
	err := _Governance.contract.Call(opts, &out, "proposal", proposalID)

	if err != nil {
		return *new(Proposal), err
	}

	out0 := *abi.ConvertType(out[0], new(Proposal)).(*Proposal)

	return out0, err

}

// Proposal is a free data retrieval call binding the contract method 0x7afa0aa3.
//
// Solidity: function proposal(uint64 proposalID) view returns((uint64,uint8,uint8,string,string,string,uint64,uint64,string[],string[],uint8,bytes,uint64,uint64) proposal)
func (_Governance *GovernanceSession) Proposal(proposalID uint64) (Proposal, error) {
	return _Governance.Contract.Proposal(&_Governance.CallOpts, proposalID)
}

// Proposal is a free data retrieval call binding the contract method 0x7afa0aa3.
//
// Solidity: function proposal(uint64 proposalID) view returns((uint64,uint8,uint8,string,string,string,uint64,uint64,string[],string[],uint8,bytes,uint64,uint64) proposal)
func (_Governance *GovernanceCallerSession) Proposal(proposalID uint64) (Proposal, error) {
	return _Governance.Contract.Proposal(&_Governance.CallOpts, proposalID)
}

// Propose is a paid mutator transaction binding the contract method 0xcee0ffe8.
//
// Solidity: function propose(uint8 proposalType, string title, string desc, uint64 blockNumber, bytes extra) returns()
func (_Governance *GovernanceTransactor) Propose(opts *bind.TransactOpts, proposalType uint8, title string, desc string, blockNumber uint64, extra []byte) (*types.Transaction, error) {
	return _Governance.contract.Transact(opts, "propose", proposalType, title, desc, blockNumber, extra)
}

// Propose is a paid mutator transaction binding the contract method 0xcee0ffe8.
//
// Solidity: function propose(uint8 proposalType, string title, string desc, uint64 blockNumber, bytes extra) returns()
func (_Governance *GovernanceSession) Propose(proposalType uint8, title string, desc string, blockNumber uint64, extra []byte) (*types.Transaction, error) {
	return _Governance.Contract.Propose(&_Governance.TransactOpts, proposalType, title, desc, blockNumber, extra)
}

// Propose is a paid mutator transaction binding the contract method 0xcee0ffe8.
//
// Solidity: function propose(uint8 proposalType, string title, string desc, uint64 blockNumber, bytes extra) returns()
func (_Governance *GovernanceTransactorSession) Propose(proposalType uint8, title string, desc string, blockNumber uint64, extra []byte) (*types.Transaction, error) {
	return _Governance.Contract.Propose(&_Governance.TransactOpts, proposalType, title, desc, blockNumber, extra)
}

// Vote is a paid mutator transaction binding the contract method 0xb040d166.
//
// Solidity: function vote(uint64 proposalID, uint8 voteResult) returns()
func (_Governance *GovernanceTransactor) Vote(opts *bind.TransactOpts, proposalID uint64, voteResult uint8) (*types.Transaction, error) {
	return _Governance.contract.Transact(opts, "vote", proposalID, voteResult)
}

// Vote is a paid mutator transaction binding the contract method 0xb040d166.
//
// Solidity: function vote(uint64 proposalID, uint8 voteResult) returns()
func (_Governance *GovernanceSession) Vote(proposalID uint64, voteResult uint8) (*types.Transaction, error) {
	return _Governance.Contract.Vote(&_Governance.TransactOpts, proposalID, voteResult)
}

// Vote is a paid mutator transaction binding the contract method 0xb040d166.
//
// Solidity: function vote(uint64 proposalID, uint8 voteResult) returns()
func (_Governance *GovernanceTransactorSession) Vote(proposalID uint64, voteResult uint8) (*types.Transaction, error) {
	return _Governance.Contract.Vote(&_Governance.TransactOpts, proposalID, voteResult)
}
