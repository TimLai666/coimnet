package webgpu

// The continuous batch kernels share one Dims uniform, one prelude and the
// buffer layouts below. Every kernel indexes a flat invocation id built from a
// two-dimensional dispatch, so a launch larger than the per-dimension
// workgroup limit still covers every element exactly once.
//
// Layouts (b batch, t step, i node, B batch size, T steps, N nodes, E edges,
// W input width, R readout nodes, S readout steps, BN = B*N, P = T*BN):
//
//	topology  u32: offsets[N+1] | slot source[E] | slot target[E] |
//	               transpose offsets[N+1] | transpose target[E] | transpose slot[E]
//	nodeIndex u32: input column[N] | readout column[N] | input node[W] | readout node[R]
//	coef      f32: bias[N] | lambda[N] | alpha[N] | dlambda/dlog_tau[N] | gate gain[N] | gate bias[N]
//	inputs    f32: [b][t][w]
//	voltage, output f32: [t][b][i] for t in [0, T]
//	history   f32: drive[P] | gate rate[P] | gate squash[P] | gate slope[P]
//	upstream  f32: [b][s][r]
//	driveGrad f32: [t][b][i]
//	partial   f32: log_tau[P] | gate gain[P] | gate bias[P] contributions
//	carry     f32: voltage gradient[BN] | recurrent output gradient[BN]
//	gradient  f32: weights in slot order[E] | bias[N] | log_tau[N] | gate gain[N] | gate bias[N]
const continuousBatchPrelude = `
struct Dims {
    step: u32,
    batch: u32,
    nodes: u32,
    steps: u32,
    width: u32,
    readout: u32,
    read_steps: u32,
    edges: u32,
    activation: u32,
    gate: u32,
    every_step: u32,
    reserved0: u32,
    reserved1: u32,
    reserved2: u32,
    reserved3: u32,
    reserved4: u32,
};

const NONE: u32 = 0xffffffffu;

fn flat_index(gid: vec3<u32>, groups: vec3<u32>) -> u32 {
    return gid.x + gid.y * groups.x * 64u;
}

// tanh without the builtin, whose large-argument behaviour differs between
// drivers: a short odd series near zero, saturation far from it, and the
// exponential form in between.
fn stable_tanh(v: f32) -> f32 {
    let a = abs(v);
    if (a < 0.125) {
        let v2 = v * v;
        return v * (1.0 + v2 * (-0.33333334 + v2 * (0.13333334 + v2 * -0.053968254)));
    }
    if (a > 20.0) {
        return sign(v);
    }
    let e = exp(-2.0 * a);
    return sign(v) * (1.0 - e) / (1.0 + e);
}

fn activate(v: f32, kind: u32) -> f32 {
    if (kind == 0u) {
        return stable_tanh(v);
    }
    if (kind == 1u) {
        return v / (1.0 + abs(v));
    }
    return max(v, 0.0) + log(1.0 + exp(-abs(v)));
}

fn activation_slope(v: f32, kind: u32) -> f32 {
    if (kind == 0u) {
        let y = stable_tanh(v);
        return 1.0 - y * y;
    }
    if (kind == 1u) {
        let d = 1.0 + abs(v);
        return 1.0 / (d * d);
    }
    if (v >= 0.0) {
        return 1.0 / (1.0 + exp(-v));
    }
    let e = exp(v);
    return e / (1.0 + e);
}
`

