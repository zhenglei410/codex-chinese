#!/usr/bin/env bash
# 交叉编译全部目标。零第三方依赖。
# 用法： VERSION=v1.0.0 bash build.sh
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-dev}"
OUT="dist"
LDFLAGS="-s -w -X main.version=${VERSION}"

mkdir -p "$OUT"

build() {
  local os="$1" arch="$2" ext="$3"
  echo "building ${os}/${arch} (version=${VERSION}) ..."
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false -ldflags "$LDFLAGS" \
    -o "${OUT}/codex-chinese-${os}-${arch}${ext}" .
}

build windows amd64 ".exe"
build windows arm64 ".exe"
build darwin  amd64 ""
build darwin  arm64 ""
build linux   amd64 ""
build linux   arm64 ""

# 托盘版仅 Windows（纯 Win32，无 cgo）
# 必须带 -H=windowsgui：否则产物是控制台子系统程序，启动时会弹一个黑窗。
build_tray() {
  local arch="$1"
  echo "building tray windows/${arch} (version=${VERSION}) ..."
  GOOS=windows GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false -ldflags "$LDFLAGS -H=windowsgui" \
    -o "${OUT}/codex-chinese-tray-windows-${arch}.exe" ./tray
}
build_tray amd64
build_tray arm64

if [ ! -f "assets/codex-chinese.ico" ]; then
  echo "missing icon: assets/codex-chinese.ico" >&2
  exit 1
fi
cp "assets/codex-chinese.ico" "${OUT}/codex-chinese.ico"

echo "--- sizes ---"
ls -lh "$OUT"

# 校验和（Windows 用户可用 certutil 校验）
if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "$OUT"
    for file in *; do
      [ -f "$file" ] && [ "$file" != "SHA256SUMS" ] && sha256sum "$file"
    done | sort -k2 > SHA256SUMS
  )
elif command -v shasum >/dev/null 2>&1; then
  (
    cd "$OUT"
    for file in *; do
      [ -f "$file" ] && [ "$file" != "SHA256SUMS" ] && shasum -a 256 "$file"
    done | sort -k2 > SHA256SUMS
  )
fi
echo "checksums written to ${OUT}/SHA256SUMS"
