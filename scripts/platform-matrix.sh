#!/usr/bin/env bash
set -euo pipefail

# OPS-04, ticket 30 stage 1: cross-compilation / platform matrix.
#
# Verifies that ./... compiles and vets on four target platforms in fixed order
# (linux/amd64, linux/arm64, darwin/arm64, windows/amd64), then runs the full
# unit tests, race tests, vet and go mod verify checks once, on the host this
# script runs on. Every command's full output is appended to
# evidence/OPS-04/platform-matrix.log (truncated at start) and the observed
# results are written to evidence/OPS-04/platform-matrix.json. A failing step is
# recorded in the JSON/log but never aborts the run; the script exits 1 if any
# step failed, 0 otherwise. Run it from anywhere; it resolves the repository
# root itself.

usage() {
  echo 'Usage: scripts/platform-matrix.sh'
  echo 'Builds and vets ./... for linux/amd64, linux/arm64, darwin/arm64 and windows/amd64,'
  echo 'then runs go test, go test -race, go vet and go mod verify on the host platform.'
  echo 'Writes evidence/OPS-04/platform-matrix.json and appends every step output to'
  echo 'evidence/OPS-04/platform-matrix.log.'
  echo 'Takes no argument.'
  echo 'Exit status: 0 all steps passed, 1 any step failed, 2 usage error.'
}

