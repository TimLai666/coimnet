#!/usr/bin/env bash
set -euo pipefail

# Records the LRN-02 memory comparison of ticket 25: the maximum resident set
# size of one StepFrom with the full reverse history against one with
# Recompute{SegmentSteps: 16}, as a coimnet-recompute-rss/v1 document.

usage() {
  cat <<'USAGE'
Usage: scripts/recompute-evidence.sh [--scale small|full] [--out FILE]
  --scale small|full  network size. full (the default) uses 50000 continuous
                      and 20000 LIF nodes; small uses 512 nodes for both.
  --out FILE          result document; default evidence/LRN-02/rss.json under
                      the repository root. A relative FILE is resolved from
                      the current directory. An existing file is replaced.

Builds the learning test binary with go test -c into a mktemp directory under
.recompute-work at the repository root, then runs TestRecomputeRSSHelper four
times, each in its own process under /usr/bin/time -l (macOS) or -v (Linux):
the continuous and the LIF core, each with the full reverse history and with
Recompute{SegmentSteps: 16}, over T = 256 steps with 8 in-edges per node.
The document keeps every run's JSON line plus max_rss_bytes, and per core the
two maxima, their difference (saving_bytes) and whether both modes produced
the same parameters. A failing run stops the script with a nonzero exit and
prints that run's stdout and stderr; no document is written then. The mktemp
directory, and .recompute-work once empty, are removed on exit.
Requires Go on PATH and /usr/bin/time.
USAGE
}

scale=full
out_file=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --scale|--out)
      if [[ $# -lt 2 ]]; then
        printf 'recompute-evidence: %s needs a value\n' "$1" >&2
        exit 2
      fi
      case $1 in
        --scale) scale=$2 ;;
        --out) out_file=$2 ;;
      esac
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'recompute-evidence: unknown option %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done
if [[ $scale != small && $scale != full ]]; then
  printf 'recompute-evidence: --scale must be small or full, not %s\n' "$scale" >&2
  exit 2
fi
if [[ ! -x /usr/bin/time ]]; then
  echo 'recompute-evidence: /usr/bin/time is required' >&2
  exit 2
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -z $out_file ]]; then
  out_file="$repo_root/evidence/LRN-02/rss.json"
elif [[ $out_file != /* ]]; then
  out_file="$PWD/$out_file"
fi
mkdir -p "$(dirname "$out_file")"
cd "$repo_root"

if [[ $(uname -s) == Darwin ]]; then
  time_command=(/usr/bin/time -l)
  # macOS reports the maximum resident set size in bytes.
  max_rss_bytes() { awk '/maximum resident set size/ {print $1}' "$1"; }
else
  time_command=(/usr/bin/time -v)
  # GNU time reports it in kbytes.
  max_rss_bytes() {
    local kb
    kb=$(awk -F': *' '/Maximum resident set size/ {print $2}' "$1")
    if [[ $kb =~ ^[0-9]+$ ]]; then echo $((10#$kb * 1024)); fi
  }
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

work_root="$repo_root/.recompute-work"
work=""
cleanup() {
  if [[ -n $work ]]; then rm -rf "$work"; fi
  rmdir "$work_root" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -p "$work_root"
work=$(mktemp -d "$work_root/run.XXXXXX")
binary="$work/learning.test"
go test -c -o "$binary" ./learning/

# run CORE MODE runs the helper once and sets line to its JSON line, rss to its
# maximum resident set size in bytes and sha to its parameters_sha256. A run
# that fails, prints no line for CORE and MODE or has no RSS reading ends the
# script after printing its output.
run() {
  local stdout="$work/$1-$2.stdout" stderr="$work/$1-$2.stderr" status=0
  "${time_command[@]}" env COIMNET_RECOMPUTE_RSS="$1:$2" COIMNET_RECOMPUTE_RSS_SCALE="$scale" \
    "$binary" -test.run '^TestRecomputeRSSHelper$' -test.count=1 > "$stdout" 2> "$stderr" || status=$?
  line=$(grep '^{"core":' "$stdout" || true)
  sha=$(sed -n 's/.*"parameters_sha256":"\([0-9a-f]*\)".*/\1/p' <<<"$line")
  rss=$(max_rss_bytes "$stderr")
  if [[ $status -eq 0 && $line == "{\"core\":\"$1\",\"mode\":\"$2\","*'}' && $line != *$'\n'* &&
    $sha =~ ^[0-9a-f]{64}$ && $rss =~ ^[0-9]+$ ]]; then
    printf '%s:%s max_rss_bytes=%s parameters_sha256=%s\n' "$1" "$2" "$rss" "$sha"
    return 0
  fi
  printf 'recompute-evidence: %s:%s failed with exit %d\n--- stdout\n' "$1" "$2" "$status" >&2
  cat "$stdout" >&2
  printf -- '--- stderr\n' >&2
  cat "$stderr" >&2
  if [[ $status -eq 0 ]]; then status=1; fi
  exit "$status"
}

go_version=$(go env GOVERSION)
goos=$(go env GOOS)
goarch=$(go env GOARCH)
uptime_text=$(uptime)
runs_json=""
cores_json=""
separator=""
for core in continuous lif; do
  run "$core" full
  full_run="${line%\}},\"max_rss_bytes\":$rss}"
  full_rss=$rss
  full_sha=$sha
  run "$core" recompute
  same=false
  if [[ $sha == "$full_sha" ]]; then same=true; fi
  runs_json+="$separator    $full_run,"$'\n'"    ${line%\}},\"max_rss_bytes\":$rss}"
  cores_json+="$separator$(printf '    {"core":"%s","full_max_rss_bytes":%d,"recompute_max_rss_bytes":%d,"saving_bytes":%d,"same_parameters":%s}' \
    "$core" "$full_rss" "$rss" $((full_rss - rss)) "$same")"
  separator=$',\n'
done

printf '{\n  "schema_version": "coimnet-recompute-rss/v1",\n  "scale": %s,\n  "generated_at": %s,\n  "uptime": %s,\n  "go_version": %s,\n  "goos": %s,\n  "goarch": %s,\n  "runs": [\n%s\n  ],\n  "cores": [\n%s\n  ]\n}\n' \
  "$(json_string "$scale")" \
  "$(json_string "$(date -u '+%Y-%m-%dT%H:%M:%SZ')")" \
  "$(json_string "$uptime_text")" \
  "$(json_string "$go_version")" \
  "$(json_string "$goos")" \
  "$(json_string "$goarch")" \
  "$runs_json" \
  "$cores_json" > "$out_file"

printf 'wrote %s\n' "$out_file"
echo 'RESULT: recompute RSS evidence recorded'
