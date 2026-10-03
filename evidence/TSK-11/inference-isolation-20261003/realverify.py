from pathlib import Path
import subprocess,json,hashlib,time,os
repo=Path('/Users/timlai/Developer/coimnet')
out=Path('/private/tmp/coimnet-infer-root-20261003')
data=Path('/Users/timlai/Developer/coimnet-data/TSK-11/dryad-path-integration/all_ds_t01_d2_cm_no2.csv.gz')
model=Path('/Users/timlai/Developer/coimnet-data/TSK-11/autonomous-rollout-20261002/train-a/model.json')
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
before=json.loads((out/'inputs-before.json').read_text())
assert before[str(data)]==digest(data) and before[str(model)]==digest(model)
checks=[]
for case,snapshot in [('valid',model),('overlap',out/'overlap-model.json'),('omitted',out/'omitted-model.json')]:
 dest=out/('final-'+case)
 assert not dest.exists(),dest
 cmd=[str(out/'realnav-final'),'infer','--data',str(data),'--snapshot',str(snapshot),'--out',str(dest)]
 start=time.monotonic()
 with (out/(case+'-final-stdout.json')).open('xb') as stdout,(out/(case+'-final-stderr.log')).open('xb') as stderr:
  result=subprocess.run(cmd,cwd=repo,env=dict(os.environ,GOMEMLIMIT='8GiB'),stdout=stdout,stderr=stderr)
 item=dict(case=case,command=cmd,exit_code=result.returncode,output_created=dest.exists(),elapsed_seconds=time.monotonic()-start,model_sha256=digest(snapshot),stderr=(out/(case+'-final-stderr.log')).read_text())
 checks.append(item)
 if case=='valid':
  assert result.returncode==0
  baseline=json.loads((out/'baseline-valid/report.json').read_text())
  current=json.loads((dest/'report.json').read_text())
  baseline.pop('runtime');current.pop('runtime')
  assert baseline==current,'legal report differs beyond runtime'
  item['report_equal_except_runtime']=True
 else:
  assert result.returncode!=0 and not dest.exists(),item
  assert 'validate inference split' in item['stderr'],item
assert before[str(data)]==digest(data) and before[str(model)]==digest(model)
(out/'real-checks-final.json').write_text(json.dumps(dict(checks=checks,data_sha256=digest(data),original_model_sha256=digest(model),binary_sha256=digest(out/'realnav-final')),indent=2)+'\n')
print(json.dumps(checks,indent=2))
