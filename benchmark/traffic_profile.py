#!/usr/bin/env python3
"""Convert daily request volumes into average and peak RPS targets."""

from __future__ import annotations

import argparse
import math
import re

SECONDS_PER_DAY = 86_400
SUFFIXES = {"k": 1_000, "m": 1_000_000, "b": 1_000_000_000}


def parse_count(value: str) -> int:
    normalized = value.strip().lower().replace("_", "").replace(",", "")
    match = re.fullmatch(r"(\d+(?:\.\d+)?)\s*([kmb]?)", normalized)
    if not match:
        raise ValueError(f"Invalid daily request volume: {value}")
    number = float(match.group(1))
    count = int(number * SUFFIXES.get(match.group(2), 1))
    if count <= 0:
        raise ValueError("Daily requests must be positive.")
    return count


def build_profile(values: str, peak_factor: float) -> list[tuple[int, float, int]]:
    if peak_factor < 1:
        raise ValueError("Peak factor must be at least 1.")
    rows = []
    previous = 0
    for raw in values.split(","):
        daily = parse_count(raw)
        average_rps = daily / SECONDS_PER_DAY
        target_rps = math.ceil(average_rps * peak_factor)
        if target_rps <= previous:
            raise ValueError("Daily request volumes must be increasing.")
        rows.append((daily, average_rps, target_rps))
        previous = target_rps
    if not rows:
        raise ValueError("Daily request volumes are required.")
    return rows


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--daily", default="1m,10m,100m", help="comma-separated daily request volumes")
    parser.add_argument("--peak-factor", type=float, default=10.0)
    parser.add_argument("--format", choices=("table", "rps"), default="table")
    args = parser.parse_args()

    try:
        rows = build_profile(args.daily, args.peak_factor)
    except ValueError as error:
        parser.error(str(error))

    if args.format == "rps":
        print(",".join(str(row[2]) for row in rows))
        return

    print(f"Daily request stress profile (peak {args.peak_factor:g}x)")
    print(f"{'Daily requests':>16}  {'Average RPS':>12}  {'Peak target RPS':>14}")
    for daily, average_rps, target_rps in rows:
        print(f"{daily:>16,}  {average_rps:>12,.2f}  {target_rps:>14,}")


if __name__ == "__main__":
    main()
