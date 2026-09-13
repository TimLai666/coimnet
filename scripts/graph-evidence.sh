#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 || $1 == --help || $1 == -h ]]; then
  echo 'Usage: scripts/graph-evidence.sh NEW_OUTPUT_DIRECTORY [NEW_STORE_FILE]'
  echo 'Builds the CLI, records doctor/binary/manifest fingerprints, then runs'
  echo '  coimnet data import --manifest manifests/malecns-v1.0.json [--out-store NEW_STORE_FILE]'
  echo 'under /usr/bin/time with a fresh temporary directory, saving report.json (or'
  echo 'import.json with the store receipt), run.log, started-at/finished-at and the'
  echo 'temp directory listing after the run. With NEW_STORE_FILE it then runs'
  echo '  coimnet data validate --store NEW_STORE_FILE'
  echo 'in a new process, saving validate.json and validate.log. Requires the three'
  echo 'official MaleCNS v1.0 files at data/malecns-v1.0/. The output directory and the'
  echo 'store file must not exist. Run from the repository root with Go on PATH.'
  if [[ $# -ge 1 && ($1 == --help || $1 == -h) ]]; then exit 0; fi
  exit 2
fi
store=${2:-}
if [[ -n $store && -e $store ]]; then
  echo "store file $store already exists" >&2
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
(cd manifests && "${hash_command[@]}" malecns-v1.0.json) > "$output/manifest.sha256"
./bin/coimnet doctor > "$output/doctor.json"
tmp=$(mktemp -d)
echo "$tmp" > "$output/temp-dir.txt"
printf 'scripts/graph-evidence.sh %q %q\n' "$1" "$store" > "$output/command.txt"
if [[ $(uname -s) == Darwin ]]; then
  time_command=(/usr/bin/time -l)
else
  time_command=(/usr/bin/time -v)
fi
import_args=(data import --manifest manifests/malecns-v1.0.json --temp-dir "$tmp")
report="$output/report.json"
if [[ -n $store ]]; then
  import_args+=(--out-store "$store")
  report="$output/import.json"
fi
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/started-at.txt"
set +e
"${time_command[@]}" ./bin/coimnet "${import_args[@]}" > "$report" 2> "$output/run.log"
status=$?
set -e
echo "exit=$status" >> "$output/run.log"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/finished-at.txt"
ls -la "$tmp" > "$output/temp-dir-after.txt"
rmdir "$tmp" 2>/dev/null || echo 'temporary directory not empty; kept for inspection' >> "$output/temp-dir-after.txt"
if [[ $status != 0 ]]; then
  echo "RESULT: data import failed with exit $status" >&2
  exit "$status"
fi
if [[ -n $store ]]; then
  "${hash_command[@]}" "$store" > "$output/store.sha256"
  date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/validate-started-at.txt"
  set +e
  "${time_command[@]}" ./bin/coimnet data validate --store "$store" > "$output/validate.json" 2> "$output/validate.log"
  status=$?
  set -e
  echo "exit=$status" >> "$output/validate.log"
  date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/validate-finished-at.txt"
  if [[ $status != 0 ]]; then
    echo "RESULT: data validate failed with exit $status" >&2
    exit "$status"
  fi
fi
echo 'RESULT: graph evidence recorded'
