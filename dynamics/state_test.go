package dynamics_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func stateFixture(t *testing.T, activation string) (*dynamics.Continuous, dynamics.Parameters, []float64, [][]float64) {
	t.Helper()
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes:      3,
		Sources:    []int{0, 1, 2, 1},
		Targets:    []int{1, 2, 0, 0},
		Delays:     []int{0, 1, 3, 2},
		DT:         0.3,
		Activation: activation,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{
		Weights: []float64{0.2, -0.15, 0.1, 0.05},
		Bias:    []float64{0.1, -0.2, 0.05},
		LogTau:  []float64{0.2, -0.1, 0.3},
	}
	initial := []float64{0.2, -0.3, 0.1}
	inputs := [][]float64{
		{0.1, 0.3, -0.2},
		{-0.2, 0.4, 0.1},
		{0.5, -0.1, 0.2},
		{0.2, 0.2, -0.4},
		{-0.3, 0.1, 0.3},
		{0.1, -0.5, 0.2},
		{0.4, 0.2, 0.1},
	}
	return m, p, initial, inputs
}

func referenceActivation(name string, v float64) float64 {
	if name == "tanh" {
		return math.Tanh(v)
	}
	return math.Max(v, 0) + math.Log1p(math.Exp(-math.Abs(v)))
}

func referenceAdvance(c dynamics.Config, p dynamics.Parameters, voltage []float64, inputs [][]float64) ([][]float64, []float64, [][]float64) {
	n := c.Nodes
	maxDelay := 0
	for _, d := range c.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	outputs := make([][]float64, len(inputs)+1)
	outputs[0] = make([]float64, n)
	for i, v := range voltage {
		outputs[0][i] = referenceActivation(c.Activation, v)
	}
	all := make([][]float64, 0, len(inputs))
	for t, in := range inputs {
		drive := append([]float64(nil), in...)
		for e, source := range c.Sources {
			past := 0
			if c.Delays[e] < t {
				past = t - c.Delays[e]
			}
			drive[c.Targets[e]] += p.Weights[e] * outputs[past][source]
		}
		next := make([]float64, n)
		out := make([]float64, n)
		tau := make([]float64, n)
		for i := range p.LogTau {
			tau[i] = math.Exp(p.LogTau[i])
			lambda := math.Exp(-c.DT / tau[i])
			alpha := -math.Expm1(-c.DT / tau[i])
			drive[i] += p.Bias[i]
			next[i] = lambda*voltage[i] + alpha*drive[i]
			out[i] = referenceActivation(c.Activation, next[i])
		}
		voltage = next
		outputs[uint64(t)+1] = out
		all = append(all, out)
	}
	start := uint64(0)
	if uint64(len(inputs)) > uint64(maxDelay) {
		start = uint64(len(inputs)) - uint64(maxDelay)
	}
	history := make([][]float64, 0, len(outputs))
	for i := start; i < uint64(len(outputs)); i++ {
		history = append(history, append([]float64(nil), outputs[i]...))
	}
	return all, voltage, history
}

