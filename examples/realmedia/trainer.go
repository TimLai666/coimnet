package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
	"unicode"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/media/realdata"
)

const (
	framePixels            = 8 * 8 * 3
	audioPerFrame          = 8000 / 4
	frameOutputSize        = framePixels + audioPerFrame
	textFeatureCount       = 64
	timeFeatureCount       = 4
	modelInputSize         = textFeatureCount + timeFeatureCount
	modelEmbed             = 8
	modelHidden            = 16
	modelSeed        int64 = 11011
)

// score is reported on clipped physical image/audio outputs, before PNG/WAV
// quantization. BalancedMSE gives the two modalities equal aggregate weight.
type score struct {
	Samples            int     `json:"samples"`
	Frames             int     `json:"frames"`
	AudioSamples       int     `json:"audio_samples"`
	ImageMSE           float64 `json:"image_mse"`
	AudioMSE           float64 `json:"audio_mse"`
	BalancedMSE        float64 `json:"balanced_mse"`
	TemporalPairs      int     `json:"temporal_pairs"`
	TemporalDeltaMSE   float64 `json:"temporal_delta_mse"`
	ClippedImageValues int     `json:"clipped_image_values"`
	ClippedAudioValues int     `json:"clipped_audio_values"`
}

type sequencePrediction struct {
	SourceID  string      `json:"source_id"`
	SegmentID string      `json:"segment_id"`
	Split     string      `json:"split"`
	Rows      [][]float64 `json:"-"`
}

type fitResult struct {
	Config       learning.Config
	Before       learning.TrainingSnapshot
	Snapshot     learning.TrainingSnapshot
	BeforeScores map[string]score
	AfterScores  map[string]score
	Predictions  []sequencePrediction
}

func fitSamples(ctx context.Context, samples []realdata.Sample, epochs int) (fitResult, error) {
	if ctx == nil {
		return fitResult{}, fmt.Errorf("realmedia: nil training context")
	}
	if epochs <= 0 {
		return fitResult{}, fmt.Errorf("realmedia: epochs must be positive")
	}
	if err := validateSamples(samples); err != nil {
		return fitResult{}, err
	}
	config, parameters, err := newModel()
	if err != nil {
		return fitResult{}, err
	}
	options := learning.DefaultOptions()
	options.LearningRate = 0.01
	options.ClipNorm = 1
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: create learning trainer: %w", err)
	}
	before := trainer.Snapshot()
	beforePredictions, err := predictSamples(ctx, before.Config, before.Parameters, samples)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: baseline inference: %w", err)
	}
	if err := validatePredictionSet(samples, beforePredictions); err != nil {
		return fitResult{}, fmt.Errorf("realmedia: baseline prediction coverage: %w", err)
	}
	beforeScores, err := scorePredictions(samples, beforePredictions)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: score baseline predictions: %w", err)
	}
	trainSamples := make([]realdata.Sample, 0, len(samples))
	for _, sample := range samples {
		if sample.Split == "train" {
			trainSamples = append(trainSamples, sample)
		}
	}
	if len(trainSamples) == 0 {
		return fitResult{}, fmt.Errorf("realmedia: no training samples")
	}
	trainingInputs := make([][][]float64, len(trainSamples))
	trainingTargets := make([][][]float64, len(trainSamples))
	for i, sample := range trainSamples {
		trainingInputs[i], trainingTargets[i], err = trainingSequence(sample)
		if err != nil {
			return fitResult{}, err
		}
	}
	for epoch := 0; epoch < epochs; epoch++ {
		for i := range trainSamples {
			if err := ctx.Err(); err != nil {
				return fitResult{}, fmt.Errorf("realmedia: training canceled: %w", err)
			}
			predicted, err := trainer.PredictAll(ctx, trainingInputs[i])
			if err != nil {
				return fitResult{}, fmt.Errorf("realmedia: training forward pass for %s/%s: %w", trainSamples[i].SourceID, trainSamples[i].SegmentID, err)
			}
			upstream, err := mseGradient(predicted, trainingTargets[i])
			if err != nil {
				return fitResult{}, fmt.Errorf("realmedia: training gradient for %s/%s: %w", trainSamples[i].SourceID, trainSamples[i].SegmentID, err)
			}
			if _, err := trainer.StepFrom(ctx, trainingInputs[i], upstream); err != nil {
				return fitResult{}, fmt.Errorf("realmedia: learning update for %s/%s: %w", trainSamples[i].SourceID, trainSamples[i].SegmentID, err)
			}
		}
	}
	after := trainer.Snapshot()
	afterPredictions, err := predictSamples(ctx, after.Config, after.Parameters, samples)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: frozen independent inference: %w", err)
	}
	if err := validatePredictionSet(samples, afterPredictions); err != nil {
		return fitResult{}, fmt.Errorf("realmedia: frozen prediction coverage: %w", err)
	}
	afterScores, err := scorePredictions(samples, afterPredictions)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: score frozen predictions: %w", err)
	}
	return fitResult{
		Config: after.Config, Before: before, Snapshot: after,
		BeforeScores: beforeScores, AfterScores: afterScores,
		Predictions: afterPredictions,
	}, nil
}

