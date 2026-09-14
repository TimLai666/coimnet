package signal

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"strings"
)

// PulseAlignConfig maps a source event to the first neural step at or after it:
// ceil(sourceTime * SimulationSteps / SourceTicks). Both timelines share zero.
// Clock assigns the output timestamp; CurrentStep does not shift this mapping.
// Calibration is explicit and is not inferred from the time unit.
//
// Inputs are point pulses from one fixed stream. Metadata other than start,
// values, and source sequence must agree. Each event is preserved individually,
// including its values and source sequence, even when several events map to
// the same step. OutputStreamID replaces the input stream ID.
//
// MaxBufferedSignals bounds pending plus incoming events. MaxValues bounds
// logical payload slots: (2 * (pending + incoming) + 1) * valuesPerSignal,
// allowing candidate inputs, outputs, and one retained source event. This is
// checked before payload validation/copying, and is not an RSS limit.
type PulseAlignConfig struct {
	SchemaVersion      Version
	Clock              Clock
	SourceUnit         TimeUnit
	SourceTicks        int64
	SimulationSteps    int64
	OutputStreamID     string
	MaxBufferedSignals int
	MaxValues          int
}

// PulseAligner aligns pulse timing without summing, interpolating, or holding
// amplitudes. Calls must be serialized. Its zero value is invalid.
type PulseAligner struct {
	config      PulseAlignConfig
	pending     []Signal
	last        *Signal
	watermark   int64
	finished    bool
	initialized bool
}

// NewPulseAligner validates an explicit time mapping and positive capacities.
func NewPulseAligner(c PulseAlignConfig) (*PulseAligner, error) {
	if err := validateSchemaVersion(c.SchemaVersion); err != nil {
		return nil, err
	}
	if err := c.Clock.Validate(); err != nil {
		return nil, err
	}
	if err := c.SourceUnit.Validate(); err != nil {
		return nil, err
	}
	if c.SourceTicks <= 0 || c.SimulationSteps <= 0 {
		return nil, fmt.Errorf("pulse time ratio must be positive")
	}
	if strings.TrimSpace(c.OutputStreamID) == "" {
		return nil, fmt.Errorf("output stream ID is required")
	}
	if c.MaxBufferedSignals <= 0 || c.MaxValues <= 0 {
		return nil, fmt.Errorf("pulse capacities must be positive")
	}
	return &PulseAligner{config: c, initialized: true}, nil
}

// Push accepts nondecreasing source timestamps and an exclusive source
// watermark. Incoming timestamps must lie between the previous and new
// watermarks, inclusive. Watermarks may repeat. Events exactly at the mark
// remain open and may arrive in any source-sequence order across chunks.
// After sorting simultaneous events, source sequences must strictly increase.
//
// A destination step is emitted only once its source position is strictly
// before the watermark, keeping all events for that step together. Output is
// ordered by source time and sequence. Empty input can advance the watermark.
// Errors or cancellation return no output and leave all state unchanged.
func (r *PulseAligner) Push(ctx context.Context, input []Signal, watermark Timestamp) ([]Signal, error) {
	if r == nil || !r.initialized {
		return nil, fmt.Errorf("uninitialized pulse aligner")
	}
	if err := watermark.Validate(); err != nil {
		return nil, err
	}
	if watermark.Unit != r.config.SourceUnit || watermark.Value < r.watermark {
		return nil, fmt.Errorf("watermark unit mismatch or time regression")
	}
	return r.process(ctx, input, watermark.Value, false)
}

// Finish flushes every pending event once, including events mapped beyond the
// last watermark. It generates no empty steps or held tail. Repeated Finish or
// Push after success fails; cancellation leaves the aligner open for retry.
func (r *PulseAligner) Finish(ctx context.Context) ([]Signal, error) {
	if r == nil || !r.initialized {
		return nil, fmt.Errorf("uninitialized pulse aligner")
	}
	return r.process(ctx, nil, r.watermark, true)
}

func (r *PulseAligner) process(ctx context.Context, input []Signal, watermark int64, final bool) ([]Signal, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.finished {
		return nil, fmt.Errorf("pulse aligner already finished")
	}
	c := r.config
	if len(input) > c.MaxBufferedSignals-len(r.pending) {
		return nil, fmt.Errorf("buffered pulse capacity exceeded")
	}
	template := r.last
	if template == nil && len(r.pending) > 0 {
		template = &r.pending[0]
	}
	for i, s := range input {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.start.Value < r.watermark || s.start.Value > watermark {
			return nil, fmt.Errorf("pulse %d outside watermark interval", i)
		}
		if len(s.values) > c.MaxValues {
			return nil, fmt.Errorf("pulse value capacity exceeded")
		}
		if template == nil {
			v := s
			template = &v
		}
		if !sameSampleMetadata(*template, s) {
			return nil, fmt.Errorf("pulse %d changes stream metadata", i)
		}
	}
	if template != nil {
		// len(pending)+len(input) <= MaxInt by the event capacity check above.
		slots := 2*(uint64(len(r.pending))+uint64(len(input))) + 1
		if slots > uint64(c.MaxValues) || uint64(len(template.values)) > uint64(c.MaxValues)/slots {
			return nil, fmt.Errorf("pulse value capacity exceeded")
		}
	}
	work := make([]Signal, 0, len(r.pending)+len(input))
	work = append(work, r.pending...)
	work = append(work, input...)
	ordered, err := OrderSignals(work)
	if err != nil {
		return nil, err
	}
	if err := ValidateChronological(ordered); err != nil {
		return nil, err
	}
	out := make([]Signal, 0)
	pending := make([]Signal, 0)
	last := r.last
	for _, s := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.start.Unit != c.SourceUnit || s.kind != KindPulse || s.duration != 0 {
			return nil, fmt.Errorf("pulse alignment requires point pulses in the declared source unit")
		}
		if r.last != nil && s.sourceSequence <= r.last.sourceSequence {
			return nil, fmt.Errorf("source sequence does not increase after emitted pulses")
		}
		step, err := pulseStep(c, s.start.Value)
		if err != nil {
			return nil, err
		}
		timestamp, err := c.Clock.TimestampAt(step)
		if err != nil {
			return nil, err
		}
		// Compare exact 128-bit source positions; units cannot be compared directly.
		hi, lo := bits.Mul64(uint64(step), uint64(c.SourceTicks))
		whi, wlo := bits.Mul64(uint64(watermark), uint64(c.SimulationSteps))
		closed := hi < whi || (hi == whi && lo < wlo)
		if !final && !closed {
			pending = append(pending, s)
			continue
		}
		spec := s.Spec()
		spec.StreamID = c.OutputStreamID
		spec.Start = timestamp
		aligned, err := NewSignal(spec)
		if err != nil {
			return nil, err
		}
		out = append(out, aligned)
		v := s
		last = &v
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.pending = pending
	r.last = last
	r.watermark = watermark
	r.finished = final
	return out, nil
}

func pulseStep(c PulseAlignConfig, sourceTime int64) (int64, error) {
	hi, lo := bits.Mul64(uint64(sourceTime), uint64(c.SimulationSteps))
	den := uint64(c.SourceTicks)
	if hi >= den {
		return 0, fmt.Errorf("pulse step overflow")
	}
	q, rem := bits.Div64(hi, lo, den)
	if q > math.MaxInt64 || (q == math.MaxInt64 && rem != 0) {
		return 0, fmt.Errorf("pulse step overflow")
	}
	if rem != 0 {
		q++
	}
	return int64(q), nil
}
