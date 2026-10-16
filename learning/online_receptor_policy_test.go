package learning_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
)

const (
	onlineReceptorPolicyContinuous = "continuous"
	onlineReceptorPolicyLIF        = "lif"
	onlineReceptorPolicyMixed      = "mixed"
)

// The fixture is deliberately small but has an active path on every core:
// both input nodes are driven, and the only edge is 0 -> 1.  The second node
// is the readout, so a later edge response is observable instead of a hidden
// internal state change.
func onlineReceptorPolicyInput() [][]float64 {
	return [][]float64{{2, 2}, {2, 2}, {2, 2}}
}

func onlineReceptorPolicyConfig(core string) (learning.Config, learning.Parameters) {
	switch core {
	case onlineReceptorPolicyContinuous:
		return learning.Config{
				Dynamics: dynamics.Config{
					Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0},
					DT: 1, Activation: "tanh",
				},
				InputSize: 2, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0, 1},
			}, learning.Parameters{
				Core:    dynamics.Parameters{Weights: []float64{.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
				Encoder: []float64{1, 0, 0, 1}, Readout: []float64{1},
			}
	case onlineReceptorPolicyLIF:
		lif := dynamics.LIFConfig{
			Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0},
			DT: 1, TauSyn: 1, ThetaMin: .1, ThetaMax: 2, VReset: -.5,
			RefractorySteps: 0, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
		}
		return learning.Config{
				LIF: &lif, InputSize: 2, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0, 1},
			}, learning.Parameters{
				Core:     dynamics.Parameters{Weights: []float64{.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
				ThetaRaw: []float64{0, 0}, Encoder: []float64{1, 0, 0, 1}, Readout: []float64{1},
			}
	case onlineReceptorPolicyMixed:
		mixed := dynamics.MixedConfig{
			Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1,
			NodeRule:   []uint8{dynamics.RuleLIF, dynamics.RuleContinuous},
			Continuous: dynamics.ContinuousRule{Activation: "tanh"},
			LIF: dynamics.LIFRule{
				TauSyn: 1, ThetaMin: .1, ThetaMax: 2, VReset: -.5,
				RefractorySteps: 0, Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
			},
		}
		return learning.Config{
				Mixed: &mixed, InputSize: 2, OutputSize: 1, ReadoutNodes: []int{1}, InputNodes: []int{0, 1},
			}, learning.Parameters{
				Core:     dynamics.Parameters{Weights: []float64{.9}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
				ThetaRaw: []float64{0}, Encoder: []float64{1, 0, 0, 1}, Readout: []float64{1},
			}
	default:
		panic("unknown online receptor policy core " + core)
	}
}

func onlineReceptorPolicyChemistry(pulse bool) modulation.ChemistryConfig {
	entries := []modulation.TimelineEntry(nil)
	if pulse {
		entries = []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}
	}
	return modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 2, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: entries},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{
				Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized,
				Kd: .5, N: 1, Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1",
			},
			{
				Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: modulation.StatusUnresponsive,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1",
			},
		}},
		Effects: []modulation.Effect{{Kind: modulation.EffectSensitivity, Receptor: 0, GammaScale: 1}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 1}},
	}
}

func onlineReceptorPolicyPlasticRule(gateReceptor, receptorWindow bool) plasticity.Rule {
	rule := plasticity.Rule{
		Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5,
		PlasticMax: 8, WMin: .0625,
	}
	if gateReceptor {
		index := 0
		rule.GateReceptor = &index
		rule.GateScale = 1
	}
	if receptorWindow {
		index := 0
		rule.DecayEReceptor = &index
		rule.DecayEBase = .5
		rule.DecayESpan = -.4
		rule.DecayEMin = .1
		rule.DecayEMax = .9
	}
	return rule
}

