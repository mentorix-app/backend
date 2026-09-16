#!/usr/bin/env bash
# Shared helpers for reading local env files from shell scripts.
#
# Precedence mirrors cmd/api: a variable already exported in the shell wins,
# then .env.local (personal, git-ignored), then .env (committed defaults).
# Source this file from a script whose cwd is the repository root.

# env_file_value FILE KEY — print KEY's value from FILE, or nothing.
env_file_value() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 0
  grep -E "^${key}=" "$file" | head -n1 | cut -d= -f2- | tr -d '\r' \
    | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

# env_value KEY — resolve KEY with the precedence above; print the value or nothing.
env_value() {
  local key="$1" value
  value="${!key:-}"
  if [[ -z "$value" ]]; then
    value="$(env_file_value .env.local "$key")"
  fi
  if [[ -z "$value" ]]; then
    value="$(env_file_value .env "$key")"
  fi
  printf '%s' "$value"
}

# require_env_files — fail with a hint when neither env file exists.
require_env_files() {
  if [[ ! -f .env && ! -f .env.local ]]; then
    echo "Missing .env and .env.local. Run 'make setup' (copies .env.example to .env.local)." >&2
    return 1
  fi
}
