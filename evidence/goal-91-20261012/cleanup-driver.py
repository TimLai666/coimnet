import datetime, gzip, hashlib, json, os, stat
from pathlib import Path

root = Path('/Users/timlai/Developer/coimnet')
goal = root / 'evidence/goal-91-20261012'
scratch = root / 'runs/.tmp/goal-91-20261012'
owned_dirs = [scratch, root / 'runs/.tmp/goal-91-next', root / 'runs/.tmp/61']

def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()

def write_new(path, obj):
    with path.open('x') as f:
        json.dump(obj, f, ensure_ascii=False, indent=2)
        f.write('\n')

full = json.loads((goal / 'full-validation/complete/verification.json').read_text())
assert full['observed_result']['all_seven_checks_passed']
metadata = json.loads((goal / 'metadata-governance-verification.json').read_text())
assert metadata['observed_result']['exit_code'] == 0
assert len(metadata['observed_result']['passed_top_level_tests']) == 13
frozen = json.loads((goal / 'full-validation/complete/source-frozen.json').read_text())
for path, expected in frozen.items():
    assert digest(root / path) == expected, path
assert len(frozen) == 708
status = json.loads((root / 'docs/requirements-status.json').read_text())
assert len(status['requirements']) == 91
assert all(row['status'] == 'passed' for row in status['requirements'])
assert sorted(p.name for p in (root / 'runs/.tmp').iterdir()) == ['61', 'goal-91-20261012', 'goal-91-next']

data_receipt = json.loads((scratch / 'native-data-copy.json').read_text())
media_receipt = json.loads((scratch / 'native-media-copy.json').read_text())
data_records = [{'path': str(Path(data_receipt['target']) / row['path']), 'source': str(Path(data_receipt['source']) / row['path']), 'bytes': row['bytes'], 'sha256': row['sha256']} for row in data_receipt['files']]
data_records += [{'path': row['target'], 'source': row['source'], 'bytes': row['bytes'], 'sha256': row['sha256']} for row in media_receipt['files']]
assert len(data_records) == 25
assert len({row['path'] for row in data_records}) == 25
data_dir = root / 'data/TSK-11'
actual = {str(p) for p in data_dir.rglob('*') if p.is_file() or p.is_symlink()}
assert actual == {row['path'] for row in data_records}
for row in data_records:
    path = Path(row['path'])
    assert path.is_relative_to(data_dir) and not path.is_symlink()
    assert path.stat().st_size == row['bytes'] and digest(path) == row['sha256'], path
    source = Path(row['source'])
    assert str(source).startswith('/Volumes/SSD/Developer/coimnet/data/TSK-11/')
    assert source.is_file() and not source.is_symlink(), source
    assert source.stat().st_size == row['bytes'] and digest(source) == row['sha256'], source
print('All 25 copied inputs and their protected SMB originals match.', flush=True)

entries = []
directories = []
for base in owned_dirs:
    assert base.is_dir() and not base.is_symlink(), base
    for current, dirs, files in os.walk(base, followlinks=False):
        parent = Path(current)
        directories.append(str(parent))
        for name in list(dirs):
            p = parent / name
            if p.is_symlink():
                assert base == scratch and p.is_relative_to(scratch / 'go-mod-cache'), p
                entries.append({'path': str(p), 'kind': 'symlink', 'target': os.readlink(p)})
                dirs.remove(name)
        for name in files:
            p = parent / name
            mode = p.lstat().st_mode
            if stat.S_ISLNK(mode):
                assert base == scratch and p.is_relative_to(scratch / 'go-mod-cache'), p
                entries.append({'path': str(p), 'kind': 'symlink', 'target': os.readlink(p)})
            else:
                assert stat.S_ISREG(mode), p
                entries.append({'path': str(p), 'kind': 'file', 'bytes': p.stat().st_size, 'sha256': digest(p)})
for row in data_records:
    entries.append({'path': row['path'], 'kind': 'file', 'bytes': row['bytes'], 'sha256': row['sha256'], 'protected_source': row['source']})
