#!/usr/bin/env bash
# Gates: all checks must pass before marking an issue done.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

echo "=== Gate 1: build ==="
go build ./...

echo "=== Gate 2: tests ==="
go test ./...

echo "=== Gate 3: secret scan ==="
gitleaks detect --config .gitleaks.toml --source . --no-git --no-banner -l error 2>/dev/null

echo "=== All gates passed ==="
