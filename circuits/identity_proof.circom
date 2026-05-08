pragma circom 2.1.6;

/*
 * IdentityProof — Obscura ZK Kimlik Devresi
 *
 * İspatlanan şey:
 *   Kullanıcı, gerçek kimliğini (telefon numarası / seed) açıklamadan
 *   Obscura DID'ine sahip olduğunu kanıtlar.
 *
 * Gizli girdiler (witness):
 *   - identity_secret  : Kimlik türetme gizli anahtarı (Poseidon hash girdisi)
 *   - phone_hash       : Poseidon(phone_number) — telefon numarasının hash'i
 *
 * Kamuya açık girdiler:
 *   - did_commitment   : Poseidon(identity_secret, phone_hash) = DID taahhüdü
 *   - nullifier_hash   : Poseidon(identity_secret, epoch) — tekrar kullanım önlemi
 *   - epoch            : Zaman dilimi (günlük, kanıtın geçerlilik süresi)
 *
 * Kamuya açık çıktılar:
 *   - verified         : 1 = kimlik doğrulandı
 *
 * Protokol:
 *   1. Kullanıcı identity_secret ve phone_hash bilir
 *   2. did_commitment = Poseidon(identity_secret, phone_hash) → zincirde kayıtlı
 *   3. nullifier = Poseidon(identity_secret, epoch) → epoch başına bir kez kullanılır
 *   4. İspat: "Bu commitment'ın sahibiyim, gizlimi açıklamıyorum"
 *
 * Uygulamalar:
 *   - Anonim grup üyeliği
 *   - Kimlik bağlama (DID ↔ telefon) sıfır bilgi ile
 *   - Sybil direnci (her telefon = bir identity)
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";
include "circomlib/circuits/bitify.circom";

template IdentityProof() {
    // ─── Gizli Girdiler ───────────────────────────────────────────────────────
    signal input identity_secret;    // Kimlik gizli anahtarı (256-bit random)
    signal input phone_hash;         // Poseidon(phone_number) — gizli kalır

    // ─── Kamuya Açık Girdiler ─────────────────────────────────────────────────
    signal input did_commitment;     // Poseidon(identity_secret, phone_hash)
    signal input nullifier_hash;     // Poseidon(identity_secret, epoch)
    signal input epoch;              // Zaman dilimi (unix_time / 86400 = gün sayısı)

    // ─── Kamuya Açık Çıktılar ─────────────────────────────────────────────────
    signal output verified;          // 1 = geçerli kimlik kanıtı

    // ─── Kısıtlamalar ─────────────────────────────────────────────────────────

    // 1. DID taahhüdü doğrulama
    //    did_commitment = Poseidon(identity_secret, phone_hash)
    component didCommit = Poseidon(2);
    didCommit.inputs[0] <== identity_secret;
    didCommit.inputs[1] <== phone_hash;
    didCommit.out === did_commitment;

    // 2. Nullifier hesaplama
    //    nullifier_hash = Poseidon(identity_secret, epoch)
    //    Aynı epoch'da aynı kimlik ile iki kez ispat üretilemez
    component nullifier = Poseidon(2);
    nullifier.inputs[0] <== identity_secret;
    nullifier.inputs[1] <== epoch;
    nullifier.out === nullifier_hash;

    // 3. Epoch sıfır olmayan (boş kanıt önleme)
    component epochCheck = IsZero();
    epochCheck.in <== epoch;
    epochCheck.out === 0;  // epoch != 0

    // 4. Çıktı: her iki taahhüt de doğruysa verified = 1
    //    Circom'da koşulsuz 1 atanır (kısıtlamalar yukarıda kontrol eder)
    verified <== 1;
}

component main {public [did_commitment, nullifier_hash, epoch]} = IdentityProof();
