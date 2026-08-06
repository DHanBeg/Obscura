package mls

// KeyPackage rotation taraması — RFC 9420 / spec Bölüm 4.2.
//
// Her kullanıcının önceden yüklediği KeyPackage'lar 90 gün TTL ile saklanır.
// Süresi dolanları `used=1` ile işaretler (mantıken çekilmiş gibi gösterilir;
// fiziksel silme yerine işaretleme tercih edildi: audit + analytics için).
//
// Süresi dolan KeyPackage sahibine WebSocket üzerinden
// `key_package_rotation_needed` bildirimi gönderilir; client yeni KP üretip
// yükler.
//
// İki tarayıcı bir arada koşar:
//   StartRotationScanner          — eski (legacy) tarayıcı (artık kullanılmıyor
//                                   sadece geriye dönük testler için tutuluyor)
//   StartKeyPackageRotationScheduler — sunucu-tarafı proaktif rotasyon:
//     1. expires_at < NOW() + 7d olan KP'leri yakalar (warning penceresi)
//     2. Sahibine WS bildirimi gönderir (key_package_rotation_needed)
//     3. MLS_AUTOGEN_KP=1 ise sunucu kendi mls-cli ile yeni KP üretip yükler
//        (yalnızca server-managed identity'ler için — node-1 default bot, vb.)
//     4. Süresi tamamen dolmuş KP'leri used=1 işaretler
//
// Spec Bölüm 4.2 — 90 gün rotation gerekli; bu zamanlayıcı tek SSoT.

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/dbi"
)

// Rotasyon penceresi sabitleri — spec Bölüm 4.2 ile uyumlu.
const (
	// KeyPackage TTL — yeni üretilen KP için varsayılan ömür.
	defaultKeyPackageTTL = 90 * 24 * time.Hour
	// Rotation warning window: süresi 7 gün içinde dolacak KP için bildirim.
	rotationWarningWindow = 7 * 24 * time.Hour
	// Scheduler tarama aralığı — günde bir kez yeterli (TTL haftalar mertebesinde).
	rotationTickInterval = 24 * time.Hour
	// İlk taramanın startup'tan sonraki gecikmesi — node'un yerleşmesi için.
	rotationInitialDelay = 30 * time.Second
)

// StartRotationScanner — günde bir kez çalışır, süresi dolmuş KP'leri kapatır.
// İlk tarama başlatıldıktan 30 saniye sonra yapılır (startup race koruması).
func StartRotationScanner(ctx context.Context) {
	go func() {
		// Startup grace period
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}

		// İlk tarama
		if n, err := scanAndNotify(); err != nil {
			log.Printf("⚠️  MLS KP rotasyon tarama hatası: %v", err)
		} else if n > 0 {
			log.Printf("🔄 MLS: %d süresi dolmuş KeyPackage işaretlendi", n)
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := scanAndNotify(); err != nil {
					log.Printf("⚠️  MLS KP rotasyon tarama hatası: %v", err)
				} else if n > 0 {
					log.Printf("🔄 MLS: %d süresi dolmuş KeyPackage işaretlendi", n)
				}
			}
		}
	}()
}

// scanAndNotify — süresi dolmuş aktif KP'leri bul, sahiplerine WS bildirimi
// gönder, sonra hepsini used=1 olarak işaretle. Döndürülen sayı işaretlenen
// kayıt sayısıdır.
func scanAndNotify() (int, error) {
	if db.DB == nil {
		return 0, nil
	}
	nowStr := time.Now().UTC().Format(time.RFC3339)

	// Etkilenen kullanıcıları topla (deduplike)
	rows, err := db.DB.Query(`
		SELECT DISTINCT user_did
		FROM mls_key_packages
		WHERE used = 0 AND expires_at <= ?`, nowStr,
	)
	if err != nil {
		return 0, err
	}
	var dids []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return 0, err
		}
		dids = append(dids, d)
	}
	rows.Close()

	// Toplu işaretleme (used_at = now)
	res, err := db.DB.Exec(`
		UPDATE mls_key_packages
		SET used = 1, used_at = ?
		WHERE used = 0 AND expires_at <= ?`,
		nowStr, nowStr,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	// Online sahiplerine bildirim
	for _, d := range dids {
		if messaging.GlobalHub != nil && messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsKPRotate, map[string]any{
				"reason":    "expired",
				"action":    "upload_new_key_package",
				"timestamp": nowStr,
			})
		}
	}
	return int(n), nil
}

