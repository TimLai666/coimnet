#!/usr/bin/env python3
"""Independent integer corridor replay and backward GAE calculation; stdlib only."""
import gzip
import hashlib
import json
import math
from pathlib import Path

HERE = Path(__file__).resolve().parent
CONFIG = {
    "corridor": {"length": 7, "time_limit": 6, "step_penalty": 0.01, "goal_reward": 1},
    "seeds": [1, 2, 3], "updates": 200, "hidden": 8, "eval_episodes": 40,
    "learning_rate": 0.01,
    "ppo": {"gamma": 0.99, "lambda": 0.95, "clip_epsilon": 0.2, "value_coef": 0.5,
            "entropy_coef": 0.01, "burn_in": 0, "time_limit": 6, "epochs": 1, "mini_batch": 1},
}


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def close(a, b):
    assert math.isfinite(a) and math.isfinite(b)
    assert math.isclose(a, b, rel_tol=1e-12, abs_tol=1e-12), (a, b)


def mix(x):
    mask = (1 << 64) - 1
    x = (x + 0x9e3779b97f4a7c15) & mask
    x = ((x ^ (x >> 30)) * 0xbf58476d1ce4e5b9) & mask
    x = ((x ^ (x >> 27)) * 0x94d049bb133111eb) & mask
    return x ^ (x >> 31)


def replay(steps, cue, total, probe=False, intervention="original"):
    assert cue in (-1, 1) and 1 <= len(steps) <= 6
    position, goal, accumulated = 3, (0 if cue == -1 else 6), 0.0
    for index, s in enumerate(steps):
        expected_cue = cue if index == 0 else 0
        if intervention == "erased":
            expected_cue = 0
        elif intervention == "flipped":
            expected_cue = -expected_cue
        expected_obs = [expected_cue, int(position == 0), int(position == 6), index / 6]
        assert s["observation" if probe else "obs"] == expected_obs
        action = s["action"]
        assert type(action) is int and action in (0, 1, 2)
        if probe:
            logits = s["output"]
            assert len(logits) == 4 and all(math.isfinite(v) for v in logits)
            assert action == max(range(3), key=lambda a: logits[a])
        else:
            assert math.isfinite(s["log_prob"]) and s["log_prob"] <= 0
            assert math.isfinite(s["value"]) and math.isfinite(s.get("bootstrap_value", 0))
        position = max(0, min(6, position + (-1 if action == 0 else 1 if action == 1 else 0)))
        done = position == goal
        timeout = not done and index + 1 == 6
        reward = -0.01 + int(done)
        close(s["reward"], reward)
        accumulated += reward
        if probe:
            assert s["evaluator_info"] == {"position": position, "goal": goal,
                    "steps": index + 1, "reached": done, "timed_out": timeout}
        else:
            assert s["done"] is done and s["timeout"] is timeout
            if not timeout:
                assert s.get("bootstrap_value", 0) == 0
        assert (done or timeout) == (index == len(steps) - 1)
    close(accumulated, total)
    return done


def advantages(steps):
    result = [0.0] * len(steps)
    next_advantage = 0.0
    for i in reversed(range(len(steps))):
        s = steps[i]
        next_value = s.get("bootstrap_value", 0) if s["timeout"] else (
            steps[i + 1]["value"] if not s["done"] else 0)
        delta = s["reward"] + 0.99 * next_value - s["value"]
        result[i] = delta if s["done"] or s["timeout"] else delta + 0.99 * 0.95 * next_advantage
        next_advantage = result[i]
    return result, [a + s["value"] for a, s in zip(result, steps)]


