package miniapp

import (
	"context"
	"strings"
	"testing"
	"time"
)

const validManifest = `{
	"name": "Harita Servisi",
	"version": "1.0.0",
	"developer_did": "did:obs:abc123",
	"permissions": ["messaging", "location"],
	"allowedDomains": ["maps.example.com"],
	"zkPermissions": ["credit_score_read"],
	"maxMemory": "128MB",
	"maxCpu": "10%",
	"minTier": 3
}`

func TestParseManifest_Valid(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("beklenmedik hata: %v", err)
	}
	if m.Name != "Harita Servisi" {
		t.Errorf("name = %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("version = %q", m.Version)
	}
	if m.MemoryMB() != 128 {
		t.Errorf("MemoryMB = %d, beklenen 128", m.MemoryMB())
	}
	if m.CPUPercent() != 10 {
		t.Errorf("CPUPercent = %d, beklenen 10", m.CPUPercent())
	}
	if m.MinTier != 3 {
		t.Errorf("MinTier = %d", m.MinTier)
	}
	if len(m.Permissions) != 2 || len(m.ZKPermissions) != 1 {
		t.Errorf("izinler yanlış parse edildi: %+v / %+v", m.Permissions, m.ZKPermissions)
	}
}

func TestParseManifest_RejectsUnknownPermission(t *testing.T) {
	raw := `{
		"name": "Kötü App", "version": "1.0.0", "developer_did": "did:obs:x",
		"permissions": ["messaging", "filesystem"],
		"maxMemory": "64MB", "maxCpu": "5%", "minTier": 2
	}`
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatal("bilinmeyen izin kabul edildi, hata bekleniyordu")
	}
	if !strings.Contains(err.Error(), "filesystem") {
		t.Errorf("hata mesajı bilinmeyen izni belirtmiyor: %v", err)
	}
}

func TestParseManifest_RejectsExcessiveMemory(t *testing.T) {
	raw := `{
		"name": "Aç Gözlü", "version": "1.0.0", "developer_did": "did:obs:x",
		"permissions": ["ui"],
		"maxMemory": "256MB", "maxCpu": "10%", "minTier": 2
	}`
	_, err := ParseManifest([]byte(raw))
	if err == nil {
		t.Fatal("256MB kabul edildi, 128MB sınırı uygulanmalıydı")
	}
	if !strings.Contains(err.Error(), "maxMemory") {
		t.Errorf("hata mesajı maxMemory'i belirtmiyor: %v", err)
	}
}

func TestParseManifest_RejectsBadTier(t *testing.T) {
	// minTier 1 (Bronze) cannot run mini apps per spec Bölüm 10.4.
	bronze := `{
		"name": "Bronz App", "version": "1.0.0", "developer_did": "did:obs:x",
		"permissions": ["ui"], "maxMemory": "64MB", "maxCpu": "5%", "minTier": 1
	}`
	if _, err := ParseManifest([]byte(bronze)); err == nil {
		t.Error("minTier=1 (Bronz) kabul edildi, reddedilmeliydi")
	}

	// Out-of-range tier.
	high := `{
		"name": "Geçersiz", "version": "1.0.0", "developer_did": "did:obs:x",
		"permissions": ["ui"], "maxMemory": "64MB", "maxCpu": "5%", "minTier": 9
	}`
	if _, err := ParseManifest([]byte(high)); err == nil {
		t.Error("minTier=9 kabul edildi, reddedilmeliydi")
	}
}

func TestParseManifest_RejectsExcessiveCPU(t *testing.T) {
	raw := `{
		"name": "CPU Canavarı", "version": "1.0.0", "developer_did": "did:obs:x",
		"permissions": ["ui"], "maxMemory": "64MB", "maxCpu": "50%", "minTier": 2
	}`
	if _, err := ParseManifest([]byte(raw)); err == nil {
		t.Fatal("maxCpu=50%% kabul edildi, 10%% sınırı uygulanmalıydı")
	}
}

func TestParseManifest_RejectsUnknownField(t *testing.T) {
	raw := `{
		"name": "X", "version": "1.0.0", "developer_did": "did:obs:x",
		"permissions": ["ui"], "maxMemory": "64MB", "maxCpu": "5%", "minTier": 2,
		"backdoor": true
	}`
	if _, err := ParseManifest([]byte(raw)); err == nil {
		t.Fatal("bilinmeyen alan 'backdoor' kabul edildi")
	}
}

// RunApp must degrade gracefully when the optional deno runtime is absent.
func TestRunApp_GracefulWithoutDeno(t *testing.T) {
	if denoAvailable() {
		t.Skip("deno yüklü — bu test yalnızca runtime yokken anlamlı")
	}
	_, err := RunApp(context.Background(), "console.log('hi')", SandboxConfig{
		MemoryMB: 128, Timeout: 2 * time.Second,
	})
	if err != ErrDenoNotInstalled {
		t.Fatalf("deno yokken ErrDenoNotInstalled bekleniyordu, alındı: %v", err)
	}
}

// When deno IS installed, a trivial program should run inside the sandbox.
func TestRunApp_WithDeno(t *testing.T) {
	if !denoAvailable() {
		t.Skip("deno PATH'te yok — sandbox çalışma testi atlanıyor (kur: https://deno.land)")
	}
	out, err := RunApp(context.Background(), "console.log('obscura')", SandboxConfig{
		MemoryMB: 64, Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("sandbox çalışma hatası: %v", err)
	}
	if !strings.Contains(out, "obscura") {
		t.Errorf("beklenen çıktı yok, alındı: %q", out)
	}
}
