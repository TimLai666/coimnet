import argparse
import hashlib
import json
import re
import subprocess
from pathlib import Path

argparse.ArgumentParser(description="Verify ticket 43 protected inputs, frozen tests, and the exact final Go source manifest. Run from any directory; no arguments. Exit 0 on agreement or nonzero on mismatch.").parse_args()

root = Path(__file__).resolve().parents[3]
stage = Path(__file__).resolve().parent
baseline = json.loads((stage / "base-source.json").read_text())
protected = json.loads((stage / "protected-manifest.json").read_text())
original_tests = json.loads((stage / "tests-frozen.json").read_text())
frozen = json.loads((stage / "tests-frozen-v3.json").read_text())
corrected_test = "learning/rl/stateful_test.go"
original_test_bytes = (stage / "learning--rl--stateful_test.go.frozen.txt").read_bytes()
assert hashlib.sha256(original_test_bytes).hexdigest() == original_tests[corrected_test]
old_clone = b"state.History[i] = append([]float64(nil), state.History[i]...)"
new_clone = b"state.History[i] = append([]float64(nil), s.Continuous.History[i]...)"
assert original_test_bytes.count(old_clone) == 1
assert (root / corrected_test).read_bytes() == original_test_bytes.replace(old_clone, new_clone)
assert all(frozen[p] == h for p, h in original_tests.items() if p != corrected_test)

def digest(path):
    return hashlib.sha256((root / path).read_bytes()).hexdigest()

for label, manifest in [("protected inputs", protected), ("frozen tests", frozen)]:
    changed = [p for p, h in manifest.items() if not (root / p).is_file() or digest(p) != h]
    assert not changed, label + " changed: " + repr(changed)
changed_go = sorted(p for p, h in baseline["files"].items() if digest(p) != h)
expected = ["dynamics/continuous.go", "learning/network.go", "learning/rl/update.go",
            "learning/rl/update_validation_test.go", "learning/trainer.go"]
assert changed_go == expected, changed_go
signature = re.compile(r"^func (?:\([^\n]+\) )?([A-Z][A-Za-z0-9_]*)\([^\n]*$", re.M)
for path in changed_go:
    if path.endswith("_test.go"):
        continue
    before = subprocess.check_output(["git", "show", baseline["commit"] + ":" + path], cwd=root, text=True)
    after = (root / path).read_text()
    for match in signature.finditer(before):
        assert match.group(0) in after, "public signature changed: " + path + ":" + match.group(1)
tracked = subprocess.check_output(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"], cwd=root).decode().split("\0")
paths = sorted(p for p in set(tracked) if p.endswith(".go") and not p.startswith("evidence/"))
source = {p: digest(p) for p in paths}
result = {"base_commit": baseline["commit"], "protected_files_unchanged": len(protected),
          "frozen_tests_unchanged": len(frozen), "existing_go_files_changed": changed_go,
          "existing_public_signatures_preserved": True, "go_file_count": len(source),
          "source_sha256": hashlib.sha256(json.dumps(source, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
          "files": source}
output = stage / "source-manifest-v2.json"
if output.exists():
    assert json.loads(output.read_text()) == result, "frozen source manifest mismatch"
else:
    with output.open("x") as stream:
        json.dump(result, stream, indent=2)
        stream.write("\n")
print(json.dumps({k: v for k, v in result.items() if k != "files"}))
