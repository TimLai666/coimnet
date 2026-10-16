package learning_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

type consolidationNormLayer uint8

const (
	consolidationNormBase consolidationNormLayer = iota
	consolidationNormSlow
	consolidationNormPlastic
)

func (l consolidationNormLayer) String() string {
	switch l {
	case consolidationNormBase:
		return "base"
	case consolidationNormSlow:
		return "slow"
	case consolidationNormPlastic:
		return "plastic"
	default:
		return "unknown"
	}
}

type consolidationNormBoundaryCase struct {
	name          string
	values        []float64
	representable bool
}

// TestConsolidationNormBoundary uses an independent high-precision oracle at
// the float64 upper boundary. The production l2 helper is deliberately not
// called here: a finite rounded result must be accepted, while a true result
// above MaxFloat64 must refuse before either memory layer or its budget moves.
func TestConsolidationNormBoundary(t *testing.T) {
	for _, layer := range []consolidationNormLayer{
		consolidationNormBase,
		consolidationNormSlow,
		consolidationNormPlastic,
	} {
		for _, tc := range consolidationNormBoundaryCases() {
			t.Run(layer.String()+"/"+tc.name, func(t *testing.T) {
				a := consolidationNormIndividual(t, layer, tc.values, 1)
				before := a.Snapshot()
				beforeChemistry := a.ChemistryReport()
				trigger := consolidationNormTrigger(layer)
				report, err := a.Consolidate(context.Background(), trigger)
				wantNorm := consolidationNormExactNorm(tc.values)
				if tc.representable {
					if math.IsInf(wantNorm, 0) || math.IsNaN(wantNorm) {
						t.Fatalf("oracle norm = %.17g, want finite", wantNorm)
					}
					if err != nil {
						t.Fatalf("Consolidate() error = %v, want success", err)
					}
					slowWant, plasticWant := consolidationNormWrite(before, trigger)
					wantReportNorms := consolidationNormReportNorms(before, slowWant, plasticWant)
					wantReportNorms[layer] = wantNorm
					consolidationNormAssertApplied(t, before, beforeChemistry, a, report, slowWant, plasticWant, wantReportNorms)
					if got := consolidationNormReportNorm(report, layer); math.Float64bits(got) != math.Float64bits(wantNorm) {
						t.Fatalf("%s report norm bits = %#x, want oracle %#x (got %.17g want %.17g)", layer, math.Float64bits(got), math.Float64bits(wantNorm), got, wantNorm)
					}
				} else {
					if !math.IsInf(wantNorm, 1) {
						t.Fatalf("oracle norm = %.17g, want +Inf", wantNorm)
					}
					consolidationNormAssertRefused(t, before, beforeChemistry, a, report, err, "consolidation report norm is not finite", nil, learning.ConsolidationReport{})
				}
				consolidationNormAssertJSON(t, report, a.Snapshot(), a.ChemistryReport())
			})
		}
	}
}

// TestConsolidationNormNormalBits pins the ordinary path to sequential
// math.Hypot rounding and checks both public memory arrays against the exact
// consolidation formula after a successful write.
func TestConsolidationNormNormalBits(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
	}{
		{name: "zero", values: []float64{0}},
		{name: "three-four", values: []float64{3, 4}},
		{name: "negative", values: []float64{-3, 4}},
		{name: "smallest-subnormal", values: []float64{math.SmallestNonzeroFloat64}},
		{name: "large-finite", values: []float64{1e308, -1e308}},
	}
	for _, layer := range []consolidationNormLayer{
		consolidationNormBase,
		consolidationNormSlow,
		consolidationNormPlastic,
	} {
		for _, tc := range cases {
			t.Run(layer.String()+"/"+tc.name, func(t *testing.T) {
				a := consolidationNormIndividual(t, layer, tc.values, 1)
				before := a.Snapshot()
				beforeChemistry := a.ChemistryReport()
				trigger := consolidationNormTrigger(layer)
				report, err := a.Consolidate(context.Background(), trigger)
				if err != nil {
					t.Fatalf("Consolidate() error = %v", err)
				}
				slowWant, plasticWant := consolidationNormWrite(before, trigger)
				consolidationNormAssertApplied(t, before, beforeChemistry, a, report, slowWant, plasticWant, consolidationNormReportNorms(before, slowWant, plasticWant))
				wantNorm := consolidationNormSequentialNorm(tc.values)
				gotNorm := consolidationNormReportNorm(report, layer)
				if math.Float64bits(gotNorm) != math.Float64bits(wantNorm) {
					t.Fatalf("%s report norm bits = %#x, want sequential Hypot %#x (got %.17g want %.17g)", layer, math.Float64bits(gotNorm), math.Float64bits(wantNorm), gotNorm, wantNorm)
				}
				consolidationNormAssertJSON(t, report, a.Snapshot(), a.ChemistryReport())
			})
		}
	}
}

