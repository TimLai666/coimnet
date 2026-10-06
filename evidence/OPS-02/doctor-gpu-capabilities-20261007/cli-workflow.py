"""Verify Doctor's JSON/help/errors with a newly built binary and clean PATH."""
import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import tempfile

argparse.ArgumentParser(description=__doc__).parse_args()
root = pathlib.Path(__file__).resolve().parents[3]
checks = []
with tempfile.TemporaryDirectory(prefix='coimnet-doctor-') as directory:
    temporary = pathlib.Path(directory)
    binary = temporary / 'coimnet'
    subprocess.run(['go', 'build', '-o', str(binary), './cmd/coimnet'], cwd=root, check=True)
    binary_sha256 = hashlib.sha256(binary.read_bytes()).hexdigest()

    def run(arguments, environment=None):
        result = subprocess.run([str(binary), *arguments], cwd=root,
                                env=environment, capture_output=True, text=True)
        checks.append({'arguments': arguments, 'exit_code': result.returncode,
                       'stdout': result.stdout, 'stderr': result.stderr})
        return result

    normal = run(['doctor'])
    assert normal.returncode == 0, normal.stderr
    report = json.loads(normal.stdout)
    assert report['schema_version'] == 'coimnet-doctor/v1'
    gpu = report['core']['gpu']
    assert all(value == 'implemented_with_constraints' for key, value in gpu.items() if key != 'details')
    assert len(gpu) == 7 and len(report['core']['cpu']) == 6
    assert gpu['details']['execution'] == 'not_probed'
    assert gpu['details']['full_graph'] == 'unverified'
    assert report['insyra']['probe']['status'] == 'passed'

    empty = temporary / 'empty-path'
    empty.mkdir()
    environment = dict(os.environ, PATH=str(empty))
    missing = run(['doctor'], environment)
    assert missing.returncode == 0, missing.stderr
    absent = json.loads(missing.stdout)
    assert absent['gpu']['status'] == 'unknown'
    assert absent['gpu']['reason'] == 'probe_tool_unavailable'
    assert absent['core'] == report['core']

    help_result = run(['doctor', '--help'])
    assert help_result.returncode == 0
    for term in ['WebGPU', 'continuous', 'Recompute', 'scalar', 'zero-delay', 'independent episode', 'CPU', 'AdamW',
                 'full graph', 'not_probed', 'Errors:']:
        assert term.lower() in help_result.stdout.lower(), term
    overview = run(['--help'])
    assert overview.returncode == 0 and 'doctor' in overview.stdout
    for arguments in [['doctor', 'extra'], ['doctor', '--unsupported']]:
        result = run(arguments)
        assert result.returncode != 0 and result.stderr
        assert not result.stdout.lstrip().startswith('{'), 'invalid arguments returned a success report'
    # Read each generated file before TemporaryDirectory removes it.
    temporary_files = {str(p.relative_to(temporary)): hashlib.sha256(p.read_bytes()).hexdigest()
                       for p in temporary.rglob('*') if p.is_file()}

assert not temporary.exists()
print(json.dumps({'status': 'passed', 'binary_sha256': binary_sha256,
                  'checks': checks, 'temporary_files': temporary_files,
                  'temporary_artifacts_removed': True}, indent=2))
