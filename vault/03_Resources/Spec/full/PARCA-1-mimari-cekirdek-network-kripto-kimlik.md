================================================================================
OBSCURA PLATFORM MASTER SPECIFICATION v3.0 - PARÇA 1/5
Bölüm 1-5: Mimari, Çekirdek, Network, Kripto, Kimlik
================================================================================

TARIH: 2026-04-26
VERSIYON: 3.0-FINAL
PLATFORM: Obscura
TOKEN: OBS

================================================================================
BOLUM 1: PLATFORM FELSEFESI VE PROTOCOL-FIRST MIMARI
================================================================================

1.1 TEMEL ILKE: PROTOCOL-FIRST + ZERO-KNOWLEDGE

Obscura "protocol-first" mimari ile tasarlanmistir. Bu ne demek?

- Backend protokolü sabittir ve dil/platform bagimsizdir
- Client sadece protokolü konusan bir arayüzdür
- Istedigin dilde/ortamda client üretebilirsin
- Çekirdek degismeden client çesitliligi artar

YENI: ZERO-KNOWLEDGE FIRST Prensibi
- Kullanici verisi mümkün oldugunca cihazda kalir
- Sunucu sadece ZK proof'lari dogrular, icerigi görmez
- Kredi puani, kimlik, token bakiyesi: hepsi ZK proof ile kanitlanir
- "Dont trust, verify" - ama verify edilen sey detay degil, özelliktir

1.2 PROTOKOL KATMANLARI

Katman 1: Transport (WebSocket/QUIC/TCP)
Katman 2: Message Framing (Binary/Protobuf)
Katman 3: Encryption (Signal Protocol + MLS for Groups)
Katman 4: Zero-Knowledge Layer (ZK-ID, ZK-Proofs)
Katman 5: Application Logic (Obscura-specific)

1.3 PLATFORM OZETI

| Ozellik               | Deger                                      |
|-----------------------|--------------------------------------------|
| Ad                    | Obscura                                    |
| Token                 | OBS                                        |
| Tip                   | Federated secure messaging + ZK economy    |
| Baslangic             | Whitelist node (5 node)                    |
| Hedef                 | Tam federasyon (acik node)                 |
| Gecis                 | ZK-ID Kredi puani (0-100)                  |
| Cekirdek Dili         | Go (node), Rust (crypto + ZK)              |
| Client                | Herhangi (Flutter onerilen)               |
| Zincir                | zk-Rollup (StarkNet/zkSync) + Substrate    |
| ZK Motoru             | Circom + snarkJS / Noir (Aztec)           |

1.4 ZERO-KNOWLEDGE KATMANLARI

| Katman | ZK Uygulamasi | Amac |
|--------|--------------|------|
| Kimlik | ZK-ID | Kullanici kimligi gizli kanit |
| Kredi  | ZK-Proof | Puan araligi kaniti (≥X) |
| Token  | zk-Rollup | Gizli transfer (miktar/alici) |
| Icerik | ZK-ML | Spam tespiti mesaj acilmadan |
| Yonetim| ZK-Vote | Oy verme tercih gizliligi |

================================================================================
BOLUM 2: CEKIRDEK SISTEM (CORE) - DETAYLI SPESIFIKASYON
================================================================================

2.1 NODE MIMARISI

Her node 5 temel modulden olusur:

MODUL A: NETWORK LAYER (Go)
-----------------------------
Gorev: P2P baglanti, mesaj yonlendirme, NAT traversal

Ana Bilesenler:
- libp2p host yapilandirmasi
- DHT (Distributed Hash Table) entegrasyonu
- QUIC ve TCP transport
- GossipSub (pub/sub mesaj yayilimi)
- YENI: MLS (Messaging Layer Security) grup yonetimi

Kritik Fonksiyonlar:
func StartNode(config NodeConfig) (*Node, error)
func ConnectToPeer(peerID PeerID, addrs []multiaddr.Multiaddr) error
func PublishMessage(topic string, data []byte) error
func SubscribeTopic(topic string) (<-chan Message, error)
func CreateMLSGroup(groupID string, members []DID) (*MLSGroup, error)
func AddMemberToMLS(groupID string, member DID, welcomeMessage []byte) error

Konfigurasyon:
type NodeConfig struct {
    PrivateKey       crypto.PrivKey
    ListenAddrs      []multiaddr.Multiaddr
    BootstrapPeers   []peer.AddrInfo
    DHTMode          dht.Mode
    PubSubEnabled    bool
    MLSEnabled       bool        // YENI
    ZKProverEnabled  bool        // YENI
}

