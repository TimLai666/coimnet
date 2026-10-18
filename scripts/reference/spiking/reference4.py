#!/usr/bin/env python3
"""Reference for modulation.Controller (documented MLP + objective gradient via
autograd), Kinetics.Step (documented boosted/unboosted update + transport) and
the Hill occupancy. Usage: python3 reference4.py ctrl.json"""
import json, math, sys
import torch
torch.set_default_dtype(torch.float64)

def softplus(z):
    return torch.clamp(z, min=0) + torch.log1p(torch.exp(-z.abs()))

def main():
    d = json.load(open(sys.argv[1]))
    bad = 0
    for i, c in enumerate(d["controller"]):
        n, H, W = c["nodes"], c["hidden"], c["window"]
        p = torch.tensor(c["params"], requires_grad=True)
        width = 3  # mean, variance, one resource
        W1 = p[:H * width].reshape(H, width); b1 = p[H * width:H * width + H]
        w2 = p[H * width + H:H * width + 2 * H]; b2 = p[-1]
        ring = [[0.0] * n for _ in range(W)]
        filled = 0
        rel = []
        for a in c["activity"]:
            ring = ring[1:] + [list(a) if a is not None else [0.0] * n]
            filled = min(filled + 1, W)
            vals = [v for row in ring for v in row]  # zeros count; divide by full window
            total = W * n
            mean = sum(vals) / total
            var = max(sum(v * v for v in vals) / total - mean * mean, 0.0)
            x = torch.tensor([mean, var, c["resource"]])
            h = torch.tanh(W1 @ x + b1)
            q = softplus(w2 @ h + b2)
            rel.append(q)
        proxy = max(c["target"] - mean, 0.0)
        loss = (rel[-1] - proxy) ** 2
        loss.backward()
        e_rel = max(abs(a.item() - b) for a, b in zip(rel, c["release"]))
        e_g = (p.grad - torch.tensor(c["grad"])).abs().max().item() / max(p.grad.abs().max().item(), 1e-12)
        if e_rel > 1e-12 or e_g > 1e-9 or abs(loss.item() - c["loss"]) > 1e-12 * max(1, loss.item()):
            bad += 1
            print(f"controller case {i}: release err {e_rel:.3g} grad rel err {e_g:.3g} loss {loss.item()} vs {c['loss']}")
    print(f"controller cases passing: {len(d['controller']) - bad}/{len(d['controller'])}")
    bad = 0
    for i, k in enumerate(d["kinetics"]):
        cfg = k["config"]["chemistry"] if "chemistry" in k["config"] else k["config"]
        R, K, dt, tau = cfg["regions"], cfg["channels"], cfg["dt"], cfg["tau"]
        F = cfg["transport"]["fraction"] if cfg.get("transport") else None
        c = [[0.0] * K for _ in range(R)]
        for t, st in enumerate(k["steps"]):
            loc = [[0.0] * K for _ in range(R)]
            for r in range(R):
                for ch in range(K):
                    b, q = st["boost"][r][ch], st["release"][r][ch]
                    if b == 0:
                        lam = math.exp(-dt / tau[ch]); loc[r][ch] = lam * c[r][ch] + tau[ch] * (1 - lam) * q
                    else:
                        rate = 1 / tau[ch] + b; lam = math.exp(-dt * rate); loc[r][ch] = lam * c[r][ch] + (1 - lam) * q / rate
            if F:
                nxt = [[0.0] * K for _ in range(R)]
                for s in range(R):
                    for ch in range(K):
                        nxt[s][ch] = loc[s][ch] * (1 - sum(F[s][u] for u in range(R) if u != s)) + sum(loc[r][ch] * F[r][s] for r in range(R) if r != s)
                loc = nxt
            c = loc
            err = max(abs(a - b) / max(abs(b), 1e-12) for ra, rb in zip(c, st["state"]) for a, b in zip(ra, rb))
            if err > 1e-12:
                bad += 1
                print(f"kinetics case {i} step {t} rel err {err:.3g}")
                break
    print(f"kinetics cases passing: {len(d['kinetics']) - bad}/{len(d['kinetics'])}")
    bad = 0
    for o in d["occupancy"]:
        c, kd, n = o["c"], o["kd"], o["n"]
        ref = 0.0 if c == 0 else 1 / (1 + math.exp(min(n * (math.log(kd) - math.log(c)), 700)))
        # direct Hill form where representable
        if 1e-100 < c < 1e100:
            ref = c ** n / (kd ** n + c ** n) if (c ** n + kd ** n) < float("inf") else ref
        if abs(ref - o["occ"]) > 1e-12:
            bad += 1
            print("occupancy", o, "ref", ref)
    print(f"occupancy samples passing: {len(d['occupancy']) - bad}/{len(d['occupancy'])}")

main()
