"""Reject tampered evidence after updating checksums and summary metadata."""
import copy
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile

folder = Path(__file__).parent
original_raw = gzip.decompress((folder / "report.json.gz").read_bytes())
report = json.loads(original_raw)
summary = json.loads((folder / "summary.json").read_text())
source = (folder / "verify_report.py").read_bytes()
extras = {name: (folder / name).read_bytes() for name in ("reproduction.json", "run2-report.json.gz", "run1.log", "run2.log")}

failures = []
for case in ("paired_output", "bad_counter", "bad_terminal", "bad_snapshot", "changed_config", "legacy_config", "repeat_count", "repeat_flag", "repeat_archive", "repeat_log", "repeat_digest"):
    altered = copy.deepcopy(report)
    metadata = copy.deepcopy(summary)
    case_extras = dict(extras)
    target = next(r for r in altered["runs"] if r["stage"] == "after" and r["mode"] == "sampled" and r["cue"] == "erased")
    if case == "paired_output":
        target["episodes"][0]["trace"][0]["output"][3] += 1
    elif case == "bad_counter":
        target["metrics"]["reached"] += 1
    elif case == "bad_terminal":
        info = target["episodes"][0]["trace"][-1]["evaluator_info"]
        info["timed_out"] = not info["timed_out"]
    elif case == "bad_snapshot":
        target["snapshot_hash"] = "0" * 64
    elif case == "changed_config":
        altered["config"]["learning_rate"] = .02
        altered["config_hash"] = hashlib.sha256(json.dumps(altered["config"], separators=(",", ":")).encode()).hexdigest()
        metadata["config"] = altered["config"]
        metadata["config_hash"] = altered["config_hash"]
    elif case == "legacy_config":
        altered["legacy_report"]["config"]["learning_rate"] = .02
        altered["legacy_report"]["config_hash"] = hashlib.sha256(json.dumps(altered["legacy_report"]["config"], separators=(",", ":")).encode()).hexdigest()
    elif case == "repeat_count":
        metadata["report_processes"] = 1
    elif case == "repeat_flag":
        metadata["report_repeat_byte_identical"] = False
    elif case in ("repeat_archive", "repeat_log", "repeat_digest"):
        manifest = json.loads(case_extras["reproduction.json"])
        execution = manifest["runs"][1]
        if case == "repeat_archive":
            changed_raw = original_raw + b"\n"
            changed_gzip = gzip.compress(changed_raw, mtime=0)
            case_extras["run2-report.json.gz"] = changed_gzip
            execution["report_sha256"] = hashlib.sha256(changed_raw).hexdigest()
            execution["report_gzip_sha256"] = hashlib.sha256(changed_gzip).hexdigest()
        elif case == "repeat_log":
            log = case_extras["run2.log"].replace(b"--- PASS:", b"--- SKIP:")
            case_extras["run2.log"] = log
            execution["log_sha256"] = hashlib.sha256(log).hexdigest()
        else:
            execution["report_sha256"] = "0" * 64
        case_extras["reproduction.json"] = json.dumps(manifest).encode()
    raw = original_raw if case.startswith("repeat_") else json.dumps(altered, separators=(",", ":")).encode()
    compressed = gzip.compress(raw, mtime=0)
    metadata["runs"] = [{k: v for k, v in run.items() if k != "episodes"} for run in altered["runs"]]
    metadata["report_sha256"] = hashlib.sha256(raw).hexdigest()
    metadata["report_gzip_sha256"] = hashlib.sha256(compressed).hexdigest()
    with tempfile.TemporaryDirectory(prefix="coimnet-short-goal-readback-") as temp:
        out = Path(temp)
        (out / "verify_report.py").write_bytes(source)
        for name, data in case_extras.items():
            (out / name).write_bytes(data)
        (out / "report.json.gz").write_bytes(compressed)
        (out / "summary.json").write_text(json.dumps(metadata))
        result = subprocess.run([sys.executable, str(out / "verify_report.py")], capture_output=True, text=True)
        rejected = result.returncode != 0 and "AssertionError" in result.stderr
        if not rejected:
            failures.append(case)
    print("PASS: reject" if rejected else "FAIL: accepted", case, "with consistent checksums and metadata")
assert not failures, failures
