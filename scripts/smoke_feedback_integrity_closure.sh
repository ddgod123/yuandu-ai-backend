#!/usr/bin/env bash
set -euo pipefail

# 反馈完整性运维闭环 API 冒烟脚本
# 用法：
#   API_BASE=http://127.0.0.1:5050 ADMIN_BEARER_TOKEN=xxx \
#   ./backend/scripts/smoke_feedback_integrity_closure.sh
#
# 可选过滤：
#   WINDOW=24h|7d|30d
#   USER_ID=123
#   FORMAT=gif
#   GUARD_REASON=low_confidence

if ! command -v curl >/dev/null 2>&1; then
  echo "[ERROR] curl not found in PATH"
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[ERROR] jq not found in PATH"
  exit 1
fi

API_BASE="${API_BASE:-http://127.0.0.1:5050}"
WINDOW="${WINDOW:-24h}"
USER_ID="${USER_ID:-}"
FORMAT="${FORMAT:-}"
GUARD_REASON="${GUARD_REASON:-}"

AUTH_ARGS=()
if [[ -n "${ADMIN_BEARER_TOKEN:-}" ]]; then
  AUTH_ARGS=(-H "Authorization: Bearer ${ADMIN_BEARER_TOKEN}")
elif [[ -n "${ADMIN_COOKIE:-}" ]]; then
  AUTH_ARGS=(--cookie "${ADMIN_COOKIE}")
else
  echo "[ERROR] missing auth, provide ADMIN_BEARER_TOKEN or ADMIN_COOKIE"
  exit 1
fi

FILTER_QS="window=${WINDOW}"
if [[ -n "${USER_ID}" ]]; then
  FILTER_QS="${FILTER_QS}&user_id=${USER_ID}"
fi
if [[ -n "${FORMAT}" ]]; then
  FILTER_QS="${FILTER_QS}&format=${FORMAT}"
fi
if [[ -n "${GUARD_REASON}" ]]; then
  FILTER_QS="${FILTER_QS}&guard_reason=${GUARD_REASON}"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

pass_count=0
fail_count=0

pass() {
  pass_count=$((pass_count + 1))
  echo "[PASS] $1"
}

fail() {
  fail_count=$((fail_count + 1))
  echo "[FAIL] $1"
}

check_json() {
  local name="$1"
  local url="$2"
  local jq_expr="$3"
  local out_file="${TMP_DIR}/${name}.json"

  if curl -fsS "${AUTH_ARGS[@]}" "${url}" -o "${out_file}"; then
    if jq -e "${jq_expr}" "${out_file}" >/dev/null 2>&1; then
      pass "${name}"
    else
      fail "${name} (json assertion failed)"
      echo "  jq: ${jq_expr}"
      echo "  body: $(cat "${out_file}" | head -c 200)"
    fi
  else
    fail "${name} (http error)"
  fi
}

check_csv() {
  local name="$1"
  local url="$2"
  local out_file="${TMP_DIR}/${name}.csv"

  if curl -fsS "${AUTH_ARGS[@]}" "${url}" -o "${out_file}"; then
    if [[ ! -s "${out_file}" ]]; then
      fail "${name} (empty csv)"
      return
    fi
    local header
    header="$(head -n 1 "${out_file}")"
    if [[ "${header}" == section,* ]]; then
      pass "${name}"
    else
      fail "${name} (unexpected header)"
      echo "  header: ${header}"
    fi
  else
    fail "${name} (http error)"
  fi
}

echo "[INFO] API_BASE=${API_BASE}"
echo "[INFO] FILTER_QS=${FILTER_QS}"

check_json \
  "overview" \
  "${API_BASE}/api/admin/video-jobs/overview?${FILTER_QS}" \
  '.feedback_integrity_health != null and (.feedback_integrity_alerts | type == "array") and (.feedback_integrity_recommendations | type == "array") and (.feedback_integrity_escalation | type == "object")'

check_json \
  "drilldown" \
  "${API_BASE}/api/admin/video-jobs/feedback-integrity/drilldown?${FILTER_QS}&limit=5" \
  '(.anomaly_jobs | type == "array") and (.top_pick_conflict_jobs | type == "array")'

check_csv \
  "feedback_integrity_csv" \
  "${API_BASE}/api/admin/video-jobs/feedback-integrity.csv?${FILTER_QS}"

check_csv \
  "feedback_integrity_trend_csv" \
  "${API_BASE}/api/admin/video-jobs/feedback-integrity-trend.csv?${FILTER_QS}"

check_csv \
  "feedback_integrity_anomalies_csv" \
  "${API_BASE}/api/admin/video-jobs/feedback-integrity-anomalies.csv?${FILTER_QS}&limit=50"

echo "----------------------------------------"
echo "[SUMMARY] pass=${pass_count} fail=${fail_count}"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 2
fi

exit 0
