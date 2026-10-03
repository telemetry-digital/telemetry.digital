<#
.SYNOPSIS
  ctrl32 telemetry — one-command installer for Windows Server / Windows 10+ (x64, PowerShell 5.1 or newer).

.DESCRIPTION
  Run in an elevated PowerShell:
    Set-ExecutionPolicy -Scope Process Bypass -Force
    irm https://portal.telemetry.digital/install.ps1 -OutFile install.ps1; .\install.ps1
  or with options:
    .\install.ps1 -OrgName "Acme s.r.o." -PgPort 5432 -HttpPort 8080
  Internet profile with your own certificate (no reverse proxy on Windows):
    .\install.ps1 -Domain telemetry.example.com -TlsCert C:\certs\fullchain.pem -TlsKey C:\certs\privkey.pem
  Reachable from anywhere through a Cloudflare Tunnel (no open ports, no certificate, no public IP; docs/cloudflare-tunnel.md):
    .\install.ps1 -Domain kamery.zakaznik.sk -CloudflareToken eyJhIjoi...

  What it does (idempotent, re-run safe):
    1. creates C:\Program Files\ctrl32-telemetry (binary, typst, PostgreSQL) and C:\ProgramData\ctrl32-telemetry (config, data, logs, backups)
    2. installs the single binary (downloaded, or -Binary PATH for a local build) and Typst (PDF reports)
    3. installs PostgreSQL 18 from the official EDB binaries into the install directory as service ctrl32-telemetry-postgres
       (skipped when -PgHost points at an existing server), creates the database and roles with random passwords
    4. writes config.toml (secrets kept on re-run), restricts the data directory to SYSTEM and Administrators
    5. applies schema migrations, registers and starts the Windows service "ctrl32-telemetry", opens firewall ports
    6. creates the first administrator; the one-time password is saved in admin-bootstrap.txt (delete after first login)
  7. with -CloudflareToken: installs cloudflared as the Windows service "Cloudflared" connected to your tunnel; the server
     listens on 127.0.0.1 only and no firewall port is opened (profile internet: TLS by Cloudflare, MFA for everyone)
  8. with -Analytics: object counting on camera pictures (docs/object-counting.md, ADR-0022): downloads an unmodified
     LGPL build of ffmpeg, ONNX Runtime (MIT) and the YOLOX nano/tiny models (Apache-2.0) from their official releases
     (pinned versions, SHA-256 verified; never part of our release files), writes [analytics] and runs the self-check.
     On an existing installation -Analytics adds only these parts (with -Binary or -Url it also updates the binary):
       .\install.ps1 -Analytics
  -Uninstall removes the services and firewall rules (data stays unless -Purge). -DryRun prints the plan only.
#>
[CmdletBinding()]
param(
  [string]$InstallDir = "$env:ProgramFiles\ctrl32-telemetry",
  [string]$DataDir = "$env:ProgramData\ctrl32-telemetry",
  [string]$Binary = "",
  [string]$Url = "https://portal.telemetry.digital/dl/ctrl32-telemetry-windows-amd64.exe",
  [string]$TypstVersion = "v0.15.1",
  [string]$PgVersion = "18.1-1",
  [string]$PgHost = "",
  [int]$PgPort = 5432,
  [string]$PgAdminUser = "postgres",
  [string]$PgAdminPassword = "",
  [int]$HttpPort = 8080,
  [int]$MqttPort = 1883,
  [string]$Domain = "",
  [string]$TlsCert = "",
  [string]$TlsKey = "",
  [string]$CloudflareToken = "",
  [string]$CloudflaredUrl = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe",
  [string]$OrgName = "My organization",
  [switch]$Analytics,
  [switch]$NoStart,
  [switch]$DryRun,
  [switch]$Uninstall,
  [switch]$Purge
)
$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Log($msg) { Write-Host "[ctrl32] $msg" -ForegroundColor Cyan }
function Warn($msg) { Write-Host "[ctrl32] $msg" -ForegroundColor Yellow }
function Die($msg) { Write-Host "[ctrl32] error: $msg" -ForegroundColor Red; exit 1 }
function Rand32 { $b = New-Object byte[] 48; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); ([Convert]::ToBase64String($b) -replace '[/+=]', '').Substring(0, 30) }
function Step($desc, [scriptblock]$action) { if ($DryRun) { Write-Host "[dry-run] $desc" -ForegroundColor DarkGray } else { Log $desc; & $action } }
function TomlPath($p) { return ($p -replace '\\', '/') }

