"""Recheck captured DAT-07 evidence; run from any directory with Python 3."""
import hashlib
import json
import math
import re
import struct
from pathlib import Path

base = Path(__file__).resolve().parent


def read(path):
    return json.loads((base / path).read_text())


audit = read("audit.json")
imported = read("build/import.json")
graph = imported["report"]
edges = audit["canonical_edges"]
assert audit["node_count"] == graph["annotated_neurons"]["nodes"] == 24
assert audit["edge_count"] == graph["annotated_neurons"]["edges"] == len(edges) == 110
assert audit["weight_sum"] == graph["annotated_neurons"]["weight_sum"]["value"] == 1514
for key in ("self_loops", "isolated_nodes", "duplicate_pairs"):
    assert audit[key] == graph["annotated_neurons"][key] == 0
assert audit["node_index_hash"] == graph["hashes"]["node_index"]
edge_hash = hashlib.sha256()
for edge in edges:
    edge_hash.update(struct.pack(">QQQBq", edge["source_index"], edge["target_index"],
                                 edge["absolute_row"], edge["valid"], edge["weight"]))
assert edge_hash.hexdigest() == audit["edge_order_hash"] == graph["hashes"]["edge_order"]
before = dict(line.split("  ", 1)[::-1] for line in (base / "build/input.sha256").read_text().splitlines())
after = dict(line.split("  ", 1)[::-1] for line in (base / "build/originals-after.sha256").read_text().splitlines())
assert len(after) == 4 and all(before[path] == sha for path, sha in after.items())
sort_listing = (base / "build/sort-after.txt").read_text().splitlines()
assert sort_listing[0] == "total 0"
assert sorted(line.split()[-1] for line in sort_listing[1:]) == [".", ".."]
incoming = [0.0] * 24
for edge in edges:
    incoming[edge["target_index"]] += edge["weight"]
weights = [.1 * edge["weight"] / incoming[edge["target_index"]]
           if incoming[edge["target_index"]] else 0 for edge in edges]
weight_hash = hashlib.sha256(json.dumps(weights, separators=(",", ":")).encode()).hexdigest()
reports, observations = {}, {}
for platform in ("macos", "ubuntu"):
    directory = f"{platform}-v10"
    report = read(f"{directory}/run-1.json")
    assert (base / directory / "run-1.json").read_bytes() == (base / directory / "run-2.json").read_bytes()
    assert report["profile"] == "real-subgraph" and report["graph_report"] == graph
    assert report["store"]["sha256"] == imported["store"]["sha256"]
    execution = read(f"{directory}/execution.json")
    assert execution["store_sha256_before"] == execution["store_sha256_after"] == report["store"]["sha256"]
    assert [item["external_id"] for item in report["neuron_ids"]] == audit["selected_ids"]
    config, model = report["model"]["config"]["dynamics"], report["model"]
    assert config["sources"] == [edge["source_index"] for edge in edges]
    assert config["targets"] == [edge["target_index"] for edge in edges]
    assert config["delays"] == [0] * 110
    assert model["initial_weights_hash"] == weight_hash != model["final_weights_hash"]
    assert model["initial_parameter_hash"] != model["final_parameter_hash"]
    assert model["initial_frozen_parameter_hash"] == model["final_frozen_parameter_hash"]
    training = report["training"]
    assert training["updates"] == len(training["steps"]) == 20
    for step in training["steps"]:
        assert all(math.isfinite(step[key]) for key in ("loss", "gradient_norm", "update_norm", "weight_delta_norm"))
    assert any(step["weight_delta_norm"] > 0 for step in training["steps"])
    observations[platform] = []
    for index in (1, 2):
        log = (base / directory / f"run-{index}.log").read_text()
        if platform == "macos":
            seconds = float(re.search(r"([\d.]+) real", log)[1])
            rss = int(re.search(r"(\d+)  maximum resident set size", log)[1])
        else:
            seconds = float(re.search(r"time \(h:mm:ss or m:ss\): 0:([\d.]+)", log)[1])
            rss = int(re.search(r"Maximum resident set size \(kbytes\): (\d+)", log)[1]) * 1024
        observations[platform].append({"run": index, "seconds": seconds, "peak_rss_bytes": rss})
    reports[platform] = report
a, b = reports["macos"], reports["ubuntu"]
differences = []
for index, (left, right) in enumerate(zip(a["training"]["steps"], b["training"]["steps"])):
    for key in ("loss", "gradient_norm", "update_norm", "weight_delta_norm"):
        if left[key] != right[key]:
            differences.append({"update": index + 1, "metric": key, "absolute_difference": abs(left[key] - right[key])})
print(json.dumps({
    "result": "passed_scoped", "profile": "real-subgraph", "nodes": 24, "edges": 110,
    "selection": graph["predicate"], "store_sha256": imported["store"]["sha256"],
    "independent_source_audit_matches": True, "all_ids_edges_and_initial_weights_match": True,
    "source_files_and_original_graph_unchanged": True, "repeat_json_identical_within_each_platform": True,
    "frozen_parameters_unchanged": True, "before_mean_loss": a["training"]["before_mean_loss"],
    "after_mean_loss": a["training"]["after_mean_loss"],
    "cross_platform_loss_summaries_equal": a["training"]["before_mean_loss"] == b["training"]["before_mean_loss"] and a["training"]["after_mean_loss"] == b["training"]["after_mean_loss"],
    "cross_platform_final_parameter_hash_equal": a["model"]["final_parameter_hash"] == b["model"]["final_parameter_hash"],
    "cross_platform_step_metric_differences": differences, "observations": observations,
    "limitations": ["Synthetic task uses the same two samples for updates and loss summaries; no held-out or biological claim.",
                    "Final parameters are not exported; cross-platform parameter equality or error bounds are not established.",
                    "CPU execution only; no full-graph training or GPU verification.",
                    "Two process observations per platform, cache not flushed and hosts not isolated; no performance guarantee."]
}, indent=2))
