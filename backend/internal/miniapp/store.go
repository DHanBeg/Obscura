package miniapp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MiniApp is the registry record for a published mini app.
type MiniApp struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	DeveloperDID string   `json:"developer_did"`
	Manifest     Manifest `json:"manifest"`
	ManifestJSON string   `json:"manifest_json,omitempty"`
	CodeHash     string   `json:"code_hash"`
	SignedBy     string   `json:"signed_by,omitempty"`
	Status       string   `json:"status"`
	CreatedAt    string   `json:"created_at"`
}

// MiniAppInstall represents a user's installation of a mini app.
type MiniAppInstall struct {
	AppID              string   `json:"app_id"`
	UserDID            string   `json:"user_did"`
	GrantedPermissions []string `json:"granted_permissions"`
	InstalledAt        string   `json:"installed_at"`
}

// MiniAppSession is a record of an active or completed app execution.
type MiniAppSession struct {
	ID        string `json:"id"`
	AppID     string `json:"app_id"`
	UserDID   string `json:"user_did"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at,omitempty"`
	ProcessID int    `json:"process_id,omitempty"`
}

// ListApps returns all approved mini apps.
func ListApps(db *sql.DB) ([]MiniApp, error) {
	rows, err := db.Query(`
		SELECT id, name, version, developer_did, manifest_json, code_hash, signed_by, status, created_at
		FROM mini_apps
		WHERE status = 'approved'
		ORDER BY created_at DESC
		LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("mini apps listelenemedi: %w", err)
	}
	defer rows.Close()

	var apps []MiniApp
	for rows.Next() {
		var a MiniApp
		if err := rows.Scan(&a.ID, &a.Name, &a.Version, &a.DeveloperDID,
			&a.ManifestJSON, &a.CodeHash, &a.SignedBy, &a.Status, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("mini app satir tarama hatasi: %w", err)
		}
		_ = json.Unmarshal([]byte(a.ManifestJSON), &a.Manifest)
		apps = append(apps, a)
	}
	if apps == nil {
		apps = []MiniApp{}
	}
	return apps, nil
}

// GetApp returns a single mini app by ID (any status).
func GetApp(db *sql.DB, id string) (*MiniApp, error) {
	var a MiniApp
	err := db.QueryRow(`
		SELECT id, name, version, developer_did, manifest_json, code_hash, signed_by, status, created_at
		FROM mini_apps WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Version, &a.DeveloperDID,
			&a.ManifestJSON, &a.CodeHash, &a.SignedBy, &a.Status, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mini app alinamadi: %w", err)
	}
	_ = json.Unmarshal([]byte(a.ManifestJSON), &a.Manifest)
	return &a, nil
}

// InstallApp records or updates a user's installation of an app with the
// explicitly granted permissions. It is idempotent on (user_did, app_id).
func InstallApp(db *sql.DB, appID, userDID string, permissions []string) error {
	granted, err := json.Marshal(permissions)
	if err != nil {
		return fmt.Errorf("permissions marshal hatasi: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`
		INSERT INTO mini_app_installs (user_did, app_id, granted_permissions_json, installed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_did, app_id) DO UPDATE SET
			granted_permissions_json = excluded.granted_permissions_json,
			installed_at = excluded.installed_at`,
		userDID, appID, string(granted), now)
	if err != nil {
		return fmt.Errorf("kurulum kaydedilemedi: %w", err)
	}
	return nil
}

// UninstallApp removes a user's installation record. Returns false if no row
// was deleted (i.e. app was not installed by this user).
func UninstallApp(db *sql.DB, appID, userDID string) (bool, error) {
	res, err := db.Exec(`DELETE FROM mini_app_installs WHERE user_did = ? AND app_id = ?`,
		userDID, appID)
	if err != nil {
		return false, fmt.Errorf("kaldirma hatasi: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetUserInstalls returns all apps installed by a user.
func GetUserInstalls(db *sql.DB, userDID string) ([]MiniAppInstall, error) {
	rows, err := db.Query(`
		SELECT app_id, user_did, granted_permissions_json, installed_at
		FROM mini_app_installs
		WHERE user_did = ?
		ORDER BY installed_at DESC`, userDID)
	if err != nil {
		return nil, fmt.Errorf("kurulumlar alinamadi: %w", err)
	}
	defer rows.Close()

	var installs []MiniAppInstall
	for rows.Next() {
		var i MiniAppInstall
		var grantedJSON string
		if err := rows.Scan(&i.AppID, &i.UserDID, &grantedJSON, &i.InstalledAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(grantedJSON), &i.GrantedPermissions)
		if i.GrantedPermissions == nil {
			i.GrantedPermissions = []string{}
		}
		installs = append(installs, i)
	}
	if installs == nil {
		installs = []MiniAppInstall{}
	}
	return installs, nil
}

// GetInstall returns the install record for a specific (user, app) pair.
// Returns nil if not installed.
func GetInstall(db *sql.DB, appID, userDID string) (*MiniAppInstall, error) {
	var i MiniAppInstall
	var grantedJSON string
	err := db.QueryRow(`
		SELECT app_id, user_did, granted_permissions_json, installed_at
		FROM mini_app_installs WHERE user_did = ? AND app_id = ?`,
		userDID, appID).
		Scan(&i.AppID, &i.UserDID, &grantedJSON, &i.InstalledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("kurulum alinamadi: %w", err)
	}
	_ = json.Unmarshal([]byte(grantedJSON), &i.GrantedPermissions)
	if i.GrantedPermissions == nil {
		i.GrantedPermissions = []string{}
	}
	return &i, nil
}

// IsInstalled reports whether the user has installed the app.
func IsInstalled(db *sql.DB, appID, userDID string) bool {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM mini_app_installs WHERE user_did = ? AND app_id = ?`,
		userDID, appID).Scan(&count)
	return count > 0
}

// UpdateGrantedPermissions replaces the permission set for an existing install.
func UpdateGrantedPermissions(db *sql.DB, appID, userDID string, permissions []string) error {
	granted, err := json.Marshal(permissions)
	if err != nil {
		return fmt.Errorf("permissions marshal hatasi: %w", err)
	}
	res, err := db.Exec(`
		UPDATE mini_app_installs SET granted_permissions_json = ?
		WHERE user_did = ? AND app_id = ?`,
		string(granted), userDID, appID)
	if err != nil {
		return fmt.Errorf("izinler guncellenemedi: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("kurulum bulunamadi")
	}
	return nil
}

// CreateSession opens a new execution session record. Returns the session ID.
func CreateSession(db *sql.DB, appID, userDID string, pid int) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO mini_app_sessions (id, app_id, user_did, started_at, process_id)
		VALUES (?, ?, ?, ?, ?)`,
		id, appID, userDID, now, pid)
	if err != nil {
		return "", fmt.Errorf("oturum olusturulamadi: %w", err)
	}
	return id, nil
}

// CloseSession marks a session as ended with the current timestamp.
func CloseSession(db *sql.DB, sessionID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE mini_app_sessions SET ended_at = ?, process_id = NULL WHERE id = ?`,
		now, sessionID)
	return err
}

