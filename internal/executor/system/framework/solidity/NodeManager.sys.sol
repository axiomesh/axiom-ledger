// SPDX-License-Identifier: UNLICENSED

pragma solidity ^0.8.20;

    enum Status {
        DataSyncer,       // 0
        Candidate,        // 1
        Active,           // 2
        PendingInactive,  // 3
        Exited            // 4
    }

    struct NodeMetaData {
        string name;
        string desc;
        string imageURL;
        string websiteURL;
    }

    struct NodeInfo {
        uint64 ID;
        string ConsensusPubKey;
        string P2PPubKey;
        string P2PID;
        address Operator;
        NodeMetaData MetaData;
        Status Status;
    }

    struct ConsensusVotingPower {
        uint64 NodeID;
        int64 ConsensusVotingPower;
    }

interface NodeManager {
    error IncorrectStatus(uint8 status);
    error PendingInactiveSetIsFull();
    error NotEnoughValidator(uint256 curNum, uint256 minNum);

    event Register(uint64 indexed nodeID, NodeInfo info);
    event JoinedCandidateSet(uint64 indexed nodeID, uint64 commissionRate, uint256 operatorLiquidStakingTokenID);
    event Exit(uint64 indexed nodeID);
    event UpdateMetaData(uint64 indexed nodeID, NodeMetaData metaData);
    event UpdateOperator(uint64 indexed nodeID, address newOperator);

    function register(string memory consensusPubKey, string memory p2pPubKey, string memory p2pID, address newOperator, string memory name, string memory desc,string memory imageURL, string memory website) external returns (uint64 nodeID);

    function joinCandidateSet(uint64 nodeID, uint64 commissionRate) external;

    function exit(uint64 nodeID) external;

    function updateMetaData(uint64 nodeID, string memory name, string memory desc, string memory imageURL, string memory websiteURL) external;

    function updateOperator(uint64 nodeID, address newOperator) external;

    function getInfoByConsensusPubKey(string memory consensusPubKey) external view returns (NodeInfo memory info);

    function getInfo(uint64 nodeID) external view returns (NodeInfo memory info);

    function getTotalCount() external view returns (uint64);

    function getInfos(uint64[] memory nodeIDs) external view returns (NodeInfo[] memory info);

    function getActiveValidatorSet() external view returns (NodeInfo[] memory info, ConsensusVotingPower[] memory votingPowers);

    function getDataSyncerSet() external view returns (NodeInfo[] memory infos);

    function getCandidateSet() external view returns (NodeInfo[] memory infos);

    function getPendingInactiveSet() external view returns (NodeInfo[] memory infos);

    function getExitedSet() external view returns (NodeInfo[] memory infos);

    function getActiveValidatorIDSet() external view returns (uint64[] memory ids);

    function getDataSyncerIDSet() external view returns (uint64[] memory ids);

    function getCandidateIDSet() external view returns (uint64[] memory ids);

    function getPendingInactiveIDSet() external view returns (uint64[] memory ids);

    function getExitedIDSet() external view returns (uint64[] memory ids);
}