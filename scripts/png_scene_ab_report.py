#!/usr/bin/env python3
"""
PNG 主线 A/B 对照报告工具

用途：
1) 对照同源视频在 default / xiaohongshu 两个场景下的 AI1→AI2→Worker 链路差异；
2) 快速验证主线关注的三项指标：
   - quality_weights 是否有效拉开
   - must_capture / avoid 命中与拒绝分布
   - pipeline_alignment_report_v1 一致性是否稳定

用法（两种输入）：
  A. 直接读 API：
     python3 scripts/png_scene_ab_report.py \
       --base-url http://127.0.0.1:8080 \
       --token "$USER_JWT" \
       --default-job 123 \
       --xhs-job 124

  B. 读本地 JSON（来自 ai1-debug 导出）：
     python3 scripts/png_scene_ab_report.py \
       --default-json /tmp/default.json \
       --xhs-json /tmp/xhs.json
"""

from __future__ import annotations

import argparse
import csv
import json
import ssl
import sys
import urllib.request
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple


WEIGHT_KEYS = ("semantic", "clarity", "loop", "efficiency")


def _as_dict(v: Any) -> Dict[str, Any]:
    return v if isinstance(v, dict) else {}


def _as_list(v: Any) -> List[Any]:
    return v if isinstance(v, list) else []


def _as_str(v: Any) -> str:
    if v is None:
        return ""
    return str(v).strip()


def _as_int(v: Any) -> int:
    try:
        return int(v)
    except Exception:
        return 0


def _as_float(v: Any) -> float:
    try:
        return float(v)
    except Exception:
        return 0.0


def _safe_ratio(n: float, d: float) -> float:
    if d <= 0:
        return 0.0
    return n / d


def _find_first_map(debug_payload: Dict[str, Any], key: str) -> Dict[str, Any]:
    output = _as_dict(debug_payload.get("output"))
    focus = _as_dict(debug_payload.get("focus"))
    trace = _as_dict(debug_payload.get("trace"))
    for container in (output, focus, trace):
        value = _as_dict(container.get(key))
        if value:
            return value
    return {}


def _parse_weights(raw: Any) -> Dict[str, float]:
    m = _as_dict(raw)
    out: Dict[str, float] = {}
    for key in WEIGHT_KEYS:
        out[key] = _as_float(m.get(key))
    return out


def _extract_checks(alignment: Dict[str, Any]) -> Dict[str, str]:
    checks: Dict[str, str] = {}
    for row in _as_list(alignment.get("consistency_checks")):
        item = _as_dict(row)
        key = _as_str(item.get("key"))
        status = _as_str(item.get("status"))
        if key:
            checks[key] = status
    return checks


