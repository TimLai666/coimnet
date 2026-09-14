// Package multichannel is a synthetic custom adapter example. It demonstrates
// extending CoImNet through public signal and learning APIs; it is not a model
// recipe or a biologically calibrated sensory mapping.
package multichannel

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/signal"
)

// Inputs keeps independent source sequence namespaces. All observations belong
// to one experience; StreamID and Channel must be level, gate, or pulses as named.
// Construct an explicit empty Observation for a channel with no samples.
// Targets are deliberately absent from this inference-only input contract.
type Inputs struct {
	Level  signal.Observation
	Gate   signal.Observation
	Pulses signal.Observation
}

// Adapt implements the fixed synthetic adapter contract multichannel/v1:
//   - 8 model steps, common zero origin, 6 features per row:
//     [level, level_present, gate, gate_present, pulse_sum, pulse_present].
//   - scalar dimensionless source values, normalized_time timestamps, encoder 1.0;
//     respectively 2, 3, and 4 source ticks per model step.
//   - level: continuous points, causal hold through step 7; leading gaps absent.
//   - gate: activity intervals, latest active interval wins; gaps stay absent.
//     Duration and value must already be known at the interval's start.
//   - pulses: ceil alignment, sum amplitudes in source time/sequence order;
//     presence remains 1 even when pulses cancel. Amplitudes are not weights.
//
// Source starts must be <= the source position of step 7; intervals may extend
// beyond it and are clipped to the requested horizon. Empty channels produce
// zero value/presence pairs. At most 32 scalar samples per channel are accepted.
// chunkSize (1..32) varies delivery batching, not time or feature order.
// The function completes each finite source before returning an owned matrix;
// it does not measure real-time latency or merge source sequence numbers.
// Final features must fit the learning encoder's finite float32 range.
// Any validation, capacity, cancellation or numerical error returns nil output.
func Adapt(ctx context.Context, input Inputs, chunkSize int) ([][]float64, error) {
	if ctx == nil {
		return nil, fmt.Errorf("multichannel: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if chunkSize < 1 || chunkSize > 32 {
		return nil, fmt.Errorf("multichannel: chunkSize must be 1..32")
	}
	version := signal.CurrentSchemaVersion()
	clock, err := signal.NewClock(version, signal.TimeUnitModelStep, 1)
	if err != nil {
		return nil, err
	}
	limits := signal.ResampleLimits{MaxBufferedSignals: 32, MaxOutputSignals: 8, MaxValues: 80}
	channels := []struct {
		name        string
		kind        signal.SignalKind
		ticks       int64
		observation signal.Observation
	}{
		{"level", signal.KindContinuous, 2, input.Level},
		{"gate", signal.KindActivity, 3, input.Gate},
		{"pulses", signal.KindPulse, 4, input.Pulses},
	}
	rows := make([][]float64, 8)
	for i := range rows {
		rows[i] = make([]float64, 6)
	}
	for col, ch := range channels {
		samples := ch.observation.Signals()
		if len(samples) > 32 {
			return nil, fmt.Errorf("%s: maximum 32 source samples", ch.name)
		}
		if err := ch.observation.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", ch.name, err)
		}
		if ch.observation.ExperienceID() != input.Level.ExperienceID() || ch.observation.StreamID() != ch.name {
			return nil, fmt.Errorf("%s: experience or stream identity mismatch", ch.name)
		}
		for _, s := range samples {
			shape := s.Shape()
			if s.Channel() != ch.name || s.Kind() != ch.kind || len(shape) != 1 || shape[0] != 1 || s.Unit() != signal.UnitDimensionless || s.StartTime().Unit != signal.TimeUnitNormalizedTime || s.EncoderVersion() != (signal.Version{Major: 1}) {
				return nil, fmt.Errorf("%s: incompatible scalar source metadata", ch.name)
			}
			if (col == 1 && s.Duration() <= 0) || (col != 1 && s.Duration() != 0) || s.StartTime().Value > 7*ch.ticks {
				return nil, fmt.Errorf("%s: invalid duration or source start outside horizon", ch.name)
			}
		}
		// Adapt the existing method signatures locally, keeping core APIs unchanged.
		var push func(context.Context, []signal.Signal, signal.Timestamp) ([]signal.Signal, error)
		var finish func(context.Context) ([]signal.Signal, error)
		switch col {
		case 0:
			r, err := signal.NewStreamingResampler(signal.ResampleConfig{SchemaVersion: version, Mode: signal.CausalHold, Clock: clock, SourceUnit: signal.TimeUnitNormalizedTime, SourceTicks: ch.ticks, SimulationSteps: 1, EndStep: 8, OutputStreamID: ch.name + "/aligned", Tail: signal.HoldLast, Limits: limits})
			if err != nil {
				return nil, err
			}
			push = func(ctx context.Context, s []signal.Signal, w signal.Timestamp) ([]signal.Signal, error) {
				b, e := r.Push(ctx, s, w)
				return b.Signals, e
			}
			finish = func(ctx context.Context) ([]signal.Signal, error) { b, e := r.Finish(ctx); return b.Signals, e }
		case 1:
			r, err := signal.NewIntervalResampler(signal.IntervalResampleConfig{SchemaVersion: version, Clock: clock, SourceUnit: signal.TimeUnitNormalizedTime, SourceTicks: ch.ticks, SimulationSteps: 1, EndStep: 8, OutputStreamID: ch.name + "/aligned", Limits: limits})
			if err != nil {
				return nil, err
			}
			push = r.Push
			finish = r.Finish
		case 2:
			r, err := signal.NewPulseAligner(signal.PulseAlignConfig{SchemaVersion: version, Clock: clock, SourceUnit: signal.TimeUnitNormalizedTime, SourceTicks: ch.ticks, SimulationSteps: 1, OutputStreamID: ch.name + "/aligned", MaxBufferedSignals: 32, MaxValues: 80})
			if err != nil {
				return nil, err
			}
			push = r.Push
			finish = r.Finish
		}
		consume := func(aligned []signal.Signal) error {
			for _, s := range aligned {
				if err := ctx.Err(); err != nil {
					return err
				}
				step := s.StartTime().Value // the declared output Clock has step size 1
				if s.StartTime().Unit != signal.TimeUnitModelStep || step < 0 || step >= 8 {
					return fmt.Errorf("%s: aligned step outside horizon", ch.name)
				}
				value := s.Values()[0]
				if col == 2 {
					value += rows[step][2*col]
				}
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return fmt.Errorf("%s: non-finite aggregate", ch.name)
				}
				rows[step][2*col] = value
				rows[step][2*col+1] = 1
			}
			return nil
		}
		for start := 0; start < len(samples); start += chunkSize {
			end := min(start+chunkSize, len(samples))
			aligned, err := push(ctx, samples[start:end], samples[end-1].StartTime())
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ch.name, err)
			}
			if err := consume(aligned); err != nil {
				return nil, err
			}
		}
		aligned, err := finish(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ch.name, err)
		}
		if err := consume(aligned); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Check final aggregates, allowing individually large pulses to cancel.
	// Match learning's input conversion contract before handing off the matrix.
	for step, row := range rows {
		for col, value := range row {
			if math.Abs(value) > math.MaxFloat32 {
				return nil, fmt.Errorf("multichannel: step %d feature %d exceeds learning float32 range", step, col)
			}
		}
	}
	return rows, nil
}
