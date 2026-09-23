package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RolesSchemaVersion names the roles report format.
const RolesSchemaVersion = "coimnet-roles/v1"

// RolesConfig compares the three image-generation modes on the image fixture with one shared core configuration.
type RolesConfig struct {
	Generator    GeneratorConfig `json:"generator"`      // Modality must be ModalityImage; Seed replaced per seed
	Holdout      []string        `json:"holdout"`        // >= 1 distinct image prompts that never train
	Epochs       int             `json:"epochs"`         // 1..10000 passes over the seen prompts for every mode
	Seeds        []uint64        `json:"seeds"`          // >= 1, distinct
	MaxToolCalls int             `json:"max_tool_calls"` // ToolPolicy budget per seed, 1..10000
}

// Validate reports whether c defines a valid image role comparison.
func (c RolesConfig) Validate() error {
	if c.Generator.Modality != ModalityImage {
		return fmt.Errorf("media: roles generator modality must be %q", ModalityImage)
	}
	if err := c.Generator.Validate(); err != nil {
		return fmt.Errorf("media: roles generator: %w", err)
	}
	conditions, err := fixturePrompts(ModalityImage)
	if err != nil {
		return fmt.Errorf("media: roles fixture: %w", err)
	}
	if len(c.Holdout) == 0 {
		return fmt.Errorf("media: roles holdout must contain at least one prompt")
	}
	known := make(map[string]struct{}, len(conditions))
	for _, prompt := range conditions {
		known[prompt] = struct{}{}
	}
	heldOut := make(map[string]struct{}, len(c.Holdout))
	for _, prompt := range c.Holdout {
		if _, ok := known[prompt]; !ok {
			return fmt.Errorf("media: roles holdout prompt %q is not an image condition", prompt)
		}
		if _, ok := heldOut[prompt]; ok {
			return fmt.Errorf("media: roles holdout contains duplicate prompt %q", prompt)
		}
		heldOut[prompt] = struct{}{}
	}
	if c.Epochs < 1 || c.Epochs > 10000 {
		return fmt.Errorf("media: roles epochs %d is outside [1, 10000]", c.Epochs)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("media: roles seeds must contain at least one seed")
	}
	seeds := make(map[uint64]struct{}, len(c.Seeds))
	for _, seed := range c.Seeds {
		if _, ok := seeds[seed]; ok {
			return fmt.Errorf("media: roles seeds contains duplicate seed %d", seed)
		}
		seeds[seed] = struct{}{}
	}
	if err := (ToolPolicy{MaxCalls: c.MaxToolCalls}).Validate(); err != nil {
		return fmt.Errorf("media: roles tool policy: %w", err)
	}
	return nil
}

// DefaultRolesConfig returns the standard image fixture comparison configuration.
func DefaultRolesConfig() RolesConfig {
	return RolesConfig{
		Generator:    DefaultGeneratorConfig(ModalityImage),
		Holdout:      []string{"blue diagonal"},
		Epochs:       60,
		Seeds:        []uint64{1, 2, 3},
		MaxToolCalls: 9,
	}
}

// RoleCapability counts correct outputs over the seen and the held-out prompts under one named measure.
type RoleCapability struct {
	Measure        string `json:"measure"`
	SeenCorrect    int    `json:"seen_correct"`
	HeldOutCorrect int    `json:"held_out_correct"`
}

// RoleScore is one prompt's final image after training, with its producing role and optional structured request.
type RoleScore struct {
	Prompt  string       `json:"prompt"`
	Seen    bool         `json:"seen"`
	Request *ToolRequest `json:"request,omitempty"`
	Judged  string       `json:"judged"`
	Correct bool         `json:"correct"` // Judged == Prompt
	Role    string       `json:"role"`    // "core", "fixed_decoder" or "external_tool"
}

