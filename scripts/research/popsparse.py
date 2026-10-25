"""Triton kernels for the population trainer's sparse core (see poplab.py).

The core of every member is run as a fixed in-degree gather: target i of
member p sums W[p, i, j] * y[src[p, i, j]] over its K slots, so the cost
follows the edges rather than nodes squared. One program owns a block of
samples of one member and walks all T steps itself, so a whole sequence is
one launch forward and one backward. The backward reads the source-to-target
transpose of the slots (padded to the largest out-degree with zero weights)
so it gathers instead of scattering, and nothing uses atomics: every sum has
a fixed order and reruns are bit-identical.

core(u, alpha, gg, gb, bias, src, w) -> y_T is a torch.autograd.Function:
  u     [P, B, T, N]  encoded input (zero outside the input nodes)
  alpha, gg, gb, bias [P, N]
  src   [P, N, K] int32 source of each slot, w [P, N, K] its weight
Each step: I = u_t + bias + sum_j w*y[src]; z = gg*I + gb;
a = min(1, alpha*(1 + z/(1+|z|))); v += a*(I - v); y = v/(1+|v|).
"""
import torch
import triton
import triton.language as tl


@triton.jit
def _fwd(U, VH, IH, ALPHA, GG, GB, BIAS, SRC, W,
         B, T, N, K,
         BB: tl.constexpr, NP: tl.constexpr):
    p = tl.program_id(0)
    blk = tl.program_id(1)
    b = blk * BB + tl.arange(0, BB)
    n = tl.arange(0, NP)
    bm = b < B
    nm = n < N
    m2 = bm[:, None] & nm[None, :]
    pn = p * N + n
    alpha = tl.load(ALPHA + pn, mask=nm, other=0.0)[None, :]
    gg = tl.load(GG + pn, mask=nm, other=0.0)[None, :]
    gb = tl.load(GB + pn, mask=nm, other=0.0)[None, :]
    bias = tl.load(BIAS + pn, mask=nm, other=0.0)[None, :]
    # row base of (p, b) in [P, B, T(+1), N] layouts
    urow = (p * B + b) * T
    vrow = (p * B + b) * (T + 1)
    v = tl.zeros((BB, NP), dtype=tl.float32)
    tl.store(VH + vrow[:, None] * N + n[None, :], v, mask=m2)
    tl.debug_barrier()
    for t in range(T):
        acc = tl.load(U + (urow + t)[:, None] * N + n[None, :], mask=m2, other=0.0)
        syn = tl.zeros((BB, NP), dtype=tl.float32)
        for j in range(K):
            s = tl.load(SRC + pn * K + j, mask=nm, other=0)
            wj = tl.load(W + pn * K + j, mask=nm, other=0.0)
            vs = tl.load(VH + (vrow + t)[:, None] * N + s[None, :], mask=m2, other=0.0)
            syn += wj[None, :] * (vs / (1 + tl.abs(vs)))
        I = acc + syn + bias
        z = gg * I + gb
        a = tl.minimum(alpha * (1 + z / (1 + tl.abs(z))), 1.0)
        v = v + a * (I - v)
        tl.store(IH + (urow + t)[:, None] * N + n[None, :], I, mask=m2)
        tl.store(VH + (vrow + t + 1)[:, None] * N + n[None, :], v, mask=m2)
        tl.debug_barrier()


@triton.jit
def _bwd(GYT, VH, IH, DI, ALPHA, GG, GB, OUT_T, OUT_W, PART,
         B, T, N, KO, NB,
         BB: tl.constexpr, NP: tl.constexpr):
    p = tl.program_id(0)
    blk = tl.program_id(1)
    b = blk * BB + tl.arange(0, BB)
    n = tl.arange(0, NP)
    bm = b < B
    nm = n < N
    m2 = bm[:, None] & nm[None, :]
    pn = p * N + n
    alpha = tl.load(ALPHA + pn, mask=nm, other=0.0)[None, :]
    gg = tl.load(GG + pn, mask=nm, other=0.0)[None, :]
    gb = tl.load(GB + pn, mask=nm, other=0.0)[None, :]
    urow = (p * B + b) * T
    vrow = (p * B + b) * (T + 1)
    vT = tl.load(VH + (vrow + T)[:, None] * N + n[None, :], mask=m2, other=0.0)
    d = 1 + tl.abs(vT)
    gvn = tl.load(GYT + (p * B + b)[:, None] * N + n[None, :], mask=m2, other=0.0) / (d * d)
    galpha = tl.zeros((NP,), dtype=tl.float32)
    ggg = tl.zeros((NP,), dtype=tl.float32)
    ggb = tl.zeros((NP,), dtype=tl.float32)
    for r in range(T):
        t = T - 1 - r
        v = tl.load(VH + (vrow + t)[:, None] * N + n[None, :], mask=m2, other=0.0)
        I = tl.load(IH + (urow + t)[:, None] * N + n[None, :], mask=m2, other=0.0)
        z = gg * I + gb
        dz = 1 + tl.abs(z)
        sq = 1 + z / dz
        raw = alpha * sq
        free = raw < 1.0
        a = tl.where(free, raw, 1.0)
        diff = I - v
        gd = tl.where(free, gvn * diff, 0.0)  # gradient reaching a, where unclamped
        slope = 1 / (dz * dz)
        dI = gvn * a + gd * alpha * gg * slope
        galpha += tl.sum(gd * sq, axis=0)
        ggg += tl.sum(gd * alpha * slope * I, axis=0)
        ggb += tl.sum(gd * alpha * slope, axis=0)
        tl.store(DI + (urow + t)[:, None] * N + n[None, :], dI, mask=m2)
        tl.debug_barrier()
        gy = tl.zeros((BB, NP), dtype=tl.float32)
        for o in range(KO):
            tg = tl.load(OUT_T + pn * KO + o, mask=nm, other=0)
            wo = tl.load(OUT_W + pn * KO + o, mask=nm, other=0.0)
            gy += wo[None, :] * tl.load(DI + (urow + t)[:, None] * N + tg[None, :], mask=m2, other=0.0)
        dv = 1 + tl.abs(v)
        gvn = gvn * (1 - a) + gy / (dv * dv)
    base = (p * NB + blk) * 3 * N
    tl.store(PART + base + n, galpha, mask=nm)
    tl.store(PART + base + N + n, ggg, mask=nm)
    tl.store(PART + base + 2 * N + n, ggb, mask=nm)


