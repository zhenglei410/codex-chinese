# Codex 环境诊断（Windows / 商店版专用）
#
# 用途：当 codex-chinese 报"找不到 Codex"或 cdp: not-ready 时，
#       在目标机器上跑一次，把输出整段贴回来即可定位。
#
# 用法：
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\diagnose_windows.ps1
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts\diagnose_windows.ps1 -TryLaunch
#
# -TryLaunch 会真的尝试带调试端口启动 Codex 并等 12 秒，用于验证
# "商店版能不能接受命令行开关"这个关键问题。
param(
  [switch]$TryLaunch,
  [int]$Port = 9229
)

$ErrorActionPreference = "SilentlyContinue"
function Line($k, $v) { Write-Output ("{0,-22} {1}" -f ($k + ":"), $v) }

Write-Output "===== 1. 系统 ====="
Line "os" ((Get-CimInstance Win32_OperatingSystem).Caption)
Line "arch" $env:PROCESSOR_ARCHITECTURE
Line "user" $env:USERNAME
Line "localappdata" $env:LOCALAPPDATA

Write-Output ""
Write-Output "===== 2. Codex 安装包（MSIX / 商店） ====="
$pkgs = Get-AppxPackage -Name "*Codex*"
if (-not $pkgs) {
  Line "appx" "未找到名为 *Codex* 的商店包"
} else {
  foreach ($p in $pkgs) {
    Line "name" $p.Name
    Line "version" $p.Version
    Line "install" $p.InstallLocation
    Line "family" $p.PackageFamilyName
    $exe = Join-Path $p.InstallLocation "app\ChatGPT.exe"
    if (Test-Path $exe) {
      $f = Get-Item $exe
      Line "ChatGPT.exe" ("存在  {0} 字节  {1}" -f $f.Length, $f.FullName)
      Line "  可读" ([bool](Get-Content $exe -TotalCount 1 -Encoding Byte))
    } else {
      Line "ChatGPT.exe" "不存在于 app\ 下"
      $alt = Get-ChildItem $p.InstallLocation -Filter "*.exe" -Recurse -Depth 2 |
             Select-Object -First 5 -ExpandProperty FullName
      foreach ($a in $alt) { Line "  other_exe" $a }
    }
  }
}

Write-Output ""
Write-Output "===== 3. 应用执行别名（推荐启动方式） ====="
foreach ($n in @("ChatGPT.exe", "Codex.exe", "codex.exe")) {
  $alias = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\$n"
  if (Test-Path $alias) {
    $item = Get-Item $alias
    Line $n ("存在  {0} 字节  target={1}" -f $item.Length, $item.Target)
  } else {
    Line $n "不存在"
  }
}

Write-Output ""
Write-Output "===== 4. 正在运行的进程 ====="
$procs = Get-Process -Name "ChatGPT", "Codex", "codex" -ErrorAction SilentlyContinue
if (-not $procs) {
  Line "process" "无 ChatGPT / Codex 进程在运行"
} else {
  foreach ($pr in $procs) {
    Line $pr.ProcessName ("pid={0}  path={1}" -f $pr.Id, $pr.Path)
  }
}

Write-Output ""
Write-Output "===== 5. 调试端口 $Port ====="
$listen = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
Line "listen" ([bool]$listen)
try {
  $r = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/json/version" -TimeoutSec 2 -UseBasicParsing
  Line "cdp" ("ready  " + ($r.Content -replace "\s+", " ").Substring(0, [Math]::Min(90, $r.Content.Length)))
} catch {
  Line "cdp" "not-ready"
}

if ($TryLaunch) {
  Write-Output ""
  Write-Output "===== 6. 带 --remote-debugging-port 试启动 ====="
  $target = $null
  foreach ($n in @("ChatGPT.exe", "Codex.exe")) {
    $a = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps\$n"
    if (Test-Path $a) { $target = $a; break }
  }
  if (-not $target -and $pkgs) {
    $cand = Join-Path $pkgs[0].InstallLocation "app\ChatGPT.exe"
    if (Test-Path $cand) { $target = $cand }
  }
  if (-not $target) {
    Line "launch" "没有可启动的目标，跳过"
  } else {
    Line "target" $target
    try {
      $p = Start-Process -FilePath $target -ArgumentList "--remote-debugging-port=$Port" -PassThru -ErrorAction Stop
      Line "started" ("pid=" + $p.Id)
      Start-Sleep -Seconds 12
      $ok = $false
      try {
        $r2 = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/json/version" -TimeoutSec 2 -UseBasicParsing
        $ok = $true
        Line "cdp_after" ("ready  " + $r2.StatusCode)
      } catch { Line "cdp_after" "not-ready（商店版可能忽略了该开关）" }
      $alive = Get-Process -Id $p.Id -ErrorAction SilentlyContinue
      Line "still_running" ([bool]$alive)
      if (-not $ok) {
        Line "结论" "该启动方式未打开调试端口，需要换 alias/提权方式排查"
      }
    } catch {
      Line "launch_failed" $_.Exception.Message
      Line "结论" "无法直接执行该路径（常见于 WindowsApps 的 ACL 限制）"
    }
  }
}

Write-Output ""
Write-Output "===== 完成，请把以上全部内容贴回 ====="
