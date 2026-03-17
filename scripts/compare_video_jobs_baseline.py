#!/usr/bin/env python3
"""Compare two video jobs baseline snapshots exported by export_video_jobs_baseline.sh."""

from __future__ import annotations

import argparse
import json
import math
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Tuple


@dataclass(frozen=True)
class MetricSpec:
    key: str
    label: str
    better: str  # up | down | neutral
    unit: str = ""


METRIC_SPECS: Tuple[MetricSpec, ...] = (
    MetricSpec("created_window", "Created Jobs", "neutral"),
    MetricSpec("done_window", "Done Jobs", "up"),
    MetricSpec("failed_window", "Failed Jobs", "down"),
    MetricSpec("duration_p50_sec", "Duration P50", "down", "s"),
    MetricSpec("duration_p95_sec", "Duration P95", "down", "s"),
    MetricSpec("cost_window", "Window Cost", "down", "CNY"),
    MetricSpec("cost_avg_window", "Avg Cost", "down", "CNY"),
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Compare before/after baseline snapshots for video jobs overview."
    )
    parser.add_argument("before", help="before snapshot json path")
    parser.add_argument("after", help="after snapshot json path")
    parser.add_argument(
        "--out-json",
        dest="out_json",
        default="",
        help="optional output path for machine-readable comparison json",
    )
    return parser.parse_args()


def load_snapshot(path: str) -> Dict[str, Any]:
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    if "overview" in raw and isinstance(raw["overview"], dict):
        snapshot = dict(raw)
        snapshot["overview"] = dict(raw["overview"])
        return snapshot
    # allow comparing overview-only payloads directly
    return {"captured_at": "", "window": "", "api_base": "", "overview": raw}


def as_float(value: Any) -> Optional[float]:
    if isinstance(value, (int, float)):
        if isinstance(value, float) and (math.isnan(value) or math.isinf(value)):
            return None
        return float(value)
    return None


def as_int(value: Any) -> int:
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, (int, float)):
        return int(value)
    return 0


def format_value(value: Optional[float], unit: str = "") -> str:
    if value is None:
        return "-"
    if unit == "s":
        return f"{value:.2f}s"
    if unit == "CNY":
        return f"¥{value:.4f}"
    if abs(value - round(value)) < 1e-9:
        return str(int(round(value)))
    return f"{value:.4f}"


def format_delta(delta: Optional[float], unit: str = "") -> str:
    if delta is None:
        return "-"
    sign = "+" if delta >= 0 else ""
    if unit == "s":
        return f"{sign}{delta:.2f}s"
    if unit == "CNY":
        return f"{sign}¥{delta:.4f}"
    if abs(delta - round(delta)) < 1e-9:
        return f"{sign}{int(round(delta))}"
    return f"{sign}{delta:.4f}"


def format_pct(base: Optional[float], delta: Optional[float]) -> str:
    if base is None or delta is None:
        return "-"
    if abs(base) < 1e-12:
        return "n/a"
    pct = delta / base * 100.0
    sign = "+" if pct >= 0 else ""
    return f"{sign}{pct:.2f}%"


def metric_trend(spec: MetricSpec, delta: Optional[float]) -> str:
    if delta is None or abs(delta) < 1e-12:
        return "equal"
    if spec.better == "neutral":
        return "n/a"
    improved = (delta > 0 and spec.better == "up") or (delta < 0 and spec.better == "down")
    return "improved" if improved else "regressed"


def render_metric_rows(before_ov: Dict[str, Any], after_ov: Dict[str, Any]) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    for spec in METRIC_SPECS:
        b = as_float(before_ov.get(spec.key))
        a = as_float(after_ov.get(spec.key))
        delta = None if b is None or a is None else a - b
        rows.append(
            {
                "key": spec.key,
                "label": spec.label,
                "before": b,
                "after": a,
                "delta": delta,
                "change_pct": None if b is None or delta is None or abs(b) < 1e-12 else (delta / b * 100.0),
                "trend": metric_trend(spec, delta),
                "unit": spec.unit,
            }
        )
    return rows


def index_format_stats(data: Any) -> Dict[str, Dict[str, Any]]:
    result: Dict[str, Dict[str, Any]] = {}
    if not isinstance(data, list):
        return result
    for item in data:
        if not isinstance(item, dict):
            continue
        fmt = str(item.get("format", "")).strip().lower()
        if not fmt:
            continue
        result[fmt] = item
    return result


