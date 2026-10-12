struct Settings {
    beta1: f32,
    beta2: f32,
    epsilon: f32,
    weight_decay: f32,
    learning_rate: f32,
    value_count: u32,
    reserved0: u32,
    reserved1: u32,
};

struct Status {
    value: atomic<u32>,
};

@group(0) @binding(0) var<storage, read> current_state: array<vec4<u32>>;
@group(0) @binding(1) var<storage, read> input: array<vec2<u32>>;
@group(0) @binding(2) var<storage, read_write> shadow_state: array<vec4<u32>>;
@group(0) @binding(3) var<storage, read_write> status: Status;
@group(0) @binding(4) var<uniform> settings: Settings;

const STATUS_NON_FINITE: u32 = 1u;
const STATUS_COUNTER_OVERFLOW: u32 = 2u;
const STATUS_INVALID_STATE: u32 = 4u;

fn finite(value: f32) -> bool {
    let exponent = bitcast<u32>(value) & 0x7f800000u;
    return exponent != 0x7f800000u;
}

fn fail(flag: u32) {
    atomicOr(&status.value, flag);
}

fn beta_power(beta: f32, step: u32) -> f32 {
    if (beta == 0.0) {
        return 0.0;
    }
    return pow(beta, f32(step));
}

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let index = gid.x;
    if (index >= settings.value_count) {
        return;
    }

    let previous = current_state[index];
    let enabled = input[index].y != 0u;
    if (!enabled) {
        shadow_state[index] = previous;
        return;
    }

    let parameter = bitcast<f32>(previous.x);
    let first_previous = bitcast<f32>(previous.y);
    let second_previous = bitcast<f32>(previous.z);
    if (!finite(parameter) || !finite(first_previous) || !finite(second_previous) || second_previous < 0.0) {
        shadow_state[index] = previous;
        fail(STATUS_INVALID_STATE | STATUS_NON_FINITE);
        return;
    }

    let gradient = bitcast<f32>(input[index].x);
    if (!finite(gradient)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    if (previous.w == 0xffffffffu) {
        shadow_state[index] = previous;
        fail(STATUS_COUNTER_OVERFLOW);
        return;
    }

    let beta1 = settings.beta1;
    let beta2 = settings.beta2;
    let epsilon = settings.epsilon;
    let weight_decay = settings.weight_decay;
    let learning_rate = settings.learning_rate;
    if (!finite(beta1) || !finite(beta2) || !finite(epsilon) || !finite(weight_decay) || !finite(learning_rate)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }

    let one = 1.0;
    let one_minus_beta1 = one - beta1;
    let one_minus_beta2 = one - beta2;
    if (!finite(one_minus_beta1) || !finite(one_minus_beta2)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }

    let gradient_square = gradient * gradient;
    if (!finite(gradient_square)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let first_decay = beta1 * first_previous;
    let first_gradient = one_minus_beta1 * gradient;
    if (!finite(first_decay) || !finite(first_gradient)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let first = first_decay + first_gradient;
    if (!finite(first)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let second_decay = beta2 * second_previous;
    let second_gradient = one_minus_beta2 * gradient_square;
    if (!finite(second_decay) || !finite(second_gradient)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let second = second_decay + second_gradient;
    if (!finite(second) || second < 0.0) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }

    let next_step = previous.w + 1u;
    let beta1_power = beta_power(beta1, next_step);
    let beta2_power = beta_power(beta2, next_step);
    if (!finite(beta1_power) || !finite(beta2_power)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let one_minus_beta1_power = one - beta1_power;
    let one_minus_beta2_power = one - beta2_power;
    if (!finite(one_minus_beta1_power) || !finite(one_minus_beta2_power) || one_minus_beta1_power <= 0.0 || one_minus_beta2_power <= 0.0) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }

    let first_hat = first / one_minus_beta1_power;
    let second_hat = second / one_minus_beta2_power;
    if (!finite(first_hat) || !finite(second_hat) || second_hat < 0.0) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let root_second = sqrt(second_hat);
    let denominator = root_second + epsilon;
    if (!finite(root_second) || !finite(denominator) || denominator <= 0.0) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let first_ratio = first_hat / denominator;
    if (!finite(first_ratio)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let adam_update = learning_rate * first_ratio;
    if (!finite(adam_update)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let decay_scale = learning_rate * weight_decay;
    if (!finite(decay_scale)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let decay_update = decay_scale * parameter;
    if (!finite(decay_update)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }
    let candidate = parameter - adam_update - decay_update;
    if (!finite(candidate)) {
        shadow_state[index] = previous;
        fail(STATUS_NON_FINITE);
        return;
    }

    shadow_state[index] = vec4<u32>(bitcast<u32>(candidate), bitcast<u32>(first), bitcast<u32>(second), next_step);
}
