import subprocess,sys,json,time,datetime,pathlib,os
ev=pathlib.Path(__file__).resolve().parent
name=sys.argv[1];cmd=json.loads(sys.argv[2])
start=time.time();date=datetime.datetime.now(datetime.timezone.utc).isoformat()
p=subprocess.run(cmd,capture_output=True,text=True)
raw=p.stdout+p.stderr
with (ev/(name+".log")).open("x") as f:f.write(raw.rstrip()+"\n" if raw else "")
record={"command":cmd,"exit_code":p.returncode,"started_at":date,"elapsed_seconds":time.time()-start,"log":name+".log","raw_stdout":p.stdout,"raw_stderr":p.stderr}
with (ev/(name+".json")).open("x") as f:json.dump(record,f,indent=2);f.write("\n")
print(json.dumps({k:v for k,v in record.items() if not k.startswith("raw_")}))
sys.exit(p.returncode)
