#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# 示例：
# PNG_BATCH_VIDEO_DIR="/Users/mac/go/src/emoji/视频测试/第一轮" \
# PNG_BATCH_LIMIT=3 \
# PNG_BATCH_SCENE=xiaohongshu \
# ./scripts/run_png_superres_batch.sh

go run ./cmd/png-superres-batch
