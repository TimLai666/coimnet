from pathlib import Path
import subprocess,os,json,hashlib,time
repo=Path('/Users/timlai/Developer/coimnet')
work=Path('/private/tmp/coimnet-realnav-root-20261002')
base=Path('/Users/timlai/Developer/coimnet-data/TSK-11/autonomous-rollout-20261002')
base.mkdir(mode=0o700)
data='/Users/timlai/Developer/coimnet-data/TSK-11/dryad-path-integration/all_ds_t01_d2_cm_no2.csv.gz'
binary=str(work/'realnav');env=os.environ.copy();env['GOMEMLIMIT']='8GiB';results=[]
def execute(name,args,expected=0):
    cmd=[binary,*args];start=time.monotonic()
    with (base/(name+'-stdout.json')).open('wb') as output,(base/(name+'-stderr.log')).open('wb') as error:
        p=subprocess.run(cmd,cwd=repo,env=env,stdout=output,stderr=error)
    results.append({'name':name,'command':cmd,'exit_code':p.returncode,'expected_exit_code':expected,'elapsed_seconds':time.monotonic()-start})
    (base/'commands.json').write_text(json.dumps(results,indent=2)+'\n')
    assert p.returncode==expected,(name,p.returncode,(base/(name+'-stderr.log')).read_text())
    print(json.dumps(results[-1]),flush=True)
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
for run in ['a','b']:
    execute('train-'+run,['train','--data',data,'--out',str(base/('train-'+run))])
    model=base/('train-'+run)/'model.json';before=sha(model)
    execute('rollout-'+run,['rollout','--data',data,'--snapshot',str(model),'--out',str(base/('rollout-'+run))])
    assert sha(model)==before,'saved model file mutated'
execute('infer',['infer','--data',data,'--snapshot',str(base/'train-a/model.json'),'--out',str(base/'infer')])
execute('old-snapshot',['rollout','--data',data,'--snapshot','/Users/timlai/Developer/coimnet-data/TSK-11/realnav-measured-20260925/model.json','--out',str(base/'old-snapshot')])
models=[base/('train-'+r)/'model.json' for r in ['a','b']];assert models[0].read_bytes()==models[1].read_bytes()
reports=[json.loads((base/('rollout-'+r)/'report.json').read_text()) for r in ['a','b']]
for r in reports:r.pop('runtime')
assert reports[0]==reports[1],'repeated rollout changed non-runtime report'
train=json.loads((base/'train-a/report.json').read_text());infer=json.loads((base/'infer/report.json').read_text());assert train['metrics']['after']['test']==infer['metrics']['after']['test']
ref=json.loads((work/'initial-pairs-reference.json').read_text());r=reports[0];assert r['source_sha256']==ref['source_sha256'];assert r['original_holdout_trial_ids']==ref['heldout_trial_ids'];assert len(r['trials'])==13 and not r['excluded']
for trial in r['trials']:
    start=ref['starts'][trial['trial_id']];assert [trial['target']['x_cm'],trial['target']['y_cm']]==start['target']
    zero=trial['strategies']['zero_output'];assert zero['steps']==200 and zero['path_cm']==0 and not zero['success'] and zero['collisions']==0
    assert abs(zero['final_distance_cm']-start['initial_distance_cm'])<1e-12
artifact_before={str(p.relative_to(base)):sha(p) for p in (base/'rollout-a').rglob('*') if p.is_file()}
common=['rollout','--data',data,'--snapshot',str(models[0])]
execute('reject-existing',common+['--out',str(base/'rollout-a')],1)
assert artifact_before=={str(p.relative_to(base)):sha(p) for p in (base/'rollout-a').rglob('*') if p.is_file()}
forbidden=repo/'rollout-output-must-not-exist-20261002';assert not forbidden.exists()
execute('reject-repo',common+['--out',str(forbidden)],1);assert not forbidden.exists()
sourcechild=Path(data).parent/'rollout-output-must-not-exist-20261002';assert not sourcechild.exists()
execute('reject-source',common+['--out',str(sourcechild)],1);assert not sourcechild.exists()
execute('reject-steps',common+['--steps','0','--out',str(base/'invalid-steps')],1);assert not (base/'invalid-steps').exists()
summary={'training_file_sha256':sha(models[0]),'snapshot_sha256':r['snapshot_sha256'],'protocol_sha256':r['protocol_sha256'],'repeated_training_models_identical':True,'repeated_rollout_except_runtime_identical':True,'fresh_infer_matches_train_test_metrics':True,'old_snapshot_load_succeeded':True,'independent_source_initial_pairs_and_targets_match':True,'zero_control_hand_calculation_match':True,'exclusive_output_unchanged':True,'repo_and_source_outputs_rejected_without_creation':True,'evaluated_trials':13,'excluded_trials':0,'initial_hits':sum(s['start_success'] for t in r['trials'] for s in [t['strategies']['model']]),'aggregates':r['aggregates'],'one_step_metrics':train['metrics'],'files':{str(p.relative_to(base)):sha(p) for p in base.rglob('*') if p.is_file()}}
(base/'verification-summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps({k:v for k,v in summary.items() if k not in ['files','one_step_metrics']},indent=2),flush=True)
