package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	featureCount    = 5 // current x/y, previous dx/dy, and the observed dt
	modelHidden     = 12
	modelSeed       = int64(20260925)
	modelInputScale = 0.2
	modelEdgeScale  = 0.15
	modelDT         = 1.0
	modelActivation = "tanh"
	trainingChunk   = 256
)

// causalSample is one teacher-forced next-displacement target. Input is
// intentionally unexported from the report model and is built only from the
// current row and prior rows.
type causalSample struct {
	TrialID      string
	Condition    string
	Timestamp    float64
	DeltaSeconds float64
	Input        []float64
	Target       [2]float64
}

type splitTrials struct {
	Train []string `json:"train_trial_ids"`
	Test  []string `json:"test_trial_ids"`
}

type metric struct {
	Samples            int     `json:"samples"`
	MSE                float64 `json:"mse"`
	AngleErrorRadians  float64 `json:"angle_error_radians"`
	AngleSamples       int     `json:"angle_samples"`
	DirectionMatchRate float64 `json:"direction_match_rate"`
}

type baselineMetrics struct {
	ZeroOutput       metric `json:"zero_output"`
	PersistentDirect metric `json:"persistent_direction"`
	LastDisplacement metric `json:"last_displacement"`
}

type metricSet struct {
	Train metric `json:"train"`
	Test  metric `json:"test"`
}

type modelRun struct {
	Config                 learning.Config
	Before                 learning.TrainingSnapshot
	After                  learning.TrainingSnapshot
	BeforeMSE              metricSet
	AfterMSE               metricSet
	Baselines              baselineMetrics
	Updates                uint64
	MeanTargetDisplacement float64
}

type savedModel struct {
	SchemaVersion         string                    `json:"schema_version"`
	SourceSHA256          string                    `json:"source_sha256"`
	FeatureRule           string                    `json:"feature_rule"`
	Preprocessing         preprocessingContract     `json:"preprocessing"`
	TrainMeanDisplacement float64                   `json:"train_mean_displacement_cm_per_sample"`
	Split                 splitTrials               `json:"split"`
	Snapshot              learning.TrainingSnapshot `json:"snapshot"`
}

func newModel() (learning.Config, learning.Parameters, error) {
	nodes := featureCount + modelHidden
	sources := make([]int, 0, featureCount*modelHidden+modelHidden*modelHidden)
	targets := make([]int, 0, cap(sources))
	for input := 0; input < featureCount; input++ {
		for hidden := featureCount; hidden < nodes; hidden++ {
			sources = append(sources, input)
			targets = append(targets, hidden)
		}
	}
	for source := featureCount; source < nodes; source++ {
		for target := featureCount; target < nodes; target++ {
			sources = append(sources, source)
			targets = append(targets, target)
		}
	}
	inputNodes := make([]int, featureCount)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, modelHidden)
	for i := range readoutNodes {
		readoutNodes[i] = featureCount + i
	}
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes:      nodes,
			Sources:    sources,
			Targets:    targets,
			DT:         modelDT,
			Activation: modelActivation,
		},
		InputSize:        featureCount,
		OutputSize:       2,
		InputNodes:       inputNodes,
		ReadoutNodes:     readoutNodes,
		ReadoutEveryStep: true,
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniformParameters(modelSeed, 0, len(sources), modelEdgeScale),
			Bias:    make([]float64, nodes),
			LogTau:  make([]float64, nodes),
		},
		Encoder: uniformParameters(modelSeed, 1, featureCount*featureCount, modelInputScale),
		// The explicit persistence term is added outside the network. Zero
		// readout weights make the initial predictor equal that causal baseline;
		// training learns only the residual correction.
		Readout: make([]float64, modelHidden*2),
	}
	for i := range parameters.Core.LogTau {
		parameters.Core.LogTau[i] = math.Log(2)
	}
	if _, err := learning.NewNetwork(config); err != nil {
		return learning.Config{}, learning.Parameters{}, err
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

func defaultTrainingOptions() learning.Options {
	options := learning.DefaultOptions()
	options.LearningRate = 0.01
	options.ClipNorm = 1
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true}
	return options
}

