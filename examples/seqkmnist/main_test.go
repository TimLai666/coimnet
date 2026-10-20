package main

import (
	"compress/gzip"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
	c0, p0 := build(base, nil)

	s := base
	s.Delays = 3
	c, p := build(s, nil)
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
	c, p = build(s, nil)
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
	_, p = build(s, nil)
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
	c0, p0 := build(base, nil)
	s := base
	s.Core, s.SurrogateScale, s.Adapt, s.Homeostasis = "lif", 5, true, true
	c, p := build(s, nil)
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
