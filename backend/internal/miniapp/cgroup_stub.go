//go:build !linux

package miniapp

import "log"

// applyCgroupLimits is a no-op on non-Linux platforms. CPU enforcement via
// cgroups is Linux-only (spec Bölüm 10.1, FAZ 3). On Windows and macOS the
// deno --v8-flags=--max-old-space-size flag still enforces the memory limit;
// CPU scheduling is left to the OS.
func applyCgroupLimits(pid int, cpuPercent int) error {
	log.Printf("[miniapp] cgroup enforcement Linux-only — no-op (pid=%d, cpuPercent=%d)", pid, cpuPercent)
	return nil
}

// cleanupCgroup is a no-op on non-Linux platforms.
func cleanupCgroup(_ int) {}
