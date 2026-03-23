#!/usr/bin/env bash
set -euo pipefail

# V1 release gate smoke:
# 1) create users + password login
# 2) single emoji download (normal user)
# 3) collection zip download (subscriber)
# 4) collection download card redeem + consume + exhausted
# 5) dangerous public write/storage API guard
#
# Usage:
#   cd backend
#   ./scripts/smoke_v1_archive_release_gate.sh
#
# Optional env:
#   API_BASE=http://127.0.0.1:5050/api
#   TEST_PASSWORD='Aa123456!'

API_BASE="${API_BASE:-http://127.0.0.1:5050/api}"
TEST_PASSWORD="${TEST_PASSWORD:-Aa123456!}"
SMOKE_TAG="smoke_v1_gate_$(date +%s)"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT_DIR}"

source .env >/dev/null 2>&1 || true

if ! curl -sS -m 3 "${API_BASE%/api}/healthz" >/dev/null 2>&1; then
  echo "[ERROR] API is not reachable: ${API_BASE%/api}/healthz"
  exit 1
fi

if [[ -z "${DB_HOST:-}" || -z "${DB_USER:-}" || -z "${DB_NAME:-}" ]]; then
  echo "[ERROR] DB_* env is required (.env)"
  exit 1
fi

PSQL=(psql -h "${DB_HOST}" -p "${DB_PORT:-5432}" -U "${DB_USER}" -d "${DB_NAME}" -Atq)
if [[ -n "${DB_PASSWORD:-}" ]]; then
  export PGPASSWORD="${DB_PASSWORD}"
fi

json_get() {
  local payload="$1"
  local field="$2"
  python3 - "$payload" "$field" <<'PY'
import json,sys
payload=sys.argv[1]
field=sys.argv[2]
try:
    data=json.loads(payload)
except Exception:
    print("")
    raise SystemExit(0)
cur=data
for part in field.split("."):
    if isinstance(cur, dict):
        cur=cur.get(part)
    else:
        cur=None
        break
if cur is None:
    print("")
elif isinstance(cur, (dict,list)):
    print(json.dumps(cur, ensure_ascii=False))
else:
    print(cur)
PY
}

expect_status() {
  local got="$1"
  local want="$2"
  local hint="$3"
  if [[ "${got}" != "${want}" ]]; then
    echo "[ERROR] ${hint}: expect=${want} got=${got}"
    return 1
  fi
  return 0
}

rand_phone() {
  python3 - <<'PY'
import random,time
seed=int(time.time()*1000)%1000000000
tail=(seed+random.randint(0,99999))%1000000000
print("19%09d"%tail)
PY
}

COLLECTION_ID="$("${PSQL[@]}" -c "select id from archive.collections where status='active' and visibility='public' and coalesce(latest_zip_key,'')<>'' order by id limit 1;")"
EMOJI_ID="$("${PSQL[@]}" -c "select e.id from archive.emojis e join archive.collections c on c.id=e.collection_id where e.status='active' and c.status='active' and c.visibility='public' order by e.id limit 1;")"

if [[ -z "${COLLECTION_ID}" || -z "${EMOJI_ID}" ]]; then
  echo "[ERROR] missing sample data: collection_id='${COLLECTION_ID}' emoji_id='${EMOJI_ID}'"
  exit 1
fi

PHONE_NORMAL="$(rand_phone)"
PHONE_SUBSCRIBER="$(rand_phone)"
PHONE_ADMIN="$(rand_phone)"

cleanup() {
  set +e
  "${PSQL[@]}" -c "delete from ops.collection_download_codes where note='${SMOKE_TAG}';" >/dev/null 2>&1
  "${PSQL[@]}" -c "delete from \"user\".users where phone in ('${PHONE_NORMAL}','${PHONE_SUBSCRIBER}','${PHONE_ADMIN}');" >/dev/null 2>&1
}
trap cleanup EXIT

echo "[INFO] collection_id=${COLLECTION_ID} emoji_id=${EMOJI_ID}"

