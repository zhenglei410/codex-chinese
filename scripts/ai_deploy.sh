#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-v2.0.7}"
export VERSION

if ! command -v go >/dev/null 2>&1; then
  echo "go-not-installed: install Go 1.21+ and retry" >&2
  exit 1
fi

bash ./build.sh
go vet ./...
go test ./...

PRELUDE_TEST="skipped-no-node"
if command -v node >/dev/null 2>&1; then
  node ./test/prelude.test.mjs
  PRELUDE_TEST="pass"
fi

count="$(find ./dist -maxdepth 1 -type f ! -name SHA256SUMS | wc -l | tr -d ' ')"
if [ "$count" -ne 8 ]; then
  echo "artifact-count: $count (expected 8)" >&2
  exit 1
fi

cat > deploy-report.json <<JSON
{
  "ok": true,
  "version": "$VERSION",
  "artifact_count": 8,
  "prelude_test": "$PRELUDE_TEST",
  "next": "Run the CLI, then require -verify exit code 0."
}
JSON

cat deploy-report.json
