package learning

import (
	"fmt"
	"math"
)

// The learning-rate schedule kinds. A linear warmup applies to all three.
const (
	ScheduleConstant = "constant"
	ScheduleStep     = "step"
	ScheduleCosine   = "cosine"
)

// Schedule shapes the learning rate as a pure function of the trainer's
// completed update count, so a restored trainer continues on the same curve
// without carrying any scheduler state of its own.
//
// WarmupUpdates ramps the rate linearly from zero to Options.LearningRate over
// that many updates and applies to every kind. After the warmup, "constant"
// holds the base rate, "step" multiplies it by StepFactor once per StepEvery
// completed updates, and "cosine" follows half a cosine from the base rate down
// to base * FinalFactor over DecayUpdates and then holds it there. A field the
// selected kind does not read must be left at zero rather than silently
// ignored.
type Schedule struct {
	Kind          string  `json:"kind"`
	WarmupUpdates uint64  `json:"warmup_updates,omitempty"`
	DecayUpdates  uint64  `json:"decay_updates,omitempty"`
	FinalFactor   float64 `json:"final_factor,omitempty"`
	StepEvery     uint64  `json:"step_every,omitempty"`
	StepFactor    float64 `json:"step_factor,omitempty"`
}

// LearningRateAt is the rate an update that raises the completed update count
// from updates to updates+1 uses. Options without a schedule hold
// Options.LearningRate at every count. A schedule NewTrainer would have
// rejected falls back to the base rate rather than inventing a curve.
func LearningRateAt(o Options, updates uint64) float64 {
	s := o.Schedule
	if s == nil {
		return o.LearningRate
	}
	if updates < s.WarmupUpdates {
		return o.LearningRate * float64(updates) / float64(s.WarmupUpdates)
	}
	switch s.Kind {
	case ScheduleStep:
		if s.StepEvery == 0 {
			return o.LearningRate
		}
		return o.LearningRate * math.Pow(s.StepFactor, float64(updates/s.StepEvery))
	case ScheduleCosine:
		if s.DecayUpdates == 0 {
			return o.LearningRate * s.FinalFactor
		}
		progress := float64(updates-s.WarmupUpdates) / float64(s.DecayUpdates)
		if progress > 1 {
			progress = 1
		}
		return o.LearningRate * (s.FinalFactor + (1-s.FinalFactor)*.5*(1+math.Cos(math.Pi*progress)))
	}
	return o.LearningRate
}

// validateSchedule rejects an unknown kind, a bound that cannot describe a
// curve, and a field the selected kind does not read, so no declared knob can
// be silently ignored.
func validateSchedule(s *Schedule) error {
	if s == nil {
		return nil
	}
	if !finite(s.FinalFactor) || !finite(s.StepFactor) {
		return fmt.Errorf("schedule final_factor or step_factor is not finite")
	}
	if s.FinalFactor < 0 || s.FinalFactor > 1 {
		return fmt.Errorf("schedule final_factor %g is outside [0, 1]", s.FinalFactor)
	}
	reads := map[string]bool{}
	switch s.Kind {
	case ScheduleConstant:
	case ScheduleStep:
		reads["step_every"], reads["step_factor"] = true, true
		if s.StepEvery == 0 {
			return fmt.Errorf("schedule kind %q needs a positive step_every", s.Kind)
		}
		if s.StepFactor <= 0 {
			return fmt.Errorf("schedule step_factor %g must be positive", s.StepFactor)
		}
	case ScheduleCosine:
		reads["decay_updates"], reads["final_factor"] = true, true
		if s.DecayUpdates == 0 {
			return fmt.Errorf("schedule kind %q needs a positive decay_updates", s.Kind)
		}
	default:
		return fmt.Errorf("unknown schedule kind %q, want %q, %q or %q", s.Kind, ScheduleConstant, ScheduleStep, ScheduleCosine)
	}
	for _, field := range []struct {
		name    string
		written bool
	}{
		{"decay_updates", s.DecayUpdates != 0}, {"final_factor", s.FinalFactor != 0},
		{"step_every", s.StepEvery != 0}, {"step_factor", s.StepFactor != 0},
	} {
		if field.written && !reads[field.name] {
			return fmt.Errorf("schedule kind %q does not read %s", s.Kind, field.name)
		}
	}
	return nil
}
