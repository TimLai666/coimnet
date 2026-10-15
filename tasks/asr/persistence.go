package asr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
)

const asrCheckpointSchema = "coimnet-asr-checkpoint/v1"

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
		return errors.New("asr: nil recognizer")
	}
	if err := asrContextError(ctx); err != nil {
		return err
	}
	if path == "" {
		return errors.New("asr: checkpoint path must not be empty")
	}
	config := m.config
	config.Alphabet = append([]rune(nil), config.Alphabet...)
	if err := validateASRConfig(config); err != nil {
		return err
	}
	if err := validateASRIndex(config, m.index); err != nil {
		return err
	}
	snapshot := m.trainer.Snapshot()
	if err := validateASRTraining(config, snapshot); err != nil {
		return err
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("asr: encode checkpoint config: %w", err)
	}
	return checkpoint.SaveTaskTrainingBundle(ctx, path, asrCheckpointSchema, configJSON, snapshot)
}

// LoadRecognizer restores the task configuration and complete optimizer state from a verified checkpoint.
func LoadRecognizer(ctx context.Context, path string) (*Recognizer, error) {
	if err := asrContextError(ctx); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("asr: checkpoint path must not be empty")
	}
	document, err := checkpoint.LoadTaskTrainingBundle(ctx, path, asrCheckpointSchema)
	if err != nil {
		return nil, fmt.Errorf("asr: load checkpoint: %w", err)
	}
	var config RecognizerConfig
	if err := checkpoint.DecodeTaskConfig(document.Config, &config); err != nil {
		return nil, fmt.Errorf("asr: decode checkpoint config: %w", err)
	}
	if err := validateASRConfig(config); err != nil {
		return nil, fmt.Errorf("asr: invalid checkpoint config: %w", err)
	}
	if err := validateASRCheckpointShape(config, document.Training); err != nil {
		return nil, err
	}
	expected, err := NewRecognizer(config)
	if err != nil {
		return nil, fmt.Errorf("asr: rebuild checkpoint model: %w", err)
	}
	if err := validateASRIndex(config, expected.index); err != nil {
		return nil, fmt.Errorf("asr: rebuild checkpoint alphabet index: %w", err)
	}
	expectedSnapshot := expected.Snapshot()
	if !reflect.DeepEqual(document.Training.Config, expectedSnapshot.Config) {
		return nil, errors.New("asr: checkpoint training config does not match task config")
	}
	if !reflect.DeepEqual(document.Training.Options, expectedSnapshot.Options) {
		return nil, errors.New("asr: checkpoint training options do not match task config")
	}
	restored, err := learning.RestoreTrainer(document.Training)
	if err != nil {
		return nil, fmt.Errorf("asr: restore checkpoint trainer: %w", err)
	}
	expected.trainer = restored
	expected.loadedCheckpointSHA256 = document.ManifestSHA256
	return expected, nil
}

func validateASRCheckpointShape(config RecognizerConfig, snapshot learning.TrainingSnapshot) error {
	network := snapshot.Config
	if config.FrontEnd.Mels != network.InputSize || config.FrontEnd.Mels != len(network.InputNodes) ||
		config.Hidden != len(network.ReadoutNodes) ||
		config.Hidden != network.Dynamics.Nodes-config.FrontEnd.Mels ||
		len(config.Alphabet) != network.OutputSize-1 {
		return errors.New("asr: checkpoint task dimensions do not match training config")
	}
	// Reconstruction is dense even if the supplied saved network is sparse.
	// Match every allocated array before running the constructor.
	mels := config.FrontEnd.Mels
	maxInt := int(^uint(0) >> 1)
	if config.Hidden <= 0 || mels <= 0 || network.Dynamics.Nodes > maxInt/config.Hidden ||
		mels > maxInt/mels || network.OutputSize > maxInt/config.Hidden {
		return errors.New("asr: checkpoint reconstruction dimensions overflow")
	}
	edges := network.Dynamics.Nodes * config.Hidden
	if len(network.Dynamics.Sources) != edges || len(network.Dynamics.Targets) != edges ||
		len(snapshot.Parameters.Core.Weights) != edges ||
		len(snapshot.Parameters.Encoder) != mels*mels ||
		len(snapshot.Parameters.Readout) != config.Hidden*network.OutputSize {
		return errors.New("asr: checkpoint dense array sizes do not match task config")
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

func asrContextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("asr: context must not be nil")
	}
	return ctx.Err()
}

func validateASRConfig(config RecognizerConfig) error {
	return config.Validate()
}

func validateASRIndex(config RecognizerConfig, index map[rune]int) error {
	if len(index) != len(config.Alphabet) {
		return errors.New("asr: alphabet index does not match the alphabet")
	}
	for i, r := range config.Alphabet {
		if index[r] != i+1 {
			return fmt.Errorf("asr: alphabet index for %q does not match position %d", r, i)
		}
	}
	return nil
}

func validateASRTraining(config RecognizerConfig, snapshot learning.TrainingSnapshot) error {
	if snapshot.SchemaVersion != checkpoint.TrainingSchemaVersion {
		return fmt.Errorf("asr: unsupported training snapshot %q", snapshot.SchemaVersion)
	}
	expected, err := NewRecognizer(config)
	if err != nil {
		return fmt.Errorf("asr: rebuild expected trainer: %w", err)
	}
	expectedSnapshot := expected.Snapshot()
	if !reflect.DeepEqual(snapshot.Config, expectedSnapshot.Config) {
		return errors.New("asr: training config does not match recognizer config")
	}
	if !reflect.DeepEqual(snapshot.Options, expectedSnapshot.Options) {
		return errors.New("asr: training options do not match recognizer config")
	}
	return nil
}
