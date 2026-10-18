#!/usr/bin/env python3
"""Independent reference for persistent individuals on the LIF and mixed
cores: per-row order (sources -> concentration -> occupancy -> effects ->
modulated core step -> local plastic update), effective weights with fixed
signs, plasticity and the slow consolidation layer. Written from the doc
comments of learning/individual.go, learning/chemistry.go,
learning/consolidation.go, modulation/*.go, plasticity/plasticity.go and the
dynamics cores. Forward only (no gradients cross Advance), float64 except the
Insyra float32 encoder/readout matmuls.

Usage: python3 reference2.py ind.json
"""
import base64
import json
import math
import sys

import numpy as np


def f32mm(a, b):
    """Insyra v0.3.4 MatMul: float32 products accumulated sequentially in
    float32 (numpy's BLAS matmul rounds differently in the last bits)."""
    a = np.asarray(a, dtype=np.float32)
    b = np.asarray(b, dtype=np.float32)
    out = np.zeros((a.shape[0], b.shape[1]), dtype=np.float32)
    for i in range(a.shape[0]):
        for j in range(b.shape[1]):
            acc = np.float32(0)
            for k in range(a.shape[1]):
                acc = np.float32(acc + np.float32(a[i, k] * b[k, j]))
            out[i, j] = acc
    return out.astype(np.float64)


def logistic(x):
    return 1 / (1 + math.exp(-x)) if x >= 0 else math.exp(x) / (1 + math.exp(x))


class Core:
    def __init__(self, cfg, params, neural):
        self.cfg = cfg
        if cfg.get("lif") is not None:
            c = cfg["lif"]
            self.n = c["nodes"]
            self.rule = [1] * self.n
            self.lifc = c
            self.act = None
        else:
            c = cfg["mixed"]
            self.n = c["nodes"]
            raw = c["node_rule"]
            self.rule = list(base64.b64decode(raw)) if isinstance(raw, str) else raw
            self.lifc = c["lif"]
            self.act = c["continuous"]["activation"]
        self.src, self.tgt = c["sources"], c["targets"]
        self.dl = c.get("delays") or [0] * len(self.src)
        self.dt = c["dt"]
        lif_nodes = [i for i in range(self.n) if self.rule[i] == 1]
        self.lif_nodes = lif_nodes
        th = params["theta_raw"]
        lc = self.lifc
        self.base = [0.0] * self.n
        for k, i in enumerate(lif_nodes):
            self.base[i] = lc["theta_min"] + (lc["theta_max"] - lc["theta_min"]) * logistic(th[k])
        self.lam = [math.exp(-self.dt / math.exp(x)) for x in params["core"]["log_tau"]]
        self.alpha = [-math.expm1(-self.dt / math.exp(x)) for x in params["core"]["log_tau"]]
        self.bias = params["core"]["bias"]
        self.kappa = math.exp(-self.dt / lc["tau_syn"])
        ad = lc["adaptation"]
        self.adapting = ad["enabled"]
        self.rho = math.exp(-self.dt / ad["tau_adapt"]) if self.adapting else 0
        self.beta = ad["beta"] if self.adapting else 0
        hb = lc.get("homeostasis")
        self.hb = hb if (hb is not None and hb["enabled"]) else None
        # state from snapshot
        self.load(neural)

    def load(self, neural):
        n = self.n
        if neural["core"] == "lif":
            s = neural["lif"]
            self.steps = s["steps"]
            self.v = list(s["voltage"])
            hist = s["history"]
            self.a = list(s["adaptation"])
            self.refr = list(s["refractory"])
            self.rate = list(s.get("rate") or [0.0] * n)
            self.h = list(s.get("homeostasis") or [0.0] * n)
        else:
            s = neural["mixed"]
            self.steps = s["steps"]
            ci, li = s["index"]["continuous_nodes"], s["index"]["lif_nodes"]
            self.v = [0.0] * n
            self.a, self.refr, self.rate, self.h = [0.0] * n, [0] * n, [0.0] * n, [0.0] * n
            for k, i in enumerate(ci):
                self.v[i] = s["continuous"]["voltage"][k]
            L = s["lif"]
            for k, i in enumerate(li):
                self.v[i] = L["voltage"][k]
                self.a[i] = L["adaptation"][k]
                self.refr[i] = L["refractory"][k]
                if L.get("rate"):
                    self.rate[i] = L["rate"][k]
                    self.h[i] = L["homeostasis"][k]
            hist = []
            for t in range(len(L["history"])):
                row = [0.0] * n
                for k, i in enumerate(ci):
                    row[i] = s["continuous"]["history"][t][k]
                for k, i in enumerate(li):
                    row[i] = L["history"][t][k]
                hist.append(row)
        first = self.steps - (len(hist) - 1)
        self.out = {first + t: list(r) for t, r in enumerate(hist)}

    def phi(self, v):
        if self.act == "tanh":
            return math.tanh(v)
        return max(v, 0) + math.log1p(math.exp(-abs(v)))

    def step(self, inp, weights, gain=None, offset=None, thr=None):
        n, lc = self.n, self.lifc
        step = self.steps
        drive = list(inp)
        for e, (s, t, d) in enumerate(zip(self.src, self.tgt, self.dl)):
            past = step - d if d < step else 0
            drive[t] += weights[e] * self.out[past][s]
        prev = self.out[step]
        out, spk = [0.0] * n, [0.0] * n
        nv, na, nr, nrate, nh = [0.0] * n, [0.0] * n, [0] * n, [0.0] * n, [0.0] * n
        for i in range(n):
            if gain is not None:
                drive[i] = gain[i] * drive[i] + offset[i]
            drive[i] += self.bias[i]
            cand = self.lam[i] * self.v[i] + self.alpha[i] * drive[i]
            if self.rule[i] == 0:
                nv[i] = cand
                out[i] = self.phi(cand)
                continue
            theta = self.base[i]
            if self.adapting:
                theta += self.a[i]
            if self.hb:
                theta += self.h[i]
            if thr is not None and thr[i] != 0:
                theta = max(theta + thr[i], lc["theta_min"])
            if self.refr[i] > 0:
                nv[i] = lc["v_reset"]
                nr[i] = self.refr[i] - 1
            else:
                if cand - theta >= 0:
                    spk[i] = 1.0
                nv[i] = (1 - spk[i]) * cand + spk[i] * lc["v_reset"]
                nr[i] = lc["refractory_steps"] if spk[i] else 0
            out[i] = self.kappa * prev[i] + spk[i]
            if self.adapting:
                na[i] = self.rho * self.a[i] + self.beta * spk[i]
            if self.hb:
                hb = self.hb
                nrate[i] = self.rate[i] + (self.dt / hb["tau_rate"]) * (spk[i] - self.rate[i])
                nh[i] = min(max(self.h[i] + hb["eta"] * self.dt * (nrate[i] - hb["target_rate"]), 0), hb["h_max"])
        self.v, self.a, self.refr, self.rate, self.h = nv, na, nr, nrate, nh
        self.steps += 1
        self.out[self.steps] = out
        return out, spk


