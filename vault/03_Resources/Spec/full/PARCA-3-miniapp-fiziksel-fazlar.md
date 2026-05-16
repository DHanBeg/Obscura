================================================================================
OBSCURA PLATFORM MASTER SPECIFICATION v3.0 - PARÇA 3/5
Bölüm 10-12: Mini App, Fiziksel Entegrasyon, Fazlar ve Yol Haritası
================================================================================

================================================================================
BOLUM 10: MINI APP MOTORU
================================================================================

10.1 MIMARI

Runtime: Deno (TypeScript/JavaScript)
Sandbox: Separate process, seccomp-bpf
Limitler:
- Bellek: 128 MB per app
- CPU: %10 tek cekirdek
- Ag: Whitelist-only
- Depolama: 10 MB IndexedDB
- YENI: ZK proof limiti: 5 proof/saniye (spam korumasi)

10.2 API BRIDGE

Mini app'lere sunulan API:

identity:
  - getUserId(): string (DID)
  - getUsername(): string
  - getAvatar(): string (URL)
  - YENI: getZkIdentity(): ZKProof (gizli kimlik kaniti)

messaging:
  - sendMessage(to: string, content: string): Promise<void>
  - openChat(userId: string): void
  - onMessage(callback: (msg) => void): () => void (unsubscribe)
  - YENI: sendGroupMessage(groupId: string, content: string): Promise<void>
  - YENI: createMLSGroup(members: string[]): Promise<string>

wallet:
  - getBalance(): Promise<number>
  - getShieldedBalance(): Promise<number> // YENI
  - requestPayment(to: string, amount: number): Promise<string> (txId)
  - YENI: requestShieldedPayment(to: string, amount: number): Promise<string>
  - onPayment(callback: (tx) => void): () => void

zk:
  - YENI: generateProof(circuit: string, inputs: object): Promise<ZKProof>
  - YENI: verifyProof(proof: ZKProof): Promise<boolean>
  - YENI: getCreditScore(): Promise<number> (gizli, sadece kendi puanin)

ui:
  - showToast(message: string): void
  - openModal(url: string): void
  - close(): void
  - YENI: requestZkPermission(permission: string): Promise<boolean>

10.3 IZIN SISTEMI

Her mini app manifest'te istedigi izinleri bildirir:
```json
{
  "name": "Harita Servisi",
  "permissions": ["location", "messaging"],
  "allowedDomains": ["maps.example.com"],
  "zkPermissions": ["credit_score_read", "identity_proof"], // YENI
  "maxMemory": "128MB",
  "maxCpu": "10%"
}
```



Kullanici ilk calistirmada onay verir.
YENI: ZK izinleri ayri onay gerektirir (gizlilik kritik).

10.4 MINI APP KATMAN KISITLARI

Katman	Mini App Calistirma	ZK API	Max Kullanici	
Bronz (0-59)	Hayir	Hayir	0	
Gumus (60-69)	Evet (kullan)	Hayir	100/gun	
Altin (70-79)	Evet (kullan)	Evet (okuma)	500/gun	
Platin (80-89)	Evet (olustur)	Evet (tam)	Limitsiz	
Elmas (90-100)	Evet (olustur)	Evet (tam)	Limitsiz	

10.5 ZK-ENABLED MINI APP ORNEGI

Uygulama: Gizli Oylama


```typescript
// Mini app kodu
import { zk, identity } from "obscura:api";

async function createPoll(question: string, options: string[]) {
  // Kullanici kimligini ZK proof ile kanitla
  const idProof = await identity.getZkIdentity();
  
  // Platin katman kontrolu
  const creditProof = await zk.generateProof("credit_threshold", {
    threshold: 80,
    secret: await identity.getSecret()
  });
  
  // Poll olustur (gizli)
  return {
    pollId: crypto.randomUUID(),
    question,
    options,
    creatorProof: idProof,
    eligibilityProof: creditProof
  };
}

async function vote(pollId: string, option: number) {
  // Oy gizliligi ZK proof ile
  const voteProof = await zk.generateProof("vote_proof", {
    pollId,
    option,
    secret: await identity.getSecret()
  });
  
  // Sadece proof gonder, tercih gizli
  await messaging.sendToNode({
    type: "ZK_VOTE",
    pollId,
    proof: voteProof
  });
}
```


================================================================================
BOLUM 11: FIZIKSEL ENTEGRASYON
================================================================================

11.1 ETKINLIK YONETIMI

Olusturma:
- Baslik, aciklama, konum, tarih/saat
- Kapasite (opsiyonel)
- Ucret (OBS veya ucretsiz)
- YENI: Katilim sarti (katman, ZK proof)

Katilim:
- "Katil" butonu
- QR kod ile check-in
- Katilimci listesi (sadece organizator)
- YENI: ZK check-in (kimlik aciklanmadan katilim kaniti)

11.2 KONUM BAZLI KESIF (OPSiyONEL)

Kullanici acik kapatir (varsayilan: kapali)
Aciksa: 1km icindeki diger Obscura kullanicilari
Gizlilik: Hassas konum degil, grid-based (1km x 1km)
YENI: ZK konum proof (tam konum aciklanmadan "yakinda oldugunu" kanitla)

ZK Konum Proof:
1. Kullanici: GPS koordinatlari (gizli)
2. Grid hesaplama: lat/lon -> 1km grid ID
3. ZK proof: "Bu grid ID'deyim" (konum detayi gizli)
4. Diger kullanicilar: Ayni grid ID'de mi kontrol et
5. Hicbir zaman tam konum paylasilmaz

