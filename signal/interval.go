package signal

import (
	"container/heap"
	"context"
	"fmt"
)

// IntervalResampleConfig samples constant-valued intervals at exact source
// positions step * SourceTicks / SimulationSteps over [StartStep, EndStep).
// Both timelines share zero; calibration is explicit. Clock assigns output
// timestamps without shifting the requested steps by its CurrentStep.
//
// Inputs are continuous, activity, or modulation signals with Duration > 0.
// Values apply on [Start, Start+Duration). Overlap uses the latest start, then
// greatest source sequence; an older interval resumes if it is still active
// when the newer one expires. Uncovered positions produce no signal.
// Metadata other than start, duration, values, and sequence must agree.
//
// Output is a point Signal using OutputStreamID, with SourceSequence set to the
// absolute neural step. Preserve this config and source data for provenance.
// Limits count buffered intervals plus incoming intervals, the complete output
// horizon, and logical value slots (buffer + one retained source + horizon).
// Limits are checked before payload validation/copying, and are not RSS limits.
type IntervalResampleConfig struct {
	SchemaVersion   Version
	Clock           Clock
	SourceUnit      TimeUnit
	SourceTicks     int64
	SimulationSteps int64
	StartStep       int64
	EndStep         int64
	OutputStreamID  string
	Limits          ResampleLimits
}

// IntervalResampler samples intervals causally with bounded retained state.
// Calls must be serialized. Its zero value is invalid.
type IntervalResampler struct {
	config      ResampleConfig
	pending     []Signal
	finalized   *Signal
	nextStep    int64
	watermark   int64
	finished    bool
	initialized bool
}

// NewIntervalResampler validates the clock, ratio, horizon and capacities.
func NewIntervalResampler(c IntervalResampleConfig) (*IntervalResampler, error) {
	// Share the existing timing/capacity validation. Interval processing below
	// does not use the point resampler's hold or interpolation behavior.
	validated, err := newResampler(ResampleConfig{
		SchemaVersion: c.SchemaVersion, Mode: CausalHold, Clock: c.Clock,
		SourceUnit: c.SourceUnit, SourceTicks: c.SourceTicks, SimulationSteps: c.SimulationSteps,
		StartStep: c.StartStep, EndStep: c.EndStep, OutputStreamID: c.OutputStreamID,
		Tail: HoldLast, Limits: c.Limits,
	})
	if err != nil {
		return nil, err
	}
	return &IntervalResampler{config: validated.config, nextStep: c.StartStep, initialized: true}, nil
}

// Push accepts intervals ordered by start and an exclusive source watermark.
// Starts must lie between the previous and new marks, inclusive; declared ends
// may be later. The caller must know each interval's value and duration at its
// start. Do not use durations learned retrospectively to claim online latency.
// Marks may repeat, and simultaneous starts at the mark remain open across
// chunks in any sequence order. After sorting, sequences strictly increase.
//
// Only source positions strictly before the mark are output. An empty chunk
// may advance the mark. Errors/cancellation produce no output or state change.
func (r *IntervalResampler) Push(ctx context.Context, input []Signal, watermark Timestamp) ([]Signal, error) {
	if r == nil || !r.initialized {
		return nil, fmt.Errorf("uninitialized interval resampler")
	}
	if err := watermark.Validate(); err != nil {
		return nil, err
	}
	if watermark.Unit != r.config.SourceUnit || watermark.Value < r.watermark {
		return nil, fmt.Errorf("watermark unit mismatch or time regression")
	}
	return r.process(ctx, input, watermark.Value, false)
}

// Finish closes all starts and samples remaining covered positions before
// EndStep. It never extends values past their declared end or fills gaps.
// Finish succeeds once; cancellation permits retry. Push after success fails.
func (r *IntervalResampler) Finish(ctx context.Context) ([]Signal, error) {
	if r == nil || !r.initialized {
		return nil, fmt.Errorf("uninitialized interval resampler")
	}
	return r.process(ctx, nil, r.watermark, true)
}