# Pre-generated bcrypt hash for password "Aa123456!"
# Keep in sync with TEST_PASSWORD when changed.
BCRYPT_HASH='$2a$10$PqIUqqvxA05iATBtPpOORu.fB3AFToDjBCnY5C3xkJ27ibww0aBwy'

create_user() {
  local phone="$1"
  local name="$2"
  local role="$3"
  "${PSQL[@]}" -c "insert into \"user\".users (phone,password_hash,display_name,avatar_url,role,status) values ('${phone}','${BCRYPT_HASH}','${name}','','${role}','active') returning id;"
}

login_user() {
  local phone="$1"
  curl -sS -X POST "${API_BASE}/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"phone\":\"${phone}\",\"password\":\"${TEST_PASSWORD}\"}"
}

echo "[1/8] create+login normal user"
normal_uid="$(create_user "${PHONE_NORMAL}" "smoke_normal" "user")"
if [[ -z "${normal_uid}" ]]; then
  echo "[ERROR] create normal user failed"
  exit 1
fi
normal_login="$(login_user "${PHONE_NORMAL}")"
NORMAL_TOKEN="$(json_get "${normal_login}" "tokens.access_token")"
if [[ -z "${NORMAL_TOKEN}" ]]; then
  echo "[ERROR] login normal failed: ${normal_login}"
  exit 1
fi
echo "  ok user_id=${normal_uid}"

echo "[2/8] normal user single emoji download"
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X GET "${API_BASE}/emojis/${EMOJI_ID}/download" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}")"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
expect_status "${status}" "200" "single emoji download" || { echo "${body}"; exit 1; }
echo "  ok status=${status}"

echo "[3/8] normal user collection zip should be blocked before entitlement"
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X GET "${API_BASE}/collections/${COLLECTION_ID}/download-zip" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}")"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
expect_status "${status}" "403" "collection zip forbidden for normal user" || { echo "${body}"; exit 1; }
echo "  ok status=${status}"

echo "[4/8] create subscriber user and verify collection zip allowed"
sub_uid="$(create_user "${PHONE_SUBSCRIBER}" "smoke_subscriber" "user")"
if [[ -z "${sub_uid}" ]]; then
  echo "[ERROR] create subscriber user failed"
  exit 1
fi
"${PSQL[@]}" -c "update \"user\".users set subscription_status='active', subscription_plan='subscriber', subscription_started_at=now(), subscription_expires_at=now()+interval '30 days' where id=${sub_uid};" >/dev/null
sub_login="$(login_user "${PHONE_SUBSCRIBER}")"
SUB_TOKEN="$(json_get "${sub_login}" "tokens.access_token")"
if [[ -z "${SUB_TOKEN}" ]]; then
  echo "[ERROR] login subscriber failed: ${sub_login}"
  exit 1
fi
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X GET "${API_BASE}/collections/${COLLECTION_ID}/download-zip" \
  -H "Authorization: Bearer ${SUB_TOKEN}")"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
expect_status "${status}" "200" "collection zip for subscriber" || { echo "${body}"; exit 1; }
echo "  ok status=${status}"

echo "[5/8] create temporary super_admin and generate collection download card"
admin_uid="$(create_user "${PHONE_ADMIN}" "smoke_admin" "super_admin")"
if [[ -z "${admin_uid}" ]]; then
  echo "[ERROR] create admin user failed"
  exit 1
fi
"${PSQL[@]}" -c "insert into \"user\".admin_roles (user_id, role) values (${admin_uid}, 'super_admin') on conflict (user_id, role) do nothing;" >/dev/null
admin_login="$(login_user "${PHONE_ADMIN}")"
ADMIN_TOKEN="$(json_get "${admin_login}" "tokens.access_token")"
if [[ -z "${ADMIN_TOKEN}" ]]; then
  echo "[ERROR] login super_admin failed: ${admin_login}"
  exit 1
fi

