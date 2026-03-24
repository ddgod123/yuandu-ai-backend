#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

mkdir -p tmp/cron
TS="$(date +%Y%m%d_%H%M%S)"
RETENTION_DAYS="${TRASH_RETENTION_DAYS:-14}"
REPORT="tmp/cron/trash_cleanup_report_${TS}.json"
LOG="tmp/cron/trash_cleanup_run_${TS}.log"

{
  echo "[start] $(date '+%F %T') retention_days=${RETENTION_DAYS} report=${REPORT}"
  go run ./cmd/cleanup-trash --apply --retention-days "${RETENTION_DAYS}" --report "${REPORT}"
  echo "[done] $(date '+%F %T')"
} >> "${LOG}" 2>&1