type consolidationNormRefusalKind uint8

const (
	consolidationNormNilContext consolidationNormRefusalKind = iota
	consolidationNormCanceledContext
	consolidationNormClosedGate
	consolidationNormZeroBudget
	consolidationNormAlreadyConsolidated
)

// TestConsolidationNormRefusalPrecedence puts the same unrepresentable vector
// behind every pre-existing refusal. Each branch must keep its original error
// priority and leave the complete public snapshot and chemistry report alone.
func TestConsolidationNormRefusalPrecedence(t *testing.T) {
	cases := []struct {
		name string
		kind consolidationNormRefusalKind
	}{
		{name: "nil-context", kind: consolidationNormNilContext},
		{name: "canceled-context", kind: consolidationNormCanceledContext},
		{name: "closed-gate", kind: consolidationNormClosedGate},
		{name: "budget-zero", kind: consolidationNormZeroBudget},
		{name: "already-consolidated", kind: consolidationNormAlreadyConsolidated},
	}
	values := []float64{math.MaxFloat64, math.Ldexp(math.MaxFloat64, -26)}
	for _, layer := range []consolidationNormLayer{
		consolidationNormBase,
		consolidationNormSlow,
		consolidationNormPlastic,
	} {
		for _, tc := range cases {
			t.Run(layer.String()+"/"+tc.name, func(t *testing.T) {
				trigger := consolidationNormTrigger(layer)
				var a *learning.Individual
				var ctx context.Context = context.Background()
				var wantErr error
				wantText := ""
				wantReport := learning.ConsolidationReport{}

				switch tc.kind {
				case consolidationNormNilContext:
					a = consolidationNormIndividual(t, layer, values, 1)
					ctx = nil
					wantText = "nil context"
				case consolidationNormCanceledContext:
					a = consolidationNormIndividual(t, layer, values, 1)
					canceled, cancel := context.WithCancel(context.Background())
					cancel()
					ctx = canceled
					wantErr = context.Canceled
				case consolidationNormClosedGate:
					a = consolidationNormIndividual(t, layer, values, 1)
					trigger.Threshold = 1
					wantErr = learning.ErrConsolidationGateClosed
					wantReport.GateClosed = true
				case consolidationNormZeroBudget:
					a = consolidationNormIndividual(t, layer, values, 0)
					wantErr = learning.ErrBudgetExhausted
					wantReport.BudgetExhausted = true
				case consolidationNormAlreadyConsolidated:
					a = consolidationNormIndividual(t, layer, []float64{0, 0}, 2)
					if _, err := a.Consolidate(context.Background(), trigger); err != nil {
						t.Fatalf("setup Consolidate() error = %v", err)
					}
					snapshot := a.Snapshot()
					consolidationNormSetLayerValues(&snapshot, layer, values)
					var err error
					a, err = learning.RestoreIndividual(snapshot)
					if err != nil {
						t.Fatalf("RestoreIndividual() error = %v", err)
					}
					wantErr = learning.ErrAlreadyConsolidated
					wantReport.Already = true
					wantReport.LastEpisode = 1
					wantReport.Used = 1
				}

				before := a.Snapshot()
				beforeChemistry := a.ChemistryReport()
				report, err := a.Consolidate(ctx, trigger)
				consolidationNormAssertRefused(t, before, beforeChemistry, a, report, err, wantText, wantErr, wantReport)
				after := a.Snapshot()
				consolidationNormAssertJSON(t, report, after, a.ChemistryReport())
			})
		}
	}
}

