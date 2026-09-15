package learning_test

import (
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/params"
)

// dynamicsFourNodeRing is the canonical edge order of the ticket 13 fixture:
// (0,1) (0,2) (0,3) (1,2) (2,3) (3,0).
func dynamicsFourNodeRing() dynamics.Config {
	return dynamics.Config{
		Nodes:   4,
		Sources: []int{0, 0, 0, 1, 2, 3},
		Targets: []int{1, 2, 3, 2, 3, 0},
		DT:      .5, Activation: "tanh",
	}
}

// The ticket 13 fixture leaves the six edges with signs +1, -1, unknown, -1,
// unknown, unknown: one positive, two negative and three unknown.
var derivedFixtureSigns = []int8{1, -1, 0, -1, 0, 0}

// derivedSigns adapts a real *params.Set to the value learning reads. The
// learning package cannot import params (params imports connectome and
// connectome's internal tests import learning), so this external test package
// is where the two meet.
func derivedSigns(set *params.Set) learning.DerivedSigns {
	return learning.DerivedSigns{Source: set.Source, EdgeSigns: set.EdgeSign, Edges: set.Edges()}
}

// TestDerivedSetSourceMatchesTheParamsPackage pins the one string learning
// repeats instead of importing.
func TestDerivedSetSourceMatchesTheParamsPackage(t *testing.T) {
	if learning.DerivedSetSource != params.SetSource {
		t.Fatalf("learning.DerivedSetSource = %q, params.SetSource = %q", learning.DerivedSetSource, params.SetSource)
	}
}

func TestSignsFromParameterSetAppliesEveryUnknownPolicy(t *testing.T) {
	set := derivedFixtureSet(t)
	if !reflect.DeepEqual(set.EdgeSign, derivedFixtureSigns) {
		t.Fatalf("the derived fixture changed: %v, want %v", set.EdgeSign, derivedFixtureSigns)
	}
	for _, tc := range []struct {
		policy string
		want   []int8
	}{
		{learning.UnknownSignFree, []int8{1, -1, 0, -1, 0, 0}},
		{learning.UnknownSignExcitatory, []int8{1, -1, 1, -1, 1, 1}},
		{learning.UnknownSignInhibitory, []int8{1, -1, -1, -1, -1, -1}},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			signs, summary, err := learning.SignsFromParameterSet(derivedSigns(set), tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(signs, tc.want) {
				t.Fatalf("signs = %v, want %v", signs, tc.want)
			}
			want := learning.SignSummary{Policy: tc.policy, PositiveEdges: 1, NegativeEdges: 2, UnknownEdges: 3}
			if summary != want {
				t.Fatalf("summary = %+v, want %+v", summary, want)
			}
		})
	}
	// The three policy names match the ones simulate declares for the same
	// question, so a protocol and a trainer never disagree about wording.
	if learning.UnknownSignExcitatory != "excitatory" || learning.UnknownSignInhibitory != "inhibitory" || learning.UnknownSignFree != "free" {
		t.Fatalf("policy names changed: %q %q %q", learning.UnknownSignFree, learning.UnknownSignExcitatory, learning.UnknownSignInhibitory)
	}
}

func TestSignsFromParameterSetRejectsUndeclaredInput(t *testing.T) {
	set := derivedFixtureSet(t)
	if _, _, err := learning.SignsFromParameterSet(learning.DerivedSigns{}, learning.UnknownSignFree); err == nil {
		t.Fatal("accepted a set with no declared source")
	}
	for _, policy := range []string{"", "exclude", "average", "Excitatory"} {
		if _, _, err := learning.SignsFromParameterSet(derivedSigns(set), policy); err == nil {
			t.Fatalf("accepted policy %q", policy)
		}
	}
	// "exclude" is simulate's name for a zero weight; a trainer has no such
	// option, so it must be rejected rather than silently treated as free.
	bad := derivedSigns(set)
	bad.EdgeSigns = append([]int8(nil), set.EdgeSign...)
	bad.EdgeSigns[0] = 2
	if _, _, err := learning.SignsFromParameterSet(bad, learning.UnknownSignFree); err == nil {
		t.Fatal("accepted a sign outside -1, 0, +1")
	}
	short := derivedSigns(set)
	short.EdgeSigns = set.EdgeSign[:2]
	if _, _, err := learning.SignsFromParameterSet(short, learning.UnknownSignFree); err == nil {
		t.Fatal("accepted a sign array shorter than the declared edge count")
	}
	wrongSource := derivedSigns(set)
	wrongSource.Source = "something-else"
	if _, _, err := learning.SignsFromParameterSet(wrongSource, learning.UnknownSignFree); err == nil {
		t.Fatalf("accepted a set whose source is not %q", params.SetSource)
	}
}

// TestSignsFromParameterSetFeedsAConfig closes the loop the ticket asks for:
// the derived signs go straight into Config.EdgeSigns.
func TestSignsFromParameterSetFeedsAConfig(t *testing.T) {
	set := derivedFixtureSet(t)
	signs, summary, err := learning.SignsFromParameterSet(derivedSigns(set), learning.UnknownSignExcitatory)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("derived fixture: %+v", summary)
	c := learning.Config{
		Dynamics:  dynamicsFourNodeRing(),
		EdgeSigns: signs, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{3},
	}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	if got := n.Config().EdgeSigns; !reflect.DeepEqual(got, signs) {
		t.Fatalf("network kept %v, want %v", got, signs)
	}
	signs[0] = -1
	if n.Config().EdgeSigns[0] != 1 {
		t.Fatal("the network aliased the caller's sign array")
	}
}
