package main

import (
	"compress/gzip"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func writeGzip(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := gzip.NewWriter(f)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestChemistryLeavesTheBaseModelUnchanged builds the model with and without
// --chem-channels: the chemistry block draws from its own stream, so every
// base configuration field and parameter is identical, and a run without it
// records no chemistry setting at all.
func TestChemistryLeavesTheBaseModelUnchanged(t *testing.T) {
	base := settings{Nodes: 40, InputNodes: 8, ReadoutNodes: 10, InDegree: 5, Seed: 3, Gate: true, GateSquash: "softsign", Activation: "softsign"}
	off := normalizeChemistry(base)
	off.ChemTau, off.ChemSource = 5, "all" // flag defaults that normalize must drop
	off = normalizeChemistry(off)
	if off != base {
		t.Fatalf("normalized settings %+v, want %+v", off, base)
	}
	b, err := json.Marshal(off)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "chem") {
		t.Fatalf("settings without chemistry record %s", b)
	}
	cfgOff, pOff, _ := build(off, nil)
	if cfgOff.Dynamics.Chemistry != nil || pOff.Core.ChemRelease != nil || pOff.Core.ChemGain != nil || pOff.Core.ChemOffset != nil {
		t.Fatal("chemistry present without --chem-channels")
	}
	for _, source := range []string{"all", "split", "input", "readout", "hidden"} {
		on := base
		on.ChemChannels, on.ChemTau, on.ChemHill, on.ChemSource, on.ChemReceptors, on.ChemInit = 3, 4, 2, source, .5, .01
		if err := checkChemistry(on); err != nil {
			t.Fatal(err)
		}
		cfgOn, pOn, _ := build(on, nil)
		chem := cfgOn.Dynamics.Chemistry
		if chem == nil || len(chem.Channels) != 3 || len(pOn.Core.ChemRelease) != 3 || len(pOn.Core.ChemGain) != len(chem.Receptors) || len(chem.Receptors) == 0 || len(chem.Receptors) == 3*on.Nodes {
			t.Fatalf("%s: chemistry block %+v", source, chem)
		}
		if want := 4 * math.Ln2; chem.Channels[0].Kd != want {
			t.Fatalf("%s: default kd %v, want %v", source, chem.Channels[0].Kd, want)
		}
		cfgOn.Dynamics.Chemistry = nil
		pOn.Core.ChemRelease, pOn.Core.ChemGain, pOn.Core.ChemOffset = nil, nil, nil
		if !reflect.DeepEqual(cfgOn, cfgOff) || !reflect.DeepEqual(pOn, pOff) {
			t.Fatalf("%s: chemistry changed the base model", source)
		}
		cfg, p, _ := build(on, nil)
		o := learning.DefaultOptions()
		o.Trainable.Gate, o.Trainable.Chemistry = true, true
		if _, err := learning.NewTrainer(cfg, p, o); err != nil {
			t.Fatalf("%s: %v", source, err)
		}
	}
	gpu := base
	gpu.ChemChannels, gpu.ChemTau, gpu.ChemHill, gpu.ChemSource, gpu.ChemReceptors, gpu.GPU = 1, 4, 2, "all", 1, true
	if err := checkChemistry(gpu); err == nil {
		t.Fatal("--chem-channels accepted with --gpu")
	}
}

func TestReadSplitGroupsColumns(t *testing.T) {
	dir := t.TempDir()
	// Pixel (row, col) holds row + 2*col, so every position is distinct.
	img := make([]byte, side*side)
	for row := 0; row < side; row++ {
		for col := 0; col < side; col++ {
			img[row*side+col] = byte(row + 2*col)
		}
	}
	// IDX magic: two zero bytes, type 0x08 (unsigned byte), dimension count.
	imageHeader := []byte{0, 0, 8, 3, 0, 0, 0, 1, 0, 0, 0, side, 0, 0, 0, side}
	labelHeader := []byte{0, 0, 8, 1, 0, 0, 0, 1}
	writeGzip(t, filepath.Join(dir, "x-images-idx3-ubyte.gz"), append(imageHeader, img...))
	writeGzip(t, filepath.Join(dir, "x-labels-idx1-ubyte.gz"), append(labelHeader, 7))
	for _, cols := range []int{1, 2, 4} {
		xs, ys, err := readSplit(dir, "x", 1, cols)
		if err != nil {
			t.Fatal(err)
		}
		if ys[0] != 7 || len(xs[0]) != side/cols {
			t.Fatalf("cols %d: label %d, %d steps", cols, ys[0], len(xs[0]))
		}
		for step, v := range xs[0] {
			if len(v) != side*cols {
				t.Fatalf("cols %d: step %d has %d values", cols, step, len(v))
			}
			for i, got := range v {
				col, row := step*cols+i/side, i%side
				if want := float64(row+2*col) / 255; got != want {
					t.Fatalf("cols %d: step %d value %d = %v, want pixel (%d, %d) = %v", cols, step, i, got, row, col, want)
				}
			}
		}
	}
}

// TestRewireKeepsDegrees checks both null models on a skewed graph: each
// keeps the edge count, adds no duplicate edge, keeps every target's
// in-degree, "swap" also every source's out-degree, and both move edges.
func TestRewireKeepsDegrees(t *testing.T) {
	const nodes = 40
	var g topology
	g.nodes = nodes
	for d := 0; d < nodes; d++ {
		for s := 0; s < nodes; s++ {
			// Hubs: low-numbered sources reach many targets.
			if (s*7+d*3)%(s+2) == 0 {
				g.sources, g.targets = append(g.sources, s), append(g.targets, d)
			}
		}
	}
	degrees := func(t topology) (in, out []int) {
		in, out = make([]int, t.nodes), make([]int, t.nodes)
		for e := range t.sources {
			out[t.sources[e]]++
			in[t.targets[e]]++
		}
		return in, out
	}
	wantIn, wantOut := degrees(g)
	original := make(map[[2]int]bool)
	for e := range g.sources {
		original[[2]int{g.sources[e], g.targets[e]}] = true
	}
	for _, mode := range []string{"in-degree", "swap"} {
		r, err := rewire(g, mode, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.sources) != len(g.sources) || len(r.targets) != len(g.targets) {
			t.Fatalf("%s: %d edges, want %d", mode, len(r.sources), len(g.sources))
		}
		seen := make(map[[2]int]bool)
		moved := 0
		for e := range r.sources {
			k := [2]int{r.sources[e], r.targets[e]}
			if seen[k] {
				t.Fatalf("%s: duplicate edge %v", mode, k)
			}
			seen[k] = true
			if !original[k] {
				moved++
			}
		}
		if moved < len(r.sources)/4 {
			t.Fatalf("%s: only %d of %d edges moved", mode, moved, len(r.sources))
		}
		in, out := degrees(r)
		for i := range in {
			if in[i] != wantIn[i] {
				t.Fatalf("%s: node %d in-degree %d, want %d", mode, i, in[i], wantIn[i])
			}
			if mode == "swap" && out[i] != wantOut[i] {
				t.Fatalf("swap: node %d out-degree %d, want %d", i, out[i], wantOut[i])
			}
		}
	}
	if _, err := rewire(g, "bogus", 1); err == nil {
		t.Fatal("unknown rewire mode accepted")
	}
	same, err := rewire(g, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	for e := range g.sources {
		if same.sources[e] != g.sources[e] || same.targets[e] != g.targets[e] {
			t.Fatal("empty mode changed the edges")
		}
	}
}

// TestMechanismsLeaveTheBaseModelUnchanged checks that each optional mechanism
// draws from its own stream: the topology, encoder, readout and (up to the
// sign parametrization) the weights stay those of the plain model.
func TestMechanismsLeaveTheBaseModelUnchanged(t *testing.T) {
	base := settings{Nodes: 40, InputNodes: 5, ReadoutNodes: 6, InDegree: 4, Seed: 7}
	c0, p0, _ := build(base, nil)

	s := base
	s.Delays = 3
	c, p, _ := build(s, nil)
	if !reflect.DeepEqual(c.Dynamics.Sources, c0.Dynamics.Sources) || !reflect.DeepEqual(p, p0) {
		t.Fatal("delays changed the topology or the parameters")
	}
	seen := map[int]bool{}
	for _, d := range c.Dynamics.Delays {
		if d < 0 || d > 3 {
			t.Fatalf("delay %d outside 0..3", d)
		}
		seen[d] = true
	}
	if len(c.Dynamics.Delays) != len(c0.Dynamics.Sources) || len(seen) != 4 {
		t.Fatalf("%d delays covering %d values, want %d covering 4", len(c.Dynamics.Delays), len(seen), len(c0.Dynamics.Sources))
	}

	s = base
	s.Dale, s.Inhibitory = true, 0.3
	c, p, _ = build(s, nil)
	weights, err := learning.EffectiveWeights(c, p)
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[int]int8{}
	negative := 0
	for e, src := range c.Dynamics.Sources {
		sign := c.EdgeSigns[e]
		if want, ok := bySource[src]; ok && want != sign {
			t.Fatalf("source %d has edges of both signs", src)
		}
		bySource[src] = sign
		if math.Abs(weights[e]-float64(sign)*math.Abs(p0.Core.Weights[e])) > 1e-12 {
			t.Fatalf("edge %d effective weight %g, want %g", e, weights[e], float64(sign)*math.Abs(p0.Core.Weights[e]))
		}
	}
	for _, sign := range bySource {
		if sign < 0 {
			negative++
		}
	}
	if negative == 0 || negative == len(bySource) {
		t.Fatalf("%d of %d sources inhibitory", negative, len(bySource))
	}

	s = base
	s.TauMax = 20
	_, p, _ = build(s, nil)
	spread := false
	for i, v := range p.Core.LogTau {
		if tau := math.Exp(v); tau < 2-1e-12 || tau > 20+1e-12 {
			t.Fatalf("node %d tau %g outside [2, 20]", i, tau)
		}
		spread = spread || math.Abs(v-math.Log(2)) > 0.1
	}
	if !spread || !reflect.DeepEqual(p.Core.Weights, p0.Core.Weights) {
		t.Fatal("tau-max did not spread tau, or changed the weights")
	}
}

func TestFreezeTurnsOffNamedGroups(t *testing.T) {
	all := learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true, Gate: true}
	got, err := freeze(all, "weights, tau,gate")
	if err != nil {
		t.Fatal(err)
	}
	want := learning.Trainable{Encoder: true, Bias: true, Readout: true}
	if got != want {
		t.Fatalf("freeze gave %+v, want %+v", got, want)
	}
	if _, err := freeze(all, "weights,nope"); err == nil {
		t.Fatal("an unknown group was accepted")
	}
}

func TestLIFCoreKeepsEdgesAndWeights(t *testing.T) {
	base := settings{Nodes: 30, InputNodes: 4, ReadoutNodes: 5, InDegree: 3, Seed: 2, Delays: 2}
	c0, p0, _ := build(base, nil)
	s := base
	s.Core, s.SurrogateScale, s.Adapt, s.Homeostasis = "lif", 5, true, true
	c, p, _ := build(s, nil)
	if c.LIF == nil || len(c.Dynamics.Sources) != 0 {
		t.Fatal("the spiking core was not selected alone")
	}
	if !reflect.DeepEqual(c.LIF.Sources, c0.Dynamics.Sources) || !reflect.DeepEqual(c.LIF.Targets, c0.Dynamics.Targets) || !reflect.DeepEqual(c.LIF.Delays, c0.Dynamics.Delays) {
		t.Fatal("the spiking core has other edges or delays")
	}
	if !reflect.DeepEqual(p.Core, p0.Core) || len(p.ThetaRaw) != s.Nodes {
		t.Fatal("the spiking core changed the weights or has no threshold per node")
	}
	if !c.LIF.Adaptation.Enabled || c.LIF.Homeostasis == nil || !c.LIF.Homeostasis.Enabled {
		t.Fatal("adaptation or homeostasis was not declared")
	}
	if _, err := learning.NewTrainer(c, p, learning.DefaultOptions()); err != nil {
		t.Fatal(err)
	}
}

// TestTieByTypeSharesOneValuePerType ties a six-node store topology with
// types A, A, B, B, unknown, A. Edges between the same type pair share a
// weight, nodes of a type share bias and tau, the unknown node and its edges
// stay individual, and the trainer accepts the declaration.
func TestTieByTypeSharesOneValuePerType(t *testing.T) {
	topo := topology{
		nodes:   6,
		sources: []int{0, 1, 2, 3, 4, 0, 5, 2},
		targets: []int{2, 3, 0, 1, 0, 4, 2, 5},
		types:   []string{"A", "A", "B", "B", "", "A"},
	}
	s := settings{Nodes: 6, InputNodes: 2, ReadoutNodes: 2, InDegree: 1, Seed: 7, Gate: true, GateSquash: "softsign", Activation: "softsign", TauMax: 20, Tie: "type"}
	c, p, removed := build(s, &topo)
	share := c.Sharing
	if share == nil {
		t.Fatal("no sharing declared")
	}
	// Weight groups: A->B (edges 0, 1, 6), B->A (2, 3, 7); edges 4 and 5
	// touch the unknown node. Bias and tau groups: A (0, 1, 5), B (2, 3).
	wantW := []int32{0, 0, 1, 1, -1, -1, 0, 1}
	wantN := []int32{2, 2, 3, 3, -1, 2}
	if !reflect.DeepEqual(share.Weights, wantW) || !reflect.DeepEqual(share.Bias, wantN) || share.Groups != 6 {
		t.Fatalf("sharing = %+v", share)
	}
	for i, g := range wantN {
		wantN[i] = g + 2
		if g < 0 {
			wantN[i] = -1
		}
	}
	if !reflect.DeepEqual(share.LogTau, wantN) {
		t.Fatalf("log_tau groups = %v, want %v", share.LogTau, wantN)
	}
	// Removed values: weights 2+2, bias 2+1, tau 2+1.
	if removed != 10 {
		t.Fatalf("removed %d values, want 10", removed)
	}
	w, tau := p.Core.Weights, p.Core.LogTau
	if w[0] != w[1] || w[0] != w[6] || w[2] != w[3] || w[2] != w[7] || w[0] == w[2] {
		t.Fatalf("tied weights %v", w)
	}
	if tau[0] != tau[1] || tau[0] != tau[5] || tau[2] != tau[3] || tau[0] == tau[2] || tau[4] == tau[0] {
		t.Fatalf("tied tau %v", tau)
	}
	if _, err := learning.NewTrainer(c, p, learning.DefaultOptions()); err != nil {
		t.Fatalf("trainer refused the tied model: %v", err)
	}
	untied := s
	untied.Tie = ""
	c0, p0, removed0 := build(untied, &topo)
	if c0.Sharing != nil || removed0 != 0 || p0.Core.Weights[4] != w[4] || p0.Core.Weights[0] != w[0] {
		t.Fatal("tying changed more than the tied values")
	}
}

// TestHoldOutKeepsTheTailForValidation splits five labelled images: the last
// two become the validation split, the rest stay for training in order, and
// a split that would leave nothing to train on is refused.
func TestHoldOutKeepsTheTailForValidation(t *testing.T) {
	xs := make([][][]float64, 5)
	ys := []int{0, 1, 2, 3, 4}
	for i := range xs {
		xs[i] = [][]float64{{float64(i)}}
	}
	trainX, valX, trainY, valY, err := holdOut(xs, ys, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(trainY, []int{0, 1, 2}) || !reflect.DeepEqual(valY, []int{3, 4}) || len(trainX) != 3 || valX[0][0][0] != 3 {
		t.Fatalf("train %v, val %v", trainY, valY)
	}
	if _, _, trainY, valY, _ = holdOut(xs, ys, 0); len(trainY) != 5 || len(valY) != 0 {
		t.Fatalf("no hold-out: train %v, val %v", trainY, valY)
	}
	for _, n := range []int{-1, 5} {
		if _, _, _, _, err := holdOut(xs, ys, n); err == nil {
			t.Fatalf("--val-count %d accepted", n)
		}
	}
}

// TestShuffledTypesKeepGroupSizes ties the six-node topology of
// TestTieByTypeSharesOneValuePerType by permuted labels: the permutation is
// seeded, keeps how many nodes carry each label, and moves at least one.
func TestShuffledTypesKeepGroupSizes(t *testing.T) {
	types := []string{"A", "A", "B", "B", "", "A", "C", "C"}
	count := func(v []string) map[string]int {
		m := map[string]int{}
		for _, s := range v {
			m[s]++
		}
		return m
	}
	moved := false
	for seed := range uint64(4) {
		got := shuffleTypes(types, seed)
		if !reflect.DeepEqual(count(got), count(types)) || !reflect.DeepEqual(got, shuffleTypes(types, seed)) {
			t.Fatalf("seed %d: %v", seed, got)
		}
		moved = moved || !reflect.DeepEqual(got, types)
	}
	if !moved || types[0] != "A" || types[4] != "" {
		t.Fatal("shuffling moved no label or changed its input")
	}
	topo := topology{
		nodes:   6,
		sources: []int{0, 1, 2, 3, 4, 0, 5, 2},
		targets: []int{2, 3, 0, 1, 0, 4, 2, 5},
		types:   []string{"A", "A", "B", "B", "", "A"},
	}
	s := settings{Nodes: 6, InputNodes: 2, ReadoutNodes: 2, InDegree: 1, Seed: 3, Tie: "type-shuffled"}
	c, _, _ := build(s, &topo)
	want, _, _ := build(settings{Nodes: 6, InputNodes: 2, ReadoutNodes: 2, InDegree: 1, Seed: 3, Tie: "type"}, &topology{nodes: 6, sources: topo.sources, targets: topo.targets, types: shuffleTypes(topo.types, 3)})
	if c.Sharing == nil || !reflect.DeepEqual(c.Sharing, want.Sharing) {
		t.Fatalf("type-shuffled sharing %+v, want %+v", c.Sharing, want.Sharing)
	}
}

// writeSplit writes count synthetic images and labels as the IDX gzip pair
// readSplit expects: image i is bright in the column band of its label.
func writeSplit(t *testing.T, dir, split string, count int) {
	t.Helper()
	be := func(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }
	images := append([]byte{0, 0, 8, 3}, be(count)...)
	images = append(images, be(side)...)
	images = append(images, be(side)...)
	labels := append([]byte{0, 0, 8, 1}, be(count)...)
	for i := range count {
		label := i % classes
		for row := range side {
			for col := range side {
				v := byte((i*7 + row*3 + col) % 40)
				if col/3 == label {
					v += 180
				}
				images = append(images, v)
			}
		}
		labels = append(labels, byte(label))
	}
	writeGzip(t, filepath.Join(dir, split+"-images-idx3-ubyte.gz"), images)
	writeGzip(t, filepath.Join(dir, split+"-labels-idx1-ubyte.gz"), labels)
}

// TestEvalFinalOnlyKeepsTheFinalAccuracies trains the same small model with
// and without --eval-final-only: training is untouched, so both end with the
// same test and validation accuracy, and the final-only run records just the
// last epoch's.
func TestEvalFinalOnlyKeepsTheFinalAccuracies(t *testing.T) {
	dir := t.TempDir()
	writeSplit(t, dir, "train", 160)
	writeSplit(t, dir, "t10k", 40)
	s := settings{DataDir: dir, Nodes: 24, InputNodes: 8, ReadoutNodes: 8, InDegree: 3, Epochs: 3, Batch: 16, LearningRate: .02,
		TrainCount: 160, TestCount: 40, ValCount: 32, Workers: 2, Gate: true, GateSquash: "softsign", Activation: "softsign", SurrogateScale: 5, ChemTau: 5, ChemHill: 2, ChemSource: "all", ChemReceptors: 1, ChemInit: .01}
	reports := make([]report, 2)
	for i, final := range []bool{false, true} {
		s.EvalFinalOnly = final
		path := filepath.Join(dir, map[bool]string{false: "every.json", true: "final.json"}[final])
		if err := run(s, path, "", "", path+".ckpt", ""); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &reports[i]); err != nil {
			t.Fatal(err)
		}
	}
	every, final := reports[0], reports[1]
	if len(every.EpochAccuracy) != 3 || len(final.EpochAccuracy) != 1 || len(final.EpochValAccuracy) != 1 {
		t.Fatalf("epoch curves: every %v, final %v / %v", every.EpochAccuracy, final.EpochAccuracy, final.EpochValAccuracy)
	}
	if final.Accuracy != every.Accuracy || final.ValAccuracy != every.ValAccuracy || every.Accuracy != every.EpochAccuracy[2] {
		t.Fatalf("final-only test/val %v/%v, every-epoch %v/%v", final.Accuracy, final.ValAccuracy, every.Accuracy, every.ValAccuracy)
	}
}

// TestEvalFinalOnlyResumesFromItsCheckpoint pins that a final-only run can
// resume: its checkpoints hold no per-epoch scores before the last epoch and
// do not record EvalFinalOnly, which readCheckpoint once refused. An
// every-epoch run must still refuse a checkpoint with unscored epochs.
func TestEvalFinalOnlyResumesFromItsCheckpoint(t *testing.T) {
	dir := t.TempDir()
	writeSplit(t, dir, "train", 160)
	writeSplit(t, dir, "t10k", 40)
	s := settings{DataDir: dir, Nodes: 24, InputNodes: 8, ReadoutNodes: 8, InDegree: 3, Epochs: 3, Batch: 16, LearningRate: .02,
		TrainCount: 160, TestCount: 40, ValCount: 32, Workers: 2, Gate: true, GateSquash: "softsign", Activation: "softsign", SurrogateScale: 5, ChemTau: 5, ChemHill: 2, ChemSource: "all", ChemReceptors: 1, ChemInit: .01, EvalFinalOnly: true}

	mid := filepath.Join(dir, "mid.ckpt")
	if err := writeAtomic(mid, checkpoint{Settings: s, Epochs: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := readCheckpoint(mid, s); err != nil {
		t.Fatalf("final-only run refused its own mid-run checkpoint: %v", err)
	}
	every := s
	every.EvalFinalOnly = false
	if _, err := readCheckpoint(mid, every); err == nil {
		t.Fatal("every-epoch run accepted a checkpoint with unscored epochs")
	}

	first, second := filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")
	ckpt := filepath.Join(dir, "run.ckpt")
	if err := run(s, first, "", "", ckpt, ""); err != nil {
		t.Fatal(err)
	}
	if err := run(s, second, "", "", ckpt, ""); err != nil {
		t.Fatalf("resuming from a finished final-only checkpoint: %v", err)
	}
	var a, b report
	for path, r := range map[string]*report{first: &a, second: &b} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, r); err != nil {
			t.Fatal(err)
		}
	}
	if a.Accuracy != b.Accuracy || a.ValAccuracy != b.ValAccuracy {
		t.Fatalf("resumed run test/val %v/%v, original %v/%v", b.Accuracy, b.ValAccuracy, a.Accuracy, a.ValAccuracy)
	}
}

// TestWarmupFraction pins the schedule --warmup builds: zero keeps the 30%
// default, a fraction sets the warmup length, and the decay always ends at
// the last update.
func TestWarmupFraction(t *testing.T) {
	for _, c := range []struct {
		warmup float64
		want   uint64
	}{{0, 300}, {.1, 100}, {.05, 50}} {
		got := schedule(settings{Warmup: c.warmup}, 1000)
		if got.WarmupUpdates != c.want || got.WarmupUpdates+got.DecayUpdates != 1000 {
			t.Fatalf("--warmup %g: warmup %d decay %d, want warmup %d of 1000", c.warmup, got.WarmupUpdates, got.DecayUpdates, c.want)
		}
	}
	s := settings{Nodes: 24, InputNodes: 8, ReadoutNodes: 8, InDegree: 3, Epochs: 1, Batch: 16, LearningRate: .02, SurrogateScale: 5, Warmup: 1}
	if err := run(s, "", "", "", "", ""); err == nil || !strings.Contains(err.Error(), "--warmup") {
		t.Fatalf("--warmup 1 error = %v", err)
	}
}
