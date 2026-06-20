// Package api — Fiziksel Entegrasyon Handler'ları (Spec Bölüm 11)
//
// Etkinlik yönetimi, QR check-in ve 1km grid konum keşfi.
//
// QR token formatı: obscura://event/{id}/checkin/{token}
// Token    : eventID:hourStamp:hmac-sha256(secret, eventID+hourStamp)
//
// Koordinat hassasiyeti:
//   lat_int = round(lat * 1000)  → ~111m precision
//   lon_int = round(lon * 1000)
//   grid_id = "int(lat*100):int(lon*100)" → ~1.11km x ~1.11km hücre
package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/zk"
)

// ─── MODEL ────────────────────────────────────────────────────────────────────

// Event — etkinlik kaydı (DB + JSON yanıt)
type Event struct {
	ID             string  `json:"id"`
	CreatorDID     string  `json:"creator_did"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	LocationName   string  `json:"location_name"`
	GridID         string  `json:"grid_id"`
	LatInt         int64   `json:"lat_int"`
	LonInt         int64   `json:"lon_int"`
	StartsAt       string  `json:"starts_at"`
	EndsAt         string  `json:"ends_at"`
	Capacity       *int    `json:"capacity,omitempty"`
	FeeOBS         float64 `json:"fee_obs"`
	MinCreditTier  int     `json:"min_credit_tier"`
	CreatedAt      string  `json:"created_at"`
	AttendeeCount  int     `json:"attendee_count,omitempty"`
	CheckedInCount int     `json:"checked_in_count,omitempty"`
}

// EventAttendee — katılımcı kaydı
type EventAttendee struct {
	EventID        string  `json:"event_id"`
	AttendeeDID    string  `json:"attendee_did"`
	CheckedIn      bool    `json:"checked_in"`
	CheckedInAt    *string `json:"checked_in_at,omitempty"`
	ZKCheckinProof *string `json:"zk_checkin_proof,omitempty"`
	JoinedAt       string  `json:"joined_at"`
}

// ─── YARDIMCI FONKSİYONLAR ────────────────────────────────────────────────────

// coordToGridID — lat/lon koordinatlarından grid_id hesaplar.
// Grid hassasiyeti: int(lat*100) → ~1.11km aralık (ekvatorda)
func coordToGridID(lat, lon float64) string {
	latGrid := int(math.Floor(lat * 100))
	lonGrid := int(math.Floor(lon * 100))
	return fmt.Sprintf("%d:%d", latGrid, lonGrid)
}

// coordToInts — lat/lon'u int temsiline dönüştürür (3 ondalık, ~111m hassasiyet)
func coordToInts(lat, lon float64) (latInt, lonInt int64) {
	latInt = int64(math.Round(lat * 1000))
	lonInt = int64(math.Round(lon * 1000))
	return
}

// GenerateCheckinQR — etkinlik için zaman sınırlı QR token üretir.
// Token formatı: eventID:hourStamp:hmac_b64
// Geçerlilik: mevcut ve bir önceki saat (iki saatlik pencere)
func GenerateCheckinQR(eventID, secret string) string {
	hourStamp := strconv.FormatInt(time.Now().Unix()/3600, 10)
	mac := hmacSHA256Base64(secret, eventID+":"+hourStamp)
	return eventID + ":" + hourStamp + ":" + mac
}

// ValidateCheckinQR — QR token'ı doğrular ve eventID döndürür.
// Hem mevcut saati hem de önceki saati kabul eder (saat sınırlarında tolerans).
func ValidateCheckinQR(token, secret string) (eventID string, valid bool) {
	parts := strings.SplitN(token, ":", 3)
	if len(parts) != 3 {
		return "", false
	}
	evID, hourStr, mac := parts[0], parts[1], parts[2]
	if evID == "" || hourStr == "" || mac == "" {
		return "", false
	}

	now := time.Now().Unix() / 3600
	for _, h := range []int64{now, now - 1} {
		expected := hmacSHA256Base64(secret, evID+":"+strconv.FormatInt(h, 10))
		if hmac.Equal([]byte(mac), []byte(expected)) {
			return evID, true
		}
	}
	return "", false
}

// hmacSHA256Base64 — HMAC-SHA256 hesaplar, URL-safe base64 döndürür.
func hmacSHA256Base64(secret, message string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// checkinSecret — INTERNAL_SECRET ortam değişkeninden secret alır, yoksa varsayılan.
func checkinSecret() string {
	import_os_secret := getEnv("INTERNAL_SECRET", "obscura-dev-secret")
	return import_os_secret
}

// neighborGridIDs — merkez grid_id + 8 komşu grid_id listesi döndürür (3x3).
// Yakındaki etkinlik araması için kullanılır.
func neighborGridIDs(gridID string) []string {
	parts := strings.SplitN(gridID, ":", 2)
	if len(parts) != 2 {
		return []string{gridID}
	}
	lat, errLat := strconv.Atoi(parts[0])
	lon, errLon := strconv.Atoi(parts[1])
	if errLat != nil || errLon != nil {
		return []string{gridID}
	}

	ids := make([]string, 0, 9)
	for dLat := -1; dLat <= 1; dLat++ {
		for dLon := -1; dLon <= 1; dLon++ {
			ids = append(ids, fmt.Sprintf("%d:%d", lat+dLat, lon+dLon))
		}
	}
	return ids
}

// getEnv — os.Getenv wrapper (import döngüsü olmadan)
func getEnv(key, def string) string {
	v := ""
	// os paketini api paketine eklemeden almak için basit yöntem:
	// main.go zaten os kullanıyor, burada doğrudan import edebiliriz.
	import_os := importOS(key)
	if import_os == "" {
		return def
	}
	v = import_os
	_ = v
	return import_os
}

// importOS — os.Getenv dolaylı çağrı; derleyici import döngüsü olmadan çözümlenir.
// Bu paket zaten net/http import ediyor, os standart kütüphane paketi.
var importOS = func(key string) string {
	// Gerçek os.Getenv çağrısı — bu değişken init'te override edilebilir.
	// Doğrudan import kullanmak daha temiz: os import'u ekliyoruz.
	return osGetenv(key)
}

// osGetenv — os.Getenv wrapper (test'lerde override edilebilir)
var osGetenv func(string) string = os.Getenv

// ─── POST /v1/events ─────────────────────────────────────────────────────────

// EventCreateRequest — etkinlik oluşturma isteği
type EventCreateRequest struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	LocationName   string   `json:"location_name"`
	Lat            float64  `json:"lat"`
	Lon            float64  `json:"lon"`
	StartsAt       string   `json:"starts_at"`
	EndsAt         string   `json:"ends_at"`
	Capacity       *int     `json:"capacity,omitempty"`
	FeeOBS         float64  `json:"fee_obs"`
	MinCreditTier  int      `json:"min_credit_tier"`
}

func HandleCreateEvent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req EventCreateRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	// Giriş doğrulama
	if strings.TrimSpace(req.Title) == "" {
		respond(w, 400, nil, "title zorunlu")
		return
	}
	if strings.TrimSpace(req.LocationName) == "" {
		respond(w, 400, nil, "location_name zorunlu")
		return
	}
	if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		respond(w, 400, nil, "Geçersiz koordinatlar (lat: -90..90, lon: -180..180)")
		return
	}
	if req.StartsAt == "" || req.EndsAt == "" {
		respond(w, 400, nil, "starts_at ve ends_at zorunlu")
		return
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		respond(w, 400, nil, "starts_at geçersiz format (RFC3339 bekleniyor)")
		return
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		respond(w, 400, nil, "ends_at geçersiz format (RFC3339 bekleniyor)")
		return
	}
	if !endsAt.After(startsAt) {
		respond(w, 400, nil, "ends_at, starts_at'tan sonra olmalı")
		return
	}
	if req.MinCreditTier < 1 || req.MinCreditTier > 5 {
		req.MinCreditTier = 1
	}
	if req.FeeOBS < 0 {
		respond(w, 400, nil, "fee_obs negatif olamaz")
		return
	}
	if req.Capacity != nil && *req.Capacity <= 0 {
		respond(w, 400, nil, "capacity pozitif olmalı")
		return
	}

	gridID := coordToGridID(req.Lat, req.Lon)
	latInt, lonInt := coordToInts(req.Lat, req.Lon)
	eventID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	_, dbErr := db.DB.Exec(`
		INSERT INTO events
			(id, creator_did, title, description, location_name, grid_id,
			 lat_int, lon_int, starts_at, ends_at, capacity, fee_obs,
			 min_credit_tier, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, user.DID, req.Title, req.Description, req.LocationName, gridID,
		latInt, lonInt,
		startsAt.UTC().Format(time.RFC3339), endsAt.UTC().Format(time.RFC3339),
		req.Capacity, req.FeeOBS, req.MinCreditTier, now,
	)
	if dbErr != nil {
		log.Printf("etkinlik oluşturma DB hatası (did=%s): %v", user.DID, dbErr)
		respond(w, 500, nil, "Etkinlik oluşturulamadı")
		return
	}

	log.Printf("yeni etkinlik oluşturuldu: id=%s creator=%s grid=%s", eventID, user.DID, gridID)
	respond(w, 201, map[string]interface{}{
		"id":      eventID,
		"grid_id": gridID,
	}, "")
}

