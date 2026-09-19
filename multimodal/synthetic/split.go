package synthetic

import "fmt"

// SplitUnseenCombinations puts every sample whose (shape, colour) is in
// holdout into eval and every other sample into train; both are index lists
// into samples, sorted. holdout must be non-empty, distinct and in range, and
// at least one sample must land on each side.
func SplitUnseenCombinations(labels []Label, holdout []Label) (train, eval []int, err error) {
	if len(holdout) == 0 {
		return nil, nil, fmt.Errorf("holdout must not be empty")
	}
	seen := make(map[Label]bool, len(holdout))
	for _, h := range holdout {
		if h.Shape < 0 || h.Shape >= Shapes || h.Colour < 0 || h.Colour >= Colours {
			return nil, nil, fmt.Errorf("holdout label (shape %d, colour %d) out of range", h.Shape, h.Colour)
		}
		if seen[h] {
			return nil, nil, fmt.Errorf("holdout label (shape %d, colour %d) is duplicated", h.Shape, h.Colour)
		}
		seen[h] = true
	}
	train = make([]int, 0, len(labels))
	eval = make([]int, 0, len(labels))
	for i, l := range labels {
		if seen[l] {
			eval = append(eval, i)
		} else {
			train = append(train, i)
		}
	}
	if len(train) == 0 || len(eval) == 0 {
		return nil, nil, fmt.Errorf("split needs at least one sample on each side, got train %d eval %d", len(train), len(eval))
	}
	return train, eval, nil
}
