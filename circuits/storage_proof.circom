pragma circom 2.1.6;

/*
 * StorageProof v2 — Obscura Veri Saklama Kanıtı (spec Bölüm 17.5)
 *
 * Node, kendisinde tutulan bir veri parçasını gerçekten depoladığını,
 * verinin içeriğini açıklamadan kanıtlar.
 *
 * Kullanım:
 *   - Storage Proof of Retrievability (PoR)
 *   - Spec Bölüm 2.1: "Node proof'u blockchain'e submit eder, diğer
 *     node'lar proof'u doğrular, veriyi indirmek zorunda kalmaz"
 *
 * v1 sorunu (audit 2026-05-13):
 *   - `valid <== 1` anlamsızdı: kısıtlamalar geçtiğinde her zaman 1.
 *   - Public input'lar (data_commitment, timestamp, ttl, shard_id) bağlı
 *     değildi. Node tek bir proof üretip onu farklı shard'larda yeniden
 *     kullanabilirdi.
 *
 * v2 düzeltmesi:
 *   - epoch (yeni public input) eski proof'ları geçersiz kılar.
 *   - proof_commitment = Poseidon(data_hash, shard_id, timestamp, epoch)
 *     çıktısı çıkar; backend (shard_id, node_secret, epoch) başına gözlenen
 *     proof_commitment'ları saklar ve duplicate'leri reddeder.
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
 *   - epoch           : Geçerlilik dönemi (gün/ saat numarası)
 *
 * Kamuya açık çıktı:
 *   - proof_commitment : Poseidon(data_hash, shard_id, timestamp, epoch)
 *                        Backend bunu (shard_id, node_secret, epoch)
 *                        başına unique tutar, duplicate'i reject eder.
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
    signal input epoch;            // Geçerlilik dönemi (yeni — eski proof'ları kapatır)

    // ─── Kamuya Açık Çıktılar ─────────────────────────────────────────────────
    signal output proof_commitment;

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

    // 4. epoch != 0 (geçerli epoch)
    component epochCheck = IsZero();
    epochCheck.in <== epoch;
    epochCheck.out === 0;

    // 5. TTL > 0 — saklama süresi devam ediyor mu?
    //    ttl 64-bit unsigned olarak değerlendirilir
    component ttlCheck = LessThan(64);
    ttlCheck.in[0] <== 0;
    ttlCheck.in[1] <== ttl;     // 0 < ttl ↔ ttl > 0
    ttlCheck.out === 1;

    // 6. Proof binding: data_hash, shard_id, timestamp, epoch tek bir
    //    çıktı içinde birleşir. Aynı (shard_id, epoch) için aynı
    //    proof_commitment iki kez submit edilemez (backend duplicate-reject).
    component pc = Poseidon(4);
    pc.inputs[0] <== data_hash;
    pc.inputs[1] <== shard_id;
    pc.inputs[2] <== timestamp;
    pc.inputs[3] <== epoch;
    proof_commitment <== pc.out;
}

component main {public [data_commitment, timestamp, ttl, shard_id, epoch]} = StorageProof();
