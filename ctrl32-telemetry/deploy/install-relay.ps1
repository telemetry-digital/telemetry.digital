<#
.SYNOPSIS
  ctrl32 camera relay — installer for a remote site on Windows 10/11 or Windows Server (x64).

.DESCRIPTION
  Run in an elevated PowerShell, next to relay.toml (the file shown once when the remote camera was created, with the
  local address of the camera filled in):
    Set-ExecutionPolicy -Scope Process Bypass -Force
    irm https://portal.telemetry.digital/install-relay.ps1 -OutFile install-relay.ps1; .\install-relay.ps1 -Config .\relay.toml

  What it does (idempotent, re-run to change the configuration or update the program):
    1. installs the program into C:\Program Files\ctrl32-camera-relay (downloaded and checked against SHA256SUMS,
       or -Binary PATH)
    2. copies relay.toml to C:\ProgramData\ctrl32-camera-relay\relay.toml, readable only by SYSTEM and Administrators
    3. tests it: the server is reachable and its certificate trusted, every camera answers, the server accepts each
       secret (stops on failure unless -Force)
    4. registers and starts the Windows service "ctrl32-camera-relay" (automatic start, restarts on failure)
  -Check only tests the installed configuration. -Uninstall removes the service (-Purge also the files).
#>
[CmdletBinding()]
param(
  [string]$Config = "",
  [string]$InstallDir = "$env:ProgramFiles\ctrl32-camera-relay",
  [string]$DataDir = "$env:ProgramData\ctrl32-camera-relay",
  [string]$Binary = "",
  [string]$Url = "https://portal.telemetry.digital/dl/ctrl32-telemetry-windows-amd64.exe",
  [switch]$Update,
  [switch]$Force,
  [switch]$Check,
  [switch]$Uninstall,
  [switch]$Purge
)
$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Log($msg) { Write-Host "[relay] $msg" -ForegroundColor Cyan }
function Warn($msg) { Write-Host "[relay] $msg" -ForegroundColor Yellow }
function Die($msg) { Write-Host "[relay] error: $msg" -ForegroundColor Red; exit 1 }

$Exe = Join-Path $InstallDir "ctrl32-telemetry.exe"
$Cfg = Join-Path $DataDir "relay.toml"
$Service = "ctrl32-camera-relay"

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { Die "run this script in an elevated PowerShell (Run as administrator)" }
if ([Environment]::Is64BitOperatingSystem -eq $false) { Die "64-bit Windows is required" }

function Test-Relay {
  & $Exe camera-relay -check -config $Cfg
  return ($LASTEXITCODE -eq 0)
}

if ($Uninstall) {
  if (Test-Path $Exe) {
    & $Exe service stop-relay -config $Cfg 2>$null | Out-Null
    & $Exe service uninstall-relay -config $Cfg 2>$null | Out-Null
  }
  Log "service removed"
  if ($Purge) {
    Remove-Item -Recurse -Force $InstallDir, $DataDir -ErrorAction SilentlyContinue
    Log "program and configuration removed"
  } else {
    Log "configuration kept in $DataDir (use -Purge to remove it)"
  }
  exit 0
}
if ($Check) {
  if (-not (Test-Path $Cfg)) { Die "not installed ($Cfg missing)" }
  if (Test-Relay) { exit 0 } else { exit 1 }
}
if ($Config -eq "" -and -not (Test-Path $Cfg)) { Die "-Config relay.toml is required on the first run" }
if ($Config -ne "" -and -not (Test-Path $Config)) { Die "$Config not found" }

# 1. program ---------------------------------------------------------------------------------------------
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$running = [bool](Get-Service -Name $Service -ErrorAction SilentlyContinue | Where-Object Status -eq Running)
if ($Binary -ne "" -or -not (Test-Path $Exe) -or $Update -or $PSBoundParameters.ContainsKey("Url")) {
  $tmp = Join-Path $env:TEMP ("ctrl32-relay-" + [guid]::NewGuid() + ".exe")
  if ($Binary -ne "") {
    Copy-Item $Binary $tmp
  } else {
    Log "downloading $Url"
    Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $tmp
    $sumsUrl = $Url.Substring(0, $Url.LastIndexOf("/")) + "/SHA256SUMS"
    $file = $Url.Substring($Url.LastIndexOf("/") + 1)
    try { $sums = (Invoke-WebRequest -UseBasicParsing -Uri $sumsUrl).Content } catch { $sums = $null }
    if ($sums) {
      if ($sums -is [byte[]]) { $sums = [Text.Encoding]::ASCII.GetString($sums) }
      $want = ($sums -split "`n" | Where-Object { $_ -match ("\s\*?" + [regex]::Escape($file) + "\s*$") } | ForEach-Object { ($_ -split "\s+")[0] }) | Select-Object -First 1
      if (-not $want) { Remove-Item $tmp; Die "$file is not listed in SHA256SUMS" }
      $have = (Get-FileHash -Algorithm SHA256 $tmp).Hash.ToLower()
      if ($have -ne $want.ToLower()) { Remove-Item $tmp; Die "checksum mismatch for $Url — not installed" }
      Log "checksum verified"
    } else {
      Warn "no SHA256SUMS next to $Url; checksum not verified"
    }
    $sig = Get-AuthenticodeSignature $tmp
    if ($sig.Status -ne "Valid") { Warn "the program is not code-signed ($($sig.Status)); Windows SmartScreen may ask before it runs" }
  }
  if ($running) { Stop-Service -Name $Service -Force }
  Move-Item -Force $tmp $Exe
  Log "program installed as $Exe"
}
& $Exe version

# 2. configuration ---------------------------------------------------------------------------------------
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
& icacls $DataDir /inheritance:r /grant:r "SYSTEM:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null
if ($Config -ne "") {
  Copy-Item -Force $Config $Cfg
  & icacls $Cfg /inheritance:r /grant:r "SYSTEM:F" "*S-1-5-32-544:F" | Out-Null
  Log "configuration installed as $Cfg (readable only by SYSTEM and Administrators)"
  Warn "the file you passed ($Config) still holds the camera secrets: delete it when done"
}

# 3. test ------------------------------------------------------------------------------------------------
Log "testing the configuration"
if (-not (Test-Relay)) {
  if (-not $Force) { Die "the test failed (see above); fix relay.toml and run again, or -Force to install anyway" }
  Warn "installing despite the failed test (-Force)"
}

# 4. service ---------------------------------------------------------------------------------------------
if (-not (Get-Service -Name $Service -ErrorAction SilentlyContinue)) {
  & $Exe service install-relay -config $Cfg
  if ($LASTEXITCODE -ne 0) { Die "service registration failed" }
}
& sc.exe failure $Service reset= 86400 actions= restart/5000/restart/5000/restart/30000 | Out-Null
Set-Service -Name $Service -StartupType Automatic
Restart-Service -Name $Service -Force -ErrorAction SilentlyContinue
if (-not (Get-Service -Name $Service | Where-Object Status -eq Running)) { Start-Service -Name $Service }
Start-Sleep -Seconds 2
if ((Get-Service -Name $Service).Status -eq "Running") {
  Log "service $Service is running — the cameras appear online on the server within seconds"
  Log "test again: .\install-relay.ps1 -Check    logs: Event Viewer -> Windows Logs -> Application (source $Service)"
} else {
  Die "the service did not start; see Event Viewer -> Windows Logs -> Application"
}
