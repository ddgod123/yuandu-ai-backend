#!/usr/bin/env bash
set -euo pipefail

# 用法：
#   ./backend/scripts/check_gif_cost_gate.sh <DATABASE_URL> [WINDOW_HOURS] [MAX_AVG_COST] [MIN_SAMPLES]
#
# 示例：
#   ./backend/scripts/check_gif_cost_gate.sh "postgres://..." 24 1.20 10

DATABASE_URL="${1:-}"
WINDOW_HOURS="${2:-24}"
MAX_AVG_COST="${3:-1.20}"
MIN_SAMPLES="${4:-10}"

if [[ -z "${DATABASE_URL}" ]]; then
  echo "[ERROR] DATABASE_URL is required"
  exit 1
fi
if ! [[ "${WINDOW_HOURS}" =~ ^[0-9]+$ ]] || [[ "${WINDOW_HOURS}" -le 0 ]]; then
  echo "[ERROR] WINDOW_HOURS must be a positive integer"
  exit 1
fi
if ! [[ "${MIN_SAMPLES}" =~ ^[0-9]+$ ]] || [[ "${MIN_SAMPLES}" -lt 0 ]]; then
  echo "[ERROR] MIN_SAMPLES must be a non-negative integer"
  exit 1
fi

read -r samples avg_cost p50_cost p95_cost max_cost <<<"$(
  psql "${DATABASE_URL}" -At -F $'\t' -v ON_ERROR_STOP=1 -c "
WITH scoped AS (
  SELECT
    c.estimated_cost
  FROM ops.video_job_costs c
  JOIN public.video_image_jobs j ON j.id = c.job_id
  WHERE LOWER(COALESCE(NULLIF(TRIM(j.requested_format), ''), 'gif')) = 'gif'
    AND LOWER(COALESCE(NULLIF(TRIM(j.status), ''), 'unknown')) = 'done'
    AND j.created_at >= NOW() - INTERVAL '${WINDOW_HOURS} hours'
),
summary AS (
  SELECT
    COUNT(*)::bigint AS samples,
    ROUND(COALESCE(AVG(estimated_cost), 0)::numeric, 6) AS avg_cost,
    ROUND(COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY estimated_cost), 0)::numeric, 6) AS p50_cost,
    ROUND(COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY estimated_cost), 0)::numeric, 6) AS p95_cost,
    ROUND(COALESCE(MAX(estimated_cost), 0)::numeric, 6) AS max_cost
  FROM scoped
)
SELECT samples, avg_cost, p50_cost, p95_cost, max_cost
FROM summary;
"
)"

samples="${samples:-0}"
avg_cost="${avg_cost:-0}"
p50_cost="${p50_cost:-0}"
p95_cost="${p95_cost:-0}"
max_cost="${max_cost:-0}"

echo "[gif_cost_gate] window=${WINDOW_HOURS}h samples=${samples} avg=${avg_cost} p50=${p50_cost} p95=${p95_cost} max=${max_cost} threshold=${MAX_AVG_COST} min_samples=${MIN_SAMPLES}"

if [[ "${samples}" -lt "${MIN_SAMPLES}" ]]; then
  echo "[gif_cost_gate] skip: insufficient samples (${samples} < ${MIN_SAMPLES})"
  exit 0
fi

if awk "BEGIN {exit !(${avg_cost} <= ${MAX_AVG_COST})}"; then
  echo "[gif_cost_gate] pass"
  exit 0
fi

echo "[gif_cost_gate] fail: avg_cost(${avg_cost}) > threshold(${MAX_AVG_COST})"
exit 2
