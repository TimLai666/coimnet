import pathlib,subprocess,json,tempfile,sys,hashlib
root=pathlib.Path(__file__).resolve().parents[3];ev=pathlib.Path(__file__).resolve().parent
phase=sys.argv[1];records=[];reports={}
def invoke(args):
 p=subprocess.run(args,cwd=root,capture_output=True,text=True);records.append({'command':args,'exit_code':p.returncode,'stdout':p.stdout,'stderr':p.stderr});return p
with tempfile.TemporaryDirectory(prefix='coimnet-ownership-cli-') as d:
 d=pathlib.Path(d);binary=d/'coimnet';p=invoke(['go','build','-o',str(binary),'./cmd/coimnet']);assert p.returncode==0,p.stderr
 for name,extra in [('plain',[]),('chemistry',['--chemistry'])]:
  args=[str(binary),'examples','run','continual-matrix','--budget','1','--episodes','1',*extra];out=d/(name+'.json')
  p=invoke([*args,'--out',str(out)]);assert p.returncode==0,p.stderr
  data=out.read_bytes();obj=json.loads(data)
  assert len(obj['seeds'])==3 and len(obj['protocol']['tasks'])==2 and len(obj['protocol']['stages'])==3
  assert all(rec['status']!='failed' for rec in obj['runs']),obj['runs']
  reports[name]={'sha256':hashlib.sha256(data).hexdigest(),'report':obj}
  p=invoke([*args,'--out',str(out)]);assert p.returncode==1 and 'exist' in p.stderr,p
  assert out.read_bytes()==data
  p=invoke([*args,'--help']);assert p.returncode==0 and '--out' in p.stdout
  missing=d/(name+'-bad.json');p=invoke([*args,'--episodes','0','--out',str(missing)]);assert p.returncode==1 and '--episodes' in p.stderr and not missing.exists()
 result={'phase':phase,'records':records,'reports':reports,'workflows':len(records)-1}
 if phase!='before':
  baseline=json.loads((ev/'cli-before.json').read_text())
  assert reports==baseline['reports'],'CLI reports changed'
 (ev/('cli-'+phase+'.json')).open('x').write(json.dumps(result,indent=2)+'\n')
print(json.dumps({'phase':phase,'workflows':len(records)-1,'reports':len(reports),'baseline_matches':phase!='before'}))
