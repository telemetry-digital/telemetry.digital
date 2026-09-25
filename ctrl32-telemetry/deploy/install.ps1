<#
.SYNOPSIS
  ctrl32 telemetry — one-command installer for Windows Server / Windows 10+ (x64, PowerShell 5.1 or newer).

.DESCRIPTION
  Run in an elevated PowerShell:
    Set-ExecutionPolicy -Scope Process Bypass -Force
    irm https://telemetry.digital/install.ps1 -OutFile install.ps1; .\install.ps1
  or with options:
    .\install.ps1 -OrgName "Acme s.r.o." -PgPort 5432 -HttpPort 8080
  Internet profile with your own certificate (no reverse proxy on Windows):
    .\install.ps1 -Domain telemetry.example.com -TlsCert C:\certs\fullchain.pem -TlsKey C:\certs\privkey.pem

  What it does (idempotent, re-run safe):
    1. creates C:\Program Files\ctrl32-telemetry (binary, typst, PostgreSQL) and C:\ProgramData\ctrl32-telemetry (config, data, logs, backups)
    2. installs the single binary (downloaded, or -Binary PATH for a local build) and Typst (PDF reports)
    3. installs PostgreSQL 18 from the official EDB binaries into the install directory as service ctrl32-telemetry-postgres
       (skipped when -PgHost points at an existing server), creates the database and roles with random passwords
    4. writes config.toml (secrets kept on re-run), restricts the data directory to SYSTEM and Administrators
    5. applies schema migrations, registers and starts the Windows service "ctrl32-telemetry", opens firewall ports
    6. creates the first administrator; the one-time password is saved in admin-bootstrap.txt (delete after first login)
  -Uninstall removes the services and firewall rules (data stays unless -Purge). -DryRun prints the plan only.
#>
[CmdletBinding()]
param(
  [string]$InstallDir = "$env:ProgramFiles\ctrl32-telemetry",
  [string]$DataDir = "$env:ProgramData\ctrl32-telemetry",
  [string]$Binary = "",
  [string]$Url = "https://telemetry.digital/dl/ctrl32-telemetry-windows-amd64.exe",
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
  [string]$OrgName = "My organization",
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
function Rand32 { $b = New-Object byte[] 24; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); ([Convert]::ToBase64String($b) -replace '[/+=]', '').Substring(0, 30) }
function Step($desc, [scriptblock]$action) { if ($DryRun) { Write-Host "[dry-run] $desc" -ForegroundColor DarkGray } else { Log $desc; & $action } }
function TomlPath($p) { return ($p -replace '\\', '/') }

$Exe = Join-Path $InstallDir "ctrl32-telemetry.exe"
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
if (-not $isAdmin -and -not $DryRun) { Die "run this script in an elevated PowerShell (Run as administrator)" }
if ([Environment]::Is64BitOperatingSystem -eq $false) { Die "64-bit Windows is required" }

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
    if ($Domain -ne "") {
      if ($TlsCert -eq "" -or $TlsKey -eq "") { Die "-Domain requires -TlsCert and -TlsKey (PEM); there is no reverse proxy on Windows" }
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
} else { Log "keeping existing $Config" }

# ---- 5. migrations, service, firewall -------------------------------------------------------------------
Step "applying schema migrations" { & $Exe migrate -config $Config }
Step "registering Windows service $AppService" {
  if (-not (Get-Service -Name $AppService -ErrorAction SilentlyContinue)) { & $Exe service install -config $Config }
}
Step "registering Windows service $AgentService (backups, updates, restarts from the UI)" {
  if (-not (Get-Service -Name $AgentService -ErrorAction SilentlyContinue)) { & $Exe service install-agent -config $Config }
  New-Item -ItemType Directory -Force -Path (Join-Path $DataDir 'backups') | Out-Null
}
Step "opening firewall ports" {
  $ports = @(@("HTTP", $(if ($Domain -ne "") { 443 } else { $HttpPort })), @("MQTT", $MqttPort))
  if ($Domain -ne "") { $ports += ,@("MQTT TLS", 8883) }
  foreach ($p in $ports) {
    $name = "ctrl32 telemetry $($p[0])"
    if (-not (Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue)) { New-NetFirewallRule -DisplayName $name -Direction Inbound -Protocol TCP -LocalPort $p[1] -Action Allow | Out-Null }
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
if ($Domain -ne "") { Log "UI: https://$Domain/  ·  MQTT: ${Domain}:8883 (TLS)" } else { Log "UI: http://$([System.Net.Dns]::GetHostName()):$HttpPort/  ·  MQTT: port $MqttPort (intranet, no TLS)" }
