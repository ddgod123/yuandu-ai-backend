#!/usr/bin/env bash
set -euo pipefail

# Smoke test for collection download cards (次卡)
#
# Required env:
#   API_BASE=http://localhost:5050/api
#   ADMIN_BEARER_TOKEN=...
#   USER_BEARER_TOKEN=...
#   COLLECTION_ID=123
#
# Optional env:
#   DOWNLOAD_TIMES=2
#   MAX_REDEEM_USERS=1
#   EXPECT_EXHAUST=1   # set to 1 to assert exhausted on (DOWNLOAD_TIMES + 1)th download

API_BASE="${API_BASE:-http://localhost:5050/api}"
ADMIN_BEARER_TOKEN="${ADMIN_BEARER_TOKEN:-}"
USER_BEARER_TOKEN="${USER_BEARER_TOKEN:-}"
COLLECTION_ID="${COLLECTION_ID:-}"
DOWNLOAD_TIMES="${DOWNLOAD_TIMES:-2}"
MAX_REDEEM_USERS="${MAX_REDEEM_USERS:-1}"
EXPECT_EXHAUST="${EXPECT_EXHAUST:-0}"

if [[ -z "$ADMIN_BEARER_TOKEN" ]]; then
  echo "[ERROR] ADMIN_BEARER_TOKEN is required"
  exit 1
fi
if [[ -z "$USER_BEARER_TOKEN" ]]; then
  echo "[ERROR] USER_BEARER_TOKEN is required"
  exit 1
fi
if [[ -z "$COLLECTION_ID" ]]; then
  echo "[ERROR] COLLECTION_ID is required"
  exit 1
fi

admin_auth=(-H "Authorization: Bearer ${ADMIN_BEARER_TOKEN}" -H "Content-Type: application/json")
user_auth=(-H "Authorization: Bearer ${USER_BEARER_TOKEN}" -H "Content-Type: application/json")

echo "[1/6] generate collection download code"
generate_resp="$(curl -sS -X POST "${API_BASE}/admin/collection-download-codes/generate" \
  "${admin_auth[@]}" \
  -d "{\"count\":1,\"collection_id\":${COLLECTION_ID},\"download_times\":${DOWNLOAD_TIMES},\"max_redeem_users\":${MAX_REDEEM_USERS},\"note\":\"smoke_collection_download_cards\"}")"

code_plain="$(python3 - <<'PY' "$generate_resp"
import json,sys
try:
    data=json.loads(sys.argv[1])
    codes=data.get("codes") or []
    print(codes[0] if codes else "")
except Exception:
    print("")
PY
)"
if [[ -z "$code_plain" ]]; then
  echo "[ERROR] failed to parse generated code"
  echo "$generate_resp"
  exit 1
fi
echo "  generated code: ${code_plain}"

echo "[2/6] validate code by user"
validate_resp="$(curl -sS -X POST "${API_BASE}/me/collection-download-code/validate" \
  "${user_auth[@]}" \
  -d "{\"code\":\"${code_plain}\"}")"
echo "  validate: ${validate_resp}"

echo "[3/6] redeem code by user"
redeem_resp="$(curl -sS -X POST "${API_BASE}/me/collection-download-code/redeem" \
  "${user_auth[@]}" \
  -d "{\"code\":\"${code_plain}\"}")"
echo "  redeem: ${redeem_resp}"

echo "[4/6] list user entitlements"
entitlements_resp="$(curl -sS -X GET "${API_BASE}/me/collection-download-entitlements?page=1&page_size=20" \
  -H "Authorization: Bearer ${USER_BEARER_TOKEN}")"
echo "  entitlements: ${entitlements_resp}"

echo "[5/6] trigger collection zip download link (consume once if non-subscriber)"
download_resp="$(curl -sS -X GET "${API_BASE}/collections/${COLLECTION_ID}/download-zip" \
  -H "Authorization: Bearer ${USER_BEARER_TOKEN}")"
echo "  download-zip: ${download_resp}"

if [[ "$EXPECT_EXHAUST" == "1" ]]; then
  echo "[6/6] verify exhausted on extra call (requires non-subscriber user)"
  # Already consumed once above; call remaining times then one extra
  remaining=$(( DOWNLOAD_TIMES ))
  for ((i=1; i<=remaining; i++)); do
    status="$(curl -sS -o /tmp/collection-card-smoke.$$ -w "%{http_code}" \
      -X GET "${API_BASE}/collections/${COLLECTION_ID}/download-zip" \
      -H "Authorization: Bearer ${USER_BEARER_TOKEN}")"
    body="$(cat /tmp/collection-card-smoke.$$ || true)"
    echo "  call #$i status=${status} body=${body}"
  done
  rm -f /tmp/collection-card-smoke.$$ || true
fi

echo "[DONE] smoke_collection_download_cards"
