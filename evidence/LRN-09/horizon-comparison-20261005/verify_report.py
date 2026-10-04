#!/usr/bin/env python3
"""Recompute corridor physics, learning targets and cue gates with stdlib only."""
import argparse
import copy
import gzip
import hashlib
import importlib.util
import json
import math
from pathlib import Path

HERE = Path(__file__).resolve().parent
REFERENCES = {"training-feedback-20261004": "27286d7e471543bc7478908dfa8dc3f4433d13a13bac072037139dc7603116de",
              "short-goal-cue-audit-20261003": "85b6a88e35dc251560fdd978b830acf92a1e1bfbca0a5ef702a15a6884effeb5"}


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def reference(name):
    packed = (HERE.parent / name / "report.json.gz").read_bytes()
    assert digest(packed) == REFERENCES[name]
    return json.loads(gzip.decompress(packed))


helper = HERE.parent / "training-feedback-20261004/verify_report.py"
assert digest(helper.read_bytes()) == "c66219126dbd69f69b84c5f3cbb0fb4f8a41791feaf289db95f85e52040689d4"
spec = importlib.util.spec_from_file_location("feedback_reference", helper)
feedback = importlib.util.module_from_spec(spec)
spec.loader.exec_module(feedback)


def evaluate(runs, pairs):
    keys, gates = set(), {}
    for run in runs:
        key = (run["seed"], run["stage"], run["mode"], run["cue"])
        assert key not in keys
        keys.add(key)
        episodes = run["episodes"]
        assert [e["env_seed"] for e in episodes] == [s for p in pairs for s in p]
        counts, returns, lengths = [0] * 6, [], []
        for e in episodes:
            cue, trace = e["original_cue"], e["trace"]
            assert cue in (-1, 1) and 1 <= len(trace) <= 6
            position, goal, total = 3, 0 if cue == -1 else 6, 0.0
            for i, s in enumerate(trace):
                observed = cue * {"original": 1, "erased": 0, "flipped": -1}[run["cue"]] if i == 0 else 0
                assert s["observation"] == [observed, int(position == 0), int(position == 6), i / 6]
                action = s["action"]
                assert type(action) is int and action in (0, 1, 2)
                if run["mode"] == "random":
                    assert "output" not in s
                else:
                    output = s["output"]
                    assert len(output) == 4 and all(math.isfinite(v) for v in output)
                    if run["mode"] == "greedy":
                        assert action == max(range(3), key=lambda a: output[a])
                position = max(0, min(6, position + (-1 if action == 0 else 1 if action == 1 else 0)))
                reached, timeout = position == goal, position != goal and i + 1 == 6
                assert s["evaluator_info"] == {"position": position, "goal": goal, "steps": i + 1, "reached": reached, "timed_out": timeout}
                feedback.close(s["reward"], -.01 + int(reached))
                total += s["reward"]
                assert (reached or timeout) == (i == len(trace) - 1)
            assert e["reached"] is reached
            feedback.close(e["return"], total)
            counts[0] += 1
            counts[1] += reached
            counts[2 if cue == -1 else 4] += 1
            counts[3 if cue == -1 else 5] += reached
            returns.append(total)
            lengths.append(len(trace))
        m = run["metrics"]
        assert [m[k] for k in ("episodes", "reached", "left_episodes", "left_reached", "right_episodes", "right_reached")] == counts
        feedback.close(m["mean_return"], sum(returns) / 40)
        feedback.close(m["mean_steps"], sum(lengths) / 40)
        by_seed = {e["env_seed"]: e for e in episodes}
        for l, r in pairs:
            left, right = by_seed[l], by_seed[r]
            assert (left["original_cue"], right["original_cue"]) == (-1, 1)
            if run["cue"] == "erased" or run["mode"] == "random":
                assert int(left["reached"]) + int(right["reached"]) <= 1
                for i, (ls, rs) in enumerate(zip(left["trace"], right["trace"])):
                    assert ls["observation"][1:] == rs["observation"][1:]
                    if run["cue"] == "erased" or i > 0:
                        assert ls["observation"] == rs["observation"]
                    assert ls["action"] == rs["action"] and ls.get("output") == rs.get("output")
    expected = {(s, stage, mode, cue) for s in (1, 2, 3) for stage in ("before", "after") for mode in ("sampled", "greedy") for cue in ("original", "erased", "flipped")}
    expected |= {(s, "random", "random", "original") for s in (1, 2, 3)}
    assert keys == expected
    for seed in (1, 2, 3):
        def metric(stage, mode="sampled", cue="original"):
            return next(r["metrics"] for r in runs if (r["seed"], r["stage"], r["mode"], r["cue"]) == (seed, stage, mode, cue))
        def rate(m):
            return m["reached"] / 40
        m = metric("after")
        gates[str(seed)] = (m["left_reached"] / 20 >= .8 and m["right_reached"] / 20 >= .8 and
            rate(m) - rate(metric("before")) >= .15 - 1e-12 and rate(m) - rate(metric("random", "random")) >= .15 - 1e-12 and
            rate(m) - rate(metric("after", cue="flipped")) >= .2 - 1e-12 and metric("after", "greedy")["reached"] == 40 and
            rate(m) - rate(metric("after", cue="erased")) >= .2 - 1e-12 and metric("after", "greedy", "erased")["reached"] <= 20 and
            metric("after", "greedy", "flipped")["reached"] == 0)
    return gates


