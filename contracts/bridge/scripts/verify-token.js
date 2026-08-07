// verify-token.js — read-only check: confirms OBSToken deployment minted the
// initial supply to the deployer, and recovers the mint tx hash from the
// Transfer(0x0 -> deployer) event log (no Etherscan API key needed).
//
//   npx hardhat run scripts/verify-token.js --network sepolia
const { ethers } = require("hardhat");

async function main() {
  const tokenAddr = process.env.OBS_TOKEN_ADDRESS;
  if (!tokenAddr) throw new Error("OBS_TOKEN_ADDRESS not set");

  const [deployer] = await ethers.getSigners();
  const token = await ethers.getContractAt("OBSToken", tokenAddr);

  const [name, symbol, decimals, totalSupply, balance] = await Promise.all([
    token.name(),
    token.symbol(),
    token.decimals(),
    token.totalSupply(),
    token.balanceOf(deployer.address),
  ]);

  console.log("token address =", tokenAddr);
  console.log("name/symbol   =", name, "/", symbol);
  console.log("totalSupply   =", ethers.formatUnits(totalSupply, decimals));
  console.log("deployer      =", deployer.address);
  console.log(
    "deployer balance =",
    ethers.formatUnits(balance, decimals),
    symbol
  );

  // publicnode's free tier treats wide eth_getLogs ranges as archive queries
  // and rejects them; keep the window small (mint is always recent).
  const latestBlock = await ethers.provider.getBlockNumber();
  const fromBlock = Math.max(0, latestBlock - 200);
  const filter = token.filters.Transfer(ethers.ZeroAddress, deployer.address);
  const events = await token.queryFilter(filter, fromBlock, "latest");
  if (events.length === 0) {
    console.log("mint tx hash  = NOT FOUND (no Transfer-from-zero log)");
  } else {
    const mint = events[0];
    console.log("mint tx hash  =", mint.transactionHash);
    console.log("mint block    =", mint.blockNumber);
    console.log(
      "explorer      =",
      `https://sepolia.etherscan.io/tx/${mint.transactionHash}`
    );
  }
  console.log(
    "contract explorer =",
    `https://sepolia.etherscan.io/address/${tokenAddr}`
  );
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
