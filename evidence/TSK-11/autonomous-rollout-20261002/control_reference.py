from pathlib import Path
import math,json
base=Path('/Users/timlai/Developer/coimnet-data/TSK-11/autonomous-rollout-20261002')
work=Path('/private/tmp/coimnet-realnav-root-20261002')
starts=json.loads((work/'initial-pairs-reference.json').read_text())['starts']
report=json.loads((base/'rollout-a/report.json').read_text());model=json.loads((base/'train-a/model.json').read_text());mean=model['train_mean_displacement_cm_per_sample'];results={}
for trial in report['trials']:
 s=starts[trial['trial_id']];x,y=s['current'];px,py=s['previous_displacement'];tx,ty=s['target'];nearest=math.hypot(x-tx,y-ty);path=0.;collisions=0;hit=-1
 for i in range(200):
  norm=math.hypot(px,py);dx,dy=(px/norm*mean,py/norm*mean) if norm else (0.,0.)
  length=math.hypot(dx,dy)
  if length>5:dx,dy=dx/length*5,dy/length*5
  nx,ny=x+dx,y+dy
  if math.hypot(nx,ny)>30:nx,ny=x,y;dx=dy=0.;collisions+=1
  segment=(nx-x)**2+(ny-y)**2
  f=max(0.,min(1.,((tx-x)*(nx-x)+(ty-y)*(ny-y))/segment)) if segment else 0.
  distance=math.hypot(x+f*(nx-x)-tx,y+f*(ny-y)-ty);nearest=min(nearest,distance);path+=math.hypot(dx,dy);x,y=nx,ny;px,py=dx,dy
  if distance<=2:hit=i+1;break
 expected={'success':hit>=0,'hit_step':hit,'steps':i+1,'path_cm':path,'final_distance_cm':math.hypot(x-tx,y-ty),'nearest_distance_cm':nearest,'collisions':collisions}
 actual=trial['strategies']['persistent_direction']
 for key,value in expected.items():
  if isinstance(value,float):assert abs(value-actual[key])<1e-10,(trial['trial_id'],key,value,actual[key])
  else:assert value==actual[key],(trial['trial_id'],key,value,actual[key])
 results[trial['trial_id']]=expected
(work/'independent-persistent-control.json').write_text(json.dumps({'method':'independent Python source-pair, vector direction, arena rejection and segment-distance calculation','mean_displacement_cm_per_sample':mean,'compared_fields':list(expected),'all_13_trials_matched':True,'absolute_tolerance':1e-10,'results':results},indent=2)+'\n')
print('13 persistent controls matched independent calculation')