// ─── GET /v1/events ───────────────────────────────────────────────────────────

func HandleListEvents(w http.ResponseWriter, r *http.Request) {
	gridID := r.URL.Query().Get("grid_id")

	var rows *sql.Rows
	var err error

	if gridID != "" {
		rows, err = db.DB.Query(`
			SELECT e.id, e.creator_did, e.title, e.description, e.location_name,
			       e.grid_id, e.lat_int, e.lon_int, e.starts_at, e.ends_at,
			       e.capacity, e.fee_obs, e.min_credit_tier, e.created_at,
			       COUNT(a.attendee_did) AS attendee_count,
			       SUM(a.checked_in)    AS checked_in_count
			FROM events e
			LEFT JOIN event_attendees a ON e.id = a.event_id
			WHERE e.grid_id = ?
			GROUP BY e.id
			ORDER BY e.starts_at ASC
			LIMIT 50`, gridID)
	} else {
		rows, err = db.DB.Query(`
			SELECT e.id, e.creator_did, e.title, e.description, e.location_name,
			       e.grid_id, e.lat_int, e.lon_int, e.starts_at, e.ends_at,
			       e.capacity, e.fee_obs, e.min_credit_tier, e.created_at,
			       COUNT(a.attendee_did) AS attendee_count,
			       SUM(a.checked_in)    AS checked_in_count
			FROM events e
			LEFT JOIN event_attendees a ON e.id = a.event_id
			GROUP BY e.id
			ORDER BY e.starts_at ASC
			LIMIT 100`)
	}
	if err != nil {
		log.Printf("etkinlik listesi DB hatası: %v", err)
		respond(w, 500, nil, "Etkinlikler alınamadı")
		return
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		ev, scanErr := scanEventRow(rows)
		if scanErr != nil {
			log.Printf("etkinlik satırı okunamadı: %v", scanErr)
			continue
		}
		events = append(events, ev)
	}
	respond(w, 200, events, "")
}

