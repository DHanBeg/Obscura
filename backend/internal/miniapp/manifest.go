// Package miniapp implements Obscura's mini app engine (spec Bölüm 10) — FAZ 2.
//
// This package is a SKELETON: process isolation, manifest validation, the
// permission model and the registry are implemented here. Full ZK-API bridge
// integration (identity/messaging/wallet/zk/ui namespaces) is FAZ 3 work.
package miniapp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// HardLimitMemoryMB is the absolute per-app memory ceiling (spec Bölüm 10.1).
const HardLimitMemoryMB = 128

// HardLimitCPUPercent is the absolute per-app CPU ceiling — 10% of one core.
const HardLimitCPUPercent = 10

// KnownPermissions is the closed set of API-bridge namespaces a manifest may
// request (spec Bölüm 10.2 + 10.3). Anything outside this set is rejected.
var KnownPermissions = map[string]bool{
	"identity":  true,
	"messaging": true,
	"wallet":    true,
	"zk":        true,
	"ui":        true,
	"location":  true,
}

// KnownZKPermissions is the closed set of ZK-scoped permissions. These require
// a separate explicit user consent dialog (spec Bölüm 10.3).
var KnownZKPermissions = map[string]bool{
	"credit_score_read": true,
	"identity_proof":    true,
}

// Tier constants mirror models.User.Tier (1=Bronze .. 5=Diamond).
const (
	TierBronze   = 1
	TierSilver   = 2
	TierGold     = 3
	TierPlatinum = 4
	TierDiamond  = 5
)

// Manifest is the parsed, validated form of a mini app manifest.json
// (spec Bölüm 10.3).
type Manifest struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	DeveloperDID   string   `json:"developer_did"`
	Permissions    []string `json:"permissions"`
	AllowedDomains []string `json:"allowedDomains"`
	ZKPermissions  []string `json:"zkPermissions"`
	MaxMemory      string   `json:"maxMemory"`
	MaxCPU         string   `json:"maxCpu"`
	MinTier        int      `json:"minTier"`
}

var (
	semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	didRe    = regexp.MustCompile(`^did:obs:[A-Za-z0-9]+$`)
	domainRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

// ParseManifest decodes and fully validates a manifest JSON blob. It rejects
// unknown permissions, malformed fields and resource requests that exceed the
// hard per-app limits (128MB memory, 10% CPU).
func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest parse hatası: %w", err)
	}

	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("manifest: name zorunlu")
	}
	if len(m.Name) > 64 {
		return nil, fmt.Errorf("manifest: name 64 karakteri aşamaz")
	}
	if !semverRe.MatchString(m.Version) {
		return nil, fmt.Errorf("manifest: version semver (X.Y.Z) formatında olmalı, alındı: %q", m.Version)
	}
	if !didRe.MatchString(m.DeveloperDID) {
		return nil, fmt.Errorf("manifest: developer_did geçersiz (did:obs:... bekleniyor)")
	}

	// Permissions — closed set.
	seenPerm := map[string]bool{}
	for _, p := range m.Permissions {
		if !KnownPermissions[p] {
			return nil, fmt.Errorf("manifest: bilinmeyen izin %q", p)
		}
		if seenPerm[p] {
			return nil, fmt.Errorf("manifest: tekrar eden izin %q", p)
		}
		seenPerm[p] = true
	}

	// ZK permissions — closed set, separate consent later.
	seenZK := map[string]bool{}
	for _, z := range m.ZKPermissions {
		if !KnownZKPermissions[z] {
			return nil, fmt.Errorf("manifest: bilinmeyen zkPermission %q", z)
		}
		if seenZK[z] {
			return nil, fmt.Errorf("manifest: tekrar eden zkPermission %q", z)
		}
		seenZK[z] = true
	}

	// Allowed domains — basic hostname sanity (no scheme, no path).
	for _, d := range m.AllowedDomains {
		if d == "" || !domainRe.MatchString(d) || strings.Contains(d, "..") {
			return nil, fmt.Errorf("manifest: geçersiz allowedDomain %q", d)
		}
	}

	// Resource limits.
	memMB, err := parseMemoryMB(m.MaxMemory)
	if err != nil {
		return nil, fmt.Errorf("manifest: maxMemory: %w", err)
	}
	if memMB > HardLimitMemoryMB {
		return nil, fmt.Errorf("manifest: maxMemory %dMB sınırı (%dMB) aşıyor", memMB, HardLimitMemoryMB)
	}

	cpuPct, err := parseCPUPercent(m.MaxCPU)
	if err != nil {
		return nil, fmt.Errorf("manifest: maxCpu: %w", err)
	}
	if cpuPct > HardLimitCPUPercent {
		return nil, fmt.Errorf("manifest: maxCpu %d%% sınırı (%d%%) aşıyor", cpuPct, HardLimitCPUPercent)
	}

	// minTier — must be a valid tier and at least Silver (Bronze cannot run).
	if m.MinTier < TierSilver || m.MinTier > TierDiamond {
		return nil, fmt.Errorf("manifest: minTier %d geçersiz (2-5 arası olmalı; Bronz mini app çalıştıramaz)", m.MinTier)
	}

	return &m, nil
}

// MemoryMB returns the validated memory budget in megabytes.
func (m *Manifest) MemoryMB() int {
	mb, _ := parseMemoryMB(m.MaxMemory)
	return mb
}

// CPUPercent returns the validated CPU budget as a whole-number percentage.
func (m *Manifest) CPUPercent() int {
	pct, _ := parseCPUPercent(m.MaxCPU)
	return pct
}

// ValidatePermissions checks that every permission in requested is declared in
// the manifest. It also enforces that ZK-scoped permissions (zkPermissions) in
// the manifest were explicitly listed — they are NOT automatically granted by
// the base permission set (spec Bölüm 10.3: "ZK permission requires explicit
// user consent via a separate dialog").
//
// Returns a non-nil error identifying the first undeclared permission.
func ValidatePermissions(m *Manifest, requested []string) error {
	declared := make(map[string]bool, len(m.Permissions))
	for _, p := range m.Permissions {
		declared[p] = true
	}
	for _, r := range requested {
		if !declared[r] {
			return fmt.Errorf("izin reddedildi: %q manifest'te beyan edilmedi", r)
		}
	}
	return nil
}

// parseMemoryMB accepts values like "128MB", "64 MB", "131072KB".
func parseMemoryMB(s string) (int, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	if t == "" {
		return 0, fmt.Errorf("boş")
	}
	var mult int
	switch {
	case strings.HasSuffix(t, "MB"):
		mult, t = 1, strings.TrimSpace(strings.TrimSuffix(t, "MB"))
	case strings.HasSuffix(t, "KB"):
		mult, t = 0, strings.TrimSpace(strings.TrimSuffix(t, "KB"))
	default:
		return 0, fmt.Errorf("MB veya KB birimi bekleniyor, alındı: %q", s)
	}
	n, err := strconv.Atoi(t)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("geçersiz sayı: %q", s)
	}
	if mult == 0 { // KB
		return n / 1024, nil
	}
	return n, nil
}

// parseCPUPercent accepts values like "10%", "5 %".
func parseCPUPercent(s string) (int, error) {
	t := strings.TrimSpace(s)
	if !strings.HasSuffix(t, "%") {
		return 0, fmt.Errorf("%% birimi bekleniyor, alındı: %q", s)
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "%"))
	n, err := strconv.Atoi(t)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("geçersiz sayı: %q", s)
	}
	return n, nil
}
