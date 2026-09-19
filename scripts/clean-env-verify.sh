#!/usr/bin/env bash
set -euo pipefail

# Re-runs the verification suite in a clean export of the committed tree
# (git archive HEAD), so nothing in the working tree can influence the result,
# and records one row per step in a coimnet-clean-env-run/v1 document.
#
# Usage: scripts/clean-env-verify.sh [--workdir DIR] [--data DIR] [--out FILE]
#   --workdir DIR  scratch directory for the export and the step outputs;
#                  default is a new mktemp directory, which is removed at the
#                  end. A directory given here is kept. Inside it the script
#                  owns and replaces src/, continual.json, report.json,
#                  report.md, first.json, resumed.json and continuous.json.
#   --data DIR     official data directory holding manifest.json; without it
#                  the data-dependent steps are recorded as blocked_data.
#   --out FILE     run document; default evidence/OPS-10/run.json relative to
#                  the repository root. Step logs go to <dirname>/logs/<step>.log.
#
# A failing step never stops the run; it is recorded and the script exits 1 at
# the end. Blocked steps are not failures.

usage() {
  cat <<'USAGE'
Usage: scripts/clean-env-verify.sh [--workdir DIR] [--data DIR] [--out FILE]
  --workdir DIR  scratch directory for the export and the step outputs; default
                 is a new mktemp directory, which is removed at the end. A
                 directory given here is kept. Inside it the script owns and
                 replaces src/, continual.json, report.json, report.md,
                 first.json, resumed.json and continuous.json.
  --data DIR     official data directory holding manifest.json; without it the
                 data-dependent steps are recorded as blocked_data.
  --out FILE     run document; default evidence/OPS-10/run.json relative to the
                 repository root. Step logs go to <dirname>/logs/<step>.log.

Every step runs inside a clean export of the committed tree (git archive HEAD),
never in the working tree. A failing step is recorded and the run continues;
the script exits 1 when any step failed. Blocked steps are not failures.
USAGE
}

workdir=""
data_dir=""
out_file=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --workdir|--data|--out)
      if [[ $# -lt 2 ]]; then
        printf 'clean-env-verify: %s needs a value\n' "$1" >&2
        exit 2
      fi
      case $1 in
        --workdir) workdir=$2 ;;
        --data) data_dir=$2 ;;
        --out) out_file=$2 ;;
      esac
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'clean-env-verify: unknown option %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

repo_root=$(git rev-parse --show-toplevel)
head=$(git rev-parse HEAD)

if [[ -z $out_file ]]; then
  out_file="$repo_root/evidence/OPS-10/run.json"