// ─── ZK permission persistence ────────────────────────────────────────────────

// MigrateZKPermissionsTable creates the app_zk_permissions table if it does
// not already exist. Call this alongside the main DB migration on startup.
//
// Schema:
//
//	app_zk_permissions (
//	    app_id      TEXT     — mini app ID
//	    permission  TEXT     — one of KnownZKPermissions (credit_score_read, identity_proof)
//	    granted_at  INTEGER  — Unix timestamp of user consent
//	    PRIMARY KEY (app_id, permission)
//	)
func MigrateZKPermissionsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_zk_permissions (
			app_id     TEXT    NOT NULL,
			permission TEXT    NOT NULL,
			granted_at INTEGER NOT NULL,
			PRIMARY KEY (app_id, permission)
		)`)
	if err != nil {
		return fmt.Errorf("app_zk_permissions tablosu olusturulamadi: %w", err)
	}
	return nil
}

// GrantZkPermission records that the user explicitly granted a ZK-scoped
// permission for the given app via the separate consent dialog (spec Bölüm 10.3).
// The call is idempotent — re-granting updates the granted_at timestamp.
func GrantZkPermission(db *sql.DB, appID, permission string) error {
	now := time.Now().UTC().Unix()
	_, err := db.Exec(`
		INSERT INTO app_zk_permissions (app_id, permission, granted_at)
		VALUES (?, ?, ?)
		ON CONFLICT(app_id, permission) DO UPDATE SET granted_at = excluded.granted_at`,
		appID, permission, now)
	if err != nil {
		return fmt.Errorf("zk izni kaydedilemedi (app=%s, perm=%s): %w", appID, permission, err)
	}
	return nil
}

// RevokeZkPermission removes a previously granted ZK permission. Returns false
// if the grant did not exist.
func RevokeZkPermission(db *sql.DB, appID, permission string) (bool, error) {
	res, err := db.Exec(`DELETE FROM app_zk_permissions WHERE app_id = ? AND permission = ?`,
		appID, permission)
	if err != nil {
		return false, fmt.Errorf("zk izni kaldirilirken hata (app=%s, perm=%s): %w", appID, permission, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// HasZkPermission reports whether the user has previously granted the given ZK
// permission for the app via the runtime consent dialog.
func HasZkPermission(db *sql.DB, appID, permission string) bool {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM app_zk_permissions WHERE app_id = ? AND permission = ?`,
		appID, permission).Scan(&count)
	return count > 0
}

// ListZkPermissions returns all ZK permissions granted for the given app.
func ListZkPermissions(db *sql.DB, appID string) ([]string, error) {
	rows, err := db.Query(`SELECT permission FROM app_zk_permissions WHERE app_id = ? ORDER BY permission`,
		appID)
	if err != nil {
		return nil, fmt.Errorf("zk izinleri listelenemedi: %w", err)
	}
	defer rows.Close()
	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	if perms == nil {
		perms = []string{}
	}
	return perms, nil
}

// ─── session ──────────────────────────────────────────────────────────────────

// GetActiveSession returns the most recent open session for the (user, app) pair.
// Returns nil if no active session exists.
func GetActiveSession(db *sql.DB, appID, userDID string) (*MiniAppSession, error) {
	var s MiniAppSession
	var endedAt sql.NullString
	var pid sql.NullInt64
	err := db.QueryRow(`
		SELECT id, app_id, user_did, started_at, ended_at, process_id
		FROM mini_app_sessions
		WHERE app_id = ? AND user_did = ? AND ended_at IS NULL
		ORDER BY started_at DESC LIMIT 1`,
		appID, userDID).
		Scan(&s.ID, &s.AppID, &s.UserDID, &s.StartedAt, &endedAt, &pid)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oturum alinamadi: %w", err)
	}
	if endedAt.Valid {
		s.EndedAt = endedAt.String
	}
	if pid.Valid {
		s.ProcessID = int(pid.Int64)
	}
	return &s, nil
}
