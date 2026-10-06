"""Check the frozen Go sources, tests and pre-existing repository inputs."""
import argparse
import hashlib
import json
import pathlib
import subprocess

argparse.ArgumentParser(description=__doc__).parse_args()
out = pathlib.Path(__file__).resolve().parent
root = out.parents[2]
counts = {}
for name in ['source-manifest.json', 'frozen-tests.json', 'protected-manifest.json']:
    files = json.loads((out / name).read_text())['files']
    mismatches = [p for p, expected in files.items()
                  if not (root / p).is_file()
                  or hashlib.sha256((root / p).read_bytes()).hexdigest() != expected]
    assert not mismatches, (name, mismatches)
    counts[name] = len(files)
source = json.loads((out / 'source-manifest.json').read_text())
paths = sorted(set(subprocess.check_output(
    ['git', 'ls-files', '-co', '--exclude-standard', '--', '*.go'], cwd=root).decode().splitlines()))
assert paths == sorted(source['files']), 'Go source file set changed'
digest = hashlib.sha256(json.dumps(source['files'], sort_keys=True,
                                  separators=(',', ':')).encode()).hexdigest()
assert digest == source['source_sha256']
print(json.dumps({'status': 'passed', 'checked': counts, 'source_sha256': digest}, indent=2))
