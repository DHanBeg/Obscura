//! End-to-end MLS group flow tests
//! Spec: docs/spec/obscura_spec_v3.txt Bölüm 6.3

use super::*;
use openmls::prelude::tls_codec::{Deserialize, Serialize};

/// Extract Welcome from an outgoing MLS message; panics if not a Welcome.
fn welcome_out(msg: MlsMessageOut) -> Welcome {
    let bytes = msg.tls_serialize_detached().expect("serialize welcome");
    let in_msg = MlsMessageIn::tls_deserialize_exact(bytes.as_slice()).expect("deserialize");
    match in_msg.extract() {
        MlsMessageBodyIn::Welcome(w) => w,
        other => panic!("expected Welcome, got {:?}", other),
    }
}

#[test]
fn full_two_party_flow() {
    // Setup: Alice and Bob each have their own provider + identity
    let alice_provider = new_provider();
    let bob_provider = new_provider();

    let alice = Identity::generate("did:obs:alice", &alice_provider).unwrap();
    let bob = Identity::generate("did:obs:bob", &bob_provider).unwrap();

    // 1. Bob publishes a KeyPackage so Alice can add him
    let bob_kp = generate_key_package(&bob, &bob_provider).unwrap();

    // 2. Alice creates a group
    let mut alice_group = create_group(&alice, &alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 0);

    // 3. Alice adds Bob (epoch 0 → 1)
    let (_commit, welcome_msg) =
        add_member(&mut alice_group, &alice, bob_kp, &alice_provider).unwrap();
    assert_eq!(alice_group.epoch().as_u64(), 1);

    // 4. Bob processes the Welcome → joins the group
    let welcome = welcome_out(welcome_msg);
    let mut bob_group = process_welcome(welcome, &bob_provider).unwrap();
    assert_eq!(bob_group.epoch().as_u64(), 1);

    // 5. Alice sends an encrypted message
    let plaintext = b"hello bob, this is end-to-end encrypted via MLS";
    let ciphertext = encrypt_message(&mut alice_group, &alice, plaintext, &alice_provider)
        .unwrap();

    // 6. Bob decrypts
    let serialized: Vec<u8> = ciphertext
        .tls_serialize_detached()
        .expect("serialize");
    let in_msg = MlsMessageIn::tls_deserialize_exact(serialized.as_slice())
        .expect("deserialize");

    let decrypted = process_message(&mut bob_group, in_msg, &bob_provider)
        .unwrap()
        .expect("application message expected");

    assert_eq!(decrypted, plaintext);
}

#[test]
fn three_party_message_flow() {
    let p_alice = new_provider();
    let p_bob = new_provider();
    let p_carol = new_provider();

    let alice = Identity::generate("did:obs:alice", &p_alice).unwrap();
    let bob = Identity::generate("did:obs:bob", &p_bob).unwrap();
    let carol = Identity::generate("did:obs:carol", &p_carol).unwrap();

    let bob_kp = generate_key_package(&bob, &p_bob).unwrap();
    let carol_kp = generate_key_package(&carol, &p_carol).unwrap();

    // Alice creates group
    let mut g_alice = create_group(&alice, &p_alice).unwrap();

    // Add Bob (epoch 1)
    let (_c1, w1) = add_member(&mut g_alice, &alice, bob_kp, &p_alice).unwrap();
    let mut g_bob = process_welcome(welcome_out(w1), &p_bob).unwrap();

    // Add Carol (epoch 2 — both Alice's group and Bob's group must process this)
    let (commit2, w2) = add_member(&mut g_alice, &alice, carol_kp, &p_alice).unwrap();
    let mut g_carol = process_welcome(welcome_out(w2), &p_carol).unwrap();

    // Bob processes the commit to advance to epoch 2
    let commit_bytes = commit2.tls_serialize_detached().unwrap();
    let commit_in = MlsMessageIn::tls_deserialize_exact(commit_bytes.as_slice()).unwrap();
    process_message(&mut g_bob, commit_in, &p_bob).unwrap();

    // All at epoch 2
    assert_eq!(g_alice.epoch().as_u64(), 2);
    assert_eq!(g_bob.epoch().as_u64(), 2);
    assert_eq!(g_carol.epoch().as_u64(), 2);

    // Alice sends a message; Bob and Carol decrypt
    let pt = b"hi all from alice";
    let ct = encrypt_message(&mut g_alice, &alice, pt, &p_alice).unwrap();
    let bytes = ct.tls_serialize_detached().unwrap();

    let in_bob = MlsMessageIn::tls_deserialize_exact(bytes.as_slice()).unwrap();
    let dec_bob = process_message(&mut g_bob, in_bob, &p_bob).unwrap().unwrap();
    assert_eq!(dec_bob, pt);

    let in_carol = MlsMessageIn::tls_deserialize_exact(bytes.as_slice()).unwrap();
    let dec_carol = process_message(&mut g_carol, in_carol, &p_carol).unwrap().unwrap();
    assert_eq!(dec_carol, pt);
}

