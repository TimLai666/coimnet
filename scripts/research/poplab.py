"""Population trainer: many independent CoImNet sparse-core models trained in
one batched pass on column-sequential KMNIST.

Each population member is its own model with its own topology, initialization,
shuffle order, learning rate, gradient clipping and AdamW state; the batching
only stacks their arithmetic into the same kernels. The per-member math follows
examples/seqkmnist (continuous core, softsign activation, softsign
input-current gate, encoder into the first input nodes, bias-free readout of
the last readout nodes after the final step, softmax cross-entropy, AdamW
beta 0.9/0.999, eps 1e-8, no weight decay, clip 1 on the batch-mean gradient,
linear warmup then cosine decay to 1e-3 of the peak rate, the first update at
rate 0). Members of different node counts are padded to the largest one; pad
nodes have no edges, no input and no readout, so they stay at zero.

Usage:
  poplab.py --data DIR --pop '[{"n":512,"k":36,"n_in":128,"n_out":512,"lr":0.01,"seed":0}, ...]'
            [--epochs 16] [--bs 128] [--val-count 10000] [--device cuda]
Prints one RESULT line per member and a TIMING line.
"""
import argparse, gzip, json, math, sys, time
import numpy as np, torch


def load(d):
    def img(f):
        b = gzip.open(f).read()
        return np.frombuffer(b, np.uint8, offset=16).reshape(-1, 28, 28)
    def lab(f):
        return np.frombuffer(gzip.open(f).read(), np.uint8, offset=8).astype(np.int64)
    return (img(f"{d}/train-images-idx3-ubyte.gz"), lab(f"{d}/train-labels-idx1-ubyte.gz"),
            img(f"{d}/t10k-images-idx3-ubyte.gz"), lab(f"{d}/t10k-labels-idx1-ubyte.gz"))


def to_steps(x):
    # step t holds column t top to bottom, scaled to [0, 1]: [N, 28 steps, 28 rows]
    return torch.from_numpy(np.ascontiguousarray(x.transpose(0, 2, 1))).float() / 255


