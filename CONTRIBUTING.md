# Contributing to Obscura

Thanks for your interest. Read this before submitting code.

## Before you start

1. Read `CLAUDE.md` (project overview, hard rules, current state)
2. Read the spec: `docs/spec/obscura_spec_v3.txt`
3. Check `docs/adr/README.md` for accepted deviations
4. Look at open issues — claim one before starting

## Setup

```bash
# Clone
git clone https://github.com/yarlikhan/obscura.git
cd obscura

# Install hooks
bash scripts/install-hooks.sh

# Backend
cd backend && go mod download && cd ..

# Frontend
cd frontend && npm ci && cd ..

# Mobile
cd mobile && npm ci && cd ..

# Desktop
cd desktop && npm ci && cd ..

# Full stack via Docker
cp .env.example .env  # then edit
make docker-up
```

## Code style

- **Go**: `gofmt`, `go vet` clean. No `panic()` outside `main()`. Errors wrapped.
- **TypeScript**: strict mode, no `any`, prefer `unknown`. Vitest for tests.
- **Rust**: `cargo fmt`, `cargo clippy -- -D warnings`. No `unwrap()` outside tests.
- **Circom**: `pragma circom 2.1.6;`. Use circomlib primitives. Test in `circuits/test/`.

## Commit messages

Format:
```
<type>(<scope>): <summary>

<body>

<footer>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `perf`, `security`

Examples:
```
feat(backend): add MLS group create endpoint
fix(frontend): correct keys/{did} URL (was bundle/)
docs(adr): document Aztec choice for zk-Rollup
```

## Pull requests

1. Branch from `main`: `git checkout -b feat/short-name`
2. Write tests
3. Run locally: `make test` should pass
4. Update CLAUDE.md feature matrix if feature status changes
5. Open PR using template
6. Address review

## Hard rules (PR will be rejected)

- No CGO in Go code
- No SQL string concatenation
- No secrets in commits (run `gitleaks` locally)
- No custom crypto primitives
- No spec deviation without ADR
- No skipping tests to make CI green
- No `--no-verify` on commits

## Need help?

- Question about Obscura design: open a Discussion
- Spec ambiguity: open an Issue with `spec-question` label
- Bug: open an Issue with `bug` label and reproduction steps
- Security vulnerability: see SECURITY.md (do not open Issue)
