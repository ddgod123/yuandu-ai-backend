#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INPUT_DIR="${1:-}"
SUFFIX="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="${2:-$ROOT_DIR/tmp/gif-loop-regression}"

if [[ -z "$INPUT_DIR" ]]; then
  echo "Usage: $0 <input_video_dir> [output_dir]"
  exit 1
fi

mkdir -p "$OUT_DIR"
BEFORE_JSON="$OUT_DIR/before-$SUFFIX.json"
AFTER_JSON="$OUT_DIR/after-$SUFFIX.json"
DIFF_JSON="$OUT_DIR/diff-$SUFFIX.json"

echo "[1/3] Run BEFORE regression"
go run "$ROOT_DIR/cmd/gif-loop-regression" \
  --input "$INPUT_DIR" \
  --use-highlight=true \
  --render=true \
  --prefer-window-sec=3.0 \
  --out-json "$BEFORE_JSON"

echo "[2/3] 请完成参数调整后按回车继续 AFTER 回归..."
read -r

echo "[2/3] Run AFTER regression"
go run "$ROOT_DIR/cmd/gif-loop-regression" \
  --input "$INPUT_DIR" \
  --use-highlight=true \
  --render=true \
  --prefer-window-sec=3.0 \
  --out-json "$AFTER_JSON"

echo "[3/3] Diff BEFORE vs AFTER"
go run "$ROOT_DIR/cmd/gif-loop-regression-diff" \
  --base "$BEFORE_JSON" \
  --target "$AFTER_JSON" \
  --out-json "$DIFF_JSON"

echo "Done."
echo "  BEFORE: $BEFORE_JSON"
echo "  AFTER : $AFTER_JSON"
echo "  DIFF  : $DIFF_JSON"
