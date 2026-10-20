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
	cfgOff, pOff := build(off, nil)
	if cfgOff.Dynamics.Chemistry != nil || pOff.Core.ChemRelease != nil || pOff.Core.ChemGain != nil || pOff.Core.ChemOffset != nil {
		t.Fatal("chemistry present without --chem-channels")
	}
	for _, source := range []string{"all", "split", "input", "readout", "hidden"} {
		on := base
		on.ChemChannels, on.ChemTau, on.ChemHill, on.ChemSource, on.ChemReceptors, on.ChemInit = 3, 4, 2, source, .5, .01
		if err := checkChemistry(on); err != nil {
			t.Fatal(err)
		}
		cfgOn, pOn := build(on, nil)
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
		cfg, p := build(on, nil)
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