/// L2 Tuğla 3 — openmls↔ts-mls interop, tek yön (ts-mls üretir → openmls çözer).
/// E1'in interop kalbi. Golden fixture (mobile/lib/mls/fixtures/two_party_golden.json,
/// Tuğla 2A'da ts-mls ile üretilip DONDURULMUŞ) — openmls Bob rolünü SADECE wire
/// byte'lardan + Bob'un özel anahtarından üstlenip aynı epoch_authenticator'a ve
/// aynı plaintext'e ulaşıyor mu.
///
/// Desen openmls'in KENDİ RFC vektör test harness'ından alındı
/// (openmls-0.6.0/src/group/mls_group/tests_and_kats/kats/passive_client.rs) —
/// icat edilmedi, openmls'in kendi kanıtlı yolunu izliyor.
#[test]
fn interop_openmls_decrypts_ts_mls_golden_fixture() {
    assert_openmls_reproduces_fixture(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../mobile/lib/mls/fixtures/two_party_golden.json"
    ));
}

/// Go tarafının (backend/internal/api/mls_relay_golden_test.go) relay'den GERİ
/// OKUDUĞU wire'ları yazdığı geçici fixture'ın yolu.
const RELAYED_FIXTURE_ENV: &str = "OBSCURA_MLS_RELAYED_FIXTURE";

/// L2 Tuğla 4d — E1'in üçüncü halkası: golden wire ARTIK doğrudan fixture'dan
/// değil, GERÇEK Go relay'inden (httptest + gerçek router + gerçek SQLite +
/// gerçek JWT) geri okunmuş hâliyle geliyor. Aynı Tuğla 3 kanıt gövdesi
/// çalıştırılır: openmls bu wire'lardan Bob rolünü üstlenip aynı
/// epoch_authenticator'a ve aynı plaintext'e ulaşmalı.
///
/// Go testi bu testi `cargo test` alt-süreci olarak çağırır (subprocess-CLI
/// deseni, CGO/FFI yok). Env değişkeni yoksa test no-op'tur — düz `cargo test`
/// koşusunu kırmaz.
#[test]
fn interop_openmls_decrypts_relayed_golden_wire() {
    let fixture_path = match std::env::var(RELAYED_FIXTURE_ENV) {
        Ok(p) if !p.is_empty() => p,
        _ => {
            eprintln!(
                "[atlandı] {} ayarlı değil — bu test yalnızca Go relay testi tarafından sürülür",
                RELAYED_FIXTURE_ENV
            );
            return;
        }
    };
    assert_openmls_reproduces_fixture(&fixture_path);
}

