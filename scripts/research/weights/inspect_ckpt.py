"""Weight analysis of seqkmnist checkpoints (continuous softsign-gated core).

Reimplements the forward pass in numpy (float64) from a checkpoint and reports
the recomputed accuracy next to the recorded one, so every statistic below is
taken on the model that was actually trained. Supports the zero-delay
continuous core with the softsign current gate and softsign activation, without
chemistry or Dale signs (those checkpoints are refused).

usage: KMNIST_DIR=data/TSK-11/kmnist LIMIT=2000 python3 inspect_ckpt.py CKPT.json...
prints one JSON line per checkpoint.
"""
import gzip, json, sys, os
import numpy as np
from collections import deque

DATA = os.environ.get('KMNIST_DIR', 'data/TSK-11/kmnist')


def idx(path, off):
    return np.frombuffer(gzip.open(path).read(), dtype=np.uint8, offset=off)


def load_split(split, limit=None):
    y = idx(f'{DATA}/{split}-labels-idx1-ubyte.gz', 8).astype(int)
    x = idx(f'{DATA}/{split}-images-idx3-ubyte.gz', 16).reshape(-1, 28, 28).astype(np.float64) / 255
    if limit:
        x, y = x[:limit], y[:limit]
    # step t = column t, top to bottom: seq[b, t, row] = img[b, row, t]
    return np.transpose(x, (0, 2, 1)).copy(), y


class Model:
    def __init__(self, path):
        c = json.load(open(path))
        self.settings = c['settings']
        self.recorded = (c.get('epoch_test_accuracy') or [None])[-1]
        t = c['trainer']
        cfg, p = t['config'], t['parameters']
        d = cfg['dynamics']
        self.n = d['nodes']
        self.src = np.array(d['sources']); self.dst = np.array(d['targets'])
        if set(d['delays']) != {0} or d.get('chemistry') or self.settings.get('inhibitory') or cfg.get('sharing'):
            raise SystemExit(f'{path}: delays, chemistry, Dale signs and tied parameters are not modelled here')
        assert d['gate'] == 'current_softsign' and d['activation'] == 'softsign'
        self.inp = np.array(cfg['input_nodes']); self.ro = np.array(cfg['readout_nodes'])
        core = p['core']
        self.w = np.array(core['weights']); self.b = np.array(core['bias'])
        self.logtau = np.array(core['log_tau']); self.gg = np.array(core['gate_gain']); self.gb = np.array(core['gate_bias'])
        self.enc = np.array(p['encoder']).reshape(cfg['input_size'], len(self.inp))
        self.rd = np.array(p['readout']).reshape(len(self.ro), cfg['output_size'])
        self.tau = np.exp(self.logtau); self.alpha = -np.expm1(-1 / self.tau)

    def W(self, w=None):
        w = self.w if w is None else w
        M = np.zeros((self.n, self.n))
        np.add.at(M, (self.src, self.dst), w)  # drive = y @ M
        return M

    def run(self, x, w=None, neuron_mask=None, record=False, ro_mask=None):
        B, T, _ = x.shape
        M = self.W(w)
        v = np.zeros((B, self.n)); y = np.zeros((B, self.n))
        rec = {'y': [], 'a': [], 'sq': []} if record else None
        for t in range(T):
            drive = y @ M
            drive[:, self.inp] += x[:, t] @ self.enc
            drive += self.b
            z = self.gg * drive + self.gb
            sq = (1 + z / (1 + np.abs(z))) / 2
            a = np.minimum(1, 2 * self.alpha * sq)
            v = (1 - a) * v + a * drive
            y = v / (1 + np.abs(v))
            if neuron_mask is not None:
                y = y * neuron_mask
            if record:
                rec['y'].append(y); rec['a'].append(a); rec['sq'].append(sq)
        yr = y[:, self.ro]
        if ro_mask is not None:
            yr = yr * ro_mask
        logits = yr @ self.rd
        if record:
            rec = {k: np.stack(v, 1) for k, v in rec.items()}  # B,T,n
        return logits, rec


def acc(logits, y):
    return float((logits.argmax(1) == y).mean())


def hops(n, src, dst, starts):
    adj = [[] for _ in range(n)]
    for s, d in zip(src, dst):
        adj[s].append(d)
    dist = np.full(n, -1); q = deque()
    for s in starts:
        dist[s] = 0; q.append(s)
    while q:
        u = q.popleft()
        for v in adj[u]:
            if dist[v] < 0:
                dist[v] = dist[u] + 1; q.append(v)
    return dist


