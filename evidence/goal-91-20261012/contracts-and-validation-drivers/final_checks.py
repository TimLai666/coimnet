import datetime,hashlib,json,os,pathlib,subprocess,time,sys
root=pathlib.Path('/Users/timlai/Developer/coimnet/runs/.tmp/goal-91-next')
logs=root/'runs/.tmp/final-validation'
logs.mkdir(exist_ok=False)
cache=pathlib.Path('/Users/timlai/Developer/coimnet/runs/.tmp/goal-91-20261012')
env=os.environ.copy()
env.update(GOCACHE=str(cache/'go-build'),GOMODCACHE=str(cache/'go-mod-cache'),GOTMPDIR=str(cache/'go-tmp'),TMPDIR=str(cache/'go-tmp'),GOPROXY='off',GOTOOLCHAIN='local',GOMAXPROCS='2')
excluded={'runs','data','models','checkpoints','bin','.git','.agents','.codex'}
paths=sorted({p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file() and p.relative_to(root).parts[0] not in excluded and p.suffix in ['.go','.wgsl']}|{'go.mod','go.sum'})
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
sources={p:sha(root/p) for p in paths}
(logs/'source-frozen.json').write_text(json.dumps(sources,indent=2)+'\n')
(logs/'environment.json').write_text(json.dumps(dict(cwd=str(root),native_disk=True,base_commit='eb8492e477c365bfd374d3c2012e955db6e12060',os_uname=list(os.uname()),go_version=subprocess.check_output(['/usr/local/go/bin/go','version'],text=True).strip(),environment_overrides={k:env[k] for k in ['GOCACHE','GOMODCACHE','GOTMPDIR','TMPDIR','GOPROXY','GOTOOLCHAIN','GOMAXPROCS']},note='Two scheduler threads and two package workers bound concurrent native verification; test cases and assertions are unchanged. This is functional validation, not a performance benchmark.'),indent=2)+'\n')
go='/usr/local/go/bin/go'
checks=[
 ('format',['/usr/local/go/bin/gofmt','-l',*[p for p in paths if p.endswith('.go')]]),
 ('build',[go,'build','-p','2','./...']),
 ('unit',[go,'test','-p','2','-count=1','-json','-timeout','45m','./...']),
 ('race',[go,'test','-p','2','-race','-count=1','-json','-timeout','45m','./...']),
 ('vet',[go,'vet','-p','2','./...']),
 ('modules',[go,'mod','verify']),
 ('governance',[go,'test','-count=1','-json','-run','^(TestGovernance|TestFormatIndex|TestOnlyNetworkPackagesImportHTTP|TestNoTelemetry|TestScanCommitScriptFlagsSecrets)','.'])]
records=[]
def verify():
 for p,h in sources.items():assert sha(root/p)==h,p
for name,args in checks:
 verify();print('start '+name,flush=True);start=time.monotonic();started=datetime.datetime.now(datetime.timezone.utc).isoformat();p=logs/(name+'.log')
 with p.open('xb') as f:r=subprocess.run(args,cwd=root,env=env,stdout=f,stderr=subprocess.STDOUT)
 raw=p.read_bytes();passed=r.returncode==0 and (name!='format' or not raw.strip())
 records.append(dict(name=name,command=args,cwd=str(root),started_at=started,ended_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),elapsed_seconds=time.monotonic()-start,exit_code=r.returncode,passed=passed,log=p.name,log_bytes=len(raw),log_sha256=sha(p)))
 (logs/'commands.json').write_text(json.dumps(records,indent=2)+'\n');print('end '+name+' passed='+str(passed),flush=True)
 if not passed:sys.exit(1)
verify();print('all complete; frozen sources='+str(len(sources)),flush=True)
