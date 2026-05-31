// distribute.ts — OBS Token genesis distribution script
//
// Aztec Sandbox / Testnet deployment'tan sonra çalıştırılır.
// Spec Bölüm 8.1 dağılımını on-chain uygular:
//
//   Topluluk:    400_000_000 OBS (40%)  — anında (vesting yok)
//   Ekip:        200_000_000 OBS (20%)  — 6 ay cliff, 24 ay vesting
//   Yatırımcı:   150_000_000 OBS (15%)  — 3 ay cliff, 18 ay vesting
//   Ekosistem:   150_000_000 OBS (15%)  — anında
//   Rezerv:      100_000_000 OBS (10%)  — anında (multisig vault)
//
// Kullanım:
//   AZTEC_RPC_URL=http://localhost:8080 \
//   DEPLOYER_PRIVATE_KEY=0x... \
//   OBS_TOKEN_ADDRESS=0x... \
//   COMMUNITY_TREASURY=0x... \
//   TEAM_VAULT=0x... \
//   INVESTOR_VAULT=0x... \
//   ECOSYSTEM_VAULT=0x... \
//   RESERVE_VAULT=0x... \
//   GENESIS_TIMESTAMP=1735689600 \
//   npx ts-node contracts/scripts/distribute.ts
//
// Veya Hardhat ile:
//   npx hardhat run contracts/scripts/distribute.ts --network aztec-testnet

import {
  AztecAddress,
  Contract,
  Fr,
  createPXEClient,
  getSandboxAccountsWallets,
  type AccountWallet,
  type PXE,
} from '@aztec/aztec.js';
import { OBSTokenContract } from '../artifacts/OBSToken.js';

// ── Constants (atomic units, 1 OBS = 1e18) ─────────────────────────────────

const ATOMIC = 10n ** 18n;

const COMMUNITY_AMOUNT  = 400_000_000n * ATOMIC;
const TEAM_AMOUNT       = 200_000_000n * ATOMIC;
const INVESTOR_AMOUNT   = 150_000_000n * ATOMIC;
const ECOSYSTEM_AMOUNT  = 150_000_000n * ATOMIC;
const RESERVE_AMOUNT    = 100_000_000n * ATOMIC;

// alloc_id sabitleri (obs_token_v2.nr ile uyumlu)
const ALLOC_COMMUNITY  = 1n;
const ALLOC_TEAM       = 2n;
const ALLOC_INVESTOR   = 3n;
const ALLOC_ECOSYSTEM  = 4n;
const ALLOC_RESERVE    = 5n;

// Vesting parametreleri (saniye)
const DAY    = 86_400n;
const MONTH  = 30n * DAY;
const TEAM_CLIFF       = 6n * MONTH;
const TEAM_DURATION    = 24n * MONTH;
const INVESTOR_CLIFF   = 3n * MONTH;
const INVESTOR_DURATION = 18n * MONTH;

// ── Environment helpers ─────────────────────────────────────────────────────

function requireEnv(key: string): string {
  const v = process.env[key];
  if (!v) {
    throw new Error(`Missing required env var: ${key}`);
  }
  return v;
}

function requireAddr(key: string): AztecAddress {
  const v = requireEnv(key);
  return AztecAddress.fromString(v);
}

// ── Distribution plan ──────────────────────────────────────────────────────

interface AllocationPlan {
  name: string;
  allocId: bigint;
  recipient: AztecAddress;
  amount: bigint;
  vestingStart: bigint;
  vestingCliff: bigint;
  vestingDuration: bigint;
}

function buildPlan(genesisTs: bigint): AllocationPlan[] {
  return [
    {
      name: 'Community',
      allocId: ALLOC_COMMUNITY,
      recipient: requireAddr('COMMUNITY_TREASURY'),
      amount: COMMUNITY_AMOUNT,
      vestingStart: genesisTs,
      vestingCliff: 0n,
      vestingDuration: 0n, // immediate
    },
    {
      name: 'Team',
      allocId: ALLOC_TEAM,
      recipient: requireAddr('TEAM_VAULT'),
      amount: TEAM_AMOUNT,
      vestingStart: genesisTs,
      vestingCliff: TEAM_CLIFF,
      vestingDuration: TEAM_DURATION,
    },
    {
      name: 'Investor',
      allocId: ALLOC_INVESTOR,
      recipient: requireAddr('INVESTOR_VAULT'),
      amount: INVESTOR_AMOUNT,
      vestingStart: genesisTs,
      vestingCliff: INVESTOR_CLIFF,
      vestingDuration: INVESTOR_DURATION,
    },
    {
      name: 'Ecosystem',
      allocId: ALLOC_ECOSYSTEM,
      recipient: requireAddr('ECOSYSTEM_VAULT'),
      amount: ECOSYSTEM_AMOUNT,
      vestingStart: genesisTs,
      vestingCliff: 0n,
      vestingDuration: 0n,
    },
    {
      name: 'Reserve',
      allocId: ALLOC_RESERVE,
      recipient: requireAddr('RESERVE_VAULT'),
      amount: RESERVE_AMOUNT,
      vestingStart: genesisTs,
      vestingCliff: 0n,
      vestingDuration: 0n,
    },
  ];
}

