package media

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	// ModalityImage selects image generation.
	ModalityImage = "image"
	// ModalityAudio selects audio generation.
	ModalityAudio = "audio"
)

// GeneratorConfig declares one text-conditioned generator on a CoImNet core.
type GeneratorConfig struct {
	Modality     string  `json:"modality"`
	Embed        int     `json:"embed"`
	Hidden       int     `json:"hidden"`
	Settle       int     `json:"settle"`
	LearningRate float64 `json:"learning_rate"`
	Seed         uint64  `json:"seed"`
}

// Validate reports whether c declares a supported generator configuration.
func (c GeneratorConfig) Validate() error {
	if c.Modality != ModalityImage && c.Modality != ModalityAudio {
		return fmt.Errorf("media: unsupported generator modality %q", c.Modality)
	}
	if c.Embed < 4 || c.Embed > 128 {
		return fmt.Errorf("media: embed %d is outside [4, 128]", c.Embed)
	}
	if c.Hidden < 8 || c.Hidden > 512 {
		return fmt.Errorf("media: hidden %d is outside [8, 512]", c.Hidden)
	}
	if c.Settle < 1 || c.Settle > 8 {
		return fmt.Errorf("media: settle %d is outside [1, 8]", c.Settle)
	}
	if math.IsNaN(c.LearningRate) || math.IsInf(c.LearningRate, 0) || c.LearningRate <= 0 {
		return fmt.Errorf("media: learning rate must be finite and positive")
	}
	return nil
}

// DefaultGeneratorConfig returns the specified defaults for modality.
func DefaultGeneratorConfig(modality string) GeneratorConfig {
	return GeneratorConfig{Modality: modality, Embed: 16, Hidden: 64, Settle: 3, LearningRate: .01, Seed: 1}
}

// Words returns the modality vocabulary in its fixed order.
func Words(modality string) ([]string, error) {
	var words []string
	switch modality {
	case ModalityImage:
		words = append(append(words, ImageColours...), ImageShapes...)
	case ModalityAudio:
		words = append(append(words, AudioPitches...), AudioPatterns...)
	default:
		return nil, fmt.Errorf("media: unsupported generator modality %q", modality)
	}
	return words, nil
}

// DecoderInput contains only the representation produced by the core.
type DecoderInput struct {
	Representation []float64 `json:"representation"`
}

// Decode clamps the representation into the fixed image or audio output range.
func Decode(modality string, in DecoderInput) ([]float64, error) {
	length, lower, upper := 0, 0.0, 1.0
	switch modality {
	case ModalityImage:
		length = ImageSize * ImageSize * 3
	case ModalityAudio:
		length, lower, upper = AudioBlock, -1, 1
	default:
		return nil, fmt.Errorf("media: unsupported decoder modality %q", modality)
	}
	if len(in.Representation) != length {
		return nil, fmt.Errorf("media: %s representation has %d values, want %d", modality, len(in.Representation), length)
	}
	output := make([]float64, length)
	for i, value := range in.Representation {
		if math.IsNaN(value) {
			return nil, fmt.Errorf("media: representation value %d is NaN", i)
		}
		if value < lower {
			value = lower
		} else if value > upper {
			value = upper
		}
		output[i] = value
	}
	return output, nil
}

// CapacitySplit reports parameter counts and which parameter groups train.
type CapacitySplit struct {
	EncoderParameters int  `json:"encoder_parameters"`
	CoreParameters    int  `json:"core_parameters"`
	ReadoutParameters int  `json:"readout_parameters"`
	DecoderParameters int  `json:"decoder_parameters"`
	EncoderTrains     bool `json:"encoder_trains"`
	CoreTrains        bool `json:"core_trains"`
	ReadoutTrains     bool `json:"readout_trains"`
}

// Generator owns a prompt encoder, one recurrent core and a trainable readout.
type Generator struct {
	config  GeneratorConfig
	words   []string
	trainer *learning.Trainer
}

// NewGenerator builds a deterministic text-conditioned generator.
func NewGenerator(c GeneratorConfig) (*Generator, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	words, err := Words(c.Modality)
	if err != nil {
		return nil, err
	}
	outputSize := ImageSize * ImageSize * 3
	if c.Modality == ModalityAudio {
		outputSize = AudioBlock
	}
	hiddenFirst := c.Embed
	nodes := c.Embed + c.Hidden
	sources := make([]int, 0, c.Embed*c.Hidden+c.Hidden*c.Hidden)
	targets := make([]int, 0, cap(sources))
	for input := 0; input < c.Embed; input++ {
		for hidden := hiddenFirst; hidden < nodes; hidden++ {
			sources = append(sources, input)
			targets = append(targets, hidden)
		}
	}
	for source := hiddenFirst; source < nodes; source++ {
		for target := hiddenFirst; target < nodes; target++ {
			sources = append(sources, source)
			targets = append(targets, target)
		}
	}
	inputNodes := make([]int, c.Embed)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, c.Hidden)
	for i := range readoutNodes {
		readoutNodes[i] = hiddenFirst + i
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        len(words),
		OutputSize:       outputSize,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniformGenerator(c.Seed, 0, len(sources), 1/math.Sqrt(float64(nodes))),
			Bias:    make([]float64, nodes),
			LogTau:  logTau,
		},
		Encoder: uniformGenerator(c.Seed, 1, len(words)*c.Embed, .5),
		Readout: uniformGenerator(c.Seed, 2, c.Hidden*outputSize, .05),
	}
	options := learning.DefaultOptions()
	options.LearningRate = c.LearningRate
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return nil, fmt.Errorf("media: create generator trainer: %w", err)
	}
	return &Generator{config: c, words: words, trainer: trainer}, nil
}

