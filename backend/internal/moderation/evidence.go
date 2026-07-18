package moderation

// evidence.go — Bölüm 2.3 kanıt tutarlılık kontrolü (TIER A).
//
// Sunucu mesaj içeriğini asla göremiyor (E2EE); bu yüzden burada YAPILAN şey
// "ciphertext'i çöz, plaintext'i doğrula" değil, VAR OLMA + ATIF tutarlılığı:
// şikayet edilen mesaj gerçekten var mı, gerçekten iddia edilen kişiden mi,
// gerçekten bu konuşmada mı, ve mağdurun sunduğu ciphertext hash'i sunucunun
// kendi kaydıyla birebir eşleşiyor mu (rastgele/uydurma message_id ile
// eşleşme ihtimalini sıfırlar). İÇERİK iddiası (ekran görüntüsündeki metin
// gerçek mi) bunun kapsamı dışında — o insan incelemesine kalır (Bölüm 2.2).
//
// Kripto primitive'i yok (imza/MAC değil, düz SHA-256 karşılaştırma) —
// bilerek: kooperatif olmayan kötü niyetli göndericiye karşı imzalı-kanıt
// mekanizması ratchet'in deniability'sini kırmadan mümkün değil (ayrı analiz,
// bkz. oturum notları). Bu fonksiyon o trade-off'u kabul eder.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrEvidenceMismatch is returned when the submitted evidence does not match
// the server's own record — the complaint must not proceed to human review as
// "corroborated" if this fires.
var ErrEvidenceMismatch = errors.New("moderation: kanıt sunucu kaydıyla eşleşmiyor")

// VerifyEvidence checks that messageID exists, was really sent by accusedDID,
// was really addressed to reporterDID (the complainant must be the message's
// recipient — only the recipient of a harmful message can be its victim), and
// that submittedCiphertextHashHex (SHA-256 hex, computed by the complainant's
// own client over the ciphertext it received) matches the ciphertext the
// server actually stored for that message.
//
// Returns (true, nil) only when every check passes. Any mismatch returns
// (false, ErrEvidenceMismatch) — never a bare bool with a swallowed reason,
// so callers can log/audit *why* a complaint failed corroboration.
func VerifyEvidence(ctx context.Context, db *sql.DB, messageID, accusedDID, reporterDID, submittedCiphertextHashHex string) (bool, error) {
	if db == nil {
		return false, errors.New("moderation: VerifyEvidence nil db")
	}
	if messageID == "" || accusedDID == "" || reporterDID == "" || submittedCiphertextHashHex == "" {
		return false, errors.New("moderation: VerifyEvidence tüm alanlar zorunlu")
	}

	var fromDID, toDID, ciphertext string
	err := db.QueryRowContext(ctx,
		`SELECT from_did, to_did, ciphertext FROM messages WHERE id = ?`, messageID,
	).Scan(&fromDID, &toDID, &ciphertext)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("%w: message_id bulunamadı", ErrEvidenceMismatch)
	}
	if err != nil {
		return false, fmt.Errorf("moderation: VerifyEvidence sorgu: %w", err)
	}

	if fromDID != accusedDID {
		return false, fmt.Errorf("%w: mesaj iddia edilen kişiden gönderilmemiş", ErrEvidenceMismatch)
	}
	if toDID != reporterDID {
		return false, fmt.Errorf("%w: şikayetçi bu mesajın alıcısı değil", ErrEvidenceMismatch)
	}

	sum := sha256.Sum256([]byte(ciphertext))
	actualHashHex := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actualHashHex, submittedCiphertextHashHex) {
		return false, fmt.Errorf("%w: ciphertext hash uyuşmuyor", ErrEvidenceMismatch)
	}

	return true, nil
}

