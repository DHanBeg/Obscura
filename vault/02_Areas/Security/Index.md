# Security Domain

## Aktif denetim sonuçları

### FAZ 1 audit (2026-05-10) — 6 critical, hepsi FIXED

| ID | Konu | Fix |
|---|---|---|
| C1 | Credit upgrade replay | `zk_nullifiers` tablosu + UNIQUE constraint |
| C2 | user_hash forgery | `users.credit_user_hash` binding + immutable |
| C3 | credit_threshold circuit user_hash kısıtlanmıyor | Circuit v2: `Poseidon(secret, TAG) === user_hash` |
| C4 | Public input order brittle | Per-circuit length validation + named constants |
| C5 | Cross-signing account hijack | Signed msg: domain + challenge + pubkey + JWT.DID |
| C6 | ZK verifier no input count | `expectedPublicSignals` map + early reject |

Detay: [[../../../docs/adr/0009-faz1-post-audit-hardening|ADR-0009]]

### Deferred to FAZ 2 (11 medium/low)

- N3: Binding rotation flow (timelocked)
- N5: MLS subprocess pool (DoS amplification)
- N6: MLS handler post-commit error handling
- N9: HandlePairStart rate limiting
- N10: WebRTC token in subprotocol header (not URL)
- N11: Gossip HMAC instead of plaintext header
- + 5 more (bkz ADR-0009)

### FAZ 2 audit — PENDING

Rate limit nedeniyle çağrılamadı, sonraki turda 3 paralel ajan çalışacak: code-reviewer + security-auditor + spec-checker.

## Sub-agent

- [[../../../.claude/agents/security-auditor|security-auditor]]
- [[../../../.claude/agents/dependency-auditor|dependency-auditor]]

## 7 KESIN spec kuralı (Bölüm 4.5)

1. Private key asla sunucudan çıkmaz
2. Şifreleme Rust'ta (FAZ 1 sapma: Go)
3. Hiçbir node tam mesajı çözemez
4. Metadata minimum (timestamp, from, to)
5. 30 gün sonra mesajlar silinir
6. ZK proof'lar public input haricinde detay açıklamaz
7. ZK circuit'ler formel olarak doğrulanmıştır (audit edilmiş)

## Process

- Her major commit ÖNCESI: code-reviewer + security-auditor dispatch
- Crypto / auth / network dokunan her PR: security-auditor zorunlu
- "Done" demeden önce: spec-checker zorunlu
