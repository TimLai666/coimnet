import hashlib,json,os,pathlib,subprocess,time
root=pathlib.Path('/Users/timlai/Developer/coimnet/runs/.tmp/goal-91-next')
work=root/'runs/.tmp/63/root-real'
work.mkdir(exist_ok=False)
env=os.environ.copy(); cache=pathlib.Path('/Users/timlai/Developer/coimnet/runs/.tmp/goal-91-20261012')
env.update(GOCACHE=str(cache/'go-build'),GOMODCACHE=str(cache/'go-mod-cache'),GOTMPDIR=str(cache/'go-tmp'),TMPDIR=str(cache/'go-tmp'),GOPROXY='off',GOTOOLCHAIN='local')
env['PATH']='/opt/homebrew/bin:'+env.get('PATH','')
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def inv(p):return {str(f.relative_to(p)):sha(f) for f in p.rglob('*') if f.is_file()}
sources={str(p.relative_to(root)):sha(p) for p in (root/'examples/realmedia').glob('*') if p.is_file()}
commands=[]
def run(name,args,timeout=1800):
 start=time.time();log=work/(name+'.log')
 with log.open('xb') as f:r=subprocess.run(args,cwd=root,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=timeout)
 row=dict(name=name,command=args,exit_code=r.returncode,elapsed_seconds=time.time()-start,log=log.name,log_sha256=sha(log));commands.append(row)
 (work/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');print(json.dumps(row),flush=True)
 assert r.returncode==0,row
go='/usr/local/go/bin/go'
for name,flags in [('unit',[]),('race',['-race'])]:run(name,[go,'test',*flags,'-p','1','-count=1','-json','-timeout=180s','./examples/realmedia'],timeout=240)
run('vet',[go,'vet','./examples/realmedia'])
exe=work/'realmedia'
run('build',[go,'build','-p','2','-o',str(exe),'./examples/realmedia'])
data=pathlib.Path('/Users/timlai/Developer/coimnet/data/TSK-11/blender-open-movies');before=inv(data)
model=work/'model';train=work/'trained';restored=work/'restored'
run('licensed-train',[str(exe),'--data-root',str(data),'--out-dir',str(train),'--model-output',str(model)])
model_before=inv(model)
run('licensed-restore',[str(exe),'--data-root',str(data),'--out-dir',str(restored),'--model-input',str(model)])
assert inv(data)==before and inv(model)==model_before
trainr=json.loads((train/'report.json').read_text());restr=json.loads((restored/'report.json').read_text())
assert trainr['training']['scores_after']==restr['training']['scores_before']==restr['training']['scores_after']
assert restr['training']['epochs']==0 and restr['training']['this_run_updates']==0
assert restr['training']['updates']==trainr['training']['updates']
assert trainr['model']['manifest_sha256']==restr['model']['manifest_sha256']
equal={}
for p in train.rglob('*'):
 if p.is_file() and (p.suffix in ['.png','.wav'] or p.name in ['timeline.json','trained-model.json']):
  q=restored/p.relative_to(train);assert q.is_file() and sha(p)==sha(q),str(p)
  equal[str(p.relative_to(train))]=sha(p)
assert len(equal)>=10,equal
assert all(sha(root/p)==h for p,h in sources.items())
(work/'verification.json').write_text(json.dumps(dict(passed=True,sources=sources,input_files=before,binary_sha256=sha(exe),commands=commands,model_files=model_before,restored_equal_files=equal,new_process=True,no_retraining=True,models_unchanged=True,score_equal=True),indent=2)+'\n')
print(json.dumps({'passed':True,'updates':trainr['training']['updates'],'before':trainr['training']['scores_before'],'after':trainr['training']['scores_after'],'equal_outputs':len(equal)}),flush=True)
