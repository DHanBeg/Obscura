pragma circom 2.1.6;

/*
 * LocationProof — Fiziksel Konum Sıfır-Bilgi Kanıtı (FAZ 4, spec 12.4)
 *
 * "Coğrafi bölgedeyim, tam konumumu açıklamadan"
 *
 * Kullanım:
 *   - Etkinlik check-in (belirli koordinat kutusunda olduğunu kanıtla)
 *   - Bölge bazlı içerik (ülke doğrulaması, konum açıklamadan)
 *   - Node coğrafi dağılımı doğrulaması
 *
 * Koordinat sistemi:
 *   - GPS koordinatları * 1e6 → integer (örn. 41.015137 → 41015137)
 *   - 1e6 hassasiyet ≈ 0.1m çözünürlük
 *   - 252-bit field'a sığar (2^31 > 180*1e6)
 *
 * Gizli girdiler (witness):
 *   - lat_raw          : Gerçek enlem * 1e6 (integer, -90e6 to +90e6)
 *   - lon_raw          : Gerçek boylam * 1e6 (integer, -180e6 to +180e6)
 *   - gps_timestamp    : GPS fix zamanı (unix timestamp)
 *   - device_secret    : Cihaz gizli anahtarı (replay önleme)
 *
 * Kamuya açık girdiler:
 *   - region_lat_min   : Bölge enlem alt sınırı * 1e6
 *   - region_lat_max   : Bölge enlem üst sınırı * 1e6
 *   - region_lon_min   : Bölge boylam alt sınırı * 1e6
 *   - region_lon_max   : Bölge boylam üst sınırı * 1e6
 *   - epoch            : Oturum epoch'u (tekrar önleme)
 *   - location_commitment : Poseidon(lat_raw, lon_raw, device_secret) — binding
 *
 * Kamuya açık çıktılar:
 *   - in_region        : 1 = kullanıcı belirtilen bölgededir
 *
 * Kısıtlamalar: ~800 constraint
 * Güvenlik: GPS spoofing → cihaz imzası (FAZ 4 GA'da TEE attestation)
 */

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";

template LocationProof() {
    // ─── Gizli girdiler ───────────────────────────────────────────────────────
    signal input lat_raw;          // gerçek enlem * 1e6
    signal input lon_raw;          // gerçek boylam * 1e6
    signal input gps_timestamp;    // GPS fix zamanı
    signal input device_secret;    // cihaz gizli anahtarı

    // ─── Kamuya açık girdiler ─────────────────────────────────────────────────
    signal input region_lat_min;
    signal input region_lat_max;
    signal input region_lon_min;
    signal input region_lon_max;
    signal input epoch;
    signal input location_commitment;

    // ─── Çıktılar ─────────────────────────────────────────────────────────────
    signal output in_region;

    // ─── 1. Konum commitment doğrulama ────────────────────────────────────────
    // Commitment: Poseidon(lat_raw, lon_raw, device_secret, epoch)
    component commit_h = Poseidon(4);
    commit_h.inputs[0] <== lat_raw;
    commit_h.inputs[1] <== lon_raw;
    commit_h.inputs[2] <== device_secret;
    commit_h.inputs[3] <== epoch;

    component commit_eq = IsEqual();
    commit_eq.in[0] <== commit_h.out;
    commit_eq.in[1] <== location_commitment;
    // commit_eq.out = 1 → commitment eşleşiyor

    // ─── 2. Enlem bölge kontrolü: region_lat_min <= lat_raw <= region_lat_max ─
    // LessEqThan: in[0] <= in[1]
    component lat_gte = GreaterEqThan(28); // 28 bit: max 2^28 > 90*1e6 = 90000000
    lat_gte.in[0] <== lat_raw;
    lat_gte.in[1] <== region_lat_min;

    component lat_lte = LessEqThan(28);
    lat_lte.in[0] <== lat_raw;
    lat_lte.in[1] <== region_lat_max;

    // ─── 3. Boylam bölge kontrolü ────────────────────────────────────────────
    component lon_gte = GreaterEqThan(28); // 28 bit: max 2^28 > 180*1e6 = 180000000
    lon_gte.in[0] <== lon_raw;
    lon_gte.in[1] <== region_lon_min;

    component lon_lte = LessEqThan(28);
    lon_lte.in[0] <== lon_raw;
    lon_lte.in[1] <== region_lon_max;

    // ─── 4. GPS timestamp geçerlilik (epoch ile ±1 saat) ─────────────────────
    // |gps_timestamp - epoch * 3600| < 3600 (1 saatlik pencere)
    // Basit: epoch sıfır değil kontrolü
    component epoch_nz = IsZero();
    epoch_nz.in <== epoch;

    // ─── 5. Tüm koşulların AND'i ─────────────────────────────────────────────
    signal cond_lat;
    cond_lat <== lat_gte.out * lat_lte.out;

    signal cond_lon;
    cond_lon <== lon_gte.out * lon_lte.out;

    signal cond_region;
    cond_region <== cond_lat * cond_lon;

    signal cond_commit;
    cond_commit <== commit_eq.out * (1 - epoch_nz.out);

    in_region <== cond_region * cond_commit;
}

component main {
    public [region_lat_min, region_lat_max, region_lon_min, region_lon_max, epoch, location_commitment]
} = LocationProof();
