#!/usr/bin/env python3
"""Independent output derivatives and raw-array metrics; no neural VJP claim."""
import gzip
import hashlib
import json
import math
from functools import lru_cache
from pathlib import Path
import statistics

HERE = Path(__file__).resolve().parent
SOURCE = HERE.parent / "training-feedback-20261004" / "report.json.gz"
SOURCE_SHA = "27286d7e471543bc7478908dfa8dc3f4433d13a13bac072037139dc7603116de"
COMPONENTS = ("policy", "value", "entropy", "total")
REPORT_SHA = "2bcc65751f3145e08b9b86becbe1d565707150698dac5b9dfa510b85fed434d2"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def close(a, b, tolerance=1e-10):
    require(math.isfinite(a) and math.isfinite(b), "nonfinite number")
    require(abs(a-b) <= tolerance * max(1, abs(a), abs(b)), f"different: {a} {b}")


def finite_tree(value):
    if isinstance(value, dict):
        for item in value.values():
            finite_tree(item)
    elif isinstance(value, list):
        for item in value:
            finite_tree(item)
    elif isinstance(value, float):
        require(math.isfinite(value), "nonfinite array")


class NumberToken(str):
    """Keep Go's original numeric lexemes rather than reformatting floats."""


def compact_tokens(value):
    if isinstance(value, NumberToken):
        return str(value)
    if isinstance(value, dict):
        return "{" + ",".join(compact_tokens(k)+":"+compact_tokens(v) for k, v in value.items()) + "}"
    if isinstance(value, list):
        return "[" + ",".join(compact_tokens(v) for v in value) + "]"
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026").replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")


@lru_cache(maxsize=1)
def initial_model_receipt():
    raw = gzip.decompress((HERE / "report.json.gz").read_bytes())
    require(hashlib.sha256(raw).hexdigest() == REPORT_SHA, "generated report fingerprint")
    lexical = json.loads(raw, parse_float=NumberToken, parse_int=NumberToken)["initial_models"]
    hashes = [hashlib.sha256(compact_tokens(m).encode()).hexdigest() for m in lexical]
    return json.loads(raw)["initial_models"], hashes


def parameter_vector(parameters):
    core = parameters["core"]
    return core["weights"] + core["bias"] + core["log_tau"] + parameters.get("theta_raw", []) + parameters["encoder"] + parameters["readout"]


def adam_delta(raw_gradient, delta, current, state, options):
    # This fixed model has no sharing, schedule, accumulation, weight decay or
    # projection. Frozen coordinates skip moments and the parameter step clock.
    mask = [True]*108 + [False]*28 + [True]*48
    require(len(raw_gradient) == len(delta) == len(current) == len(mask), "optimizer vector width")
    gradient_norm = norm([g for g, enabled in zip(raw_gradient, mask) if enabled])
    scale = min(1, options["clip_norm"]/gradient_norm) if gradient_norm else 1
    for j, (g, actual, enabled) in enumerate(zip(raw_gradient, delta, mask)):
        if not enabled:
            require(actual == 0, "frozen coordinate")
            continue
        d = g*scale
        b1, b2 = options["beta1"], options["beta2"]
        state["first"][j] = b1*state["first"][j] + (1-b1)*d
        state["second"][j] = b2*state["second"][j] + (1-b2)*d*d
        state["steps"][j] += 1
        first = state["first"][j]/(1-b1**state["steps"][j])
        second = state["second"][j]/(1-b2**state["steps"][j])
        after = current[j] - options["learning_rate"]*first/(math.sqrt(second)+options["epsilon"])
        close(actual, after-current[j])
        current[j] += actual


def groups(g):
    core = g["Core"]
    result = {"weights": core["Weights"] or [], "bias": core["Bias"] or [],
              "log_tau": core["LogTau"] or [], "encoder": g["Encoder"] or [],
              "readout": g["Readout"] or [], "theta_raw": g["ThetaRaw"] or [],
              "core_initial": core["Initial"] or []}
    for name, rows in (("inputs", g["Inputs"]), ("core_inputs", core["Inputs"])):
        result[name] = [x for row in (rows or []) for x in row]
    result["shared_core"] = result["weights"] + result["bias"]
    result["trainable"] = result["shared_core"] + result["readout"]
    return result


def norm(values):
    return math.sqrt(math.fsum(x*x for x in values))


def measure(gs, name):
    arrays = {c: gs[c][name] for c in COMPONENTS}
    require(len({len(v) for v in arrays.values()}) == 1, "gradient shape")
    norms = {c: norm(v) for c, v in arrays.items()}
    dot = math.fsum(a*b for a, b in zip(arrays["policy"], arrays["value"]))
    scale = sum(norms[c] for c in COMPONENTS[:3])
    residue = norm([t-p-v-e for t, p, v, e in zip(*(arrays[c] for c in ("total", "policy", "value", "entropy")))])
    relative = residue/scale if scale else 0
    require((scale != 0 or residue == 0) and relative <= 1e-5, "gradient sum")
    return {"norms": norms, "policy_value_dot": dot,
            "policy_value_cosine": dot/(norms["policy"]*norms["value"]) if norms["policy"] and norms["value"] else None,
            "value_policy_ratio": norms["value"]/norms["policy"] if norms["policy"] else None,
            "sum_relative_residual": relative}


