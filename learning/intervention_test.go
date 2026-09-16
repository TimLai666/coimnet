package learning_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// nodeIntervention builds a node-kind item (clamp_voltage, force_spike or
// silence) with a finite value and a nonempty interval.
func nodeIntervention(kind string, targets []int, value float64, start, end uint64) learning.Intervention {
	return learning.Intervention{Kind: kind, Targets: targets, Value: value, Start: start, End: end}
}

func channelIntervention(kind string, channel int, value float64, start, end uint64) learning.Intervention {
	return learning.Intervention{Kind: kind, Channel: channel, Value: value, Start: start, End: end}
}

func swapIntervention(a, b int, start, end uint64) learning.Intervention {
	return learning.Intervention{Kind: learning.InterventionSwapRegions, Regions: [2]int{a, b}, Start: start, End: end}
}

func shuffleIntervention(targets []int, start, end uint64) learning.Intervention {
	return learning.Intervention{Kind: learning.InterventionShuffleDelays, Targets: targets, Start: start, End: end}
}

// authorizedPlan is the minimal plan every per-rule failure case starts from.
func authorizedPlan(items ...learning.Intervention) learning.InterventionPlan {
	return learning.InterventionPlan{Authorized: true, Reason: "fixture: approved", Items: items}
}

// planShape is the shape the table starts from unless a case overrides it.
func planShape() learning.InterventionShape {
	return learning.InterventionShape{Nodes: 2, Spiking: true, Channels: 1, Regions: 2, MaxDelay: 1, Edges: 1}
}