func onlineReceptorPolicyIndividual(t *testing.T, core string, pulse, gateReceptor, receptorWindow bool, fast float64) *learning.Individual {
	t.Helper()
	c, p := onlineReceptorPolicyConfig(core)
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatalf("NewIndividual(%s): %v", core, err)
	}
	if err := a.EnableChemistry(onlineReceptorPolicyChemistry(pulse)); err != nil {
		t.Fatalf("EnableChemistry(%s): %v", core, err)
	}
	if err := a.EnablePlasticity(plasticity.Config{
		Rule: onlineReceptorPolicyPlasticRule(gateReceptor, receptorWindow), Edges: []int{0},
	}); err != nil {
		t.Fatalf("EnablePlasticity(%s): %v", core, err)
	}
	if fast == 0 {
		return a
	}
	snapshot := a.Snapshot()
	if snapshot.Plastic == nil || len(snapshot.Plastic.State.Plastic) != 1 {
		t.Fatalf("%s fixture has no one-edge fast state: %+v", core, snapshot.Plastic)
	}
	snapshot.Plastic.State.Plastic[0] = fast
	restored, err := learning.RestoreIndividual(snapshot)
	if err != nil {
		t.Fatalf("RestoreIndividual(%s fast=%g): %v", core, fast, err)
	}
	return restored
}

func onlineReceptorPolicyRewardGate() learning.RewardGate {
	mapper := &modulation.RewardMapper{Baseline: modulation.BaselineNone, Kind: modulation.KindRelu}
	return learning.RewardGateFunc(func(score float64) ([]float64, error) {
		record, err := mapper.Map(score)
		if err != nil {
			return nil, err
		}
		return record.Applied, nil
	})
}

func onlineReceptorPolicyLearner(t *testing.T, a *learning.Individual, policy learning.OnlinePolicy) *learning.OnlineLearner {
	t.Helper()
	var gate learning.RewardGate
	if policy.AllowPlastic {
		gate = onlineReceptorPolicyRewardGate()
	}
	learner, err := learning.NewOnlineLearner(a, policy, gate)
	if err != nil {
		t.Fatalf("NewOnlineLearner(%+v): %v", policy, err)
	}
	return learner
}

func onlineReceptorPolicyObservation(t *testing.T) signal.Observation {
	t.Helper()
	observation, err := signal.NewObservation(signal.CurrentSchemaVersion(), "online-receptor-policy", "stream-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func onlineReceptorPolicyFeedback(t *testing.T, actionID, source string, step int64, score float64) signal.Feedback {
	t.Helper()
	version, err := signal.NewVersion(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: signal.CurrentSchemaVersion(), ExperienceID: "online-receptor-policy", ActionID: actionID,
		ProducedAt:  signal.Timestamp{Value: step, Unit: signal.TimeUnitModelStep},
		AvailableAt: signal.Timestamp{Value: step, Unit: signal.TimeUnitModelStep},
		Source:      source, Score: score, ModelVersion: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return feedback
}

func onlineReceptorPolicyAt(step int64) signal.Timestamp {
	return signal.Timestamp{Value: step, Unit: signal.TimeUnitModelStep}
}

func onlineReceptorPolicyPolicies() []struct {
	name   string
	policy learning.OnlinePolicy
} {
	clock := signal.TimeUnitModelStep
	return []struct {
		name   string
		policy learning.OnlinePolicy
	}{
		{name: "gradient", policy: learning.OnlinePolicy{AllowGradient: true, ClockUnit: clock}},
		{name: "plastic", policy: learning.OnlinePolicy{AllowPlastic: true, ClockUnit: clock}},
		{name: "mixed", policy: learning.OnlinePolicy{AllowGradient: true, AllowPlastic: true, ClockUnit: clock}},
		{name: "evaluate", policy: learning.OnlinePolicy{AllowGradient: true, Evaluate: true, ClockUnit: clock}},
	}
}

func onlineReceptorPolicyNormalizeSnapshot(snapshot learning.IndividualSnapshot) learning.IndividualSnapshot {
	if snapshot.Plastic != nil {
		plastic := *snapshot.Plastic
		rule := plastic.Config.Rule
		rule.GateReceptor = nil
		rule.GateScale = 0
		plastic.Config.Rule = rule
		snapshot.Plastic = &plastic
	}
	return snapshot
}

func onlineReceptorPolicyRowsEqualBits(got, want [][]float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if math.Float64bits(got[i][j]) != math.Float64bits(want[i][j]) {
				return false
			}
		}
	}
	return true
}

