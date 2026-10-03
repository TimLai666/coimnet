package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	realnavMemoryInputSize  = 6
	realnavMemoryNodeCount  = 8
	realnavMemoryOutputSize = 2
	realnavMemoryBlockSize  = 16
	realnavMemoryDT         = 1.0
	realnavMemoryActivation = "tanh"
	realnavMemoryEdgeScale  = 0.02
	realnavMemoryInputScale = 0.2
)

// metric is the scored-row, total-weighted mean squared error of one split.
type metric struct {
	Samples int     `json:"samples"`
	MSE     float64 `json:"mse"`
}

// newModel builds the fixed eight-node continuous model for one seed. All
// eight nodes receive the dense six-feature encoder and all eight contribute to
// the two-column readout; the core contains every directed pair, including
// self-edges, with zero delays.
func newModel(seed int64) (learning.Config, learning.Parameters, error) {
	sources := make([]int, 0, realnavMemoryNodeCount*realnavMemoryNodeCount)
	targets := make([]int, 0, cap(sources))
	delays := make([]int, 0, cap(sources))
	for source := 0; source < realnavMemoryNodeCount; source++ {
		for target := 0; target < realnavMemoryNodeCount; target++ {
			sources = append(sources, source)
			targets = append(targets, target)
			delays = append(delays, 0)
		}
	}
	readoutNodes := make([]int, realnavMemoryNodeCount)
	for i := range readoutNodes {
		readoutNodes[i] = i
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      realnavMemoryNodeCount,
			Sources:    sources,
			Targets:    targets,
			Delays:     delays,
			DT:         realnavMemoryDT,
			Activation: realnavMemoryActivation,
		},
		InputSize:        realnavMemoryInputSize,
		OutputSize:       realnavMemoryOutputSize,
		ReadoutNodes:     readoutNodes,
		ReadoutEveryStep: true,
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniformParameters(seed, 0, len(sources), realnavMemoryEdgeScale),
			Bias:    make([]float64, realnavMemoryNodeCount),
			LogTau:  make([]float64, realnavMemoryNodeCount),
		},
		Encoder: uniformParameters(seed, 1, realnavMemoryInputSize*realnavMemoryNodeCount, realnavMemoryInputScale),
		Readout: make([]float64, realnavMemoryNodeCount*realnavMemoryOutputSize),
	}
	for i, tau := range []float64{2, 4, 8, 16, 64, 256, 1024, 4096} {
		parameters.Core.LogTau[i] = math.Log(tau)
	}
	if _, err := learning.NewNetwork(config); err != nil {
		return learning.Config{}, learning.Parameters{}, fmt.Errorf("realnavmemory: build model: %w", err)
	}
	return config, parameters, nil
}

func uniformParameters(seed, stream int64, count int, scale float64) []float64 {
	rng := rand.New(rand.NewSource(seed + stream*7919))
	values := make([]float64, count)
	for i := range values {
		values[i] = (2*rng.Float64() - 1) * scale
	}
	return values
}

func trainingOptions() learning.Options {
	options := learning.DefaultOptions()
	options.LearningRate = 0.001
	options.ClipNorm = 1
	options.WeightDecay = 0
	options.Truncation = 0
	options.Recompute = nil
	options.Trainable = learning.Trainable{
		Encoder: true,
		Weights: true,
		Bias:    true,
		Tau:     true,
		Readout: true,
	}
	return options
}