def side_stats(points):
    out = {}
    for cue, side in ((-1, "left"), (1, "right")):
        values = [p for p in points if p["original_cue"] == cue]
        wins = [p for p in values if p["rollout"]["steps"][-1]["done"]]
        first = [p["advantages"][0] for p in values]
        all_adv = [a for p in values for a in p["advantages"]]
        timeouts = [p for p in values if p["rollout"]["steps"][-1]["timeout"]]
        predicted = [s["value"] for p in values for s in p["rollout"]["steps"]]
        boot = [p["rollout"]["steps"][-1].get("bootstrap_value", 0) for p in timeouts]
        out[side] = {"episodes": len(values), "successes": len(wins),
            "success_updates": [p["update"] for p in wins],
            "last_success_update": max((p["update"] for p in wins), default=None),
            "first_action_counts": {str(a): sum(p["rollout"]["steps"][0]["action"] == a for p in values) for a in range(3)},
            "positive_first_advantages": sum(a > 0 for a in first),
            "positive_timeout_first_advantages": sum(p["advantages"][0] > 0 for p in timeouts),
            "positive_step_advantages": sum(a > 0 for a in all_adv),
            "step_count": len(all_adv), "absolute_advantage_sum": sum(abs(a) for a in all_adv),
            "value_range": [min(predicted), max(predicted)] if predicted else [],
            "timeout_bootstrap_range": [min(boot), max(boot)] if boot else [],
            "success_absolute_advantage_sum": sum(abs(a) for p in wins for a in p["advantages"]),
            "timeout_absolute_advantage_sum": sum(abs(a) for p in timeouts for a in p["advantages"])}
    return out


