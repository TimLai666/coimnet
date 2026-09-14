#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || $1 == --help || $1 == -h ]]; then
  echo 'Usage: scripts/simulate-evidence.sh NEW_OUTPUT_DIRECTORY PROTOCOL_FILE'
  echo 'Builds the CLI, records doctor/binary/protocol/store fingerprints and the'
  echo 'machine load, then runs'
  echo '  coimnet simulate run --store data/malecns-v1.0/graph-v1.coimgraph --protocol PROTOCOL_FILE'
  echo 'under /usr/bin/time, saving report.json, run.log, started-at/finished-at and'
  echo 'the fingerprint of the state snapshot written after the run. The state file'
  echo 'itself is removed once hashed; only its size and SHA-256 are kept.'
  echo 'Requires the whole-brain graph store at data/malecns-v1.0/graph-v1.coimgraph.'
  echo 'The output directory must not exist. Run from the repository root with Go on PATH.'
  if [[ $# -ge 1 && ($1 == --help || $1 == -h) ]]; then exit 0; fi
  exit 2
fi
protocol=$2
store=data/malecns-v1.0/graph-v1.coimgraph
if [[ ! -f $protocol ]]; then
  echo "protocol file $protocol does not exist" >&2
  exit 2
fi
if [[ ! -f $store ]]; then
  echo "graph store $store does not exist" >&2
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
./bin/coimnet doctor > "$output/doctor.json"
uptime > "$output/load-average.txt"
printf 'scripts/simulate-evidence.sh %q %q\n' "$1" "$protocol" > "$output/command.txt"
if [[ $(uname -s) == Darwin ]]; then
  time_command=(/usr/bin/time -l)
else
  time_command=(/usr/bin/time -v)
fi
state="$output/state.json"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/started-at.txt"
set +e
"${time_command[@]}" ./bin/coimnet simulate run \
  --store "$store" --protocol "$protocol" --state-out "$state" \
  > "$output/report.json" 2> "$output/run.log"
status=$?
set -e
echo "exit=$status" >> "$output/run.log"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/finished-at.txt"
uptime >> "$output/load-average.txt"
if [[ -f $state ]]; then
  wc -c < "$state" | tr -d ' ' > "$output/state-bytes.txt"
  (cd "$output" && "${hash_command[@]}" state.json) > "$output/state.sha256"
  rm "$state"
  echo 'state snapshot hashed and removed; only its size and SHA-256 are kept' >> "$output/run.log"
fi
if [[ $status != 0 ]]; then
  echo "RESULT: simulate run failed with exit $status" >&2
  exit "$status"
fi
echo 'RESULT: simulate evidence recorded'
