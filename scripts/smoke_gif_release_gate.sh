#!/usr/bin/env bash
set -euo pipefail

# GIF 发布门禁一键回归脚本（创建/取消/下载/删除/巡检）
#
# 基础用法（非破坏性）：
#   API_BASE=http://127.0.0.1:5050 \
#   USER_BEARER_TOKEN=xxx \
#   SOURCE_VIDEO_KEY=emoji/user-video/20260315/xxx_raw.mp4 \
#   DATABASE_URL='postgres://mac@localhost:5432/emojiDB?sslmode=disable' \
#   ./backend/scripts/smoke_gif_release_gate.sh
#
# 启用“删单张 + ZIP 重建”破坏性测试：
#   ENABLE_DELETE_TEST=1 DELETE_CONFIRM=I_UNDERSTAND_DELETE ...
#
# 可选参数：
#   DONE_JOB_ID=26                 # 指定用于下载/删除验证的已完成任务
#   POLL_TIMEOUT_SEC=120           # 取消态确认超时
#   POLL_INTERVAL_SEC=2
#   RUN_SQL_CHECKS=1               # 默认：DATABASE_URL 有值时自动开启
#   SKIP_DOWNLOAD_VALIDATE=0       # =1 时不下载并校验 zip 文件内容
#   ALLOW_INSECURE_SSL=1           # =1 时下载 zip 使用 curl -k（开发环境自签名证书）

if ! command -v curl >/dev/null 2>&1; then
  echo "[ERROR] curl not found in PATH"
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[ERROR] jq not found in PATH"
  exit 1
fi

API_BASE="${API_BASE:-http://127.0.0.1:5050}"
SOURCE_VIDEO_KEY="${SOURCE_VIDEO_KEY:-}"
DONE_JOB_ID="${DONE_JOB_ID:-}"
POLL_TIMEOUT_SEC="${POLL_TIMEOUT_SEC:-120}"
POLL_INTERVAL_SEC="${POLL_INTERVAL_SEC:-2}"
ENABLE_DELETE_TEST="${ENABLE_DELETE_TEST:-0}"
DELETE_CONFIRM="${DELETE_CONFIRM:-}"
SKIP_DOWNLOAD_VALIDATE="${SKIP_DOWNLOAD_VALIDATE:-0}"
ALLOW_INSECURE_SSL="${ALLOW_INSECURE_SSL:-0}"
DATABASE_URL="${DATABASE_URL:-}"
RUN_SQL_CHECKS="${RUN_SQL_CHECKS:-}"

if [[ -z "${SOURCE_VIDEO_KEY}" ]]; then
  echo "[ERROR] SOURCE_VIDEO_KEY is required"
  exit 1
fi

if [[ -z "${RUN_SQL_CHECKS}" ]]; then
  if [[ -n "${DATABASE_URL}" ]]; then
    RUN_SQL_CHECKS="1"
  else
    RUN_SQL_CHECKS="0"
  fi
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

if [[ "${ENABLE_DELETE_TEST}" == "1" && "${DELETE_CONFIRM}" != "I_UNDERSTAND_DELETE" ]]; then
  echo "[ERROR] destructive delete test requires DELETE_CONFIRM=I_UNDERSTAND_DELETE"
  exit 1
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
  echo "  body: $(cat "${out_file}" | head -c 240)"
  return 1
}

validate_zip_file() {
  local url="$1"
  local file_path="$2"
  local curl_args=(-LfsS)
  if [[ "${ALLOW_INSECURE_SSL}" == "1" ]]; then
    curl_args+=(-k)
  fi
  if ! curl "${curl_args[@]}" "${url}" -o "${file_path}"; then
    echo "[ERROR] download zip failed: ${url}"
    return 1
  fi
  local size
  size="$(wc -c < "${file_path}" | tr -d ' ')"
  if [[ "${size}" -lt 128 ]]; then
    echo "[ERROR] zip too small: ${size} bytes"
    return 1
  fi
  if command -v unzip >/dev/null 2>&1; then
    unzip -t "${file_path}" >/dev/null 2>&1
    return $?
  fi
  python3 - "${file_path}" <<'PY'
import sys, zipfile
path = sys.argv[1]
with zipfile.ZipFile(path, "r") as zf:
    bad = zf.testzip()
    if bad:
        raise SystemExit(f"bad entry: {bad}")
    if len(zf.namelist()) == 0:
        raise SystemExit("empty zip")
print("ok")
PY
}

echo "[INFO] API_BASE=${API_BASE}"
echo "[INFO] SOURCE_VIDEO_KEY=${SOURCE_VIDEO_KEY}"
echo "[INFO] ENABLE_DELETE_TEST=${ENABLE_DELETE_TEST}"
echo "[INFO] RUN_SQL_CHECKS=${RUN_SQL_CHECKS}"
echo "[INFO] ALLOW_INSECURE_SSL=${ALLOW_INSECURE_SSL}"

