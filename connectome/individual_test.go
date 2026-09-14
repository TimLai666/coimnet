package connectome

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func TestIndividualLearningPreservesAnatomyStore(t *testing.T) {
	manifest, _ := happyFixture(t, []int{7, 4})
	g := build(t, manifest, fixtureLimits()).Graph
	ctx := context.Background()
	dir := t.TempDir()
	beforePath := filepath.Join(dir, "before.coimgraph")
	before, err := Save(ctx, beforePath, g)
	if err != nil {
		t.Fatal(err)
	}
	c := learning.Config{Dynamics: dynamics.Config{Nodes: int(g.NodeCount()), DT: .5, Activation: "tanh"}, InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1}}
	p := learning.Parameters{Core: dynamics.Parameters{Bias: make([]float64, c.Dynamics.Nodes), LogTau: make([]float64, c.Dynamics.Nodes)}, Encoder: make([]float64, c.Dynamics.Nodes), Readout: []float64{.8}}
	for j := range p.Encoder {
		p.Encoder[j] = .3
	}
	if err = g.StreamAnnotatedEdges(ctx, func(e EdgeRecord) error {
		c.Dynamics.Sources = append(c.Dynamics.Sources, int(e.Source))
		c.Dynamics.Targets = append(c.Dynamics.Targets, int(e.Target))
		p.Core.Weights = append(p.Core.Weights, .02)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a, err := learning.NewIndividual(c, p, learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	baseline := a.Snapshot()
	for range 10 {
		if _, err = a.TrainEpisode(ctx, [][]float64{{.4}, {0}, {0}}, []float64{.5}); err != nil {
			t.Fatal(err)
		}
	}
	if reflect.DeepEqual(baseline.Parameters, a.Snapshot().Parameters) {
		t.Fatal("fixture did not actually train")
	}
	if _, err = a.Advance(ctx, [][]float64{{.2}, {0}}); err != nil {
		t.Fatal(err)
	}
	if err = a.ResetNeural(ctx, make([]float64, c.Dynamics.Nodes)); err != nil {
		t.Fatal(err)
	}
	if err = a.ResetParameters(ctx, baseline.Parameters); err != nil {
		t.Fatal(err)
	}
	if err = a.ResetOptimizer(ctx, learning.DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	afterPath := filepath.Join(dir, "after.coimgraph")
	after, err := Save(ctx, afterPath, g)
	if err != nil {
		t.Fatal(err)
	}
	if before.SHA256 != after.SHA256 || fileSHA256(t, beforePath) != fileSHA256(t, afterPath) {
		t.Fatal("learning/reset changed immutable source graph")
	}
	original, err := Load(ctx, beforePath, storeLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertSameGraph(t, original, g)
	t.Logf("anatomy SHA-256 unchanged: %s", before.SHA256)
}
