// hardhat.config.js — Ethereum-side OBSBridge build/deploy config.
require("@nomicfoundation/hardhat-toolbox");
require("dotenv").config({ path: "../../.env" });

const RPC_URL =
  process.env.ETH_RPC_URL || "https://ethereum-sepolia-rpc.publicnode.com";
// Deployer key — supply via .env, NEVER commit. Falls back to a throwaway
// hardhat default only for local `--network hardhat` runs.
const DEPLOYER_PRIVKEY = process.env.ETH_DEPLOYER_PRIVKEY || "";

/** @type {import('hardhat/config').HardhatUserConfig} */
module.exports = {
  solidity: {
    version: "0.8.20",
    settings: {
      optimizer: { enabled: true, runs: 200 },
    },
  },
  paths: {
    sources: "./contracts", // OBSBridge.sol / OBSToken.sol live here
  },
  networks: {
    hardhat: {},
    localhost: {
      url: "http://127.0.0.1:8545",
    },
    sepolia: {
      url: RPC_URL,
      accounts: DEPLOYER_PRIVKEY ? [DEPLOYER_PRIVKEY] : [],
    },
  },
  etherscan: {
    apiKey: process.env.ETHERSCAN_API_KEY || "",
  },
};
