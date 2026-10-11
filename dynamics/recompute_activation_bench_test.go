package dynamics_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

const (
	recomputeActivationBenchmarkNodes   = 2048
	recomputeActivationBenchmarkSteps   = 64
	recomputeActivationBenchmarkSegment = 16
)

type recomputeActivationBenchmarkFixture struct {
	model    *dynamics.Continuous
	params   dynamics.Parameters
	initial  []float64
	inputs   [][]float64
	upstream [][]float64
}

func newRecomputeActivationBenchmarkFixture(activation string, delay, degree int) (*recomputeActivationBenchmarkFixture, error) {
	n := recomputeActivationBenchmarkNodes
	edges := n * degree
	sources := make([]int, edges)
	targets := make([]int, edges)
	delays := make([]int, edges)
	weights := make([]float64, edges)
	for target := 0; target < n; target++ {
		for offset := 0; offset < degree; offset++ {
			e := target*degree + offset
			sources[e] = (target*17 + offset*31 + 5) % n
			targets[e] = target
			delays[e] = delay
			weights[e] = 0.0005 * float64((e%11)-5)
		}
	}

	model, err := dynamics.NewContinuous(dynamics.Config{
		Nodes:      n,
		Sources:    sources,
		Targets:    targets,
		Delays:     delays,
		DT:         0.25,
		Activation: activation,
	})
	if err != nil {
		return nil, err
	}
	params := dynamics.Parameters{
		Weights: weights,
		Bias:    make([]float64, n),
		LogTau:  make([]float64, n),
	}
	initial := make([]float64, n)
	for node := 0; node < n; node++ {
		params.Bias[node] = 0.002 * float64((node%7)-3)
		params.LogTau[node] = 0.12 + 0.01*float64(node%5)
		initial[node] = 0.01 * float64((node%9)-4)
	}
	inputs := make([][]float64, recomputeActivationBenchmarkSteps)
	upstream := make([][]float64, recomputeActivationBenchmarkSteps)
	for step := 0; step < recomputeActivationBenchmarkSteps; step++ {
		inputs[step] = make([]float64, n)
		upstream[step] = make([]float64, n)
		for node := 0; node < n; node++ {
			inputs[step][node] = 0.004 * float64((step+node)%13-6)
			upstream[step][node] = 0.003 * float64((2*step+3*node)%17-8)
		}
	}
	return &recomputeActivationBenchmarkFixture{
		model:    model,
		params:   params,
		initial:  initial,
		inputs:   inputs,
		upstream: upstream,
	}, nil
}

var recomputeActivationBenchmarkOutputs [][]float64
var recomputeActivationBenchmarkGradient dynamics.Gradient

// BenchmarkRecomputeActivation measures only the public recompute calls after
// constructing one fixed 2,048-node, 64-step fixture per subbenchmark. The
// degree-64 cases are the required high-fan-out measurements; degree 1 and 0
// are controls. All subbenchmarks use segment 16, and each activation is run
// at delay 0 and delay 3.
func BenchmarkRecomputeActivation(b *testing.B) {
	for _, activation := range []string{"tanh", "softplus"} {
		for _, delay := range []int{0, 3} {
			for _, degree := range []int{64, 1, 0} {
				fixture, err := newRecomputeActivationBenchmarkFixture(activation, delay, degree)
				if err != nil {
					b.Fatal(err)
				}
				name := fmt.Sprintf("%s-d%d-deg%d", activation, delay, degree)
				b.Run(name+"-forward", func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						outputs, err := fixture.model.ForwardRecompute(context.Background(), fixture.params, fixture.initial, fixture.inputs, recomputeActivationBenchmarkSegment)
						if err != nil {
							b.Fatal(err)
						}
						recomputeActivationBenchmarkOutputs = outputs
					}
				})
				b.Run(name+"-backward", func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						gradient, err := fixture.model.BackwardRecompute(context.Background(), fixture.params, fixture.initial, fixture.inputs, fixture.upstream, 0, recomputeActivationBenchmarkSegment)
						if err != nil {
							b.Fatal(err)
						}
						recomputeActivationBenchmarkGradient = gradient
					}
				})
			}
		}
	}
}
