package webgpu

import (
	"context"
	"fmt"
)

// StepChecked computes a hardware candidate, passes an owned snapshot to accept,
// and commits all banks only if accept and the final context check succeed.
// Unlike Step, it permits a zero rate while updating unfrozen moments/counters.
// accept must not call this optimizer's locking methods recursively.
func (o *DeviceAdamW) StepChecked(ctx context.Context, gradient []float32, mask []bool, rate float32, accept func(AdamWSnapshot) error) (AdamWMetrics, error) {
	if o == nil {
		return AdamWMetrics{}, fmt.Errorf("%w: nil receiver", ErrClosed)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return AdamWMetrics{}, ErrClosed
	}
	if o.unusable != nil {
		return o.metrics, o.unusable
	}
	if err := validateAdamContext(ctx); err != nil {
		return o.metrics, err
	}
	if accept == nil {
		return o.metrics, fmt.Errorf("%w: candidate acceptance callback is nil", ErrInvalidConfig)
	}
	if len(gradient) != o.values || len(mask) != o.values {
		return o.metrics, fmt.Errorf("%w: gradient length %d and mask length %d, want %d", ErrShape, len(gradient), len(mask), o.values)
	}
	if err := validateAdamValues(gradient, "gradient"); err != nil {
		return o.metrics, err
	}
	if !isFinite32(rate) {
		return o.metrics, fmt.Errorf("%w: learning rate is %v", ErrNonFinite, rate)
	}
	if rate < 0 {
		return o.metrics, fmt.Errorf("%w: learning rate must be non-negative, got %v", ErrInvalidConfig, rate)
	}

	metrics := o.metrics
	defer func() { o.metrics = metrics }()
	shadowBank, metrics, err := o.executeAdamCandidateLocked(ctx, gradient, mask, rate)
	if err != nil {
		return metrics, err
	}
	candidate, err := o.snapshotAdamBankLocked(ctx, shadowBank, &metrics)
	if err != nil {
		return metrics, err
	}
	if err := accept(candidate); err != nil {
		return metrics, fmt.Errorf("checked AdamW candidate callback rejected: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return metrics, err
	}
	// The candidate bank has been fully read back and accepted. No operation
	// below can fail, so this is the single commit point for the device state.
	o.currentBank = shadowBank
	return metrics, nil
}
