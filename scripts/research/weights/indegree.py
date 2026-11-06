"""Per-neuron view: does in-degree (or reciprocity) predict how useful a neuron is?"""
import json, sys, os
import numpy as np
sys.path.insert(0, os.path.dirname(__file__))
from inspect_ckpt import Model, load_split, acc

x, y = load_split('t10k', int(os.environ.get('LIMIT', '1000')))
for f in sys.argv[1:]:
    m = Model(f)
    logits, rec = m.run(x, record=True)
    base = acc(logits, y)
    yy = rec['y']; a = rec['a']
    indeg = np.bincount(m.dst, minlength=m.n); outdeg = np.bincount(m.src, minlength=m.n)
    ystd = yy.reshape(-1, m.n).std(0); sat = (np.abs(yy) > 0.9).reshape(-1, m.n).mean(0)
    # drive magnitude per neuron (|sum of incoming weights*y|) ~ |v|
    absy = np.abs(yy).reshape(-1, m.n).mean(0)
    win = np.zeros(m.n); np.add.at(win, m.dst, np.abs(m.w))
    hidden = np.ones(m.n, bool); hidden[m.inp] = False; hidden[m.ro] = False
    # bins of in-degree for hidden neurons
    qs = np.quantile(indeg[hidden], [0, .25, .5, .75, 1])
    rows = []
    for lo, hi in zip(qs[:-1], qs[1:]):
        sel = hidden & (indeg >= lo) & (indeg <= hi)
        if sel.sum() == 0:
            continue
        mask = np.ones(m.n); mask[sel] = 0
        drop = base - acc(m.run(x, neuron_mask=mask)[0], y)
        rows.append({'indeg': [int(lo), int(hi)], 'count': int(sel.sum()), 'y_std': round(float(ystd[sel].mean()), 3),
                     'abs_y': round(float(absy[sel].mean()), 3), 'sat': round(float(sat[sel].mean()), 4),
                     'gate': round(float(a.mean((0, 1))[sel].mean()), 3), 'tau_med': round(float(np.median(m.tau[sel])), 2),
                     'sum_abs_w_in': round(float(win[sel].mean()), 2), 'mean_abs_w_in': round(float((win[sel] / np.maximum(indeg[sel], 1)).mean()), 3),
                     'acc_drop_if_removed': round(drop, 4), 'drop_per_neuron_x1000': round(1000 * drop / sel.sum(), 3)})
    print(json.dumps({'file': os.path.basename(f), 'base': base, 'indeg_cv': round(float(indeg.std() / indeg.mean()), 3),
                      'corr_indeg_ystd': round(float(np.corrcoef(indeg[hidden], ystd[hidden])[0, 1]), 3),
                      'corr_indeg_meanabsw': round(float(np.corrcoef(indeg[hidden], win[hidden] / np.maximum(indeg[hidden], 1))[0, 1]), 3),
                      'bins': rows}))
