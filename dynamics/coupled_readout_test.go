package dynamics

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

type coupledReadoutTestSequence struct {
	mode     string
	cancel   context.CancelFunc
	reverses []*coupledReadoutTestReverse
}

type coupledReadoutTestReverse struct {
	sequence *coupledReadoutTestSequence
	observed [][]float64
	buffer   []float64
}

func (s *coupledReadoutTestSequence) Forward(context.Context, int, []float64) (*Modulation, error) {
	return &Modulation{}, nil
}

func (s *coupledReadoutTestSequence) NewReverse(context.Context, int, int) (ModulationReverse, error) {
	r := &coupledReadoutTestReverse{sequence: s}
	s.reverses = append(s.reverses, r)
	return r, nil
}

func (r *coupledReadoutTestReverse) BackwardStep(context.Context, int, ModulationGradient) ([]ActivityGradient, error) {
	return nil, nil
}

func (r *coupledReadoutTestReverse) BackwardReadout(_ context.Context, step int, up []float64) ([]float64, error) {
	r.observed = append(r.observed, append([]float64(nil), up...))
	switch r.sequence.mode {
	case "error":
		return nil, errors.New("readout reverse failed")
	case "shape":
		return []float64{0}, nil
	case "nonfinite":
		return []float64{math.NaN(), 0}, nil
	case "cancel":
		r.sequence.cancel()
	}
	if r.buffer == nil {
		r.buffer = make([]float64, len(up))
	}
	for i, value := range up {
		r.buffer[i] = float64(step+2) * value
	}
	// Exercise both callback input ownership and reusable output buffers.
	up[0] = 999
	return r.buffer, nil
}

func coupledReadoutTestFixture(t *testing.T) (*Continuous, Parameters, []float64, [][]float64, [][]float64) {
	t.Helper()
	m, err := NewContinuous(Config{Nodes: 2, Sources: []int{0, 1}, Targets: []int{1, 0}, Delays: []int{1, 0}, DT: .3, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := Parameters{Weights: []float64{.3, -.2}, Bias: []float64{.1, -.05}, LogTau: []float64{.1, -.2}}
	return m, p, []float64{.2, -.1}, [][]float64{{.4, -.2}, {-.3, .5}, {.2, -.4}}, [][]float64{{.31, -.27}, {-.22, .45}, {.37, -.19}}
}

func TestContinuousCoupledReadoutUsesDirectAdjointsAndFreshOwnedSessions(t *testing.T) {
	m, p, initial, inputs, up := coupledReadoutTestFixture(t)
	original := make([][]float64, len(up))
	scaled := make([][]float64, len(up))
	for step, row := range up {
		original[step] = append([]float64(nil), row...)
		scaled[step] = make([]float64, len(row))
		for i, value := range row {
			scaled[step][i] = float64(step+2) * value
		}
	}
	plain, err := m.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	want, err := m.Backward(context.Background(), plain, scaled, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := &coupledReadoutTestSequence{}
	trace, err := m.ForwardCoupled(context.Background(), p, initial, inputs, s)
	if err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		got, err := m.Backward(context.Background(), trace, up, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("readout reverse gradient=%#v, want independently transformed reference %#v", got, want)
		}
		if len(s.reverses) != pass+1 {
			t.Fatalf("reverse sessions=%d, want %d", len(s.reverses), pass+1)
		}
		if !reflect.DeepEqual(s.reverses[pass].observed, original) {
			t.Fatalf("callback received recurrent adjoints or wrong order: %v", s.reverses[pass].observed)
		}
		if !reflect.DeepEqual(up, original) {
			t.Fatal("callback mutated caller upstream")
		}
	}
	if s.reverses[0] == s.reverses[1] {
		t.Fatal("reverse sessions shared mutable readout state")
	}
}

func TestContinuousCoupledReadoutFailuresPublishZeroGradient(t *testing.T) {
	m, p, initial, inputs, up := coupledReadoutTestFixture(t)
	for _, mode := range []string{"shape", "nonfinite", "error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &coupledReadoutTestSequence{mode: mode, cancel: cancel}
			trace, err := m.ForwardCoupled(ctx, p, initial, inputs, s)
			if err != nil {
				t.Fatal(err)
			}
			before := trace.Outputs()
			got, err := m.Backward(ctx, trace, up, 0)
			if err == nil || !reflect.DeepEqual(got, Gradient{}) {
				t.Fatalf("gradient=%#v error=%v, want zero gradient and refusal", got, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error=%v", err)
			}
			if !reflect.DeepEqual(before, trace.Outputs()) {
				t.Fatal("readout reverse failure changed trace")
			}
		})
	}
}
