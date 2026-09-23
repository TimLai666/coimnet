package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// recomputeTrainerSteps is the episode length of every recompute trainer
// fixture.
const recomputeTrainerSteps = 23

// recomputeTrainerGraph is the 5 node, 8 edge graph of the internal recompute
// tests. Edge 3 carries a delay of 2, so restarting a segment needs more than
// one history row.
func recomputeTrainerGraph() (sources, targets, delays []int) {
	return []int{0, 1, 2, 3, 4, 1, 3, 0},
		[]int{1, 2, 3, 4, 0, 3, 1, 4},
		[]int{1, 0, 1, 2, 0, 1, 0, 1}
}

func recomputeTrainerContinuousConfig() learning.Config {
	s, t, d := recomputeTrainerGraph()
	return learning.Config{
		Dynamics:  dynamics.Config{Nodes: 5, Sources: s, Targets: t, Delays: d, DT: .5, Activation: "tanh"},
		InputSize: 2, InputNodes: []int{0, 1}, ReadoutNodes: []int{3, 4}, OutputSize: 2, ReadoutEveryStep: true,
	}
}

func recomputeTrainerLIFConfig() learning.Config {
	s, t, d := recomputeTrainerGraph()
	lif := dynamics.LIFConfig{
		Nodes: 5, Sources: s, Targets: t, Delays: d,
		DT: .5, TauSyn: .7, ThetaMin: .3, ThetaMax: 1.4, VReset: -.5, RefractorySteps: 1,
		Adaptation: dynamics.LIFAdaptation{Enabled: true, TauAdapt: .9, Beta: .4},
		Surrogate:  dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
	}
	return learning.Config{LIF: &lif, InputSize: 2, InputNodes: []int{0, 1}, ReadoutNodes: []int{3, 4}, OutputSize: 2, ReadoutEveryStep: true}
}

// recomputeTrainerRows draws a rows x width matrix uniformly from [lo, hi)
// with the fixed generator PCG(seed, 0).
func recomputeTrainerRows(seed uint64, rows, width int, lo, hi float64) [][]float64 {
	r := rand.New(rand.NewPCG(seed, 0))
	out := make([][]float64, rows)
	for i := range out {
		out[i] = make([]float64, width)
		for j := range out[i] {
			out[i][j] = lo + (hi-lo)*r.Float64()
		}
	}
	return out
}

func recomputeTrainerVector(seed uint64, n int, lo, hi float64) []float64 {
	return recomputeTrainerRows(seed, 1, n, lo, hi)[0]
}

type recomputeTrainerFixture struct {
	name     string
	config   learning.Config
	params   learning.Parameters
	options  learning.Options
	input    [][]float64
	target   []float64
	upstream [][]float64
}

// recomputeTrainerFixtures returns the continuous and the LIF fixture. The LIF
// input is drawn from [0, 3) and its encoder from [0, 1), so the input nodes
// cross their thresholds, and its threshold group is trainable.
func recomputeTrainerFixtures() []recomputeTrainerFixture {
	lifOptions := learning.DefaultOptions()
	lifOptions.Trainable.Theta = true
	return []recomputeTrainerFixture{
		{
			name:   "continuous",
			config: recomputeTrainerContinuousConfig(),
			params: learning.Parameters{
				Core: dynamics.Parameters{
					Weights: recomputeTrainerVector(11, 8, -1, 1),
					Bias:    recomputeTrainerVector(12, 5, -.2, .2),
					LogTau:  recomputeTrainerVector(13, 5, -.3, .3),
				},
				Encoder: recomputeTrainerVector(14, 4, -1, 1),
				Readout: recomputeTrainerVector(15, 4, -1, 1),
			},
			options:  learning.DefaultOptions(),
			input:    recomputeTrainerRows(16, recomputeTrainerSteps, 2, -1, 1),
			target:   recomputeTrainerVector(17, 2, -1, 1),
			upstream: recomputeTrainerRows(18, recomputeTrainerSteps, 2, -1, 1),
		},
		{
			name:   "lif",
			config: recomputeTrainerLIFConfig(),
			params: learning.Parameters{
				Core: dynamics.Parameters{
					Weights: recomputeTrainerVector(21, 8, -1, 1),
					Bias:    recomputeTrainerVector(22, 5, -.2, .2),
					LogTau:  recomputeTrainerVector(23, 5, -.3, .3),
				},
				ThetaRaw: recomputeTrainerVector(24, 5, -.5, .5),
				Encoder:  recomputeTrainerVector(25, 4, 0, 1),
				Readout:  recomputeTrainerVector(26, 4, -1, 1),
			},
			options:  lifOptions,
			input:    recomputeTrainerRows(27, recomputeTrainerSteps, 2, 0, 3),
			target:   recomputeTrainerVector(28, 2, -1, 1),
			upstream: recomputeTrainerRows(29, recomputeTrainerSteps, 2, -1, 1),
		},
	}
}

