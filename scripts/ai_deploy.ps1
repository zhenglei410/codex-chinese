# 一键部署并自证（Windows）。供 AI 代理直接运行。
# 用法： $env:VERSION="v1.0.0"; powershell -NoProfile -ExecutionPolicy Bypass -File scripts\ai_deploy.ps1
# 输出： JSON 报告 + 写入 deploy-report.json
$ErrorActionPreference = "Continue"
Set-Location (Join-Path $PSScriptRoot "..")

$Version = if ($env:VERSION) { $env:VERSION } else { "dev" }
$Report = "deploy-report.json"

function Fail([string]$Reason) {
  $o = [ordered]@{ ok = $false; version = $Version; reason = $Reason }
  $j = $o | ConvertTo-Json -Compress
  Write-Output $j
  Set-Content -Path $Report -Value $j -Encoding UTF8
  exit 1
}

# 1. 检查 Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  Fail "go-not-installed: see docs/AI_DEPLOY.md#2"
}
$GoLine = (go version) 2>$null
if (-not $GoLine) { Fail "go-not-executable" }
$GoVer = ($GoLine -split "\s+")[2]
if ($GoVer -notmatch '^go(\d+)\.(\d+)') { Fail "go-version-unknown: $GoVer" }
$major = [int]$Matches[1]; $minor = [int]$Matches[2]
if ($major -lt 1 -or ($major -eq 1 -and $minor -lt 21)) { Fail "go-too-old: $GoVer (need >= 1.21)" }

# 2. 构建
$env:VERSION = $Version
& (Join-Path $PSScriptRoot "..\build.ps1") | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "build-failed" }

# 3. 静态检查（先清掉 build.ps1 可能残留的交叉编译环境）
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
go vet ./... | Out-Null
$VetExit = $LASTEXITCODE
if ($VetExit -ne 0) { Fail "vet-failed" }

# 3.5 注入脚本回归测试（prelude 是内联 JS，go vet 看不到它的逻辑错误）
$PreludeTest = "skipped-no-node"
if (Get-Command node -ErrorAction SilentlyContinue) {
  node test/prelude.test.mjs | Out-Null
  if ($LASTEXITCODE -ne 0) { Fail "prelude-test-failed: run 'node test/prelude.test.mjs'" }
  $PreludeTest = "pass"
} else {
  Write-Host "WARN: 未检测到 node，跳过 test/prelude.test.mjs（建议安装 node >= 18）"
}

# 4. 校验产物
$Artifacts = @(
  @{ target = "windows/amd64"; path = "dist/codex-chinese-windows-amd64.exe" },
  @{ target = "windows/arm64"; path = "dist/codex-chinese-windows-arm64.exe" },
  @{ target = "darwin/amd64";  path = "dist/codex-chinese-darwin-amd64" },
  @{ target = "darwin/arm64";  path = "dist/codex-chinese-darwin-arm64" },
  @{ target = "linux/amd64";   path = "dist/codex-chinese-linux-amd64" },
  @{ target = "linux/arm64";   path = "dist/codex-chinese-linux-arm64" },
  @{ target = "windows/amd64"; path = "dist/codex-chinese-tray-windows-amd64.exe" },
  @{ target = "windows/arm64"; path = "dist/codex-chinese-tray-windows-arm64.exe" }
)
$items = @()
foreach ($a in $Artifacts) {
  if (-not (Test-Path $a.path)) { Fail ("missing-artifact: " + $a.path) }
  $len = (Get-Item $a.path).Length
  $items += [ordered]@{ target = $a.target; path = $a.path; bytes = $len }
}
if ($items.Count -ne 8) { Fail ("artifact-count: " + $items.Count + " (expect 8: 6 CLI + 2 tray)") }

# 5. 版本自证（用本机 Windows 产物）
$hostBin = "dist\codex-chinese-windows-amd64.exe"
if (-not (Test-Path $hostBin)) { $hostBin = "dist\codex-chinese-linux-amd64" }
$VersionLine = (& $hostBin -version 2>$null | Select-Object -First 1)
if (-not $VersionLine -or -not $VersionLine.StartsWith("codex-chinese ")) {
  Fail ("version-check-failed: " + $VersionLine)
}

# 6. 环境诊断（2 = 本机无 Codex，属正常）
& $hostBin -doctor 2>$null | Out-Null
$DoctorExit = $LASTEXITCODE
if ($DoctorExit -ne 0 -and $DoctorExit -ne 2) { Fail ("doctor-failed: " + $DoctorExit) }

# 7. 输出报告
$reportObj = [ordered]@{
  ok = $true
  version = $Version
  go_version = $GoVer
  vet_exit = $VetExit
  prelude_test = $PreludeTest
  artifacts = $items
  doctor_exit = $DoctorExit
  notes = "doctor_exit=2 means Codex not installed on this machine"
}
$json = $reportObj | ConvertTo-Json -Depth 5
Write-Output $json
Set-Content -Path $Report -Value $json -Encoding UTF8
