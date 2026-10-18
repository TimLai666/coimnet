#!/usr/bin/env bash
# Conventional-regime context runs (one core).
root="$(cd "$(dirname "$0")/../../.." && pwd)"; B="$root/runs/.tmp/ref-ocr"; cd "$root"
for m in small mlp512 cnn; do
  python3 -I scripts/reference/ocr/torch_regularized.py --dump "$B/proto-s24" --model $m --out "$B/reg-$m" > "$B/reg-$m.stdout" 2> "$B/reg-$m.stderr"
done