if [[ $# != 0 ]]; then
  if [[ ${1} == --help || ${1} == -h ]]; then
    usage
    exit 0
  fi
  usage
  exit 2
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

if ! command -v python3 >/dev/null 2>&1; then
  echo "platform-matrix: python3 is required for JSON generation and timing" >&2
  exit 2
fi

outdir="$root/evidence/OPS-04"
mkdir -p "$outdir"
logfile="$outdir/platform-matrix.log"
jsonfile="$outdir/platform-matrix.json"
: >"$logfile"

host_goos=$(go env GOOS)
host_goarch=$(go env GOARCH)
go_version=$(go version)
host_uptime=$(uptime)

{
  echo "platform-matrix (OPS-04, ticket 30 stage 1)"
  echo "repository: $root"
  echo "go: $go_version"
  echo "host: $host_goos/$host_goarch"
  echo "uptime: $host_uptime"
} >>"$logfile"

status=0
pl_goos=(linux linux darwin windows)
pl_arch=(amd64 arm64 arm64 amd64)
build_ok=() vet_ok=() build_sec=() vet_sec=()

log_header() {
  {
    echo
    echo "== $*"
    printf '   at %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  } >>"$logfile"
}

run_step() {
  local out
  out=$(python3 -c '
import subprocess, sys, time
logpath, cmd = sys.argv[1], sys.argv[2:]
t0 = time.monotonic()
with open(logpath, "a") as fh:
    rc = subprocess.run(cmd, stdout=fh, stderr=subprocess.STDOUT).returncode
elapsed = round(time.monotonic() - t0, 3)
print("1" if rc == 0 else "0", "%.3f" % elapsed, sep=" ")
' "$logfile" "$@")
  step_ok=${out%% *}
  step_elapsed=${out##* }
}

report_step() {
  local label=$1 ok=$2 sec=$3
  if [[ $ok == 1 ]]; then
    printf '[ok   ] %s (%ss)\n' "$label" "$sec"
  else
    printf '[FAIL ] %s (see %s)\n' "$label" "${logfile#$root/}" >&2
  fi
}

for i in 0 1 2 3; do
  goos=${pl_goos[$i]}
  arch=${pl_arch[$i]}
  log_header "env GOOS=$goos GOARCH=$arch go build ./..."
  run_step env GOOS="$goos" GOARCH="$arch" go build ./...
  build_ok[$i]=$step_ok
  build_sec[$i]=$step_elapsed
  report_step "$goos/$arch go build ./..." "$step_ok" "$step_elapsed"
  log_header "env GOOS=$goos GOARCH=$arch go vet ./..."
  run_step env GOOS="$goos" GOARCH="$arch" go vet ./...
  vet_ok[$i]=$step_ok
  vet_sec[$i]=$step_elapsed
  report_step "$goos/$arch go vet ./..." "$step_ok" "$step_elapsed"
done

log_header 'go test -count=1 ./...'
run_step go test -count=1 ./...
host_unit_ok=$step_ok
host_unit_sec=$step_elapsed
report_step 'go test -count=1 ./...' "$host_unit_ok" "$host_unit_sec"

log_header 'go test -race -count=1 ./dynamics/ ./learning/ ./simulate/'
run_step go test -race -count=1 ./dynamics/ ./learning/ ./simulate/
host_race_ok=$step_ok
host_race_sec=$step_elapsed
report_step 'go test -race -count=1 ./dynamics/ ./learning/ ./simulate/' "$host_race_ok" "$host_race_sec"

log_header 'go vet ./...'
run_step go vet ./...
host_vet_ok=$step_ok
host_vet_sec=$step_elapsed
report_step 'go vet ./...' "$host_vet_ok" "$host_vet_sec"

log_header 'go mod verify'
run_step go mod verify
host_mv_ok=$step_ok
host_mv_sec=$step_elapsed
report_step 'go mod verify' "$host_mv_ok" "$host_mv_sec"

for i in 0 1 2 3; do
  if [[ ${build_ok[$i]} != 1 || ${vet_ok[$i]} != 1 ]]; then status=1; fi
done
if [[ $host_unit_ok != 1 || $host_race_ok != 1 || $host_vet_ok != 1 || $host_mv_ok != 1 ]]; then status=1; fi

platform_lines=""
for i in 0 1 2 3; do
  platform_lines+="${pl_goos[$i]} ${pl_arch[$i]} ${build_ok[$i]} ${vet_ok[$i]}"$'\n'
done

export PM_GO_VERSION="$go_version"
export PM_HOST_GOOS="$host_goos"
export PM_HOST_GOARCH="$host_goarch"
export PM_UPTIME="$host_uptime"
export PM_PLATFORM_LINES="$platform_lines"
export PM_UNIT_OK="$host_unit_ok" PM_UNIT_SEC="$host_unit_sec"
export PM_RACE_OK="$host_race_ok" PM_RACE_SEC="$host_race_sec"
export PM_VET_OK="$host_vet_ok" PM_VET_SEC="$host_vet_sec"
export PM_MV_OK="$host_mv_ok" PM_MV_SEC="$host_mv_sec"

python3 - "$jsonfile" <<'PY'
import json
import os
import sys
from datetime import datetime, timezone

json_path = sys.argv[1]
host_goos = os.environ["PM_HOST_GOOS"]
host_goarch = os.environ["PM_HOST_GOARCH"]

test_keys = ("unit", "race", "vet", "mod_verify")
ok_var = {"unit": "PM_UNIT_OK", "race": "PM_RACE_OK",
          "vet": "PM_VET_OK", "mod_verify": "PM_MV_OK"}
sec_var = {"unit": "PM_UNIT_SEC", "race": "PM_RACE_SEC",
           "vet": "PM_VET_SEC", "mod_verify": "PM_MV_SEC"}

tests = {}
seconds = {}
for key in test_keys:
    tests[key] = os.environ[ok_var[key]] == "1"
    seconds[key] = float(os.environ[sec_var[key]])

platforms = []
for line in os.environ["PM_PLATFORM_LINES"].splitlines():
    goos, goarch, built, vetted = line.split()
    executed = goos == host_goos and goarch == host_goarch
    entry = {
        "goos": goos,
        "goarch": goarch,
        "compiled": built == "1",
        "vetted": vetted == "1",
        "executed": executed,
    }
    if executed:
        entry["tests"] = dict(tests, seconds=dict(seconds))
    else:
        entry["tests"] = None
    platforms.append(entry)

payload = {
    "schema_version": "coimnet-platform-matrix/v1",
    "generated_at": datetime.now(timezone.utc)
        .isoformat(timespec="seconds")
        .replace("+00:00", "Z"),
    "go_version": os.environ["PM_GO_VERSION"],
    "host": {
        "goos": host_goos,
        "goarch": host_goarch,
        "uptime": os.environ["PM_UPTIME"],
    },
    "platforms": platforms,
    "note": "executed is true only for the host this script ran on; a compiled platform has not been run.",
}

with open(json_path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, indent=2)
    handle.write("\n")
PY

compiled_count=0
for i in 0 1 2 3; do
  if [[ ${build_ok[$i]} == 1 ]]; then compiled_count=$((compiled_count + 1)); fi
done
resultline="RESULT: compiled=$compiled_count/4 executed=$host_goos/$host_goarch"
echo "$resultline"
echo "$resultline" >>"$logfile"

exit $status