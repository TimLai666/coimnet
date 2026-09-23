#!/usr/bin/env bash
set -euo pipefail

# Re-runs the verification suite in a clean export of the committed tree
# (git archive HEAD), so nothing in the working tree can influence the result,
# and records one row per step in a coimnet-clean-env-run/v1 document.
#
# Usage: scripts/clean-env-verify.sh [--workdir DIR] [--data DIR] [--out FILE] [--list-steps]
#   --workdir DIR  scratch directory for the export and the step outputs;
#                  default is a new mktemp directory, which is removed at the
#                  end. A directory given here is kept. The script replaces
#                  src/ and the paths used by its steps inside this directory.
#   --data DIR     official MaleCNS data directory holding the graph store,
#                  derived parameters and the three Feather source files.
#                  Without it, data-dependent steps are recorded as blocked_data.
#   --out FILE     run document; default evidence/OPS-10/run.json relative to
#                  the repository root. Step logs go to <dirname>/logs/<step>.log.
#   --list-steps   print every step and its condition, then exit without export
#                  or execution.
#
# A failing step never stops the run; it is recorded and the script exits 1 at
# the end. Blocked steps are not failures.

usage() {
  cat <<'USAGE'
Usage: scripts/clean-env-verify.sh [--workdir DIR] [--data DIR] [--out FILE] [--list-steps]
  --list-steps   print each step and its condition (always, needs --data, or
                 always blocked), then exit without exporting or executing.
  --workdir DIR  scratch directory for the export and the step outputs; default
                 is a new mktemp directory, which is removed at the end. A
                 directory given here is kept. The script replaces src/ and
                 the paths used by its steps inside this directory.
  --data DIR     official MaleCNS directory. It must contain graph-v1.coimgraph,
                 params-derive-v1.coimparams, connectome-weights-male-cns-v1.0-
                 minconf-0.5.feather, body-annotations-male-cns-v1.0-minconf-
                 0.5.feather and body-neurotransmitters-male-cns-v1.0.feather.
                 It is linked at data/malecns-v1.0 in the clean export.
                 Without it, data-dependent steps are recorded as blocked_data.
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
list_steps_requested=false

list_steps() {
  local steps=(
    $'go_mod_download\talways'
    $'go_mod_verify\talways'
    $'go_build\talways'
    $'go_vet\talways'
    $'go_test\talways'
    $'doctor\talways'
    $'delayed_correlation\talways'
    $'threshold_training\talways'
    $'adaptive_evaluation\talways'
    $'continual_matrix\talways'
    $'modulation_intervention\talways'
    $'offline_teacher\talways'
    $'snapshot_restore\talways'
    $'example_ocr\talways'
    $'example_asr\talways'
    $'example_textgen\talways'
    $'example_media_image\talways'
    $'example_media_audio\talways'
    $'example_roles\talways'
    $'example_video\talways'
    $'example_gridnav\talways'
    $'example_nav2d\talways'
    $'example_multimodal\talways'
    $'example_attribution\talways'
    $'example_ablate\talways'
    $'benchmark_fixture\talways'
    $'multitask_suite\talways'
    $'recompute_rss_small\talways'
    $'real_subgraph_import\tneeds --data'
    $'real_subgraph_train\tneeds --data'
    $'full_graph_validate\tneeds --data'
    $'full_graph_short_training\tneeds --data'
    $'full_graph_resume\tneeds --data'
    $'benchmark_full_graph\tneeds --data'
    $'real_task_classes\talways blocked'
    $'device_backend\talways blocked'
    $'completion_report\talways'
  )
  printf '%s\n' "${steps[@]}"
}

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
    --list-steps)
      list_steps_requested=true
      shift
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

if [[ $list_steps_requested == true ]]; then
  list_steps
  exit 0
fi

repo_root=$(git rev-parse --show-toplevel)
head=$(git rev-parse HEAD)

if [[ -n $data_dir ]]; then
  if [[ ! -d $data_dir ]]; then
    printf 'clean-env-verify: official data directory does not exist: %s\n' "$data_dir" >&2
    exit 2
  fi
  data_dir=$(cd "$data_dir" && pwd -P)
  required_data_files=(
    graph-v1.coimgraph
    params-derive-v1.coimparams
    connectome-weights-male-cns-v1.0-minconf-0.5.feather
    body-annotations-male-cns-v1.0-minconf-0.5.feather
    body-neurotransmitters-male-cns-v1.0.feather
  )
  for file in "${required_data_files[@]}"; do
    if [[ ! -f $data_dir/$file ]]; then
      printf 'clean-env-verify: official data directory is missing required file: %s/%s\n' "$data_dir" "$file" >&2
      exit 2
    fi
  done
