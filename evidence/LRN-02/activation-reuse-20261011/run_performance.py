"""Compare frozen baseline/improved Go test binaries in independent processes."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import statistics
import subprocess
import time

root = Path.cwd()
scratch = root / 'runs/.tmp/recompute-activation-20261011'
out = scratch / 'performance'
out.mkdir()
env = os.environ.copy()
env['GOMAXPROCS'] = '1'
pattern = re.compile(r'^(BenchmarkRecomputeActivation/\S+)\s+\d+\s+([0-9.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$', re.M)
records = []
for pair in range(8):
    # Pair zero warms both executables and is retained, but excluded from medians.
    modes = ['baseline', 'improved'] if pair % 2 == 0 else ['improved', 'baseline']
    for mode in modes:
        binary = scratch / 'bin' / ('dynamics-' + mode + '-candidate5.test')
        cmd = ['/usr/bin/time', '-l', str(binary), '-test.run', '^$',
               '-test.bench', '^BenchmarkRecomputeActivation$',
               '-test.benchtime=3x', '-test.count=1', '-test.cpu=1']
        started = datetime.datetime.now(datetime.timezone.utc).isoformat()
        load = os.getloadavg()
        tick = time.monotonic()
        proc = subprocess.run(cmd, env=env, capture_output=True)
        stem = str(pair) + '-' + mode
        (out / (stem + '.stdout.log')).write_bytes(proc.stdout)
        (out / (stem + '.stderr.log')).write_bytes(proc.stderr)
        parsed = {name: dict(ns_per_op=float(ns), bytes_per_op=int(size), allocs_per_op=int(allocs))
                  for name, ns, size, allocs in pattern.findall(proc.stdout.decode())}
        rss = re.search(r'(\d+)\s+maximum resident set size', proc.stderr.decode())
        row = dict(pair=pair, warmup=pair == 0, mode=mode, command=cmd,
                   started_at=started, elapsed_seconds=round(time.monotonic()-tick, 3),
                   load_average=load, exit_code=proc.returncode,
                   binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                   stdout_sha256=hashlib.sha256(proc.stdout).hexdigest(),
                   stderr_sha256=hashlib.sha256(proc.stderr).hexdigest(),
                   max_rss_bytes=int(rss.group(1)) if rss else None, benchmarks=parsed)
        records.append(row)
        (out / 'runs.json').write_text(json.dumps(records, indent=2) + '\n')
        print(json.dumps({k: v for k, v in row.items() if k != 'benchmarks'}), flush=True)
        assert proc.returncode == 0 and rss and len(parsed) == 24, stem

comparison = {}
for name in records[0]['benchmarks']:
    samples = {mode: [r['benchmarks'][name] for r in records if r['mode'] == mode and not r['warmup']]
               for mode in ('baseline', 'improved')}
    medians = {mode: {metric: statistics.median(s[metric] for s in samples[mode])
                      for metric in ('ns_per_op', 'bytes_per_op', 'allocs_per_op')}
               for mode in samples}
    improvement = 1 - medians['improved']['ns_per_op'] / medians['baseline']['ns_per_op']
    comparison[name] = dict(samples_per_mode=7, median=medians,
                            time_reduction_fraction=improvement,
                            required_gate='-deg64-' in name,
                            passed=improvement >= 0.15 if '-deg64-' in name else None)
summary = dict(threshold_fraction=0.15, alternating_independent_pairs=7,
               warmup_pairs_excluded=1, comparison=comparison,
               passed=all(c['passed'] for c in comparison.values() if c['required_gate']),
               limitations=['Synthetic CPU fixture; not a whole-connectome timing guarantee.',
                            'Shared desktop load is recorded, not controlled.',
                            'RSS covers each whole benchmark process, including fixtures and Go GC.'])
(out / 'comparison.json').write_text(json.dumps(summary, indent=2) + '\n')
print(json.dumps(summary), flush=True)
assert summary['passed'], 'The predeclared dense-graph improvement gate failed.'