func onlineReceptorPolicyValuesEqualBits(got, want any) bool {
	return onlineReceptorPolicyReflectEqualBits(reflect.ValueOf(got), reflect.ValueOf(want))
}

func onlineReceptorPolicyReflectEqualBits(got, want reflect.Value) bool {
	if !got.IsValid() || !want.IsValid() {
		return !got.IsValid() && !want.IsValid()
	}
	if got.Type() != want.Type() {
		return false
	}
	switch got.Kind() {
	case reflect.Interface:
		if got.IsNil() || want.IsNil() {
			return got.IsNil() && want.IsNil()
		}
		return onlineReceptorPolicyReflectEqualBits(got.Elem(), want.Elem())
	case reflect.Pointer:
		if got.IsNil() || want.IsNil() {
			return got.IsNil() && want.IsNil()
		}
		return onlineReceptorPolicyReflectEqualBits(got.Elem(), want.Elem())
	case reflect.Float64:
		return math.Float64bits(got.Float()) == math.Float64bits(want.Float())
	case reflect.Float32:
		return math.Float32bits(float32(got.Float())) == math.Float32bits(float32(want.Float()))
	case reflect.Array, reflect.Slice:
		if got.Kind() == reflect.Slice && (got.IsNil() || want.IsNil()) {
			return got.IsNil() && want.IsNil()
		}
		if got.Len() != want.Len() {
			return false
		}
		for i := 0; i < got.Len(); i++ {
			if !onlineReceptorPolicyReflectEqualBits(got.Index(i), want.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if got.IsNil() || want.IsNil() {
			return got.IsNil() && want.IsNil()
		}
		if got.Len() != want.Len() {
			return false
		}
		iter := got.MapRange()
		for iter.Next() {
			other := want.MapIndex(iter.Key())
			if !other.IsValid() || !onlineReceptorPolicyReflectEqualBits(iter.Value(), other) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < got.NumField(); i++ {
			if !onlineReceptorPolicyReflectEqualBits(got.Field(i), want.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Bool:
		return got.Bool() == want.Bool()
	case reflect.String:
		return got.String() == want.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return got.Int() == want.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return got.Uint() == want.Uint()
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return got.IsNil() == want.IsNil() && (got.IsNil() || got.Pointer() == want.Pointer())
	default:
		return reflect.DeepEqual(got.Interface(), want.Interface())
	}
}

func onlineReceptorPolicyAssertSnapshot(t *testing.T, label string, got, want learning.IndividualSnapshot) {
	t.Helper()
	got = onlineReceptorPolicyNormalizeSnapshot(got)
	want = onlineReceptorPolicyNormalizeSnapshot(want)
	if !onlineReceptorPolicyValuesEqualBits(got, want) {
		t.Fatalf("%s snapshot differs after normalizing only GateReceptor/GateScale", label)
	}
}

func onlineReceptorPolicyAssertChemistry(t *testing.T, label string, got, want learning.ChemistryReport) {
	t.Helper()
	if !onlineReceptorPolicyValuesEqualBits(got, want) {
		t.Fatalf("%s chemistry report differs bitwise: got=%+v want=%+v", label, got, want)
	}
}

func onlineReceptorPolicyAssertRows(t *testing.T, label string, got, want [][]float64) {
	t.Helper()
	if !onlineReceptorPolicyRowsEqualBits(got, want) {
		t.Fatalf("%s output differs bitwise: got=%v want=%v", label, got, want)
	}
}

func onlineReceptorPolicyHasActivity(rows [][]float64) bool {
	for _, row := range rows {
		for _, value := range row {
			if value != 0 {
				return true
			}
		}
	}
	return false
}

func TestOnlineReceptorPolicyActMatrix(t *testing.T) {
	cores := []string{onlineReceptorPolicyContinuous, onlineReceptorPolicyLIF, onlineReceptorPolicyMixed}
	for _, core := range cores {
		for _, policyCase := range onlineReceptorPolicyPolicies() {
			for _, gateReceptor := range []bool{false, true} {
				for _, pulse := range []bool{false, true} {
					for _, fast := range []float64{0, .25} {
						name := fmt.Sprintf("%s/%s/gate-%t/pulse-%t/fast-%g", core, policyCase.name, gateReceptor, pulse, fast)
						t.Run(name, func(t *testing.T) {
							subject := onlineReceptorPolicyIndividual(t, core, pulse, gateReceptor, true, fast)
							initial := subject.Snapshot()
							controlSnapshot := onlineReceptorPolicyNormalizeSnapshot(initial)
							control, err := learning.RestoreIndividual(controlSnapshot)
							if err != nil {
								t.Fatalf("RestoreIndividual(control): %v", err)
							}
							learner := onlineReceptorPolicyLearner(t, subject, policyCase.policy)
							record, err := learner.Act(context.Background(), onlineReceptorPolicyObservation(t), onlineReceptorPolicyInput())
							if err != nil {
								t.Fatalf("Act(): %v", err)
							}
							controlOutput, err := control.Advance(context.Background(), onlineReceptorPolicyInput())
							if err != nil {
								t.Fatalf("control Advance(): %v", err)
							}
							onlineReceptorPolicyAssertRows(t, "Act/control", record.Output, controlOutput)
							if !onlineReceptorPolicyHasActivity(record.Output) {
								t.Fatal("Act fixture produced no observable activity")
							}
							after := subject.Snapshot()
							onlineReceptorPolicyAssertSnapshot(t, "Act/control", after, control.Snapshot())
							onlineReceptorPolicyAssertChemistry(t, "Act/control", subject.ChemistryReport(), control.ChemistryReport())
							if !onlineReceptorPolicyValuesEqualBits(after.Parameters, initial.Parameters) {
								t.Fatal("Act changed base parameters")
							}
							if want := fast * .125; math.Float64bits(after.Plastic.State.Plastic[0]) != math.Float64bits(want) {
								t.Fatalf("Act fast state = %.17g, want only three DecayP=.5 passes from %.17g", after.Plastic.State.Plastic[0], fast)
							}
							log := learner.Log()
							if len(log) != 1 || !onlineReceptorPolicyValuesEqualBits(log[0], record) {
								t.Fatalf("Act log = %+v, want the append-only action %+v", log, record)
							}
						})
					}
				}
			}
		}
	}
}

func TestOnlineReceptorPolicyUpdateMatrix(t *testing.T) {
	cores := []string{onlineReceptorPolicyContinuous, onlineReceptorPolicyLIF, onlineReceptorPolicyMixed}
	for _, core := range cores {
		for _, gateReceptor := range []bool{false, true} {
			for _, pulse := range []bool{false, true} {
				for _, score := range []float64{0, 1} {
					for _, receptorWindow := range []bool{false, true} {
						name := fmt.Sprintf("%s/gate-%t/pulse-%t/score-%g/window-%s", core, gateReceptor, pulse, score, map[bool]string{false: "fixed", true: "receptor"}[receptorWindow])
						t.Run(name, func(t *testing.T) {
							subject := onlineReceptorPolicyIndividual(t, core, pulse, gateReceptor, receptorWindow, 0)
							initial := subject.Snapshot()
							controlSnapshot := onlineReceptorPolicyNormalizeSnapshot(initial)
							control, err := learning.RestoreIndividual(controlSnapshot)
							if err != nil {
								t.Fatalf("RestoreIndividual(control): %v", err)
							}
							policy := learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}
							learner := onlineReceptorPolicyLearner(t, subject, policy)
							record, err := learner.Act(context.Background(), onlineReceptorPolicyObservation(t), onlineReceptorPolicyInput())
							if err != nil {
								t.Fatalf("Act(): %v", err)
							}
							controlOutput, err := control.Advance(context.Background(), onlineReceptorPolicyInput())
							if err != nil {
								t.Fatalf("control Advance(): %v", err)
							}
							onlineReceptorPolicyAssertRows(t, "Act/control", record.Output, controlOutput)
							if !onlineReceptorPolicyHasActivity(record.Output) {
								t.Fatal("Update fixture produced no observable activity")
							}
							if err := learner.Receive(context.Background(), onlineReceptorPolicyFeedback(t, record.ActionID, "score", int64(record.Step), score)); err != nil {
								t.Fatalf("Receive(): %v", err)
							}
							if _, _, err := control.AdvanceGated(context.Background(), onlineReceptorPolicyInput(), []float64{score, score, score}); err != nil {
								t.Fatalf("control AdvanceGated(): %v", err)
							}
							beforeUpdate := subject.Snapshot()
							report, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(record.Step)))
							if err != nil {
								t.Fatalf("Update(): %v", err)
							}
							if report.Consumed != 1 || report.PlasticSteps != 1 || report.GradientSteps != 0 || report.Deferred != 0 {
								t.Fatalf("Update report = %+v, want one plastic-only consumption", report)
							}
							onlineReceptorPolicyAssertSnapshot(t, "Update/control", subject.Snapshot(), control.Snapshot())
							onlineReceptorPolicyAssertChemistry(t, "Update/control", subject.ChemistryReport(), control.ChemistryReport())
							log := learner.Log()
							if len(log) != 1 || !onlineReceptorPolicyValuesEqualBits(log[0], record) {
								t.Fatalf("Update rewrote action log: %+v", log)
							}
							if score == 1 && onlineReceptorPolicyValuesEqualBits(subject.Snapshot().Plastic.State.Plastic, beforeUpdate.Plastic.State.Plastic) {
								t.Fatal("legal score=1 did not change the fast state")
							}
							if score == 0 && !onlineReceptorPolicyValuesEqualBits(subject.Snapshot().Plastic.State.Plastic, beforeUpdate.Plastic.State.Plastic) {
								t.Fatal("score=0 changed the fast state beyond the declared decay")
							}
						})
					}
				}
			}
		}
	}
}

func TestOnlineReceptorPolicyNative(t *testing.T) {
	for _, core := range []string{onlineReceptorPolicyContinuous, onlineReceptorPolicyLIF, onlineReceptorPolicyMixed} {
		t.Run(core, func(t *testing.T) {
			input := onlineReceptorPolicyInput()
			native := onlineReceptorPolicyIndividual(t, core, true, true, true, 0)
			before := native.Snapshot()
			output, err := native.Advance(context.Background(), input)
			if err != nil {
				t.Fatalf("native Advance(): %v", err)
			}
			if !onlineReceptorPolicyHasActivity(output) {
				t.Fatal("native positive-control fixture produced no activity")
			}
			if onlineReceptorPolicyValuesEqualBits(native.Snapshot().Plastic.State.Plastic, before.Plastic.State.Plastic) {
				t.Fatal("native active receptor did not produce a learnable fast-state change")
			}

			witness := onlineReceptorPolicyIndividual(t, core, true, true, true, 0)
			witnessOutput, witnessReport, err := witness.AdvanceGated(context.Background(), input, []float64{0, 0, 0})
			if err != nil {
				t.Fatalf("native zero-explicit-gate witness: %v", err)
			}
			if !witnessReport.GateFromReceptor || witnessReport.Steps != len(input) {
				t.Fatalf("native witness report = %+v, want receptor gate on every step", witnessReport)
			}
			onlineReceptorPolicyAssertRows(t, "native/zero-gate", output, witnessOutput)
			if !onlineReceptorPolicyValuesEqualBits(native.Snapshot(), witness.Snapshot()) {
				t.Fatal("native Advance and zero-explicit-gate witness differ with the same receptor declaration")
			}
			onlineReceptorPolicyAssertChemistry(t, "native/zero-gate", native.ChemistryReport(), witness.ChemistryReport())

			learnerIndividual := onlineReceptorPolicyIndividual(t, core, true, true, true, 0)
			learner := onlineReceptorPolicyLearner(t, learnerIndividual, learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep})
			onlineBefore := learnerIndividual.Snapshot()
			action, err := learner.Act(context.Background(), onlineReceptorPolicyObservation(t), input)
			if err != nil {
				t.Fatalf("online Act(): %v", err)
			}
			if err := learner.Receive(context.Background(), onlineReceptorPolicyFeedback(t, action.ActionID, "score", int64(action.Step), 1)); err != nil {
				t.Fatalf("online Receive(): %v", err)
			}
			if _, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(action.Step))); err != nil {
				t.Fatalf("online score=1 Update(): %v", err)
			}
			if onlineReceptorPolicyValuesEqualBits(learnerIndividual.Snapshot().Plastic.State.Plastic, onlineBefore.Plastic.State.Plastic) {
				t.Fatal("online score=1 did not learn")
			}

			declaration := learnerIndividual.Snapshot()
			if declaration.Plastic == nil || declaration.Plastic.Config.Rule.GateReceptor == nil || *declaration.Plastic.Config.Rule.GateReceptor != 0 || declaration.Plastic.Config.Rule.GateScale != 1 {
				t.Fatalf("online path lost receptor declaration: %+v", declaration.Plastic)
			}
			ordinaryControlSnapshot := onlineReceptorPolicyNormalizeSnapshot(declaration)
			ordinaryControl, err := learning.RestoreIndividual(ordinaryControlSnapshot)
			if err != nil {
				t.Fatalf("RestoreIndividual(closed ordinary control): %v", err)
			}
			if _, err := learnerIndividual.Advance(context.Background(), input); err != nil {
				t.Fatalf("ordinary Advance after online Update(): %v", err)
			}
			if _, err := ordinaryControl.Advance(context.Background(), input); err != nil {
				t.Fatalf("closed ordinary control Advance(): %v", err)
			}
			onlineReceptorPolicyAssertChemistry(t, "ordinary after online Update", learnerIndividual.ChemistryReport(), ordinaryControl.ChemistryReport())
			if onlineReceptorPolicyValuesEqualBits(learnerIndividual.Snapshot().Plastic.State.Plastic, ordinaryControl.Snapshot().Plastic.State.Plastic) {
				t.Fatal("ordinary Advance after online Update matched the closed-gate control; receptor declaration no longer gates native plasticity")
			}
			beforeRejected := learnerIndividual.Snapshot()
			beforeRejectedChemistry := learnerIndividual.ChemistryReport()
			_, rejectedReport, err := learnerIndividual.AdvanceGated(context.Background(), input, []float64{1, 1, 1})
			if err == nil {
				t.Fatal("public non-zero AdvanceGated combined an explicit gate with GateReceptor")
			}
			if rejectedReport != (learning.PlasticReport{}) {
				t.Fatalf("rejected AdvanceGated report = %+v, want zero", rejectedReport)
			}
			if !onlineReceptorPolicyValuesEqualBits(learnerIndividual.Snapshot(), beforeRejected) {
				t.Fatal("rejected AdvanceGated changed the complete snapshot")
			}
			onlineReceptorPolicyAssertChemistry(t, "rejected AdvanceGated", learnerIndividual.ChemistryReport(), beforeRejectedChemistry)
		})
	}
}

