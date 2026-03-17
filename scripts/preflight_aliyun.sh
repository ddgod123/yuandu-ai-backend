#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${1:-/etc/emoji/emoji.env}"

PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  echo "[PASS] $*"
}

warn() {
  WARN_COUNT=$((WARN_COUNT + 1))
  echo "[WARN] $*"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  echo "[FAIL] $*"
}

check_cmd() {
  local cmd="$1"
  if command -v "$cmd" >/dev/null 2>&1; then
    pass "command available: ${cmd}"
  else
    fail "command missing: ${cmd}"
  fi
}

parse_host_port() {
  local addr="$1"
  if [[ "$addr" == *:* ]]; then
    echo "${addr%%:*}" "${addr##*:}"
  else
    echo "$addr" "6379"
  fi
}

echo "== Emoji 阿里云部署预检 =="
echo "env file: ${ENV_FILE}"

if [[ -f "${ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  pass "loaded env file"
else
  warn "env file not found, only command-level checks will run"
fi

echo
echo "-- Runtime Commands --"
check_cmd ffmpeg
check_cmd ffprobe
check_cmd curl
check_cmd bash

if command -v ffmpeg >/dev/null 2>&1; then
  encoders="$(ffmpeg -hide_banner -encoders 2>/dev/null || true)"
  if echo "${encoders}" | grep -q "libx264"; then
    pass "ffmpeg encoder available: libx264"
  else
    fail "ffmpeg encoder missing: libx264 (mp4 output will fail)"
  fi
  if echo "${encoders}" | grep -Eq "libwebp|libwebp_anim"; then
    pass "ffmpeg encoder available: webp"
  else
    warn "ffmpeg encoder missing: libwebp/libwebp_anim (webp will be skipped)"
  fi
fi

echo
echo "-- Env Variables --"
required_env=(
  APP_ENV
  APP_PORT
  DB_HOST
  DB_PORT
  DB_USER
  DB_NAME
  DB_SEARCH_PATH
  JWT_SECRET
  REDIS_ADDR
  ASYNQ_REDIS_ADDR
  QINIU_ACCESS_KEY
  QINIU_SECRET_KEY
  QINIU_BUCKET
  QINIU_DOMAIN
)

for key in "${required_env[@]}"; do
  if [[ -n "${!key:-}" ]]; then
    pass "env set: ${key}"
  else
    fail "env missing: ${key}"
  fi
done

echo
echo "-- Connectivity --"
if command -v psql >/dev/null 2>&1; then
  if [[ -n "${DB_HOST:-}" && -n "${DB_PORT:-}" && -n "${DB_USER:-}" && -n "${DB_NAME:-}" ]]; then
    if PGPASSWORD="${DB_PASSWORD:-}" psql \
      "host=${DB_HOST} port=${DB_PORT} user=${DB_USER} dbname=${DB_NAME} sslmode=${DB_SSLMODE:-disable} connect_timeout=3" \
      -c "select 1;" >/dev/null 2>&1; then
      pass "postgres connectivity ok"
    else
      fail "postgres connectivity failed"
    fi
  else
    warn "postgres env incomplete, skip DB connectivity check"
  fi
else
  warn "psql not found, skip DB connectivity check"
fi

if command -v redis-cli >/dev/null 2>&1; then
  if [[ -n "${ASYNQ_REDIS_ADDR:-}" ]]; then
    read -r redis_host redis_port < <(parse_host_port "${ASYNQ_REDIS_ADDR}")
    redis_args=(-h "${redis_host}" -p "${redis_port}")
    if [[ -n "${ASYNQ_REDIS_PASSWORD:-}" ]]; then
      redis_args+=(-a "${ASYNQ_REDIS_PASSWORD}")
    fi
    if redis-cli "${redis_args[@]}" ping >/dev/null 2>&1; then
      pass "redis connectivity ok"
    else
      fail "redis connectivity failed"
    fi
  else
    warn "ASYNQ_REDIS_ADDR missing, skip redis check"
  fi
else
  warn "redis-cli not found, skip redis connectivity check"
fi

echo
echo "-- System Capacity --"
avail_kb="$(df -k / | awk 'NR==2 {print $4}')"
if [[ -n "${avail_kb}" ]]; then
  if (( avail_kb > 20 * 1024 * 1024 )); then
    pass "disk free space > 20GB"
  else
    warn "disk free space <= 20GB, consider cleanup/expansion"
  fi
else
  warn "unable to detect disk free space"
fi

echo
echo "== Preflight Summary =="
echo "PASS: ${PASS_COUNT}"
echo "WARN: ${WARN_COUNT}"
echo "FAIL: ${FAIL_COUNT}"

if (( FAIL_COUNT > 0 )); then
  exit 1
fi

exit 0
