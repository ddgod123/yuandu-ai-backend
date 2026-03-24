#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DATA_AUDIT_SCRIPT="${ROOT_DIR}/scripts/cron_data_audit_daily.sh"
TRASH_SCRIPT="${ROOT_DIR}/scripts/cron_cleanup_trash_daily.sh"

mkdir -p "${ROOT_DIR}/tmp/cron"

if [[ ! -x "${DATA_AUDIT_SCRIPT}" ]]; then
  chmod +x "${DATA_AUDIT_SCRIPT}"
fi
if [[ ! -x "${TRASH_SCRIPT}" ]]; then
  chmod +x "${TRASH_SCRIPT}"
fi

CURRENT="$(crontab -l 2>/dev/null || true)"
FILTERED="$(printf '%s\n' "${CURRENT}" | grep -v 'cron_data_audit_daily.sh' | grep -v 'cron_cleanup_trash_daily.sh' || true)"

LINE1="10 3 * * * /bin/bash ${DATA_AUDIT_SCRIPT}"
LINE2="40 3 * * * /bin/bash ${TRASH_SCRIPT}"

{
  printf '%s\n' "${FILTERED}"
  printf '%s\n' "${LINE1}"
  printf '%s\n' "${LINE2}"
} | sed '/^[[:space:]]*$/N;/^\n$/D' | crontab -

echo "installed cron entries:"
crontab -l | grep -E 'cron_data_audit_daily.sh|cron_cleanup_trash_daily.sh' || true

