#!/usr/bin/env bash
# Plain PyTorch MLP baselines on the frozen-protocol data/order (one core).
set -u
root="$(cd "$(dirname "$0")/../../.." && pwd)"; B="$root/runs/.tmp/ref-ocr"; cd "$root"
for spec in "28,24 tanh" "29 tanh" "128 tanh" "128,128 tanh" "512 relu" "128 relu"; do
  set -- $spec; name="mlp-${1//,/x}-$2"
  [ -s "$B/$name/result.json" ] && continue
  python3 -I scripts/reference/ocr/torch_mlp.py --data-dir data/TSK-11/kmnist --dump "$B/proto-s24" --layers "$1" --act "$2" --out "$B/$name" > "$B/$name.stdout" 2> "$B/$name.stderr"
done
