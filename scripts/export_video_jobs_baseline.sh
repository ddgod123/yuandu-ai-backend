#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://127.0.0.1:5050}"
WINDOW="${WINDOW:-24h}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
OUT_PATH="${1:-}"

if [[ -z "${ADMIN_TOKEN}" ]]; then
  echo "ERROR: ADMIN_TOKEN is required." >&2
  echo "Usage: ADMIN_TOKEN=<jwt> WINDOW=24h|7d|30d API_BASE=http://127.0.0.1:5050 $0 [output.json]" >&2
  exit 1
fi

if [[ -z "${OUT_PATH}" ]]; then
  ts="$(date +%Y%m%d-%H%M%S)"
  OUT_PATH="tmp/video-jobs-baseline-${WINDOW}-${ts}.json"
fi

mkdir -p "$(dirname "${OUT_PATH}")"
url="${API_BASE%/}/api/admin/video-jobs/overview?window=${WINDOW}"

curl -fsSL \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${url}" \
  | python3 - "${WINDOW}" "${API_BASE}" > "${OUT_PATH}" <<'PY'
import datetime
import json
import sys

window = sys.argv[1]
api_base = sys.argv[2].rstrip("/")
overview = json.load(sys.stdin)

snapshot = {
    "captured_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    "window": window,
    "api_base": api_base,
    "overview": overview,
}
json.dump(snapshot, sys.stdout, ensure_ascii=False, indent=2)
print()
PY

echo "Baseline snapshot exported: ${OUT_PATH}"