MODUL B: STORAGE LAYER (Go + Rust)
-----------------------------------
Gorev: Mesaj parcalama, sifreleme, dagitik depolama

Is Akisi:
1. Gelen sifreli mesaj al
2. 256KB shard'lara bol (eger buyukse)
3. Reed-Solomon 4-of-6 erasure coding uygula
4. Her parcayi farkli node'a gonder
5. Metadata'yi DHT'ye kaydet

YENI: ZK Storage Proof
- Her shard icin ZK proof uret (veri tutuldugunu kanitla)
- Node proof'u blockchain'e submit eder
- Diger node'lar proof'u dogrular, veriyi indirmek zorunda kalmaz

Veri Yapisi:
type Shard struct {
    ID        string    // SHA-256 hash
    Data      []byte    // AES-256-GCM encrypted
    Index     int       // Shard sirasi
    Total     int       // Toplam shard sayisi
    Parity    bool      // Parity shard mi?
    Timestamp int64
    TTL       int64     // 30 gun
    ZKProof   []byte    // YENI: Storage proof
}

Depolama Kontrati:
- Hicbir node tam mesaji tutmaz
- Minimum 3 farkli node'da replika
- 30 gun veya alici okuyana kadar sakla
- TTL dolunca otomatik sil
- YENI: ZK proof ile depolama kaniti (proof of storage)

MODUL C: CRYPTO LAYER (Rust)
-----------------------------
Gorev: Butun sifreleme islemleri + ZK proof uretimi

Implementasyon:
- Crate: obscura-crypto
- Dil: Rust 1.75+
- FFI: Go icin cgo baglantisi

Ana Fonksiyonlar:
pub fn generate_identity_keypair() -> (PrivateKey, PublicKey)
pub fn create_prekey_bundle(identity_key: PrivateKey) -> PreKeyBundle
pub fn encrypt_message(
    session: &mut SessionRecord,
    plaintext: &[u8]
) -> Result<CiphertextMessage, SignalError>
pub fn decrypt_message(
    session: &mut SessionRecord,
    ciphertext: &[u8]
) -> Result<Vec<u8>, SignalError>

YENI: MLS Fonksiyonlari:
pub fn mls_create_group(cipher_suite: CipherSuite) -> Result<MLSGroup, MLSError>
pub fn mls_add_member(group: &mut MLSGroup, key_package: KeyPackage) -> Result<Welcome, MLSError>
pub fn mls_encrypt_message(group: &MLSGroup, plaintext: &[u8]) -> Result<MLSCiphertext, MLSError>
pub fn mls_decrypt_message(group: &mut MLSGroup, ciphertext: &[u8]) -> Result<Vec<u8>, MLSError>

YENI: ZK Proof Fonksiyonlari:
pub fn zk_generate_identity_proof(
    did: &DID,
    secret: &Secret
) -> Result<ZKProof, ZKError>
pub fn zk_verify_identity_proof(
    proof: &ZKProof,
    public_params: &PublicParams
) -> Result<bool, ZKError>
pub fn zk_generate_credit_proof(
    score: u8,
    threshold: u8,
    secret: &Secret
) -> Result<ZKProof, ZKError>
pub fn zk_verify_credit_proof(
    proof: &ZKProof,
    threshold: u8,
    public_params: &PublicParams
) -> Result<bool, ZKError>

Signal Protocol Entegrasyonu:
- X3DH anahtar anlasmasi
- Double Ratchet (her mesajda yeni anahtar)
- Sesli/goruntulu aramalar icin SRTP

YENI: MLS Protocol Entegrasyonu:
- TreeKEM anahtar dagilimi
- Grup yonetimi (ekleme/cikarma)
- Forward secrecy ve post-compromise security
- 10,000+ uyeli gruplar destegi

MODUL D: CONSENSUS LAYER (Go)
------------------------------
Gorev: Node yonetimi, multisig, guncelleme onayi

Multisig Yapisi:
- 5 node varsa 3/5 onay gerekli
- Her kritik islem (protokol guncelleme, yeni node ekleme) icin
- Onaylar zincir uzerinde immutable kaydedilir

YENI: ZK Governance Vote
- Oylama tercihleri ZK proof ile gizlenir
- Sonuc dogru sayilir ama kim neye oy verdi bilinmez
- Anti-bribery (rüşvet korumasi)

