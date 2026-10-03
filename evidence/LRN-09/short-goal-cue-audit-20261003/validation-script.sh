#!/bin/bash
set -euo pipefail
cd /Users/timlai/Developer/coimnet
out=/private/tmp/coimnet-short-goal-cue-audit-20261003
python3 - <<'MANIFEST'
from pathlib import Path
import hashlib, subprocess
root=Path('.')
paths=[s for s in subprocess.check_output(['rg','--files','--hidden','-g','!.git','-g','!.codebase-memory','-g','!data','-g','!runs','-g','!bin','-g','!evidence'],text=True).splitlines() if s.endswith(('.go','.sh'))]
paths+=['go.mod','go.sum']
with Path('/private/tmp/coimnet-short-goal-cue-audit-20261003/source.sha256').open('x') as f:
 for s in sorted(set(paths)): f.write(hashlib.sha256(Path(s).read_bytes()).hexdigest()+'  '+s+'\n')
print('source_files',len(set(paths)))
MANIFEST
python3 - <<'FORMAT' > "$out/format.log"
from pathlib import Path
import subprocess
paths=[s.split('  ',1)[1] for s in Path('/private/tmp/coimnet-short-goal-cue-audit-20261003/source.sha256').read_text().splitlines() if s.endswith('.go')]
result=subprocess.check_output(['gofmt','-l',*paths],text=True)
assert not result,result
print('PASS:',len(paths),'Go sources formatted')
FORMAT
go version > "$out/environment.log"
go env GOOS GOARCH CGO_ENABLED >> "$out/environment.log"
go build ./... > "$out/build.log" 2>&1
printf 'build PASS\n'
go test -p 2 -timeout 30m ./... > "$out/test.log" 2>&1
printf 'normal PASS\n'
go test -race -p 2 -timeout 30m ./... > "$out/race.log" 2>&1
printf 'race PASS\n'
go vet ./... > "$out/vet.log" 2>&1
go mod verify > "$out/mod-verify.log" 2>&1
go mod tidy -diff > "$out/mod-tidy.log" 2>&1
(cd docs/handoff && shasum -a 256 -c SHA256SUMS.txt) > "$out/handoff-checksums.log" 2>&1
printf 'vet, dependency and handoff PASS\n'