def _extract_summary(debug_payload: Dict[str, Any]) -> Dict[str, Any]:
    output = _as_dict(debug_payload.get("output"))
    ai2_instruction = _as_dict(output.get("ai2_instruction"))
    alignment = _find_first_map(debug_payload, "pipeline_alignment_report_v1")
    ai2_obs = _find_first_map(debug_payload, "ai2_execution_observability_v1")

    scenario = _as_dict(alignment.get("scenario"))
    scene = _as_str(scenario.get("scene")) or _as_str(_as_dict(ai2_instruction.get("advanced_options")).get("scene"))
    scene_label = _as_str(scenario.get("scene_label"))
    if not scene_label:
        scene_label = _as_str(_as_dict(ai2_instruction.get("strategy_profile")).get("scene_label"))

    weights = _parse_weights(ai2_obs.get("effective_quality_weights"))
    if not any(weights.values()):
        weights = _parse_weights(ai2_instruction.get("quality_weights"))

    must_capture = [str(x).strip() for x in _as_list(ai2_instruction.get("must_capture")) if str(x).strip()]
    avoid = [str(x).strip() for x in _as_list(ai2_instruction.get("avoid")) if str(x).strip()]

    frame_summary = _as_dict(ai2_obs.get("frame_quality_summary"))
    reject_counts = _as_dict(frame_summary.get("reject_counts"))
    candidate_explainability = _as_dict(ai2_obs.get("candidate_explainability"))

    total_candidates = _as_int(candidate_explainability.get("total_candidates"))
    must_capture_hit_frames = _as_int(candidate_explainability.get("must_capture_hit_frames"))
    avoid_hit_frames = _as_int(candidate_explainability.get("avoid_hit_frames"))

    kept_frames = _as_int(frame_summary.get("kept_frames"))
    total_frames = _as_int(frame_summary.get("total_frames"))

    checks = _extract_checks(alignment)
    fail_checks = sorted([k for k, v in checks.items() if v and v.lower() not in ("pass", "ok", "done", "success")])

    return {
        "job_id": _as_int(debug_payload.get("job_id")),
        "requested_format": _as_str(debug_payload.get("requested_format")),
        "scene": scene or "default",
        "scene_label": scene_label or scene or "default",
        "weights": weights,
        "must_capture_count": len(must_capture),
        "avoid_count": len(avoid),
        "must_capture_hit_frames": must_capture_hit_frames,
        "avoid_hit_frames": avoid_hit_frames,
        "total_candidates": total_candidates,
        "must_capture_hit_ratio": _safe_ratio(must_capture_hit_frames, total_candidates),
        "avoid_hit_ratio": _safe_ratio(avoid_hit_frames, total_candidates),
        "kept_frames": kept_frames,
        "total_frames": total_frames,
        "kept_ratio": _safe_ratio(kept_frames, total_frames),
        "reject_counts": {
            "blur": _as_int(reject_counts.get("blur")),
            "brightness": _as_int(reject_counts.get("brightness")),
            "exposure": _as_int(reject_counts.get("exposure")),
            "resolution": _as_int(reject_counts.get("resolution")),
            "still_blur": _as_int(reject_counts.get("still_blur")),
            "watermark": _as_int(reject_counts.get("watermark")),
            "near_dup": _as_int(reject_counts.get("near_dup")),
            "total_reject": _as_int(reject_counts.get("total_reject")),
        },
        "alignment_status": _as_str(_as_dict(alignment.get("summary")).get("status")),
        "consistency_checks": checks,
        "failed_checks": fail_checks,
    }


def _fetch_debug_from_api(
    base_url: str,
    token: str,
    job_id: int,
    insecure: bool = False,
    timeout_sec: int = 20,
) -> Dict[str, Any]:
    base = base_url.rstrip("/")
    url = f"{base}/api/video-jobs/{job_id}/ai1-debug"
    req = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
    ssl_ctx = None
    if insecure:
        ssl_ctx = ssl.create_default_context()
        ssl_ctx.check_hostname = False
        ssl_ctx.verify_mode = ssl.CERT_NONE
    with urllib.request.urlopen(req, timeout=timeout_sec, context=ssl_ctx) as resp:
        raw = resp.read().decode("utf-8")
        return _as_dict(json.loads(raw))


def _load_json_file(path: str) -> Dict[str, Any]:
    content = Path(path).read_text(encoding="utf-8")
    return _as_dict(json.loads(content))


def _format_pct(v: float) -> str:
    return f"{v * 100:.1f}%"


