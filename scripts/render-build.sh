#!/usr/bin/env bash
# Render build: compile API binary only. Migrations run in preDeployCommand
# (see scripts/render-migrate.sh and render.yaml).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

mkdir -p bin
go build -o bin/api ./cmd/api
echo "Built bin/api"