func TestOnlineReceptorPolicyFrozen(t *testing.T) {
	for _, core := range []string{onlineReceptorPolicyContinuous, onlineReceptorPolicyLIF, onlineReceptorPolicyMixed} {
		t.Run(core, func(t *testing.T) {
			a := onlineReceptorPolicyIndividual(t, core, true, true, true, .25)
			if err := a.FreezePlasticity(true); err != nil {
				t.Fatalf("FreezePlasticity(true): %v", err)
			}
			beforeAct := a.Snapshot()
			learner := onlineReceptorPolicyLearner(t, a, learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep})
			action, err := learner.Act(context.Background(), onlineReceptorPolicyObservation(t), onlineReceptorPolicyInput())
			if err != nil {
				t.Fatalf("frozen Act(): %v", err)
			}
			afterAct := a.Snapshot()
			if !onlineReceptorPolicyValuesEqualBits(afterAct.Plastic.State, beforeAct.Plastic.State) {
				t.Fatal("FreezePlasticity(true) did not hold the complete fast state during Act")
			}
			if onlineReceptorPolicyValuesEqualBits(afterAct.Neural, beforeAct.Neural) {
				t.Fatal("frozen Act() failed to advance the neural state")
			}
			if report := a.ChemistryReport(); report.Steps != 3 {
				t.Fatalf("frozen Act chemistry report = %+v, want three steps", report)
			}
			if err := learner.Receive(context.Background(), onlineReceptorPolicyFeedback(t, action.ActionID, "score", int64(action.Step), 1)); err != nil {
				t.Fatalf("Receive(): %v", err)
			}
			beforeUpdate := a.Snapshot()
			report, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(action.Step)))
			if err != nil {
				t.Fatalf("frozen Update(): %v", err)
			}
			if report.Consumed != 1 || report.PlasticSteps != 1 || report.GradientSteps != 0 {
				t.Fatalf("frozen Update report = %+v", report)
			}
			afterUpdate := a.Snapshot()
			if !onlineReceptorPolicyValuesEqualBits(afterUpdate.Plastic.State, beforeUpdate.Plastic.State) {
				t.Fatal("FreezePlasticity(true) did not hold the complete fast state during Update")
			}
			if onlineReceptorPolicyValuesEqualBits(afterUpdate.Neural, beforeUpdate.Neural) {
				t.Fatal("frozen Update() failed to advance the neural state")
			}
			if a.ChemistryReport().Steps != 3 {
				t.Fatalf("frozen Update chemistry report = %+v, want three steps", a.ChemistryReport())
			}
		})
	}
}

