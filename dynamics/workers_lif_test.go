// This file is an internal package test because the controlled parallel LIF
// pass has to be compared against the reference mode array by array, including
// the package-private adaptation, rate and threshold offset buffers.
package dynamics

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
)

// lifWorkersConfig merges the two hand fixtures of the spiking core so the
// bit-identical comparison exercises every mechanism at once: node 0 drives
// node 1 through a zero delay edge and node 2 through a delay-2 edge, node 1
// feeds back to node 0 through a delay-1 edge, the short term adaptation is
// enabled and the slow stabiliser is declared and enabled. dt = ln 2 with
// tau = tau_syn = tau_adapt = 1 gives lambda = alpha = kappa = rho = 0.5
// exactly and theta_raw 0 gives theta_base = 1 exactly, the values fixed by
// lifHandConfig and lifHomeostasisHandConfig.
func lifWorkersConfig() LIFConfig {
	return LIFConfig{
		Nodes:           3,
		Sources:         []int{0, 0, 1},
		Targets:         []int{1, 2, 0},
		Delays:          []int{0, 2, 1},
		DT:              math.Ln2,
		TauSyn:          1,
		ThetaMin:        0,
		ThetaMax:        2,
		VReset:          -1,
		RefractorySteps: 1,
		Adaptation:      LIFAdaptation{Enabled: true, TauAdapt: 1, Beta: 0.5},
		Homeostasis:     &LIFHomeostasis{Enabled: true, TauRate: 2 * math.Ln2, TargetRate: .25, Eta: 1 / math.Ln2, HMax: .4},
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 1},
	}
}

// lifWorkersParams drives neuron 0 hard for four steps so the fixture spikes,
// then stops so the slow stabiliser can be seen falling back.
func lifWorkersParams() (LIFParameters, []float64, [][]float64) {
	p := LIFParameters{
		Weights: []float64{2, 4, 3}, Bias: []float64{0, 0, 0},
		LogTau: []float64{0, 0, 0}, ThetaRaw: []float64{0, 0, 0},
	}
	inputs := make([][]float64, 9)
	for t := range inputs {
		inputs[t] = []float64{0, 0, 0}
		if t < 4 {
			inputs[t][0] = 4
		}
	}
	return p, []float64{0, 0, 0}, inputs
}

func lifWorkersFixture() (LIFConfig, LIFParameters, []float64, [][]float64) {
	p, initial, inputs := lifWorkersParams()
	return lifWorkersConfig(), p, initial, inputs
}

func newLIFWithWorkers(t *testing.T, c LIFConfig, workers int) *LIF {
	t.Helper()
	c2 := c
	c2.Workers = workers
	m, err := NewLIF(c2)
	if err != nil {
		t.Fatalf("workers=%d NewLIF: %v", workers, err)
	}
	return m
}

func lifWorkersRowsEqual(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for t := range a {
		if !lifWorkersFloatsEqual(a[t], b[t]) {
			return false
		}
	}
	return true
}

func lifWorkersFloatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func lifWorkersIntsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lifWorkersStateEqual compares every LIFState array elementwise with ==.
func lifWorkersStateEqual(a, b LIFState) bool {
	return a.Steps == b.Steps &&
		lifWorkersFloatsEqual(a.Voltage, b.Voltage) &&
		lifWorkersRowsEqual(a.History, b.History) &&
		lifWorkersFloatsEqual(a.Adaptation, b.Adaptation) &&
		lifWorkersIntsEqual(a.Refractory, b.Refractory) &&
		lifWorkersFloatsEqual(a.Rate, b.Rate) &&
		lifWorkersFloatsEqual(a.Homeostasis, b.Homeostasis)
}

