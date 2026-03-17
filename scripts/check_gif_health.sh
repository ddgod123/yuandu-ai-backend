#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="$SCRIPT_DIR/check_gif_health.sql"

if ! command -v psql >/dev/null 2>&1; then
  echo "[ERROR] psql not found in PATH"
  exit 1
fi

if [[ $# -gt 0 ]]; then
  # 示例：./check_gif_health.sh "$DATABASE_URL"
  psql "$@" -f "$SQL_FILE"
  exit 0
fi

if [[ -n "${DATABASE_URL:-}" ]]; then
  psql "$DATABASE_URL" -f "$SQL_FILE"
  exit 0
fi

echo "Usage: $0 <psql connection args>"
echo "  example: $0 \"postgres://user:pass@127.0.0.1:5432/emoji?sslmode=disable\""
echo "  or export DATABASE_URL and run: $0"
exit 1
