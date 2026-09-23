package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	rand "math/rand/v2"
	"strings"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const fixedDecoderPixels = ImageSize * ImageSize * 3

// FixedDecoder is a small linear autoencoder over the 192 image values, trained once on the nine image targets and then
// locked: Encode(x) = tanh(W·x + b) (Latent values), Decode(z) = clamp(V·z + c, 0, 1). Hash is the SHA-256 of the JSON of
// its parameters, recorded so a report can name exactly which decoder supplied part of the capability.
type FixedDecoder struct {
	Latent int       `json:"latent"`
	W      []float64 `json:"w"` // Latent × 192, row-major
	B      []float64 `json:"b"` // Latent
	V      []float64 `json:"v"` // 192 × Latent, row-major
	C      []float64 `json:"c"` // 192
}

// TrainFixedDecoder trains the autoencoder on the nine RenderImage targets with full-batch gradient descent on the mean
// squared reconstruction error: latent 1..64, epochs 1..100000, rate > 0, weights initialised uniform [-0.1, 0.1) from
// rand.New(rand.NewPCG(seed, 7)) in the order W, B, V, C. It is deterministic for the same arguments.
func TrainFixedDecoder(latent, epochs int, rate float64, seed uint64) (*FixedDecoder, error) {
	if latent < 1 || latent > 64 {
		return nil, fmt.Errorf("media: latent size %d is outside [1, 64]", latent)
	}
	if epochs < 1 || epochs > 100000 {
		return nil, fmt.Errorf("media: epochs %d is outside [1, 100000]", epochs)
	}
	if !finite(rate) || rate <= 0 {
		return nil, fmt.Errorf("media: learning rate must be finite and positive")
	}
	rng := rand.New(rand.NewPCG(seed, 7))
	decoder := &FixedDecoder{
		Latent: latent,
		W:      fixedUniform(rng, latent*fixedDecoderPixels),
		B:      fixedUniform(rng, latent),
		V:      fixedUniform(rng, fixedDecoderPixels*latent),
		C:      fixedUniform(rng, fixedDecoderPixels),
	}
	targets, err := fixedDecoderTargets()
	if err != nil {
		return nil, err
	}
	gradW := make([]float64, len(decoder.W))
	gradB := make([]float64, len(decoder.B))
	gradV := make([]float64, len(decoder.V))
	gradC := make([]float64, len(decoder.C))
	latentValues := make([]float64, latent)
	deltaLatent := make([]float64, latent)
	decoded := make([]float64, fixedDecoderPixels)
	for epoch := 0; epoch < epochs; epoch++ {
		clear(gradW)
		clear(gradB)
		clear(gradV)
		clear(gradC)
		for _, target := range targets {
			clear(deltaLatent)
			for j := 0; j < latent; j++ {
				sum := decoder.B[j]
				row := j * fixedDecoderPixels
				for k, x := range target {
					sum += decoder.W[row+k] * x
				}
				latentValues[j] = math.Tanh(sum)
			}
			for i := 0; i < fixedDecoderPixels; i++ {
				sum := decoder.C[i]
				row := i * latent
				for j, z := range latentValues {
					sum += decoder.V[row+j] * z
				}
				decoded[i] = sum
				if sum < 0 {
					decoded[i] = 0
				} else if sum > 1 {
					decoded[i] = 1
				}
			}
			for i, targetValue := range target {
				raw := decoded[i]
				clamped := math.Max(0, math.Min(1, raw))
				// Continue gradients only when saturation hides a reconstruction error.
				if (raw <= 0 && targetValue == 0) || (raw >= 1 && targetValue == 1) {
					continue
				}
				delta := 2 * (clamped - targetValue) / float64(len(targets)*fixedDecoderPixels)
				gradC[i] += delta
				row := i * latent
				for j, z := range latentValues {
					gradV[row+j] += delta * z
					deltaLatent[j] += delta * decoder.V[row+j]
				}
			}
			for j, z := range latentValues {
				delta := deltaLatent[j] * (1 - z*z)
				gradB[j] += delta
				row := j * fixedDecoderPixels
				for k, x := range target {
					gradW[row+k] += delta * x
				}
			}
		}
		for i := range decoder.W {
			decoder.W[i] -= rate * gradW[i]
		}
		for i := range decoder.B {
			decoder.B[i] -= rate * gradB[i]
		}
		for i := range decoder.V {
			decoder.V[i] -= rate * gradV[i]
		}
		for i := range decoder.C {
			decoder.C[i] -= rate * gradC[i]
		}
	}
	if _, err := decoder.ReconstructionMSE(); err != nil {
		return nil, fmt.Errorf("media: validate trained fixed decoder: %w", err)
	}
	return decoder, nil
}