func consolidationNormBoundaryCases() []consolidationNormBoundaryCase {
	max := math.MaxFloat64
	finiteTail := math.Ldexp(max, -27)
	overflowTail := math.Ldexp(max, -26)
	finite := consolidationNormPairCases("upper-finite", max, finiteTail, true)
	overflow := consolidationNormPairCases("upper-overflow", max, overflowTail, false)
	threeFiniteX := math.Nextafter(math.Nextafter(math.Nextafter(max, 0), 0), 0)
	threeFiniteTail := math.Ldexp(max, -26) * 1.3
	threeOverflowX := math.Nextafter(max, 0)
	threeOverflowTail := math.Ldexp(max, -26)
	threeFinite := consolidationNormTripleCases("three-exact-finite", threeFiniteX, threeFiniteTail, true)
	threeOverflow := consolidationNormTripleCases("three-exact-overflow", threeOverflowX, threeOverflowTail, false)
	obvious := []consolidationNormBoundaryCase{
		{name: "obvious-overflow-positive", values: []float64{1.3e308, 1.3e308}, representable: false},
		{name: "obvious-overflow-negative", values: []float64{-1.3e308, -1.3e308}, representable: false},
		{name: "obvious-overflow-mixed", values: []float64{1.3e308, -1.3e308}, representable: false},
		{name: "obvious-overflow-mixed-reversed", values: []float64{-1.3e308, 1.3e308}, representable: false},
	}
	return append(append(append(append(finite, overflow...), threeFinite...), threeOverflow...), obvious...)
}

func consolidationNormPairCases(name string, first, second float64, representable bool) []consolidationNormBoundaryCase {
	return []consolidationNormBoundaryCase{
		{name: name + "-positive", values: []float64{first, second}, representable: representable},
		{name: name + "-positive-reversed", values: []float64{second, first}, representable: representable},
		{name: name + "-negative", values: []float64{-first, -second}, representable: representable},
		{name: name + "-negative-reversed", values: []float64{-second, -first}, representable: representable},
		{name: name + "-mixed", values: []float64{first, -second}, representable: representable},
		{name: name + "-mixed-reversed", values: []float64{-second, first}, representable: representable},
		{name: name + "-mixed-other", values: []float64{-first, second}, representable: representable},
		{name: name + "-mixed-other-reversed", values: []float64{second, -first}, representable: representable},
	}
}

func consolidationNormTripleCases(name string, first, second float64, representable bool) []consolidationNormBoundaryCase {
	return []consolidationNormBoundaryCase{
		{name: name + "-positive-first", values: []float64{first, second, second}, representable: representable},
		{name: name + "-positive-middle", values: []float64{second, first, second}, representable: representable},
		{name: name + "-positive-last", values: []float64{second, second, first}, representable: representable},
		{name: name + "-negative-first", values: []float64{-first, -second, -second}, representable: representable},
		{name: name + "-negative-middle", values: []float64{-second, -first, -second}, representable: representable},
		{name: name + "-negative-last", values: []float64{-second, -second, -first}, representable: representable},
	}
}

func consolidationNormIndividual(t *testing.T, layer consolidationNormLayer, values []float64, budget uint64) *learning.Individual {
	t.Helper()
	if len(values) == 0 {
		t.Fatal("consolidation fixture needs at least one value")
	}
	weights := make([]float64, len(values))
	if layer == consolidationNormBase {
		weights = append([]float64(nil), values...)
	}
	sources := make([]int, len(values))
	targets := make([]int, len(values))
	delays := make([]int, len(values))
	edges := make([]int, len(values))
	for edge := range values {
		delays[edge] = edge + 1
		edges[edge] = edge
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 1, Sources: sources, Targets: targets, Delays: delays, DT: 1, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, InputNodes: []int{0}, ReadoutNodes: []int{0},
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: weights, Bias: []float64{0}, LogTau: []float64{0}},
		Encoder: []float64{1}, Readout: []float64{1},
	}
	a, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0})
	if err != nil {
		t.Fatalf("NewIndividual() error = %v", err)
	}
	if err := a.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: math.MaxFloat64, WMin: .01},
		Edges: edges,
	}); err != nil {
		t.Fatalf("EnablePlasticity() error = %v", err)
	}
	if err := a.EnableChemistry(modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{1}},
		Sources:   []modulation.SourceSpec{{Kind: modulation.SourceExternalTimeline, Timeline: &modulation.ExternalTimeline{ChannelCount: 1}}},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{{
			Cells: []int{0}, Signal: "engineering", Status: modulation.StatusHypothesized, Kd: 1, N: 1,
			Evidence: "report boundary probe", MeasurementKind: "engineering declaration", MappingVersion: "consolidation-norm/v1",
		}}},
	}); err != nil {
		t.Fatalf("EnableChemistry() error = %v", err)
	}
	if err := a.EnableConsolidation(budget); err != nil {
		t.Fatalf("EnableConsolidation() error = %v", err)
	}
	if layer == consolidationNormBase {
		return a
	}
	snapshot := a.Snapshot()
	consolidationNormSetLayerValues(&snapshot, layer, values)
	a, err = learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual() error = %v", err)
	}
	return a
}

