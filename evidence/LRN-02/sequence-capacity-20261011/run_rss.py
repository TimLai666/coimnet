import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time

root = Path.cwd()
scratch = root / 'runs/.tmp/recompute-capacity-20261011'
logs = scratch / 'logs/rss-native'
logs.mkdir()
env = os.environ.copy()
env.update(GOCACHE=str(scratch / 'gocache'), GOTMPDIR=str(scratch), TMPDIR=str(scratch), GOPROXY='off', GOSUMDB='off')
binary = scratch / 'learning.test'
cmd = ['go', 'test', '-c', '-o', str(binary), './learning']
with (logs / 'rss-build.log').open('xb') as out:
    subprocess.run(cmd, env=env, stdout=out, stderr=subprocess.STDOUT, check=True)
runs = []
for core in ['continuous', 'lif']:
    for mode in ['full', 'recompute']:
        run_env = env.copy()
        run_env.update(COIMNET_RECOMPUTE_RSS=core+':'+mode, COIMNET_RECOMPUTE_RSS_SCALE='full')
        label = 'rss-'+core+'-'+mode
        cmd = ['/usr/bin/time', '-l', str(binary), '-test.run', '^TestRecomputeRSSHelper$', '-test.count=1', '-test.timeout=30m']
        tick = time.monotonic()
        stdout_path, stderr_path = logs / (label+'.stdout'), logs / (label+'.stderr')
        with stdout_path.open('xb') as out, stderr_path.open('xb') as err:
            subprocess.run(cmd, env=run_env, stdout=out, stderr=err, check=True)
        data = [json.loads(line) for line in stdout_path.read_text().splitlines() if line.startswith('{"core":')]
        assert len(data)==1
        row = data[0]
        rss = re.search(r'(\d+)\s+maximum resident set size', stderr_path.read_text())
        assert rss
        row.update(max_rss_bytes=int(rss.group(1)), elapsed_seconds=round(time.monotonic()-tick,3), command=cmd, stdout_sha256=hashlib.sha256(stdout_path.read_bytes()).hexdigest(), stderr_sha256=hashlib.sha256(stderr_path.read_bytes()).hexdigest())
        runs.append(row)
        with (logs / 'rss-runs.json').open('w') as out:
            json.dump(runs,out,indent=2);out.write('\n')
        print(json.dumps(row),flush=True)
for core in ['continuous','lif']:
    full, recompute = [row for row in runs if row['core']==core]
    assert full['parameters_sha256']==recompute['parameters_sha256']
    assert full['gradient_norm']==recompute['gradient_norm']
    assert full['update_norm']==recompute['update_norm']
    assert recompute['max_rss_bytes']<full['max_rss_bytes']
print('PASS: both parameter fingerprints and gradient/update norms match; recompute uses less RSS')