def compare_format_stats(before_ov: Dict[str, Any], after_ov: Dict[str, Any]) -> List[Dict[str, Any]]:
    before_map = index_format_stats(before_ov.get("format_stats_24h"))
    after_map = index_format_stats(after_ov.get("format_stats_24h"))
    formats = sorted(set(before_map.keys()) | set(after_map.keys()))
    rows: List[Dict[str, Any]] = []
    for fmt in formats:
        b = before_map.get(fmt, {})
        a = after_map.get(fmt, {})
        b_req = as_int(b.get("requested_jobs"))
        a_req = as_int(a.get("requested_jobs"))
        b_gen = as_int(b.get("generated_jobs"))
        a_gen = as_int(a.get("generated_jobs"))
        b_rate = as_float(b.get("success_rate")) or 0.0
        a_rate = as_float(a.get("success_rate")) or 0.0
        b_size = as_float(b.get("avg_artifact_size_bytes")) or 0.0
        a_size = as_float(a.get("avg_artifact_size_bytes")) or 0.0
        rows.append(
            {
                "format": fmt,
                "requested_before": b_req,
                "requested_after": a_req,
                "generated_before": b_gen,
                "generated_after": a_gen,
                "success_rate_before": b_rate,
                "success_rate_after": a_rate,
                "success_rate_delta_pp": (a_rate - b_rate) * 100.0,
                "avg_size_before": b_size,
                "avg_size_after": a_size,
                "avg_size_delta_pct": ((a_size - b_size) / b_size * 100.0) if b_size > 0 else None,
            }
        )
    return rows


def index_stage_durations(data: Any) -> Dict[str, Dict[str, Any]]:
    result: Dict[str, Dict[str, Any]] = {}
    if not isinstance(data, list):
        return result
    for item in data:
        if not isinstance(item, dict):
            continue
        transition = str(item.get("transition", "")).strip()
        if not transition:
            from_stage = str(item.get("from_stage", "")).strip()
            to_stage = str(item.get("to_stage", "")).strip()
            if from_stage and to_stage:
                transition = f"{from_stage} -> {to_stage}"
        if not transition:
            continue
        result[transition] = item
    return result


def compare_stage_durations(before_ov: Dict[str, Any], after_ov: Dict[str, Any]) -> List[Dict[str, Any]]:
    before_map = index_stage_durations(before_ov.get("stage_durations"))
    after_map = index_stage_durations(after_ov.get("stage_durations"))
    transitions = sorted(set(before_map.keys()) | set(after_map.keys()))
    rows: List[Dict[str, Any]] = []
    for transition in transitions:
        b = before_map.get(transition, {})
        a = after_map.get(transition, {})
        b_avg = as_float(b.get("avg_sec")) or 0.0
        a_avg = as_float(a.get("avg_sec")) or 0.0
        b_p95 = as_float(b.get("p95_sec")) or 0.0
        a_p95 = as_float(a.get("p95_sec")) or 0.0
        rows.append(
            {
                "transition": transition,
                "count_before": as_int(b.get("count")),
                "count_after": as_int(a.get("count")),
                "avg_before": b_avg,
                "avg_after": a_avg,
                "avg_delta_pct": ((a_avg - b_avg) / b_avg * 100.0) if b_avg > 0 else None,
                "p95_before": b_p95,
                "p95_after": a_p95,
                "p95_delta_pct": ((a_p95 - b_p95) / b_p95 * 100.0) if b_p95 > 0 else None,
            }
        )
    rows.sort(key=lambda item: abs(item["avg_after"] - item["avg_before"]), reverse=True)
    return rows


def print_header(before: Dict[str, Any], after: Dict[str, Any]) -> None:
    print("=== Video Jobs Baseline Compare ===")
    print(f"Before: {before.get('captured_at', '-') or '-'} | window={before.get('window', '-') or '-'}")
    print(f"After : {after.get('captured_at', '-') or '-'} | window={after.get('window', '-') or '-'}")
    print("")


