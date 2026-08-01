# ADR 0017: BFT konsensüsün gerçek kapsamı — OBS ledger mutabakatı, governance değil

Date: 2026-08-02
Status: Accepted
Decider: project lead
Spec ref: Bölüm 12.3 (FAZ 3), Modül D "Consensus Layer"

## Context

`internal/consensus/bft.go` — Tendermint-tarzı propose/prevote/precommit/commit
motoru — main.go dışında hiçbir yerden çağrılmıyordu (`if false` bloğu içinde,
bkz. commit b5521c3). Kod kendisi çalışır durumda (round state machine, oy
toplama, quorum mantığı doğru) ama şu 4 parça hiç yazılmamıştı: leader
election, gerçek transport, validator-set/stake bağlantısı, mempool.

Bu boşluğu doldurmadan önce asıl soru: **bu motor gerçekte neyi konsensüse
bağlayacak?**

Spec'in "MODUL D: CONSENSUS LAYER" tanımı (satır 194-215) şunu tarif ediyor:
"Node yönetimi, multisig, güncelleme onayı" — 3/5 imza + ZK oy + 48 saat
timelock + otomatik uygulama. **Bu, `internal/dao` + `internal/governance`
tarafından ZATEN karşılanmış** (timelock, guardian veto, ZK vote, süper
çoğunluk — Phase-Status.md FAZ 4). Yani spec'in orijinal "Consensus Layer"
maddesi bft.go'yu gerektirmiyor olabilir.

Gerçek boşluk başka yerde: `ADR-0005` (Aztec rollup), satır 72 —
*"Settlement is to our own SQLite ledger; no external rollup involved."*
`internal/token/token.go`'daki `Transfer`/`Mint` grep edildi — sıfır
federation/p2p/broadcast çağrısı, sadece yerel SQLite yazımı. Federasyondaki
her node kendi OBS bakiye durumunu bağımsız tutuyor; node'lar arası hiçbir
mutabakat mekanizması yok. Permissionless çok-node federasyon (FAZ 3'ün asıl
amacı) gerçekten ayakta olduğunda bakiyeler ıraksayabilir.

`internal/sequencer` zaten gerçek VRF (stub değil, ECDSA-tabanlı) ile
stake-ağırlıklı epoch rotasyonu yapıyor (`sequencer.Global`, main.go'da
`staking.NodeOperatorStakeOBS` ile canlı bağlı), `ActiveSequencer()` ile
"bu epoch'un lideri kim" sorusuna cevap veriyor, `SubmitBatch`/
`computeMerkleRoot` ile tx-hash listesinden merkle root üretiyor.

## Decision

BFT'nin gerçek işi: **OBS token ledger'ındaki durum-değiştiren
operasyonların (mint/transfer/stake/slash/bridge-unlock) sırasını ve
içeriğini federasyondaki node'lar arasında Byzantine-toleranslı şekilde
anlaştırmak.** "Blok" = bir grup ledger operasyonu.

Leader-election ve mempool sıfırdan YAZILMAYACAK — `internal/sequencer`
üzerine kurulacak:
- Proposer = `sequencer.Global.ActiveSequencer()` (mevcut, test edilmiş, canlı).
- txRoot = sequencer'ın merkle-root mekanizması (mevcut `computeMerkleRoot`).
- BFT'nin katkısı sadece: bu öneriye çoğunluk onayı toplamak (prevote/
  precommit/quorum) ve deterministik, tek-seferlik commit sağlamak.

Governance/multisig/timelock için `internal/dao` zaten yeterli — bft.go
onu tekrar etmeyecek.

## Kapsam (bu ADR ile onaylanan uygulama sırası)

Tek-node döngüsü önce (3. node'suz test edilebilir çekirdek):
0 (bu ADR) → 1 (proposer=sequencer) → 2 (mempool/txRoot=sequencer) →
6 (persistence+parentHash) → 7 (onCommit→ledger apply, idempotency/replay-guard) →
8 (main.go wiring, if false kaldır) → 9 (testler).

Ertelenen (ikinci node hazır olmadan anlamsız/test edilemez):
3 (GossipSub transport), 4 (oy imzalama+doğrulama), 5 (stake-ağırlıklı
validator-set/quorum), 10 (çok-node smoke test).

## Consequences

- **Positive:** Sıfırdan leader-election/mempool yazılmıyor, mevcut
  test edilmiş sequencer koduna dayanıyor — daha az yeni yüzey, daha az risk.
- **Positive:** Governance ile karışmıyor, dao.go'ya dokunulmuyor.
- **Negative:** Tek-node döngüsü BFT'nin asıl değerini (Byzantine hata
  toleransı, çok-node mutabakatı) henüz kanıtlamaz — bu ancak adım 3-4-5-10
  ile gelir. Tek-node fazı sadece "state machine + ledger-apply doğru
  çalışıyor mu" sorusuna cevap verir.
- **Negative:** Adım 7 (onCommit → gerçek token.Transfer/Mint çağrıları)
  para/bakiye etkiliyor — idempotency/replay-guard eksiksiz olmazsa
  double-mint/double-transfer riski var. Bu adımda özel dikkat gerekiyor
  (kullanıcı tarafından da açıkça istendi).