elif [[ $out_file != /* ]]; then
  out_file="$PWD/$out_file"
fi
out_dir=$(dirname "$out_file")
mkdir -p "$out_dir/logs"
out_dir=$(cd "$out_dir" && pwd)
out_file="$out_dir/$(basename "$out_file")"
log_dir="$out_dir/logs"

workdir_is_default=false
if [[ -z $workdir ]]; then
  workdir=$(mktemp -d)
  workdir_is_default=true
fi
mkdir -p "$workdir"
workdir=$(cd "$workdir" && pwd)
src="$workdir/src"

if command -v sha256sum >/dev/null 2>&1; then
  hash_command=(sha256sum)
else
  hash_command=(shasum -a 256)
fi

json_string() {
  local value=$1
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\t'/\\t}
  value=${value//$'\r'/\\r}
  value=${value//$'\n'/\\n}
  printf '"%s"' "$value"
}

# command_text quotes an argument vector the way scripts/verify.sh logs it.
command_text() {
  local text
  text=$(printf '%q ' "$@")
  printf '%s' "${text% }"
}

config_hash() {
  printf '%s\n%s' "$head" "$1" | "${hash_command[@]}" | cut -d' ' -f1
}

steps_json=""
failed=0

record() { # step command status log_path
  local entry
  entry=$(printf '    {"step": %s, "command": %s, "config_hash": %s, "status": %s, "log": %s}' \
    "$(json_string "$1")" \
    "$(json_string "$2")" \
    "$(json_string "$(config_hash "$2")")" \
    "$(json_string "$3")" \
    "$(json_string "$4")")
  if [[ -n $steps_json ]]; then
    steps_json="$steps_json,"$'\n'
  fi
  steps_json="$steps_json$entry"
  printf '%-26s %s\n' "$1" "$3"
  if [[ $3 == failed ]]; then failed=1; fi
}

# run_step STEP -- COMMAND... runs one command inside the clean export.
run_step() {
  local step=$1
  shift
  local log="$log_dir/$step.log"
  local text
  text=$(command_text "$@")
  printf 'COMMAND: %s\n' "$text" > "$log"
  local status=0
  (cd "$src" && "$@") >> "$log" 2>&1 || status=$?
  printf 'EXIT: %d\n' "$status" >> "$log"
  if [[ $status -eq 0 ]]; then
    record "$step" "$text" passed "logs/$step.log"
  else
    record "$step" "$text" failed "logs/$step.log"
  fi
}

block_step() { # step command status reason
  local log="$log_dir/$1.log"
  printf 'COMMAND: %s\n%s\n' "$2" "$4" > "$log"
  record "$1" "$2" "$3" "logs/$1.log"
}

printf 'head: %s\nworkdir: %s\nout: %s\n\n' "$head" "$workdir" "$out_file"

rm -rf "$src"
mkdir -p "$src"
git -C "$repo_root" archive HEAD | tar -x -C "$src"

run_step go_mod_download go mod download
run_step go_mod_verify go mod verify
run_step go_build go build ./...
run_step go_vet go vet ./...
run_step go_test go test -count=1 ./...
run_step doctor go run ./cmd/coimnet doctor
run_step delayed_correlation go run ./cmd/coimnet examples run delayed
run_step threshold_training go run ./cmd/coimnet examples run lif-threshold
# examples run evaluate requires --mode and --out; this step is the adaptive mode.
rm -f "$workdir/evaluate.json"
run_step adaptive_evaluation go run ./cmd/coimnet examples run evaluate --mode adaptive --out "$workdir/evaluate.json"

rm -f "$workdir/continual.json"
run_step continual_matrix go run ./cmd/coimnet examples run continual-matrix --out "$workdir/continual.json"

run_step modulation_intervention go test -count=1 -run 'TestIntervene|TestIntervention' ./learning/ ./simulate/
run_step offline_teacher go test -count=1 -run 'TestOffline|TestReplay|TestBlocked' ./teacher/

# snapshot_restore keeps the three-step train/resume/train shape of
# scripts/verify.sh: the resumed run must match the continuous run byte for byte.
snapshot_log="$log_dir/snapshot_restore.log"
rm -f "$workdir/first.json" "$workdir/resumed.json" "$workdir/continuous.json"
snapshot_commands=(
  "$(command_text go run ./cmd/coimnet train delayed --steps 80 --checkpoint "$workdir/first.json")"
  "$(command_text go run ./cmd/coimnet resume --checkpoint "$workdir/first.json" --steps 40 --out "$workdir/resumed.json")"
  "$(command_text go run ./cmd/coimnet train delayed --steps 120 --checkpoint "$workdir/continuous.json")"
  "$(command_text cmp "$workdir/resumed.json" "$workdir/continuous.json")"
)
snapshot_text="${snapshot_commands[0]} && ${snapshot_commands[1]} && ${snapshot_commands[2]} && ${snapshot_commands[3]}"
printf 'COMMAND: %s\n' "$snapshot_text" > "$snapshot_log"
snapshot_status=0
(
  cd "$src" &&
    go run ./cmd/coimnet train delayed --steps 80 --checkpoint "$workdir/first.json" &&
    go run ./cmd/coimnet resume --checkpoint "$workdir/first.json" --steps 40 --out "$workdir/resumed.json" &&
    go run ./cmd/coimnet train delayed --steps 120 --checkpoint "$workdir/continuous.json" &&
    cmp "$workdir/resumed.json" "$workdir/continuous.json"
) >> "$snapshot_log" 2>&1 || snapshot_status=$?
printf 'EXIT: %d\n' "$snapshot_status" >> "$snapshot_log"
if [[ $snapshot_status -eq 0 ]]; then
  record snapshot_restore "$snapshot_text" passed logs/snapshot_restore.log
else
  record snapshot_restore "$snapshot_text" failed logs/snapshot_restore.log
fi

real_subgraph_text=$(command_text go run ./cmd/coimnet data validate --manifest "${data_dir:-DATA}/manifest.json")
if [[ -n $data_dir && -f "$data_dir/manifest.json" ]]; then
  run_step real_subgraph go run ./cmd/coimnet data validate --manifest "$data_dir/manifest.json"
elif [[ -n $data_dir ]]; then
  block_step real_subgraph "$real_subgraph_text" blocked_data \
    "official data directory not provided: no manifest.json under $data_dir"
else
  block_step real_subgraph "$real_subgraph_text" blocked_data \
    "official data directory not provided"
fi

# The two steps below have no entry point in this repository yet: the command is
# the one their ticket declares (ticket 30 item 11, ticket 29 item 1), kept so the
# blocked record names what would have to run.
block_step full_graph_short_training \
  "$(command_text go run ./cmd/coimnet examples run full-graph-short-training)" blocked_data \
  "OPS-07 full-graph flow is a separate ticket; not run here"
block_step real_task_classes \
  "$(command_text go run ./cmd/coimnet examples run '<task>' --data DATA)" blocked_data \
  "TSK-11 real data not provided"

rm -f "$workdir/report.json" "$workdir/report.md"
run_step completion_report go run ./cmd/coimnet report \
  --status docs/requirements-status.json \
  --evidence-dir evidence \
  --out-json "$workdir/report.json" \
  --out-md "$workdir/report.md"

workdir_removed=false
if [[ $workdir_is_default == true ]]; then
  if rm -rf "$workdir"; then
    workdir_removed=true
  fi
fi

printf '{\n  "schema_version": "coimnet-clean-env-run/v1",\n  "head": %s,\n  "generated_at": %s,\n  "workdir_removed": %s,\n  "steps": [\n%s\n  ]\n}\n' \
  "$(json_string "$head")" \
  "$(json_string "$(date -u '+%Y-%m-%dT%H:%M:%SZ')")" \
  "$workdir_removed" \
  "$steps_json" > "$out_file"

printf '\nwrote %s\n' "$out_file"
if [[ $failed -ne 0 ]]; then
  printf 'RESULT: at least one step failed\n'
  exit 1
fi
printf 'RESULT: no step failed\n'
