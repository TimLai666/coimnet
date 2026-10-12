package ocr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
)

const ocrCheckpointSchema = "coimnet-ocr-checkpoint/v1"

// LoadedCheckpointSHA256 identifies the manifest verified when loading this
// model. New models return an empty string; later training retains the origin.
func (m *Recognizer) LoadedCheckpointSHA256() string {
	if m == nil {
		return ""
	}
	return m.loadedCheckpointSHA256
}

// Save publishes a complete task model and optimizer checkpoint without replacing an existing path.
func (m *Recognizer) Save(ctx context.Context, path string) error {
	if m == nil || m.trainer == nil {
		return errors.New("ocr: nil recognizer")
	}
	if err := ocrContextError(ctx); err != nil {
		return err
	}
	if path == "" {
		return errors.New("ocr: checkpoint path must not be empty")
	}
	config := m.config
	config.Alphabet = append([]rune(nil), config.Alphabet...)
	if err := validateOCRConfig(config); err != nil {
		return err
	}
	if err := validateOCRIndex(config, m.index); err != nil {
		return err
	}
	snapshot := m.trainer.Snapshot()
	if err := validateOCRTraining(config, snapshot); err != nil {
		return err
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("ocr: encode checkpoint config: %w", err)
	}
	return checkpoint.SaveTaskTrainingBundle(ctx, path, ocrCheckpointSchema, configJSON, snapshot)
}

// LoadRecognizer restores the task configuration and complete optimizer state from a verified checkpoint.
func LoadRecognizer(ctx context.Context, path string) (*Recognizer, error) {
	if err := ocrContextError(ctx); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("ocr: checkpoint path must not be empty")
	}
	document, err := checkpoint.LoadTaskTrainingBundle(ctx, path, ocrCheckpointSchema)
	if err != nil {
		return nil, fmt.Errorf("ocr: load checkpoint: %w", err)
	}
	var config RecognizerConfig
	if err := checkpoint.DecodeTaskConfig(document.Config, &config); err != nil {
		return nil, fmt.Errorf("ocr: decode checkpoint config: %w", err)
	}
	if err := validateOCRConfig(config); err != nil {
		return nil, fmt.Errorf("ocr: invalid checkpoint config: %w", err)
	}
	if err := validateOCRCheckpointShape(config, document.Training); err != nil {
		return nil, err
	}
	expected, err := NewRecognizer(config)
	if err != nil {
		return nil, fmt.Errorf("ocr: rebuild checkpoint model: %w", err)
	}
	if err := validateOCRIndex(config, expected.index); err != nil {
		return nil, fmt.Errorf("ocr: rebuild checkpoint alphabet index: %w", err)
	}
	expectedSnapshot := expected.Snapshot()
	if !reflect.DeepEqual(document.Training.Config, expectedSnapshot.Config) {
		return nil, errors.New("ocr: checkpoint training config does not match task config")
	}
	if !reflect.DeepEqual(document.Training.Options, expectedSnapshot.Options) {
		return nil, errors.New("ocr: checkpoint training options do not match task config")
	}
	restored, err := learning.RestoreTrainer(document.Training)
	if err != nil {
		return nil, fmt.Errorf("ocr: restore checkpoint trainer: %w", err)
	}
	expected.trainer = restored
	expected.loadedCheckpointSHA256 = document.ManifestSHA256
	return expected, nil
}

func validateOCRCheckpointShape(config RecognizerConfig, snapshot learning.TrainingSnapshot) error {
	network := snapshot.Config
	inputSize, err := recognizerInputSize(config)
	if err != nil {
		return fmt.Errorf("ocr: checkpoint input dimensions: %w", err)
	}
	nodes, err := profileSum(config.Height, config.Hidden, "network nodes")
	if err != nil {
		return fmt.Errorf("ocr: checkpoint network dimensions: %w", err)
	}
	edges, err := profileProduct(nodes, config.Hidden, "dense edge")
	if err != nil {
		return fmt.Errorf("ocr: checkpoint edge dimensions: %w", err)
	}
	encoderLength, err := profileProduct(inputSize, config.Height, "encoder")
	if err != nil {
		return fmt.Errorf("ocr: checkpoint encoder dimensions: %w", err)
	}
	readoutLength, err := profileProduct(config.Hidden, len(config.Alphabet)+1, "readout")
	if err != nil {
		return fmt.Errorf("ocr: checkpoint readout dimensions: %w", err)
	}
	if inputSize != network.InputSize || config.Height != len(network.InputNodes) ||
		config.Hidden != len(network.ReadoutNodes) || nodes != network.Dynamics.Nodes ||
		len(config.Alphabet)+1 != network.OutputSize {
		return errors.New("ocr: checkpoint task dimensions do not match training config")
	}
	// Reconstruction is dense even if the supplied saved network is sparse.
	// Match every allocated array before running the constructor.
	if len(network.Dynamics.Sources) != edges || len(network.Dynamics.Targets) != edges ||
		len(snapshot.Parameters.Core.Weights) != edges ||
		len(snapshot.Parameters.Encoder) != encoderLength ||
		len(snapshot.Parameters.Readout) != readoutLength {
		return errors.New("ocr: checkpoint dense array sizes do not match task config")
	}
	return nil
}

// Config returns an owned copy of the recognizer's task configuration.
func (m *Recognizer) Config() RecognizerConfig {
	if m == nil {
		return RecognizerConfig{}
	}
	config := m.config
	config.Alphabet = append([]rune(nil), config.Alphabet...)
	return config
}

func ocrContextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("ocr: context must not be nil")
	}
	return ctx.Err()
}

func validateOCRConfig(config RecognizerConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	for i, r := range config.Alphabet {
		if !utf8.ValidRune(r) {
			return fmt.Errorf("ocr: alphabet rune at position %d is not a Unicode scalar value", i)
		}
	}
	return nil
}

func validateOCRIndex(config RecognizerConfig, index map[rune]int) error {
	if len(index) != len(config.Alphabet) {
		return errors.New("ocr: alphabet index does not match the alphabet")
	}
	for i, r := range config.Alphabet {
		if index[r] != i+1 {
			return fmt.Errorf("ocr: alphabet index for %q does not match position %d", r, i)
		}
	}
	return nil
}

func validateOCRTraining(config RecognizerConfig, snapshot learning.TrainingSnapshot) error {
	if snapshot.SchemaVersion != checkpoint.TrainingSchemaVersion {
		return fmt.Errorf("ocr: unsupported training snapshot %q", snapshot.SchemaVersion)
	}
	expected, err := NewRecognizer(config)
	if err != nil {
		return fmt.Errorf("ocr: rebuild expected trainer: %w", err)
	}
	expectedSnapshot := expected.Snapshot()
	if !reflect.DeepEqual(snapshot.Config, expectedSnapshot.Config) {
		return errors.New("ocr: training config does not match recognizer config")
	}
	if !reflect.DeepEqual(snapshot.Options, expectedSnapshot.Options) {
		return errors.New("ocr: training options do not match recognizer config")
	}
	return nil
}