// Config returns this generator's configuration.
func (g *Generator) Config() GeneratorConfig {
	if g == nil {
		return GeneratorConfig{}
	}
	return g.config
}

// Capacity reports parameter counts and the generator's training split.
func (g *Generator) Capacity() CapacitySplit {
	if g == nil || g.trainer == nil {
		return CapacitySplit{}
	}
	snapshot := g.trainer.Snapshot()
	return CapacitySplit{
		EncoderParameters: len(snapshot.Parameters.Encoder),
		CoreParameters:    len(snapshot.Parameters.Core.Weights) + len(snapshot.Parameters.Core.Bias) + len(snapshot.Parameters.Core.LogTau),
		ReadoutParameters: len(snapshot.Parameters.Readout),
		DecoderParameters: 0,
		EncoderTrains:     snapshot.Options.Trainable.Encoder,
		CoreTrains:        snapshot.Options.Trainable.Weights || snapshot.Options.Trainable.Bias || snapshot.Options.Trainable.Tau || snapshot.Options.Trainable.Theta,
		ReadoutTrains:     snapshot.Options.Trainable.Readout,
	}
}

// Snapshot returns the trainer's independent training snapshot.
func (g *Generator) Snapshot() learning.TrainingSnapshot {
	if g == nil || g.trainer == nil {
		return learning.TrainingSnapshot{}
	}
	return g.trainer.Snapshot()
}

// Representation runs the two-word prompt through the core and returns its last-row output.
func (g *Generator) Representation(ctx context.Context, prompt string) ([]float64, error) {
	if g == nil || g.trainer == nil {
		return nil, fmt.Errorf("media: nil generator")
	}
	if ctx == nil {
		return nil, fmt.Errorf("media: nil context")
	}
	first, second, err := g.promptIndices(prompt)
	if err != nil {
		return nil, err
	}
	input := make([][]float64, 2*g.config.Settle)
	for i := range input {
		input[i] = make([]float64, len(g.words))
		word := first
		if i >= g.config.Settle {
			word = second
		}
		input[i][word] = 1
	}
	outputs, err := g.trainer.PredictAll(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("media: predict representation: %w", err)
	}
	return append([]float64(nil), outputs[len(outputs)-1]...), nil
}

// Generate decodes the representation produced for prompt.
func (g *Generator) Generate(ctx context.Context, prompt string) ([]float64, error) {
	representation, err := g.Representation(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return Decode(g.config.Modality, DecoderInput{Representation: representation})
}

// Step trains on one prompt and target, returning the pre-update step result and mean squared error.
func (g *Generator) Step(ctx context.Context, prompt string, target []float64) (learning.StepResult, float64, error) {
	var zero learning.StepResult
	if g == nil || g.trainer == nil {
		return zero, 0, fmt.Errorf("media: nil generator")
	}
	if ctx == nil {
		return zero, 0, fmt.Errorf("media: nil context")
	}
	if _, _, err := g.promptIndices(prompt); err != nil {
		return zero, 0, err
	}
	outputSize := ImageSize * ImageSize * 3
	if g.config.Modality == ModalityAudio {
		outputSize = AudioBlock
	}
	if len(target) != outputSize {
		return zero, 0, fmt.Errorf("media: target has %d values, want %d", len(target), outputSize)
	}
	for i, value := range target {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return zero, 0, fmt.Errorf("media: target value %d is non-finite", i)
		}
	}
	first, second, _ := g.promptIndices(prompt)
	input := make([][]float64, 2*g.config.Settle)
	for i := range input {
		input[i] = make([]float64, len(g.words))
		word := first
		if i >= g.config.Settle {
			word = second
		}
		input[i][word] = 1
	}
	outputs, err := g.trainer.PredictAll(ctx, input)
	if err != nil {
		return zero, 0, fmt.Errorf("media: predict training representation: %w", err)
	}
	last := len(outputs) - 1
	upstream := make([][]float64, len(outputs))
	for i := range upstream {
		upstream[i] = make([]float64, outputSize)
	}
	mse := 0.0
	for i, value := range outputs[last] {
		delta := value - target[i]
		mse += delta * delta / float64(outputSize)
		upstream[last][i] = 2 * delta / float64(outputSize)
	}
	result, err := g.trainer.StepFrom(ctx, input, upstream)
	if err != nil {
		return zero, 0, fmt.Errorf("media: train generator: %w", err)
	}
	return result, mse, nil
}

// promptIndices validates prompt word order and returns their vocabulary indexes.
func (g *Generator) promptIndices(prompt string) (int, int, error) {
	parts := strings.Split(prompt, " ")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("media: prompt must contain exactly two words")
	}
	firstLimit := len(g.words) / 2
	first, second := -1, -1
	for i, word := range g.words {
		if parts[0] == word && i < firstLimit {
			first = i
		}
		if parts[1] == word && i >= firstLimit {
			second = i
		}
	}
	if first < 0 || second < 0 {
		return 0, 0, fmt.Errorf("media: unsupported or out-of-order prompt %q", prompt)
	}
	return first, second, nil
}

// uniformGenerator draws count deterministic values from [-scale, scale).
func uniformGenerator(seed, stream uint64, count int, scale float64) []float64 {
	rng := rand.New(rand.NewPCG(seed, stream))
	values := make([]float64, count)
	for i := range values {
		values[i] = (2*rng.Float64() - 1) * scale
	}
	return values
}
