from pathlib import Path
import os,subprocess,json,time,hashlib,sys
repo=Path('/Users/timlai/Developer/coimnet')
out=Path('/private/tmp/coimnet-infer-root-20261003')
env=os.environ.copy();env['GOMEMLIMIT']='8GiB'
files=sorted(set(subprocess.check_output(['git','ls-files','*.go'],cwd=repo,text=True).splitlines()+subprocess.check_output(['git','ls-files','--others','--exclude-standard','*.go'],cwd=repo,text=True).splitlines()))
source={f:hashlib.sha256((repo/f).read_bytes()).hexdigest() for f in files+['go.mod','go.sum']}
(out/'source-before.json').write_text(json.dumps(source,sort_keys=True,indent=2)+'\n')
commands=[('format',['gofmt','-l',*files]),('targeted',['go','test','-count=1','-v','./examples/realnav','./tasks/nav2d/trajectory']),('targeted-race',['go','test','-race','-count=1','-v','./examples/realnav','./tasks/nav2d/trajectory']),('build',['go','build','./...']),('test',['go','test','-p','4','-timeout','30m','./...']),('race',['go','test','-p','4','-race','-timeout','30m','./...']),('vet',['go','vet','./...']),('mod-verify',['go','mod','verify']),('mod-tidy',['go','mod','tidy','-diff'])]
results=[]
for name,cmd in commands:
 start=time.monotonic()
 with (out/(name+'.log')).open('wb') as log:r=subprocess.run(cmd,cwd=repo,env=env,stdout=log,stderr=subprocess.STDOUT)
 item={'name':name,'command':cmd,'exit_code':r.returncode,'elapsed_seconds':time.monotonic()-start,'log':name+'.log','GOMEMLIMIT':'8GiB'}
 if name=='format' and (out/(name+'.log')).stat().st_size: item['formatted']=False
 else:item['formatted']=True
 results.append(item);(out/'checks.json').write_text(json.dumps(results,indent=2)+'\n');print(json.dumps(item),flush=True)
 if r.returncode or not item['formatted']:sys.exit(1)
source_after={f:hashlib.sha256((repo/f).read_bytes()).hexdigest() for f in source}
(out/'source-after.json').write_text(json.dumps(source_after,sort_keys=True,indent=2)+'\n')
assert source_after==source,'Go source changed during verification'
print('SOURCE_UNCHANGED',flush=True)
