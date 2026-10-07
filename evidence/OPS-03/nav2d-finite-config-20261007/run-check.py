from pathlib import Path
import subprocess,sys,json,time,datetime,os
ev=Path("evidence/OPS-03/nav2d-finite-config-20261007")
name=sys.argv[1];cmd=sys.argv[2:]
env=os.environ.copy();overrides={}
if name=="tidy":env["GOPROXY"]="off";overrides["GOPROXY"]="off"
start=time.time();started=datetime.datetime.now(datetime.timezone.utc).isoformat()
with (ev/(name+".log")).open("x") as out:
 r=subprocess.run(cmd,stdout=out,stderr=subprocess.STDOUT,env=env)
j={"command":cmd,"exit_code":r.returncode,"started_at":started,"elapsed_seconds":time.time()-start,"log":name+".log","environment_overrides":overrides}
(ev/(name+".json")).write_text(json.dumps(j,indent=2)+"\n")
print(json.dumps(j));print((ev/(name+".log")).read_text()[-2500:])
sys.exit(r.returncode)
