import sys,subprocess,pathlib,time,json,datetime,os
root=pathlib.Path(__file__).resolve().parents[3]
ev=pathlib.Path(__file__).resolve().parent
name=sys.argv[1];cmd=sys.argv[2:]
env=os.environ.copy()
overrides={}
if name=='mod-tidy-diff':env['GOPROXY']='off';overrides['GOPROXY']='off'
start=time.time();started=datetime.datetime.now(datetime.timezone.utc).isoformat()
with (ev/(name+'.log')).open('x') as out:
 result=subprocess.run(cmd,cwd=root,env=env,stdout=out,stderr=subprocess.STDOUT)
record={'name':name,'command':cmd,'started_at':started,'elapsed_seconds':time.time()-start,'exit_code':result.returncode,'log':name+'.log','environment_overrides':overrides}
(ev/(name+'.json')).write_text(json.dumps(record,indent=2)+'\n')
print(json.dumps(record))
print((ev/(name+'.log')).read_text()[-1200:])
sys.exit(result.returncode)
