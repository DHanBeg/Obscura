//go:build linux

package miniapp

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// cgroupRoot is the v1 CPU cgroup mount point. On systems with only cgroup v2
// this path will not exist; applyCgroupLimits detects that and logs a warning
// instead of returning a hard error so the app still runs.
const cgroupRoot = "/sys/fs/cgroup/cpu"

// applyCgroupLimits creates a CPU cgroup for the deno subprocess identified by
// pid and writes cpu.shares to limit it to cpuPercent% of one core.
//
// cpu.shares mapping (v1 semantics):
//   100% of one core = 1024 shares
//   cpuPercent%      = round(cpuPercent * 1024 / 100)
//
// The cgroup directory is deno-{pid} under cgroupRoot. Tasks are added by
// writing the pid to the cgroup's tasks file.
//
// This function is a FAZ 3 implementation. It requires cgroupv1 mounted at
// /sys/fs/cgroup/cpu. If the mount is absent (cgroupv2-only host) the function
// emits a warning log and returns nil so RunApp continues without enforcement.
func applyCgroupLimits(pid int, cpuPercent int) error {
	cgroupDir := filepath.Join(cgroupRoot, fmt.Sprintf("deno-%d", pid))

	// Detect whether the v1 CPU hierarchy is available.
	if _, err := os.Stat(cgroupRoot); os.IsNotExist(err) {
		log.Printf("[miniapp] cgroup enforcement atlandı: %s bulunamadı (cgroupv2-only host?), pid=%d", cgroupRoot, pid)
		return nil
	}

	// Create the per-app cgroup directory.
	if err := os.MkdirAll(cgroupDir, 0o755); err != nil {
		return fmt.Errorf("cgroup dizini olusturulamadi %s: %w", cgroupDir, err)
	}

	// Compute shares: 1024 = 100% of one core.
	shares := cpuPercent * 1024 / 100
	if shares < 2 {
		shares = 2 // kernel minimum
	}

	sharesPath := filepath.Join(cgroupDir, "cpu.shares")
	if err := os.WriteFile(sharesPath, []byte(fmt.Sprintf("%d", shares)), 0o644); err != nil {
		return fmt.Errorf("cpu.shares yazilamadi (%s): %w", sharesPath, err)
	}

	// Assign the subprocess to this cgroup.
	tasksPath := filepath.Join(cgroupDir, "tasks")
	if err := os.WriteFile(tasksPath, []byte(fmt.Sprintf("%d", pid)), 0o644); err != nil {
		return fmt.Errorf("cgroup tasks yazilamadi (pid=%d): %w", pid, err)
	}

	log.Printf("[miniapp] cgroup uygulandı: pid=%d, shares=%d (%d%%), dizin=%s", pid, shares, cpuPercent, cgroupDir)
	return nil
}

// cleanupCgroup removes the per-app cgroup directory after the subprocess exits.
// Errors are logged but not propagated — cleanup is best-effort.
func cleanupCgroup(pid int) {
	cgroupDir := filepath.Join(cgroupRoot, fmt.Sprintf("deno-%d", pid))
	if err := os.Remove(cgroupDir); err != nil && !os.IsNotExist(err) {
		log.Printf("[miniapp] cgroup temizleme hatası (pid=%d): %v", pid, err)
	}
}
