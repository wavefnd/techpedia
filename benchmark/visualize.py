#!/usr/bin/env python3
"""Render a TechPedia benchmark JSON report as a standalone HTML dashboard."""

from __future__ import annotations

import argparse
import html
import json
import math
from pathlib import Path
from typing import Any


def number(value: float | int, digits: int = 0) -> str:
    return f"{value:,.{digits}f}"


def duration(value: float) -> str:
    return f"{value:,.0f}ms" if value >= 100 else f"{value:,.1f}ms"


def daily_label(value: int) -> str:
    if value >= 100_000_000:
        return number(value)
    if value >= 10_000:
        return number(value)
    return number(value)


def chart_frame(title: str, body: str, description: str) -> str:
    return f"""
    <figure class="chart">
      <figcaption><strong>{html.escape(title)}</strong><span>{html.escape(description)}</span></figcaption>
      <div class="chart-viewport">
        <svg viewBox="0 0 960 340" role="img" aria-label="{html.escape(title)}">
          {body}
        </svg>
      </div>
    </figure>"""


def throughput_chart(stages: list[dict[str, Any]]) -> str:
    width, height = 960, 340
    left, top, bottom = 72, 24, 70
    plot_height = height - top - bottom
    maximum = max(max(stage["target_rps"], stage["achieved_rps"]) for stage in stages) or 1
    positive = [min(stage["target_rps"], stage["achieved_rps"]) for stage in stages if stage["achieved_rps"] > 0]
    logarithmic = bool(positive) and maximum / min(positive) > 20
    scale_max = math.log10(maximum + 1) if logarithmic else maximum

    def scaled(value: float) -> float:
        transformed = math.log10(value + 1) if logarithmic else value
        return plot_height * transformed / scale_max

    slot = (width - left - 24) / len(stages)
    bar_width = min(30, slot * 0.28)
    parts = []
    tick_values = [maximum * tick / 4 for tick in range(5)]
    if logarithmic:
        tick_values = [value for value in (10, 100, 1_000, 10_000, 100_000) if value <= maximum * 1.1]
    for value in tick_values:
        y = top + plot_height - scaled(value)
        parts.append(f'<line class="grid" x1="{left}" y1="{y:.1f}" x2="936" y2="{y:.1f}"/>')
        parts.append(f'<text class="axis" x="{left - 10}" y="{y + 4:.1f}" text-anchor="end">{value:,.0f}</text>')
    for index, stage in enumerate(stages):
        center = left + slot * (index + 0.5)
        target_height = scaled(stage["target_rps"])
        achieved_height = scaled(stage["achieved_rps"])
        color = "pass" if stage["sustainable"] else "fail"
        parts.append(f'<rect class="target" x="{center - bar_width - 2:.1f}" y="{top + plot_height - target_height:.1f}" width="{bar_width:.1f}" height="{target_height:.1f}"/>')
        parts.append(f'<rect class="{color}" x="{center + 2:.1f}" y="{top + plot_height - achieved_height:.1f}" width="{bar_width:.1f}" height="{achieved_height:.1f}"/>')
        parts.append(f'<text class="axis" x="{center:.1f}" y="{height - 42}" text-anchor="middle">{stage["target_rps"]:,}</text>')
        parts.append(f'<text class="status-{color}" x="{center:.1f}" y="{height - 22}" text-anchor="middle">{"Pass" if stage["sustainable"] else "Fail"}</text>')
    parts.append('<text class="legend" x="710" y="18">Target RPS</text><rect class="target" x="690" y="8" width="12" height="12"/>')
    parts.append('<text class="legend" x="838" y="18">Actual RPS</text><rect class="pass" x="818" y="8" width="12" height="12"/>')
    scale_label = "Log scale" if logarithmic else "Linear scale"
    return chart_frame("Target vs achieved throughput", "".join(parts), f"{scale_label}, horizontal axis is execution order")


