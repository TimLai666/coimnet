package learning_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// The tolerated deviations: a delta is either computed from float64 state for
// the voltage and the plastic state, or from a readout that crossed Insyra's
// float32 boundary. Hand constants for the two cases are asserted with the
// matching tolerance.
const (
	boundaryTol = 1e-6
	stateTol    = 1e-12
)

// Method expressions pin the public entry points to the exact signatures the
// learning runner exposes, so a refactor that changes one of them fails here
// instead of silently breaking a caller.
var (
	_ func(*learning.Individual, context.Context, [][]float64) ([][]float64, error)                                                      = (*learning.Individual).Advance
	_ func(*learning.Individual, context.Context, [][]float64, []float64) ([][]float64, learning.PlasticReport, error)                   = (*learning.Individual).AdvanceGated
	_ func(*learning.Individual, context.Context, learning.InterventionPlan, [][]float64) ([][]float64, learning.InterventionLog, error) = (*learning.Individual).Intervene
	_ func(*learning.Individual) learning.InterventionShape                                                                              = (*learning.Individual).InterventionShape
)

func closeRows(got, want [][]float64, tol float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if math.Abs(got[i][j]-want[i][j]) > tol {
				return false
			}
		}
	}
	return true
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestInterveneClampVoltageHoldsThenReleases runs 4 rows of constant input on
// the two isolated tanh nodes and clamps node 0 to a fixed 0.25 on rows 1 and
// 2 only. tau = 1 and dt = 1 give lambda = exp(-1) and alpha = 1-exp(-1), so
// the free evolution is closed form:
//
//	row 0  cand alpha              out tanh(alpha)
//	row 1  cand clamped to 0.25    out tanh(0.25)
//	row 2  cand clamped to 0.25    out tanh(0.25)
//	row 3  cand lambda*0.25+alpha  out tanh(lambda*0.25+alpha)
//
// and a later ordinary advance continues from the released voltage
// lambda*0.25+alpha, proving the clamp did not leak into the next call.
func TestInterveneClampVoltageHoldsThenReleases(t *testing.T) {
	a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	input := [][]float64{{1, 0}, {1, 0}, {1, 0}, {1, 0}}
	plan := authorizedPlan(nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0.25, 1, 3))
	out, log, err := a.Intervene(context.Background(), plan, input)
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	wantOut := [][]float64{{0.5595106570525964}, {0.24491866240370913}, {0.24491866240370913}, {0.6194366106524835}}
	if !closeRows(out, wantOut, boundaryTol) {
		t.Fatalf("clamped output:\n got %v\nwant %v", out, wantOut)
	}
	if log.Reason != "fixture: approved" || len(log.Entries) != 1 {
		t.Fatalf("log = %+v, want one entry carrying the reason", log)
	}
	e := log.Entries[0]
	if e.Kind != learning.InterventionClampVoltage || !equalInts(e.Targets, []int{0}) || e.Start != 1 || e.End != 3 || e.AppliedRows != 2 {
		t.Fatalf("entry = %+v, want clamp row window [1,3) applied on 2 rows", e)
	}
	// The activity delta is the L2 distance to the un-clamped twin through the
	// float32 readout boundary; the twin rows are tanh(alpha), tanh(alpha+0.8647),
	// its image, and its image.
	if math.Abs(e.ActivityDelta-0.684772379365581) > boundaryTol {
		t.Fatalf("ActivityDelta = %.12g, want %.12g", e.ActivityDelta, 0.684772379365581)
	}
	if e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("a clamp cannot move plastic or base state: %+v", e)
	}
	// Released: the next ordinary advance continues from the free voltage
	// lambda*0.25+alpha, giving out = tanh(0.8984985375725405).
	next := advance(t, a, [][]float64{{1, 0}})
	wantNext := [][]float64{{0.7155659954988624}}
	if !closeRows(next, wantNext, boundaryTol) {
		t.Fatalf("released continuation:\n got %v\nwant %v", next, wantNext)
	}
	v := voltageOf(t, a)
	if !closeEnough(v[0], 0.8984985375725405) {
		t.Fatalf("released voltage = %.17g, want %.17g", v[0], 0.8984985375725405)
	}
	// The record survives a JSON round trip with its deltas intact.
	raw, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip learning.InterventionLog
	if err = json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundtrip, log) {
		t.Fatalf("log round trip changed the record:\n got %+v\nwant %+v", roundtrip, log)
	}
}

