package learning_test

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// TestReusedNetworkSeesParameterChanges pins that a network reused across
// calls answers exactly like a fresh one, including after the caller changes
// the encoder or readout slice in place, and that a value float32 cannot hold
// is still refused after a valid call with the same slice.
func TestReusedNetworkSeesParameterChanges(t *testing.T) {
	ctx, input := context.Background(), everyStepInput()
	reused := everyStepNetwork(t, true)
	p := everyStepParameters()
	check := func(step string) {
		t.Helper()
		got, err := reused.PredictAll(ctx, p, input)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		want, err := everyStepNetwork(t, true).PredictAll(ctx, p, input)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			for j := range want[i] {
				if math.Float64bits(got[i][j]) != math.Float64bits(want[i][j]) {
					t.Fatalf("%s: row %d col %d = %v, fresh network gives %v", step, i, j, got[i][j], want[i][j])
				}
			}
		}
	}
	check("first call")
	check("same parameters")
	p.Encoder[1] = -.7
	check("encoder changed in place")
	p.Readout[2] = 1.5
	check("readout changed in place")
	p.Encoder[0] = math.Copysign(0, -1)
	check("encoder sign of zero changed")
	p.Readout[0] = math.MaxFloat64
	if _, err := reused.PredictAll(ctx, p, input); err == nil || !strings.Contains(err.Error(), "float32") {
		t.Fatalf("readout beyond float32 accepted: %v", err)
	}
}

// TestReusedNetworkConcurrentPredict runs one network from several goroutines
// with two parameter sets, so the race detector sees the shared tensors.
func TestReusedNetworkConcurrentPredict(t *testing.T) {
	ctx, input := context.Background(), everyStepInput()
	n := everyStepNetwork(t, true)
	a, b := everyStepParameters(), everyStepParameters()
	b.Encoder[2], b.Readout[3] = .9, -.2
	want := map[int][][]float64{}
	for i, p := range []learning.Parameters{a, b} {
		out, err := everyStepNetwork(t, true).PredictAll(ctx, p, input)
		if err != nil {
			t.Fatal(err)
		}
		want[i] = out
	}
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < 20; k++ {
				which := (g + k) % 2
				p := a
				if which == 1 {
					p = b
				}
				out, err := n.PredictAll(ctx, p, input)
				if err != nil {
					errs <- err.Error()
					return
				}
				for i := range out {
					for j := range out[i] {
						if out[i][j] != want[which][i][j] {
							errs <- "prediction differs from a fresh network"
							return
						}
					}
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}