def validate(report, source):
    finite_tree(report)
    require(report["schema_version"] == "coimnet-ppo-gradient-diagnostic/v1", "schema")
    require(report["source_gzip_sha256"] == SOURCE_SHA, "source identity")
    require(report["source_raw_sha256"] == "919306c03712d793357373ed6910ccd97f71485448dac55756c99cf3635da96a", "raw identity")
    require(report["config"] == source["config"], "config")
    points = report["points"]
    require(len(points) == len(source["training"]) == 600, "count")
    c = report["config"]["ppo"]
    models = report["initial_models"]
    saved_models, initial_hashes = initial_model_receipt()
    require(len(models) == 3, "initial model count")
    require(models == saved_models, "initial model coordinates changed")
    expected_options = {"learning_rate": .01, "beta1": .9, "beta2": .999, "epsilon": 1e-8,
                        "weight_decay": 0, "clip_norm": 1, "truncation": 0,
                        "trainable": {"encoder": False, "weights": True, "bias": True, "tau": False, "readout": True}}
    current_vectors, adam_states = [], []
    for model_index, model in enumerate(models):
        require(model["config"] == models[0]["config"], "initial topology")
        require(initial_hashes[model_index] == source["training"][model_index*200]["snapshot_before"], "initial snapshot binding")
        options = model["optimizer"]["options"]
        require(options == expected_options, "fixed optimizer settings")
        current_vectors.append(parameter_vector(model["parameters"]))
        adam_states.append({name: list(values) for name, values in model["optimizer"]["state"].items()})
    metrics = []
    for index, (point, old) in enumerate(zip(points, source["training"])):
        seed, update = index//200+1, index % 200+1
        require(point["seed"] == old["seed"] == seed and point["update"] == old["update"] == update, "order")
        for name in ("snapshot_before", "snapshot_after"):
            require(point[name] == old[name], "model identity")
        if update > 1:
            require(point["snapshot_before"] == points[index-1]["snapshot_after"], "snapshot chain")
        split = point["split"]
        out, steps = split["outputs"], old["rollout"]["steps"]
        require(len(out) == len(steps), "output shape")
        for name in COMPONENTS:
            require(len(split["upstreams"][name]) == len(out), "upstream rows")
        loss_parts = {name: [] for name in ("policy", "value", "entropy", "ratio", "clipped")}
        for t, (row, step, advantage, target) in enumerate(zip(out, steps, old["advantages"], old["targets"])):
            require(len(row) == 4, "output width")
            logits, value = row[:-1], row[-1]
            shift = max(logits)
            lse = shift + math.log(sum(math.exp(x-shift) for x in logits))
            logp = [x-lse for x in logits]
            p = [math.exp(x) for x in logp]
            close(logp[step["action"]], step["log_prob"])
            close(value, step["value"])
            ratio = math.exp(logp[step["action"]]-step["log_prob"])
            clipped = min(max(ratio, 1-c["clip_epsilon"]), 1+c["clip_epsilon"])
            active = ratio*advantage <= clipped*advantage
            policy = [-advantage*ratio*((j == step["action"])-p[j]) if active else 0 for j in range(3)] + [0]
            entropy = -sum(q*l for q, l in zip(p, logp))
            regularizer = [c["entropy_coef"]*q*(entropy+l) for q, l in zip(p, logp)] + [0]
            val = [0, 0, 0, 2*c["value_coef"]*(value-target)]
            loss_parts["policy"].append(-min(ratio*advantage, clipped*advantage))
            loss_parts["value"].append(c["value_coef"]*(value-target)**2)
            loss_parts["entropy"].append(-c["entropy_coef"]*entropy)
            loss_parts["ratio"].append(ratio)
            loss_parts["clipped"].append(not active)
            expected = {"policy": policy, "value": val, "entropy": regularizer,
                        "total": [a+b+d for a, b, d in zip(policy, val, regularizer)]}
            for name, want in expected.items():
                actual = split["upstreams"][name][t]
                require(len(actual) == 4, "upstream width")
                for a, b in zip(actual, want):
                    close(a, b, 1e-12)
        original_report = old["update_report"]
        for name in ("policy", "value", "entropy", "ratio"):
            close(sum(loss_parts[name])/len(out), original_report["mean_"+name])
        close(sum(sum(loss_parts[name]) for name in ("policy", "value", "entropy"))/len(out), original_report["mean_loss"])
        close(sum(loss_parts["clipped"])/len(out), original_report["clipped_fraction"])
        require(original_report["rollouts"] == original_report["epochs"] == original_report["version_checks"] == 1 and original_report["transitions"] == len(out), "original report shape")
        gs = {name: groups(split["gradients"][name]) for name in COMPONENTS}
        for name in COMPONENTS:
            for group, length in (("weights", 96), ("bias", 12), ("log_tau", 12), ("encoder", 16), ("readout", 48), ("theta_raw", 0), ("core_initial", 12)):
                require(len(gs[name][group]) == length, "fixed model gradient shape")
            g = split["gradients"][name]
            require(len(g["Inputs"]) == len(out) and all(len(row) == 4 for row in g["Inputs"]), "input gradient shape")
            require(len(g["Core"]["Inputs"]) == len(out) and all(len(row) == 12 for row in g["Core"]["Inputs"]), "core input gradient shape")
        measured = {name: measure(gs, name) for name in gs["total"]}
        step_result = split["optimizer_step"]
        close(step_result["gradient_norm"], measured["trainable"]["norms"]["total"])
        require(step_result["updates"] == update and step_result["applied"] and not step_result["loss_known"], "optimizer clock")
        delta = split["parameter_delta"]
        require(all(x == 0 for x in delta["core"]["log_tau"] + delta["encoder"]), "frozen delta")
        close(step_result["update_norm"], norm(delta["core"]["weights"] + delta["core"]["bias"] + delta["readout"]))
        raw_gradient = [x for name in ("weights", "bias", "log_tau", "theta_raw", "encoder", "readout") for x in gs["total"][name]]
        adam_delta(raw_gradient, parameter_vector(delta), current_vectors[seed-1], adam_states[seed-1], models[seed-1]["optimizer"]["options"])
        metrics.append({"seed": seed, "update": update, "cue": old["original_cue"],
                        "reached": steps[-1]["done"], "groups": measured,
                        "clip_scale": min(1, 1/step_result["gradient_norm"]) if step_result["gradient_norm"] else 1,
                        "update_norm": step_result["update_norm"]})
    rows = []
    for seed in (1, 2, 3):
        seed_metrics = [m for m in metrics if m["seed"] == seed]
        for first, last in ((1, 200), (1, 20), (181, 200)):
            part = [m for m in seed_metrics if first <= m["update"] <= last]
            core = [m["groups"]["shared_core"] for m in part]
            cos = [g["policy_value_cosine"] for g in core if g["policy_value_cosine"] is not None]
            ratios = [g["value_policy_ratio"] for g in core if g["value_policy_ratio"] is not None]
            rows.append({"seed": seed, "first_update": first, "last_update": last,
                         "count": len(part), "shared_core_opposing": sum(x < 0 for x in cos),
                         "shared_core_cosine_defined": len(cos),
                         "shared_core_cosine_median": statistics.median(cos) if cos else None,
                         "shared_core_value_larger": sum(g["norms"]["value"] > g["norms"]["policy"] for g in core),
                         "shared_core_value_policy_ratio_median": statistics.median(ratios) if ratios else None,
                         "gradient_clipped": sum(m["clip_scale"] < 1 for m in part)})
    return {"counts": rows, "updates": metrics,
            "max_gradient_sum_relative_residual": max(g["sum_relative_residual"] for m in metrics for g in m["groups"].values())}


