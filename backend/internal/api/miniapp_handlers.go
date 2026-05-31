package api

// Mini App Motoru HTTP endpoints — spec Bölüm 10 (FAZ 2 skeleton).
//
// Registry + permission model. The Deno sandbox runner lives in
// internal/miniapp/sandbox.go; full ZK-API bridge integration is FAZ 3.
//
// Endpoints (mounted under the authenticated /v1 subrouter):
//   POST   /v1/apps                 publish app   (developer: Platinum+)
//   GET    /v1/apps                 list approved apps
//   GET    /v1/apps/{id}            app detail + manifest
//   POST   /v1/apps/{id}/install    install + grant permissions (Silver+)
//   POST   /v1/apps/{id}/run        run installed app (tier + install + perms)
//   DELETE /v1/apps/{id}/install    uninstall
//
// Tier model (spec Bölüm 10.4): 1=Bronze 2=Silver 3=Gold 4=Platinum 5=Diamond
//   Bronze   : cannot run, cannot create
//   Silver   : can run
//   Gold     : can run
//   Platinum : can run + create
//   Diamond  : can run + create

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/media"
	"obscura.network/core/internal/miniapp"
)

const (
	maxMiniAppBody  = 2 << 20 // 2MB: manifest + code
	miniAppRunLimit = 10 * time.Second
)

// ─── Request types ───────────────────────────────────────────────────────────

type PublishAppRequest struct {
	Manifest json.RawMessage `json:"manifest"` // raw manifest.json
	Code     string          `json:"code"`     // app source (TypeScript/JavaScript)
	SignedBy string          `json:"signed_by,omitempty"`
}

type InstallAppRequest struct {
	// Permissions the user explicitly grants. Must be a subset of the
	// manifest's declared permissions.
	GrantedPermissions []string `json:"granted_permissions"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func logMiniAppPermission(userDID, appID, permission, action string) {
	_, _ = db.DB.Exec(`
		INSERT INTO mini_app_permissions_log (id, user_did, app_id, permission, action, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), userDID, appID, permission, action,
		time.Now().UTC().Format(time.RFC3339))
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// POST /v1/apps — publish a mini app. Developer must be Platinum+ (tier >= 4).
func HandlePublishApp(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if user.Tier < miniapp.TierPlatinum {
		respond(w, 403, nil, "Mini app yayınlamak için Platin katman gerekli (spec Bölüm 10.4)")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMiniAppBody)
	var req PublishAppRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if len(req.Manifest) == 0 || req.Code == "" {
		respond(w, 400, nil, "manifest ve code zorunlu")
		return
	}

	m, err := miniapp.ParseManifest(req.Manifest)
	if err != nil {
		respond(w, 400, nil, "Manifest doğrulanamadı: "+err.Error())
		return
	}
	// Manifest's developer_did must match the authenticated publisher.
	if m.DeveloperDID != user.DID {
		respond(w, 403, nil, "manifest.developer_did kimliğinizle eşleşmiyor")
		return
	}

	sum := sha256.Sum256([]byte(req.Code))
	codeHash := hex.EncodeToString(sum[:])
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Kodu MinIO'ya yükle — key: miniapp/{id}/code.ts
	objectKey := "miniapp/" + id + "/code.ts"
	codeReader := strings.NewReader(req.Code)
	if _, err := media.Upload(r.Context(), objectKey, codeReader, int64(len(req.Code)), "text/typescript"); err != nil {
		respond(w, 500, nil, "Mini app kodu depolanamadı: "+err.Error())
		return
	}

	_, err = db.DB.Exec(`
		INSERT INTO mini_apps
			(id, name, version, developer_did, manifest_json, code_hash, signed_by, status, created_at, code_object_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		id, m.Name, m.Version, m.DeveloperDID, string(req.Manifest), codeHash, req.SignedBy, now, objectKey)
	if err != nil {
		// MinIO'ya yüklendi ama DB'ye yazılamadı — temizle
		_ = media.Delete(r.Context(), objectKey)
		respond(w, 500, nil, "Mini app kaydedilemedi: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"id":         id,
		"name":       m.Name,
		"version":    m.Version,
		"code_hash":  codeHash,
		"status":     "pending",
		"created_at": now,
	}, "")
}

// GET /v1/apps — list approved apps.
func HandleListApps(w http.ResponseWriter, r *http.Request) {
	if getUser(r) == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	rows, err := db.DB.Query(`
		SELECT id, name, version, developer_did, code_hash, status, created_at
		FROM mini_apps
		WHERE status = 'approved'
		ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer rows.Close()

	type appRow struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Version      string `json:"version"`
		DeveloperDID string `json:"developer_did"`
		CodeHash     string `json:"code_hash"`
		Status       string `json:"status"`
		CreatedAt    string `json:"created_at"`
	}
	var out []appRow
	for rows.Next() {
		var a appRow
		if err := rows.Scan(&a.ID, &a.Name, &a.Version, &a.DeveloperDID, &a.CodeHash, &a.Status, &a.CreatedAt); err != nil {
			respond(w, 500, nil, "DB hatası: "+err.Error())
			return
		}
		out = append(out, a)
	}
	respond(w, 200, out, "")
}

