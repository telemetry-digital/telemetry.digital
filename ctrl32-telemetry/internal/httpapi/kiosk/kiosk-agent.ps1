# ctrl32 telemetry - kiosk agent (spec 034, ADR-0025). Runs as SYSTEM (scheduled task "ctrl32 kiosk agent"): every
# few seconds it reports the state of this PC to the server and performs what an operator asked for, from a closed
# list only - restart the browsers, reboot, report now, update these scripts, screenshot of the kiosk pages. It never
# runs anything the server sends as text. Its credential is in agent.json, readable by SYSTEM and administrators only.
$ErrorActionPreference = 'Continue'
$base = 'C:\ProgramData\ctrl32-kiosk'
$udir = Join-Path $base 'user'
$log = Join-Path $base 'agent.log'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

function Log($m) {
    if ((Test-Path $log) -and (Get-Item $log).Length -gt 1MB) { Move-Item $log "$log.1" -Force }
    Add-Content -Path $log -Value ("{0:yyyy-MM-dd HH:mm:ss} {1}" -f (Get-Date), $m)
}
function Read-Json($p) { try { return (Get-Content $p -Raw -Encoding UTF8 -ErrorAction Stop | ConvertFrom-Json) } catch { return $null } }
# plain strings: lines from Get-Content carry PSPath and other note properties that ConvertTo-Json would serialize
function Tail($p, $n) { if (Test-Path $p) { return @(Get-Content $p -Tail $n | ForEach-Object { [string]$_ }) } return @() }
function Scripts-Version { $v = Get-Content (Join-Path $base 'version.txt') -ErrorAction SilentlyContinue | Select-Object -First 1; if ($v) { return [string]$v } return $null }

$cfg = Read-Json (Join-Path $base 'agent.json')
if (-not $cfg -or -not $cfg.server -or -not $cfg.token) { Log 'agent.json missing or incomplete - run the setup script again'; exit 1 }
$server = $cfg.server.TrimEnd('/')
$headers = @{ 'X-Kiosk-Token' = $cfg.token }
$statePath = Join-Path $base 'agent-state.json'
$state = Read-Json $statePath
$lastSeq = if ($state -and $state.last_seq) { [int64]$state.last_seq } else { [int64]0 }

function Post-Json($path, $obj) {
    $body = [Text.Encoding]::UTF8.GetBytes(($obj | ConvertTo-Json -Depth 6 -Compress))
    return Invoke-RestMethod -Method Post -Uri ($server + $path) -Headers $headers -ContentType 'application/json; charset=utf-8' -Body $body -TimeoutSec 30 -UseBasicParsing
}

# ---- what the PC reports --------------------------------------------------------------------------------------------
$wu = @{ pending = $null; checked = $null }
$wuJob = $null
$wuNext = (Get-Date).AddMinutes(2)
function Update-WindowsUpdates {
    # searching Windows Update takes a while: in a background job every 6 hours
    if ($script:wuJob -and $script:wuJob.State -ne 'Running') {
        $r = Receive-Job $script:wuJob -ErrorAction SilentlyContinue
        if ($r -ne $null) { $script:wu = @{ pending = [int]$r; checked = (Get-Date).ToUniversalTime().ToString('o') } }
        Remove-Job $script:wuJob -Force; $script:wuJob = $null
    }
    if (-not $script:wuJob -and (Get-Date) -ge $script:wuNext) {
        $script:wuNext = (Get-Date).AddHours(6)
        $script:wuJob = Start-Job { try { (New-Object -ComObject Microsoft.Update.Session).CreateUpdateSearcher().Search("IsInstalled=0 and IsHidden=0 and Type='Software'").Updates.Count } catch { -1 } }
    }
}