def _print_compare(default_row: Dict[str, Any], xhs_row: Dict[str, Any]) -> None:
    print("=== PNG 主线 A/B 对照报告 ===")
    print(f"default job: {default_row['job_id']}  | scene: {default_row['scene_label']} ({default_row['scene']})")
    print(f"xhs job:     {xhs_row['job_id']}  | scene: {xhs_row['scene_label']} ({xhs_row['scene']})")
    print("")

    print("[1] quality_weights 对照")
    for key in WEIGHT_KEYS:
        d = float(default_row["weights"].get(key, 0.0))
        x = float(xhs_row["weights"].get(key, 0.0))
        delta = x - d
        print(f"- {key:>10}: default={d:.4f} | xhs={x:.4f} | delta={delta:+.4f}")
    print("")

    print("[2] must/avoid 命中与拒绝分布")
    print(
        f"- default 命中: must={default_row['must_capture_hit_frames']}/{default_row['total_candidates']} "
        f"({_format_pct(default_row['must_capture_hit_ratio'])}), "
        f"avoid={default_row['avoid_hit_frames']}/{default_row['total_candidates']} "
        f"({_format_pct(default_row['avoid_hit_ratio'])})"
    )
    print(
        f"- xhs     命中: must={xhs_row['must_capture_hit_frames']}/{xhs_row['total_candidates']} "
        f"({_format_pct(xhs_row['must_capture_hit_ratio'])}), "
        f"avoid={xhs_row['avoid_hit_frames']}/{xhs_row['total_candidates']} "
        f"({_format_pct(xhs_row['avoid_hit_ratio'])})"
    )
    print(
        f"- default 保留率: {default_row['kept_frames']}/{default_row['total_frames']} "
        f"({_format_pct(default_row['kept_ratio'])})"
    )
    print(
        f"- xhs     保留率: {xhs_row['kept_frames']}/{xhs_row['total_frames']} "
        f"({_format_pct(xhs_row['kept_ratio'])})"
    )
    print(f"- default reject_counts: {json.dumps(default_row['reject_counts'], ensure_ascii=False)}")
    print(f"- xhs     reject_counts: {json.dumps(xhs_row['reject_counts'], ensure_ascii=False)}")
    print("")

    print("[3] pipeline_alignment_report 一致性")
    print(f"- default alignment_status: {default_row['alignment_status'] or '(empty)'}")
    print(f"- xhs     alignment_status: {xhs_row['alignment_status'] or '(empty)'}")
    print(f"- default failed_checks: {default_row['failed_checks']}")
    print(f"- xhs     failed_checks: {xhs_row['failed_checks']}")
    print("")

    check_keys = sorted(set(default_row["consistency_checks"].keys()) | set(xhs_row["consistency_checks"].keys()))
    if check_keys:
        print("[4] consistency_checks 对照")
        for key in check_keys:
            dv = default_row["consistency_checks"].get(key, "")
            xv = xhs_row["consistency_checks"].get(key, "")
            print(f"- {key}: default={dv or '-'} | xhs={xv or '-'}")
        print("")


def _write_json(path: str, payload: Dict[str, Any]) -> None:
    Path(path).write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def _write_csv(path: str, rows: List[Tuple[str, Dict[str, Any]]]) -> None:
    header = [
        "variant",
        "job_id",
        "scene",
        "scene_label",
        "w_semantic",
        "w_clarity",
        "w_loop",
        "w_efficiency",
        "must_capture_count",
        "avoid_count",
        "must_capture_hit_frames",
        "avoid_hit_frames",
        "total_candidates",
        "must_capture_hit_ratio",
        "avoid_hit_ratio",
        "kept_frames",
        "total_frames",
        "kept_ratio",
        "alignment_status",
        "failed_checks",
        "reject_blur",
        "reject_brightness",
        "reject_exposure",
        "reject_resolution",
        "reject_still_blur",
        "reject_watermark",
        "reject_near_dup",
        "reject_total",
    ]
    with Path(path).open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=header)
        writer.writeheader()
        for variant, row in rows:
            writer.writerow(
                {
                    "variant": variant,
                    "job_id": row["job_id"],
                    "scene": row["scene"],
                    "scene_label": row["scene_label"],
                    "w_semantic": row["weights"].get("semantic", 0.0),
                    "w_clarity": row["weights"].get("clarity", 0.0),
                    "w_loop": row["weights"].get("loop", 0.0),
                    "w_efficiency": row["weights"].get("efficiency", 0.0),
                    "must_capture_count": row["must_capture_count"],
                    "avoid_count": row["avoid_count"],
                    "must_capture_hit_frames": row["must_capture_hit_frames"],
                    "avoid_hit_frames": row["avoid_hit_frames"],
                    "total_candidates": row["total_candidates"],
                    "must_capture_hit_ratio": round(row["must_capture_hit_ratio"], 6),
                    "avoid_hit_ratio": round(row["avoid_hit_ratio"], 6),
                    "kept_frames": row["kept_frames"],
                    "total_frames": row["total_frames"],
                    "kept_ratio": round(row["kept_ratio"], 6),
                    "alignment_status": row["alignment_status"],
                    "failed_checks": "|".join(row["failed_checks"]),
                    "reject_blur": row["reject_counts"]["blur"],
                    "reject_brightness": row["reject_counts"]["brightness"],
                    "reject_exposure": row["reject_counts"]["exposure"],
                    "reject_resolution": row["reject_counts"]["resolution"],
                    "reject_still_blur": row["reject_counts"]["still_blur"],
                    "reject_watermark": row["reject_counts"]["watermark"],
                    "reject_near_dup": row["reject_counts"]["near_dup"],
                    "reject_total": row["reject_counts"]["total_reject"],
                }
            )


