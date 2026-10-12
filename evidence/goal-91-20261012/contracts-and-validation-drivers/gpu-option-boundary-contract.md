# Device trainer option validation correction

Root confirmed the constructor's quantizeGPUOptions currently accepts negative
Beta1, Beta2 and WeightDecay smaller than float32 representable magnitude because
conversion produces negative zero. CPU NewTrainer rejects those original values.

The bounded correction is to validate original Options before float32 conversion,
then keep all existing kernel representability checks and valid quantization.
Public NewGPUDeviceTrainer must reject all three raw invalid values with
webgpu.ErrInvalidConfig, naming the field, before any GPU initialization. No
trainer is returned. Legal zero Beta1/Beta2/WeightDecay remains supported. Other
public profiles, precision rules and saved snapshots remain unchanged.

Test-only slot: learning/gpu_device_option_test.go. Include independent CPU
NewTrainer rejection as control, raw negative math.SmallestNonzeroFloat64 for
each field, typed device error and field text. Hardware absence is a failure
of this preflight check, never a skip. Root reviews and runs RED before the
implementation worker adds original-option validation in gpu_device_trainer.go.

Checks: relevant NewGPUDeviceTrainer tests, original quantization/restore tests,
CPU/legacy/device comparisons, true Metal bundle continuation, full validation.

Root refinement after reading validateOptions: its generic range error cannot
name the rejected field. All five kernel options require nonnegative original
values; the existing shared quantizeGPUKernelOption already rejects non-finite
and post-conversion ranges. Root adds a field-named original value<0 check before
conversion in that helper. It covers the three confirmed negative-zero cases
and the same pattern for epsilon/rate without duplicating Options range rules.
All non-kernel raw options remain unmodified and validated by NewTrainer.
The correction is three lines; Root implements this bounded slot directly after
reviewing a separate worker's test and recording actual Metal RED.
