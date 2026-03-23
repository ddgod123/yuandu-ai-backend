#!/usr/bin/env bash
set -euo pipefail

# 用途：
# 清理 archive.collections 中 source=user_video_mvp 的历史视频测试合集，
# 通过后台管理员硬删除接口触发 DB + 七牛对象同步清理。

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

API_BASE="${API_BASE:-http://127.0.0.1:5050}"
WORK_DIR="${WORK_DIR:-tmp/cleanup}"
mkdir -p "$WORK_DIR"
TS="$(date +%Y%m%d_%H%M%S)"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "ERROR: DATABASE_URL is empty, please configure backend/.env"
  exit 1
fi

if ! curl -fsS "${API_BASE}/healthz" >/dev/null; then
  echo "ERROR: backend service not reachable at ${API_BASE}"
  exit 1
fi

PRE_CSV="${WORK_DIR}/video_generated_collections_pre_${TS}.csv"
IDS_TXT="${WORK_DIR}/video_generated_collection_ids_${TS}.txt"
DEL_CSV="${WORK_DIR}/video_generated_delete_${TS}.csv"
DEL_LOG="${WORK_DIR}/video_generated_delete_${TS}.log"

echo "[1/4] Export pre-delete snapshot..."
psql "$DATABASE_URL" -F ',' -A -P footer=off -c \
  "select c.id,c.title,c.source,c.file_count,coalesce(c.qiniu_prefix,''),coalesce(v.id::text,''),to_char(c.created_at,'YYYY-MM-DD HH24:MI:SS')
   from archive.collections c
   left join archive.video_jobs v on v.result_collection_id = c.id
   where c.source='user_video_mvp'
   order by c.id" > "$PRE_CSV"

psql "$DATABASE_URL" -A -t -c \
  "select id from archive.collections where source='user_video_mvp' order by id" > "$IDS_TXT"

TARGET_COUNT="$(wc -l < "$IDS_TXT" | tr -d ' ')"
if [[ "$TARGET_COUNT" == "0" ]]; then
  echo "No source=user_video_mvp collections found. Nothing to delete."
  echo "snapshot: $PRE_CSV"
  exit 0
fi

TOKEN="$(python3 - <<'PY'
import base64, json, hmac, hashlib, time
def b64u(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b'=').decode()
header = b64u(json.dumps({"alg":"HS256","typ":"JWT"}, separators=(',', ':')).encode())
now = int(time.time())
payload = b64u(json.dumps({
    "sub": "1",
    "role": "super_admin",
    "iat": now,
    "exp": now + 24 * 3600
}, separators=(',', ':')).encode())
msg = f"{header}.{payload}"
sig = b64u(hmac.new(b"change-me", msg.encode(), hashlib.sha256).digest())
print(f"{msg}.{sig}")
PY
)"

echo "[2/4] Delete target collections via admin hard-delete API..."
echo "id,http_code,deleted_objects,storage_delete_error,error" > "$DEL_CSV"
total=0
ok=0
fail=0

while IFS= read -r id; do
  [[ -z "$id" ]] && continue
  total=$((total + 1))
  body="$(mktemp)"
  code="$(curl -sS -o "$body" -w "%{http_code}" -X DELETE \
    "${API_BASE}/api/admin/collections/${id}" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json")"

  if [[ "$code" == "200" ]]; then
    deleted_objects="$(jq -r '.result.deleted_objects // 0' "$body" 2>/dev/null || echo 0)"
    storage_err="$(jq -r '.result.storage_delete_error // ""' "$body" 2>/dev/null || echo "")"
    printf '%s,%s,%s,"%s",\n' "$id" "$code" "$deleted_objects" "${storage_err//\"/\"\"}" >> "$DEL_CSV"
    echo "[$total] id=$id OK deleted_objects=$deleted_objects storage_error=$storage_err" >> "$DEL_LOG"
    ok=$((ok + 1))
  else
    err="$(jq -r '.error // .message // "unknown"' "$body" 2>/dev/null || cat "$body")"
    printf '%s,%s,0,,"%s"\n' "$id" "$code" "${err//\"/\"\"}" >> "$DEL_CSV"
    echo "[$total] id=$id FAIL code=$code err=$err" >> "$DEL_LOG"
    fail=$((fail + 1))
  fi
  rm -f "$body"
done < "$IDS_TXT"

echo "done total=$total ok=$ok fail=$fail" | tee -a "$DEL_LOG"

echo "[3/4] Post-delete verification..."
psql "$DATABASE_URL" -P pager=off -c \
  "select coalesce(source,'') as source, count(*) from archive.collections group by 1 order by 2 desc;"

psql "$DATABASE_URL" -P pager=off -c \
  "with target as (select id from archive.collections where source='user_video_mvp'),
        jobs as (select v.id from archive.video_jobs v join target t on t.id=v.result_collection_id)
   select
     (select count(*) from target) as collections,
     (select count(*) from archive.emojis e join target t on t.id=e.collection_id) as emojis,
     (select count(*) from archive.collection_zips z join target t on t.id=z.collection_id) as zips,
     (select count(*) from jobs) as linked_jobs,
     (select count(*) from archive.video_job_artifacts a join jobs j on j.id=a.job_id) as artifacts,
     (select count(*) from archive.video_job_gif_candidates c join jobs j on j.id=c.job_id) as gif_candidates,
     (select count(*) from public.video_image_outputs o join jobs j on j.id=o.job_id) as public_outputs,
     (select count(*) from public.video_image_packages p join jobs j on j.id=p.job_id) as public_packages;"

deleted_objects_sum="$(awk -F',' 'NR>1{sum+=$3} END{print sum+0}' "$DEL_CSV")"
storage_err_count="$(awk -F',' 'NR>1 && $4!=\"\\\"\\\"\"{c++} END{print c+0}' "$DEL_CSV")"

echo "[4/4] Completed."
echo "pre_snapshot: $PRE_CSV"
echo "id_list: $IDS_TXT"
echo "delete_csv: $DEL_CSV"
echo "delete_log: $DEL_LOG"
echo "deleted_objects_sum: $deleted_objects_sum"
echo "storage_delete_error_count: $storage_err_count"

