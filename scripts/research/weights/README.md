# Reading trained weights

Scripts for inspecting trained `seqkmnist` checkpoints (zero-delay continuous core, softsign current gate, softsign activation; no chemistry, Dale signs or tied parameters). Each reads checkpoint JSON files and prints one JSON line per file.

| Script | What it measures | Needs data |
|---|---|---|
| `inspect_ckpt.py` | Recomputed accuracy next to the recorded one, tau and gate statistics, gate clamp share, effective time scale, dead or saturated neurons, weight signs, readout use, magnitude and saliency pruning, hidden-neuron ablation by hop distance | KMNIST (`KMNIST_DIR`, `LIMIT` test images) |
| `graph.py` | Reciprocity, clustering, direct input edges per readout, inputs reachable within 1 and 2 hops | no |
| `indegree.py` | Per-neuron contribution (ablation) of hidden neurons grouped by in-degree quartile | KMNIST |
| `clamp.py` | Share of neurons stuck at the gate clamp a = 1, which passes no gradient to gate or tau | KMNIST |
| `seqlab-ab.sh` | PyTorch `seqlab.py` A/B on column KMNIST: current gate vs the step-scaling `dt_softsign` gate vs a random graph with 40% reciprocal edges | KMNIST |

Run Python with `python3 -I` when the checkpoints come from another machine. The first study (2026-11-06) found that recomputed and recorded test accuracy match exactly, and its results are in the project's research notes (`weight-reading-2026-11.md`).
