$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not (Test-Path ".env")) {
  Write-Error "Missing .env. Copy .env.example to .env first."
}

if (-not (Get-Command migrate -ErrorAction SilentlyContinue)) {
  Write-Error "migrate CLI not found. Install with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.2"
}

$databaseUrlLine = Get-Content .env | Where-Object { $_ -match '^DATABASE_URL=' } | Select-Object -First 1
if (-not $databaseUrlLine) {
  Write-Error "DATABASE_URL is not set in .env"
}

$db = ($databaseUrlLine -replace '^DATABASE_URL=', '').Trim()
if (-not $db) {
  Write-Error "DATABASE_URL is empty in .env"
}

$expected = 0
Get-ChildItem -Path "db/migrations" -Filter "*.up.sql" | ForEach-Object {
  if ($_.Name -match '^(\d+)_') {
    $num = [int]$Matches[1]
    if ($num -gt $expected) { $expected = $num }
  }
}

Write-Host "=== Migration check ==="

try {
  $versionOut = migrate -path "db/migrations" -database $db version 2>&1 | Out-String
  $versionOut = $versionOut.Trim()
} catch {
  Write-Error "FAIL: cannot read migration version (is Postgres up? DATABASE_URL correct?): $_"
}

Write-Host "  migrate: $versionOut"

if ($versionOut -match 'dirty') {
  Write-Error "FAIL: database is in dirty state — resolve before running migrate up"
}

$current = 0
if ($versionOut -match '^(\d+)') {
  $current = [int]$Matches[1]
}

Write-Host "  DB version: $current"
Write-Host "  Expected:   $expected"

if ($current -lt $expected) {
  Write-Error "FAIL: migrations behind — run: .\scripts\migrate.ps1 up"
}

if ($current -gt $expected) {
  Write-Error "FAIL: DB version ($current) is ahead of repo ($expected)"
}

Write-Host "Status: OK (up to date)"
