import gzip, hashlib, json
from pathlib import Path

root = Path('/Users/timlai/Developer/coimnet')
source = root / 'runs/.tmp/goal-91-next/runs/.tmp/final-validation-complete'
target = root / 'evidence/goal-91-20261012/full-validation/complete'

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

commands = json.loads((source / 'commands.json').read_text())
assert [c['name'] for c in commands] == ['format', 'build', 'unit', 'race', 'vet', 'modules', 'governance']
assert all(c['passed'] and c['exit_code'] == 0 for c in commands)
frozen = json.loads((source / 'source-frozen.json').read_text())
assert len(frozen) == 708
for path, expected in frozen.items():
    assert sha((root / path).read_bytes()) == expected, path
    assert sha((root / 'runs/.tmp/goal-91-next' / path).read_bytes()) == expected, path
assert not target.exists()
target.mkdir()
preserved = []
counts = {}
for entry in sorted(source.iterdir()):
    assert entry.is_file() and not entry.is_symlink(), entry
    raw = entry.read_bytes()
    name = entry.name + '.gz' if entry.suffix == '.log' else entry.name
    payload = gzip.compress(raw, mtime=0) if entry.suffix == '.log' else raw
    with (target / name).open('xb') as f:
        f.write(payload)
    assert (gzip.decompress(payload) if entry.suffix == '.log' else payload) == raw
    preserved.append({'source': str(entry), 'artifact': str((target / name).relative_to(root)), 'source_bytes': len(raw), 'source_sha256': sha(raw), 'artifact_bytes': len(payload), 'artifact_sha256': sha(payload), 'gzip': entry.suffix == '.log'})
for c in commands:
    raw = (source / c['log']).read_bytes()
    assert sha(raw) == c['log_sha256'] and len(raw) == c['log_bytes']
    if c['name'] not in ('unit', 'race', 'governance'):
        continue
    events = []
    for line in raw.decode().splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    failed = [e for e in events if e.get('Action') == 'fail']
    assert not failed, failed
    assert b'WARNING: DATA RACE' not in raw
    top = lambda e: bool(e.get('Test')) and '/' not in e['Test']
    counts[c['name']] = {
        'packages_passed': sum(e.get('Action') == 'pass' and not e.get('Test') for e in events),
        'packages_skipped': sum(e.get('Action') == 'skip' and not e.get('Test') for e in events),
        'top_level_tests_passed': sum(e.get('Action') == 'pass' and top(e) for e in events),
        'top_level_tests_skipped': sum(e.get('Action') == 'skip' and top(e) for e in events),
        'skipped_tests': [{'package': e['Package'], 'test': e.get('Test')} for e in events if e.get('Action') == 'skip'],
        'failed': 0,
    }
verification = {
    'schema_version': 'coimnet-integrated-validation/v1',
    'requirement': ['TSK-11', 'OPS-05'],
    'reproduction_command': [{'name': c['name'], 'argv': c['command'], 'environment': 'environment.json'} for c in commands],
    'environment': json.loads((source / 'environment.json').read_text()),
    'observed_result': {'all_seven_checks_passed': True, 'source_files': len(frozen), 'main_and_isolated_copy_match': True, 'counts': counts, 'elapsed_seconds': {c['name']: c['elapsed_seconds'] for c in commands}},
    'test_log': [str((target / (c['log'] + '.gz')).relative_to(root)) for c in commands],
    'input_fingerprints': {'base_commit': 'eb8492e477c365bfd374d3c2012e955db6e12060', 'source_manifest': str((target / 'source-frozen.json').relative_to(root)), 'source_manifest_sha256': sha((target / 'source-frozen.json').read_bytes())},
    'limitations': ['Native APFS functional validation on Apple M3. GOMAXPROCS=2 and -p 2 bound concurrency; cases and assertions are unchanged. No performance claim.', 'Hardware-specific helper and unavailable profile skips are recorded individually. Actual Metal trainer and new-process bundle verification is separately recorded in device-training-20261012.', 'Earlier timeout, repository-boundary failures and comment-only routing incident remain preserved under failed-attempts.'],
}
for name, obj in [('preservation.json', {'files': preserved}), ('verification.json', verification)]:
    with (target / name).open('x') as f:
        json.dump(obj, f, indent=2, ensure_ascii=False)
        f.write('\n')
print(json.dumps({'counts': counts, 'source_files': len(frozen), 'checks': {c['name']: c['elapsed_seconds'] for c in commands}}, ensure_ascii=False))
