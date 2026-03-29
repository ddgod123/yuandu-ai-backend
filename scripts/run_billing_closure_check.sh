#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

if [[ $# -eq 0 ]]; then
  echo "Usage:"
  echo "  scripts/run_billing_closure_check.sh --job-ids 215,216 [--strict]"
  echo "  scripts/run_billing_closure_check.sh --format png --limit 20"
  exit 1
fi

go run ./cmd/check-billing-closure "$@"

