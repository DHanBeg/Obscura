//go:build linux

package miniapp

// seccomp_linux.go — seccomp-bpf syscall filter stub for mini app sandbox.
//
// TODO(FAZ-3): seccomp-bpf tam implementasyon
//
// Full implementation requires either:
//   a) github.com/seccomp/libseccomp-golang (CGO — incompatible with CGO_ENABLED=0), or
//   b) a hand-written BPF program passed via syscall.RawSyscall(syscall.SYS_PRCTL, ...)
//      + a landlock/seccomp shim launcher process wrapping deno.
//
// FAZ 3 approach: a minimal C shim (obscura-sandbox-launcher) sets seccomp on
// itself before exec'ing deno, keeping the Go binary CGO-free. The shim is
// compiled into the Docker image at /usr/local/bin/obscura-launcher.
//
// Allowed syscall whitelist (minimum viable Deno runtime):
//   read, write, readv, writev        — I/O
//   exit, exit_group                   — termination
//   futex                              — sync primitives
//   clock_gettime, clock_nanosleep     — timekeeping
//   nanosleep                          — sleep
//   mmap, munmap, mprotect, brk        — memory management
//   epoll_wait, epoll_ctl, epoll_create1 — event loop
//   recvfrom, sendto, recvmsg, sendmsg — network (whitelist-enforced by deno flags)
//   getpid, gettid, tgkill            — thread management
//   rt_sigaction, rt_sigreturn        — signals

import "log"

// applySeccompFilter attaches a seccomp-bpf syscall filter to the process
// identified by pid. In FAZ 3 this is a stub — it logs the intent and returns
// nil so that RunApp continues. The actual BPF filter installation will be
// handled by the obscura-sandbox-launcher shim (FAZ 3 deliverable).
//
// TODO(FAZ-3): seccomp-bpf tam implementasyon — replace stub with shim exec.
func applySeccompFilter(pid int) error {
	// TODO(FAZ-3): seccomp-bpf tam implementasyon
	// Stub: log the intent; no actual prctl call here (CGO_ENABLED=0 constraint).
	log.Printf("[miniapp][seccomp] stub: pid=%d için seccomp-bpf filtresi uygulanacak (FAZ-3 tamamlandığında aktif)", pid)
	return nil
}
