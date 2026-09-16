#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 || $1 == --help || $1 == -h ]]; then
  echo 'Usage: scripts/plasticity-evidence.sh NEW_OUTPUT_DIRECTORY PROTOCOL_FILE [PARAMS_FILE]'
  echo 'Builds the CLI, records doctor/binary/protocol/store fingerprints and the'
  echo 'machine load, then runs'
  echo '  coimnet simulate compare --store data/malecns-v1.0/graph-v1.coimgraph'
  echo '    --protocol PROTOCOL_FILE --out-dir OUTPUT_DIRECTORY/cells'
  echo 'under /usr/bin/time, saving compare.json, run.log, started-at/finished-at and'
  echo 'one run report per cell under cells/.'
  echo 'The protocol must declare the run plasticity block and learning_variants, so'
  echo 'the matrix is the three learning cells original, plastic and'
  echo 'learned_then_frozen over one stimulus. Every cell is a whole-brain run and the'
  echo 'two plastic cells advance one step per core call, which is the wall clock cost'
  echo 'the run report states; no step, seed or limit is lowered here.'
  echo 'With PARAMS_FILE the run adds --params PARAMS_FILE, which the derived_release/v1'
  echo 'parameter source requires, and the SHA-256 of that parameter set is recorded in'
  echo 'params.sha256. The parameter set itself is read only and never modified.'
  echo 'Requires the whole-brain graph store at data/malecns-v1.0/graph-v1.coimgraph.'
  echo 'The output directory must not exist. Run from the repository root with Go on PATH.'
  if [[ $# -ge 1 && ($1 == --help || $1 == -h) ]]; then exit 0; fi
  exit 2
fi
protocol=$2
parameters=${3:-}
store=data/malecns-v1.0/graph-v1.coimgraph
if [[ ! -f $protocol ]]; then
  echo "protocol file $protocol does not exist" >&2
  exit 2
fi
if [[ -n $parameters && ! -f $parameters ]]; then
  echo "parameter set $parameters does not exist" >&2
  exit 2
fi
if [[ ! -f $store ]]; then
  echo "graph store $store does not exist" >&2
  exit 2
fi
# This script exists to record the learning matrix, so a protocol without it is
# a mistake rather than a smaller run: use scripts/compare-evidence.sh for that.
if ! grep -q '"plasticity"' "$protocol" || ! grep -q '"learning_variants"' "$protocol"; then
  echo "protocol $protocol declares no plasticity block and learning_variants; use scripts/compare-evidence.sh" >&2
  exit 2
fi
mkdir "$1"
output=$(cd "$1" && pwd)
mkdir -p bin
go build -o bin/coimnet ./cmd/coimnet
if command -v sha256sum >/dev/null 2>&1; then
  hash_command=(sha256sum)
else
  hash_command=(shasum -a 256)
fi
"${hash_command[@]}" bin/coimnet > "$output/binary.sha256"
"${hash_command[@]}" "$protocol" > "$output/protocol.sha256"
"${hash_command[@]}" "$store" > "$output/store.sha256"
run_args=(simulate compare --store "$store" --protocol "$protocol")
if [[ -n $parameters ]]; then
  "${hash_command[@]}" "$parameters" > "$output/params.sha256"
  run_args+=(--params "$parameters")
fi
run_args+=(--out-dir "$output/cells")
./bin/coimnet doctor > "$output/doctor.json"
uptime > "$output/load-average.txt"
printf 'scripts/plasticity-evidence.sh %q %q %q\n' "$1" "$protocol" "$parameters" > "$output/command.txt"
if [[ $(uname -s) == Darwin ]]; then
  time_command=(/usr/bin/time -l)
else
  time_command=(/usr/bin/time -v)
fi
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/started-at.txt"
set +e
"${time_command[@]}" ./bin/coimnet "${run_args[@]}" \
  > "$output/compare.json" 2> "$output/run.log"
status=$?
set -e
echo "exit=$status" >> "$output/run.log"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/finished-at.txt"
uptime >> "$output/load-average.txt"
if [[ $status != 0 ]]; then
  echo "RESULT: simulate compare failed with exit $status" >&2
  exit "$status"
fi
echo 'RESULT: plasticity evidence recorded'