Governance Akisi:
1. Teklif olustur (metadata + islem)
2. Node'lara yayinla
3. 3/5 imza topla
4. YENI: ZK vote proof'lari topla
5. Zaman kilidi bekle (48 saat)
6. Otomatik uygula

MODUL E: ZK LAYER (Rust - YENI)
--------------------------------
Gorev: Tum zero-knowledge islemleri

Implementasyon:
- Crate: obscura-zk
- Dil: Rust (noir-lang veya circom entegrasyonu)
- Circuit'ler: Circom / Noir

Ana Circuit'ler:
1. identity_proof.circom: DID sahipligi kaniti
2. credit_threshold.circom: Puan esik kaniti
3. token_balance.circom: Bakiye kaniti (miktar gizli)
4. storage_proof.circom: Veri depolama kaniti
5. vote_proof.circom: Oy gizliligi kaniti

2.2 NODE TIPLERI (PHASE 1-2)

| Tip        | Sayi | Gorevler                                      |
|------------|------|-----------------------------------------------|
| Bootstrap  | 1    | Yeni node'lara peer listesi, ilk senkronizasyon|
| Relay      | 2    | Mesaj yonlendirme, P2P kopru                 |
| Storage    | 2    | Shard depolama, replikasyon yonetimi         |
| ZK Prover  | 1    | YENI: ZK proof uretimi ve dogrulama          |

Her node tum modulleri icerir ama yapilandirmaya gore agirlikli rol oynar.

================================================================================
BOLUM 3: NETWORK VE NODE YAPISI
================================================================================

3.1 NETWORK TOPOLOJISI

Phase 1-2 (Whitelist):
[Client A] <---> [Node 1 - Bootstrap]
                      |
[Client B] <---> [Node 2 - Relay] <---> [Node 3 - Storage]
                      |
[Client C] <---> [Node 4 - Relay] <---> [Node 5 - Storage]
                      |
[Client D] <---> [Node 6 - ZK Prover] <---> [Blockchain]

Phase 3 (Federation):
Her node esit, mesh topoloji. Her client en yakin node'a baglanir.
ZK Prover node'lari proof uretiminde uzmanlasir.

3.2 MESAJ YONLENDIRME

Senaryo 1: Ayni Node Uzerinde
Client A -> Node 1 -> (internal) -> Client B
Gecikme: <10ms

Senaryo 2: Farkli Node'lar
Client A -> Node 1 -> P2P -> Node 2 -> Client B
Gecikme: <100ms (yerel), <300ms (kuresel)

Senaryo 3: Offline Alici
Client A -> Node 1 -> Storage (shard) -> Bildirim -> Client B online olunca cek

Senaryo 4: YENI - Grup Mesaji (MLS)
Client A -> Node 1 -> MLS Group Encrypt -> Broadcast -> Tum uyelere dagitim
Gecikme: <200ms (yerel), <500ms (kuresel, 1000+ uyeli grup)

3.3 DISCOVERY MEKANIZMASI

Bootstrap Node Adresleri:
- Hardcoded DNS (bootstrap.obscura.network)
- IPFS gibi diger P2P aglardan fallback
- Son baglanilan node'larin cache'i
- YENI: ENS (Ethereum Name Service) fallback

Peer Discovery:
1. Bootstrap'tan 5 node al
2. Her node'dan kendi peer listesini iste
3. DHT'ye kendi adresini kaydet
4. Periyodik olarak peer listesini yenile
5. YENI: ZK proof ile node yetki dogrulamasi

================================================================================
BOLUM 4: KRIPTOGRAFI VE GUVENLIK
================================================================================

4.1 SIFRELEME YIGINI

Katman 1: Transport (TLS 1.3 veya QUIC crypto)
Katman 2: P2P (Noise Protocol - libp2p default)
Katman 3: Application E2EE (Signal Protocol - birebir)
Katman 4: Application E2EE (MLS Protocol - gruplar)
Katman 5: ZK Layer (Circom/Noir circuits)

4.2 ANAHTAR YONETIMI

Identity Key (Ed25519):
- Olusturulma: Ilk kayitta
- Saklama: Cihaz guvenli alani (Keychain/Keystore)
- Yedekleme: 12 kelime mnemonic (BIP39) veya social recovery
- YENI: ZK-ID ile cihaz bagimsiz kimlik kaniti

PreKey Yonetimi:
- Signed PreKey: Haftalik rotasyon
- One-time PreKeys: Her mesajda tuketilir, 100 adet onceden uretilir
- PreKey doldurma: <20 kaldiginda otomatik yenileme