func newRecomputeTrainer(t *testing.T, fx recomputeTrainerFixture, o learning.Options) *learning.Trainer {
	t.Helper()
	tr, err := learning.NewTrainer(fx.config, fx.params, o)
	if err != nil {
		t.Fatalf("%s: NewTrainer: %v", fx.name, err)
	}
	return tr
}

// assertRecomputeBits fails at the first value whose bits differ.
func assertRecomputeBits(t *testing.T, label string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d values, want %d", label, len(got), len(want))
	}
	for i := range want {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s[%d] is %g (bits %#x), want %g (bits %#x)", label, i, got[i], math.Float64bits(got[i]), want[i], math.Float64bits(want[i]))
		}
	}
}

func assertRecomputeRows(t *testing.T, label string, got, want [][]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d rows, want %d", label, len(got), len(want))
	}
	for r := range want {
		assertRecomputeBits(t, fmt.Sprintf("%s row %d", label, r), got[r], want[r])
	}
}

// assertRecomputeResult compares every field of two step results, floats by
// their bits, and then the whole structs with reflect.DeepEqual.
func assertRecomputeResult(t *testing.T, label string, got, want learning.StepResult) {
	t.Helper()
	gv, wv := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := range gv.NumField() {
		name := gv.Type().Field(i).Name
		g, w := gv.Field(i), wv.Field(i)
		if g.Kind() == reflect.Float64 {
			if math.Float64bits(g.Float()) != math.Float64bits(w.Float()) {
				t.Fatalf("%s: %s is %g (bits %#x), want %g (bits %#x)", label, name, g.Float(), math.Float64bits(g.Float()), w.Float(), math.Float64bits(w.Float()))
			}
			continue
		}
		if !reflect.DeepEqual(g.Interface(), w.Interface()) {
			t.Fatalf("%s: %s is %v, want %v", label, name, g.Interface(), w.Interface())
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: result %+v, want %+v", label, got, want)
	}
}

// TestRecomputeTrainerMatchesFullHistory trains a recompute trainer next to a
// full-history trainer through three Step and three StepFrom calls and requires
// every result, the final parameters and the optimizer state to agree bit for
// bit. A fixed-sign copy of the continuous fixture checks that the recomputed
// history integrates the effective weights, not the stored log magnitudes.
func TestRecomputeTrainerMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	fixtures := recomputeTrainerFixtures()
	signed := fixtures[0]
	signed.name = "continuous-fixed-signs"
	signed.config.EdgeSigns = []int8{1, -1, 0, 1, -1, 0, 1, -1}
	for _, fx := range append(fixtures, signed) {
		if fx.config.LIF != nil {
			spikes, err := newRecomputeTrainer(t, fx, fx.options).Spikes(ctx, fx.input)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, row := range spikes {
				for _, v := range row {
					if v != 0 {
						count++
					}
				}
			}
			if count == 0 {
				t.Fatalf("%s: the fixture never spikes, so it does not exercise the reset path", fx.name)
			}
		}
		for _, segment := range []int{1, 4, 7} {
			for _, truncation := range []int{0, 3} {
				t.Run(fmt.Sprintf("%s/segment=%d/truncation=%d", fx.name, segment, truncation), func(t *testing.T) {
					full := fx.options
					full.Truncation = truncation
					recompute := full
					recompute.Recompute = &learning.Recompute{SegmentSteps: segment}
					want, got := newRecomputeTrainer(t, fx, full), newRecomputeTrainer(t, fx, recompute)
					for i := range 3 {
						w, err := want.Step(ctx, fx.input, fx.target)
						if err != nil {
							t.Fatalf("full Step %d: %v", i, err)
						}
						g, err := got.Step(ctx, fx.input, fx.target)
						if err != nil {
							t.Fatalf("recompute Step %d: %v", i, err)
						}
						assertRecomputeResult(t, fmt.Sprintf("Step %d", i), g, w)
					}
					for i := range 3 {
						w, err := want.StepFrom(ctx, fx.input, fx.upstream)
						if err != nil {
							t.Fatalf("full StepFrom %d: %v", i, err)
						}
						g, err := got.StepFrom(ctx, fx.input, fx.upstream)
						if err != nil {
							t.Fatalf("recompute StepFrom %d: %v", i, err)
						}
						assertRecomputeResult(t, fmt.Sprintf("StepFrom %d", i), g, w)
					}
					ws, gs := want.Snapshot(), got.Snapshot()
					for _, group := range []struct {
						name      string
						got, want []float64
					}{
						{"weights", gs.Parameters.Core.Weights, ws.Parameters.Core.Weights},
						{"bias", gs.Parameters.Core.Bias, ws.Parameters.Core.Bias},
						{"log_tau", gs.Parameters.Core.LogTau, ws.Parameters.Core.LogTau},
						{"theta_raw", gs.Parameters.ThetaRaw, ws.Parameters.ThetaRaw},
						{"encoder", gs.Parameters.Encoder, ws.Parameters.Encoder},
						{"readout", gs.Parameters.Readout, ws.Parameters.Readout},
						{"optimizer first", gs.Optimizer.First, ws.Optimizer.First},
						{"optimizer second", gs.Optimizer.Second, ws.Optimizer.Second},
					} {
						assertRecomputeBits(t, group.name, group.got, group.want)
					}
					if len(gs.Optimizer.Steps) != len(ws.Optimizer.Steps) {
						t.Fatalf("optimizer steps: %d values, want %d", len(gs.Optimizer.Steps), len(ws.Optimizer.Steps))
					}
					for i := range ws.Optimizer.Steps {
						if gs.Optimizer.Steps[i] != ws.Optimizer.Steps[i] {
							t.Fatalf("optimizer steps[%d] is %d, want %d", i, gs.Optimizer.Steps[i], ws.Optimizer.Steps[i])
						}
					}
					if gs.Updates != ws.Updates || ws.Updates != 6 {
						t.Fatalf("updates: recompute %d, full %d, want 6", gs.Updates, ws.Updates)
					}
				})
			}
		}
	}
}