def verify(report):
    prior = reference("short-goal-cue-audit-20261003")
    old_feedback = reference("training-feedback-20261004")
    assert report["schema_version"] == "coimnet-ppo-horizon-comparison/v1"
    assert report["config"] == feedback.CONFIG == prior["config"]
    assert report["config_hash"] == digest(json.dumps(report["config"], separators=(",", ":")).encode()) == prior["config_hash"]
    assert report["eval_stream"] == 0x1005 and report["eval_pairs"] == prior["eval_pairs"]
    assert report["source_gzip_sha256"] == REFERENCES
    arms = report["arms"]
    assert [a["mode"] for a in arms] == ["continuing_bootstrap", "six_step_terminal"]
    assert [p["feedback"] for p in arms[0]["training"]] == old_feedback["training"]
    assert arms[0]["runs"] == prior["runs"]
    summaries = []
    for arm in arms:
        points = arm["training"]
        assert [(p["feedback"]["seed"], p["feedback"]["update"]) for p in points] == [(s, u) for s in (1, 2, 3) for u in range(1, 201)]
        histories = {}
        for record in points:
            p, end = record["feedback"], record["collected_end"]
            seed, u, rollout, stats = p["seed"], p["update"], p["rollout"], p["update_report"]
            assert p["env_seed"] == feedback.mix(seed ^ ((u * 0x9e3779b97f4a7c15) & ((1 << 64) - 1)))
            assert rollout["initial_neural"] == old_feedback["training"][0]["rollout"]["initial_neural"]
            assert "initial_plastic" not in rollout and "initial_chemical" not in rollout
            raw = copy.deepcopy(rollout["steps"])
            raw[-1] = end
            feedback.replay(raw, p["original_cue"], p["return"])
            expected = copy.deepcopy(raw)
            if arm["mode"] == "six_step_terminal" and end["timeout"]:
                expected[-1]["done"], expected[-1]["timeout"] = True, False
                expected[-1].pop("bootstrap_value", None)
            assert rollout["steps"] == expected
            aa, targets = feedback.advantages(expected)
            assert len(p["advantages"]) == len(p["targets"]) == len(expected)
            for actual, value in zip(p["advantages"] + p["targets"], aa + targets):
                feedback.close(actual, value)
            assert stats["rollouts"] == stats["epochs"] == stats["version_checks"] == 1 and stats["transitions"] == len(expected)
            feedback.close(stats["mean_policy"], -sum(aa) / len(aa))
            feedback.close(stats["mean_value"], .5 * sum(a * a for a in aa) / len(aa))
            feedback.close(stats["mean_loss"], stats["mean_policy"] + stats["mean_value"] + stats["mean_entropy"])
            assert stats["mean_ratio"] == 1 and stats["clipped_fraction"] == 0
            assert rollout["policy_version"] == stats["policy_version_before"]
            for h in (p["snapshot_before"], p["snapshot_after"], stats["policy_version_before"], stats["policy_version_after"]):
                assert len(h) == 64 and all(c in "0123456789abcdef" for c in h)
            if u > 1:
                assert (p["snapshot_before"], stats["policy_version_before"]) == histories[seed]
            else:
                assert p["snapshot_before"] == old_feedback["training"][(seed - 1) * 200]["snapshot_before"]
            histories[seed] = (p["snapshot_after"], stats["policy_version_after"])
        for run in arm["runs"]:
            if run["stage"] != "random":
                expected_hash = histories[run["seed"]][0] if run["stage"] == "after" else points[(run["seed"] - 1) * 200]["feedback"]["snapshot_before"]
                assert run["snapshot_hash"] == expected_hash
        gates = evaluate(arm["runs"], report["eval_pairs"])
        assert gates == arm["gate_by_seed"] and all(gates.values()) is arm["gate"]
        summaries.append({"mode": arm["mode"], "gate_by_seed": gates, "gate": all(gates.values()),
            "training": [{"seed": s, "reached": sum(p["collected_end"]["done"] for p in points if p["feedback"]["seed"] == s),
                          "timed_out": sum(p["collected_end"]["timeout"] for p in points if p["feedback"]["seed"] == s)} for s in (1, 2, 3)],
            "runs": [{k: v for k, v in r.items() if k != "episodes"} for r in arm["runs"]]})
    assert [r for r in arms[0]["runs"] if r["stage"] != "after"] == [r for r in arms[1]["runs"] if r["stage"] != "after"]
    for i in (0, 200, 400):
        assert arms[0]["training"][i]["collected_end"] == arms[1]["training"][i]["collected_end"]
        collected = copy.deepcopy(arms[1]["training"][i]["feedback"]["rollout"])
        collected["steps"][-1] = arms[1]["training"][i]["collected_end"]
        assert collected == arms[0]["training"][i]["feedback"]["rollout"]
    return {"schema_version": "coimnet-ppo-horizon-summary/v1", "training_episodes": 1200, "evaluation_episodes": 3120,
            "control_matches_prior": True, "arms": summaries,
            "limits": ["Fixed seeds and synthetic network only; no general navigation or biological claim.",
                       "Python independently checks physics, targets, losses and gates; Go checks neural replay and PCG collection.",
                       "After policy divergence, training trajectories are not paired."]}