// GET /v1/apps/{id} — app detail + manifest.
func HandleGetApp(w http.ResponseWriter, r *http.Request) {
	if getUser(r) == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	id := mux.Vars(r)["id"]

	var name, version, devDID, manifestJSON, codeHash, signedBy, status, createdAt string
	err := db.DB.QueryRow(`
		SELECT name, version, developer_did, manifest_json, code_hash, signed_by, status, created_at
		FROM mini_apps WHERE id = ?`, id).
		Scan(&name, &version, &devDID, &manifestJSON, &codeHash, &signedBy, &status, &createdAt)
	if err != nil {
		respond(w, 404, nil, "Mini app bulunamadı")
		return
	}

	respond(w, 200, map[string]any{
		"id":            id,
		"name":          name,
		"version":       version,
		"developer_did": devDID,
		"manifest":      json.RawMessage(manifestJSON),
		"code_hash":     codeHash,
		"signed_by":     signedBy,
		"status":        status,
		"created_at":    createdAt,
	}, "")
}

// POST /v1/apps/{id}/install — install + grant permissions. User must be Silver+.
func HandleInstallApp(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if user.Tier < miniapp.TierSilver {
		respond(w, 403, nil, "Mini app kullanmak için en az Gümüş katman gerekli (spec Bölüm 10.4)")
		return
	}
	appID := mux.Vars(r)["id"]

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req InstallAppRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	var manifestJSON, status string
	err := db.DB.QueryRow(`SELECT manifest_json, status FROM mini_apps WHERE id = ?`, appID).
		Scan(&manifestJSON, &status)
	if err != nil {
		respond(w, 404, nil, "Mini app bulunamadı")
		return
	}
	if status != "approved" {
		respond(w, 403, nil, "Mini app onaylı değil (status="+status+")")
		return
	}

	m, err := miniapp.ParseManifest([]byte(manifestJSON))
	if err != nil {
		respond(w, 500, nil, "Kayıtlı manifest bozuk: "+err.Error())
		return
	}
	// Tier gate from the manifest itself.
	if user.Tier < m.MinTier {
		respond(w, 403, nil, "Bu mini app daha yüksek katman gerektiriyor")
		return
	}

	// Granted permissions must be a subset of declared permissions.
	declared := map[string]bool{}
	for _, p := range m.Permissions {
		declared[p] = true
	}
	for _, g := range req.GrantedPermissions {
		if !declared[g] {
			respond(w, 400, nil, "İzin manifest'te tanımlı değil: "+g)
			return
		}
	}

	grantedJSON, _ := json.Marshal(req.GrantedPermissions)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.DB.Exec(`
		INSERT INTO mini_app_installs (user_did, app_id, granted_permissions_json, installed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_did, app_id) DO UPDATE SET
			granted_permissions_json = excluded.granted_permissions_json,
			installed_at = excluded.installed_at`,
		user.DID, appID, string(grantedJSON), now)
	if err != nil {
		respond(w, 500, nil, "Kurulum kaydedilemedi: "+err.Error())
		return
	}

	for _, g := range req.GrantedPermissions {
		logMiniAppPermission(user.DID, appID, g, "grant")
	}

	respond(w, 200, map[string]any{
		"app_id":              appID,
		"granted_permissions": req.GrantedPermissions,
		"installed_at":        now,
	}, "")
}