func TestOnlineReceptorPolicyErrorsAndRetry(t *testing.T) {
	for _, core := range []string{onlineReceptorPolicyContinuous, onlineReceptorPolicyLIF, onlineReceptorPolicyMixed} {
		for _, contextCase := range []struct {
			name string
			ctx  func() context.Context
		}{
			{name: "nil-context", ctx: func() context.Context { return nil }},
			{name: "canceled-context", ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}},
		} {
			t.Run(core+"/Act/"+contextCase.name, func(t *testing.T) {
				a := onlineReceptorPolicyIndividual(t, core, true, true, true, 0)
				learner := onlineReceptorPolicyLearner(t, a, learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep})
				before := a.Snapshot()
				beforeChemistry := a.ChemistryReport()
				if _, err := learner.Act(contextCase.ctx(), onlineReceptorPolicyObservation(t), onlineReceptorPolicyInput()); err == nil {
					t.Fatal("Act accepted an invalid context")
				}
				if !onlineReceptorPolicyValuesEqualBits(a.Snapshot(), before) {
					t.Fatal("failed Act changed the complete snapshot")
				}
				onlineReceptorPolicyAssertChemistry(t, "failed Act", a.ChemistryReport(), beforeChemistry)
				if got := learner.Log(); len(got) != 0 {
					t.Fatalf("failed Act appended to the log: %+v", got)
				}
			})
		}

		t.Run(core+"/Update/non-finite-reward-gate", func(t *testing.T) {
			a := onlineReceptorPolicyIndividual(t, core, true, true, true, 0)
			bad := true
			gate := learning.RewardGateFunc(func(score float64) ([]float64, error) {
				if bad {
					return []float64{math.NaN()}, nil
				}
				return []float64{score}, nil
			})
			learner, err := learning.NewOnlineLearner(a, learning.OnlinePolicy{AllowPlastic: true, ClockUnit: signal.TimeUnitModelStep}, gate)
			if err != nil {
				t.Fatalf("NewOnlineLearner(): %v", err)
			}
			action, err := learner.Act(context.Background(), onlineReceptorPolicyObservation(t), onlineReceptorPolicyInput())
			if err != nil {
				t.Fatalf("Act(): %v", err)
			}
			if err := learner.Receive(context.Background(), onlineReceptorPolicyFeedback(t, action.ActionID, "score", int64(action.Step), 1)); err != nil {
				t.Fatalf("Receive(): %v", err)
			}
			before := a.Snapshot()
			beforeChemistry := a.ChemistryReport()
			beforeLog := learner.Log()
			report, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(action.Step)))
			if err == nil {
				t.Fatal("Update accepted a non-finite reward gate")
			}
			if report != (learning.UpdateReport{}) {
				t.Fatalf("failed Update report = %+v, want zero", report)
			}
			if !onlineReceptorPolicyValuesEqualBits(a.Snapshot(), before) {
				t.Fatal("failed Update changed the complete snapshot")
			}
			onlineReceptorPolicyAssertChemistry(t, "failed Update", a.ChemistryReport(), beforeChemistry)
			if got := learner.Log(); !onlineReceptorPolicyValuesEqualBits(got, beforeLog) {
				t.Fatalf("failed Update changed action log: got=%+v want=%+v", got, beforeLog)
			}

			bad = false
			retry, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(action.Step)))
			if err != nil {
				t.Fatalf("legal retry Update(): %v", err)
			}
			if retry.Consumed != 1 || retry.PlasticSteps != 1 || retry.GradientSteps != 0 {
				t.Fatalf("legal retry report = %+v", retry)
			}
		})
	}
}

