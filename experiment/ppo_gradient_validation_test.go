package experiment

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

func TestPPOGradientHandReference(t *testing.T) {
	c := shortGoalConfig().PPO
	u, err := ppoGradientUpstreams([][]float64{{0, 0, .3}}, []rl.Transition{{Action: 0, LogProb: -math.Log(2)}}, []float64{2}, []float64{.7}, c)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]float64{"policy": {-1, 1, 0}, "value": {0, 0, -.4}, "entropy": {0, 0, 0}, "total": {-1, 1, -.4}}
	for name, row := range want {
		for j, x := range row {
			if math.Abs(u[name][0][j]-x) > 1e-12 {
				t.Fatalf("%s[%d]=%v, want %v", name, j, u[name][0][j], x)
			}
		}
	}
}

func TestPPOGradientClippingEntropyAndBurnIn(t *testing.T) {
	c := shortGoalConfig().PPO
	out := [][]float64{{math.Log(3), 0, .3}, {math.Log(3), 0, .3}}
	steps := []rl.Transition{{Action: 0, LogProb: math.Log(.25)}, {Action: 0, LogProb: math.Log(.25)}}
	c.BurnIn = 1
	u, err := ppoGradientUpstreams(out, steps, []float64{2, 2}, []float64{.7, .7}, c)
	if err != nil {
		t.Fatal(err)
	}
	for name := range u {
		for _, v := range u[name][0] {
			if v != 0 {
				t.Fatal("burn-in has direct upstream")
			}
		}
	}
	for _, v := range u["policy"][1] {
		if v != 0 {
			t.Fatal("clipped positive advantage has policy gradient")
		}
	}
	// p=(.75,.25); d(-coefficient*H)/dz0 = coefficient*.75*.25*log(3).
	want := c.EntropyCoef * .75 * .25 * math.Log(3)
	if math.Abs(u["entropy"][1][0]-want) > 1e-12 || math.Abs(u["entropy"][1][1]+want) > 1e-12 {
		t.Fatal("entropy derivative differs from hand reference")
	}
}

