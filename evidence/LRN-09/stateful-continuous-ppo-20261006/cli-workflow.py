"""Exercise the CLI slice of scripts/verify.sh in a disposable local directory."""
import argparse
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path

parser = argparse.ArgumentParser(description="Build the CoImNet CLI, run doctor/data/fixture checks, and compare 80+40 versus 120 training steps in a temporary directory. Exit 0 on success, nonzero on failure. Temporary artifacts are cleaned up.")
parser.add_argument("--result", default="cli-validation.json", help="New result JSON filename within this evidence directory; must not exist (default: cli-validation.json). Example: --result reproduction-cli.json")
cli_options = parser.parse_args()
if Path(cli_options.result).name != cli_options.result:
    parser.error("result must be a filename without directories")
root = Path(__file__).resolve().parents[3]
stage = Path(__file__).resolve().parent
checks = []

def run(arguments, output=None):
    proc = subprocess.run(arguments, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    print(json.dumps({"command": arguments, "exit_code": proc.returncode}), flush=True)
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.decode(errors="replace"))
    if output is not None:
        output.write_bytes(proc.stdout)
        json.loads(proc.stdout)
    checks.append({"command": arguments, "exit_code": proc.returncode,
                   "stdout_sha256": hashlib.sha256(proc.stdout).hexdigest()})

with tempfile.TemporaryDirectory(prefix="coimnet-ticket43-cli-") as temporary:
    folder = Path(temporary)
    binary = str(folder / "coimnet")
    run(["go", "build", "-o", binary, "./cmd/coimnet"])
    for label, args in [("doctor", ["doctor"]), ("sources", ["data", "sources"]),
                        ("delayed", ["examples", "run", "delayed"]),
                        ("lif", ["examples", "run", "lif-threshold"])]:
        run([binary] + args, folder / (label + ".json"))
    first = folder / "first.json"
    resumed = folder / "resumed.json"
    continuous = folder / "continuous.json"
    run([binary, "train", "delayed", "--steps", "80", "--checkpoint", str(first)], folder / "train-first.json")
    run([binary, "resume", "--checkpoint", str(first), "--steps", "40", "--out", str(resumed)], folder / "train-resumed.json")
    run([binary, "train", "delayed", "--steps", "120", "--checkpoint", str(continuous)], folder / "train-continuous.json")
    assert resumed.read_bytes() == continuous.read_bytes(), "CLI resume differs from continuous training"
    checkpoint_digest = hashlib.sha256(resumed.read_bytes()).hexdigest()
    checkpoint_bytes = len(resumed.read_bytes())
assert not folder.exists(), "temporary CLI artifacts remain"
result = {"checks": checks, "resume_matches_continuous_bytes": True,
          "checkpoint_sha256": checkpoint_digest, "checkpoint_bytes": checkpoint_bytes,
          "temporary_artifacts_removed": True,
          "limits": ["doctor invocation is not a GPU capability acceptance test",
                     "small synthetic fixtures do not validate navigation learning or full-graph training"]}
with (stage / cli_options.result).open("x") as stream:
    json.dump(result, stream, indent=2)
    stream.write("\n")
print(json.dumps({k: v for k, v in result.items() if k != "checks"}))