# ---- object counting (-Analytics, ADR-0022) ---------------------------------------------------------------
# Third-party parts downloaded from their official releases at install time, pinned and SHA-256 verified.
# They are never bundled into our release files.
$OrtVersion = "1.30.0"                                   # ONNX Runtime (MIT), github.com/microsoft/onnxruntime
$OrtSha256 = "c6ba983baf5681af108599675d2a89c2d145512d02de28aed0bff177cd0ba949"     # onnxruntime-win-x64-1.30.0.zip
$FfmpegRelease = "autobuild-2026-08-31-13-27"            # github.com/BtbN/FFmpeg-Builds (month-end builds are kept)
$FfmpegBuild = "ffmpeg-n8.1.2-50-g1a748fe2cd"            # FFmpeg 8.1.2, LGPL v3 build (no --enable-gpl)
$FfmpegBranch = "8.1"
$FfmpegSha256 = "e9712ffbdb03ef71bbab660c75b835bfe698ef6fad0247c76d8d394a39a3db63"  # ...-win64-lgpl-shared-8.1.zip
$YoloxRelease = "0.1.1rc0"                               # github.com/Megvii-BaseDetection/YOLOX (Apache-2.0)
$YoloxNanoSha256 = "c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d"
$YoloxTinySha256 = "427cc366d34e27ff7a03e2899b5e3671425c262ea2291f88bb942bc1cc70b0f7"
$YoloxLicenseSha256 = "577c03d505ec80f667ebf96ebd0cc4f6825c817ca1088ee348c48aaabd51bd92"

# Fetch: download over HTTPS only and keep the file only when its SHA-256 matches the pin
function Fetch($url, $sha, $dest) {
  if ((Test-Path $dest) -and ((Get-FileHash -Algorithm SHA256 -Path $dest).Hash -eq $sha.ToUpperInvariant())) { return }
  if (-not $url.StartsWith("https://")) { Die "only HTTPS downloads are allowed: $url" }
  $part = "$dest.part"
  $old = $ProgressPreference; $ProgressPreference = "SilentlyContinue"   # the progress bar makes PowerShell 5.1 very slow
  try { Invoke-WebRequest -Uri $url -OutFile $part -UseBasicParsing } catch { Remove-Item -Force $part -ErrorAction SilentlyContinue; Die "download failed: $url ($($_.Exception.Message))" } finally { $ProgressPreference = $old }
  if ((Get-FileHash -Algorithm SHA256 -Path $part).Hash -ne $sha.ToUpperInvariant()) {
    Remove-Item -Force $part; Die "SHA-256 of $url does not match the pinned $sha - nothing installed from it"
  }
  Move-Item -Force $part $dest
}

