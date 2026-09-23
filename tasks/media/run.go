package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/learning"
)

// ReportSchemaVersion identifies the JSON schema used by media run reports.
const ReportSchemaVersion = "coimnet-media/v1"

// RunConfig is one complete generation run for one modality.
type RunConfig struct {
	Modality  string          `json:"modality"`
	Generator GeneratorConfig `json:"generator"`
	Holdout   []string        `json:"holdout"`
	Epochs    int             `json:"epochs"`
	Blocks    int             `json:"blocks"`
	Seeds     []uint64        `json:"seeds"`
}

// Validate reports whether the run configuration names valid conditions and limits.
func (c RunConfig) Validate() error {
	_, err := Words(c.Modality)
	if err != nil {
		return fmt.Errorf("media: modality: %w", err)
	}
	if c.Generator.Modality != c.Modality {
		return fmt.Errorf("media: generator modality %q must match modality %q", c.Generator.Modality, c.Modality)
	}
	if err := c.Generator.Validate(); err != nil {
		return fmt.Errorf("media: generator: %w", err)
	}
	if len(c.Holdout) == 0 {
		return fmt.Errorf("media: holdout must contain at least one prompt")
	}
	conditions, err := fixturePrompts(c.Modality)
	if err != nil {
		return fmt.Errorf("media: holdout: %w", err)
	}
	known := make(map[string]struct{}, len(conditions))
	for _, prompt := range conditions {
		known[prompt] = struct{}{}
	}
	held := make(map[string]struct{}, len(c.Holdout))
	for _, prompt := range c.Holdout {
		if _, ok := known[prompt]; !ok {
			return fmt.Errorf("media: holdout prompt %q is not a %s condition", prompt, c.Modality)
		}
		if _, ok := held[prompt]; ok {
			return fmt.Errorf("media: holdout contains duplicate prompt %q", prompt)
		}
		held[prompt] = struct{}{}
	}
	if c.Epochs < 1 || c.Epochs > 10000 {
		return fmt.Errorf("media: epochs %d is outside [1, 10000]", c.Epochs)
	}
	if c.Modality == ModalityImage && c.Blocks != 1 {
		return fmt.Errorf("media: blocks must be 1 for image modality")
	}
	if c.Modality == ModalityAudio && (c.Blocks < 1 || c.Blocks > 8) {
		return fmt.Errorf("media: blocks %d is outside [1, 8] for audio modality", c.Blocks)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("media: seeds must contain at least one seed")
	}
	seeds := make(map[uint64]struct{}, len(c.Seeds))
	for _, seed := range c.Seeds {
		if _, ok := seeds[seed]; ok {
			return fmt.Errorf("media: seeds contains duplicate seed %d", seed)
		}
		seeds[seed] = struct{}{}
	}
	return nil
}

// DefaultRunConfig returns the standard finite-fixture run configuration.
func DefaultRunConfig(modality string) RunConfig {
	config := RunConfig{
		Modality:  modality,
		Generator: DefaultGeneratorConfig(modality),
		Epochs:    60,
		Blocks:    1,
		Seeds:     []uint64{1, 2, 3},
	}
	switch modality {
	case ModalityImage:
		config.Holdout = []string{"blue diagonal"}
	case ModalityAudio:
		config.Holdout = []string{"high pulse"}
		config.Blocks = 3
	}
	return config
}

// ConditionScore records one generated output and its independent classifier result.
type ConditionScore struct {
	Prompt     string  `json:"prompt"`
	Seen       bool    `json:"seen"`
	MSE        float64 `json:"mse"`
	Classified string  `json:"classified"`
	Correct    bool    `json:"correct"`
	Confidence float64 `json:"confidence"`
}

// GroupScore summarises one model before and after training.
type GroupScore struct {
	Name           string           `json:"name"`
	Capacity       CapacitySplit    `json:"capacity"`
	Updates        int              `json:"updates"`
	Before         []ConditionScore `json:"before"`
	After          []ConditionScore `json:"after"`
	SeenCorrect    int              `json:"seen_correct"`
	HeldOutCorrect int              `json:"held_out_correct"`
}

// Temporal reports the audio consistency check across generated blocks.
type Temporal struct {
	Blocks     int `json:"blocks"`
	Prompts    int `json:"prompts"`
	Consistent int `json:"consistent"`
}

