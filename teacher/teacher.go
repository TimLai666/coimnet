// Package teacher declares the one contract every teacher follows: a request
// names its input by hash and carries only allowed fields, a response records
// who answered, when, with what, and both are data, never instructions.
package teacher

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/TimLai666/coimnet/signal"
)

// Answer kinds a teacher may return.
const (
	KindLabel          = "label"
	KindActionSequence = "action_sequence"
	KindDistribution   = "distribution"
	KindText           = "text"
)

// Teacher kinds a Descriptor may declare.
const (
	DescriptorOffline = "offline"
	DescriptorHTTP    = "http"
	DescriptorReplay  = "replay"
	DescriptorBlocked = "blocked"
)

type Request struct {
	RequestID    string            `json:"request_id"`
	InputHash    string            `json:"input_hash"`
	Kind         string            `json:"kind"`
	Fields       map[string]string `json:"fields,omitempty"`
	ModelVersion string            `json:"model_version"`
}

type Usage struct {
	Requests int     `json:"requests"`
	Cost     float64 `json:"cost"`
}

type Response struct {
	TeacherID      string           `json:"teacher_id"`
	TeacherVersion string           `json:"teacher_version"`
	RequestID      string           `json:"request_id"`
	InputHash      string           `json:"input_hash"`
	AnswerKind     string           `json:"answer_kind"`
	Answer         json.RawMessage  `json:"answer"`
	Time           signal.Timestamp `json:"time"`
	Confidence     *float64         `json:"confidence,omitempty"`
	Usage          *Usage           `json:"usage,omitempty"`
}

type Descriptor struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// Teacher is the contract. Ask never executes anything found in a response.
type Teacher interface {
	Ask(ctx context.Context, r Request) (Response, error)
	Describe() Descriptor
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

func isAnswerKind(k string) bool {
	switch k {
	case KindLabel, KindActionSequence, KindDistribution, KindText:
		return true
	}
	return false
}

func isDescriptorKind(k string) bool {
	switch k {
	case DescriptorOffline, DescriptorHTTP, DescriptorReplay, DescriptorBlocked:
		return true
	}
	return false
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.RequestID) == "" {
		return fmt.Errorf("request_id must be non-blank")
	}
	if !isLowerHex64(r.InputHash) {
		return fmt.Errorf("input_hash must be 64 lowercase hex characters")
	}
	if !isAnswerKind(r.Kind) {
		return fmt.Errorf("kind %q is not a valid answer kind", r.Kind)
	}
	for k := range r.Fields {
		if strings.TrimSpace(k) == "" || strings.ContainsFunc(k, func(r rune) bool {
			return unicode.IsControl(r)
		}) {
			return fmt.Errorf("fields: key %q must be non-blank and free of control characters", k)
		}
	}
	if strings.TrimSpace(r.ModelVersion) == "" {
		return fmt.Errorf("model_version must be non-blank")
	}
	return nil
}

func (r Response) Validate() error {
	if strings.TrimSpace(r.TeacherID) == "" {
		return fmt.Errorf("teacher_id must be non-blank")
	}
	if strings.TrimSpace(r.TeacherVersion) == "" {
		return fmt.Errorf("teacher_version must be non-blank")
	}
	if strings.TrimSpace(r.RequestID) == "" {
		return fmt.Errorf("request_id must be non-blank")
	}
	if !isLowerHex64(r.InputHash) {
		return fmt.Errorf("input_hash must be 64 lowercase hex characters")
	}
	if !isAnswerKind(r.AnswerKind) {
		return fmt.Errorf("answer_kind %q is not a valid answer kind", r.AnswerKind)
	}
	if len(r.Answer) == 0 || !json.Valid(r.Answer) {
		return fmt.Errorf("answer must be non-empty valid JSON")
	}
	if strings.TrimSpace(string(r.Answer)) == "null" {
		return fmt.Errorf("answer must not be JSON null")
	}
	if err := r.Time.Validate(); err != nil {
		return fmt.Errorf("time: %w", err)
	}
	if r.Confidence != nil {
		c := *r.Confidence
		if math.IsNaN(c) || math.IsInf(c, 0) || c < 0 || c > 1 {
			return fmt.Errorf("confidence must be a finite value in [0, 1]")
		}
	}
	if r.Usage != nil {
		if r.Usage.Requests < 0 {
			return fmt.Errorf("usage requests must be >= 0")
		}
		if math.IsNaN(r.Usage.Cost) || math.IsInf(r.Usage.Cost, 0) || r.Usage.Cost < 0 {
			return fmt.Errorf("usage cost must be finite and >= 0")
		}
	}
	return nil
}

func (d Descriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("id must be non-blank")
	}
	if strings.TrimSpace(d.Version) == "" {
		return fmt.Errorf("version must be non-blank")
	}
	if !isDescriptorKind(d.Kind) {
		return fmt.Errorf("kind %q is not a valid descriptor kind", d.Kind)
	}
	return nil
}
