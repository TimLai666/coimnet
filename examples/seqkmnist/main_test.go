package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
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