func TestLIFWorkersAreBitIdentical(t *testing.T) {
	c, p, initial, inputs := lifWorkersFixture()
	models := make(map[int]*LIF)
	for _, w := range []int{0, 1, 2, 7} {
		models[w] = newLIFWithWorkers(t, c, w)
	}
	refTr, err := models[1].Forward(context.Background(), p, initial, inputs)
	if err != nil {
		t.Fatal(err)
	}
	fired := 0
	for _, row := range refTr.Spikes() {
		for _, v := range row {
			fired += int(v)
		}
	}
	if fired == 0 {
		t.Fatal("fixture never spiked, the comparison would be vacuous")
	}

	type fwdResult struct {
		outputs, spikes, adapt, rate, homeo [][]float64
		volt                                []float64
	}
	fwds := make(map[int]fwdResult)
	for w, m := range models {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatalf("w=%d Forward: %v", w, err)
		}
		fwds[w] = fwdResult{
			outputs: tr.Outputs(), spikes: tr.Spikes(), volt: tr.FinalVoltage(),
			adapt: tr.adapt[1:], rate: tr.rate[1:], homeo: tr.homeo[1:],
		}
	}
	refFwd := fwds[1]
	for w, fr := range fwds {
		if !lifWorkersRowsEqual(fr.outputs, refFwd.outputs) {
			t.Fatalf("w=%d Forward outputs differ", w)
		}
		if !lifWorkersRowsEqual(fr.spikes, refFwd.spikes) {
			t.Fatalf("w=%d Forward spikes differ", w)
		}
		if !lifWorkersFloatsEqual(fr.volt, refFwd.volt) {
			t.Fatalf("w=%d Forward final voltage differs", w)
		}
		if !lifWorkersRowsEqual(fr.adapt, refFwd.adapt) {
			t.Fatalf("w=%d Forward adaptation differs", w)
		}
		if !lifWorkersRowsEqual(fr.rate, refFwd.rate) {
			t.Fatalf("w=%d Forward rate estimate differs", w)
		}
		if !lifWorkersRowsEqual(fr.homeo, refFwd.homeo) {
			t.Fatalf("w=%d Forward threshold offset differs", w)
		}
	}

	// Advance in three segments: every LIFState array must be bit-identical
	// across workers, and the reference worker's sectioned state must equal the
	// single Forward end state, not just itself.
	boundaries := []int{2, 5, 9}
	states := make(map[int]LIFState)
	allOutputs := make(map[int][][]float64)
	allSpikes := make(map[int][][]float64)
	for w, m := range models {
		s, err := m.NewState(initial)
		if err != nil {
			t.Fatal(err)
		}
		prev := 0
		for _, b := range boundaries {
			var out, spk [][]float64
			s, out, spk, err = m.Advance(context.Background(), p, s, inputs[prev:b])
			if err != nil {
				t.Fatalf("w=%d Advance segment: %v", w, err)
			}
			allOutputs[w] = append(allOutputs[w], out...)
			allSpikes[w] = append(allSpikes[w], spk...)
			prev = b
		}
		states[w] = s
	}
	refState := states[1]
	for w := range models {
		if !lifWorkersStateEqual(states[w], refState) {
			t.Fatalf("w=%d sectioned LIFState differs", w)
		}
		if !lifWorkersRowsEqual(allOutputs[w], refFwd.outputs) {
			t.Fatalf("w=%d Advance outputs differ from Forward reference", w)
		}
		if !lifWorkersRowsEqual(allSpikes[w], refFwd.spikes) {
			t.Fatalf("w=%d Advance spikes differ from Forward reference", w)
		}
	}
	rows := min(len(inputs), lifMaxDelay(c)) + 1
	if !lifWorkersFloatsEqual(refState.Voltage, refFwd.volt) {
		t.Fatal("sectioned Advance voltage differs from Forward")
	}
	if !lifWorkersRowsEqual(refState.History, cloneRows(refTr.syn[len(inputs)+1-rows:])) {
		t.Fatal("sectioned Advance history differs from Forward")
	}
	if !lifWorkersFloatsEqual(refState.Adaptation, refTr.adapt[len(inputs)]) {
		t.Fatal("sectioned Advance adaptation differs from Forward")
	}
	if !lifWorkersIntsEqual(refState.Refractory, refTr.refract[len(inputs)]) {
		t.Fatal("sectioned Advance refractory differs from Forward")
	}
	if !lifWorkersFloatsEqual(refState.Rate, refTr.rate[len(inputs)]) {
		t.Fatal("sectioned Advance rate differs from Forward")
	}
	if !lifWorkersFloatsEqual(refState.Homeostasis, refTr.homeo[len(inputs)]) {
		t.Fatal("sectioned Advance homeostasis differs from Forward")
	}
}

