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
// These are primitives, not a complete continuous network Forward or
// Backward pass. This package does not implement optimizer parameter updates,
// training orchestration, model-state checkpoint restore, or CPU fallback.
// A missing or software-only adapter is reported as ErrUnavailable; execution
// errors are returned to the caller and are never silently rerun on the CPU.
// Metrics identify the selected adapter and backend and expose measured
// initialization, shader compilation, topology transfer, warmup, per-call
// transfer, dispatch, and readback durations.
package webgpu
