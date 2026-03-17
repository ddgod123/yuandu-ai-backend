#!/usr/bin/env bash
set -euo pipefail

# output_id 级反馈闭环冒烟：
# 1) 选取已完成 GIF 任务及 output_id
# 2) 提交 feedback(action + output_id)
# 3) SQL 校验 feedback 行、output 归属、evaluation 映射
# 4) (可选) 创建后续任务，校验评测与 rerank 日志
#
# 用法（基础）：
#   API_BASE=http://127.0.0.1:5050 \
#   USER_BEARER_TOKEN=xxx \
#   DATABASE_URL='postgres://.../emojiDB?sslmode=disable' \
#   ./backend/scripts/smoke_output_feedback_e2e.sh
#
# 用法（含后续任务验证）：
#   API_BASE=http://127.0.0.1:5050 \
#   USER_BEARER_TOKEN=xxx \
#   DATABASE_URL='postgres://.../emojiDB?sslmode=disable' \
#   CREATE_FOLLOWUP_JOB=1 \
#   SOURCE_VIDEO_KEY='emoji/video/.../raw.mp4' \
#   ./backend/scripts/smoke_output_feedback_e2e.sh

if ! command -v curl >/dev/null 2>&1; then
  echo "[ERROR] curl not found in PATH"
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[ERROR] jq not found in PATH"
  exit 1
fi
if ! command -v psql >/dev/null 2>&1; then
  echo "[ERROR] psql not found in PATH"
  exit 1
fi

API_BASE="${API_BASE:-http://127.0.0.1:5050}"
ACTION="${ACTION:-top_pick}"
DONE_JOB_ID="${DONE_JOB_ID:-}"
CREATE_FOLLOWUP_JOB="${CREATE_FOLLOWUP_JOB:-0}"
SOURCE_VIDEO_KEY="${SOURCE_VIDEO_KEY:-}"
POLL_TIMEOUT_SEC="${POLL_TIMEOUT_SEC:-300}"
POLL_INTERVAL_SEC="${POLL_INTERVAL_SEC:-2}"
DATABASE_URL="${DATABASE_URL:-}"

if [[ -z "${DATABASE_URL}" ]]; then
  echo "[ERROR] DATABASE_URL is required"
  exit 1
fi

USER_AUTH_ARGS=()
if [[ -n "${USER_BEARER_TOKEN:-}" ]]; then
  USER_AUTH_ARGS=(-H "Authorization: Bearer ${USER_BEARER_TOKEN}")
elif [[ -n "${USER_COOKIE:-}" ]]; then
  USER_AUTH_ARGS=(--cookie "${USER_COOKIE}")
else
  echo "[ERROR] missing user auth: provide USER_BEARER_TOKEN or USER_COOKIE"
  exit 1
fi

if [[ "${CREATE_FOLLOWUP_JOB}" == "1" && -z "${SOURCE_VIDEO_KEY}" ]]; then
  echo "[ERROR] SOURCE_VIDEO_KEY is required when CREATE_FOLLOWUP_JOB=1"
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

pass_count=0
warn_count=0
fail_count=0

pass() {
  pass_count=$((pass_count + 1))
  echo "[PASS] $1"
}

warn() {
  warn_count=$((warn_count + 1))
  echo "[WARN] $1"
}

fail() {
  fail_count=$((fail_count + 1))
  echo "[FAIL] $1"
}

request_json() {
  local method="$1"
  local url="$2"
  local body="$3"
  local out_file="$4"
  local status=""
  if [[ -n "${body}" ]]; then
    status="$(curl -sS -o "${out_file}" -w "%{http_code}" \
      -X "${method}" \
      -H "Content-Type: application/json" \
      "${USER_AUTH_ARGS[@]}" \
      --data "${body}" \
      "${url}")"
  else
    status="$(curl -sS -o "${out_file}" -w "%{http_code}" \
      -X "${method}" \
      "${USER_AUTH_ARGS[@]}" \
      "${url}")"
  fi
  echo "${status}"
}

assert_http_2xx_json() {
  local name="$1"
  local status="$2"
  local out_file="$3"
  if [[ "${status}" =~ ^2 ]]; then
    pass "${name}"
    return 0
  fi
  fail "${name} (http=${status})"
  echo "  body: $(head -c 320 "${out_file}")"
  return 1
}

run_scalar_sql() {
  local sql="$1"
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -At -c "${sql}" | tr -d '[:space:]'
}

echo "[INFO] API_BASE=${API_BASE}"
echo "[INFO] ACTION=${ACTION}"
echo "[INFO] DONE_JOB_ID=${DONE_JOB_ID:-auto}"
echo "[INFO] CREATE_FOLLOWUP_JOB=${CREATE_FOLLOWUP_JOB}"

# 0) health
health_status="$(curl -sS -o "${TMP_DIR}/healthz.json" -w "%{http_code}" "${API_BASE}/healthz" || true)"
if [[ "${health_status}" == "200" ]]; then
  pass "healthz"
