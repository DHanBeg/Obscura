# Obscura Smart Contracts

This directory holds Obscura's on-chain layer:

| Path | Chain | Language | What |
|------|-------|----------|------|
| `aztec/obs_token_v2.nr` | Aztec | Noir (Aztec.nr) | OBS token — transparent supply views + shielded balances/transfers |
| `aztec/staking.nr` | Aztec | Noir | Staking (lock / unlock / rewards) |
| `aztec/proof_registry.nr` | Aztec | Noir | ZK proof registry |
| `bridge/OBSBridge.sol` | Ethereum | Solidity 0.8.20 | Cross-chain lock/unlock escrow |

The Go RPC bridge lives at `backend/internal/blockchain/`:
- `aztec.go` — low-level proof submission skeleton (`AztecBridge`)
- `aztec_client.go` — OBS-token PXE client (`AztecClient`: `Transfer`, `TotalSupply`, `Healthcheck`)

---

## Security rules (do not skip)

- Real private keys are NEVER committed. Deploy outputs land in `.env.aztec` /
  `.env.bridge`, both gitignored. Keys come from env or a keystore alias only.
- Owner-only functions are gated by a 3/5 multi-sig.
- Token + staking carry an emergency `pause()` (guardian) / `unpause()` (owner).
- Upgrades go through a transparent proxy with a 48h timelock.
- Audit + 30-day testnet soak before mainnet.

---

## Aztec — OBS Token

### 1. Run a local sandbox

```bash
# Option A: Docker
docker run -p 8080:8080 -p 9000:9000 aztecprotocol/aztec-sandbox:latest

# Option B: aztec-up toolchain
bash <(curl -s https://install.aztec.network)
aztec start --sandbox
```

### 2. Install tooling

```bash
npm install -g @aztec/cli @aztec/aztec.js
# aztec-nargo (Noir compiler for Aztec) ships with the aztec-up toolchain.
```

### 3. Deploy

```bash
# Defaults: AZTEC_SANDBOX_URL=http://localhost:8080
bash contracts/aztec/deploy.sh
```

The script compiles `obs_token_v2.nr`, deploys `OBSToken` with the
constructor `(owner, minter, guardian, genesis_ts)`, parses the deployed
address, and appends `OBS_TOKEN_ADDRESS=<addr>` to `.env.aztec`.

Override the constructor roles via env:

```bash
OBS_OWNER=0x...      # 3/5 multisig
OBS_MINTER=0x...     # staking contract (defaults to owner)
OBS_GUARDIAN=0x...   # emergency pause (defaults to owner)
bash contracts/aztec/deploy.sh
```

### 4. Testnet (Aztec Testnet Alpha)

```bash
AZTEC_SANDBOX_URL=https://api.aztec.network/aztec-testnet \
  bash contracts/aztec/deploy.sh
```

### OBSToken API surface

Private (caller-shielded):
- `transfer(from, to, amount)` — shielded transfer with 0.01 OBS fee (50% burn, 50% pool)
- `balance_of(owner) -> Field` — only the owner derives a meaningful result
- `get_balance(owner) -> Field` — alias of `balance_of`
- `mint(to, amount)` — owner-only, capped at `ABSOLUTE_MAX_SUPPLY`
- `mint_inflation(now_ts)` — minter-only, 2%/yr schedule
- `burn(from, amount)`, `claim_vested(...)`, `genesis_distribute(...)`

Public (auditable):
- `total_supply() -> Field` (alias `get_total_supply`)
- `get_circulating_supply`, `get_fee_pool`, `get_burned`, `is_paused`
- `pause()` (guardian), `unpause()` (owner)
- `set_minter`, `set_guardian`, `transfer_ownership` (owner)

---

## Ethereum — OBSBridge

```bash
cd contracts/bridge
npm install

# Compile + test
npx hardhat compile
npx hardhat test

# Deploy (constructor: obsToken, owner, relayer)
OBS_TOKEN_ADDRESS=0x... \
ETH_RPC_URL=https://sepolia.infura.io/v3/<key> \
ETH_DEPLOYER_PRIVKEY=0x... \
  npx hardhat run deploy.js --network sepolia
```

The deploy script refuses to run without a real `OBS_TOKEN_ADDRESS` (the
constructor reverts on the zero address) and appends
`BRIDGE_CONTRACT_ADDRESS=<addr>` to `.env.bridge`.

---

## Backend wiring

After deploy, export the address so the Go node picks it up:

```bash
export AZTEC_SANDBOX_URL=http://localhost:8080
export OBS_TOKEN_ADDRESS=$(grep OBS_TOKEN_ADDRESS ../.env.aztec | tail -1 | cut -d= -f2)
```

`blockchain.NewAztecClient()` reads `AZTEC_SANDBOX_URL`, `OBS_TOKEN_ADDRESS`,
`AZTEC_ADMIN_PRIVKEY`, and `OBSCURA_AZTEC_STRICT`. When the sandbox is
unreachable it degrades gracefully (stub tx hash) unless
`OBSCURA_AZTEC_STRICT=1` is set.
