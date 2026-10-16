package learning_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// Read-only observers may run while an intervention is in flight. A refused
// operation has no completed state to publish, including when delays are
// temporarily permuted. Check both persisted state and the last report.
func TestInterveneNormSnapshotReaders(t *testing.T) {
	for _, core := range []string{"continuous", "lif", "mixed"} {
		for _, shuffle := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/shuffle%t", core, shuffle), func(t *testing.T) {
				a := interventionNormIndividual(t, core, "controller", 2, false, 0, 0)
				before, report := a.Snapshot(), a.ChemistryReport()
				plan := interventionNormPlan(learning.InterventionFixConcentration, 1.3e308, shuffle)
				ready, stop, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
				violations := make(chan string, 1)
				reads := 0
				go func() {
					defer close(done)
					read := func() {
						state, observed := a.Snapshot(), a.ChemistryReport()
						reads++
						if !reflect.DeepEqual(state, before) || !reflect.DeepEqual(observed, report) {
							select {
							case violations <- "reader observed a refused candidate":
							default:
							}
						}
					}
					read()
					close(ready)
					for {
						select {
						case <-stop:
							return
						default:
							read()
						}
					}
				}()
				<-ready
				for k := 0; k < 100; k++ {
					out, log, err := a.Intervene(context.Background(), plan, [][]float64{{0, 0, 0}})
					if err == nil || out != nil || !reflect.DeepEqual(log, learning.InterventionLog{}) {
						t.Errorf("numeric refusal %d returned out=%v log=%+v err=%v", k, out, log, err)
					}
				}
				close(stop)
				<-done
				if reads < 1 {
					t.Fatal("observer did not read")
				}
				select {
				case problem := <-violations:
					t.Fatal(problem)
				default:
				}
				if !reflect.DeepEqual(a.Snapshot(), before) || !reflect.DeepEqual(a.ChemistryReport(), report) {
					t.Fatal("refusal changed final state")
				}
			})
		}
	}
}