def latency_chart(stages: list[dict[str, Any]]) -> str:
    width, height = 960, 340
    left, top, bottom = 72, 24, 70
    plot_height = height - top - bottom
    values = [stage["latency"][key] for stage in stages for key in ("p50_ms", "p95_ms", "p99_ms")]
    maximum = max(max(values), 10)
    log_max = math.log10(maximum + 1)
    slot = (width - left - 24) / max(len(stages) - 1, 1)
    parts = []
    for value in (10, 100, 1_000, 10_000):
        if value > maximum * 1.15:
            continue
        y = top + plot_height - math.log10(value + 1) / log_max * plot_height
        parts.append(f'<line class="grid" x1="{left}" y1="{y:.1f}" x2="936" y2="{y:.1f}"/>')
        parts.append(f'<text class="axis" x="{left - 10}" y="{y + 4:.1f}" text-anchor="end">{value:,}ms</text>')
    series = (("p50_ms", "p50"), ("p95_ms", "p95"), ("p99_ms", "p99"))
    for key, css_class in series:
        points = []
        for index, stage in enumerate(stages):
            x = left + slot * index if len(stages) > 1 else width / 2
            value = stage["latency"][key]
            y = top + plot_height - math.log10(value + 1) / log_max * plot_height
            points.append(f"{x:.1f},{y:.1f}")
            parts.append(f'<circle class="{css_class}" cx="{x:.1f}" cy="{y:.1f}" r="4"><title>{key}: {value:,.2f}ms</title></circle>')
        parts.append(f'<polyline class="line {css_class}" points="{" ".join(points)}"/>')
    for index, stage in enumerate(stages):
        x = left + slot * index if len(stages) > 1 else width / 2
        parts.append(f'<text class="axis" x="{x:.1f}" y="{height - 30}" text-anchor="middle">{stage["target_rps"]:,}</text>')
    parts.append('<text class="legend p50-text" x="700" y="18">p50</text><text class="legend p95-text" x="770" y="18">p95</text><text class="legend p99-text" x="840" y="18">p99</text>')
    return chart_frame("Response latency", "".join(parts), "Log scale, lower is better")


def table_rows(stages: list[dict[str, Any]]) -> str:
    rows = []
    for stage in stages:
        state = "Pass" if stage["sustainable"] else "Fail"
        reason = ", ".join(stage.get("failure_reasons", [])) or "-"
        rows.append(f"""
          <tr>
            <td><span class="badge {'ok' if stage['sustainable'] else 'bad'}">{state}</span></td>
            <td>{stage['target_rps']:,}</td>
            <td>{stage['achieved_rps']:,.0f}</td>
            <td>{stage['achievement_ratio'] * 100:,.1f}%</td>
            <td>{duration(stage['latency']['p50_ms'])}</td>
            <td>{duration(stage['latency']['p95_ms'])}</td>
            <td>{duration(stage['latency']['p99_ms'])}</td>
            <td>{stage['error_rate'] * 100:,.2f}%</td>
            <td>{stage['dropped']:,}</td>
            <td>{stage['peak_in_flight']:,}</td>
            <td>{daily_label(stage.get('modeled_daily_requests', stage['target_rps'] * 86_400))}</td>
            <td class="reason">{html.escape(reason)}</td>
          </tr>""")
    return "".join(rows)


