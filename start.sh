#!/usr/bin/env bash
set -Eeuo pipefail

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root_dir"

force_recreate=false
if [[ "${1:-}" == "--force-recreate" ]]; then
  force_recreate=true
elif [[ $# -gt 0 ]]; then
  echo "Usage: ./start.sh [--force-recreate]" >&2
  exit 2
fi

load_env_file() {
  local file=$1
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*$ || "$line" =~ ^[[:space:]]*# ]] && continue
    if [[ ! "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$ ]]; then
      echo "Invalid .env entry (value omitted)." >&2
      exit 1
    fi
    key=${BASH_REMATCH[1]}
    value=${BASH_REMATCH[2]}
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    if [[ "$value" =~ ^\"(.*)\"$ || "$value" =~ ^\'(.*)\'$ ]]; then
      value=${BASH_REMATCH[1]}
    fi
    if [[ -z "${!key+x}" ]]; then
      printf -v "$key" '%s' "$value"
      export "$key"
    fi
  done < "$file"
}

if [[ -f .env ]]; then
  load_env_file .env
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_value() {
  local name=$1
  local value=${!name:-}
  if [[ -z "$value" ]]; then
    echo "HTTPS mode requires $name." >&2
    exit 1
  fi
}

require_command docker
docker compose version >/dev/null

if [[ "${TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD:-}" == *replace-with-* ]]; then
  echo "Replace the sample TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD before starting." >&2
  exit 1
fi

compose=(docker compose)
https_enabled=${TECHPEDIA_ENABLE_HTTPS:-false}

if [[ "$https_enabled" == "true" ]]; then
  for name in \
    TECHPEDIA_DOMAIN \
    TECHPEDIA_PUBLIC_URL \
    TECHPEDIA_TLS_EMAIL; do
    require_value "$name"
  done

  expected_public_url="https://${TECHPEDIA_DOMAIN}"
  if [[ "${TECHPEDIA_PUBLIC_URL%/}" != "$expected_public_url" ]]; then
    echo "TECHPEDIA_PUBLIC_URL must be $expected_public_url." >&2
    exit 1
  fi
  if [[ -z "${TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME:-}" ]]; then
    echo "Warning: no bootstrap owner username; an owner must already exist in the database." >&2
  elif [[ -n "${TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD:-}" && "${TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD}" == *replace-with-* ]]; then
    echo "Replace the sample TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD." >&2
    exit 1
  fi

  compose+=(--profile production)
  public_url=$expected_public_url
else
  public_url=${TECHPEDIA_PUBLIC_URL:-http://localhost:8080}
fi

up_args=(-d --build --remove-orphans)
if [[ "$force_recreate" == "true" ]]; then
  up_args+=(--force-recreate)
fi

"${compose[@]}" config --quiet
"${compose[@]}" up "${up_args[@]}"

for _ in {1..60}; do
  if "${compose[@]}" exec -T techpedia wget -q -O /dev/null http://127.0.0.1:8080/api/health 2>/dev/null; then
    echo "TechPedia started: $public_url"
    "${compose[@]}" ps
    exit 0
  fi
  sleep 2
done

echo "TechPedia health check timed out." >&2
"${compose[@]}" ps >&2
"${compose[@]}" logs --tail=100 techpedia >&2
exit 1