// continuousBatchForwardWGSL advances one step for every (sequence, node).
// The synaptic sum runs serially over the target's CSR slots, which keep the
// declared edge order, then adds the bias, exactly as dynamics.Continuous.
const continuousBatchForwardWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> inputs: array<f32>;
@group(0) @binding(1) var<storage, read> node_index: array<u32>;
@group(0) @binding(2) var<storage, read> coef: array<f32>;
@group(0) @binding(3) var<storage, read> topology: array<u32>;
@group(0) @binding(4) var<storage, read> slot_weights: array<f32>;
@group(0) @binding(5) var<storage, read_write> voltage: array<f32>;
@group(0) @binding(6) var<storage, read_write> output: array<f32>;
@group(0) @binding(7) var<storage, read_write> history: array<f32>;
@group(0) @binding(8) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let index = flat_index(gid, groups);
    let n = dims.nodes;
    let bn = dims.batch * n;
    if (index >= bn) {
        return;
    }
    let b = index / n;
    let i = index - b * n;
    let t = dims.step;
    var acc = 0.0;
    let column = node_index[i];
    if (column != NONE) {
        acc = inputs[(b * dims.steps + t) * dims.width + column];
    }
    let row = t * bn + b * n;
    let begin = topology[i];
    let end = topology[i + 1u];
    for (var slot = begin; slot < end; slot = slot + 1u) {
        acc = acc + slot_weights[slot] * output[row + topology[n + 1u + slot]];
    }
    let drive = acc + coef[i];
    let v = voltage[t * bn + index];
    let cell = t * bn + index;
    let plane = dims.steps * bn;
    var next: f32;
    if (dims.gate == 0u) {
        next = coef[n + i] * v + coef[2u * n + i] * drive;
    } else {
        let z = coef[4u * n + i] * drive + coef[5u * n + i];
        var squash: f32;
        var slope: f32;
        if (dims.gate == 1u) {
            squash = 1.0 / (1.0 + exp(-z));
            slope = squash * (1.0 - squash);
        } else {
            let d = 1.0 + abs(z);
            squash = (1.0 + z / d) * 0.5;
            slope = 0.5 / (d * d);
        }
        var rate = 2.0 * coef[2u * n + i] * squash;
        if (rate > 1.0) {
            rate = 1.0;
        }
        history[plane + cell] = rate;
        history[2u * plane + cell] = squash;
        history[3u * plane + cell] = slope;
        next = (1.0 - rate) * v + rate * drive;
    }
    history[cell] = drive;
    voltage[(t + 1u) * bn + index] = next;
    output[(t + 1u) * bn + index] = activate(next, dims.activation);
}
`

// continuousBatchReadoutWGSL gathers the readout nodes of the requested
// steps: every step, or only the last one.
const continuousBatchReadoutWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> node_index: array<u32>;
@group(0) @binding(1) var<storage, read> output: array<f32>;
@group(0) @binding(2) var<storage, read_write> readout: array<f32>;
@group(0) @binding(3) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let index = flat_index(gid, groups);
    let per = dims.read_steps * dims.readout;
    if (index >= dims.batch * per) {
        return;
    }
    let b = index / per;
    let rest = index - b * per;
    let s = rest / dims.readout;
    let r = rest - s * dims.readout;
    var t = dims.steps - 1u;
    if (dims.every_step != 0u) {
        t = s;
    }
    let n = dims.nodes;
    let node = node_index[2u * n + dims.width + r];
    readout[index] = output[(t + 1u) * dims.batch * n + b * n + node];
}
`

// continuousBatchNodeBackwardWGSL is the per-neuron half of one reverse step:
// it joins the readout upstream with the recurrent output gradient, applies
// the activation slope and the integrator or gate chain rule, stores the
// drive gradient and the per-cell parameter contributions, and carries the
// voltage gradient to the previous step.
const continuousBatchNodeBackwardWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> node_index: array<u32>;
@group(0) @binding(1) var<storage, read> coef: array<f32>;
@group(0) @binding(2) var<storage, read> upstream: array<f32>;
@group(0) @binding(3) var<storage, read> voltage: array<f32>;
@group(0) @binding(4) var<storage, read> history: array<f32>;
@group(0) @binding(5) var<storage, read_write> drive_grad: array<f32>;
@group(0) @binding(6) var<storage, read_write> partial: array<f32>;
@group(0) @binding(7) var<storage, read_write> carry: array<f32>;
@group(0) @binding(8) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let index = flat_index(gid, groups);
    let n = dims.nodes;
    let bn = dims.batch * n;
    if (index >= bn) {
        return;
    }
    let b = index / n;
    let i = index - b * n;
    let t = dims.step;
    var gy = carry[bn + index];
    let column = node_index[n + i];
    if (column != NONE) {
        if (dims.every_step != 0u) {
            gy = gy + upstream[(b * dims.read_steps + t) * dims.readout + column];
        } else if (t + 1u == dims.steps) {
            gy = gy + upstream[b * dims.readout + column];
        }
    }
    let cell = t * bn + index;
    let plane = dims.steps * bn;
    let v = voltage[cell];
    let dv = carry[index] + gy * activation_slope(voltage[cell + bn], dims.activation);
    let drive = history[cell];
    let lambda = coef[n + i];
    let alpha = coef[2u * n + i];
    let dlambda = coef[3u * n + i];
    var grad = 0.0;
    var tau = 0.0;
    var gain = 0.0;
    var bias = 0.0;
    var previous = 0.0;
    if (dims.gate == 0u) {
        grad = alpha * dv;
        tau = dv * (v - drive) * dlambda;
        previous = lambda * dv;
    } else {
        let rate = history[plane + cell];
        let squash = history[2u * plane + cell];
        let slope = history[3u * plane + cell];
        grad = rate * dv;
        // A rate clamped at 1 passes no gradient to the gate.
        if (2.0 * alpha * squash <= 1.0) {
            let da = dv * (drive - v);
            let dz = da * 2.0 * alpha * slope;
            grad = grad + dz * coef[4u * n + i];
            gain = dz * drive;
            bias = dz;
            tau = -(da * 2.0 * squash * dlambda);
        }
        previous = (1.0 - rate) * dv;
    }
    drive_grad[cell] = grad;
    partial[cell] = tau;
    partial[plane + cell] = gain;
    partial[2u * plane + cell] = bias;
    carry[index] = previous;
}
`

// continuousBatchScatterWGSL sends one step's drive gradient back to the
// source outputs through the transpose CSR, so each (sequence, source) sums
// its outgoing edges serially and no float atomics are needed.
const continuousBatchScatterWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> topology: array<u32>;
@group(0) @binding(1) var<storage, read> slot_weights: array<f32>;
@group(0) @binding(2) var<storage, read> drive_grad: array<f32>;
@group(0) @binding(3) var<storage, read_write> carry: array<f32>;
@group(0) @binding(4) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let index = flat_index(gid, groups);
    let n = dims.nodes;
    let e = dims.edges;
    let bn = dims.batch * n;
    if (index >= bn) {
        return;
    }
    let b = index / n;
    let j = index - b * n;
    let row = dims.step * bn + b * n;
    let base = n + 1u + 2u * e;
    let begin = topology[base + j];
    let end = topology[base + j + 1u];
    var acc = 0.0;
    for (var k = begin; k < end; k = k + 1u) {
        let target_node = topology[base + n + 1u + k];
        let slot = topology[base + n + 1u + e + k];
        acc = acc + drive_grad[row + target_node] * slot_weights[slot];
    }
    carry[bn + index] = acc;
}
`

