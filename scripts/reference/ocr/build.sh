#!/usr/bin/env bash
# Assemble and build the reference driver: unmodified copies of
# examples/realocr/{main.go,model.go} (realocr's main renamed realocrMain)
# plus driver/driver.go. Build dir is a dot directory so `go ./...` skips it.
set -euo pipefail
root="$(cd "$(dirname "$0")/../../.." && pwd)"
build="$root/runs/.tmp/ref-ocr/build"
rm -rf "$build"
mkdir -p "$build"
sed 's/^func main() {/func realocrMain() {/' "$root/examples/realocr/main.go" > "$build/realocr_main.go"
cp "$root/examples/realocr/model.go" "$build/realocr_model.go"
cp "$root/scripts/reference/ocr/driver/driver.go" "$build/driver.go"
cd "$root"
go build -tags refocr -o "$root/runs/.tmp/ref-ocr/refdriver" "./runs/.tmp/ref-ocr/build"
sha256sum "$root/examples/realocr/main.go" "$root/examples/realocr/model.go" "$root/scripts/reference/ocr/driver/driver.go" > "$root/runs/.tmp/ref-ocr/build-inputs.sha256"
echo "built $root/runs/.tmp/ref-ocr/refdriver"
