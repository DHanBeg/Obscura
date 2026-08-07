// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";

/// @title OBSToken — Obscura bridge TEST token
/// @notice Sepolia-only ERC-20 used to exercise OBSBridge.lock()/unlock().
///         Not the mainnet OBS token; has no market value. Mints the full
///         supply to the deployer so there is a balance to lock in tests.
contract OBSToken is ERC20 {
    constructor(uint256 initialSupply) ERC20("Obscura Test Token", "OBS") {
        _mint(msg.sender, initialSupply);
    }
}
