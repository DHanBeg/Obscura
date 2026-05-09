#!/bin/sh
# Install Obscura git hooks
# Usage: bash scripts/install-hooks.sh

set -e
ROOT="$(git rev-parse --show-toplevel)"

mkdir -p "$ROOT/.git/hooks"
cp "$ROOT/scripts/hooks/pre-commit" "$ROOT/.git/hooks/pre-commit"
chmod +x "$ROOT/.git/hooks/pre-commit"

echo "✓ Hooks installed:"
echo "  - pre-commit (gitleaks, gofmt, go vet, tsc, cargo fmt)"
echo ""
echo "Optional installs (recommended):"
echo "  - gitleaks: brew install gitleaks  OR  go install github.com/gitleaks/gitleaks/v8@latest"
echo "  - golangci-lint: https://golangci-lint.run/usage/install/"
