#!/usr/bin/env python3
"""Recompute the 4-NN baseline and check selection/leakage on the dumped indices."""
import gzip, json, sys, os
import numpy as np
d, dump = sys.argv[1], sys.argv[2]
def img(p):
    b = gzip.open(p).read(); return np.frombuffer(b, np.uint8, offset=16).reshape(-1, 784)
def lab(p):
    b = gzip.open(p).read(); return np.frombuffer(b, np.uint8, offset=8).astype(int)
init = json.load(open(os.path.join(dump, "init.json")))
tri, trl = img(f"{d}/train-images-idx3-ubyte.gz"), lab(f"{d}/train-labels-idx1-ubyte.gz")
tei, tel = img(f"{d}/t10k-images-idx3-ubyte.gz"), lab(f"{d}/t10k-labels-idx1-ubyte.gz")
tr, te = np.array(init["train_indices"]), np.array(init["test_indices"])
Xtr, ytr = tri[tr] / 255.0, trl[tr]; Xte, yte = tei[te] / 255.0, tel[te]
# selection checks
print("train per class", np.bincount(ytr), "test per class", np.bincount(yte))
print("test indices start at per-class rank 20:", all(sorted(np.where(tel == c)[0][20:220]) == sorted(te[yte == c]) for c in range(10)))
print("unique train/test", len(set(tr)), len(set(te)))
# exact-duplicate images across train/test (pixel-identical)
h = {tri[i].tobytes() for i in tr}
print("test images pixel-identical to a selected train image:", sum(tei[i].tobytes() in h for i in te))
D = (Xte**2).sum(1)[:, None] + (Xtr**2).sum(1)[None, :] - 2 * Xte @ Xtr.T
for k in (1, 3, 4, 5):
    nn = np.argsort(D, 1, kind="stable")[:, :k]
    pred = np.array([np.bincount(ytr[r], minlength=10).argmax() for r in nn])
    print(f"{k}-NN accuracy", (pred == yte).mean())
