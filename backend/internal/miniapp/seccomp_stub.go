//go:build !linux

package miniapp

import "log"

// applySeccompFilter is a no-op on non-Linux platforms.
// seccomp-bpf is a Linux kernel feature (spec Bölüm 10, FAZ 3).
//
// TODO(FAZ-3): seccomp-bpf tam implementasyon — Linux only, shim launcher.
func applySeccompFilter(pid int) error {
	log.Printf("[miniapp][seccomp] seccomp-bpf Linux-only — no-op (pid=%d)", pid)
	return nil
}