fi

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
last_step_status=""

record() { # step command status log_path
  local entry
  last_step_status=$3
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

if [[ -n $data_dir ]]; then
  mkdir -p "$src/data"
  ln -s "$data_dir" "$src/data/malecns-v1.0"
fi

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

rm -f "$workdir/ocr.json" "$workdir/asr.json" "$workdir/textgen.json" \
  "$workdir/nav2d.json" "$workdir/multimodal.json" "$workdir/attribution.json" \
  "$workdir/roles.json" "$workdir/benchmark-fixture.json"
rm -rf "$workdir/media-image" "$workdir/media-audio" "$workdir/video" "$workdir/ablate" \
  "$workdir/fullgraph" "$workdir/tsk09"
run_step example_ocr go run ./cmd/coimnet examples run ocr --out "$workdir/ocr.json"
run_step example_asr go run ./cmd/coimnet examples run asr --out "$workdir/asr.json"
run_step example_textgen go run ./cmd/coimnet examples run textgen --out "$workdir/textgen.json"
run_step example_media_image go run ./cmd/coimnet examples run media --modality image --out-dir "$workdir/media-image"
run_step example_media_audio go run ./cmd/coimnet examples run media --modality audio --out-dir "$workdir/media-audio"
run_step example_roles go run ./cmd/coimnet examples run roles --out "$workdir/roles.json"
run_step example_video go run ./cmd/coimnet examples run video --packager none --out-dir "$workdir/video"
run_step example_gridnav go run ./cmd/coimnet examples run gridnav
run_step example_nav2d go run ./cmd/coimnet examples run nav2d --out "$workdir/nav2d.json"
run_step example_multimodal go run ./cmd/coimnet examples run multimodal --out "$workdir/multimodal.json"
run_step example_attribution go run ./cmd/coimnet examples run attribution --out "$workdir/attribution.json"
# ablate writes to DIR/ablation, so DIR must exist before it runs.
mkdir -p "$workdir/ablate"
run_step example_ablate go run ./cmd/coimnet examples run ablate --out-dir "$workdir/ablate"
run_step benchmark_fixture go run ./cmd/coimnet benchmark --out "$workdir/benchmark-fixture.json"
run_step multitask_suite env COIMNET_TSK09_EVIDENCE="$workdir/tsk09" \
  go test -count=1 -timeout 0 -run '^TestMultiTaskEvidence$' ./experiment/
rm -f "$workdir/rss-small.json"
run_step recompute_rss_small bash scripts/recompute-evidence.sh --scale small --out "$workdir/rss-small.json"

memory_gated_step() { # step command...
  local step=$1
  shift
  local log="$log_dir/$step.log"
  local text status memory_line memory_pattern
  text=$(command_text "$@")
  printf 'COMMAND: %s\n' "$text" > "$log"
  status=0
  (cd "$src" && "$@") >> "$log" 2>&1 || status=$?
  printf 'EXIT: %d\n' "$status" >> "$log"
  if [[ $status -eq 0 ]]; then
    record "$step" "$text" passed "logs/$step.log"
    return
  fi
  memory_pattern='the plan estimates [0-9]+ MiB, above the [0-9]+ MiB limit'
  if grep -Eq "$memory_pattern" "$log"; then
    memory_line=$(grep -E "$memory_pattern" "$log" | head -n 1 || true)
    printf 'BLOCKED: %s\n' "$memory_line" >> "$log"
    record "$step" "$text" blocked_hardware "logs/$step.log"
  else
    record "$step" "$text" failed "logs/$step.log"
  fi
}

if [[ -n $data_dir ]]; then
  rm -f "$workdir/alin.coimgraph" "$workdir/benchmark-full.json"
  run_step real_subgraph_import go run ./cmd/coimnet data import \
    --manifest examples/realsubgraph/malecns-alin.json \
    --max-memory-bytes 2147483648 --sort-buffer-bytes 536870912 \
    --out-store "$workdir/alin.coimgraph"
  run_step real_subgraph_train go run ./examples/realsubgraph \
    --store "$workdir/alin.coimgraph" \
    --expected-predicate-hash 4565f8edcd638de884b6ecbf90b6300d478772ef4b5ff8407b69fb976490d063 \
    --profile real-subgraph --updates 20
  run_step full_graph_validate go run ./cmd/coimnet data validate \
    --store "$data_dir/graph-v1.coimgraph" --params "$data_dir/params-derive-v1.coimparams"
  rm -rf "$workdir/fullgraph"
  memory_gated_step full_graph_short_training go run ./cmd/coimnet examples run full-graph-short-training \
    --store "$data_dir/graph-v1.coimgraph" --params "$data_dir/params-derive-v1.coimparams" \
    --protocol evidence/NAT-05/compare-fullgraph-derived-continuous.json --out-dir "$workdir/fullgraph"
  full_graph_short_status=$last_step_status
  if [[ $full_graph_short_status == passed ]]; then
    run_step full_graph_resume go run ./cmd/coimnet examples run full-graph-short-training --resume "$workdir/fullgraph"
  else
    block_step full_graph_resume \
      "$(command_text go run ./cmd/coimnet examples run full-graph-short-training --resume "$workdir/fullgraph")" \
      "$full_graph_short_status" \
      "not run because full_graph_short_training status was $full_graph_short_status"
  fi
  memory_gated_step benchmark_full_graph go run ./cmd/coimnet benchmark \
    --store "$data_dir/graph-v1.coimgraph" --params "$data_dir/params-derive-v1.coimparams" \
    --protocol evidence/NAT-05/compare-fullgraph-derived-continuous.json --steps 8 --repeat 2 \
    --out "$workdir/benchmark-full.json"
else
  data_reason="requires --data DIR containing graph-v1.coimgraph, params-derive-v1.coimparams, connectome-weights-male-cns-v1.0-minconf-0.5.feather, body-annotations-male-cns-v1.0-minconf-0.5.feather and body-neurotransmitters-male-cns-v1.0.feather"
  block_step real_subgraph_import \
    "$(command_text go run ./cmd/coimnet data import --manifest examples/realsubgraph/malecns-alin.json --max-memory-bytes 2147483648 --sort-buffer-bytes 536870912 --out-store "$workdir/alin.coimgraph")" \
    blocked_data "$data_reason"
  block_step real_subgraph_train \
    "$(command_text go run ./examples/realsubgraph --store "$workdir/alin.coimgraph" --expected-predicate-hash 4565f8edcd638de884b6ecbf90b6300d478772ef4b5ff8407b69fb976490d063 --profile real-subgraph --updates 20)" \
    blocked_data "$data_reason"
  block_step full_graph_validate \
    "$(command_text go run ./cmd/coimnet data validate --store DATA/graph-v1.coimgraph --params DATA/params-derive-v1.coimparams)" \
    blocked_data "$data_reason"
  block_step full_graph_short_training \
    "$(command_text go run ./cmd/coimnet examples run full-graph-short-training --store DATA/graph-v1.coimgraph --params DATA/params-derive-v1.coimparams --protocol evidence/NAT-05/compare-fullgraph-derived-continuous.json --out-dir "$workdir/fullgraph")" \
    blocked_data "$data_reason"
  block_step full_graph_resume \
    "$(command_text go run ./cmd/coimnet examples run full-graph-short-training --resume "$workdir/fullgraph")" \
    blocked_data "not run because full_graph_short_training status was blocked_data; $data_reason"
  block_step benchmark_full_graph \
    "$(command_text go run ./cmd/coimnet benchmark --store DATA/graph-v1.coimgraph --params DATA/params-derive-v1.coimparams --protocol evidence/NAT-05/compare-fullgraph-derived-continuous.json --steps 8 --repeat 2 --out "$workdir/benchmark-full.json")" \
    blocked_data "$data_reason"
fi

block_step real_task_classes \
  "$(command_text go run ./cmd/coimnet examples run '<task>' --data DATA)" blocked_data \
  "TSK-11 requires user-provided authorized data for each task class"
block_step device_backend 'GPU backend execution' blocked_hardware \
  "ticket 30 item 16: GPU backend technology and target machine have not been selected"

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
