package signal

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"reflect"
	"strings"
)

// ResamplingMode declares whether an output may depend on later source samples.
type ResamplingMode string

const (
	// CausalHold uses the latest sample at or before each requested source position.
	CausalHold ResamplingMode = "causal_hold"
	// OfflineLinear interpolates between surrounding samples, including a future one.
	OfflineLinear ResamplingMode = "offline_linear"
)

// TailPolicy defines how a finite stream covers the remaining requested steps.
type TailPolicy string

// HoldLast repeats the last sample to EndStep. Leading gaps and empty streams
// remain absent; they are never filled with zeros.
const HoldLast TailPolicy = "hold_last"

// ResampleLimits bounds buffered samples, the requested output horizon, and
// logical value slots (buffer + one retained sample + entire output horizon).
// These limits are checked before copying payloads; they are not RSS limits.
type ResampleLimits struct {
	MaxBufferedSignals int
	MaxOutputSignals   int
	MaxValues          int
}

// ResampleConfig maps absolute neural steps to source ticks using the exact ratio
// step * SourceTicks / SimulationSteps. Both timelines have a shared zero origin.
// Clock separately assigns output timestamps; its CurrentStep does not select
// the horizon. The caller must explicitly supply any physical calibration.
//
// Inputs are point samples (Duration == 0) from one fixed continuous, activity,
// or modulation stream. All metadata except start, sequence, and values must
// agree. Simultaneous samples are sorted by SourceSequence and the last wins.
// Pulse, intervention, interval signals, filtering, and audio decoding require
// their own semantics and are rejected here.
//
// Output samples cover [StartStep, EndStep), retain channel/value metadata, use
// OutputStreamID, and set SourceSequence to the absolute neural step. Keep this
// configuration with the output to preserve the source-to-output time mapping.
type ResampleConfig struct {
	SchemaVersion   Version
	Mode            ResamplingMode
	Clock           Clock
	SourceUnit      TimeUnit
	SourceTicks     int64
	SimulationSteps int64
	StartStep       int64
	EndStep         int64
	OutputStreamID  string
	Tail            TailPolicy
	Limits          ResampleLimits
}

// ResampledBatch labels preprocessing mode explicitly. Signals are ordinary
// immutable point samples, suitable for constructing an Observation.
type ResampledBatch struct {
	Mode    ResamplingMode `json:"mode"`
	Signals []Signal       `json:"signals"`
}

// StreamingResampler aligns one sampled stream with bounded retained state.
// Use separate instances for different channels, with the same output Clock.
// Calls must be serialized. Its zero value is invalid.
type StreamingResampler struct {
	config      ResampleConfig
	pending     []Signal
	held        *Signal
	nextStep    int64
	watermark   int64
	finished    bool
	initialized bool
}

// NewStreamingResampler accepts only CausalHold. Offline preprocessing must use
// ResampleOffline and cannot be installed as this realtime stream processor.
func NewStreamingResampler(config ResampleConfig) (*StreamingResampler, error) {
	if config.Mode != CausalHold {
		return nil, fmt.Errorf("streaming resampling requires causal_hold")
	}
	return newResampler(config)
}

func newResampler(c ResampleConfig) (*StreamingResampler, error) {
	if err := validateSchemaVersion(c.SchemaVersion); err != nil {
		return nil, err
	}
	if c.Mode != CausalHold && c.Mode != OfflineLinear {
		return nil, fmt.Errorf("unsupported resampling mode %q", c.Mode)
	}
	if err := c.Clock.Validate(); err != nil {
		return nil, err
	}
	if err := c.SourceUnit.Validate(); err != nil {
		return nil, err
	}
	if c.SourceTicks <= 0 || c.SimulationSteps <= 0 {
		return nil, fmt.Errorf("source time ratio must be positive")
	}
	if c.StartStep < 0 || c.EndStep < c.StartStep {
		return nil, fmt.Errorf("invalid resampling step range")
	}
	if strings.TrimSpace(c.OutputStreamID) == "" {
		return nil, fmt.Errorf("output stream ID is required")
	}
	if c.Tail != HoldLast {
		return nil, fmt.Errorf("unsupported tail policy %q", c.Tail)
	}
	l := c.Limits
	if l.MaxBufferedSignals <= 0 || l.MaxOutputSignals <= 0 || l.MaxValues <= 0 {
		return nil, fmt.Errorf("resampling limits must be positive")
	}
	if c.EndStep-c.StartStep > int64(l.MaxOutputSignals) {
		return nil, fmt.Errorf("output signal capacity exceeded")
	}
	if c.StartStep < c.EndStep {
		if _, err := c.Clock.TimestampAt(c.EndStep - 1); err != nil {
			return nil, err
		}
		if _, _, err := sourcePosition(c, c.EndStep-1); err != nil {
			return nil, err
		}
	}
	return &StreamingResampler{config: c, nextStep: c.StartStep, initialized: true}, nil
}