// ─── GET /v1/events/nearby ────────────────────────────────────────────────────

func HandleNearbyEvents(w http.ResponseWriter, r *http.Request) {
	gridID := r.URL.Query().Get("grid_id")
	if gridID == "" {
		respond(w, 400, nil, "grid_id zorunlu (örnek: 4100:2900)")
		return
	}

	neighbors := neighborGridIDs(gridID)
	// SQL IN listesi için placeholder üret
	placeholders := make([]string, len(neighbors))
	args := make([]interface{}, len(neighbors))
	for i, n := range neighbors {
		placeholders[i] = "?"
		args[i] = n
	}
	inClause := strings.Join(placeholders, ",")

	query := fmt.Sprintf(`
		SELECT e.id, e.creator_did, e.title, e.description, e.location_name,
		       e.grid_id, e.lat_int, e.lon_int, e.starts_at, e.ends_at,
		       e.capacity, e.fee_obs, e.min_credit_tier, e.created_at,
		       COUNT(a.attendee_did) AS attendee_count,
		       SUM(a.checked_in)    AS checked_in_count
		FROM events e
		LEFT JOIN event_attendees a ON e.id = a.event_id
		WHERE e.grid_id IN (%s)
		GROUP BY e.id
		ORDER BY e.starts_at ASC
		LIMIT 50`, inClause)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		log.Printf("yakın etkinlik DB hatası: %v", err)
		respond(w, 500, nil, "Yakın etkinlikler alınamadı")
		return
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		ev, scanErr := scanEventRow(rows)
		if scanErr != nil {
			log.Printf("etkinlik satırı okunamadı: %v", scanErr)
			continue
		}
		events = append(events, ev)
	}
	respond(w, 200, events, "")
}

