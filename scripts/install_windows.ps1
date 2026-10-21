# codex-chinese Windows 安装器
# - 默认下载官方 Release 资产（不需要 Go）
# - 校验 SHA256
# - 快捷方式直接指向 codex-chinese*.exe（禁止 wscript/.vbs）
#
# 用法：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install_windows.ps1
#   ... -Mode cli            # 只装命令行版
#   ... -Version v2.0.7      # 指定版本
#   ... -AssetDir .\dist     # 用本地已构建产物（离线）
#   ... -Uninstall           # 卸载
param(
  [string]$Version = "latest",
  [ValidateSet("tray", "cli")][string]$Mode = "tray",
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\codex-chinese",
  [string]$AssetDir = "",
  [switch]$NoShortcut,
  [switch]$NoAutostart,
  [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$Repo = "zhenglei410/codex-chinese"

function Get-Arch {
  $a = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
  if ($a -eq [System.Runtime.InteropServices.Architecture]::Arm64) { return "arm64" }
  return "amd64"
}

function Remove-Shortcuts {
  $desktop = [Environment]::GetFolderPath("Desktop")
  $startMenu = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
  $names = @("Codex 中文.lnk", "Codex 中文（启动）.lnk")
  foreach ($p in @(
      (Join-Path $desktop $names[0]), (Join-Path $desktop $names[1]),
      (Join-Path $startMenu $names[0]), (Join-Path $startMenu $names[1]))) {
    if (Test-Path $p) { Remove-Item $p -Force; Write-Host "已删除快捷方式: $p" }
  }
}

function Stop-InstalledProcesses {
  $prefix = $InstallDir.TrimEnd('\') + '\'
  $processes = Get-CimInstance Win32_Process |
    Where-Object {
      $_.Name -match '^codex-chinese.*\.exe$' -and
      $_.ExecutablePath -and
      $_.ExecutablePath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)
    }
  foreach ($process in $processes) {
    try {
      Stop-Process -Id $process.ProcessId -Force -ErrorAction Stop
      Write-Host "已停止旧进程: $($process.Name) pid=$($process.ProcessId)"
    } catch {
      throw "无法停止旧进程 $($process.Name) pid=$($process.ProcessId): $($_.Exception.Message)"
    }
  }
  Start-Sleep -Milliseconds 400
}

if ($Uninstall) {
  Remove-Shortcuts
  Stop-InstalledProcesses
  Remove-ItemProperty -LiteralPath "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "CodexChinese" -ErrorAction SilentlyContinue
  if (Test-Path $InstallDir) { Remove-Item $InstallDir -Recurse -Force; Write-Host "已删除目录: $InstallDir" }
  Write-Host "卸载完成。Codex 本体未被修改。" -ForegroundColor Green
  exit 0
}

$arch = Get-Arch
$cliName = "codex-chinese-windows-$arch.exe"
$trayName = "codex-chinese-tray-windows-$arch.exe"
$iconName = "codex-chinese.ico"

# 1. 解析版本
if ($Version -eq "latest") {
  try {
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "codex-chinese-installer" } -TimeoutSec 20
    $Version = $rel.tag_name
  } catch {
    Write-Host "获取最新版本失败：$($_.Exception.Message)" -ForegroundColor Red
    Write-Host "可指定 -Version，或用 -AssetDir 指向本地 dist 目录。" -ForegroundColor Yellow
    exit 1
  }
}
Write-Host "目标版本: $Version  ($arch)" -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$tmp = Join-Path $env:TEMP ("codex-chinese-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

function Fetch-Asset([string]$Name) {
  $dest = Join-Path $tmp $Name
  if ($AssetDir -and (Test-Path (Join-Path $AssetDir $Name))) {
    Copy-Item (Join-Path $AssetDir $Name) $dest -Force
  } else {
    $url = "https://github.com/$Repo/releases/download/$Version/$Name"
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing -TimeoutSec 120
  }
  return $dest
}

try {
  Write-Host "下载 $cliName ..."
  $cli = Fetch-Asset $cliName
  $tray = $null
  Write-Host "准备 $iconName ..."
  $icon = Fetch-Asset $iconName
  if ($Mode -eq "tray") {
    Write-Host "下载 $trayName ..."
    $tray = Fetch-Asset $trayName
  }

  # 2. 校验 SHA256（有 SHA256SUMS 才校验）
  $sumsPath = Join-Path $tmp "SHA256SUMS"
  $haveSums = $false
  if ($AssetDir -and (Test-Path (Join-Path $AssetDir "SHA256SUMS"))) {
    Copy-Item (Join-Path $AssetDir "SHA256SUMS") $sumsPath -Force; $haveSums = $true
  } else {
    try {
      Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/SHA256SUMS" -OutFile $sumsPath -UseBasicParsing -TimeoutSec 60
      $haveSums = $true
    } catch { $haveSums = $false }
  }
  if ($haveSums) {
    $map = @{}
    foreach ($line in Get-Content $sumsPath) {
      if ($line -match '^\s*([0-9a-fA-F]{64})\s+\*?(.+?)\s*$') { $map[$Matches[2]] = $Matches[1] }
    }
    foreach ($f in @($cli, $tray, $icon)) {
      if (-not $f) { continue }
      $name = Split-Path $f -Leaf
      if ($map.ContainsKey($name)) {
        $actual = (Get-FileHash $f -Algorithm SHA256).Hash.ToLower()
        if ($actual -ne $map[$name].ToLower()) { throw "SHA256 校验失败: $name" }
        Write-Host "SHA256 OK: $name" -ForegroundColor Green
      }
    }
  } else {
    Write-Host "未取得 SHA256SUMS，跳过校验（不建议）" -ForegroundColor Yellow
  }

  # 3. 安装。先停止旧托盘/supervisor，避免正在使用的 exe 无法覆盖。
  Stop-InstalledProcesses
  Copy-Item $cli (Join-Path $InstallDir "codex-chinese.exe") -Force
  if ($tray) { Copy-Item $tray (Join-Path $InstallDir "codex-chinese-tray.exe") -Force }
  $iconPath = Join-Path $InstallDir "codex-chinese.ico"
  Copy-Item $icon $iconPath -Force
  Write-Host "已安装到: $InstallDir" -ForegroundColor Green

  $runKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
  if ($Mode -eq "tray" -and -not $NoAutostart) {
    New-Item -Path $runKey -Force | Out-Null
    $autostartTarget = Join-Path $InstallDir "codex-chinese-tray.exe"
    Set-ItemProperty -LiteralPath $runKey -Name "CodexChinese" `
      -Value ('"' + $autostartTarget + '"') -Type String
    Write-Host "已启用登录后自动启动: $autostartTarget" -ForegroundColor Green
  } else {
    Remove-ItemProperty -LiteralPath $runKey -Name "CodexChinese" -ErrorAction SilentlyContinue
  }

  # 4. 快捷方式（直接指向 exe）
  if (-not $NoShortcut) {
    # Remove stale shortcuts that pointed at an old binary or a fixed Codex
    # version directory. The tool discovers the current MSIX package at runtime.
    Remove-Shortcuts
    $target = if ($tray) { Join-Path $InstallDir "codex-chinese-tray.exe" } else { Join-Path $InstallDir "codex-chinese.exe" }
    $ws = New-Object -ComObject WScript.Shell
    $desktop = [Environment]::GetFolderPath("Desktop")
    $startMenu = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
    New-Item -ItemType Directory -Force -Path $startMenu | Out-Null
    foreach ($lnk in @(
        (Join-Path $desktop "Codex 中文.lnk"),
        (Join-Path $startMenu "Codex 中文.lnk"))) {
      $sc = $ws.CreateShortcut($lnk)
      $sc.TargetPath = $target
      $sc.WorkingDirectory = $InstallDir
      $sc.IconLocation = "$iconPath,0"
      $sc.Description = "强制 Codex 中文界面"
      $sc.Save()
      Write-Host "已创建快捷方式: $lnk -> $target" -ForegroundColor Green
    }
  }

  Write-Host ""
  Write-Host "下一步：" -ForegroundColor Cyan
  Write-Host "  1) 默认已配置登录自启动；重启 Codex 可让中文立即生效"
  if ($Mode -eq "tray") {
    Write-Host "  2) 双击桌面「Codex 中文」；托盘版会自动启动并启用中文"
    Write-Host "     登录 Windows 后也会自动运行；不需要时在安装时加 -NoAutostart"
  } else {
    Write-Host "  2) 运行: $InstallDir\codex-chinese.exe"
  }
  Write-Host "  3) 校验: & '$InstallDir\codex-chinese.exe' -verify   (应输出 PASS)"
}
finally {
  Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
