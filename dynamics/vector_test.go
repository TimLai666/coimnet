package dynamics

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
)

func TestVectorLayoutCounts(t *testing.T) {
	tests := []struct {
		shape    string
		wantW    int
		wantEdge int
	}{
		{"scalar", 4, 1},
		{"matrix", 36, 9},
	}
	for _, tc := range tests {
		l, err := NewVectorLayout(3, 4, 3, tc.shape)
		if err != nil {
			t.Fatal(err)
		}
		if got := l.NodeValues(); got != 9 {
			t.Fatalf("%s: NodeValues() = %d, want 9", tc.shape, got)
		}
		if got := l.WeightValues(); got != tc.wantW {
			t.Fatalf("%s: WeightValues() = %d, want %d", tc.shape, got, tc.wantW)
		}
		w, b, lt := l.ParameterCount()
		if w != tc.wantW || b != 9 || lt != 3 {
			t.Fatalf("%s: ParameterCount() = (%d, %d, %d), want (%d, 9, 3)", tc.shape, w, b, lt, tc.wantW)
		}
		values := make([]float64, l.NodeValues())
		for i := range values {
			values[i] = float64(i)
		}
		if got := l.NodeSlice(values, 2); len(got) != 3 || got[0] != 6 || got[2] != 8 {
			t.Fatalf("%s: NodeSlice(values, 2) = %v, want [6 7 8]", tc.shape, got)
		}
		weights := make([]float64, l.WeightValues())
		edge := l.EdgeMatrix(weights, 1)
		if len(edge) != tc.wantEdge {
			t.Fatalf("%s: EdgeMatrix(weights, 1) length = %d, want %d", tc.shape, len(edge), tc.wantEdge)
		}
	}
}

func TestApplyEdgeScalarBroadcast(t *testing.T) {
	l, err := NewVectorLayout(1, 1, 3, "scalar")
	if err != nil {
		t.Fatal(err)
	}
	drive := []float64{0, 0, 0}
	if err := l.ApplyEdge([]float64{2}, 0, []float64{1, 2, 3}, drive); err != nil {
		t.Fatal(err)
	}
	want := []float64{2, 4, 6}
	for i := range want {
		if drive[i] != want[i] {
			t.Fatalf("drive[%d] = %g, want %g", i, drive[i], want[i])
		}
	}
}

func TestApplyEdgeMatrixHandComputed(t *testing.T) {
	l, err := NewVectorLayout(1, 1, 2, "matrix")
	if err != nil {
		t.Fatal(err)
	}
	drive := []float64{0, 0}
	source := []float64{1, 1}
	W := []float64{1, 2, 3, 4} // row-major [[1,2],[3,4]]
	if err := l.ApplyEdge(W, 0, source, drive); err != nil {
		t.Fatal(err)
	}
	if drive[0] != 3 || drive[1] != 7 {
		t.Fatalf("after first apply drive = %v, want [3 7]", drive)
	}
	if err := l.ApplyEdge(W, 0, source, drive); err != nil {
		t.Fatal(err)
	}
	if drive[0] != 6 || drive[1] != 14 {
		t.Fatalf("after second apply drive = %v, want [6 14]", drive)
	}
}

func TestApplyEdgeScalarIsBitIdenticalToEagerLoop(t *testing.T) {
	l, err := NewVectorLayout(1, 1, 1, "scalar")
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		w := 2*rng.Float64() - 1
		y := []float64{2*rng.Float64() - 1}
		var eager float64
		eager += w * y[0]
		drive := []float64{0}
		if err := l.ApplyEdge([]float64{w}, 0, y, drive); err != nil {
			t.Fatal(err)
		}
		if drive[0] != eager {
			t.Fatalf("iter %d: ApplyEdge = %g, eager = %g, want bit-identical", i, drive[0], eager)
		}
	}
}

func TestConfigVectorFieldsValidation(t *testing.T) {
	valid := func() Config {
		return Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh"}
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // empty means accepted
	}{
		{"state -1", func(c *Config) { c.StateDimension = -1 }, "state dimension must be in [0, 64]"},
		{"state 65", func(c *Config) { c.StateDimension = 65 }, "state dimension must be in [0, 64]"},
		{"bad edge shape", func(c *Config) { c.EdgeShape = "foo" }, "unsupported edge shape"},
		{"scalar with matrix", func(c *Config) { c.StateDimension = 1; c.EdgeShape = "matrix" }, "matrix requires a vector state dimension"},
		{"vector not wired", func(c *Config) { c.StateDimension = 3; c.EdgeShape = "matrix" }, "use NewVectorContinuous for a vector state"},
		{"default scalar ok", func(c *Config) {}, ""},
		{"explicit scalar ok", func(c *Config) { c.EdgeShape = "scalar" }, ""},
		{"dim 1 ok", func(c *Config) { c.StateDimension = 1 }, ""},
		{"dim 1 scalar ok", func(c *Config) { c.StateDimension = 1; c.EdgeShape = "scalar" }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(&c)
			_, err := NewContinuous(c)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("NewContinuous rejected valid config: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("NewContinuous accepted config, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
	// The pre-existing default config keeps its fingerprint: the new keys
	// must stay out of the JSON output.
	c := valid()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "state_dimension") || strings.Contains(string(data), "edge_shape") {
		t.Fatalf("default config JSON gained vector keys: %s", data)
	}
}