// trialInputs returns fresh rows. The shuffled control delays each complete
// 16-row stimulus block by one block and permutes that past block with a seed
// derived only from the control seed and the current block index.
func trialInputs(trial historyTrial, control string, seed int64) ([][]float64, error) {
	if control != "delivered" && control != "no_stimulus" && control != "shuffled_stimulus" {
		return nil, fmt.Errorf("realnavmemory: unknown control %q", control)
	}
	if len(trial.Steps) == 0 {
		return nil, fmt.Errorf("realnavmemory: trial %q has no steps", trial.ID)
	}
	if len(trial.Steps) > historyMaxSteps {
		return nil, fmt.Errorf("realnavmemory: trial %q has %d steps, exceeds %d", trial.ID, len(trial.Steps), historyMaxSteps)
	}
	for i, step := range trial.Steps {
		for j, value := range step.Input {
			if !historyFinite(value) {
				return nil, fmt.Errorf("realnavmemory: trial %q input[%d][%d] is non-finite", trial.ID, i, j)
			}
		}
		if step.Input[5] != 0 && step.Input[5] != 1 {
			return nil, fmt.Errorf("realnavmemory: trial %q input[%d][5] stimulus %g is not 0 or 1", trial.ID, i, step.Input[5])
		}
		if !historyFinite(step.T) {
			return nil, fmt.Errorf("realnavmemory: trial %q step %d time is non-finite", trial.ID, i)
		}
	}
	inputs := make([][]float64, len(trial.Steps))
	for i, step := range trial.Steps {
		inputs[i] = make([]float64, realnavMemoryInputSize)
		copy(inputs[i], step.Input[:])
		if control == "no_stimulus" {
			inputs[i][5] = 0
		}
	}
	if control != "shuffled_stimulus" {
		return inputs, nil
	}

	blockCount := (len(trial.Steps) + realnavMemoryBlockSize - 1) / realnavMemoryBlockSize
	shuffledBlocks := make([][]float64, blockCount)
	for block := 1; block < blockCount; block++ {
		previousStart := (block - 1) * realnavMemoryBlockSize
		if previousStart+realnavMemoryBlockSize > len(trial.Steps) {
			continue
		}
		values := make([]float64, realnavMemoryBlockSize)
		for i := range values {
			values[i] = trial.Steps[previousStart+i].Input[5]
		}
		permuteStimulusBlock(values, seed+int64(block))
		shuffledBlocks[block] = values
	}
	for i := range inputs {
		block := i / realnavMemoryBlockSize
		if block == 0 || shuffledBlocks[block] == nil {
			inputs[i][5] = 0
			continue
		}
		inputs[i][5] = shuffledBlocks[block][i%realnavMemoryBlockSize]
	}
	return inputs, nil
}

func permuteStimulusBlock(values []float64, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := len(values) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		values[i], values[j] = values[j], values[i]
	}
}

// maskedMSE computes sum(error^2)/(2*scoredRows) and its exact output VJP.
// Unscored rows remain zero upstream and their targets are never inspected.
func maskedMSE(predicted [][]float64, trial historyTrial) (loss float64, upstream [][]float64, samples int, err error) {
	if len(predicted) == 0 || len(predicted) != len(trial.Steps) {
		return 0, nil, 0, fmt.Errorf("realnavmemory: prediction and trial row counts differ or are empty")
	}
	upstream = make([][]float64, len(predicted))
	for i, row := range predicted {
		if len(row) != realnavMemoryOutputSize {
			return 0, nil, 0, fmt.Errorf("realnavmemory: prediction row %d has width %d, want %d", i, len(row), realnavMemoryOutputSize)
		}
		for j, value := range row {
			if !historyFinite(value) {
				return 0, nil, 0, fmt.Errorf("realnavmemory: prediction[%d][%d] is non-finite", i, j)
			}
		}
		upstream[i] = make([]float64, realnavMemoryOutputSize)
		if trial.Steps[i].Scored {
			if !historyFinite(trial.Steps[i].Target[0]) || !historyFinite(trial.Steps[i].Target[1]) {
				return 0, nil, 0, fmt.Errorf("realnavmemory: scored target row %d is non-finite", i)
			}
			samples++
		}
	}
	if samples == 0 {
		return 0, nil, 0, fmt.Errorf("realnavmemory: trial has no scored rows")
	}
	denominator := 2 * float64(samples)
	for i, row := range predicted {
		if !trial.Steps[i].Scored {
			continue
		}
		for j := 0; j < realnavMemoryOutputSize; j++ {
			errorValue := row[j] - trial.Steps[i].Target[j]
			loss += errorValue * errorValue
			upstream[i][j] = 2 * errorValue / denominator
		}
	}
	loss /= denominator
	if !historyFinite(loss) {
		return 0, nil, 0, fmt.Errorf("realnavmemory: masked MSE is non-finite")
	}
	return loss, upstream, samples, nil
}

