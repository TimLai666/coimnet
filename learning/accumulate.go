package learning

import "fmt"

// GradientAccumulator is the running sum of the gradients Step has taken since
// the last applied update, with how many of them the sum holds. It exists only
// in the middle of an accumulation window: when Count reaches
// Options.AccumulateSteps the trainer averages, clips, runs AdamW and drops the
// accumulator, so a trainer that applies every step never stores one and its
// snapshot is byte-identical to a snapshot written before this field existed.
type GradientAccumulator struct {
	Sum   []float64 `json:"sum"`
	Count int       `json:"count"`
}

// accumulateSteps resolves Options.AccumulateSteps: zero means one, which is
// the behaviour of every snapshot written before the field existed.
func accumulateSteps(o Options) int {
	if o.AccumulateSteps <= 0 {
		return 1
	}
	return o.AccumulateSteps
}

// lossScale resolves Options.LossScale: zero means one, so an unset field
// leaves the gradient untouched.
func lossScale(o Options) float64 {
	if o.LossScale == 0 {
		return 1
	}
	return o.LossScale
}

func copyAccumulator(a *GradientAccumulator) *GradientAccumulator {
	if a == nil {
		return nil
	}
	return &GradientAccumulator{Sum: append([]float64(nil), a.Sum...), Count: a.Count}
}

// validateAccumulator rejects a restored accumulator that cannot describe a
// partial window: a sum that does not cover the parameter vector, a
// non-finite entry, a count outside [0, AccumulateSteps), or a zero count
// carrying a nonzero sum. A nil accumulator is the normal "no window open"
// value and is always legal.
func validateAccumulator(a *GradientAccumulator, count int, o Options) error {
	if a == nil {
		return nil
	}
	if len(a.Sum) != count {
		return fmt.Errorf("gradient accumulator has %d values, the model has %d parameters", len(a.Sum), count)
	}
	steps := accumulateSteps(o)
	if a.Count < 0 || a.Count >= steps {
		return fmt.Errorf("gradient accumulator count %d is outside [0, %d)", a.Count, steps)
	}
	for i, v := range a.Sum {
		if !finite(v) {
			return fmt.Errorf("non-finite gradient accumulator value at parameter %d", i)
		}
		if a.Count == 0 && v != 0 {
			return fmt.Errorf("gradient accumulator holds no gradients but parameter %d is nonzero", i)
		}
	}
	return nil
}
