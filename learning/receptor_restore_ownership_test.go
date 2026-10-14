package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/TimLai666/coimnet/learning"
	"reflect"
	"strings"
	"testing"
)

func receptorOwnershipSeed(t *testing.T, profile, refs string) learning.IndividualSnapshot {
	t.Helper()
	s := compositionIndividual(t, profile, 0, true, .75, 0, 0).Snapshot()
	s.Plastic.Slow = nil
	s.Chemical.State.Concentration = [][]float64{{1}}
	other := s.Chemical.Config.Receptors.Records[0]
	other.Cells = append([]int(nil), other.Cells...)
	other.Signal = "fixture-alternate"
	other.Kd = 100
	s.Chemical.Config.Receptors.Records = append(s.Chemical.Config.Receptors.Records, other)
	idx := 0
	if refs == "gate" || refs == "both" {
		s.Plastic.Config.Rule.GateReceptor = &idx
		s.Plastic.Config.Rule.GateScale = 1
	}
	if refs == "window" || refs == "both" {
		s.Plastic.Config.Rule.DecayEReceptor = &idx
		s.Plastic.Config.Rule.DecayEBase = .5
		s.Plastic.Config.Rule.DecayESpan = .25
		s.Plastic.Config.Rule.DecayEMin = .25
		s.Plastic.Config.Rule.DecayEMax = .75
	}
	return s
}
func receptorOwnershipCopy(t *testing.T, s learning.IndividualSnapshot) learning.IndividualSnapshot {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var result learning.IndividualSnapshot
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestReceptorCopyMutationPreservesLearning(t *testing.T) {
	for _, profile := range []string{"continuous", "lif", "mixed"} {
		for _, refs := range []string{"gate", "window", "both"} {
			for _, boundary := range []string{"enable-input", "restore-input", "Snapshot"} {
				t.Run(profile+"/"+refs+"/"+boundary, func(t *testing.T) {
					seed := receptorOwnershipSeed(t, profile, refs)
					control, err := learning.RestoreIndividual(receptorOwnershipCopy(t, seed))
					if err != nil {
						t.Fatal(err)
					}
					var a *learning.Individual
					if boundary == "enable-input" {
						base := receptorOwnershipCopy(t, seed)
						base.Plastic = nil
						a, err = learning.RestoreIndividual(base)
						if err == nil {
							err = a.EnablePlasticity(seed.Plastic.Config)
						}
					} else {
						a, err = learning.RestoreIndividual(seed)
					}
					if err != nil {
						t.Fatal(err)
					}
					outside := seed
					if boundary == "Snapshot" {
						outside = a.Snapshot()
					}
					if outside.Plastic.Config.Rule.GateReceptor != nil {
						*outside.Plastic.Config.Rule.GateReceptor = 1
					}
					if outside.Plastic.Config.Rule.DecayEReceptor != nil {
						*outside.Plastic.Config.Rule.DecayEReceptor = 1
					}
					rows := [][]float64{{1.25}, {1.25}, {1.25}, {1.25}, {1.25}}
					gates := make([]float64, len(rows))
					if refs == "window" {
						for k := range gates {
							gates[k] = .3
						}
					}
					want, wr, err := control.AdvanceGated(context.Background(), rows, gates)
					if err != nil {
						t.Fatal(err)
					}
					got, gr, err := a.AdvanceGated(context.Background(), rows, gates)
					if err != nil {
						t.Fatal(err)
					}
					ws, gs := control.Snapshot(), a.Snapshot()
					if ws.Plastic.State.Eligibility[0] <= 0 || ws.Plastic.State.Plastic[0] <= 0 {
						t.Fatal("learning control produced no positive change")
					}
					if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gr, wr) || !reflect.DeepEqual(gs, ws) {
						t.Fatalf("external %s changed selected receptor or learning: got fast=%v eligibility=%v rule=%+v; want fast=%v eligibility=%v rule=%+v", boundary, gs.Plastic.State.Plastic, gs.Plastic.State.Eligibility, gs.Plastic.Config.Rule, ws.Plastic.State.Plastic, ws.Plastic.State.Eligibility, ws.Plastic.Config.Rule)
					}
				})
			}
		}
	}
}
func TestRestoreReceptorBindingMatchesEnable(t *testing.T) {
	for _, profile := range []string{"continuous", "lif", "mixed"} {
		for _, refs := range []string{"gate", "window", "both"} {
			for _, problem := range []string{"missing-chemical", "no-records", "out-of-range"} {
				t.Run(profile+"/"+refs+"/"+problem, func(t *testing.T) {
					seed := receptorOwnershipSeed(t, profile, refs)
					switch problem {
					case "missing-chemical":
						seed.Chemical = nil
					case "no-records":
						seed.Chemical.Config.Receptors.Records = nil
					case "out-of-range":
						if seed.Plastic.Config.Rule.GateReceptor != nil {
							*seed.Plastic.Config.Rule.GateReceptor = 2
						}
						if seed.Plastic.Config.Rule.DecayEReceptor != nil {
							*seed.Plastic.Config.Rule.DecayEReceptor = 2
						}
					}
					before, err := json.Marshal(seed)
					if err != nil {
						t.Fatal(err)
					}
					base := receptorOwnershipCopy(t, seed)
					base.Plastic = nil
					reference, err := learning.RestoreIndividual(base)
					if err != nil {
						t.Fatal(err)
					}
					enableErr := reference.EnablePlasticity(seed.Plastic.Config)
					if enableErr == nil {
						t.Fatal("Enable accepted the invalid receptor declaration")
					}
					got, restoreErr := learning.RestoreIndividual(seed)
					if restoreErr == nil || got != nil || !strings.Contains(restoreErr.Error(), enableErr.Error()) {
						t.Fatalf("Restore = %v, %v; want nil and Enable refusal %v", got != nil, restoreErr, enableErr)
					}
					after, err := json.Marshal(seed)
					if err != nil {
						t.Fatal(err)
					}
					if string(before) != string(after) {
						t.Fatal("refused Restore mutated input snapshot")
					}
				})
			}
		}
	}
}
func TestRestoreReceptorDefaultsRemainLegal(t *testing.T) {
	for _, profile := range []string{"continuous", "lif", "mixed"} {
		for _, chemical := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chemistry%v", profile, chemical), func(t *testing.T) {
				seed := receptorOwnershipSeed(t, profile, "none")
				if !chemical {
					seed.Chemical = nil
				}
				a, err := learning.RestoreIndividual(seed)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := a.AdvanceGated(context.Background(), [][]float64{{1.25}, {1.25}, {1.25}}, []float64{.3, .3, .3}); err != nil {
					t.Fatal(err)
				}
				got := a.Snapshot().Plastic.Config.Rule
				if got.GateReceptor != nil || got.DecayEReceptor != nil {
					t.Fatal("legacy absent receptor fields became declared")
				}
			})
		}
	}
}
