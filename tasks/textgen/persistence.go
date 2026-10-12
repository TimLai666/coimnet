package textgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

const textgenCheckpointSchema = "coimnet-textgen-checkpoint/v1"

// LoadedCheckpointSHA256 identifies the manifest verified when loading this
// model. New models return an empty string; later training retains the origin.
func (m *Model) LoadedCheckpointSHA256() string {
	if m == nil {
		return ""
	}
	return m.loadedCheckpointSHA256
}

type modelCheckpointConfig struct {
	Model      ModelConfig      `json:"model"`
	Vocabulary tokenizer.Config `json:"vocabulary"`
	VocabHash  string           `json:"vocab_hash"`
}

// Save publishes a complete task model and optimizer checkpoint without replacing an existing path.
func (m *Model) Save(ctx context.Context, path string) error {
	if m == nil || m.trainer == nil {
		return errors.New("textgen: nil model")
	}
	if err := textgenContextError(ctx); err != nil {
		return err
	}
	if path == "" {
		return errors.New("textgen: checkpoint path must not be empty")
	}
	vocab, err := reproducibleByteVocab(m.vocab)
	if err != nil {
		return err
	}
	if err := m.config.Validate(); err != nil {
		return fmt.Errorf("textgen: invalid model config: %w", err)
	}
	snapshot := m.trainer.Snapshot()
	if err := validateTextgenTraining(m.config, vocab, snapshot); err != nil {
		return err
	}
	config := modelCheckpointConfig{Model: m.config, Vocabulary: vocab.Config(), VocabHash: vocab.Hash()}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("textgen: encode checkpoint config: %w", err)
	}
	return checkpoint.SaveTaskTrainingBundle(ctx, path, textgenCheckpointSchema, configJSON, snapshot)
}

// LoadModel restores the task configuration and complete optimizer state from a verified checkpoint.
func LoadModel(ctx context.Context, path string) (*Model, error) {
	if err := textgenContextError(ctx); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("textgen: checkpoint path must not be empty")
	}
	document, err := checkpoint.LoadTaskTrainingBundle(ctx, path, textgenCheckpointSchema)
	if err != nil {
		return nil, fmt.Errorf("textgen: load checkpoint: %w", err)
	}
	var config modelCheckpointConfig
	if err := checkpoint.DecodeTaskConfig(document.Config, &config); err != nil {
		return nil, fmt.Errorf("textgen: decode checkpoint config: %w", err)
	}
	if err := config.Model.Validate(); err != nil {
		return nil, fmt.Errorf("textgen: invalid checkpoint model config: %w", err)
	}
	// Check size before vocabulary maps and dense model arrays are allocated.
	network := document.Training.Config
	if network.InputSize < 256 || len(config.Vocabulary.Specials) != network.InputSize-256 ||
		network.OutputSize != network.InputSize ||
		config.Model.Embed != len(network.InputNodes) ||
		config.Model.Hidden != len(network.ReadoutNodes) ||
		config.Model.Embed != network.Dynamics.Nodes-config.Model.Hidden {
		return nil, errors.New("textgen: checkpoint task dimensions do not match training config")
	}
	vocab, err := tokenizer.New(config.Vocabulary)
	if err != nil {
		return nil, fmt.Errorf("textgen: rebuild checkpoint vocabulary: %w", err)
	}
	if config.VocabHash != vocab.Hash() {
		return nil, errors.New("textgen: checkpoint vocabulary hash does not match its config")
	}
	expected, err := NewModel(vocab, config.Model)
	if err != nil {
		return nil, fmt.Errorf("textgen: rebuild checkpoint model: %w", err)
	}
	expectedSnapshot := expected.Snapshot()
	if !reflect.DeepEqual(document.Training.Config, expectedSnapshot.Config) {
		return nil, errors.New("textgen: checkpoint training config does not match task config")
	}
	if !reflect.DeepEqual(document.Training.Options, expectedSnapshot.Options) {
		return nil, errors.New("textgen: checkpoint training options do not match task config")
	}
	restored, err := learning.RestoreTrainer(document.Training)
	if err != nil {
		return nil, fmt.Errorf("textgen: restore checkpoint trainer: %w", err)
	}
	expected.trainer = restored
	expected.loadedCheckpointSHA256 = document.ManifestSHA256
	return expected, nil
}

func textgenContextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("textgen: context must not be nil")
	}
	return ctx.Err()
}

func reproducibleByteVocab(vocab tokenizer.Vocabulary) (*tokenizer.ByteVocab, error) {
	byteVocab, ok := vocab.(*tokenizer.ByteVocab)
	if !ok || byteVocab == nil {
		return nil, errors.New("textgen: checkpoint requires an actual *tokenizer.ByteVocab")
	}
	config := byteVocab.Config()
	rebuilt, err := tokenizer.New(config)
	if err != nil {
		return nil, fmt.Errorf("textgen: invalid vocabulary config: %w", err)
	}
	if byteVocab.Hash() == "" || byteVocab.Hash() != rebuilt.Hash() {
		return nil, errors.New("textgen: vocabulary config and hash do not match")
	}
	return byteVocab, nil
}

func validateTextgenTraining(config ModelConfig, vocab *tokenizer.ByteVocab, snapshot learning.TrainingSnapshot) error {
	if snapshot.SchemaVersion != checkpoint.TrainingSchemaVersion {
		return fmt.Errorf("textgen: unsupported training snapshot %q", snapshot.SchemaVersion)
	}
	expected, err := NewModel(vocab, config)
	if err != nil {
		return fmt.Errorf("textgen: rebuild expected trainer: %w", err)
	}
	expectedSnapshot := expected.Snapshot()
	if !reflect.DeepEqual(snapshot.Config, expectedSnapshot.Config) {
		return errors.New("textgen: training config does not match model config")
	}
	if !reflect.DeepEqual(snapshot.Options, expectedSnapshot.Options) {
		return errors.New("textgen: training options do not match model config")
	}
	return nil
}
