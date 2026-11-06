import json, sys, os
import numpy as np

for f in sys.argv[1:]:
    c = json.load(open(f)); t = c['trainer']; d = t['config']['dynamics']
    n = d['nodes']; src = np.array(d['sources']); dst = np.array(d['targets'])
    inp = np.array(t['config']['input_nodes']); ro = np.array(t['config']['readout_nodes'])
    w = np.array(t['parameters']['core']['weights'])
    A = np.zeros((n, n), bool); A[src, dst] = True
    recip = (A & A.T).sum() / A.sum()
    # expected reciprocity for random with same density
    dens = A.sum() / (n * n)
    # clustering (undirected)
    U = (A | A.T).astype(float)
    deg = U.sum(1); tri = np.diag(U @ U @ U) / 2
    clus = np.mean(np.where(deg > 1, 2 * tri / np.maximum(deg * (deg - 1), 1), 0))
    isin = np.zeros(n, bool); isin[inp] = True
    # direct input sources per readout
    direct = np.array([isin[src[dst == r]].sum() for r in ro])
    # distinct input nodes reachable within 2 hops (receptive field) per readout
    Af = A.astype(float)
    R1 = Af[inp][:, ro] > 0
    R2 = (Af[inp] @ Af)[:, ro] > 0
    rf1 = R1.sum(0); rf2 = (R1 | R2).sum(0)
    # weight on reciprocal edges vs one-way
    rec_e = A[dst, src]
    # sign: product of weights in reciprocal pairs (positive feedback loops?)
    W = np.zeros((n, n)); W[src, dst] = w
    pairs = np.argwhere(np.triu(A & A.T, 1))
    prod_pos = float(np.mean([W[i, j] * W[j, i] > 0 for i, j in pairs])) if len(pairs) else float('nan')
    print(json.dumps({'file': os.path.basename(f), 'acc': (c.get('epoch_test_accuracy') or [None])[-1],
                      'recip': round(float(recip), 3), 'recip_rand': round(float(dens), 4), 'clustering': round(float(clus), 3),
                      'direct_input_edges_per_readout': round(float(direct.mean()), 2),
                      'readouts_without_direct_input': int((direct == 0).sum()),
                      'input_rf_1hop': round(float(rf1.mean()), 1), 'input_rf_2hop': round(float(rf2.mean()), 1),
                      'absw_recip_vs_oneway': [round(float(np.abs(w[rec_e]).mean()), 3) if rec_e.any() else None, round(float(np.abs(w[~rec_e]).mean()), 3)],
                      'recip_pair_same_sign_frac': round(prod_pos, 3)}))