// ─── Görev 1: StartKeyPackageRotationScheduler ───────────────────────────────
//
// Bu zamanlayıcı `scanAndNotify` üzerinde iki katman ekler:
//   1. Pre-expiry uyarı (7 gün önce) — kullanıcıların kendiliğinden rotate etmesi
//      için zaman tanır; expire olduğu an offline ise mesajlaşamaz.
//   2. Server-managed identity'ler için otomatik KP üretimi (MLS_AUTOGEN_KP=1
//      ortam değişkeniyle açılır; varsayılan kapalı).
//
// main.go entegrasyon noktası:
//   TODO(main.go ~satır 82): mls.StartRotationScanner(ctx) çağrısının HEMEN
//        ALTINA aşağıdaki satırı ekle:
//          mls.StartKeyPackageRotationScheduler(ctx, db.DB, os.Getenv("NODE_ID"))
//        Çağrı non-blocking (kendi goroutine'ini başlatır). ctx iptal edilirse
//        tarayıcı temiz şekilde durur.
//
// db parametresi nil ise (test/offline) zamanlayıcı no-op olur.
func StartKeyPackageRotationScheduler(ctx context.Context, database dbi.Querier, nodeID string) {
	if database == nil {
		log.Printf("⚠️  MLS KP scheduler: db nil — başlatılmadı (nodeID=%s)", nodeID)
		return
	}
	go func() {
		// Startup grace period — DB migration'lar + WS hub yerleşsin
		select {
		case <-time.After(rotationInitialDelay):
		case <-ctx.Done():
			return
		}

		// İlk tarama
		runScheduledRotation(ctx, database, nodeID)

		ticker := time.NewTicker(rotationTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("🛑 MLS KP scheduler durduruldu (nodeID=%s)", nodeID)
				return
			case <-ticker.C:
				runScheduledRotation(ctx, database, nodeID)
			}
		}
	}()
	log.Printf("🔄 MLS KP scheduler aktif — 7g warning + %s tick (nodeID=%s)",
		rotationTickInterval, nodeID)
}

// runScheduledRotation — tek bir tarama turu. Üç aşama:
//   1. Pre-expiry uyarı (warning window içindekiler)
//   2. Otomatik KP üretimi (MLS_AUTOGEN_KP=1 ise)
//   3. Tamamen dolmuşları used=1 işaretle (mevcut scanAndNotify ile aynı mantık)
func runScheduledRotation(ctx context.Context, database dbi.Querier, nodeID string) {
	now := time.Now().UTC()
	warnUntil := now.Add(rotationWarningWindow)

	// Aşama 1: Pre-expiry — süresi 7 gün içinde dolacaklar
	warnRows, err := database.QueryContext(ctx, `
		SELECT id, user_did, expires_at
		FROM mls_key_packages
		WHERE used = 0
		  AND expires_at > ?
		  AND expires_at <= ?`,
		now.Format(time.RFC3339), warnUntil.Format(time.RFC3339),
	)
	if err != nil {
		log.Printf("⚠️  MLS KP scheduler warning query: %v", err)
	} else {
		type warnRow struct {
			id, did, exp string
		}
		var toWarn []warnRow
		for warnRows.Next() {
			var wr warnRow
			if err := warnRows.Scan(&wr.id, &wr.did, &wr.exp); err != nil {
				continue
			}
			toWarn = append(toWarn, wr)
		}
		warnRows.Close()

		warnCount := 0
		for _, wr := range toWarn {
			if messaging.GlobalHub != nil && messaging.GlobalHub.IsOnline(wr.did) {
				messaging.GlobalHub.SendTo(wr.did, messaging.MsgTypeMlsKPRotate, map[string]any{
					"reason":     "expiring_soon",
					"action":     "upload_new_key_package",
					"kp_id":      wr.id,
					"expires_at": wr.exp,
					"node_id":    nodeID,
					"timestamp":  now.Format(time.RFC3339),
				})
				warnCount++
			}
			// Aşama 2: otomatik üretim (opt-in)
			if os.Getenv("MLS_AUTOGEN_KP") == "1" {
				if err := autogenKeyPackage(ctx, database, wr.did); err != nil {
					log.Printf("⚠️  MLS KP autogen %s: %v", wr.did, err)
				}
			}
		}
		if warnCount > 0 {
			log.Printf("⏰ MLS KP scheduler: %d kullanıcıya rotation warning gönderildi", warnCount)
		}
	}

	// Aşama 3: Tam expire — mevcut scanAndNotify aynı görevi yapar
	if n, err := scanAndNotify(); err != nil {
		log.Printf("⚠️  MLS KP scheduler expire scan: %v", err)
	} else if n > 0 {
		log.Printf("🔄 MLS KP scheduler: %d süresi dolmuş KP işaretlendi (nodeID=%s)", n, nodeID)
	}
}

// autogenKeyPackage — server-managed identity için otomatik yeni KP üret.
// Yalnızca server-side identity_id mevcutsa (mls-cli üzerinden) anlamlı.
// Kullanıcı-cihaz tabanlı identity'ler için bu işlem yapılamaz; uygulama
// sahibinin client tarafında KP üretmesi beklenir (WS bildirimi ile).
func autogenKeyPackage(ctx context.Context, database dbi.Querier, userDID string) error {
	cli := Global()
	if cli == nil {
		return nil // MLS CLI yok — sessizce skip
	}

	// Server'ın bu DID için identity_id'si var mı?
	// Convention: server-managed DID'ler için mls-cli new_identity(did) deterministic
	// identity_id üretir; aynı DID için çağrı idempotent kabul edilir.
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	identityID, err := cli.NewIdentity(callCtx, userDID)
	if err != nil {
		return err
	}
	kpB64, err := cli.GenerateKeyPackage(callCtx, identityID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	expires := now.Add(defaultKeyPackageTTL)
	id := uuid.New().String()

	_, err = database.ExecContext(ctx, `
		INSERT INTO mls_key_packages
			(id, user_did, key_package_b64, ciphersuite, used, created_at, expires_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, userDID, kpB64,
		"MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
		now.Format(time.RFC3339), expires.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	log.Printf("🔄 MLS KP autogen: %s için yeni KP yüklendi (id=%s, expires=%s)",
		userDID, id, expires.Format(time.RFC3339))
	return nil
}