def _parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="PNG 主线 default vs xiaohongshu A/B 报告")
    p.add_argument("--base-url", default="", help="API base url, e.g. http://127.0.0.1:8080")
    p.add_argument("--token", default="", help="用户 JWT（调用 /api/video-jobs/:id/ai1-debug）")
    p.add_argument("--default-job", type=int, default=0, help="default 场景 job id")
    p.add_argument("--xhs-job", type=int, default=0, help="xiaohongshu 场景 job id")
    p.add_argument("--default-json", default="", help="default 场景 ai1-debug JSON 文件")
    p.add_argument("--xhs-json", default="", help="xiaohongshu 场景 ai1-debug JSON 文件")
    p.add_argument("--insecure", action="store_true", help="忽略 HTTPS 证书校验")
    p.add_argument("--timeout-sec", type=int, default=20, help="HTTP 超时时间（秒）")
    p.add_argument("--out-json", default="", help="输出聚合 JSON 文件路径")
    p.add_argument("--out-csv", default="", help="输出 CSV 文件路径")
    return p.parse_args()


def main() -> int:
    args = _parse_args()

    use_api = bool(args.base_url and args.token and args.default_job > 0 and args.xhs_job > 0)
    use_files = bool(args.default_json and args.xhs_json)
    if not use_api and not use_files:
        print(
            "参数不足：请使用 API 模式（--base-url --token --default-job --xhs-job）"
            "或文件模式（--default-json --xhs-json）",
            file=sys.stderr,
        )
        return 2

    try:
        if use_api:
            default_payload = _fetch_debug_from_api(
                base_url=args.base_url,
                token=args.token,
                job_id=args.default_job,
                insecure=args.insecure,
                timeout_sec=args.timeout_sec,
            )
            xhs_payload = _fetch_debug_from_api(
                base_url=args.base_url,
                token=args.token,
                job_id=args.xhs_job,
                insecure=args.insecure,
                timeout_sec=args.timeout_sec,
            )
        else:
            default_payload = _load_json_file(args.default_json)
            xhs_payload = _load_json_file(args.xhs_json)
    except Exception as e:
        print(f"加载输入失败: {e}", file=sys.stderr)
        return 1

    default_row = _extract_summary(default_payload)
    xhs_row = _extract_summary(xhs_payload)
    _print_compare(default_row, xhs_row)

    bundle = {
        "schema_version": "png_scene_ab_report_v1",
        "default": default_row,
        "xiaohongshu": xhs_row,
        "weight_delta_xhs_minus_default": {
            key: round(float(xhs_row["weights"].get(key, 0.0)) - float(default_row["weights"].get(key, 0.0)), 6)
            for key in WEIGHT_KEYS
        },
    }

    if args.out_json:
        _write_json(args.out_json, bundle)
        print(f"[OK] 已写入 JSON: {args.out_json}")
    if args.out_csv:
        _write_csv(args.out_csv, [("default", default_row), ("xiaohongshu", xhs_row)])
        print(f"[OK] 已写入 CSV: {args.out_csv}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())

