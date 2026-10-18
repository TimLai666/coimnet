#!/usr/bin/env python3
"""Counts which features the random cases in cases.json exercised."""
import json, sys, collections
cases = json.load(open(sys.argv[1]))
c = collections.Counter()
for k in cases:
    if k["kind"] == "core":
        cfg = k["config"]; c["core cases"] += 1
        c[f"core window={k['window']}"] += 1
        c[f"core act={cfg['activation']}"] += 1
        if cfg.get("delays") and max(cfg["delays"]) > 0: c["core delays>0"] += 1
        s, t = cfg.get("sources") or [], cfg.get("targets") or []
        if any(a == b for a, b in zip(s, t)): c["core self-loop"] += 1
        if len(set(zip(s, t))) < len(s): c["core duplicate edge"] += 1
        if k.get("state") and k["seg_window"] > 0: c["stateful segment window>0"] += 1
        if k.get("state"): c["stateful segment"] += 1
        continue
    cfg, o = k["config"], k["options"]; c["net cases"] += 1
    if cfg.get("input_nodes") is not None: c["InputNodes subset"] += 1
    if cfg.get("readout_every_step"): c["ReadoutEveryStep"] += 1
    if any(cfg.get("edge_signs") or []): c["fixed-sign edges"] += 1
    if cfg.get("sharing"): c["sharing"] += 1
    if o.get("masks"): c["masks"] += 1
    if o.get("ranges"): c["ranges"] += 1
    if o.get("schedule"): c[f"schedule {o['schedule']['kind']}"] += 1
    if (o.get("accumulate_steps") or 0) > 1: c["accumulate>1"] += 1
    if (o.get("loss_scale") or 0) not in (0, 1): c["loss scale != 1"] += 1
    if o["weight_decay"] > 0: c["weight decay"] += 1
    c[f"truncation={o['truncation']}"] += 1
    for st in k["steps"]:
        r = st["result"]; c["trainer steps"] += 1
        if not r.get("applied"): c["steps not applied (accumulating)"] += 1
        if o["clip_norm"] > 0 and r.get("gradient_norm", 0) > o["clip_norm"]: c["steps with clip active (norm>clip)"] += 1
        for key in (r.get("projected") or {}): c[f"projected {key}"] += 1
for key in sorted(c): print(f"{c[key]:6d}  {key}")
