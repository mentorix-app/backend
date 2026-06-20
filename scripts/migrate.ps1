param(
  [Parameter(Mandatory = $false)]
  [string]$Command = "up"
)

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

Write-Host "Running migrations ($Command)..."
migrate -path "db/migrations" -database "$db" $Command
