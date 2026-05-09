# ADR 0001: Use modernc.org/sqlite over CGO SQLite

Date: 2026-04-30
Status: Accepted
Decider: project lead
Spec ref: Bölüm 14.1 (Backend dilleri)

## Context

Backend needs SQLite. Standard `mattn/go-sqlite3` requires CGO, which:
- Requires C toolchain on every build host (gcc, sqlite-dev)
- Complicates Docker images (must use Debian/Alpine with build tools)
- Blocks easy cross-compilation (Linux → Windows binary needs MinGW)
- Larger final binaries
- Slower builds

`modernc.org/sqlite` is a pure-Go transpilation of SQLite. Same API, no CGO.

## Options considered

### Option A: mattn/go-sqlite3 (CGO)
- Pros: Mature, fastest performance, well-documented
- Cons: CGO required, Docker pain, cross-compile pain
- Effort: S (current)
- Risk: Low

### Option B: modernc.org/sqlite (pure Go)
- Pros: No CGO, easy Docker (`CGO_ENABLED=0`), trivial cross-compile, smaller binaries
- Cons: ~30% slower than CGO version, larger build cache (1GB+), some pragmas unsupported
- Effort: S (drop-in replacement)
- Risk: Low

### Option C: Embedded BadgerDB / BoltDB
- Pros: Pure Go, fast for KV
- Cons: Not relational, would require schema redesign, no SQL queries
- Effort: XL
- Risk: High

## Decision

**Option B**: `modernc.org/sqlite`. Drop CGO requirement entirely.

## Rationale

The 30% performance hit is acceptable for our workload (we're not running 100k QPS per node). The operational simplicity wins decisively:
- One Dockerfile pattern across all platforms
- No "works on my machine" build issues
- Smaller, statically linked binaries (~15MB vs ~25MB)
- Faster CI builds (no apt install for build deps)

If we ever hit a real perf wall, swap is one import line.

## Consequences

- **Positive**: Zero CGO complexity, reproducible builds, easy cross-compile, smaller images
- **Negative**: ~30% query throughput hit (acceptable)
- **Neutral**: Driver name is `"sqlite"` not `"sqlite3"` — minor code change
- **Tech debt**: None

## Implementation plan

1. Replace `_ "github.com/mattn/go-sqlite3"` with `_ "modernc.org/sqlite"` in `backend/internal/db/database.go`
2. Change `sql.Open("sqlite3", ...)` to `sql.Open("sqlite", ...)`
3. Update `Dockerfile`: remove `apk add gcc sqlite-dev`, add `ENV CGO_ENABLED=0`
4. Remove `CGO_ENABLED=1` build flag from any `go build` invocations
5. Verify: `go test -race ./...` passes; image builds successfully

## References

- modernc.org/sqlite: https://pkg.go.dev/modernc.org/sqlite
- Performance comparison: https://github.com/cvilsmeier/go-sqlite-bench
