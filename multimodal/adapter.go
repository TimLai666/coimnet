package multimodal

import (
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/signal"
)

// Layout fixes the modality order and widths the adapter emits; every sample
// must declare exactly these modalities (missing ones as Present false).
type Layout struct {
	Order  []string       `json:"order"`
	Widths map[string]int `json:"widths"`
}

// Validate checks that the layout is non-empty, has unique names and declares
// a width >= 1 for every ordered modality.
func (l Layout) Validate() error {
	if len(l.Order) == 0 {
		return fmt.Errorf("layout order must not be empty")
	}
	seen := make(map[string]bool, len(l.Order))
	for _, name := range l.Order {
		if seen[name] {
			return fmt.Errorf("layout declares duplicate modality %q", name)
		}
		seen[name] = true
		w, ok := l.Widths[name]
		if !ok {
			return fmt.Errorf("layout has no width for modality %q", name)
		}
		if w < 1 {
			return fmt.Errorf("layout width for modality %q must be >= 1", name)
		}
	}
	return nil
}

// Width is the total adapter width: each modality contributes its values
// followed by one presence channel.
func (l Layout) Width() int {
	total := 0
	for _, name := range l.Order {
		total += l.Widths[name] + 1
	}
	return total
}

// Vector encodes one sample as [values_1..., presence_1, values_2...,
// presence_2, ...] in Layout.Order: a present modality contributes its values
// and 1; a missing one contributes width zeros and 0. There is no field for
// EntityID or EventID in the output and never will be. A sample whose
// modality set or shapes differ from the layout is an error naming the
// modality.
func Vector(s Sample, l Layout) ([]float64, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	for name := range s.Modalities {
		if _, ok := l.Widths[name]; !ok {
			return nil, fmt.Errorf("sample modality %q is not declared by layout", name)
		}
	}
	out := make([]float64, 0, l.Width())
	for _, name := range l.Order {
		m, ok := s.Modalities[name]
		if !ok {
			return nil, fmt.Errorf("sample is missing modality %q required by layout", name)
		}
		w := l.Widths[name]
		size, err := shapeSize(m.Shape)
		if err != nil {
			return nil, fmt.Errorf("modality %q: %v", name, err)
		}
		if size != w {
			return nil, fmt.Errorf("modality %q: shape size %d does not match layout width %d", name, size, w)
		}
		if !m.Present {
			out = append(out, make([]float64, w)...)
			out = append(out, 0)
			continue
		}
		if len(m.Values) != size {
			return nil, fmt.Errorf("modality %q: shape size %d does not match %d values", name, size, len(m.Values))
		}
		for i, v := range m.Values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("modality %q: values[%d] must be finite", name, i)
			}
		}
		out = append(out, m.Values...)
		out = append(out, 1)
	}
	return out, nil
}

// Pair indexes two samples; A and B are positions in the input slice.
type Pair struct {
	A int `json:"a"`
	B int `json:"b"`
}

// PairSynchronous returns index pairs (A, B) with A < B of distinct samples
// of the same EventID where the timestamp of modalityA in one sample equals
// the timestamp of modalityB in the other. The result is sorted and pairs
// never read Values. An unknown modality name is an error.
func PairSynchronous(samples []Sample, modalityA, modalityB string) ([]Pair, error) {
	known := make(map[string]bool)
	for _, s := range samples {
		for name := range s.Timestamps {
			known[name] = true
		}
	}
	if !known[modalityA] {
		return nil, fmt.Errorf("unknown modality %q", modalityA)
	}
	if !known[modalityB] {
		return nil, fmt.Errorf("unknown modality %q", modalityB)
	}
	timesA := make([]signal.Timestamp, len(samples))
	hasA := make([]bool, len(samples))
	timesB := make([]signal.Timestamp, len(samples))
	hasB := make([]bool, len(samples))
	for i, s := range samples {
		if t, ok := s.Timestamps[modalityA]; ok {
			timesA[i], hasA[i] = t, true
		}
		if t, ok := s.Timestamps[modalityB]; ok {
			timesB[i], hasB[i] = t, true
		}
	}
	var pairs []Pair
	for i := 0; i < len(samples); i++ {
		for j := i + 1; j < len(samples); j++ {
			if samples[i].EventID != samples[j].EventID {
				continue
			}
			pair := false
			if hasA[i] && hasB[j] && timesA[i] == timesB[j] {
				pair = true
			} else if hasA[j] && hasB[i] && timesA[j] == timesB[i] {
				pair = true
			}
			if pair {
				pairs = append(pairs, Pair{A: i, B: j})
			}
		}
	}
	return pairs, nil
}

// PairAsynchronous returns index pairs (A, B) with A < B of distinct samples
// of the same EntityID with different EventIDs. The result is sorted and
// pairs never read Values.
func PairAsynchronous(samples []Sample) ([]Pair, error) {
	var pairs []Pair
	for i := 0; i < len(samples); i++ {
		for j := i + 1; j < len(samples); j++ {
			if samples[i].EntityID == samples[j].EntityID && samples[i].EventID != samples[j].EventID {
				pairs = append(pairs, Pair{A: i, B: j})
			}
		}
	}
	return pairs, nil
}
