// Package webgpu provides explicit float32 WebGPU primitives for sparse
// synaptic drive and its local sparse derivative.
//
// SparseDrive is constructed from a fixed node count and parallel source and
// target edge lists. It builds a CSR topology once, preserving declaration
// order within each target, and uploads that topology once. Execute uploads
// the per-call base input, source output, and edge weights, dispatches the WGSL
// kernel, and reads the result back to the host.
//
// The operation is:
//
//	result[target] = input[target] + sum(weight[e] * output[source[e]])
//
// SparseBackward computes local float32 derivatives for that weighted sum.
// It returns the identity-path input gradient, source-output gradients, and
// one weight gradient per declared edge. The caller applies any activation
// derivative before calling Backward; CSR edge results are restored to
// declaration order on the host.
//
// DeviceAdamW keeps the AdamW p/m/v/counter state: parameters, first and
// second moments, and per-parameter update counters remain resident on the
// GPU. A successful Step swaps the device banks only after every chunk
// succeeds, so the update commits atomically across the full array. Snapshot
// explicitly reads the resident state back into owned host slices.
//
// StepChecked computes a candidate on the device, reads the complete float32
// candidate into an owned host mirror, and gives it to the acceptance callback
// before swapping the resident bank. It permits a zero learning rate while
// still advancing the checked optimizer state. learning.NewGPUDeviceTrainer and
// learning.RestoreGPUDeviceTrainer use this checked path; their neural state,
// activations, and sparse weight transfers remain host-side.
//
// ContinuousBatch is the batched continuous core: a scalar, undelayed leaky
// integrator, optionally gated (GateSigmoid, GateSoftsign), with tanh,
// softsign or softplus activation, starting every sequence at zero voltage.
// Forward runs all steps of a B-sequence minibatch in one submission and keeps
// the voltage, output, drive and gate history resident; Backward runs full
// BPTT over it, writes per-(step, sequence, node) intermediates, reduces every
// parameter gradient in separate kernels (no float atomics) and scatters to
// sources through a transpose CSR. Only the readout nodes and the final
// gradients are read back. dynamics.NewGPUContinuousBatch and
// learning.NewGPUBatchTrainer build on it. It is implemented and checked
// against the float64 CPU core on gogpu's software adapter in tests only; it
// is not yet verified on hardware.
//
// The other types are primitives, not a complete continuous network Forward or
// Backward pass, neural-state system, or training integration. The package
// never silently falls back to the CPU.
// A missing or software-only adapter is reported as ErrUnavailable; execution
// errors are returned to the caller and are never silently rerun on the CPU.
// Every executor opens its adapter and device from one process-wide instance
// and releases only its own device and adapter on Close. Once this process
// has opened a hardware device (HardwareAdapterSeen), a later missing adapter
// is reported as ErrDevice instead, so hardware tests fail rather than skip
// when an adapter that worked is lost.
// Metrics identify the selected adapter and backend and expose measured
// initialization, shader compilation, topology transfer, warmup, per-call
// transfer, dispatch, and readback durations. AdamW MaxBytes is a logical
// budget, not allocator VRAM or process RSS, and these metrics do not claim a
// speedup.
package webgpu
