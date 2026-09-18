// Package multimodal defines the paired-modality data contract (TSK-10): a
// Sample pairs the modalities of one event of one entity, a missing modality
// is encoded by presence 0 and width zeros (never a zero vector with presence
// 1), and EntityID/EventID never enter the adapter output.
package multimodal

import (
	"fmt"
	"math"
	"strings"

	"github.com/TimLai666/coimnet/signal"
)

// SchemaVersion identifies the multimodal sample contract.
const SchemaVersion = "coimnet-multimodal-sample/v1"

// Modality is one modality of one sample. Present false means the modality is
// missing for this sample: Values must then be nil (not a zero vector) and
// Shape still declares the width the adapter reserves. Present true needs
// len(Values) == product(Shape) > 0 with finite values.
type Modality struct {
	Present bool      `json:"present"`
	Values  []float64 `json:"values"`
	Shape   []int     `json:"shape"`
}

// Sample pairs the modalities of one event of one entity. EntityID and
// EventID are read only by evaluators for pairing and grouping; the adapter
// has no path that encodes them.
type Sample struct {
	EntityID   string                      `json:"entity_id"`
	EventID    string                      `json:"event_id"`
	Modalities map[string]Modality         `json:"modalities"`
	Timestamps map[string]signal.Timestamp `json:"timestamps"`
}

// Validate checks the sample data contract: ids non-blank, at least one
// modality, every modality valid ("values must be nil" for a missing one,
// "shape" for a mismatch, "finite" for values), every Timestamps key names a
// modality, and Present modalities have a timestamp.
func (s Sample) Validate() error {
	if strings.TrimSpace(s.EntityID) == "" {
		return fmt.Errorf("entity_id must not be blank")
	}
	if strings.TrimSpace(s.EventID) == "" {
		return fmt.Errorf("event_id must not be blank")
	}
	if len(s.Modalities) == 0 {
		return fmt.Errorf("sample must have at least one modality")
	}
	for name, m := range s.Modalities {
		if err := validateModality(name, m); err != nil {
			return err
		}
	}
	for key := range s.Timestamps {
		if _, ok := s.Modalities[key]; !ok {
			return fmt.Errorf("timestamp %q does not name a modality", key)
		}
	}
	for name, m := range s.Modalities {
		if m.Present {
			if _, ok := s.Timestamps[name]; !ok {
				return fmt.Errorf("modality %q is present but has no timestamp", name)
			}
		}
	}
	return nil
}

func validateModality(name string, m Modality) error {
	size, err := shapeSize(m.Shape)
	if err != nil {
		return fmt.Errorf("modality %q: shape: %v", name, err)
	}
	if !m.Present {
		if m.Values != nil {
			return fmt.Errorf("modality %q: missing modality values must be nil", name)
		}
		return nil
	}
	if len(m.Values) != size {
		return fmt.Errorf("modality %q: shape declares %d values, got %d values", name, size, len(m.Values))
	}
	for i, v := range m.Values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("modality %q: values[%d] must be finite", name, i)
		}
	}
	return nil
}

func shapeSize(shape []int) (int, error) {
	if len(shape) == 0 {
		return 0, fmt.Errorf("shape must contain at least one dimension")
	}
	maxInt := int(^uint(0) >> 1)
	size := 1
	for i, dim := range shape {
		if dim <= 0 {
			return 0, fmt.Errorf("shape[%d] must be positive", i)
		}
		if size > maxInt/dim {
			return 0, fmt.Errorf("shape capacity overflows int")
		}
		size *= dim
	}
	return size, nil
}