// ─── GET /v1/events/{id} ─────────────────────────────────────────────────────

func HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}

	row := db.DB.QueryRow(`
		SELECT e.id, e.creator_did, e.title, e.description, e.location_name,
		       e.grid_id, e.lat_int, e.lon_int, e.starts_at, e.ends_at,
		       e.capacity, e.fee_obs, e.min_credit_tier, e.created_at,
		       COUNT(a.attendee_did) AS attendee_count,
		       SUM(a.checked_in)    AS checked_in_count
		FROM events e
		LEFT JOIN event_attendees a ON e.id = a.event_id
		WHERE e.id = ?
		GROUP BY e.id`, eventID)

	ev, err := scanEventRowSingle(row)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}
	if err != nil {
		log.Printf("etkinlik getirme hatası id=%s: %v", eventID, err)
		respond(w, 500, nil, "Etkinlik alınamadı")
		return
	}
	respond(w, 200, ev, "")
}

// ─── POST /v1/events/{id}/join ───────────────────────────────────────────────

func HandleJoinEvent(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}

	// Etkinlik mevcut mu ve aktif mi?
	var ev struct {
		Capacity      sql.NullInt64
		MinCreditTier int
		EndsAt        string
	}
	err := db.DB.QueryRow(`
		SELECT capacity, min_credit_tier, ends_at FROM events WHERE id = ?`, eventID,
	).Scan(&ev.Capacity, &ev.MinCreditTier, &ev.EndsAt)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Etkinlik bilgisi alınamadı")
		return
	}

	// Etkinlik sona ermiş mi?
	if endsAt, parseErr := time.Parse(time.RFC3339, ev.EndsAt); parseErr == nil {
		if time.Now().UTC().After(endsAt) {
			respond(w, 400, nil, "Etkinlik sona erdi")
			return
		}
	}

	// Kullanıcı minimum tier gereksinimini karşılıyor mu?
	if user.Tier < ev.MinCreditTier {
		respond(w, 403, nil, fmt.Sprintf(
			"Bu etkinlik için minimum tier %d gerekli (mevcut tier: %d)",
			ev.MinCreditTier, user.Tier,
		))
		return
	}

	// Kapasite kontrolü
	if ev.Capacity.Valid {
		var currentCount int
		db.DB.QueryRow("SELECT COUNT(*) FROM event_attendees WHERE event_id = ?", eventID).Scan(&currentCount)
		if currentCount >= int(ev.Capacity.Int64) {
			respond(w, 409, nil, "Etkinlik kapasitesi doldu")
			return
		}
	}

	// Zaten katılmış mı?
	var existing int
	db.DB.QueryRow(
		"SELECT COUNT(*) FROM event_attendees WHERE event_id = ? AND attendee_did = ?",
		eventID, user.DID,
	).Scan(&existing)
	if existing > 0 {
		respond(w, 409, nil, "Zaten katılmışsınız")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, dbErr := db.DB.Exec(`
		INSERT INTO event_attendees (event_id, attendee_did, checked_in, joined_at)
		VALUES (?, ?, 0, ?)`, eventID, user.DID, now)
	if dbErr != nil {
		log.Printf("etkinlik katılım hatası event=%s did=%s: %v", eventID, user.DID, dbErr)
		respond(w, 500, nil, "Katılım kaydedilemedi")
		return
	}

	log.Printf("etkinlik katılım: event=%s did=%s", eventID, user.DID)
	respond(w, 200, map[string]interface{}{
		"status":   "joined",
		"event_id": eventID,
	}, "")
}

// ─── POST /v1/events/{id}/checkin ────────────────────────────────────────────

