# ctrl32 telemetry - kiosk agent (spec 034, ADR-0025). Runs as SYSTEM (scheduled task "ctrl32 kiosk agent"): every
# few seconds it reports the state of this PC to the server and performs what an operator asked for, from a closed
# list only - restart the browsers, reboot, report now, update these scripts, screenshot. It never runs anything the
# server sends as text. Its credential is in agent.json, readable by SYSTEM and administrators only.
$ErrorActionPreference = 'Continue'
$base = 'C:\ProgramData\ctrl32-kiosk'
$udir = Join-Path $base 'user'
$log = Join-Path $base 'agent.log'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

function Log($m) {
    if ((Test-Path $log) -and (Get-Item $log).Length -gt 1MB) { Move-Item $log "$log.1" -Force }
    Add-Content -Path $log -Value ("{0:yyyy-MM-dd HH:mm:ss} {1}" -f (Get-Date), $m)
}
function Read-Json($p) { try { return (Get-Content $p -Raw -Encoding UTF8 | ConvertFrom-Json) } catch { return $null } }
function Tail($p, $n) { if (Test-Path $p) { return @(Get-Content $p -Tail $n) } return @() }

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
    $ver = (Get-Content (Join-Path $base 'version.txt') -ErrorAction SilentlyContinue | Select-Object -First 1)
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
    $allowed = @('kiosk-agent.ps1', 'kiosk-launcher.ps1', 'capture.ps1', 'launcher.json')
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

function Send-Screenshot($seq) {
    $loc = Read-Json (Join-Path $base 'local.json')
    $user = if ($loc -and $loc.kiosk_user) { $loc.kiosk_user } else { throw 'no kiosk user in local.json' }
    $shot = Join-Path $udir 'shot.jpg'
    Remove-Item $shot -ErrorAction SilentlyContinue
    $a = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$base\capture.ps1`""
    $p = New-ScheduledTaskPrincipal -UserId "$env:COMPUTERNAME\$user" -LogonType Interactive -RunLevel Limited
    Register-ScheduledTask -TaskName 'ctrl32 kiosk screenshot' -Action $a -Principal $p -Force | Out-Null
    Start-ScheduledTask -TaskName 'ctrl32 kiosk screenshot'
    for ($i = 0; $i -lt 30 -and -not (Test-Path $shot); $i++) { Start-Sleep -Seconds 1 }
    Unregister-ScheduledTask -TaskName 'ctrl32 kiosk screenshot' -Confirm:$false -ErrorAction SilentlyContinue
    if (-not (Test-Path $shot)) { throw "no screenshot (is $user signed in?)" }
    $bytes = [IO.File]::ReadAllBytes($shot)
    Remove-Item $shot -Force
    Invoke-RestMethod -Method Post -Uri ($server + '/api/v1/kiosk/screenshot?seq=' + $seq) -Headers $headers -ContentType 'image/jpeg' -Body $bytes -TimeoutSec 60 -UseBasicParsing | Out-Null
    return "screenshot sent ($([int]($bytes.Length / 1024)) KB)"
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
Log "agent started (scripts $(Get-Content (Join-Path $base 'version.txt') -ErrorAction SilentlyContinue | Select-Object -First 1))"
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