func assertStateRowsClose(t *testing.T, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("row count %d, want %d", len(got), len(want))
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("row %d length %d, want %d", i, len(got[i]), len(want[i]))
		}
		for j := range got[i] {
			if math.Abs(got[i][j]-want[i][j]) > 2e-14*(1+math.Abs(want[i][j])) {
				t.Fatalf("row %d value %d: got %.17g want %.17g", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func assertStateClose(t *testing.T, got, want dynamics.State) {
	t.Helper()
	if got.SchemaVersion != want.SchemaVersion || got.ConfigHash != want.ConfigHash || got.Steps != want.Steps {
		t.Fatalf("metadata got %#v want %#v", got, want)
	}
	if len(got.Voltage) != len(want.Voltage) {
		t.Fatalf("voltage length %d, want %d", len(got.Voltage), len(want.Voltage))
	}
	for i := range got.Voltage {
		if math.Abs(got.Voltage[i]-want.Voltage[i]) > 2e-14*(1+math.Abs(want.Voltage[i])) {
			t.Fatalf("voltage %d: got %.17g want %.17g", i, got.Voltage[i], want.Voltage[i])
		}
	}
	assertStateRowsClose(t, got.History, want.History)
}

func appendRows(dst [][]float64, src [][]float64) [][]float64 {
	for _, row := range src {
		dst = append(dst, append([]float64(nil), row...))
	}
	return dst
}

func TestContinuousStateFullAndSplitAdvanceMatchIndependentReference(t *testing.T) {
	for _, activation := range []string{"tanh", "softplus"} {
		t.Run(activation, func(t *testing.T) {
			m, p, initial, inputs := stateFixture(t, activation)
			state, err := m.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			if state.SchemaVersion != dynamics.ContinuousStateVersion {
				t.Fatalf("schema version %q", state.SchemaVersion)
			}
			encoded, err := json.Marshal(m.Config())
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(encoded)
			if state.ConfigHash != hex.EncodeToString(sum[:]) {
				t.Fatalf("config hash %q", state.ConfigHash)
			}

			wantOutputs, wantVoltage, wantHistory := referenceAdvance(m.Config(), p, initial, inputs)
			full, fullOutputs, err := m.Advance(context.Background(), p, state, inputs)
			if err != nil {
				t.Fatal(err)
			}
			assertStateRowsClose(t, fullOutputs, wantOutputs)
			legacy, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			assertStateRowsClose(t, fullOutputs, legacy.Outputs())
			assertStateClose(t, full, dynamics.State{SchemaVersion: state.SchemaVersion, ConfigHash: state.ConfigHash, Steps: uint64(len(inputs)), Voltage: wantVoltage, History: wantHistory})

			var splitState = state
			var splitOutputs [][]float64
			for _, boundary := range []int{1, 3, 4, len(inputs)} {
				part, outputs, err := m.Advance(context.Background(), p, splitState, inputs[len(splitOutputs):boundary])
				if err != nil {
					t.Fatal(err)
				}
				splitState = part
				splitOutputs = appendRows(splitOutputs, outputs)
			}
			assertStateRowsClose(t, splitOutputs, wantOutputs)
			assertStateClose(t, splitState, full)
			if err := m.ValidateState(splitState); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContinuousStateDelayZeroUsesPriorStepAndHistoryIsBounded(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{0}, DT: 1, Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Weights: []float64{0.5}, Bias: []float64{0}, LogTau: []float64{0}}
	state, err := m.NewState([]float64{0})
	if err != nil {
		t.Fatal(err)
	}
	first, out, err := m.Advance(context.Background(), p, state, [][]float64{{1}, {0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.History) != 1 || len(out) != 3 {
		t.Fatalf("history/output sizes: %d/%d", len(first.History), len(out))
	}
	if math.Abs(out[0][0]-math.Tanh(1-math.Exp(-1))) > 1e-12 {
		t.Fatalf("first output %.17g", out[0][0])
	}
	v1 := 1 - math.Exp(-1)
	wantV2 := math.Exp(-1)*v1 + (1-math.Exp(-1))*0.5*math.Tanh(v1)
	if math.Abs(out[1][0]-math.Tanh(wantV2)) > 1e-12 {
		t.Fatalf("second output %.17g", out[1][0])
	}
	if first.History[0][0] != out[len(out)-1][0] {
		t.Fatalf("latest history %v, latest output %v", first.History[0], out[len(out)-1])
	}
}

func TestContinuousStateHugeDelayDoesNotPreallocateDelayHistory(t *testing.T) {
	const hugeDelay = 1 << 30
	m, err := dynamics.NewContinuous(dynamics.Config{
		Nodes: 1, Sources: []int{0}, Targets: []int{0}, Delays: []int{hugeDelay}, DT: 1, Activation: "tanh",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Weights: []float64{0.5}, Bias: []float64{0}, LogTau: []float64{0}}
	state, err := m.NewState([]float64{0})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 1 || cap(state.History) > 2 {
		t.Fatalf("new state allocated delay-sized history: len=%d cap=%d", len(state.History), cap(state.History))
	}
	state, outputs, err := m.Advance(context.Background(), p, state, [][]float64{{1}, {0}, {0}, {0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 5 || len(outputs) != 4 {
		t.Fatalf("early history/output sizes: %d/%d", len(state.History), len(outputs))
	}
	if state.Steps != 4 {
		t.Fatalf("steps %d", state.Steps)
	}
}

func TestContinuousStateClonesInputsParametersAndSnapshots(t *testing.T) {
	m, p, initial, inputs := stateFixture(t, "tanh")
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	originalState := cloneStateForTest(state)
	originalP := dynamics.Parameters{Weights: append([]float64(nil), p.Weights...), Bias: append([]float64(nil), p.Bias...), LogTau: append([]float64(nil), p.LogTau...)}
	originalInputs := cloneTestRows(inputs)
	advanced, outputs, err := m.Advance(context.Background(), p, state, inputs[:2])
	if err != nil {
		t.Fatal(err)
	}
	inputs[0][0] = 999
	p.Weights[0] = 999
	initial[0] = 999
	outputs[0][0] = 999
	if !reflect.DeepEqual(state, originalState) {
		t.Fatal("advance changed input state")
	}
	if p.Weights[0] != 999 {
		t.Fatal("test setup did not mutate parameters")
	}
	if inputs[0][0] != 999 || initial[0] != 999 {
		t.Fatal("test setup did not mutate caller buffers")
	}
	if advanced.History[0][0] == 999 {
		t.Fatal("outputs and state history share a mutable element")
	}

	advancedBeforeSecond := cloneStateForTest(advanced)
	second, _, err := m.Advance(context.Background(), originalP, advancedBeforeSecond, originalInputs[2:3])
	if err != nil {
		t.Fatal(err)
	}
	advancedBeforeSecond.History[0][0] = 888
	if second.History[0][0] == 888 {
		t.Fatal("snapshot rows were aliased")
	}
}

func cloneTestRows(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i := range rows {
		out[i] = append([]float64(nil), rows[i]...)
	}
	return out
}

func TestContinuousStateRejectsInvalidStatesAndConfigurations(t *testing.T) {
	m, p, initial, inputs := stateFixture(t, "tanh")
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	valid, _, err := m.Advance(context.Background(), p, state, inputs[:1])
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name string
		edit func(*dynamics.State)
	}{
		{"schema", func(s *dynamics.State) { s.SchemaVersion = "wrong" }},
		{"hash", func(s *dynamics.State) { s.ConfigHash = strings.Repeat("0", sha256.Size*2) }},
		{"voltage length", func(s *dynamics.State) { s.Voltage = s.Voltage[:1] }},
		{"missing history", func(s *dynamics.State) { s.History = nil }},
		{"history row length", func(s *dynamics.State) { s.History[0] = s.History[0][:1] }},
		{"history nil row", func(s *dynamics.State) { s.History[0] = nil }},
		{"history nonfinite", func(s *dynamics.State) { s.History[0][0] = math.NaN() }},
		{"history tanh range", func(s *dynamics.State) { s.History[0][0] = 2 }},
		{"latest mismatch", func(s *dynamics.State) { s.History[len(s.History)-1][0] += 0.1 }},
		{"steps/history mismatch", func(s *dynamics.State) { s.Steps++ }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			bad := cloneStateForTest(valid)
			tc.edit(&bad)
			if err := m.ValidateState(bad); err == nil {
				t.Fatal("accepted malformed state")
			}
			got, outputs, err := m.Advance(context.Background(), p, bad, inputs[:1])
			if err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
				t.Fatalf("bad state result %#v %v %v", got, outputs, err)
			}
		})
	}

	other, err := dynamics.NewContinuous(dynamics.Config{Nodes: 3, Sources: []int{0}, Targets: []int{1}, DT: .3, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	if err := other.ValidateState(valid); err == nil {
		t.Fatal("accepted state from another topology")
	}
	if got, outputs, err := other.Advance(context.Background(), p, valid, inputs[:1]); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("wrong topology result %#v %v %v", got, outputs, err)
	}
}

func cloneStateForTest(s dynamics.State) dynamics.State {
	return dynamics.State{
		SchemaVersion: s.SchemaVersion,
		ConfigHash:    s.ConfigHash,
		Steps:         s.Steps,
		Voltage:       append([]float64(nil), s.Voltage...),
		History:       cloneTestRows(s.History),
	}
}

func TestContinuousStateRejectsMalformedInputsParametersAndZeroModels(t *testing.T) {
	m, p, initial, inputs := stateFixture(t, "softplus")
	state, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	parameterCases := []dynamics.Parameters{
		{Weights: []float64{0}, Bias: p.Bias, LogTau: p.LogTau},
		{Weights: p.Weights, Bias: p.Bias[:1], LogTau: p.LogTau},
		{Weights: p.Weights, Bias: p.Bias, LogTau: []float64{math.NaN(), 0, 0}},
		{Weights: p.Weights, Bias: p.Bias, LogTau: []float64{1000, 0, 0}},
		{Weights: p.Weights, Bias: p.Bias, LogTau: []float64{-1000, 0, 0}},
	}
	for i, bad := range parameterCases {
		if got, outputs, err := m.Advance(context.Background(), bad, state, inputs[:1]); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
			t.Fatalf("parameters case %d result %#v %v %v", i, got, outputs, err)
		}
	}
	for i, badInputs := range [][][]float64{nil, {}, {{0, 1}}, {{0, math.Inf(1), 0}}, {{0, math.NaN(), 0}}} {
		if got, outputs, err := m.Advance(context.Background(), p, state, badInputs); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
			t.Fatalf("inputs case %d result %#v %v %v", i, got, outputs, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, outputs, err := m.Advance(ctx, p, state, inputs[:1]); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("canceled result %#v %v %v", got, outputs, err)
	}
	if got, outputs, err := m.Advance(nil, p, state, inputs[:1]); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("nil context result %#v %v %v", got, outputs, err)
	}
	for name, zero := range map[string]*dynamics.Continuous{"nil": nil, "zero": new(dynamics.Continuous)} {
		t.Run(name, func(t *testing.T) {
			if _, err := zero.NewState([]float64{0}); err == nil {
				t.Fatal("NewState accepted zero model")
			}
			if err := zero.ValidateState(state); err == nil {
				t.Fatal("ValidateState accepted zero model")
			}
			if got, outputs, err := zero.Advance(context.Background(), p, state, inputs[:1]); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
				t.Fatalf("zero model result %#v %v %v", got, outputs, err)
			}
		})
	}
}

func TestContinuousStateFailureAtomicOnNonfiniteResultAndStepOverflow(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Bias: []float64{math.MaxFloat64}, LogTau: []float64{0}}
	state, err := m.NewState([]float64{0})
	if err != nil {
		t.Fatal(err)
	}
	before := cloneStateForTest(state)
	if got, outputs, err := m.Advance(context.Background(), p, state, [][]float64{{math.MaxFloat64}}); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("nonfinite result %#v %v %v", got, outputs, err)
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatal("failed advance changed source state")
	}
	overflow := cloneStateForTest(state)
	overflow.Steps = ^uint64(0)
	if got, outputs, err := m.Advance(context.Background(), dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}}, overflow, [][]float64{{0}}); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("step overflow result %#v %v %v", got, outputs, err)
	}
}

func TestContinuousStateCapacityChecksBeforeAllocatingOutputsOrHistory(t *testing.T) {
	m, err := dynamics.NewContinuous(dynamics.Config{Nodes: 2, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	p := dynamics.Parameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}}
	state, err := m.NewState([]float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	tooManyOutputs := make([][]float64, dynamics.MaxStateValues/2+1)
	if got, outputs, err := m.Advance(context.Background(), p, state, tooManyOutputs); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("per-call capacity result %#v %v %v", got, outputs, err)
	}

	const nodes = dynamics.MaxStateValues / 1024
	wide, err := dynamics.NewContinuous(dynamics.Config{Nodes: nodes, Sources: []int{0}, Targets: []int{0}, Delays: []int{nodes}, DT: 1, Activation: "tanh"})
	if err != nil {
		t.Fatal(err)
	}
	wideP := dynamics.Parameters{Weights: []float64{0}, Bias: make([]float64, nodes), LogTau: make([]float64, nodes)}
	wideState, err := wide.NewState(make([]float64, nodes))
	if err != nil {
		t.Fatal(err)
	}
	wideState.Steps = dynamics.MaxStateValues/uint64(nodes) - 1
	wideState.History = make([][]float64, dynamics.MaxStateValues/uint64(nodes))
	for i := range wideState.History {
		wideState.History[i] = make([]float64, nodes)
	}
	if err := wide.ValidateState(wideState); err != nil {
		t.Fatal(err)
	}
	if got, outputs, err := wide.Advance(context.Background(), wideP, wideState, [][]float64{make([]float64, nodes)}); err == nil || !reflect.DeepEqual(got, dynamics.State{}) || outputs != nil {
		t.Fatalf("history capacity result %#v %v %v", got, outputs, err)
	}
}