// sourcePosition retains the exact fractional remainder. Neither timestamp
// comparisons nor repeated stepping use floating point or rounded increments.
func sourcePosition(c ResampleConfig, step int64) (int64, uint64, error) {
	hi, lo := bits.Mul64(uint64(step), uint64(c.SourceTicks))
	den := uint64(c.SimulationSteps)
	if hi >= den {
		return 0, 0, fmt.Errorf("source timestamp overflow")
	}
	q, r := bits.Div64(hi, lo, den)
	if q > math.MaxInt64 {
		return 0, 0, fmt.Errorf("source timestamp overflow")
	}
	return int64(q), r, nil
}

// Push accepts monotonically timed samples and an exclusive source watermark.
// The caller promises that no later chunk contains a timestamp below that mark.
// Samples exactly at the watermark stay open: more simultaneous samples may
// arrive in the next chunk, in any sequence order. All chunk timestamps must be
// between the previous and new watermarks, inclusive. A watermark may repeat.
//
// Only output positions strictly before the watermark are emitted. Simultaneous
// groups are finalized before use. Errors and cancellation return no output and
// leave the processor unchanged. An empty chunk may advance the watermark.
func (r *StreamingResampler) Push(ctx context.Context, input []Signal, watermark Timestamp) (ResampledBatch, error) {
	if r == nil || !r.initialized {
		return ResampledBatch{}, fmt.Errorf("uninitialized resampler")
	}
	if err := watermark.Validate(); err != nil {
		return ResampledBatch{}, err
	}
	if watermark.Unit != r.config.SourceUnit || watermark.Value < r.watermark {
		return ResampledBatch{}, fmt.Errorf("watermark unit mismatch or time regression")
	}
	return r.process(ctx, input, watermark.Value, false)
}

// Finish closes the remaining simultaneous group and applies the declared tail
// policy through EndStep. It succeeds only once. Cancellation leaves it open.
func (r *StreamingResampler) Finish(ctx context.Context) (ResampledBatch, error) {
	if r == nil || !r.initialized {
		return ResampledBatch{}, fmt.Errorf("uninitialized resampler")
	}
	return r.process(ctx, nil, r.watermark, true)
}

// ResampleOffline aligns a complete finite stream in either declared mode.
// OfflineLinear may use later source values; never use its output to claim
// realtime latency. CausalHold has the same result as chunked Push + Finish.
func ResampleOffline(ctx context.Context, config ResampleConfig, input []Signal) (ResampledBatch, error) {
	r, err := newResampler(config)
	if err != nil {
		return ResampledBatch{}, err
	}
	return r.process(ctx, input, 0, true)
}

