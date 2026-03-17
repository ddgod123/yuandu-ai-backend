#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
One-shot baseline flow:
  1) export before snapshot
  2) run exercise command
  3) export after snapshot
  4) compare before/after and write report

Usage:
  ADMIN_TOKEN=<jwt> ./scripts/run_video_jobs_baseline_flow.sh \
    --exercise-cmd 'go run ./cmd/your-loadtest --count 200'

Options:
  --exercise-cmd <cmd>   Command to run between before/after snapshots (default: true)
  --window <value>       24h | 7d | 30d (default: 24h)
  --api-base <url>       API base (default: http://127.0.0.1:5050)
  --admin-token <jwt>    Override ADMIN_TOKEN env
  --out-dir <dir>        Output directory (default: tmp)
  --tag <name>           Optional tag added to output filename
  --continue-on-fail     Continue and compare even if exercise command fails
  -h, --help             Show this help
EOF
}

WINDOW="${WINDOW:-24h}"
API_BASE="${API_BASE:-http://127.0.0.1:5050}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
OUT_DIR="tmp"
TAG=""
EXERCISE_CMD="true"
CONTINUE_ON_FAIL="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --exercise-cmd)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --exercise-cmd requires a value" >&2
        exit 1
      fi
      EXERCISE_CMD="$2"
      shift 2
      ;;
    --window)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --window requires a value" >&2
        exit 1
      fi
      WINDOW="$2"
      shift 2
      ;;
    --api-base)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --api-base requires a value" >&2
        exit 1
      fi
      API_BASE="$2"
      shift 2
      ;;
    --admin-token)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --admin-token requires a value" >&2
        exit 1
      fi
      ADMIN_TOKEN="$2"
      shift 2
      ;;
    --out-dir)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --out-dir requires a value" >&2
        exit 1
      fi
      OUT_DIR="$2"
      shift 2
      ;;
    --tag)
      if [[ $# -lt 2 ]]; then
        echo "ERROR: --tag requires a value" >&2
        exit 1
      fi
      TAG="$2"
      shift 2
      ;;
    --continue-on-fail)
      CONTINUE_ON_FAIL="1"
      shift 1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "${ADMIN_TOKEN}" ]]; then
  echo "ERROR: ADMIN_TOKEN is required (env or --admin-token)." >&2
  exit 1
fi

if [[ ! -x "./scripts/export_video_jobs_baseline.sh" ]]; then
  echo "ERROR: missing script ./scripts/export_video_jobs_baseline.sh" >&2
  exit 1
fi
if [[ ! -x "./scripts/compare_video_jobs_baseline.py" ]]; then
  echo "ERROR: missing script ./scripts/compare_video_jobs_baseline.py" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"

ts="$(date +%Y%m%d-%H%M%S)"
safe_tag="$(echo "${TAG}" | tr -cs '[:alnum:]_-' '-')"
name_prefix="video-jobs-baseline-${WINDOW}"
if [[ -n "${safe_tag}" ]]; then
  name_prefix="${name_prefix}-${safe_tag}"
fi
name_prefix="${name_prefix}-${ts}"

before_json="${OUT_DIR%/}/${name_prefix}-before.json"
after_json="${OUT_DIR%/}/${name_prefix}-after.json"
compare_json="${OUT_DIR%/}/${name_prefix}-compare.json"
compare_txt="${OUT_DIR%/}/${name_prefix}-compare.txt"

echo "==> Export before snapshot"
ADMIN_TOKEN="${ADMIN_TOKEN}" WINDOW="${WINDOW}" API_BASE="${API_BASE}" \
  ./scripts/export_video_jobs_baseline.sh "${before_json}"

echo "==> Run exercise command"
echo "    ${EXERCISE_CMD}"
set +e
bash -lc "${EXERCISE_CMD}"
exercise_status=$?
set -e
if [[ ${exercise_status} -ne 0 ]]; then
  echo "WARN: exercise command failed with status ${exercise_status}" >&2
  if [[ "${CONTINUE_ON_FAIL}" != "1" ]]; then
    echo "ERROR: stop because --continue-on-fail is not set" >&2
    exit "${exercise_status}"
  fi
fi

echo "==> Export after snapshot"
ADMIN_TOKEN="${ADMIN_TOKEN}" WINDOW="${WINDOW}" API_BASE="${API_BASE}" \
  ./scripts/export_video_jobs_baseline.sh "${after_json}"

echo "==> Compare snapshots"
./scripts/compare_video_jobs_baseline.py "${before_json}" "${after_json}" --out-json "${compare_json}" | tee "${compare_txt}"

echo ""
echo "Flow done."
echo "  before : ${before_json}"
echo "  after  : ${after_json}"
echo "  compare: ${compare_json}"
echo "  report : ${compare_txt}"

if [[ ${exercise_status} -ne 0 ]]; then
  echo "NOTE: exercise command failed previously (status=${exercise_status}), compare was still generated." >&2
  exit "${exercise_status}"
fi

