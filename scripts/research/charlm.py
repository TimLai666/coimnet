"""Byte-level language model baselines (GRU/LSTM) on the exact split, vocabulary
and windows of examples/charlm, so bits per byte compare directly.

The split, vocabulary and evaluation windows follow examples/charlm/main.go:
.jsonl lines carry a "text" field, document i goes to test when i % 100 == 0,
to validation when i % 100 == 1, else to training (each stripped and ended by
a newline); a .txt file is cut 90/5/5. The vocabulary is the training bytes in
byte order plus one id for unseen bytes. Training draws random windows; the
optimizer is AdamW (no weight decay, clip norm 1) with 10% linear warmup and
cosine decay to 1e-3 of the peak, one update per batch of windows."""
import argparse, hashlib, json, math, time
import numpy as np, torch, torch.nn as nn, torch.nn.functional as F


def read_text(path, limit):
    raw = open(path, "rb").read(limit)
    if not path.endswith(".jsonl"):
        n = len(raw); a, b = n * 90 // 100, n * 95 // 100
        return raw[:a], raw[a:b], raw[b:], hashlib.sha256(raw).hexdigest()
    train, val, test = bytearray(), bytearray(), bytearray()
    for i, line in enumerate(raw.split(b"\n")):
        try:
            doc = json.loads(line)
        except ValueError:
            break  # the limit can cut the last line short
        text = doc["text"].strip().encode() + b"\n"
        {0: test, 1: val}.get(i % 100, train).extend(text)
    return bytes(train), bytes(val), bytes(test), hashlib.sha256(raw).hexdigest()


class Vocab:
    def __init__(s, train):
        s.bytes = sorted(set(train))
        s.ids = np.full(256, len(s.bytes), np.int64)
        for i, b in enumerate(s.bytes):
            s.ids[b] = i
    def size(s): return len(s.bytes) + 1
    def encode(s, text): return s.ids[np.frombuffer(text, np.uint8)]


class RNNLM(nn.Module):
    def __init__(s, kind, h, vocab):
        super().__init__()
        s.vocab = vocab
        s.rnn = {"gru": nn.GRU, "lstm": nn.LSTM}[kind](vocab, h, batch_first=True)
        s.R = nn.Linear(h, vocab)
    def params(s): return sum(p.numel() for p in s.parameters())
    def forward(s, ids):  # ids [B,T] -> logits [B,T,V]
        y, _ = s.rnn(F.one_hot(ids, s.vocab).float())
        return s.R(y)


def bits(model, stream, window, count, device):
    starts = list(range(0, len(stream) - window, window))[:count]
    model.eval(); nll = 0.0
    with torch.no_grad():
        for i in range(0, len(starts), 256):
            chunk = np.stack([stream[a:a + window + 1] for a in starts[i:i + 256]])
            x = torch.from_numpy(chunk).to(device)
            logits = model(x[:, :-1]).double()
            nll += F.cross_entropy(logits.reshape(-1, logits.shape[-1]), x[:, 1:].reshape(-1), reduction="sum").item()
    model.train()
    return nll / (len(starts) * window) / math.log(2)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--text", required=True); ap.add_argument("--max-bytes", type=int, default=32 << 20)
    ap.add_argument("--kind", default="gru"); ap.add_argument("--hidden", type=int, required=True)
    ap.add_argument("--window", type=int, default=64); ap.add_argument("--updates", type=int, default=2000)
    ap.add_argument("--batch", type=int, default=64); ap.add_argument("--lr", type=float, default=1e-2)
    ap.add_argument("--eval-every", type=int, default=250); ap.add_argument("--eval-windows", type=int, default=256)
    ap.add_argument("--seed", type=int, default=0); ap.add_argument("--threads", type=int, default=1)
    ap.add_argument("--device", default="cpu"); ap.add_argument("--report")
    a = ap.parse_args()
    torch.manual_seed(a.seed); torch.set_num_threads(a.threads)
    tr_b, va_b, te_b, digest = read_text(a.text, a.max_bytes)
    v = Vocab(tr_b)
    train, val, test = v.encode(tr_b), v.encode(va_b), v.encode(te_b)
    model = RNNLM(a.kind, a.hidden, v.size()).to(a.device)
    print(f"vocabulary {v.size()} train {len(train)} val {len(val)} test {len(test)} bytes; {a.kind} h{a.hidden} parameters {model.params()}", flush=True)
    opt = torch.optim.AdamW(model.parameters(), lr=a.lr, weight_decay=0)
    warmup = a.updates // 10
    def factor(u):
        if u < warmup: return (u + 1) / warmup
        p = (u - warmup) / max(1, a.updates - warmup)
        return 1e-3 + (1 - 1e-3) * 0.5 * (1 + math.cos(math.pi * p))
    sched = torch.optim.lr_scheduler.LambdaLR(opt, factor)
    rng = np.random.default_rng(a.seed); curve = []; t0 = time.time()
    for u in range(a.updates):
        starts = rng.integers(0, len(train) - a.window, a.batch)
        x = torch.from_numpy(np.stack([train[s:s + a.window + 1] for s in starts])).to(a.device)
        logits = model(x[:, :-1])
        loss = F.cross_entropy(logits.reshape(-1, logits.shape[-1]), x[:, 1:].reshape(-1))
        opt.zero_grad(); loss.backward()
        nn.utils.clip_grad_norm_(model.parameters(), 1.0)
        opt.step(); sched.step()
        if (u + 1) % a.eval_every == 0 or u + 1 == a.updates:
            vb = bits(model, val, a.window, a.eval_windows, a.device)
            curve.append({"updates": u + 1, "train_batch_loss": loss.item(), "val_bits_per_byte": vb, "train_seconds": time.time() - t0})
            print(f"update {u + 1} batch loss {loss.item():.3f} val {vb:.4f} bits/byte ({time.time() - t0:.0f}s)", flush=True)
    tb = bits(model, test, a.window, a.eval_windows, a.device)
    print(f"test {tb:.4f} bits/byte", flush=True)
    if a.report:
        with open(a.report, "x") as f:
            json.dump({"settings": vars(a), "text_sha256": digest, "vocabulary": v.size(), "parameters": model.params(),
                       "curve": curve, "test_bits_per_byte": tb, "train_seconds": time.time() - t0}, f, indent=2)


if __name__ == "__main__":
    main()
