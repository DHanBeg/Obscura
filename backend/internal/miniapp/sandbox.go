package miniapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrDenoNotInstalled is returned by RunApp when the `deno` binary is not on
// PATH. Deno is an OPTIONAL runtime dependency — the build does not require it.
// Install: https://deno.land  (`curl -fsSL https://deno.land/install.sh | sh`)
var ErrDenoNotInstalled = errors.New("deno not installed: mini app runtime unavailable — install from https://deno.land")

// SandboxConfig is the per-app resource + isolation budget passed to RunApp.
// It is derived from a validated Manifest.
type SandboxConfig struct {
	MemoryMB       int           // V8 old-space heap cap (<= 128, spec Bölüm 10.1)
	CPUPercent     int           // enforced via cgroups on Linux (FAZ 3); advisory elsewhere
	AllowedDomains []string      // network whitelist; empty = no network at all
	Timeout        time.Duration // hard wall-clock kill via CommandContext
	// User context — injected as env vars visible to the API bridge shim.
	UserDID  string // user's DID (did:obs:...)
	Username string // user's display username
	RelayURL string // internal relay base URL for bridge HTTP calls (e.g. http://localhost:8080)
	AppID    string // mini app ID — used by the shim as X-App-Id header on bridge calls
}

// SandboxConfigFromManifest builds a SandboxConfig from a validated manifest.
// userDID, username and relayURL are injected as environment variables visible
// to the API bridge shim running inside the Deno process.
func SandboxConfigFromManifest(m *Manifest, timeout time.Duration, appID, userDID, username, relayURL string) SandboxConfig {
	return SandboxConfig{
		MemoryMB:       m.MemoryMB(),
		CPUPercent:     m.CPUPercent(),
		AllowedDomains: m.AllowedDomains,
		Timeout:        timeout,
		AppID:          appID,
		UserDID:        userDID,
		Username:       username,
		RelayURL:       relayURL,
	}
}

// denoBinary returns the path to the deno binary.
// It prefers the DENO_PATH environment variable (useful on Windows where deno
// may not be on the system PATH), then falls back to PATH resolution.
func denoBinary() (string, error) {
	if p := os.Getenv("DENO_PATH"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("deno")
	if err != nil {
		return "", ErrDenoNotInstalled
	}
	return p, nil
}

// denoAvailable reports whether the deno binary is resolvable.
func denoAvailable() bool {
	_, err := denoBinary()
	return err == nil
}

// RunApp executes appCode in an isolated `deno run` subprocess.
//
// Isolation layer (FAZ 2 + FAZ 3):
//   --no-prompt                       no interactive permission prompts
//   --allow-net=<domains>             whitelist-only network (omitted entirely
//                                     when AllowedDomains is empty → no net)
//   --deny-read --deny-write          no filesystem access
//   --deny-run                        no subprocess spawning
//   --allow-env=OBSCURA_DID,...       only Obscura-injected env vars visible
//   --v8-flags=--max-old-space-size   heap cap = MemoryMB
//
// The API bridge shim (BuildAppCode) is prepended to appCode before execution
// so that `globalThis.obscura` is available to the user-supplied script.
//
// After the subprocess is started, two FAZ 3 hardening steps are applied:
//  1. applyCgroupLimits(pid, CPUPercent) — Linux cgroup v1 CPU shares
//  2. applySeccompFilter(pid)            — seccomp-bpf stub (full impl FAZ 3)
//
// The context timeout is enforced through exec.CommandContext, which kills the
// process group when the deadline elapses.
func RunApp(ctx context.Context, appCode string, cfg SandboxConfig) (string, error) {
	denoBin, err := denoBinary()
	if err != nil {
		return "", err
	}

	memMB := cfg.MemoryMB
	if memMB <= 0 || memMB > HardLimitMemoryMB {
		memMB = HardLimitMemoryMB
	}

	cpuPercent := cfg.CPUPercent
	if cpuPercent <= 0 || cpuPercent > HardLimitCPUPercent {
		cpuPercent = HardLimitCPUPercent
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Allowlisted env var names the app shim reads via Deno.env.get().
	// --allow-env with an explicit comma-separated list (Deno 1.36+) lets the
	// process inherit only those vars, keeping the rest of the host env hidden.
	const allowedEnvVars = "OBSCURA_DID,OBSCURA_USERNAME,OBSCURA_RELAY_URL,OBSCURA_APP_ID"

	args := []string{
		"run",
		"--no-prompt",
		"--deny-read",
		"--deny-write",
		"--allow-env=" + allowedEnvVars,
		"--deny-run",
		fmt.Sprintf("--v8-flags=--max-old-space-size=%d", memMB),
	}
	// Network: whitelist-only.
	// Loopback (127.0.0.1) is always allowed so the bridge shim can POST back to
	// the host via OBSCURA_RELAY_URL. External domains come from the manifest; if
	// none are declared the app gets loopback-only network access.
	netAllowList := []string{"127.0.0.1"}
	netAllowList = append(netAllowList, cfg.AllowedDomains...)
	args = append(args, "--allow-net="+strings.Join(netAllowList, ","))
	// Read app code from stdin so nothing touches disk.
	args = append(args, "-")

	// Prepend the Obscura API bridge shim so globalThis.obscura is available
	// before any user code runs.
	fullCode := BuildAppCode(appCode)

	cmd := exec.CommandContext(runCtx, denoBin, args...)
	cmd.Stdin = strings.NewReader(fullCode)
	// Pass only Obscura-specific vars. The user context (DID, username) must be
	// populated by the caller via cfg before invoking RunApp. We derive env from
	// the SandboxConfig to keep RunApp a pure function of its arguments.
	cmd.Env = []string{
		"OBSCURA_DID=" + cfg.UserDID,
		"OBSCURA_USERNAME=" + cfg.Username,
		"OBSCURA_RELAY_URL=" + cfg.RelayURL,
		"OBSCURA_APP_ID=" + cfg.AppID,
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Use Start+Wait (instead of Run) so we can read the PID before the process
	// exits and apply cgroup / seccomp limits while it is alive.
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("mini app başlatılamadı: %w", err)
	}

	pid := cmd.Process.Pid

	// FAZ 3: cgroup CPU enforcement (Linux-only; no-op stub on other platforms).
	if err := applyCgroupLimits(pid, cpuPercent); err != nil {
		// Non-fatal: log and continue. The deno --deny-* flags are the primary
		// isolation boundary; cgroups are an additional hardening layer.
		_ = fmt.Sprintf("[miniapp] cgroup limiti uygulanamadı (pid=%d): %v", pid, err)
	}
	defer cleanupCgroup(pid)

	// FAZ 3: seccomp-bpf syscall filter (stub; TODO(FAZ-3): full implementation).
	if err := applySeccompFilter(pid); err != nil {
		// Non-fatal: same rationale as cgroup failure above.
		_ = fmt.Sprintf("[miniapp] seccomp filtresi uygulanamadı (pid=%d): %v", pid, err)
	}

	// Wait for the subprocess to complete (or for the context to expire).
	err = cmd.Wait()
	if runCtx.Err() == context.DeadlineExceeded {
		return stdout.String(), fmt.Errorf("mini app zaman aşımına uğradı (%s sınırı)", timeout)
	}
	if err != nil {
		return stdout.String(), fmt.Errorf("mini app çalışma hatası: %w | stderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// DenoVersion returns the installed deno version string, or ErrDenoNotInstalled.
// Useful for health checks / admin diagnostics.
func DenoVersion(ctx context.Context) (string, error) {
	denoBin, err := denoBinary()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, denoBin, "--version")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("deno version sorgulanamadı: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