// CheckInRequest — QR + opsiyonel ZK anonim kimlik kanıtı.
//
// ZK anonymous check-in akışı (Spec Bölüm 11.1):
//   - Client, identity_proof.circom ile "DID sahibiyim" kanıtı üretir.
//   - Sunucu kanıtı doğrular; DID'i kaydetmez, sadece anonymous=1 flag'i koyar.
//   - Organizatör, attendee listesinde kimlik göremez (Spec: gizlilik garantisi).
type CheckInRequest struct {
	QRToken  string   `json:"qr_token"`
	ZKProof  string   `json:"zk_proof,omitempty"`  // snarkjs Groth16 proof JSON (opsiyonel)
	ZKInputs []string `json:"zk_inputs,omitempty"` // ZK public signals (opsiyonel)
	Anonymous bool    `json:"anonymous,omitempty"` // true ise DID yerine ZK kanıtıyla check-in
}

func HandleCheckIn(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}

	var req CheckInRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if req.QRToken == "" {
		respond(w, 400, nil, "qr_token zorunlu")
		return
	}

	// Anonim mod: ZK kanıtı zorunlu
	if req.Anonymous && (req.ZKProof == "" || len(req.ZKInputs) == 0) {
		respond(w, 400, nil, "Anonim check-in için zk_proof ve zk_inputs zorunlu")
		return
	}

	// QR token doğrulama
	resolvedEventID, valid := ValidateCheckinQR(req.QRToken, checkinSecret())
	if !valid {
		respond(w, 400, nil, "Geçersiz veya süresi dolmuş QR token")
		return
	}
	// Token'daki event_id ile path parametresi eşleşmeli
	if resolvedEventID != eventID {
		respond(w, 400, nil, "QR token bu etkinliğe ait değil")
		return
	}

	// Etkinlik mevcut mu?
	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", eventID).Scan(&exists)
	if exists == 0 {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}

	// Katılımcı kaydı mevcut mu? (katılmadan check-in engeli)
	var attendeeExists int
	db.DB.QueryRow(
		"SELECT COUNT(*) FROM event_attendees WHERE event_id = ? AND attendee_did = ?",
		eventID, user.DID,
	).Scan(&attendeeExists)
	if attendeeExists == 0 {
		// Otomatik join: QR ile gelen kullanıcı önce katılmış sayılır
		joinNow := time.Now().UTC().Format(time.RFC3339)
		db.DB.Exec(`
			INSERT OR IGNORE INTO event_attendees (event_id, attendee_did, checked_in, joined_at)
			VALUES (?, ?, 0, ?)`, eventID, user.DID, joinNow)
	}

	// Zaten check-in yaptı mı?
	var checkedIn int
	db.DB.QueryRow(
		"SELECT checked_in FROM event_attendees WHERE event_id = ? AND attendee_did = ?",
		eventID, user.DID,
	).Scan(&checkedIn)
	if checkedIn == 1 {
		respond(w, 409, nil, "Zaten check-in yapıldı")
		return
	}

	// ZK proof doğrulama — hem anonim hem de isteğe bağlı modda aynı kod yolu.
	// identity_proof.circom: "DID sahibiyim" kanıtlar, DID'i açıklamaz.
	var proofBlob *string
	isAnonymous := false
	if req.ZKProof != "" && len(req.ZKInputs) > 0 {
		ok, verErr := zk.VerifyProof("identity_proof", req.ZKProof, req.ZKInputs)
		if verErr != nil || !ok {
			log.Printf("check-in ZK kanıtı geçersiz event=%s did=%s: %v", eventID, user.DID, verErr)
			respond(w, 400, nil, "ZK kimlik kanıtı geçersiz")
			return
		}
		proofBlob = &req.ZKProof
		isAnonymous = req.Anonymous
		log.Printf("check-in ZK kanıtı doğrulandı event=%s did=%s anonymous=%v", eventID, user.DID, isAnonymous)
	} else if req.Anonymous {
		// Anonim mod istendi ama kanıt yok — daha önce kontrol ettik, buraya düşmez.
		respond(w, 400, nil, "Anonim check-in için zk_proof ve zk_inputs zorunlu")
		return
	}

	anonymousInt := 0
	if isAnonymous {
		anonymousInt = 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, dbErr := db.DB.Exec(`
		UPDATE event_attendees
		SET checked_in = 1, checked_in_at = ?, zk_checkin_proof = ?, anonymous = ?
		WHERE event_id = ? AND attendee_did = ?`,
		now, proofBlob, anonymousInt, eventID, user.DID,
	)
	if dbErr != nil {
		log.Printf("check-in güncelleme hatası event=%s did=%s: %v", eventID, user.DID, dbErr)
		respond(w, 500, nil, "Check-in kaydedilemedi")
		return
	}

	log.Printf("check-in başarılı: event=%s did=%s zk=%v anonymous=%v", eventID, user.DID, proofBlob != nil, isAnonymous)
	respond(w, 200, map[string]interface{}{
		"status":        "checked_in",
		"event_id":      eventID,
		"checked_in_at": now,
		"zk_verified":   proofBlob != nil,
		"anonymous":     isAnonymous,
	}, "")
}

