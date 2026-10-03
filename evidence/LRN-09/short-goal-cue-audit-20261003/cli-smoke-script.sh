#!/bin/bash
set -euo pipefail
cd /Users/timlai/Developer/coimnet
out=/private/tmp/coimnet-short-goal-cue-audit-20261003
mkdir "$out/smoke"
bin="$out/smoke/coimnet"
go build -o "$bin" ./cmd/coimnet
"$bin" doctor > "$out/smoke/doctor.json"
"$bin" data sources > "$out/smoke/data-sources.json"
"$bin" examples run delayed > "$out/smoke/delayed.json"
"$bin" examples run lif-threshold > "$out/smoke/lif-threshold.json"
"$bin" train delayed --steps 80 --checkpoint "$out/smoke/first.json" > "$out/smoke/train-first.json"
"$bin" resume --checkpoint "$out/smoke/first.json" --steps 40 --out "$out/smoke/resumed.json" > "$out/smoke/train-resumed.json"
"$bin" train delayed --steps 120 --checkpoint "$out/smoke/continuous.json" > "$out/smoke/train-continuous.json"
cmp "$out/smoke/resumed.json" "$out/smoke/continuous.json"
printf 'PASS: doctor, sources, delayed, lif-threshold and new-process 80+40/120 resume\n'
