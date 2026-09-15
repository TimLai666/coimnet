package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// signsConfig is the mixed fixture: a three-neuron ring plus one shortcut, so
// edges 0 and 2 can carry a fixed sign while edges 1 and 3 stay free.
func signsConfig() learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 3, Sources: []int{0, 1, 2, 0}, Targets: []int{1, 2, 0, 2},
			DT: .5, Activation: "tanh",
		},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{2},
	}
}

// signsParameters stores raw values: edges 0 and 2 hold log magnitudes, edges
// 1 and 3 hold their weights directly.
func signsParameters() learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{math.Log(.6), .4, math.Log(.5), -.3},
			Bias:    []float64{.05, -.05, .1},
			LogTau:  []float64{.1, 0, -.1},
		},
		Encoder: []float64{1, 0, 0},
		Readout: []float64{1},
	}
}

func signsInput() ([][]float64, []float64) {
	return [][]float64{{.9}, {-.4}, {.3}, {.2}}, []float64{.5}
}

// TestEffectiveWeightsMapsRawValuesThroughTheDeclaredSigns pins the
// parametrization itself: free edges pass through and fixed edges become
// sign * exp(raw).
func TestEffectiveWeightsMapsRawValuesThroughTheDeclaredSigns(t *testing.T) {
	c, p := signsConfig(), signsParameters()
	c.EdgeSigns = []int8{1, 0, -1, 0}
	got, err := learning.EffectiveWeights(c, p)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{math.Exp(math.Log(.6)), .4, -math.Exp(math.Log(.5)), -.3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("effective weight %d = %.17g, want %.17g", i, got[i], want[i])
		}
	}
	// Free everywhere is the identity, and the returned slice is owned.
	free := signsConfig()
	identity, err := learning.EffectiveWeights(free, p)
	if err != nil {
		t.Fatal(err)
	}
	identity[0] = 1234
	if p.Core.Weights[0] == 1234 {
		t.Fatal("EffectiveWeights aliased the caller's raw weights")
	}
	for i, raw := range p.Core.Weights {
		if i == 0 {
			continue
		}
		if identity[i] != raw {
			t.Fatalf("free edge %d = %.17g, want the raw value %.17g", i, identity[i], raw)
		}
	}
}

// TestEdgeSignsAndMinLogMagnitudeAreValidated keeps a malformed declaration
// from reaching the core.
func TestEdgeSignsAndMinLogMagnitudeAreValidated(t *testing.T) {
	base, p := signsConfig(), signsParameters()
	for name, mutate := range map[string]func(c *learning.Config){
		"short signs":            func(c *learning.Config) { c.EdgeSigns = []int8{1, 0, -1} },
		"long signs":             func(c *learning.Config) { c.EdgeSigns = []int8{1, 0, -1, 0, 0} },
		"sign out of range":      func(c *learning.Config) { c.EdgeSigns = []int8{2, 0, -1, 0} },
		"non-finite floor":       func(c *learning.Config) { c.MinLogMagnitude = math.Inf(-1) },
		"unrepresentable floor":  func(c *learning.Config) { c.EdgeSigns = []int8{1, 0, -1, 0}; c.MinLogMagnitude = 1000 },
		"nan floor":              func(c *learning.Config) { c.MinLogMagnitude = math.NaN() },
		"floor without any sign": func(c *learning.Config) { c.MinLogMagnitude = math.Inf(1) },
	} {
		c := base
		mutate(&c)
		if _, err := learning.NewNetwork(c); err == nil {
			t.Fatalf("NewNetwork accepted %s", name)
		}
		if _, err := learning.EffectiveWeights(c, p); err == nil {
			t.Fatalf("EffectiveWeights accepted %s", name)
		}
	}
	// A raw value that underflows to a zero magnitude is not representable.
	under := base
	under.EdgeSigns = []int8{1, 0, -1, 0}
	q := signsParameters()
	q.Core.Weights[0] = -800
	if _, err := learning.EffectiveWeights(under, q); err == nil {
		t.Fatal("EffectiveWeights accepted an underflowing log magnitude")
	}
}

