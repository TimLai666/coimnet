package rl_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning/rl"
)

func TestStatefulRolloutClonePreservesHistoryAndIndependence(t *testing.T) {
	ind := newPPOIndividual(t, 109, .01)
	if _, err := ind.Advance(context.Background(), [][]float64{{.7, -.1, .3, .2}}); err != nil {
		t.Fatal(err)
	}
	r := rl.Rollout{InitialNeural: ind.Snapshot().Neural, Steps: []rl.Transition{{Obs: []float64{.1, .2, .3, .4}}}}
	if len(r.InitialNeural.Continuous.History) == 0 || r.InitialNeural.Continuous.History[0][0] == 0 {
		t.Fatal("fixture must contain actual saved history")
	}
	clone := cloneRolloutStateful(r)
	if !reflect.DeepEqual(r, clone) {
		t.Fatal("comparison fixture must preserve all rollout and saved-history values")
	}
	voltage := r.InitialNeural.Continuous.Voltage[0]
	history := r.InitialNeural.Continuous.History[0][0]
	observation := r.Steps[0].Obs[0]
	clone.InitialNeural.Continuous.Voltage[0] += 1
	clone.InitialNeural.Continuous.History[0][0] += 1
	clone.Steps[0].Obs[0] += 1
	if r.InitialNeural.Continuous.Voltage[0] != voltage || r.InitialNeural.Continuous.History[0][0] != history || r.Steps[0].Obs[0] != observation {
		t.Fatal("comparison fixture aliases the original rollout")
	}
}
