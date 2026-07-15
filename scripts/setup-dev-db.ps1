# Create the pdlife development database inside an existing local MariaDB
# container and run every migration into it.
#
# Safe to re-run: applied migrations are recorded in a _dev_migrations
# table and skipped next time. That table is a convenience for this script
# only — production tracks migrations by hand (see migrations/README.md),
# and the ALTER TABLE migrations are not re-runnable without it.
#
# This only ever adds a database alongside whatever else the container
# hosts — it never touches another project's data.
#
# Usage (from the repo root):
#   ./scripts/setup-dev-db.ps1
#   ./scripts/setup-dev-db.ps1 -Recreate            # drop and rebuild from scratch
#   ./scripts/setup-dev-db.ps1 -Container other-container

param(
    [string]$Container = "nhe-mariadb-dev",
    # Drops pdlife's own database and rebuilds it. Only ever touches the
    # database named by DB_NAME in .env — anything else in the container is
    # left alone. Everything in it is throwaway (cmd/seed_dev recreates the
    # test account), so this is the answer whenever migration state is
    # unclear.
    [switch]$Recreate
)

$ErrorActionPreference = "Stop"

Set-Location (Join-Path $PSScriptRoot "..")

function Fail($msg) {
    Write-Host "ERROR: $msg" -ForegroundColor Red
    exit 1
}

# ---- 1. checks -------------------------------------------------------

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Fail "docker was not found. Start Docker Desktop and try again."
}

$running = docker ps --filter "name=^/$Container$" --format "{{.Names}}"
if ($running -ne $Container) {
    Fail "container '$Container' is not running. Start it in Docker Desktop, then re-run."
}

if (-not (Test-Path ".env")) {
    Fail ".env not found. Copy .env.example to .env and fill it in first."
}

# ---- 2. read .env ----------------------------------------------------

$envVars = @{}
foreach ($line in Get-Content ".env") {
    $t = $line.Trim()
    if (-not $t -or $t.StartsWith("#") -or -not $t.Contains("=")) { continue }
    $i = $t.IndexOf("=")
    $envVars[$t.Substring(0, $i).Trim()] = $t.Substring($i + 1).Trim().Trim('"').Trim("'")
}

$dbName = $envVars["DB_NAME"]
$dbUser = $envVars["DB_USER"]
$dbPass = $envVars["DB_PASSWORD"]
$dbHost = $envVars["DB_HOST"]

foreach ($pair in @(@("DB_NAME", $dbName), @("DB_USER", $dbUser), @("DB_PASSWORD", $dbPass))) {
    if (-not $pair[1]) { Fail "$($pair[0]) is not set in .env" }
}

# The whole point is a throwaway local database. If .env is pointed at a
# real server, stop rather than seed a dev schema into it by surprise.
if ($dbHost -and $dbHost -notin @("localhost", "127.0.0.1")) {
    Fail "DB_HOST is '$dbHost', not localhost. This script is only for the local container; refusing to run."
}

# ---- 3. create database + user --------------------------------------

$containerEnv = docker inspect $Container --format '{{range .Config.Env}}{{println .}}{{end}}'
$rootLine = $containerEnv | Where-Object { $_ -like "MARIADB_ROOT_PASSWORD=*" -or $_ -like "MYSQL_ROOT_PASSWORD=*" } | Select-Object -First 1
if (-not $rootLine) {
    Fail "could not find a root password on container '$Container'."
}
$rootPw = $rootLine.Substring($rootLine.IndexOf("=") + 1)

if ($Recreate) {
    Write-Host "==> Dropping database '$dbName' (only this one)" -ForegroundColor Yellow
    $dropSql = "DROP DATABASE IF EXISTS ``$dbName``;"
    $tmpDrop = Join-Path ([System.IO.Path]::GetTempPath()) "pdlife-drop.sql"
    [System.IO.File]::WriteAllText($tmpDrop, $dropSql, (New-Object System.Text.UTF8Encoding($false)))
    docker cp $tmpDrop "${Container}:/tmp/pdlife-drop.sql" | Out-Null
    Remove-Item $tmpDrop -Force
    docker exec $Container mariadb -uroot "--password=$rootPw" -e "SOURCE /tmp/pdlife-drop.sql;"
    if ($LASTEXITCODE -ne 0) { Fail "could not drop '$dbName'." }
}

Write-Host "==> Creating database '$dbName' and user '$dbUser' in $Container"