def render_report(report: dict[str, Any]) -> str:
    stages = report.get("stages", [])
    if not stages:
        raise ValueError("Benchmark result has no stages.")
    maximum = int(report.get("maximum_sustainable_rps", 0))
    failure = int(report.get("first_unsustainable_rps", 0))
    peak_factor = float(report.get("peak_factor", 1))
    daily_capacity = int(maximum * 86_400 / peak_factor)
    profile = str(report.get("profile_name", "staircase"))
    boundary = f"at least {maximum:,} RPS"
    if failure:
        boundary = f"at least {maximum:,}, below {failure:,} RPS"
    return f"""<!doctype html>
<html lang="ko">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>TechPedia isolated benchmark</title>
  <style>
    :root {{ color-scheme: light dark; --bg:#f5f7f9; --surface:#fff; --text:#18212a; --muted:#64717d; --line:#d9e0e6; --blue:#2774ae; --green:#16845b; --red:#c53b35; --amber:#b46a00; }}
    @media (prefers-color-scheme:dark) {{ :root {{ --bg:#15191d; --surface:#20262b; --text:#edf2f5; --muted:#a9b4bd; --line:#3a444c; --blue:#70b7e8; --green:#5cd39d; --red:#ff817a; --amber:#f2b455; }} }}
    * {{ box-sizing:border-box }} body {{ margin:0; background:var(--bg); color:var(--text); font:14px/1.5 system-ui,sans-serif }} main {{ width:min(1240px,calc(100% - 32px)); margin:32px auto 64px }}
    h1 {{ margin:0; font-size:28px }} .subtitle {{ color:var(--muted); margin:4px 0 24px }} .kpis {{ display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px }}
    .kpi,.chart,.table-wrap {{ background:var(--surface); border:1px solid var(--line); border-radius:6px }} .kpi {{ padding:16px }} .kpi span {{ color:var(--muted); display:block }} .kpi strong {{ display:block; margin-top:6px; font-size:24px }}
    .charts {{ display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; width:100%; min-width:0; overflow:hidden; margin-top:12px }} .chart {{ width:100%; max-width:100%; min-width:0; margin:0; padding:14px; overflow:hidden }} figcaption {{ display:flex; flex-wrap:wrap; justify-content:space-between; gap:4px 12px; max-width:100%; margin-bottom:8px; overflow:hidden }} figcaption strong,figcaption span {{ min-width:0; overflow-wrap:anywhere }} figcaption span {{ color:var(--muted); font-size:12px; text-align:right }} .chart-viewport {{ width:100%; max-width:100%; min-width:0; overflow-x:auto }} svg {{ display:block; width:100%; max-width:100%; height:auto }}
    .grid {{ stroke:var(--line); stroke-width:1 }} .axis,.legend {{ fill:var(--muted); font-size:11px }} .target {{ fill:var(--blue); opacity:.38 }} .pass {{ fill:var(--green) }} .fail {{ fill:var(--red) }} .status-pass {{ fill:var(--green); font-size:11px }} .status-fail {{ fill:var(--red); font-size:11px }}
    .line {{ fill:none; stroke-width:2.5 }} .line.p50 {{ stroke:var(--blue) }} .line.p95 {{ stroke:var(--amber) }} .line.p99 {{ stroke:var(--red) }} circle.p50 {{ stroke:var(--blue); fill:var(--blue) }} circle.p95 {{ stroke:var(--amber); fill:var(--amber) }} circle.p99 {{ stroke:var(--red); fill:var(--red) }} .p50-text {{ fill:var(--blue) }} .p95-text {{ fill:var(--amber) }} .p99-text {{ fill:var(--red) }}
    .table-wrap {{ margin-top:12px; overflow:auto }} table {{ width:100%; min-width:1120px; border-collapse:collapse }} th,td {{ border-bottom:1px solid var(--line); padding:10px 12px; text-align:right; white-space:nowrap }} th {{ color:var(--muted); background:var(--surface); position:sticky; top:0 }} th:first-child,td:first-child,.reason {{ text-align:left }} .reason {{ max-width:300px; white-space:normal }} .badge {{ border-radius:3px; color:#fff; padding:3px 7px; font-weight:700 }} .badge.ok {{ background:var(--green) }} .badge.bad {{ background:var(--red) }}
    .footnote {{ color:var(--muted); margin-top:14px; font-size:12px }}
    @media (max-width:800px) {{ main {{ width:min(calc(100% - 20px),1240px); margin-top:18px }} .kpis,.charts {{ grid-template-columns:repeat(2,minmax(0,1fr)) }} .kpi strong {{ font-size:19px }} }}
    @media (max-width:520px) {{ .kpis {{ grid-template-columns:minmax(0,1fr) }} .charts {{ display:block }} .chart {{ width:auto; max-width:none }} .chart + .chart {{ margin-top:12px }} .chart-viewport svg {{ width:720px; max-width:none }} h1 {{ font-size:23px }} }}
  </style>
</head>
<body><main>
  <h1>TechPedia isolated benchmark</h1>
  <p class="subtitle">Profile {html.escape(profile)} · {html.escape(report.get('started_at', ''))} · Peak factor {peak_factor:g}×</p>
  <section class="kpis">
    <div class="kpi"><span>Sustainable boundary</span><strong>{boundary}</strong></div>
    <div class="kpi"><span>Modeled daily requests</span><strong>{daily_label(daily_capacity)}</strong></div>
    <div class="kpi"><span>Document fixtures</span><strong>{int(report.get('documents', 0)):,}</strong></div>
    <div class="kpi"><span>Load workers</span><strong>{int(report.get('workers', 0)):,}</strong></div>
  </section>
  <section class="charts">{throughput_chart(stages)}{latency_chart(stages)}</section>
  <section class="table-wrap"><table>
    <thead><tr><th>Result</th><th>Target RPS</th><th>Actual RPS</th><th>Achieved</th><th>p50</th><th>p95</th><th>p99</th><th>Error rate</th><th>Missed</th><th>Peak concurrent</th><th>Daily model</th><th>Failure reason</th></tr></thead>
    <tbody>{table_rows(stages)}</tbody>
  </table></section>
  <p class="footnote">Daily model = target RPS x 86,400 / peak factor. This measures an isolated Go engine, not a production SLA.</p>
</main></body></html>"""


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    report = json.loads(args.input.read_text(encoding="utf-8"))
    rendered = render_report(report)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(rendered, encoding="utf-8")
    print(f"Python report created: {args.output}")


if __name__ == "__main__":
    main()