YENI: MLS KeyPackage:
- KeyPackage: Her grup uyesi icin onceden uretilir
- LeafNode: Grup agacindaki pozisyon
- Update: Periyodik olarak yenilenir (forward secrecy)

4.3 MESAJ SIFRELEME AKISI

Birebir (Signal):
Gonderen:
1. Alicinin public key'ini DHT'den veya cache'den al
2. X3DH handshake (eger yeni oturum)
3. Double Ratchet ile sifrele
4. Ciphertext + metadata (timestamp, message ID) olustur
5. Node'a gonder

Alici:
1. Node'dan ciphertext al
2. Kendi private key'i ile coz
3. Double Ratchet state guncelle
4. plaintext'i goster

Grup (MLS - YENI):
Gonderen:
1. Grup state'ini al (agac yapisi)
2. TreeKEM ile grup anahtarini guncelle
3. MLS encrypt ile mesaji sifrele
4. Welcome/Commit mesaji (uyelik degisikligi varsa)
5. Broadcast ile tum uyelere gonder

Alici:
1. Grup mesajini al
2. Kendi LeafNode private key'i ile coz
3. Grup state guncelle (ratchet)
4. plaintext'i goster

4.4 ZK PROOF AKISI (YENI)

Kredi Puani Kaniti:
1. Kullanici cihazinda: puan >= threshold kontrolu
2. Circom circuit calistir (credit_threshold.circom)
3. Proof + public inputs uret
4. Node'a gonder (proof, threshold, public_params)
5. Node snarkJS/Noir ile dogrula
6. Katman yukseltme veya yetki ver