class Chem:
    def __init__(self, part):
        c = part["config"]
        self.c = c
        ch = c["chemistry"]
        self.R, self.K, self.dt, self.tau = ch["regions"], ch["channels"], ch["dt"], ch["tau"]
        self.lam = [math.exp(-self.dt / t) for t in self.tau]
        self.alpha = [1 - l for l in self.lam]
        tr = ch.get("transport")
        self.F = tr["fraction"] if tr else None
        self.conc = [list(r) for r in part["state"]["concentration"]]

    def release(self, step):
        rates = [0.0] * self.K
        for k, src in enumerate(self.c["sources"]):
            assert src["kind"] == "external_timeline"
            for e in src["timeline"]["entries"] or []:
                if e["step"] == step and e["channel"] == k:
                    rates[k] = e["rate"]
        return rates

    def row(self, step, nodes):
        q = self.release(step)
        local = [[self.lam[k] * self.conc[r][k] + self.tau[k] * self.alpha[k] * q[k] for k in range(self.K)] for r in range(self.R)]
        if self.F is not None:
            nxt = []
            for s in range(self.R):
                out = sum(self.F[s][t] for t in range(self.R) if t != s)
                nxt.append([local[s][k] * (1 - out) + sum(local[r][k] * self.F[r][s] for r in range(self.R) if r != s) for k in range(self.K)])
            local = nxt
        self.conc = [[max(x, 0.0) for x in r] for r in local]
        recs = self.c["receptors"]
        region = self.c["regions"]["node_region"]
        occ = []  # (receptor, cell, occ)
        for ri, rec in enumerate(recs["records"]):
            for cell in rec["cells"]:
                cval = self.conc[region[cell]][rec["channel"]]
                o = 0.0
                if rec["status"] == "hypothesized":
                    o = 0.0 if cval == 0 else cval ** rec["engineering_n"] / (rec["engineering_kd"] ** rec["engineering_n"] + cval ** rec["engineering_n"])
                occ.append((ri, cell, o))
        mix = recs.get("mix") or "sum"
        gain, offset, thr = [1.0] * nodes, [0.0] * nodes, [0.0] * nodes
        for kind in ("sensitivity", "threshold"):
            effs = [e for e in self.c["effects"] if e["kind"] == kind]
            total, owner = [0.0] * nodes, [None] * nodes
            for e in effs:
                for (ri, cell, o) in occ:
                    if ri != e["receptor"]:
                        continue
                    owner[cell] = owner[cell] or e
                    total[cell] = max(total[cell], o) if mix == "max" else total[cell] + o
            for cell in range(nodes):
                e = owner[cell]
                if e is None:
                    continue
                if kind == "sensitivity":
                    gmin = e.get("gamma_min") or 0.5
                    gmax = e.get("gamma_max") or 2.0
                    bmax = e.get("beta_abs_max") or 1.0
                    gain[cell] = min(max(1 + e.get("gamma_scale", 0) * total[cell], gmin), gmax)
                    offset[cell] = min(max(e.get("beta_scale", 0) * total[cell], -bmax), bmax)
                else:
                    tm = e["theta_abs_max"]
                    thr[cell] = min(max(e.get("theta_scale", 0) * total[cell], -tm), tm)
        nrec = len(recs["records"])
        avg = []
        for ri in range(nrec):
            vals = [o for (r, c, o) in occ if r == ri]
            avg.append(sum(vals) / len(vals) if vals else 0.0)
        return gain, offset, thr, occ, avg


