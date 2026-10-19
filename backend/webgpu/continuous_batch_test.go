package webgpu

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gogpu/naga"
)

// TestContinuousBatchKernelsCompile runs every batched kernel through the
// same pure Go WGSL compiler the device path uses, so a syntax or type error
// fails on a machine without a GPU.
func TestContinuousBatchKernelsCompile(t *testing.T) {
	for _, spec := range continuousBatchKernelSpec {
		if _, err := naga.Compile(spec.source); err != nil {
			t.Errorf("%s: %v", spec.label, err)
		}
		// One storage binding per declared buffer plus the uniform.
		if got, want := strings.Count(spec.source, "var<storage"), spec.readOnly+spec.readWrite; got != want {
			t.Errorf("%s declares %d storage bindings, spec says %d", spec.label, got, want)
		}
		if got := strings.Count(spec.source, "var<storage, read_write>"); got != spec.readWrite {
			t.Errorf("%s declares %d read-write bindings, spec says %d", spec.label, got, spec.readWrite)
		}
		if spec.readOnly+spec.readWrite > 8 {
			t.Errorf("%s needs %d storage bindings, above the portable limit 8", spec.label, spec.readOnly+spec.readWrite)
		}
	}
}

func TestContinuousBatchLayoutBuildsCSRAndTranspose(t *testing.T) {
	// Edges in no particular order, with a duplicate (0->1 twice) and a
	// self-loop, so both orders and both permutations are visible.
	cfg := ContinuousBatchConfig{
		Nodes:        3,
		Sources:      []int{2, 0, 1, 0, 2, 1},
		Targets:      []int{1, 1, 2, 1, 0, 1},
		Activation:   ActivationTanh,
		Gate:         GateSigmoid,
		InputNodes:   []int{2, 0},
		ReadoutNodes: []int{1},
	}
	l, err := buildContinuousBatchLayout(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n, e := 3, 6
	offsets := l.topology[:n+1]
	slotSource := l.topology[n+1 : n+1+e]
	slotTarget := l.topology[n+1+e : n+1+2*e]
	tOffsets := l.topology[n+1+2*e : 2*n+2+2*e]
	tTarget := l.topology[2*n+2+2*e : 2*n+2+3*e]
	tSlot := l.topology[2*n+2+3*e:]
	wantSlotEdge := []int{4, 0, 1, 3, 5, 2} // target 0: e4; target 1: e0 e1 e3 e5; target 2: e2
	equalInts(t, "slot edges", l.slotEdge, wantSlotEdge)
	equalU32(t, "offsets", offsets, []uint32{0, 1, 5, 6})
	for slot, edge := range l.slotEdge {
		if int(slotSource[slot]) != cfg.Sources[edge] || int(slotTarget[slot]) != cfg.Targets[edge] {
			t.Fatalf("slot %d holds %d->%d, edge %d is %d->%d", slot, slotSource[slot], slotTarget[slot], edge, cfg.Sources[edge], cfg.Targets[edge])
		}
	}
	// By source, declared order: source 0: e1 e3; source 1: e2 e5; source 2: e0 e4.
	equalU32(t, "transpose offsets", tOffsets, []uint32{0, 2, 4, 6})
	wantEdges := []int{1, 3, 2, 5, 0, 4}
	for k, edge := range wantEdges {
		if l.slotEdge[tSlot[k]] != edge || int(tTarget[k]) != cfg.Targets[edge] {
			t.Fatalf("transpose entry %d is slot %d target %d, want edge %d target %d", k, tSlot[k], tTarget[k], edge, cfg.Targets[edge])
		}
	}
	none := noneIndex
	equalU32(t, "node index", l.nodeIndex, []uint32{1, none, 0, none, 0, none, 2, 0, 1})
	if l.width != 2 || l.readout != 1 || l.gate != 1 || l.activation != 0 {
		t.Fatalf("layout %+v", l)
	}
}

func TestContinuousBatchLayoutDefaultsInputToEveryNode(t *testing.T) {
	l, err := buildContinuousBatchLayout(ContinuousBatchConfig{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Activation: ActivationSoftsign, ReadoutNodes: []int{0, 1}, ReadoutEveryStep: true})
	if err != nil {
		t.Fatal(err)
	}
	equalU32(t, "node index", l.nodeIndex, []uint32{0, 1, 0, 1, 0, 1, 0, 1})
	if l.readSteps(5) != 5 {
		t.Fatalf("every-step layout reads %d steps of 5", l.readSteps(5))
	}
	l.everyStep = false
	if l.readSteps(5) != 1 {
		t.Fatalf("last-step layout reads %d steps", l.readSteps(5))
	}
}

func TestContinuousBatchRefusesInvalidConfigBeforeDeviceOpen(t *testing.T) {
	base := func() ContinuousBatchConfig {
		return ContinuousBatchConfig{Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2}, Activation: ActivationTanh, ReadoutNodes: []int{2}}
	}
	cases := map[string]func(*ContinuousBatchConfig){
		"zero nodes":          func(c *ContinuousBatchConfig) { c.Nodes = 0 },
		"edge out of range":   func(c *ContinuousBatchConfig) { c.Targets[0] = 3 },
		"edge length":         func(c *ContinuousBatchConfig) { c.Targets = c.Targets[:1] },
		"activation":          func(c *ContinuousBatchConfig) { c.Activation = "relu" },
		"empty activation":    func(c *ContinuousBatchConfig) { c.Activation = "" },
		"gate":                func(c *ContinuousBatchConfig) { c.Gate = "voltage" },
		"no readout":          func(c *ContinuousBatchConfig) { c.ReadoutNodes = nil },
		"readout range":       func(c *ContinuousBatchConfig) { c.ReadoutNodes = []int{3} },
		"duplicate readout":   func(c *ContinuousBatchConfig) { c.ReadoutNodes = []int{1, 1} },
		"empty input nodes":   func(c *ContinuousBatchConfig) { c.InputNodes = []int{} },
		"duplicate input":     func(c *ContinuousBatchConfig) { c.InputNodes = []int{0, 0} },
		"negative input node": func(c *ContinuousBatchConfig) { c.InputNodes = []int{-1} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := base()
			mutate(&cfg)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := NewContinuousBatch(ctx, cfg)
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error %v is not ErrInvalidConfig (checked after device open?)", err)
			}
		})
	}
}

func TestContinuousBatchRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewContinuousBatch(ctx, ContinuousBatchConfig{Nodes: 1, Activation: ActivationTanh, ReadoutNodes: []int{0}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
	if _, err := NewContinuousBatch(nil, ContinuousBatchConfig{}); err == nil { //nolint:staticcheck // nil context is the case under test
		t.Fatal("nil context accepted")
	}
}

func TestContinuousBatchValidatesParameterShapes(t *testing.T) {
	l, err := buildContinuousBatchLayout(ContinuousBatchConfig{Nodes: 2, Sources: []int{0, 1, 1}, Targets: []int{1, 0, 1}, Activation: ActivationTanh, Gate: GateSoftsign, ReadoutNodes: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	good := func() ContinuousBatchParameters {
		v := func(n int) []float32 { return make([]float32, n) }
		return ContinuousBatchParameters{Weights: v(3), Bias: v(2), Lambda: v(2), Alpha: v(2), TauSlope: v(2), GateGain: v(2), GateBias: v(2)}
	}
	if err := l.validateParameters(good()); err != nil {
		t.Fatal(err)
	}
	p := good()
	p.Weights = p.Weights[:2]
	if err := l.validateParameters(p); !errors.Is(err, ErrShape) {
		t.Fatalf("short weights: %v", err)
	}
	p = good()
	p.GateBias = nil
	if err := l.validateParameters(p); !errors.Is(err, ErrShape) {
		t.Fatalf("missing gate bias: %v", err)
	}
	p = good()
	p.Alpha[1] = float32(math.Inf(1))
	if err := l.validateParameters(p); !errors.Is(err, ErrNonFinite) {
		t.Fatalf("infinite alpha: %v", err)
	}
	l.gate = 0
	if err := l.validateParameters(good()); !errors.Is(err, ErrShape) {
		t.Fatalf("gate values on an ungated layout: %v", err)
	}
}

func TestContinuousBatchEpisodeCounts(t *testing.T) {
	l, err := buildContinuousBatchLayout(ContinuousBatchConfig{Nodes: 4, Sources: []int{0, 1, 2}, Targets: []int{1, 2, 3}, Activation: ActivationTanh, InputNodes: []int{0}, ReadoutNodes: []int{2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	counts, err := l.episodeCounts(5, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]uint64{"inputs": 35, "voltage": 160, "output": 160, "history": 560, "upstream": 10, "drive": 140, "partial": 420, "carry": 40, "readout": 10, "gradient": 19, "inputGrad": 35}
	for name, count := range want {
		if counts[name] != count {
			t.Errorf("%s count %d, want %d", name, counts[name], count)
		}
	}
	if _, err := l.episodeCounts(0, 3); !errors.Is(err, ErrShape) {
		t.Fatalf("zero batch: %v", err)
	}
	if _, err := l.episodeCounts(1<<20, 1<<12); !errors.Is(err, ErrCapacity) {
		t.Fatalf("u32 overflow: %v", err)
	}
}

func TestDispatchGroupsCoversEveryInvocation(t *testing.T) {
	for _, tc := range []struct {
		count uint64
		limit uint32
		x, y  uint32
	}{{1, 65535, 1, 1}, {64, 65535, 1, 1}, {65, 65535, 2, 1}, {64 * 10, 4, 4, 3}, {0, 8, 1, 1}} {
		x, y, err := dispatchGroups(tc.count, tc.limit)
		if err != nil || x != tc.x || y != tc.y {
			t.Fatalf("dispatchGroups(%d, %d) = %d, %d, %v; want %d, %d", tc.count, tc.limit, x, y, err, tc.x, tc.y)
		}
		if uint64(x)*uint64(y)*64 < tc.count {
			t.Fatalf("grid %dx%d misses invocations of %d", x, y, tc.count)
		}
	}
	if _, _, err := dispatchGroups(64*17, 4); !errors.Is(err, ErrCapacity) {
		t.Fatalf("oversized grid: %v", err)
	}
}

func TestContinuousBatchDimsEncoding(t *testing.T) {
	l := continuousBatchLayout{nodes: 9, edges: 20, width: 3, readout: 2, activation: 1, gate: 2, everyStep: true}
	data := l.encodeDims(4, 6, 11)
	if uint64(len(data)) != continuousBatchUniformBytes {
		t.Fatalf("uniform has %d bytes", len(data))
	}
	want := []uint32{4, 6, 9, 11, 3, 2, 11, 20, 1, 2, 1, 0, 0, 0, 0, 0}
	for i, w := range want {
		if got := binary.LittleEndian.Uint32(data[4*i:]); got != w {
			t.Fatalf("dims word %d = %d, want %d", i, got, w)
		}
	}
}

func TestContinuousBatchSplitGradientRestoresDeclaredOrder(t *testing.T) {
	l := continuousBatchLayout{nodes: 2, edges: 3, gate: 1, slotEdge: []int{2, 0, 1}}
	sums := []float32{10, 20, 30, 1, 2, 3, 4, 5, 6, 7, 8}
	g := l.splitGradient(sums, []float32{9})
	equalF32(t, "weights", g.Weights, []float32{20, 30, 10})
	equalF32(t, "bias", g.Bias, []float32{1, 2})
	equalF32(t, "log tau", g.LogTau, []float32{3, 4})
	equalF32(t, "gate gain", g.GateGain, []float32{5, 6})
	equalF32(t, "gate bias", g.GateBias, []float32{7, 8})
	l.gate = 0
	if g := l.splitGradient(sums, nil); g.GateGain != nil || g.GateBias != nil {
		t.Fatal("ungated split returned gate gradients")
	}
}

func equalInts(t *testing.T, name string, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}

func equalU32(t *testing.T, name string, got, want []uint32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}

func equalF32(t *testing.T, name string, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}
