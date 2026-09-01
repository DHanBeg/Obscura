// Package logredact — METADATA FIX 2 (kimlik-log hijyeni). Tek nokta: tüm
// backend log satırları ham DID/telefon yerine bu paketin çıktısını kullanır.
// Öncesinde ÜÇ ayrı ad-hoc kırpma helper'ı vardı (messaging/hub.go:shortDID,
// api/handlers.go:shortDIDStr, scanner/scanner.go:truncate) — hepsi ham
// prefix döndürüyordu (hash DEĞİL, str[:n]), bu yüzden gerçek kimliği
// (nginx access.log $request'in tam URL logladığı gerçeğiyle aynı sınıf
// bulgu — bkz. metadata denetimi Faz 0 madde 5) doğrudan taşıyordu.
//
// v1 (sha256(s)[:4], saltsız) iki kusur taşıyordu: (1) de-anon trivial —
// bilinen bir DID'i kendi hash'leyip log'daki 8-hex ile eşleştirmek saldırgan
// için tek satır kod; (2) 32-bit çıktı ~77k farklı DID'de doğum günü
// çakışması riski taşıyor, operatör debug'ını (iki farklı kullanıcı aynı
// redaksiyonla görünür) sessizce bozar. v2: keyed HMAC-SHA256, 64-bit çıktı.
// Anahtar olmadan (log-only erişimi olan biri, log aggregator, 3. parti log
// servisi) hash tersine çevrilemez/eşlenemez — bu, "kazara log'a bakan"
// tehdit modeline karşı gerçek bir savunma sağlar.
//
// Bu paket YİNE DE tam bir GÜVENLİK SINIRI DEĞİL: operatör (DB'ye VE
// OBSCURA_LOG_REDACT_KEY'e erişimi olan) zaten users.did/phone tablosunu düz
// okuyabiliyor. Deterministik (aynı DID/telefon + aynı key → aynı çıktı,
// KASITLI — operatör aynı aktörün log satırlarını debug için ilişkilendirsin
// diye) — log satırları arası korelasyon ("aynı aktör X sonra Y yaptı") hâlâ
// mümkün, kimin olduğu (key olmadan) değil. Linkability'yi de kapatmak
// (sunucunun kim-kime yazdığını hiç görmemesi) ayrı, çok daha büyük bir
// protokol işi — metadata-minimal / sealed-sender kapsamı, bu paketin işi
// değil (v1.1 borcu, bkz. proje defteri).
package logredact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"obscura.network/core/internal/secrets"
)

// redactKey — process-wide HMAC anahtarı (env: OBSCURA_LOG_REDACT_KEY).
// Bilinçli olarak diğer pepper'lardan (OBSCURA_PHONE_PEPPER,
// OBSCURA_MESSAGE_OWNER_PEPPER) AYRI: farklı alanların sırrı karışmamalı —
// biri sızarsa öbürü etkilenmesin, rotasyon bağımsız kalsın. Paket-seviyesi
// var init + secrets.Require: D1 fail-safe (açık dev opt-in değilse prod,
// eksikse boot FATAL), dev'de sabit placeholder + uyarı — C10 fail-open
// kökü burada da tekrarlanmıyor.
var redactKey = []byte(secrets.Require("OBSCURA_LOG_REDACT_KEY"))

func redact(s string) string {
	if s == "" {
		return "?"
	}
	h := hmac.New(sha256.New, redactKey)
	h.Write([]byte(s))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8]) // 16 hex karakter (64 bit)
}

// DID — bir DID'in log-güvenli, deterministik, tersine-çevrilemez (casual)
// temsili.
func DID(did string) string {
	return redact(did)
}

// Phone — bir E.164 telefon numarasının log-güvenli, deterministik temsili.
func Phone(phone string) string {
	return redact(phone)
}

// GroupID — bir grup/konuşma kimliğinin log-güvenli, deterministik temsili.
// DID değil ama aynı sınıf bulgu: "hangi gruba kaç davet iletildi" gibi
// satırlar group_id + sayaç ile de sosyal-grafik metadata'sı taşıyor (bkz.
// messaging/hub.go:routeGroupInvite).
func GroupID(groupID string) string {
	return redact(groupID)
}
