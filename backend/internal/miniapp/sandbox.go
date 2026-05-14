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
	CPUPercent     int           // advisory; enforced via cgroups in FAZ 3
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
// Isolation layer (FAZ 2):
//   --no-prompt                       no interactive permission prompts
//   --allow-net=<domains>             whitelist-only network (omitted entirely
//                                     when AllowedDomains is empty → no net)
//   --deny-read --deny-write          no filesystem access
//   --deny-env --deny-run             no env vars, no subprocess spawning
//   --v8-flags=--max-old-space-size   heap cap = MemoryMB
//
// The context timeout is enforced through exec.CommandContext, which kills the
// process group when the deadline elapses.
//
// TODO(FAZ 3): seccomp-bpf syscall filtering. seccomp is Linux-only and needs
// a thin launcher (libseccomp / a prctl shim) wrapping the deno process. The
// --deny-* permission flags above are the FAZ 2 isolation boundary; seccomp is
// FAZ 3 kernel-level hardening. CPUPercent is likewise advisory in FAZ 2 and
// becomes a real limit once apps run inside a cgroup.
func RunApp(ctx context.Context, appCode string, cfg SandboxConfig) (string, error) {
	if !denoAvailable() {
		return "", ErrDenoNotInstalled
	}

	memMB := cfg.MemoryMB
	if memMB <= 0 || memMB > HardLimitMemoryMB {
		memMB = HardLimitMemoryMB
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

	err := cmd.Run()
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