// RunSeed contains one seed's core, frozen-core and diagnostic results.
type RunSeed struct {
	Seed           uint64         `json:"seed"`
	Core           GroupScore     `json:"core"`
	FrozenCore     GroupScore     `json:"frozen_core"`
	CoreDisconnect CoreDisconnect `json:"core_disconnect"`
	Temporal       *Temporal      `json:"temporal,omitempty"`
	Failed         bool           `json:"failed"`
	Error          string         `json:"error,omitempty"`
}

// CoreDisconnect reports whether removing all core weights changes output representations.
type CoreDisconnect struct {
	OutputChanged bool    `json:"output_changed"`
	MaxAbsDelta   float64 `json:"max_abs_delta"`
}

// RunReport records a complete deterministic media fixture experiment.
type RunReport struct {
	SchemaVersion string    `json:"schema_version"`
	Config        RunConfig `json:"config"`
	ConfigHash    string    `json:"config_hash"`
	Runs          []RunSeed `json:"runs"`
	Assumptions   []string  `json:"assumptions"`
}

// RunMedia runs every configured seed, including the frozen-core control and diagnostics.
func RunMedia(ctx context.Context, c RunConfig) (RunReport, error) {
	if ctx == nil {
		return RunReport{}, fmt.Errorf("media: context is nil")
	}
	if err := c.Validate(); err != nil {
		return RunReport{}, err
	}
	configJSON, err := json.Marshal(c)
	if err != nil {
		return RunReport{}, fmt.Errorf("media: marshal run config: %w", err)
	}
	hash := sha256.Sum256(configJSON)
	conditions, err := fixturePrompts(c.Modality)
	if err != nil {
		return RunReport{}, err
	}
	report := RunReport{
		SchemaVersion: ReportSchemaVersion,
		Config:        c,
		ConfigHash:    hex.EncodeToString(hash[:]),
		Runs:          make([]RunSeed, len(c.Seeds)),
		Assumptions: []string{
			"Pixel or sample MSE says nothing about perceptual quality.",
			"This proves generation on a finite fixture of nine conditions, not open-ended image or audio generation.",
			"The decoder is a fixed clamp with no parameters; the prompt reaches the output only through the core.",
			"The frozen-core control trains only the encoder and readout, showing what the periphery alone can do with the same budget.",
		},
	}
	for i, seed := range c.Seeds {
		run := RunSeed{Seed: seed}
		if err := runMediaSeed(ctx, c, seed, conditions, &run); err != nil {
			run.Failed = true
			run.Error = err.Error()
		}
		report.Runs[i] = run
	}
	return report, nil
}

// runMediaSeed runs the core model, periphery-only control and diagnostics for one seed.
func runMediaSeed(ctx context.Context, c RunConfig, seed uint64, conditions []string, result *RunSeed) error {
	generatorConfig := c.Generator
	generatorConfig.Seed = seed
	core, err := NewGenerator(generatorConfig)
	if err != nil {
		return fmt.Errorf("create core generator: %w", err)
	}
	frozen, err := frozenCoreGenerator(core)
	if err != nil {
		return fmt.Errorf("create frozen-core control: %w", err)
	}
	seen, heldOut := partitionPrompts(conditions, c.Holdout)
	result.Core = GroupScore{Name: "core", Capacity: core.Capacity()}
	result.FrozenCore = GroupScore{Name: "frozen_core", Capacity: frozen.Capacity()}
	result.Core.Before, err = scoreConditions(ctx, core, conditions, heldOut)
	if err != nil {
		return fmt.Errorf("score core before training: %w", err)
	}
	result.FrozenCore.Before, err = scoreConditions(ctx, frozen, conditions, heldOut)
	if err != nil {
		return fmt.Errorf("score frozen-core before training: %w", err)
	}
	if err := trainMediaConditions(ctx, c.Modality, seen, c.Epochs, core, frozen); err != nil {
		return err
	}
	result.Core.Updates = int(core.Snapshot().Updates)
	result.FrozenCore.Updates = int(frozen.Snapshot().Updates)
	result.Core.After, err = scoreConditions(ctx, core, conditions, heldOut)
	if err != nil {
		return fmt.Errorf("score core after training: %w", err)
	}
	result.FrozenCore.After, err = scoreConditions(ctx, frozen, conditions, heldOut)
	if err != nil {
		return fmt.Errorf("score frozen-core after training: %w", err)
	}
	setGroupCorrect(&result.Core)
	setGroupCorrect(&result.FrozenCore)
	result.CoreDisconnect, err = measureCoreDisconnect(ctx, core, conditions)
	if err != nil {
		return fmt.Errorf("measure core disconnect: %w", err)
	}
	if c.Modality == ModalityAudio {
		result.Temporal, err = measureTemporal(ctx, core, conditions, c.Blocks)
		if err != nil {
			return fmt.Errorf("measure audio temporal consistency: %w", err)
		}
	}
	return nil
}

