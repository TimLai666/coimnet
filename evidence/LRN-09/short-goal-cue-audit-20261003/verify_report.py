"""Independent integer corridor/readback checks for the short-deadline paired audit."""
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
expected_config = {
    "corridor": {"length": 7, "time_limit": 6, "step_penalty": .01, "goal_reward": 1},
    "seeds": [1, 2, 3], "updates": 200, "hidden": 8, "eval_episodes": 40, "learning_rate": .01,
    "ppo": {"gamma": .99, "lambda": .95, "clip_epsilon": .2, "value_coef": .5,
            "entropy_coef": .01, "burn_in": 0, "time_limit": 6, "epochs": 1, "mini_batch": 1}
}
assert report["config"] == expected_config
assert report["legacy_report"]["config"] == expected_config
assert report["legacy_report"]["config_hash"] == report["config_hash"]
assert len(report["legacy_report"]["results"]) == 3
assert {r["seed"] for r in report["legacy_report"]["results"]} == {1, 2, 3}
assert summary["report_processes"] == 2 and summary["report_repeat_byte_identical"] is True
assert report["schema_version"] == "coimnet-short-goal-cue-audit/v1"
assert report["config"]["ppo"]["time_limit"] == 6
assert report["eval_stream"] == 0x1005
assert hashlib.sha256(json.dumps(report["config"], separators=(",", ":")).encode()).hexdigest() == report["config_hash"]
# Fixed protocol pairs; Go TestPPOShortGoalPairs checks each against gridnav.Reset.
expected_pairs = [[1001, 1000], [1002, 1004], [1003, 1005], [1006, 1007], [1008, 1009], [1010, 1013], [1011, 1015], [1012, 1018], [1014, 1020], [1016, 1022], [1017, 1023], [1019, 1024], [1021, 1026], [1025, 1027], [1028, 1032], [1029, 1033], [1030, 1034], [1031, 1036], [1035, 1037], [1039, 1038]]
pairs = report["eval_pairs"]
assert pairs == expected_pairs
assert pairs == summary["eval_pairs"] and len(pairs) == 20
assert [p[0] for p in pairs] == sorted(p[0] for p in pairs)
assert [p[1] for p in pairs] == sorted(p[1] for p in pairs)
assert {s for pair in pairs for s in pair} == set(range(1000, 1040))
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
        assert 1 <= len(trace) <= 6
        cue = original_cue * {"original": 1, "flipped": -1, "erased": 0}[run["cue"]]
        assert trace[0]["observation"] == [cue, 0, 0, 0]
        position, goal, total = 3, (0 if original_cue == -1 else 6), 0
        for i, step in enumerate(trace):
            if i:
                assert step["observation"] == [0, int(position == 0), int(position == 6), i / 6]
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
            assert info["timed_out"] == (position != goal and i + 1 == 6)
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
                        greedy["reached"] == 40 and
                        rate(trained) - rate(metrics("after", cue="erased")) >= .2 - 1e-12 and
                        metrics("after", "greedy", "erased")["reached"] <= 20 and
                        metrics("after", "greedy", "flipped")["reached"] == 0)
assert gates == report["short_goal_cue_gate_by_seed"] == summary["short_goal_cue_gate_by_seed"]
assert all(gates.values()) == report["short_goal_cue_gate"] == summary["short_goal_cue_gate"]
# Paired erased inputs/actions/outputs must agree until either episode ends.
# Rewards and hidden goal differ, but neither is fed back to the policy.
for run in report["runs"]:
    by_seed = {ep["env_seed"]: ep for ep in run["episodes"]}
    for left_seed, right_seed in pairs:
        left, right = by_seed[left_seed], by_seed[right_seed]
        assert (left["original_cue"], right["original_cue"]) == (-1, 1)
        if run["cue"] == "erased" or run["mode"] == "random":
            assert int(left["reached"]) + int(right["reached"]) <= 1
            for index, (lstep, rstep) in enumerate(zip(left["trace"], right["trace"])):
                if run["mode"] == "random" and index == 0:
                    # Random ignores the observation; its recorded original cues differ.
                    assert lstep["observation"][1:] == rstep["observation"][1:]
                else:
                    assert lstep["observation"] == rstep["observation"]
                assert lstep["action"] == rstep["action"]
                assert lstep.get("output") == rstep.get("output")
    if run["cue"] == "erased" or run["mode"] == "random":
        assert run["metrics"]["reached"] <= 20
    if run["stage"] == "after":
        legacy = next(r for r in report["legacy_report"]["results"] if r["seed"] == run["seed"])
        assert not legacy["failed"] and legacy["updates"] == 200
        assert run["snapshot_hash"] == legacy["final_snapshot_hash"]
# Independent integer proof: no action sequence can visit both endpoints in 6.
import itertools
for actions in itertools.product(range(3), repeat=6):
    position, visited = 3, set()
    for action in actions:
        position = max(0, min(6, position + {0: -1, 1: 1, 2: 0}[action]))
        if position in (0, 6):
            visited.add(position)
    assert len(visited) <= 1
reproduction = json.loads((folder / "reproduction.json").read_text())
assert reproduction["schema_version"] == "coimnet-short-goal-reproduction/v1"
assert len(reproduction["runs"]) == 2
assert [r["process_index"] for r in reproduction["runs"]] == [1, 2]
assert [r["report"] for r in reproduction["runs"]] == ["report.json.gz", "run2-report.json.gz"]
assert [r["log"] for r in reproduction["runs"]] == ["run1.log", "run2.log"]
for execution in reproduction["runs"]:
    archived = (folder / execution["report"]).read_bytes()
    archived_raw = gzip.decompress(archived)
    assert archived_raw == raw
    assert hashlib.sha256(archived_raw).hexdigest() == execution["report_sha256"] == summary["report_sha256"]
    assert hashlib.sha256(archived).hexdigest() == execution["report_gzip_sha256"]
    log = (folder / execution["log"]).read_bytes()
    assert hashlib.sha256(log).hexdigest() == execution["log_sha256"]
    assert b"--- PASS: TestPPOShortGoalCueEvidence" in log and b"FAIL" not in log and b"SKIP" not in log
    assert "COIMNET_SHORT_GOAL_CUE_EVIDENCE=" in execution["command"]
print("PASS: 39 unique runs; 1560 complete traces; positions, rewards, terminal flags, cue controls, metrics, gates, paired erased prefixes, 729 action sequences and fingerprints agree")
