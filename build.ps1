# 交叉编译全部目标（Windows PowerShell 可用）。
# 用法： $env:VERSION="v1.0.0"; .\build.ps1
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$Version = if ($env:VERSION) { $env:VERSION } else { "dev" }
$Out = "dist"
$LdFlags = "-s -w -X main.version=$Version"
New-Item -ItemType Directory -Force -Path $Out | Out-Null

# 保存原环境，构建后恢复，避免污染调用方的 GOOS/GOARCH
$savedGoos = $env:GOOS
$savedGoarch = $env:GOARCH
$savedCgo = $env:CGO_ENABLED

function Build-Target([string]$Os, [string]$Arch, [string]$Ext) {
  Write-Host "building $Os/$Arch (version=$Version) ..."
  $env:GOOS = $Os
  $env:GOARCH = $Arch
  $env:CGO_ENABLED = "0"
  go build -trimpath -buildvcs=false -ldflags $LdFlags -o "$Out/codex-chinese-$Os-$Arch$Ext" .
}

try {
  Build-Target "windows" "amd64" ".exe"
  Build-Target "windows" "arm64" ".exe"
  Build-Target "darwin"  "amd64" ""
  Build-Target "darwin"  "arm64" ""
  Build-Target "linux"   "amd64" ""
  Build-Target "linux"   "arm64" ""
}
finally {
  $env:GOOS = $savedGoos
  $env:GOARCH = $savedGoarch
  $env:CGO_ENABLED = $savedCgo
}

Write-Host "--- sizes ---"
Get-ChildItem $Out | Format-Table Name, Length
