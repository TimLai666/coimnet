#!/usr/bin/env python3
"""Plain PyTorch MLP baselines on the same 6000/2000 KMNIST selection.

Uses the train/test indices and per-epoch order dumped by the Go driver
(init.json / orders.json) so data and order equal the framework runs. Same
training regime as the framework: one sample per update, Adam lr 0.002,
global-norm clip 1, 20 epochs. Standard PyTorch init, biases included.

--layers 28,24 gives 784-28-24-10 (the framework's effective layer shape);
--layers 29 gives a single hidden layer with a similar parameter count.
"""
import argparse, gzip, json, os, sys, time

import numpy as np
import torch
import torch.nn as nn

torch.set_num_threads(1)


def idx_images(p):
    with gzip.open(p, "rb") as f:
        b = f.read()
    return np.frombuffer(b, np.uint8, offset=16).reshape(-1, 784)


def idx_labels(p):
    with gzip.open(p, "rb") as f:
        b = f.read()
    return np.frombuffer(b, np.uint8, offset=8).astype(np.int64)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data-dir", default="data/TSK-11/kmnist")
    ap.add_argument("--dump", required=True)
    ap.add_argument("--layers", default="28,24")
    ap.add_argument("--act", default="tanh")
    ap.add_argument("--epochs", type=int, default=20)
    ap.add_argument("--lr", type=float, default=0.002)
    ap.add_argument("--seed", type=int, default=0)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()
    torch.manual_seed(a.seed)
    init = json.load(open(os.path.join(a.dump, "init.json")))
    orders = json.load(open(os.path.join(a.dump, "orders.json")))
    Xtr = torch.tensor(idx_images(os.path.join(a.data_dir, "train-images-idx3-ubyte.gz"))[init["train_indices"]] / 255.0, dtype=torch.float32)
    ytr = torch.tensor(idx_labels(os.path.join(a.data_dir, "train-labels-idx1-ubyte.gz"))[init["train_indices"]])
    Xte = torch.tensor(idx_images(os.path.join(a.data_dir, "t10k-images-idx3-ubyte.gz"))[init["test_indices"]] / 255.0, dtype=torch.float32)
    yte = torch.tensor(idx_labels(os.path.join(a.data_dir, "t10k-labels-idx1-ubyte.gz"))[init["test_indices"]])
    sizes = [784] + [int(x) for x in a.layers.split(",") if x] + [10]
    mods = []
    for i in range(len(sizes) - 1):
        mods.append(nn.Linear(sizes[i], sizes[i + 1]))
        if i < len(sizes) - 2:
            mods.append(nn.Tanh() if a.act == "tanh" else nn.ReLU())
    net = nn.Sequential(*mods)
    nparam = sum(p.numel() for p in net.parameters())
    opt = torch.optim.Adam(net.parameters(), lr=a.lr)
    lossf = nn.CrossEntropyLoss()

    def ev(X, y):
        with torch.no_grad():
            o = net(X)
            return float((o.argmax(1) == y).float().mean()), float(lossf(o, y))

    curve = []
    t0 = time.time()
    for ep in range(a.epochs):
        for i in orders[ep]:
            opt.zero_grad()
            l = lossf(net(Xtr[i : i + 1]), ytr[i : i + 1])
            l.backward()
            torch.nn.utils.clip_grad_norm_(net.parameters(), 1.0)
            opt.step()
        te = ev(Xte, yte)
        curve.append({"epoch": ep + 1, "test_acc": te[0], "test_ce": te[1]})
        print(f"{a.layers} epoch {ep+1} test {te[0]:.4f} {time.time()-t0:.0f}s", file=sys.stderr, flush=True)
    res = {"layers": sizes, "act": a.act, "params": nparam, "after": ev(Xte, yte), "train": ev(Xtr, ytr), "curve": curve}
    os.makedirs(a.out, exist_ok=True)
    json.dump(res, open(os.path.join(a.out, "result.json"), "w"), indent=1)
    print(json.dumps({k: res[k] for k in ["layers", "params", "after", "train"]}))


if __name__ == "__main__":
    main()
