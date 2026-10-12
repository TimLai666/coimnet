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
logs.mkdir(exist_ok=False)
env = os.environ.copy()
env.update({
    'GOMODCACHE': str(scratch / 'go-mod-cache'),
    'GOCACHE': str(scratch / 'go-build'),
    'GOTMPDIR': str(scratch / 'go-tmp'),
    'TMPDIR': str(scratch / 'go-tmp'),
    'GOPROXY': 'off',
    'GOTOOLCHAIN': 'local',
})
paths = subprocess.check_output(['git', 'ls-files', '-c', '-o', '--exclude-standard'], text=True).splitlines()
sources = sorted({p for p in paths if p.endswith('.go')} | {'go.mod', 'go.sum'})
fingerprints = {p: hashlib.sha256((root / p).read_bytes()).hexdigest() for p in sources}
with (logs / 'source-frozen.json').open('x') as f:
    json.dump(fingerprints, f, indent=2)
    f.write('\n')

go = '/usr/local/go/bin/go'
checks = [
    ('format', ['/usr/local/go/bin/gofmt', '-l', *[p for p in sources if p.endswith('.go')]]),
    ('build', [go, 'build', '-p', '2', './...']),
    ('unit', [go, 'test', '-p', '1', '-count=1', '-json', '-timeout', '20m', './...']),
    ('race', [go, 'test', '-p', '1', '-race', '-count=1', '-json', '-timeout', '20m', './...']),
    ('vet', [go, 'vet', '-p', '2', './...']),
    ('modules', [go, 'mod', 'verify']),
    ('governance', [go, 'test', '-count=1', '-json', '-run', '^(TestGovernance|TestFormatIndex|TestOnlyNetworkPackagesImportHTTP|TestNoTelemetry|TestScanCommitScriptFlagsSecrets)', '.']),
]
records = []
for name, command in checks:
    for path, digest in fingerprints.items():
        if hashlib.sha256((root / path).read_bytes()).hexdigest() != digest:
            raise RuntimeError('frozen source changed: ' + path)
    print('start ' + name, flush=True)
    start_time = datetime.datetime.now(datetime.timezone.utc).isoformat()
    start = time.monotonic()
    log = logs / (name + '.log')
    with log.open('xb') as output:
        result = subprocess.run(command, cwd=root, env=env, stdout=output, stderr=subprocess.STDOUT)
    data = log.read_bytes()
    passed = result.returncode == 0 and (name != 'format' or not data.strip())
    records.append({
        'name': name, 'command': command, 'cwd': str(root),
        'started_at': start_time, 'ended_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'elapsed_seconds': time.monotonic() - start, 'exit_code': result.returncode,
        'passed': passed, 'log': str(log.relative_to(root)),
        'log_bytes': len(data), 'log_sha256': hashlib.sha256(data).hexdigest(),
    })
    record_path = logs / 'full-commands.json'
    previous = record_path.read_bytes() if record_path.exists() else None
    next_data = (json.dumps(records, indent=2) + '\n').encode()
    if previous is None:
        with record_path.open('xb') as f:
            f.write(next_data)
    else:
        if record_path.read_bytes() != previous:
            raise RuntimeError('command record changed')
        record_path.write_bytes(next_data)
    print('end ' + name + ' passed=' + str(passed) + ' exit=' + str(result.returncode), flush=True)
    if not passed:
        sys.exit(1)
for path, digest in fingerprints.items():
    if hashlib.sha256((root / path).read_bytes()).hexdigest() != digest:
        raise RuntimeError('source changed after validation: ' + path)
print('all complete; frozen sources=' + str(len(fingerprints)), flush=True)
