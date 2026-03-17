#!/usr/bin/env python3
"""Generate SQL comments (Chinese) for tables/columns that currently lack Chinese comments."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def load_env_file(path: Path) -> None:
    if not path.exists():
        return
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        if not key:
            continue
        value = value.strip().strip('"').strip("'")
        os.environ.setdefault(key, value)


def dsn() -> str:
    host = os.getenv("DB_HOST", "localhost")
    port = os.getenv("DB_PORT", "5432")
    user = os.getenv("DB_USER", os.getenv("USER", "postgres"))
    dbname = os.getenv("DB_NAME", "emojiDB")
    sslmode = os.getenv("DB_SSLMODE", "disable")
    parts = [
        f"host={host}",
        f"port={port}",
        f"user={user}",
        f"dbname={dbname}",
        f"sslmode={sslmode}",
    ]
    password = os.getenv("DB_PASSWORD", "")
    if password:
        parts.append(f"password={password}")
    return " ".join(parts)


def run_psql(sql: str) -> list[tuple[str, ...]]:
    proc = subprocess.run(
        ["psql", dsn(), "-v", "ON_ERROR_STOP=1", "-At", "-F", "\t", "-c", sql],
        cwd=ROOT,
        check=True,
        text=True,
        capture_output=True,
    )
    rows: list[tuple[str, ...]] = []
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        rows.append(tuple(line.split("\t")))
    return rows


def q_ident(name: str) -> str:
    return '"' + name.replace('"', '""') + '"'


def q_lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


TABLE_COMMENT_MAP: dict[tuple[str, str], str] = {
    ("action", "collection_downloads"): "合集下载记录表",
    ("action", "collection_favorites"): "合集收藏关系表",
    ("action", "collection_likes"): "合集点赞关系表",
    ("archive", "card_themes"): "小红书卡片主题配置表",
    ("archive", "collection_users"): "合集与用户关联表（所有者/协作者）",
    ("archive", "collection_zips"): "合集压缩包产物记录表",
    ("audit", "home_daily_stats"): "首页每日统计快照表",
    ("ops", "creator_profiles"): "创作者档案配置表",
    ("taxonomy", "ips"): "IP标签管理表",
}


GENERIC_COLUMN_COMMENT_MAP: dict[str, str] = {
    "id": "主键ID",
    "user_id": "用户ID",
    "collection_id": "合集ID",
    "emoji_id": "表情ID",
    "category_id": "分类ID",
    "ip_id": "IP标签ID",
    "creator_profile_id": "创作者档案ID",
    "job_id": "视频任务ID",
    "account_id": "算力账户ID",
    "result_collection_id": "结果合集ID",
    "created_by": "创建人ID",
    "name": "名称",
    "name_zh": "中文名称",
    "name_en": "英文名称",
    "slug": "唯一标识slug",
    "title": "标题",
    "description": "描述",
    "note": "备注",
    "remark": "备注",
    "message": "消息内容",
    "config": "配置JSON",
    "metadata": "扩展元数据JSON",
    "options": "任务参数JSON",
    "metrics": "任务指标JSON",
    "details": "成本明细JSON",
    "zip_key": "压缩包存储Key",
    "zip_name": "压缩包文件名",
    "zip_hash": "压缩包哈希",
    "latest_zip_key": "最新压缩包存储Key",
    "latest_zip_name": "最新压缩包文件名",
    "latest_zip_size": "最新压缩包大小（字节）",
    "latest_zip_at": "最新压缩包时间",
    "download_code": "下载码",
    "source_video_key": "源视频存储Key",
    "source_video_url": "源视频访问地址",
    "qiniu_key": "七牛对象Key",
    "mime_type": "MIME类型",
    "file_name": "文件名",
    "file_size": "文件大小（字节）",
    "size_bytes": "文件大小（字节）",
    "storage_bytes_raw": "原始数据存储大小（字节）",
    "storage_bytes_output": "输出数据存储大小（字节）",
    "output_count": "输出文件数量",
    "output_formats": "输出格式列表",
    "status": "状态",
    "stage": "处理阶段",
    "progress": "处理进度（百分比）",
    "priority": "优先级",
    "type": "类型",
    "level": "日志级别",
    "role": "角色",
    "scope": "作用域",
    "target": "目标标识",
    "action": "动作",
    "severity": "风险等级",
    "reason": "原因",
    "error_message": "错误信息",
    "currency": "币种",
    "pricing_version": "计费版本",
    "cpu_ms": "CPU耗时（毫秒）",
    "gpu_ms": "GPU耗时（毫秒）",
    "asr_seconds": "音频识别时长（秒）",
    "ocr_frames": "OCR处理帧数",
    "available_points": "可用点数",
    "frozen_points": "冻结点数",
    "debt_points": "欠费点数",
    "total_consumed_points": "累计消耗点数",
    "total_recharged_points": "累计充值点数",
    "points": "点数变动",
    "reserved_points": "预扣点数",
    "settled_points": "结算点数",
    "available_before": "变更前可用点数",
    "available_after": "变更后可用点数",
    "frozen_before": "变更前冻结点数",
    "frozen_after": "变更后冻结点数",
    "debt_before": "变更前欠费点数",
    "debt_after": "变更后欠费点数",
    "uploaded_at": "上传时间",
    "queued_at": "排队时间",
    "started_at": "开始时间",
    "finished_at": "完成时间",
    "settled_at": "结算时间",
    "expires_at": "过期时间",
    "created_at": "创建时间",
    "updated_at": "更新时间",
    "deleted_at": "删除时间（软删除）",
    "stat_date": "统计日期",
    "total_collections": "合集总数",
    "total_emojis": "表情总数",
    "today_new_emojis": "当日新增表情数",
    "display_order": "展示排序",
    "cover_url": "封面图地址",
    "avatar_url": "头像地址",
    "website_url": "个人网站地址",
    "location": "所在地",
    "username": "用户名",
    "bio": "个人简介",
    "is_system": "是否系统内置",
    "is_designer": "是否设计师",
    "verified_at": "认证时间",
    "last_login_at": "最后登录时间",
    "last_login_ip": "最后登录IP",
    "subscription_status": "订阅状态",
    "subscription_plan": "订阅套餐",
    "subscription_expires_at": "订阅到期时间",
    "subscription_started_at": "订阅开始时间",
    "width": "宽度（像素）",
    "height": "高度（像素）",
    "duration_ms": "时长（毫秒）",
    "window_label": "统计窗口标签",
    "confirm_windows": "连续确认窗口数",
    "from_rollout_percent": "调整前rollout百分比",
    "to_rollout_percent": "调整后rollout百分比",
    "recommendation_state": "建议状态",
    "recommendation_reason": "建议原因",
    "consecutive_required": "连续确认阈值",
    "consecutive_matched": "已连续匹配窗口数",
}


SPECIFIC_COLUMN_COMMENT_MAP: dict[tuple[str, str, str], str] = {
    ("action", "collection_downloads", "ip"): "下载来源IP",
    ("archive", "video_jobs", "queued_at"): "进入队列时间",
    ("archive", "video_jobs", "progress"): "处理进度（0-100）",
    ("archive", "video_jobs", "priority"): "任务优先级",
    ("archive", "video_jobs", "title"): "任务标题",
    ("archive", "video_jobs", "status"): "任务状态",
    ("archive", "video_jobs", "stage"): "任务处理阶段",
    ("audit", "category_daily_stats", "id"): "分类统计记录ID",
    ("audit", "search_term_daily_stats", "id"): "搜索词统计记录ID",
    ("ops", "upload_tasks", "started_at"): "任务启动时间",
    ("ops", "creator_profiles", "status"): "档案状态",
    ("taxonomy", "ips", "sort"): "排序值",
    ("taxonomy", "ips", "status"): "启用状态",
    ("ops", "video_quality_settings", "id"): "配置主键（固定为1）",
}


def table_comment(schema: str, table: str, current_comment: str) -> str:
    if (schema, table) in TABLE_COMMENT_MAP:
        return TABLE_COMMENT_MAP[(schema, table)]
    if current_comment and any("\u4e00" <= ch <= "\u9fff" for ch in current_comment):
        return current_comment
    return f"{schema}模式{table}业务表"


def column_comment(schema: str, table: str, column: str, current_comment: str) -> str:
    key = (schema, table, column)
    if key in SPECIFIC_COLUMN_COMMENT_MAP:
        return SPECIFIC_COLUMN_COMMENT_MAP[key]
    if column in GENERIC_COLUMN_COMMENT_MAP:
        return GENERIC_COLUMN_COMMENT_MAP[column]
    return f"{table}表字段：{column}"


def main() -> None:
    load_env_file(ROOT / ".env")
    load_env_file(ROOT / "backend" / ".env")

    missing_tables = run_psql(
        """
