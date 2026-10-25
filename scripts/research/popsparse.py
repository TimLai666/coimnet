"""Triton kernels for the population trainer's sparse core (see poplab.py).

The core of every member is run as a fixed in-degree gather: target i of
member p sums W[p, i, j] * y[src[p, i, j]] over its K slots, so the cost
follows the edges rather than nodes squared. One program owns a block of
samples of one member and walks all T steps itself, so a whole sequence is
one launch forward and one backward. The backward reads the source-to-target
transpose of the slots (padded to the largest out-degree with zero weights)
so it gathers instead of scattering, and nothing uses atomics: every sum has
a fixed order and reruns are bit-identical.

core(u, alpha, gg, gb, bias, w, src, out_t, out_slot) -> y_T [P, B, N] is a torch.autograd.Function:
  u     [P, T, N, B]  encoded input (zero outside the input nodes), sample-minor
                      so that a slot's gather over a block of samples is contiguous
  alpha, gg, gb, bias [P, N]
  src   [P, N, K] int32 source of each slot, w [P, N, K] its weight
Each step: I = u_t + bias + sum_j w*y[src]; z = gg*I + gb;
a = min(1, alpha*(1 + z/(1+|z|))); v += a*(I - v); y = v/(1+|v|).
"""
import os
import torch
import triton
import triton.language as tl


@triton.jit
def _fwd(U, VH, IH, ALPHA, GG, GB, BIAS, SRC, W,
         B, T, N, K,
         BB: tl.constexpr, NP: tl.constexpr):
    # Layouts are sample-minor, [P, T(+1), N, B], so a slot's gather of one
    # source over a block of samples reads contiguous memory.
    p = tl.program_id(0)
    blk = tl.program_id(1)
    b = blk * BB + tl.arange(0, BB)
    n = tl.arange(0, NP)
    bm = b < B
    nm = n < N
    m2 = nm[:, None] & bm[None, :]
    pn = p * N + n
    alpha = tl.load(ALPHA + pn, mask=nm, other=0.0)[:, None]
    gg = tl.load(GG + pn, mask=nm, other=0.0)[:, None]
    gb = tl.load(GB + pn, mask=nm, other=0.0)[:, None]
    bias = tl.load(BIAS + pn, mask=nm, other=0.0)[:, None]
    cols = b[None, :]
    v = tl.zeros((NP, BB), dtype=tl.float32)
    tl.store(VH + ((p * (T + 1)) * N + n[:, None]) * B + cols, v, mask=m2)
    tl.debug_barrier()
    for t in range(T):
        here = (p * T + t) * N
        prev = (p * (T + 1) + t) * N
        acc = tl.load(U + (here + n[:, None]) * B + cols, mask=m2, other=0.0)
        for j in range(K):
            s = tl.load(SRC + pn * K + j, mask=nm, other=0)
            wj = tl.load(W + pn * K + j, mask=nm, other=0.0)
            vs = tl.load(VH + (prev + s[:, None]) * B + cols, mask=m2, other=0.0)
            acc += wj[:, None] * (vs / (1 + tl.abs(vs)))
        I = acc + bias
        z = gg * I + gb
        a = tl.minimum(alpha * (1 + z / (1 + tl.abs(z))), 1.0)
        v = v + a * (I - v)
        tl.store(IH + (here + n[:, None]) * B + cols, I, mask=m2)
        tl.store(VH + (prev + N + n[:, None]) * B + cols, v, mask=m2)
        tl.debug_barrier()