// mediaTrainer is the training operation shared by generators and test counters.
type mediaTrainer interface {
	Step(context.Context, string, []float64) (learning.StepResult, float64, error)
}

// trainMediaConditions passes only the listed prompts to each model in stable epoch order.
func trainMediaConditions(ctx context.Context, modality string, prompts []string, epochs int, trainers ...mediaTrainer) error {
	for epoch := 0; epoch < epochs; epoch++ {
		for _, prompt := range prompts {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("training canceled: %w", err)
			}
			target, err := renderPrompt(modality, prompt)
			if err != nil {
				return fmt.Errorf("render training target %q: %w", prompt, err)
			}
			for i, trainer := range trainers {
				if trainer == nil {
					return fmt.Errorf("training model %d is nil", i)
				}
				if _, _, err := trainer.Step(ctx, prompt, target); err != nil {
					return fmt.Errorf("train model %d on %q: %w", i, prompt, err)
				}
			}
		}
	}
	return nil
}

// frozenCoreGenerator restores a copy of the initial generator with only its periphery trainable.
func frozenCoreGenerator(source *Generator) (*Generator, error) {
	snapshot := source.Snapshot()
	snapshot.Options.Trainable = learning.Trainable{Encoder: true, Readout: true}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return nil, fmt.Errorf("restore frozen-core trainer: %w", err)
	}
	return &Generator{config: source.config, words: append([]string(nil), source.words...), trainer: trainer}, nil
}

// fixturePrompts returns every prompt in the fixture's stable nested vocabulary order.
func fixturePrompts(modality string) ([]string, error) {
	words, err := Words(modality)
	if err != nil {
		return nil, err
	}
	prompts := make([]string, 0, 9)
	for _, first := range words[:3] {
		for _, second := range words[3:] {
			prompts = append(prompts, first+" "+second)
		}
	}
	return prompts, nil
}

// partitionPrompts separates training conditions from conditions reserved for evaluation.
func partitionPrompts(conditions, holdout []string) ([]string, map[string]struct{}) {
	heldOut := make(map[string]struct{}, len(holdout))
	for _, prompt := range holdout {
		heldOut[prompt] = struct{}{}
	}
	seen := make([]string, 0, len(conditions)-len(holdout))
	for _, prompt := range conditions {
		if _, held := heldOut[prompt]; !held {
			seen = append(seen, prompt)
		}
	}
	return seen, heldOut
}

// renderPrompt creates the fixed target for one supported fixture prompt.
func renderPrompt(modality, prompt string) ([]float64, error) {
	if modality == ModalityImage {
		condition, err := ParseImagePrompt(prompt)
		if err != nil {
			return nil, err
		}
		return RenderImage(condition)
	}
	condition, err := ParseAudioPrompt(prompt)
	if err != nil {
		return nil, err
	}
	return RenderAudio(condition)
}

// scoreConditions generates and independently classifies every prompt in fixture order.
func scoreConditions(ctx context.Context, generator *Generator, conditions []string, heldOut map[string]struct{}) ([]ConditionScore, error) {
	scores := make([]ConditionScore, 0, len(conditions))
	for _, prompt := range conditions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("scoring canceled: %w", err)
		}
		generated, err := generator.Generate(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("generate %q: %w", prompt, err)
		}
		target, err := renderPrompt(generator.config.Modality, prompt)
		if err != nil {
			return nil, fmt.Errorf("render target %q: %w", prompt, err)
		}
		mse, err := meanSquaredError(generated, target)
		if err != nil {
			return nil, fmt.Errorf("score %q: %w", prompt, err)
		}
		classified, confidence, err := classifyPrompt(generator.config.Modality, generated)
		if err != nil {
			return nil, fmt.Errorf("classify %q: %w", prompt, err)
		}
		if confidence == 0 {
			classified = ""
		}
		_, held := heldOut[prompt]
		scores = append(scores, ConditionScore{
			Prompt: prompt, Seen: !held, MSE: mse, Classified: classified,
			Correct: classified == prompt, Confidence: confidence,
		})
	}
	return scores, nil
}

