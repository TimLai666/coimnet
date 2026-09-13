package dynamics_test

import (
	"context"
	"github.com/TimLai666/coimnet/dynamics"
	"math"
	"testing"
)

func TestSmallTimeStepRetainsDriveAndGradient(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1e-18, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}
	tr, err := m.Forward(context.Background(), p, []float64{0}, [][]float64{{1e18}})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(tr.FinalVoltage()[0]-1) > 1e-15 {
		t.Fatalf("small dt lost drive: %v", tr.FinalVoltage())
	}
	g, err := m.Backward(context.Background(), tr, [][]float64{{1}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := 1 - math.Pow(math.Tanh(1), 2)
	if math.Abs(g.Inputs[0][0]/1e-18-want) > 1e-14 || math.Abs(g.LogTau[0]+want) > 1e-14 {
		t.Fatalf("incorrect derivatives: %+v", g)
	}
}
