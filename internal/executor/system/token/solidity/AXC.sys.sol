// SPDX-License-Identifier: UNLICENSED

pragma solidity ^0.8.20;

interface AXC {
    /**
     * @dev Returns the value of tokens in existence.
     */
    function totalSupply() external view returns (uint256);
}