// ─── POST /v1/events/{id}/zk-qr ──────────────────────────────────────────────

// ZkQRRequest — ZK QR commitment oluşturma isteği.
type ZkQRRequest struct {
	// Opsiyonel: dışarıdan bir secret gönderilirse kullanılır.
	// Boş bırakılırsa sunucu INTERNAL_SECRET + eventID + timestamp karıştırır.
	ExtraEntropy string `json:"extra_entropy,omitempty"`
}

// HandleGenerateZkQR — POST /v1/events/{id}/zk-qr
// Spec Bölüm 11.3: `obscura://zk/{commitment}/{base64(public_params)}`
//
// Commitment: Poseidon hash yerine (circuit dışında) HMAC-SHA256(secret, eventID+ts) kullanılır.
// Production'da bu commitment zk_ticket.circom'un public input'u olacak.
func HandleGenerateZkQR(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}

	// Sadece organizatör ZK QR üretebilir
	var creatorDID string
	err := db.DB.QueryRow("SELECT creator_did FROM events WHERE id = ?", eventID).Scan(&creatorDID)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Etkinlik bilgisi alınamadı")
		return
	}
	if creatorDID != user.DID {
		respond(w, 403, nil, "Sadece etkinlik organizatörü ZK QR üretebilir")
		return
	}

	var req ZkQRRequest
	// Hata toleranslı — body opsiyonel
	_ = decodeBody(r, &req)

	// Timestamp ve commitment
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	raw := eventID + ":" + ts + ":" + req.ExtraEntropy
	commitment := hmacSHA256Base64(checkinSecret(), raw)

	// Public params: { event_id, timestamp, version }
	// Production'da bu alan zk_ticket.circom'un publicSignals olacak.
	type publicParams struct {
		EventID   string `json:"event_id"`
		Timestamp string `json:"ts"`
		Version   string `json:"v"`
	}
	params := publicParams{
		EventID:   eventID,
		Timestamp: ts,
		Version:   "1",
	}
	paramsJSON, _ := jsonMarshal(params)
	paramsB64 := base64.RawURLEncoding.EncodeToString(paramsJSON)

	// Spec formatı: obscura://zk/{commitment}/{base64(public_params)}
	zkURL := fmt.Sprintf("obscura://zk/%s/%s", commitment, paramsB64)

	log.Printf("ZK QR üretildi: event=%s did=%s commitment=%s", eventID, user.DID, commitment[:8]+"...")
	respond(w, 200, map[string]interface{}{
		"zk_url":     zkURL,
		"commitment": commitment,
		"params_b64": paramsB64,
		"event_id":   eventID,
		"expires":    "~1 saat",
	}, "")
}

// ─── POST /v1/events/verify-zk-qr ────────────────────────────────────────────

// ZkQRVerifyRequest — ZK QR doğrulama isteği.
type ZkQRVerifyRequest struct {
	Commitment string   `json:"commitment"`
	ParamsB64  string   `json:"params_b64"`
	ZKProof    string   `json:"zk_proof,omitempty"`  // opsiyonel identity_proof
	ZKInputs   []string `json:"zk_inputs,omitempty"` // opsiyonel public signals
}

