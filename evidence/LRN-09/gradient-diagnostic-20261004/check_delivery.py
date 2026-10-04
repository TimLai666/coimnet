from pathlib import Path
import hashlib,json,re,subprocess
root=Path(__file__).resolve().parents[3]
e=root/'evidence/LRN-09/gradient-diagnostic-20261004'
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
v=json.loads((e/'verification.json').read_text())
for name,digest in v['artifact_sha256'].items():assert sha(e/name)==digest,name
for line in (e/'source.sha256').read_text().splitlines():
 digest,name=line.split('  ',1);assert sha(root/name)==digest,name
original={name:digest for digest,name in (line.split('  ',1) for line in (e/'protected.sha256').read_text().splitlines())}
for name,digest in original.items():assert sha(root/name)==digest,name
status=json.loads((root/'docs/requirements-status.json').read_text())['requirements']
assert len(status)==91 and sum(r['status']=='passed' for r in status)==89
by_id={r['id']:r for r in status}
assert by_id['TSK-11']['status']==by_id['OPS-05']['status']=='specified'
baseline=json.loads(subprocess.check_output(['git','show','359c52891217a113c7cae6a4da06c7cf7b233302:docs/requirements-status.json']))['requirements']
expected={r['id']:r for r in baseline}
expected['LRN-09']['evidence'].append('../evidence/LRN-09/gradient-diagnostic-20261004/verification.json')
assert by_id==expected,'unexpected requirement changes'
files=['ENG.md','delivery-status.md','docs/INDEX.md','docs/tickets/39-ppo-gradient-diagnostic.md','experiment/gridnav/README.md','evidence/LRN-09/gradient-diagnostic-20261004/analysis.md','evidence/LRN-09/gradient-diagnostic-20261004/review.md']
count=0
for name in files:
 p=root/name
 for link in re.findall(r'\]\(([^)]+)\)',p.read_text()):
  if link.startswith(('http:','https:','mailto:','codex:','#')):continue
  target=link.split('#')[0]
  assert (p.parent/target).exists(),(name,target)
  count+=1
all_go=[p for p in subprocess.check_output(['git','ls-files','-z']).decode().split('\0') if p.endswith('.go')]+['experiment/ppo_gradient_diagnostic_test.go','experiment/ppo_gradient_validation_test.go']
versions=set()
for p in all_go:versions.update(re.findall(r'coimnet-[a-z0-9-]+/v[0-9]+',(root/p).read_text()))
index=(root/'docs/INDEX.md').read_text()
previous=set()
for name in all_go:
 if name.startswith('experiment/ppo_gradient_'):continue
 previous.update(re.findall(r'coimnet-[a-z0-9-]+/v[0-9]+',subprocess.check_output(['git','show','359c52891217a113c7cae6a4da06c7cf7b233302:'+name]).decode()))
new=versions-previous
assert new=={'coimnet-ppo-gradient-diagnostic/v1'} and all(version in index for version in new),'new schema undocumented'
print('New schema documented:',sorted(new),'; existing undocumented tokens',len(versions-set(re.findall(r'coimnet-[a-z0-9-]+/v[0-9]+',index))),'(includes negative-test tokens; follow-up recorded)')
assert subprocess.check_output(['git','diff','--check'])==b''
print('PASS: local document links',count,'; schemas',len(versions),'; source files',v['input_fingerprints']['source_file_count'],'; protected originals',len(original),'; artifacts',len(v['artifact_sha256']),'; requirements 89/91; only LRN-09 path appended')
