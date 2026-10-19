package webgpu

import "context"

// NewContinuousBatchSoftwareForTest opens the batched executor on any adapter,
// including gogpu's CPU software adapter, which interprets the same SPIR-V the
// hardware path runs. It exists only in test builds; production constructors
// refuse a software adapter with ErrUnavailable.
func NewContinuousBatchSoftwareForTest(ctx context.Context, cfg ContinuousBatchConfig) (*ContinuousBatch, error) {
	return newContinuousBatch(ctx, cfg, true)
}