// RoleMode is one generation mode for one seed.
type RoleMode struct {
	Mode           string          `json:"mode"` // "core_generated", "fixed_decoder" or "external_tool"
	Capacity       CapacitySplit   `json:"capacity"`
	Updates        int             `json:"updates"`                // training steps actually taken by this mode's core
	DecoderHash    string          `json:"decoder_hash,omitempty"` // fixed_decoder only: FixedDecoder.Hash()
	Tool           string          `json:"tool,omitempty"`         // external_tool only: ExternalImageTool
	CoreBefore     RoleCapability  `json:"core_before"`            // CoreCapability's measure on the untrained core
	CoreCapability RoleCapability  `json:"core_capability"`        // what the core itself produced, after training
	ToolResult     *RoleCapability `json:"tool_result,omitempty"`  // external_tool only; never added into CoreCapability
	ToolCounts     *ToolCounts     `json:"tool_counts,omitempty"`  // external_tool only
	Scores         []RoleScore     `json:"scores"`                 // all nine prompts in fixturePrompts order, after training
}

// DecoderAlone is what the frozen decoder reproduces by itself from the targets' own latents.
type DecoderAlone struct {
	ReconstructionMSE float64 `json:"reconstruction_mse"`
	Correct           int     `json:"correct"` // targets whose Decode(Encode(RenderImage)) ClassifyImage judges correctly, of 9
}

// RoleReport is one seed of the comparison; the three modes and their roles are named separately.
type RoleReport struct {
	Seed          uint64       `json:"seed"`
	CoreGenerated RoleMode     `json:"core_generated"`
	FixedDecoder  RoleMode     `json:"fixed_decoder"`
	ExternalTool  RoleMode     `json:"external_tool"`
	DecoderAlone  DecoderAlone `json:"decoder_alone"`
	LatentStats   LatentStats  `json:"latent_stats"` // the fixed-decoder core's latents against the decoder's own, after training
	Failed        bool         `json:"failed"`
	Error         string       `json:"error,omitempty"`
}

// RolesReport is the full comparison.
type RolesReport struct {
	SchemaVersion string       `json:"schema_version"` // RolesSchemaVersion
	Config        RolesConfig  `json:"config"`
	ConfigHash    string       `json:"config_hash"` // same algorithm as RunMedia's
	Runs          []RoleReport `json:"runs"`
	Assumptions   []string     `json:"assumptions"`
}

// imageRenderer renders a structured request and reports external-tool accounting.
type imageRenderer interface {
	Render(context.Context, ToolRequest) (ToolOutput, error)
	Counts() ToolCounts
}

// RunRoles trains, for every seed, three cores built from c.Generator with that seed on the seen prompts only, c.Epochs
// passes each, then scores all nine prompts. core_generated trains NewGenerator toward RenderImage like RunMedia's core;
// its pixels come from Generate (role "core", measure "judged_pixels"). fixed_decoder trains NewLatentGenerator toward
// the latents of DefaultFixedDecoder, which is built once per call, shared by every seed and never trained; its pixels
// are that decoder's Decode of the core's latent (role "fixed_decoder", measure "judged_pixels_through_fixed_decoder",
// with DecoderHash, DecoderAlone and LatentStats). external_tool trains NewRequestHead; after training one ExternalTool
// with ToolPolicy{MaxCalls: c.MaxToolCalls} renders each prompt's request (role "external_tool"); CoreCapability counts
// only requests equal to the prompt's own two words ("correct_requests") and ToolResult counts the judged tool pixels
// ("judged_tool_pixels") without ever adding them to CoreCapability. A render error such as an exhausted budget leaves
// that score's Judged empty and shows up in ToolCounts. CoreBefore uses the same measure before any training step, and
// no tool call is spent before training. A failing seed records Failed and Error and the next seed still runs.
func RunRoles(ctx context.Context, c RolesConfig) (RolesReport, error) {
	return runRoles(ctx, c, func(policy ToolPolicy) (imageRenderer, error) {
		return NewExternalTool(policy)
	})
}

