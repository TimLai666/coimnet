package dynamics_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func TestForwardFromStateExtremeCountersAndDelays(t *testing.T) {
	for _, delay := range []int{0, math.MaxInt} {
		m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{delay}, DT: 1, Activation: "tanh"})
		if err != nil {
			t.Fatal(err)
		}
		p := dynamics.Parameters{Weights: []float64{.2}, Bias: []float64{.1}, LogTau: []float64{0}}
		s, err := m.NewState([]float64{.3})
		if err != nil {
			t.Fatal(err)
		}
		if delay == 0 {
			s.Steps = math.MaxUint64 - 1
		}
		inputs := [][]float64{{.4}}
		tr, err := m.ForwardFromState(context.Background(), p, s, inputs)
		if err != nil {
			t.Fatalf("delay %d: %v", delay, err)
		}
		_, want, err := m.Advance(context.Background(), p, s, inputs)
		if err != nil || !reflect.DeepEqual(tr.Outputs(), want) {
			t.Fatalf("delay %d: forward/advance differ: %v", delay, err)
		}
		if delay == 0 {
			tr, err = m.ForwardFromState(context.Background(), p, s, [][]float64{{.4}, {.4}})
			if tr != nil || err == nil || !strings.Contains(err.Error(), "counter overflow") {
				t.Fatalf("step overflow: trace=%v err=%v", tr, err)
			}
		}
	}
}

func TestForwardFromStateCancellationAndFutureHistoryCapacity(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{Nodes: dynamics.MaxStateValues / 2, Sources: []int{0}, Targets: []int{0}, Delays: []int{2}, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	row := make([]float64, m.Config().Nodes)
	s, err := m.NewState(row)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr, err := m.ForwardFromState(ctx, dynamics.Parameters{}, s, [][]float64{row})
	if tr != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: trace=%v err=%v", tr, err)
	}
	// The input and output matrices fit the existing per-matrix limit, but
	// the final retained history would require three rows and must be refused.
	p := dynamics.Parameters{Weights: []float64{.2}, Bias: row, LogTau: row}
	tr, err = m.ForwardFromState(context.Background(), p, s, [][]float64{row, row})
	if tr != nil || err == nil || !strings.Contains(err.Error(), "history") {
		t.Fatalf("future history capacity: trace=%v err=%v", tr, err)
	}
}
