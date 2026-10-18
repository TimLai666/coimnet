#!/usr/bin/env python3
"""Independent PyTorch reference for the CoImNet spiking/mixed/vector cores,
the learning.Network boundary on those cores, and the plasticity rules.

The math here is written from the documentation (doc comments in dynamics/,
plasticity/, docs/model-and-mechanisms.md), not by transliterating the Go
reverse passes: gradients come from torch autograd, with the declared
fast_sigmoid surrogate as a custom autograd function, the reset detached and
the slow stabiliser offset treated as a constant.

Usage: python3 reference.py cases.json
"""
import base64
import json
import math
import sys

import torch

torch.set_default_dtype(torch.float64)
F64 = torch.float64


class FastSigmoidSpike(torch.autograd.Function):
    """Hard 0/1 event forward; backward psi(u) = 1/(1+scale*|u|)^2."""

    @staticmethod
    def forward(ctx, u, scale):
        ctx.save_for_backward(u)
        ctx.scale = scale
        return (u >= 0).to(u.dtype)

    @staticmethod
    def backward(ctx, g):
        (u,) = ctx.saved_tensors
        psi = 1.0 / (1.0 + ctx.scale * u.abs()) ** 2
        return g * psi, None


spike_fn = FastSigmoidSpike.apply


def T(x, grad=False):
    t = torch.tensor(x, dtype=F64)
    if grad:
        t.requires_grad_(True)
    return t


def leak(logtau, dt):
    tau = torch.exp(logtau)
    lam = torch.exp(-dt / tau)
    alpha = -torch.expm1(-dt / tau)
    return lam, alpha


# OFFSET is the current case's truncation offset (boundaries at OFFSET,
# OFFSET+W, ...); the run_*_case functions set it before replaying a case.
OFFSET = 0


