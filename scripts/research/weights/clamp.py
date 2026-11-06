"""Gate clamp: how many neurons are stuck at the a=1 clamp (no gradient to gate/tau)?"""
import json, sys, os
import numpy as np
sys.path.insert(0, os.path.dirname(__file__))
from inspect_ckpt import Model, load_split, acc

x, y = load_split('t10k', 500)
for f in sys.argv[1:]:
    m = Model(f)
    _, rec = m.run(x, record=True)
    a = rec['a']
    clip = (a >= 1 - 1e-12).mean((0, 1))
    fast = m.tau < 1 / np.log(2)
    print(json.dumps({'file': os.path.basename(f),
                      'neurons_tau_below_1.44': round(float(fast.mean()), 3),
                      'clip_frac_all': round(float(clip.mean()), 3),
                      'neurons_clipped_>10%': round(float((clip > .1).mean()), 3),
                      'neurons_clipped_>50%': round(float((clip > .5).mean()), 3),
                      'neurons_clipped_>90%': round(float((clip > .9).mean()), 3),
                      'clip_frac_among_fast': round(float(clip[fast].mean()), 3) if fast.any() else None}))