# Backticks are doubled because ` is PowerShell's escape character inside a
# double-quoted here-string; the database name contains a hyphen and so has
# to be quoted for MariaDB.
$setupSql = @"
CREATE DATABASE IF NOT EXISTS ``$dbName`` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '$dbUser'@'%' IDENTIFIED BY '$dbPass';
ALTER USER '$dbUser'@'%' IDENTIFIED BY '$dbPass';
GRANT ALL PRIVILEGES ON ``$dbName``.* TO '$dbUser'@'%';
FLUSH PRIVILEGES;
CREATE TABLE IF NOT EXISTS ``$dbName``.``_dev_migrations`` (
  ``filename`` VARCHAR(255) NOT NULL,
  ``applied_at`` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (``filename``)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
"@

$tmpSetup = Join-Path ([System.IO.Path]::GetTempPath()) "pdlife-setup.sql"
# UTF8 without BOM — MariaDB reads a BOM as part of the first statement.
[System.IO.File]::WriteAllText($tmpSetup, $setupSql, (New-Object System.Text.UTF8Encoding($false)))
docker cp $tmpSetup "${Container}:/tmp/pdlife-setup.sql" | Out-Null
Remove-Item $tmpSetup -Force

docker exec $Container mariadb -uroot "--password=$rootPw" --default-character-set=utf8mb4 -e "SOURCE /tmp/pdlife-setup.sql;"
if ($LASTEXITCODE -ne 0) { Fail "could not create the database or user." }

# ---- 4. run migrations ----------------------------------------------

# Filename order is the intended order: the dates increase, and where two
# share a date the alphabetical tie-break already puts them right
# (create before rename, create before fix).
$migrations = Get-ChildItem "migrations/*.sql" | Sort-Object Name

$appliedRaw = docker exec $Container mariadb -uroot "--password=$rootPw" -N -B $dbName -e "SELECT filename FROM ``_dev_migrations``;"
$applied = @{}
foreach ($f in ($appliedRaw -split "`n")) {
    $f = $f.Trim()
    if ($f) { $applied[$f] = $true }
}

$todo = $migrations | Where-Object { -not $applied.ContainsKey($_.Name) }

# A database built before this script started tracking migrations has the
# tables but an empty _dev_migrations, so every migration looks pending.
# Replaying them would die on the first ALTER ("duplicate column"), which
# is a confusing way to find out. There is no way to work out after the
# fact which ones ran, and the data here is throwaway, so say so plainly.
$alreadyBuilt = docker exec $Container mariadb -uroot "--password=$rootPw" -N -B -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = '$dbName' AND table_name = 'users';"
if ($applied.Count -eq 0 -and $alreadyBuilt.Trim() -eq "1") {
    Fail @"
'$dbName' already has tables but no migration history — it predates this
script's tracking.

Rebuild it (the data in it is throwaway):
    ./scripts/setup-dev-db.ps1 -Recreate
    go run ./cmd/seed_dev
"@
}

if ($todo.Count -eq 0) {
    Write-Host "==> All $($migrations.Count) migrations already applied"
} else {
    Write-Host "==> Running $($todo.Count) new migration(s) ($($applied.Count) already applied)"
}

foreach ($m in $todo) {
    Write-Host "    $($m.Name)"
    # docker cp rather than a pipe: the migrations contain Thai text (ENUM
    # values), and piping through PowerShell would re-encode it.
    docker cp $m.FullName "${Container}:/tmp/pdlife-migration.sql" | Out-Null
    docker exec $Container mariadb -uroot "--password=$rootPw" --default-character-set=utf8mb4 $dbName -e "SOURCE /tmp/pdlife-migration.sql;"
    if ($LASTEXITCODE -ne 0) { Fail "migration failed: $($m.Name)" }

    $escaped = $m.Name.Replace("'", "''")
    docker exec $Container mariadb -uroot "--password=$rootPw" $dbName -e "INSERT INTO ``_dev_migrations`` (filename) VALUES ('$escaped');"
    if ($LASTEXITCODE -ne 0) { Fail "could not record migration as applied: $($m.Name)" }
}

docker exec $Container rm -f /tmp/pdlife-setup.sql /tmp/pdlife-migration.sql | Out-Null

# ---- 5. report -------------------------------------------------------

$tables = docker exec $Container mariadb -uroot "--password=$rootPw" -N -B -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = '$dbName';"
Write-Host ""
Write-Host "==> Done. '$dbName' now has $($tables.Trim()) tables." -ForegroundColor Green
Write-Host "    Run the app with:  go run ."
Write-Host ""
Write-Host "    Food Check starts empty. To load its dataset:  go run ./cmd/migrate_foodcheck"
