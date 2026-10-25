"""Times one training update of the population sparse core (poplab.py
--kernel sparse) on random KMNIST-shaped input, for kernel tuning sweeps.
The POP_* environment variables of popsparse.py set the launch shapes.

  popbench.py [--members 32] [--n 512] [--k 36] [--bs 128] [--reps 20]
"""
import argparse, json, os, time
import torch
import poplab

ap = argparse.ArgumentParser()
ap.add_argument("--members", type=int, default=32)
ap.add_argument("--n", type=int, default=512)
ap.add_argument("--k", type=int, default=36)
ap.add_argument("--bs", type=int, default=128)
ap.add_argument("--reps", type=int, default=20)
a = ap.parse_args()
torch.set_num_threads(1)
dev = torch.device("cuda")
members = [{"n": a.n, "k": a.k, "n_in": 128, "n_out": a.n, "lr": 0.01, "seed": s} for s in range(a.members)]
pop = poplab.Population(members, 28, 10, dev, "sparse")
g = torch.Generator(device=dev).manual_seed(0)
x = torch.rand(a.members, a.bs, 28, 28, device=dev, generator=g)
y = torch.randint(0, 10, (a.members, a.bs), device=dev, generator=g)
lrs = torch.full((a.members,), 1e-3, device=dev)


def step():
    logits = pop.forward(x)
    loss = torch.nn.functional.cross_entropy(logits.reshape(-1, 10), y.reshape(-1), reduction="none").view(a.members, -1).mean(1).sum()
    loss.backward()
    pop.adam_step(lrs)


for _ in range(3):
    step()
torch.cuda.synchronize()
t0 = time.time()
for _ in range(a.reps):
    step()
torch.cuda.synchronize()
ms = (time.time() - t0) / a.reps * 1000
tune = {k: v for k, v in os.environ.items() if k.startswith("POP_")}
print("BENCH", json.dumps({"members": a.members, "n": a.n, "k": a.k, "bs": a.bs, "ms_per_update": ms,
                           "ms_per_member_update": ms / a.members, "tune": tune}), flush=True)
