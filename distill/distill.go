// Package distill builds student training targets from teacher answers. Only
// the student's own encoders exist: a teacher answer is decoded under the
// declared kind and re-encoded by the student, teacher token ids never enter
// the student, and held-out inputs are never sent to a teacher.
package distill

import (
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/teacher"
)

// SchemaVersion identifies the distillation data schema.
const SchemaVersion = "coimnet-distill/v1"

var (
	// ErrTokenIDsRejected means a teacher answered with a numeric array under
	// a kind the student decodes as its own strings.
	ErrTokenIDsRejected = errors.New("distill: teacher token ids never enter the student")
	// ErrUnsupportedKind means the answer kind is not handled by this distiller.
	ErrUnsupportedKind = errors.New("distill: answer kind is not handled by this distiller")
)

// Target is the student's own training target for one input.
type Target struct {
	Values  []float64 `json:"values,omitempty"`
	Classes []int     `json:"classes,omitempty"`
}

// Split partitions inputs by hash; holdout inputs are never sent to a teacher.
type Split struct {
	Train   []string
	Holdout []string
}

// Validate checks that both partitions are sets of lower-hex-64 hashes, that
// train is non-empty and that no hash appears twice or in both partitions.
func (s Split) Validate() error {
	if len(s.Train) == 0 {
		return errors.New("distill: split train must be non-empty")
	}
	for _, h := range s.Train {
		if !isLowerHex64(h) {
			return fmt.Errorf("distill: split train hash %q is not 64 lowercase hex characters", h)
		}
	}
	for _, h := range s.Holdout {
		if !isLowerHex64(h) {
			return fmt.Errorf("distill: split holdout hash %q is not 64 lowercase hex characters", h)
		}
	}
	trainSeen := make(map[string]bool, len(s.Train))
	for _, h := range s.Train {
		if trainSeen[h] {
			return fmt.Errorf("distill: split train hash %q appears more than once", h)
		}
		trainSeen[h] = true
	}
	holdoutSeen := make(map[string]bool, len(s.Holdout))
	for _, h := range s.Holdout {
		if holdoutSeen[h] {
			return fmt.Errorf("distill: split holdout hash %q appears more than once", h)
		}
		holdoutSeen[h] = true
		if trainSeen[h] {
			return fmt.Errorf("distill: split train and holdout overlap on hash %q", h)
		}
	}
	return nil
}

func isLowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// LabelDistiller collects teacher targets for the train partition only.
type LabelDistiller struct {
	Teacher      teacher.Teacher
	Encoder      StudentEncoder
	Kind         string
	ModelVersion string
	Fields       map[string]string
}

// Example is one collected student target next to the teacher response it came
// from.
type Example struct {
	InputHash string           `json:"input_hash"`
	Target    Target           `json:"target"`
	Response  teacher.Response `json:"response"`
}

// CollectReport counts what a collection run asked for and kept.
type CollectReport struct {
	Asked        int      `json:"asked"`
	Answered     int      `json:"answered"`
	NoAnswer     int      `json:"no_answer"`
	Deduplicated int      `json:"deduplicated"`
	Holdout      int      `json:"holdout"`
	AskedHashes  []string `json:"asked_hashes,omitempty"`
}
