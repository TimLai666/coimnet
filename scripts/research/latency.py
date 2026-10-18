"""Batch-1 inference latency (one image = 28 column steps), 1 thread, plus peak RSS."""
import json, sys, time, resource, torch
sys.path.insert(0, __file__.rsplit("/", 1)[0])
from seqlab import RNNBase, CoimCore, make_mask
torch.set_num_threads(1)
spec = json.loads(sys.argv[1])
if spec["model"] in ("gru", "lstm", "rnn"):
    m = RNNBase(spec["model"], spec["h"])
else:
    m = CoimCore(spec["n"], spec["n_in"], spec["n_out"], make_mask(spec["topo"], spec["n"], spec["n_in"], spec["n_out"], spec.get("k", 0), 0), gate=spec.get("gate", False))
m.eval(); x = torch.rand(2000, 28, 28)
ts = []
with torch.inference_mode():
    for i in range(50): m(x[i:i+1])
    for i in range(2000):
        t0 = time.perf_counter(); m(x[i:i+1]); ts.append(time.perf_counter() - t0)
ts.sort()
print(json.dumps({"spec": spec, "params": m.params(), "p50_ms": ts[1000]*1e3, "p95_ms": ts[1900]*1e3, "peak_rss_mib": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss/1024}))