def load():
    data = SOURCE.read_bytes()
    require(hashlib.sha256(data).hexdigest() == SOURCE_SHA, "source file changed")
    return json.loads(gzip.decompress(data))


def verify_reproduction(receipt, directory, raw):
    require(receipt["exact_bytes_equal"] is True and len(receipt["runs"]) == 2, "reproduction receipt")
    for index, entry in enumerate(receipt["runs"]):
        report_name = ("report.json.gz", "run2-report.json.gz")[index]
        log_name = ("run1.log", "run2.log")[index]
        require(entry["run"] == index+1 and entry["fresh_process"] is True and entry["updates"] == 600, "reproduction run")
        require(entry["compressed_report"] == report_name and entry["test_log"] == log_name, "reproduction paths")
        compressed = (directory / report_name).read_bytes()
        candidate = gzip.decompress(compressed)
        require(candidate == raw, "reproduction byte equality")
        require(entry["raw_sha256"] == hashlib.sha256(candidate).hexdigest() == REPORT_SHA, "reproduction raw fingerprint")
        require(entry["compressed_sha256"] == hashlib.sha256(compressed).hexdigest(), "reproduction compressed fingerprint")
        require(entry["raw_bytes"] == len(candidate) and entry["compressed_bytes"] == len(compressed), "reproduction size")
        log = (directory / log_name).read_text()
        require("--- PASS: TestPPOGradientEvidence" in log and "\nPASS\n" in log and "FAIL" not in log, "reproduction test log")


if __name__ == "__main__":
    raw = gzip.decompress((HERE / "report.json.gz").read_bytes())
    verify_reproduction(json.loads((HERE / "reproduction.json").read_text()), HERE, raw)
    summary = validate(json.loads(raw), load())
    expected = json.loads((HERE / "summary.json").read_text())
    require(summary == expected, "summary mismatch")
    print(json.dumps({"updates": 600, "report_raw_sha256": hashlib.sha256(raw).hexdigest(),
                      "max_gradient_sum_relative_residual": summary["max_gradient_sum_relative_residual"],
                      "checks": "initial snapshot fingerprints, objective derivatives and PPO reports, all gradient arrays and metrics, every Adam delta coordinate, frozen deltas, source chain, two-process byte equality"}, indent=2))