func newModel() (learning.Config, learning.Parameters, error) {
	const nodes = modelEmbed + modelHidden
	sources := make([]int, 0, modelEmbed*modelHidden+modelHidden*modelHidden)
	targets := make([]int, 0, cap(sources))
	for input := 0; input < modelEmbed; input++ {
		for hidden := modelEmbed; hidden < nodes; hidden++ {
			sources = append(sources, input)
			targets = append(targets, hidden)
		}
	}
	for source := modelEmbed; source < nodes; source++ {
		for target := modelEmbed; target < nodes; target++ {
			sources = append(sources, source)
			targets = append(targets, target)
		}
	}
	inputNodes := make([]int, modelEmbed)
	readoutNodes := make([]int, modelHidden)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	for i := range readoutNodes {
		readoutNodes[i] = modelEmbed + i
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        modelInputSize,
		OutputSize:       frameOutputSize,
		InputNodes:       inputNodes,
		ReadoutNodes:     readoutNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniform(modelSeed, 0, len(sources), 1/math.Sqrt(float64(nodes))),
			Bias:    make([]float64, nodes),
			LogTau:  logTau,
		},
		Encoder: uniform(modelSeed, 1, modelInputSize*modelEmbed, 0.5),
		Readout: uniform(modelSeed, 2, modelHidden*frameOutputSize, 0.05),
	}
	if _, err := learning.NewNetwork(config); err != nil {
		return learning.Config{}, learning.Parameters{}, fmt.Errorf("realmedia: build network: %w", err)
	}
	return config, parameters, nil
}

func uniform(seed int64, stream int64, count int, scale float64) []float64 {
	rng := rand.New(rand.NewSource(seed + stream*7919))
	values := make([]float64, count)
	for i := range values {
		values[i] = (2*rng.Float64() - 1) * scale
	}
	return values
}

func predictSamples(ctx context.Context, config learning.Config, parameters learning.Parameters, samples []realdata.Sample) ([]sequencePrediction, error) {
	network, err := learning.NewNetwork(config)
	if err != nil {
		return nil, fmt.Errorf("realmedia: create frozen inference network: %w", err)
	}
	predictions := make([]sequencePrediction, 0, len(samples))
	for _, sample := range samples {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input := makeInputRows(sample.Prompt, len(sample.Frames))
		rows, err := network.PredictAll(ctx, parameters, input)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", sample.SourceID, sample.SegmentID, err)
		}
		predictions = append(predictions, sequencePrediction{
			SourceID: sample.SourceID, SegmentID: sample.SegmentID, Split: sample.Split, Rows: rows,
		})
	}
	return predictions, nil
}