@triton.jit
def _bwd(GYT, VH, IH, DI, ALPHA, GG, GB, OUT_T, OUT_W, PART, SRC, GWP,
         B, T, N, KO, NB, K,
         BB: tl.constexpr, NP: tl.constexpr, FUSE: tl.constexpr):
    # With FUSE, the weight gradient of this block's samples is accumulated
    # here as well, into GWP[p, blk, j, n], instead of by _gw afterwards.
    p = tl.program_id(0)
    blk = tl.program_id(1)
    b = blk * BB + tl.arange(0, BB)
    n = tl.arange(0, NP)
    bm = b < B
    nm = n < N
    m2 = nm[:, None] & bm[None, :]
    pn = p * N + n
    alpha = tl.load(ALPHA + pn, mask=nm, other=0.0)[:, None]
    gg = tl.load(GG + pn, mask=nm, other=0.0)[:, None]
    gb = tl.load(GB + pn, mask=nm, other=0.0)[:, None]
    cols = b[None, :]
    vT = tl.load(VH + ((p * (T + 1) + T) * N + n[:, None]) * B + cols, mask=m2, other=0.0)
    d = 1 + tl.abs(vT)
    gvn = tl.load(GYT + (p * N + n[:, None]) * B + cols, mask=m2, other=0.0) / (d * d)
    galpha = tl.zeros((NP,), dtype=tl.float32)
    ggg = tl.zeros((NP,), dtype=tl.float32)
    ggb = tl.zeros((NP,), dtype=tl.float32)
    for r in range(T):
        t = T - 1 - r
        here = (p * T + t) * N
        v = tl.load(VH + ((p * (T + 1) + t) * N + n[:, None]) * B + cols, mask=m2, other=0.0)
        I = tl.load(IH + (here + n[:, None]) * B + cols, mask=m2, other=0.0)
        z = gg * I + gb
        dz = 1 + tl.abs(z)
        sq = 1 + z / dz
        raw = alpha * sq
        free = raw < 1.0
        a = tl.where(free, raw, 1.0)
        gd = tl.where(free, gvn * (I - v), 0.0)  # gradient reaching a, where unclamped
        slope = 1 / (dz * dz)
        dI = gvn * a + gd * alpha * gg * slope
        galpha += tl.sum(gd * sq, axis=1)
        ggg += tl.sum(gd * alpha * slope * I, axis=1)
        ggb += tl.sum(gd * alpha * slope, axis=1)
        tl.store(DI + (here + n[:, None]) * B + cols, dI, mask=m2)
        if FUSE:
            gbase = (p * NB + blk) * K * N
            for j in range(K):
                sj = tl.load(SRC + pn * K + j, mask=nm, other=0)
                vs = tl.load(VH + ((p * (T + 1) + t) * N + sj[:, None]) * B + cols, mask=m2, other=0.0)
                c = tl.sum(dI * (vs / (1 + tl.abs(vs))), axis=1)
                gp = GWP + gbase + j * N + n
                tl.store(gp, tl.load(gp, mask=nm, other=0.0) + c, mask=nm)
        tl.debug_barrier()
        gy = tl.zeros((NP, BB), dtype=tl.float32)
        for o in range(KO):
            tg = tl.load(OUT_T + pn * KO + o, mask=nm, other=-1)
            wo = tl.load(OUT_W + pn * KO + o, mask=nm, other=0.0)
            # Padding slots (-1) past a source's out-degree load nothing.
            gy += wo[:, None] * tl.load(DI + (here + tg[:, None]) * B + cols, mask=m2 & (tg >= 0)[:, None], other=0.0)
        dv = 1 + tl.abs(v)
        gvn = gvn * (1 - a) + gy / (dv * dv)
    base = (p * NB + blk) * 3 * N
    tl.store(PART + base + n, galpha, mask=nm)
    tl.store(PART + base + N + n, ggg, mask=nm)
    tl.store(PART + base + 2 * N + n, ggb, mask=nm)


@triton.jit
def _gw(DI, VH, SRC, GW, B, T, N, K,
        BI: tl.constexpr, KP: tl.constexpr, RB: tl.constexpr):
    # gW[p, i, j] = sum over (t, b) of dI[p, t, i, b] * y[p, t, src[p, i, j], b].
    # One program per (target block, member) holds all K slots, so each dI
    # tile is read once; the target block is the fast grid axis, so the
    # programs of one member run together and share its history in L2.
    it = tl.program_id(0)
    p = tl.program_id(1)
    i = it * BI + tl.arange(0, BI)
    jj = tl.arange(0, KP)
    im = i < N
    pi = p * N + i
    sm = im[:, None] & (jj < K)[None, :]
    s = tl.load(SRC + pi[:, None] * K + jj[None, :], mask=sm, other=0)
    acc = tl.zeros((BI, KP), dtype=tl.float32)
    for t in range(T):
        for b0 in range(0, B, RB):
            b = b0 + tl.arange(0, RB)
            bm = b < B
            di = tl.load(DI + ((p * T + t) * N + i[:, None]) * B + b[None, :], mask=im[:, None] & bm[None, :], other=0.0)
            vs = tl.load(VH + ((p * (T + 1) + t) * N + s[:, :, None]) * B + b[None, None, :], mask=sm[:, :, None] & bm[None, None, :], other=0.0)
            acc += tl.sum(di[:, None, :] * (vs / (1 + tl.abs(vs))), axis=2)
    tl.store(GW + pi[:, None] * K + jj[None, :], acc, mask=sm)