function Get-Report {
    $os = Get-CimInstance Win32_OperatingSystem
    $cs = Get-CimInstance Win32_ComputerSystem
    $cpu = (Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average
    $dec = $null
    try { $dec = [Math]::Round(((Get-Counter '\GPU Engine(*engtype_VideoDecode)\Utilization Percentage' -ErrorAction Stop).CounterSamples | Measure-Object CookedValue -Sum).Sum) } catch {}
    $disk = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='C:'"
    $loc = Read-Json (Join-Path $base 'local.json')
    $ls = Read-Json (Join-Path $udir 'launcher-state.json')
    $browserPath = if ($ls -and $ls.browser) { $ls.browser } else { 'C:\Program Files\Google\Chrome\Application\chrome.exe' }
    $browserVer = $null
    if (Test-Path $browserPath) { $browserVer = (Get-Item $browserPath).VersionInfo.ProductVersion }
    $profiles = if ($loc -and $loc.profiles) { $loc.profiles } else { Join-Path $udir 'profiles' }
    $kiosks = @(Get-CimInstance Win32_Process -Filter "Name='chrome.exe' OR Name='msedge.exe'" | Where-Object { $_.CommandLine -like '*--kiosk*' -and $_.CommandLine -like '*--user-data-dir*' }).Count
    $reboot = (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired') -or (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending')
    $ips = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } | ForEach-Object { $_.IPAddress })
    $ver = Scripts-Version
    return @{
        hostname = $env:COMPUTERNAME; windows = "$($os.Caption) $($os.Version)"; boot = $os.LastBootUpTime.ToUniversalTime().ToString('o')
        uptime_s = [int]((Get-Date) - $os.LastBootUpTime).TotalSeconds; user = $cs.UserName; model = "$($cs.Manufacturer) $($cs.Model)"
        cpu_pct = [int]$cpu; gpu_decode_pct = $dec
        ram_total_mb = [int]($os.TotalVisibleMemorySize / 1024); ram_free_mb = [int]($os.FreePhysicalMemory / 1024)
        disk_free_gb = [Math]::Round($disk.FreeSpace / 1GB, 1); disk_total_gb = [Math]::Round($disk.Size / 1GB, 1)
        browser = $browserVer; browser_windows = $kiosks; ip = $ips
        windows_updates = $script:wu.pending; windows_updates_checked = $script:wu.checked; reboot_pending = $reboot
        scripts_version = $ver; powershell = $PSVersionTable.PSVersion.ToString()
        launcher = $ls; launcher_log = @(Tail (Join-Path $udir 'launcher.log') 8); agent_log = @(Tail $log 5)
        kiosk_user = $(if ($loc) { $loc.kiosk_user } else { $null })
    }
}

# ---- the closed list of operations -----------------------------------------------------------------------------------
function Restart-Browsers {
    $p = @(Get-CimInstance Win32_Process -Filter "Name='chrome.exe' OR Name='msedge.exe'" | Where-Object { $_.CommandLine -like '*--user-data-dir*' -and $_.CommandLine -like '*profile-*' })
    foreach ($x in $p) { Stop-Process -Id $x.ProcessId -Force -ErrorAction SilentlyContinue }
    return "stopped $($p.Count) browser processes; the launcher starts the kiosks again within 30 s"
}

function Update-Scripts {
    $r = Invoke-RestMethod -Method Get -Uri ($server + '/api/v1/kiosk/scripts') -Headers $headers -TimeoutSec 60 -UseBasicParsing
    $allowed = @('kiosk-agent.ps1', 'kiosk-launcher.ps1', 'launcher.json')
    foreach ($name in $allowed) {
        $text = $r.files.$name
        if (-not $text) { throw "the server sent no $name" }
        $dst = Join-Path $base $name
        [IO.File]::WriteAllText("$dst.new", $text, (New-Object Text.UTF8Encoding $false))
        Move-Item "$dst.new" $dst -Force
    }
    Set-Content (Join-Path $base 'version.txt') $r.version -Encoding ASCII
    # the launcher starts again with the new script (its kiosks keep running)
    Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" | Where-Object { $_.CommandLine -like '*kiosk-launcher.ps1*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
    Start-ScheduledTask -TaskName 'ctrl32 kiosk launcher' -ErrorAction SilentlyContinue
    return "scripts $($r.version) installed"
}

# A screenshot comes from each kiosk browser itself (Chrome DevTools Protocol, Page.captureScreenshot) on the debugging
# port the launcher opened for that monitor on 127.0.0.1 only: just the page of the display, never the rest of the
# desktop, and no screen-capture code on this PC.
function Get-PageShot($port) {
    try { $list = Invoke-RestMethod -Uri "http://127.0.0.1:$port/json/list" -TimeoutSec 5 -UseBasicParsing }
    catch { throw "debugging port $port not reachable (the kiosk window is not running or was started by older scripts; the launcher restarts it within 30 s)" }
    # the display page, not one of the browser's own pages (chrome://...)
    $pages = @($list | Where-Object { $_.type -eq 'page' -and $_.webSocketDebuggerUrl -and $_.url -like 'http*' })
    $page = @($pages | Where-Object { $_.url -like '*/display*' }) + $pages | Select-Object -First 1
    if (-not $page) { throw "no page in the browser on port $port" }
    $ws = [System.Net.WebSockets.ClientWebSocket]::new()
    $cts = [System.Threading.CancellationTokenSource]::new(30000)
    try {
        [void]$ws.ConnectAsync([Uri]$page.webSocketDebuggerUrl, $cts.Token).GetAwaiter().GetResult()
        $req = [Text.Encoding]::UTF8.GetBytes('{"id":1,"method":"Page.captureScreenshot","params":{"format":"jpeg","quality":70}}')
        [void]$ws.SendAsync([ArraySegment[byte]]::new($req), [System.Net.WebSockets.WebSocketMessageType]::Text, $true, $cts.Token).GetAwaiter().GetResult()
        $buf = New-Object byte[] 262144
        for ($n = 0; $n -lt 20; $n++) {
            $ms = New-Object IO.MemoryStream
            do {
                $r = $ws.ReceiveAsync([ArraySegment[byte]]::new($buf), $cts.Token).GetAwaiter().GetResult()
                $ms.Write($buf, 0, $r.Count)
            } until ($r.EndOfMessage -or $r.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close)
            if ($r.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close) { throw "the browser on port $port closed the connection" }
            $text = [Text.Encoding]::UTF8.GetString($ms.ToArray())
            if ($text -notmatch '^\{"id":1,') { continue }   # an event, not the answer
            # a pattern, not ConvertFrom-Json: Windows PowerShell refuses JSON over 2 MB
            $m = [regex]::Match($text, '"data":"([A-Za-z0-9+/=]+)"')
            if ($m.Success) { return , [Convert]::FromBase64String($m.Groups[1].Value) }   # one byte[], not unrolled
            $e = [regex]::Match($text, '"message":"([^"]*)"')
            throw "the browser on port $port refused the screenshot: $($e.Groups[1].Value)"
        }
        throw "no answer from the browser on port $port"
    } finally { $ws.Dispose(); $cts.Dispose() }
}