// Encode maps image pixels to the fixed decoder's tanh latent representation.
func (d *FixedDecoder) Encode(pixels []float64) ([]float64, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	if len(pixels) != fixedDecoderPixels {
		return nil, fmt.Errorf("media: image has %d values, want %d", len(pixels), fixedDecoderPixels)
	}
	for i, value := range pixels {
		if !finite(value) {
			return nil, fmt.Errorf("media: image value %d is non-finite", i)
		}
	}
	latent := make([]float64, d.Latent)
	for j := range latent {
		sum := d.B[j]
		row := j * fixedDecoderPixels
		for k, pixel := range pixels {
			sum += d.W[row+k] * pixel
		}
		latent[j] = math.Tanh(sum)
	}
	return latent, nil
}

// Decode maps a latent representation through the fixed linear decoder and clamps image pixels to [0, 1].
func (d *FixedDecoder) Decode(latent []float64) ([]float64, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	if len(latent) != d.Latent {
		return nil, fmt.Errorf("media: latent has %d values, want %d", len(latent), d.Latent)
	}
	for i, value := range latent {
		if !finite(value) {
			return nil, fmt.Errorf("media: latent value %d is non-finite", i)
		}
	}
	pixels := make([]float64, fixedDecoderPixels)
	for i := range pixels {
		sum := d.C[i]
		row := i * d.Latent
		for j, value := range latent {
			sum += d.V[row+j] * value
		}
		pixels[i] = math.Max(0, math.Min(1, sum))
	}
	return pixels, nil
}