// ─── SEALED-SENDER KANITI (Adım 8) ─────────────────────────────────────────
//
// Sealed mesajlarda from_did DB'de PLAINTEXT SAKLANMAZ (bkz. ADR-0016,
// models.go IsSealedSender, handlers.go HandleSendMessage) — bu yüzden
// VerifyEvidence'daki "fromDID != accusedDID" karşılaştırması bu mesajlar
// için hiç çalışmaz (fromDID her zaman ""). Çözüm Signal deseni: şikayetçi
// kendi Unseal() çıktısından çıkan sertifika alanlarını + ham Ed25519
// imzasını kanıt olarak sunar (bkz. mobile/lib/sender-attestation-cache.ts);
// sunucu içeriği HİÇ ÇÖZMEDEN — sadece bu imzayı bağımsızca yeniden
// doğrulayarak — "bu identity_dh_pub'ın sahibi gerçekten bu DID'i iddia
// etti" bağını kurar. boundary_test.go'daki yasaklı-çağrı taraması bunu
// engellemez: burada zarf açma / ratchet durumu okuma çağrılarından hiçbiri
// yok, sadece ed25519.Verify ile saf imza doğrulaması.
//
// certSigningPrefix/certSigningBytes crypto/src/sealed_sender.rs
// (SenderCertificate::signing_bytes, CERT_SIGNING_PREFIX) ve
// mobile/lib/sealed-sender.ts (certSigningBytes, CERT_SIGNING_PREFIX) ile
// BİREBİR aynı olmalı — üçünden biri kayarsa imza hiçbir zaman doğrulanmaz.

const certSigningPrefix = "obscura-sender-cert-v1"

// SealedCertEvidence — şikayetçinin Unseal() çıktısından sunduğu, sertifikayı
// yeniden doğrulamak için gereken ham alanlar (hex-encoded).
type SealedCertEvidence struct {
	IdentityDHPubHex string // sertifikadaki gönderen X25519 kimlik açık anahtarı (32 byte)
	SigningPubHex    string // sertifikadaki gönderen Ed25519 imzalama açık anahtarı (32 byte)
	ExpiresAt        uint64 // sertifika geçerlilik sonu (unix saniye, 0 = süresiz)
	SignatureHex     string // sertifikanın ham Ed25519 imzası (64 byte)
}

// certSigningBytes — crypto/src/sealed_sender.rs SenderCertificate::signing_bytes
// ile birebir: prefix‖did‖identity_dh_pub‖signing_pub‖expires_at_be(8 byte).
func certSigningBytes(did string, identityDHPub, signingPub []byte, expiresAt uint64) []byte {
	buf := make([]byte, 0, len(certSigningPrefix)+len(did)+len(identityDHPub)+len(signingPub)+8)
	buf = append(buf, certSigningPrefix...)
	buf = append(buf, did...)
	buf = append(buf, identityDHPub...)
	buf = append(buf, signingPub...)
	var expBE [8]byte
	binary.BigEndian.PutUint64(expBE[:], expiresAt)
	return append(buf, expBE[:]...)
}

