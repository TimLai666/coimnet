#!/usr/bin/env python3
"""Check the bounded horizon evidence and unchanged historical inputs."""
import argparse
import hashlib
import json
import re
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
BASE = "69a809fcdaa576a347a273474624b8649a73c4a2"

if __name__ == "__main__":
    argparse.ArgumentParser(description=__doc__, epilog="Run from any directory after verification.json is saved. No arguments are required; mismatched evidence exits with an assertion failure.").parse_args()


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def manifest(name, expected):
    rows = [line.split("  ", 1) for line in (HERE / name).read_text().splitlines()]
    assert len(rows) == expected and len({p for _, p in rows}) == expected
    for digest, path in rows:
        assert sha(ROOT / path) == digest, path
    return {p for _, p in rows}


v = json.loads((HERE / "verification.json").read_text())
assert v["input_fingerprints"]["base_commit"] == BASE
assert set(v["artifact_sha256"]) == {p.name for p in HERE.iterdir() if p.is_file() and p.name not in ("verification.json", "delivery.json")}, "unbound evidence artifact"
for name, digest in v["artifact_sha256"].items():
    assert sha(HERE / name) == digest, name
assert {"verify_report.py", "verify_report_test.py", "check_delivery.py"} <= v["artifact_sha256"].keys()
sources = manifest("source.sha256", 646)
manifest("protected.sha256", 883)
paths = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=ROOT).decode().split("\0")
assert sources == {p for p in paths if p.endswith((".go", ".sh")) or p in ("go.mod", "go.sum")}
red = dict((path, digest) for digest, path in (line.split("  ", 1) for line in (HERE / "red-source.sha256").read_text().splitlines()))
for p in ("experiment/ppo_horizon_validation_test.go", "experiment/ppo_horizon_evidence_test.go"):
    assert sha(ROOT / p) == red[p], ("test changed after red", p)

expected = json.loads(subprocess.check_output(["git", "show", BASE + ":docs/requirements-status.json"], cwd=ROOT))
for r in expected["requirements"]:
    if r["id"] == "LRN-09":
        r["evidence"].append("../evidence/LRN-09/horizon-comparison-20261005/verification.json")
actual = json.loads((ROOT / "docs/requirements-status.json").read_text())
assert actual == expected, "unexpected requirement change"
rows = {r["id"]: r for r in actual["requirements"]}
assert len(rows) == 91 and sum(r["status"] == "passed" for r in rows.values()) == 89
assert rows["TSK-11"]["status"] == rows["OPS-05"]["status"] == "specified"

files = ["ENG.md", "delivery-status.md", "docs/INDEX.md", "docs/tickets/40-six-step-deadline-training-comparison.md", "experiment/gridnav/README.md"]
files += [str(p.relative_to(ROOT)) for p in HERE.glob("*.md")]
links = 0
for name in files:
    p = ROOT / name
    for link in re.findall(r"\]\(([^)]+)\)", p.read_text()):
        if link.startswith(("https:", "http:", "mailto:", "codex:", "#")):
            continue
        assert (p.parent / link.split("#")[0]).exists(), (name, link)
        links += 1
index = (ROOT / "docs/INDEX.md").read_text()
for schema in ("comparison", "reproduction", "summary"):
    assert "coimnet-ppo-horizon-" + schema + "/v1" in index
for name in ("test.log", "race.log"):
    log = (HERE / name).read_text()
    assert len(re.findall(r"^ok\s", log, re.M)) == 57 and "FAIL" not in log, name
assert "Ran 4 tests" in (HERE / "final-verifier-tests.log").read_text()
assert (HERE / "final-verifier-tests.log").read_text().rstrip().endswith("OK")
assert subprocess.check_output(["gofmt", "-l", *sorted(p for p in sources if p.startswith("experiment/ppo_horizon_"))], cwd=ROOT) == b""
assert subprocess.check_output(["git", "diff", "--check"], cwd=ROOT) == b""
print(f"PASS: source 646; protected 883; local links {links}; new schemas 3; requirements 89/91; only LRN-09 evidence appended; full normal/race 57 each; verifier scripts pinned")