// TestInterveneSilenceZeroesOutput silences node 1 (the readout) on both rows
// of a two node continuous core whose edge 1->0 carries a trace into node 0.
// The silenced node contributes a zero output and zero persisted trace, so
// node 0's second row sees no synaptic drive and its voltage is exactly
// lambda*alpha, while the readout stays 0 on every row.
func TestInterveneSilenceZeroesOutput(t *testing.T) {
	cfg := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{1}, Targets: []int{0}, Delays: []int{0}, DT: 1, Activation: "tanh"},
		InputSize:    2,
		OutputSize:   1,
		ReadoutNodes: []int{1},
		InputNodes:   []int{0, 1},
	}
	params := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{0.5}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1},
		Readout: []float64{1},
	}
	a := newIndividual(t, cfg, params)
	plan := authorizedPlan(nodeIntervention(learning.InterventionSilence, []int{1}, 0, 0, 2))
	out, log, err := a.Intervene(context.Background(), plan, [][]float64{{1, 0.9}, {0, 0.9}})
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	if want := [][]float64{{0}, {0}}; !closeRows(out, want, stateTol) {
		t.Fatalf("silenced readout:\n got %v\nwant %v", out, want)
	}
	// Node 0 is a free neuron: row 0 integrates its own input of 1 to alpha,
	// and row 1 integrates no drive at all, so its voltage is lambda*alpha.
	v := voltageOf(t, a)
	if !closeEnough(v[0], 0.23254415793482963) {
		t.Fatalf("node 0 voltage = %.17g, want %.17g", v[0], 0.23254415793482963)
	}
	s := a.Snapshot()
	last := s.Neural.Continuous.History[len(s.Neural.Continuous.History)-1]
	if last[1] != 0 {
		t.Fatalf("silenced node 1 persisted a trace of %v", last[1])
	}
	e := log.Entries[0]
	if e.AppliedRows != 2 {
		t.Fatalf("entry = %+v, want silence applied on 2 rows", e)
	}
	// The twin readout runs free: tanh(alpha*0.9) and tanh(lambda*(alpha*0.9)+alpha*0.9).
	if math.Abs(e.ActivityDelta-0.830328023) > boundaryTol {
		t.Fatalf("ActivityDelta = %.12g, want %.12g", e.ActivityDelta, 0.830328023)
	}
	if e.PlasticDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("silence cannot move plastic or base state: %+v", e)
	}
}

