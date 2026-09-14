// This file is an internal package test because continuing a LIF trajectory has
// to be compared against the package-private forward trace: the synaptic rows,
// the adaptation and the refractory counters are not exported.
package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

// lifChunkings splits the same twelve step sequence in four ways, including one
// step at a time and uneven parts, so the ring buffer is refilled at every
// possible offset.
var lifChunkings = [][]int{
	{12},
	{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	{5, 7},
	{3, 3, 3, 3},
}

// lifRandomFixture builds a small spiking graph. The first three edges take
// delays 0, 1 and 2 so every fixture exercises the whole delay range, and the
// remaining edges, endpoints, parameters and inputs come from the seed.
func lifRandomFixture(seed uint64, adapt bool, refractory int) (LIFConfig, LIFParameters, []float64, [][]float64) {
	rng := rand.New(rand.NewPCG(seed, seed+0x9e3779b97f4a7c15))
	n := 3 + rng.IntN(3)
	c := LIFConfig{
		Nodes:           n,
		DT:              0.4,
		TauSyn:          0.9,
		ThetaMin:        0.3,
		ThetaMax:        1.6,
		VReset:          -0.5,
		RefractorySteps: refractory,
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	if adapt {
		c.Adaptation = LIFAdaptation{Enabled: true, TauAdapt: 1.3, Beta: 0.4}
	}
	for e := range 2 * n {
		delay := e
		if e >= 3 {
			delay = rng.IntN(3)
		}
		c.Sources = append(c.Sources, rng.IntN(n))
		c.Targets = append(c.Targets, rng.IntN(n))
		c.Delays = append(c.Delays, delay)
	}
	p := LIFParameters{
		Weights:  make([]float64, len(c.Sources)),
		Bias:     make([]float64, n),
		LogTau:   make([]float64, n),
		ThetaRaw: make([]float64, n),
	}
	for e := range p.Weights {
		p.Weights[e] = 2*rng.Float64() - 0.5
	}
	initial := make([]float64, n)
	for i := range n {
		p.Bias[i] = 0.4*rng.Float64() - 0.1
		p.LogTau[i] = 0.5*rng.Float64() - 0.25
		p.ThetaRaw[i] = 2*rng.Float64() - 1
		initial[i] = 0.4*rng.Float64() - 0.2
	}
	inputs := make([][]float64, 12)
	for t := range inputs {
		inputs[t] = make([]float64, n)
		for i := range n {
			inputs[t][i] = 6 * rng.Float64()
		}
	}
	return c, p, initial, inputs
}

func lifCloneState(s LIFState) LIFState {
	return LIFState{
		SchemaVersion: s.SchemaVersion,
		ConfigHash:    s.ConfigHash,
		Steps:         s.Steps,
		Voltage:       append([]float64(nil), s.Voltage...),
		History:       cloneRows(s.History),
		Adaptation:    append([]float64(nil), s.Adaptation...),
		Refractory:    append([]int(nil), s.Refractory...),
	}
}

func lifMaxDelay(c LIFConfig) int {
	maxDelay := 0
	for _, d := range c.Delays {
		if d > maxDelay {
			maxDelay = d
		}
	}
	return maxDelay
}

// lifChainAdvance continues one state across the given chunk sizes and returns
// the concatenated outputs and spikes together with the final state.
func lifChainAdvance(t *testing.T, m *LIF, p LIFParameters, s LIFState, inputs [][]float64, chunks []int) (LIFState, [][]float64, [][]float64) {
	t.Helper()
	var outputs, spikes [][]float64
	done := 0
	for _, size := range chunks {
		next, out, spk, err := m.Advance(context.Background(), p, s, inputs[done:done+size])
		if err != nil {
			t.Fatalf("advance at step %d: %v", done, err)
		}
		if err := m.ValidateState(next); err != nil {
			t.Fatalf("continuation after step %d rejected: %v", done+size, err)
		}
		if next.Steps != s.Steps+uint64(size) {
			t.Fatalf("step counter %d, want %d", next.Steps, s.Steps+uint64(size))
		}
		s = next
		outputs = append(outputs, out...)
		spikes = append(spikes, spk...)
		done += size
	}
	if done != len(inputs) {
		t.Fatalf("chunks covered %d of %d steps", done, len(inputs))
	}
	return s, outputs, spikes
}

func TestLIFAdvanceMatchesForwardBitForBitAcrossChunkings(t *testing.T) {
	events := 0
	for _, seed := range []uint64{1, 2, 3, 5, 8} {
		for _, adapt := range []bool{false, true} {
			for _, refractory := range []int{0, 1, 2} {
				name := fmt.Sprintf("seed%d_adapt%v_refractory%d", seed, adapt, refractory)
				t.Run(name, func(t *testing.T) {
					c, p, initial, inputs := lifRandomFixture(seed, adapt, refractory)
					m, err := NewLIF(c)
					if err != nil {
						t.Fatal(err)
					}
					tr, err := m.Forward(context.Background(), p, initial, inputs)
					if err != nil {
						t.Fatal(err)
					}
					wantOutputs, wantSpikes := tr.Outputs(), tr.Spikes()
					fired := 0
					for _, row := range wantSpikes {
						for _, v := range row {
							fired += int(v)
						}
					}
					if fired == 0 {
						t.Fatal("fixture never spiked, the comparison would be vacuous")
					}
					events += fired

					start, err := m.NewState(initial)
					if err != nil {
						t.Fatal(err)
					}
					if start.Steps != 0 || len(start.History) != 1 {
						t.Fatalf("new state steps %d history %d rows", start.Steps, len(start.History))
					}
					for _, chunks := range lifChunkings {
						final, outputs, spikes := lifChainAdvance(t, m, p, start, inputs, chunks)
						if !reflect.DeepEqual(outputs, wantOutputs) {
							t.Fatalf("chunks %v: outputs differ from Forward", chunks)
						}
						if !reflect.DeepEqual(spikes, wantSpikes) {
							t.Fatalf("chunks %v: spikes differ from Forward", chunks)
						}
						if !reflect.DeepEqual(final.Voltage, tr.FinalVoltage()) {
							t.Fatalf("chunks %v: voltage %v want %v", chunks, final.Voltage, tr.FinalVoltage())
						}
						if !reflect.DeepEqual(final.Adaptation, tr.adapt[len(inputs)]) {
							t.Fatalf("chunks %v: adaptation %v want %v", chunks, final.Adaptation, tr.adapt[len(inputs)])
						}
						if !reflect.DeepEqual(final.Refractory, tr.refract[len(inputs)]) {
							t.Fatalf("chunks %v: refractory %v want %v", chunks, final.Refractory, tr.refract[len(inputs)])
						}
						rows := min(len(inputs), lifMaxDelay(c)) + 1
						wantHistory := cloneRows(tr.syn[len(inputs)+1-rows:])
						if !reflect.DeepEqual(final.History, wantHistory) {
							t.Fatalf("chunks %v: history %v want %v", chunks, final.History, wantHistory)
						}
					}
				})
			}
		}
	}
	if events < 100 {
		t.Fatalf("only %d spikes across every fixture", events)
	}
}

func TestLIFAdvanceReproducesHandCalculatedTimingInTwoChunks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		adapt      bool
		spikes     [][]float64
		outputs    [][]float64
		voltage    []float64
		adaptation []float64
		refractory []int
	}{
		{
			name:       "adaptation_off",
			spikes:     [][]float64{{1, 0, 0}, {0, 1, 0}, {1, 0, 0}},
			outputs:    [][]float64{{1, 0, 0}, {.5, 1, 0}, {1.25, .5, 0}},
			voltage:    []float64{-1, -1, 0},
			adaptation: []float64{0, 0, 0},
			refractory: []int{1, 0, 0},
		},
		{
			name:       "adaptation_on",
			adapt:      true,
			spikes:     [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 0}},
			outputs:    [][]float64{{1, 0, 0}, {.5, 1, 0}, {.25, .5, 0}},
			voltage:    []float64{1.1, -1, 0},
			adaptation: []float64{.125, .25, 0},
			refractory: []int{0, 0, 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewLIF(lifHandConfig(tc.adapt))
			if err != nil {
				t.Fatal(err)
			}
			p, initial, inputs := lifHandInputs()
			start, err := m.NewState(initial)
			if err != nil {
				t.Fatal(err)
			}
			final, outputs, spikes := lifChainAdvance(t, m, p, start, inputs[:3], []int{1, 2})
			lifRows(t, "spikes", spikes, tc.spikes, lifTol)
			lifRows(t, "outputs", outputs, tc.outputs, lifTol)
			lifRows(t, "voltage", [][]float64{final.Voltage}, [][]float64{tc.voltage}, lifTol)
			lifRows(t, "adaptation", [][]float64{final.Adaptation}, [][]float64{tc.adaptation}, lifTol)
			if !reflect.DeepEqual(final.Refractory, tc.refractory) {
				t.Fatalf("refractory %v want %v", final.Refractory, tc.refractory)
			}
			// maxDelay is 2, so three steps retain the traces of times 1, 2 and 3.
			lifRows(t, "history", final.History, tc.outputs, lifTol)
			if final.Steps != 3 {
				t.Fatalf("steps %d", final.Steps)
			}
		})
	}
}