# 0) 健康检查
health_status="$(curl -sS -o "${TMP_DIR}/healthz.json" -w "%{http_code}" "${API_BASE}/healthz" || true)"
if [[ "${health_status}" == "200" ]]; then
  pass "healthz"
else
  fail "healthz (http=${health_status})"
fi

# 1) GIF 能力检查
cap_status="$(request_json "GET" "${API_BASE}/api/video-jobs/capabilities" "" "${TMP_DIR}/capabilities.json")"
if assert_http_2xx_json "capabilities api" "${cap_status}" "${TMP_DIR}/capabilities.json"; then
  if jq -e '.formats[]? | select(.format=="gif") | .supported == true' "${TMP_DIR}/capabilities.json" >/dev/null 2>&1; then
    pass "gif capability supported"
  else
    fail "gif capability supported"
  fi
fi

# 2) 创建任务
create_payload="$(jq -nc \
  --arg title "GIF门禁回归-$(date +%Y%m%d%H%M%S)" \
  --arg key "${SOURCE_VIDEO_KEY}" \
  '{title:$title,source_video_key:$key,output_formats:["gif"],auto_highlight:true}')"
create_status="$(request_json "POST" "${API_BASE}/api/video-jobs" "${create_payload}" "${TMP_DIR}/create.json")"
created_job_id=""
if assert_http_2xx_json "create gif job" "${create_status}" "${TMP_DIR}/create.json"; then
  created_job_id="$(jq -r '.id // 0' "${TMP_DIR}/create.json")"
  if [[ "${created_job_id}" =~ ^[0-9]+$ && "${created_job_id}" -gt 0 ]]; then
    pass "created job id=${created_job_id}"
  else
    fail "create response missing id"
  fi
fi

# 3) 取消任务
if [[ "${created_job_id}" =~ ^[0-9]+$ && "${created_job_id}" -gt 0 ]]; then
  cancel_status="$(request_json "POST" "${API_BASE}/api/video-jobs/${created_job_id}/cancel" "{}" "${TMP_DIR}/cancel.json")"
  assert_http_2xx_json "cancel job ${created_job_id}" "${cancel_status}" "${TMP_DIR}/cancel.json" || true

  # 4) 轮询确认已取消
  deadline=$(( $(date +%s) + POLL_TIMEOUT_SEC ))
  final_status=""
  while [[ $(date +%s) -lt ${deadline} ]]; do
    get_status="$(request_json "GET" "${API_BASE}/api/video-jobs/${created_job_id}" "" "${TMP_DIR}/job_${created_job_id}.json")"
    if [[ "${get_status}" =~ ^2 ]]; then
      final_status="$(jq -r '.status // ""' "${TMP_DIR}/job_${created_job_id}.json")"
      if [[ "${final_status}" == "cancelled" ]]; then
        break
      fi
    fi
    sleep "${POLL_INTERVAL_SEC}"
  done
  if [[ "${final_status}" == "cancelled" ]]; then
    pass "job ${created_job_id} reached cancelled"
  else
    fail "job ${created_job_id} not cancelled in time (status=${final_status:-unknown})"
  fi
fi

# 5) 选取 done 任务用于下载/删除验证
if [[ -z "${DONE_JOB_ID}" ]]; then
  list_status="$(request_json "GET" "${API_BASE}/api/video-jobs?status=done&limit=30" "" "${TMP_DIR}/done_jobs.json")"
  if [[ "${list_status}" =~ ^2 ]]; then
    DONE_JOB_ID="$(jq -r '.items[0].id // 0' "${TMP_DIR}/done_jobs.json")"
  fi
fi
if [[ ! "${DONE_JOB_ID}" =~ ^[0-9]+$ || "${DONE_JOB_ID}" -le 0 ]]; then
  fail "resolve done job id for download/delete checks"
else
  pass "use done job id=${DONE_JOB_ID}"
fi

