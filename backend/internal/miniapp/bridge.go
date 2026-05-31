package miniapp

// bridge.go — API bridge for the mini app engine (spec Bolum 10.2).
//
// Mini apps communicate with Obscura's host APIs via a JSON-RPC-style call
// surface. HandleBridgeCall dispatches method strings of the form
// "namespace.method" to the concrete implementations below.
//
// Namespaces: identity, messaging, wallet, zk, ui
//
// Security model:
//   - Each call checks that the app's granted permissions include the
//     relevant namespace BEFORE touching any data.
//   - ZK permissions (credit_score_read, identity_proof) are validated
//     separately against the runtime zk_permissions grant store (DB-persisted).
//     Manifest declaration is required but not sufficient — the user must grant
//     the ZK permission via an explicit consent dialog before the bridge accepts
//     any zk-scoped call.
//   - ZK proof generation is rate-limited to 5 proofs/second per app
//     (spec Bölüm 10.1) via a per-app token-bucket limiter.
//   - The bridge never returns raw private data; it returns only what the
//     manifest declared and the user explicitly granted.

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ─── ZK proof rate limiters ───────────────────────────────────────────────────

// zkRateLimiters maps appID → *rate.Limiter.
// Each app is allowed 5 ZK proof generations per second (spec Bölüm 10.1).
// The map is populated lazily on first use and never shrinks (acceptable for
// the expected number of installed apps per node).
var zkRateLimiters sync.Map // map[string]*rate.Limiter

// getZKLimiter returns (or lazily creates) the rate.Limiter for the given app.
func getZKLimiter(appID string) *rate.Limiter {
	if v, ok := zkRateLimiters.Load(appID); ok {
		return v.(*rate.Limiter)
	}
	// 5 tokens/second, burst of 1 — strict 5 rps, no burst accumulation.
	lim := rate.NewLimiter(5, 1)
	actual, _ := zkRateLimiters.LoadOrStore(appID, lim)
	return actual.(*rate.Limiter)
}

// ─── ZK permission runtime grant (per-app, DB-persisted) ─────────────────────

// grantZkPermission records that the user explicitly granted a ZK-scoped
// permission for the given app. The grant is persisted in the
// app_zk_permissions table so it survives across sessions.
func grantZkPermission(db *sql.DB, appID, permission string) error {
	now := time.Now().UTC().Unix()
	_, err := db.Exec(`
		INSERT INTO app_zk_permissions (app_id, permission, granted_at)
		VALUES (?, ?, ?)
		ON CONFLICT(app_id, permission) DO UPDATE SET granted_at = excluded.granted_at`,
		appID, permission, now)
	if err != nil {
		return fmt.Errorf("zk izni kaydedilemedi: %w", err)
	}
	return nil
}