// TestAllZeroEdgeSignsAreBitIdenticalToNoDeclarationAtAll is the compatibility
// touchstone of root decision 2.
func TestAllZeroEdgeSignsAreBitIdenticalToNoDeclarationAtAll(t *testing.T) {
	x, target := signsInput()
	run := func(signs []int8) learning.TrainingSnapshot {
		t.Helper()
		c := signsConfig()
		c.EdgeSigns = signs
		tr, err := learning.NewTrainer(c, signsParameters(), learning.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		for step := range 20 {
			if _, err := tr.Step(context.Background(), x, target); err != nil {
				t.Fatalf("step %d: %v", step, err)
			}
		}
		return tr.Snapshot()
	}
	before := run(nil)
	after := run([]int8{0, 0, 0, 0})
	beforeJSON, err := json.Marshal(struct {
		P learning.Parameters
		O learning.AdamState
		U uint64
	}{before.Parameters, before.Optimizer, before.Updates})
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, err := json.Marshal(struct {
		P learning.Parameters
		O learning.AdamState
		U uint64
	}{after.Parameters, after.Optimizer, after.Updates})
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("all-zero edge signs changed the result\n%s\n%s", beforeJSON, afterJSON)
	}
	if after.Config.EdgeSigns == nil {
		t.Fatal("an explicit all-zero declaration must survive the snapshot")
	}
}

