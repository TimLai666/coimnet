#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 1 || $1 == --help || $1 == -h ]]; then
  echo 'Usage: scripts/verify.sh NEW_OUTPUT_DIRECTORY'
  echo 'Runs build, tests, race, vet, module verification, doctor, the delayed fixture and the lif-threshold fixture.'
  echo 'The output directory must not exist. Run from the repository root with Go on PATH.'
  if [[ $# == 1 && ($1 == --help || $1 == -h) ]]; then exit 0; fi
  exit 2
fi
mkdir "$1"
output=$(cd "$1" && pwd)
exec > >(tee "$output/validation.log") 2>&1

run() {
  printf 'COMMAND:'
  printf ' %q' "$@"
  printf '\n'
  "$@"
}

run go version
run go env GOOS GOARCH CGO_ENABLED
run date -u '+%Y-%m-%dT%H:%M:%SZ'
if command -v sha256sum >/dev/null 2>&1; then
  hash_command=(sha256sum)
else
  hash_command=(shasum -a 256)
fi
{
  find . -type d \( -name .git -o -name .codebase-memory -o -name data -o -name runs -o -name bin -o -name evidence \) -prune -o -type f \( -name '*.go' -o -name '*.sh' \) -print | sed 's|^\./||'
  printf '%s\n' go.mod go.sum
} | LC_ALL=C sort | while IFS= read -r source_file; do
  "${hash_command[@]}" "$source_file"
done > "$output/source.sha256"
unformatted=$(while IFS= read -r entry; do
  source_file=${entry#*  }
  if [[ $source_file == *.go ]]; then gofmt -l "$source_file"; fi
done < "$output/source.sha256")
if [[ -n "$unformatted" ]]; then
  printf 'Unformatted Go files:\n%s\n' "$unformatted"
  exit 1
fi
run go mod verify
run go build ./...
run go test -count=1 ./...
run go test -race -count=1 ./...
run go vet ./...
run go build -o "$output/coimnet" ./cmd/coimnet
"$output/coimnet" doctor > "$output/doctor.json"
"$output/coimnet" data sources > "$output/data-sources.json"
"$output/coimnet" examples run delayed > "$output/delayed.json"
"$output/coimnet" examples run lif-threshold > "$output/lif-threshold.json"
"$output/coimnet" train delayed --steps 80 --checkpoint "$output/first.json" > "$output/train-first.json"
"$output/coimnet" resume --checkpoint "$output/first.json" --steps 40 --out "$output/resumed.json" > "$output/train-resumed.json"
"$output/coimnet" train delayed --steps 120 --checkpoint "$output/continuous.json" > "$output/train-continuous.json"
run cmp "$output/resumed.json" "$output/continuous.json"
echo 'RESULT: all verification commands passed'
