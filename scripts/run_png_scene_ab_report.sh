#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PY_SCRIPT="$ROOT_DIR/scripts/png_scene_ab_report.py"

if [[ ! -f "$PY_SCRIPT" ]]; then
  echo "missing script: $PY_SCRIPT" >&2
  exit 1
fi

python3 "$PY_SCRIPT" "$@"

