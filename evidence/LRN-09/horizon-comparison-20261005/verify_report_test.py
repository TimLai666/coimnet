#!/usr/bin/env python3
"""Meaningful corruption tests for the saved horizon comparison."""
import copy
import gzip
import json
import unittest

import verify_report as v


class ReadbackTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.report = json.loads(gzip.decompress((v.HERE / "report.json.gz").read_bytes()))

    def test_hand_terminal_reference(self):
        steps = [{"value": value, "reward": reward, "done": i == 2, "timeout": False}
                 for i, (value, reward) in enumerate(zip((1, 2, 3), (0, 0, 1)))]
        aa, targets = v.feedback.advantages(steps)
        for a, b in zip(aa + targets, [.1232045, -.911, -2, 1.1232045, 1.089, 1]):
            v.feedback.close(a, b)

    def test_saved_reports_and_summary(self):
        v.verify_saved()

    def test_first_collection_corruption_with_recomputed_targets(self):
        for index in (0, 200, 400):
            for field in ("value", "log_prob"):
                with self.subTest(index=index, field=field):
                    altered = copy.deepcopy(self.report)
                    p = altered["arms"][1]["training"][index]["feedback"]
                    p["rollout"]["steps"][0][field] += .01
                    aa, targets = v.feedback.advantages(p["rollout"]["steps"])
                    p["advantages"], p["targets"] = aa, targets
                    stats, n = p["update_report"], len(aa)
                    stats["mean_policy"] = -sum(aa) / n
                    stats["mean_value"] = .5 * sum(a * a for a in aa) / n
                    stats["mean_loss"] = stats["mean_policy"] + stats["mean_value"] + stats["mean_entropy"]
                    with self.assertRaises(AssertionError):
                        v.verify(altered)

    def test_corruptions_are_rejected(self):
        def point(r):
            return r["arms"][1]["training"][0]

        mutations = {
            "point_missing": lambda r: r["arms"][1]["training"].pop(),
            "raw_timeout_changed": lambda r: point(r)["collected_end"].update(timeout=False),
            "used_termination_changed": lambda r: point(r)["feedback"]["rollout"]["steps"][-1].update(done=False),
            "bootstrap_restored": lambda r: point(r)["feedback"]["rollout"]["steps"][-1].update(bootstrap_value=.1),
            "target_changed": lambda r: point(r)["feedback"]["targets"].__setitem__(0, .123),
            "advantage_changed": lambda r: point(r)["feedback"]["advantages"].__setitem__(0, .123),
            "reward_changed": lambda r: point(r)["feedback"]["rollout"]["steps"][0].update(reward=2),
            "invalid_action": lambda r: point(r)["feedback"]["rollout"]["steps"][0].update(action=3),
            "nonfinite_value": lambda r: point(r)["feedback"]["rollout"]["steps"][0].update(value=float("nan")),
            "seed_changed": lambda r: point(r)["feedback"].update(env_seed=0),
            "snapshot_chain_changed": lambda r: r["arms"][1]["training"][1]["feedback"].update(snapshot_before="0" * 64),
            "initial_model_changed": lambda r: point(r)["feedback"].update(snapshot_before="0" * 64),
            "condition_missing": lambda r: r["arms"][1]["runs"].pop(),
            "success_summary_changed": lambda r: r["arms"][1]["runs"][0]["metrics"].update(reached=99),
            "gate_changed": lambda r: r["arms"][1]["gate_by_seed"].update({"1": not r["arms"][1]["gate_by_seed"]["1"]}),
            "eval_cue_changed": lambda r: r["arms"][1]["runs"][7]["episodes"][0]["trace"][0]["observation"].__setitem__(0, 1),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name):
                altered = copy.deepcopy(self.report)
                mutate(altered)
                with self.assertRaises(AssertionError):
                    v.verify(altered)


if __name__ == "__main__":
    unittest.main(verbosity=2)