def effective_base(cfg, raw):
    signs = cfg.get("edge_signs") or []
    out = list(raw)
    for i, s in enumerate(signs):
        if s != 0:
            out[i] = s * math.exp(raw[i])
    return out


def plastic_effective(base, signs, pc, fast, slow):
    out = list(base)
    for k, e in enumerate(pc["edges"]):
        s = signs[e] if signs else 0
        sl = slow[k] if slow is not None else 0.0
        if s == 0:
            out[e] = (base[e] + sl) + fast["plastic"][k]
        else:
            mag = (abs(base[e]) + sl) + fast["plastic"][k]
            out[e] = s * max(mag, pc["rule"]["w_min"])
    return out


def plastic_step(pc, fast, pre, post, sp_pre, sp_post, gate, src, tgt):
    rule = pc["rule"]
    stdp = rule["kind"] == "stdp_pair"
    nf = {"eligibility": [], "plastic": []}
    if stdp:
        nf["pre_trace"], nf["post_trace"] = [], []
    for k, e in enumerate(pc["edges"]):
        j, i = src[e], tgt[e]
        el = rule["decay_e"] * fast["eligibility"][k]
        if stdp:
            ptr = rule["decay_pre"] * fast["pre_trace"][k]
            qtr = rule["decay_post"] * fast["post_trace"][k]
            if sp_post[i]:
                el += rule["a_plus"] * ptr
            if sp_pre[j]:
                el -= rule["a_minus"] * qtr
            nf["pre_trace"].append(ptr + (1 if sp_pre[j] else 0))
            nf["post_trace"].append(qtr + (1 if sp_post[i] else 0))
        else:
            el += pre[j] * post[i]
        pl = rule["decay_p"] * fast["plastic"][k] + gate * el
        m = rule["plastic_max"]
        nf["eligibility"].append(el)
        nf["plastic"].append(max(-m, min(m, pl)))
    return nf


