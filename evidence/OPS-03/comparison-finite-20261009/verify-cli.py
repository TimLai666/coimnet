import pathlib,subprocess,json,tempfile,sys,hashlib
ev=pathlib.Path(__file__).resolve().parent
phase=sys.argv[1]
records=[]
def invoke(cmd):
 p=subprocess.run(cmd,capture_output=True,text=True)
 records.append({"command":cmd,"exit_code":p.returncode,"stdout":p.stdout,"stderr":p.stderr})
 return p
with tempfile.TemporaryDirectory(prefix="coimnet-comparison-cli-") as d:
 d=pathlib.Path(d);binary=d/"coimnet"
 p=invoke(["go","build","-o",str(binary),"./cmd/coimnet"]);assert p.returncode==0,p.stderr
 commands={
 "attribution":["examples","run","attribution","--episodes","1","--eval","1","--hidden","4","--recurrent","1","--groups","normal,frozen_core","--resamples","100"],
 "continual":["examples","run","continual-matrix","--budget","1","--episodes","1"]
 }
 outputs={}
 for name,args in commands.items():
  f=d/(name+".json")
  p=invoke([str(binary),*args,"--out",str(f)]);assert p.returncode==0,p.stderr
  data=f.read_bytes();obj=json.loads(data)
  def walk(x):
   if isinstance(x,dict):
    def failed(v):
     if isinstance(v,list):return any(failed(y) for y in v)
     return v is True
    assert not failed(x.get("failed",False)),x
    for v in x.values():walk(v)
   elif isinstance(x,list):
    for v in x:walk(v)
  walk(obj)
  outputs[name]={"sha256":hashlib.sha256(data).hexdigest(),"report":obj}
  p=invoke([str(binary),*args,"--out",str(f)]);assert p.returncode==1 and "exist" in p.stderr,p
  assert f.read_bytes()==data
  p=invoke([str(binary),*args,"--help"]);assert p.returncode==0 and "--out" in p.stdout
 if phase!="before":
  for value in ["NaN","+Inf","-Inf","0","1","-0.1","1.1"]:
   f=d/("invalid-"+value+".json")
   p=invoke([str(binary),*commands["attribution"],"--interval",value,"--out",str(f)])
   assert p.returncode==1 and "--interval" in p.stderr and "strictly between 0 and 1" in p.stderr,p
   assert not f.exists() and not p.stdout,p
 with (ev/("cli-report-"+phase+".json")).open("x") as f:json.dump({"phase":phase,"records":records,"reports":outputs},f,indent=2);f.write("\n")
print(json.dumps({"workflows":len(records)-1,"positive_reports":len(outputs),"phase":phase}))