func consolidationNormSetLayerValues(snapshot *learning.IndividualSnapshot, layer consolidationNormLayer, values []float64) {
	if snapshot.Plastic == nil || snapshot.Plastic.Slow == nil {
		panic("consolidation fixture has no enabled slow state")
	}
	values = append([]float64(nil), values...)
	switch layer {
	case consolidationNormBase:
		snapshot.Parameters.Core.Weights = values
	case consolidationNormSlow:
		snapshot.Plastic.Slow.Values = values
	case consolidationNormPlastic:
		snapshot.Plastic.State.Plastic = values
	default:
		panic("unknown consolidation norm layer")
	}
}

func consolidationNormTrigger(layer consolidationNormLayer) learning.ConsolidationTrigger {
	rate := 1.0
	if layer == consolidationNormPlastic {
		rate = math.SmallestNonzeroFloat64
	}
	return learning.ConsolidationTrigger{Receptor: 0, Threshold: 0, Rate: rate, Retain: 1}
}

func consolidationNormWrite(before learning.IndividualSnapshot, trigger learning.ConsolidationTrigger) ([]float64, []float64) {
	slow := make([]float64, len(before.Plastic.Slow.Values))
	plastic := make([]float64, len(before.Plastic.State.Plastic))
	for k := range slow {
		slow[k] = before.Plastic.Slow.Values[k] + trigger.Rate*before.Plastic.State.Plastic[k]
		plastic[k] = trigger.Retain * before.Plastic.State.Plastic[k]
	}
	return slow, plastic
}

func consolidationNormExactNorm(values []float64) float64 {
	sum := new(big.Float).SetPrec(8192)
	term := new(big.Float).SetPrec(8192)
	for _, value := range values {
		term.SetFloat64(value)
		term.Mul(term, term)
		sum.Add(sum, term)
	}
	sum.Sqrt(sum)
	result, _ := sum.Float64()
	return result
}

func consolidationNormSequentialNorm(values []float64) float64 {
	var norm float64
	for _, value := range values {
		norm = math.Hypot(norm, value)
	}
	return norm
}

func consolidationNormReportNorm(report learning.ConsolidationReport, layer consolidationNormLayer) float64 {
	switch layer {
	case consolidationNormBase:
		return report.BaseL2
	case consolidationNormSlow:
		return report.SlowL2
	case consolidationNormPlastic:
		return report.PlasticL2
	default:
		panic("unknown consolidation norm layer")
	}
}

func consolidationNormReportNorms(before learning.IndividualSnapshot, slowWant, plasticWant []float64) [3]float64 {
	return [3]float64{
		consolidationNormSequentialNorm(before.Parameters.Core.Weights),
		consolidationNormSequentialNorm(slowWant),
		consolidationNormSequentialNorm(plasticWant),
	}
}