SELECT n.nspname AS schema_name, c.relname AS table_name, COALESCE(obj_description(c.oid,'pg_class'),'') AS current_comment
FROM pg_class c
JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE c.relkind IN ('r','p')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg_toast%'
  AND (obj_description(c.oid,'pg_class') IS NULL OR obj_description(c.oid,'pg_class') !~ '[一-龥]')
ORDER BY n.nspname, c.relname;
        """.strip()
    )

    missing_columns = run_psql(
        """
SELECT n.nspname AS schema_name,
       c.relname AS table_name,
       a.attname AS column_name,
       COALESCE(col_description(c.oid,a.attnum),'') AS current_comment
FROM pg_class c
JOIN pg_namespace n ON n.oid=c.relnamespace
JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
WHERE c.relkind IN ('r','p')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg_toast%'
  AND (col_description(c.oid,a.attnum) IS NULL OR col_description(c.oid,a.attnum) !~ '[一-龥]')
ORDER BY n.nspname, c.relname, a.attnum;
        """.strip()
    )

    lines: list[str] = ["BEGIN;", ""]

    for schema, table, current in missing_tables:
        comment = table_comment(schema, table, current)
        lines.append(f"COMMENT ON TABLE {q_ident(schema)}.{q_ident(table)} IS {q_lit(comment)};")

    if missing_tables:
        lines.append("")

    for schema, table, column, current in missing_columns:
        comment = column_comment(schema, table, column, current)
        lines.append(
            f"COMMENT ON COLUMN {q_ident(schema)}.{q_ident(table)}.{q_ident(column)} IS {q_lit(comment)};"
        )

    lines.append("")
    lines.append("COMMIT;")
    lines.append("")

    migration_path = ROOT / "backend" / "migrations" / "045_fill_missing_zh_comments.sql"
    migration_path.write_text("\n".join(lines), encoding="utf-8")

    print(f"Generated {migration_path}")
    print(f"tables: {len(missing_tables)}, columns: {len(missing_columns)}")


if __name__ == "__main__":
    main()
