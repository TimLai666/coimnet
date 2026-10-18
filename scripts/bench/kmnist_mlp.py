#!/usr/bin/env python3
"""PyTorch MLP baseline for examples/benchocr on KMNIST.

train:   fit an MLP on the balanced train selection examples/realocr uses
         (one image per update, Adam, global-norm clip 1) and save it.
measure: in a fresh process, load the model and write a coimnet-bench/v1
         result on the same balanced test selection, one image per call.

    python3 scripts/bench/kmnist_mlp.py train --data-dir data/kmnist --out runs/mlp.pt
    python3 scripts/bench/kmnist_mlp.py measure --data-dir data/kmnist --model runs/mlp.pt --out runs/mlp.json
"""
import argparse
import gzip
import hashlib
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coimbench  # noqa: E402

import numpy as np  # noqa: E402
import torch  # noqa: E402
import torch.nn as nn  # noqa: E402

TASK = "kmnist-t10k-single-character"
CLASSES = 10


def idx(path, magic, dims):
    with gzip.open(path, "rb") as f:
        b = f.read()
    if int.from_bytes(b[:4], "big") != magic:
        raise SystemExit(f"{path}: not an IDX file with magic {magic}")
    shape = [int.from_bytes(b[4 + 4 * i:8 + 4 * i], "big") for i in range(dims)]
    data = np.frombuffer(b, np.uint8, offset=4 + 4 * dims)
    return data.reshape(shape)


def balanced(labels, limit):
    """Round-robin by label, the selection examples/realocr and benchocr use."""
    by_class = [[] for _ in range(CLASSES)]
    for i, label in enumerate(labels):
        by_class[int(label)].append(i)
    out, rnd = [], 0
    while len(out) < limit:
        progress = False
        for c in range(CLASSES):
            if len(out) < limit and rnd < len(by_class[c]):
                out.append(by_class[c][rnd])
                progress = True
        if not progress:
            raise SystemExit(f"only {len(out)} balanced examples for the requested {limit}")
        rnd += 1
    return out


def split_id(indices):
    digest = hashlib.sha256(",".join(str(i) for i in indices).encode()).hexdigest()
    return f"balanced_round_robin_by_label_v1:{len(indices)}:{digest}"


def load(data_dir, split, count):
    prefix = "train" if split == "train" else "t10k"
    labels = idx(os.path.join(data_dir, f"{prefix}-labels-idx1-ubyte.gz"), 2049, 1)
    images = idx(os.path.join(data_dir, f"{prefix}-images-idx3-ubyte.gz"), 2051, 3)
    sel = balanced(labels, count)
    x = torch.tensor(images[sel].reshape(len(sel), 784) / 255.0, dtype=torch.float32)
    y = torch.tensor(labels[sel].astype(np.int64))
    return x, y, sel


def build(layers, act):
    sizes = [784] + layers + [CLASSES]
    mods = []
    for i in range(len(sizes) - 1):
        mods.append(nn.Linear(sizes[i], sizes[i + 1]))
        if i < len(sizes) - 2:
            mods.append(nn.Tanh() if act == "tanh" else nn.ReLU())
    return nn.Sequential(*mods)


def train(a):
    torch.manual_seed(a.seed)
    x, y, _ = load(a.data_dir, "train", a.train_count)
    layers = [int(v) for v in a.layers.split(",") if v]
    net = build(layers, a.act)
    opt = torch.optim.Adam(net.parameters(), lr=a.lr)
    lossf = nn.CrossEntropyLoss()
    g = torch.Generator().manual_seed(a.seed)
    for epoch in range(a.epochs):
        for i in torch.randperm(len(x), generator=g).tolist():
            opt.zero_grad()
            loss = lossf(net(x[i:i + 1]), y[i:i + 1])
            loss.backward()
            torch.nn.utils.clip_grad_norm_(net.parameters(), 1.0)
            opt.step()
        print(f"epoch {epoch + 1}/{a.epochs} loss {loss.item():.4f}", file=sys.stderr)
    with open(a.out, "xb") as f:
        torch.save({"layers": layers, "act": a.act, "state": net.state_dict()}, f)


def measure(a):
    torch.set_num_threads(a.threads)
    x, y, sel = load(a.data_dir, "test", a.test_count)
    idle = coimbench.peak_rss()
    saved = torch.load(a.model, weights_only=True)
    net = build(saved["layers"], saved["act"])
    net.load_state_dict(saved["state"])
    net.eval()
    predictions = [0] * len(x)

    def infer(i):
        predictions[i] = int(net(x[i:i + 1]).argmax(1).item())

    with torch.inference_mode():
        latency = coimbench.measure(a.warmup, len(x), infer)
    accuracy = sum(int(p == t) for p, t in zip(predictions, y.tolist())) / len(x)
    nparam = sum(p.numel() for p in net.parameters())
    coimbench.write_result(
        a.out, framework="pytorch", model=a.name or os.path.splitext(os.path.basename(a.model))[0],
        task=TASK, split=split_id(sel),
        host=coimbench.current_host(a.machine, a.gpu, "python" + sys.version.split()[0] + " torch" + torch.__version__),
        parameters=nparam, trainable=nparam,
        peak_rss_bytes=coimbench.peak_rss(), idle_rss_bytes=idle,
        device="cpu", threads=a.threads, latency=latency,
        quality={"accuracy": accuracy}, higher_is_better=["accuracy"],
        notes=["one image per call under torch.inference_mode; latency covers forward and argmax"],
    )


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    t = sub.add_parser("train")
    t.add_argument("--data-dir", required=True)
    t.add_argument("--train-count", type=int, default=6000)
    t.add_argument("--layers", default="28,24")
    t.add_argument("--act", default="tanh")
    t.add_argument("--epochs", type=int, default=20)
    t.add_argument("--lr", type=float, default=0.002)
    t.add_argument("--seed", type=int, default=0)
    t.add_argument("--out", required=True)
    m = sub.add_parser("measure")
    m.add_argument("--data-dir", required=True)
    m.add_argument("--model", required=True)
    m.add_argument("--name", default="")
    m.add_argument("--test-count", type=int, default=2000)
    m.add_argument("--warmup", type=int, default=50)
    m.add_argument("--threads", type=int, default=1)
    m.add_argument("--machine", default="")
    m.add_argument("--gpu", default="")
    m.add_argument("--out", required=True)
    a = ap.parse_args()
    train(a) if a.cmd == "train" else measure(a)


if __name__ == "__main__":
    main()