// ── Main ───────────────────────────────────────────────────────────────────

async function main() {
  const rpcUrl = requireEnv('AZTEC_RPC_URL');
  const tokenAddr = requireAddr('OBS_TOKEN_ADDRESS');
  const genesisTs = BigInt(requireEnv('GENESIS_TIMESTAMP'));

  console.log('================================================================');
  console.log('  OBS Token Genesis Distribution');
  console.log('================================================================');
  console.log(`  RPC:           ${rpcUrl}`);
  console.log(`  Token:         ${tokenAddr.toString()}`);
  console.log(`  Genesis TS:    ${genesisTs} (${new Date(Number(genesisTs) * 1000).toISOString()})`);
  console.log('');

  // PXE client + deployer wallet
  const pxe: PXE = createPXEClient(rpcUrl);
  const [deployerWallet] = await getSandboxAccountsWallets(pxe);
  console.log(`  Deployer:      ${deployerWallet.getAddress().toString()}`);
  console.log('');

  // Contract instance
  const token = await Contract.at(tokenAddr, OBSTokenContract.artifact, deployerWallet);

  // Plan
  const plan = buildPlan(genesisTs);

  // Toplam doğrulama (genesis supply'ı aşmamalı)
  const totalPlanned = plan.reduce((acc, p) => acc + p.amount, 0n);
  const expectedTotal = 1_000_000_000n * ATOMIC;
  if (totalPlanned !== expectedTotal) {
    throw new Error(
      `Plan total mismatch: ${totalPlanned} != ${expectedTotal} (expected 1B OBS)`
    );
  }

  console.log('Distribution plan:');
  for (const p of plan) {
    const obs = p.amount / ATOMIC;
    const vest = p.vestingDuration === 0n
      ? 'immediate'
      : `cliff=${p.vestingCliff / DAY}d duration=${p.vestingDuration / DAY}d`;
    console.log(`  - ${p.name.padEnd(10)} ${obs.toString().padStart(12)} OBS → ${p.recipient.toString().slice(0, 18)}... (${vest})`);
  }
  console.log('');

  // Çağrıları sıralı yap (her allocation için bir tx)
  for (const p of plan) {
    console.log(`[${p.name}] sending genesis_distribute...`);
    try {
      const tx = await token.methods
        .genesis_distribute(
          new Fr(p.allocId),
          p.recipient,
          new Fr(p.amount),
          p.vestingStart,
          p.vestingCliff,
          p.vestingDuration,
        )
        .send();
      const receipt = await tx.wait();
      console.log(`  [${p.name}] OK — tx: ${receipt.txHash.toString()}`);
    } catch (err) {
      console.error(`  [${p.name}] FAILED:`, err);
      throw err;
    }
  }

  console.log('');
  console.log('================================================================');
  console.log('  Verifying on-chain allocation state...');
  console.log('================================================================');

  for (const p of plan) {
    const dist = await token.methods
      .get_alloc_distributed(new Fr(p.allocId))
      .view();
    const distBn = BigInt(dist.toString());
    const ok = distBn === p.amount;
    const mark = ok ? '[OK]  ' : '[FAIL]';
    console.log(`  ${mark} ${p.name.padEnd(10)} on-chain=${distBn / ATOMIC} OBS expected=${p.amount / ATOMIC} OBS`);
    if (!ok) {
      throw new Error(`Allocation mismatch for ${p.name}`);
    }
  }

  const totalSupply = await token.methods.get_total_supply().view();
  const circulating = await token.methods.get_circulating_supply().view();
  console.log('');
  console.log(`  Total supply:        ${BigInt(totalSupply.toString()) / ATOMIC} OBS`);
  console.log(`  Circulating supply:  ${BigInt(circulating.toString()) / ATOMIC} OBS (immediate only)`);
  console.log('');
  console.log('Genesis distribution complete.');
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(1);
});