def verify_saved():
    reproduction = json.loads((HERE / "reproduction.json").read_text())
    assert reproduction["schema_version"] == "coimnet-ppo-horizon-reproduction/v1"
    assert [r["process_index"] for r in reproduction["runs"]] == [1, 2]
    assert [r["report"] for r in reproduction["runs"]] == ["report.json.gz", "run2-report.json.gz"]
    assert [r["log"] for r in reproduction["runs"]] == ["run1.log", "run2.log"]
    raws = []
    for run in reproduction["runs"]:
        packed, log = (HERE / run["report"]).read_bytes(), (HERE / run["log"]).read_bytes()
        raw = gzip.decompress(packed)
        assert digest(packed) == run["gzip_sha256"] and digest(raw) == run["raw_sha256"] and digest(log) == run["log_sha256"]
        assert "COIMNET_PPO_HORIZON_EVIDENCE=" in run["command"]
        assert b"--- PASS: TestPPOHorizonEvidence" in log and b"--- FAIL" not in log and b"SKIP" not in log
        raws.append(raw)
    assert len(raws) == 2 and raws[0] == raws[1]
    summary = verify(json.loads(raws[0]))
    assert summary == json.loads((HERE / "summary.json").read_text())
    return summary


if __name__ == "__main__":
    argparse.ArgumentParser(description=__doc__, epilog="Checks the saved reports beside this script. No arguments are required; an invalid report exits with an assertion failure.").parse_args()
    result = verify_saved()
    print(json.dumps({"passed": True, "training": result["training_episodes"], "evaluation": result["evaluation_episodes"], "control_matches_prior": True}))
