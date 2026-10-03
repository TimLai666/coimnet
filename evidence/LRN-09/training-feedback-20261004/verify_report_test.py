#!/usr/bin/env python3
"""Reject plausible artifact corruption rather than trusting saved summaries."""
import copy
import gzip
import json
import unittest

import verify_report as v


class VerificationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.report = json.loads(gzip.decompress((v.HERE / "report.json.gz").read_bytes()))
        cls.previous = json.loads(gzip.decompress((v.HERE.parent / "short-goal-cue-audit-20261003/report.json.gz").read_bytes()))

    def test_saved(self):
        self.assertEqual(v.verify_saved()["training_episodes"], 600)

    def test_corruptions(self):
        changes = {
            "config": lambda p: p["config"].update(updates=201),
            "cue": lambda p: p["training"][0].update(original_cue=1),
            "reward": lambda p: p["training"][0]["rollout"]["steps"][0].update(reward=0.99),
            "early_end": lambda p: p["training"][0]["rollout"]["steps"][0].update(done=True),
            "bootstrap": lambda p: p["training"][0]["rollout"]["steps"][-1].update(bootstrap_value=0),
            "advantage": lambda p: p["training"][0]["advantages"].__setitem__(0, 0),
            "target": lambda p: p["training"][0]["targets"].__setitem__(0, 0),
            "chain": lambda p: p["training"][1].update(snapshot_before="0" * 64),
            "version": lambda p: p["training"][1]["rollout"].update(policy_version="0" * 64),
            "probe_action": lambda p: p["probes"][0]["episodes"][0]["trace"][0].update(action=0),
            "probe_checkpoint": lambda p: p["probes"][0].update(snapshot_hash="0" * 64),
            "missing_probe": lambda p: p["probes"].pop(),
            "prior_snapshot": lambda p: p["legacy_report"]["results"][0].update(final_snapshot_hash="0" * 64),
        }
        for name, change in changes.items():
            with self.subTest(name=name):
                report = copy.deepcopy(self.report)
                change(report)
                with self.assertRaises(AssertionError):
                    v.verify(report, self.previous)

    def test_hand_computed_gae(self):
        base = {"reward": 0.99, "value": 0.25, "done": True, "timeout": False}
        self.assertEqual(v.advantages([base]), ([0.74], [0.99]))
        timeout = dict(base, reward=-0.01, done=False, timeout=True, bootstrap_value=0.5)
        a, target = v.advantages([timeout])
        self.assertAlmostEqual(a[0], 0.235)
        self.assertAlmostEqual(target[0], 0.485)


if __name__ == "__main__":
    unittest.main(verbosity=2)
