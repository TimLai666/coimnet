import hashlib,json,re,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[3]
stage=Path(__file__).resolve().parent
baseline=json.loads((stage/'baseline.json').read_text())
frozen=json.loads((stage/'frozen-tests.json').read_text())
def digest(p): return hashlib.sha256((root/p).read_bytes()).hexdigest()
protected={p:h for p,h in baseline['files'].items() if p.startswith(('evidence/','docs/handoff/')) or p in ('go.mod','go.sum','LICENSE')}
assert all(digest(p)==h for p,h in protected.items()), 'Protected input or historical evidence changed'
assert all(digest(p)==h for p,h in frozen.items()), 'Frozen test changed'
changed_go=sorted(p for p,h in baseline['files'].items() if p.endswith('.go') and digest(p)!=h)
assert changed_go==['distill/distribution.go','tasks/ocr/ctc/ctc.go'], changed_go
signatures={
'distill/distribution.go':r'^func \(d DistributionDistiller\) Loss\(.*$',
'tasks/ocr/ctc/ctc.go':r'^func (?:Loss|LossWith)\(.*$'}
for p,pattern in signatures.items():
    before=subprocess.check_output(['git','show',baseline['base_commit']+':'+p],cwd=root,text=True)
    after=(root/p).read_text()
    assert re.findall(pattern,before,re.M)==re.findall(pattern,after,re.M),p+' public signatures changed'
source={p:digest(p) for p in sorted(set(p for p in baseline['files'] if p.endswith('.go'))|set(frozen))}
result={'base_commit':baseline['base_commit'],'protected_files_unchanged':len(protected),'frozen_tests_unchanged':len(frozen),'existing_go_files_changed':changed_go,'public_signatures_unchanged':True,'go_file_count':len(source),'source_sha256':hashlib.sha256(json.dumps(source,sort_keys=True,separators=(',',':')).encode()).hexdigest(),'files':source}
output=stage/'source-manifest.json'
if output.exists():
    assert json.loads(output.read_text())==result,'Frozen source manifest mismatch'
else:
    with output.open('x') as f: json.dump(result,f,indent=2); f.write('\n')
print(json.dumps({k:v for k,v in result.items() if k!='files'}))
