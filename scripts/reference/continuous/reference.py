#!/usr/bin/env python3
"""Independent PyTorch reference for the CoImNet scalar continuous core,
learning.Network and learning.Trainer, written from the documented math
(doc comments, ENG.md, ticket 17/25), not from the Go reverse pass.

Gradients come from torch autograd. The float64 core and the float32 Insyra
encoder/readout boundary are mirrored: encoder x@E and readout h@R plus MSE run
in float32, the core in float64, with explicit casts at the two boundaries.

Usage: python3 reference.py runs/.tmp/ref-continuous/cases.json
"""
import json
import math
import sys

import torch

torch.set_num_threads(1)
F64, F32 = torch.float64, torch.float32


# ---------------------------------------------------------------- core ----
def act(v, kind):
    # Documented softplus log(1+exp(v)), written in its stable form; torch's
    # built-in softplus switches to the identity above 20, which is not it.
    return torch.tanh(v) if kind == "tanh" else torch.clamp(v, min=0) + torch.log1p(torch.exp(-v.abs()))


def core_forward(cfg, W, b, logtau, v0, inputs, window, state=None):
    """Documented continuous core.

    v[t+1] = lambda*v[t] + alpha*(u[t] + sum_e w_e*y_src(t - d_e) + b)
    lambda = exp(-dt/tau), alpha = 1 - lambda, tau = exp(logtau), y = act(v).
    Delay zero reads the state at the beginning of step t; before step zero the
    initial output is held constant. Window > 0 detaches at fixed boundaries
    0, w, 2w, ... including delayed edges, without resetting voltage.

    With a saved state (stateful segment), outputs at absolute steps <= S come
    from the saved history and are constants; only the membrane leak path from
    the segment start voltage is differentiable.
    """
    n = cfg["nodes"]
    src, tgt = cfg["sources"] or [], cfg["targets"] or []
    dl = cfg.get("delays") or [0] * len(src)
    dt = cfg["dt"]
    tau = torch.exp(logtau)
    lam = torch.exp(-dt / tau)
    alpha = 1 - lam  # autograd derivative is the same as -expm1
    alpha = -torch.expm1(-dt / tau)
    T = len(inputs)
    V = [v0]
    if state is None:
        Y = [act(v0, cfg["activation"])]
        hist, S, hstart = None, 0, 0
    else:
        hist = torch.tensor(state["history"], dtype=F64)
        S = state["steps"]
        hstart = S - (len(state["history"]) - 1)
        Y = [hist[-1]]
    srcT = torch.tensor(src, dtype=torch.long)
    tgtT = torch.tensor(tgt, dtype=torch.long)
    for t in range(T):
        start = (t // window) * window if window > 0 else 0
        vin = V[t]
        if start > 0 and t == start:
            vin = vin.detach()
        contrib = []
        for e in range(len(src)):
            past_local = t - dl[e]
            if state is not None:
                past_abs = max(0, S + t - dl[e])
                if past_abs <= S:
                    contrib.append(hist[past_abs - hstart][src[e]])
                    continue
                past_local = past_abs - S
            else:
                past_local = max(0, past_local)
            y = Y[past_local][src[e]]
            if start > 0 and past_local <= start:
                y = y.detach()
            contrib.append(y)
        syn = torch.zeros(n, dtype=F64)
        if src:
            vals = W * torch.stack(contrib)
            syn = syn.index_add(0, tgtT, vals)
        drive = inputs[t] + syn + b
        vn = lam * vin + alpha * drive
        V.append(vn)
        Y.append(act(vn, cfg["activation"]))
    return torch.stack(Y[1:]), V


def check_core(case, report):
    cfg = case["config"]
    p = case["params"]
    n = cfg["nodes"]
    W = torch.tensor(p["weights"] or [], dtype=F64, requires_grad=True)
    b = torch.tensor(p["bias"], dtype=F64, requires_grad=True)
    lt = torch.tensor(p["log_tau"], dtype=F64, requires_grad=True)
    v0 = torch.tensor(case["initial"], dtype=F64, requires_grad=True)
    u = torch.tensor(case["inputs"], dtype=F64, requires_grad=True)
    up = torch.tensor(case["upstream"], dtype=F64)
    Y, _ = core_forward(cfg, W, b, lt, v0, u, case["window"])
    (Y * up).sum().backward()
    g = case["grad"]
    tag = f"core seed {case['seed']}"
    cmp(report, tag + " outputs", Y.detach(), case["outputs"], 1e-12)
    cmp(report, tag + " dW", W.grad if W.numel() else torch.zeros(0), g["Weights"] or [], 1e-9)
    cmp(report, tag + " db", b.grad, g["Bias"], 1e-9)
    cmp(report, tag + " dlogtau", lt.grad, g["LogTau"], 1e-9)
    cmp(report, tag + " dInputs", u.grad, g["Inputs"], 1e-9)
    cmp(report, tag + " dInitial", v0.grad, g["Initial"], 1e-9)
    if case.get("state"):
        st = case["state"]
        W2 = W.detach().clone().requires_grad_(True)
        b2 = b.detach().clone().requires_grad_(True)
        lt2 = lt.detach().clone().requires_grad_(True)
        v02 = torch.tensor(st["voltage"], dtype=F64, requires_grad=True)
        u2 = torch.tensor(case["inputs"], dtype=F64, requires_grad=True)
        Y2, _ = core_forward(cfg, W2, b2, lt2, v02, u2, case["seg_window"], state=st)
        (Y2 * up).sum().backward()
        g2 = case["seg_grad"]
        cmp(report, tag + " seg outputs", Y2.detach(), case["seg_outputs"], 1e-12)
        cmp(report, tag + " seg dW", W2.grad if W2.numel() else torch.zeros(0), g2["Weights"] or [], 1e-9)
        cmp(report, tag + " seg db", b2.grad, g2["Bias"], 1e-9)
        cmp(report, tag + " seg dlogtau", lt2.grad, g2["LogTau"], 1e-9)
        cmp(report, tag + " seg dInputs", u2.grad, g2["Inputs"], 1e-9)
        cmp(report, tag + " seg dInitial", v02.grad, g2["Initial"], 1e-9)


# ------------------------------------------------------------- network ----
def net_grad(cfg, params, x_rows, target=None, upstream=None, window=0):
    """learning.Network.LossGradient (target) or LossGradientFrom (upstream).

    encoder [input, input-node] (float32) -> scatter to InputNodes -> core
    (float64, zero initial voltage) -> readout nodes (last step or every step)
    -> float32 readout [selected, output] -> MSE on the last row (float32).
    """
    dyn = cfg["dynamics"]
    n = dyn["nodes"]
    signs = cfg.get("edge_signs") or []
    inodes = cfg.get("input_nodes")
    rnodes = cfg["readout_nodes"]
    every = cfg.get("readout_every_step", False)
    I, O = cfg["input_size"], cfg["output_size"]
    T = len(x_rows)
    width = len(inodes) if inodes is not None else n
    x32 = torch.tensor(x_rows, dtype=F32, requires_grad=True)
    E32 = torch.tensor(params["encoder"], dtype=F64).to(F32).reshape(I, width).requires_grad_(True)
    z32 = x32 @ E32
    z = z32.detach().to(F64)
    u_sel = z.clone().requires_grad_(True)
    if inodes is not None:
        u = torch.zeros(T, n, dtype=F64).index_copy(1, torch.tensor(inodes), u_sel)
    else:
        u = u_sel
    raw = torch.tensor(params["core"]["weights"] or [], dtype=F64, requires_grad=True)
    if signs and any(signs):
        s = torch.tensor(signs, dtype=F64)
        W = torch.where(s != 0, s * torch.exp(raw), raw)
    else:
        W = raw
    b = torch.tensor(params["core"]["bias"], dtype=F64, requires_grad=True)
    lt = torch.tensor(params["core"]["log_tau"], dtype=F64, requires_grad=True)
    v0 = torch.zeros(n, dtype=F64, requires_grad=True)
    Y, _ = core_forward(dyn, W, b, lt, v0, u, window)
    rn = torch.tensor(rnodes)
    h = Y[:, rn] if every else Y[-1, rn]
    h32 = h.detach().to(F32).requires_grad_(True)
    R32 = torch.tensor(params["readout"], dtype=F64).to(F32).reshape(len(rnodes), O).requires_grad_(True)
    pred = h32 @ R32
    loss = None
    if target is not None:
        last = pred[-1] if every else pred
        t32 = torch.tensor(target, dtype=F64).to(F32)
        loss = ((last - t32) ** 2).mean()
        loss.backward()
    else:
        up = torch.tensor(upstream, dtype=F64).to(F32)
        seed = up if every else up[-1]
        pred.backward(seed)
    # float32 dL/dh crosses to the float64 core.
    h.backward(h32.grad.to(F64))
    # float64 core input gradient crosses back to the float32 encoder.
    z32.backward(u_sel.grad.to(F32))
    zero = lambda t, like: t if t is not None else torch.zeros_like(like)
    return {
        "loss": None if loss is None else float(loss.detach()),
        "Weights": zero(raw.grad, raw), "Bias": b.grad, "LogTau": lt.grad,
        "Encoder": E32.grad.reshape(-1), "Readout": R32.grad.reshape(-1),
        "Inputs": x32.grad, "CoreInputs": u_sel.grad if inodes is None else None,
    }


def check_net_grads(case, report):
    cfg, p = case["config"], case["params"]
    tag = f"net seed {case['seed']}"
    r = net_grad(cfg, p, case["input"], target=case["target"], window=case["window"])
    g = case["grad"]
    if abs(r["loss"] - case["loss"]) > 1e-5 * max(1, abs(r["loss"])):
        report.append((tag + " loss", r["loss"], case["loss"]))
    cmp_grad(report, tag + " LossGradient", r, g)
    r = net_grad(cfg, p, case["input"], upstream=case["upstream"], window=case["window"])
    cmp_grad(report, tag + " LossGradientFrom", r, case["grad_from"])


def cmp_grad(report, tag, r, g):
    # The core arrays depend on float32 rounding of dL/dh, so relative 1e-5.
    cmp(report, tag + " dW", r["Weights"], g["Weights"] or [], 2e-5)
    cmp(report, tag + " db", r["Bias"], g["Bias"], 2e-5)
    cmp(report, tag + " dlogtau", r["LogTau"], g["LogTau"], 2e-5)
    cmp(report, tag + " dEncoder", r["Encoder"], g["Encoder"], 2e-5)
    cmp(report, tag + " dReadout", r["Readout"], g["Readout"], 2e-5)
    cmp(report, tag + " dInputs", r["Inputs"], g["Inputs"], 2e-5)


# ------------------------------------------------------------- trainer ----
def flat_params(p):
    c = p["core"]
    return [*(c["weights"] or []), *c["bias"], *c["log_tau"], *(p.get("theta_raw") or []), *p["encoder"], *p["readout"]]


def flat_grad(g):
    return [*(g["Weights"] or []), *g["Bias"], *g["LogTau"], *g["Encoder"], *g["Readout"]]


def lr_at(o, updates):
    """Schedule doc: warmup ramps from 0 over WarmupUpdates; step multiplies by
    StepFactor once per StepEvery completed updates; cosine decays over
    DecayUpdates after the warmup to base*FinalFactor."""
    s = o.get("schedule")
    base = o["learning_rate"]
    if not s:
        return base
    wu = s.get("warmup_updates", 0)
    if updates < wu:
        return base * updates / wu
    if s["kind"] == "step":
        return base * s["step_factor"] ** (updates // s["step_every"])
    if s["kind"] == "cosine":
        prog = min(1.0, (updates - wu) / s["decay_updates"])
        ff = s.get("final_factor", 0)
        return base * (ff + (1 - ff) * 0.5 * (1 + math.cos(math.pi * prog)))
    return base


def masks(cfg, o, sizes):
    nW, nB, nL, nE, nR = sizes
    t = o["trainable"]
    m = o.get("masks") or {}
    edges, nodes = m.get("edges"), m.get("nodes")
    out = []
    out += [t["weights"] and (edges[i] if edges else True) for i in range(nW)]
    out += [t["bias"] and (nodes[i] if nodes else True) for i in range(nB)]
    out += [t["tau"] and (nodes[i] if nodes else True) for i in range(nL)]
    out += [t["encoder"]] * nE + [t["readout"]] * nR
    return out


def groups_of(cfg, nW, nB):
    s = cfg.get("sharing")
    if not s:
        return None
    gs = {}
    for fam, off in (("weights", 0), ("bias", nW), ("log_tau", nW + nB)):
        for i, g in enumerate(s.get(fam) or []):
            if g >= 0:
                gs.setdefault(g, []).append(off + i)
    return gs


def norm_reduced(v, mask, groups):
    seen = set()
    member = {}
    if groups:
        for g, ms in groups.items():
            for m in ms:
                member[m] = g
    acc = 0.0
    for i, x in enumerate(v):
        if not mask[i]:
            continue
        g = member.get(i)
        if g is not None:
            if g in seen:
                continue
            seen.add(g)
        acc += x * x
    return math.sqrt(acc)


def optimizer_step(cfg, o, before, grad):
    """One Trainer.Step after the gradient: sharing sum -> loss scale round
    trip -> masked norm -> accumulation -> clip of the window average ->
    AdamW (decoupled decay, per-parameter bias correction) -> projection."""
    p = flat_params(before["parameters"])
    c = before["parameters"]["core"]
    nW, nB, nL = len(c["weights"] or []), len(c["bias"]), len(c["log_tau"])
    nE, nR = len(before["parameters"]["encoder"]), len(before["parameters"]["readout"])
    mask = masks(cfg, o, (nW, nB, nL, nE, nR))
    grad = list(grad)
    groups = groups_of(cfg, nW, nB)
    if groups:
        for ms in groups.values():
            tot = sum(grad[m] for m in ms)
            for m in ms:
                grad[m] = tot
            if not all(mask[m] for m in ms):
                for m in ms:
                    mask[m] = False
    f = o.get("loss_scale", 0) or 1
    for i in range(len(grad)):
        if mask[i]:
            grad[i] = (grad[i] * f) / f
    norm = norm_reduced(grad, mask, groups)
    window = o.get("accumulate_steps", 0) or 1
    acc = before.get("accumulator")
    updates = before["updates"]
    res = {"gradient_norm": norm}
    mean = grad
    if window > 1:
        s = list(acc["sum"]) if acc else [0.0] * len(grad)
        cnt = acc["count"] if acc else 0
        for i in range(len(grad)):
            if mask[i]:
                s[i] += grad[i]
        cnt += 1
        if cnt < window:
            res.update(applied=False, params=p, first=before["optimizer"]["first"], second=before["optimizer"]["second"], steps=before["optimizer"]["steps"], acc={"sum": s, "count": cnt}, updates=updates)
            return res
        mean = [s[i] / cnt if mask[i] else 0.0 for i in range(len(grad))]
        clipn = norm_reduced(mean, mask, groups)
    else:
        clipn = norm
    scale = o["clip_norm"] / clipn if (o["clip_norm"] > 0 and clipn > o["clip_norm"]) else 1.0
    lr = lr_at(o, updates)
    first = list(before["optimizer"]["first"])
    second = list(before["optimizer"]["second"])
    steps = list(before["optimizer"]["steps"])
    b1, b2, eps, wd = o["beta1"], o["beta2"], o["epsilon"], o["weight_decay"]
    for i in range(len(p)):
        if not mask[i]:
            continue
        steps[i] += 1
        d = mean[i] * scale
        first[i] = b1 * first[i] + (1 - b1) * d
        second[i] = b2 * second[i] + (1 - b2) * d * d
        mh = first[i] / (1 - b1 ** steps[i])
        vh = second[i] / (1 - b2 ** steps[i])
        p[i] = p[i] * (1 - lr * wd) - lr * mh / (math.sqrt(vh) + eps)
    # Projection (ranges + fixed-sign floor), masked entries untouched.
    proj = {}
    def bump(key):
        proj[key] = proj.get(key, 0) + 1
    rng = o.get("ranges") or {}
    signs = cfg.get("edge_signs") or []
    floor = cfg.get("min_log_magnitude", 0) or -20
    wmax = rng.get("weight_magnitude_max", 0)
    for i in range(nW):
        if not mask[i]:
            continue
        if i < len(signs) and signs[i] != 0:
            if wmax > 0 and p[i] > math.log(wmax):
                p[i] = math.log(wmax); bump("weights")
            if p[i] < floor:
                p[i] = floor; bump("min_log_magnitude")
        elif wmax > 0 and abs(p[i]) > wmax:
            p[i] = min(max(p[i], -wmax), wmax); bump("weights")
    bmax = rng.get("bias_abs_max", 0)
    for i in range(nW, nW + nB):
        if mask[i] and bmax > 0 and abs(p[i]) > bmax:
            p[i] = min(max(p[i], -bmax), bmax); bump("bias")
    lmin, lmax = rng.get("log_tau_min", 0), rng.get("log_tau_max", 0)
    for i in range(nW + nB, nW + nB + nL):
        if not mask[i]:
            continue
        if lmax != 0 and p[i] > lmax:
            p[i] = lmax; bump("log_tau")
        elif lmin != 0 and p[i] < lmin:
            p[i] = lmin; bump("log_tau")
    old = flat_params(before["parameters"])
    unorm = math.sqrt(sum((p[i] - old[i]) ** 2 for i in range(len(p)) if mask[i]))
    res.update(projected=proj, update_norm=unorm)
    res.update(applied=True, params=p, first=first, second=second, steps=steps, acc=None, updates=updates + 1, lr=lr)
    return res


def check_trainer(case, report):
    cfg, o = case["config"], case["options"]
    tag = f"net seed {case['seed']}"
    for k, st in enumerate(case["steps"]):
        if st.get("error"):
            report.append((tag + f" step {k} go error", st["error"], ""))
            break
        before = st["before"]
        # (a) torch gradient at the Go pre-step parameters vs Go LossGradient.
        r = net_grad(cfg, before["parameters"], st["input"], target=st["target"], window=o["truncation"])
        cmp_grad(report, tag + f" step {k}", r, st["grad"])
        if abs(r["loss"] - st["result"]["loss"]) > 1e-5 * max(1, abs(r["loss"])):
            report.append((tag + f" step {k} loss", r["loss"], st["result"]["loss"]))
        # (b) documented optimizer on the Go gradient vs the Go post-step state.
        res = optimizer_step(cfg, o, before, flat_grad(st["grad"]))
        after = st["after"]
        cmp(report, tag + f" step {k} params", torch.tensor(res["params"], dtype=F64), flat_params(after["parameters"]), 1e-10, atol=1e-13)
        cmp(report, tag + f" step {k} adam first", torch.tensor(res["first"], dtype=F64), after["optimizer"]["first"], 1e-10, atol=1e-15)
        cmp(report, tag + f" step {k} adam second", torch.tensor(res["second"], dtype=F64), after["optimizer"]["second"], 1e-10, atol=1e-18)
        if list(res["steps"]) != list(after["optimizer"]["steps"]):
            report.append((tag + f" step {k} adam steps", res["steps"], after["optimizer"]["steps"]))
        if res["updates"] != after["updates"] or res["applied"] != st["result"].get("applied", False):
            report.append((tag + f" step {k} applied/updates", (res["applied"], res["updates"]), (st["result"].get("applied"), after["updates"])))
        gn = st["result"].get("gradient_norm", 0)
        if abs(res["gradient_norm"] - gn) > 1e-10 * max(1, gn):
            report.append((tag + f" step {k} gradient_norm", res["gradient_norm"], gn))
        if res["applied"] and abs(res["lr"] - st["result"].get("learning_rate", 0)) > 1e-15:
            report.append((tag + f" step {k} lr", res["lr"], st["result"].get("learning_rate")))
        if res["applied"]:
            if res["projected"] != (st["result"].get("projected") or {}):
                report.append((tag + f" step {k} projected", res["projected"], st["result"].get("projected")))
            un = st["result"].get("update_norm", 0)
            if abs(res["update_norm"] - un) > 1e-10 * max(1e-3, un):
                report.append((tag + f" step {k} update_norm", res["update_norm"], un))
        ga = after.get("accumulator")
        if (res["acc"] is None) != (ga is None):
            report.append((tag + f" step {k} accumulator presence", res["acc"], ga))
        elif ga is not None:
            cmp(report, tag + f" step {k} accumulator", torch.tensor(res["acc"]["sum"], dtype=F64), ga["sum"], 1e-12, atol=1e-15)


# ------------------------------------------------------------- helpers ----
def cmp(report, tag, got, want, rtol, atol=None):
    got = torch.as_tensor(got, dtype=F64).reshape(-1)
    want = torch.tensor(want, dtype=F64).reshape(-1) if not isinstance(want, torch.Tensor) else want.to(F64).reshape(-1)
    if got.numel() != want.numel():
        report.append((tag + " shape", got.numel(), want.numel()))
        return
    if got.numel() == 0:
        return
    scale = max(1.0, float(want.abs().max()))
    a = atol if atol is not None else rtol * scale
    err = (got - want).abs()
    bad = err > a + rtol * want.abs()
    if bool(bad.any()):
        i = int(err.argmax())
        report.append((tag, f"max abs err {float(err.max()):.3e} at {i} (ref {float(got[i]):.17g}, go {float(want[i]):.17g})", int(bad.sum())))


def main():
    cases = json.load(open(sys.argv[1]))
    report = []
    counts = {"core": 0, "net": 0, "steps": 0}
    for c in cases:
        if c.get("error"):
            report.append((f"{c['kind']} seed {c['seed']} go error", c["error"], ""))
            continue
        if c["kind"] == "core":
            check_core(c, report)
            counts["core"] += 1
        else:
            check_net_grads(c, report)
            check_trainer(c, report)
            counts["net"] += 1
            counts["steps"] += len(c["steps"])
    print("checked", counts)
    for r in report:
        print("MISMATCH", *r)
    print("mismatches:", len(report))


if __name__ == "__main__":
    main()
