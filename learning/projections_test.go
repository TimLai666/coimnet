package learning_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/signal"
)

func bindingFixture(t *testing.T) (learning.Config, learning.Parameters, []signal.NeuronCandidate, signal.Projection, signal.Projection) {
	t.Helper()
	candidates := []signal.NeuronCandidate{{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "b"}}, {ID: signal.NeuronID{Namespace: "fixture", ExternalID: "a"}}, {ID: signal.NeuronID{Namespace: "fixture", ExternalID: "c"}}}
	c := learning.Config{Dynamics: dynamics.Config{Nodes: 3, Sources: []int{1, 2}, Targets: []int{0, 0}, DT: 0.5, Activation: "tanh"}, InputSize: 2, OutputSize: 1, ReadoutNodes: []int{0}}
	p := learning.Parameters{Core: dynamics.Parameters{Weights: []float64{0.3, -0.2}, Bias: []float64{0.1, -0.1, 0.2}, LogTau: []float64{0, 0, 0}}}
	input, err := signal.NewProjection(signal.ProjectionSpec{SchemaVersion: signal.CurrentSchemaVersion(), Direction: signal.ProjectionInput, Source: "fixture/v1", Artificial: true, ChannelShape: []int{2}, Selection: signal.NeuronSelection{Mode: signal.SelectExplicit, Neurons: []signal.NeuronID{candidates[1].ID, candidates[2].ID}}, Weights: []float64{0.5, 0.1, -0.3, 0.2}}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	output, err := signal.NewProjection(signal.ProjectionSpec{SchemaVersion: signal.CurrentSchemaVersion(), Direction: signal.ProjectionOutput, Source: "fixture/v1", Artificial: true, ChannelShape: []int{1}, Selection: signal.NeuronSelection{Mode: signal.SelectExplicit, Neurons: []signal.NeuronID{candidates[0].ID}}, Weights: []float64{0.8}}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	return c, p, candidates, input, output
}

func TestBindProjectionsRoundTripAndReplacement(t *testing.T) {
	c, p, candidates, input, output := bindingFixture(t)
	cfg, params, err := learning.BindProjections(c, p, candidates, input, output)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.InputNodes, []int{1, 2}) || !reflect.DeepEqual(cfg.ReadoutNodes, []int{0}) {
		t.Fatal("incorrect binding order")
	}
	trainer, err := learning.NewTrainer(cfg, params, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	x := [][]float64{{0.7, -0.2}, {0.1, 0.4}, {-0.3, 0.2}}
	for i := 0; i < 3; i++ {
		if _, err := trainer.Step(context.Background(), x, []float64{0.4}); err != nil {
			t.Fatal(err)
		}
	}
	before := trainer.Snapshot()
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := signal.DecodeProjection(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	cfg2, params2, err := learning.BindProjections(c, p, candidates, input, restored)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, cfg2) || !reflect.DeepEqual(params, params2) {
		t.Fatal("persisted projection reconstructed a different model")
	}
	spec := output.Spec()
	spec.Selection.Neurons = []signal.NeuronID{candidates[2].ID}
	spec.Weights = []float64{0.25}
	replacement, err := signal.NewProjection(spec, candidates)
	if err != nil {
		t.Fatal(err)
	}
	replacedConfig, replacedParams, err := learning.BindProjections(before.Config, before.Parameters, candidates, input, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replacedParams.Core, before.Parameters.Core) {
		t.Fatal("replacement altered core parameters")
	}
	next, err := learning.NewTrainer(replacedConfig, replacedParams, learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if next.Snapshot().Updates != 0 {
		t.Fatal("replacement reused optimizer")
	}
	if _, err := next.Step(context.Background(), x, []float64{0.7}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, trainer.Snapshot()) {
		t.Fatal("replacement mutated old trainer")
	}
	oldPrediction, err := trainer.Predict(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	newPrediction, err := next.Predict(context.Background(), x)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(oldPrediction, newPrediction) {
		t.Fatal("replacement did not affect output")
	}
	params2.Core.Bias[0] = 99
	cfg2.Dynamics.Sources[0] = 0
	if p.Core.Bias[0] != 0.1 || c.Dynamics.Sources[0] != 1 || params.Core.Bias[0] != 0.1 {
		t.Fatal("binding shares mutable inputs")
	}
}

func TestBindProjectionsRejectsInvalidBoundary(t *testing.T) {
	c, p, candidates, input, output := bindingFixture(t)
	for _, tc := range []struct {
		name string
		edit func(*learning.Config, *learning.Parameters, *[]signal.NeuronCandidate, *signal.Projection, *signal.Projection)
	}{
		{"direction", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			*i, *o = *o, *i
		}},
		{"input width", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			c.InputSize++
		}},
		{"output width", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			c.OutputSize++
		}},
		{"node count", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			*g = (*g)[:2]
		}},
		{"absent ID", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			(*g)[0].ID.ExternalID = "absent"
		}},
		{"core shape", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			p.Core.Bias = nil
		}},
		{"core nonfinite", func(c *learning.Config, p *learning.Parameters, g *[]signal.NeuronCandidate, i, o *signal.Projection) {
			p.Core.Bias[0] = math.Inf(1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc, pp, gg, ii, oo := bindingFixture(t)
			tc.edit(&cc, &pp, &gg, &ii, &oo)
			badConfig, badParams, err := learning.BindProjections(cc, pp, gg, ii, oo)
			if err == nil || !reflect.DeepEqual(badConfig, learning.Config{}) || !reflect.DeepEqual(badParams, learning.Parameters{}) {
				t.Fatal("invalid binding published candidate")
			}
		})
	}
	if _, _, err := learning.BindProjections(c, p, candidates, input, output); err != nil {
		t.Fatal("invalid calls affected valid binding", err)
	}
}
