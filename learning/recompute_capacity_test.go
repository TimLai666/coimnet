package learning_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	recomputeCapacityNodes = dynamics.MaxStateValues/3 + 1
	recomputeCapacitySteps = 4
)

// recomputeCapacityConfig is intentionally just over the three-row state
// boundary. The two delayed edges are enough to require a delay-2 prefix, but
// the episode itself remains small enough for the full-history forward path.
func recomputeCapacityConfig(lif bool) learning.Config {
	c := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      recomputeCapacityNodes,
			Sources:    []int{0, 1, 1},
			Targets:    []int{0, 1, 1},
			Delays:     []int{0, 2, 2},
			DT:         0.5,
			Activation: "tanh",
		},
		InputSize:        1,
		InputNodes:       []int{0, 1},
		ReadoutNodes:     []int{1},
		OutputSize:       1,
		ReadoutEveryStep: true,
	}
	if lif {
		c.Dynamics = dynamics.Config{}
		c.LIF = &dynamics.LIFConfig{
			Nodes:           recomputeCapacityNodes,
			Sources:         []int{0, 1, 1},
			Targets:         []int{0, 1, 1},
			Delays:          []int{0, 2, 2},
			DT:              0.5,
			TauSyn:          0.7,
			ThetaMin:        0.2,
			ThetaMax:        1.2,
			VReset:          -0.5,
			RefractorySteps: 1,
			Adaptation:      dynamics.LIFAdaptation{Enabled: true, TauAdapt: 1.1, Beta: 0.4},
			Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
		}
	}
	return c
}

func recomputeCapacityParameters(lif bool) learning.Parameters {
	bias := make([]float64, recomputeCapacityNodes)
	logTau := make([]float64, recomputeCapacityNodes)
	bias[0], bias[1] = 0.05, 0.1
	logTau[0], logTau[1] = 0.1, -0.1
	p := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.2, 0.35, -0.1},
			Bias:    bias,
			LogTau:  logTau,
		},
		Encoder: []float64{2.2, 2.0},
		Readout: []float64{0.7},
	}
	if lif {
		p.ThetaRaw = make([]float64, recomputeCapacityNodes)
		p.Core.Bias[1] = 0.15
		p.Encoder = []float64{2.8, 2.5}
	}
	return p
}

func recomputeCapacityInputs(lif bool) [][]float64 {
	values := []float64{0.2, 0.3, 0.1, 0.4}
	if lif {
		values = []float64{1.2, 1.1, 1.3, 1.0}
	}
	inputs := make([][]float64, recomputeCapacitySteps)
	for i, value := range values {
		inputs[i] = []float64{value}
	}
	return inputs
}

func recomputeCapacityTarget() []float64 { return []float64{0.5} }

func recomputeCapacityUpstream() [][]float64 {
	return [][]float64{{0.1}, {0.2}, {0.3}, {0.4}}
}

func recomputeCapacityOptions(lif bool) learning.Options {
	o := learning.DefaultOptions()
	if lif {
		o.Trainable.Theta = true
	}
	return o
}

func newRecomputeCapacityTrainer(t *testing.T, lif bool, o learning.Options) *learning.Trainer {
	t.Helper()
	tr, err := learning.NewTrainer(recomputeCapacityConfig(lif), recomputeCapacityParameters(lif), o)
	if err != nil {
		t.Fatalf("NewTrainer lif=%t: %v", lif, err)
	}
	return tr
}

// assertRecomputeCapacitySnapshot compares the public training state while
// deliberately clearing only the option that is supposed to differ.
func assertRecomputeCapacitySnapshot(t *testing.T, label string, got, want learning.TrainingSnapshot) {
	t.Helper()
	gotOptions, wantOptions := got.Options, want.Options
	gotOptions.Recompute, wantOptions.Recompute = nil, nil
	if !reflect.DeepEqual(got.Config, want.Config) || !reflect.DeepEqual(gotOptions, wantOptions) {
		t.Fatalf("%s configuration/options differ", label)
	}
	assertRecomputeBits(t, label+" weights", got.Parameters.Core.Weights, want.Parameters.Core.Weights)
	assertRecomputeBits(t, label+" bias", got.Parameters.Core.Bias, want.Parameters.Core.Bias)
	assertRecomputeBits(t, label+" log_tau", got.Parameters.Core.LogTau, want.Parameters.Core.LogTau)
	assertRecomputeBits(t, label+" theta_raw", got.Parameters.ThetaRaw, want.Parameters.ThetaRaw)
	assertRecomputeBits(t, label+" encoder", got.Parameters.Encoder, want.Parameters.Encoder)
	assertRecomputeBits(t, label+" readout", got.Parameters.Readout, want.Parameters.Readout)
	assertRecomputeBits(t, label+" optimizer first", got.Optimizer.First, want.Optimizer.First)
	assertRecomputeBits(t, label+" optimizer second", got.Optimizer.Second, want.Optimizer.Second)
	if !reflect.DeepEqual(got.Optimizer.Steps, want.Optimizer.Steps) {
		t.Fatalf("%s optimizer steps differ", label)
	}
	if got.Updates != want.Updates || !reflect.DeepEqual(got.Accumulator, want.Accumulator) {
		t.Fatalf("%s update state got updates=%d accumulator=%+v want updates=%d accumulator=%+v", label, got.Updates, got.Accumulator, want.Updates, want.Accumulator)
	}
}

func TestRecomputeCapacityTrainerMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	for _, lif := range []bool{false, true} {
		name := "continuous"
		if lif {
			name = "lif"
		}
		for _, segment := range []int{1, 2} {
			t.Run(name+"/segment="+strconv.Itoa(segment), func(t *testing.T) {
				fullOptions := recomputeCapacityOptions(lif)
				recomputeOptions := fullOptions
				recomputeOptions.Recompute = &learning.Recompute{SegmentSteps: segment}
				full := newRecomputeCapacityTrainer(t, lif, fullOptions)
				recompute := newRecomputeCapacityTrainer(t, lif, recomputeOptions)
				input := recomputeCapacityInputs(lif)
				target := recomputeCapacityTarget()
				upstream := recomputeCapacityUpstream()

				want, err := full.Step(ctx, input, target)
				if err != nil {
					t.Fatalf("full Step: %v", err)
				}
				got, err := recompute.Step(ctx, input, target)
				if err != nil {
					t.Fatalf("recompute Step: %v", err)
				}
				assertRecomputeResult(t, "Step", got, want)

				wantFrom, err := full.StepFrom(ctx, input, upstream)
				if err != nil {
					t.Fatalf("full StepFrom: %v", err)
				}
				gotFrom, err := recompute.StepFrom(ctx, input, upstream)
				if err != nil {
					t.Fatalf("recompute StepFrom: %v", err)
				}
				assertRecomputeResult(t, "StepFrom", gotFrom, wantFrom)

				assertRecomputeCapacitySnapshot(t, "final", recompute.Snapshot(), full.Snapshot())
			})
		}
	}
}

func recomputeMaxIntDelayConfig(lif bool) learning.Config {
	maxDelay := int(^uint(0) >> 1)
	c := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      2,
			Sources:    []int{0},
			Targets:    []int{0},
			Delays:     []int{maxDelay},
			DT:         0.5,
			Activation: "tanh",
		},
		InputSize:        1,
		InputNodes:       []int{0},
		ReadoutNodes:     []int{0},
		OutputSize:       1,
		ReadoutEveryStep: true,
	}
	if lif {
		c.Dynamics = dynamics.Config{}
		c.LIF = &dynamics.LIFConfig{
			Nodes: 2, Sources: []int{0}, Targets: []int{0}, Delays: []int{maxDelay},
			DT: 0.5, TauSyn: 0.7, ThetaMin: 0.2, ThetaMax: 1.2, VReset: -0.5,
			RefractorySteps: 1,
			Surrogate:       dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 1.7},
		}
	}
	return c
}

func recomputeMaxIntDelayParameters(lif bool) learning.Parameters {
	p := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{0.2},
			Bias:    []float64{0.1, 0},
			LogTau:  []float64{0, 0},
		},
		Encoder: []float64{1.2},
		Readout: []float64{0.7},
	}
	if lif {
		p.ThetaRaw = []float64{0, 0}
		p.Encoder[0] = 2.5
	}
	return p
}

func TestRecomputeMaxIntDelayTrainerMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	for _, lif := range []bool{false, true} {
		name := "continuous"
		input := [][]float64{{0.2}, {0.3}, {0.4}}
		if lif {
			name = "lif"
			input = [][]float64{{1.2}, {1.1}, {1.3}}
		}
		t.Run(name, func(t *testing.T) {
			fullOptions := recomputeCapacityOptions(lif)
			recomputeOptions := fullOptions
			recomputeOptions.Recompute = &learning.Recompute{SegmentSteps: 1}
			config := recomputeMaxIntDelayConfig(lif)
			parameters := recomputeMaxIntDelayParameters(lif)
			full, err := learning.NewTrainer(config, parameters, fullOptions)
			if err != nil {
				t.Fatalf("full NewTrainer: %v", err)
			}
			recompute, err := learning.NewTrainer(config, parameters, recomputeOptions)
			if err != nil {
				t.Fatalf("recompute NewTrainer: %v", err)
			}
			want, err := full.Step(ctx, input, []float64{0.5})
			if err != nil {
				t.Fatalf("full Step: %v", err)
			}
			got, err := recompute.Step(ctx, input, []float64{0.5})
			if err != nil {
				t.Fatalf("recompute Step: %v", err)
			}
			assertRecomputeResult(t, "Step", got, want)
			upstream := [][]float64{{0.1}, {0.2}, {0.3}}
			wantFrom, err := full.StepFrom(ctx, input, upstream)
			if err != nil {
				t.Fatalf("full StepFrom: %v", err)
			}
			gotFrom, err := recompute.StepFrom(ctx, input, upstream)
			if err != nil {
				t.Fatalf("recompute StepFrom: %v", err)
			}
			assertRecomputeResult(t, "StepFrom", gotFrom, wantFrom)
			assertRecomputeCapacitySnapshot(t, "final", recompute.Snapshot(), full.Snapshot())
		})
	}
}

// The persistent state API keeps its own history capacity guard. Recompute
// training may cross this boundary, but an Individual using the same graph
// must still reject the second one-row advance once delay-2 history needs a
// third row.
func TestRecomputeCapacityPersistentAdvanceStillRejects(t *testing.T) {
	ctx := context.Background()
	for _, lif := range []bool{false, true} {
		name := "continuous"
		if lif {
			name = "lif"
		}
		t.Run(name, func(t *testing.T) {
			individual, err := learning.NewIndividual(
				recomputeCapacityConfig(lif),
				recomputeCapacityParameters(lif),
				recomputeCapacityOptions(lif),
				make([]float64, recomputeCapacityNodes),
			)
			if err != nil {
				t.Fatalf("NewIndividual: %v", err)
			}
			if _, err := individual.Advance(ctx, [][]float64{{0.2}}); err != nil {
				t.Fatalf("first one-row Advance: %v", err)
			}
			if _, err := individual.Advance(ctx, [][]float64{{0.2}}); err == nil {
				t.Fatal("second one-row Advance unexpectedly succeeded beyond persistent delay history capacity")
			}
		})
	}
}
