#!/usr/bin/env python3
"""Replay the bounded audit without changing production files.

All generated files and Go caches stay in the requested project-local work
directory. A fresh cache may download the versions pinned by go.mod/go.sum.
Use --module-cache with an existing project-local cache for offline replay.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("neutral", "history", "initial", "candidate-unit", "candidate-race"))
    parser.add_argument("--work", required=True, help="New directory below runs/.tmp/")
    parser.add_argument("--module-cache", help="Existing project-local Go module cache; offline replay")
    args = parser.parse_args()
    evidence = Path(__file__).resolve().parent
    root = evidence.parents[2]
    baseline = json.loads((evidence / "baseline.json").read_text())
    for name, expected in baseline["sources"].items():
        if hashlib.sha256((root / name).read_bytes()).hexdigest() != expected:
            raise SystemExit("Source differs from recorded audit: " + name)
    work = (root / args.work).resolve()
    if not work.is_relative_to(root / "runs/.tmp"):
        raise SystemExit("--work must be below this project's runs/.tmp/")
    module_cache = (root / args.module_cache).resolve() if args.module_cache else work / "go-mod-cache"
    if not module_cache.is_relative_to(root) or (args.module_cache and not module_cache.is_dir()):
        raise SystemExit("--module-cache must be an existing project-local directory")
    work.mkdir()  # Refuse to overwrite any earlier run.
    env = dict(os.environ)
    for key, name in (("GOCACHE", "go-cache"), ("GOTMPDIR", "go-tmp"), ("TMPDIR", "go-tmp")):
        path = work / name
        path.mkdir(exist_ok=True)
        env[key] = str(path)
    module_cache.mkdir(exist_ok=True)
    env["GOMODCACHE"] = str(module_cache)
    env["GOTOOLCHAIN"] = "local"
    if args.module_cache:
        env.update(GOPROXY="off", GOSUMDB="off")
    overlay = {}

    def materialize(source, target):
        path = work / target
        path.write_bytes((evidence / source).read_bytes())
        return str(path)

    if args.mode == "neutral":
        command = ["go", "run", materialize("neutral-probe.go.txt", "neutral_probe.go")]
    else:
        if args.mode == "history":
            overlay[str(root / "learning/composition_history_probe_test.go")] = materialize("history-probe-final-test.go.txt", "history_probe_test.go")
            package, pattern = "./learning", "^TestComposition"
        elif args.mode == "initial":
            overlay[str(root / "learning/composition_initial_bridge_probe_test.go")] = materialize("initial-bridge-hand-final-test.go.txt", "initial_bridge_probe_test.go")
            package, pattern = "./learning", "^TestCompositionInitialRecomputeHand$"
        else:
            overlay[str(root / "modulation/neutral_proposal_test.go")] = materialize("neutral-validation-proposal-test.go.txt", "neutral_proposal_test.go")
            overlay[str(root / "modulation/effects.go")] = materialize("neutral-validation-candidate.go.txt", "effects_candidate.go")
            package, pattern = "./modulation", "."
        overlay_path = work / "overlay.json"
        overlay_path.write_text(json.dumps({"Replace": overlay}))
        command = ["go", "test"]
        if args.mode == "candidate-race":
            command.append("-race")
        command += ["-overlay", str(overlay_path), "-json", "-count=1", "-run", pattern, package]
    result = subprocess.run(command, cwd=root, env=env, capture_output=True)
    (work / "output.log").write_bytes(result.stdout + result.stderr)
    if result.returncode:
        sys.stdout.buffer.write(result.stdout + result.stderr)
        return result.returncode
    if args.mode == "neutral":
        rows = [json.loads(line) for line in result.stdout.splitlines()]
        nonneutral = sum(not row["matches_declared_neutral"] for row in rows)
        print(json.dumps({"cases": len(rows), "non_neutral": nonneutral, "expected": 9, "scope": "bug reproduced on unchanged source"}))
        return 0 if len(rows) == 15 and nonneutral == 9 else 1
    print(json.dumps({"mode": args.mode, "exit_code": 0, "production_sources_changed": False, "candidate_adopted": False}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