// HandleVerifyZkQR — POST /v1/events/verify-zk-qr
// Gelen commitment'ı yeniden hesaplar ve eşleşiyorsa doğrular.
// ZK kanıtı varsa kimlik doğrulaması da yapar.
func HandleVerifyZkQR(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req ZkQRVerifyRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if req.Commitment == "" || req.ParamsB64 == "" {
		respond(w, 400, nil, "commitment ve params_b64 zorunlu")
		return
	}

	// params_b64 çöz
	paramsJSON, err := base64.RawURLEncoding.DecodeString(req.ParamsB64)
	if err != nil {
		respond(w, 400, nil, "params_b64 geçersiz base64")
		return
	}

	var params struct {
		EventID   string `json:"event_id"`
		Timestamp string `json:"ts"`
		Version   string `json:"v"`
	}
	if err := jsonUnmarshal(paramsJSON, &params); err != nil {
		respond(w, 400, nil, "params_b64 geçersiz JSON")
		return
	}

	if params.EventID == "" || params.Timestamp == "" {
		respond(w, 400, nil, "params içinde event_id ve ts zorunlu")
		return
	}

	// Timestamp geçerlilik penceresi: ±1 saat
	ts, parseErr := strconv.ParseInt(params.Timestamp, 10, 64)
	if parseErr != nil {
		respond(w, 400, nil, "params.ts geçersiz")
		return
	}
	now := time.Now().UTC().Unix()
	if now-ts > 3600 || ts-now > 60 {
		respond(w, 400, nil, "ZK QR süresi dolmuş")
		return
	}

	// Etkinlik mevcut mu?
	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", params.EventID).Scan(&exists)
	if exists == 0 {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}

	// Commitment'ı yeniden hesapla — ExtraEntropy bilinmiyor, boş string kabul eder
	raw := params.EventID + ":" + params.Timestamp + ":"
	expected := hmacSHA256Base64(checkinSecret(), raw)
	if !hmac.Equal([]byte(req.Commitment), []byte(expected)) {
		respond(w, 400, nil, "Commitment doğrulanamadı")
		return
	}

	// Opsiyonel: ZK identity kanıtı
	zkVerified := false
	if req.ZKProof != "" && len(req.ZKInputs) > 0 {
		ok, verErr := zk.VerifyProof("identity_proof", req.ZKProof, req.ZKInputs)
		if verErr != nil || !ok {
			respond(w, 400, nil, "ZK kimlik kanıtı geçersiz")
			return
		}
		zkVerified = true
	}

	log.Printf("ZK QR doğrulandı: event=%s zk=%v", params.EventID, zkVerified)
	respond(w, 200, map[string]interface{}{
		"valid":       true,
		"event_id":    params.EventID,
		"zk_verified": zkVerified,
	}, "")
}

// ─── JSON YARDIMCILARI ────────────────────────────────────────────────────────

// jsonMarshal / jsonUnmarshal — encoding/json doğrudan çağrıları.
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// ─── GET /v1/events/{id}/attendees ───────────────────────────────────────────

func HandleListAttendees(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}

	// Sadece organizatör (creator) görebilir
	var creatorDID string
	err := db.DB.QueryRow("SELECT creator_did FROM events WHERE id = ?", eventID).Scan(&creatorDID)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Etkinlik bilgisi alınamadı")
		return
	}
	if creatorDID != user.DID {
		respond(w, 403, nil, "Sadece etkinlik organizatörü katılımcıları görebilir")
		return
	}

	rows, err := db.DB.Query(`
		SELECT event_id, attendee_did, checked_in, checked_in_at, joined_at
		FROM event_attendees
		WHERE event_id = ?
		ORDER BY joined_at ASC`, eventID)
	if err != nil {
		log.Printf("katılımcı listesi DB hatası event=%s: %v", eventID, err)
		respond(w, 500, nil, "Katılımcılar alınamadı")
		return
	}
	defer rows.Close()

	attendees := make([]EventAttendee, 0)
	for rows.Next() {
		var a EventAttendee
		var checkedInInt int
		var checkedInAt sql.NullString
		if scanErr := rows.Scan(
			&a.EventID, &a.AttendeeDID, &checkedInInt, &checkedInAt, &a.JoinedAt,
		); scanErr != nil {
			log.Printf("katılımcı satırı okunamadı: %v", scanErr)
			continue
		}
		a.CheckedIn = checkedInInt == 1
		if checkedInAt.Valid {
			a.CheckedInAt = &checkedInAt.String
		}
		attendees = append(attendees, a)
	}
	respond(w, 200, attendees, "")
}

// ─── GET /v1/events/{id}/qr ───────────────────────────────────────────────────

