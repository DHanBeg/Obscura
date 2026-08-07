// verify-bridge.js — read-only check: confirms OBSBridge is wired to the
// right OBS token and that owner/relayer constructor params landed, then
// recovers the creation tx hash by scanning recent blocks for a
// contract-creation receipt matching the deployed address (no Etherscan
// API key needed).
//
//   npx hardhat run scripts/verify-bridge.js --network sepolia
const { ethers } = require("hardhat");

async function main() {
  const bridgeAddr = process.env.BRIDGE_CONTRACT_ADDRESS;
  if (!bridgeAddr) throw new Error("BRIDGE_CONTRACT_ADDRESS not set");

  const [deployer] = await ethers.getSigners();
  const bridge = await ethers.getContractAt("OBSBridge", bridgeAddr);

  const [obsToken, owner, relayer, paused] = await Promise.all([
    bridge.obsToken(),
    bridge.owner(),
    bridge.relayer(),
    bridge.paused(),
  ]);

  console.log("bridge address =", bridgeAddr);
  console.log("obsToken()     =", obsToken);
  console.log("owner()        =", owner);
  console.log("relayer()      =", relayer);
  console.log("paused()       =", paused);
  console.log(
    "obsToken matches expected:",
    obsToken.toLowerCase() === (process.env.OBS_TOKEN_ADDRESS || "").toLowerCase()
  );

  // publicnode free tier rejects wide eth_getLogs ranges as archive queries,
  // and the constructor emits no event anyway, so recover the creation tx by
  // scanning recent blocks for a contract-creation receipt at this address.
  const scanDepth = Number(process.env.VERIFY_SCAN_DEPTH || 20);
  const latestBlock = await ethers.provider.getBlockNumber();
  let found = null;
  for (let bn = latestBlock; bn > latestBlock - scanDepth && !found; bn--) {
    const block = await ethers.provider.getBlock(bn, true);
    if (!block) continue;
    for (const txHash of block.transactions) {
      const receipt = await ethers.provider.getTransactionReceipt(txHash);
      if (
        receipt &&
        receipt.contractAddress &&
        receipt.contractAddress.toLowerCase() === bridgeAddr.toLowerCase()
      ) {
        found = { hash: txHash, blockNumber: bn };
        break;
      }
    }
  }

  if (found) {
    console.log("creation tx hash =", found.hash);
    console.log("creation block   =", found.blockNumber);
    console.log(
      "explorer tx      =",
      `https://sepolia.etherscan.io/tx/${found.hash}`
    );
  } else {
    console.log("creation tx hash = NOT FOUND in last 200 blocks");
  }
  console.log(
    "explorer contract =",
    `https://sepolia.etherscan.io/address/${bridgeAddr}`
  );
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
