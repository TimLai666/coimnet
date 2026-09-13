#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 1 || $1 == --help || $1 == -h ]]; then
  echo 'Usage: scripts/graph-evidence.sh NEW_OUTPUT_DIRECTORY'
  echo 'Builds the CLI, records doctor/binary/manifest fingerprints, then runs'
  echo '  coimnet data import --manifest manifests/malecns-v1.0.json'
  echo 'under /usr/bin/time with a fresh temporary directory, saving report.json,'
  echo 'run.log, started-at/finished-at and the temp directory listing after the run.'
  echo 'Requires the three official MaleCNS v1.0 files at data/malecns-v1.0/. The'
  echo 'output directory must not exist. Run from the repository root with Go on PATH.'
  if [[ $# == 1 && ($1 == --help || $1 == -h) ]]; then exit 0; fi
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
printf 'scripts/graph-evidence.sh %q\n' "$1" > "$output/command.txt"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/started-at.txt"
set +e
if [[ $(uname -s) == Darwin ]]; then
  /usr/bin/time -l ./bin/coimnet data import --manifest manifests/malecns-v1.0.json --temp-dir "$tmp" > "$output/report.json" 2> "$output/run.log"
else
  /usr/bin/time -v ./bin/coimnet data import --manifest manifests/malecns-v1.0.json --temp-dir "$tmp" > "$output/report.json" 2> "$output/run.log"
fi
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
echo 'RESULT: graph evidence recorded'
