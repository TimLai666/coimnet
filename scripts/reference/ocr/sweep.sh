#!/usr/bin/env bash
# Framework capacity sweep (frozen-protocol data/order/lr/epochs, seed 20260924).
# Runs sequentially on one core: GOMAXPROCS=1.
set -u
root="$(cd "$(dirname "$0")/../../.." && pwd)"
B="$root/runs/.tmp/ref-ocr"
cd "$root"
run() { name=$1; shift; [ -s "$B/sweep-$name/result.json" ] && return; GOMAXPROCS=1 "$B/refdriver" --seed 20260924 --eval-every 5 --out "$B/sweep-$name" "$@" > "$B/sweep-$name.stdout" 2> "$B/sweep-$name.stderr"; }
run h128 --mode recognizer --hidden 128
run h24-readall --mode custom --hidden 24 --readout all
run in128-h128 --mode custom --input-nodes 128 --hidden 128
run h512-e5 --mode recognizer --hidden 512 --epochs 5   # 20 epochs would take ~2 h on this shared box
