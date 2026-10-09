import pathlib,subprocess,json,tempfile,tarfile,io,sys,hashlib,os
root=pathlib.Path(__file__).resolve().parents[3];ev=pathlib.Path(__file__).resolve().parent
base=json.loads((ev/'baseline.json').read_text());paths=[p for p in base['files'] if pathlib.Path(p).suffix in {'.go','.mod','.sum','.c','.h','.m','.mm','.cc','.cpp'} or p=='LICENSE']
archive=subprocess.check_output(['git','archive',base['head'],'--',*paths],cwd=root)
with tempfile.TemporaryDirectory(prefix='coimnet-ownership-original-') as d:
 d=pathlib.Path(d)
 with tarfile.open(fileobj=io.BytesIO(archive)) as tf:
  for member in tf.getmembers():
   if member.isdir():continue
   assert member.isfile() and member.name in paths,member.name
   out=d/member.name;out.parent.mkdir(parents=True,exist_ok=True);out.open('xb').write(tf.extractfile(member).read())
 main=d/'scratch/main.go';main.parent.mkdir();main.open('xb').write((ev/'valid-probe.go.txt').read_bytes())
 env=dict(os.environ,COIMNET_CHECK_CWD=str(d));p=subprocess.run(['python3',str(ev/'check.py'),'sdk-original','go','run',str(main)],env=env);assert p.returncode==0
baseline=(ev/'sdk-baseline.log').read_bytes();assert (ev/'sdk-original.log').read_bytes()==baseline
for name in ['sdk-after-a','sdk-after-b']:
 with tempfile.TemporaryDirectory(prefix='coimnet-ownership-current-') as d:
  main=pathlib.Path(d)/'main.go';main.open('xb').write((ev/'valid-probe.go.txt').read_bytes())
  p=subprocess.run(['python3',str(ev/'check.py'),name,'go','run',str(main)],cwd=root);assert p.returncode==0
  assert (ev/(name+'.log')).read_bytes()==baseline,name
reports=json.loads(baseline)
for name in ['plain','nil-comparison','chemistry','interval-half']:
 r=reports[name];assert r['error']=='',r
 assert all(rec['status']!='failed' for rec in r['report']['runs'])
 assert len(r['report']['seeds'])==3
assert reports['cancelled']['error']=='context canceled'
assert all(rec['status']=='failed' for rec in reports['builder-failure']['report']['runs'])
assert reports['empty-rows']['report']['protocol']['evaluation']['state_switch']['concentration']==[None,[]]
result={'reports':len(reports),'fresh_current_processes':2,'raw_report_bytes_equal_original':True,'original_head':base['head'],'archive_sha256':hashlib.sha256(archive).hexdigest(),'archived_files':len(paths),'probe_sha256':hashlib.sha256((ev/'valid-probe.go.txt').read_bytes()).hexdigest(),'nil_empty_rows_preserved':True,'cancel_and_build_failure_preserved':True}
(ev/'sdk-comparison.json').open('x').write(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
