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
	"crypto/sha256"
	"database/sql"
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