func TestLIFWorkersGradientBitIdentical(t *testing.T) {
	c, p, initial, inputs := lifWorkersFixture()
	up := make([][]float64, len(inputs))
	for t := range up {
		up[t] = []float64{0.2, -0.3, 0.1}
		if t%2 == 0 {
			up[t][0] = -0.4
		}
	}
	grads := make(map[int]LIFGradient)
	for _, w := range []int{1, 7} {
		m := newLIFWithWorkers(t, c, w)
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			t.Fatal(err)
		}
		g, err := m.Backward(context.Background(), tr, up, 0)
		if err != nil {
			t.Fatal(err)
		}
		grads[w] = g
	}
	ref := grads[1]
	for w, g := range grads {
		if !lifWorkersFloatsEqual(g.Weights, ref.Weights) {
			t.Fatalf("w=%d Weights gradient differs", w)
		}
		if !lifWorkersFloatsEqual(g.Bias, ref.Bias) {
			t.Fatalf("w=%d Bias gradient differs", w)
		}
		if !lifWorkersFloatsEqual(g.LogTau, ref.LogTau) {
			t.Fatalf("w=%d LogTau gradient differs", w)
		}
		if !lifWorkersFloatsEqual(g.ThetaRaw, ref.ThetaRaw) {
			t.Fatalf("w=%d ThetaRaw gradient differs", w)
		}
	}
}

func TestLIFWorkersRejectBadCount(t *testing.T) {
	c := lifWorkersConfig()
	for _, w := range []int{-1, 1025} {
		c2 := c
		c2.Workers = w
		if _, err := NewLIF(c2); err == nil {
			t.Fatalf("workers=%d should be rejected", w)
		}
	}
}

func TestLIFPartitionBuildsOnlyForParallel(t *testing.T) {
	c := lifWorkersConfig()
	for _, w := range []int{0, 1, 2} {
		m := newLIFWithWorkers(t, c, w)
		if w == 0 || w == 1 {
			if m.partition != nil {
				t.Fatalf("workers=%d: reference mode must not build a partition", w)
			}
			continue
		}
		if m.partition == nil {
			t.Fatalf("workers=%d: partition not built", w)
		}
		if got := m.partition.Workers(); got != w {
			t.Fatalf("workers=%d: partition.Workers()=%d, want %d", w, got, w)
		}
	}
}