def simulate(snapshot, inputs, gates):
    cfg, params = snapshot["config"], snapshot["parameters"]
    core = Core(cfg, params, snapshot["neural"])
    n = core.n
    pp = snapshot.get("plastic")
    fast = pp["state"] if pp else None
    slow = (pp.get("slow") or {}).get("values") if pp else None
    if pp and pp.get("slow") is not None and slow is None:
        slow = []
    chem = Chem(snapshot["chemical"]) if snapshot.get("chemical") else None
    enc = f32mm(inputs, np.asarray(params["encoder"]).reshape(cfg["input_size"], -1))
    base = effective_base(cfg, params["core"]["weights"])
    signs = cfg.get("edge_signs") or []
    readouts, spikes_all = [], []
    for t in range(len(inputs)):
        row = [0.0] * n
        for j, nid in enumerate(cfg["input_nodes"]):
            row[nid] = float(enc[t, j])
        mod = (None, None, None)
        if chem:
            gain, offset, thr, occ, avg = chem.row(core.steps, n)
            mod = (gain, offset, thr)
        w = plastic_effective(base, signs, pp["config"], fast, slow) if pp else base
        out, spk = core.step(row, w, *mod)
        spikes_all.append(spk)
        if pp:
            if cfg.get("lif") is not None:
                post, sp_pre, sp_post = spk, spk, spk
            else:
                post = [spk[i] if core.rule[i] == 1 else out[i] for i in range(n)]
                sp_pre = sp_post = None
            fast = plastic_step(pp["config"], fast, out, post, sp_pre, sp_post, gates[t], cfg["mixed" if cfg.get("mixed") else "lif"]["sources"], cfg["mixed" if cfg.get("mixed") else "lif"]["targets"])
        h = [out[i] for i in cfg["readout_nodes"]]
        readouts.append(f32mm([h], np.asarray(params["readout"]).reshape(len(h), cfg["output_size"]))[0])
    return np.array(readouts), core, fast, chem


def rel(a, b):
    a, b = np.asarray(a, dtype=np.float64), np.asarray(b, dtype=np.float64)
    if a.shape != b.shape:
        return float("inf")
    if a.size == 0:
        return 0.0
    return float(np.max(np.abs(a - b)) / max(np.max(np.abs(a)), np.max(np.abs(b)), 1e-12))


def neural_vector(neural):
    if neural["core"] == "lif":
        s = neural["lif"]
        return s["voltage"], s["history"][-1]
    s = neural["mixed"]
    return s["continuous"]["voltage"] + s["lif"]["voltage"], s["continuous"]["history"][-1] + s["lif"]["history"][-1]