generate_resp="$(curl -sS -X POST "${API_BASE}/admin/collection-download-codes/generate" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"count\":1,\"collection_id\":${COLLECTION_ID},\"download_times\":2,\"max_redeem_users\":1,\"note\":\"${SMOKE_TAG}\"}")"
CARD_CODE="$(python3 - <<'PY' "${generate_resp}"
import json,sys
try:
    data=json.loads(sys.argv[1]); codes=data.get("codes") or []
    print(codes[0] if codes else "")
except Exception:
    print("")
PY
)"
if [[ -z "${CARD_CODE}" ]]; then
  echo "[ERROR] generate card failed: ${generate_resp}"
  exit 1
fi
echo "  ok code=${CARD_CODE}"

echo "[6/8] normal user validate+redeem card"
validate_resp="$(curl -sS -X POST "${API_BASE}/me/collection-download-code/validate" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"code\":\"${CARD_CODE}\"}")"
valid_flag="$(json_get "${validate_resp}" "valid")"
if [[ "${valid_flag}" != "True" && "${valid_flag}" != "true" ]]; then
  echo "[ERROR] validate card failed: ${validate_resp}"
  exit 1
fi
redeem_resp="$(curl -sS -X POST "${API_BASE}/me/collection-download-code/redeem" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"code\":\"${CARD_CODE}\"}")"
remaining="$(json_get "${redeem_resp}" "remaining_download_times")"
if [[ -z "${remaining}" ]]; then
  echo "[ERROR] redeem card failed: ${redeem_resp}"
  exit 1
fi
echo "  ok remaining=${remaining}"

echo "[7/8] consume card and verify exhausted"
for i in 1 2 3; do
  resp_file="$(mktemp)"
  status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X GET "${API_BASE}/collections/${COLLECTION_ID}/download-zip" \
    -H "Authorization: Bearer ${NORMAL_TOKEN}")"
  body="$(cat "${resp_file}")"; rm -f "${resp_file}"
  echo "  call#${i} status=${status}"
  if [[ "${i}" -le 2 ]]; then
    expect_status "${status}" "200" "card consume call#${i}" || { echo "${body}"; exit 1; }
  else
    expect_status "${status}" "403" "card exhausted" || { echo "${body}"; exit 1; }
    exhausted_code="$(json_get "${body}" "error")"
    if [[ "${exhausted_code}" != "collection_download_entitlement_exhausted" ]]; then
      echo "[ERROR] unexpected exhausted error: ${body}"
      exit 1
    fi
  fi
done

echo "[8/8] dangerous API guard checks"
# anonymous content-write must not be allowed
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X POST "${API_BASE}/collections" -H "Content-Type: application/json" -d '{}')"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
if [[ "${status}" != "401" && "${status}" != "403" ]]; then
  echo "[ERROR] anonymous POST /collections is not blocked: status=${status} body=${body}"
  exit 1
fi
# normal user content-write must be forbidden
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X POST "${API_BASE}/collections" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}" -H "Content-Type: application/json" -d '{}')"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
expect_status "${status}" "403" "normal user POST /collections forbidden" || { echo "${body}"; exit 1; }

# storage url forbidden for non-emoji key
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X GET "${API_BASE}/storage/url?key=private/a.txt" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}")"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
expect_status "${status}" "403" "storage url non-emoji key forbidden" || { echo "${body}"; exit 1; }

# storage private=1 forbidden for non-admin
resp_file="$(mktemp)"
status="$(curl -sS -o "${resp_file}" -w "%{http_code}" -X GET "${API_BASE}/storage/url?key=emoji/a.txt&private=1" \
  -H "Authorization: Bearer ${NORMAL_TOKEN}")"
body="$(cat "${resp_file}")"; rm -f "${resp_file}"
expect_status "${status}" "403" "storage private=1 forbidden for non-admin" || { echo "${body}"; exit 1; }

echo ""
echo "[PASS] V1 archive release smoke passed"
echo "  normal_user=${PHONE_NORMAL}"
echo "  subscriber_user=${PHONE_SUBSCRIBER}"
echo "  admin_user=${PHONE_ADMIN}"
echo "  collection_id=${COLLECTION_ID} emoji_id=${EMOJI_ID}"