# Expand-Selected: extract the entries of a ZIP (without its top directory) that $want accepts
function Expand-Selected($zip, $dest, [scriptblock]$want) {
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  $z = [IO.Compression.ZipFile]::OpenRead($zip)
  try {
    foreach ($e in $z.Entries) {
      if ($e.Name -eq "") { continue }
      $i = $e.FullName.IndexOf('/'); $rel = $e.FullName.Substring($i + 1)
      if (-not (& $want $rel)) { continue }
      $out = Join-Path $dest ($rel -replace '/', '\')
      New-Item -ItemType Directory -Force -Path (Split-Path $out) | Out-Null
      [IO.Compression.ZipFileExtensions]::ExtractToFile($e, $out, $true)
    }
  } finally { $z.Dispose() }
}

# Write-AnalyticsConfig: replace the [analytics] section of config.toml (keeps threads and max_cameras)
function Write-AnalyticsConfig($ffmpeg, $ortLib, $models) {
  $bytes = [IO.File]::ReadAllBytes($Config)
  $bom = ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
  $lines = [IO.File]::ReadAllText($Config) -split "`r?`n"
  $out = New-Object System.Collections.Generic.List[string]
  $in = $false; $threads = $null; $max = $null
  foreach ($l in $lines) {
    if ($l -match '^\[analytics\]\s*$') { $in = $true; continue }
    if ($in -and $l -match '^\[') { $in = $false }
    if ($in) {
      if ($l -match '^\s*threads\s*=') { $threads = $l }
      if ($l -match '^\s*max_cameras\s*=') { $max = $l }
      continue
    }
    $out.Add($l)
  }
  while ($out.Count -gt 0 -and $out[$out.Count - 1].Trim() -eq "") { $out.RemoveAt($out.Count - 1) }
  if (-not $threads) { $threads = "threads = 1          # inference threads per camera" }
  if (-not $max) { $max = "max_cameras = 4      # cameras analysed at the same time" }
  $out.Add("")
  $out.Add("[analytics]")
  $out.Add("# Object counting on camera pictures - written by install.ps1 -Analytics on $(Get-Date -Format s) (docs/object-counting.md).")
  $out.Add("# ffmpeg (LGPL build), ONNX Runtime $OrtVersion and YOLOX $YoloxRelease downloaded from their official releases.")
  $out.Add("ffmpeg = `"$(TomlPath $ffmpeg)`"")
  $out.Add("onnxruntime = `"$(TomlPath $ortLib)`"")
  $out.Add("model_nano = `"$(TomlPath (Join-Path $models 'yolox_nano.onnx'))`"")
  $out.Add("model_tiny = `"$(TomlPath (Join-Path $models 'yolox_tiny.onnx'))`"")
  $out.Add("nano_sha256 = `"$YoloxNanoSha256`"")
  $out.Add("tiny_sha256 = `"$YoloxTinySha256`"")
  $out.Add($threads)
  $out.Add($max)
  [IO.File]::WriteAllText($Config, (($out -join "`r`n") + "`r`n"), (New-Object Text.UTF8Encoding($bom)))
}

function Install-Analytics {
  $an = Join-Path $InstallDir "analytics"
  $models = Join-Path $DataDir "models"
  $tmp = Join-Path $env:TEMP "ctrl32-analytics"
  foreach ($d in @($an, $models, $tmp)) { if (-not (Test-Path $d)) { New-Item -ItemType Directory -Force -Path $d | Out-Null } }
  # ONNX Runtime (MIT), CPU package; LICENSE and ThirdPartyNotices.txt stay in its directory
  $odir = Join-Path $an "onnxruntime-$OrtVersion"
  $ortLib = Join-Path $odir "lib\onnxruntime.dll"
  $verFile = Join-Path $odir "VERSION_NUMBER"
  if ((Test-Path $ortLib) -and (Test-Path $verFile) -and ((Get-Content -Raw $verFile).Trim() -eq $OrtVersion)) {
    Log "ONNX Runtime $OrtVersion already installed in $odir"
  } else {
    $zip = Join-Path $tmp "onnxruntime-win-x64-$OrtVersion.zip"
    Fetch "https://github.com/microsoft/onnxruntime/releases/download/v$OrtVersion/onnxruntime-win-x64-$OrtVersion.zip" $OrtSha256 $zip
    $new = "$odir.new"; if (Test-Path $new) { Remove-Item -Recurse -Force $new }
    Expand-Selected $zip $new { param($p) ($p -like 'lib/*.dll') -or (@('LICENSE', 'ThirdPartyNotices.txt', 'VERSION_NUMBER', 'Privacy.md', 'README.md') -contains $p) }
    if (Test-Path $odir) { Remove-Item -Recurse -Force $odir }
    Move-Item $new $odir
    Remove-Item -Force $zip
    Log "installed ONNX Runtime $OrtVersion (MIT) in $odir"
  }
  # ffmpeg: an unmodified LGPL build as a separate program (ADR-0009 addendum)
  $fdir = Join-Path $an "$FfmpegBuild-lgpl"
  $ffmpeg = Join-Path $fdir "bin\ffmpeg.exe"
  $have = $false
  if (Test-Path $ffmpeg) { try { $have = ((& $ffmpeg -hide_banner -version | Out-String) -match 'ffmpeg version') } catch { $have = $false } }
  if ($have) {
    Log "ffmpeg $($FfmpegBuild.Substring(7)) (LGPL) already installed in $fdir"
  } else {
    $zip = Join-Path $tmp "$FfmpegBuild-win64-lgpl-shared-$FfmpegBranch.zip"
    Fetch "https://github.com/BtbN/FFmpeg-Builds/releases/download/$FfmpegRelease/$FfmpegBuild-win64-lgpl-shared-$FfmpegBranch.zip" $FfmpegSha256 $zip
    $new = "$fdir.new"; if (Test-Path $new) { Remove-Item -Recurse -Force $new }
    Expand-Selected $zip $new { param($p) ($p -like 'bin/*.dll') -or (@('bin/ffmpeg.exe', 'bin/ffprobe.exe', 'LICENSE.txt') -contains $p) }
    $v = (& (Join-Path $new "bin\ffmpeg.exe") -hide_banner -version | Out-String)
    if ($v -match '--enable-gpl' -or $v -match '--enable-nonfree') { Remove-Item -Recurse -Force $new; Die "the downloaded ffmpeg is not an LGPL build - not installed" }
    if (Test-Path $fdir) { Remove-Item -Recurse -Force $fdir }
    Move-Item $new $fdir
    Remove-Item -Force $zip
    Log "installed ffmpeg $($FfmpegBuild.Substring(7)) (LGPL v3, unmodified upstream build) in $fdir"
  }
  # YOLOX nano and tiny (Apache-2.0) with the licence next to them
  $yb = "https://github.com/Megvii-BaseDetection/YOLOX/releases/download/$YoloxRelease"
  Fetch "$yb/yolox_nano.onnx" $YoloxNanoSha256 (Join-Path $models "yolox_nano.onnx")
  Fetch "$yb/yolox_tiny.onnx" $YoloxTinySha256 (Join-Path $models "yolox_tiny.onnx")
  Fetch "https://raw.githubusercontent.com/Megvii-BaseDetection/YOLOX/$YoloxRelease/LICENSE" $YoloxLicenseSha256 (Join-Path $models "LICENSE-YOLOX.txt")
  Set-Content -Encoding ASCII -Path (Join-Path $models "SOURCES.txt") -Value @(
    "Object counting models (installed by install.ps1 -Analytics; not part of ctrl32 telemetry)",
    "yolox_nano.onnx  sha256 $YoloxNanoSha256  $yb/yolox_nano.onnx",
    "yolox_tiny.onnx  sha256 $YoloxTinySha256  $yb/yolox_tiny.onnx",
    "Licence: Apache-2.0 (LICENSE-YOLOX.txt), Megvii-BaseDetection/YOLOX $YoloxRelease")
  Set-Content -Encoding ASCII -Path (Join-Path $an "SOURCES.txt") -Value @(
    "Third-party programs for object counting (installed by install.ps1 -Analytics; not part of ctrl32 telemetry)",
    "ONNX Runtime $OrtVersion (MIT): https://github.com/microsoft/onnxruntime/releases/tag/v$OrtVersion - licence in onnxruntime-$OrtVersion\LICENSE",
    "ffmpeg $($FfmpegBuild.Substring(7)) LGPL v3 build: https://github.com/BtbN/FFmpeg-Builds/releases/tag/$FfmpegRelease - licence in $FfmpegBuild-lgpl\LICENSE.txt",
    "ffmpeg source: https://github.com/FFmpeg/FFmpeg (release/$FfmpegBranch) and https://github.com/BtbN/FFmpeg-Builds",
    "ffmpeg runs as a separate, unmodified program; it is not linked into ctrl32-telemetry.")
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  Write-AnalyticsConfig $ffmpeg $ortLib $models
  Log "wrote [analytics] in $Config"
  # self-check: ffmpeg version and licence, ONNX Runtime version, models (SHA-256, one test inference)
  & $Exe analytics-worker -selfcheck -config $Config
  if ($LASTEXITCODE -eq 0) {
    Log "object counting is installed. Switch it on per camera: Object counting -> add an area with its entrances -> Cameras -> Configure: draw a line, choose the entrance and the classes, enable (with a reason)."
  } else {
    Warn "the analytics self-check failed (see above); counting with sensors works, cameras stay 'not installed'"
  }
}

$Exe = Join-Path $InstallDir "ctrl32-telemetry.exe"
$Cloudflared = Join-Path $InstallDir "cloudflared.exe"
$Tunnel = $CloudflareToken -ne ""
if ($Tunnel -and $Domain -eq "") { Die "-CloudflareToken needs -Domain: the public hostname of the tunnel (Cloudflare Zero Trust -> Networks -> Tunnels -> Public hostname)" }
if ($Tunnel -and ($TlsCert -ne "" -or $TlsKey -ne "")) { Die "-CloudflareToken and -TlsCert/-TlsKey exclude each other: Cloudflare terminates TLS" }
$Typst = Join-Path $InstallDir "typst.exe"
$PgDir = Join-Path $InstallDir "pgsql"
$PgData = Join-Path $DataDir "pgdata"
$Config = Join-Path $DataDir "config.toml"
$DbEnv = Join-Path $DataDir "db.env"
$PgService = "ctrl32-telemetry-postgres"
$AgentService = "ctrl32-telemetry-agent"
$AppService = "ctrl32-telemetry"
$LocalPg = [string]::IsNullOrEmpty($PgHost)
if ($LocalPg) { $PgHost = "127.0.0.1" }

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
# -Analytics on an existing installation adds only the object counting parts (unless -Binary or -Url updates it too)
$AnalyticsOnly = $Analytics -and -not $Uninstall -and (Test-Path $Config) -and (Test-Path $Exe) -and ($Binary -eq "") -and -not $PSBoundParameters.ContainsKey('Url')
function Test-Writable($dir) { try { $f = Join-Path $dir ".ctrl32-write-test"; [IO.File]::WriteAllText($f, "x"); Remove-Item -Force $f; return $true } catch { return $false } }
$writable = $AnalyticsOnly -and (Test-Writable $InstallDir) -and (Test-Writable $DataDir)
if (-not $isAdmin -and -not $DryRun -and -not $writable) { Die "run this script in an elevated PowerShell (Run as administrator)" }
if ([Environment]::Is64BitOperatingSystem -eq $false) { Die "64-bit Windows is required" }
if ($AnalyticsOnly) {
  Log "existing installation found ($Config): adding object counting only"
  Step "installing object counting (ffmpeg LGPL build, ONNX Runtime $OrtVersion, YOLOX models)" { Install-Analytics }
  if (-not $NoStart -and -not $DryRun -and (Get-Service -Name $AppService -ErrorAction SilentlyContinue)) {
    Step "restarting service $AppService" { & $Exe service restart -config $Config }
  }
  exit 0
}

# ---- uninstall ------------------------------------------------------------------------------------------
if ($Uninstall) {
  Step "stopping and removing service $AppService" {
    if (Test-Path $Exe) { & $Exe service stop -config $Config 2>$null | Out-Null; & $Exe service uninstall -config $Config 2>$null | Out-Null }
  }
  if (Get-Service -Name $PgService -ErrorAction SilentlyContinue) {
    Step "stopping and unregistering PostgreSQL service $PgService" {
      Stop-Service -Name $PgService -Force -ErrorAction SilentlyContinue
      & (Join-Path $PgDir "bin\pg_ctl.exe") unregister -N $PgService | Out-Null
    }
  }
  if ((Get-Service -Name "Cloudflared" -ErrorAction SilentlyContinue) -and (Test-Path $Cloudflared)) {
    Step "removing the Cloudflare Tunnel service Cloudflared" { & $Cloudflared service uninstall 2>$null | Out-Null }
  }
  Step "removing firewall rules" { Get-NetFirewallRule -DisplayName "ctrl32 telemetry*" -ErrorAction SilentlyContinue | Remove-NetFirewallRule }
  if ($Purge) {
    Step "deleting $InstallDir and $DataDir (all data!)" { Remove-Item -Recurse -Force $InstallDir -ErrorAction SilentlyContinue; Remove-Item -Recurse -Force $DataDir -ErrorAction SilentlyContinue }
  } else {
    Log "kept $InstallDir and $DataDir (database, config, backups); add -Purge to delete everything"
  }
  Log "uninstalled"
  exit 0
}

# ---- 1. directories -------------------------------------------------------------------------------------
Step "creating directories" {
  foreach ($d in @($InstallDir, $DataDir, (Join-Path $DataDir "reports"), (Join-Path $DataDir "firmware"), (Join-Path $DataDir "backups"), (Join-Path $DataDir "logs"), (Join-Path $DataDir "tls"))) {
    if (-not (Test-Path $d)) { New-Item -ItemType Directory -Force -Path $d | Out-Null }
  }
  # secrets live here: SYSTEM (service account) and Administrators only
  & icacls $DataDir /inheritance:r /grant:r "SYSTEM:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null
}

# ---- 2. binary and Typst --------------------------------------------------------------------------------
Step "installing the ctrl32-telemetry binary" {
  if ($Binary -ne "") { Copy-Item -Force $Binary $Exe }
  else {
    $tmp = "$Exe.download"
    Invoke-WebRequest -Uri $Url -OutFile $tmp -UseBasicParsing
    if (Test-Path $Exe) { & $Exe service stop -config $Config 2>$null | Out-Null }
    Move-Item -Force $tmp $Exe
  }
  & $Exe version
}
Step "installing Typst $TypstVersion (PDF reports)" {
  $have = $false
  if (Test-Path $Typst) { try { $have = ((& $Typst --version) -match [regex]::Escape($TypstVersion.TrimStart('v'))) } catch { $have = $false } }
  if (-not $have) {
    $zip = Join-Path $env:TEMP "typst.zip"; $dir = Join-Path $env:TEMP "typst-extract"
    Invoke-WebRequest -Uri "https://github.com/typst/typst/releases/download/$TypstVersion/typst-x86_64-pc-windows-msvc.zip" -OutFile $zip -UseBasicParsing
    if (Test-Path $dir) { Remove-Item -Recurse -Force $dir }
    Expand-Archive -Path $zip -DestinationPath $dir -Force
    Copy-Item -Force (Get-ChildItem -Path $dir -Recurse -Filter typst.exe | Select-Object -First 1).FullName $Typst
    Remove-Item -Force $zip; Remove-Item -Recurse -Force $dir
  }
}

# ---- 3. PostgreSQL --------------------------------------------------------------------------------------
if (Test-Path $DbEnv) {
  $env_ = @{}; Get-Content $DbEnv | ForEach-Object { if ($_ -match '^([A-Z_]+)=(.*)$') { $env_[$matches[1]] = $matches[2] } }
  $DbAdminPassword = $env_["DB_ADMIN_PASSWORD"]; $DbAppPassword = $env_["DB_APP_PASSWORD"]; if ($env_["DB_PORT"]) { $PgPort = [int]$env_["DB_PORT"] }
  Log "keeping database passwords from $DbEnv"
} else {
  $DbAdminPassword = Rand32; $DbAppPassword = Rand32
  Step "writing $DbEnv" { Set-Content -Path $DbEnv -Value @("DB_PORT=$PgPort", "DB_ADMIN_PASSWORD=$DbAdminPassword", "DB_APP_PASSWORD=$DbAppPassword") -Encoding ASCII }
}
$PgBin = Join-Path $PgDir "bin"
if ($LocalPg) {
  Step "installing PostgreSQL $PgVersion from the official EDB binaries into $PgDir" {
    if (-not (Test-Path (Join-Path $PgBin "pg_ctl.exe"))) {
      $zip = Join-Path $env:TEMP "postgresql-$PgVersion-windows-x64-binaries.zip"; $dir = Join-Path $env:TEMP "pgsql-extract"
      Invoke-WebRequest -Uri "https://get.enterprisedb.com/postgresql/postgresql-$PgVersion-windows-x64-binaries.zip" -OutFile $zip -UseBasicParsing
      if (Test-Path $dir) { Remove-Item -Recurse -Force $dir }
      Expand-Archive -Path $zip -DestinationPath $dir -Force
      Move-Item -Force (Join-Path $dir "pgsql") $PgDir
      Remove-Item -Force $zip; Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
    }
  }
  Step "initializing the database cluster in $PgData" {
    if (-not (Test-Path (Join-Path $PgData "PG_VERSION"))) {
      $pw = Join-Path $env:TEMP "ctrl32-pgpw.txt"; Set-Content -Path $pw -Value $DbAdminPassword -Encoding ASCII -NoNewline
      & (Join-Path $PgBin "initdb.exe") -D $PgData -U postgres -A scram-sha-256 --pwfile=$pw -E UTF8 --locale=C | Out-Null
      Remove-Item -Force $pw
      Add-Content -Path (Join-Path $PgData "postgresql.conf") -Value @("", "# ctrl32 telemetry", "listen_addresses = '127.0.0.1'", "port = $PgPort", "log_destination = 'stderr'", "logging_collector = on", "log_directory = 'log'")
      # the service runs as LocalSystem; the cluster must be owned by it
      & icacls $PgData /inheritance:r /grant:r "SYSTEM:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null
    }
  }
  Step "registering and starting service $PgService" {
    if (-not (Get-Service -Name $PgService -ErrorAction SilentlyContinue)) {
      & (Join-Path $PgBin "pg_ctl.exe") register -N $PgService -D $PgData -S auto -w | Out-Null
    }
    Start-Service -Name $PgService
    $ok = $false
    for ($i = 0; $i -lt 60 -and -not $ok; $i++) { Start-Sleep -Seconds 1; & (Join-Path $PgBin "pg_isready.exe") -h 127.0.0.1 -p $PgPort 2>$null | Out-Null; $ok = ($LASTEXITCODE -eq 0) }
    if (-not $ok) { Die "PostgreSQL did not become ready on port $PgPort" }
  }
  $PgAdminUser = "postgres"; $PgAdminPassword = $DbAdminPassword
}
Step "creating database ctrl32 and role ctrl32" {
  $env:PGPASSWORD = $PgAdminPassword
  $psql = if ($LocalPg) { Join-Path $PgBin "psql.exe" } else { "psql" }
  $sql = @"
DO `$`$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='ctrl32') THEN CREATE ROLE ctrl32 LOGIN SUPERUSER PASSWORD '$DbAdminPassword'; END IF; END `$`$;
"@
  & $psql -h $PgHost -p $PgPort -U $PgAdminUser -d postgres -v ON_ERROR_STOP=1 -tAc $sql | Out-Null
  $exists = (& $psql -h $PgHost -p $PgPort -U $PgAdminUser -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='ctrl32'")
  if ("$exists".Trim() -ne "1") { & $psql -h $PgHost -p $PgPort -U $PgAdminUser -d postgres -tAc "CREATE DATABASE ctrl32 OWNER ctrl32" | Out-Null }
  Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue
}

# ---- 4. config ------------------------------------------------------------------------------------------
if (-not (Test-Path $Config)) {
  Step "writing $Config" {
    if ($Tunnel) {
      # behind the Cloudflare Tunnel: cloudflared connects to 127.0.0.1, Cloudflare terminates TLS
      $profile = "internet"; $base = "https://$Domain"
      $http = "listen = `"127.0.0.1:$HttpPort`"`nbase_url = `"$base`""
      $mqtt = "listen = `"127.0.0.1:$MqttPort`"`nws_listen = `"127.0.0.1:1884`""
    } elseif ($Domain -ne "") {
      if ($TlsCert -eq "" -or $TlsKey -eq "") { Die "-Domain requires -TlsCert and -TlsKey (PEM), or -CloudflareToken for a Cloudflare Tunnel" }
      Copy-Item -Force $TlsCert (Join-Path $DataDir "tls\fullchain.pem"); Copy-Item -Force $TlsKey (Join-Path $DataDir "tls\privkey.pem")
      $profile = "internet"; $listen = ":443"; $base = "https://$Domain"
      $http = "listen = `"$listen`"`nbase_url = `"$base`"`ntls_cert = `"$(TomlPath (Join-Path $DataDir 'tls\fullchain.pem'))`"`ntls_key = `"$(TomlPath (Join-Path $DataDir 'tls\privkey.pem'))`""
      $mqtt = "listen = `"127.0.0.1:$MqttPort`"`nws_listen = `"127.0.0.1:1884`"`ntls_listen = `":8883`"`ntls_cert = `"$(TomlPath (Join-Path $DataDir 'tls\fullchain.pem'))`"`ntls_key = `"$(TomlPath (Join-Path $DataDir 'tls\privkey.pem'))`""
    } else {
      $profile = "intranet"; $base = "http://$([System.Net.Dns]::GetHostName()):$HttpPort"
      $http = "listen = `":$HttpPort`"`nbase_url = `"$base`""
      $mqtt = "listen = `":$MqttPort`"`nws_listen = `":1884`""
    }
    $toml = @"
# ctrl32 telemetry configuration — generated by install.ps1 on $(Get-Date -Format s). Edit and restart the service.
profile = "$profile"

[http]
$http
public_read = false

[mqtt]
$mqtt

[db]
url = "postgres://ctrl32_app:$DbAppPassword@$PgHost`:$PgPort/ctrl32?sslmode=disable"
admin_url = "postgres://ctrl32:$DbAdminPassword@$PgHost`:$PgPort/ctrl32?sslmode=disable"
app_password = "$DbAppPassword"

[smtp]
port = 587

[log]
level = "info"
file = "$(TomlPath (Join-Path $DataDir 'logs\server.log'))"

[gap]
check_interval = "1m"

[security]
pepper = "$(Rand32)"
secret_key = "$(Rand32)"

[report]
typst_path = "$(TomlPath $Typst)"
storage_dir = "$(TomlPath (Join-Path $DataDir 'reports'))"

[fota]
storage_dir = "$(TomlPath (Join-Path $DataDir 'firmware'))"

[agent]
socket = "$(TomlPath (Join-Path $DataDir 'agent.sock'))"
pg_dump = "$(TomlPath (Join-Path $PgBin 'pg_dump.exe'))"

# Self-hosted map tiles for the map widgets (no Internet needed): a raster PMTiles archive.
# [map]
# pmtiles = "$(TomlPath (Join-Path $DataDir 'map.pmtiles'))"
# attribution = "© OpenStreetMap contributors"
"@
    Set-Content -Path $Config -Value $toml -Encoding UTF8
  }
} else {
  Log "keeping existing $Config"
  if ($Tunnel) { Warn "for the tunnel the existing config needs: profile = `"internet`", [http] listen = `"127.0.0.1:$HttpPort`", base_url = `"https://$Domain`", [mqtt] listen on 127.0.0.1 - edit $Config and restart the service" }
}

# ---- 4b. object counting (optional) ---------------------------------------------------------------------
if ($Analytics) { Step "installing object counting (ffmpeg LGPL build, ONNX Runtime $OrtVersion, YOLOX models)" { Install-Analytics } }

# ---- 5. migrations, service, firewall -------------------------------------------------------------------
Step "applying schema migrations" { & $Exe migrate -config $Config }
Step "registering Windows service $AppService" {
  if (-not (Get-Service -Name $AppService -ErrorAction SilentlyContinue)) { & $Exe service install -config $Config }
}
Step "registering Windows service $AgentService (backups, updates, restarts from the UI)" {
  if (-not (Get-Service -Name $AgentService -ErrorAction SilentlyContinue)) { & $Exe service install-agent -config $Config }
  New-Item -ItemType Directory -Force -Path (Join-Path $DataDir 'backups') | Out-Null
}
if ($Tunnel) { Log "Cloudflare Tunnel: no inbound firewall port is opened" } else { Step "opening firewall ports" {
  $ports = @(@("HTTP", $(if ($Domain -ne "") { 443 } else { $HttpPort })), @("MQTT", $MqttPort))
  if ($Domain -ne "") { $ports += ,@("MQTT TLS", 8883) }
  foreach ($p in $ports) {
    $name = "ctrl32 telemetry $($p[0])"
    if (-not (Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue)) { New-NetFirewallRule -DisplayName $name -Direction Inbound -Protocol TCP -LocalPort $p[1] -Action Allow | Out-Null }
  }
} }
if ($Tunnel) {
  Step "installing cloudflared (Cloudflare Tunnel) as the Windows service Cloudflared" {
    Invoke-WebRequest -UseBasicParsing -Uri $CloudflaredUrl -OutFile "$Cloudflared.download"
    if (Get-Service -Name "Cloudflared" -ErrorAction SilentlyContinue) { & $Cloudflared service uninstall 2>$null | Out-Null; Start-Sleep -Seconds 2 }
    Move-Item -Force "$Cloudflared.download" $Cloudflared
    & $Cloudflared service install $CloudflareToken
    if ($LASTEXITCODE -ne 0) { Die "cloudflared service install failed (check the token)" }
    Start-Sleep -Seconds 3; Get-Service -Name "Cloudflared" | Format-List Status, StartType | Out-String | Write-Host
  }
}
if (-not $NoStart) {
  Step "starting service $AgentService" { & $Exe service restart-agent -config $Config 2>$null; if ($LASTEXITCODE -ne 0) { & $Exe service start-agent -config $Config } }
  Step "starting service $AppService" { & $Exe service restart -config $Config 2>$null; if ($LASTEXITCODE -ne 0) { & $Exe service start -config $Config }; Start-Sleep -Seconds 3; Get-Service -Name $AppService | Format-List Status, StartType | Out-String | Write-Host }
}

# ---- 6. first administrator -----------------------------------------------------------------------------
Step "creating the first administrator" {
  $out = Join-Path $DataDir "admin-bootstrap.txt"
  $txt = (& $Exe admin bootstrap -config $Config -org-name $OrgName -username admin -name Administrator 2>&1 | Out-String)
  if ($txt -match "duplicate key") { Log "admin user already exists" }
  elseif ($LASTEXITCODE -ne 0) { Write-Host $txt }
  else { Set-Content -Path $out -Value $txt -Encoding UTF8; Log "one-time password of admin saved in $out (delete after the first login)" }
}

Log "done. Service: Get-Service $AppService · logs: $(Join-Path $DataDir 'logs\server.log') · config: $Config"
if ($Tunnel) { Log "UI: https://$Domain/ through the Cloudflare Tunnel (no open ports). In Cloudflare the public hostname $Domain must point to http://localhost:$HttpPort (service type HTTP)." }
elseif ($Domain -ne "") { Log "UI: https://$Domain/  ·  MQTT: ${Domain}:8883 (TLS)" } else { Log "UI: http://$([System.Net.Dns]::GetHostName()):$HttpPort/  ·  MQTT: port $MqttPort (intranet, no TLS)" }
