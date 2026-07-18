package api

// sealed_policy.go — Adım 10: kademeli geçiş anahtarı.
//
// Sunucu ŞU AN HER ZAMAN hem eski (zarfsız, plaintext from_did) hem yeni
// (sealed) formatı AYNI ANDA kabul eder — bkz. models.go
// SendMessageRequest.EffectiveEncryptionType() ("mls"/"signal"/"sealed"
// hepsi geçerli, göndermeyen client "signal" varsayılanına düşer) ve
// HandleSendMessage'daki tek dallanma (req.IsSealedSender()). Bu dosya o
// varsayılan-kabul davranışını DEĞİŞTİRMİYOR — yalnızca operatöre, hazır
// olduğunda eski formatı REDDETME anahtarı veriyor.
//
// VARSAYILAN KAPALI ve BİLİNÇLİ: dağıtılmış eski sürümler (ör. v1.4.7,
// BURADA_KALDIK.md) sealed hiç göndermiyor — sealAndEncryptMessage/
// receiveMessage (mobile/lib/e2e.ts) mevcut chat ekranına (chat/[id].tsx)
// henüz BAĞLANMADI BİLE, ekran hâlâ eski encryptMessage/decryptMessage'ı
// çağırıyor. Yani bugün prod'da HİÇBİR client (eski ya da şu anki) sealed
// göndermiyor — bu anahtar erken açılırsa TÜM 1:1 mesajlaşma durur. Açma
// kararı, sealed'ın gerçekten UI'a bağlandığı ve eski APK'ların
// (mağaza/Firebase dağıtımı) yeterince yenilendiği doğrulandıktan SONRA,
// operatör tarafından elle verilmeli.
//
// Karışık dönem (biri sealed gönderirken karşı taraf eski client'sa) —
// bkz. docs/adr/0016-sealed-sender-threat-model.md "Adım 10" eki: mesaj
// KAYBOLMAZ (her zamanki gibi kalıcı yazılır/iletilir), sadece eski client
// zarfı çözemez ve mevcut "çözülemeyen mesaj" yedek yoluna düşer — bu yeni
// bir hata sınıfı DEĞİL, Double Ratchet'in zaten sahip olduğu (oturum
// dışı/atlanmış anahtar) çözülemez-mesaj durumuyla aynı kategoridedir.
// Kalıcı çözüm (alıcının sealed'ı destekleyip desteklemediğini gönderen
// client'ın ÖNCEDEN bilmesi) ayrı bir kapasite-anlaşması özelliği ister;
// bu adımın kapsamı değil — ADR'de açıkça "çözülmedi, ertelendi" olarak
// işaretlendi.

import (
	"log"
	"os"

	"obscura.network/core/internal/models"
)

// sealedSenderRequired — process-wide bayrak, InitSealedSenderPolicyFromEnv
// ile başlatılır (bkz. cmd/node/main.go).
var sealedSenderRequired bool

// InitSealedSenderPolicyFromEnv OBSCURA_SEALED_SENDER_REQUIRED'dan okur.
// "true" DIŞINDA her şey (boş dahil) kapalı sayılır — varsayılan kapalı
// bilinçli bir tasarım kararı, yanlışlıkla açılma riskini azaltır.
func InitSealedSenderPolicyFromEnv() {
	sealedSenderRequired = os.Getenv("OBSCURA_SEALED_SENDER_REQUIRED") == "true"
	if sealedSenderRequired {
		log.Println("[Sealed-Sender] ZORUNLU mod aktif — grup DIŞI mesajlarda zarfsız (eski) format artık reddedilecek.")
	}
}

// sealedSenderRequiredForRequest — mandatory politika BU istek için geçerli
// mi? Grup mesajları HER ZAMAN muaf: sealed-sender zarfı tek-alıcı X25519
// DH'ına dayanır (bkz. sealed_sender.rs seal(), TEK recipient_identity_pub
// parametresi alır) — grup fanout'una mimari olarak uygulanamaz. Zorunlu
// modu grup mesajlarına da uygulamak TÜM grup mesajlaşmasını (encryption_type
// "mls" ya da grup "signal") kalıcı olarak kırar; bu yüzden IsGroup burada
// kapsam dışı, madde 15 sadece 1:1'i hedefliyor.
func sealedSenderRequiredForRequest(req *models.SendMessageRequest) bool {
	return sealedSenderRequired && !req.IsGroup
}