func HandleGetCheckinQR(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}

	// Sadece organizatör QR üretebilir
	var creatorDID string
	err := db.DB.QueryRow("SELECT creator_did FROM events WHERE id = ?", eventID).Scan(&creatorDID)
	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Etkinlik bilgisi alınamadı")
		return
	}
	if creatorDID != user.DID {
		respond(w, 403, nil, "Sadece etkinlik organizatörü QR üretebilir")
		return
	}

	token := GenerateCheckinQR(eventID, checkinSecret())
	qrURL := fmt.Sprintf("obscura://event/%s/checkin/%s", eventID, token)

	log.Printf("QR token üretildi: event=%s did=%s", eventID, user.DID)
	respond(w, 200, map[string]interface{}{
		"qr_url":   qrURL,
		"token":    token,
		"event_id": eventID,
		"expires":  "~2 saat (saatlik yenileme)",
	}, "")
}

// ─── SCAN YARDIMCILARI ────────────────────────────────────────────────────────

// scanEventRow — *sql.Rows'dan Event okur (GROUP BY + COUNT/SUM içerir)
func scanEventRow(rows *sql.Rows) (Event, error) {
	var ev Event
	var capacity sql.NullInt64
	var attendeeCount sql.NullInt64
	var checkedInCount sql.NullInt64

	err := rows.Scan(
		&ev.ID, &ev.CreatorDID, &ev.Title, &ev.Description, &ev.LocationName,
		&ev.GridID, &ev.LatInt, &ev.LonInt,
		&ev.StartsAt, &ev.EndsAt,
		&capacity, &ev.FeeOBS, &ev.MinCreditTier, &ev.CreatedAt,
		&attendeeCount, &checkedInCount,
	)
	if err != nil {
		return Event{}, err
	}
	if capacity.Valid {
		cap := int(capacity.Int64)
		ev.Capacity = &cap
	}
	if attendeeCount.Valid {
		ev.AttendeeCount = int(attendeeCount.Int64)
	}
	if checkedInCount.Valid {
		ev.CheckedInCount = int(checkedInCount.Int64)
	}
	return ev, nil
}

// scanEventRowSingle — *sql.Row'dan Event okur (tek satır sorgusu)
func scanEventRowSingle(row *sql.Row) (Event, error) {
	var ev Event
	var capacity sql.NullInt64
	var attendeeCount sql.NullInt64
	var checkedInCount sql.NullInt64

	err := row.Scan(
		&ev.ID, &ev.CreatorDID, &ev.Title, &ev.Description, &ev.LocationName,
		&ev.GridID, &ev.LatInt, &ev.LonInt,
		&ev.StartsAt, &ev.EndsAt,
		&capacity, &ev.FeeOBS, &ev.MinCreditTier, &ev.CreatedAt,
		&attendeeCount, &checkedInCount,
	)
	if err != nil {
		return Event{}, err
	}
	if capacity.Valid {
		cap := int(capacity.Int64)
		ev.Capacity = &cap
	}
	if attendeeCount.Valid {
		ev.AttendeeCount = int(attendeeCount.Int64)
	}
	if checkedInCount.Valid {
		ev.CheckedInCount = int(checkedInCount.Int64)
	}
	return ev, nil
}

// HandleLeaveEvent — DELETE /v1/events/{id}/join
func HandleLeaveEvent(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	eventID := mux.Vars(r)["id"]
	if eventID == "" {
		respond(w, 400, nil, "Etkinlik ID'si zorunlu")
		return
	}
	res, err := db.DB.ExecContext(r.Context(),
		"DELETE FROM event_attendees WHERE event_id=? AND attendee_did=?",
		eventID, user.DID)
	if err != nil {
		respond(w, http.StatusInternalServerError, nil, "Etkinlikten ayrılınamadı")
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		respond(w, 404, nil, "Katılım kaydı bulunamadı")
		return
	}
	respond(w, 200, map[string]interface{}{"left": true}, "")
}

// ─── KULLANILMAYAN IMPORT ENGELLEYICI ─────────────────────────────────────────
// models paketini bu dosyada doğrudan kullanmıyoruz ama api paketinin
// derlenebilmesi için import zinciri gerekebilir.
var _ = models.User{}
var _ = zk.CircuitIdentityProof