// lifBigFixture builds a 64 node, 512 edge spiking graph with every mechanism
// enabled and the fixed seed of the continuous fixture, so allocation and
// benchmark measurements are deterministic.
func lifBigFixture(t testing.TB) (LIFConfig, LIFParameters, []float64, [][]float64) {
	t.Helper()
	rnd := rand.New(rand.NewPCG(42, 42))
	const nodes = 64
	const edges = 512
	sources := make([]int, edges)
	targets := make([]int, edges)
	delays := make([]int, edges)
	for e := 0; e < edges; e++ {
		sources[e] = rnd.IntN(nodes)
		targets[e] = rnd.IntN(nodes)
		delays[e] = rnd.IntN(4)
	}
	c := LIFConfig{
		Nodes: nodes, Sources: sources, Targets: targets, Delays: delays,
		DT: 0.4, TauSyn: 0.9, ThetaMin: 0.3, ThetaMax: 1.6, VReset: -0.5,
		RefractorySteps: 1,
		Adaptation:      LIFAdaptation{Enabled: true, TauAdapt: 1.3, Beta: 0.4},
		Homeostasis:     &LIFHomeostasis{Enabled: true, TauRate: 2, TargetRate: .2, Eta: .1, HMax: 1},
		Surrogate:       LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
	}
	p := LIFParameters{
		Weights: make([]float64, edges), Bias: make([]float64, nodes),
		LogTau: make([]float64, nodes), ThetaRaw: make([]float64, nodes),
	}
	for e := range p.Weights {
		p.Weights[e] = 2*rnd.Float64() - 0.5
	}
	initial := make([]float64, nodes)
	for i := range nodes {
		p.Bias[i] = 0.4*rnd.Float64() - 0.1
		p.LogTau[i] = 0.5*rnd.Float64() - 0.25
		p.ThetaRaw[i] = 2*rnd.Float64() - 1
		initial[i] = 0.4*rnd.Float64() - 0.2
	}
	inputs := make([][]float64, 32)
	for t := range inputs {
		inputs[t] = make([]float64, nodes)
		for i := range nodes {
			inputs[t][i] = 6 * rnd.Float64()
		}
	}
	return c, p, initial, inputs
}

func TestLIFForwardAllocsAreConstant(t *testing.T) {
	c, p, initial, _ := lifBigFixture(t)
	makeInputs := func(n int) [][]float64 {
		r := rand.New(rand.NewPCG(99, 99))
		in := make([][]float64, n)
		for t := range in {
			row := make([]float64, c.Nodes)
			for i := range row {
				row[i] = r.NormFloat64() * 0.1
			}
			in[t] = row
		}
		return in
	}
	measure := func(workers, steps int) float64 {
		c2 := c
		c2.Workers = workers
		m, err := NewLIF(c2)
		if err != nil {
			t.Fatal(err)
		}
		inputs := makeInputs(steps)
		return testing.AllocsPerRun(20, func() {
			tr, err := m.Forward(context.Background(), p, initial, inputs)
			if err != nil {
				t.Fatal(err)
			}
			_ = tr
		})
	}

	ref8 := measure(1, 8)
	ref32 := measure(1, 32)
	w48 := measure(4, 8)
	w432 := measure(4, 32)

	t.Logf("workers=1 allocs/run:  8 steps=%.0f 32 steps=%.0f", ref8, ref32)
	t.Logf("workers=4 allocs/run:  8 steps=%.0f 32 steps=%.0f", w48, w432)

	// 每步 13 次是既有 Forward 迴圈在啟用慢速穩定時的基準：drive、cand、voltage、
	// spike、synapse、adapt、reset、rate、homeo、derivative、refract 各一次，
	// 加上活化時的 nextRate、nextH。Workers 1 的參考模式不得增加每步配置，所以
	// 24 步差的 delta 不得超過 13*24；另保留 1 次 race 模式的餘裕，同 Continuous
	// 測試每步 3 次但以 4*24 斷言的慣例，故上限取 14*24。
	delta := ref32 - ref8
	t.Logf("workers=1 alloc delta: %.0f (max allowed %d)", math.Abs(delta), 14*24)
	if math.Abs(delta) > 14*24 {
		t.Fatalf("allocs not constant: workers=1 8-step=%.0f 32-step=%.0f delta=%.0f > %d", ref8, ref32, delta, 14*24)
	}
}

func BenchmarkLIFForwardWorkers1(b *testing.B) {
	c, p, initial, inputs := lifBigFixture(b)
	c.Workers = 1
	m, err := NewLIF(c)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			b.Fatal(err)
		}
		_ = tr
	}
}

func BenchmarkLIFForwardWorkers4(b *testing.B) {
	c, p, initial, inputs := lifBigFixture(b)
	c.Workers = 4
	m, err := NewLIF(c)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr, err := m.Forward(context.Background(), p, initial, inputs)
		if err != nil {
			b.Fatal(err)
		}
		_ = tr
	}
}