// classifyPrompt returns the independent classifier's prompt and confidence.
func classifyPrompt(modality string, generated []float64) (string, float64, error) {
	if modality == ModalityImage {
		condition, confidence, err := ClassifyImage(generated)
		if err != nil || confidence == 0 {
			return "", confidence, err
		}
		return condition.Prompt(), confidence, nil
	}
	condition, confidence, err := ClassifyAudio(generated)
	if err != nil || confidence == 0 {
		return "", confidence, err
	}
	return condition.Prompt(), confidence, nil
}

// meanSquaredError computes the finite element-wise mean squared error.
func meanSquaredError(generated, target []float64) (float64, error) {
	if len(generated) != len(target) || len(target) == 0 {
		return 0, fmt.Errorf("output length %d does not match target length %d", len(generated), len(target))
	}
	total := 0.0
	for i, value := range generated {
		delta := value - target[i]
		term := delta * delta
		if math.IsNaN(term) || math.IsInf(term, 0) {
			return 0, fmt.Errorf("squared error at value %d is non-finite", i)
		}
		total += term / float64(len(target))
	}
	if math.IsNaN(total) || math.IsInf(total, 0) {
		return 0, fmt.Errorf("mean squared error is non-finite")
	}
	return total, nil
}

// setGroupCorrect counts seen and held-out correct classifications after training.
func setGroupCorrect(group *GroupScore) {
	group.SeenCorrect, group.HeldOutCorrect = 0, 0
	for _, score := range group.After {
		if !score.Correct {
			continue
		}
		if score.Seen {
			group.SeenCorrect++
		} else {
			group.HeldOutCorrect++
		}
	}
}

// measureCoreDisconnect compares trained representations with all core weights removed.
func measureCoreDisconnect(ctx context.Context, trained *Generator, conditions []string) (CoreDisconnect, error) {
	disconnected := &Generator{config: trained.config, words: append([]string(nil), trained.words...)}
	snapshot := trained.Snapshot()
	for i := range snapshot.Parameters.Core.Weights {
		snapshot.Parameters.Core.Weights[i] = 0
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return CoreDisconnect{}, fmt.Errorf("restore disconnected trainer: %w", err)
	}
	disconnected.trainer = trainer
	result := CoreDisconnect{}
	for _, prompt := range conditions {
		before, err := trained.Representation(ctx, prompt)
		if err != nil {
			return CoreDisconnect{}, fmt.Errorf("read trained representation %q: %w", prompt, err)
		}
		after, err := disconnected.Representation(ctx, prompt)
		if err != nil {
			return CoreDisconnect{}, fmt.Errorf("read disconnected representation %q: %w", prompt, err)
		}
		for i := range before {
			delta := math.Abs(before[i] - after[i])
			if math.IsNaN(delta) || math.IsInf(delta, 0) {
				return CoreDisconnect{}, fmt.Errorf("representation delta for %q at value %d is non-finite", prompt, i)
			}
			if delta > result.MaxAbsDelta {
				result.MaxAbsDelta = delta
			}
		}
	}
	result.OutputChanged = result.MaxAbsDelta > 0
	return result, nil
}

// measureTemporal classifies the prompt block and each following repeated-word block.
func measureTemporal(ctx context.Context, generator *Generator, conditions []string, blocks int) (*Temporal, error) {
	result := &Temporal{Blocks: blocks, Prompts: len(conditions)}
	for _, prompt := range conditions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("temporal scoring canceled: %w", err)
		}
		first, second, err := generator.promptIndices(prompt)
		if err != nil {
			return nil, fmt.Errorf("parse temporal prompt %q: %w", prompt, err)
		}
		input := make([][]float64, (blocks+1)*generator.config.Settle)
		for row := range input {
			input[row] = make([]float64, len(generator.words))
			word := first
			if row >= generator.config.Settle {
				word = second
			}
			input[row][word] = 1
		}
		outputs, err := generator.trainer.PredictAll(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("predict temporal prompt %q: %w", prompt, err)
		}
		consistent := true
		for block := 0; block < blocks; block++ {
			row := (block+2)*generator.config.Settle - 1
			decoded, err := Decode(generator.config.Modality, DecoderInput{Representation: outputs[row]})
			if err != nil {
				return nil, fmt.Errorf("decode temporal block %d for %q: %w", block, prompt, err)
			}
			classified, confidence, err := classifyPrompt(generator.config.Modality, decoded)
			if err != nil {
				return nil, fmt.Errorf("classify temporal block %d for %q: %w", block, prompt, err)
			}
			if confidence == 0 || classified != prompt {
				consistent = false
			}
		}
		if consistent {
			result.Consistent++
		}
	}
	return result, nil
}