class Population:
    """Stacked parameters of P members, every tensor with a leading P axis."""

    def __init__(s, members, d_in, classes, device, kernel="dense"):
        P = len(members)
        N = max(m["n"] for m in members)
        K = max(m["k"] for m in members)
        s.P, s.N, s.members, s.kernel = P, N, members, kernel
        slots = torch.zeros(P, N, K, dtype=torch.int32)  # source of each target's slot
        wk = torch.zeros(P, N, K)
        smask = torch.zeros(P, N, K)
        mask = torch.zeros(P, N, N)  # mask[p, src, tgt]
        emask = torch.zeros(P, 1, N)
        rmask = torch.zeros(P, N, 1)
        nmask = torch.zeros(P, N)
        W = torch.zeros(P, N, N)
        E = torch.zeros(P, d_in, N)
        R = torch.zeros(P, N, classes)
        for p, m in enumerate(members):
            n, k, ni, no = m["n"], m["k"], m["n_in"], m["n_out"]
            g = torch.Generator().manual_seed(int(m["seed"]) * 1000003 + 17)
            for t in range(n):
                src = torch.randperm(n, generator=g)[:k]
                w0 = torch.randn(k, generator=g) / math.sqrt(k)
                mask[p, src, t] = 1
                W[p, src, t] = w0
                slots[p, t, :k], wk[p, t, :k], smask[p, t, :k] = src.int(), w0, 1
            E[p, :, :ni] = torch.randn(d_in, ni, generator=g) / math.sqrt(d_in)
            emask[p, 0, :ni] = 1
            b = 1 / math.sqrt(no)
            R[p, n - no:n] = (torch.rand(no, classes, generator=g) * 2 - 1) * b
            rmask[p, n - no:n] = 1
            nmask[p, :n] = 1
        s.mask, s.emask, s.rmask, s.nmask = (t.to(device) for t in (mask, emask, rmask, nmask))
        logtau = torch.full((P, N), math.log(2.0)) * nmask.cpu()
        s.params = {
            "W": W if kernel == "dense" else wk, "b": torch.zeros(P, N), "logtau": logtau,
            "gg": torch.zeros(P, N), "gb": torch.zeros(P, N), "E": E, "R": R,
        }
        s.params = {k: v.to(device).requires_grad_() for k, v in s.params.items()}
        s.gmask = {"W": s.mask if kernel == "dense" else smask.to(device), "b": s.nmask, "logtau": s.nmask, "gg": s.nmask, "gb": s.nmask,
                   "E": s.emask.expand(P, d_in, N), "R": s.rmask.expand(P, N, classes)}
        if kernel == "sparse":
            import popsparse
            s.core = popsparse.core
            s.slots = slots.to(device)
            ot, osl, _ = popsparse.transpose_slots(slots, None)
            s.out_t, s.out_slot = ot.to(device), osl.to(device)
        s.m = {k: torch.zeros_like(v) for k, v in s.params.items()}
        s.v = {k: torch.zeros_like(v) for k, v in s.params.items()}
        s.updates = 0
        s.count = [int(m["k"]) * m["n"] + d_in * m["n_in"] + 4 * m["n"] + m["n_out"] * classes for m in members]

    def forward(s, x):
        """x [P, B, T, d] -> logits [P, B, classes]."""
        p = s.params
        P, B, T, _ = x.shape
        u = torch.matmul(x, p["E"].unsqueeze(1))  # [P, B, T, N]
        if s.kernel == "sparse":
            alpha = -torch.expm1(-torch.exp(-p["logtau"]))
            y = s.core(u, alpha, p["gg"], p["gb"], p["b"], p["W"], s.slots, s.out_t, s.out_slot)
            return torch.bmm(y, p["R"] * s.rmask)
        W = p["W"] * s.mask
        alpha = (-torch.expm1(-torch.exp(-p["logtau"]))).unsqueeze(1)  # 1 - exp(-dt/tau)
        gg, gb, bias = p["gg"].unsqueeze(1), p["gb"].unsqueeze(1), p["b"].unsqueeze(1)
        v = x.new_zeros(P, B, s.N)
        y = v
        for t in range(T):
            I = torch.baddbmm(u[:, :, t] + bias, y, W)
            z = gg * I + gb
            a = torch.clamp(alpha * (1 + z / (1 + z.abs())), max=1.0)
            v = v + a * (I - v)
            y = v / (1 + v.abs())
        return torch.bmm(y, p["R"] * s.rmask)

    @torch.no_grad()
    def adam_step(s, lrs, clip=1.0, b1=0.9, b2=0.999, eps=1e-8):
        s.updates += 1
        sq = None
        for k, w in s.params.items():
            g = w.grad.mul_(s.gmask[k])
            n2 = g.pow(2).flatten(1).sum(1)
            sq = n2 if sq is None else sq + n2
        norm = sq.sqrt()
        scale = torch.where(norm > clip, clip / norm, torch.ones_like(norm))
        c1, c2 = 1 - b1 ** s.updates, 1 - b2 ** s.updates
        for k, w in s.params.items():
            shape = (-1,) + (1,) * (w.dim() - 1)
            g = w.grad * scale.view(shape)
            s.m[k].mul_(b1).add_(g, alpha=1 - b1)
            s.v[k].mul_(b2).addcmul_(g, g, value=1 - b2)
            upd = (s.m[k] / c1) / ((s.v[k] / c2).sqrt() + eps)
            w.sub_(lrs.view(shape) * upd)
            w.grad = None


def lr_at(peak, updates, steps, warmup_frac=0.3, final=1e-3):
    warm = int(steps * 3 / 10) if warmup_frac == 0.3 else int(steps * warmup_frac)
    if updates < warm:
        return peak * updates / warm
    prog = min((updates - warm) / max(steps - warm, 1), 1.0)
    return peak * (final + (1 - final) * 0.5 * (1 + math.cos(math.pi * prog)))


