================================================================================
OBSCURA PLATFORM MASTER SPECIFICATION v3.0 - PARÇA 2/5
Bölüm 6-9: Mesajlaşma, Kredi, Token, Client
================================================================================

================================================================================
BOLUM 6: MESAJLASMA SISTEMI
================================================================================

6.1 MESAJ TIPLERI

| Tip | Aciklama | Protokol | Max Boyut |
|-----|----------|----------|-----------|
| Text | Duz metin | Signal/MLS | 4KB |
| Image | Resim | Signal/MLS + MinIO | 10MB |
| Voice | Ses kaydi | Signal/MLS | 50MB |
| File | Dosya | Signal/MLS + MinIO | 100MB |
| Location | Konum | Signal/MLS | 1KB |
| Call Invite | Arama daveti | Signal/MLS | 512B |
| Call Accept | Arama kabul | Signal/MLS | 512B |
| Call End | Arama sonu | Signal/MLS | 512B |
| Group Invite | Grup daveti | MLS | 1KB |
| ZK Proof | Kredi kaniti | ZK Layer | 2KB |

6.2 BIREBIR MESAJLASMA AKISI (Signal Protocol)

Gonderen Client:
1. Alici DID'sini cozumle
2. Public key'i DHT/cache'den al
3. X3DH handshake (yeni oturum ise)
4. Double Ratchet ile sifrele
5. Ciphertext + metadata olustur
6. WebSocket ile node'a gonder

Node:
1. Gelen mesaji al
2. Alici node'unu DHT'den bul
3. Mesaji yonlendir (online) veya storage'a kaydet (offline)
4. Offline ise push bildirim tetikle

Alici Client:
1. Node'dan mesaji al (WebSocket veya polling)
2. Private key ile coz
3. Double Ratchet state guncelle
4. plaintext goster
5. Okundu bilgisi gonder (opsiyonel)

6.3 GRUP MESAJLASMA AKISI (MLS Protocol - YENI)

Grup Olusturma:
1. Olusturucu: Grup ID belirle
2. Her uye icin KeyPackage iste (DHT veya direkt)
3. MLS group olustur (TreeKEM agaci)
4. Welcome mesaji her uyeye gonder
5. Group state senkronize et

Grup Mesaji Gonderme:
1. Gonderen: Grup state al
2. TreeKEM ile grup anahtar guncelle
3. MLS encrypt
4. Ciphertext broadcast
5. Her uye kendi LeafNode ile cozer

Uye Ekleme:
1. Yeni uye KeyPackage uret
2. Olusturucu: Add proposal olustur
3. Commit mesaji uret
4. Welcome + Commit yayinla
5. Tum uyeler state guncelle

Uye Cikarma:
1. Admin: Remove proposal olustur
2. Commit mesaji uret
3. Blank LeafNode ile yer tut
4. Kalan uyeler yeni agac ile devam

6.4 MESAJ DURUMLARI

| Durum | Aciklama |
|-------|----------|
| sending | Gonderiliyor |
| sent | Node'a ulasti |
| delivered | Alici node'una ulasti |
| read | Alici okudu |
| failed | Gonderim basarisiz |
| expired | TTL doldu |

6.5 OFFLINE MESAJ YONETIMI

Senario: Alici offline

