---
name: signal-protocol-impl
description: Implement Signal Protocol (X3DH + Double Ratchet) for Obscura E2EE. Use when working on prekeys, ratchet state, or message encryption.
---

# Signal Protocol (X3DH + Double Ratchet)

## Why Signal

Spec mandate (Bölüm 4.3): "Birebir mesajlar için Signal Protocol". Industry standard, formally verified, used by WhatsApp/Signal/Wire.

## Recommended libraries

- **Rust**: `libsignal-protocol-rust` (Signal's official) → use via FFI for production
- **TypeScript** (browser): `@signalapp/libsignal-client` (WASM)
- **Go**: no official; use Rust via subprocess or stdlib for parts

## Key types

| Key | Lifetime | Purpose |
|-----|----------|---------|
| Identity Key (Ed25519 + X25519 derived) | Permanent | DID derivation, signing |
| Signed PreKey | ~1 week | Signed by identity, used in X3DH |
| One-time PreKey | Single use | Used in X3DH, then deleted |
| Ratchet Key (per session) | Updated each msg | Forward secrecy |

## X3DH handshake (initial session)

Initiator (Alice) wants to message Bob:

```
1. Alice fetches Bob's prekey bundle from server:
   { identity_key, signed_prekey, signed_prekey_signature, one_time_prekey }

2. Alice verifies signed_prekey_signature with Bob's identity_key

3. Alice generates ephemeral key EK_a

4. Compute shared secrets (DH operations):
   DH1 = DH(IK_a, SPK_b)
   DH2 = DH(EK_a, IK_b)
   DH3 = DH(EK_a, SPK_b)
   DH4 = DH(EK_a, OPK_b)  // if one-time prekey used

5. Master key = KDF(DH1 || DH2 || DH3 || DH4)

6. Alice initializes Double Ratchet with master_key + Bob's SPK as DH partner
7. Alice sends initial message:
   { sender_identity, ephemeral_key, used_one_time_prekey_id, ciphertext }
```

Bob processes:
```
1. Use his identity_key + signed_prekey + matched one_time_prekey to compute same DH outputs
2. Derive same master_key
3. Initialize Double Ratchet
4. Decrypt message
5. Delete one-time_prekey (single use)
```

## Double Ratchet (per-message)

Each message:
1. Sender ratchets sending chain forward → new message key
2. Encrypt plaintext with message key (AES-256-GCM)
3. Send: { ratchet_public_key, message_number, previous_chain_length, ciphertext }

On receive:
1. If ratchet_public_key changed → DH ratchet step (new root key, new chain)
2. Skip stored message keys for this chain → up to message_number
3. Derive message key for this index → decrypt
4. Store skipped keys (out-of-order delivery)

## Storage (SQLite)

```sql
CREATE TABLE signal_sessions (
    peer_did TEXT PRIMARY KEY,
    state_blob BLOB NOT NULL,  -- encrypted with device key
    updated_at INTEGER NOT NULL
);

CREATE TABLE signal_prekeys (
    id INTEGER PRIMARY KEY,
    private_key BLOB NOT NULL,  -- encrypted
    used INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE signal_signed_prekey (
    id INTEGER PRIMARY KEY,
    private_key BLOB NOT NULL,  -- encrypted
    public_key BLOB NOT NULL,
    signature BLOB NOT NULL,
    created_at INTEGER NOT NULL
);
```

## Server endpoints (Obscura)

```
POST /v1/keys/upload
{
  "identity_key": "<base64>",
  "signed_prekey": { "id": 1, "key": "<base64>", "signature": "<base64>" },
  "one_time_prekeys": [
    { "id": 1, "key": "<base64>" },
    ... (100 prekeys)
  ]
}

GET /v1/keys/{did}
→ {
  "identity_key": "...",
  "signed_prekey": { ... },
  "one_time_prekey": { "id": N, "key": "..." }   // server picks unused, marks used
}
```

Server NEVER sees private keys. Stores only public material for distribution.

## PreKey replenishment

Client checks remaining unused one-time prekeys after each session init. If <20 remaining, generate 100 new ones and upload.

## Critical security rules

1. **Identity key never leaves device** — stored in OS keychain (iOS/macOS), Keystore (Android), DPAPI (Windows)
2. **State storage encrypted at rest** — SQLCipher or device-bound encryption
3. **No plaintext logs ever** — log only opaque IDs
4. **One-time prekey deleted after first use** — server marks `used=1`, deletes after grace period
5. **Signed prekey rotation** — every 7 days, keep old one for 30 days for late messages
6. **Verify signed prekey signature** — abort handshake if invalid
7. **Ratchet state persistence** — must be atomic; partial state = corruption = re-init session

## Test against vectors

Signal publishes test vectors at https://github.com/signalapp/libsignal/tree/main/tests
Run conformance tests on every release.

## File ownership (Obscura)

- `crypto/src/signal/**` — Rust crate (FAZ 1 target)
- `frontend/lib/e2ee.ts` + `e2ee-session.ts` — current TS impl
- `mobile/lib/e2ee.ts` (TODO)
- `backend/internal/api/keys.go` — server-side prekey distribution
