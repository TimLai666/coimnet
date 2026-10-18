#!/usr/bin/env python3
"""PyTorch MLP side of examples/onlineocr: the same KMNIST stream protocol.

pretrain: fit the 784-28-24-10 tanh MLP on the scenario's pretraining images
          (600 per class in file order, labels 0-4 for new-classes, all ten for
          domain), one image per Adam step in a fixed order, and save the model
          with its optimizer state.
run:      predict each stream image, score it, then (learner sgd) take one Adam
          step on its true label, continuing the saved optimizer state. frozen
          never updates. Writes the same coimnet-online-ocr/v1 fields.

    python3 scripts/bench/kmnist_online.py pretrain --data-dir D --scenario domain --out M.pt
    python3 scripts/bench/kmnist_online.py run --data-dir D --scenario domain --model M.pt --learner sgd --out R.json
"""
import argparse
import json
import math
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import coimbench  # noqa: E402
from kmnist_mlp import CLASSES, balanced, build, idx, split_id  # noqa: E402

import torch  # noqa: E402
import torch.nn as nn  # noqa: E402

PRETRAIN_PER_CLASS, STREAM_FROM, STREAM_PER_CLASS = 600, 600, 200
TUNE_FROM, TUNE_PER_CLASS, TEST_COUNT, WINDOW = 800, 100, 2000, 200
ALL = list(range(CLASSES))


def scenario_classes(name):
    if name == "new-classes":
        return [0, 1, 2, 3, 4], False
    if name == "domain":
        return ALL, True
    raise SystemExit(f"unknown scenario {name!r}")


def round_robin(labels, classes, start, count):
    by_class = [[] for _ in range(CLASSES)]
    for i, label in enumerate(labels):
        by_class[int(label)].append(i)
    out = []
    for k in range(start, start + count):
        for c in classes:
            if k >= len(by_class[c]):
                raise SystemExit(f"class {c} has no image at position {k}")
            out.append(by_class[c][k])
    return out


def load(data_dir, split):
    labels = idx(os.path.join(data_dir, f"{split}-labels-idx1-ubyte.gz"), 2049, 1)
    images = idx(os.path.join(data_dir, f"{split}-images-idx3-ubyte.gz"), 2051, 3).reshape(-1, 784)
    return labels, images


def tensors(labels, images, sel, invert):
    x = torch.tensor(images[sel] / 255.0, dtype=torch.float32)
    if invert:
        x = 1 - x
    return x, torch.tensor(labels[sel].astype("int64"))


def step(net, opt, lossf, x, y):
    opt.zero_grad()
    lossf(net(x), y).backward()
    torch.nn.utils.clip_grad_norm_(net.parameters(), 1.0)
    opt.step()


def pretrain(a):
    torch.set_num_threads(1)
    torch.manual_seed(a.seed)
    classes, _ = scenario_classes(a.scenario)
    labels, images = load(a.data_dir, "train")
    x, y = tensors(labels, images, round_robin(labels, classes, 0, PRETRAIN_PER_CLASS), False)
    net = build([28, 24], "tanh")
    opt = torch.optim.Adam(net.parameters(), lr=a.lr)
    lossf = nn.CrossEntropyLoss()
    for e in range(a.epochs):
        for i in range(len(x)):
            step(net, opt, lossf, x[i:i + 1], y[i:i + 1])
        print(f"epoch {e + 1}/{a.epochs}", file=sys.stderr)
    with open(a.out, "xb") as f:
        torch.save({"state": net.state_dict(), "opt": opt.state_dict(), "lr": a.lr}, f)


