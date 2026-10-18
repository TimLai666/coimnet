#!/usr/bin/env python3
"""Context only: what the same 6000-image training split supports with a
conventional regime (minibatch 64, AdamW, dropout, early stopping on a
validation split carved out of the training images; the 2000 test images are
scored once, at the best-validation epoch).

Models: mlp512 (784-512-10 ReLU, dropout .3), small (784-28-24-10 tanh, the
framework's layer shape, no dropout), cnn (2x conv32/64 + fc128, dropout .3).
"""
import argparse, gzip, json, os, sys
import numpy as np
import torch, torch.nn as nn

torch.set_num_threads(1)


def idx(p, off, shape):
    b = gzip.open(p).read()
    return np.frombuffer(b, np.uint8, offset=off).reshape(shape)


def build(name):
    if name == "mlp512":
        return nn.Sequential(nn.Linear(784, 512), nn.ReLU(), nn.Dropout(.3), nn.Linear(512, 10))
    if name == "small":
        return nn.Sequential(nn.Linear(784, 28), nn.Tanh(), nn.Linear(28, 24), nn.Tanh(), nn.Linear(24, 10))
    return nn.Sequential(nn.Unflatten(1, (1, 28, 28)), nn.Conv2d(1, 32, 3, padding=1), nn.ReLU(), nn.MaxPool2d(2),
                         nn.Conv2d(32, 64, 3, padding=1), nn.ReLU(), nn.MaxPool2d(2), nn.Flatten(),
                         nn.Linear(64 * 49, 128), nn.ReLU(), nn.Dropout(.3), nn.Linear(128, 10))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data-dir", default="data/TSK-11/kmnist")
    ap.add_argument("--dump", required=True)
    ap.add_argument("--model", default="mlp512")
    ap.add_argument("--epochs", type=int, default=40)
    ap.add_argument("--wd", type=float, default=1e-4)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()
    torch.manual_seed(0)
    init = json.load(open(os.path.join(a.dump, "init.json")))
    d = a.data_dir
    tri = idx(f"{d}/train-images-idx3-ubyte.gz", 16, (-1, 784)); trl = idx(f"{d}/train-labels-idx1-ubyte.gz", 8, (-1,)).astype(np.int64)
    tei = idx(f"{d}/t10k-images-idx3-ubyte.gz", 16, (-1, 784)); tel = idx(f"{d}/t10k-labels-idx1-ubyte.gz", 8, (-1,)).astype(np.int64)
    tr = np.array(init["train_indices"])
    # balanced round-robin order: the last 600 selected are 60 per class
    fit, val = tr[:5400], tr[5400:]
    T = lambda x: torch.tensor(x / 255.0, dtype=torch.float32)
    Xf, yf, Xv, yv = T(tri[fit]), torch.tensor(trl[fit]), T(tri[val]), torch.tensor(trl[val])
    Xt, yt = T(tei[init["test_indices"]]), torch.tensor(tel[init["test_indices"]])
    net = build(a.model)
    opt = torch.optim.AdamW(net.parameters(), lr=1e-3, weight_decay=a.wd)
    lf = nn.CrossEntropyLoss()

    def acc(X, y):
        net.eval()
        with torch.no_grad():
            r = float((net(X).argmax(1) == y).float().mean())
        net.train()
        return r

    best, best_state, curve = -1, None, []
    for ep in range(a.epochs):
        perm = torch.randperm(len(yf))
        for i in range(0, len(perm), 64):
            j = perm[i:i + 64]
            opt.zero_grad(); lf(net(Xf[j]), yf[j]).backward(); opt.step()
        v = acc(Xv, yv); curve.append({"epoch": ep + 1, "val": v})
        if v > best:
            best, best_ep = v, ep + 1
            best_state = {k: t.clone() for k, t in net.state_dict().items()}
        print(f"{a.model} epoch {ep+1} val {v:.4f}", file=sys.stderr, flush=True)
    net.load_state_dict(best_state)
    res = {"model": a.model, "params": sum(p.numel() for p in net.parameters()), "fit": len(fit), "val": len(val),
           "best_val": best, "best_epoch": best_ep, "test_at_best_val": acc(Xt, yt), "curve": curve}
    os.makedirs(a.out, exist_ok=True)
    json.dump(res, open(os.path.join(a.out, "result.json"), "w"), indent=1)
    print(json.dumps({k: res[k] for k in ["model", "params", "best_val", "best_epoch", "test_at_best_val"]}))


if __name__ == "__main__":
    main()
