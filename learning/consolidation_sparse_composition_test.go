package learning_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestConsolidateCompositionUsesSparseCoreEdgeIndex(t *testing.T) {
	for _, mode := range []struct {
		sign     int8
		negative bool
	}{{0, false}, {0, true}, {1, false}, {-1, false}} {
		t.Run(fmt.Sprintf("sign%d/negative%v", mode.sign, mode.negative), func(t *testing.T) {
			seed := compositionIndividual(t, "continuous", mode.sign, true, 1e308, -1e308, 1e308).Snapshot()
			c, p := seed.Config, seed.Parameters
			c.Dynamics.Sources, c.Dynamics.Targets, c.Dynamics.Delays = []int{0, 0, 0}, []int{1, 1, 1}, []int{0, 0, 0}
			c.EdgeSigns = []int8{0, 0, mode.sign}
			p.Core.Weights = []float64{.125, .125, p.Core.Weights[0]}
			if mode.negative {
				p.Core.Weights[2] = -1e308
			}
			a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, 2))
			if err != nil {
				t.Fatal(err)
			}
			pc := seed.Plastic.Config
			pc.Edges = []int{2}
			if err := a.EnablePlasticity(pc); err != nil {
				t.Fatal(err)
			}
			if err := a.EnableConsolidation(1); err != nil {
				t.Fatal(err)
			}
			if err := a.EnableChemistry(seed.Chemical.Config); err != nil {
				t.Fatal(err)
			}
			s := a.Snapshot()
			s.Plastic.Slow.Values[0], s.Plastic.State.Plastic[0] = -1e308, 1e308
			if mode.negative {
				s.Plastic.Slow.Values[0], s.Plastic.State.Plastic[0] = 1e308, -1e308
			}
			a, err = learning.RestoreIndividual(s)
			if err != nil {
				t.Fatal(err)
			}
			before := a.Snapshot()
			control, err := learning.RestoreIndividual(before)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := control.Advance(context.Background(), [][]float64{{0}}); err != nil {
				t.Fatalf("pre-state: %v", err)
			}
			report, err := a.Consolidate(context.Background(), learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: 1, Retain: 1})
			if err == nil || report.Applied {
				t.Fatalf("sparse edge 2 overflow was accepted: %+v, %v", report, err)
			}
			if !reflect.DeepEqual(before, a.Snapshot()) {
				t.Fatal("sparse overflow consumed state or budget")
			}
		})
	}
}
