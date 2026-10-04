#!/usr/bin/env python3
"""Mutations must fail independent validation rather than just change metrics."""
import copy
import gzip
import json
import math
import unittest

from verify_report import HERE, load, validate, verify_reproduction


class ReportMutations(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.report = json.loads(gzip.decompress((HERE / "report.json.gz").read_bytes()))
        cls.source = load()

    def reject(self, mutate):
        changed = copy.deepcopy(self.report)
        mutate(changed)
        with self.assertRaises((ValueError, KeyError, TypeError)):
            validate(changed, self.source)

    def test_source(self):
        self.reject(lambda r: r.__setitem__("source_gzip_sha256", "changed"))

    def test_count(self):
        self.reject(lambda r: r["points"].pop())

    def test_model(self):
        self.reject(lambda r: r["points"][0].__setitem__("snapshot_before", "changed"))

    def test_order(self):
        self.reject(lambda r: r["points"][0].__setitem__("update", 2))

    def test_upstream(self):
        self.reject(lambda r: r["points"][0]["split"]["upstreams"]["value"][0].__setitem__(3, 1))

    def test_gradient(self):
        self.reject(lambda r: r["points"][0]["split"]["gradients"]["policy"]["Core"]["Weights"].__setitem__(0, 1))

    def test_nan(self):
        self.reject(lambda r: r["points"][0]["split"]["gradients"]["total"]["Encoder"].__setitem__(0, math.nan))

    def test_frozen_delta(self):
        self.reject(lambda r: r["points"][0]["split"]["parameter_delta"]["encoder"].__setitem__(0, 1))

    def test_optimizer_norm(self):
        self.reject(lambda r: r["points"][0]["split"]["optimizer_step"].__setitem__("gradient_norm", 100))

    def test_update_norm(self):
        self.reject(lambda r: r["points"][0]["split"]["optimizer_step"].__setitem__("update_norm", 100))

    def test_output_width(self):
        self.reject(lambda r: r["points"][0]["split"]["outputs"][0].pop())

    def test_component_shape(self):
        self.reject(lambda r: r["points"][0]["split"]["gradients"]["policy"]["Readout"].pop())

    def test_initial_model_coordinates(self):
        self.reject(lambda r: r["initial_models"][0]["parameters"]["readout"].__setitem__(0, 1))

    def test_permuted_delta_coordinates(self):
        def mutate(r):
            delta = r["points"][0]["split"]["parameter_delta"]["core"]["weights"]
            delta[0], delta[1] = delta[1], delta[0]
        self.reject(mutate)

    def test_original_ppo_report(self):
        source = copy.deepcopy(self.source)
        source["training"][0]["update_report"]["mean_value"] += 1
        with self.assertRaises(ValueError):
            validate(self.report, source)

    def test_reproduction_fingerprint(self):
        receipt = json.loads((HERE / "reproduction.json").read_text())
        receipt["runs"][1]["raw_sha256"] = "changed"
        with self.assertRaises(ValueError):
            verify_reproduction(receipt, HERE, gzip.decompress((HERE / "report.json.gz").read_bytes()))

    def test_reproduction_equality(self):
        receipt = json.loads((HERE / "reproduction.json").read_text())
        receipt["exact_bytes_equal"] = False
        with self.assertRaises(ValueError):
            verify_reproduction(receipt, HERE, gzip.decompress((HERE / "report.json.gz").read_bytes()))


if __name__ == "__main__":
    unittest.main(verbosity=2)