// Hash returns the SHA-256 hash of the decoder's JSON parameters.
func (d *FixedDecoder) Hash() string {
	if d == nil {
		return ""
	}
	data, err := json.Marshal(d)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ReconstructionMSE returns the mean over the nine image targets of each target's mean squared reconstruction error.
func (d *FixedDecoder) ReconstructionMSE() (float64, error) {
	targets, err := fixedDecoderTargets()
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, target := range targets {
		latent, err := d.Encode(target)
		if err != nil {
			return 0, err
		}
		reconstructed, err := d.Decode(latent)
		if err != nil {
			return 0, err
		}
		for i, value := range target {
			delta := reconstructed[i] - value
			total += delta * delta / float64(fixedDecoderPixels*len(targets))
		}
	}
	return total, nil
}

// Parameters returns the total number of trainable values stored in the decoder.
func (d *FixedDecoder) Parameters() int {
	if d == nil {
		return 0
	}
	return len(d.W) + len(d.B) + len(d.V) + len(d.C)
}

// DefaultFixedDecoder trains the 8-dimensional decoder at rate 0.5 and seed 1. Measured MSE: 3000 epochs, 0.00002752.
func DefaultFixedDecoder() (*FixedDecoder, error) {
	return TrainFixedDecoder(8, 3000, 0.5, 1)
}

// LatentGenerator trains a text-conditioned neural generator to produce representations consumed by a frozen decoder.
type LatentGenerator struct {
	config  GeneratorConfig
	words   []string
	trainer *learning.Trainer
	decoder *FixedDecoder
}

// NewLatentGenerator builds an image generator whose output is the frozen decoder's latent representation.
func NewLatentGenerator(c GeneratorConfig, d *FixedDecoder) (*LatentGenerator, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Modality != ModalityImage {
		return nil, fmt.Errorf("media: latent generator requires image modality")
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	words, err := Words(c.Modality)
	if err != nil {
		return nil, err
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
	modelConfig := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        len(words),
		OutputSize:       d.Latent,
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
		Readout: uniformGenerator(c.Seed, 2, c.Hidden*d.Latent, .05),
	}
	options := learning.DefaultOptions()
	options.LearningRate = c.LearningRate
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}
	trainer, err := learning.NewTrainer(modelConfig, parameters, options)
	if err != nil {
		return nil, fmt.Errorf("media: create latent generator trainer: %w", err)
	}
	decoder := &FixedDecoder{Latent: d.Latent, W: append([]float64(nil), d.W...), B: append([]float64(nil), d.B...), V: append([]float64(nil), d.V...), C: append([]float64(nil), d.C...)}
	return &LatentGenerator{config: c, words: words, trainer: trainer, decoder: decoder}, nil
}

// Latent returns the core's latent output for an image prompt.
func (g *LatentGenerator) Latent(ctx context.Context, prompt string) ([]float64, error) {
	input, err := g.input(ctx, prompt)
	if err != nil {
		return nil, err
	}
	outputs, err := g.trainer.PredictAll(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("media: predict latent representation: %w", err)
	}
	return append([]float64(nil), outputs[len(outputs)-1]...), nil
}

// Generate decodes the core's latent output into image pixels.
func (g *LatentGenerator) Generate(ctx context.Context, prompt string) ([]float64, error) {
	latent, err := g.Latent(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return g.decoder.Decode(latent)
}

// Step trains the core toward the decoder's latent for the prompt's rendered image while leaving the decoder unchanged.
func (g *LatentGenerator) Step(ctx context.Context, prompt string) (learning.StepResult, float64, error) {
	var zero learning.StepResult
	input, err := g.input(ctx, prompt)
	if err != nil {
		return zero, 0, err
	}
	condition, err := ParseImagePrompt(prompt)
	if err != nil {
		return zero, 0, err
	}
	pixels, err := RenderImage(condition)
	if err != nil {
		return zero, 0, err
	}
	target, err := g.decoder.Encode(pixels)
	if err != nil {
		return zero, 0, err
	}
	outputs, err := g.trainer.PredictAll(ctx, input)
	if err != nil {
		return zero, 0, fmt.Errorf("media: predict latent training output: %w", err)
	}
	last := len(outputs) - 1
	upstream := make([][]float64, len(outputs))
	for i := range upstream {
		upstream[i] = make([]float64, g.decoder.Latent)
	}
	loss := 0.0
	for i, value := range outputs[last] {
		delta := value - target[i]
		loss += delta * delta / float64(g.decoder.Latent)
		upstream[last][i] = 2 * delta / float64(g.decoder.Latent)
	}
	result, err := g.trainer.StepFrom(ctx, input, upstream)
	if err != nil {
		return zero, 0, fmt.Errorf("media: train latent generator: %w", err)
	}
	return result, loss, nil
}

// Capacity reports parameter counts and training status for the core and the frozen decoder.
func (g *LatentGenerator) Capacity() CapacitySplit {
	if g == nil || g.trainer == nil {
		return CapacitySplit{}
	}
	snapshot := g.trainer.Snapshot()
	return CapacitySplit{
		EncoderParameters: len(snapshot.Parameters.Encoder),
		CoreParameters:    len(snapshot.Parameters.Core.Weights) + len(snapshot.Parameters.Core.Bias) + len(snapshot.Parameters.Core.LogTau),
		ReadoutParameters: len(snapshot.Parameters.Readout),
		DecoderParameters: g.decoder.Parameters(),
		EncoderTrains:     snapshot.Options.Trainable.Encoder,
		CoreTrains:        snapshot.Options.Trainable.Weights || snapshot.Options.Trainable.Bias || snapshot.Options.Trainable.Tau || snapshot.Options.Trainable.Theta,
		ReadoutTrains:     snapshot.Options.Trainable.Readout,
	}
}

// LatentStats contains per-dimension moments and the mean absolute latent difference across fixture targets.
type LatentStats struct {
	CoreMean     []float64 `json:"core_mean"`
	CoreStd      []float64 `json:"core_std"`
	TargetMean   []float64 `json:"target_mean"`
	TargetStd    []float64 `json:"target_std"`
	MeanAbsError float64   `json:"mean_abs_error"`
}

// Stats compares the core and decoder latents across the nine image targets.
func (g *LatentGenerator) Stats(ctx context.Context) (LatentStats, error) {
	if g == nil || g.trainer == nil || g.decoder == nil {
		return LatentStats{}, fmt.Errorf("media: nil latent generator")
	}
	if ctx == nil {
		return LatentStats{}, fmt.Errorf("media: nil context")
	}
	prompts, err := fixturePrompts(ModalityImage)
	if err != nil {
		return LatentStats{}, err
	}
	stats := LatentStats{
		CoreMean:   make([]float64, g.decoder.Latent),
		CoreStd:    make([]float64, g.decoder.Latent),
		TargetMean: make([]float64, g.decoder.Latent),
		TargetStd:  make([]float64, g.decoder.Latent),
	}
	cores := make([][]float64, len(prompts))
	targets := make([][]float64, len(prompts))
	for i, prompt := range prompts {
		condition, err := ParseImagePrompt(prompt)
		if err != nil {
			return LatentStats{}, err
		}
		pixels, err := RenderImage(condition)
		if err != nil {
			return LatentStats{}, err
		}
		targets[i], err = g.decoder.Encode(pixels)
		if err != nil {
			return LatentStats{}, err
		}
		cores[i], err = g.Latent(ctx, prompt)
		if err != nil {
			return LatentStats{}, err
		}
		for j := range stats.CoreMean {
			stats.CoreMean[j] += cores[i][j] / float64(len(prompts))
			stats.TargetMean[j] += targets[i][j] / float64(len(prompts))
			stats.MeanAbsError += math.Abs(cores[i][j]-targets[i][j]) / float64(len(prompts)*g.decoder.Latent)
		}
	}
	for i := range prompts {
		for j := range stats.CoreMean {
			coreDelta := cores[i][j] - stats.CoreMean[j]
			targetDelta := targets[i][j] - stats.TargetMean[j]
			stats.CoreStd[j] += coreDelta * coreDelta / float64(len(prompts))
			stats.TargetStd[j] += targetDelta * targetDelta / float64(len(prompts))
		}
	}
	for j := range stats.CoreStd {
		stats.CoreStd[j] = math.Sqrt(stats.CoreStd[j])
		stats.TargetStd[j] = math.Sqrt(stats.TargetStd[j])
	}
	return stats, nil
}

// input validates a prompt and creates the generator's two-word settled input sequence.
func (g *LatentGenerator) input(ctx context.Context, prompt string) ([][]float64, error) {
	if g == nil || g.trainer == nil {
		return nil, fmt.Errorf("media: nil latent generator")
	}
	if ctx == nil {
		return nil, fmt.Errorf("media: nil context")
	}
	if _, err := ParseImagePrompt(prompt); err != nil {
		return nil, err
	}
	first, second, err := promptIndices(prompt, g.words)
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
	return input, nil
}

// promptIndices resolves the ordered colour and shape words in an image prompt.
func promptIndices(prompt string, words []string) (int, int, error) {
	parts := strings.Split(prompt, " ")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("media: prompt must contain exactly two words")
	}
	firstLimit := len(words) / 2
	first, second := -1, -1
	for i, word := range words {
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

// fixedUniform samples count values from the fixed decoder's initialization interval.
func fixedUniform(rng *rand.Rand, count int) []float64 {
	values := make([]float64, count)
	for i := range values {
		values[i] = rng.Float64()*0.2 - 0.1
	}
	return values
}

// fixedDecoderTargets renders the complete colour-shape image fixture.
func fixedDecoderTargets() ([][]float64, error) {
	targets := make([][]float64, 0, len(ImageColours)*len(ImageShapes))
	for _, colour := range ImageColours {
		for _, shape := range ImageShapes {
			pixels, err := RenderImage(ImageCondition{Colour: colour, Shape: shape})
			if err != nil {
				return nil, err
			}
			targets = append(targets, pixels)
		}
	}
	return targets, nil
}

// validate checks the decoder parameter shapes and finite values.
func (d *FixedDecoder) validate() error {
	if d == nil {
		return fmt.Errorf("media: nil fixed decoder")
	}
	if d.Latent < 1 || d.Latent > 64 {
		return fmt.Errorf("media: invalid fixed decoder latent size %d", d.Latent)
	}
	if len(d.W) != d.Latent*fixedDecoderPixels || len(d.B) != d.Latent || len(d.V) != fixedDecoderPixels*d.Latent || len(d.C) != fixedDecoderPixels {
		return fmt.Errorf("media: invalid fixed decoder parameter lengths")
	}
	for _, values := range [][]float64{d.W, d.B, d.V, d.C} {
		for _, value := range values {
			if !finite(value) {
				return fmt.Errorf("media: fixed decoder parameter is non-finite")
			}
		}
	}
	return nil
}

// finite reports whether value is neither NaN nor infinite.
func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
