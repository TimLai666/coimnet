// Package tool runs only the tools on its allow-list, with arguments
// validated against each tool's schema before anything executes.
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Field is one member of a Schema: it names the JSON kind the argument must
// have and whether the member must be present.
type Field struct {
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

// Schema is the only argument shape a tool accepts: a flat JSON object whose
// members are named in Fields with a kind and a required flag. Unknown
// members, wrong kinds, missing required members and non-object arguments are
// refused before the tool runs, and the same check applies whether a person or
// a model wrote the arguments.
type Schema struct {
	Fields   map[string]Field `json:"fields"`
	MaxBytes int              `json:"max_bytes"`
}

const (
	defaultMaxBytes = 4096
	kindString      = "string"
	kindNumber      = "number"
	kindBoolean     = "boolean"
	kindInteger     = "integer"
)

// Validate reports whether the schema is usable: every field has a known kind
// and MaxBytes is zero or positive.
func (s Schema) Validate() error {
	switch s.MaxBytes {
	case 0:
		// Zero means defaultMaxBytes at use.
	default:
		if s.MaxBytes < 0 {
			return errors.New("tool: max_bytes must not be negative")
		}
	}
	for name, f := range s.Fields {
		switch f.Kind {
		case kindString, kindNumber, kindBoolean, kindInteger:
		default:
			return fmt.Errorf("tool: field %q has unknown kind %q", name, f.Kind)
		}
	}
	return nil
}

// Handler runs one validated call; it receives the decoded, validated members
// only and returns the result as raw data.
type Handler func(ctx context.Context, args map[string]any) (json.RawMessage, error)

// Registry holds the allow-list of tools and counts how many calls ran and
// how many were refused before running.
type Registry struct {
	Allowed  map[string]Schema
	handlers map[string]Handler
	calls    int
	refused  int
	mu       sync.Mutex
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		Allowed:  make(map[string]Schema),
		handlers: make(map[string]Handler),
	}
}

// Register adds a tool: a blank name, a duplicate, an invalid schema or a nil
// handler is an error.
func (r *Registry) Register(name string, s Schema, h Handler) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("tool: tool name must not be blank")
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if h == nil {
		return errors.New("tool: handler must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.handlers[name]; ok {
		return fmt.Errorf("tool: tool %q is already registered", name)
	}
	r.handlers[name] = h
	r.Allowed[name] = s
	return nil
}

// ErrToolNotAllowed reports a call to a name that is not on the allow-list.
var ErrToolNotAllowed = errors.New("tool: name is not in the allow-list")

// ErrArgumentsRejected reports arguments that do not match the schema.
var ErrArgumentsRejected = errors.New("tool: arguments do not match the schema")

// Invoke is the one explicit path to run a tool: the name must be registered,
// args must be a JSON object within MaxBytes that matches the schema exactly,
// and the result is the handler's raw JSON returned as data. Nothing in a
// result is ever executed, parsed as an instruction or fed back into another
// Invoke by this package.
func (r *Registry) Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	r.mu.Lock()
	h, ok := r.handlers[name]
	if !ok {
		r.refused++
		r.mu.Unlock()
		return nil, ErrToolNotAllowed
	}
	schema := r.Allowed[name]
	r.mu.Unlock()

	validated, err := validateArgs(schema, args)
	if err != nil {
		r.mu.Lock()
		r.refused++
		r.mu.Unlock()
		return nil, ErrArgumentsRejected
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("tool: %s: %w", name, err)
	}

	r.mu.Lock()
	r.calls++
	r.mu.Unlock()

	return h(ctx, validated)
}

// validateArgs decodes args as a JSON object with json.Number values and
// checks it against the schema: the byte size, the object shape, and every
// member's presence and kind.
func validateArgs(s Schema, args json.RawMessage) (map[string]any, error) {
	limit := s.MaxBytes
	if limit == 0 {
		limit = defaultMaxBytes
	}
	if len(args) > limit {
		return nil, errors.New("tool: args exceed max_bytes")
	}

	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("tool: args contain trailing data")
	}

	for name, field := range s.Fields {
		val, ok := m[name]
		if !ok {
			if field.Required {
				return nil, fmt.Errorf("tool: missing required argument %q", name)
			}
			continue
		}
		if !kindMatches(field.Kind, val) {
			return nil, fmt.Errorf("tool: argument %q is not of kind %q", name, field.Kind)
		}
	}
	for name := range m {
		if _, ok := s.Fields[name]; !ok {
			return nil, fmt.Errorf("tool: unknown argument %q", name)
		}
	}
	return m, nil
}

// kindMatches reports whether val has the JSON kind named by kind. Numbers
// arrive as json.Number; integer requires a literal without a decimal point
// or exponent.
func kindMatches(kind string, val any) bool {
	switch kind {
	case kindString:
		_, ok := val.(string)
		return ok
	case kindBoolean:
		_, ok := val.(bool)
		return ok
	case kindNumber:
		_, ok := val.(json.Number)
		return ok
	case kindInteger:
		n, ok := val.(json.Number)
		if !ok {
			return false
		}
		return !strings.ContainsAny(n.String(), ".eE")
	default:
		return false
	}
}

// Counts mirrors the running counts of accepted calls and refused calls.
type Counts struct {
	Calls   int
	Refused int
}

// Counts returns a snapshot of how many calls ran and how many were refused.
func (r *Registry) Counts() Counts {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Counts{Calls: r.calls, Refused: r.refused}
}
