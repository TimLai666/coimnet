package dynamics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"
)

func assertBitEqual(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("value %d: got %.17g want %.17g", i, got[i], want[i])
		}
	}
}

func assertBitEqualRows(t *testing.T, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("row count %d, want %d", len(got), len(want))
	}
	for i := range got {
		assertBitEqual(t, got[i], want[i])
	}
}

func assertCloseGrid(t *testing.T, got, want []float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length %d, want %d", len(got), len(want))
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol*(1+math.Abs(want[i])) {
			t.Fatalf("value %d: got %.12g want %.12g", i, got[i], want[i])
		}
	}
}

func TestVectorContinuousC1MatchesContinuousBitForBit(t *testing.T) {
	cfg := Config{Nodes: 3, Sources: []int{0, 1, 2, 1}, Targets: []int{1, 2, 0, 0}, Delays: []int{0, 1, 3, 2}, DT: 0.3, Activation: "tanh"}
	vec, err := NewVectorContinuous(Config{Nodes: 3, Sources: []int{0, 1, 2, 1}, Targets: []int{1, 2, 0, 0}, Delays: []int{0, 1, 3, 2}, DT: 0.3, Activation: "tanh", StateDimension: 1, EdgeShape: "scalar"})
	if err != nil {
		t.Fatal(err)
	}
	sca, err := NewContinuous(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{Weights: []float64{0.2, -0.15, 0.1, 0.05}, Bias: []float64{0.1, -0.2, 0.05}, LogTau: []float64{0.2, -0.1, 0.3}}
	sp := Parameters{Weights: []float64{0.2, -0.15, 0.1, 0.05}, Bias: []float64{0.1, -0.2, 0.05}, LogTau: []float64{0.2, -0.1, 0.3}}
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

	vtr, err := vec.Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	ctr, err := sca.Forward(context.Background(), sp, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vtr.Outputs()) != len(inputs)+1 || len(vtr.Voltages()) != len(inputs)+1 {
		t.Fatalf("vector trace sizes: outputs %d voltages %d, want %d+1", len(vtr.Outputs()), len(vtr.Voltages()), len(inputs))
	}
	for i, v := range initial {
		if vtr.Outputs()[0][i] != math.Tanh(v) {
			t.Fatalf("row 0 output %d: got %.17g want %.17g", i, vtr.Outputs()[0][i], math.Tanh(v))
		}
	}
	assertBitEqual(t, vtr.FinalVoltage(), ctr.FinalVoltage())
	assertBitEqual(t, vtr.Voltages()[len(inputs)], vtr.FinalVoltage())
	for step := 1; step <= len(inputs); step++ {
		assertBitEqual(t, vtr.Outputs()[step], ctr.Outputs()[step-1])
	}

	vstate, err := vec.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	cstate, err := sca.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	fullV, fullVOut, err := vec.Advance(context.Background(), p, vstate, inputs)
	if err != nil {
		t.Fatal(err)
	}
	fullC, fullCOut, err := sca.Advance(context.Background(), sp, cstate, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if fullV.Steps != fullC.Steps {
		t.Fatalf("steps %d, want %d", fullV.Steps, fullC.Steps)
	}
	assertBitEqual(t, fullV.Voltage, fullC.Voltage)
	assertBitEqualRows(t, fullV.History, fullC.History)
	assertBitEqualRows(t, fullVOut, fullCOut)
	if err := vec.ValidateState(fullV); err != nil {
		t.Fatal(err)
	}

	vs, vouts := vstate, [][]float64(nil)
	cs, couts := cstate, [][]float64(nil)
	start := 0
	for _, boundary := range []int{2, 5, len(inputs)} {
		var err error
		vs, vouts, err = vec.Advance(context.Background(), p, vs, inputs[start:boundary])
		if err != nil {
			t.Fatal(err)
		}
		cs, couts, err = sca.Advance(context.Background(), sp, cs, inputs[start:boundary])
		if err != nil {
			t.Fatal(err)
		}
		if vs.Steps != cs.Steps {
			t.Fatalf("split steps %d, want %d", vs.Steps, cs.Steps)
		}
		assertBitEqual(t, vs.Voltage, cs.Voltage)
		assertBitEqualRows(t, vs.History, cs.History)
		assertBitEqualRows(t, vouts, couts)
		start = boundary
	}
	assertBitEqual(t, vs.Voltage, fullV.Voltage)
	assertBitEqualRows(t, vs.History, fullV.History)
	if err := vec.ValidateState(vs); err != nil {
		t.Fatal(err)
	}
}

func TestVectorContinuousHandComputedC2(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: math.Log(2), Activation: "tanh", StateDimension: 2, EdgeShape: "matrix"})
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{Weights: []float64{1, 2, 3, 4}, Bias: []float64{0, 0, 0, 0}, LogTau: []float64{0, 0}}
	// DT = ln 2 and tau = 1 give lambda = alpha = 0.5 exactly.
	tr, err := m.Forward(context.Background(), p, []float64{0, 0, 0, 0}, [][]float64{{1, 1, 0, 0}, {0, 0, 0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	const tol = 1e-12
	h := math.Tanh(0.5)
	// Step 0: node 0 gets drive [1,1], so v0 = 0.5*[1,1], out0 = [h,h].
	assertCloseGrid(t, tr.Voltages()[1][0:2], []float64{0.5, 0.5}, tol)
	assertCloseGrid(t, tr.Outputs()[1][0:2], []float64{h, h}, tol)
	// Step 0: node 1 drive is W*out0(initial) = W*[0,0] = [0,0], so v1 = out1 = 0.
	assertCloseGrid(t, tr.Voltages()[1][2:4], []float64{0, 0}, tol)
	assertCloseGrid(t, tr.Outputs()[1][2:4], []float64{0, 0}, tol)
	// Step 1: node 1 drive = W*out0(step0) = [1*H+2*H, 3*H+4*H] = [3h, 7h];
	// v1 = 0.5*[3h,7h] and out1 = tanh over that.
	assertCloseGrid(t, tr.Voltages()[2][2:4], []float64{1.5 * h, 3.5 * h}, tol)
	assertCloseGrid(t, tr.Outputs()[2][2:4], []float64{math.Tanh(1.5 * h), math.Tanh(3.5 * h)}, tol)
	// Step 1: node 0 has no incoming edge, drive 0, so v0 = 0.5*[0.5,0.5].
	assertCloseGrid(t, tr.Voltages()[2][0:2], []float64{0.25, 0.25}, tol)
	assertCloseGrid(t, tr.Outputs()[2][0:2], []float64{math.Tanh(0.25), math.Tanh(0.25)}, tol)
}

func TestVectorContinuousScalarBroadcastC3(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 0.5, Activation: "tanh", StateDimension: 3, EdgeShape: "scalar"})
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{Weights: []float64{2}, Bias: []float64{0, 0, 0, 0, 0, 0}, LogTau: []float64{0, 0}}
	tr, err := m.Forward(context.Background(), p, []float64{1, 2, 3, 0, 0, 0}, [][]float64{{0, 0, 0, 0, 0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	// One zero-delay step: target drive = 2 * tanh(source initial voltage)
	// broadcast over the three components; target initial v = 0.
	alpha := -math.Expm1(-0.5)
	src := []float64{1, 2, 3}
	want := []float64{alpha * 2 * math.Tanh(src[0]), alpha * 2 * math.Tanh(src[1]), alpha * 2 * math.Tanh(src[2])}
	assertCloseGrid(t, tr.Voltages()[1][3:6], want, 1e-12)
}

func TestVectorContinuousRejectsShapes(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{0}, DT: 1, Activation: "tanh", StateDimension: 3, EdgeShape: "matrix"})
	if err != nil {
		t.Fatal(err)
	}
	const nv = 6
	initial := make([]float64, nv)
	inputs := [][]float64{make([]float64, nv)}
	ok := VectorParameters{Weights: make([]float64, 9), Bias: make([]float64, nv), LogTau: []float64{0, 0}}
	if _, err := m.Forward(context.Background(), ok, initial, inputs); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Forward(context.Background(), ok, make([]float64, nv-1), inputs); err == nil {
		t.Fatal("accepted wrong initial length")
	}
	if _, err := m.Forward(context.Background(), ok, initial, [][]float64{make([]float64, nv-1)}); err == nil {
		t.Fatal("accepted wrong input width")
	}
	badWeights := VectorParameters{Weights: make([]float64, 8), Bias: ok.Bias, LogTau: ok.LogTau}
	if _, err := m.Forward(context.Background(), badWeights, initial, inputs); err == nil {
		t.Fatal("accepted wrong weights length")
	}
	badBias := VectorParameters{Weights: ok.Weights, Bias: make([]float64, nv-1), LogTau: ok.LogTau}
	if _, err := m.Forward(context.Background(), badBias, initial, inputs); err == nil {
		t.Fatal("accepted wrong bias length")
	}
	badLogTau := VectorParameters{Weights: ok.Weights, Bias: ok.Bias, LogTau: []float64{0}}
	if _, err := m.Forward(context.Background(), badLogTau, initial, inputs); err == nil {
		t.Fatal("accepted wrong log_tau length")
	}
	if _, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh", StateDimension: 0}); err == nil {
		t.Fatal("accepted C=0")
	}
	if _, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh", StateDimension: 65}); err == nil {
		t.Fatal("accepted C=65")
	}
}