// lifValidStateFixture returns a model and a state advanced by one step, so the
// history, adaptation and refractory counters are all populated.
func lifValidStateFixture(t *testing.T) (*LIF, LIFParameters, [][]float64, LIFState) {
	t.Helper()
	m, err := NewLIF(lifHandConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	p, initial, inputs := lifHandInputs()
	start, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	valid, _, _, err := m.Advance(context.Background(), p, start, inputs[:1])
	if err != nil {
		t.Fatal(err)
	}
	return m, p, inputs, valid
}

func TestLIFNewStateAndValidateStateContract(t *testing.T) {
	m, _, _, valid := lifValidStateFixture(t)
	if err := m.ValidateState(valid); err != nil {
		t.Fatalf("rejected a state it produced: %v", err)
	}
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var decoded LIFState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, valid) {
		t.Fatalf("json round trip changed the state: %#v", decoded)
	}
	if err := m.ValidateState(decoded); err != nil {
		t.Fatalf("rejected a decoded state: %v", err)
	}

	fresh, err := m.NewState([]float64{0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.SchemaVersion != LIFStateVersion {
		t.Fatalf("schema version %q", fresh.SchemaVersion)
	}
	sum := sha256.Sum256(func() []byte { b, _ := json.Marshal(m.Config()); return b }())
	if fresh.ConfigHash != fmt.Sprintf("%x", sum) {
		t.Fatalf("config hash %q", fresh.ConfigHash)
	}
	zeros := [][]float64{{0, 0, 0}}
	if !reflect.DeepEqual(fresh.History, zeros) || !reflect.DeepEqual(fresh.Adaptation, []float64{0, 0, 0}) || !reflect.DeepEqual(fresh.Refractory, []int{0, 0, 0}) {
		t.Fatalf("new state is not a zero prehistory: %#v", fresh)
	}
	if err := m.ValidateState(fresh); err != nil {
		t.Fatal(err)
	}

	// A different tau_syn changes the canonical config JSON, so the fingerprint
	// of a state from one model must not be accepted by the other.
	otherConfig := lifHandConfig(true)
	otherConfig.TauSyn = 2
	other, err := NewLIF(otherConfig)
	if err != nil {
		t.Fatal(err)
	}
	otherState, err := other.NewState([]float64{0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if otherState.ConfigHash == fresh.ConfigHash {
		t.Fatal("different tau_syn produced the same fingerprint")
	}
	if err := other.ValidateState(valid); err == nil {
		t.Fatal("accepted a state built for another configuration")
	}
}

func TestLIFValidateStateRejectsEachDefectWithItsOwnMessage(t *testing.T) {
	m, p, inputs, valid := lifValidStateFixture(t)
	disabled, err := NewLIF(lifHandConfig(false))
	if err != nil {
		t.Fatal(err)
	}
	disabledState, _, _, err := disabled.Advance(context.Background(), func() LIFParameters { q, _, _ := lifHandInputs(); return q }(), func() LIFState {
		s, err := disabled.NewState([]float64{0, 0, 0})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}(), inputs[:1])
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		model *LIF
		state LIFState
		edit  func(*LIFState)
	}{
		{name: "schema", edit: func(s *LIFState) { s.SchemaVersion = "coimnet-lif-state/v0" }},
		{name: "fingerprint", edit: func(s *LIFState) { s.ConfigHash = strings.Repeat("0", sha256.Size*2) }},
		{name: "voltage length", edit: func(s *LIFState) { s.Voltage = s.Voltage[:1] }},
		{name: "voltage nonfinite", edit: func(s *LIFState) { s.Voltage[0] = math.Inf(1) }},
		{name: "missing history", edit: func(s *LIFState) { s.History = nil }},
		{name: "history row count", edit: func(s *LIFState) { s.Steps += 2 }},
		{name: "history row length", edit: func(s *LIFState) { s.History[0] = s.History[0][:1] }},
		{name: "history nonfinite", edit: func(s *LIFState) { s.History[0][0] = math.NaN() }},
		{name: "history negative", edit: func(s *LIFState) { s.History[0][0] = -1e-9 }},
		{name: "history above decayed sum", edit: func(s *LIFState) { s.History[0][0] = 1e3 }},
		{name: "adaptation length", edit: func(s *LIFState) { s.Adaptation = s.Adaptation[:1] }},
		{name: "adaptation nonfinite", edit: func(s *LIFState) { s.Adaptation[0] = math.NaN() }},
		{name: "adaptation negative", edit: func(s *LIFState) { s.Adaptation[0] = -0.5 }},
		{name: "refractory length", edit: func(s *LIFState) { s.Refractory = s.Refractory[:1] }},
		{name: "refractory range", edit: func(s *LIFState) { s.Refractory[0] = m.config.RefractorySteps + 1 }},
		{name: "refractory negative", edit: func(s *LIFState) { s.Refractory[0] = -1 }},
		{
			name:  "adaptation while disabled",
			model: disabled,
			state: disabledState,
			edit:  func(s *LIFState) { s.Adaptation[0] = 0.25 },
		},
	}
	messages := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, source := m, valid
			if tc.model != nil {
				model, source = tc.model, tc.state
			}
			bad := lifCloneState(source)
			tc.edit(&bad)
			err := model.ValidateState(bad)
			if err == nil {
				t.Fatal("accepted a malformed state")
			}
			if previous, seen := messages[err.Error()]; seen {
				t.Fatalf("message %q is shared with %q", err.Error(), previous)
			}
			messages[err.Error()] = tc.name
			state, outputs, spikes, advanceErr := model.Advance(context.Background(), p, bad, inputs[:1])
			if advanceErr == nil || !reflect.DeepEqual(state, LIFState{}) || outputs != nil || spikes != nil {
				t.Fatalf("advance on a malformed state: %#v %v %v %v", state, outputs, spikes, advanceErr)
			}
		})
	}
}

