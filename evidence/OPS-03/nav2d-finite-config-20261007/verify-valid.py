from pathlib import Path
import subprocess,json,hashlib,datetime,time
ev=Path("evidence/OPS-03/nav2d-finite-config-20261007")
probe=Path("/tmp/coimnet-finite-valid-probe.go")
assert probe.read_bytes()==(ev/"valid-probe.go.txt").read_bytes()
before=json.loads((ev/"valid-before.json").read_bytes())
records=[]
for name in ["valid-after-report","valid-repeat-report"]:
 start=time.time();cmd=["go","run",str(probe)];r=subprocess.run(cmd,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 with (ev/(name+".json")).open("xb") as f:f.write(r.stdout)
 with (ev/(name+".stderr.log")).open("xb") as f:f.write(r.stderr)
 records.append({"command":cmd,"exit_code":r.returncode,"elapsed_seconds":time.time()-start,"report":name+".json","stderr":name+".stderr.log"})
 assert r.returncode==0
 report=json.loads(r.stdout);assert report==before,"changed valid report"
reports=[before["Nav"],*before["Suite"]["tasks"],before["Attribution"],before["CustomNav"],before["CustomAttribution"]]
assert len(reports)==8
assert all(not run["failed"] for report in reports for run in report["runs"])
result={"status":"passed","source_manifest":"source-freeze.json","probe_sha256":hashlib.sha256(probe.read_bytes()).hexdigest(),"reference":"valid-before.json","whole_report_equality":True,"reports":len(reports),"run_records":sum(len(report["runs"]) for report in reports),"new_processes":records,"verified_at":datetime.datetime.now(datetime.timezone.utc).isoformat()}
with (ev/"valid-comparison.json").open("x") as f:json.dump(result,f,indent=2);f.write("\n")
print(json.dumps(result))