# one picture per monitor, each sent on its own
function Send-Screenshot($seq) {
    $lc = Read-Json (Join-Path $base 'launcher.json')
    $first = if ($lc -and $lc.cdp_port) { [int]$lc.cdp_port } else { 9222 }
    $ls = Read-Json (Join-Path $udir 'launcher-state.json')
    $count = if ($ls -and $ls.screens) { @($ls.screens).Count } else { 1 }
    if ($lc -and $lc.monitors -gt 0) { $count = [Math]::Min($count, [int]$lc.monitors) }
    $sent = 0; $kb = 0; $failed = @()
    for ($i = 0; $i -lt $count; $i++) {
        try {
            $bytes = Get-PageShot ($first + $i)
            Invoke-RestMethod -Method Post -Uri ($server + '/api/v1/kiosk/screenshot?seq=' + $seq + '&monitor=' + $i) -Headers $headers -ContentType 'image/jpeg' -Body $bytes -TimeoutSec 60 -UseBasicParsing | Out-Null
            $sent++; $kb += [int]($bytes.Length / 1024)
        } catch { $failed += "monitor $($i + 1): $($_.Exception.Message)" }
    }
    if ($sent -eq 0) { throw ($failed -join '; ') }
    $msg = "screenshots of $sent of $count monitors sent ($kb KB)"
    if ($failed) { $msg += '; ' + ($failed -join '; ') }
    return $msg
}

function Invoke-Op($c) {
    switch ($c.action) {
        'report'           { return 'reported' }
        'restart_browsers' { return Restart-Browsers }
        'update_scripts'   { return Update-Scripts }
        'screenshot'       { return Send-Screenshot $c.seq }
        'reboot'           { $script:rebootBy = $c.by; return 'rebooting in 10 s' }
        default            { throw "unknown operation $($c.action)" }
    }
}

# ---- the loop ---------------------------------------------------------------------------------------------------------
Log "agent started (scripts $(Scripts-Version))"
# the screen-capture helper of the scripts before 0.86.5; an older agent that installed these scripts left a stub
Remove-Item (Join-Path $base 'capture.ps1') -Force -ErrorAction SilentlyContinue
$result = $null
$rebootBy = $null
$restartSelf = $false
while ($true) {
    $poll = 15
    try {
        Update-WindowsUpdates
        $resp = Post-Json '/api/v1/kiosk/report' @{ report = (Get-Report); result = $result }
        $result = $null
        if ($rebootBy) {
            Log "reboot requested by $rebootBy"
            & shutdown.exe /r /f /t 10 /d p:0:0 /c "ctrl32 telemetry: reboot requested by $rebootBy" | Out-Null
            Start-Sleep -Seconds 120
        }
        if ($restartSelf) { Log 'restarting with the new scripts'; exit 1 }   # the task starts it again within a minute
        if ($resp.poll_s) { $poll = [int]$resp.poll_s }
        $c = $resp.command
        if ($c -and [int64]$c.seq -gt $lastSeq) {
            $lastSeq = [int64]$c.seq
            @{ last_seq = $lastSeq } | ConvertTo-Json | Set-Content $statePath -Encoding ASCII
            Log "operation $($c.action) by $($c.by)"
            try { $msg = Invoke-Op $c; $result = @{ seq = $c.seq; action = $c.action; ok = $true; message = $msg } }
            catch { $result = @{ seq = $c.seq; action = $c.action; ok = $false; message = $_.Exception.Message }; Log "operation $($c.action) failed: $($_.Exception.Message)" }
            if ($c.action -eq 'update_scripts' -and $result.ok) { $restartSelf = $true }
            continue   # report the result at once
        }
    } catch { Log "report failed: $($_.Exception.Message)"; $poll = 30 }
    Start-Sleep -Seconds $poll
}