@triton.jit
def _gw(DI, VH, SRC, GW, B, T, N, K,
        BI: tl.constexpr, RB: tl.constexpr):
    # gW[p, i, j] = sum over (b, t) of dI[p, b, t, i] * y[p, b, t, src[p, i, j]]
    p = tl.program_id(0)
    it = tl.program_id(1)
    i = it * BI + tl.arange(0, BI)
    im = i < N
    pi = p * N + i
    R = B * T
    for j in range(K):
        s = tl.load(SRC + pi * K + j, mask=im, other=0)
        acc = tl.zeros((BI,), dtype=tl.float32)
        for r0 in range(0, R, RB):
            r = r0 + tl.arange(0, RB)
            rm = r < R
            bb = r // T
            tt = r - bb * T
            m2 = rm[:, None] & im[None, :]
            di = tl.load(DI + ((p * B + bb) * T + tt)[:, None] * N + i[None, :], mask=m2, other=0.0)
            vs = tl.load(VH + ((p * B + bb) * (T + 1) + tt)[:, None] * N + s[None, :], mask=m2, other=0.0)
            acc += tl.sum(di * (vs / (1 + tl.abs(vs))), axis=0)
        tl.store(GW + pi * K + j, acc, mask=im)


def transpose_slots(src, w_shape_only_n):
    """Out-edge lists of every source, padded to the largest out-degree.
    Returns (out_target [P, N, KO] int32, out_slot [P, N, KO] int64 flat slot
    index into [N*K] or -1 for padding)."""
    P, N, K = src.shape
    s = src.long().cpu()
    deg = torch.zeros(P, N, dtype=torch.long)
    for p in range(P):
        deg[p] = torch.bincount(s[p].reshape(-1), minlength=N)
    KO = max(int(deg.max()), 1)
    out_t = torch.zeros(P, N, KO, dtype=torch.int32)
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


class Core(torch.autograd.Function):
    BB = 16

    @staticmethod
    def forward(ctx, u, alpha, gg, gb, bias, w, src, out_t, out_slot):
        P, B, T, N = u.shape
        K = src.shape[2]
        NP = triton.next_power_of_2(N)
        u = u.contiguous()
        VH = torch.empty(P, B, T + 1, N, device=u.device, dtype=torch.float32)
        IH = torch.empty(P, B, T, N, device=u.device, dtype=torch.float32)
        BB = Core.BB
        grid = (P, triton.cdiv(B, BB))
        _fwd[grid](u, VH, IH, alpha.contiguous(), gg.contiguous(), gb.contiguous(), bias.contiguous(), src, w.contiguous(),
                   B, T, N, K, BB=BB, NP=NP, num_warps=8)
        ctx.save_for_backward(VH, IH, alpha, gg, gb, w, src, out_t, out_slot)
        vT = VH[:, :, T]
        return vT / (1 + vT.abs())

    @staticmethod
    def backward(ctx, gyT):
        VH, IH, alpha, gg, gb, w, src, out_t, out_slot = ctx.saved_tensors
        P, B, T1, N = VH.shape
        T = T1 - 1
        K = src.shape[2]
        KO = out_t.shape[2]
        NP = triton.next_power_of_2(N)
        BB = Core.BB
        NB = triton.cdiv(B, BB)
        flat_w = torch.cat([w.reshape(P, N * K), torch.zeros(P, 1, device=w.device, dtype=w.dtype)], 1)
        out_w = torch.gather(flat_w, 1, torch.where(out_slot < 0, N * K, out_slot).reshape(P, -1)).view(P, N, KO).contiguous()
        DI = torch.empty(P, B, T, N, device=VH.device, dtype=torch.float32)
        PART = torch.empty(P, NB, 3, N, device=VH.device, dtype=torch.float32)
        _bwd[(P, NB)](gyT.contiguous(), VH, IH, DI, alpha.contiguous(), gg.contiguous(), gb.contiguous(), out_t, out_w, PART,
                      B, T, N, KO, NB, BB=BB, NP=NP, num_warps=8)
        GW = torch.empty(P, N, K, device=VH.device, dtype=torch.float32)
        BI = 64
        _gw[(P, triton.cdiv(N, BI))](DI, VH, src, GW, B, T, N, K, BI=BI, RB=64, num_warps=4)
        part = PART.sum(1)
        gbias = DI.sum((1, 2))
        return DI, part[:, 0], part[:, 1], part[:, 2], gbias, GW, None, None, None


def core(u, alpha, gg, gb, bias, w, src, out_t, out_slot):
    return Core.apply(u, alpha, gg, gb, bias, w, src, out_t, out_slot)