assert len({row['path'] for row in entries}) == len(entries)
for src in (root / 'runs/.tmp/goal-91-next/evidence').rglob('*'):
    if src.is_file():
        rel = src.relative_to(root / 'runs/.tmp/goal-91-next')
        main = root / rel
        assert main.is_file(), rel
        historical = {
            'evidence/OPS-05/verification.json': 'evidence/OPS-05/verification-20260925-before-device-training.json',
            'evidence/TSK-11/verification.json': 'evidence/TSK-11/verification-20260925-with-20261002-rollout.json',
        }
        if str(rel) in historical:
            assert digest(src) == digest(root / historical[str(rel)]), rel
        elif str(rel) not in ['evidence/OPS-05/device-adam-20261012/verification.json', 'evidence/TSK-11/task-checkpoint-20261012/environment-and-source.json']:
            assert digest(src) == digest(main), rel

inventory = {'recorded_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'owned_dirs': [str(p) for p in owned_dirs], 'entries': entries, 'directories': directories, 'scope': 'All unique temporary roots were created for this goal; initial next-copy is provenance, not the final snapshot. Final 708-source manifest, curated scoped receipts and this current inventory account for later changes.', 'global_cache_symlink_targets_read_or_deleted': False}
raw = (json.dumps(inventory, ensure_ascii=False, indent=2) + '\n').encode()
with (goal / 'cleanup-inventory.json.gz').open('xb') as f:
    f.write(gzip.compress(raw, mtime=0))
print('Current inventory preserved; regular files=' + str(sum(e['kind'] == 'file' for e in entries)), flush=True)

removed_files = 0
removed_bytes = 0
removed_links = 0
for row in entries:
    p = Path(row['path'])
    if row['kind'] == 'symlink':
        assert p.is_symlink() and os.readlink(p) == row['target'], p
        p.unlink()
        removed_links += 1
    else:
        assert not p.is_symlink() and p.is_file(), p
        assert p.stat().st_size == row['bytes'] and digest(p) == row['sha256'], p
        p.unlink()
        removed_files += 1
        removed_bytes += row['bytes']
for directory in sorted(directories, key=lambda p: len(Path(p).parts), reverse=True):
    Path(directory).rmdir()
for directory in sorted([p for p in data_dir.rglob('*') if p.is_dir()] + [data_dir], key=lambda p: len(p.parts), reverse=True):
    directory.rmdir()
for directory in [root / 'data', root / 'runs/.tmp', root / 'runs']:
    if directory.exists() and not any(directory.iterdir()):
        directory.rmdir()
for path, expected in frozen.items():
    assert digest(root / path) == expected, path
assert not any(p.exists() for p in owned_dirs)
assert not data_dir.exists()
for row in data_records:
    p = Path(row['source'])
    assert p.is_file() and p.stat().st_size == row['bytes']
write_new(goal / 'cleanup.json', {'completed_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'completed': True, 'removed_regular_files': removed_files, 'removed_bytes': removed_bytes, 'removed_symlinks': removed_links, 'removed_owned_roots': [str(p) for p in owned_dirs], 'native_data_copies_removed': len(data_records), 'protected_sources_verified_before_cleanup': data_records, 'protected_source_paths_present_after_cleanup': True, 'source_files_verified_after_cleanup': len(frozen), 'global_cache_symlink_targets_read_or_deleted': False, 'inventory': 'evidence/goal-91-20261012/cleanup-inventory.json.gz', 'inventory_raw_sha256': hashlib.sha256(raw).hexdigest(), 'inventory_compressed_sha256': digest(goal / 'cleanup-inventory.json.gz'), 'curated_evidence_and_failed_attempts_preserved': True, 'historical_copy_receipt_not_rewritten': True})
print(json.dumps({'completed': True, 'removed_files': removed_files, 'removed_bytes': removed_bytes, 'removed_symlinks': removed_links, 'protected_inputs': len(data_records), 'source_files': len(frozen)}), flush=True)