first_emoji_id=""
if [[ "${DONE_JOB_ID}" =~ ^[0-9]+$ && "${DONE_JOB_ID}" -gt 0 ]]; then
  # 6) 详情结果
  result_status="$(request_json "GET" "${API_BASE}/api/video-jobs/${DONE_JOB_ID}/result" "" "${TMP_DIR}/result_${DONE_JOB_ID}.json")"
  if assert_http_2xx_json "result api job=${DONE_JOB_ID}" "${result_status}" "${TMP_DIR}/result_${DONE_JOB_ID}.json"; then
    emoji_count="$(jq -r '(.emojis | length) // 0' "${TMP_DIR}/result_${DONE_JOB_ID}.json")"
    if [[ "${emoji_count}" =~ ^[0-9]+$ && "${emoji_count}" -gt 0 ]]; then
      pass "result emojis count=${emoji_count}"
      first_emoji_id="$(jq -r '.emojis[0].id // 0' "${TMP_DIR}/result_${DONE_JOB_ID}.json")"
    else
      fail "result emojis empty"
    fi
  fi

  # 7) 下载 zip（删除前）
  dl1_status="$(request_json "GET" "${API_BASE}/api/video-jobs/${DONE_JOB_ID}/download-zip" "" "${TMP_DIR}/download_zip_before.json")"
  if assert_http_2xx_json "download zip api (before delete) job=${DONE_JOB_ID}" "${dl1_status}" "${TMP_DIR}/download_zip_before.json"; then
    zip_url_before="$(jq -r '.url // ""' "${TMP_DIR}/download_zip_before.json")"
    zip_name_before="$(jq -r '.name // ""' "${TMP_DIR}/download_zip_before.json")"
    if [[ -n "${zip_url_before}" && -n "${zip_name_before}" ]]; then
      pass "download zip payload valid (before delete)"
      if [[ "${SKIP_DOWNLOAD_VALIDATE}" != "1" ]]; then
        if validate_zip_file "${zip_url_before}" "${TMP_DIR}/before.zip"; then
          pass "zip file valid (before delete)"
        else
          fail "zip file valid (before delete)"
        fi
      fi
    else
      fail "download zip payload missing fields (before delete)"
    fi
  fi

  # 8) 删单张 + 再次下载 zip（破坏性，默认关闭）
  if [[ "${ENABLE_DELETE_TEST}" == "1" ]]; then
    if [[ "${first_emoji_id}" =~ ^[0-9]+$ && "${first_emoji_id}" -gt 0 ]]; then
      del_payload="$(jq -nc --argjson emoji_id "${first_emoji_id}" '{emoji_id:$emoji_id,remove_zip:true}')"
      del_status="$(request_json "POST" "${API_BASE}/api/video-jobs/${DONE_JOB_ID}/delete-output" "${del_payload}" "${TMP_DIR}/delete_output.json")"
      assert_http_2xx_json "delete output job=${DONE_JOB_ID} emoji=${first_emoji_id}" "${del_status}" "${TMP_DIR}/delete_output.json" || true

      dl2_status="$(request_json "GET" "${API_BASE}/api/video-jobs/${DONE_JOB_ID}/download-zip" "" "${TMP_DIR}/download_zip_after.json")"
      if assert_http_2xx_json "download zip api (after delete) job=${DONE_JOB_ID}" "${dl2_status}" "${TMP_DIR}/download_zip_after.json"; then
        zip_url_after="$(jq -r '.url // ""' "${TMP_DIR}/download_zip_after.json")"
        if [[ -n "${zip_url_after}" ]]; then
          pass "download zip payload valid (after delete)"
          if [[ "${SKIP_DOWNLOAD_VALIDATE}" != "1" ]]; then
            if validate_zip_file "${zip_url_after}" "${TMP_DIR}/after.zip"; then
              pass "zip file valid (after delete)"
            else
              fail "zip file valid (after delete)"
            fi
          fi
        else
          fail "download zip payload missing url (after delete)"
        fi
      fi
    else
      fail "delete test cannot run: first emoji id unavailable"
    fi
  fi
fi

# 9) SQL 巡检
if [[ "${RUN_SQL_CHECKS}" == "1" ]]; then
  if [[ -z "${DATABASE_URL}" ]]; then
    fail "sql checks enabled but DATABASE_URL is empty"
  else
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    if "${script_dir}/check_gif_health.sh" "${DATABASE_URL}" >/dev/null; then
      pass "sql check_gif_health"
    else
      fail "sql check_gif_health"
    fi
    if "${script_dir}/check_feedback_integrity.sh" "${DATABASE_URL}" >/dev/null; then
      pass "sql check_feedback_integrity"
    else
      fail "sql check_feedback_integrity"
    fi
    if [[ "${DONE_JOB_ID}" =~ ^[0-9]+$ && "${DONE_JOB_ID}" -gt 0 ]]; then
      if "${script_dir}/check_video_job_package.sh" "${DONE_JOB_ID}" "${DATABASE_URL}" >/dev/null; then
        pass "sql check_video_job_package job=${DONE_JOB_ID}"
      else
        fail "sql check_video_job_package job=${DONE_JOB_ID}"
      fi
    fi
  fi
fi

echo "----------------------------------------"
echo "[SUMMARY] pass=${pass_count} fail=${fail_count}"
if [[ "${fail_count}" -gt 0 ]]; then
  exit 2
fi
exit 0