func fitSamples(ctx context.Context, train, test []causalSample, epochs int) (modelRun, error) {
	if ctx == nil {
		return modelRun{}, errors.New("realnav: nil training context")
	}
	if epochs <= 0 {
		return modelRun{}, errors.New("realnav: epochs must be positive")
	}
	if len(train) == 0 {
		return modelRun{}, errors.New("realnav: no training samples")
	}
	config, parameters, err := newModel()
	if err != nil {
		return modelRun{}, fmt.Errorf("build model: %w", err)
	}
	options := defaultTrainingOptions()
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return modelRun{}, fmt.Errorf("create trainer: %w", err)
	}
	before := trainer.Snapshot()
	meanTargetDisplacement := meanTargetDisplacement(train)
	beforeMetric, err := evaluateModel(ctx, before.Config, before.Parameters, train, test)
	if err != nil {
		return modelRun{}, fmt.Errorf("evaluate initial model: %w", err)
	}
	sequences := groupSamples(train)
	for epoch := 0; epoch < epochs; epoch++ {
		for _, sequence := range sequences {
			for start := 0; start < len(sequence); start += trainingChunk {
				if err := ctx.Err(); err != nil {
					return modelRun{}, err
				}
				end := start + trainingChunk
				if end > len(sequence) {
					end = len(sequence)
				}
				input, target := sampleRows(sequence[start:end])
				predicted, err := trainer.PredictAll(ctx, input)
				if err != nil {
					return modelRun{}, fmt.Errorf("training forward pass: %w", err)
				}
				effective, err := addModelBaseline(predicted, sequence[start:end], meanTargetDisplacement)
				if err != nil {
					return modelRun{}, fmt.Errorf("training persistence baseline: %w", err)
				}
				upstream, err := mseGradient(effective, target)
				if err != nil {
					return modelRun{}, fmt.Errorf("training gradient: %w", err)
				}
				if _, err := trainer.StepFrom(ctx, input, upstream); err != nil {
					return modelRun{}, fmt.Errorf("training update: %w", err)
				}
			}
		}
	}
	after := trainer.Snapshot()
	afterMetric, err := evaluateModel(ctx, after.Config, after.Parameters, train, test)
	if err != nil {
		return modelRun{}, fmt.Errorf("evaluate trained model: %w", err)
	}
	baselines := baselineMetricsFor(test, meanTargetDisplacement)
	return modelRun{Config: after.Config, Before: before, After: after, BeforeMSE: beforeMetric, AfterMSE: afterMetric, Baselines: baselines, Updates: after.Updates, MeanTargetDisplacement: meanTargetDisplacement}, nil
}

func groupSamples(samples []causalSample) [][]causalSample {
	byTrial := make(map[string][]causalSample)
	order := make([]string, 0)
	for _, sample := range samples {
		if _, ok := byTrial[sample.TrialID]; !ok {
			order = append(order, sample.TrialID)
		}
		byTrial[sample.TrialID] = append(byTrial[sample.TrialID], sample)
	}
	sort.Strings(order)
	sequences := make([][]causalSample, 0, len(order))
	for _, trialID := range order {
		sequence := byTrial[trialID]
		sort.SliceStable(sequence, func(i, j int) bool { return sequence[i].Timestamp < sequence[j].Timestamp })
		sequences = append(sequences, sequence)
	}
	return sequences
}

func sampleRows(samples []causalSample) ([][]float64, [][]float64) {
	input := make([][]float64, len(samples))
	target := make([][]float64, len(samples))
	for i, sample := range samples {
		input[i] = append([]float64(nil), sample.Input...)
		target[i] = []float64{sample.Target[0], sample.Target[1]}
	}
	return input, target
}

func mseGradient(predicted, target [][]float64) ([][]float64, error) {
	if len(predicted) != len(target) || len(predicted) == 0 {
		return nil, errors.New("prediction and target row counts differ or are empty")
	}
	upstream := make([][]float64, len(predicted))
	denom := float64(len(predicted) * 2)
	for i := range predicted {
		if len(predicted[i]) != 2 || len(target[i]) != 2 {
			return nil, errors.New("prediction and target rows must have width 2")
		}
		upstream[i] = make([]float64, 2)
		for j := range upstream[i] {
			value := 2 * (predicted[i][j] - target[i][j]) / denom
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errors.New("non-finite MSE gradient")
			}
			upstream[i][j] = value
		}
	}
	return upstream, nil
}

func evaluateModel(ctx context.Context, config learning.Config, parameters learning.Parameters, train, test []causalSample) (metricSet, error) {
	network, err := learning.NewNetwork(config)
	if err != nil {
		return metricSet{}, err
	}
	meanTargetDisplacement := meanTargetDisplacement(train)
	trainMetric, err := evaluateSplit(ctx, network, parameters, train, meanTargetDisplacement)
	if err != nil {
		return metricSet{}, err
	}
	testMetric, err := evaluateSplit(ctx, network, parameters, test, meanTargetDisplacement)
	if err != nil {
		return metricSet{}, err
	}
	return metricSet{Train: trainMetric, Test: testMetric}, nil
}

func evaluateSplit(ctx context.Context, network *learning.Network, parameters learning.Parameters, samples []causalSample, meanTargetDisplacement float64) (metric, error) {
	var result metric
	for _, sequence := range groupSamples(samples) {
		if err := ctx.Err(); err != nil {
			return metric{}, err
		}
		for start := 0; start < len(sequence); start += trainingChunk {
			end := start + trainingChunk
			if end > len(sequence) {
				end = len(sequence)
			}
			chunk := sequence[start:end]
			input, target := sampleRows(chunk)
			predicted, err := network.PredictAll(ctx, parameters, input)
			if err != nil {
				return metric{}, err
			}
			predicted, err = addModelBaseline(predicted, chunk, meanTargetDisplacement)
			if err != nil {
				return metric{}, err
			}
			for i := range predicted {
				addMetric(&result, chunk[i], predicted[i], target[i])
			}
		}
	}
	finalizeMetric(&result)
	return result, nil
}

