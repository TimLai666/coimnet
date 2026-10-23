"""Rapid prototyping lab: CoImNet continuous-core math vs GRU/LSTM on
column-sequential KMNIST (28 steps x 28 pixels), equal parameter budgets."""
import argparse, gzip, json, math, time, sys
import numpy as np, torch, torch.nn as nn, torch.nn.functional as F

def load(d):
    def img(f):
        b = gzip.open(f).read(); return np.frombuffer(b, np.uint8, offset=16).reshape(-1, 28, 28).astype(np.float32) / 255
    def lab(f):
        return np.frombuffer(gzip.open(f).read(), np.uint8, offset=8).astype(np.int64)
    return (img(f"{d}/train-images-idx3-ubyte.gz"), lab(f"{d}/train-labels-idx1-ubyte.gz"),
            img(f"{d}/t10k-images-idx3-ubyte.gz"), lab(f"{d}/t10k-labels-idx1-ubyte.gz"))

class CoimCore(nn.Module):
    """v' = lam*v + alpha*(E x + W y + b), y = tanh(v); W restricted to mask.
    Input enters n_in input nodes, readout reads n_out readout nodes."""
    def __init__(s, n, n_in, n_out, mask, d_in=28, classes=10, tau0=2.0, learn_tau=True, gate=False, act="tanh"):
        super().__init__()
        s.n, s.n_in, s.n_out = n, n_in, n_out
        s.register_buffer("mask", mask.float())
        fan = mask.sum(0).clamp(min=1)  # in-degree per target (mask[src,tgt])
        s.W = nn.Parameter(torch.randn(n, n) / fan.sqrt() * mask)
        s.E = nn.Parameter(torch.randn(d_in, n_in) / math.sqrt(d_in))
        s.b = nn.Parameter(torch.zeros(n))
        s.logtau = nn.Parameter(torch.full((n,), math.log(tau0)), requires_grad=learn_tau)
        s.R = nn.Linear(n_out, classes)
        s.gate = gate
        s.act = {"tanh": torch.tanh, "softsign": F.softsign}[act]
        if gate == True or gate == "input":  # input-dependent update gate from raw x
            s.gate = "input"; s.G = nn.Parameter(torch.zeros(d_in, n)); s.gb = nn.Parameter(torch.zeros(n))
        elif gate in ("local", "local_softsign"):  # gate from the neuron's own total input current: 2 params/neuron
            s.gg = nn.Parameter(torch.zeros(n)); s.gb = nn.Parameter(torch.zeros(n))
    def params(s):
        c = int(s.mask.sum()) + s.E.numel() + s.n * (2 if s.logtau.requires_grad else 1) + sum(p.numel() for p in s.R.parameters())
        if s.gate == "input": c += s.G.numel() + s.n
        elif s.gate in ("local", "local_softsign"): c += 2 * s.n
        return c
    def forward(s, x):  # x [B,T,d]
        B, T, _ = x.shape
        v = x.new_zeros(B, s.n); y = s.act(v)
        W = s.W * s.mask
        alpha = -torch.expm1(-1.0 / torch.exp(s.logtau)); lam = 1 - alpha
        pad = s.n - s.n_in
        for t in range(T):
            u = F.pad(x[:, t] @ s.E, (0, pad))
            a = alpha
            I = u + y @ W + s.b
            if s.gate == "input": a = alpha * 2 * torch.sigmoid(x[:, t] @ s.G + s.gb)
            elif s.gate == "local": a = alpha * 2 * torch.sigmoid(s.gg * I + s.gb)
            elif s.gate == "local_softsign": a = alpha * (1 + F.softsign(s.gg * I + s.gb))  # 2*sigmoid without exp
            a = a.clamp(max=1.0)
            v = (1 - a) * v + a * I
            y = s.act(v)
        return s.R(y[:, s.n - s.n_out:])

class RNNBase(nn.Module):
    def __init__(s, kind, h, d_in=28, classes=10):
        super().__init__()
        s.rnn = {"gru": nn.GRU, "lstm": nn.LSTM, "rnn": nn.RNN}[kind](d_in, h, batch_first=True)
        s.R = nn.Linear(h, classes)
    def params(s): return sum(p.numel() for p in s.parameters())
    def forward(s, x):
        o, _ = s.rnn(x); return s.R(o[:, -1])

def make_mask(kind, n, n_in, n_out, k, seed):
    g = torch.Generator().manual_seed(seed)
    if kind == "dense":
        m = torch.ones(n, n)
    elif kind == "random":  # each target gets k random sources
        m = torch.zeros(n, n)
        for t in range(n):
            m[torch.randperm(n, generator=g)[:k], t] = 1
    m[:, :n_in] = m[:, :n_in]  # input nodes may also receive recurrent input
    return m