// continuousBatchWeightReduceWGSL sums each CSR slot's weight gradient over
// every step and sequence: per-step partial sums first, then across steps.
const continuousBatchWeightReduceWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> topology: array<u32>;
@group(0) @binding(1) var<storage, read> drive_grad: array<f32>;
@group(0) @binding(2) var<storage, read> output: array<f32>;
@group(0) @binding(3) var<storage, read_write> gradient: array<f32>;
@group(0) @binding(4) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let slot = flat_index(gid, groups);
    let n = dims.nodes;
    if (slot >= dims.edges) {
        return;
    }
    let source_node = topology[n + 1u + slot];
    let target_node = topology[n + 1u + dims.edges + slot];
    let bn = dims.batch * n;
    var total = 0.0;
    for (var t = 0u; t < dims.steps; t = t + 1u) {
        var part = 0.0;
        for (var b = 0u; b < dims.batch; b = b + 1u) {
            let row = t * bn + b * n;
            part = part + drive_grad[row + target_node] * output[row + source_node];
        }
        total = total + part;
    }
    gradient[slot] = total;
}
`

// continuousBatchNodeReduceWGSL sums the bias, log_tau and gate
// contributions of each neuron over every step and sequence.
const continuousBatchNodeReduceWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> drive_grad: array<f32>;
@group(0) @binding(1) var<storage, read> partial: array<f32>;
@group(0) @binding(2) var<storage, read_write> gradient: array<f32>;
@group(0) @binding(3) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let i = flat_index(gid, groups);
    let n = dims.nodes;
    if (i >= n) {
        return;
    }
    let bn = dims.batch * n;
    let plane = dims.steps * bn;
    var bias = 0.0;
    var tau = 0.0;
    var gain = 0.0;
    var offset = 0.0;
    for (var t = 0u; t < dims.steps; t = t + 1u) {
        var pb = 0.0;
        var pt = 0.0;
        var pg = 0.0;
        var po = 0.0;
        for (var b = 0u; b < dims.batch; b = b + 1u) {
            let cell = t * bn + b * n + i;
            pb = pb + drive_grad[cell];
            pt = pt + partial[cell];
            pg = pg + partial[plane + cell];
            po = po + partial[2u * plane + cell];
        }
        bias = bias + pb;
        tau = tau + pt;
        gain = gain + pg;
        offset = offset + po;
    }
    let e = dims.edges;
    gradient[e + i] = bias;
    gradient[e + n + i] = tau;
    gradient[e + 2u * n + i] = gain;
    gradient[e + 3u * n + i] = offset;
}
`

// continuousBatchInputGatherWGSL copies the drive gradient of the input nodes
// into the [b][t][w] layout of the inputs.
const continuousBatchInputGatherWGSL = continuousBatchPrelude + `
@group(0) @binding(0) var<storage, read> node_index: array<u32>;
@group(0) @binding(1) var<storage, read> drive_grad: array<f32>;
@group(0) @binding(2) var<storage, read_write> input_grad: array<f32>;
@group(0) @binding(3) var<uniform> dims: Dims;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) groups: vec3<u32>) {
    let index = flat_index(gid, groups);
    let per = dims.steps * dims.width;
    if (index >= dims.batch * per) {
        return;
    }
    let b = index / per;
    let rest = index - b * per;
    let t = rest / dims.width;
    let w = rest - t * dims.width;
    let n = dims.nodes;
    let node = node_index[2u * n + w];
    input_grad[index] = drive_grad[t * dims.batch * n + b * n + node];
}
`
