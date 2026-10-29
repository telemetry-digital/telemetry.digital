# ctrl32 telemetry - kiosk launcher (spec 034). Runs at the logon of the kiosk user: one Chrome window in kiosk mode
# per monitor, each with its own profile (so each monitor is its own paired display), started again when it closes;
# the taskbar hides itself so it never covers a display. The server keeps this file up to date (Displays -> Kiosk PCs
# -> Update scripts); settings come from launcher.json (from the server) and local.json (from the setup on this PC).
$ErrorActionPreference = 'SilentlyContinue'
$base = 'C:\ProgramData\ctrl32-kiosk'
$udir = Join-Path $base 'user'
$log = Join-Path $udir 'launcher.log'
Add-Type -AssemblyName System.Windows.Forms

function Log($m) {
    if ((Test-Path $log) -and (Get-Item $log).Length -gt 1MB) { Move-Item $log "$log.1" -Force }
    Add-Content -Path $log -Value ("{0:yyyy-MM-dd HH:mm:ss} {1}" -f (Get-Date), $m)
}
function Read-Json($p) { try { return (Get-Content $p -Raw -Encoding UTF8 | ConvertFrom-Json) } catch { return $null } }

function Find-Browser($preferred) {
    $list = @($preferred,
        'C:\Program Files\Google\Chrome\Application\chrome.exe',
        'C:\Program Files (x86)\Google\Chrome\Application\chrome.exe',
        (Join-Path $env:LOCALAPPDATA 'Google\Chrome\Application\chrome.exe'),
        'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe',
        'C:\Program Files\Microsoft\Edge\Application\msedge.exe')
    foreach ($p in $list) { if ($p -and (Test-Path $p)) { return $p } }
    return $null
}

function Start-Kiosk($cfg, $profiles, $browser, $i, $screen) {
    $b = $screen.Bounds
    $prof = Join-Path $profiles "profile-$i"
    $url = $cfg.url + '?kiosk=' + $cfg.kiosk + '&monitor=' + $i
    $cargs = @(
        "--user-data-dir=`"$prof`"",
        '--kiosk', '--no-first-run', '--no-default-browser-check', '--noerrdialogs', '--disable-infobars',
        '--disable-session-crashed-bubble', '--disable-features=Translate,InfiniteSessionRestore',
        '--autoplay-policy=no-user-gesture-required',
        "--window-position=$($b.X),$($b.Y)", "--window-size=$($b.Width),$($b.Height)"
    )
    # a page over plain HTTP in the local network needs to count as secure for wake lock and full screen
    if ($cfg.origin -like 'http://*') { $cargs += "--unsafely-treat-insecure-origin-as-secure=$($cfg.origin)" }
    $cargs += $url
    # a profile that crashed would otherwise ask to restore its pages
    $pref = Join-Path $prof 'Default\Preferences'
    if (Test-Path $pref) {
        (Get-Content $pref -Raw) -replace '"exit_type":"Crashed"', '"exit_type":"Normal"' -replace '"exited_cleanly":false', '"exited_cleanly":true' | Set-Content $pref -Encoding UTF8
    }
    Start-Process -FilePath $browser -ArgumentList $cargs
    Log "started monitor $i at $($b.X),$($b.Y) $($b.Width)x$($b.Height)"
}

Add-Type -TypeDefinition @'
using System; using System.Runtime.InteropServices;
public static class Ctrl32AppBar {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int l, t, r, b; }
  [StructLayout(LayoutKind.Sequential)] public struct APPBARDATA { public int cbSize; public IntPtr hWnd; public uint uCallbackMessage, uEdge; public RECT rc; public IntPtr lParam; }
  [DllImport("shell32.dll")] static extern IntPtr SHAppBarMessage(uint msg, ref APPBARDATA d);
  [DllImport("user32.dll")] static extern IntPtr FindWindow(string c, string w);
  public static void AutoHide() {
    var d = new APPBARDATA(); d.cbSize = Marshal.SizeOf(d); d.hWnd = FindWindow("Shell_TrayWnd", null);
    if (((long)SHAppBarMessage(4, ref d) & 1) == 0) { d.lParam = (IntPtr)1; SHAppBarMessage(10, ref d); } // ABM_GETSTATE / ABM_SETSTATE
  }
}
'@

New-Item -ItemType Directory -Force -Path $udir | Out-Null
Start-Sleep -Seconds 15   # let the network and the monitors settle after the logon
Log "launcher started as $env:USERNAME"
[Ctrl32AppBar]::AutoHide()
while ($true) {
    $cfg = Read-Json (Join-Path $base 'launcher.json')
    $loc = Read-Json (Join-Path $base 'local.json')
    $profiles = if ($loc -and $loc.profiles) { $loc.profiles } else { Join-Path $udir 'profiles' }
    $browser = Find-Browser ($(if ($loc) { $loc.browser } else { $null }))
    # monitors from the top left, row by row (stacked monitors: profile-0 = the top one)
    $screens = @([System.Windows.Forms.Screen]::AllScreens | Sort-Object { $_.Bounds.Y }, { $_.Bounds.X })
    $running = @(Get-CimInstance Win32_Process -Filter "Name='chrome.exe' OR Name='msedge.exe'" | ForEach-Object { $_.CommandLine })
    $want = $screens.Count
    if ($cfg -and $cfg.monitors -gt 0) { $want = [Math]::Min($screens.Count, [int]$cfg.monitors) }
    $up = @()
    if ($cfg -and $cfg.url -and $browser) {
        for ($i = 0; $i -lt $want; $i++) {
            $tag = "profile-$i"
            if (-not ($running | Where-Object { $_ -like "*$tag*" })) { Start-Kiosk $cfg $profiles $browser $i $screens[$i]; Start-Sleep -Seconds 5 }
            else { $up += $i }
        }
    } elseif (-not $browser) { Log 'no Chrome or Edge found' }
    # what the agent reports to the server (it runs as SYSTEM and cannot see this session's monitors)
    $state = @{ at = (Get-Date).ToUniversalTime().ToString('o'); user = $env:USERNAME; browser = $browser; running = $up
        screens = @($screens | ForEach-Object { @{ x = $_.Bounds.X; y = $_.Bounds.Y; w = $_.Bounds.Width; h = $_.Bounds.Height; primary = $_.Primary } }) }
    $state | ConvertTo-Json -Depth 4 -Compress | Set-Content (Join-Path $udir 'launcher-state.json') -Encoding UTF8
    [Ctrl32AppBar]::AutoHide()   # Windows may show it again (explorer restart, a new monitor)
    Start-Sleep -Seconds 30
}