def run(a):
    torch.set_num_threads(1)
    pretrained, invert = scenario_classes(a.scenario)
    labels, images = load(a.data_dir, "train")
    tlabels, timages = load(a.data_dir, "t10k")
    start, per = (STREAM_FROM, STREAM_PER_CLASS) if a.stream == "main" else (TUNE_FROM, TUNE_PER_CLASS)
    stream = round_robin(labels, ALL, start, per)
    test = balanced(tlabels, TEST_COUNT)
    saved = torch.load(a.model, weights_only=True)
    net = build([28, 24], "tanh")
    net.load_state_dict(saved["state"])
    opt = torch.optim.Adam(net.parameters(), lr=saved["lr"])
    opt.load_state_dict(saved["opt"])
    lossf = nn.CrossEntropyLoss()

    def predict(x):
        with torch.inference_mode():
            return int(net(x).argmax(1).item())

    def evaluate():
        out = {}
        for shift in ([False, True] if invert else [False]):
            x, y = tensors(tlabels, timages, test, shift)
            with torch.inference_mode():
                p = net(x).argmax(1)
            hit = (p == y)
            if shift:
                out["shifted"] = hit.float().mean().item()
                continue
            out["original"] = hit.float().mean().item()
            if not invert:
                old = torch.tensor([int(v) in pretrained for v in y])
                out["original_old"] = hit[old].float().mean().item()
                out["original_new"] = hit[~old].float().mean().item()
        return out

    before = evaluate()
    xs, ys = tensors(labels, images, stream, invert)
    hits = win = old_hits = old_n = new_hits = new_n = 0
    windows, predict_ms, update_ms = [], [], 0.0
    for i in range(len(xs)):
        x, y = xs[i:i + 1], ys[i:i + 1]
        t0 = time.perf_counter_ns()
        p = predict(x)
        predict_ms.append((time.perf_counter_ns() - t0) / 1e6)
        ok = p == int(y.item())
        hits += ok
        win += ok
        if not invert:
            if int(y.item()) in pretrained:
                old_n += 1
                old_hits += ok
            else:
                new_n += 1
                new_hits += ok
        t0 = time.perf_counter_ns()
        if a.learner == "sgd":
            step(net, opt, lossf, x, y)
        update_ms += (time.perf_counter_ns() - t0) / 1e6
        if (i + 1) % WINDOW == 0:
            windows.append(win / WINDOW)
            win = 0
    after = evaluate()
    predict_ms.sort()
    res = {
        "schema_version": "coimnet-online-ocr/v1", "framework": "pytorch", "scenario": a.scenario,
        "learner": a.learner, "stream": a.stream,
        "stream_split": split_id(stream).replace("balanced_round_robin_by_label_v1", "round_robin_by_label_v1"),
        "test_split": split_id(test),
        "host": coimbench.current_host(a.machine, "", "python" + sys.version.split()[0] + " torch" + torch.__version__),
        "parameters": sum(p.numel() for p in net.parameters()),
        "prequential": windows, "prequential_all": hits / len(xs),
        "test_before": before, "test": after,
        "predict_p50_ms": predict_ms[math.ceil(.5 * len(predict_ms)) - 1],
        "update_mean_ms": update_ms / len(xs), "peak_rss_bytes": coimbench.peak_rss(),
    }
    if not invert:
        res["prequential_old"], res["prequential_new"] = old_hits / old_n, new_hits / new_n
    with open(a.out, "x") as f:
        json.dump(res, f, indent=2)
        f.write("\n")
    print(f"{a.scenario} {a.learner} prequential {res['prequential_all']:.4f} test {after}")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    t = sub.add_parser("pretrain")
    t.add_argument("--data-dir", required=True)
    t.add_argument("--scenario", required=True)
    t.add_argument("--epochs", type=int, default=20)
    t.add_argument("--lr", type=float, default=0.002)
    t.add_argument("--seed", type=int, default=0)
    t.add_argument("--out", required=True)
    r = sub.add_parser("run")
    r.add_argument("--data-dir", required=True)
    r.add_argument("--scenario", required=True)
    r.add_argument("--model", required=True)
    r.add_argument("--learner", choices=["frozen", "sgd"], required=True)
    r.add_argument("--stream", choices=["main", "tune"], default="main")
    r.add_argument("--machine", default="")
    r.add_argument("--out", required=True)
    a = ap.parse_args()
    pretrain(a) if a.cmd == "pretrain" else run(a)


if __name__ == "__main__":
    main()