func TestInterventionPlanValidate(t *testing.T) {
	cases := []struct {
		name          string
		plan          learning.InterventionPlan
		shape         learning.InterventionShape
		wantOK        bool
		notAuthorized bool
		contains      []string
	}{
		// Rule 1: the plan is an experiment declaration and must say so.
		{"rule1 not authorized", learning.InterventionPlan{Authorized: false, Reason: "approved", Items: []learning.Intervention{nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10)}}, planShape(), false, true, nil},
		{"rule1 blank reason", learning.InterventionPlan{Authorized: true, Reason: "   ", Items: []learning.Intervention{nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10)}}, planShape(), false, true, nil},
		{"rule1 ok authorized with reason", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10)), planShape(), true, false, nil},

		// Rule 2: items exist and every kind is one of the eight.
		{"rule2 empty items", authorizedPlan(), planShape(), false, false, []string{"items"}},
		{"rule2 unknown kind", authorizedPlan(learning.Intervention{Kind: "ablate", Start: 0, End: 10}), planShape(), false, false, []string{"items[0]", "kind"}},
		{"rule2 ok all eight kinds", authorizedPlan(
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 1),
			nodeIntervention(learning.InterventionForceSpike, []int{1}, 0, 1, 2),
			nodeIntervention(learning.InterventionSilence, []int{0}, 0, 2, 3),
			channelIntervention(learning.InterventionBlockChannel, 0, 0, 3, 4),
			channelIntervention(learning.InterventionFixConcentration, 0, 0, 4, 5),
			channelIntervention(learning.InterventionRemoveChannel, 0, 0, 5, 6),
			shuffleIntervention(nil, 6, 7),
			swapIntervention(0, 1, 7, 8),
		), planShape(), true, false, nil},

		// Rule 3: a nonempty interval and a finite value.
		{"rule3 start equals end", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 10, 10)), planShape(), false, false, []string{"items[0]", "start"}},
		{"rule3 start after end", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 10, 5)), planShape(), false, false, []string{"items[0]", "start"}},
		{"rule3 nan value", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, math.NaN(), 0, 10)), planShape(), false, false, []string{"items[0]", "value"}},
		{"rule3 infinite value", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, math.Inf(1), 0, 10)), planShape(), false, false, []string{"items[0]", "value"}},
		{"rule3 ok interval and finite value", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, 1.5, 0, 10)), planShape(), true, false, nil},

		// Rule 4: node kinds name ascending in-range targets; force_spike needs
		// a spiking core.
		{"rule4 empty targets", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, nil, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule4 duplicate targets", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0, 0}, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule4 descending targets", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{1, 0}, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule4 out of range target", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{2}, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule4 negative target", authorizedPlan(nodeIntervention(learning.InterventionSilence, []int{-1}, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule4 force_spike without spiking core", authorizedPlan(nodeIntervention(learning.InterventionForceSpike, []int{0}, 0, 0, 10)), learning.InterventionShape{Nodes: 2, Spiking: false, Channels: 1, Regions: 2, MaxDelay: 1, Edges: 1}, false, false, []string{"items[0]", "spiking"}},
		{"rule4 ok a node target", authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{1}, 0, 0, 10)), planShape(), true, false, nil},
		{"rule4 ok silence in range", authorizedPlan(nodeIntervention(learning.InterventionSilence, []int{0, 1}, 0, 0, 10)), planShape(), true, false, nil},
		{"rule4 ok force_spike on spiking core", authorizedPlan(nodeIntervention(learning.InterventionForceSpike, []int{0}, 0, 0, 10)), planShape(), true, false, nil},

		// Rule 5: channel kinds need enabled chemistry and an in-range
		// channel; fix_concentration needs a non-negative value.
		{"rule5 no chemistry", authorizedPlan(channelIntervention(learning.InterventionBlockChannel, 0, 0, 0, 10)), learning.InterventionShape{Nodes: 2, Spiking: true, Channels: 0, Regions: 0, MaxDelay: 1, Edges: 1}, false, false, []string{"items[0]", "channel"}},
		{"rule5 out of range channel", authorizedPlan(channelIntervention(learning.InterventionBlockChannel, 1, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "channel"}},
		{"rule5 negative channel", authorizedPlan(channelIntervention(learning.InterventionFixConcentration, -1, 0, 0, 10)), planShape(), false, false, []string{"items[0]", "channel"}},
		{"rule5 negative fixed value", authorizedPlan(channelIntervention(learning.InterventionFixConcentration, 0, -1, 0, 10)), planShape(), false, false, []string{"items[0]", "value"}},
		{"rule5 ok block channel", authorizedPlan(channelIntervention(learning.InterventionBlockChannel, 0, 0, 0, 10)), planShape(), true, false, nil},
		{"rule5 ok fix concentration at zero", authorizedPlan(channelIntervention(learning.InterventionFixConcentration, 0, 0, 0, 10)), planShape(), true, false, nil},
		{"rule5 ok remove channel", authorizedPlan(channelIntervention(learning.InterventionRemoveChannel, 0, 0, 0, 10)), planShape(), true, false, nil},

		// Rule 6: swap_regions needs two distinct in-range regions under at
		// least two declared regions.
		{"rule6 fewer than two regions", authorizedPlan(swapIntervention(0, 1, 0, 10)), learning.InterventionShape{Nodes: 2, Spiking: true, Channels: 1, Regions: 1, MaxDelay: 1, Edges: 1}, false, false, []string{"items[0]", "regions"}},
		{"rule6 same region", authorizedPlan(swapIntervention(1, 1, 0, 10)), planShape(), false, false, []string{"items[0]", "regions"}},
		{"rule6 out of range region", authorizedPlan(swapIntervention(0, 2, 0, 10)), planShape(), false, false, []string{"items[0]", "regions"}},
		{"rule6 ok distinct regions", authorizedPlan(swapIntervention(1, 0, 0, 10)), planShape(), true, false, nil},

		// Rule 7: shuffle_delays needs a positive delay to shuffle and, when
		// targets are declared, ascending in-range edge indices.
		{"rule7 nothing to shuffle", authorizedPlan(shuffleIntervention(nil, 0, 10)), learning.InterventionShape{Nodes: 2, Spiking: true, Channels: 1, Regions: 2, MaxDelay: 0, Edges: 1}, false, false, []string{"items[0]", "max_delay"}},
		{"rule7 out of range edge", authorizedPlan(shuffleIntervention([]int{1}, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule7 duplicate edges", authorizedPlan(shuffleIntervention([]int{0, 0}, 0, 10)), planShape(), false, false, []string{"items[0]", "targets"}},
		{"rule7 ok no target edges", authorizedPlan(shuffleIntervention(nil, 0, 10)), planShape(), true, false, nil},
		{"rule7 ok one edge target", authorizedPlan(shuffleIntervention([]int{0}, 0, 10)), planShape(), true, false, nil},

		// Rule 8: two items of the same kind must not overlap in time on the
		// same target.
		{"rule8 overlapping clamps on one node", authorizedPlan(
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10),
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 1, 5, 15),
		), planShape(), false, false, []string{"items[0]", "items[1]", "overlap"}},
		{"rule8 overlapping silence sharing a target", authorizedPlan(
			nodeIntervention(learning.InterventionSilence, []int{0, 1}, 0, 0, 10),
			nodeIntervention(learning.InterventionSilence, []int{0, 1}, 0, 5, 15),
		), planShape(), false, false, []string{"items[0]", "items[1]", "overlap"}},
		{"rule8 overlapping channel kinds on one channel", authorizedPlan(
			channelIntervention(learning.InterventionBlockChannel, 0, 0, 0, 10),
			channelIntervention(learning.InterventionFixConcentration, 0, 1, 5, 15),
		), planShape(), false, false, []string{"items[0]", "items[1]", "overlap"}},
		{"rule8 overlapping shuffles over all edges", authorizedPlan(
			shuffleIntervention(nil, 0, 10),
			shuffleIntervention(nil, 5, 15),
		), planShape(), false, false, []string{"items[0]", "items[1]", "overlap"}},
		{"rule8 overlapping swaps sharing a region", authorizedPlan(
			swapIntervention(0, 1, 0, 10),
			swapIntervention(1, 0, 5, 15),
		), planShape(), false, false, []string{"items[0]", "items[1]", "overlap"}},
		{"rule8 ok overlapping clamps on different nodes", authorizedPlan(
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10),
			nodeIntervention(learning.InterventionClampVoltage, []int{1}, 1, 5, 15),
		), planShape(), true, false, nil},
		{"rule8 ok separable intervals on one node", authorizedPlan(
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10),
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 1, 10, 20),
		), planShape(), true, false, nil},
		{"rule8 ok different kinds may overlap", authorizedPlan(
			nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 10),
			channelIntervention(learning.InterventionBlockChannel, 0, 0, 0, 10),
		), planShape(), true, false, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.plan.Validate(tc.shape)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Validate() error = nil, want a refusal")
			}
			if tc.notAuthorized {
				if !errors.Is(err, learning.ErrInterventionNotAuthorized) {
					t.Fatalf("Validate() error = %v, want ErrInterventionNotAuthorized via errors.Is", err)
				}
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("Validate() error = %q, want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestInterventionShapeFromIndividual derives the shape from the same fixtures
// the chemical and plastic tests run: two continuous neurons (chemistry off and
// on), the two-neuron LIF core, and a LIF core with one positive-delay edge.
func TestInterventionShapeFromIndividual(t *testing.T) {
	plain := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	shape := plain.InterventionShape()
	if shape.Nodes != 2 || shape.Spiking || shape.Channels != 0 || shape.Regions != 0 || shape.MaxDelay != 0 || shape.Edges != 0 {
		t.Fatalf("continuous without chemistry: shape = %+v", shape)
	}

	chem := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	decl := sensitivityDeclaration()
	decl.Chemistry.Regions = 1
	decl.Regions.NodeRegion = []int{0, 0}
	enableChemistry(t, chem, decl)
	shape = chem.InterventionShape()
	if shape.Nodes != 2 || shape.Spiking || shape.Channels != 1 || shape.Regions != 1 || shape.MaxDelay != 0 || shape.Edges != 0 {
		t.Fatalf("continuous with chemistry: shape = %+v", shape)
	}

	lif := newIndividual(t, chemLIFConfig(), chemLIFParameters())
	shape = lif.InterventionShape()
	if !shape.Spiking || shape.Nodes != 2 || shape.Channels != 0 || shape.Regions != 0 || shape.Edges != 0 || shape.MaxDelay != 0 {
		t.Fatalf("LIF without chemistry: shape = %+v", shape)
	}

	core := plasticLIFCore()
	core.Delays = []int{2}
	delayedLIF := learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0}}
	maxDelay := newIndividual(t, delayedLIF, plasticLIFParameters())
	shape = maxDelay.InterventionShape()
	if shape.Nodes != 2 || !shape.Spiking || shape.Channels != 0 || shape.Regions != 0 || shape.Edges != 1 || shape.MaxDelay != 2 {
		t.Fatalf("LIF with one delayed edge: shape = %+v", shape)
	}
}
