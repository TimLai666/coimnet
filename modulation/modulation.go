// Package modulation is the first layer of chemical modulation: a declared
// source turns the state of one step into a non-negative release rate per
// modulation channel. What happens to a released rate afterwards, the
// concentration dynamics, is a separate layer and is not in this package.
//
// A source sees an observation, the node activity of the current step, the
// feedback that has already arrived and the declared resources of the task. It
// never sees a training target: no type of this package carries one, and the
// package does not import the learning package, so a target cannot reach a
// release rate by any path an import could open.
package modulation

import (
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/signal"
)

// ReleaseTimeUnit is the clock a release step is counted in. Feedback is
// compared against the step it is offered to, so its timestamps must be on this
// same clock; feedback on any other clock is rejected rather than converted.
const ReleaseTimeUnit = signal.TimeUnitModelStep

// SourceContext is everything a source may read. Observation is nil for a
// source that ignores it. Activity is the node-level activity of the current
// step, supplied by the caller, and is nil when the caller has none. Feedback
// has already been filtered with signal.AvailableFeedback, and every source
// re-checks it anyway. Resources holds the declared resource amounts of the
// task.
//
// There is deliberately no target field: a source that cannot name the answer
// cannot leak it.
type SourceContext struct {
	Observation *signal.Observation
	Activity    []float64
	Feedback    []signal.Feedback
	Resources   map[string]float64
}

// Source produces the release rate of one step. Channels is the width of the
// slice Release returns; Release returns one non-negative, finite rate per
// channel, or an error. A source never mutates the context it is given.
type Source interface {
	Channels() int
	Release(step uint64, c SourceContext) ([]float64, error)
}

// ErrControllerNotImplemented is returned by NewController. The trainable
// controller source is reserved by name here so that the four implemented
// sources are not mistaken for the whole list; it is MOD-07 and is a separate
// ticket.
var ErrControllerNotImplemented = errors.New("modulation: the trainable controller source is not implemented (MOD-07)")

// Controller is the reserved name of the trainable controller source. It has no
// behaviour: NewController always fails with ErrControllerNotImplemented.
type Controller struct{}

// NewController always returns ErrControllerNotImplemented.
func NewController() (*Controller, error) { return nil, ErrControllerNotImplemented }

// checkFeedback rejects feedback that has not arrived at this step. The caller
// is expected to have filtered it already, which is exactly why a source does
// not rely on that: an early feedback is a failure, not a value to skip.
func (c SourceContext) checkFeedback(step uint64) error {
	if len(c.Feedback) == 0 {
		return nil
	}
	now, err := stepTimestamp(step)
	if err != nil {
		return err
	}
	for i, f := range c.Feedback {
		if err := f.ValidateAvailableAt(now); err != nil {
			return fmt.Errorf("modulation: feedback %d cannot be used at step %d: %w", i, step, err)
		}
	}
	return nil
}

func stepTimestamp(step uint64) (signal.Timestamp, error) {
	if step > math.MaxInt64 {
		return signal.Timestamp{}, fmt.Errorf("modulation: step %d does not fit an int64 timestamp", step)
	}
	return signal.NewTimestamp(int64(step), ReleaseTimeUnit)
}

// checkedRelease is the single gate every source returns through. A release
// rate is a rate of arrival: it is finite, it is never negative, and there is
// exactly one of it per declared channel. A zero is stored as a positive zero
// so that two runs cannot differ in the sign of a zero.
func checkedRelease(rates []float64, channels int) ([]float64, error) {
	if channels <= 0 {
		return nil, fmt.Errorf("modulation: a source must declare at least one channel, got %d", channels)
	}
	if len(rates) != channels {
		return nil, fmt.Errorf("modulation: source released %d rates for %d channels", len(rates), channels)
	}
	for i, v := range rates {
		if !finite(v) {
			return nil, fmt.Errorf("modulation: channel %d released a non-finite rate", i)
		}
		if v < 0 {
			return nil, fmt.Errorf("modulation: channel %d released a negative rate %v", i, v)
		}
		if v == 0 {
			rates[i] = 0
		}
	}
	return rates, nil
}

// nonNegative clamps at zero without producing a negative zero.
func nonNegative(v float64) float64 {
	if v > 0 {
		return v
	}
	return 0
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
