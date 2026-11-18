from __future__ import annotations

import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from traffic_profile import build_profile, parse_count  # noqa: E402
from visualize import render_report  # noqa: E402


class TrafficProfileTest(unittest.TestCase):
    def test_suffix_and_peak_conversion(self) -> None:
        self.assertEqual(parse_count("10m"), 10_000_000)
        rows = build_profile("1m,10m,100m", 10)
        self.assertEqual([row[2] for row in rows], [116, 1158, 11575])

    def test_rejects_non_increasing_targets(self) -> None:
        with self.assertRaises(ValueError):
            build_profile("10m,1m", 10)


class VisualizationTest(unittest.TestCase):
    def test_standalone_report_contains_metrics(self) -> None:
        report = {
            "started_at": "2026-07-18T00:00:00Z",
            "documents": 1000,
            "workers": 4096,
            "profile_name": "test",
            "peak_factor": 10,
            "maximum_sustainable_rps": 390,
            "first_unsustainable_rps": 406,
            "stages": [
                {
                    "target_rps": 390,
                    "achieved_rps": 382.6,
                    "achievement_ratio": 0.981,
                    "error_rate": 0,
                    "dropped": 0,
                    "peak_in_flight": 141,
                    "modeled_daily_requests": 3_369_600,
                    "latency": {"p50_ms": 181.8, "p95_ms": 432.6, "p99_ms": 549.9},
                    "sustainable": True,
                }
            ],
        }
        output = render_report(report)
        self.assertIn("at least 390, below 406 RPS", output)
        self.assertIn("<svg", output)
        self.assertNotIn("https://", output)


if __name__ == "__main__":
    unittest.main()