// verifySealedCertificate — crypto/src/sealed_sender.rs
// SenderCertificate::verify() ile aynı üç kontrolü yapar (imza geçerli mi,
// DID gerçekten identity_dh_pub'dan türemiş mi, süresi dolmuş mu) — ama
// zarfı AÇMAZ, sadece bu alanlar üzerinde imza doğrulaması yapar.
//
// `accusedDID` şikayetçinin iddia ettiği reported_did'dir — signing_bytes'a
// bu string girer, bu yüzden imzayı üreten taraf (accused) gerçekten KENDİ
// DID'i için sertifika üretmemişse (ör. şikayetçi başka birinin DID'ini
// buraya yazarsa) imza asla uyuşmaz; ayrı bir DID↔anahtar kontrolüne gerek
// kalmadan bu sahtecilik burada elenir. Yine de Rust'taki gibi açıkça de
// doğrulanır (savunma derinliği + hata mesajı netliği).
func verifySealedCertificate(accusedDID string, ev SealedCertEvidence, now uint64) error {
	identityDHPub, err := hex.DecodeString(ev.IdentityDHPubHex)
	if err != nil || len(identityDHPub) != 32 {
		return fmt.Errorf("%w: geçersiz identity_dh_pub", ErrEvidenceMismatch)
	}
	signingPub, err := hex.DecodeString(ev.SigningPubHex)
	if err != nil || len(signingPub) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: geçersiz signing_pub", ErrEvidenceMismatch)
	}
	signature, err := hex.DecodeString(ev.SignatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: geçersiz imza", ErrEvidenceMismatch)
	}

	msg := certSigningBytes(accusedDID, identityDHPub, signingPub, ev.ExpiresAt)
	if !ed25519.Verify(ed25519.PublicKey(signingPub), msg, signature) {
		return fmt.Errorf("%w: sertifika imzası geçersiz", ErrEvidenceMismatch)
	}

	sum := sha256.Sum256(identityDHPub)
	expectedDID := "did:obs:" + hex.EncodeToString(sum[:16])
	if accusedDID != expectedDID {
		return fmt.Errorf("%w: sertifika DID'i kimlik anahtarıyla eşleşmiyor", ErrEvidenceMismatch)
	}

	if ev.ExpiresAt != 0 && now > ev.ExpiresAt {
		return fmt.Errorf("%w: sertifika süresi dolmuş", ErrEvidenceMismatch)
	}
	return nil
}

// VerifySealedEvidence — VerifyEvidence'ın sealed-sender karşılığı. Mesaj
// varlığı, alıcı (reporter == to_did) ve ciphertext hash kontrolleri
// VerifyEvidence ile AYNIdır; yalnızca "gerçekten accused'dan mı geldi"
// kanıtının kaynağı değişir: plaintext from_did karşılaştırması yerine
// imza doğrulaması. Mesaj gerçekten "sealed" olarak işaretli değilse
// reddeder — bu yol yalnızca sealed mesajlar için geçerlidir, çağıran
// (HandleSpamReport) sealed olmayanlar için VerifyEvidence'ı kullanmaya
// devam eder.
func VerifySealedEvidence(ctx context.Context, db *sql.DB, messageID, accusedDID, reporterDID, submittedCiphertextHashHex string, cert SealedCertEvidence, now uint64) (bool, error) {
	if db == nil {
		return false, errors.New("moderation: VerifySealedEvidence nil db")
	}
	if messageID == "" || accusedDID == "" || reporterDID == "" || submittedCiphertextHashHex == "" {
		return false, errors.New("moderation: VerifySealedEvidence tüm alanlar zorunlu")
	}
	if cert.IdentityDHPubHex == "" || cert.SigningPubHex == "" || cert.SignatureHex == "" {
		return false, errors.New("moderation: VerifySealedEvidence sertifika alanları zorunlu")
	}

	var toDID, ciphertext, encryptionType string
	err := db.QueryRowContext(ctx,
		`SELECT to_did, ciphertext, encryption_type FROM messages WHERE id = ?`, messageID,
	).Scan(&toDID, &ciphertext, &encryptionType)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("%w: message_id bulunamadı", ErrEvidenceMismatch)
	}
	if err != nil {
		return false, fmt.Errorf("moderation: VerifySealedEvidence sorgu: %w", err)
	}

	if encryptionType != "sealed" {
		return false, fmt.Errorf("%w: mesaj sealed-sender değil", ErrEvidenceMismatch)
	}
	if toDID != reporterDID {
		return false, fmt.Errorf("%w: şikayetçi bu mesajın alıcısı değil", ErrEvidenceMismatch)
	}

	sum := sha256.Sum256([]byte(ciphertext))
	actualHashHex := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actualHashHex, submittedCiphertextHashHex) {
		return false, fmt.Errorf("%w: ciphertext hash uyuşmuyor", ErrEvidenceMismatch)
	}

	if err := verifySealedCertificate(accusedDID, cert, now); err != nil {
		return false, err
	}

	return true, nil
}