func scorePredictions(samples []realdata.Sample, predictions []sequencePrediction) (map[string]score, error) {
	if err := validatePredictionSet(samples, predictions); err != nil {
		return nil, err
	}
	byKey := make(map[string]sequencePrediction, len(predictions))
	for _, prediction := range predictions {
		byKey[prediction.SourceID+"/"+prediction.SegmentID] = prediction
	}
	type totals struct {
		score
		imageSquared    float64
		audioSquared    float64
		temporalSquared float64
	}
	aggregates := make(map[string]*totals)
	for _, sample := range samples {
		prediction, ok := byKey[sample.SourceID+"/"+sample.SegmentID]
		if !ok {
			return nil, fmt.Errorf("missing prediction for %s/%s", sample.SourceID, sample.SegmentID)
		}
		current := aggregates[sample.Split]
		if current == nil {
			current = &totals{}
			aggregates[sample.Split] = current
		}
		current.Samples++
		current.Frames += len(sample.Frames)
		previousPredicted := make([]float64, framePixels)
		previousTarget := make([]float64, framePixels)
		for frameIndex, row := range prediction.Rows {
			frame := sample.Frames[frameIndex]
			if frameIndex > 0 {
				current.TemporalPairs++
			}
			for i, target := range frame.Pixels {
				value := row[i] / imageOutputScale()
				if value < 0 {
					value = 0
					current.ClippedImageValues++
				} else if value > 1 {
					value = 1
					current.ClippedImageValues++
				}
				delta := value - target
				current.imageSquared += delta * delta
				if frameIndex > 0 {
					temporalDelta := (value - previousPredicted[i]) - (target - previousTarget[i])
					current.temporalSquared += temporalDelta * temporalDelta
				}
				previousPredicted[i] = value
				previousTarget[i] = target
			}
			for i := frame.AudioStart; i < frame.AudioEnd; i++ {
				value := row[len(frame.Pixels)+i-frame.AudioStart] / audioOutputScale()
				if value < -1 {
					value = -1
					current.ClippedAudioValues++
				} else if value > 1 {
					value = 1
					current.ClippedAudioValues++
				}
				delta := value - sample.Audio[i]
				current.audioSquared += delta * delta
				current.AudioSamples++
			}
		}
	}
	result := make(map[string]score, len(aggregates))
	for split, current := range aggregates {
		imageValues := current.Frames * framePixels
		if imageValues > 0 {
			current.ImageMSE = current.imageSquared / float64(imageValues)
		}
		if current.AudioSamples > 0 {
			current.AudioMSE = current.audioSquared / float64(current.AudioSamples)
		}
		if current.TemporalPairs > 0 {
			current.TemporalDeltaMSE = current.temporalSquared / float64(current.TemporalPairs*framePixels)
		}
		current.BalancedMSE = (current.ImageMSE + current.AudioMSE) / 2
		result[split] = current.score
	}
	return result, nil
}

func validatePredictionSet(samples []realdata.Sample, predictions []sequencePrediction) error {
	if len(samples) != len(predictions) {
		return fmt.Errorf("got %d predictions for %d source samples", len(predictions), len(samples))
	}
	seen := make(map[string]bool, len(predictions))
	for i, sample := range samples {
		prediction := predictions[i]
		key := sample.SourceID + "/" + sample.SegmentID
		if seen[key] || prediction.SourceID != sample.SourceID || prediction.SegmentID != sample.SegmentID || prediction.Split != sample.Split {
			return fmt.Errorf("prediction %d does not uniquely match source sample %s", i, key)
		}
		seen[key] = true
		if len(prediction.Rows) != len(sample.Frames) {
			return fmt.Errorf("prediction for %s has %d rows, want %d", key, len(prediction.Rows), len(sample.Frames))
		}
		for row, values := range prediction.Rows {
			if len(values) != frameOutputSize {
				return fmt.Errorf("prediction for %s row %d has %d values, want %d", key, row, len(values), frameOutputSize)
			}
			for column, value := range values {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return fmt.Errorf("prediction for %s row %d value %d is non-finite", key, row, column)
				}
			}
		}
	}
	return nil
}

func trainingSequence(sample realdata.Sample) ([][]float64, [][]float64, error) {
	inputs := makeInputRows(sample.Prompt, len(sample.Frames))
	targets := make([][]float64, len(sample.Frames))
	for frameIndex, frame := range sample.Frames {
		if len(frame.Pixels) != framePixels || frame.AudioStart < 0 || frame.AudioEnd-frame.AudioStart != audioPerFrame || frame.AudioEnd > len(sample.Audio) {
			return nil, nil, fmt.Errorf("realmedia: %s/%s frame %d has incompatible image/audio dimensions", sample.SourceID, sample.SegmentID, frameIndex)
		}
		targets[frameIndex] = make([]float64, frameOutputSize)
		for i, value := range frame.Pixels {
			targets[frameIndex][i] = value * imageOutputScale()
		}
		for i, value := range sample.Audio[frame.AudioStart:frame.AudioEnd] {
			targets[frameIndex][framePixels+i] = value * audioOutputScale()
		}
	}
	return inputs, targets, nil
}

func mseGradient(predicted, targets [][]float64) ([][]float64, error) {
	if len(predicted) != len(targets) || len(predicted) == 0 {
		return nil, fmt.Errorf("prediction/target row count mismatch")
	}
	gradient := make([][]float64, len(predicted))
	for row := range predicted {
		if len(predicted[row]) != len(targets[row]) || len(predicted[row]) == 0 {
			return nil, fmt.Errorf("prediction/target width mismatch at row %d", row)
		}
		gradient[row] = make([]float64, len(predicted[row]))
		factor := 2 / float64(len(predicted[row])*len(predicted))
		for i, value := range predicted[row] {
			gradient[row][i] = (value - targets[row][i]) * factor
		}
	}
	return gradient, nil
}