11.3 QR KOPRU

Kullanim:
- Profil paylasma (QR okut, ekle)
- Grup daveti
- Etkinlik check-in
- Cihaz ekleme (cross-signing)
- YENI: ZK-ID paylasma (kimlik aciklanmadan)

Format: `obscura://{action}/{payload}`

YENI: ZK QR Format:
`obscura://zk/{zk_proof_commitment}/{public_params}`
- QR'da sadece commitment ve public params
- Gercek kimlik veya detay yok
- Kars taraf proof'u node'dan dogrular

11.4 NFC ENTEGRASYONU (YENI)

Kullanim:
- Etkinlik check-in (telefonu yaklastir)
- Cihaz pairing (tap-to-pair)
- Odeme (OBS ile NFC odeme)

Guvenlik:
- NFC mesajlari Signal Protocol ile sifreli
- ZK proof ile kimlik kaniti
- Replay attack korumasi (timestamp + nonce)

================================================================================
BOLUM 12: FAZLAR VE YOL HARITASI
================================================================================

12.1 FAZ 1: MVP (Gun 0-90)

Basari Kriterleri:
- 10,000 aktif kullanici
- 7 gun kesintisiz calisma
- 4/5 node online %99.9
- P99 gecikme < 2 saniye
- YENI: 100 ZK proof/saniye throughput

Deliverables:
[x] 5 node kurulumu (Turkiye)
[x] E2EE mesajlasma (Signal)
[x] MLS grup mesajlasma (temel)
[x] Flutter client (5 platform)
[x] Telefon dogrulama
[x] Kredi puani sistemi (temel)
[x] ZK-ID kimlik sistemi (temel)
[x] P2P sesli arama
[x] Otomatik node secimi
[x] ZK proof altyapisi (Circom devreye)

Teknik Detaylar:
- Circom circuit'leri: identity_proof, credit_threshold
- snarkJS ile proof uretim/dogrulama
- Groth16 (hizli, ama trusted setup gerekli)
- Testnet: Goerli veya Sepolia

12.2 FAZ 2: CEKIRDEK (Gun 90-180)

Basari Kriterleri:
- Protokol migrasyonu (< 4 saat kesinti)
- 100+ validator
- 50+ mini app
- 10K gunluk islem
- YENI: 1000 ZK proof/saniye
- YENI: 1000+ uyeli MLS gruplar

Yeni Deliverables:
[x] zk-Rollup entegrasyonu (zkSync veya Aztec)
[x] OBS wallet (shielded + transparent)
[x] Mini app motoru (Deno + ZK API)
[x] ZK-ML icerik filtreleme (temel)
[x] Airdrop dagilimi (ZK ile)
[x] Yonetisim portalı (ZK vote)
[x] MLS grup: 5000+ kisi destegi
[x] Node staking ve slash mekanizmasi

Teknik Detaylar:
- zkSync Era: Solidity ile smart contract
- veya Aztec: Noir ile native privacy
- ZK-ML: ONNX Runtime + ZK proof (basit modeller)
- Trusted setup ceremony (Groth16 icin)

12.3 FAZ 3: FEDERASYON (Gun 180-365)

Basari Kriterleri:
- 50+ harici node
- 3 kita, <100ms gecikme
- Kendi kendini surduren ekonomi
- YENI: 10,000 ZK proof/saniye
- YENI: 10,000+ uyeli MLS gruplar

Yeni Deliverables:
[x] Acik node kaydi (permissionless)
[x] Inter-node optimizasyon
[x] Byzantine fault tolerance
[x] Tam topluluk yonetimi
[x] YENI: ZK-ML gelismis icerik moderasyonu
[x] YENI: Post-quantum kripto hazirligi (CRYSTALS-Kyber)
[x] YENI: Cross-chain bridge (Ethereum, Polkadot)

Teknik Detaylar:
- PLONK veya STARK (trusted setup gerektirmez)
- Recursive proofs (proof'un proof'u)
- Hardware acceleration (GPU/FPGA)
- Formal verification (kritik circuit'ler)

12.4 FAZ 4: OTONOMI (Gun 365+)

Basari Kriterleri:
- Merkezi ekip < %10 karar alma
- Tam DAO yonetimi
- Kuantum dayanikli kripto
- YENI: 100,000+ ZK proof/saniye

Yeni Deliverables:
[x] Tam DAO yonetimi (ZK vote ile)
[x] Kuantum dayanikli kripto (CRYSTALS-Kyber/Dilithium)
[x] AI-powered node optimizasyonu
[x] Cross-platform ZK client (WebAssembly)
[x] YENI: ZK rollup sequencer decentralization
[x] YENI: Physical proof-of-location (GPS + ZK)

12.5 YOL HARITASI OZET

Faz	Sure	Odak	ZK Seviyesi	
1	0-90 gun	MVP, temel ZK	Identity + Credit	
2	90-180 gun	Ekonomi, gizlilik	Token + Vote + ML	
3	180-365 gun	Federasyon, olcek	Recursive + Cross-chain	
4	365+ gun	Otonomi, gelecek	Quantum + AI	

================================================================================
PARÇA 3 SONU
Devam: PARÇA 4 (Eksikler Dosyasi - Dis Servisler ve API'ler)
================================================================================

```

**Parça 3 tamamlandı.** "Devam" yazarsanız Parça 4'ü (Eksikler Dosyası - Dış Servisler ve API'ler) göndereyim.

```
