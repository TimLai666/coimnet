// Package experiment contains reproducible, explicitly scoped reference tasks.
package experiment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/internal/delayedfixture"
	"github.com/TimLai666/coimnet/learning"
)

// Episode keeps observations separate from the trainer's target.
type Episode = delayedfixture.Episode

// DelayedConfig is the complete preregistered synthetic task protocol.
type DelayedConfig struct {
	Seeds        []uint64 `json:"seeds"`
	Updates      int      `json:"updates"`
	TrainSeed    uint64   `json:"train_seed"`
	TestSeed     uint64   `json:"test_seed"`
	TestCount    int      `json:"test_count"`
	LearningRate float64  `json:"learning_rate"`
	MaxMSE       float64  `json:"max_mse"`
	MaxLossRatio float64  `json:"max_loss_ratio"`
}

// DefaultDelayedConfig fixes the protocol before evaluation. This is a
// learnability check on a generated fixture, not a natural task accuracy claim.
func DefaultDelayedConfig() DelayedConfig {
	return DelayedConfig{[]uint64{7, 42, 123}, 600, 1001, 1003, 128, .02, .005, .1}
}

// DelayedRun reports every control and seed, including failed learning gates.
type DelayedRun struct {
	Seed                uint64  `json:"seed"`
	BeforeMSE           float64 `json:"before_mse"`
	AfterMSE            float64 `json:"after_mse"`
	FrozenMSE           float64 `json:"frozen_mse"`
	ShuffledMSE         float64 `json:"shuffled_mse"`
	CoreUpdateNorm      float64 `json:"core_update_norm"`
	PeripheryUpdateNorm float64 `json:"periphery_update_norm"`
	ParameterHash       string  `json:"parameter_hash"`
	Passed              bool    `json:"passed"`
}

// DelayedReport separates synthetic learning evidence from biological claims.
type DelayedReport struct {
	SchemaVersion  string        `json:"schema_version"`
	Profile        string        `json:"profile"`
	Generator      string        `json:"generator"`
	ShuffleMethod  string        `json:"shuffle_method"`
	ShuffleSeed    uint64        `json:"shuffle_seed"`
	Config         DelayedConfig `json:"config"`
	TopologyHash   string        `json:"topology_hash"`
	Runs           []DelayedRun  `json:"runs"`
	MeanAfterMSE   float64       `json:"mean_after_mse"`
	StddevAfterMSE float64       `json:"stddev_after_mse"`
	Passed         bool          `json:"passed"`
}

// DelayedEpisode uses a counter-based SplitMix64 generator. No mutable random
// stream exists: seed plus sample index determines all five input steps.
// The pulse occurs only at step zero; the target is 0.4 times that pulse.
func DelayedEpisode(seed, index uint64) Episode {
	return delayedfixture.DelayedEpisode(seed, index)
}

// NewDelayedTrainer fixes a three-neuron chain with one memory self-connection.
// Only core weights train. The fixed encoder injects into neuron 0; the fixed
// one-dimensional readout observes neuron 2 only, with no raw-input bypass.
func NewDelayedTrainer(seed uint64, rate float64, freeze bool) (*learning.Trainer, error) {
	return delayedfixture.NewDelayedTrainer(seed, rate, freeze)
}