def main():
    data = json.load(open(sys.argv[1]))
    bad = 0
    worst = {}
    for case in data["individual"]:
        issues = []
        snap0 = {"config": case["config"], "parameters": case["params"], "neural": None}
        # Rebuild the fresh snapshot: zero synaptic history, initial voltage.
        cfg = case["config"]
        core_cfg = cfg.get("lif") or cfg.get("mixed")
        n = core_cfg["nodes"]
        if cfg.get("lif") is not None:
            hb = cfg["lif"].get("homeostasis")
            on = hb is not None and hb["enabled"]
            snap0["neural"] = {"core": "lif", "lif": {"steps": 0, "voltage": case["initial"], "history": [[0.0] * n], "adaptation": [0.0] * n, "refractory": [0] * n,
                                                      "rate": [0.0] * n if on else None, "homeostasis": [0.0] * n if on else None}}
        else:
            raw = cfg["mixed"]["node_rule"]
            rule = list(base64.b64decode(raw)) if isinstance(raw, str) else raw
            ci = [i for i in range(n) if rule[i] == 0]
            li = [i for i in range(n) if rule[i] == 1]
            act = cfg["mixed"]["continuous"]["activation"]
            phi = (lambda v: math.tanh(v)) if act == "tanh" else (lambda v: max(v, 0) + math.log1p(math.exp(-abs(v))))
            hb = cfg["mixed"]["lif"].get("homeostasis")
            on = hb is not None and hb["enabled"]
            snap0["neural"] = {"core": "mixed", "mixed": {"steps": 0, "index": {"continuous_nodes": ci, "lif_nodes": li},
                                                          "continuous": {"voltage": [case["initial"][i] for i in ci], "history": [[phi(case["initial"][i]) for i in ci]]},
                                                          "lif": {"voltage": [case["initial"][i] for i in li], "history": [[0.0] * len(li)], "adaptation": [0.0] * len(li), "refractory": [0] * len(li),
                                                                  "rate": [0.0] * len(li) if on else None, "homeostasis": [0.0] * len(li) if on else None}}}
        if case["plasticity"]:
            pc = case["plasticity"]
            K = len(pc["edges"])
            st = {"eligibility": [0.0] * K, "plastic": [0.0] * K}
            if pc["rule"]["kind"] == "stdp_pair":
                st["pre_trace"], st["post_trace"] = [0.0] * K, [0.0] * K
            snap0["plastic"] = {"config": pc, "state": st, "slow": None}
        if case["chemistry"]:
            cc = case["chemistry"]
            snap0["chemical"] = {"config": cc, "state": {"concentration": [[0.0] * cc["chemistry"]["channels"] for _ in range(cc["chemistry"]["regions"])]}}
        out, core, fast, chem = simulate(snap0, case["input"], case["gate"])
        e = rel(out, case["outputs"])
        worst["readout"] = max(worst.get("readout", 0), e)
        if e > 1e-5:
            issues.append(f"readout rel err {e:.3g}")
        fin = case["final"]
        v, x = neural_vector(fin["neural"])
        if fin["neural"]["core"] == "lif":
            rv, rx = core.v, core.out[core.steps]
        else:
            ci, li = fin["neural"]["mixed"]["index"]["continuous_nodes"], fin["neural"]["mixed"]["index"]["lif_nodes"]
            rv = [core.v[i] for i in ci] + [core.v[i] for i in li]
            rx = [core.out[core.steps][i] for i in ci] + [core.out[core.steps][i] for i in li]
        for name, a, b in [("voltage", rv, v), ("trace", rx, x)]:
            e = rel(a, b)
            worst[name] = max(worst.get(name, 0), e)
            if e > 1e-9:
                issues.append(f"final {name} rel err {e:.3g}")
        if fast is not None:
            for key in fast:
                e = rel(fast[key], fin["plastic"]["state"][key])
                worst["plastic." + key] = max(worst.get("plastic." + key, 0), e)
                if e > 1e-9:
                    issues.append(f"final plastic {key} rel err {e:.3g}")
        if chem is not None:
            e = rel(chem.conc, fin["chemical"]["state"]["concentration"])
            worst["conc"] = max(worst.get("conc", 0), e)
            if e > 1e-12:
                issues.append(f"final concentration rel err {e:.3g}")
        for k, val in case["checks"].items():
            if val is not True:
                issues.append(f"go invariant {k} failed")
        cons = case.get("consolidation")
        if cons:
            before, after, rep = cons["before"], cons["after"], cons["report"]
            pc = case["plasticity"]
            rate_retain = None
            # recover Rate and Retain from the report-independent formula check
            pb = before["state"]["plastic"]
            sb = before["slow"]["values"]
            sa = after["slow"]["values"]
            pa = after["state"]["plastic"]
            # slow_after - slow_before = Rate * plastic_before; plastic_after = Retain*plastic_before
            ratios = [(sa[k] - sb[k]) / pb[k] for k in range(len(pb)) if abs(pb[k]) > 1e-9]
            rets = [pa[k] / pb[k] for k in range(len(pb)) if abs(pb[k]) > 1e-9]
            if ratios and (max(ratios) - min(ratios) > 1e-9 or max(rets) - min(rets) > 1e-9):
                issues.append("consolidation is not slow += Rate*plastic, plastic *= Retain")
            if rep.get("applied"):
                if abs(rep.get("slow_l2", 0) - math.sqrt(sum(s * s for s in sa))) > 1e-12:
                    issues.append("slow_l2 mismatch")
                if abs(rep.get("plastic_l2", 0) - math.sqrt(sum(s * s for s in pa))) > 1e-12:
                    issues.append("plastic_l2 mismatch")
            # next advance integrates (base + slow) + plastic
            nxt, _, _, _ = simulate(cons["state_before_next"], cons["next_input"], [0, 0, 0])
            e = rel(nxt, cons["next_out"])
            worst["post_consolidation_readout"] = max(worst.get("post_consolidation_readout", 0), e)
            if e > 1e-5:
                issues.append(f"post-consolidation readout rel err {e:.3g}")
        if issues:
            bad += 1
            print(f"[{case['kind']} seed={case['seed']} plastic={case['plasticity'] is not None} chem={case['chemistry'] is not None}]", "; ".join(issues[:6]))
    print(f"individual cases passing: {len(data['individual']) - bad}/{len(data['individual'])}")
    print("worst:", {k: f"{v:.3g}" for k, v in worst.items()})
    r = data["repros"]
    print("R1 gain at zero occupancy (doc says 1):", r["R1_gamma_bounds_exclude_one"]["gain"], r["R1b_gamma_max_below_one"]["gain"], "validate:", r["R1c_validate"])
    print("R2 neural_activity source, fresh individual:", r["R2_neural_source_fresh_individual"])
    print("R3 threshold floor spikes (expect 0):", r["R3_threshold_floor"])


if __name__ == "__main__":
    main()
