param(
  [string]$OutputDir = (Join-Path (Split-Path $PSScriptRoot -Parent) "assets")
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$PngPath = Join-Path $OutputDir "codex-chinese-icon.png"
$IcoPath = Join-Path $OutputDir "codex-chinese.ico"

function New-IconBitmap([int]$Size) {
  $bitmap = New-Object System.Drawing.Bitmap($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
  $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
  $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
  $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
  $graphics.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
  $graphics.Clear([System.Drawing.Color]::Transparent)

  $scale = $Size / 1024.0
  $graphics.ScaleTransform($scale, $scale)

  $shadowPath = New-Object System.Drawing.Drawing2D.GraphicsPath
  $shadowPath.AddEllipse(92, 108, 840, 840)
  $shadowBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(44, 0, 0, 0))
  $graphics.FillPath($shadowBrush, $shadowPath)
  $shadowBrush.Dispose()
  $shadowPath.Dispose()

  $circle = New-Object System.Drawing.RectangleF(82, 82, 860, 860)
  $background = New-Object System.Drawing.Drawing2D.LinearGradientBrush(
    $circle,
    [System.Drawing.Color]::FromArgb(255, 38, 42, 54),
    [System.Drawing.Color]::FromArgb(255, 20, 22, 29),
    135.0
  )
  $graphics.FillEllipse($background, $circle)
  $background.Dispose()

  $innerRing = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(30, 255, 255, 255), 5)
  $graphics.DrawEllipse($innerRing, 102, 102, 820, 820)
  $innerRing.Dispose()

  $cyan = [System.Drawing.Color]::FromArgb(255, 205, 249, 255)
  $white = [System.Drawing.Color]::FromArgb(255, 248, 250, 252)

  $outerPen = New-Object System.Drawing.Pen($cyan, 84)
  $outerPen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
  $outerPen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
  $outerPen.LineJoin = [System.Drawing.Drawing2D.LineJoin]::Round
  $graphics.DrawArc($outerPen, 238, 238, 548, 548, 42, 276)
  $outerPen.Dispose()

  $chevronPen = New-Object System.Drawing.Pen($white, 66)
  $chevronPen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
  $chevronPen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
  $chevronPen.LineJoin = [System.Drawing.Drawing2D.LineJoin]::Round
  $chevron = New-Object System.Drawing.Drawing2D.GraphicsPath
  $chevron.AddLine(414, 390, 548, 512)
  $chevron.AddLine(548, 512, 414, 634)
  $graphics.DrawPath($chevronPen, $chevron)
  $chevron.Dispose()
  $chevronPen.Dispose()

  $nodeBrush = New-Object System.Drawing.SolidBrush($cyan)
  $graphics.FillEllipse($nodeBrush, 688, 496, 32, 32)
  $nodeBrush.Dispose()

  $graphics.Dispose()
  return $bitmap
}

$master = New-IconBitmap 1024
$master.Save($PngPath, [System.Drawing.Imaging.ImageFormat]::Png)
$master.Dispose()

$sizes = @(16, 24, 32, 48, 64, 128, 256)
$images = @()
foreach ($size in $sizes) {
  $bitmap = New-IconBitmap $size
  $stream = New-Object System.IO.MemoryStream
  $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
  $bitmap.Dispose()
  $images += ,@($size, $stream.ToArray())
  $stream.Dispose()
}

$file = [System.IO.File]::Create($IcoPath)
$writer = New-Object System.IO.BinaryWriter($file)
$writer.Write([UInt16]0)
$writer.Write([UInt16]1)
$writer.Write([UInt16]$images.Count)

$offset = 6 + (16 * $images.Count)
foreach ($image in $images) {
  $size = [int]$image[0]
  $data = [byte[]]$image[1]
  $writer.Write([byte]$(if ($size -ge 256) { 0 } else { $size }))
  $writer.Write([byte]$(if ($size -ge 256) { 0 } else { $size }))
  $writer.Write([byte]0)
  $writer.Write([byte]0)
  $writer.Write([UInt16]1)
  $writer.Write([UInt16]32)
  $writer.Write([UInt32]$data.Length)
  $writer.Write([UInt32]$offset)
  $offset += $data.Length
}
foreach ($image in $images) {
  $writer.Write([byte[]]$image[1])
}
$writer.Flush()
$writer.Dispose()
$file.Dispose()

Write-Output "PNG: $PngPath"
Write-Output "ICO: $IcoPath"
