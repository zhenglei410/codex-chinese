#!/usr/bin/env bash
# 一键部署并自证。供 AI 代理直接运行。
# 用法：VERSION=v1.0.0 bash scripts/ai_deploy.sh
# 输出：标准输出的 JSON 报告 + 写入 deploy-report.json
set -uo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-dev}"
REPORT="deploy-report.json"

fail() {
  local reason="$1"
  printf '{"ok":false,"version":"%s","reason":"%s"}\n' "$VERSION" "$reason"
  printf '{"ok":false,"version":"%s","reason":"%s"}\n' "$VERSION" "$reason" > "$REPORT"
  exit 1
}

# ---------- 1. 检查 Go ----------
command -v go >/dev/null 2>&1 || fail "go-not-installed: see docs/AI_DEPLOY.md#2"
GO_VER="$(go version | awk '{print $3}')"
GO_MAJOR_MINOR="$(echo "$GO_VER" | sed -E 's/^go([0-9]+)\.([0-9]+).*/\1.\2/')"
case "$GO_MAJOR_MINOR" in
  1.2[1-9]|1.[3-9][0-9]|[2-9].*) ;;
  *) fail "go-too-old: $GO_VER (need >= 1.21)" ;;
esac

# ---------- 2. 构建 ----------
if ! VERSION="$VERSION" bash build.sh >/tmp/codex-chinese-build.log 2>&1; then
  tail -20 /tmp/codex-chinese-build.log >&2
  fail "build-failed: see /tmp/codex-chinese-build.log"
fi

# ---------- 3. 静态检查 ----------
go vet ./... >/tmp/codex-chinese-vet.log 2>&1
VET_EXIT=$?
[ "$VET_EXIT" -eq 0 ] || { cat /tmp/codex-chinese-vet.log >&2; fail "vet-failed"; }

# ---------- 3.5 注入脚本回归测试 ----------
# prelude 是内联在 main.go 里的 JS，go vet 看不到它的逻辑错误。
# 有 node 就必须跑；没有 node 记为 skipped（不阻断纯 Go 环境的构建）。
PRELUDE_TEST="skipped-no-node"
if command -v node >/dev/null 2>&1; then
  if node test/prelude.test.mjs >/tmp/codex-chinese-prelude.log 2>&1; then
    PRELUDE_TEST="pass"
  else
    tail -20 /tmp/codex-chinese-prelude.log >&2
    fail "prelude-test-failed: see /tmp/codex-chinese-prelude.log"
  fi
else
  printf 'WARN: 未检测到 node，跳过 test/prelude.test.mjs（建议安装 node >= 18）\n' >&2
fi

# ---------- 4. 校验产物 ----------
size_of() {
  if stat -c%s "$1" >/dev/null 2>&1; then stat -c%s "$1"; else stat -f%z "$1"; fi
}

ARTIFACTS=(
  "windows/amd64|dist/codex-chinese-windows-amd64.exe"
  "windows/arm64|dist/codex-chinese-windows-arm64.exe"
  "darwin/amd64|dist/codex-chinese-darwin-amd64"
  "darwin/arm64|dist/codex-chinese-darwin-arm64"
  "linux/amd64|dist/codex-chinese-linux-amd64"
  "linux/arm64|dist/codex-chinese-linux-arm64"
  "windows/amd64|dist/codex-chinese-tray-windows-amd64.exe"
  "windows/arm64|dist/codex-chinese-tray-windows-arm64.exe"
)
JSON_ITEMS=""
COUNT=0
for item in "${ARTIFACTS[@]}"; do
  target="${item%%|*}"
  path="${item##*|}"
  [ -f "$path" ] || fail "missing-artifact: $path"
  bytes="$(size_of "$path")"
  COUNT=$((COUNT + 1))
  [ -z "$JSON_ITEMS" ] || JSON_ITEMS="$JSON_ITEMS,"
  JSON_ITEMS="$JSON_ITEMS{\"target\":\"$target\",\"path\":\"$path\",\"bytes\":$bytes}"
done
[ "$COUNT" -eq 8 ] || fail "artifact-count: $COUNT (expect 8: 6 CLI + 2 tray)"

# ---------- 5. 版本自证 ----------
HOST_OS="$(go env GOOS)"
HOST_ARCH="$(go env GOARCH)"
HOST_BIN="dist/codex-chinese-${HOST_OS}-${HOST_ARCH}"
[ "$HOST_OS" = "windows" ] && HOST_BIN="${HOST_BIN}.exe"
[ -f "$HOST_BIN" ] || HOST_BIN="dist/codex-chinese-linux-amd64"

VERSION_LINE="$("$HOST_BIN" -version 2>/dev/null | head -1)"
case "$VERSION_LINE" in
  codex-chinese\ *) ;;
  *) fail "version-check-failed: $VERSION_LINE" ;;
esac

# ---------- 6. 环境诊断（exit 2 = 本机无 Codex，属正常） ----------
"$HOST_BIN" -doctor >/tmp/codex-chinese-doctor.log 2>&1
DOCTOR_EXIT=$?
[ "$DOCTOR_EXIT" -eq 0 ] || [ "$DOCTOR_EXIT" -eq 2 ] || fail "doctor-failed: $DOCTOR_EXIT"

# ---------- 7. 输出报告 ----------
REPORT_JSON="{\"ok\":true,\"version\":\"$VERSION\",\"go_version\":\"$GO_VER\",\"vet_exit\":$VET_EXIT,\"prelude_test\":\"$PRELUDE_TEST\",\"artifacts\":[$JSON_ITEMS],\"doctor_exit\":$DOCTOR_EXIT,\"notes\":\"doctor_exit=2 means Codex not installed on this machine\"}"
printf '%s\n' "$REPORT_JSON"
printf '%s\n' "$REPORT_JSON" > "$REPORT"
