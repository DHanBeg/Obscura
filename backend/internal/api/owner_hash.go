package api

// owner_hash.go — Madde 15, Adım 7: sealed mesajlarda yetkilendirme.
//
// Sealed mesajlarda from_did kalıcı yazılmıyor (bkz. ADR-0016) — sunucu
// isteği JWT ile kimliği doğrulanmış olarak alır ama bunu DB'ye yazmaz.
// Bu yüzden "bu mesaj benim mi" karşılaştırması (silme/geri alma/durum
// sorgusu) plaintext from_did üzerinden yapılamaz.
//
// Çözüm: owner_hash = HMAC-SHA256(pepper, DID+":"+msgID). Gönderim anında
// hesaplanıp saklanır; sonraki her yetki kontrolünde caller'ın JWT'den gelen
// DID'i + route'taki msgID ile aynı hesap tekrarlanır ve hmac.Equal ile
// karşılaştırılır (timing-attack'a karşı sabit-zamanlı).
//
// Özellikler:
//   - Karşılaştırılabilir: exact-match doğrulama çalışır, sunucu ekstra
//     bilgiye ihtiyaç duymadan "bu DID mi gönderdi" sorusunu yanıtlar.
//   - Tersine çevrilemez: pepper olmadan hash'ten DID çıkarılamaz.
//   - Korelasyonsuz: msgID her mesajda farklı olduğundan aynı gönderenin
//     farklı mesajları FARKLI hash üretir — DB dump'a bakan biri "bu mesajlar
//     aynı kişiden" diye kümeleyemez.
//
// Artık risk (bkz. ADR-0016 "residual risk"): pepper sızarsa VE saldırgan o
// konuşmanın küçük üye listesini biliyorsa, adayları tek tek deneyip hash'i
// kırabilir. Bu, karşılaştırılabilir HER şemanın doğasında olan bir durum
// (subscriber store'un phone_hash'i de aynı sınıf riski taşır), kusur değil.
// Ek sertleştirme (EncryptField benzeri zarf-şifreleme katmanı) gelecekte
// ayrı bir adım olarak değerlendirilebilir, şimdi eklenmedi.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"os"
)

// devMessageOwnerPepper — INSECURE dev-only fallback, subscriber.devPepper
// ile aynı desen. Üretimde ASLA kullanılmamalı; deterministik ve herkese açık.
const devMessageOwnerPepper = "obscura-insecure-dev-message-owner-pepper-change-me"

// messageOwnerPepper — process-wide pepper, InitMessageOwnerPepperFromEnv ile
// başlatılır (bkz. cmd/node/main.go).
var messageOwnerPepper []byte

// InitMessageOwnerPepperFromEnv OBSCURA_MESSAGE_OWNER_PEPPER'dan pepper yükler.
// Bilinçli olarak OBSCURA_PHONE_PEPPER'dan AYRI: farklı alanların sırrı
// karışmamalı (biri sızarsa öbürü etkilenmesin, rotasyon bağımsız kalsın).
// Prod'da eksikse FATAL (process exit), dev'de uyarı + fallback.
func InitMessageOwnerPepperFromEnv() {
	if raw := os.Getenv("OBSCURA_MESSAGE_OWNER_PEPPER"); raw != "" {
		messageOwnerPepper = []byte(raw)
		return
	}

	if os.Getenv("OBSCURA_ENV") == "production" {
		log.Fatal("OBSCURA_MESSAGE_OWNER_PEPPER env required in production")
	}

	log.Println("[GÜVENLİK UYARISI] OBSCURA_MESSAGE_OWNER_PEPPER ayarlanmamış — geliştirme fallback pepper kullanılıyor. Üretimde ASLA kullanılmamalı.")
	messageOwnerPepper = []byte(devMessageOwnerPepper)
}

// computeOwnerHash — HMAC-SHA256(pepper, did+":"+msgID), URL-safe base64.
func computeOwnerHash(did, msgID string) string {
	h := hmac.New(sha256.New, messageOwnerPepper)
	h.Write([]byte(did + ":" + msgID))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// ownerHashMatches — caller'ın (did, msgID) çifti stored owner_hash ile
// eşleşiyor mu, sabit-zamanlı karşılaştırma ile kontrol eder.
func ownerHashMatches(did, msgID, storedHash string) bool {
	if storedHash == "" {
		return false
	}
	expected := computeOwnerHash(did, msgID)
	return hmac.Equal([]byte(expected), []byte(storedHash))
}
