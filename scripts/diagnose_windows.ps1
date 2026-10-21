param(
  [int]$Port = 0
)

$ErrorActionPreference = "Continue"
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\codex-chinese"
$Cli = Join-Path $InstallDir "codex-chinese.exe"
$Tray = Join-Path $InstallDir "codex-chinese-tray.exe"

function Section([string]$Name) {
  Write-Output ""
  Write-Output "===== $Name ====="
}

function Value([string]$Name, $Data) {
  Write-Output ("{0,-24} {1}" -f ($Name + ":"), $Data)
}

Section "系统"
Value "os" (Get-CimInstance Win32_OperatingSystem).Caption
Value "arch" $env:PROCESSOR_ARCHITECTURE
Value "user" $env:USERNAME

Section "安装"
Value "install_dir" $InstallDir
Value "cli_exists" (Test-Path -LiteralPath $Cli)
Value "tray_exists" (Test-Path -LiteralPath $Tray)
if (Test-Path -LiteralPath $Cli) {
  $version = (& $Cli -version 2>&1 | Select-Object -First 1)
  Value "version" $version
}

Section "Codex 包"
$packages = Get-AppxPackage -Name "OpenAI.Codex" | Sort-Object Version -Descending
if (-not $packages) {
  Value "appx" "not-found"
} else {
  foreach ($pkg in $packages) {
    Value "full_name" $pkg.PackageFullName
    Value "family_name" $pkg.PackageFamilyName
    Value "root" $pkg.InstallLocation
  }
}

Section "进程"
$processes = Get-CimInstance Win32_Process |
  Where-Object { $_.Name -match "^(ChatGPT|Codex|codex-chinese)" }
if (-not $processes) {
  Value "processes" "none"
} else {
  $processes | Select-Object Name,ProcessId,ParentProcessId,CreationDate,CommandLine |
    Format-List
}

Section "CDP"
$ports = @(9229..9258)
if ($Port -gt 0) { $ports = @($Port) }
$found = @()
foreach ($p in $ports) {
  $listener = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue
  if (-not $listener) { continue }
  try {
    $targets = Invoke-RestMethod -Uri "http://127.0.0.1:$p/json" -TimeoutSec 2
    $pages = @($targets | Where-Object { $_.type -eq "page" })
    $found += [pscustomobject]@{
      Port = $p
      OwningProcess = @($listener)[0].OwningProcess
      Targets = ($pages | ForEach-Object { $_.url }) -join ", "
    }
  } catch {
    Value "port_$p" "listening, CDP unavailable"
  }
}
if ($found) {
  $found | Format-Table -AutoSize
} else {
  Value "cdp" "not-ready (9229-9258)"
}

Section "自启动与快捷方式"
$run = (Get-ItemProperty -LiteralPath `
  "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" `
  -ErrorAction SilentlyContinue).CodexChinese
Value "autostart" $run

$ws = New-Object -ComObject WScript.Shell
$links = @(
  (Join-Path ([Environment]::GetFolderPath("Desktop")) "Codex 中文.lnk"),
  (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Codex 中文.lnk")
)
foreach ($lnk in $links) {
  if (Test-Path -LiteralPath $lnk) {
    $sc = $ws.CreateShortcut($lnk)
    Value "shortcut" "$lnk -> $($sc.TargetPath) $($sc.Arguments)"
  } else {
    Value "shortcut" "missing: $lnk"
  }
}

Section "程序自检"
if (Test-Path -LiteralPath $Cli) {
  & $Cli -doctor
  Value "doctor_exit" $LASTEXITCODE
  & $Cli -verify
  Value "verify_exit" $LASTEXITCODE
} else {
  Value "cli" "missing; cannot run -doctor/-verify"
}

Section "日志"
foreach ($path in @(
    (Join-Path $env:TEMP "codex-chinese-tray.log"),
    (Join-Path $env:TEMP "codex-chinese-supervisor.log"))) {
  Value "log" $path
  if (Test-Path -LiteralPath $path) {
    Get-Content -Tail 40 -Encoding UTF8 -LiteralPath $path
  }
}

Write-Output ""
Write-Output "完成。成功标准：verify_exit=0 且输出 PASS。"
