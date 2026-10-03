"""Independent integer corridor/readback checks for the synthetic audit."""
import gzip
import hashlib
import json
import math
from pathlib import Path

folder = Path(__file__).parent
compressed = (folder / "report.json.gz").read_bytes()
raw = gzip.decompress(compressed)
report = json.loads(raw)
summary = json.loads((folder / "summary.json").read_text())
assert hashlib.sha256(raw).hexdigest() == summary["report_sha256"]
assert hashlib.sha256(compressed).hexdigest() == summary["report_gzip_sha256"]
assert report["config"]["corridor"] == {
    "length": 7, "time_limit": 20, "step_penalty": .01, "goal_reward": 1
}
assert report["config"]["seeds"] == [1, 2, 3]
assert report["config"]["updates"] == 200
assert {k: report[k] for k in ("schema_version", "config", "config_hash", "eval_stream", "gate_criteria")} == {k: summary[k] for k in ("schema_version", "config", "config_hash", "eval_stream", "gate_criteria")}
assert [{k: v for k, v in run.items() if k != "episodes"} for run in report["runs"]] == summary["runs"]
keys = set()
episode_count = 0
for run in report["runs"]:
    key = (run["seed"], run["stage"], run["mode"], run["cue"])
    assert key not in keys
    keys.add(key)
    episodes = run["episodes"]
    assert {ep["env_seed"] for ep in episodes} == set(range(1000, 1040))
    assert len(episodes) == 40
    episode_count += len(episodes)
    reached = left = left_reached = right = right_reached = 0
    returns, steps = [], []
    for ep in episodes:
        original_cue = ep["original_cue"]
        assert original_cue in (-1, 1)
        trace = ep["trace"]
        assert 1 <= len(trace) <= 20
        cue = original_cue * {"original": 1, "flipped": -1, "erased": 0}[run["cue"]]
        assert trace[0]["observation"] == [cue, 0, 0, 0]
        position, goal, total = 3, (0 if original_cue == -1 else 6), 0
        for i, step in enumerate(trace):
            if i:
                assert step["observation"] == [0, int(position == 0), int(position == 6), i / 20]
            action = step["action"]
            assert action in (0, 1, 2)
            if run["mode"] == "random":
                assert "output" not in step
            else:
                output = step["output"]
                assert len(output) == 4 and all(math.isfinite(v) for v in output)
                if run["mode"] == "greedy":
                    assert action == max(range(3), key=lambda a: output[a])
            position = max(0, min(6, position + {0: -1, 1: 1, 2: 0}[action]))
            assert step["reward"] == -.01 + (1 if position == goal else 0)
            total += step["reward"]
            info = step["evaluator_info"]
            assert (info["position"], info["goal"], info["steps"]) == (position, goal, i + 1)
            assert info["reached"] == (position == goal)
            assert info["timed_out"] == (position != goal and i + 1 == 20)
            assert i == len(trace) - 1 or not info["reached"]
        assert ep["return"] == total
        assert ep["reached"] == (position == goal)
        assert trace[-1]["evaluator_info"]["reached"] or trace[-1]["evaluator_info"]["timed_out"]
        reached += ep["reached"]
        left += original_cue == -1
        right += original_cue == 1
        left_reached += ep["reached"] and original_cue == -1
        right_reached += ep["reached"] and original_cue == 1
        returns.append(total)
        steps.append(len(trace))
    metrics = run["metrics"]
    assert [metrics[k] for k in ("episodes", "reached", "left_episodes", "left_reached", "right_episodes", "right_reached")] == [40, reached, left, left_reached, right, right_reached]
    assert math.isclose(metrics["mean_return"], sum(returns) / 40, rel_tol=1e-14, abs_tol=1e-14)
    assert metrics["mean_steps"] == sum(steps) / 40
assert len(keys) == 39 and episode_count == 1560
gates = {}
for seed in (1, 2, 3):
    def metrics(stage, mode="sampled", cue="original"):
        return next(r["metrics"] for r in report["runs"] if
                    (r["seed"], r["stage"], r["mode"], r["cue"]) == (seed, stage, mode, cue))
    def rate(m):
        return m["reached"] / m["episodes"]
    trained, before = metrics("after"), metrics("before")
    random, flipped = metrics("random", "random"), metrics("after", cue="flipped")
    greedy = metrics("after", "greedy")
    gates[str(seed)] = (trained["left_reached"] / 20 >= .8 and
                        trained["right_reached"] / 20 >= .8 and
                        rate(trained) - rate(before) >= .15 - 1e-12 and
                        rate(trained) - rate(random) >= .15 - 1e-12 and
                        rate(trained) - rate(flipped) >= .2 - 1e-12 and
                        greedy["reached"] == 40)
assert gates == report["goal_cue_gate_by_seed"] == summary["goal_cue_gate_by_seed"]
assert all(gates.values()) == report["goal_cue_gate"] == summary["goal_cue_gate"]
print("PASS: 39 unique runs; 1560 complete traces; positions, rewards, terminal flags, cue controls, metrics, gates and fingerprints agree")