func consolidationNormAssertApplied(t *testing.T, before learning.IndividualSnapshot, beforeChemistry learning.ChemistryReport, a *learning.Individual, report learning.ConsolidationReport, slowWant, plasticWant []float64, wantReportNorms [3]float64) {
	t.Helper()
	if !report.Applied || report.GateClosed || report.Already || report.BudgetExhausted {
		t.Fatalf("successful consolidation report = %+v", report)
	}
	beforeUsed := before.Plastic.Slow.Used
	if report.Episode != before.Optimizer.Episodes || report.LastEpisode != before.Optimizer.Episodes+1 || report.Used != beforeUsed+1 {
		t.Fatalf("successful consolidation counters = {episode %d last %d used %d}, want {%d %d %d}", report.Episode, report.LastEpisode, report.Used, before.Optimizer.Episodes, before.Optimizer.Episodes+1, beforeUsed+1)
	}
	if math.Float64bits(report.Occupancy) != math.Float64bits(0) {
		t.Fatalf("successful consolidation occupancy = %.17g, want zero", report.Occupancy)
	}
	if math.Float64bits(report.BaseL2) != math.Float64bits(wantReportNorms[consolidationNormBase]) {
		t.Fatalf("BaseL2 bits = %#x, want %#x", math.Float64bits(report.BaseL2), math.Float64bits(wantReportNorms[consolidationNormBase]))
	}
	if math.Float64bits(report.SlowL2) != math.Float64bits(wantReportNorms[consolidationNormSlow]) {
		t.Fatalf("SlowL2 bits = %#x, want %#x", math.Float64bits(report.SlowL2), math.Float64bits(wantReportNorms[consolidationNormSlow]))
	}
	if math.Float64bits(report.PlasticL2) != math.Float64bits(wantReportNorms[consolidationNormPlastic]) {
		t.Fatalf("PlasticL2 bits = %#x, want %#x", math.Float64bits(report.PlasticL2), math.Float64bits(wantReportNorms[consolidationNormPlastic]))
	}
	expected := before
	expectedPlastic := *before.Plastic
	expectedSlow := *before.Plastic.Slow
	expectedSlow.Values = append([]float64(nil), slowWant...)
	expectedSlow.LastEpisode = before.Optimizer.Episodes + 1
	expectedSlow.Used = beforeUsed + 1
	expectedPlastic.Slow = &expectedSlow
	expectedState := before.Plastic.State
	expectedState.Plastic = append([]float64(nil), plasticWant...)
	expectedPlastic.State = expectedState
	expected.Plastic = &expectedPlastic
	after := a.Snapshot()
	consolidationNormAssertBits(t, "slow values", after.Plastic.Slow.Values, slowWant)
	consolidationNormAssertBits(t, "plastic values", after.Plastic.State.Plastic, plasticWant)
	if !reflect.DeepEqual(after, expected) {
		t.Fatalf("successful consolidation snapshot = %+v, want %+v", after, expected)
	}
	if !reflect.DeepEqual(a.ChemistryReport(), beforeChemistry) {
		t.Fatal("successful consolidation changed the chemistry report")
	}
}

func consolidationNormAssertRefused(t *testing.T, before learning.IndividualSnapshot, beforeChemistry learning.ChemistryReport, a *learning.Individual, report learning.ConsolidationReport, err error, wantText string, wantErr error, wantReport learning.ConsolidationReport) {
	t.Helper()
	if err == nil || (wantText != "" && err.Error() != wantText) || (wantErr != nil && !errors.Is(err, wantErr)) {
		t.Fatalf("refused Consolidate() report = %+v, error = %v, want text %q error %v", report, err, wantText, wantErr)
	}
	if !reflect.DeepEqual(report, wantReport) {
		t.Fatalf("refused consolidation report = %+v, want %+v", report, wantReport)
	}
	if !reflect.DeepEqual(a.Snapshot(), before) {
		t.Fatalf("refused consolidation changed the complete snapshot:\nbefore=%+v\nafter=%+v", before, a.Snapshot())
	}
	if !reflect.DeepEqual(a.ChemistryReport(), beforeChemistry) {
		t.Fatalf("refused consolidation changed the chemistry report:\nbefore=%+v\nafter=%+v", beforeChemistry, a.ChemistryReport())
	}
}

func consolidationNormAssertBits(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] bits = %#x, want %#x (got %.17g want %.17g)", name, i, math.Float64bits(got[i]), math.Float64bits(want[i]), got[i], want[i])
		}
	}
}

func consolidationNormAssertJSON(t *testing.T, report learning.ConsolidationReport, snapshot learning.IndividualSnapshot, chemistry learning.ChemistryReport) {
	t.Helper()
	for name, value := range map[string]any{
		"report": report, "snapshot": snapshot, "chemistry report": chemistry,
	} {
		if _, err := json.Marshal(value); err != nil {
			t.Fatalf("json.Marshal(%s) error = %v", name, err)
		}
	}
}
