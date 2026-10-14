package plasticity_test

import (
	"fmt"
	"github.com/TimLai666/coimnet/plasticity"
	"reflect"
	"testing"
)

func ruleOwnershipFixture(kind, refs string) plasticity.Config {
	idx := 0
	r := plasticity.Rule{Kind: kind, DecayE: .5, DecayP: .5, PlasticMax: 10, WMin: .0625}
	if kind == plasticity.RuleSTDPPair {
		r.DecayPre = .5
		r.DecayPost = .25
		r.APlus = 1
		r.AMinus = .5
	}
	if refs == "gate" || refs == "both-shared" {
		r.GateReceptor = &idx
		r.GateScale = 1
	}
	if refs == "window" || refs == "both-shared" {
		r.DecayEReceptor = &idx
		r.DecayEBase = .5
		r.DecayESpan = .25
		r.DecayEMin = .25
		r.DecayEMax = .75
	}
	return plasticity.Config{Rule: r, Edges: []int{0}}
}
func TestRuleReceptorPointersAreIndependent(t *testing.T) {
	for _, kind := range []string{plasticity.RuleHebbianRate, plasticity.RuleSTDPPair} {
		for _, refs := range []string{"gate", "window", "both-shared"} {
			for _, boundary := range []string{"input", "Config"} {
				t.Run(fmt.Sprintf("%s/%s/%s", kind, refs, boundary), func(t *testing.T) {
					input := ruleOwnershipFixture(kind, refs)
					m, err := plasticity.New(input, 1)
					if err != nil {
						t.Fatal(err)
					}
					original := ruleOwnershipFixture(kind, refs)
					copy := input
					if boundary == "Config" {
						copy = m.Config()
					}
					if copy.Rule.GateReceptor != nil {
						*copy.Rule.GateReceptor = 7
					}
					if copy.Rule.DecayEReceptor != nil {
						*copy.Rule.DecayEReceptor = 7
					}
					if got := m.Config(); !reflect.DeepEqual(got, original) {
						t.Fatalf("external %s changed immutable rule: got %+v, want %+v", boundary, got.Rule, original.Rule)
					}
				})
			}
		}
	}
}
func TestRuleReceptorNilFieldsStayAbsent(t *testing.T) {
	for _, kind := range []string{plasticity.RuleHebbianRate, plasticity.RuleSTDPPair} {
		input := ruleOwnershipFixture(kind, "none")
		m, err := plasticity.New(input, 1)
		if err != nil {
			t.Fatal(err)
		}
		output := m.Config()
		if output.Rule.GateReceptor != nil || output.Rule.DecayEReceptor != nil || !reflect.DeepEqual(output, input) {
			t.Fatalf("absent receptor fields changed: %+v", output)
		}
	}
	var absent *plasticity.Model
	if got := absent.Config(); !reflect.DeepEqual(got, plasticity.Config{}) {
		t.Fatalf("nil model Config = %+v", got)
	}
}
