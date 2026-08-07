// lock-test.js — first real OBS lock test on Sepolia.
//
//   npx hardhat run scripts/lock-test.js --network sepolia
//
// Two txs: approve(bridge, amount) then bridge.lock(amount, destChain, recipient).
// Env:
//   OBS_TOKEN_ADDRESS     OBS ERC-20 (required)
//   ETH_BRIDGE_CONTRACT   OBSBridge address (required)
//   LOCK_AMOUNT_OBS       whole-token units (default: 10)
//   LOCK_DEST_CHAIN       destChain string emitted in the Locked event (default: polkadot)
//   LOCK_RECIPIENT_SS58   destination Paseo/Substrate SS58 address (required)
const { ethers } = require("hardhat");

async function main() {
  const tokenAddr = process.env.OBS_TOKEN_ADDRESS;
  const bridgeAddr = process.env.ETH_BRIDGE_CONTRACT;
  const recipient = process.env.LOCK_RECIPIENT_SS58;
  if (!tokenAddr) throw new Error("OBS_TOKEN_ADDRESS not set");
  if (!bridgeAddr) throw new Error("ETH_BRIDGE_CONTRACT not set");
  if (!recipient) throw new Error("LOCK_RECIPIENT_SS58 not set");

  const amountWhole = process.env.LOCK_AMOUNT_OBS || "10";
  const destChain = process.env.LOCK_DEST_CHAIN || "polkadot";
  const amount = ethers.parseUnits(amountWhole, 18);

  const [signer] = await ethers.getSigners();
  const token = await ethers.getContractAt("OBSToken", tokenAddr);
  const bridge = await ethers.getContractAt("OBSBridge", bridgeAddr);

  console.log("signer      =", signer.address);
  console.log("token       =", tokenAddr);
  console.log("bridge      =", bridgeAddr);
  console.log("amount      =", amountWhole, "OBS");
  console.log("destChain   =", destChain);
  console.log("recipient   =", recipient);
  console.log("");

  console.log("--- tx 1: approve ---");
  const approveTx = await token.approve(bridgeAddr, amount);
  console.log("approve tx hash =", approveTx.hash);
  const approveReceipt = await approveTx.wait();
  console.log(
    "approve status  =",
    approveReceipt.status === 1 ? "success" : "FAILED",
    "block =",
    approveReceipt.blockNumber
  );
  const allowance = await token.allowance(signer.address, bridgeAddr);
  console.log("allowance now   =", ethers.formatUnits(allowance, 18), "OBS");
  console.log("");

  console.log("--- tx 2: lock ---");
  const lockTx = await bridge.lock(amount, destChain, recipient);
  console.log("lock tx hash    =", lockTx.hash);
  const lockReceipt = await lockTx.wait();
  console.log(
    "lock status     =",
    lockReceipt.status === 1 ? "success" : "FAILED",
    "block =",
    lockReceipt.blockNumber
  );

  const lockedBalance = await bridge.lockedBalances(signer.address);
  const totalLocked = await bridge.totalLocked();
  console.log("lockedBalances[signer] =", ethers.formatUnits(lockedBalance, 18), "OBS");
  console.log("totalLocked            =", ethers.formatUnits(totalLocked, 18), "OBS");

  console.log("");
  console.log("explorer approve =", `https://sepolia.etherscan.io/tx/${approveTx.hash}`);
  console.log("explorer lock    =", `https://sepolia.etherscan.io/tx/${lockTx.hash}`);
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