// TestRecomputeGradientHorizon reads how far back the gradient of a step
// reached: the truncation window when it is shorter than the episode, the
// whole episode otherwise, on applied and accumulate-only steps alike and
// whether or not the history is recomputed.
func TestRecomputeGradientHorizon(t *testing.T) {
	ctx := context.Background()
	for _, fx := range recomputeTrainerFixtures() {
		for _, tc := range []struct{ truncation, want int }{
			{0, recomputeTrainerSteps}, {3, 3}, {50, recomputeTrainerSteps},
		} {
			for _, recompute := range []*learning.Recompute{nil, {SegmentSteps: 4}} {
				for _, accumulate := range []int{0, 2} {
					name := fmt.Sprintf("%s/truncation=%d/recompute=%t/accumulate=%d", fx.name, tc.truncation, recompute != nil, accumulate)
					t.Run(name, func(t *testing.T) {
						o := fx.options
						o.Truncation, o.Recompute, o.AccumulateSteps = tc.truncation, recompute, accumulate
						tr := newRecomputeTrainer(t, fx, o)
						step, err := tr.Step(ctx, fx.input, fx.target)
						if err != nil {
							t.Fatalf("Step: %v", err)
						}
						from, err := tr.StepFrom(ctx, fx.input, fx.upstream)
						if err != nil {
							t.Fatalf("StepFrom: %v", err)
						}
						for _, r := range []struct {
							label   string
							result  learning.StepResult
							applied bool
						}{
							{"Step", step, accumulate == 0}, {"StepFrom", from, true},
						} {
							if r.result.Applied != r.applied {
								t.Fatalf("%s applied %t, want %t", r.label, r.result.Applied, r.applied)
							}
							if r.result.GradientHorizonSteps != tc.want {
								t.Fatalf("%s gradient horizon %d steps, want %d", r.label, r.result.GradientHorizonSteps, tc.want)
							}
						}
					})
				}
			}
		}
	}
}