else
  fail "healthz (http=${health_status})"
fi

# 1) current user
me_status="$(request_json "GET" "${API_BASE}/api/me" "" "${TMP_DIR}/me.json")"
if ! assert_http_2xx_json "me api" "${me_status}" "${TMP_DIR}/me.json"; then
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
USER_ID="$(jq -r '.id // 0' "${TMP_DIR}/me.json")"
if [[ ! "${USER_ID}" =~ ^[0-9]+$ || "${USER_ID}" -le 0 ]]; then
  fail "resolve user id from /api/me"
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
pass "resolved user id=${USER_ID}"

# 2) resolve done job id
if [[ -z "${DONE_JOB_ID}" ]]; then
  list_status="$(request_json "GET" "${API_BASE}/api/video-jobs?status=done&limit=30" "" "${TMP_DIR}/done_jobs.json")"
  if assert_http_2xx_json "list done jobs" "${list_status}" "${TMP_DIR}/done_jobs.json"; then
    DONE_JOB_ID="$(jq -r '.items[0].id // 0' "${TMP_DIR}/done_jobs.json")"
  fi
fi
if [[ ! "${DONE_JOB_ID}" =~ ^[0-9]+$ || "${DONE_JOB_ID}" -le 0 ]]; then
  fail "resolve done job id"
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
pass "target done job id=${DONE_JOB_ID}"

# 3) fetch result and pick first output_id
result_status="$(request_json "GET" "${API_BASE}/api/video-jobs/${DONE_JOB_ID}/result" "" "${TMP_DIR}/result_${DONE_JOB_ID}.json")"
if ! assert_http_2xx_json "result api job=${DONE_JOB_ID}" "${result_status}" "${TMP_DIR}/result_${DONE_JOB_ID}.json"; then
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
TARGET_OUTPUT_ID="$(jq -r '.emojis[]? | select((.output_id // 0) > 0) | .output_id' "${TMP_DIR}/result_${DONE_JOB_ID}.json" | head -n1)"
TARGET_EMOJI_ID="$(jq -r '.emojis[]? | select((.output_id // 0) > 0) | .id' "${TMP_DIR}/result_${DONE_JOB_ID}.json" | head -n1)"
if [[ ! "${TARGET_OUTPUT_ID:-0}" =~ ^[0-9]+$ || "${TARGET_OUTPUT_ID:-0}" -le 0 ]]; then
  fail "resolve output_id from result"
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
pass "target output_id=${TARGET_OUTPUT_ID} emoji_id=${TARGET_EMOJI_ID:-0}"

# 4) submit output-level feedback
feedback_payload="$(jq -nc \
  --arg action "${ACTION}" \
  --argjson output_id "${TARGET_OUTPUT_ID}" \
  --argjson emoji_id "${TARGET_EMOJI_ID:-0}" \
  '{action:$action,output_id:$output_id,emoji_id:$emoji_id,metadata:{source:"smoke_output_feedback_e2e"}}')"
feedback_status="$(request_json "POST" "${API_BASE}/api/video-jobs/${DONE_JOB_ID}/feedback" "${feedback_payload}" "${TMP_DIR}/feedback_${DONE_JOB_ID}.json")"
if ! assert_http_2xx_json "submit feedback job=${DONE_JOB_ID}" "${feedback_status}" "${TMP_DIR}/feedback_${DONE_JOB_ID}.json"; then
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
resp_output_id="$(jq -r '.output_id // 0' "${TMP_DIR}/feedback_${DONE_JOB_ID}.json")"
if [[ "${resp_output_id}" == "${TARGET_OUTPUT_ID}" ]]; then
  pass "feedback response output_id matches"
else
  fail "feedback response output_id mismatch (expected=${TARGET_OUTPUT_ID}, got=${resp_output_id})"
fi

# 5) SQL verify feedback row and output mapping
action_sql="$(printf "%s" "${ACTION}" | tr '[:upper:]' '[:lower:]' | sed "s/'/''/g")"
feedback_row="$(psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -At <<SQL
SELECT id || '|' || COALESCE(output_id::text,'') || '|' || COALESCE(action,'')
FROM public.video_image_feedback
WHERE job_id = ${DONE_JOB_ID}
  AND user_id = ${USER_ID}
  AND output_id = ${TARGET_OUTPUT_ID}
  AND lower(coalesce(action,'')) = '${action_sql}'
ORDER BY id DESC
LIMIT 1;
SQL
)"

if [[ -z "${feedback_row}" ]]; then
  fail "sql feedback row not found (job_id=${DONE_JOB_ID}, output_id=${TARGET_OUTPUT_ID})"
  echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
  exit 1
fi
FEEDBACK_ID="$(echo "${feedback_row}" | cut -d'|' -f1)"
pass "sql feedback row found id=${FEEDBACK_ID}"