func (r *StreamingResampler) process(ctx context.Context, input []Signal, watermark int64, final bool) (ResampledBatch, error) {
	if ctx == nil {
		return ResampledBatch{}, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return ResampledBatch{}, err
	}
	if r.finished {
		return ResampledBatch{}, fmt.Errorf("resampler already finished")
	}
	c := r.config
	if len(input) > c.Limits.MaxBufferedSignals-len(r.pending) {
		return ResampledBatch{}, fmt.Errorf("buffered signal capacity exceeded")
	}
	template := r.held
	if template == nil && len(r.pending) > 0 {
		template = &r.pending[0]
	}
	// Account for payloads before Signal.Validate, which itself copies values.
	for i, s := range input {
		if err := ctx.Err(); err != nil {
			return ResampledBatch{}, err
		}
		if !final && (s.start.Value < r.watermark || s.start.Value > watermark) {
			return ResampledBatch{}, fmt.Errorf("sample %d outside watermark interval", i)
		}
		if len(s.values) > c.Limits.MaxValues {
			return ResampledBatch{}, fmt.Errorf("sample value capacity exceeded")
		}
		if template == nil {
			v := s
			template = &v
		}
		if !sameSampleMetadata(*template, s) {
			return ResampledBatch{}, fmt.Errorf("sample %d changes stream metadata", i)
		}
	}
	if template != nil {
		slots := uint64(len(input)) + uint64(len(r.pending)) + 1 + uint64(c.EndStep-c.StartStep)
		if slots > uint64(c.Limits.MaxValues) || uint64(len(template.values)) > uint64(c.Limits.MaxValues)/slots {
			return ResampledBatch{}, fmt.Errorf("resampling value capacity exceeded")
		}
	}
	work := make([]Signal, 0, len(r.pending)+len(input))
	work = append(work, r.pending...)
	work = append(work, input...)
	ordered, err := OrderSignals(work)
	if err != nil {
		return ResampledBatch{}, err
	}
	if err := ValidateChronological(ordered); err != nil {
		return ResampledBatch{}, err
	}
	for _, s := range ordered {
		if s.start.Unit != c.SourceUnit || s.duration != 0 || (s.kind != KindContinuous && s.kind != KindActivity && s.kind != KindModulation) {
			return ResampledBatch{}, fmt.Errorf("resampling requires point samples in the declared unit of continuous, activity, or modulation kind")
		}
		if r.held != nil && s.sourceSequence <= r.held.sourceSequence {
			return ResampledBatch{}, fmt.Errorf("source sequence does not increase after finalized samples")
		}
	}
	// Keep open groups intact so duplicates split across chunks are still rejected.
	pending := make([]Signal, 0)
	groups := make([]Signal, 0, len(ordered)+1)
	if r.held != nil {
		groups = append(groups, *r.held)
	}
	for _, s := range ordered {
		if !final && s.start.Value == watermark {
			pending = append(pending, s)
			continue
		}
		if len(groups) > 0 && groups[len(groups)-1].start.Value == s.start.Value {
			groups[len(groups)-1] = s
		} else {
			groups = append(groups, s)
		}
	}
	out := ResampledBatch{Mode: c.Mode, Signals: make([]Signal, 0)}
	next := r.nextStep
	left := -1
	for next < c.EndStep {
		if err := ctx.Err(); err != nil {
			return ResampledBatch{}, err
		}
		whole, rem, err := sourcePosition(c, next)
		if err != nil {
			return ResampledBatch{}, err
		}
		if !final && whole >= watermark {
			break
		}
		for left+1 < len(groups) && groups[left+1].start.Value <= whole {
			left++
		}
		if left >= 0 {
			spec := groups[left].Spec()
			if c.Mode == OfflineLinear && left+1 < len(groups) && (whole > groups[left].start.Value || rem > 0) {
				right := groups[left+1]
				// Form both weights exactly before their float64 rounding.
				// Rounding large time differences separately can select the right
				// endpoint early (for example, 2^53 / (2^53 + 1)).
				var numerator, denominator, remainder big.Int
				numerator.SetInt64(whole - groups[left].start.Value)
				denominator.SetInt64(c.SimulationSteps)
				numerator.Mul(&numerator, &denominator)
				remainder.SetUint64(rem)
				numerator.Add(&numerator, &remainder)
				denominator.Mul(&denominator, big.NewInt(right.start.Value-groups[left].start.Value))
				var ratio big.Rat
				fraction, _ := ratio.SetFrac(&numerator, &denominator).Float64()
				numerator.Sub(&denominator, &numerator)
				complement, _ := ratio.SetFrac(&numerator, &denominator).Float64()
				for i, a := range spec.Values {
					b := right.values[i]
					if a == b {
						continue
					}
					// Convex form avoids overflowing b-a for finite opposite-sign endpoints.
					v := complement*a + fraction*b
					// Roundoff cannot expand the endpoints' declared valid range.
					spec.Values[i] = math.Max(math.Min(a, b), math.Min(math.Max(a, b), v))
				}
			}
			spec.StreamID = c.OutputStreamID
			spec.SourceSequence = uint64(next)
			spec.Start, err = c.Clock.TimestampAt(next)
			if err != nil {
				return ResampledBatch{}, err
			}
			s, err := NewSignal(spec)
			if err != nil {
				return ResampledBatch{}, err
			}
			out.Signals = append(out.Signals, s)
		}
		next++
	}
	if err := ctx.Err(); err != nil {
		return ResampledBatch{}, err
	}
	// Commit only after every validation and output construction has succeeded.
	r.pending = pending
	r.nextStep = next
	r.watermark = watermark
	r.finished = final
	if len(groups) > 0 {
		v := groups[len(groups)-1]
		r.held = &v
	}
	return out, nil
}

func sameSampleMetadata(a, b Signal) bool {
	return a.schemaVersion == b.schemaVersion && a.experienceID == b.experienceID && a.streamID == b.streamID &&
		a.channel == b.channel && a.kind == b.kind && a.unit == b.unit && a.encoderVersion == b.encoderVersion &&
		a.interventionTarget == b.interventionTarget && reflect.DeepEqual(a.shape, b.shape) &&
		reflect.DeepEqual(a.validRange, b.validRange) && reflect.DeepEqual(a.quality, b.quality)
}