@torch.no_grad()
def accuracy(pop, X, Y, chunk=0):
    chunk = chunk or max(8192 // pop.P, 16)
    correct = torch.zeros(pop.P, device=X.device)
    for i in range(0, len(X), chunk):
        xb = X[i:i + chunk].unsqueeze(0).expand(pop.P, -1, -1, -1)
        correct += (pop.forward(xb).argmax(-1) == Y[i:i + chunk]).float().sum(1)
    return (correct / len(X)).tolist()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", required=True)
    ap.add_argument("--pop", required=True, help="JSON list of members, or @file")
    ap.add_argument("--epochs", type=int, default=16)
    ap.add_argument("--bs", type=int, default=128)
    ap.add_argument("--val-count", type=int, default=10000)
    ap.add_argument("--train-count", type=int, default=60000)
    ap.add_argument("--device", default="cpu")
    ap.add_argument("--threads", type=int, default=1)
    ap.add_argument("--eval-every", action="store_true", help="score val after every epoch")
    ap.add_argument("--compile", default="", help="torch.compile mode for the loss (e.g. default, max-autotune-no-cudagraphs)")
    ap.add_argument("--tf32", action="store_true", help="allow TF32 matmuls (changes rounding)")
    ap.add_argument("--kernel", default="dense", choices=["dense", "sparse"], help="dense masked bmm, or the Triton gather kernels of popsparse.py")
    a = ap.parse_args()
    torch.set_num_threads(a.threads)
    spec = open(a.pop[1:]).read() if a.pop.startswith("@") else a.pop
    members = json.loads(spec)
    dev = torch.device(a.device)
    Xtr, Ytr, Xte, Yte = load(a.data)
    Xtr, Ytr = Xtr[:a.train_count], Ytr[:a.train_count]
    k = len(Xtr) - a.val_count
    Xv, Yv = to_steps(Xtr[k:]).to(dev), torch.from_numpy(Ytr[k:]).to(dev)
    Xt, Yt = to_steps(Xtr[:k]).to(dev), torch.from_numpy(Ytr[:k]).to(dev)
    Xs, Ys = to_steps(Xte).to(dev), torch.from_numpy(Yte).to(dev)
    torch.backends.cuda.matmul.allow_tf32 = a.tf32
    pop = Population(members, 28, 10, dev, a.kernel)

    def batch_loss(xb, yb):
        logits = pop.forward(xb)
        return torch.nn.functional.cross_entropy(logits.reshape(-1, 10), yb.reshape(-1), reduction="none").view(pop.P, -1).mean(1).sum()
    if a.compile:
        batch_loss = torch.compile(batch_loss, mode=a.compile, dynamic=False)
    P, bs = pop.P, a.bs
    per_epoch = len(Xt) // bs
    steps = a.epochs * per_epoch
    peaks = [float(m["lr"]) for m in members]
    gens = [torch.Generator(device=dev).manual_seed(int(m["seed"]) * 7919 + 99) for m in members]
    print(f"members {P} pad-nodes {pop.N} updates {steps} params {pop.count}", flush=True)
    if dev.type == "cuda":
        torch.cuda.synchronize()
    t0 = time.time()
    curves = [[] for _ in range(P)]
    lr_table = torch.tensor([[lr_at(pk, u, steps) for pk in peaks] for u in range(steps)], device=dev)
    for ep in range(a.epochs):
        order = torch.stack([torch.randperm(len(Xt), generator=g, device=dev) for g in gens])  # [P, Ntr]
        for i in range(per_epoch):
            idx = order[:, i * bs:(i + 1) * bs]
            xb, yb = Xt[idx], Yt[idx]  # [P, B, T, d], [P, B]
            batch_loss(xb, yb).backward()
            pop.adam_step(lr_table[pop.updates])
        if a.eval_every or ep == a.epochs - 1:
            va = accuracy(pop, Xv, Yv)
            for p in range(P):
                curves[p].append(va[p])
            print(f"epoch {ep} val {['%.4f' % v for v in va]} ({time.time() - t0:.1f}s)", flush=True)
    if dev.type == "cuda":
        torch.cuda.synchronize()
    train_s = time.time() - t0
    te = accuracy(pop, Xs, Ys)
    for p, m in enumerate(members):
        print("RESULT", json.dumps({"member": m, "params": pop.count[p], "epochs": a.epochs, "bs": bs,
                                    "val_acc": curves[p][-1], "val_curve": curves[p], "test_acc": te[p]}), flush=True)
    print("TIMING", json.dumps({"members": P, "train_seconds": train_s, "seconds_per_member": train_s / P,
                                "device": str(dev), "updates": steps}), flush=True)


if __name__ == "__main__":
    main()
