package learning_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func TestStepFromStateRejectsCandidateThatOnlyRunsFromZero(t *testing.T) {
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{0}, DT: 1, Activation: "softplus"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{0}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{0}, Bias: []float64{0}, LogTau: []float64{0}}, Encoder: []float64{1}, Readout: []float64{1}}
	o := learning.DefaultOptions()
	o.LearningRate = 1e10
	o.Beta1, o.Beta2 = 0, 0
	o.ClipNorm = 1
	o.Trainable = learning.Trainable{Weights: true}
	ind, err := learning.NewIndividual(c, p, o, []float64{1e36})
	if err != nil {
		t.Fatal(err)
	}
	initial := ind.Snapshot().Neural
	input, upstream := [][]float64{{0}}, [][]float64{{-1}}
	for _, accumulate := range []int{1, 2} {
		t.Run(map[int]string{1: "immediate", 2: "open_accumulation"}[accumulate], func(t *testing.T) {
			options := o
			options.AccumulateSteps = accumulate
			tr, err := learning.NewTrainer(c, p, options)
			if err != nil {
				t.Fatal(err)
			}
			if accumulate == 2 {
				result, err := tr.StepFromState(context.Background(), initial, input, upstream)
				if err != nil || result.Applied || result.Accumulated != 1 {
					t.Fatalf("first accumulation=%+v err=%v", result, err)
				}
			}
			before := tr.Snapshot()
			saved := ind.Snapshot()
			result, err := tr.StepFromState(context.Background(), initial, input, upstream)
			if err == nil || !strings.Contains(err.Error(), "candidate update rejected") {
				t.Fatalf("candidate result=%+v err=%v; must reject saved-state replay overflow before commit", result, err)
			}
			if !reflect.DeepEqual(result, learning.StepResult{}) {
				t.Fatalf("published result on error: %+v", result)
			}
			if !reflect.DeepEqual(before, tr.Snapshot()) {
				t.Fatal("rejected candidate changed parameters, optimizer, count or open accumulator")
			}
			if !reflect.DeepEqual(saved, ind.Snapshot()) {
				t.Fatal("saved state was modified")
			}
			zero, err := learning.NewTrainer(c, p, o)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := zero.StepFrom(context.Background(), input, upstream)
			if err != nil || !accepted.Applied {
				t.Fatalf("legacy fresh-zero candidate should remain executable: %+v err=%v", accepted, err)
			}
		})
	}
}