def print_metric_table(rows: Iterable[Dict[str, Any]]) -> None:
    print("[Core Metrics]")
    print(f"{'Metric':<18} {'Before':>12} {'After':>12} {'Delta':>12} {'Change':>10} {'Trend':>10}")
    for item in rows:
        unit = str(item.get("unit", ""))
        before = format_value(item.get("before"), unit)
        after = format_value(item.get("after"), unit)
        delta = format_delta(item.get("delta"), unit)
        change = format_pct(item.get("before"), item.get("delta"))
        trend = str(item.get("trend", "-"))
        label = str(item.get("label", ""))[:18]
        print(f"{label:<18} {before:>12} {after:>12} {delta:>12} {change:>10} {trend:>10}")
    print("")


def print_format_table(rows: List[Dict[str, Any]]) -> None:
    print("[Format Success & Size]")
    if not rows:
        print("No format stats.")
        print("")
        return
    print(f"{'Format':<8} {'Success(B->A)':<22} {'Size(B->A)':<28} {'Req(B->A)':<16}")
    for item in rows:
        rate_before = item["success_rate_before"] * 100.0
        rate_after = item["success_rate_after"] * 100.0
        rate_delta = item["success_rate_delta_pp"]
        rate_text = f"{rate_before:.1f}% -> {rate_after:.1f}% ({rate_delta:+.1f}pp)"
        size_before = item["avg_size_before"]
        size_after = item["avg_size_after"]
        size_delta_pct = item["avg_size_delta_pct"]
        if size_before <= 0 and size_after <= 0:
            size_text = "- -> -"
        else:
            size_text = f"{size_before/1024:.1f}KB -> {size_after/1024:.1f}KB"
            if size_delta_pct is not None:
                size_text += f" ({size_delta_pct:+.1f}%)"
        req_text = f"{item['requested_before']} -> {item['requested_after']}"
        print(f"{item['format']:<8} {rate_text:<22} {size_text:<28} {req_text:<16}")
    print("")


def print_stage_table(rows: List[Dict[str, Any]], limit: int = 10) -> None:
    print("[Stage Duration Delta Top]")
    if not rows:
        print("No stage duration stats.")
        print("")
        return
    print(f"{'Transition':<28} {'Avg(B->A)':<22} {'P95(B->A)':<22} {'Count(B->A)':<14}")
    for item in rows[:limit]:
        avg_text = f"{item['avg_before']:.2f}s -> {item['avg_after']:.2f}s"
        if item["avg_delta_pct"] is not None:
            avg_text += f" ({item['avg_delta_pct']:+.1f}%)"
        p95_text = f"{item['p95_before']:.2f}s -> {item['p95_after']:.2f}s"
        if item["p95_delta_pct"] is not None:
            p95_text += f" ({item['p95_delta_pct']:+.1f}%)"
        count_text = f"{item['count_before']} -> {item['count_after']}"
        print(f"{item['transition']:<28} {avg_text:<22} {p95_text:<22} {count_text:<14}")
    print("")


def build_result(
    before: Dict[str, Any],
    after: Dict[str, Any],
    metrics: List[Dict[str, Any]],
    format_rows: List[Dict[str, Any]],
    stage_rows: List[Dict[str, Any]],
) -> Dict[str, Any]:
    return {
        "before": {
            "captured_at": before.get("captured_at", ""),
            "window": before.get("window", ""),
            "api_base": before.get("api_base", ""),
        },
        "after": {
            "captured_at": after.get("captured_at", ""),
            "window": after.get("window", ""),
            "api_base": after.get("api_base", ""),
        },
        "metrics": metrics,
        "formats": format_rows,
        "stages": stage_rows,
    }


def main() -> int:
    args = parse_args()
    before = load_snapshot(args.before)
    after = load_snapshot(args.after)

    before_ov = before.get("overview", {})
    after_ov = after.get("overview", {})

    metrics = render_metric_rows(before_ov, after_ov)
    format_rows = compare_format_stats(before_ov, after_ov)
    stage_rows = compare_stage_durations(before_ov, after_ov)

    print_header(before, after)
    print_metric_table(metrics)
    print_format_table(format_rows)
    print_stage_table(stage_rows, limit=10)

    if args.out_json:
        out = build_result(before, after, metrics, format_rows, stage_rows)
        Path(args.out_json).parent.mkdir(parents=True, exist_ok=True)
        Path(args.out_json).write_text(json.dumps(out, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"Comparison json exported: {args.out_json}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