func evaluateTrainer(ctx context.Context, trainer *learning.Trainer, samples []causalSample, meanTargetDisplacement float64) (metric, error) {
	if trainer == nil {
		return metric{}, errors.New("realnav: nil inference trainer")
	}
	var result metric
	for _, sequence := range groupSamples(samples) {
		if err := ctx.Err(); err != nil {
			return metric{}, err
		}
		for start := 0; start < len(sequence); start += trainingChunk {
			end := start + trainingChunk
			if end > len(sequence) {
				end = len(sequence)
			}
			chunk := sequence[start:end]
			input, target := sampleRows(chunk)
			predicted, err := trainer.PredictAll(ctx, input)
			if err != nil {
				return metric{}, err
			}
			predicted, err = addModelBaseline(predicted, chunk, meanTargetDisplacement)
			if err != nil {
				return metric{}, err
			}
			for i := range predicted {
				addMetric(&result, chunk[i], predicted[i], target[i])
			}
		}
	}
	finalizeMetric(&result)
	return result, nil
}

func addMetric(m *metric, sample causalSample, prediction, target []float64) {
	_ = sample
	m.Samples++
	dx, dy := prediction[0], prediction[1]
	errX, errY := dx-target[0], dy-target[1]
	m.MSE += (errX*errX + errY*errY) / 2
	if dx*dx+dy*dy > 1e-18 && target[0]*target[0]+target[1]*target[1] > 1e-18 {
		angle := math.Acos(clamp((dx*target[0]+dy*target[1])/(math.Hypot(dx, dy)*math.Hypot(target[0], target[1])), -1, 1))
		m.AngleErrorRadians += angle
		m.AngleSamples++
	}
	if dx*dx+dy*dy > 1e-18 && target[0]*target[0]+target[1]*target[1] > 1e-18 {
		if dx*target[0]+dy*target[1] > 0 {
			m.DirectionMatchRate++
		}
	}
}

func finalizeMetric(m *metric) {
	if m.Samples == 0 {
		return
	}
	m.MSE /= float64(m.Samples)
	if m.AngleSamples > 0 {
		m.AngleErrorRadians /= float64(m.AngleSamples)
	}
	m.DirectionMatchRate /= float64(m.Samples)
}

func baselineMetricsFor(test []causalSample, meanSpeed float64) baselineMetrics {
	return baselineMetrics{
		ZeroOutput:       evaluateBaseline(test, func(sample causalSample) [2]float64 { return zeroBaseline(sample) }),
		PersistentDirect: evaluateBaseline(test, func(sample causalSample) [2]float64 { return directionBaseline(sample, meanSpeed) }),
		LastDisplacement: evaluateBaseline(test, func(sample causalSample) [2]float64 { return lastDisplacementBaseline(sample) }),
	}
}

func evaluateBaseline(samples []causalSample, predict func(causalSample) [2]float64) metric {
	var result metric
	for _, sample := range samples {
		prediction := predict(sample)
		addMetric(&result, sample, []float64{prediction[0], prediction[1]}, []float64{sample.Target[0], sample.Target[1]})
	}
	finalizeMetric(&result)
	return result
}

func meanTargetDisplacement(samples []causalSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var total float64
	for _, sample := range samples {
		total += math.Hypot(sample.Target[0], sample.Target[1])
	}
	return total / float64(len(samples))
}

func addModelBaseline(predicted [][]float64, samples []causalSample, meanTargetDisplacement float64) ([][]float64, error) {
	if len(predicted) != len(samples) {
		return nil, errors.New("prediction and sample row counts differ")
	}
	effective := make([][]float64, len(predicted))
	for i, row := range predicted {
		if len(row) != 2 {
			return nil, errors.New("prediction rows must have width 2")
		}
		base := directionBaseline(samples[i], meanTargetDisplacement)
		effective[i] = []float64{row[0] + base[0], row[1] + base[1]}
	}
	return effective, nil
}

func directionBaseline(sample causalSample, meanSpeed float64) [2]float64 {
	dx, dy := sample.Input[2]*displacementScale, sample.Input[3]*displacementScale
	norm := math.Hypot(dx, dy)
	if norm == 0 {
		return [2]float64{}
	}
	return [2]float64{dx / norm * meanSpeed, dy / norm * meanSpeed}
}

func zeroBaseline(sample causalSample) [2]float64 {
	_ = sample
	return [2]float64{}
}

func lastDisplacementBaseline(sample causalSample) [2]float64 {
	return [2]float64{sample.Input[2] * displacementScale, sample.Input[3] * displacementScale}
}

func snapshotFingerprint(snapshot learning.TrainingSnapshot) string {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func restoreTrainer(snapshot learning.TrainingSnapshot) (*learning.Trainer, error) {
	return learning.RestoreTrainer(snapshot)
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func isFinitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