func TestOnlineReceptorPolicyEvaluate(t *testing.T) {
	for _, core := range []string{onlineReceptorPolicyContinuous, onlineReceptorPolicyLIF, onlineReceptorPolicyMixed} {
		t.Run(core, func(t *testing.T) {
			a := onlineReceptorPolicyIndividual(t, core, true, true, true, .25)
			policy := learning.OnlinePolicy{AllowGradient: true, AllowPlastic: true, Evaluate: true, ClockUnit: signal.TimeUnitModelStep}
			learner := onlineReceptorPolicyLearner(t, a, policy)
			action, err := learner.Act(context.Background(), onlineReceptorPolicyObservation(t), onlineReceptorPolicyInput())
			if err != nil {
				t.Fatalf("Act(): %v", err)
			}
			if err := learner.Receive(context.Background(), onlineReceptorPolicyFeedback(t, action.ActionID, "score", int64(action.Step), 1)); err != nil {
				t.Fatalf("Receive(): %v", err)
			}
			before := a.Snapshot()
			beforeChemistry := a.ChemistryReport()
			beforeLog := learner.Log()
			report, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(action.Step)))
			if !errors.Is(err, learning.ErrEvaluateMode) {
				t.Fatalf("Evaluate Update() error = %v, want ErrEvaluateMode", err)
			}
			if report != (learning.UpdateReport{}) {
				t.Fatalf("Evaluate Update report = %+v, want zero", report)
			}
			if !onlineReceptorPolicyValuesEqualBits(a.Snapshot(), before) {
				t.Fatal("Evaluate refusal changed the complete snapshot")
			}
			onlineReceptorPolicyAssertChemistry(t, "Evaluate refusal", a.ChemistryReport(), beforeChemistry)
			if got := learner.Log(); !onlineReceptorPolicyValuesEqualBits(got, beforeLog) {
				t.Fatalf("Evaluate refusal changed action log: got=%+v want=%+v", got, beforeLog)
			}
			if _, err := learner.Update(context.Background(), onlineReceptorPolicyAt(int64(action.Step))); !errors.Is(err, learning.ErrEvaluateMode) {
				t.Fatalf("second Evaluate Update() error = %v, want the queued feedback to remain", err)
			}
		})
	}
}