def analyze(path, x, y, deep=True):
    m = Model(path)
    out = {'file': os.path.basename(path), 'n': m.n, 'edges': len(m.w), 'recorded_test': m.recorded}
    logits, rec = m.run(x, record=True)
    out['acc'] = acc(logits, y)
    # --- graph structure
    din = hops(m.n, m.src, m.dst, m.inp)          # hops from input
    dout = hops(m.n, m.dst, m.src, m.ro)          # hops to readout (reverse graph)
    on_path = (din >= 0) & (dout >= 0)
    out['unreachable_from_input'] = int((din < 0).sum())
    out['cannot_reach_readout'] = int((dout < 0).sum())
    out['not_on_input_readout_path'] = int((~on_path).sum())
    ro_d = din[m.ro]
    out['readout_hops_from_input'] = {int(k): int(v) for k, v in zip(*np.unique(ro_d, return_counts=True))}
    edge_useful = on_path[m.src] & on_path[m.dst]
    out['edges_off_path'] = int((~edge_useful).sum())
    indeg = np.bincount(m.dst, minlength=m.n); outdeg = np.bincount(m.src, minlength=m.n)
    out['indeg_mean_sd_max'] = [float(indeg.mean()), float(indeg.std()), int(indeg.max())]
    out['outdeg_mean_sd_max'] = [float(outdeg.mean()), float(outdeg.std()), int(outdeg.max())]
    out['zero_outdeg'] = int((outdeg == 0).sum()); out['zero_indeg'] = int((indeg == 0).sum())
    out['inp_ro_overlap'] = int(len(set(m.inp) & set(m.ro)))
    # reciprocity
    es = set(zip(m.src.tolist(), m.dst.tolist()))
    out['reciprocal_frac'] = float(np.mean([(d, s) in es for s, d in es]))
    out['self_loops'] = int((m.src == m.dst).sum())
    # --- tau / gate
    out['tau_pct'] = np.percentile(m.tau, [5, 25, 50, 75, 95]).round(3).tolist()
    out['gate_cap_2alpha_pct'] = np.percentile(np.minimum(1, 2 * m.alpha), [5, 50, 95]).round(3).tolist()
    a = rec['a']; yy = rec['y']
    out['gate_mean'] = float(a.mean())
    out['gate_clipped_frac'] = float((a >= 1 - 1e-12).mean())
    per_neuron_gate = a.mean((0, 1))
    out['gate_per_neuron_pct'] = np.percentile(per_neuron_gate, [5, 25, 50, 75, 95]).round(3).tolist()
    # temporal variability of the gate within a sequence (is it used as a gate, or constant?)
    out['gate_std_over_time_mean'] = float(a.std(1).mean())
    out['gate_std_over_samples_mean'] = float(a.std(0).mean())
    # effective memory: product of (1-a) means retention; effective timescale 1/mean(a)
    eff_tau = 1 / np.maximum(per_neuron_gate, 1e-6)
    out['eff_timescale_pct'] = np.percentile(eff_tau, [5, 25, 50, 75, 95]).round(2).tolist()
    # --- activity
    ystd = yy.reshape(-1, m.n).std(0)
    out['dead_neurons(std<0.01)'] = int((ystd < 0.01).sum())
    out['saturated_frac(|y|>0.9)'] = float((np.abs(yy) > 0.9).mean())
    out['y_abs_mean'] = float(np.abs(yy).mean())
    # --- sign structure
    out['w_pos_frac'] = float((m.w > 0).mean())
    out['w_abs_pct'] = np.percentile(np.abs(m.w), [10, 50, 90, 99]).round(4).tolist()
    # per-source sign consistency vs binomial null (Dale-like emergence?)
    cons = []; null = []
    rng = np.random.default_rng(0)
    p = (m.w > 0).mean()
    for s in range(m.n):
        ws = m.w[m.src == s]
        if len(ws) >= 5:
            f = (ws > 0).mean(); cons.append(max(f, 1 - f))
            g = (rng.random(len(ws)) < p).mean(); null.append(max(g, 1 - g))
    out['src_sign_consistency_vs_null'] = [float(np.mean(cons)), float(np.mean(null))]
    # --- readout
    rn = np.linalg.norm(m.rd, axis=1)
    out['readout_row_norm_pct'] = np.percentile(rn, [5, 50, 95]).round(3).tolist()
    # readout importance ~ norm * std(y at last step)
    yl = yy[:, -1, :][:, m.ro]
    imp = rn * yl.std(0)
    srt = np.sort(imp)[::-1]
    out['readout_imp_top_share_25pct'] = float(srt[:len(srt) // 4].sum() / srt.sum())
    # --- correlations: which node properties track hop distance
    out['tau_by_hop_from_input'] = {int(h): float(np.median(m.tau[din == h])) for h in np.unique(din) if (din == h).sum() >= 5}
    out['gate_by_hop_from_input'] = {int(h): float(np.median(per_neuron_gate[din == h])) for h in np.unique(din) if (din == h).sum() >= 5}
    out['tau_input_vs_readout_vs_hidden'] = [float(np.median(m.tau[m.inp])), float(np.median(m.tau[m.ro])), float(np.median(np.delete(m.tau, np.r_[m.inp, m.ro])))]
    if deep:
        # pruning sensitivity: magnitude vs saliency (|w| * std(y_src))
        sal = np.abs(m.w) * ystd[m.src]
        res = {}
        for frac in [0.25, 0.5, 0.75, 0.9]:
            for name, score in [('mag', np.abs(m.w)), ('sal', sal)]:
                k = int(frac * len(m.w))
                cut = np.argsort(score)[:k]
                w2 = m.w.copy(); w2[cut] = 0
                res[f'{name}{int(frac*100)}'] = round(acc(m.run(x, w=w2)[0], y), 4)
        res['offpath_edges_zeroed'] = round(acc(m.run(x, w=np.where(edge_useful, m.w, 0))[0], y), 4)
        out['prune_acc'] = res
        # neuron-group ablation by hop distance from input
        grp = {}
        hidden = np.ones(m.n, bool); hidden[m.inp] = False; hidden[m.ro] = False
        for h in sorted(set(din[hidden].tolist())):
            mask = np.ones(m.n); mask[hidden & (din == h)] = 0
            grp[int(h)] = [int((hidden & (din == h)).sum()), round(acc(m.run(x, neuron_mask=mask)[0], y), 4)]
        mask = np.ones(m.n); mask[hidden] = 0
        grp['all_hidden'] = [int(hidden.sum()), round(acc(m.run(x, neuron_mask=mask)[0], y), 4)]
        out['ablate_hidden_by_hop'] = grp
    return out


if __name__ == '__main__':
    limit = int(os.environ.get('LIMIT', '2000'))
    deep = os.environ.get('DEEP', '1') == '1'
    x, y = load_split('t10k', limit)
    for f in sys.argv[1:]:
        r = analyze(f, x, y, deep)
        print(json.dumps(r), flush=True)