func (r *IntervalResampler) process(ctx context.Context, input []Signal, watermark int64, final bool) ([]Signal, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.finished {
		return nil, fmt.Errorf("interval resampler already finished")
	}
	c := r.config
	if len(input) > c.Limits.MaxBufferedSignals-len(r.pending) {
		return nil, fmt.Errorf("buffered interval capacity exceeded")
	}
	template := r.finalized
	if template == nil && len(r.pending) > 0 {
		template = &r.pending[0]
	}
	for i, s := range input {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.start.Value < r.watermark || s.start.Value > watermark {
			return nil, fmt.Errorf("interval %d outside watermark range", i)
		}
		if len(s.values) > c.Limits.MaxValues {
			return nil, fmt.Errorf("interval value capacity exceeded")
		}
		if r.finalized != nil && s.sourceSequence <= r.finalized.sourceSequence {
			return nil, fmt.Errorf("source sequence does not increase after finalized intervals")
		}
		if template == nil {
			v := s
			template = &v
		}
		if !sameSampleMetadata(*template, s) {
			return nil, fmt.Errorf("interval %d changes stream metadata", i)
		}
	}
	if template != nil {
		slots := uint64(len(input)) + uint64(len(r.pending)) + 1 + uint64(c.EndStep-c.StartStep)
		if slots > uint64(c.Limits.MaxValues) || uint64(len(template.values)) > uint64(c.Limits.MaxValues)/slots {
			return nil, fmt.Errorf("interval value capacity exceeded")
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
	finalized := r.finalized
	for _, s := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.start.Unit != c.SourceUnit || s.duration <= 0 || (s.kind != KindContinuous && s.kind != KindActivity && s.kind != KindModulation) {
			return nil, fmt.Errorf("interval resampling requires positive-duration continuous, activity, or modulation signals in the declared unit")
		}
		if (final || s.start.Value < watermark) && (finalized == nil || s.sourceSequence > finalized.sourceSequence) {
			v := s
			finalized = &v
		}
	}
	out := make([]Signal, 0)
	next := r.nextStep
	active := intervalPriority{}
	cursor := 0
	for next < c.EndStep {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		whole, _, err := sourcePosition(c, next)
		if err != nil {
			return nil, err
		}
		if !final && whole >= watermark {
			break
		}
		for cursor < len(ordered) && ordered[cursor].start.Value <= whole {
			heap.Push(&active, cursor)
			cursor++
		}
		// Starts and ends are integer ticks, so whole alone decides inclusion even
		// at fractional source positions. Never round a fractional time upward.
		for len(active) > 0 {
			s := ordered[active[0]]
			if s.start.Value+s.duration > whole {
				break
			}
			heap.Pop(&active)
		}
		if len(active) > 0 {
			spec := ordered[active[0]].Spec()
			spec.Duration = 0
			spec.StreamID = c.OutputStreamID
			spec.SourceSequence = uint64(next)
			spec.Start, err = c.Clock.TimestampAt(next)
			if err != nil {
				return nil, err
			}
			s, err := NewSignal(spec)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		next++
	}
	pending := make([]Signal, 0)
	if !final {
		var nextSource int64
		if next < c.EndStep {
			nextSource, _, err = sourcePosition(c, next)
			if err != nil {
				return nil, err
			}
		}
		for _, s := range ordered {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// Keep open starts for cross-chunk duplicate detection even if the horizon
			// is exhausted, plus intervals that can cover an unprocessed position.
			if s.start.Value == watermark || (next < c.EndStep && s.start.Value+s.duration > nextSource) {
				pending = append(pending, s)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.pending = pending
	r.finalized = finalized
	r.nextStep = next
	r.watermark = watermark
	r.finished = final
	return out, nil
}

// Ordered interval indices are also their overwrite priority. A max-heap
// restores older active intervals after newer ones expire, without scanning
// every interval at every output step.
type intervalPriority []int

func (h intervalPriority) Len() int           { return len(h) }
func (h intervalPriority) Less(i, j int) bool { return h[i] > h[j] }
func (h intervalPriority) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intervalPriority) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intervalPriority) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
