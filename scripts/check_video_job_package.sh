#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="$SCRIPT_DIR/check_video_job_package.sql"

if ! command -v psql >/dev/null 2>&1; then
  echo "[ERROR] psql not found in PATH"
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <job_id> [psql connection args]"
  echo "  example: $0 22 \"postgres://user:pass@127.0.0.1:5432/emoji?sslmode=disable\""
  echo "  or: export DATABASE_URL=... && $0 22"
  exit 1
fi

JOB_ID="$1"
shift

if ! [[ "$JOB_ID" =~ ^[0-9]+$ ]] || [[ "$JOB_ID" -le 0 ]]; then
  echo "[ERROR] invalid job_id: $JOB_ID"
  exit 1
fi

if [[ $# -gt 0 ]]; then
  psql "$@" -v job_id="$JOB_ID" -f "$SQL_FILE"
  exit 0
fi

if [[ -n "${DATABASE_URL:-}" ]]; then
  psql "$DATABASE_URL" -v job_id="$JOB_ID" -f "$SQL_FILE"
  exit 0
fi

echo "[ERROR] missing connection args and DATABASE_URL is empty"
exit 1