func makeInputRows(prompt string, frameCount int) [][]float64 {
	if frameCount <= 0 {
		return nil
	}
	tokens := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	text := make([]float64, textFeatureCount)
	for _, token := range tokens {
		addHashed(text, "u:"+token, 1)
	}
	for i := 1; i < len(tokens); i++ {
		addHashed(text, "b:"+tokens[i-1]+" "+tokens[i], 0.75)
	}
	norm := math.Sqrt(float64(max(1, len(tokens))))
	for i := range text {
		text[i] /= norm
	}
	rows := make([][]float64, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		row := make([]float64, modelInputSize)
		copy(row, text)
		position := 0.0
		if frameCount > 1 {
			position = float64(frame) / float64(frameCount-1)
		}
		row[textFeatureCount] = 2*position - 1
		row[textFeatureCount+1] = math.Sin(2 * math.Pi * position)
		row[textFeatureCount+2] = math.Cos(2 * math.Pi * position)
		row[textFeatureCount+3] = 1
		rows[frame] = row
	}
	return rows
}

func addHashed(destination []float64, text string, amount float64) {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(text))
	value := hash.Sum64()
	bucket := int(value % uint64(len(destination)))
	if value&(uint64(1)<<63) == 0 {
		destination[bucket] += amount
	} else {
		destination[bucket] -= amount
	}
}

func imageOutputScale() float64 { return math.Sqrt(float64(frameOutputSize) / (2 * framePixels)) }
func audioOutputScale() float64 { return math.Sqrt(float64(frameOutputSize) / (2 * audioPerFrame)) }

func validateSamples(samples []realdata.Sample) error {
	if len(samples) == 0 {
		return fmt.Errorf("realmedia: imported dataset has no samples")
	}
	seen := make(map[string]bool, len(samples))
	trainCount := 0
	for _, sample := range samples {
		key := sample.SourceID + "/" + sample.SegmentID
		if sample.SourceID == "" || sample.SegmentID == "" || seen[key] {
			return fmt.Errorf("realmedia: sample identity is empty or duplicated: %q", key)
		}
		seen[key] = true
		if sample.Split != "train" && sample.Split != "validation" && sample.Split != "test" {
			return fmt.Errorf("realmedia: sample %s has invalid split %q", key, sample.Split)
		}
		if sample.PromptSource != "human_annotation" || strings.TrimSpace(sample.Prompt) == "" {
			return fmt.Errorf("realmedia: sample %s lacks an annotated text prompt", key)
		}
		if sample.FrameRate != 4 || sample.AudioSampleRate != 8000 || len(sample.Frames) == 0 ||
			sample.DurationMillis != int64(len(sample.Frames)*1000/sample.FrameRate) || len(sample.Audio) != len(sample.Frames)*audioPerFrame {
			return fmt.Errorf("realmedia: sample %s has invalid frame/audio sequence lengths", key)
		}
		if sample.Split == "train" {
			trainCount++
		}
		lastAudioEnd := 0
		for frameIndex, frame := range sample.Frames {
			if frame.Index != frameIndex || frame.TimeMillis != sample.StartMillis+int64(frameIndex*1000/sample.FrameRate) ||
				len(frame.Pixels) != framePixels || frame.AudioStart != lastAudioEnd || frame.AudioEnd-frame.AudioStart != audioPerFrame || frame.AudioEnd > len(sample.Audio) {
				return fmt.Errorf("realmedia: sample %s frame %d has invalid decoded dimensions or synchronization", key, frameIndex)
			}
			lastAudioEnd = frame.AudioEnd
			for _, value := range frame.Pixels {
				if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
					return fmt.Errorf("realmedia: sample %s has an invalid decoded pixel", key)
				}
			}
		}
		if lastAudioEnd != len(sample.Audio) {
			return fmt.Errorf("realmedia: sample %s frame/audio clocks do not cover the full clip", key)
		}
		for _, value := range sample.Audio {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
				return fmt.Errorf("realmedia: sample %s has an invalid decoded audio value", key)
			}
		}
	}
	if trainCount == 0 {
		return fmt.Errorf("realmedia: imported dataset has no training samples")
	}
	return nil
}
