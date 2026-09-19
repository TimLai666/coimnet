package ctc

import (
	"errors"
	"fmt"
	"math"
)

// Decode is greedy CTC decoding: per-frame argmax, collapse consecutive
// repeats, drop blanks. Ties go to the lower class index. A frame with a
// non-finite value is an error.
func Decode(logits [][]float64) ([]int, error) {
	if len(logits) == 0 {
		return nil, errors.New("ctc: decode needs at least one frame")
	}
	classes := len(logits[0])
	if classes < 2 {
		return nil, fmt.Errorf("ctc: decode needs at least two classes, got %d", classes)
	}
	out := make([]int, 0, len(logits))
	last := -1
	for t, row := range logits {
		if len(row) != classes {
			return nil, fmt.Errorf("ctc: frame %d has %d classes, want %d", t, len(row), classes)
		}
		for k, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("ctc: frame %d class %d value %v is not finite", t, k, v)
			}
		}
		best := 0
		for k := 1; k < classes; k++ {
			if row[k] > row[best] {
				best = k
			}
		}
		if best == 0 {
			last = -1
			continue
		}
		if best != last {
			out = append(out, best)
			last = best
		}
	}
	return out, nil
}
