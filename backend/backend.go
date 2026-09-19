// Package backend declares what each execution backend can run and how a
// requested backend is chosen, with an explicit CPU fallback. It covers the
// device-capability part of ticket 30 (OPS-05 first stage); the CPU backend
// declares every capability true and is the reference path.
package backend

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Capabilities lists what a backend can execute. Every field is a hard yes/no.
type Capabilities struct {
	ContinuousForward bool `json:"continuous_forward"`
	SpikingForward    bool `json:"spiking_forward"`
	SparseBackward    bool `json:"sparse_backward"`
	VariableWeights   bool `json:"variable_weights"`
	TeacherLoss       bool `json:"teacher_loss"`
	DeviceStateSave   bool `json:"device_state_save"`
	Deterministic     bool `json:"deterministic"`
}

// Requirement is what one configuration needs, in the same shape.
type Requirement Capabilities

// Backend is a named execution target whose supported operations are
// declared by its Capabilities.
type Backend interface {
	Name() string
	Capabilities() Capabilities
}

// CPU is the reference backend: every capability true.
type CPU struct{}

// Name returns "cpu".
func (CPU) Name() string {
	return "cpu"
}

// Capabilities returns the CPU reference implementation, with every field true.
func (CPU) Capabilities() Capabilities {
	return Capabilities{
		ContinuousForward: true,
		SpikingForward:    true,
		SparseBackward:    true,
		VariableWeights:   true,
		TeacherLoss:       true,
		DeviceStateSave:   true,
		Deterministic:     true,
	}
}

// capJSONNames holds the JSON name of each Capabilities field in declaration
// order, so Missing can report stable names derived from the same tags.
var capJSONNames = func() []string {
	typ := reflect.TypeOf(Capabilities{})
	names := make([]string, typ.NumField())
	for i := range names {
		names[i] = strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
	}
	return names
}()

// Missing returns the JSON names of the required capabilities `have` lacks,
// sorted; nil when none.
func Missing(have Capabilities, need Requirement) []string {
	h := reflect.ValueOf(have)
	n := reflect.ValueOf(need)
	var missing []string
	for i := 0; i < n.NumField(); i++ {
		if n.Field(i).Bool() && !h.Field(i).Bool() {
			missing = append(missing, capJSONNames[i])
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return missing
}

// Check is Missing as an error: nil, or `backend "<name>" lacks: a, b`.
func Check(b Backend, need Requirement) error {
	if missing := Missing(b.Capabilities(), need); len(missing) > 0 {
		return fmt.Errorf("backend %q lacks: %s", b.Name(), strings.Join(missing, ", "))
	}
	return nil
}

// FallbackReport says which backend ran and why; FellBack false means the
// requested backend ran.
type FallbackReport struct {
	Requested string   `json:"requested"`
	Used      string   `json:"used"`
	FellBack  bool     `json:"fell_back"`
	Reason    string   `json:"reason,omitempty"`
	Missing   []string `json:"missing,omitempty"`
}

// Select returns the backend in available whose Name equals requested (an
// empty requested means "cpu"; when available has no cpu, CPU{} is treated
// as present). If found and Check passes, it is returned with a FellBack
// false report. If not found or Check fails, allowFallback true returns
// CPU{} with a FellBack true report filled in (Reason "not registered" or
// "lacks: a, b" and the Missing list), but only when CPU itself passes
// Check, otherwise an error. allowFallback false returns an error naming the
// requested backend and the reason, with a zero FallbackReport. Two backends
// with the same name in available is an error.
func Select(requested string, need Requirement, available []Backend, allowFallback bool) (Backend, FallbackReport, error) {
	name := requested
	if name == "" {
		name = "cpu"
	}
	var matches []Backend
	for _, b := range available {
		if b.Name() == name {
			matches = append(matches, b)
		}
	}
	if name == "cpu" && len(matches) == 0 {
		matches = append(matches, CPU{})
	}
	if len(matches) > 1 {
		return nil, FallbackReport{}, fmt.Errorf("duplicate backend %q registered", name)
	}

	if len(matches) == 1 {
		b := matches[0]
		missing := Missing(b.Capabilities(), need)
		if len(missing) > 0 {
			reason := "lacks: " + strings.Join(missing, ", ")
			if !allowFallback {
				return nil, FallbackReport{}, fmt.Errorf("backend %q %s", name, reason)
			}
			if err := Check(CPU{}, need); err != nil {
				return nil, FallbackReport{}, fmt.Errorf("cpu fallback unavailable for %q: %w", name, err)
			}
			return CPU{}, FallbackReport{
				Requested: name,
				Used:      "cpu",
				FellBack:  true,
				Reason:    reason,
				Missing:   missing,
			}, nil
		}
		return b, FallbackReport{Requested: name, Used: b.Name()}, nil
	}

	if !allowFallback {
		return nil, FallbackReport{}, fmt.Errorf("backend %q not registered", name)
	}
	if err := Check(CPU{}, need); err != nil {
		return nil, FallbackReport{}, fmt.Errorf("cpu fallback unavailable for %q: %w", name, err)
	}
	return CPU{}, FallbackReport{
		Requested: name,
		Used:      "cpu",
		FellBack:  true,
		Reason:    "not registered",
	}, nil
}