// TestInterveneKeepsBaseParameters forces node 0 to spike on both rows of a
// plastic LIF run with no external input. The eligibility steps through the
// forced trace, so the intervention visibly moves the fast plastic state while
// the base parameters, which Intervene must not touch, stay bit identical.
//
//	row 0: forced spike gives node 0 trace 1, node 1 silent
//	row 1: node 1 sees the trace 0.9*1 above its threshold and fires, and the
//	       forced spike adds one more unit to node 0's trace, so eligibility
//	       reaches pre*post = (exp(-1)+1)*1 on the only edge.
func TestInterveneKeepsBaseParameters(t *testing.T) {
	a := newIndividual(t, plasticLIFConfig(), plasticLIFParameters())
	if err := a.EnablePlasticity(plasticConfig(0)); err != nil {
		t.Fatal(err)
	}
	plan := authorizedPlan(nodeIntervention(learning.InterventionForceSpike, []int{0}, 0, 0, 2))
	out, log, err := a.Intervene(context.Background(), plan, [][]float64{{0}, {0}})
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	wantOut := [][]float64{{0}, {1}}
	if !closeRows(out, wantOut, boundaryTol) {
		t.Fatalf("forced spike output:\n got %v\nwant %v", out, wantOut)
	}
	s := a.Snapshot()
	if s.Plastic == nil {
		t.Fatal("the snapshot lost the plastic part")
	}
	elig := s.Plastic.State.Eligibility
	if len(elig) != 1 || !closeEnough(elig[0], 1.3678794411714423) {
		t.Fatalf("eligibility = %v, want [1.3678794411714423]", elig)
	}
	if plastic := s.Plastic.State.Plastic; len(plastic) != 1 || plastic[0] != 0 {
		t.Fatalf("plastic value = %v, want [0] under a closed gate", plastic)
	}
	// The snapshot parameters are byte identical to the fixture: Intervene does
	// not train, it disturbs activity only.
	if !reflect.DeepEqual(s.Parameters, plasticLIFParameters()) {
		t.Fatalf("base parameters moved:\n got %+v\nwant %+v", s.Parameters, plasticLIFParameters())
	}
	e := log.Entries[0]
	if e.AppliedRows != 2 {
		t.Fatalf("entry = %+v, want force_spike applied on 2 rows", e)
	}
	// The twin fired nothing, so the readout moved by exactly one unit event
	// and the eligibility moved by the exact hand value.
	if math.Abs(e.ActivityDelta-1.0) > stateTol {
		t.Fatalf("ActivityDelta = %.12g, want 1", e.ActivityDelta)
	}
	if !closeEnough(e.PlasticDelta, 1.3678794411714423) {
		t.Fatalf("PlasticDelta = %.17g, want %.17g", e.PlasticDelta, 1.3678794411714423)
	}
	if e.BaseParameterDelta != 0 {
		t.Fatalf("BaseParameterDelta = %g, want 0", e.BaseParameterDelta)
	}
}

// TestInterveneForceSpikeOnLIF checks the state consequences of one forced
// spike: the membrane resets to VReset, the refractory counter starts at
// RefractorySteps, and the persisted trace carries the forced unit event. On a
// non-spiking core the same plan is refused before anything is touched.
func TestInterveneForceSpikeOnLIF(t *testing.T) {
	core := plasticLIFCore()
	core.RefractorySteps = 3
	cfg := learning.Config{LIF: &core, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0}}
	a := newIndividual(t, cfg, plasticLIFParameters())
	plan := authorizedPlan(nodeIntervention(learning.InterventionForceSpike, []int{0}, 0, 0, 1))
	out, log, err := a.Intervene(context.Background(), plan, [][]float64{{0}})
	if err != nil {
		t.Fatalf("Intervene() error = %v", err)
	}
	if !closeRows(out, [][]float64{{0}}, stateTol) {
		t.Fatalf("forced spike output = %v, want [[0]]", out)
	}
	s := a.Snapshot()
	li := s.Neural.LIF
	if li.Voltage[0] != -0.5 {
		t.Fatalf("membrane = %v, want VReset -0.5 after a forced spike", li.Voltage)
	}
	if li.Refractory[0] != 3 {
		t.Fatalf("refractory = %v, want 3 after a forced spike", li.Refractory)
	}
	last := li.History[len(li.History)-1]
	if last[0] != 1 {
		t.Fatalf("persisted trace = %v, want the forced unit event 1", last)
	}
	e := log.Entries[0]
	if e.AppliedRows != 1 || e.ActivityDelta != 0 || e.BaseParameterDelta != 0 {
		t.Fatalf("entry = %+v, want one applied row with no readout or parameter movement", e)
	}
	// The continuous core has no events to force: Validate refuses the plan
	// before any state moves.
	c := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
	before := c.Snapshot()
	_, _, err = c.Intervene(context.Background(), authorizedPlan(nodeIntervention(learning.InterventionForceSpike, []int{0}, 0, 0, 1)), [][]float64{{1, 0}})
	if err == nil {
		t.Fatal("force_spike on a continuous core must be refused")
	}
	if !strings.Contains(err.Error(), "spiking") {
		t.Fatalf("refusal = %v, want a \"spiking\" mention", err)
	}
	if !reflect.DeepEqual(c.Snapshot(), before) {
		t.Fatal("a refused force_spike changed the individual")
	}
}

