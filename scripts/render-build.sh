#!/usr/bin/env bash
# Render build: compile the API binary and the fingerprint refresh run by the pre-deploy step.
# Migrations run in preDeployCommand (see scripts/render-migrate.sh and render.yaml).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

mkdir -p bin
go build -o bin/api ./cmd/api
go build -o bin/refresh-fingerprints ./cmd/refresh-fingerprints
echo "Built bin/api and bin/refresh-fingerprints"
