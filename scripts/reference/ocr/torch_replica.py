#!/usr/bin/env python3
"""Independent PyTorch replica of the tasks/ocr single-character model.

Architecture as actually built by tasks/ocr/recognizer.go NewRecognizer with
InputProfile "single-character" (read from the code, not the docs):

  * nodes = 28 input nodes + H hidden nodes; dense edges from every node
    (input and hidden) to every hidden node, edge index s*H+h; no edge into an
    input node; delays 0; dt = 1; tanh; log_tau = log 2 (frozen).
  * leaky update (dynamics/continuous.go): v' = lam*v + alpha*(in + W^T y + b),
    y' = tanh(v'), lam = exp(-dt/tau), alpha = -expm1(-dt/tau), v0 = y0 = 0.
  * two neural steps: step 1 drives input nodes with x@E (E: 784x28, float32
    Insyra matmul), step 2 has zero external input.
  * readout: final-step tanh activity of the H hidden nodes (or every node
    with readout_all) times R (nodes x 11, float32 Insyra matmul), no bias.
  * loss: softmax cross-entropy over the 10 non-blank columns 1..10 only.
  * optimizer: per-sample AdamW (b1 .9, b2 .999, eps 1e-8 outside sqrt,
    wd 0), global-norm clip 1 over trainable groups (weights, bias, encoder,
    readout; tau frozen), lr 0.002, one update per sample.

Initial parameters, the selected sample indices and the per-epoch order are
read from the Go driver's dump, so data order and init are identical.
"""
import argparse, gzip, json, math, os, sys, time

import numpy as np
import torch

torch.set_num_threads(1)
torch.set_default_dtype(torch.float64)


def read_idx_images(path):
    with gzip.open(path, "rb") as f:
        b = f.read()
    n = int.from_bytes(b[4:8], "big")
    return np.frombuffer(b, dtype=np.uint8, offset=16).reshape(n, 784)


def read_idx_labels(path):
    with gzip.open(path, "rb") as f:
        b = f.read()
    return np.frombuffer(b, dtype=np.uint8, offset=8).astype(np.int64)


