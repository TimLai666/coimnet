import pathlib,subprocess,json,tempfile,tarfile,io,sys,hashlib
root=pathlib.Path(__file__).resolve().parents[3];ev=pathlib.Path(__file__).resolve().parent
b=json.loads((ev/"baseline.json").read_text())
files=[p for p in b["sha256"] if pathlib.Path(p).suffix in {".go",".mod",".sum",".c",".h",".m",".mm",".cc",".cpp"} or p=="LICENSE"]
prefix=sys.argv[1] if len(sys.argv)>1 else "valid-isolated"
def run(name,main,cwd):
 result=subprocess.run(["python3",str(ev/"check.py"),name,json.dumps(["go","run",str(main)])],cwd=cwd)
 if result.returncode:sys.exit(result.returncode)
with tempfile.TemporaryDirectory(prefix="coimnet-comparison-base-") as scratch:
 d=pathlib.Path(scratch)
 archive=subprocess.check_output(["git","archive",b["base_commit"],"--",*files],cwd=root)
 with tarfile.open(fileobj=io.BytesIO(archive)) as t:
  for member in t.getmembers():
   if member.isdir():continue
   assert member.isfile() and member.name in files,member.name
   p=d/member.name;p.parent.mkdir(parents=True,exist_ok=True)
   with p.open("xb") as f:f.write(t.extractfile(member).read())
 main=d/"scratch"/"main.go";main.parent.mkdir();main.write_bytes((ev/"valid-probe.go.txt").read_bytes())
 run(prefix+"-before",main,d)
 with (ev/(prefix+"-baseline.json")).open("x") as f:json.dump({"git_commit":b["base_commit"],"file_count":len(files),"source_sha256":hashlib.sha256((ev/"valid-probe.go.txt").read_bytes()).hexdigest(),"archive_sha256":hashlib.sha256(archive).hexdigest(),"execution":"go run from temporary archive of original tracked source and module files","no_worktree_change":True},f,indent=2);f.write("\n")
for name in [prefix+"-after-a",prefix+"-after-b"]:
 with tempfile.TemporaryDirectory(prefix="coimnet-comparison-current-") as d:
  main=pathlib.Path(d)/"main.go";main.write_bytes((ev/"valid-probe.go.txt").read_bytes())
  run(name,main,root)
before=json.loads((ev/(prefix+"-before.json")).read_text())["raw_stdout"]
for name in [prefix+"-after-a",prefix+"-after-b"]:
 assert json.loads((ev/(name+".json")).read_text())["raw_stdout"]==before,name
reports=json.loads(before)
def nofailed(v):
 if isinstance(v,dict):
  def failed(x):
   if isinstance(x,list):return any(failed(y) for y in x)
   return x is True
  assert not failed(v.get("failed",False)),v
  for x in v.values():nofailed(x)
 elif isinstance(v,list):
  for x in v:nofailed(x)
for name,report in reports.items():
 nofailed(report)
 if name.startswith("continual-") and name!="continual-nil":
  interval=float(name.removeprefix("continual-"))
  assert report["protocol"]["comparison"]["interval"]==interval
  assert report["comparison"]["interval"]==interval
with (ev/(prefix+"-comparison.json")).open("x") as f:json.dump({"reports":len(reports),"new_current_processes":2,"all_report_bytes_identical_to_original_source":True,"declared_and_realized_intervals_match":True,"failed_records":0,"prior_error_cases":22,"priority_comparison":"sdk-comparison.json","baseline_source":prefix+"-baseline.json","source":"valid-probe.go.txt"},f,indent=2);f.write("\n")
print("Seven reports with independent declarations match original source and two current processes")