output_owner_count="$(run_scalar_sql "
SELECT COUNT(*)
FROM public.video_image_outputs
WHERE id = ${TARGET_OUTPUT_ID}
  AND job_id = ${DONE_JOB_ID}
  AND format = 'gif'
  AND file_role = 'main';
")"
if [[ "${output_owner_count}" == "1" ]]; then
  pass "output belongs to target gif main artifact"
else
  fail "output ownership check failed (count=${output_owner_count})"
fi

eval_info="$(psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -At <<SQL
SELECT COALESCE(id::text,'0') || '|' || COALESCE(candidate_id::text,'0')
FROM archive.video_job_gif_evaluations
WHERE output_id = ${TARGET_OUTPUT_ID}
ORDER BY id DESC
LIMIT 1;
SQL
)"
if [[ -z "${eval_info}" ]]; then
  fail "evaluation row not found for output_id=${TARGET_OUTPUT_ID}"
else
  eval_id="$(echo "${eval_info}" | cut -d'|' -f1)"
  eval_candidate_id="$(echo "${eval_info}" | cut -d'|' -f2)"
  pass "evaluation row found id=${eval_id}"
  if [[ "${eval_candidate_id}" =~ ^[0-9]+$ && "${eval_candidate_id}" -gt 0 ]]; then
    pass "evaluation has candidate mapping candidate_id=${eval_candidate_id}"
  else
    warn "evaluation candidate_id is empty (old jobs may not have candidate mapping)"
  fi
fi

# 6) optional followup job: verify signal enters next run
if [[ "${CREATE_FOLLOWUP_JOB}" == "1" ]]; then
  create_payload="$(jq -nc \
    --arg title "output-feedback-e2e-$(date +%Y%m%d%H%M%S)" \
    --arg source_video_key "${SOURCE_VIDEO_KEY}" \
    '{title:$title,source_video_key:$source_video_key,output_formats:["gif"],auto_highlight:true}')"
  create_status="$(request_json "POST" "${API_BASE}/api/video-jobs" "${create_payload}" "${TMP_DIR}/create_followup.json")"
  if assert_http_2xx_json "create followup gif job" "${create_status}" "${TMP_DIR}/create_followup.json"; then
    follow_job_id="$(jq -r '.id // 0' "${TMP_DIR}/create_followup.json")"
    if [[ "${follow_job_id}" =~ ^[0-9]+$ && "${follow_job_id}" -gt 0 ]]; then
      pass "followup job id=${follow_job_id}"
      deadline=$(( $(date +%s) + POLL_TIMEOUT_SEC ))
      final_status=""
      while [[ $(date +%s) -lt ${deadline} ]]; do
        poll_status="$(request_json "GET" "${API_BASE}/api/video-jobs/${follow_job_id}" "" "${TMP_DIR}/followup_job.json")"
        if [[ "${poll_status}" =~ ^2 ]]; then
          final_status="$(jq -r '.status // \"\"' "${TMP_DIR}/followup_job.json")"
          if [[ "${final_status}" == "done" || "${final_status}" == "failed" || "${final_status}" == "cancelled" ]]; then
            break
          fi
        fi
        sleep "${POLL_INTERVAL_SEC}"
      done
      if [[ "${final_status}" == "done" ]]; then
        pass "followup job done"
        eval_count="$(run_scalar_sql "SELECT COUNT(*) FROM archive.video_job_gif_evaluations WHERE job_id = ${follow_job_id};")"
        if [[ "${eval_count}" =~ ^[0-9]+$ && "${eval_count}" -gt 0 ]]; then
          pass "followup evaluation rows=${eval_count}"
        else
          fail "followup evaluation rows missing"
        fi

        rerank_count="$(run_scalar_sql "SELECT COUNT(*) FROM ops.video_job_gif_rerank_logs WHERE job_id = ${follow_job_id};")"
        feedback_group="$(run_scalar_sql "SELECT COALESCE(metrics->'highlight_feedback_v1'->>'group','') FROM public.video_image_jobs WHERE id = ${follow_job_id};")"
        feedback_applied="$(run_scalar_sql "SELECT COALESCE(metrics->'highlight_feedback_v1'->>'applied','') FROM public.video_image_jobs WHERE id = ${follow_job_id};")"
        if [[ "${feedback_group}" == "treatment" && "${feedback_applied}" == "true" ]]; then
          if [[ "${rerank_count}" =~ ^[0-9]+$ && "${rerank_count}" -gt 0 ]]; then
            pass "followup rerank logs present count=${rerank_count}"
          else
            fail "followup rerank logs missing while treatment+applied"
          fi
        else
          warn "followup rerank strict check skipped (group=${feedback_group}, applied=${feedback_applied}, rerank_count=${rerank_count})"
        fi
      else
        fail "followup job not done (status=${final_status:-unknown})"
      fi
    else
      fail "create followup response missing id"
    fi
  fi
fi

echo "[SUMMARY] pass=${pass_count} warn=${warn_count} fail=${fail_count}"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 1
fi