class Replica:
    def __init__(self, init, f32=True):
        self.H = init["hidden"]
        self.height = init["height"]
        self.N = self.height + self.H
        self.readout_all = init.get("readout_all", False)
        t = lambda k: torch.tensor(init[k], dtype=torch.float64)
        self.E = t("encoder").reshape(784, self.height).requires_grad_()
        self.W = t("weights").reshape(self.N, self.H).requires_grad_()
        self.b = t("bias").requires_grad_()
        self.logtau = t("log_tau")
        rn = self.N if self.readout_all else self.H
        self.R = t("readout").reshape(rn, 11).requires_grad_()
        tau = torch.exp(self.logtau)
        self.lam = torch.exp(-1.0 / tau)
        self.alpha = -torch.expm1(-1.0 / tau)
        self.f32 = f32
        self.params = [self.W, self.b, self.E, self.R]
        self.m = [torch.zeros_like(p) for p in self.params]
        self.v = [torch.zeros_like(p) for p in self.params]
        self.step_count = 0

    def mm(self, a, b):
        if self.f32:
            return (a.float() @ b.float()).double()
        return a @ b

    def logits(self, x):  # x: (B,784) float64 -> (B,11)
        B = x.shape[0]
        z = self.mm(x, self.E)  # (B,28)
        drive0 = torch.cat([z, torch.zeros(B, self.H)], 1) + self.b
        v1 = self.alpha * drive0
        y1 = torch.tanh(v1)
        rec = y1 @ self.W  # (B,H) into hidden nodes
        drive1 = torch.cat([torch.zeros(B, self.height), rec], 1) + self.b
        v2 = self.lam * v1 + self.alpha * drive1
        y2 = torch.tanh(v2)
        h = y2 if self.readout_all else y2[:, self.height:]
        return self.mm(h, self.R)

    def train_step(self, x, label, lr, clip=1.0, b1=0.9, b2=0.999, eps=1e-8):
        out = self.logits(x[None, :])[0]
        nb = out[1:]
        loss = torch.logsumexp(nb, 0) - nb[label]
        for p in self.params:
            p.grad = None
        loss.backward()
        with torch.no_grad():
            grads = [p.grad for p in self.params]
            norm = math.sqrt(sum(float((g * g).sum()) for g in grads))
            scale = clip / norm if clip > 0 and norm > clip else 1.0
            self.step_count += 1
            c1 = 1 - b1 ** self.step_count
            c2 = 1 - b2 ** self.step_count
            for p, g, m, v in zip(self.params, grads, self.m, self.v):
                d = g * scale
                m.mul_(b1).add_((1 - b1) * d)
                v.mul_(b2).add_((1 - b2) * d * d)
                p.sub_(lr * (m / c1) / (torch.sqrt(v / c2) + eps))
        return float(loss), norm

    @torch.no_grad()
    def evaluate(self, X, y):
        out = self.logits(X)[:, 1:]
        pred = out.argmax(1)
        ce = (torch.logsumexp(out, 1) - out[torch.arange(len(y)), y]).mean()
        return float((pred == y).double().mean()), float(ce)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data-dir", default="data/TSK-11/kmnist")
    ap.add_argument("--dump", required=True, help="Go driver output dir with init.json and orders.json")
    ap.add_argument("--epochs", type=int, default=20)
    ap.add_argument("--lr", type=float, default=0.002)
    ap.add_argument("--log-steps", type=int, default=3000)
    ap.add_argument("--f64", action="store_true", help="float64 encoder/readout matmuls instead of Insyra's float32")
    ap.add_argument("--out", required=True)
    a = ap.parse_args()
    init = json.load(open(os.path.join(a.dump, "init.json")))
    orders = json.load(open(os.path.join(a.dump, "orders.json")))
    tri = read_idx_images(os.path.join(a.data_dir, "train-images-idx3-ubyte.gz"))
    trl = read_idx_labels(os.path.join(a.data_dir, "train-labels-idx1-ubyte.gz"))
    tei = read_idx_images(os.path.join(a.data_dir, "t10k-images-idx3-ubyte.gz"))
    tel = read_idx_labels(os.path.join(a.data_dir, "t10k-labels-idx1-ubyte.gz"))
    Xtr = torch.tensor(tri[init["train_indices"]] / 255.0)
    ytr = torch.tensor(trl[init["train_indices"]])
    assert ytr.tolist() == init["train_labels"], "label mismatch with Go dump"
    Xte = torch.tensor(tei[init["test_indices"]] / 255.0)
    yte = torch.tensor(tel[init["test_indices"]])
    m = Replica(init, f32=not a.f64)
    before = m.evaluate(Xte, yte)
    steps, epochs = [], []
    t0 = time.time()
    u = 0
    for ep in range(a.epochs):
        tot = 0.0
        for i in orders[ep]:
            loss, gn = m.train_step(Xtr[i], int(ytr[i]), a.lr)
            tot += loss
            if u < a.log_steps:
                steps.append({"update": u, "loss": loss, "grad_norm": gn})
            u += 1
        epochs.append({"epoch": ep + 1, "mean_train_loss": tot / len(orders[ep]), "elapsed_s": time.time() - t0})
        print(f"epoch {ep+1} loss {tot/len(orders[ep]):.4f} {time.time()-t0:.0f}s", file=sys.stderr, flush=True)
    after = m.evaluate(Xte, yte)
    train = m.evaluate(Xtr, ytr)
    os.makedirs(a.out, exist_ok=True)
    final = {k: p.detach().reshape(-1).tolist() for k, p in zip(["weights", "bias", "encoder", "readout"], m.params)}
    json.dump(final, open(os.path.join(a.out, "final.json"), "w"))
    json.dump(steps, open(os.path.join(a.out, "steps.json"), "w"))
    res = {"f32_boundaries": not a.f64, "before": {"accuracy": before[0], "ce": before[1]}, "after": {"accuracy": after[0], "ce": after[1]},
           "train": {"accuracy": train[0], "ce": train[1]}, "epochs": epochs, "updates": u}
    json.dump(res, open(os.path.join(a.out, "result.json"), "w"), indent=1)
    print(json.dumps({k: res[k] for k in ["before", "after", "train"]}))


if __name__ == "__main__":
    main()
