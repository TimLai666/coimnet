package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/media/realdata"
)

const (
	mediaModelTaskSchema = "coimnet-realmedia-model/v1"
	mediaEncoderProfile  = "signed-fnv1a-unigram-bigram-v1"
	mediaDecoderProfile  = "linear-rgb-audio-v1"
	mediaScaleProfile    = "sqrt-output-dimension-v1"
)

// mediaModelConfig binds the input features and output decoder to the source.
type mediaModelConfig struct {
	ManifestSHA256 string              `json:"manifest_sha256"`
	Output         realdata.OutputSpec `json:"output"`
	EncoderProfile string              `json:"encoder_profile"`
	DecoderProfile string              `json:"decoder_profile"`
	ScaleProfile   string              `json:"scale_profile"`
	TextFeatures   int                 `json:"text_features"`
	TimeFeatures   int                 `json:"time_features"`
	Seed           int64               `json:"seed"`
	ImageScale     float64             `json:"image_scale"`
	AudioScale     float64             `json:"audio_scale"`
}

type mediaModelArtifact struct {
	Mode           string `json:"mode"`
	Path           string `json:"path"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Updates        uint64 `json:"updates"`
}

func newMediaModelConfig(manifestSHA256 string, output realdata.OutputSpec) mediaModelConfig {
	return mediaModelConfig{
		ManifestSHA256: manifestSHA256,
		Output:         output,
		EncoderProfile: mediaEncoderProfile,
		DecoderProfile: mediaDecoderProfile,
		ScaleProfile:   mediaScaleProfile,
		TextFeatures:   textFeatureCount,
		TimeFeatures:   timeFeatureCount,
		Seed:           modelSeed,
		ImageScale:     imageOutputScale(),
		AudioScale:     audioOutputScale(),
	}
}

func saveMediaModel(ctx context.Context, directory string, config mediaModelConfig, snapshot learning.TrainingSnapshot) error {
	if err := mediaContextError(ctx); err != nil {
		return err
	}
	if err := validateMediaModelConfig(config); err != nil {
		return fmt.Errorf("realmedia: invalid model config: %w", err)
	}
	owned, err := validateMediaTrainingSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("realmedia: invalid model snapshot: %w", err)
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("realmedia: encode model config: %w", err)
	}
	if err := checkpoint.SaveTaskTrainingBundle(ctx, directory, mediaModelTaskSchema, configBytes, owned); err != nil {
		return fmt.Errorf("realmedia: save task model bundle: %w", err)
	}
	return nil
}

func loadMediaModel(ctx context.Context, directory string, expected mediaModelConfig) (learning.TrainingSnapshot, string, error) {
	var empty learning.TrainingSnapshot
	if err := mediaContextError(ctx); err != nil {
		return empty, "", err
	}
	if err := validateMediaModelConfig(expected); err != nil {
		return empty, "", fmt.Errorf("realmedia: invalid expected model config: %w", err)
	}
	document, err := checkpoint.LoadTaskTrainingBundle(ctx, directory, mediaModelTaskSchema)
	if err != nil {
		return empty, "", fmt.Errorf("realmedia: load task model bundle: %w", err)
	}
	var got mediaModelConfig
	if err := checkpoint.DecodeTaskConfig(document.Config, &got); err != nil {
		return empty, "", fmt.Errorf("realmedia: decode model config: %w", err)
	}
	if err := validateMediaModelConfig(got); err != nil {
		return empty, "", fmt.Errorf("realmedia: invalid saved model config: %w", err)
	}
	if !reflect.DeepEqual(got, expected) {
		return empty, "", fmt.Errorf("realmedia: saved model config does not match the current source and decoder profile")
	}
	owned, err := validateMediaTrainingSnapshot(document.Training)
	if err != nil {
		return empty, "", fmt.Errorf("realmedia: invalid saved model snapshot: %w", err)
	}
	if document.ManifestSHA256 == "" {
		return empty, "", fmt.Errorf("realmedia: loaded task model has no verified bundle manifest SHA-256")
	}
	return owned, document.ManifestSHA256, nil
}

func fitFromSnapshot(ctx context.Context, samples []realdata.Sample, snapshot learning.TrainingSnapshot) (fitResult, error) {
	if err := mediaContextError(ctx); err != nil {
		return fitResult{}, err
	}
	if err := validateSamples(samples); err != nil {
		return fitResult{}, err
	}
	owned, err := validateMediaTrainingSnapshot(snapshot)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: validate restored model: %w", err)
	}
	before, err := learning.RestoreTrainer(owned)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: restore inference trainer: %w", err)
	}
	beforeSnapshot := before.Snapshot()
	afterSnapshot := before.Snapshot()
	predictions, err := predictSamples(ctx, beforeSnapshot.Config, beforeSnapshot.Parameters, samples)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: restored inference: %w", err)
	}
	scores, err := scorePredictions(samples, predictions)
	if err != nil {
		return fitResult{}, fmt.Errorf("realmedia: score restored inference: %w", err)
	}
	afterScores := make(map[string]score, len(scores))
	for split, value := range scores {
		afterScores[split] = value
	}
	return fitResult{
		Config:       beforeSnapshot.Config,
		Before:       beforeSnapshot,
		Snapshot:     afterSnapshot,
		BeforeScores: scores,
		AfterScores:  afterScores,
		Predictions:  predictions,
	}, nil
}

// run retains the original entry point for callers that do not use model flags.
func run(ctx context.Context, dataRoot, outDir string) error {
	return runWithModel(ctx, dataRoot, outDir, "", "")
}

func mediaContextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("realmedia: nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("realmedia: context ended: %w", err)
	}
	return nil
}

func validateMediaModelConfig(config mediaModelConfig) error {
	if !validMediaSHA256(config.ManifestSHA256) {
		return fmt.Errorf("manifest SHA-256 must be 64 lowercase hexadecimal characters")
	}
	if err := validateExampleOutput(config.Output); err != nil {
		return err
	}
	want := newMediaModelConfig(config.ManifestSHA256, config.Output)
	if !reflect.DeepEqual(config, want) {
		return fmt.Errorf("config does not match the fixed realmedia model profile")
	}
	return nil
}

func validateMediaTrainingSnapshot(snapshot learning.TrainingSnapshot) (learning.TrainingSnapshot, error) {
	expectedConfig, expectedParameters, err := newModel()
	if err != nil {
		return learning.TrainingSnapshot{}, err
	}
	expectedTrainer, err := learning.NewTrainer(expectedConfig, expectedParameters, mediaTrainingOptions())
	if err != nil {
		return learning.TrainingSnapshot{}, err
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return learning.TrainingSnapshot{}, err
	}
	owned := trainer.Snapshot()
	expected := expectedTrainer.Snapshot()
	if !reflect.DeepEqual(owned.Config, expected.Config) {
		return learning.TrainingSnapshot{}, fmt.Errorf("learning config does not match newModel topology and dimensions")
	}
	if !reflect.DeepEqual(owned.Options, expected.Options) {
		return learning.TrainingSnapshot{}, fmt.Errorf("learning options do not match the fixed learning-rate, clipping and trainable profile")
	}
	return owned, nil
}

func mediaTrainingOptions() learning.Options {
	options := learning.DefaultOptions()
	options.LearningRate = 0.01
	options.ClipNorm = 1
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Bias: true, Tau: true, Readout: true}
	return options
}

func validMediaSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || pathWithin(left, right) || pathWithin(right, left)
}