// TestInterveneUnauthorizedChangesNothing walks the refusal paths: a plan that
// does not say it experiments, an unknown kind, and the declared channel and
// region kinds the learning path does not implement yet. Every refusal leaves
// the individual exactly as it was.
func TestInterveneUnauthorizedChangesNothing(t *testing.T) {
	clamp := nodeIntervention(learning.InterventionClampVoltage, []int{0}, 0, 0, 1)
	cases := []struct {
		name          string
		build         func(t *testing.T) *learning.Individual
		plan          learning.InterventionPlan
		wantErr       string
		authorization bool
	}{
		{
			"unauthorized",
			func(t *testing.T) *learning.Individual {
				return newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
			},
			learning.InterventionPlan{Authorized: false, Reason: "approved", Items: []learning.Intervention{clamp}},
			"authorized",
			true,
		},
		{
			"blank reason",
			func(t *testing.T) *learning.Individual {
				return newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
			},
			learning.InterventionPlan{Authorized: true, Reason: "   ", Items: []learning.Intervention{clamp}},
			"authorized",
			true,
		},
		{
			"unknown kind",
			func(t *testing.T) *learning.Individual {
				return newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
			},
			authorizedPlan(learning.Intervention{Kind: "ablate", Start: 0, End: 1}),
			"unknown intervention kind",
			false,
		},
		{
			"block channel",
			func(t *testing.T) *learning.Individual {
				a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
				enableChemistry(t, a, sensitivityDeclaration())
				return a
			},
			authorizedPlan(channelIntervention(learning.InterventionBlockChannel, 0, 0, 0, 1)),
			"is not implemented yet",
			false,
		},
		{
			"fix concentration",
			func(t *testing.T) *learning.Individual {
				a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
				enableChemistry(t, a, sensitivityDeclaration())
				return a
			},
			authorizedPlan(channelIntervention(learning.InterventionFixConcentration, 0, 1, 0, 1)),
			"is not implemented yet",
			false,
		},
		{
			"remove channel",
			func(t *testing.T) *learning.Individual {
				a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
				enableChemistry(t, a, sensitivityDeclaration())
				return a
			},
			authorizedPlan(channelIntervention(learning.InterventionRemoveChannel, 0, 0, 0, 1)),
			"is not implemented yet",
			false,
		},
		{
			"swap regions",
			func(t *testing.T) *learning.Individual {
				a := newIndividual(t, chemContinuousConfig(), chemContinuousParameters())
				enableChemistry(t, a, sensitivityDeclaration())
				return a
			},
			authorizedPlan(swapIntervention(0, 1, 0, 1)),
			"is not implemented yet",
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.build(t)
			before := a.Snapshot()
			_, _, err := a.Intervene(context.Background(), tc.plan, [][]float64{{1, 0}})
			if err == nil {
				t.Fatalf("Intervene() must refuse %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("refusal = %v, want it to mention %q", err, tc.wantErr)
			}
			if tc.authorization && !errors.Is(err, learning.ErrInterventionNotAuthorized) {
				t.Fatalf("refusal = %v, want errors.Is to recognise ErrInterventionNotAuthorized", err)
			}
			if !reflect.DeepEqual(a.Snapshot(), before) {
				t.Fatal("the refused plan changed the individual")
			}
		})
	}
}
