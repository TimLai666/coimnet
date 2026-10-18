#!/usr/bin/env bash
# Reproduces the spiking/mixed/vector/modulation/plasticity reference audit.
# Writes only under runs/.tmp/ref-spiking/.
set -euo pipefail
root="$(cd "$(dirname "$0")/../../.." && pwd)"
tmp="$root/runs/.tmp/ref-spiking"
here="$root/scripts/reference/spiking"
mkdir -p "$tmp"
cd "$root"
export GOFLAGS="${GOFLAGS:--p=2}"
go run ./scripts/reference/spiking/driver -out "$tmp/cases.json" -n "${N:-40}" -seed "${SEED:-1}"
python3 "$here/reference.py" "$tmp/cases.json" | grep -v '^  '
go run ./scripts/reference/spiking/driver2 -out "$tmp/ind.json" -n "${N2:-20}"
python3 "$here/reference2.py" "$tmp/ind.json"
go run ./scripts/reference/spiking/driver3
go run ./scripts/reference/spiking/driver4 "$tmp/ctrl.json"
python3 "$here/reference4.py" "$tmp/ctrl.json"
