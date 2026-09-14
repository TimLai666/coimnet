#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 || $1 == --help || $1 == -h ]]; then
  echo 'Usage: scripts/derive-evidence.sh NEW_OUTPUT_DIRECTORY RULES_FILE OUT_PARAMS'
  echo 'Builds the CLI, records doctor/binary/rules/store fingerprints, the SHA-256 of'
  echo 'every source the rules file names, the free space of data/ and the machine load,'
  echo 'then runs'
  echo '  coimnet data derive --store data/malecns-v1.0/graph-v1.coimgraph --rules RULES_FILE'
  echo '    --out OUT_PARAMS --temp-dir data/malecns-v1.0/tmp-derive'
  echo '    --max-memory-bytes 6442450944 --max-temp-bytes 42949672960'
  echo 'under /usr/bin/time, saving derive.json and derive.log, then runs'
  echo '  coimnet data validate --params OUT_PARAMS --store data/malecns-v1.0/graph-v1.coimgraph'
  echo 'into validate.json and records the size and SHA-256 of the parameter set file.'
  echo 'The parameter set is kept: it lives outside git and later runs read it.'
  echo 'The derivation streams 45.6M T-bar and 311.8M synapse rows through four bounded'
  echo 'external sorts and takes tens of minutes; no limit is lowered and nothing is'
  echo 'subsampled. The temporary directory is created here and removed afterwards.'
  echo 'Requires the whole-brain graph store at data/malecns-v1.0/graph-v1.coimgraph and'
  echo 'the four release files at the paths the rules file names, relative to its own'
  echo 'directory. The output directory and OUT_PARAMS must not exist. Run from the'
  echo 'repository root with Go on PATH.'
  if [[ $# -ge 1 && ($1 == --help || $1 == -h) ]]; then exit 0; fi
  exit 2
fi
rules=$2
out_params=$3
store=data/malecns-v1.0/graph-v1.coimgraph
temp=data/malecns-v1.0/tmp-derive
if [[ ! -f $rules ]]; then
  echo "rules file $rules does not exist" >&2
  exit 2
fi
if [[ ! -f $store ]]; then
  echo "graph store $store does not exist" >&2
  exit 2
fi
if [[ -e $out_params ]]; then
  echo "parameter set $out_params already exists" >&2
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
"${hash_command[@]}" "$rules" > "$output/rules.sha256"
"${hash_command[@]}" "$store" > "$output/store.sha256"
# Every source the rules file names, resolved against the rules file directory.
rules_dir=$(cd "$(dirname "$rules")" && pwd)
: > "$output/sources.sha256"
while IFS= read -r relative; do
  [[ -n $relative ]] || continue
  (cd "$rules_dir" && "${hash_command[@]}" "$relative") >> "$output/sources.sha256"
done < <(grep -o '"path"[[:space:]]*:[[:space:]]*"[^"]*"' "$rules" | sed 's/.*"\(.*\)"$/\1/')
./bin/coimnet doctor > "$output/doctor.json"
df -h data > "$output/disk-free.txt"
uptime > "$output/load-average.txt"
printf 'scripts/derive-evidence.sh %q %q %q\n' "$1" "$rules" "$out_params" > "$output/command.txt"
if [[ $(uname -s) == Darwin ]]; then
  time_command=(/usr/bin/time -l)
else
  time_command=(/usr/bin/time -v)
fi
mkdir "$temp"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/started-at.txt"
set +e
"${time_command[@]}" ./bin/coimnet data derive \
  --store "$store" --rules "$rules" --out "$out_params" --temp-dir "$temp" \
  --max-memory-bytes 6442450944 --max-temp-bytes 42949672960 \
  > "$output/derive.json" 2> "$output/derive.log"
status=$?
set -e
echo "exit=$status" >> "$output/derive.log"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/finished-at.txt"
uptime >> "$output/load-average.txt"
df -h data >> "$output/disk-free.txt"
ls -la "$temp" > "$output/temp-dir-after.txt"
rmdir "$temp" 2>/dev/null || echo 'temporary directory not empty; kept for inspection' >> "$output/temp-dir-after.txt"
if [[ $status != 0 ]]; then
  echo "RESULT: data derive failed with exit $status" >&2
  exit "$status"
fi
"${hash_command[@]}" "$out_params" > "$output/params.sha256"
wc -c < "$out_params" | tr -d ' ' > "$output/params-bytes.txt"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/validate-started-at.txt"
set +e
"${time_command[@]}" ./bin/coimnet data validate --params "$out_params" --store "$store" \
  > "$output/validate.json" 2> "$output/validate.log"
status=$?
set -e
echo "exit=$status" >> "$output/validate.log"
date -u '+%Y-%m-%dT%H:%M:%SZ' > "$output/validate-finished-at.txt"
if [[ $status != 0 ]]; then
  echo "RESULT: data validate --params failed with exit $status" >&2
  exit "$status"
fi
echo 'RESULT: derive evidence recorded'