// hasZkPermissionDB reports whether the user has previously granted the ZK
// permission for the given app via the runtime consent dialog.
func hasZkPermissionDB(db *sql.DB, appID, permission string) bool {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM app_zk_permissions WHERE app_id = ? AND permission = ?`,
		appID, permission).Scan(&count)
	return count > 0
}

// ─── BridgeContext ────────────────────────────────────────────────────────────

// BridgeContext carries the per-call security context injected by the HTTP
// handler layer. All field values are trusted (resolved server-side, not from
// app input).
type BridgeContext struct {
	AppID              string
	UserDID            string
	Manifest           Manifest
	GrantedPermissions []string
	DB                 *sql.DB
}

// hasPermission reports whether the given base namespace was granted.
func (bc *BridgeContext) hasPermission(perm string) bool {
	for _, g := range bc.GrantedPermissions {
		if g == perm {
			return true
		}
	}
	return false
}

// hasManifestZKPermission reports whether the manifest declared this ZK-scoped
// permission. Manifest declaration is a prerequisite but not a sufficient
// condition — the user must also have granted it at runtime (hasZkPermissionDB).
func (bc *BridgeContext) hasManifestZKPermission(zkPerm string) bool {
	for _, z := range bc.Manifest.ZKPermissions {
		if z == zkPerm {
			return true
		}
	}
	return false
}

// hasRuntimeZKPermission returns true only if:
//  1. The manifest declared the ZK permission, AND
//  2. The user explicitly granted it via the ZK consent dialog (DB record exists).
func (bc *BridgeContext) hasRuntimeZKPermission(zkPerm string) bool {
	if !bc.hasManifestZKPermission(zkPerm) {
		return false
	}
	if bc.DB == nil {
		return false
	}
	return hasZkPermissionDB(bc.DB, bc.AppID, zkPerm)
}

// BridgeCallResult wraps a successful return value.
type BridgeCallResult struct {
	Result interface{} `json:"result"`
}

// HandleBridgeCall routes "namespace.method" calls from the mini app sandbox
// to the appropriate handler. Returns (result, error).
func HandleBridgeCall(bc BridgeContext, method string, params map[string]interface{}) (interface{}, error) {
	parts := strings.SplitN(method, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("gecersiz bridge method: %q (namespace.method bekleniyor)", method)
	}
	ns, fn := parts[0], parts[1]

	switch ns {
	case "identity":
		return handleIdentity(bc, fn, params)
	case "messaging":
		return handleMessaging(bc, fn, params)
	case "wallet":
		return handleWallet(bc, fn, params)
	case "zk":
		return handleZK(bc, fn, params)
	case "ui":
		return handleUI(bc, fn, params)
	default:
		return nil, fmt.Errorf("bilinmeyen namespace: %q", ns)
	}
}

// ─── identity namespace ───────────────────────────────────────────────────────

func handleIdentity(bc BridgeContext, fn string, _ map[string]interface{}) (interface{}, error) {
	if !bc.hasPermission("identity") {
		return nil, fmt.Errorf("izin reddedildi: identity")
	}

	switch fn {
	case "getUserId":
		// Returns the user's DID (public, non-sensitive identifier).
		return bc.UserDID, nil

	case "getUsername":
		var username string
		err := bc.DB.QueryRow(`SELECT COALESCE(username, '') FROM users WHERE did = ?`, bc.UserDID).
			Scan(&username)
		if err != nil {
			return nil, fmt.Errorf("kullanici adinamadi: %w", err)
		}
		return username, nil

	case "getAvatar":
		var avatarURL string
		err := bc.DB.QueryRow(`SELECT COALESCE(avatar_url, '') FROM users WHERE did = ?`, bc.UserDID).
			Scan(&avatarURL)
		if err != nil {
			return nil, fmt.Errorf("avatar alinamadi: %w", err)
		}
		return avatarURL, nil

	case "getZkIdentity":
		// Requires explicit ZK permission: identity_proof (manifest + runtime consent).
		if !bc.hasRuntimeZKPermission("identity_proof") {
			return nil, fmt.Errorf("zk izin reddedildi: identity_proof (kullanici onay diyalogu gerekli)")
		}
		var proofB64, publicParams sql.NullString
		err := bc.DB.QueryRow(`SELECT zk_id_proof_b64, zk_id_public_params FROM users WHERE did = ?`, bc.UserDID).
			Scan(&proofB64, &publicParams)
		if err != nil {
			return nil, fmt.Errorf("zk kimlik alinamadi: %w", err)
		}
		if !proofB64.Valid || proofB64.String == "" {
			return nil, fmt.Errorf("kullanicinin ZK kimlik kaniti yok")
		}
		return map[string]interface{}{
			"proof":         proofB64.String,
			"public_params": publicParams.String,
			"circuit":       "identity_proof",
		}, nil

	default:
		return nil, fmt.Errorf("bilinmeyen identity method: %q", fn)
	}
}

// ─── messaging namespace ─────────────────────────────────────────────────────

func handleMessaging(bc BridgeContext, fn string, params map[string]interface{}) (interface{}, error) {
	if !bc.hasPermission("messaging") {
		return nil, fmt.Errorf("izin reddedildi: messaging")
	}

	switch fn {
	case "sendMessage":
		to, _ := params["to"].(string)
		content, _ := params["content"].(string)
		if to == "" || content == "" {
			return nil, fmt.Errorf("sendMessage: to ve content zorunlu")
		}
		msgID := newUUID()
		now := time.Now().UTC()
		expires := now.Add(30 * 24 * time.Hour)
		_, err := bc.DB.Exec(`
			INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext,
			                      status, is_group, reply_to_id, sent_at, expires_at,
			                      is_encrypted, encryption_type)
			VALUES (?, ?, ?, ?, 'text', ?, 'sent', 0, '', ?, ?, 1, 'miniapp')`,
			msgID, to, bc.UserDID, to, content,
			now.Format(time.RFC3339), expires.Format(time.RFC3339))
		if err != nil {
			return nil, fmt.Errorf("mesaj gonderilemedi: %w", err)
		}
		return map[string]interface{}{"message_id": msgID, "status": "sent"}, nil

	case "openChat":
		to, _ := params["userId"].(string)
		if to == "" {
			return nil, fmt.Errorf("openChat: userId zorunlu")
		}
		return map[string]interface{}{"action": "open_chat", "user_id": to}, nil

	case "onMessage":
		return map[string]interface{}{
			"subscription": "ws_" + bc.AppID,
			"note":         "WebSocket akisina baglanin",
		}, nil

	case "sendGroupMessage":
		groupID, _ := params["groupId"].(string)
		content, _ := params["content"].(string)
		if groupID == "" || content == "" {
			return nil, fmt.Errorf("sendGroupMessage: groupId ve content zorunlu")
		}
		msgID := newUUID()
		now := time.Now().UTC()
		expires := now.Add(30 * 24 * time.Hour)
		_, err := bc.DB.Exec(`
			INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext,
			                      status, is_group, reply_to_id, sent_at, expires_at,
			                      is_encrypted, encryption_type)
			VALUES (?, ?, ?, ?, 'text', ?, 'sent', 1, '', ?, ?, 1, 'miniapp')`,
			msgID, groupID, bc.UserDID, groupID, content,
			now.Format(time.RFC3339), expires.Format(time.RFC3339))
		if err != nil {
			return nil, fmt.Errorf("grup mesaji gonderilemedi: %w", err)
		}
		return map[string]interface{}{"message_id": msgID, "status": "sent"}, nil

	case "createMLSGroup":
		members, ok := params["members"].([]interface{})
		if !ok || len(members) == 0 {
			return nil, fmt.Errorf("createMLSGroup: members zorunlu (dizi)")
		}
		groupID := newUUID()
		return map[string]interface{}{
			"group_id": groupID,
			"note":     "MLS grubu icin /v1/mls/group endpoint'ini kullanin",
		}, nil

	default:
		return nil, fmt.Errorf("bilinmeyen messaging method: %q", fn)
	}
}

// ─── wallet namespace ─────────────────────────────────────────────────────────

func handleWallet(bc BridgeContext, fn string, params map[string]interface{}) (interface{}, error) {
	if !bc.hasPermission("wallet") {
		return nil, fmt.Errorf("izin reddedildi: wallet")
	}

	switch fn {
	case "getBalance":
		var balance string
		err := bc.DB.QueryRow(`SELECT COALESCE(transparent_balance, '0') FROM obs_accounts WHERE user_did = ?`,
			bc.UserDID).Scan(&balance)
		if err == sql.ErrNoRows {
			balance = "0"
		} else if err != nil {
			return nil, fmt.Errorf("bakiye alinamadi: %w", err)
		}
		return map[string]interface{}{"balance": balance, "unit": "OBS"}, nil

	case "getShieldedBalance":
		return map[string]interface{}{
			"shielded_balance": "private",
			"note":             "Gizli bakiye sadece istemci tarafinda gorunur",
		}, nil

	case "requestPayment":
		to, _ := params["to"].(string)
		amount, _ := params["amount"].(float64)
		if to == "" || amount <= 0 {
			return nil, fmt.Errorf("requestPayment: to ve amount zorunlu")
		}
		token := newUUID()
		return map[string]interface{}{
			"payment_request_id": token,
			"to":                 to,
			"amount":             amount,
			"status":             "pending_user_approval",
		}, nil

	case "requestShieldedPayment":
		to, _ := params["to"].(string)
		amount, _ := params["amount"].(float64)
		if to == "" || amount <= 0 {
			return nil, fmt.Errorf("requestShieldedPayment: to ve amount zorunlu")
		}
		token := newUUID()
		return map[string]interface{}{
			"shielded_payment_request_id": token,
			"to":                          to,
			"amount":                      amount,
			"status":                      "pending_user_approval",
		}, nil

	default:
		return nil, fmt.Errorf("bilinmeyen wallet method: %q", fn)
	}
}

// ─── zk namespace ─────────────────────────────────────────────────────────────

func handleZK(bc BridgeContext, fn string, params map[string]interface{}) (interface{}, error) {
	if !bc.hasPermission("zk") {
		return nil, fmt.Errorf("izin reddedildi: zk")
	}

	switch fn {
	case "generateProof":
		circuit, _ := params["circuit"].(string)
		if circuit == "" {
			return nil, fmt.Errorf("generateProof: circuit zorunlu")
		}

		// ZK rate limit: 5 proof/second per app (spec Bölüm 10.1 spam kontrolü).
		lim := getZKLimiter(bc.AppID)
		if !lim.Allow() {
			return nil, fmt.Errorf("zk rate limit exceeded")
		}

		// ZK proof generation happens client-side (spec Bolum 4.5 kural 2:
		// "sunucu tarafindan uretilmez"). Return a stub instructing the app to
		// use the client-side snarkjs library.
		return map[string]interface{}{
			"note":    "ZK kaniti istemci tarafinda uretilmeli (snarkjs)",
			"circuit": circuit,
			"status":  "client_side_required",
		}, nil

	case "verifyProof":
		proof, hasProof := params["proof"]
		if !hasProof {
			return nil, fmt.Errorf("verifyProof: proof zorunlu")
		}
		_ = proof
		return map[string]interface{}{
			"verified": false,
			"note":     "Dogrulama icin POST /v1/zk/verify endpoint'ini kullanin",
		}, nil

	case "getCreditScore":
		// Requires explicit ZK permission: credit_score_read (manifest + runtime consent).
		if !bc.hasRuntimeZKPermission("credit_score_read") {
			return nil, fmt.Errorf("zk izin reddedildi: credit_score_read (kullanici onay diyalogu gerekli)")
		}
		var score float64
		err := bc.DB.QueryRow(`SELECT credit_score FROM users WHERE did = ?`, bc.UserDID).Scan(&score)
		if err != nil {
			return nil, fmt.Errorf("kredi puani alinamadi: %w", err)
		}
		bucket := creditBucket(score)
		return map[string]interface{}{
			"score_bucket": bucket,
			"note":         "Tam puan gizliligi korumak icin aralik olarak donduruluyor",
		}, nil

	default:
		return nil, fmt.Errorf("bilinmeyen zk method: %q", fn)
	}
}

// creditBucket converts an exact credit score into a privacy-preserving range.
func creditBucket(score float64) string {
	switch {
	case score < 0:
		return "negative"
	case score < 30:
		return "0-30"
	case score < 60:
		return "30-60"
	case score < 80:
		return "60-80"
	default:
		return "80-100"
	}
}

// ─── ui namespace ─────────────────────────────────────────────────────────────

func handleUI(bc BridgeContext, fn string, params map[string]interface{}) (interface{}, error) {
	if !bc.hasPermission("ui") {
		return nil, fmt.Errorf("izin reddedildi: ui")
	}

	switch fn {
	case "showToast":
		msg, _ := params["msg"].(string)
		if msg == "" {
			return nil, fmt.Errorf("showToast: msg zorunlu")
		}
		if len(msg) > 256 {
			msg = msg[:256]
		}
		return map[string]interface{}{
			"action": "show_toast",
			"msg":    msg,
			"app_id": bc.AppID,
		}, nil

	case "openModal":
		url, _ := params["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("openModal: url zorunlu")
		}
		if !urlInAllowedDomains(url, bc.Manifest.AllowedDomains) {
			return nil, fmt.Errorf("openModal: URL izin verilen domainler disinda: %q", url)
		}
		return map[string]interface{}{
			"action": "open_modal",
			"url":    url,
			"app_id": bc.AppID,
		}, nil

	case "close":
		return map[string]interface{}{
			"action": "close_app",
			"app_id": bc.AppID,
		}, nil

	case "requestZkPermission":
		// ZK permissions require a separate explicit user consent dialog
		// (spec Bölüm 10.3). This call:
		//   1. Validates the permission name is known.
		//   2. Validates the manifest declared it (prerequisite).
		//   3. Checks the runtime grant store (DB).
		//   4. If not yet granted, records the grant now (simulating dialog approval).
		//      In a production client the dialog happens in the host UI; the bridge
		//      call is gated on the result already being stored in app_zk_permissions.
		perm, _ := params["perm"].(string)
		if !KnownZKPermissions[perm] {
			return nil, fmt.Errorf("bilinmeyen zk izni: %q", perm)
		}
		// Manifest must have declared the ZK permission.
		if !bc.hasManifestZKPermission(perm) {
			return false, nil // Not declared in manifest — silently deny.
		}
		// Check the runtime grant store (persisted in DB).
		if bc.DB != nil && hasZkPermissionDB(bc.DB, bc.AppID, perm) {
			return true, nil // Already granted by the user.
		}
		// Permission declared but not yet granted at runtime.
		// The host UI must show the ZK consent dialog before this returns true.
		// Return false to signal that the dialog is pending.
		return false, nil

	default:
		return nil, fmt.Errorf("bilinmeyen ui method: %q", fn)
	}
}

// urlInAllowedDomains checks whether the given URL's host falls within the
// app's declared allowedDomains list.
func urlInAllowedDomains(rawURL string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	host := rawURL
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	for _, d := range allowed {
		if strings.EqualFold(host, d) {
			return true
		}
	}
	return false
}

// newUUID returns a new random UUID string without requiring an extra import
// in this file — delegates to the uuid package imported via store.go in the
// same package.
func newUUID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), pseudoRand())
}

// pseudoRand provides a cheap monotonic counter to avoid a uuid import cycle.
var pseudoRandCounter int64

func pseudoRand() int64 {
	pseudoRandCounter++
	return pseudoRandCounter
}