def run(model, data, epochs, lr, bs, seed, log, device="cpu", cols=1):
    Xtr, Ytr, Xte, Yte = data
    torch.manual_seed(seed)
    opt = torch.optim.AdamW(model.parameters(), lr=lr, weight_decay=0.0)
    steps = epochs * (len(Xtr) // bs)
    sched = torch.optim.lr_scheduler.OneCycleLR(opt, lr, total_steps=steps)
    # columns as time; cols adjacent columns share one step (28/cols steps)
    Xtr_t = torch.from_numpy(Xtr).transpose(1, 2).reshape(len(Xtr), 28 // cols, 28 * cols).contiguous().to(device)
    Xte_t = torch.from_numpy(Xte).transpose(1, 2).reshape(len(Xte), 28 // cols, 28 * cols).contiguous().to(device)
    Ytr_t, Yte_t = torch.from_numpy(Ytr).to(device), torch.from_numpy(Yte).to(device)
    t0 = time.time()
    for ep in range(epochs):
        model.train(); perm = torch.randperm(len(Xtr_t), device=device)
        for i in range(0, len(perm) - bs + 1, bs):
            idx = perm[i:i + bs]
            loss = F.cross_entropy(model(Xtr_t[idx]), Ytr_t[idx])
            opt.zero_grad(); loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            opt.step(); sched.step()
        acc = evaluate(model, Xte_t, Yte_t)
        log(f"  ep{ep} loss {loss.item():.3f} test {acc:.4f} ({time.time()-t0:.0f}s)")
    return acc

@torch.no_grad()
def evaluate(model, X, Y):
    model.eval(); c = 0
    for i in range(0, len(X), 1000):
        c += (model(X[i:i+1000]).argmax(1) == Y[i:i+1000]).sum().item()
    return c / len(X)

if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", required=True); ap.add_argument("--spec", required=True)
    ap.add_argument("--epochs", type=int, default=8); ap.add_argument("--lr", type=float, default=3e-3)
    ap.add_argument("--bs", type=int, default=128); ap.add_argument("--seed", type=int, default=0)
    ap.add_argument("--threads", type=int, default=1)
    ap.add_argument("--device", default="cpu", help="torch device, e.g. cuda")
    ap.add_argument("--val", action="store_true", help="hold out the last 6000 train images and score on them instead of test")
    ap.add_argument("--val-count", type=int, default=0, help="hold out the last N train images, train on the rest, and report both val and test accuracy")
    a = ap.parse_args(); torch.set_num_threads(a.threads)
    data = load(a.data)
    if a.val and a.val_count:
        sys.exit("--val and --val-count are exclusive")
    if a.val:
        Xtr, Ytr, _, _ = data; data = (Xtr[:-6000], Ytr[:-6000], Xtr[-6000:], Ytr[-6000:])
    held = None
    if a.val_count:
        Xtr, Ytr, Xte, Yte = data
        if not 0 < a.val_count < len(Xtr):
            sys.exit(f"--val-count must be between 1 and {len(Xtr) - 1}")
        held = (Xtr[-a.val_count:], Ytr[-a.val_count:]); data = (Xtr[:-a.val_count], Ytr[:-a.val_count], Xte, Yte)
    spec = json.loads(a.spec); torch.manual_seed(a.seed)
    cols = spec.get("cols", 1); d_in = 28 * cols
    if spec["model"] in ("gru", "lstm", "rnn"):
        m = RNNBase(spec["model"], spec["h"], d_in=d_in)
    else:
        n = spec["n"]; mask = make_mask(spec["topo"], n, spec["n_in"], spec["n_out"], spec.get("k", 0), a.seed)
        m = CoimCore(n, spec["n_in"], spec["n_out"], mask, learn_tau=spec.get("learn_tau", True), gate=spec.get("gate", False), act=spec.get("act", "tanh"), tau0=spec.get("tau0", 2.0), d_in=d_in)
    m = m.to(a.device)
    log = lambda s: print(s, flush=True)
    log(f"{json.dumps(spec)} params={m.params()}")
    acc = run(m, data, a.epochs, a.lr, a.bs, a.seed, log, a.device, cols)
    result = {"spec": spec, "params": m.params(), "acc": acc, "seed": a.seed, "lr": a.lr, "val": a.val}
    if held is not None:
        Xv = torch.from_numpy(held[0]).transpose(1, 2).reshape(len(held[0]), 28 // cols, 28 * cols).contiguous().to(a.device)
        result["val_count"] = a.val_count; result["val_acc"] = evaluate(m, Xv, torch.from_numpy(held[1]).to(a.device))
    print("RESULT", json.dumps(result), flush=True)