func TestLIFStateRejectsZeroModels(t *testing.T) {
	_, p, inputs, valid := lifValidStateFixture(t)
	for name, zero := range map[string]*LIF{"nil": nil, "zero": new(LIF)} {
		t.Run(name, func(t *testing.T) {
			if _, err := zero.NewState([]float64{0, 0, 0}); err == nil {
				t.Fatal("NewState accepted a zero model")
			}
			if err := zero.ValidateState(valid); err == nil {
				t.Fatal("ValidateState accepted a zero model")
			}
			state, outputs, spikes, err := zero.Advance(context.Background(), p, valid, inputs[:1])
			if err == nil || !reflect.DeepEqual(state, LIFState{}) || outputs != nil || spikes != nil {
				t.Fatalf("zero model result %#v %v %v %v", state, outputs, spikes, err)
			}
		})
	}
	m, err := NewLIF(lifHandConfig(false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.NewState([]float64{0, 0}); err == nil {
		t.Fatal("NewState accepted a wrong initial voltage length")
	}
	if _, err := m.NewState([]float64{0, math.NaN(), 0}); err == nil {
		t.Fatal("NewState accepted a non-finite initial voltage")
	}
}

func TestLIFAdvanceRejectsInvalidArgumentsWithoutTouchingThem(t *testing.T) {
	m, p, inputs, valid := lifValidStateFixture(t)
	zero := LIFState{}
	reject := func(t *testing.T, name string, q LIFParameters, s LIFState, in [][]float64, ctx context.Context) {
		t.Helper()
		state, outputs, spikes, err := m.Advance(ctx, q, s, in)
		if err == nil || !reflect.DeepEqual(state, zero) || outputs != nil || spikes != nil {
			t.Fatalf("%s: %#v %v %v %v", name, state, outputs, spikes, err)
		}
	}
	background := context.Background()
	for _, tc := range []struct {
		name   string
		inputs [][]float64
	}{
		{"nil sequence", nil},
		{"empty sequence", [][]float64{}},
		{"narrow row", [][]float64{{0, 0}}},
		{"wide row", [][]float64{{0, 0, 0, 0}}},
		{"nil row", [][]float64{nil}},
		{"nan input", [][]float64{{0, math.NaN(), 0}}},
		{"inf input", [][]float64{{0, 0, math.Inf(-1)}}},
		{"late bad row", [][]float64{{0, 0, 0}, {0, math.NaN(), 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) { reject(t, tc.name, p, valid, tc.inputs, background) })
	}
	for _, tc := range []struct {
		name string
		edit func(*LIFParameters)
	}{
		{"weights length", func(q *LIFParameters) { q.Weights = q.Weights[:1] }},
		{"bias length", func(q *LIFParameters) { q.Bias = q.Bias[:1] }},
		{"log_tau length", func(q *LIFParameters) { q.LogTau = q.LogTau[:1] }},
		{"theta_raw length", func(q *LIFParameters) { q.ThetaRaw = q.ThetaRaw[:1] }},
		{"weights nonfinite", func(q *LIFParameters) { q.Weights[0] = math.NaN() }},
		{"log_tau overflows tau", func(q *LIFParameters) { q.LogTau[0] = 1000 }},
		{"log_tau underflows tau", func(q *LIFParameters) { q.LogTau[0] = -1000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := cloneLIFParameters(p)
			tc.edit(&bad)
			reject(t, tc.name, bad, valid, inputs[:1], background)
		})
	}
	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reject(t, "cancelled", p, valid, inputs[:1], ctx)
	})
	t.Run("nil context", func(t *testing.T) { reject(t, "nil context", p, valid, inputs[:1], nil) })
	t.Run("smooth reference mode", func(t *testing.T) {
		smooth, err := NewLIF(lifHandConfig(true))
		if err != nil {
			t.Fatal(err)
		}
		smooth.smooth = true
		state, outputs, spikes, err := smooth.Advance(background, p, valid, inputs[:1])
		if err == nil || !reflect.DeepEqual(state, zero) || outputs != nil || spikes != nil {
			t.Fatalf("smooth mode result %#v %v %v %v", state, outputs, spikes, err)
		}
	})

	t.Run("arguments unchanged", func(t *testing.T) {
		before := lifCloneState(valid)
		beforeInputs := cloneRows(inputs)
		beforeParams := cloneLIFParameters(p)
		reject(t, "nan input", p, valid, [][]float64{{0, math.NaN(), 0}}, background)
		if !reflect.DeepEqual(valid, before) || !reflect.DeepEqual(inputs, beforeInputs) || !reflect.DeepEqual(p, beforeParams) {
			t.Fatal("a rejected advance changed its arguments")
		}
	})
}

func TestLIFAdvanceRejectsOverflowAndCapacityBeforeAllocating(t *testing.T) {
	m, err := NewLIF(LIFConfig{
		Nodes: 2, DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 2, VReset: -1,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := LIFParameters{Bias: []float64{0, 0}, LogTau: []float64{0, 0}, ThetaRaw: []float64{0, 0}}
	state, err := m.NewState([]float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	overflow := lifCloneState(state)
	overflow.Steps = math.MaxUint64 - 1
	if err := m.ValidateState(overflow); err != nil {
		t.Fatal(err)
	}
	if s, out, spk, err := m.Advance(context.Background(), p, overflow, [][]float64{{0, 0}, {0, 0}}); err == nil || !reflect.DeepEqual(s, LIFState{}) || out != nil || spk != nil {
		t.Fatalf("step overflow result %#v %v %v %v", s, out, spk, err)
	}
	tooMany := make([][]float64, MaxStateValues/2+1)
	if s, out, spk, err := m.Advance(context.Background(), p, state, tooMany); err == nil || !reflect.DeepEqual(s, LIFState{}) || out != nil || spk != nil {
		t.Fatalf("output capacity result %#v %v %v %v", s, out, spk, err)
	}

	const nodes = MaxStateValues / 1024
	wide, err := NewLIF(LIFConfig{
		Nodes: nodes, Sources: []int{0}, Targets: []int{0}, Delays: []int{nodes},
		DT: 1, TauSyn: 1, ThetaMin: .5, ThetaMax: 2, VReset: -1,
		Surrogate: LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	wideP := LIFParameters{Weights: []float64{0}, Bias: make([]float64, nodes), LogTau: make([]float64, nodes), ThetaRaw: make([]float64, nodes)}
	wideState, err := wide.NewState(make([]float64, nodes))
	if err != nil {
		t.Fatal(err)
	}
	wideState.Steps = MaxStateValues/uint64(nodes) - 1
	wideState.History = make([][]float64, MaxStateValues/uint64(nodes))
	for i := range wideState.History {
		wideState.History[i] = make([]float64, nodes)
	}
	if err := wide.ValidateState(wideState); err != nil {
		t.Fatal(err)
	}
	if s, out, spk, err := wide.Advance(context.Background(), wideP, wideState, [][]float64{make([]float64, nodes)}); err == nil || !reflect.DeepEqual(s, LIFState{}) || out != nil || spk != nil {
		t.Fatalf("history capacity result %#v %v %v %v", s, out, spk, err)
	}
}

func TestLIFAdvanceIsDeterministicAndOwnsItsResults(t *testing.T) {
	c, p, initial, inputs := lifRandomFixture(11, true, 2)
	m, err := NewLIF(c)
	if err != nil {
		t.Fatal(err)
	}
	start, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	first, outputs, spikes, err := m.Advance(context.Background(), p, start, inputs[:4])
	if err != nil {
		t.Fatal(err)
	}
	second, outputs2, spikes2, err := m.Advance(context.Background(), p, start, inputs[:4])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(outputs, outputs2) || !reflect.DeepEqual(spikes, spikes2) {
		t.Fatal("identical calls produced different results")
	}

	firstCopy := lifCloneState(first)
	outputs[0][0] = 999
	spikes[0][0] = 999
	// The last trace is inside the retained delay window, so it is the row a
	// shared buffer would expose.
	outputs[len(outputs)-1][0] = 999
	if !reflect.DeepEqual(first, firstCopy) {
		t.Fatal("returned outputs alias the returned state")
	}
	first.History[len(first.History)-1][0] = 888
	first.Voltage[0] = 888
	first.Adaptation[0] = 888
	continued, out3, spk3, err := m.Advance(context.Background(), p, firstCopy, inputs[4:6])
	if err != nil {
		t.Fatal(err)
	}
	continuedCopy := lifCloneState(continued)
	out4, spk4 := cloneRows(out3), cloneRows(spk3)
	firstCopy.History[0][0] = 777
	again, out5, spk5, err := m.Advance(context.Background(), p, lifCloneState(continuedCopy), inputs[6:8])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out3, out4) || !reflect.DeepEqual(spk3, spk4) {
		t.Fatal("a later advance rewrote earlier results")
	}
	if len(out5) != 2 || len(spk5) != 2 {
		t.Fatalf("continuation sizes %d %d", len(out5), len(spk5))
	}
	if !reflect.DeepEqual(continued, continuedCopy) {
		t.Fatal("a later advance changed an earlier state")
	}
	if again.Steps != 8 {
		t.Fatalf("steps %d", again.Steps)
	}
}