def verify(report, previous):
    assert report["schema_version"] == "coimnet-ppo-training-feedback/v1"
    assert report["config"] == CONFIG and report["config"] == previous["config"]
    assert report["config_hash"] == previous["config_hash"]
    assert report["legacy_report"] == previous["legacy_report"]
    points, probes = report["training"], report["probes"]
    assert len(points) == 600 and len(probes) == 99
    assert [(p["seed"], p["update"]) for p in points] == [(s, u) for s in (1, 2, 3) for u in range(1, 201)]
    assert [(p["seed"], p["update"], p["cue"]) for p in probes] == [
        (s, u, c) for s in (1, 2, 3) for u in range(0, 201, 20) for c in ("original", "erased", "flipped")]
    summaries = []
    for seed, legacy in zip((1, 2, 3), report["legacy_report"]["results"]):
        ts = [p for p in points if p["seed"] == seed]
        assert legacy["seed"] == seed and not legacy["failed"] and legacy["updates"] == 200
        assert len(legacy["curve"]) == 200
        versions, hashes = [], []
        for p, curve in zip(ts, legacy["curve"]):
            u, rollout, r = p["update"], p["rollout"], p["update_report"]
            assert p["env_seed"] == mix(seed ^ ((u * 0x9e3779b97f4a7c15) & ((1 << 64) - 1)))
            assert rollout["initial_neural"]["core"] == "continuous"
            state = rollout["initial_neural"]["continuous"]
            assert state["steps"] == 0 and state["voltage"] == [0] * 12 and state["history"] == [[0] * 12]
            assert "initial_plastic" not in rollout and "initial_chemical" not in rollout
            steps = rollout["steps"]
            replay(steps, p["original_cue"], p["return"])
            aa, targets = advantages(steps)
            assert len(p["advantages"]) == len(steps) == len(p["targets"])
            for actual, expected in zip(p["advantages"] + p["targets"], aa + targets):
                close(actual, expected)
            assert curve["update"] == u
            close(curve["return"], p["return"])
            close(curve["loss"], r["mean_loss"])
            assert r["rollouts"] == r["epochs"] == r["version_checks"] == 1
            assert r["transitions"] == len(steps)
            close(r["mean_policy"], -sum(aa) / len(aa))
            close(r["mean_value"], 0.5 * sum(a * a for a in aa) / len(aa))
            close(r["mean_loss"], r["mean_policy"] + r["mean_value"] + r["mean_entropy"])
            assert r["mean_ratio"] == 1 and r["clipped_fraction"] == 0
            assert rollout["policy_version"] == r["policy_version_before"]
            for h in (p["snapshot_before"], p["snapshot_after"], r["policy_version_before"], r["policy_version_after"]):
                assert len(h) == 64 and all(c in "0123456789abcdef" for c in h)
            if hashes:
                assert p["snapshot_before"] == hashes[-1] and r["policy_version_before"] == versions[-1]
            hashes.append(p["snapshot_after"])
            versions.append(r["policy_version_after"])
        assert hashes[-1] == legacy["final_snapshot_hash"]
        assert sum(p["rollout"]["steps"][-1]["done"] for p in ts) == legacy["terminal_episodes"]
        assert sum(p["rollout"]["steps"][-1]["timeout"] for p in ts) == legacy["timeout_episodes"]
        ps = [p for p in probes if p["seed"] == seed]
        checkpoints = []
        for u in range(0, 201, 20):
            variants = [p for p in ps if p["update"] == u]
            expected_hash = ts[0]["snapshot_before"] if u == 0 else ts[u - 1]["snapshot_after"]
            entry = {"update": u, "variants": {}}
            for p in variants:
                assert p["snapshot_hash"] == expected_hash
                assert [e["env_seed"] for e in p["episodes"]] == [1001, 1000]
                result = []
                for e, cue in zip(p["episodes"], (-1, 1)):
                    assert e["original_cue"] == cue
                    assert replay(e["trace"], cue, e["return"], True, p["cue"]) is e["reached"]
                    result.append({"side": "left" if cue == -1 else "right", "reached": e["reached"],
                        "first_action": e["trace"][0]["action"], "actions": [s["action"] for s in e["trace"]]})
                entry["variants"][p["cue"]] = result
            for e in range(2):
                original = variants[0]["episodes"][e]["trace"][0]["output"][:3]
                erased = variants[1]["episodes"][e]["trace"][0]["output"][:3]
                assert any(a != b for a, b in zip(original, erased))
                assert variants[0]["episodes"][e]["trace"][0]["output"] == variants[2]["episodes"][1-e]["trace"][0]["output"]
            entry["cue_effects"] = []
            for e, side in enumerate(("left", "right")):
                a = variants[0]["episodes"][e]["trace"]
                b = variants[1]["episodes"][e]["trace"]
                shared = []
                for i, (x, y) in enumerate(zip(a, b)):
                    if i > 0 and x["observation"] != y["observation"]:
                        break
                    shared.append({"step": i, "logit_max_abs_delta": max(abs(v-w) for v, w in zip(x["output"][:3], y["output"][:3])),
                                   "same_action": x["action"] == y["action"]})
                    if x["action"] != y["action"]:
                        break
                entry["cue_effects"].append({"side": side, "shared_path": shared})
            checkpoints.append(entry)
        summaries.append({"seed": seed, "sides": side_stats(ts),
            "windows": [{"updates": [start, start + 19], "sides": side_stats(ts[start-1:start+19])} for start in range(1, 201, 20)],
            "checkpoints": checkpoints})
    return {"schema_version": "coimnet-ppo-training-feedback-summary/v1", "training_episodes": 600,
            "probe_episodes": 198, "seeds": summaries,
            "limits": ["Observational association does not isolate policy/value gradient interference or exploration causality.",
                       "Frozen probe input changes logits; this does not prove cue-dependent navigation.",
                       "Python independently replays physics, GAE, losses and seed mixing, not neural forward or PCG reset.",
                       "Same-platform synthetic corridor only; no biological or general navigation claim."]}


def verify_saved():
    reproduction = json.loads((HERE / "reproduction.json").read_text())
    raw_reports = []
    for run in reproduction["runs"]:
        packed = (HERE / run["report"]).read_bytes()
        raw = gzip.decompress(packed)
        assert digest(packed) == run["gzip_sha256"] and digest(raw) == run["raw_sha256"]
        log = (HERE / run["log"]).read_bytes()
        assert digest(log) == run["log_sha256"]
        assert b"--- PASS: TestPPOTrainingFeedbackEvidence" in log and b"--- FAIL" not in log
        raw_reports.append(raw)
    assert len(raw_reports) == 2 and raw_reports[0] == raw_reports[1]
    old = gzip.decompress((HERE.parent / "short-goal-cue-audit-20261003/report.json.gz").read_bytes())
    assert digest(old) == reproduction["previous_report_sha256"]
    summary = verify(json.loads(raw_reports[0]), json.loads(old))
    assert summary == json.loads((HERE / "summary.json").read_text())
    return summary


if __name__ == "__main__":
    summary = verify_saved()
    print(json.dumps({"passed": True, "training": summary["training_episodes"], "probes": summary["probe_episodes"]}))