func fitModel(ctx context.Context, trials []historyTrial, control string, seed int64, epochs int) (before, after learning.TrainingSnapshot, curve []float64, err error) {
	if ctx == nil {
		return before, after, nil, fmt.Errorf("realnavmemory: nil training context")
	}
	if epochs <= 0 {
		return before, after, nil, fmt.Errorf("realnavmemory: epochs must be positive")
	}
	ordered, err := orderedHistoryTrials(trials)
	if err != nil {
		return before, after, nil, err
	}
	config, parameters, err := newModel(seed)
	if err != nil {
		return before, after, nil, err
	}
	trainer, err := learning.NewTrainer(config, parameters, trainingOptions())
	if err != nil {
		return before, after, nil, fmt.Errorf("realnavmemory: create trainer: %w", err)
	}
	before = trainer.Snapshot()
	curve = make([]float64, 0, epochs)
	for epoch := 0; epoch < epochs; epoch++ {
		for _, trial := range ordered {
			if err := ctx.Err(); err != nil {
				return before, after, nil, err
			}
			input, err := trialInputs(trial, control, seed)
			if err != nil {
				return before, after, nil, err
			}
			predicted, err := trainer.PredictAll(ctx, input)
			if err != nil {
				return before, after, nil, fmt.Errorf("realnavmemory: training forward pass: %w", err)
			}
			_, upstream, samples, err := maskedMSE(predicted, trial)
			if err != nil {
				return before, after, nil, err
			}
			if samples == 0 {
				return before, after, nil, fmt.Errorf("realnavmemory: trial %q has no scored rows", trial.ID)
			}
			result, err := trainer.StepFrom(ctx, input, upstream)
			if err != nil {
				return before, after, nil, fmt.Errorf("realnavmemory: training update: %w", err)
			}
			if result.GradientHorizonSteps != len(input) {
				return before, after, nil, fmt.Errorf("realnavmemory: trial %q gradient horizon %d, want %d", trial.ID, result.GradientHorizonSteps, len(input))
			}
		}
		trainMetric, err := evaluateTrainer(ctx, trainer, ordered, control, seed)
		if err != nil {
			return before, after, nil, err
		}
		curve = append(curve, trainMetric.MSE)
	}
	after = trainer.Snapshot()
	return before, after, curve, nil
}

func evaluateModel(ctx context.Context, snapshot learning.TrainingSnapshot, trials []historyTrial, control string, seed int64) (metric, error) {
	if ctx == nil {
		return metric{}, fmt.Errorf("realnavmemory: nil evaluation context")
	}
	ordered, err := orderedHistoryTrials(trials)
	if err != nil {
		return metric{}, err
	}
	network, err := learning.NewNetwork(snapshot.Config)
	if err != nil {
		return metric{}, fmt.Errorf("realnavmemory: restore evaluation network: %w", err)
	}
	var weightedLoss float64
	totalSamples := 0
	for _, trial := range ordered {
		if err := ctx.Err(); err != nil {
			return metric{}, err
		}
		input, err := trialInputs(trial, control, seed)
		if err != nil {
			return metric{}, err
		}
		predicted, err := network.PredictAll(ctx, snapshot.Parameters, input)
		if err != nil {
			return metric{}, fmt.Errorf("realnavmemory: evaluation forward pass: %w", err)
		}
		loss, _, samples, err := maskedMSE(predicted, trial)
		if err != nil {
			return metric{}, err
		}
		weightedLoss += loss * float64(samples)
		totalSamples += samples
	}
	if totalSamples == 0 {
		return metric{}, fmt.Errorf("realnavmemory: evaluation has no scored rows")
	}
	result := metric{Samples: totalSamples, MSE: weightedLoss / float64(totalSamples)}
	if !historyFinite(result.MSE) {
		return metric{}, fmt.Errorf("realnavmemory: evaluation MSE is non-finite")
	}
	return result, nil
}

func evaluateTrainer(ctx context.Context, trainer *learning.Trainer, trials []historyTrial, control string, seed int64) (metric, error) {
	if trainer == nil {
		return metric{}, fmt.Errorf("realnavmemory: nil trainer")
	}
	ordered, err := orderedHistoryTrials(trials)
	if err != nil {
		return metric{}, err
	}
	var weightedLoss float64
	totalSamples := 0
	for _, trial := range ordered {
		if err := ctx.Err(); err != nil {
			return metric{}, err
		}
		input, err := trialInputs(trial, control, seed)
		if err != nil {
			return metric{}, err
		}
		predicted, err := trainer.PredictAll(ctx, input)
		if err != nil {
			return metric{}, fmt.Errorf("realnavmemory: train metric forward pass: %w", err)
		}
		loss, _, samples, err := maskedMSE(predicted, trial)
		if err != nil {
			return metric{}, err
		}
		weightedLoss += loss * float64(samples)
		totalSamples += samples
	}
	if totalSamples == 0 {
		return metric{}, fmt.Errorf("realnavmemory: train metric has no scored rows")
	}
	return metric{Samples: totalSamples, MSE: weightedLoss / float64(totalSamples)}, nil
}

func orderedHistoryTrials(trials []historyTrial) ([]historyTrial, error) {
	if len(trials) == 0 {
		return nil, fmt.Errorf("realnavmemory: no trials")
	}
	ordered := append([]historyTrial(nil), trials...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for i, trial := range ordered {
		if trial.ID == "" {
			return nil, fmt.Errorf("realnavmemory: trial %d has empty ID", i)
		}
		if i > 0 && ordered[i-1].ID == trial.ID {
			return nil, fmt.Errorf("realnavmemory: duplicate trial ID %q", trial.ID)
		}
	}
	return ordered, nil
}