Token Bakiye Kaniti:
1. Kullanici: bakiye >= amount kontrolu
2. Circuit: token_balance.circom
3. Proof uret (miktar gizli, sadece "yeterli" kaniti)
4. Node dogrula
5. Islem onayla (zk-Rollup'e gonder)

4.5 GUVENLIK KURALLARI (KESIN)

KURAL 1: Private key asla sunucudan cikmaz
KURAL 2: Sifreleme Rust'ta yapilir, Go'da asla
KURAL 3: Hicbir node tam mesaji cozemez
KURAL 4: Metadata bile minimum tutulur (timestamp, from, to)
KURAL 5: 30 gun sonra mesajlar silinir (otomatik)
KURAL 6: ZK proof'lar public input haric hicbir detay aciklamaz
KURAL 7: ZK circuit'ler formel olarak dogrulanmistir (audit edilmis)

================================================================================
BOLUM 5: KIMLIK VE DOGRULAMA SISTEMI
================================================================================

5.1 UC KATMANLI KIMLIK

| Katman  | Format              | Kullanim                    | ZK Durumu |
|---------|---------------------|-----------------------------|-----------|
| Login   | +90XXXXXXXXXX       | Sadece giris, SMS dogrulama| Acik      |
| Display | @kullaniciadi       | Insan tarafindan okunur    | Acik      |
| Protocol| did:obs:hash        | Makine tarafindan, kalici  | ZK-ID ile |
| ZK-ID   | zk:obs:proof        | Gizli kanit, detay yok     | Gizli     |

5.2 KAYIT AKISI (DETAYLI)

Adim 1: Telefon Girisi
- UI: Telefon numarasi input (+ ulke kodu otomatik)
- Validasyon: E.164 format kontrolu
- Ornek: +905551234567

Adim 2: SMS Dogrulama
Servis: Twilio, MessageBird, veya Vonage
API: Asagida Bolum 17'te detayli

Adim 3: TOTP Uretimi
- 6 haneli, 5 dakika gecerli
- Sunucu tarafinda Redis/cache'te saklanir (key: phone_number, ttl: 300s)
- Maksimum 3 deneme, sonra 15 dakika kilitleme

Adim 4: Kimlik Olusturma
- Ed25519 keypair uret (client tarafinda)
- DID olustur: did:obs:sha256(public_key)
- PreKey bundle olustur (100 one-time key)
- YENI: ZK-ID secret olustur (random seed)
- YENI: ZK-ID public params olustur (circuit input)

Adim 5: ZK-ID Kaydi (YENI)
- ZK proof: "Bu DID'ye sahibim" (identity_proof.circom)
- Proof client'ta uretilir
- Node proof'u dogrular, secret'i gormez
- DID + ZK-ID public params node'a kaydedilir

Adim 6: Sunucu Kaydi
POST /v1/register
{
  "phone": "+905551234567",
  "otp": "123456",
  "identity_key": "base64_encoded_pubkey",
  "signed_prekey": "base64_encoded",
  "one_time_prekeys": ["base64_encoded", ...],
  "did": "did:obs:a1b2c3...",
  "zk_id_proof": "base64_encoded_proof",        // YENI
  "zk_id_public": "base64_encoded_public_params" // YENI
}

Adim 7: Yedekleme Uyari
- 12 kelime mnemonic goster
- Kullanici kaydetmeyi onaylayana kadar devam etme
- Social recovery kurulumu (opsiyonel)
- YENI: ZK-ID secret'i mnemonic ile birlikte yedekle

5.3 GIRIS AKISI (Mevcut Kullanici)

Adim 1: Telefon + SMS dogrulama
Adim 2: Cihaz dogrulama (cross-signing)
- Yeni cihaz varsa, mevcut cihazdan onay iste
- veya mnemonic ile kurtar

Adim 3: ZK-ID Dogrulama (YENI)
- Cihaz ZK-ID secret'i mnemonic'den turetir
- "Bu DID'ye sahibim" proof'u uretir
- Node proof'u dogrular
- Cihaz yetkilendirilir

Adim 4: PreKey senkronizasyonu
- Sunucudan kalan one-time key sayisini kontrol et
- <20 ise yeni uret ve gonder
- YENI: MLS KeyPackage senkronizasyonu

5.4 CROSS-SIGNING (COKLU CIHAZ)

Cihaz Turu    | Anahtar Uretimi         | Yetki | ZK-ID Durumu
--------------|-------------------------|-------|-------------
Birincil      | Yeni Ed25519            | Tam yetki, diger cihazlari onaylar | ZK-ID master
Ikincil       | Yeni Ed25519            | Birincil onayi gerekli | ZK-ID derived
Kurtarma      | Turetilmis (mnemonic)   | 7 gun timelock + email | ZK-ID recovery

Cihaz Ekleme Akisi:
1. Yeni cihaz: "Cihaz ekle" sec, QR kod goster
2. Eski cihaz: QR'yi oku, cross-sign imzala
3. YENI: ZK-ID derived secret uret (master'dan)
4. Sunucu: Imzayi dogrula, yeni cihazi yetkilendir
5. YENI: Yeni cihaz ZK proof uretme yetkisi alir

5.5 KREDI PUANI SISTEMI (YENI - ZK TABANLI)

Baslangic Puani: 20-100 (rastgele, Sybil korumasi)
Hesaplama: Cihazda local, sunucu detayi gormez

Puan Bilesenleri:
| Davranis | Etki | ZK Kaniti |
|----------|------|-----------|
| Hesap yasi | +1/ay | proof(age >= X) |
| Mesaj aktivitesi | +0.1/gun | proof(activity >= X) |
| Spam raporu | -5/rapor | proof(no_spam) |
| Dolandiricilik | -20/olay | proof(trustworthy) |
| Topluluk katkisi | +2/katkı | proof(contribution) |
| Node calistirma | +10/ay | proof(node_active) |

Katman Gecisleri (ZK Proof ile):
| Katman | Puan | ZK Proof | Ozellikler |
|--------|------|----------|------------|
| 1 (Bronz) | 0-59 | proof(score < 60) | Temel mesajlasma |
| 2 (Gumus) | 60-69 | proof(score >= 60) | Grup sohbetleri, sesli arama |
| 3 (Altin) | 70-79 | proof(score >= 70) AND proof(unique_human) | Mini app'ler, dosya paylasimi |
| 4 (Platin) | 80-89 | proof(score >= 80) AND proof(stake > X) | Yonetim oylari, node teklifi |
| 5 (Elmas) | 90-100 | proof(score >= 90) AND proof(long_term) | Tüm özellikler + ödül çarpani |

ZK Proof Uretim Akisi:
1. Kullanici cihazinda puan hesapla (local data)
2. Threshold kontrolu yap (örn: >= 70)
3. Circom circuit calistir (credit_threshold.circom)
4. Proof + public inputs (threshold, timestamp) uret
5. Node'a gonder
6. Node dogrula, katman yükselt
7. YENI: Proof blockchain'e kaydet (immutable)

================================================================================
PARÇA 1 SONU
Devam: PARÇA 2 (Mesajlasma, Kredi, Token, Client)
================================================================================


```