def transpose_slots(src, w_shape_only_n):
    """Out-edge lists of every source, padded to the largest out-degree.
    Returns (out_target [P, N, KO] int32 or -1 for padding, out_slot [P, N, KO] int64 flat slot
    index into [N*K] or -1 for padding)."""
    P, N, K = src.shape
    s = src.long().cpu()
    deg = torch.zeros(P, N, dtype=torch.long)
    for p in range(P):
        deg[p] = torch.bincount(s[p].reshape(-1), minlength=N)
    KO = max(int(deg.max()), 1)
    out_t = torch.full((P, N, KO), -1, dtype=torch.int32)
    out_slot = torch.full((P, N, KO), -1, dtype=torch.long)
    for p in range(P):
        flat = s[p].reshape(-1)  # slot e = i*K + j has source flat[e], target i
        order = torch.argsort(flat, stable=True)
        srcs = flat[order]
        start = torch.zeros(N + 1, dtype=torch.long)
        start[1:] = torch.cumsum(deg[p], 0)
        pos = torch.arange(N * K) - start[srcs]
        out_t[p, srcs, pos] = (order // K).int()
        out_slot[p, srcs, pos] = order
    return out_t, out_slot, KO


# Launch shapes; the environment overrides are for tuning sweeps.
TUNE = {k: int(os.environ.get("POP_" + k, d)) for k, d in
        (("BB", 32), ("WARPS", 16), ("GW_BI", 8), ("GW_RB", 32), ("GW_WARPS", 8), ("FUSE_GW", 1))}


class Core(torch.autograd.Function):
    BB = TUNE["BB"]

    @staticmethod
    def forward(ctx, u, alpha, gg, gb, bias, w, src, out_t, out_slot):
        P, T, N, B = u.shape
        K = src.shape[2]
        NP = triton.next_power_of_2(N)
        U = u.contiguous()
        VH = torch.empty(P, T + 1, N, B, device=u.device, dtype=torch.float32)
        IH = torch.empty(P, T, N, B, device=u.device, dtype=torch.float32)
        BB = Core.BB
        grid = (P, triton.cdiv(B, BB))
        _fwd[grid](U, VH, IH, alpha.contiguous(), gg.contiguous(), gb.contiguous(), bias.contiguous(), src, w.contiguous(),
                   B, T, N, K, BB=BB, NP=NP, num_warps=TUNE["WARPS"])
        ctx.save_for_backward(VH, IH, alpha, gg, gb, w, src, out_t, out_slot)
        vT = VH[:, T].transpose(1, 2)  # [P, B, N]
        return vT / (1 + vT.abs())

    @staticmethod
    def backward(ctx, gyT):
        VH, IH, alpha, gg, gb, w, src, out_t, out_slot = ctx.saved_tensors
        P, T1, N, B = VH.shape
        T = T1 - 1
        K = src.shape[2]
        KO = out_t.shape[2]
        NP = triton.next_power_of_2(N)
        BB = Core.BB
        NB = triton.cdiv(B, BB)
        flat_w = torch.cat([w.reshape(P, N * K), torch.zeros(P, 1, device=w.device, dtype=w.dtype)], 1)
        out_w = torch.gather(flat_w, 1, torch.where(out_slot < 0, N * K, out_slot).reshape(P, -1)).view(P, N, KO).contiguous()
        DI = torch.empty(P, T, N, B, device=VH.device, dtype=torch.float32)
        PART = torch.empty(P, NB, 3, N, device=VH.device, dtype=torch.float32)
        fuse = TUNE["FUSE_GW"]
        GWP = torch.zeros(P, NB, K, N, device=VH.device, dtype=torch.float32) if fuse else PART
        _bwd[(P, NB)](gyT.transpose(1, 2).contiguous(), VH, IH, DI, alpha.contiguous(), gg.contiguous(), gb.contiguous(), out_t, out_w, PART, src, GWP,
                      B, T, N, KO, NB, K, BB=BB, NP=NP, FUSE=fuse, num_warps=TUNE["WARPS"])
        if fuse:
            GW = GWP.sum(1).transpose(1, 2).contiguous()
        else:
            GW = torch.empty(P, N, K, device=VH.device, dtype=torch.float32)
            BI = TUNE["GW_BI"]
            _gw[(triton.cdiv(N, BI), P)](DI, VH, src, GW, B, T, N, K, BI=BI, KP=triton.next_power_of_2(K), RB=TUNE["GW_RB"], num_warps=TUNE["GW_WARPS"])
        part = PART.sum(1)
        gbias = DI.sum((1, 3))
        return DI, part[:, 0], part[:, 1], part[:, 2], gbias, GW, None, None, None


def core(u, alpha, gg, gb, bias, w, src, out_t, out_slot):
    return Core.apply(u, alpha, gg, gb, bias, w, src, out_t, out_slot)