func TestPPOGradientReplayAndFiniteDifference(t *testing.T) {
	ctx := context.Background()
	c := shortGoalConfig()
	ind, err := newPPOIndividual(1, c.Hidden, c.LearningRate)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := collectPPOEpisode(ctx, ind, c.Corridor, 1001, rand.New(rand.NewPCG(1, 0x1001)))
	if err != nil {
		t.Fatal(err)
	}
	original := hash(ind.Snapshot())
	rolloutHash := hash(r)
	split, updated, _, err := diagnosePPOGradient(ctx, ind, r, c.PPO)
	if err != nil {
		t.Fatal(err)
	}
	if hash(ind.Snapshot()) != original || hash(r) != rolloutHash || hash(updated.Snapshot()) == original {
		t.Fatal("caller changed or update missing")
	}
	network, err := learning.NewNetwork(ind.Snapshot().Config)
	if err != nil {
		t.Fatal(err)
	}
	input := make([][]float64, len(r.Steps))
	for i, s := range r.Steps {
		input[i] = s.Obs
	}
	// Verify each parameter group against a scalar inner product with a frozen
	// upstream. The finite difference never recalculates collector targets.
	for _, component := range []string{"policy", "value", "entropy", "total"} {
		g := ppoGradientGroups(split.Gradients[component])
		for _, group := range []string{"weights", "bias", "log_tau", "encoder", "readout"} {
			t.Run(component+"/"+group, func(t *testing.T) {
				p := ind.Snapshot().Parameters
				var values []float64
				switch group {
				case "weights":
					values = p.Core.Weights
				case "bias":
					values = p.Core.Bias
				case "log_tau":
					values = p.Core.LogTau
				case "encoder":
					values = p.Encoder
				case "readout":
					values = p.Readout
				}
				var derivative float64
				for i, x := range g[group] {
					derivative += x * math.Sin(float64(i+1))
				}
				scalar := func(sign float64) float64 {
					for i := range values {
						values[i] += sign * .002 * math.Sin(float64(i+1))
					}
					out, err := network.PredictAll(ctx, p, input)
					if err != nil {
						t.Fatal(err)
					}
					for i := range values {
						values[i] -= sign * .002 * math.Sin(float64(i+1))
					}
					var v float64
					for i, row := range out {
						for j, x := range row {
							v += x * split.Upstreams[component][i][j]
						}
					}
					return v
				}
				fd := (scalar(1) - scalar(-1)) / .004
				t.Logf("finite difference %.9g; raw derivative %.9g", fd, derivative)
				if math.Abs(fd-derivative) > 1e-4+.01*math.Max(math.Abs(fd), math.Abs(derivative)) {
					t.Fatalf("finite difference %g, gradient %g", fd, derivative)
				}
			})
		}
	}
	if hash(ind.Snapshot()) != original {
		t.Fatal("finite difference changed caller")
	}
	// The optimizer updates only these three groups, even though the complete
	// derivative also contains frozen time constants and encoder parameters.
	for _, row := range [][]float64{split.ParameterDelta.Core.LogTau, split.ParameterDelta.Encoder} {
		for _, x := range row {
			if x != 0 {
				t.Fatal("frozen parameter changed")
			}
		}
	}
	altered := r
	altered.PolicyVersion = "stale"
	if _, _, _, err := diagnosePPOGradient(ctx, ind, altered, c.PPO); err == nil {
		t.Fatal("accepted stale rollout")
	}
	altered = r
	altered.InitialNeural = updated.Snapshot().Neural
	at, err := learning.RestoreIndividual(updated.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := at.Advance(ctx, [][]float64{r.Steps[0].Obs}); err != nil {
		t.Fatal(err)
	}
	altered.InitialNeural = at.Snapshot().Neural
	if _, _, _, err := diagnosePPOGradient(ctx, ind, altered, c.PPO); err == nil {
		t.Fatal("accepted nonzero initial state")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, _, err := diagnosePPOGradient(canceled, ind, r, c.PPO); err == nil {
		t.Fatal("accepted canceled context")
	}
	if hash(ind.Snapshot()) != original {
		t.Fatal("invalid input mutated caller")
	}
}

func TestPPOGradientSumRejectsCorruption(t *testing.T) {
	zero := learning.Gradient{}
	g := map[string]learning.Gradient{"policy": zero, "value": zero, "entropy": zero, "total": zero}
	if err := checkPPOGradientSum(g); err != nil {
		t.Fatal(err)
	}
	g["total"] = learning.Gradient{Readout: []float64{1}}
	if err := checkPPOGradientSum(g); err == nil {
		t.Fatal("accepted wrong shape")
	}
	for _, name := range []string{"policy", "value", "entropy", "total"} {
		g[name] = learning.Gradient{Readout: []float64{0}}
	}
	g["total"] = learning.Gradient{Readout: []float64{1}}
	if err := checkPPOGradientSum(g); err == nil {
		t.Fatal("accepted nonzero total of zeros")
	}
	g["total"] = learning.Gradient{Readout: []float64{math.NaN()}}
	if err := checkPPOGradientSum(g); err == nil {
		t.Fatal("accepted NaN")
	}
	delete(g, "policy")
	if err := checkPPOGradientSum(g); err == nil {
		t.Fatal("accepted missing component")
	}
}

func TestPPOGradientRejectsInvalid(t *testing.T) {
	c := shortGoalConfig().PPO
	tests := []struct {
		name string
		out  [][]float64
		a, b []float64
		step rl.Transition
	}{
		{"empty", nil, nil, nil, rl.Transition{}},
		{"width", [][]float64{{0}}, []float64{1}, []float64{1}, rl.Transition{}},
		{"nan", [][]float64{{math.NaN(), 0, 0}}, []float64{1}, []float64{1}, rl.Transition{}},
		{"inf target", [][]float64{{0, 0, 0}}, []float64{1}, []float64{math.Inf(1)}, rl.Transition{}},
		{"action", [][]float64{{0, 0, 0}}, []float64{1}, []float64{1}, rl.Transition{Action: 2}},
		{"positive old logp", [][]float64{{0, 0, 0}}, []float64{1}, []float64{1}, rl.Transition{LogProb: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ppoGradientUpstreams(tc.out, []rl.Transition{tc.step}, tc.a, tc.b, c); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}
