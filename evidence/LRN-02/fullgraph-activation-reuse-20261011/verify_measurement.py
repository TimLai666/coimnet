"""Independently verify preserved raw measurements without running training."""
import hashlib
import json
from pathlib import Path
import re
import statistics

if not __debug__:
    raise RuntimeError("Verification requires enabled Python assertions; omit -O and PYTHONOPTIMIZE.")

HERE = Path(__file__).resolve().parent

def read(name):
    return json.loads((HERE / name).read_text())

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

records = read("runs.json")
protocol = read("protocol.json")
summary = read("performance-summary.json")
copies = read("data-copy.json")
builds = {r["variant"]: r for r in read("builds.json")}
expected_files = {"store": copies[0]["sha256"], "params": copies[1]["sha256"],
                  "protocol": sha(HERE.parents[2] / "evidence/NAT-05/compare-fullgraph-derived-continuous.json")}
expected_order = [(p, v) for p in range(8) for v in
                  (["baseline", "improved"] if p % 2 == 0 else ["improved", "baseline"])]
assert [(r["pair"], r["variant"]) for r in records] == expected_order
assert protocol["pairs"] == 8 and protocol["measured_pairs"] == 7
reference = None
documents = {}
for record in records:
    name = f"pair-{record['pair']}-{record['variant']}"
    documents[(record["pair"], record["variant"])] = doc = read(name + ".json")
    assert record["exit_code"] == 0 and record["warmup"] == (record["pair"] == 0)
    assert record["binary_sha256"] == builds[record["variant"]]["binary_sha256"]
    assert doc["files"] == expected_files
    assert (doc["Nodes"], doc["Edges"], doc["GOMAXPROCS"], doc["memory_limit_bytes"]) == (165122, 25563197, 1, 8 * 2**30)
    assert (doc["GoVersion"], doc["GOOS"], doc["GOARCH"]) == ("go1.26.5", "darwin", "arm64")
    for suffix in (".json", ".stderr"):
        assert sha(HERE / (name + suffix)) == record["logs"][suffix]
    text = (HERE / (name + ".stderr")).read_text()
    for field, label in (("max_rss_bytes", "maximum resident set size"), ("peak_footprint_bytes", "peak memory footprint")):
        assert int(re.search(r"(\d+)\s+" + label, text).group(1)) == record[field]
    stages = doc["stages"]
    sig = {"topology": doc["topology_sha256"], "input": doc["input_fingerprints"],
           "stages": {k: {field: value[field] for field in ("digests", "recompute_stats", "step_result") if field in value}
                      for k, value in stages.items()}}
    if reference is None:
        reference = sig
    assert sig == reference, name
    expected_shapes = {"weights": 25563197, "bias": 165122, "log_tau": 165122, "initial": 165122, "inputs": 16 * 165122}
    assert stages["forward"]["digests"]["outputs"]["values"] == 16 * 165122
    assert stages["forward"]["digests"]["outputs"]["nonzero"] > 0
    for field, size in expected_shapes.items():
        digest = stages["backward"]["digests"][field]
        assert digest["values"] == size and digest["nonzero"] > 0
    assert stages["backward"]["recompute_stats"] == {"Steps": 16, "Segment": 8, "Checkpoints": 2, "CheckpointRows": 2, "MaxSegmentRows": 9, "FullHistoryRows": 17}
    result = stages["training_step"]["step_result"]
    assert result["applied"] and result["updates"] == 1 and result["gradient_horizon_steps"] == 8
    assert result["update_norm"] > 0 and result["gradient_norm"] > 0
    for field, size in (("weights", 25563197), ("bias", 165122), ("log_tau", 165122), ("encoder", 24), ("readout", 1314)):
        digest = stages["training_step"]["digests"]["parameters_" + field]
        assert digest["values"] == size and digest["nonzero"] > 0
    for field in ("adam_first", "adam_second", "adam_steps"):
        digest = stages["training_step"]["digests"][field]
        assert digest["values"] == 25894779 and digest["nonzero"] > 0
    assert stages["training_step"]["digests"]["adam_steps"]["nonzero"] == 25894779

recomputed = {}
for stage in ("forward", "backward", "training_step"):
    row = summary["stages"][stage]
    for variant in ("baseline", "improved"):
        for field in ("ms", "allocated_bytes", "allocations", "gc_cycles"):
            values = [documents[(p, variant)]["stages"][stage][field] for p in range(1, 8)]
            assert row[variant][field] == {"raw": values, "median": statistics.median(values), "min": min(values), "max": max(values)}
    ratios = [documents[(p, "improved")]["stages"][stage]["ms"] / documents[(p, "baseline")]["stages"][stage]["ms"] for p in range(1, 8)]
    paired = 100 * (1 - statistics.median(ratios))
    assert ratios == row["paired_ratios_new_over_old"]
    assert paired == row["median_paired_reduction_percent"]
    assert 100 * (1 - row["improved"]["ms"]["median"] / row["baseline"]["ms"]["median"]) == row["ratio_of_medians_reduction_percent"]
    recomputed[stage] = paired
for variant in ("baseline", "improved"):
    for field in ("max_rss_bytes", "peak_footprint_bytes", "wall_seconds"):
        values = [r[field] for r in records if r["variant"] == variant and not r["warmup"]]
        assert summary["memory"][variant][field] == {"raw": values, "median": statistics.median(values), "min": min(values), "max": max(values)}
print(json.dumps({"passed": True, "independent_processes": 16, "measured_pairs": 7,
                  "nonzero_gradients": list(expected_shapes), "all_outputs_parameters_adam_exact": True,
                  "raw_log_hashes_verified": True, "summary_recomputed_from_raw": True,
                  "paired_reductions_percent": recomputed, "protocol_sha256": sha(HERE / "protocol.json")}, indent=2))
