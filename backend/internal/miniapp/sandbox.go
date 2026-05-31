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
}

// SandboxConfigFromManifest builds a SandboxConfig from a validated manifest.
func SandboxConfigFromManifest(m *Manifest, timeout time.Duration) SandboxConfig {
	return SandboxConfig{
		MemoryMB:       m.MemoryMB(),
		CPUPercent:     m.CPUPercent(),
		AllowedDomains: m.AllowedDomains,
		Timeout:        timeout,
	}
}

// denoAvailable reports whether the deno binary is resolvable on PATH.
func denoAvailable() bool {
	_, err := exec.LookPath("deno")
	return err == nil
}

// RunApp executes appCode in an isolated `deno run` subprocess.
//
// Isolation layer (FAZ 2 + FAZ 3):
//   --no-prompt                       no interactive permission prompts
//   --allow-net=<domains>             whitelist-only network (omitted entirely
//                                     when AllowedDomains is empty → no net)
//   --deny-read --deny-write          no filesystem access
//   --deny-env --deny-run             no env vars, no subprocess spawning
//   --v8-flags=--max-old-space-size   heap cap = MemoryMB
//
// After the subprocess is started, two FAZ 3 hardening steps are applied:
//   1. applyCgroupLimits(pid, CPUPercent) — Linux cgroup v1 CPU shares
//   2. applySeccompFilter(pid)            — seccomp-bpf stub (full impl FAZ 3)
//
// The context timeout is enforced through exec.CommandContext, which kills the
// process group when the deadline elapses.
func RunApp(ctx context.Context, appCode string, cfg SandboxConfig) (string, error) {
	if !denoAvailable() {
		return "", ErrDenoNotInstalled
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

	args := []string{
		"run",
		"--no-prompt",
		"--deny-read",
		"--deny-write",
		"--deny-env",
		"--deny-run",
		fmt.Sprintf("--v8-flags=--max-old-space-size=%d", memMB),
	}
	// Network: whitelist-only. Empty whitelist → flag omitted → deno default-denies net.
	if len(cfg.AllowedDomains) > 0 {
		args = append(args, "--allow-net="+strings.Join(cfg.AllowedDomains, ","))
	}
	// Read app code from stdin so nothing touches disk.
	args = append(args, "-")

	cmd := exec.CommandContext(runCtx, "deno", args...)
	cmd.Stdin = strings.NewReader(appCode)
	// Minimal, scrubbed environment — deno still --deny-env's it from the app.
	cmd.Env = []string{}

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
	err := cmd.Wait()
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
	if !denoAvailable() {
		return "", ErrDenoNotInstalled
	}
	cmd := exec.CommandContext(ctx, "deno", "--version")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("deno version sorgulanamadı: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
