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
REPORT="tmp/cron/data_audit_report_${TS}.json"
LOG="tmp/cron/data_audit_run_${TS}.log"

{
  echo "[start] $(date '+%F %T') report=${REPORT}"
  go run ./cmd/data-audit --report "${REPORT}"
  echo "[done] $(date '+%F %T')"
} >> "${LOG}" 2>&1