// runRoles runs role comparisons using newRenderer to construct one renderer per seed.
func runRoles(ctx context.Context, c RolesConfig, newRenderer func(ToolPolicy) (imageRenderer, error)) (RolesReport, error) {
	if ctx == nil {
		return RolesReport{}, fmt.Errorf("media: roles context is nil")
	}
	if err := c.Validate(); err != nil {
		return RolesReport{}, err
	}
	if newRenderer == nil {
		return RolesReport{}, fmt.Errorf("media: roles renderer factory is nil")
	}
	configJSON, err := json.Marshal(c)
	if err != nil {
		return RolesReport{}, fmt.Errorf("media: marshal roles config: %w", err)
	}
	hash := sha256.Sum256(configJSON)
	conditions, err := fixturePrompts(ModalityImage)
	if err != nil {
		return RolesReport{}, err
	}
	decoder, err := DefaultFixedDecoder()
	if err != nil {
		return RolesReport{}, fmt.Errorf("media: create fixed decoder: %w", err)
	}
	decoderAlone, err := scoreDecoderAlone(decoder)
	if err != nil {
		return RolesReport{}, fmt.Errorf("media: score decoder alone: %w", err)
	}
	report := RolesReport{
		SchemaVersion: RolesSchemaVersion,
		Config:        c,
		ConfigHash:    hex.EncodeToString(hash[:]),
		Runs:          make([]RoleReport, len(c.Seeds)),
		Assumptions: []string{
			"The fixture has nine images of three colours and three shapes; it proves the three modes and their bookkeeping, not open-ended image generation.",
			"In fixed_decoder mode the judged pixels depend on the core's latent and on the frozen decoder named by decoder_hash; decoder_alone shows what that decoder reproduces by itself.",
			"In external_tool mode the core's capability is only its structured request; the tool's pixels are reported as tool_result and never counted as core capability.",
			"The external tool here is a local stand-in that renders the requested fixture image; no network or third-party model is called.",
			"The fixed decoder is an image autoencoder; time alignment does not apply to single images and is not measured here.",
		},
	}
	for i, seed := range c.Seeds {
		run := RoleReport{Seed: seed, DecoderAlone: decoderAlone}
		if err := runRolesSeed(ctx, c, seed, conditions, decoder, newRenderer, &run); err != nil {
			run.Failed = true
			run.Error = err.Error()
		}
		report.Runs[i] = run
	}
	return report, nil
}

// runRolesSeed trains and scores each mode for one seed using only the seen prompts.
func runRolesSeed(ctx context.Context, c RolesConfig, seed uint64, conditions []string, decoder *FixedDecoder, newRenderer func(ToolPolicy) (imageRenderer, error), result *RoleReport) error {
	seen, heldOut := partitionPrompts(conditions, c.Holdout)
	generatorConfig := c.Generator
	generatorConfig.Seed = seed
	core, err := NewGenerator(generatorConfig)
	if err != nil {
		return fmt.Errorf("create core generator: %w", err)
	}
	latent, err := NewLatentGenerator(generatorConfig, decoder)
	if err != nil {
		return fmt.Errorf("create fixed-decoder generator: %w", err)
	}
	requestHead, err := NewRequestHead(generatorConfig)
	if err != nil {
		return fmt.Errorf("create external-tool request head: %w", err)
	}
	renderer, err := newRenderer(ToolPolicy{MaxCalls: c.MaxToolCalls})
	if err != nil {
		return fmt.Errorf("create external image renderer: %w", err)
	}

	result.CoreGenerated = RoleMode{Mode: "core_generated", Capacity: core.Capacity()}
	result.FixedDecoder = RoleMode{Mode: "fixed_decoder", Capacity: latent.Capacity(), DecoderHash: decoder.Hash()}
	result.ExternalTool = RoleMode{Mode: "external_tool", Capacity: requestHead.Capacity(), Tool: ExternalImageTool}
	result.CoreGenerated.CoreBefore, _, err = scoreCore(ctx, conditions, heldOut, "judged_pixels", "core", core.Generate)
	if err != nil {
		return fmt.Errorf("score core before training: %w", err)
	}
	result.FixedDecoder.CoreBefore, _, err = scoreCore(ctx, conditions, heldOut, "judged_pixels_through_fixed_decoder", "fixed_decoder", latent.Generate)
	if err != nil {
		return fmt.Errorf("score fixed decoder before training: %w", err)
	}
	result.ExternalTool.CoreBefore, err = scoreRequests(ctx, conditions, heldOut, requestHead)
	if err != nil {
		return fmt.Errorf("score tool requests before training: %w", err)
	}

	result.CoreGenerated.Updates, err = trainRoleConditions(ctx, seen, c.Epochs, func(ctx context.Context, prompt string) error {
		target, err := renderImagePrompt(prompt)
		if err != nil {
			return err
		}
		_, _, err = core.Step(ctx, prompt, target)
		return err
	})
	if err != nil {
		return fmt.Errorf("train core generator: %w", err)
	}
	result.FixedDecoder.Updates, err = trainRoleConditions(ctx, seen, c.Epochs, func(ctx context.Context, prompt string) error {
		_, _, err := latent.Step(ctx, prompt)
		return err
	})
	if err != nil {
		return fmt.Errorf("train fixed-decoder generator: %w", err)
	}
	result.ExternalTool.Updates, err = trainRoleConditions(ctx, seen, c.Epochs, func(ctx context.Context, prompt string) error {
		_, _, err := requestHead.Step(ctx, prompt)
		return err
	})
	if err != nil {
		return fmt.Errorf("train external-tool request head: %w", err)
	}

	result.CoreGenerated.CoreCapability, result.CoreGenerated.Scores, err = scoreCore(ctx, conditions, heldOut, "judged_pixels", "core", core.Generate)
	if err != nil {
		return fmt.Errorf("score core after training: %w", err)
	}
	result.FixedDecoder.CoreCapability, result.FixedDecoder.Scores, err = scoreCore(ctx, conditions, heldOut, "judged_pixels_through_fixed_decoder", "fixed_decoder", latent.Generate)
	if err != nil {
		return fmt.Errorf("score fixed decoder after training: %w", err)
	}
	result.ExternalTool.CoreCapability, result.ExternalTool.Scores, result.ExternalTool.ToolResult, result.ExternalTool.ToolCounts, err = scoreExternalTool(ctx, conditions, heldOut, requestHead, renderer)
	if err != nil {
		return fmt.Errorf("score external tool after training: %w", err)
	}
	result.LatentStats, err = latent.Stats(ctx)
	if err != nil {
		return fmt.Errorf("measure fixed-decoder latent distribution: %w", err)
	}
	return nil
}