// TestSnapshotWithoutEdgeSignsLoadsAsAllFree covers every checkpoint written
// before this ticket.
func TestSnapshotWithoutEdgeSignsLoadsAsAllFree(t *testing.T) {
	tr, err := learning.NewTrainer(signsConfig(), signsParameters(), learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(tr.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"edge_signs", "min_log_magnitude", "masks", "ranges"} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("an unused %s field was written: %s", field, encoded)
		}
	}
	var decoded learning.TrainingSnapshot
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Config.EdgeSigns != nil || decoded.Config.MinLogMagnitude != 0 {
		t.Fatalf("an old snapshot did not load as all-free: %+v", decoded.Config.EdgeSigns)
	}
	restored, err := learning.RestoreTrainer(decoded)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := signsInput()
	want, err := tr.Predict(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Predict(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != want[0] {
		t.Fatalf("restored prediction %.17g != %.17g", got[0], want[0])
	}
}

// TestFixedSignEdgesNeverFlipUnderALargeLearningRate is the acceptance item of
// root decision 2: one thousand updates at learning rate 1.0 and the two fixed
// edges keep their declared sign with a strictly positive magnitude.
func TestFixedSignEdgesNeverFlipUnderALargeLearningRate(t *testing.T) {
	c, p := signsConfig(), signsParameters()
	c.EdgeSigns = []int8{1, 0, -1, 0}
	o := learning.DefaultOptions()
	o.LearningRate = 1
	o.Trainable = learning.Trainable{Weights: true, Readout: true}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	x, target := signsInput()
	minMagnitude := [2]float64{math.Inf(1), math.Inf(1)}
	for step := range 1000 {
		if _, err := tr.Step(context.Background(), x, target); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		s := tr.Snapshot()
		w, err := learning.EffectiveWeights(s.Config, s.Parameters)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for slot, edge := range [2]int{0, 2} {
			want := float64(c.EdgeSigns[edge])
			if w[edge] == 0 || math.Signbit(w[edge]) != math.Signbit(want) {
				t.Fatalf("step %d: edge %d effective weight %.17g flipped away from sign %+d", step, edge, w[edge], c.EdgeSigns[edge])
			}
			if m := math.Abs(w[edge]); m < minMagnitude[slot] {
				minMagnitude[slot] = m
			}
		}
	}
	for slot, edge := range [2]int{0, 2} {
		if !(minMagnitude[slot] > 0) {
			t.Fatalf("edge %d reached magnitude %g", edge, minMagnitude[slot])
		}
		t.Logf("edge %d smallest magnitude over 1000 steps: %.17g", edge, minMagnitude[slot])
	}
	s := tr.Snapshot()
	t.Logf("final raw weights: %v", s.Parameters.Core.Weights)
}

// TestMinLogMagnitudeProjectionKeepsTheMagnitudeRepresentable drives the floor
// directly instead of waiting for a run to find it.
func TestMinLogMagnitudeProjectionKeepsTheMagnitudeRepresentable(t *testing.T) {
	c, p := signsConfig(), signsParameters()
	c.EdgeSigns = []int8{1, 0, -1, 0}
	c.MinLogMagnitude = -4
	p.Core.Weights[0] = -3.9 // one learning-rate step below the floor
	o := learning.DefaultOptions()
	o.LearningRate = 1
	o.Trainable = learning.Trainable{Weights: true}
	tr, err := learning.NewTrainer(c, p, o)
	if err != nil {
		t.Fatal(err)
	}
	x, target := signsInput()
	projected := 0
	for step := range 30 {
		result, err := tr.Step(context.Background(), x, target)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		projected += result.Projected["min_log_magnitude"]
		s := tr.Snapshot()
		for _, edge := range []int{0, 2} {
			if s.Parameters.Core.Weights[edge] < c.MinLogMagnitude {
				t.Fatalf("step %d: raw edge %d = %.17g is below the floor %g", step, edge, s.Parameters.Core.Weights[edge], c.MinLogMagnitude)
			}
		}
	}
	if projected == 0 {
		t.Fatal("the floor never engaged, the fixture proves nothing")
	}
	t.Logf("min_log_magnitude projections over 30 steps: %d", projected)
	// A free edge is never touched by the floor.
	if tr.Snapshot().Parameters.Core.Weights[1] == p.Core.Weights[1] {
		t.Fatal("the free edge did not move at all")
	}
}

// TestChainRuleGradientMatchesCentralFiniteDifferences verifies
// d loss/d rho = d loss/d w * w against the continuous core, with free and
// fixed edges in the same model.
func TestChainRuleGradientMatchesCentralFiniteDifferences(t *testing.T) {
	c, p := signsConfig(), signsParameters()
	c.EdgeSigns = []int8{1, 0, -1, 0}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	x, target := signsInput()
	_, g, err := n.LossGradient(context.Background(), p, x, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	loss := func() float64 {
		t.Helper()
		y, err := n.Predict(context.Background(), p, x)
		if err != nil {
			t.Fatal(err)
		}
		d := y[0] - target[0]
		return d * d
	}
	const eps = 1e-2
	var report strings.Builder
	fmt.Fprintf(&report, "%-6s %-6s %-22s %-22s %s\n", "edge", "sign", "analytic", "central difference", "relative error")
	worst := 0.0
	for edge := range p.Core.Weights {
		old := p.Core.Weights[edge]
		p.Core.Weights[edge] = old + eps
		up := loss()
		p.Core.Weights[edge] = old - eps
		down := loss()
		p.Core.Weights[edge] = old
		fd := (up - down) / (2 * eps)
		rel := math.Abs(g.Core.Weights[edge]-fd) / math.Abs(fd)
		fmt.Fprintf(&report, "%-6d %-6d %-22.15g %-22.15g %.3g\n", edge, c.EdgeSigns[edge], g.Core.Weights[edge], fd, rel)
		if rel > worst {
			worst = rel
		}
	}
	t.Logf("chain-rule gradient against central finite differences (eps=%g)\n%s", eps, report.String())
	if worst > 1e-4 {
		t.Fatalf("worst relative error %.3g exceeds 1e-4", worst)
	}
	// Without the chain rule the fixed edges would report d loss/d w instead,
	// so make sure the two really differ on this fixture.
	free := signsConfig()
	fn, err := learning.NewNetwork(free)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := learning.EffectiveWeights(c, p)
	if err != nil {
		t.Fatal(err)
	}
	q := signsParameters()
	q.Core.Weights = effective
	_, wg, err := fn.LossGradient(context.Background(), q, x, target, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range []int{0, 2} {
		want := wg.Core.Weights[edge] * effective[edge]
		if math.Abs(g.Core.Weights[edge]-want) > 1e-12*math.Abs(want) {
			t.Fatalf("edge %d chain rule gave %.17g, want d loss/d w * w = %.17g", edge, g.Core.Weights[edge], want)
		}
		if math.Abs(wg.Core.Weights[edge]-g.Core.Weights[edge]) < 1e-9 {
			t.Fatalf("edge %d: the fixture cannot tell the chain rule apart from d loss/d w", edge)
		}
	}
}

// TestFixedSignWeightsReachTheCoreAndThePersistentPath makes sure every entry
// point converts raw values, not only the trainer.
func TestFixedSignWeightsReachTheCoreAndThePersistentPath(t *testing.T) {
	c, p := signsConfig(), signsParameters()
	c.EdgeSigns = []int8{1, 0, -1, 0}
	n, err := learning.NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := learning.EffectiveWeights(c, p)
	if err != nil {
		t.Fatal(err)
	}
	q := signsParameters()
	q.Core.Weights = effective
	reference, err := learning.NewNetwork(signsConfig())
	if err != nil {
		t.Fatal(err)
	}
	x, _ := signsInput()
	got, err := n.Predict(context.Background(), p, x)
	if err != nil {
		t.Fatal(err)
	}
	want, err := reference.Predict(context.Background(), q, x)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != want[0] {
		t.Fatalf("Predict with fixed signs = %.17g, want %.17g", got[0], want[0])
	}
	// The persistent individual drives the same core through its own path.
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, 3))
	if err != nil {
		t.Fatal(err)
	}
	referenceIndividual, err := learning.NewIndividual(signsConfig(), q, learning.DefaultOptions(), make([]float64, 3))
	if err != nil {
		t.Fatal(err)
	}
	gotRows, err := individual.Advance(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	wantRows, err := referenceIndividual.Advance(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	for i := range wantRows {
		if gotRows[i][0] != wantRows[i][0] {
			t.Fatalf("Advance step %d = %.17g, want %.17g", i, gotRows[i][0], wantRows[i][0])
		}
	}
}