def window_start(t, W):
    if W <= 0 or t < OFFSET:
        return 0
    return OFFSET + ((t - OFFSET) // W) * W


def edge_drive(n, C, sources, targets, delays, out_rows, weights, t, W, matrix=False):
    """sum_e w_e * out_src(t - d_e) into the targets; reads of rows at or
    before the current window start are detached (truncated BPTT)."""
    s0 = window_start(t, W)
    contrib = [torch.zeros(C, dtype=F64) for _ in range(n)]
    for e, (s, tg, d) in enumerate(zip(sources, targets, delays)):
        past = t - d if d < t else 0
        val = out_rows[past][s * C:(s + 1) * C] if C > 1 else out_rows[past][s:s + 1]
        if W > 0 and s0 > 0 and past <= s0:
            val = val.detach()
        if matrix:
            Wm = weights[e * C * C:(e + 1) * C * C].reshape(C, C)
            contrib[tg] = contrib[tg] + Wm @ val
        else:
            contrib[tg] = contrib[tg] + weights[e] * val
    return torch.cat(contrib)


# ---------------------------------------------------------------- LIF / mixed
def spiking_forward(n, rule, cfg_lif, act, dt, sources, targets, delays, weights, bias, logtau,
                    theta_raw_full, initial, inputs, W=0, mod=None):
    """Mixed forward; an all-LIF rule is the LIF core. rule[i]==1 is LIF.
    Returns dict with outputs (list of rows 1..T), spikes, voltages."""
    lam, alpha = leak(logtau, dt)
    is_lif = torch.tensor([r == 1 for r in rule])
    if cfg_lif is not None:
        tmin, tmax = cfg_lif["theta_min"], cfg_lif["theta_max"]
        base = tmin + (tmax - tmin) * torch.sigmoid(theta_raw_full)
        kappa = math.exp(-dt / cfg_lif["tau_syn"])
        ad = cfg_lif["adaptation"]
        adapting = ad["enabled"]
        rho = math.exp(-dt / ad["tau_adapt"]) if adapting else 0.0
        beta = ad["beta"] if adapting else 0.0
        hb = cfg_lif.get("homeostasis")
        homeo = hb is not None and hb["enabled"]
        vreset = cfg_lif["v_reset"]
        R = cfg_lif["refractory_steps"]
        scale = cfg_lif["surrogate"]["scale"]
    def phi(v):
        return torch.tanh(v) if act == "tanh" else softplus(v)
    v = initial
    out0 = torch.where(is_lif, torch.zeros(n), phi(initial)) if act else torch.zeros(n)
    out_rows = [out0]
    a = torch.zeros(n)
    rate = torch.zeros(n)
    h = torch.zeros(n)
    refr = torch.zeros(n, dtype=torch.long)
    outs, spikes, volts = [], [], []
    for t in range(len(inputs)):
        s0 = window_start(t, W)
        prev_out = out_rows[t]
        if W > 0 and t == s0 and s0 > 0:
            v = v.detach()
            a = a.detach()
            prev_out = prev_out.detach()
        drive = inputs[t] + edge_drive(n, 1, sources, targets, delays, out_rows, weights, t, W)
        if mod is not None:
            if mod.get("gain") is not None:
                drive = T(mod["gain"][t]) * drive
            if mod.get("offset") is not None:
                drive = drive + T(mod["offset"][t])
        drive = drive + bias
        cand = lam * v + alpha * drive
        if cfg_lif is not None:
            theta = base.clone()
            if adapting:
                theta = theta + a
            if homeo:
                theta = theta + h  # constant of the reverse pass (h carries no grad)
            if mod is not None and mod.get("threshold") is not None:
                shift = T(mod["threshold"][t])
                theta = torch.where(shift != 0, torch.clamp(theta + shift, min=tmin), theta)
            u = cand - theta
            spk = spike_fn(u, scale)
            refractory = refr > 0
            spk = torch.where(refractory | ~is_lif, torch.zeros(n), spk)
            sd = spk.detach()
            v_lif = torch.where(refractory, torch.full((n,), vreset), (1 - sd) * cand + sd * vreset)
            new_refr = torch.where(refractory, refr - 1, torch.where(sd > 0, torch.full((n,), R, dtype=torch.long), torch.zeros(n, dtype=torch.long)))
            out_lif = kappa * prev_out + spk
            a_new = rho * a + beta * spk if adapting else a
            if homeo:
                rate_new = rate + (dt / hb["tau_rate"]) * (sd - rate)
                h_new = torch.clamp(h + hb["eta"] * dt * (rate_new - hb["target_rate"]), 0, hb["h_max"])
            else:
                rate_new, h_new = rate, h
        else:
            spk = torch.zeros(n)
            v_lif, out_lif, a_new, rate_new, h_new, new_refr = cand, cand, a, rate, h, refr
        v_new = torch.where(is_lif, v_lif, cand)
        out_new = torch.where(is_lif, out_lif, phi(cand) if act else cand)
        # Only LIF nodes own adaptation, rate and offset state.
        a = torch.where(is_lif, a_new, torch.zeros(n))
        rate = torch.where(is_lif, rate_new, torch.zeros(n))
        h = torch.where(is_lif, h_new, torch.zeros(n))
        refr = torch.where(is_lif, new_refr, torch.zeros(n, dtype=torch.long))
        v = v_new
        out_rows.append(out_new)
        outs.append(out_new)
        spikes.append(spk)
        volts.append(v_new)
    return {"outputs": outs, "spikes": spikes, "voltages": volts}


def softplus(v):
    # exact softplus; torch.nn.functional.softplus switches to the identity above
    # its threshold (20), which is an approximation the Go core does not make.
    return torch.clamp(v, min=0) + torch.log1p(torch.exp(-v.abs()))


def rel_err(a, b):
    a = torch.as_tensor(a, dtype=F64)
    b = torch.as_tensor(b, dtype=F64)
    if a.shape != b.shape:
        return float("inf")
    if a.numel() == 0:
        return 0.0
    d = (a - b).abs().max().item()
    s = max(a.abs().max().item(), b.abs().max().item(), 1e-12)
    return d / s


def node_rule_from(case, n):
    raw = case.get("node_rule")
    if raw is None:
        return [1] * n
    if isinstance(raw, str):
        return list(base64.b64decode(raw))
    return raw


def run_spiking_case(case, family):
    global OFFSET
    OFFSET = case.get("offset", 0)
    cfg = case["config"]
    n = cfg["nodes"]
    if family == "lif":
        rule = [1] * n
        cfg_lif = cfg
        act = None
    else:
        rule = node_rule_from(case, n)
        cfg_lif = cfg["lif"]
        act = cfg["continuous"]["activation"]
    p = case["params"]
    lif_nodes = [i for i in range(n) if rule[i] == 1]
    issues = []
    results = {}
    for W in [0, case["window"]]:
        w = T(p["weights"], True)
        b = T(p["bias"], True)
        lt = T(p["log_tau"], True)
        th = T(p["theta_raw"], True)
        full_th = torch.zeros(n)
        full_th = full_th.index_put((torch.tensor(lif_nodes, dtype=torch.long),), th) if lif_nodes else full_th
        init = T(case["initial"], True)
        inputs = [T(r) for r in case["inputs"]]
        res = spiking_forward(n, rule, cfg_lif, act, cfg["dt"], cfg["sources"], cfg["targets"], cfg["delays"] or [0] * len(cfg["sources"]),
                              w, b, lt, full_th, init, inputs, W=W)
        if W == 0:
            for key, go_key in [("outputs", "outputs"), ("spikes", "spikes"), ("voltages", "voltages")]:
                e = rel_err(torch.stack(res[key]).detach(), case[go_key])
                results[f"fwd_{key}"] = e
                if e > 1e-12:
                    issues.append(f"forward {key} rel err {e:.3g}")
        up = [T(r) for r in case["upstream"]]
        L = sum((u * o).sum() for u, o in zip(up, res["outputs"]))
        inp_leaves = inputs
        for x in inp_leaves:
            x.requires_grad_(False)
        # input gradients: rebuild with grad-enabled inputs
        inputs_g = [T(r, True) for r in case["inputs"]]
        w2, b2, lt2, th2, init2 = T(p["weights"], True), T(p["bias"], True), T(p["log_tau"], True), T(p["theta_raw"], True), T(case["initial"], True)
        full2 = torch.zeros(n)
        full2 = full2.index_put((torch.tensor(lif_nodes, dtype=torch.long),), th2) if lif_nodes else full2
        res2 = spiking_forward(n, rule, cfg_lif, act, cfg["dt"], cfg["sources"], cfg["targets"], cfg["delays"] or [0] * len(cfg["sources"]),
                               w2, b2, lt2, full2, init2, inputs_g, W=W)
        L2 = sum((u * o).sum() for u, o in zip(up, res2["outputs"]))
        L2.backward()
        g = case["grads"][str(W)]
        for name, leaf in [("Weights", w2), ("Bias", b2), ("LogTau", lt2), ("ThetaRaw", th2), ("Initial", init2)]:
            ref = leaf.grad if leaf.grad is not None else torch.zeros_like(leaf)
            e = rel_err(ref, g[name])
            results[f"W{W}_{name}"] = e
            if e > 1e-9:
                issues.append(f"window {W} grad {name} rel err {e:.3g}")
        refin = torch.stack([x.grad if x.grad is not None else torch.zeros(n) for x in inputs_g])
        e = rel_err(refin, g["Inputs"])
        results[f"W{W}_Inputs"] = e
        if e > 1e-9:
            issues.append(f"window {W} grad Inputs rel err {e:.3g}")
        if family == "lif":
            if not g.get("recompute_equal_%d" % W, case["grads"].get("recompute_equal_%d" % W, True)):
                issues.append(f"BackwardRecompute != Backward (window {W})")
    # modulated advance
    mod = case["modulation"]
    with torch.no_grad():
        full = torch.zeros(n)
        if lif_nodes:
            full[lif_nodes] = T(p["theta_raw"])
        res = spiking_forward(n, rule, cfg_lif, act, cfg["dt"], cfg["sources"], cfg["targets"], cfg["delays"] or [0] * len(cfg["sources"]),
                              T(p["weights"]), T(p["bias"]), T(p["log_tau"]), full, T(case["initial"]), [T(r) for r in case["inputs"]], W=0, mod=mod)
    for key, go_key in [("outputs", "mod_outputs"), ("spikes", "mod_spikes")]:
        e = rel_err(torch.stack(res[key]), case[go_key])
        results[f"mod_{key}"] = e
        if e > 1e-12:
            issues.append(f"modulated {key} rel err {e:.3g}")
    for k, v in case["checks"].items():
        if v is not True:
            issues.append(f"go invariant {k} failed")
    for k, v in case["grads"].items():
        if k.startswith("recompute_equal") and v is not True:
            issues.append(f"go invariant {k} failed")
    return issues, results


# ---------------------------------------------------------------- vector
def vector_forward(cfg, w, b, lt, init, inputs, W):
    n, C = cfg["nodes"], cfg["state_dimension"]
    matrix = cfg.get("edge_shape") == "matrix"
    lam, alpha = leak(lt, cfg["dt"])
    lamC = lam.repeat_interleave(C)
    alphaC = alpha.repeat_interleave(C)
    act = torch.tanh if cfg["activation"] == "tanh" else softplus
    v = init
    rows = [act(init)]
    delays = cfg.get("delays") or [0] * len(cfg["sources"])
    for t in range(len(inputs)):
        s0 = window_start(t, W)
        if W > 0 and t == s0 and s0 > 0:
            v = v.detach()
        d = inputs[t] + edge_drive(n, C, cfg["sources"], cfg["targets"], delays, rows, w, t, W, matrix) + b
        v = lamC * v + alphaC * d
        rows.append(act(v))
    return rows


def run_vector_case(case):
    global OFFSET
    OFFSET = case.get("offset", 0)
    cfg = case["config"]
    p = case["params"]
    issues, results = [], {}
    for W in [0, case["window"]]:
        w, b, lt, init = T(p["weights"], True), T(p["bias"], True), T(p["log_tau"], True), T(case["initial"], True)
        inputs = [T(r, True) for r in case["inputs"]]
        rows = vector_forward(cfg, w, b, lt, init, inputs, W)
        if W == 0:
            e = rel_err(torch.stack(rows).detach(), case["outputs"])
            results["fwd"] = e
            if e > 1e-12:
                issues.append(f"vector forward rel err {e:.3g}")
        L = sum((T(u) * o).sum() for u, o in zip(case["upstream"], rows[1:]))
        L.backward()
        g = case["grads"][str(W)]
        for name, leaf in [("Weights", w), ("Bias", b), ("LogTau", lt), ("Initial", init)]:
            e = rel_err(leaf.grad, g[name])
            results[f"W{W}_{name}"] = e
            if e > 1e-9:
                issues.append(f"vector window {W} grad {name} rel err {e:.3g}")
        e = rel_err(torch.stack([x.grad for x in inputs]), g["Inputs"])
        if e > 1e-9:
            issues.append(f"vector window {W} grad Inputs rel err {e:.3g}")
    for k, v in case["checks"].items():
        if v is not True:
            issues.append(f"go invariant {k} failed")
    return issues, results


# ---------------------------------------------------------------- network
def run_net_case(case):
    global OFFSET
    OFFSET = 0
    cfg = case["config"]
    p = case["params"]
    kind = case["kind"]
    W = case["window"]
    issues, results = [], {}
    if kind == "vector":
        core = cfg["dynamics"]
        n, C = core["nodes"], core["state_dimension"]
    elif kind == "lif":
        core = cfg["lif"]
        n, C = core["nodes"], 1
    else:
        core = cfg["mixed"]
        n, C = core["nodes"], 1
    signs = cfg.get("edge_signs")
    if isinstance(signs, str):  # []int8 marshals as numbers, but be safe
        signs = list(base64.b64decode(signs))
    in_nodes = cfg["input_nodes"]
    ro_nodes = cfg["readout_nodes"]
    out_size = cfg["output_size"]

    def build(every, upstream=None):
        x = T(case["input"], True)
        E = T(p["encoder"], True)
        Rm = T(p["readout"], True)
        raw_w = T(p["core"]["weights"], True)
        b = T(p["core"]["bias"], True)
        lt = T(p["core"]["log_tau"], True)
        th = T(p["theta_raw"], True) if p.get("theta_raw") else None
        if signs:
            sg = torch.tensor([float(s) for s in signs])
            w = torch.where(sg != 0, sg * torch.exp(raw_w), raw_w)
        else:
            w = raw_w
        steps = len(case["input"])
        width = len(in_nodes) * C
        # Insyra encoder matmul is float32; reproduce its rounding of the core input.
        enc32 = x.float() @ E.reshape(cfg["input_size"], width).float()
        enc = enc32.double()
        inputs = []
        for t in range(steps):
            row = [torch.zeros((), dtype=F64)] * (n * C)
            row = list(row)
            for j, nid in enumerate(in_nodes):
                for k in range(C):
                    row[nid * C + k] = enc[t, j * C + k]
            inputs.append(torch.stack(row))
        if kind == "vector":
            rows = vector_forward(core, w, b, lt, torch.zeros(n * C), inputs, W)[1:]
        else:
            if kind == "lif":
                rule, cl, act = [1] * n, core, None
                lif_nodes = list(range(n))
            else:
                rule = node_rule_from({"node_rule": core["node_rule"]}, n)
                cl, act = core["lif"], core["continuous"]["activation"]
                lif_nodes = [i for i in range(n) if rule[i] == 1]
            full = torch.zeros(n)
            full = full.index_put((torch.tensor(lif_nodes, dtype=torch.long),), th)
            rows = spiking_forward(n, rule, cl, act, core["dt"], core["sources"], core["targets"], core.get("delays") or [0] * len(core["sources"]),
                                   w, b, lt, full, torch.zeros(n), inputs, W=W)["outputs"]
        Rmat = Rm.reshape(len(ro_nodes) * C, out_size)
        def sel(row):
            return torch.cat([row[i * C:(i + 1) * C] for i in ro_nodes])
        if every:
            pred = torch.stack([sel(r) for r in rows]) @ Rmat
            L = (T(upstream) * pred).sum()
        else:
            pred = sel(rows[-1]) @ Rmat
            L = ((pred - T(case["target"])) ** 2).mean()
        L.backward()
        leaves = {"Encoder": E, "Readout": Rm, "Weights": raw_w, "Bias": b, "LogTau": lt, "Inputs": x}
        if th is not None:
            leaves["ThetaRaw"] = th
        return L, pred, leaves

    L, pred, leaves = build(False)
    e = abs(L.item() - case["loss"]) / max(abs(case["loss"]), 1e-6)
    results["loss"] = e
    if e > 1e-4:
        issues.append(f"net loss rel err {e:.3g} (ref {L.item():.6g} go {case['loss']:.6g})")
    e = rel_err(pred.detach(), case["pred"])
    if e > 1e-5:
        issues.append(f"net pred rel err {e:.3g}")
    for label, grad in [("last", case["grad"]), ("every", case["grad_every"])]:
        if label == "every":
            L, pred, leaves = build(True, case["upstream_every"])
        for name, leaf in leaves.items():
            if name in ("Weights", "Bias", "LogTau"):
                go = grad["Core"][name]
            else:
                go = grad[name]
            ref = leaf.grad if leaf.grad is not None else torch.zeros_like(leaf)
            if go is None:
                go = [0.0] * ref.numel()
            e = rel_err(ref.reshape(-1), torch.tensor(go, dtype=F64).reshape(-1))
            results[f"{label}_{name}"] = e
            if e > 1e-4:
                issues.append(f"net[{label}] grad {name} rel err {e:.3g}")
    return issues, results


# ---------------------------------------------------------------- plasticity
def run_plastic_case(case):
    rule = case["rule"]
    edges, src, tgt = case["edges"], case["sources"], case["targets"]
    stdp = rule["kind"] == "stdp_pair"
    K = len(edges)
    elig = [0.0] * K
    plastic = [0.0] * K
    pre_tr = [0.0] * K
    post_tr = [0.0] * K
    issues = []
    for t, st in enumerate(case["steps"]):
        de = st["decay_e"] if st["decay_e"] is not None else rule["decay_e"]
        clamped = 0
        for k, e in enumerate(edges):
            j, i = src[e], tgt[e]
            if stdp:
                ptr = rule.get("decay_pre", 0) * pre_tr[k]
                qtr = rule.get("decay_post", 0) * post_tr[k]
                el = de * elig[k]
                if st["spikes_post"][i] != 0:
                    el += rule.get("a_plus", 0) * ptr
                if st["spikes_pre"][j] != 0:
                    el -= rule.get("a_minus", 0) * qtr
                pre_tr[k] = ptr + (1 if st["spikes_pre"][j] != 0 else 0)
                post_tr[k] = qtr + (1 if st["spikes_post"][i] != 0 else 0)
            else:
                el = de * elig[k] + st["pre"][j] * st["post"][i]
            pl = rule["decay_p"] * plastic[k] + st["gate"] * el
            m = rule["plastic_max"]
            if pl > m or pl < -m:
                clamped += 1
            pl = max(-m, min(m, pl))
            elig[k], plastic[k] = el, pl
        s = st["state"]
        for name, ref in [("eligibility", elig), ("plastic", plastic)] + ([("pre_trace", pre_tr), ("post_trace", post_tr)] if stdp else []):
            e = rel_err(ref, s[name])
            if e > 1e-12:
                issues.append(f"plastic step {t} {name} rel err {e:.3g}")
        if clamped != st["clamped"]:
            issues.append(f"plastic step {t} clamped {st['clamped']} want {clamped}")
    # effective weights
    for slow, key, hkey in [(case["slow"], "effective", "held"), (None, "effective_noslow", "held_noslow")]:
        out = list(case["base"])
        held = 0
        for k, e in enumerate(edges):
            sgn = case["signs"][e]
            sl = slow[k] if slow is not None else 0.0
            if sgn == 0:
                out[e] = case["base"][e] + sl + plastic[k]
            else:
                mag = abs(case["base"][e]) + sl + plastic[k]
                if mag < rule["w_min"]:
                    mag, held = rule["w_min"], held + 1
                out[e] = sgn * mag
        e = rel_err(out, case[key])
        if e > 1e-12:
            issues.append(f"{key} rel err {e:.3g}")
        if held != case[hkey]:
            issues.append(f"{hkey} {case[hkey]} want {held}")
    return issues, {}


def main():
    path = sys.argv[1]
    data = json.load(open(path))
    summary = {}
    worst = {}
    for family, fn in [("lif", lambda c: run_spiking_case(c, "lif")), ("mixed", lambda c: run_spiking_case(c, "mixed")),
                       ("vector", run_vector_case), ("net", run_net_case), ("plastic", run_plastic_case)]:
        bad = 0
        for case in data[family]:
            issues, results = fn(case)
            for k, v in results.items():
                key = f"{family}.{case.get('kind', '')}.{k}"
                worst[key] = max(worst.get(key, 0.0), v)
            if issues:
                bad += 1
                print(f"[{family}{'/' + case['kind'] if 'kind' in case else ''} seed={case['seed']}]", "; ".join(issues[:8]))
        summary[family] = (len(data[family]) - bad, len(data[family]))
    print("summary (passing/total):", summary)
    print("worst relative errors:")
    for k in sorted(worst):
        print(f"  {k}: {worst[k]:.3g}")


if __name__ == "__main__":
    main()
