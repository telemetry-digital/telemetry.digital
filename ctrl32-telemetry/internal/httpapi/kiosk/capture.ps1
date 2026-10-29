# ctrl32 telemetry - screenshot of the kiosk user's desktop (spec 034). The kiosk agent starts it in the user's
# session only when an operator asked for a screenshot (allowed for this PC, with a reason, audited); the picture is
# sent to the server once and deleted here.
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
$out = 'C:\ProgramData\ctrl32-kiosk\user\shot.jpg'
$v = [System.Windows.Forms.SystemInformation]::VirtualScreen
$bmp = New-Object System.Drawing.Bitmap $v.Width, $v.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($v.Left, $v.Top, 0, 0, $bmp.Size)
$k = [Math]::Min(1.0, 2400.0 / $v.Width)
$small = New-Object System.Drawing.Bitmap $bmp, ([int]($v.Width * $k)), ([int]($v.Height * $k))
$enc = [System.Drawing.Imaging.ImageCodecInfo]::GetImageEncoders() | Where-Object { $_.MimeType -eq 'image/jpeg' }
$p = New-Object System.Drawing.Imaging.EncoderParameters 1
$p.Param[0] = New-Object System.Drawing.Imaging.EncoderParameter ([System.Drawing.Imaging.Encoder]::Quality), ([long]70)
$small.Save("$out.tmp", $enc, $p)
Move-Item "$out.tmp" $out -Force
$g.Dispose(); $bmp.Dispose(); $small.Dispose()
