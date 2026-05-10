pragma circom 2.1.6;

/*
 * StorageProof — Obscura Veri Saklama Kanıtı (spec Bölüm 17.5)
 *
 * Node, kendisinde tutulan bir veri parçasını gerçekten depoladığını,
 * verinin içeriğini açıklamadan kanıtlar.
 *
 * Kullanım:
 *   - Storage Proof of Retrievability (PoR)
 *   - Spec Bölüm 2.1: "Node proof'u blockchain'e submit eder, diğer
 *     node'lar proof'u doğrular, veriyi indirmek zorunda kalmaz"
 *
 * Gizli girdiler (witness):
 *   - data_hash       : İçerik Poseidon hash'i (256-bit field element)
 *   - node_secret     : Node kimlik gizli anahtarı (replay/spoof önlemi)
 *
 * Kamuya açık girdiler:
 *   - data_commitment : Beklenen Poseidon(data_hash, node_secret)
 *   - timestamp       : Saklama anı (epoch, replay window)
 *   - ttl             : Kalan TTL (saniye)
 *   - shard_id        : Hangi shard'ı kanıtlıyor
 *
 * Kamuya açık çıktı:
 *   - valid           : 1 = geçerli (commit eşleşti + ttl > 0)
 *
 * NOT: Bu basitleştirilmiş bir PoR. Spec Bölüm 2.1'de yer alan tam
 * "node verinin tamamını tutuyor" kanıtı için Merkle tree challenge-
 * response veya Compact PoR şart. Bu version "node bu shard'ı tanıyor
 * ve TTL'i geçerli" kanıtı verir.
 */

include "circomlib/circuits/poseidon.circom";
include "circomlib/circuits/comparators.circom";

template StorageProof() {
    // ─── Gizli Girdiler ───────────────────────────────────────────────────────
    signal input data_hash;     // Veri içeriğinin Poseidon hash'i
    signal input node_secret;   // Node gizli anahtarı

    // ─── Kamuya Açık Girdiler ─────────────────────────────────────────────────
    signal input data_commitment;  // Beklenen Poseidon(data_hash, node_secret)
    signal input timestamp;        // Saklama zamanı
    signal input ttl;              // Kalan TTL
    signal input shard_id;         // Shard tanımlayıcısı

    // ─── Kamuya Açık Çıktılar ─────────────────────────────────────────────────
    signal output valid;

    // ─── Kısıtlamalar ─────────────────────────────────────────────────────────

    // 1. Commitment doğrulama: data_commitment = Poseidon(data_hash, node_secret)
    component dataHasher = Poseidon(2);
    dataHasher.inputs[0] <== data_hash;
    dataHasher.inputs[1] <== node_secret;
    dataHasher.out === data_commitment;

    // 2. timestamp != 0 (boş kanıt önleme)
    component tsCheck = IsZero();
    tsCheck.in <== timestamp;
    tsCheck.out === 0;

    // 3. shard_id != 0 (geçerli shard)
    component sidCheck = IsZero();
    sidCheck.in <== shard_id;
    sidCheck.out === 0;

    // 4. TTL > 0 — saklama süresi devam ediyor mu?
    //    ttl 64-bit unsigned olarak değerlendirilir
    component ttlCheck = LessThan(64);
    ttlCheck.in[0] <== 0;
    ttlCheck.in[1] <== ttl;     // 0 < ttl ↔ ttl > 0
    ttlCheck.out === 1;

    // 5. Her şey geçtiyse valid = 1
    valid <== 1;
}

component main {public [data_commitment, timestamp, ttl, shard_id]} = StorageProof();