// RunDelayed runs matched budgets for learned, frozen and shuffled-feedback
// conditions. Holdout observations and labels never enter Step. Canceled runs
// return an error; completed failed seeds remain in the report.
func RunDelayed(ctx context.Context, c DelayedConfig) (DelayedReport, error) {
	var report DelayedReport
	if ctx == nil {
		return report, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if len(c.Seeds) == 0 || c.Updates <= 0 || c.TestCount <= 0 || c.TrainSeed == c.TestSeed || !finite(c.LearningRate) || c.LearningRate <= 0 || !finite(c.MaxMSE) || c.MaxMSE <= 0 || !finite(c.MaxLossRatio) || c.MaxLossRatio <= 0 || c.MaxLossRatio >= 1 {
		return report, fmt.Errorf("invalid delayed-association protocol")
	}
	if c.Updates > 100000 || c.TestCount > 100000 || len(c.Seeds) > 100 {
		return report, fmt.Errorf("protocol exceeds fixture resource limits")
	}
	seen := map[uint64]bool{}
	for _, seed := range c.Seeds {
		if seen[seed] {
			return report, fmt.Errorf("duplicate seed %d", seed)
		}
		seen[seed] = true
	}
	c.Seeds = append([]uint64(nil), c.Seeds...)
	// Distinct seeds alone do not ensure distinct counter streams: a seed
	// shifted by the counter increment reproduces another split's samples.
	trainingCounters := make(map[uint64]bool, c.Updates)
	for i := 0; i < c.Updates; i++ {
		trainingCounters[c.TrainSeed+uint64(i)*0x9e3779b97f4a7c15] = true
	}
	for i := 0; i < c.TestCount; i++ {
		if trainingCounters[c.TestSeed+uint64(i)*0x9e3779b97f4a7c15] {
			return report, fmt.Errorf("training and holdout counter streams overlap")
		}
	}
	shuffleSeed := c.TrainSeed ^ 0xd1b54a32d192ed03
	indices := shuffledTrainingIndices(shuffleSeed, c.Updates)
	report = DelayedReport{SchemaVersion: "coimnet-delayed-report/v2", Profile: "fixture", Generator: "delayed-pulse-splitmix64-v1", ShuffleMethod: "training-label-fisher-yates-splitmix64-modulo-v1", ShuffleSeed: shuffleSeed, Config: c, Passed: true}
	for _, seed := range c.Seeds {
		tr, err := NewDelayedTrainer(seed, c.LearningRate, false)
		if err != nil {
			return report, err
		}
		frozen, err := NewDelayedTrainer(seed, c.LearningRate, true)
		if err != nil {
			return report, err
		}
		shuffled, err := NewDelayedTrainer(seed, c.LearningRate, false)
		if err != nil {
			return report, err
		}
		initial := tr.Snapshot()
		report.TopologyHash = hash(initial.Config.Dynamics)
		// The initial snapshot is retained, then evaluated only after training.
		for i := 0; i < c.Updates; i++ {
			ep := DelayedEpisode(c.TrainSeed, uint64(i))
			wrong := DelayedEpisode(c.TrainSeed, uint64(indices[i]))
			if _, err = tr.Step(ctx, ep.Input, ep.Target); err != nil {
				return report, err
			}
			if _, err = frozen.Step(ctx, ep.Input, ep.Target); err != nil {
				return report, err
			}
			if _, err = shuffled.Step(ctx, ep.Input, wrong.Target); err != nil {
				return report, err
			}
		}
		baseline, err := learning.RestoreTrainer(initial)
		if err != nil {
			return report, err
		}
		values := make([]float64, 4)
		for j, model := range []*learning.Trainer{baseline, tr, frozen, shuffled} {
			values[j], err = EvaluateDelayed(ctx, model, c.TestSeed, c.TestCount)
			if err != nil {
				return report, err
			}
		}
		final := tr.Snapshot()
		r := DelayedRun{Seed: seed, BeforeMSE: values[0], AfterMSE: values[1], FrozenMSE: values[2], ShuffledMSE: values[3], CoreUpdateNorm: distance(initial.Parameters.Core.Weights, final.Parameters.Core.Weights), PeripheryUpdateNorm: math.Hypot(distance(initial.Parameters.Encoder, final.Parameters.Encoder), distance(initial.Parameters.Readout, final.Parameters.Readout)), ParameterHash: hash(final.Parameters)}
		r.Passed = r.AfterMSE <= c.MaxMSE && r.AfterMSE <= r.BeforeMSE*c.MaxLossRatio && r.AfterMSE < r.ShuffledMSE && r.FrozenMSE == r.BeforeMSE && r.CoreUpdateNorm > 0 && r.PeripheryUpdateNorm == 0
		report.Runs = append(report.Runs, r)
		report.Passed = report.Passed && r.Passed
		report.MeanAfterMSE += r.AfterMSE / float64(len(c.Seeds))
	}
	for _, r := range report.Runs {
		d := r.AfterMSE - report.MeanAfterMSE
		report.StddevAfterMSE += d * d / float64(len(report.Runs))
	}
	report.StddevAfterMSE = math.Sqrt(report.StddevAfterMSE)
	return report, nil
}

// EvaluateDelayed uses a separate split and never supplies feedback for updates.
func EvaluateDelayed(ctx context.Context, tr *learning.Trainer, seed uint64, count int) (float64, error) {
	if ctx == nil || tr == nil {
		return 0, fmt.Errorf("context and trainer are required")
	}
	if count <= 0 || count > 100000 {
		return 0, fmt.Errorf("test count must be between 1 and 100000")
	}
	var mse float64
	for i := 0; i < count; i++ {
		ep := DelayedEpisode(seed, uint64(i))
		pred, err := tr.Predict(ctx, ep.Input)
		if err != nil {
			return 0, err
		}
		d := pred[0] - ep.Target[0]
		mse += d * d / float64(count)
	}
	return mse, nil
}

// Permute only the training labels, retaining their exact empirical distribution.
// The fixed modulo rule is versioned in the report for cross-platform replay.
func shuffledTrainingIndices(seed uint64, count int) []int {
	indices := make([]int, count)
	for i := range indices {
		indices[i] = i
	}
	for i, counter := count-1, uint64(0); i > 0; i, counter = i-1, counter+1 {
		j := int(mix(seed+counter*0x9e3779b97f4a7c15) % uint64(i+1))
		indices[i], indices[j] = indices[j], indices[i]
	}
	return indices
}
func mix(x uint64) uint64 {
	return delayedfixture.Mix(x)
}
func hash(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func distance(a, b []float64) float64 {
	var d float64
	for i, v := range a {
		d = math.Hypot(d, v-b[i])
	}
	return d
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
