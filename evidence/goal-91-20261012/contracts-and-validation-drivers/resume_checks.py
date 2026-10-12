import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path.cwd()
scratch = root / 'runs/.tmp/goal-91-20261012'
logs = scratch / 'logs'
frozen = json.loads((logs / 'source-frozen.json').read_bytes())
record_path = logs / 'full-commands.json'
old = record_path.read_bytes()
records = json.loads(old)
assert [x['name'] for x in records] == ['format', 'build', 'unit', 'race']
assert all(x['passed'] for x in records[:3]) and not records[3]['passed']
failed = records[3]
failed_log = (root / failed['log']).read_bytes()
assert hashlib.sha256(failed_log).hexdigest() == failed['log_sha256']
assert b'panic: test timed out after 20m0s' in failed_log
assert b'WARNING: DATA RACE' not in failed_log
with (logs / 'failed-attempts.json').open('x') as f:
    json.dump([failed], f, indent=2)
    f.write('\n')
records = records[:3]
env = os.environ.copy()
env.update(GOMODCACHE=str(scratch / 'go-mod-cache'),
           GOCACHE=str(scratch / 'go-build'),
           GOTMPDIR=str(scratch / 'go-tmp'), TMPDIR=str(scratch / 'go-tmp'),
           GOPROXY='off', GOTOOLCHAIN='local')
go = '/usr/local/go/bin/go'
checks = [
    ('race', [go, 'test', '-p', '2', '-race', '-count=1', '-json', '-timeout', '45m', './...']),
    ('vet', [go, 'vet', '-p', '2', './...']),
    ('modules', [go, 'mod', 'verify']),
    ('governance', [go, 'test', '-count=1', '-json', '-run', '^(TestGovernance|TestFormatIndex|TestOnlyNetworkPackagesImportHTTP|TestNoTelemetry|TestScanCommitScriptFlagsSecrets)', '.']),
]

def verify_sources():
    for path, sha in frozen.items():
        assert hashlib.sha256((root / path).read_bytes()).hexdigest() == sha, path

for name, command in checks:
    verify_sources()
    print('start ' + name, flush=True)
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    start = time.monotonic()
    log = logs / (name + '-repeat.log' if name == 'race' else name + '.log')
    with log.open('xb') as output:
        result = subprocess.run(command, cwd=root, env=env, stdout=output, stderr=subprocess.STDOUT)
    data = log.read_bytes()
    records.append({'name': name, 'command': command, 'cwd': str(root),
                    'started_at': started,
                    'ended_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    'elapsed_seconds': time.monotonic() - start,
                    'exit_code': result.returncode, 'passed': result.returncode == 0,
                    'log': str(log.relative_to(root)), 'log_bytes': len(data),
                    'log_sha256': hashlib.sha256(data).hexdigest()})
    assert record_path.read_bytes() == old, 'command record changed'
    new = (json.dumps(records, indent=2) + '\n').encode()
    record_path.write_bytes(new)
    old = new
    print('end ' + name + ' passed=' + str(result.returncode == 0), flush=True)
    if result.returncode:
        sys.exit(1)
verify_sources()
print('all complete; frozen sources=' + str(len(frozen)), flush=True)
