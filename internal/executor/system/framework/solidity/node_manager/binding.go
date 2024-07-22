// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package node_manager

import (
	"math/big"

	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/pkg/packer"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = big.NewInt
	_ = common.Big1
	_ = types.AxcUnit
	_ = abi.ConvertType
	_ = packer.RevertError{}
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
	Primary         string
	Workers         []string
}

// NodeMetaData is an auto generated low-level Go binding around an user-defined struct.
type NodeMetaData struct {
	Name       string
	Desc       string
	ImageURL   string
	WebsiteURL string
}

type NodeManager interface {

	// Exit is a paid mutator transaction binding the contract method 0x0f143c6a.
	//
	// Solidity: function exit(uint64 nodeID) returns()
	Exit(nodeID uint64) error

	// JoinCandidateSet is a paid mutator transaction binding the contract method 0xece07dd6.
	//
	// Solidity: function joinCandidateSet(uint64 nodeID, uint64 commissionRate) returns()
	JoinCandidateSet(nodeID uint64, commissionRate uint64) error

	// UpdateMetaData is a paid mutator transaction binding the contract method 0x37aa97c4.
	//
	// Solidity: function updateMetaData(uint64 nodeID, string name, string desc, string imageURL, string websiteURL) returns()
	UpdateMetaData(nodeID uint64, name string, desc string, imageURL string, websiteURL string) error

	// UpdateOperator is a paid mutator transaction binding the contract method 0xb016e7d6.
	//
	// Solidity: function updateOperator(uint64 nodeID, address newOperator) returns()
	UpdateOperator(nodeID uint64, newOperator common.Address) error

	// GetActiveValidatorIDSet is a free data retrieval call binding the contract method 0xe4eee358.
	//
	// Solidity: function getActiveValidatorIDSet() view returns(uint64[] ids)
	GetActiveValidatorIDSet() ([]uint64, error)

	// GetActiveValidatorSet is a free data retrieval call binding the contract method 0x24408a68.
	//
	// Solidity: function getActiveValidatorSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info, (uint64,int64)[] votingPowers)
	GetActiveValidatorSet() ([]NodeInfo, []ConsensusVotingPower, error)

	// GetCandidateIDSet is a free data retrieval call binding the contract method 0x3817e9e7.
	//
	// Solidity: function getCandidateIDSet() view returns(uint64[] ids)
	GetCandidateIDSet() ([]uint64, error)

	// GetCandidateSet is a free data retrieval call binding the contract method 0x9c137646.
	//
	// Solidity: function getCandidateSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
	GetCandidateSet() ([]NodeInfo, error)

	// GetDataSyncerIDSet is a free data retrieval call binding the contract method 0xee299445.
	//
	// Solidity: function getDataSyncerIDSet() view returns(uint64[] ids)
	GetDataSyncerIDSet() ([]uint64, error)

	// GetDataSyncerSet is a free data retrieval call binding the contract method 0x39937e75.
	//
	// Solidity: function getDataSyncerSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
	GetDataSyncerSet() ([]NodeInfo, error)

	// GetExitedIDSet is a free data retrieval call binding the contract method 0x711049f1.
	//
	// Solidity: function getExitedIDSet() view returns(uint64[] ids)
	GetExitedIDSet() ([]uint64, error)

	// GetExitedSet is a free data retrieval call binding the contract method 0x4c434612.
	//
	// Solidity: function getExitedSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
	GetExitedSet() ([]NodeInfo, error)

	// GetInfo is a free data retrieval call binding the contract method 0x1ba603d6.
	//
	// Solidity: function getInfo(uint64 nodeID) view returns((uint64,string,string,string,address,(string,string,string,string),uint8) info)
	GetInfo(nodeID uint64) (NodeInfo, error)

	// GetInfos is a free data retrieval call binding the contract method 0x84e7ee6e.
	//
	// Solidity: function getInfos(uint64[] nodeIDs) view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] info)
	GetInfos(nodeIDs []uint64) ([]NodeInfo, error)

	// GetPendingInactiveIDSet is a free data retrieval call binding the contract method 0x23bed95e.
	//
	// Solidity: function getPendingInactiveIDSet() view returns(uint64[] ids)
	GetPendingInactiveIDSet() ([]uint64, error)

	// GetPendingInactiveSet is a free data retrieval call binding the contract method 0xcea3bc64.
	//
	// Solidity: function getPendingInactiveSet() view returns((uint64,string,string,string,address,(string,string,string,string),uint8)[] infos)
	GetPendingInactiveSet() ([]NodeInfo, error)

	// GetTotalCount is a free data retrieval call binding the contract method 0x56d42bb3.
	//
	// Solidity: function getTotalCount() view returns(uint64)
	GetTotalCount() (uint64, error)
}

// EventExit represents a Exit event raised by the NodeManager contract.
type EventExit struct {
	NodeID uint64
}

func (_event *EventExit) Pack(abi abi.ABI) (log *types.EvmLog, err error) {
	return packer.PackEvent(_event, abi.Events["Exit"])
}

// EventJoinedCandidateSet represents a JoinedCandidateSet event raised by the NodeManager contract.
type EventJoinedCandidateSet struct {
	NodeID                       uint64
	CommissionRate               uint64
	OperatorLiquidStakingTokenID *big.Int
}

func (_event *EventJoinedCandidateSet) Pack(abi abi.ABI) (log *types.EvmLog, err error) {
	return packer.PackEvent(_event, abi.Events["JoinedCandidateSet"])
}

// EventRegister represents a Register event raised by the NodeManager contract.
type EventRegister struct {
	NodeID uint64
	Info   NodeInfo
}

func (_event *EventRegister) Pack(abi abi.ABI) (log *types.EvmLog, err error) {
	return packer.PackEvent(_event, abi.Events["Register"])
}

// EventUpdateMetaData represents a UpdateMetaData event raised by the NodeManager contract.
type EventUpdateMetaData struct {
	NodeID   uint64
	MetaData NodeMetaData
}

func (_event *EventUpdateMetaData) Pack(abi abi.ABI) (log *types.EvmLog, err error) {
	return packer.PackEvent(_event, abi.Events["UpdateMetaData"])
}

// EventUpdateOperator represents a UpdateOperator event raised by the NodeManager contract.
type EventUpdateOperator struct {
	NodeID      uint64
	NewOperator common.Address
}

func (_event *EventUpdateOperator) Pack(abi abi.ABI) (log *types.EvmLog, err error) {
	return packer.PackEvent(_event, abi.Events["UpdateOperator"])
}

// ErrorIncorrectStatus represents a IncorrectStatus error raised by the NodeManager contract.
type ErrorIncorrectStatus struct {
	Status uint8
}

func (_error *ErrorIncorrectStatus) Pack(abi abi.ABI) error {
	return packer.PackError(_error, abi.Errors["IncorrectStatus"])
}

// ErrorNotEnoughValidator represents a NotEnoughValidator error raised by the NodeManager contract.
type ErrorNotEnoughValidator struct {
	CurNum *big.Int
	MinNum *big.Int
}

func (_error *ErrorNotEnoughValidator) Pack(abi abi.ABI) error {
	return packer.PackError(_error, abi.Errors["NotEnoughValidator"])
}

// ErrorPendingInactiveSetIsFull represents a PendingInactiveSetIsFull error raised by the NodeManager contract.
type ErrorPendingInactiveSetIsFull struct {
}

func (_error *ErrorPendingInactiveSetIsFull) Pack(abi abi.ABI) error {
	return packer.PackError(_error, abi.Errors["PendingInactiveSetIsFull"])
}
