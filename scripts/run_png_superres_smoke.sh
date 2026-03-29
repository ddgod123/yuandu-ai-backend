#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# 可选：指定一个已完成的 PNG 基准任务作为源
# BASE_JOB_ID=251 ./scripts/run_png_superres_smoke.sh

go run ./cmd/png-superres-smoke
