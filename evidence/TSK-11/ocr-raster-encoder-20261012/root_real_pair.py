import hashlib,json,os,pathlib,subprocess,time
root=pathlib.Path('/Users/timlai/Developer/coimnet/runs/.tmp/goal-91-next')
work=root/'runs/.tmp/62/real-pair-final'
work.mkdir(exist_ok=False)
env=os.environ.copy()
cache=pathlib.Path('/Users/timlai/Developer/coimnet/runs/.tmp/goal-91-20261012')
env.update(GOCACHE=str(cache/'go-build'),GOMODCACHE=str(cache/'go-mod-cache'),GOTMPDIR=str(cache/'go-tmp'),TMPDIR=str(cache/'go-tmp'),GOPROXY='off',GOTOOLCHAIN='local')
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def inventory(p):return {str(f.relative_to(p)):sha(f) for f in p.rglob('*') if f.is_file()}
sources={str(p.relative_to(root)):sha(p) for base in ['tasks/ocr','examples/realocr'] for p in (root/base).glob('*.go')}
data=pathlib.Path('/Users/timlai/Developer/coimnet/data/TSK-11/kmnist')
before=inventory(data);commands=[]
def run(name,args):
 start=time.time();log=work/(name+'.log')
 with log.open('xb') as f:r=subprocess.run(args,cwd=root,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=1800)
 row=dict(name=name,command=args,exit_code=r.returncode,elapsed_seconds=time.time()-start,log=log.name,log_sha256=sha(log));commands.append(row)
 (work/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 print(json.dumps(row),flush=True)
 assert r.returncode==0,row
exe=work/'realocr'
run('build',['/usr/local/go/bin/go','build','-p','2','-o',str(exe),'./examples/realocr'])
for mode in ['fixed','trainable']:
 args=[str(exe),'--data-dir',str(data),'--train-count','1000','--test-count','200','--epochs','1','--seed','20260924','--encoder',mode]
 run(mode+'-train',args+['--model-output',str(work/(mode+'-model')),'--report',str(work/(mode+'-train.json'))])
 model_before=inventory(work/(mode+'-model'))
 run(mode+'-restore',args+['--model-input',str(work/(mode+'-model')),'--report',str(work/(mode+'-restore.json'))])
 assert inventory(work/(mode+'-model'))==model_before
 train=json.loads((work/(mode+'-train.json')).read_text());restored=json.loads((work/(mode+'-restore.json')).read_text())
 assert train['after']==restored['before']==restored['after']
 assert restored['train']['epochs']==0 and restored['train']['updates']==0
 assert train['encoder_profile']==restored['encoder_profile']
 assert train['model']['sha256']==restored['model']['sha256']
 (work/(mode+'-model-files.json')).write_text(json.dumps(model_before,indent=2)+'\n')
 print(json.dumps({'mode':mode,'before':train['before'],'after':train['after'],'encoder_profile':train['encoder_profile'],'restored_equal':True}),flush=True)
assert inventory(data)==before
assert all(sha(root/p)==h for p,h in sources.items())
(work/'verification.json').write_text(json.dumps(dict(passed=True,sources=sources,data_files=before,binary_sha256=sha(exe),commands=commands,new_process=True,no_retraining=True,models_unchanged=True),indent=2)+'\n')
