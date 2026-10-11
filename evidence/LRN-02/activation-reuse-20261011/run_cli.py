import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

root = Path.cwd()
scratch = root / 'runs/.tmp/recompute-activation-20261011'
out = scratch / 'cli'
out.mkdir()
env = os.environ.copy()
env.update(GOCACHE=str(scratch / 'gocache'), GOTMPDIR=str(scratch / 'gotmp'), TMPDIR=str(scratch / 'tmp'), GOPROXY='off', GOSUMDB='off')
binary = str(scratch / 'coimnet')
plans = [
    ('doctor', [binary, 'doctor']),
    ('data-sources', [binary, 'data', 'sources']),
    ('delayed', [binary, 'examples', 'run', 'delayed']),
    ('lif-threshold', [binary, 'examples', 'run', 'lif-threshold']),
    ('train-first', [binary, 'train', 'delayed', '--steps', '80', '--checkpoint', str(out / 'first.json')]),
    ('train-resumed', [binary, 'resume', '--checkpoint', str(out / 'first.json'), '--steps', '40', '--out', str(out / 'resumed.json')]),
    ('train-continuous', [binary, 'train', 'delayed', '--steps', '120', '--checkpoint', str(out / 'continuous.json')]),
    ('continuous-api', ['go', 'doc', './dynamics.Continuous.ForwardRecompute']),
    ('lif-api', ['go', 'doc', './dynamics.LIF.ForwardRecompute']),
]
records = []
for label, cmd in plans:
    tick = time.monotonic()
    started = datetime.datetime.now(datetime.timezone.utc).isoformat()
    log = out / (label + '.log')
    with log.open('xb') as stream:
        r = subprocess.run(cmd, env=env, stdout=stream, stderr=subprocess.STDOUT)
    row = dict(command=cmd, started_at=started, elapsed_seconds=round(time.monotonic()-tick, 3), exit_code=r.returncode, log=log.name, sha256=hashlib.sha256(log.read_bytes()).hexdigest())
    records.append(row)
    (out / 'commands.json').write_text(json.dumps(records, indent=2)+'\n')
    print(json.dumps(row), flush=True)
    assert r.returncode == 0
assert (out / 'continuous.json').read_bytes() == (out / 'resumed.json').read_bytes()
(out / 'resume-comparison.json').write_text(json.dumps(dict(equal=True, sha256=hashlib.sha256((out / 'continuous.json').read_bytes()).hexdigest()), indent=2)+'\n')
print('PASS: CLI flows, SDK docs and cross-process resume equality')
