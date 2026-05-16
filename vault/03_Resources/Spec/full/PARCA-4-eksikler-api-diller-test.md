================================================================================
OBSCURA PLATFORM MASTER SPECIFICATION v3.0 - PARÇA 4/5
Bölüm 13-16: Eksikler Dosyası, API'ler, Diller, Test, Sonuç
================================================================================

================================================================================
BOLUM 13: EKSIKLER DOSYASI - DIS SERVISLER VE API'LER
================================================================================

BU BOLUMDE PLATFORMUN CALISMASI ICIN GEREKLI DIS SERVISLER,
API'LER VE BAGLANTI DETAYLARI LISTELENMISTIR.
BU SERVISLER HAZIR DEGILDIR, AYRICA KURULMALIDIR.

--------------------------------------------------------------------------------
13.1 SMS DOGRULAMA SERVISI
--------------------------------------------------------------------------------

AMAC: Kayit sirasinda telefon dogrulama kodu gonderme

ONERILEN SAGLAYICILAR:
1. Twilio (https://www.twilio.com)
   - Ucret: $0.0075 / SMS (Turkiye)
   - API: REST
   - Ozellik: Global kapsama, guvenilir

2. MessageBird (https://www.messagebird.com)
   - Ucret: €0.05 / SMS (Turkiye)
   - API: REST
   - Ozellik: Avrupa odakli, iyi dokumantasyon

3. Vonage (https://www.vonage.com)
   - Ucret: $0.05 / SMS
   - API: REST
   - Ozellik: Sesli arama fallback (TTS ile kod)

4. YENI: Yerel SMS Gateway (Turkiye)
   - Netgsm (https://www.netgsm.com.tr)
   - Vatan SMS (https://www.vatansms.com)
   - Ucret: ~₺0.02 / SMS
   - Ozellik: Daha ucuz, sadece Turkiye

KURULUM ADIMLARI:

1. Hesap Olusturma:
   - Web sitesine git
   - Isletme hesabi olustur
   - KYC tamamla (kimlik, adres)
   - Odeme yontemi ekle

2. API Anahtari Alma:
   - Dashboard -> API Keys
   - Yeni anahtar olustur
   - IP whitelist (sunucu IP'lerini ekle)

3. Twilio Entegrasyonu (Ornek):
   
   KUTUPHANE KURULUMU:
   ```

Go
go get github.com/twilio/twilio-go

Python
pip install twilio

Node.js
npm install twilio

   ```

   KOD ORNEGI (Go):
   ```go
   package main
   
   import (
       "github.com/twilio/twilio-go"
       openapi "github.com/twilio/twilio-go/rest/api/v2010"
   )
   
   func sendSMS(toPhone, code string) error {
       client := twilio.NewRestClient(
           os.Getenv("TWILIO_ACCOUNT_SID"),
           os.Getenv("TWILIO_AUTH_TOKEN"),
       )
       
       params := &openapi.CreateMessageParams{}
       params.SetTo(toPhone)
       params.SetFrom(os.Getenv("TWILIO_PHONE_NUMBER"))
       params.SetBody(fmt.Sprintf("Obscura kodunuz: %s", code))
       
       _, err := client.Api.CreateMessage(params)
       return err
   }
   ```

4. Webhook Ayarlari (Opsiyonel - durum raporlari icin):
   - Twilio Console -> Phone Numbers -> Manage -> Active Numbers
   - Webhook URL: https://api.obscura.network/v1/sms/status
   - Method: POST

5. Test:
   - Test telefon numarasi ekle (Twilio verified numbers)
   - 5-10 test SMS gonder
   - Teslimat raporlarini kontrol et

6. Uretim Gecisi:
   - Gercek telefon numarasi satin al (Twilio'dan)
   - A2P 10DLC kaydi (ABD icin, Turkiye gerekmez)
   - Gunluk limit ayarla (spam korumasi)

ENVIRONMENT DEGISKENLERI:

```


TWILIO_ACCOUNT_SID=ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
TWILIO_AUTH_TOKEN=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
TWILIO_PHONE_NUMBER=+1234567890

```

ALTERNATIF: Yerel SMS gateway (Turkiye icin)
- Netgsm (https://www.netgsm.com.tr)
- Vatan SMS (https://www.vatansms.com)
- Daha ucuz, sadece Turkiye

---

13.2 SESLI ARAMA TURN SUNUCUSU

AMAC: P2P WebRTC baglantisi basarisiz oldugunda relay

ONERILEN SAGLAYICILAR:
1. Twilio TURN (ayni hesapla)
   - Ucret: 0.40/GB
   - Regions: Global

2. Coturn (Kendi sunucun)
   - Ucret: Sadece sunucu maliyeti
   - Kontrol: Tam

3. Xirsys (https://www.xirsys.com)
   - Ucret: 49/ay (baslangic)
   - Ozellik: WebRTC odakli

KURULUM (Coturn - Kendi Sunucu):

1. Sunucu Gereksinimleri:
   - 2 vCPU, 4GB RAM, 100GB SSD
   - Ubuntu 22.04 LTS
   - Public IP, acik portlar: 3478, 5349, 10000-20000

2. Kurulum:
   
   
```bash
   sudo apt update
   sudo apt install coturn
   
   # Yapilandirma
   sudo nano /etc/turnserver.conf
   ```

3. Yapilandirma (turnserver.conf):
   
   
```


   listening-port=3478
   tls-listening-port=5349
   fingerprint
   lt-cred-mech
   realm=obscura.network
   server-name=turn.obscura.network
   
   # Statik kullanici (dinamik icin database bagla)
   user=obscura_user:guclu_sifre_123
   
   # SSL sertifikalari
   cert=/etc/letsencrypt/live/turn.obscura.network/fullchain.pem
   pkey=/etc/letsencrypt/live/turn.obscura.network/privkey.pem
   
   # Loglama
   log-file=/var/log/turnserver.log
   verbose
   ```

4. SSL Sertifikasi (Let's Encrypt):
   
   

```bash
   sudo certbot certonly --standalone -d turn.obscura.network
   ```

5. Servis Baslatma:
   
   
```bash
   sudo systemctl enable coturn
   sudo systemctl start coturn
   ```

6. Test:
   
   
```bash
   # Trickle ICE test
   https://webrtc.github.io/samples/src/content/peerconnection/trickle-ice/
   
   # Sunucu: turn.obscura.network
   # Kullanici: obscura_user
   # Sifre: guclu_sifre_123
   ```

7. WebRTC Client Entegrasyonu:
   
   
```javascript
   // JavaScript ornegi
   const pc = new RTCPeerConnection({
     iceServers: [
       { urls: "stun:stun.l.google.com:19302" },
       {
         urls: "turn:turn.obscura.network:3478",
         username: "obscura_user",
         credential: "guclu_sifre_123"
       }
     ]
   });
   ```

ENVIRONMENT DEGISKENLERI:

```


TURN_SERVER_URL=turn:turn.obscura.network:3478
TURN_SERVER_USERNAME=obscura_user
TURN_SERVER_PASSWORD=guclu_sifre_123

```

---

13.3 PUSH BILDIRIM SERVISI

AMAC: Offline kullanicilara mesaj bildirimi

PLATFORM BAZLI:

iOS - APNs (Apple Push Notification service)

1. Apple Developer Hesabi (99/yil)
2. App ID olustur (com.obscura.app)
3. Push Notification capability ac
4. APNs Auth Key olustur:
   - Keys -> All -> (+)
   - Apple Push Notifications service (APNs)
   - Indir: AuthKey_XXXXXXXXXX.p8

5. Sunucu Entegrasyonu (Go ornegi):
   
   
```go
   import "github.com/sideshow/apns2"
   
   func sendPush(deviceToken string, title, body string) error {
       client := apns2.NewTokenClient(&token.Token{
           AuthKey: apnsAuthKey,
           KeyID:   "XXXXXXXXXX",
           TeamID:  "YYYYYYYYYY",
       })
       
       notification := &apns2.Notification{
           DeviceToken: deviceToken,
           Topic:       "com.obscura.app",
           Payload: map[string]interface{}{
               "aps": map[string]interface{}{
                   "alert": map[string]interface{}{
                       "title": title,
                       "body":  body,
                   },
                   "badge": 1,
                   "sound": "default",
               },
               "message_id": "uuid", // Obscura ozel
           },
       }
       
       _, err := client.Push(notification)
       return err
   }
   ```

Android - Firebase Cloud Messaging (FCM)

1. Firebase Console (https://console.firebase.google.com)
2. Yeni proje: "Obscura"
3. Android app ekle (package: com.obscura.app)
4. google-services.json indir (Android app'e ekle)
5. Sunucu anahtari al:
   - Project Settings -> Cloud Messaging -> Server Key

6. Sunucu Entegrasyonu:
   
   
```go
   import fcm "github.com/NaySoftware/go-fcm"
   
   func sendAndroidPush(registrationToken, title, body string) error {
       data := map[string]string{
           "title":      title,
           "body":       body,
           "message_id": "uuid",
       }
       
       c := fcm.NewFcmClient("SERVER_KEY")
       c.NewFcmRegIdsMsg([]string{registrationToken}, data)
       
       status, err := c.Send()
       return err
   }
   ```

Web - Web Push API

1. VAPID anahtar cifti uret:
   
   
```bash
   npm install -g web-push
   web-push generate-vapid-keys
   ```

2. Service Worker (JavaScript):
   
   
```javascript
   self.addEventListener('push', event => {
     const data = event.data.json();
     self.registration.showNotification(data.title, {
       body: data.body,
       icon: '/icon.png',
       data: { messageId: data.message_id }
     });
   });
   ```

3. Sunucu Entegrasyonu:
   
   
```go
   import "github.com/SherClockHolmes/webpush-go"
   
   func sendWebPush(subscription, title, body string) error {
       s := &webpush.Subscription{}
       json.Unmarshal([]byte(subscription), s)
       
       resp, err := webpush.SendNotification([]byte(payload), s, &webpush.Options{
           VAPIDPublicKey:  vapidPublicKey,
           VAPIDPrivateKey: vapidPrivateKey,
           TTL:             30,
       })
       return err
   }
   ```

GIZLILIK NOTU:
- Bildirim icerigi SIFRELI olmali
- Sunucu sadece: "Yeni mesaj var" gonderebilir
- Gercek icerik client tarafinda cozulur

---

13.4 BULUT DEPOLAMA (RESIM/DOSYA ICIN)

AMAC: Buyuk dosyalar icin gecici depolama (shard'lar haric)

ONERILEN: MinIO (S3-compatible, self-hosted)
ALTERNATIF: AWS S3, Wasabi, DigitalOcean Spaces

MinIO Kurulumu:

```bash
# Docker ile
docker run -p 9000:9000 -p 9001:9001 \
  --name minio \
  -v /mnt/data:/data \
  -e "MINIO_ROOT_USER=obscuraadmin" \
  -e "MINIO_ROOT_PASSWORD=guclu_sifre_123" \
  quay.io/minio/minio server /data --console-address ":9001"
```



Kullanim:
- Her dosya S3 bucket'a yuklenir
- URL: presigned (gecici erisim)
- Sifreleme: Client-side (AES-256), MinIO sadece ciphertext gorur

---

13.5 DNS VE DOMAIN

Gereken Domainler:
- obscura.network (ana domain)
- api.obscura.network (API endpoint)
- turn.obscura.network (TURN sunucu)
- bootstrap.obscura.network (Node discovery)
- YENI: zk.obscura.network (ZK prover endpoint)
- YENI: rollup.obscura.network (zk-Rollup endpoint)

DNS Kayitlari:


```
A     obscura.network     -> Load Balancer IP
A     api.obscura.network -> API sunucu IP
A     turn.obscura.network-> TURN sunucu IP
A     zk.obscura.network  -> ZK Prover sunucu IP
CNAME bootstrap.obscura.network -> node1.obscura.network
CNAME rollup.obscura.network -> zkrollup-provider.com

TXT   _dmarc.obscura.network -> "v=DMARC1; p=quarantine; rua=mailto:dmarc@obscura.network"
TXT   obscura.network -> "v=spf1 include:_spf.google.com ~all"
```



SSL Sertifikalari:
- Let's Encrypt (ucretsiz, 90 gun)
- Certbot otomatik yenileme
- Wildcard sertifika: .obscura.network

---

13.6 IZLEME VE LOG (MONITORING)

ONERILEN: Prometheus + Grafana (self-hosted)

Kurulum:


```bash
# Prometheus
docker run -p 9090:9090 \
  -v /opt/prometheus:/etc/prometheus \
  prom/prometheus

# Grafana
docker run -p 3000:3000 \
  -v /opt/grafana:/var/lib/grafana \
  grafana/grafana
```



Metrikler:
- Node CPU/memory/network
- Mesaj gecikmesi (latency)
- Hata oranlari
- Aktif kullanici sayisi
- Kredi puani dagilimi
- YENI: ZK proof uretim/dogrulama sayisi
- YENI: ZK proof gecikmesi
- YENI: zk-Rollup islem sayisi

Alerting:
- PagerDuty veya OpsGenie entegrasyonu
- Kritik: Node down, yuksek hata orani
- YENI: ZK prover down, yuksek proof hata orani

---

13.7 CI/CD (SUREKLI ENTEGRASYON)

ONERILEN: GitHub Actions + self-hosted runner

GitHub Actions Workflow (.github/workflows/build.yml):


```yaml
name: Build and Test

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Setup Rust
        uses: actions-rs/toolchain@v1
        with:
          toolchain: stable
      
      - name: Setup Node.js (ZK)
        uses: actions/setup-node@v3
        with:
          node-version: '18'
      
      - name: Install Circom
        run: |
          git clone https://github.com/iden3/circom.git
          cd circom && cargo build --release
          sudo cp target/release/circom /usr/local/bin/
      
      - name: Install snarkJS
        run: npm install -g snarkjs
      
      - name: Build Go
        run: cd core && go build -v ./...
      
      - name: Build Rust
        run: cd crypto && cargo build --release
      
      - name: Build ZK Circuits
        run: |
          cd zk
          circom identity_proof.circom --r1cs --wasm --sym
          circom credit_threshold.circom --r1cs --wasm --sym
      
      - name: Test
        run: |
          cd core && go test ./...
          cd ../crypto && cargo test
          cd ../zk && npm test
      
      - name: Security Scan
        uses: securecodewarrior/github-action-add-sarif@v1
      
      - name: Build Flutter
        run: |
          cd client
          flutter build apk
          flutter build ios --no-codesign
```



Self-hosted Runner (guvenlik icin):


```bash
# Node sunucusunda
./config.sh --url https://github.com/yarlikhan/obscura --token XXXXX
./run.sh
```



---

13.8 ZK ALTYAPISI KURULUMU (YENI)

AMAC: Zero-knowledge proof uretimi ve dogrulama

Gereksinimler:
- Node.js 18+
- Rust 1.75+
- Circom 2.0+
- snarkJS 0.7+

Kurulum:


```bash
# Circom kurulumu
git clone https://github.com/iden3/circom.git
cd circom
cargo build --release
sudo cp target/release/circom /usr/local/bin/

# snarkJS kurulumu
npm install -g snarkjs

# Powers of Tau (trusted setup)
snarkjs powersoftau new bn128 12 pot12_0000.ptau -v
snarkjs powersoftau contribute pot12_0000.ptau pot12_0001.ptau --name="First contribution" -v
snarkjs powersoftau prepare phase2 pot12_0001.ptau pot12_final.ptau -v

# Circuit derleme
circom identity_proof.circom --r1cs --wasm --sym
snarkjs groth16 setup identity_proof.r1cs pot12_final.ptau identity_proof_0000.zkey
snarkjs zkey contribute identity_proof_0000.zkey identity_proof_0001.zkey --name="1st Contributor Name" -v
snarkjs zkey export verificationkey identity_proof_0001.zkey verification_key.json
```



Circuit Test:


```bash
# Witness olusturma
node identity_proof_js/generate_witness.js identity_proof_js/identity_proof.wasm input.json witness.wtns

# Proof uretimi
snarkjs groth16 prove identity_proof_0001.zkey witness.wtns proof.json public.json

# Proof dogrulama
snarkjs groth16 verify verification_key.json public.json proof.json
```



ENVIRONMENT DEGISKENLERI:


```
ZK_CIRCUIT_PATH=/opt/obscura/zk/circuits
ZK_PROVING_KEY_PATH=/opt/obscura/zk/keys
ZK_VERIFICATION_KEY_PATH=/opt/obscura/zk/verification
SNARKJS_PATH=/usr/local/bin/snarkjs
```


---

13.9 EKSIKLER OZET TABLOSU

Servis                    Durum    Maliyet (Aylik)  Oncelik
SMS Gateway               Eksik    50-200           Kritik
TURN Sunucusu             Eksik    50 veya 0        Kritik
Push Notifications        Eksik    0 (Firebase)      Kritik
S3/MinIO Storage          Eksik    20-100           Orta
Monitoring                Eksik    0 (self)          Orta
CI/CD                     Eksik    0 (GitHub)        Dusuk
DNS/Domain                Eksik    10/yil           Kritik
SSL Sertifikasi           Eksik    0 (Let's Encrypt) Kritik
ZK Altyapisi (Circom)     Eksik    0 (self)          Kritik
zk-Rollup (Aztec/zkSync)  Eksik    100-500          Yüksek

TOPLAM BASLANGIC: 300-800/ay (5 node + servisler + ZK)

================================================================================
BOLUM 14: DIL VE TEKNOLOJI HARITASI
================================================================================

14.1 BACKEND (CEKIRDEK)

Dil: Go 1.21+
Neden: Performans, concurrency, dagitik sistemler icin olgun ekosistem

Kutuphaneler:
- github.com/libp2p/go-libp2p (P2P networking)
- github.com/gorilla/websocket (WebSocket server)
- google.golang.org/protobuf (Protocol Buffers)
- github.com/prometheus/client_golang (Metrikler)
- YENI: github.com/iden3/go-rapidsnark (ZK proof Go binding)

Dil: Rust 1.75+
Neden: Bellek guvenligi, kriptografi performansi, ZK circuit'ler

Kutuphaneler:
- libsignal-protocol (Signal Protocol)
- openmls (MLS Protocol) // YENI
- ed25519-dalek (Imza)
- aes-gcm (Sifreleme)
- sha2 (Hash)
- YENI: circom-compat (Circom circuit Rust binding)
- YENI: arkworks (ZK proof sistemleri)

14.2 ZK KATMANI (YENI)

Dil: JavaScript/TypeScript (Node.js) + Rust
Neden: Circom/snarkJS ekosistemi Node.js tabanli

Kutuphaneler:
- snarkjs (ZK proof uretim/dogrulama)
- circomlib (Temel circuit'ler)
- ffjavascript (Finite field aritmetigi)
- YENI: noir-lang (Alternatif ZK dili)

Circuit'ler:
- identity_proof.circom: DID sahipligi
- credit_threshold.circom: Kredi puani esik
- token_balance.circom: Gizli bakiye
- storage_proof.circom: Veri depolama kaniti
- vote_proof.circom: Gizli oy

14.3 CLIENT

Onerilen: Flutter 3.19+
Diller: Dart
Kutuphaneler:
- flutter_rust_bridge (Rust FFI)
- web_socket_channel (WebSocket)
- sqflite (SQLite)
- firebase_messaging (Push)
- flutter_webrtc (WebRTC)
- YENI: flutter_zk_bridge (ZK proof FFI)

Alternatifler:
- React Native (JavaScript)
- SwiftUI (iOS native)
- Jetpack Compose (Android native)

14.4 BLOCKCHAIN

Dil: Solidity (zkSync) veya Noir (Aztec)
zkSync Era:
- Solidity smart contract
- Hardhat/Foundry gelistirme
- zkSync CLI deployment

Aztec (Alternatif):
- Noir dili
- Aztec sandbox
- Native privacy

Substrate (Governance):
- Rust FRAME pallet
- Custom runtime
- On-chain governance

14.5 AI/ML

Dil: Python 3.11+
Framework: ONNX Runtime
Modeller:
- Content filtering (TensorFlow Lite -> ONNX)
- Sentiment analysis (Hugging Face -> ONNX)
- YENI: ZK-ML (ONNX model ZK proof ile)

14.6 MINI APP

Dil: TypeScript 5.3+
Runtime: Deno 1.40+
ZK API: WebAssembly (ZK proof client-side)

================================================================================
BOLUM 15: TEST VE KALITE KRITERLERI
================================================================================

15.1 TEST PIRAMIDI

Unit Tests (> %80 coverage):
- Her fonksiyon icin
- Mock dis bagimliliklar
- Rust: cargo test
- Go: go test
- Dart: flutter test
- YENI: Circom: circom tester
- YENI: ZK: snarkJS test

Integration Tests:
- Node'lar arasi iletisim
- Client-server entegrasyonu
- End-to-end mesaj akisi
- YENI: ZK proof uretim/dogrulama akisi
- YENI: MLS grup yonetimi

E2E Tests:
- Tam kullanici senaryolari
- Flutter integration_test
- Gercek cihazlarda
- YENI: ZK katman yukseltme senaryosu

15.2 PERFORMANS HEDEFLERI

Metrik                        Hedef
Mesaj gecikmesi               < 100ms (yerel), < 300ms (kuresel)
Sesli arama baslama           < 2 saniye
Uygulama acilis               < 3 saniye
Bildirim teslimat             < 5 saniye
Node senkronizasyon           < 1 dakika (ilk)
YENI: ZK proof uretim         < 3 saniye (client)
YENI: ZK proof dogrulama      < 500ms (node)
YENI: MLS grup encrypt        < 100ms (1000 kisi)
YENI: MLS grup decrypt        < 50ms

15.3 GUVENLIK TESTLERI

- Penetrasyon testi (yilda 1)
- Bug bounty programi
- Formal verification (kripto moduller)
- Dependency scanning (SNYK, Dependabot)
- YENI: ZK circuit audit (yilda 1)
- YENI: Trusted setup ceremony (coklu katilimci)
- YENI: Side-channel attack testi (ZK proof)

================================================================================
BOLUM 16: SORUN GIDERME VE SSS
================================================================================

SORU: Node nasil baslatilir?
YANIT: cd core && go run cmd/node/main.go --config=config.yaml

SORU: Client nasil derlenir?
YANIT: cd client && flutter build apk --release

SORU: Yeni node nasil eklenir?
YANIT: Faz 3'ten once: whitelist'e ekle, imzala, dagit.
Faz 3'ten sonra: application + stake + onay.

SORU: Mesaj neden gitmiyor?
KONTROL:
1. WebSocket baglantisi acik mi?
2. Alici online mi? (DHT'de gorunuyor mu?)
3. Shard storage dolu mu?
4. Loglara bak: core/logs/node.log

SORU: Kripto hatasi aliyorum?
KONTROL:
1. Rust FFI dogru yuklendi mi?
2. libsignal versiyonu uyumlu mu?
3. Key formatlari dogru mu? (base64, raw bytes)

SORU: ZK proof hatasi aliyorum? (YENI)
KONTROL:
1. Circom dogru kuruldu mu? (circom --version)
2. snarkJS versiyonu uyumlu mu?
3. Powers of Tau dosyasi dogru mu?
4. Circuit input formati dogru mu?
5. Witness olusturma basarili mi?

SORU: Test agi nasil kurulur?
YANIT:
1. 5 node local calistir (farkli portlar)
2. Bootstrap localhost:10000
3. Client'ta bootstrap adresini degistir
4. Test et
5. YENI: ZK circuit'leri local test et
6. YENI: snarkJS ile proof uretim/dogrulama test et

===================

================================================================================ EKLER ================================================================================
EK A: PROTOKOL MESAJ FORMATI (Protobuf)
syntax = "proto3"; package obscura;
message Envelope {  string message_id = 1;  int64 timestamp = 2;  string from_did = 3;  string to_did = 4;  bytes ciphertext = 5;  MessageType type = 6;  ZKProof zk_proof = 7; // YENI }
enum MessageType {  TEXT = 0;  IMAGE = 1;  VOICE = 2;  FILE = 3;  CALL_INVITE = 4;  CALL_ACCEPT = 5;  CALL_END = 6;  GROUP_INVITE = 7; // YENI  ZK_PROOF = 8; // YENI }
message SignalMessage {  bytes ratchet_key = 1;  uint32 counter = 2;  uint32 previous_counter = 3;  bytes ciphertext = 4; }
message MLSMessage { // YENI  bytes group_id = 1;  uint32 epoch = 2;  bytes ciphertext = 3;  bytes auth_tag = 4; }
message ZKProof { // YENI  string circuit_id = 1;  bytes proof_data = 2;  repeated string public_inputs = 3;  int64 timestamp = 4; }
EK B: API ENDPOINTLERI
POST /v1/register              Kayit POST /v1/login                 Giris (telefon + OTP) POST /v1/verify-otp            OTP dogrulama GET /v1/keys/{did} Public key al POST /v1/messages              Mesaj gonder GET /v1/messages              Mesajlari al (long-polling veya WS) WS /v1/stream                Real-time mesaj akisi POST /v1/call/invite           Arama daveti POST /v1/call/answer           Arama yaniti GET /v1/nodes                 Node listesi POST /v1/governance/proposal   Teklif olustur POST /v1/governance/vote       Oy kullan
YENI: POST /v1/zk/prove              ZK proof gonder POST /v1/zk/verify             ZK proof dogrula GET /v1/zk/circuits           Mevcut circuit listesi POST /v1/credit/upgrade        Katman yukseltme (ZK proof ile) GET /v1/credit/score          Kendi puanini gor (ZK ile gizli) POST /v1/wallet/shielded       Gizli transfer GET /v1/wallet/balance        Bakiye (transparent) POST /v1/mls/group             MLS grup olustur POST /v1/mls/join              MLS gruba katil
================================================================================ SONUC ================================================================================
Bu dokuman Obscura platformunun eksiksiz teknik spesifikasyonudur (v3.0). Bu dokumani okuyan gelistirici:
1. 
Cekirdek sistemi kurabilir (Go + Rust)
2. 
ZK altyapisini kurabilir (Circom + snarkJS)
3. 
Herhangi bir dilde client uretebilir (protocol-first)
4. 
Gerekli dis servisleri tanimlayabilir ve entegre edebilir
5. 
Fazlari takip ederek platformu hayata gecirebilir
Eksikler listesi (Bolum 13) tamamlandiginda platform calisir durumda olur.
================================================================================ DOKUMAN BILGISI ================================================================================
Versiyon: 3.0-FINAL Tarih: 2026-04-26 Yazar: YarlikHan + AI Assistant Parca: 4/5 (Devam ediyor) Sayfa: ~50 sayfa (TXT formatinda) Satir: ~2500 satir
```
================================================================================
