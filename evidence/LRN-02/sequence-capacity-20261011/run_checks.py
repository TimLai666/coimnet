import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path.cwd()
scratch = root / 'runs/.tmp/recompute-capacity-20261011'
logs = scratch / 'logs'
logs.mkdir(exist_ok=True)
env = os.environ.copy()
env.update(GOCACHE=str(scratch / 'gocache'), GOTMPDIR=str(scratch), TMPDIR=str(scratch), GOPROXY='off', GOSUMDB='off')
plans = {
    'red': [('red', ['go', 'test', '-json', '-count=1', '-timeout=30m', './dynamics', './learning', '-run', 'ForwardRecompute|RecomputeCapacity'])],
    'green': [('green', ['go', 'test', '-json', '-count=1', '-timeout=30m', './dynamics', './learning', '-run', 'Recompute'])],
    'full': [
        ('modules', ['go', 'mod', 'verify']),
        ('build', ['go', 'build', './...']),
        ('unit', ['go', 'test', '-json', '-p', '2', '-count=1', '-timeout=30m', './...']),
        ('race', ['go', 'test', '-race', '-json', '-p', '2', '-count=1', '-timeout=30m', './...']),
        ('vet', ['go', 'vet', './...']),
        ('dependencies', ['go', 'list', '-deps', './...']),
        ('target-flow', ['bash', 'scripts/check-target-flow.sh']),
        ('cli-build', ['go', 'build', '-o', str(scratch / 'coimnet'), './cmd/coimnet']),
    ],
}
stage = sys.argv[1]
records = []
for label, cmd in plans[stage]:
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    tick = time.monotonic()
    log = logs / (label + '.log')
    with log.open('xb') as output:
        proc = subprocess.run(cmd, env=env, stdout=output, stderr=subprocess.STDOUT)
    record = dict(command=cmd, started_at=started, elapsed_seconds=round(time.monotonic()-tick, 3), exit_code=proc.returncode, log=log.name, sha256=hashlib.sha256(log.read_bytes()).hexdigest())
    records.append(record)
    (logs / (stage + '-commands.json')).write_text(json.dumps(records, indent=2) + '\n')
    print(json.dumps(record), flush=True)
    if proc.returncode:
        sys.exit(proc.returncode)