// renderImagePrompt parses and renders one image fixture prompt.
func renderImagePrompt(prompt string) ([]float64, error) {
	condition, err := ParseImagePrompt(prompt)
	if err != nil {
		return nil, err
	}
	return RenderImage(condition)
}

// trainRoleConditions trains a mode once per seen prompt for each epoch and returns successful updates.
func trainRoleConditions(ctx context.Context, prompts []string, epochs int, step func(context.Context, string) error) (int, error) {
	updates := 0
	for epoch := 0; epoch < epochs; epoch++ {
		for _, prompt := range prompts {
			if err := ctx.Err(); err != nil {
				return updates, fmt.Errorf("training canceled: %w", err)
			}
			if err := step(ctx, prompt); err != nil {
				return updates, fmt.Errorf("train on %q: %w", prompt, err)
			}
			updates++
		}
	}
	return updates, nil
}

// scoreCore generates and classifies every fixture prompt for one core-producing mode.
func scoreCore(ctx context.Context, conditions []string, heldOut map[string]struct{}, measure, role string, generate func(context.Context, string) ([]float64, error)) (RoleCapability, []RoleScore, error) {
	capability := RoleCapability{Measure: measure}
	scores := make([]RoleScore, 0, len(conditions))
	for _, prompt := range conditions {
		if err := ctx.Err(); err != nil {
			return RoleCapability{}, nil, fmt.Errorf("scoring canceled: %w", err)
		}
		pixels, err := generate(ctx, prompt)
		if err != nil {
			return RoleCapability{}, nil, fmt.Errorf("generate %q: %w", prompt, err)
		}
		judged, _, err := classifyPrompt(ModalityImage, pixels)
		if err != nil {
			return RoleCapability{}, nil, fmt.Errorf("classify %q: %w", prompt, err)
		}
		_, held := heldOut[prompt]
		score := RoleScore{Prompt: prompt, Seen: !held, Judged: judged, Correct: judged == prompt, Role: role}
		scores = append(scores, score)
		countRoleCapability(&capability, score)
	}
	return capability, scores, nil
}

