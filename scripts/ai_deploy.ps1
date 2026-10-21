param(
  [string]$Version = "v2.0.6",
  [switch]$NoInstall,
  [switch]$Start
)

$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $Root

$ReportPath = Join-Path $Root "deploy-report.json"
$Artifacts = @(
  "dist\codex-chinese-windows-amd64.exe",
  "dist\codex-chinese-windows-arm64.exe",
  "dist\codex-chinese-darwin-amd64",
  "dist\codex-chinese-darwin-arm64",
  "dist\codex-chinese-linux-amd64",
  "dist\codex-chinese-linux-arm64",
  "dist\codex-chinese-tray-windows-amd64.exe",
  "dist\codex-chinese-tray-windows-arm64.exe"
)

function Save-Report([hashtable]$Report) {
  $Report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $ReportPath -Encoding UTF8
}

function Fail([string]$Reason) {
  $Report = [ordered]@{
    ok = $false
    version = $Version
    reason = $Reason
    generated_at = (Get-Date).ToString("o")
  }
  Save-Report $Report
  Write-Output ($Report | ConvertTo-Json -Depth 8)
  exit 1
}

$Go = Get-Command go -ErrorAction SilentlyContinue
if (-not $Go) {
  Fail "go-not-installed: install Go 1.21+ and retry"
}
$GoVersion = (& go version) 2>$null
if ($LASTEXITCODE -ne 0 -or -not $GoVersion) {
  Fail "go-not-executable"
}

$env:VERSION = $Version
& (Join-Path $Root "build.ps1")
if ($LASTEXITCODE -ne 0) {
  Fail "build-failed"
}

Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

& go vet ./...
if ($LASTEXITCODE -ne 0) {
  Fail "go-vet-failed"
}

& go test ./...
if ($LASTEXITCODE -ne 0) {
  Fail "go-test-failed"
}

$PreludeTest = "skipped-no-node"
if (Get-Command node -ErrorAction SilentlyContinue) {
  & node test\prelude.test.mjs
  if ($LASTEXITCODE -ne 0) {
    Fail "prelude-test-failed"
  }
  $PreludeTest = "pass"
}

$ArtifactRows = @()
foreach ($Path in $Artifacts) {
  if (-not (Test-Path -LiteralPath $Path)) {
    Fail "missing-artifact: $Path"
  }
  $File = Get-Item -LiteralPath $Path
  if ($File.Length -gt 6MB) {
    Fail "artifact-too-large: $Path ($($File.Length) bytes)"
  }
  $ArtifactRows += [ordered]@{
    path = $Path
    bytes = $File.Length
    sha256 = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLower()
  }
}
if ($ArtifactRows.Count -ne 8) {
  Fail "artifact-count: $($ArtifactRows.Count)"
}

$SumLines = foreach ($File in Get-ChildItem -LiteralPath "dist" -File |
    Where-Object Name -ne "SHA256SUMS" | Sort-Object Name) {
  $Hash = (Get-FileHash -LiteralPath $File.FullName -Algorithm SHA256).Hash.ToLower()
  "$Hash  $($File.Name)"
}
Set-Content -LiteralPath "dist\SHA256SUMS" -Value $SumLines -Encoding ascii

$Install = $null
if (-not $NoInstall) {
  & powershell.exe -NoProfile -ExecutionPolicy Bypass `
    -File (Join-Path $Root "scripts\install_windows.ps1") `
    -Version $Version -AssetDir (Join-Path $Root "dist") -Mode tray
  if ($LASTEXITCODE -ne 0) {
    Fail "install-failed"
  }

  $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\codex-chinese"
  $InstalledCli = Join-Path $InstallDir "codex-chinese.exe"
  $InstalledTray = Join-Path $InstallDir "codex-chinese-tray.exe"
  $BuiltCli = Join-Path $Root "dist\codex-chinese-windows-amd64.exe"
  $BuiltTray = Join-Path $Root "dist\codex-chinese-tray-windows-amd64.exe"

  foreach ($Pair in @(
      @($BuiltCli, $InstalledCli),
      @($BuiltTray, $InstalledTray)
    )) {
    $BuiltHash = (Get-FileHash -LiteralPath $Pair[0] -Algorithm SHA256).Hash
    $InstalledHash = (Get-FileHash -LiteralPath $Pair[1] -Algorithm SHA256).Hash
    if ($BuiltHash -ne $InstalledHash) {
      Fail "installed-hash-mismatch: $($Pair[1])"
    }
  }

  $Autostart = (Get-ItemProperty -LiteralPath `
    "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" `
    -ErrorAction SilentlyContinue).CodexChinese
  if (-not $Autostart -or $Autostart.Trim('"') -ne $InstalledTray) {
    Fail "autostart-not-configured"
  }

  $TrayBytes = [IO.File]::ReadAllBytes($InstalledTray)
  $PeOffset = [BitConverter]::ToInt32($TrayBytes, 0x3c)
  $Subsystem = [BitConverter]::ToUInt16($TrayBytes, $PeOffset + 24 + 68)
  if ($Subsystem -ne 2) {
    Fail "tray-subsystem-not-gui: $Subsystem"
  }

  $Shell = New-Object -ComObject WScript.Shell
  $Shortcuts = @(
    (Join-Path ([Environment]::GetFolderPath("Desktop")) "Codex 中文.lnk"),
    (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Codex 中文.lnk")
  )
  foreach ($ShortcutPath in $Shortcuts) {
    if (-not (Test-Path -LiteralPath $ShortcutPath)) {
      Fail "shortcut-missing: $ShortcutPath"
    }
    $Shortcut = $Shell.CreateShortcut($ShortcutPath)
    if ($Shortcut.TargetPath -ne $InstalledTray) {
      Fail "shortcut-target-mismatch: $ShortcutPath -> $($Shortcut.TargetPath)"
    }
  }

  $Install = [ordered]@{
    dir = $InstallDir
    cli = $InstalledCli
    tray = $InstalledTray
    autostart = $Autostart
    tray_subsystem = $Subsystem
    shortcuts = $Shortcuts
  }
}

$Started = $false
if ($Start) {
  $Tray = Join-Path $env:LOCALAPPDATA "Programs\codex-chinese\codex-chinese-tray.exe"
  Start-Process -FilePath explorer.exe -ArgumentList ('"' + $Tray + '"')
  $Started = $true
}

$Report = [ordered]@{
  ok = $true
  version = $Version
  generated_at = (Get-Date).ToString("o")
  go_version = $GoVersion
  prelude_test = $PreludeTest
  artifact_count = $ArtifactRows.Count
  artifacts = $ArtifactRows
  install = $Install
  tray_started = $Started
  next = if ($Started) {
    "Codex may restart once. After it reopens run codex-chinese.exe -verify."
  } else {
    "Restart Codex after deployment, then run codex-chinese.exe -verify."
  }
}
Save-Report $Report
Write-Output ($Report | ConvertTo-Json -Depth 8)