func TestVectorStateRoundTrip(t *testing.T) {
	m, err := NewVectorContinuous(Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: 0.5, Activation: "tanh", StateDimension: 2, EdgeShape: "matrix"})
	if err != nil {
		t.Fatal(err)
	}
	p := VectorParameters{Weights: []float64{1, 0, 0, 1}, Bias: []float64{0.1, -0.1, 0.2, -0.2}, LogTau: []float64{0.3, -0.2}}
	initial := []float64{0.1, 0.2, -0.1, -0.2}
	inputs := [][]float64{{0.1, 0.2, -0.1, -0.2}, {-0.2, 0.1, 0.3, -0.4}, {0.3, -0.1, 0.2, 0.1}}
	s0, err := m.NewState(initial)
	if err != nil {
		t.Fatal(err)
	}
	if s0.SchemaVersion != VectorStateVersion {
		t.Fatalf("schema version %q", s0.SchemaVersion)
	}
	encoded, err := json.Marshal(m.config)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	if s0.ConfigHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("config hash %q, want %q", s0.ConfigHash, hex.EncodeToString(sum[:]))
	}
	if err := m.ValidateState(s0); err != nil {
		t.Fatal(err)
	}
	s1, out, err := m.Advance(context.Background(), p, s0, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(inputs) || s1.Steps != uint64(len(inputs)) || len(s1.History) != 2 {
		t.Fatalf("advance sizes: outputs %d steps %d history %d", len(out), s1.Steps, len(s1.History))
	}
	if err := m.ValidateState(s1); err != nil {
		t.Fatal(err)
	}
	s2, _, err := m.Advance(context.Background(), p, s1, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Steps != 2*uint64(len(inputs)) || len(s2.History) != 2 {
		t.Fatalf("second advance: steps %d history %d", s2.Steps, len(s2.History))
	}
	if err := m.ValidateState(s2); err != nil {
		t.Fatal(err)
	}
	shortened := cloneVectorState(s1)
	shortened.History = shortened.History[:1]
	if err := m.ValidateState(shortened); err == nil {
		t.Fatal("accepted shortened history")
	}
	nilHistory := cloneVectorState(s1)
	nilHistory.History = nil
	if err := m.ValidateState(nilHistory); err == nil {
		t.Fatal("accepted nil history")
	}
}

func cloneVectorState(s VectorState) VectorState {
	return VectorState{
		SchemaVersion: s.SchemaVersion,
		ConfigHash:    s.ConfigHash,
		Steps:         s.Steps,
		Voltage:       append([]float64(nil), s.Voltage...),
		History:       cloneTestRowsForVector(s.History),
	}
}

func cloneTestRowsForVector(rows [][]float64) [][]float64 {
	out := make([][]float64, len(rows))
	for i := range rows {
		out[i] = append([]float64(nil), rows[i]...)
	}
	return out
}