1. Gonderen -> Node A
2. Node A -> Storage (shard'lara bol)
3. Node A -> DHT'ye metadata kaydet
4. Node A -> Push bildirim servisi (FCM/APNs)
5. Alici online olunca:
   a. Node B'ye baglanir
   b. DHT'den metadata ceker
   c. Storage node'lardan shard'lari toplar
   d. Reed-Solomon ile birlestirir
   e. Decrypt eder

6.6 MESAJ GECMISI VE SILME

Local Storage (Client):
- SQLite: Mesajlar local sifreli
- TTL: 30 gun (varsayilan)
- Otomatik silme: TTL dolunca

Node Storage:
- Shard'lar 30 gun veya okunana kadar
- Okunduktan sonra 7 gun (yedek)
- Tam silme: 7 gun sonra

Global Silme (Recall):
- Gonderen: "Bu mesaji geri al" talebi
- Node: Tum shard'lari sil
- Alici: Local kopyayi sil (eger online)
- ZK proof ile silme kaniti (YENI)

================================================================================
BOLUM 7: KREDI PUANI VE KATMAN SISTEMI (ZK TABANLI)
================================================================================

7.1 KREDI PUANI MATRISI

| Davranis | Puan | Frekans | Max | ZK Circuit |
|----------|------|---------|-----|------------|
| Hesap yasi | +1 | /ay | +24 | age_proof.circom |
| Gunluk giris | +0.5 | /gun | +15 | activity_proof.circom |
| Mesaj gonderme | +0.1 | /mesaj | +10 | msg_count_proof.circom |
| Sesli arama | +0.2 | /arama | +5 | call_proof.circom |
| Grup olusturma | +2 | /grup | +10 | group_proof.circom |
| Spam raporu (alma) | -5 | /rapor | -50 | spam_victim_proof.circom |
| Spam raporu (verme, yanlis) | -3 | /rapor | -30 | spam_false_proof.circom |
| Dolandiricilik | -20 | /olay | -100 | fraud_proof.circom |
| Topluluk katkisi | +5 | /katki | +25 | contribution_proof.circom |
| Node calistirma | +10 | /ay | +60 | node_proof.circom |
| Diger kullanici onayi | +1 | /onay | +20 | endorsement_proof.circom |
| Iyi davranis streak | +2 | /7gun | +20 | streak_proof.circom |

7.2 KATMAN AYRICALIKLARI

Katman 1: BRONZ (0-59)
- Birebir mesajlasma (metin)
- Sesli arama (1-1, 5 dk limit)
- Temel profil
- Gunluk: 50 mesaj limit

Katman 2: GUMUS (60-69)
- + Grup mesajlasma (max 50 kisi, Signal)
- + Sesli arama (limitsiz)
- + Goruntulu arama (1-1)
- + Dosya paylasimi (max 5MB)
- + Gunluk: 200 mesaj
- + Grup: max 100 kisi (MLS)

Katman 3: ALTIN (70-79)
- + Grup mesajlasma (max 500 kisi, MLS)
- + Goruntulu arama (grup, max 10)
- + Dosya paylasimi (max 50MB)
- + Mini app kullanimi
- + OBS wallet erisimi
- + Gunluk: 1000 mesaj
- + Grup: max 1000 kisi (MLS)
- + ZK-ID ile gizli transfer

Katman 4: PLATIN (80-89)
- + Grup mesajlasma (max 5000 kisi, MLS)
- + Goruntulu arama (grup, max 50)
- + Dosya paylasimi (max 100MB)
- + Mini app olusturma
- + Yonetim oylari (ZK vote)
- + Node teklif hakki
- + Gunluk: limitsiz mesaj
- + Grup: max 10000 kisi (MLS)
- + Staking hakki

Katman 5: ELMAS (90-100)
- + Tum ozellikler limitsiz
- + Onayli hesap rozeti
- + Ozel destek kanali
- + Airdrop onceligi
- + Governance veto hakki (1/5)
- + Revenue sharing (node gelirinden pay)
- + Grup: limitsiz (MLS)

7.3 ZK PROOF URETIM AKISI (DETAYLI)

Adim 1: Veri Toplama (Local)
- Cihaz local SQLite'dan davranis verilerini ceker
- Puan hesaplama algoritmasi calistirir
- Mevcut puan ve hedef threshold karsilastirir

Adim 2: Witness Olusturma
```json
{
  "private_inputs": {
    "current_score": 72,
    "account_age_months": 8,
    "daily_login_count": 45,
    "message_count": 120,
    "spam_reports_received": 0,
    "fraud_flags": 0,
    "secret": "0xabc123..."
  },
  "public_inputs": {
    "threshold": 70,
    "did": "did:obs:a1b2c3...",
    "timestamp": 1714100000
  }
}
```



Adim 3: Circuit Calistirma
- Circom: credit_threshold.circom
- Constraints: 10,000
- Proof tipi: Groth16 (hizli dogrulama)

Adim 4: Proof Cikti


```json
{
  "proof": "base64_encoded_proof",
  "public_signals": [
    "70",           // threshold
    "1714100000",   // timestamp
    "did_hash..."   // DID commitment
  ]
}
```



Adim 5: Node Dogrulama
- snarkJS groth16Verify calistir
- Verification key ile kontrol
- Proof gecerli ise katman yukselt
- YENI: Proof hash'i blockchain'e kaydet

Adim 6: Katman Yükseltme
- Node kullanici kaydini guncelle
- Yeni yetkileri aktif et
- Client'a bildirim gonder
- ZK proof archive (immutable log)

7.4 SYBIL KORUMASI

Problem: Coklu fake hesap

Cozum:
1. Telefon dogrulama (1 numara = 1 hesap)
2. ZK-ID: Her hesap benzersiz, taklit edilemez
3. Baslangic puani rastgele (20-100): Botlar avantajli baslayamaz
4. Proof of Humanity: Yuz tanima veya sosyal graf (opsiyonel)
5. Graph analysis: Node'lar arasi iliski agi (gizli, ZK ile)

================================================================================
BOLUM 8: TOKEN EKONOMISI (ZK-ROLLUP TABANLI)
================================================================================

8.1 OBS TOKEN OZELLIKLERI

Ozellik	Deger	
Ad	Obscura Token	
Sembol	OBS	
Toplam Arz	1,000,000,000 (1 milyar)	
Baslangic Dagilim	40% topluluk, 20% ekip, 15% yatirimci, 15% ekosistem, 10% rezerv	
Enflasyon	%2/yil (staking odulu)	
Yakim	Islem ucretlerinin %50'si yakilir	
Gizlilik	zk-Rollup ile miktar ve alici gizli	

8.2 ZK-ROLLUP MIMARISI (YENI)

Secenek 1: StarkNet
- Dil: Cairo
- Proof: STARK (kuantum dayanikli)
- Maliyet: Dusuk (batch islem)
- Ekosistem: Genis

Secenek 2: zkSync Era
- Dil: Solidity (EVM uyumlu)
- Proof: SNARK
- Maliyet: Cok dusuk
- Ekosistem: EVM projeleri

Secenek 3: Aztec
- Dil: Noir
- Proof: PLONK + TurboPLONK
- Ozellik: Native privacy (en uygun)
- Gizlilik: En yuksek

ONERI: Aztec veya zkSync Era
- Aztec: Native privacy OBS icin ideal
- zkSync: EVM uyumlulugu, kolay entegrasyon

8.3 GIZLI TRANSFER AKISI

Normal Transfer (Acik):
1. Gonderen: Alici adres + miktar belirle
2. Wallet: Transaction olustur
3. Node: zk-Rollup'e gonder
4. Rollup: Batch'le, proof uret
5. L1 (Ethereum): Proof dogrula, state guncelle

Gizli Transfer (ZK - YENI):
1. Gonderen: Alici shielded adres + miktar
2. Wallet: ZK proof uret (token_balance.circom)
   - Private: bakiye, miktar
   - Public: nullifier, commitment, root
3. Transaction: proof + public inputs
4. Node: zk-Rollup'e gonder
5. Rollup: Proof dogrula, state guncelle (miktar gizli)
6. L1: Proof dogrula, detay bilinmez

8.4 CUZDAN YAPISI

Adres Tipi	Kullanim	Gizlilik	
Transparent	Acik transfer	Acik	
Shielded	Gizli transfer	Tam gizli	
Viewing Key	Bakiye kontrolu	Sadece sahip	

8.5 STAKING VE ODULLER

Staking:
- Min: 1000 OBS
- Lock: 30 gun (min)
- APY: %5-15 (degisken)

Node Operator:
- Min stake: 10,000 OBS
- Odul: Islem ucretlerinin %30'u
- Slash: Kötü davranis (proof uretme hatasi, cevrimdisi)

Governance:
- Min: 5000 OBS + Platin katman
- Vote: ZK proof ile gizli
- Proposal: OBS yakimi gerektirir

8.6 ISLEM UCRETLERI

Islem	Ucret (OBS)	ZK Proof	
Mesaj gonderme	0 (ucretsiz)	Hayir	
Dosya paylasimi	0.01	Hayir	
Mini app deploy	10	Hayir	
Gizli transfer	0.05	Evet	
Stake	0.1	Hayir	
Governance vote	0.01	Evet (ZK vote)	
Katman yukseltme	0	Evet (ZK credit)	

================================================================================
BOLUM 9: CLIENT SPESIFIKASYONU
================================================================================

9.1 ONERILEN: FLUTTER 3.19+

Neden Flutter:
- Cross-platform: iOS, Android, Web, Desktop (macOS, Windows, Linux)
- Tek kod tabani
- Performans: Native compile
- Ekosistem: Zengin plugin'ler

Dil: Dart 3.0+

9.2 CLIENT MIMARISI

Katman 1: UI (Flutter Widgets)
- Material 3 / Cupertino design
- Dark/Light tema
- Responsive (telefon, tablet, desktop)

Katman 2: State Management (Riverpod / Bloc)
- Kullanici state
- Mesaj state
- Wallet state
- ZK proof state

Katman 3: Service Layer
- WebSocket service
- API service
- Crypto service (Rust FFI)
- ZK service (Rust FFI - YENI)
- Storage service (SQLite)

Katman 4: FFI Layer (Rust)
- obscura_crypto crate
- obscura_zk crate (YENI)
- Platform channel (Dart <-> Rust)

9.3 RUST FFI ENTEGRASYONU (YENI)

Kutuphane: flutter_rust_bridge

Cargo.toml:


```toml
[package]
name = "obscura_client_ffi"
version = "0.1.0"
edition = "2021"

[dependencies]
obscura-crypto = { path = "../crypto" }
obscura-zk = { path = "../zk" }
flutter_rust_bridge = "2.0"

[lib]
crate-type = ["cdylib", "staticlib"]
```



Dart Binding:


```dart
import 'ffi_bridge.dart';

// ZK Proof uretimi
final proof = await ZkModule.generateCreditProof(
  score: 72,
  threshold: 70,
  secret: secretKey,
);

// Node'a gonder
await apiService.submitZkProof(proof);
```


9.4 LOCAL VERITABANI

SQLite (sqflite):
- Mesajlar (sifreli)
- Kullanici profili
- ZK-ID secret (encrypted with device key)
- Kredi puani verileri
- Wallet bakiyesi (shielded address)

Sifreleme:
- SQLCipher: Veritabani dosyasi sifreli
- Anahtar: Device key + Biometric (opsiyonel)

9.5 BILDIRIM SISTEMI

iOS: APNs
- Certificate-based auth
- Background fetch
- Silent push (mesaj senkronizasyonu)

Android: FCM
- High priority push
- Data messages (silent)
- Notification channels

Web: Web Push API
- VAPID keys
- Service Worker
- Push API

Gizlilik:
- Push icerigi: "Yeni mesaj var" (sadece)
- Gercek icerik: Client tarafinda cozulur
- Badge count: Client hesaplar

9.6 WEBRTC SESLI/GORUNTULU ARAMA

Kutuphane: flutter_webrtc

Akis:
1. Arama daveti (Signal/MLS mesaji)
2. ICE candidate toplama (STUN/TURN)
3. SDP offer/answer
4. PeerConnection kurulumu
5. Media stream baslatma
6. SRTP sifreleme (Signal Protocol)

TURN Sunucusu:
- Coturn (self-hosted)
- veya Twilio TURN
- Kimlik: ZK proof ile (opsiyonel)

9.7 CLIENT GEREKSINIMLERI

Platform	Min Versiyon	RAM	Depolama	
iOS	14.0+	2GB	100MB	
Android	8.0+	2GB	100MB	
Web	Chrome 90+	4GB	IndexedDB	
macOS	11.0+	4GB	200MB	
Windows	10+	4GB	200MB	
Linux	Ubuntu 20.04+	4GB	200MB	

================================================================================
PARÇA 2 SONU
Devam: PARÇA 3 (Mini App, Fiziksel Entegrasyon, Fazlar)
================================================================================

```

**Parça 2 tamamlandı.** "Devam" yazarsanız Parça 3'ü (Mini App, Fiziksel Entegrasyon, Fazlar) göndereyim.


```