// DELETE /v1/apps/{id}/install — uninstall.
func HandleUninstallApp(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	appID := mux.Vars(r)["id"]

	res, err := db.DB.Exec(`DELETE FROM mini_app_installs WHERE user_did = ? AND app_id = ?`,
		user.DID, appID)
	if err != nil {
		respond(w, 500, nil, "Kaldırma hatası: "+err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		respond(w, 404, nil, "Kurulum bulunamadı")
		return
	}
	logMiniAppPermission(user.DID, appID, "*", "revoke")
	respond(w, 200, map[string]any{"uninstalled": true, "app_id": appID}, "")
}

// POST /v1/apps/{id}/run — run an installed app in the Deno sandbox.
// Checks: tier (Silver+), install present, manifest tier gate. Then spawns the
// sandbox. If deno is not installed the call returns a clear 503.
func HandleRunApp(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	if user.Tier < miniapp.TierSilver {
		respond(w, 403, nil, "Mini app çalıştırmak için en az Gümüş katman gerekli (spec Bölüm 10.4)")
		return
	}
	appID := mux.Vars(r)["id"]

	// Must be installed.
	var grantedJSON string
	err := db.DB.QueryRow(`SELECT granted_permissions_json FROM mini_app_installs WHERE user_did = ? AND app_id = ?`,
		user.DID, appID).Scan(&grantedJSON)
	if err != nil {
		respond(w, 403, nil, "Mini app kurulu değil — önce yükleyin")
		return
	}

	var manifestJSON, status string
	err = db.DB.QueryRow(`SELECT manifest_json, status FROM mini_apps WHERE id = ?`, appID).
		Scan(&manifestJSON, &status)
	if err != nil {
		respond(w, 404, nil, "Mini app bulunamadı")
		return
	}
	if status != "approved" {
		respond(w, 403, nil, "Mini app onaylı değil")
		return
	}

	m, err := miniapp.ParseManifest([]byte(manifestJSON))
	if err != nil {
		respond(w, 500, nil, "Kayıtlı manifest bozuk: "+err.Error())
		return
	}
	if user.Tier < m.MinTier {
		respond(w, 403, nil, "Bu mini app daha yüksek katman gerektiriyor")
		return
	}

	// Permission audit: log that the app run was attempted with its granted set.
	logMiniAppPermission(user.DID, appID, "run", "invoke")

	// MinIO'dan kod nesne anahtarını al
	var codeObjectKey string
	if err := db.DB.QueryRow(`SELECT code_object_key FROM mini_apps WHERE id = ?`, appID).Scan(&codeObjectKey); err != nil || codeObjectKey == "" {
		respond(w, 500, nil, "Mini app kod konumu bulunamadı")
		return
	}

	// Kodu MinIO'dan çek
	ctx, cancel := context.WithTimeout(r.Context(), miniAppRunLimit+5*time.Second)
	defer cancel()

	codeBytes, err := media.Download(ctx, codeObjectKey)
	if err != nil {
		respond(w, 500, nil, "Mini app kodu indirilemedi: "+err.Error())
		return
	}
	if codeBytes == nil {
		respond(w, 404, nil, "Mini app kodu depolamada bulunamadı (silinmiş olabilir)")
		return
	}

	// Sandbox yapılandırması
	cfg := miniapp.SandboxConfigFromManifest(m, miniAppRunLimit)

	// Deno sandbox'ında çalıştır
	stdout, runErr := miniapp.RunApp(ctx, string(codeBytes), cfg)
	if runErr != nil {
		if errors.Is(runErr, miniapp.ErrDenoNotInstalled) {
			respond(w, 503, map[string]any{
				"app_id": appID,
				"hint":   "Sunucuya Deno yükleyin: https://deno.land",
			}, "Mini app runtime yok: deno bulunamadı")
			return
		}
		respond(w, 500, map[string]any{
			"app_id": appID,
			"stdout": stdout,
			"error":  runErr.Error(),
		}, "Mini app çalışma hatası")
		return
	}

	logMiniAppPermission(user.DID, appID, "run", "complete")
	respond(w, 200, map[string]any{
		"app_id": appID,
		"stdout": stdout,
	}, "")
}
