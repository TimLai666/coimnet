package distill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/TimLai666/coimnet/teacher"
)

// StudentEncoder turns a decoded teacher answer into a Target. Only the
// student's own encoders exist: a teacher answer is decoded as a JSON string
// (text, label) or an array of strings (action_sequence) and re-encoded by the
// student; a numeric array under those kinds is ErrTokenIDsRejected.
type StudentEncoder interface {
	Encode(kind string, answer json.RawMessage) (Target, error)
}

// Tokenizer is the student's own vocabulary.
type Tokenizer interface {
	VocabHash() string
	Encode(text string) ([]int, error)
}

// TextEncoder maps a text answer to the student tokenizer's class ids.
type TextEncoder struct {
	Tokenizer Tokenizer
}

// LabelEncoder maps a label answer to the index of its class.
type LabelEncoder struct {
	Classes []string
}

// ActionEncoder maps an action-sequence answer to the index of each action.
type ActionEncoder struct {
	Actions []string
}

// Encode implements StudentEncoder for kind text.
func (e TextEncoder) Encode(kind string, answer json.RawMessage) (Target, error) {
	if kind != teacher.KindText {
		return Target{}, ErrUnsupportedKind
	}
	if e.Tokenizer == nil {
		return Target{}, errors.New("distill: text encoder: nil tokenizer")
	}
	s, err := decodeStringAnswer(kind, answer)
	if err != nil {
		return Target{}, err
	}
	ids, err := e.Tokenizer.Encode(s)
	if err != nil {
		return Target{}, fmt.Errorf("distill: text encoder: tokenize %q: %w", s, err)
	}
	return Target{Classes: ids}, nil
}

// Encode implements StudentEncoder for kind label.
func (e LabelEncoder) Encode(kind string, answer json.RawMessage) (Target, error) {
	if kind != teacher.KindLabel {
		return Target{}, ErrUnsupportedKind
	}
	s, err := decodeStringAnswer(kind, answer)
	if err != nil {
		return Target{}, err
	}
	for i, class := range e.Classes {
		if class == s {
			return Target{Classes: []int{i}}, nil
		}
	}
	return Target{}, fmt.Errorf("distill: label encoder: unknown label %q", s)
}

// Encode implements StudentEncoder for kind action_sequence.
func (e ActionEncoder) Encode(kind string, answer json.RawMessage) (Target, error) {
	if kind != teacher.KindActionSequence {
		return Target{}, ErrUnsupportedKind
	}
	actions, err := decodeStringArray(answer)
	if err != nil {
		return Target{}, err
	}
	ids := make([]int, len(actions))
	for i, action := range actions {
		id := -1
		for j, known := range e.Actions {
			if known == action {
				id = j
				break
			}
		}
		if id < 0 {
			return Target{}, fmt.Errorf("distill: action encoder: unknown action %q", action)
		}
		ids[i] = id
	}
	return Target{Classes: ids}, nil
}

// decodeStringAnswer decodes a non-null JSON string answer. A numeric array
// under a string kind is a teacher token-id array and is rejected.
func decodeStringAnswer(kind string, answer json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(answer)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", fmt.Errorf("distill: %s answer must be a non-null JSON string", kind)
	}
	if trimmed[0] == '[' {
		if isNumericArray(trimmed) {
			return "", ErrTokenIDsRejected
		}
		return "", fmt.Errorf("distill: %s answer must be a JSON string, got an array", kind)
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return "", fmt.Errorf("distill: %s answer must be a JSON string: %w", kind, err)
	}
	return s, nil
}

// decodeStringArray decodes a non-null JSON array of strings. A numeric array
// is a teacher token-id array and is rejected.
func decodeStringArray(answer json.RawMessage) ([]string, error) {
	trimmed := bytes.TrimSpace(answer)
	if len(trimmed) == 0 || string(trimmed) == "null" || trimmed[0] != '[' {
		return nil, errors.New("distill: action_sequence answer must be a JSON array of strings")
	}
	var actions []string
	if err := json.Unmarshal(trimmed, &actions); err != nil {
		if isNumericArray(trimmed) {
			return nil, ErrTokenIDsRejected
		}
		return nil, fmt.Errorf("distill: action_sequence answer must be a JSON array of strings: %w", err)
	}
	return actions, nil
}

// isNumericArray reports whether the JSON value is an array of numbers.
func isNumericArray(b []byte) bool {
	var nums []json.Number
	return json.Unmarshal(b, &nums) == nil
}