/// Tuğla 3'ün kanıt gövdesi — fixture yoluna göre parametrik.
/// Davranış Tuğla 3'teki hâliyle birebir aynıdır (commit 461fa99); tek fark
/// yolun sabit yerine argüman olması, böylece relay'den dönen wire'lar da
/// AYNI kanıttan geçirilebiliyor.
fn assert_openmls_reproduces_fixture(fixture_path: &str) {
    use base64::Engine as _;
    use openmls_traits::storage::StorageProvider;

    let raw = std::fs::read_to_string(fixture_path).unwrap_or_else(|e| {
        panic!(
            "[ADIM: fixture-oku] golden fixture okunamadı ({}): {}",
            fixture_path, e
        )
    });
    let fixture: serde_json::Value = serde_json::from_str(&raw)
        .unwrap_or_else(|e| panic!("[ADIM: fixture-oku] JSON parse hatası: {}", e));

    let b64 = |field: &str| -> Vec<u8> {
        let s = fixture[field]
            .as_str()
            .unwrap_or_else(|| panic!("[ADIM: fixture-oku] alan eksik/string değil: {}", field));
        base64::engine::general_purpose::STANDARD
            .decode(s)
            .unwrap_or_else(|e| panic!("[ADIM: fixture-oku] base64 decode hatası ({}): {}", field, e))
    };
    let priv_field = |field: &str| -> Vec<u8> {
        let s = fixture["bob_private_key_package"][field]
            .as_str()
            .unwrap_or_else(|| panic!("[ADIM: fixture-oku] bob_private_key_package.{} eksik", field));
        base64::engine::general_purpose::STANDARD
            .decode(s)
            .unwrap_or_else(|e| panic!("[ADIM: fixture-oku] base64 decode hatası (bob_private_key_package.{}): {}", field, e))
    };

    let bob_kp_wire = b64("bob_key_package_wire_b64");
    let welcome_wire = b64("welcome_wire_b64");
    let app_msg_wire = b64("application_message_wire_b64");
    let expected_plaintext = fixture["plaintext"]
        .as_str()
        .unwrap_or_else(|| panic!("[ADIM: fixture-oku] plaintext eksik"));
    let expected_epoch_auth_hex = fixture["epoch_authenticator_hex"]
        .as_str()
        .unwrap_or_else(|| panic!("[ADIM: fixture-oku] epoch_authenticator_hex eksik"));
    let expected_epoch = fixture["epoch"]
        .as_u64()
        .unwrap_or_else(|| panic!("[ADIM: fixture-oku] epoch eksik"));

    let init_priv = priv_field("init_private_key_b64");
    let hpke_priv = priv_field("hpke_private_key_b64");

    let provider = new_provider();

    // ─── ADIM: keypackage-inject — Bob'un ts-mls'te üretilmiş KeyPackage'ını
    // + özel anahtarını openmls'in storage'ına elle yerleştir. openmls'in
    // KENDİ passive_client.rs KAT'ı aynı deseni kullanıyor. KeyPackageBundle::new
    // SADECE "test-utils" feature'ı altında public (Cargo.toml dev-dependencies).
    let kp_msg = MlsMessageIn::tls_deserialize_exact(bob_kp_wire.as_slice())
        .unwrap_or_else(|e| panic!("[ADIM: keypackage-inject] Bob KeyPackage wire deserialize hatası: {:?}", e));
    let key_package: KeyPackage = match kp_msg.extract() {
        MlsMessageBodyIn::KeyPackage(kp_in) => kp_in
            .validate(provider.crypto(), ProtocolVersion::Mls10)
            .unwrap_or_else(|e| panic!("[ADIM: keypackage-inject] KeyPackageIn::validate hatası: {:?}", e)),
        other => panic!("[ADIM: keypackage-inject] beklenmeyen wireformat: {:?}", other),
    };

    // KeyPackageBundle'ın alanları pub(crate), constructor'ı (::new) SADECE
    // "test-utils" feature'ı altında public — o feature wasm-bindgen-test'i
    // sürüklüyor, wasm-bindgen-test → windows-sys, windows-sys bu makinede
    // eksik olan dlltool.exe'yi istiyor (GNU toolchain, MinGW binutils tam
    // kurulu değil) → derlenmiyor. Bypass: KeyPackageBundle KOŞULSUZ olarak
    // Serialize+Deserialize türetiyor (openmls'in kendisi storage için
    // kullanıyor, test-utils'e bağlı değil) — serde üzerinden inşa edip
    // constructor privacy'sini (API kısıtı, tip güvenliğini değil) atlıyoruz.
    // EncryptionPrivateKey tipi de crate-private (treesync::node pub(crate)) —
    // Rust tipini hiç kurmuyoruz, sadece onun serde şeklini (tek alan "key",
    // HpkePrivateKey ile aynı iç yapı) ham JSON olarak taklit ediyoruz.
    let priv_init_key: HpkePrivateKey = init_priv.into();
    let priv_enc_inner: HpkePrivateKey = hpke_priv.clone().into();
    let priv_enc_inner_json = serde_json::to_value(&priv_enc_inner)
        .unwrap_or_else(|e| panic!("[ADIM: keypackage-inject] HpkePrivateKey serialize hatası: {}", e));
    let bundle_json = serde_json::json!({
        "key_package": key_package,
        "private_init_key": priv_init_key,
        "private_encryption_key": { "key": priv_enc_inner_json },
    });
    let bundle: KeyPackageBundle = serde_json::from_value(bundle_json).unwrap_or_else(|e| {
        panic!(
            "[ADIM: keypackage-inject] KeyPackageBundle serde-bypass inşası başarısız (alan adı uyuşmazlığı olabilir): {}",
            e
        )
    });

    let hash_ref = key_package
        .hash_ref(provider.crypto())
        .unwrap_or_else(|e| panic!("[ADIM: keypackage-inject] hash_ref hatası: {:?}", e));
    provider
        .storage()
        .write_key_package(&hash_ref, &bundle)
        .unwrap_or_else(|e| panic!("[ADIM: keypackage-inject] write_key_package hatası: {:?}", e));

    // ─── ADIM: welcome-process — SADECE wire byte'lardan gruba katıl.
    let welcome_msg = MlsMessageIn::tls_deserialize_exact(welcome_wire.as_slice())
        .unwrap_or_else(|e| panic!("[ADIM: welcome-process] Welcome wire deserialize hatası: {:?}", e));
    let welcome = match welcome_msg.extract() {
        MlsMessageBodyIn::Welcome(w) => w,
        other => panic!("[ADIM: welcome-process] mesaj Welcome değil: {:?}", other),
    };

    let join_config = MlsGroupJoinConfig::builder()
        .use_ratchet_tree_extension(true)
        .build();

    let staged = StagedWelcome::new_from_welcome(&provider, &join_config, welcome, None)
        .unwrap_or_else(|e| panic!("[ADIM: welcome-process] StagedWelcome::new_from_welcome hatası: {:?}", e));
    let group = staged
        .into_group(&provider)
        .unwrap_or_else(|e| panic!("[ADIM: welcome-process] into_group hatası: {:?}", e));

    // ─── DOĞRULA (a): epoch_authenticator byte-byte eşleşmeli (ts-mls'in
    // hesapladığıyla openmls'in hesapladığı AYNI RFC 9420 değeri).
    let actual_epoch_auth_hex = hex::encode(group.epoch_authenticator().as_slice());
    assert_eq!(
        actual_epoch_auth_hex, expected_epoch_auth_hex,
        "[ADIM: epoch-auth] epoch_authenticator eşleşmedi — openmls ve ts-mls FARKLI state'e ulaştı"
    );

    // ─── DOĞRULA (c): epoch numarası.
    assert_eq!(
        group.epoch().as_u64(),
        expected_epoch,
        "[ADIM: epoch-auth] epoch numarası eşleşmedi"
    );

    // ─── DOĞRULA (b): ts-mls'in şifrelediği application-message'ı openmls çözsün.
    let app_msg = MlsMessageIn::tls_deserialize_exact(app_msg_wire.as_slice())
        .unwrap_or_else(|e| panic!("[ADIM: app-decrypt] application-message wire deserialize hatası: {:?}", e));
    // mod.rs::process_message ile AYNI desen (extract()+match) — tahmin değil,
    // bu repoda zaten kanıtlanmış çalışan kod.
    let protocol_msg: ProtocolMessage = match app_msg.extract() {
        MlsMessageBodyIn::PrivateMessage(m) => m.into(),
        MlsMessageBodyIn::PublicMessage(m) => m.into(),
        other => panic!("[ADIM: app-decrypt] beklenmeyen wireformat: {:?}", other),
    };

    let mut group = group;
    let processed = group
        .process_message(&provider, protocol_msg)
        .unwrap_or_else(|e| panic!("[ADIM: app-decrypt] process_message hatası: {:?}", e));

    let plaintext = match processed.into_content() {
        ProcessedMessageContent::ApplicationMessage(app) => app.into_bytes(),
        other => panic!("[ADIM: app-decrypt] application-message değil: {:?}", other),
    };

    let plaintext_str = String::from_utf8(plaintext).expect("plaintext UTF-8 olmalı");
    // Çağıran süreç (Tuğla 4d'de Go relay testi) çözülen plaintext'i stdout'tan
    // doğrulayabilsin diye yazdırılır — assert'in yerine geçmez, ek görünürlük.
    eprintln!("[çözüldü] plaintext = {}", plaintext_str);
    assert_eq!(
        plaintext_str, expected_plaintext,
        "[ADIM: app-decrypt] çözülen plaintext beklenenle eşleşmedi"
    );
}