// scoreRequests evaluates the structured requests emitted by a request head.
func scoreRequests(ctx context.Context, conditions []string, heldOut map[string]struct{}, head *RequestHead) (RoleCapability, error) {
	capability := RoleCapability{Measure: "correct_requests"}
	for _, prompt := range conditions {
		if err := ctx.Err(); err != nil {
			return RoleCapability{}, fmt.Errorf("request scoring canceled: %w", err)
		}
		request, err := head.Request(ctx, prompt)
		if err != nil {
			return RoleCapability{}, fmt.Errorf("request %q: %w", prompt, err)
		}
		condition, err := ParseImagePrompt(prompt)
		if err != nil {
			return RoleCapability{}, err
		}
		score := RoleScore{Prompt: prompt, Seen: isSeen(heldOut, prompt), Correct: request.Colour == condition.Colour && request.Shape == condition.Shape}
		countRoleCapability(&capability, score)
	}
	return capability, nil
}

// scoreExternalTool records the request capability separately from judged tool pixels.
func scoreExternalTool(ctx context.Context, conditions []string, heldOut map[string]struct{}, head *RequestHead, renderer imageRenderer) (RoleCapability, []RoleScore, *RoleCapability, *ToolCounts, error) {
	core := RoleCapability{Measure: "correct_requests"}
	tool := RoleCapability{Measure: "judged_tool_pixels"}
	scores := make([]RoleScore, 0, len(conditions))
	for _, prompt := range conditions {
		if err := ctx.Err(); err != nil {
			return RoleCapability{}, nil, nil, nil, fmt.Errorf("external-tool scoring canceled: %w", err)
		}
		request, err := head.Request(ctx, prompt)
		if err != nil {
			return RoleCapability{}, nil, nil, nil, fmt.Errorf("request %q: %w", prompt, err)
		}
		condition, err := ParseImagePrompt(prompt)
		if err != nil {
			return RoleCapability{}, nil, nil, nil, err
		}
		_, held := heldOut[prompt]
		score := RoleScore{Prompt: prompt, Seen: !held, Request: &request, Role: "external_tool"}
		if request.Colour == condition.Colour && request.Shape == condition.Shape {
			if score.Seen {
				core.SeenCorrect++
			} else {
				core.HeldOutCorrect++
			}
		}
		output, renderErr := renderer.Render(ctx, request)
		if renderErr == nil {
			judged, _, classifyErr := classifyPrompt(ModalityImage, output.Pixels)
			if classifyErr != nil {
				return RoleCapability{}, nil, nil, nil, fmt.Errorf("classify rendered image for %q: %w", prompt, classifyErr)
			}
			score.Judged = judged
			score.Correct = judged == prompt
			countRoleCapability(&tool, score)
		}
		scores = append(scores, score)
	}
	counts := renderer.Counts()
	return core, scores, &tool, &counts, nil
}

// scoreDecoderAlone classifies each target after it passes through the frozen decoder.
func scoreDecoderAlone(decoder *FixedDecoder) (DecoderAlone, error) {
	mse, err := decoder.ReconstructionMSE()
	if err != nil {
		return DecoderAlone{}, err
	}
	conditions, err := fixturePrompts(ModalityImage)
	if err != nil {
		return DecoderAlone{}, err
	}
	result := DecoderAlone{ReconstructionMSE: mse}
	for _, prompt := range conditions {
		target, err := renderImagePrompt(prompt)
		if err != nil {
			return DecoderAlone{}, err
		}
		latent, err := decoder.Encode(target)
		if err != nil {
			return DecoderAlone{}, err
		}
		pixels, err := decoder.Decode(latent)
		if err != nil {
			return DecoderAlone{}, err
		}
		judged, _, err := classifyPrompt(ModalityImage, pixels)
		if err != nil {
			return DecoderAlone{}, err
		}
		if judged == prompt {
			result.Correct++
		}
	}
	return result, nil
}

// countRoleCapability adds one correct score to the seen or held-out count.
func countRoleCapability(capability *RoleCapability, score RoleScore) {
	if !score.Correct {
		return
	}
	if score.Seen {
		capability.SeenCorrect++
	} else {
		capability.HeldOutCorrect++
	}
}

// isSeen reports whether prompt is absent from the held-out set.
func isSeen(heldOut map[string]struct{}, prompt string) bool {
	_, held := heldOut[prompt]
	return !held
}