func TestRecomputeOptionsValidation(t *testing.T) {
	fx := recomputeTrainerFixtures()[0]

	t.Run("segment below one", func(t *testing.T) {
		for _, segment := range []int{0, -1} {
			o := fx.options
			o.Recompute = &learning.Recompute{SegmentSteps: segment}
			if _, err := learning.NewTrainer(fx.config, fx.params, o); err == nil || !strings.Contains(err.Error(), "segment_steps") {
				t.Fatalf("segment %d: NewTrainer error %v, want a segment_steps error", segment, err)
			}
		}
	})

	t.Run("mixed core", func(t *testing.T) {
		if _, err := learning.NewTrainer(mixedNodeConfig(), mixedNodeParameters(), learning.DefaultOptions()); err != nil {
			t.Fatalf("the mixed fixture without recompute: %v", err)
		}
		o := learning.DefaultOptions()
		o.Recompute = &learning.Recompute{SegmentSteps: 2}
		_, err := learning.NewTrainer(mixedNodeConfig(), mixedNodeParameters(), o)
		if err == nil || !strings.Contains(err.Error(), "recompute supports the continuous and LIF cores") {
			t.Fatalf("NewTrainer error %v, want the recompute support error", err)
		}
	})

	t.Run("restore", func(t *testing.T) {
		snapshot := newRecomputeTrainer(t, fx, fx.options).Snapshot()
		snapshot.Options.Recompute = &learning.Recompute{SegmentSteps: 0}
		if _, err := learning.RestoreTrainer(snapshot); err == nil || !strings.Contains(err.Error(), "segment_steps") {
			t.Fatalf("RestoreTrainer error %v, want a segment_steps error", err)
		}
		snapshot.Options.Recompute = &learning.Recompute{SegmentSteps: 5}
		restored, err := learning.RestoreTrainer(snapshot)
		if err != nil {
			t.Fatalf("RestoreTrainer with segment 5: %v", err)
		}
		if got := restored.Snapshot().Options.Recompute; got == nil || got.SegmentSteps != 5 {
			t.Fatalf("restored recompute %+v, want segment 5", got)
		}
	})

	t.Run("default options json", func(t *testing.T) {
		data, err := json.Marshal(learning.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "recompute") {
			t.Fatalf("DefaultOptions JSON mentions recompute: %s", data)
		}
	})

	t.Run("snapshot copies", func(t *testing.T) {
		o := fx.options
		o.Recompute = &learning.Recompute{SegmentSteps: 3}
		tr := newRecomputeTrainer(t, fx, o)
		first := tr.Snapshot().Options.Recompute
		if first == nil || first == o.Recompute || *first != *o.Recompute {
			t.Fatalf("snapshot recompute %p %+v, want a copy of %p %+v", first, first, o.Recompute, o.Recompute)
		}
		o.Recompute.SegmentSteps = 9
		first.SegmentSteps = 8
		if again := tr.Snapshot().Options.Recompute; again == first || again.SegmentSteps != 3 {
			t.Fatalf("second snapshot recompute %p %+v, want an independent segment 3", again, again)
		}
	})
}

// TestRecomputePredictUnchanged runs the frozen episodes of a recompute
// trainer, which never recompute, against a full-history trainer.
func TestRecomputePredictUnchanged(t *testing.T) {
	ctx := context.Background()
	for _, fx := range recomputeTrainerFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			o := fx.options
			o.Recompute = &learning.Recompute{SegmentSteps: 4}
			full, recompute := newRecomputeTrainer(t, fx, fx.options), newRecomputeTrainer(t, fx, o)
			want, err := full.PredictAll(ctx, fx.input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := recompute.PredictAll(ctx, fx.input)
			if err != nil {
				t.Fatal(err)
			}
			assertRecomputeRows(t, "PredictAll", got, want)
			last, err := recompute.Predict(ctx, fx.input)
			if err != nil {
				t.Fatal(err)
			}
			assertRecomputeBits(t, "Predict", last, want[len(want)-1])
			if fx.config.LIF == nil {
				return
			}
			wantSpikes, err := full.Spikes(ctx, fx.input)
			if err != nil {
				t.Fatal(err)
			}
			gotSpikes, err := recompute.Spikes(ctx, fx.input)
			if err != nil {
				t.Fatal(err)
			}
			assertRecomputeRows(t, "Spikes", gotSpikes, wantSpikes)
		})
	}
}
