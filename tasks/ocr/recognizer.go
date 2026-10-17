package ocr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/internal/categorical"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr/ctc"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// RecognizerConfig fixes the line model and its input representation. Empty
// and columns use one neural step per image column. aligned-columns adds a
// trailing zero propagation step. single-character flattens one complete
// image into the first of two neural steps and classifies only the final row.
// In every mode class 0 is blank and class k is Alphabet[k-1].
type RecognizerConfig struct {
	Height           int     `json:"height"`
	Hidden           int     `json:"hidden"`
	Alphabet         []rune  `json:"alphabet"`
	LearningRate     float64 `json:"learning_rate"`
	Seed             uint64  `json:"seed"`
	TrainableEncoder bool    `json:"trainable_encoder,omitempty"`
	InputProfile     string  `json:"input_profile,omitempty"`
	CharacterWidth   int     `json:"character_width,omitempty"`
}

// Validate reports the first field that cannot build a recognizer, including
// profile-specific widths and derived dimensions that would overflow an
// allocation. Alphabet entries must be Unicode scalar values.
func (c RecognizerConfig) Validate() error {
	if c.Height < 1 {
		return fmt.Errorf("ocr: height %d must be at least 1", c.Height)
	}
	if c.Hidden < 2 {
		return fmt.Errorf("ocr: hidden %d must be at least 2", c.Hidden)
	}
	if len(c.Alphabet) == 0 {
		return errors.New("ocr: alphabet is empty")
	}
	seen := make(map[rune]bool, len(c.Alphabet))
	for i, r := range c.Alphabet {
		if !utf8.ValidRune(r) {
			return fmt.Errorf("ocr: alphabet rune at position %d is not a Unicode scalar value", i)
		}
		if seen[r] {
			return fmt.Errorf("ocr: alphabet repeats %q at position %d", r, i)
		}
		seen[r] = true
	}
	if !(c.LearningRate > 0) || math.IsInf(c.LearningRate, 1) {
		return fmt.Errorf("ocr: learning rate %v must be positive and finite", c.LearningRate)
	}
	return c.validateInputProfile()
}

// Recognizer owns a recurrent OCR model and converts images according to its
// configured input profile before producing logits.
type Recognizer struct {
	trainer                *learning.Trainer
	config                 RecognizerConfig
	index                  map[rune]int
	loadedCheckpointSHA256 string
}

// TrainReport records the loss before a profile-aware update, whether a
// column-profile CTC target had no valid alignment, and the scored frame and
// label counts. single-character always scores one final frame.
type TrainReport struct {
	Loss       float64 `json:"loss"`
	Impossible bool    `json:"impossible"`
	Frames     int     `json:"frames"`
	Labels     int     `json:"labels"`
}

// EvalReport is one Evaluate call: the character error rate over the
// concatenated edit counts of every sample, the share of lines decoded exactly
// and how many samples were read.
type EvalReport struct {
	CER              EditReport `json:"cer"`
	ExactLines       float64    `json:"exact_lines"`
	Samples          int        `json:"samples"`
	EmptyOutputs     int        `json:"empty_outputs"`
	EmptyOutputRatio float64    `json:"empty_output_ratio"`
}

// NewRecognizer builds the continuous core with Height input nodes, Hidden
// recurrent nodes, and a blank-inclusive readout. The core, readout, bias,
// time constants and optimizer use the deterministic legacy initialization;
// single-character uses a seeded whole-image encoder while other profiles use
// the legacy identity encoder.
func NewRecognizer(c RecognizerConfig) (*Recognizer, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	c.Alphabet = append([]rune(nil), c.Alphabet...)
	inputSize, err := recognizerInputSize(c)
	if err != nil {
		return nil, err
	}
	nodes, err := profileSum(c.Height, c.Hidden, "network nodes")
	if err != nil {
		return nil, err
	}
	edges, err := profileProduct(nodes, c.Hidden, "dense edge")
	if err != nil {
		return nil, err
	}
	sources := make([]int, 0, edges)
	targets := make([]int, 0, edges)
	for s := 0; s < nodes; s++ {
		for h := 0; h < c.Hidden; h++ {
			sources = append(sources, s)
			targets = append(targets, c.Height+h)
		}
	}
	inputNodes := make([]int, c.Height)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, c.Hidden)
	for i := range readoutNodes {
		readoutNodes[i] = c.Height + i
	}
	classes := len(c.Alphabet) + 1
	cfg := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        inputSize,
		OutputSize:       classes,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	// Encoder rows are [input, input-node]. Single-character consumes the
	// complete row-major image and uses an independent deterministic projection;
	// the column profiles preserve the identity encoder exactly.
	encoderLen, err := profileProduct(inputSize, c.Height, "encoder")
	if err != nil {
		return nil, err
	}
	encoder := make([]float64, encoderLen)
	if c.InputProfile == inputProfileSingleCharacter {
		rng := rand.New(rand.NewPCG(c.Seed, 42))
		scale := 1 / math.Sqrt(float64(inputSize))
		for i := range encoder {
			encoder[i] = (2*rng.Float64() - 1) * scale
		}
	} else {
		for i := 0; i < c.Height; i++ {
			encoder[i*c.Height+i] = 1
		}
	}
	bias := make([]float64, nodes)
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	p := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniform(rand.NewPCG(c.Seed, 0), edges),
			Bias:    bias,
			LogTau:  logTau,
		},
		Encoder: encoder,
		Readout: uniform(rand.NewPCG(c.Seed, 1), c.Hidden*classes),
	}
	o := learning.DefaultOptions()
	o.LearningRate = c.LearningRate
	o.Trainable = learning.Trainable{Encoder: c.TrainableEncoder, Weights: true, Bias: true, Readout: true}
	tr, err := learning.NewTrainer(cfg, p, o)
	if err != nil {
		return nil, err
	}
	index := make(map[rune]int, len(c.Alphabet))
	for i, r := range c.Alphabet {
		index[r] = i + 1
	}
	return &Recognizer{trainer: tr, config: c, index: index}, nil
}

// NewRecognizerWith builds the same model as NewRecognizer but starts from
// the given parameters instead of the seeded initialization, so a caller can
// replay a trained model or run an intervened copy of one. The parameter
// shapes must match the model the configuration describes; the optimizer state
// starts empty and the update count at zero, and the recognizer owns its own
// copy of the parameters.
func NewRecognizerWith(c RecognizerConfig, p learning.Parameters) (*Recognizer, error) {
	r, err := NewRecognizer(c)
	if err != nil {
		return nil, err
	}
	s := r.trainer.Snapshot()
	s.Parameters = p
	tr, err := learning.RestoreTrainer(s)
	if err != nil {
		return nil, err
	}
	r.trainer = tr
	return r, nil
}

// Parameters returns an independent deep copy of the recognizer's current
// parameters, so a caller can read or change them without touching the
// trainer.
func (r *Recognizer) Parameters() learning.Parameters {
	if r == nil {
		return learning.Parameters{}
	}
	return r.trainer.Snapshot().Parameters
}

// uniform draws n values uniformly from [-0.3, 0.3] with the given source.
func uniform(src *rand.PCG, n int) []float64 {
	rng := rand.New(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = rng.Float64()*.6 - .3
	}
	return out
}

// Columns turns a line image into the raw column sequence: row t is column t
// of the image, top to bottom. It is available independently of the selected
// profile and always retains the legacy representation.
func (r *Recognizer) Columns(img glyphs.Image) ([][]float64, error) {
	if r == nil {
		return nil, errors.New("ocr: nil recognizer")
	}
	if img.Height != r.config.Height {
		return nil, fmt.Errorf("ocr: image height %d, want %d", img.Height, r.config.Height)
	}
	if img.Width <= 0 {
		return nil, errors.New("ocr: image has no columns")
	}
	area, err := rasterImageArea(img.Width, img.Height)
	if err != nil {
		return nil, err
	}
	if len(img.Pixels) != area {
		return nil, fmt.Errorf("ocr: %d pixels for a %dx%d image", len(img.Pixels), img.Width, img.Height)
	}
	for i, pixel := range img.Pixels {
		if math.IsNaN(pixel) || math.IsInf(pixel, 0) || pixel < 0 || pixel > 1 {
			return nil, fmt.Errorf("ocr: image pixel %d must be finite and in [0,1]", i)
		}
	}
	out := make([][]float64, img.Width)
	for x := range out {
		col := make([]float64, img.Height)
		for y := range col {
			col[y] = img.Pixels[y*img.Width+x]
		}
		out[x] = col
	}
	return out, nil
}

// profileLogits returns all neural-step logits and the rows scored by the
// profile. aligned-columns excludes its blind first row; single-character
// keeps only its final row. Inputs and logits are built before any update.
func (r *Recognizer) profileLogits(ctx context.Context, img glyphs.Image) ([][]float64, [][]float64, error) {
	if r == nil || r.trainer == nil {
		return nil, nil, errors.New("ocr: nil recognizer")
	}
	if err := ocrContextError(ctx); err != nil {
		return nil, nil, err
	}
	inputs, err := r.Inputs(img)
	if err != nil {
		return nil, nil, err
	}
	all, err := r.trainer.PredictAll(ctx, inputs)
	if err != nil {
		return nil, nil, err
	}
	visible := all
	switch r.config.InputProfile {
	case inputProfileAlignedColumns, inputProfileSingleCharacter:
		if len(all) < 2 {
			return nil, nil, errors.New("ocr: profile produced too few neural steps")
		}
		if r.config.InputProfile == inputProfileAlignedColumns {
			visible = all[1:]
		} else {
			visible = all[len(all)-1:]
		}
	}
	return inputs, visible, nil
}

// logits reads one image according to the configured profile and returns the
// output rows used for decoding or loss computation.
func (r *Recognizer) logits(ctx context.Context, img glyphs.Image) ([][]float64, error) {
	_, visible, err := r.profileLogits(ctx, img)
	if err != nil {
		return nil, err
	}
	return visible, nil
}

// labels turns text into CTC class indices, where rune Alphabet[k-1] is class
// k and blank 0 is never a label. A rune outside the alphabet is an error.
func (r *Recognizer) labels(text string) ([]int, error) {
	out := make([]int, 0, len(text))
	for _, ru := range text {
		k, ok := r.index[ru]
		if !ok {
			return nil, fmt.Errorf("ocr: %q is not in the alphabet", ru)
		}
		out = append(out, k)
	}
	return out, nil
}

func (r *Recognizer) singleLabel(text string) (int, error) {
	if !utf8.ValidString(text) {
		return 0, errors.New("ocr: single-character target is not valid UTF-8")
	}
	runes := []rune(text)
	if len(runes) != 1 {
		return 0, errors.New("ocr: single-character target must contain exactly one rune")
	}
	label, ok := r.index[runes[0]]
	if !ok {
		return 0, fmt.Errorf("ocr: %q is not in the alphabet", runes[0])
	}
	return label, nil
}

func singleCharacterLoss(logits []float64, target int) (float64, []float64, error) {
	if len(logits) < 2 || target < 1 || target >= len(logits) {
		return 0, nil, errors.New("ocr: single-character logits or target is invalid")
	}
	for class, value := range logits {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, nil, fmt.Errorf("ocr: single-character logit class %d is not finite", class)
		}
	}
	maxLogit := logits[1]
	for _, value := range logits[2:] {
		if value > maxLogit {
			maxLogit = value
		}
	}
	sum := 0.0
	probabilities := make([]float64, len(logits))
	for class := 1; class < len(logits); class++ {
		value := math.Exp(logits[class] - maxLogit)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, nil, errors.New("ocr: single-character normalization is not finite")
		}
		sum += value
	}
	if !(sum > 0) || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0, nil, errors.New("ocr: single-character normalization is invalid")
	}
	loss := -(logits[target] - maxLogit - math.Log(sum))
	if math.IsNaN(loss) || math.IsInf(loss, 0) {
		return 0, nil, errors.New("ocr: single-character loss is not finite")
	}
	if err := categorical.Gradient(probabilities[1:], logits[1:], target-1, 1, 1); err != nil {
		return 0, nil, fmt.Errorf("ocr: single-character gradient: %w", err)
	}
	return loss, probabilities, nil
}

// TrainLine performs one atomic profile-aware update. Column profiles use CTC;
// aligned-columns prepends a zero upstream row for its blind step;
// single-character applies non-blank softmax cross-entropy only to the final
// row and keeps the first row's gradient zero.
func (r *Recognizer) TrainLine(ctx context.Context, img glyphs.Image, text string) (TrainReport, error) {
	var zero TrainReport
	if r == nil || r.trainer == nil {
		return zero, errors.New("ocr: nil recognizer")
	}
	if err := ocrContextError(ctx); err != nil {
		return zero, err
	}
	var ids []int
	var singleTarget int
	var err error
	if r.config.InputProfile == inputProfileSingleCharacter {
		singleTarget, err = r.singleLabel(text)
	} else {
		ids, err = r.labels(text)
	}
	if err != nil {
		return zero, err
	}
	inputs, visible, err := r.profileLogits(ctx, img)
	if err != nil {
		return zero, err
	}
	var upstream [][]float64
	var report TrainReport
	if r.config.InputProfile == inputProfileSingleCharacter {
		loss, finalGradient, err := singleCharacterLoss(visible[len(visible)-1], singleTarget)
		if err != nil {
			return zero, err
		}
		upstream = make([][]float64, len(inputs))
		for i := range upstream {
			upstream[i] = make([]float64, len(visible[len(visible)-1]))
		}
		copy(upstream[len(upstream)-1], finalGradient)
		report = TrainReport{Loss: loss, Frames: len(visible), Labels: 1}
	} else {
		loss, gradient, ctcReport, err := ctc.LossWith(visible, ids, ctc.Options{ZeroOnImpossible: true})
		if err != nil {
			return zero, err
		}
		upstream = gradient
		if r.config.InputProfile == inputProfileAlignedColumns {
			full := make([][]float64, len(inputs))
			for i := range full {
				full[i] = make([]float64, len(gradient[0]))
			}
			for i := range gradient {
				copy(full[i+1], gradient[i])
			}
			upstream = full
		}
		report = TrainReport{Loss: loss, Impossible: ctcReport.Impossible, Frames: ctcReport.Frames, Labels: ctcReport.Labels}
		if ctcReport.Impossible {
			return report, nil
		}
	}
	if _, err := r.trainer.StepFrom(ctx, inputs, upstream); err != nil {
		return zero, err
	}
	return report, nil
}

// Infer decodes column profiles with greedy CTC. single-character ignores the
// blank class and returns the largest non-blank class from the final row.
func (r *Recognizer) Infer(ctx context.Context, img glyphs.Image) (string, error) {
	if r == nil || r.trainer == nil {
		return "", errors.New("ocr: nil recognizer")
	}
	if err := ocrContextError(ctx); err != nil {
		return "", err
	}
	_, logits, err := r.profileLogits(ctx, img)
	if err != nil {
		return "", err
	}
	if r.config.InputProfile == inputProfileSingleCharacter {
		row := logits[len(logits)-1]
		if len(row) < 2 {
			return "", errors.New("ocr: single-character logits have no non-blank classes")
		}
		for class, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return "", fmt.Errorf("ocr: single-character logit class %d is not finite", class)
			}
		}
		best := 1
		for class := 2; class < len(row); class++ {
			if row[class] > row[best] {
				best = class
			}
		}
		if best < 1 || best > len(r.config.Alphabet) {
			return "", fmt.Errorf("ocr: decoded class %d outside the alphabet", best)
		}
		return string(r.config.Alphabet[best-1]), nil
	}
	ids, err := ctc.Decode(logits)
	if err != nil {
		return "", err
	}
	out := make([]rune, 0, len(ids))
	for _, k := range ids {
		if k < 1 || k > len(r.config.Alphabet) {
			return "", fmt.Errorf("ocr: decoded class %d outside the alphabet", k)
		}
		out = append(out, r.config.Alphabet[k-1])
	}
	return string(out), nil
}

// Evaluate infers every sample and aggregates: the character error rate over
// the concatenated counts (the summed edits divided by the summed reference
// length, not the mean of the per-line rates), the share of lines decoded
// exactly, and the number of samples. An empty set reports zero samples and an
// undefined rate.
func (r *Recognizer) Evaluate(ctx context.Context, samples []glyphs.Sample) (EvalReport, error) {
	var zero EvalReport
	if r == nil || r.trainer == nil {
		return zero, errors.New("ocr: nil recognizer")
	}
	if err := ocrContextError(ctx); err != nil {
		return zero, err
	}
	var subs, dels, ins, reference, exact, emptyOutputs int
	for i, s := range samples {
		hypothesis, err := r.Infer(ctx, s.Image)
		if err != nil {
			return zero, fmt.Errorf("ocr: sample %d: %w", i, err)
		}
		e := CER(s.Text, hypothesis)
		subs += e.Substitutions
		dels += e.Deletions
		ins += e.Insertions
		reference += e.ReferenceLength
		if hypothesis == s.Text {
			exact++
		}
		if hypothesis == "" {
			emptyOutputs++
		}
	}
	report := EvalReport{CER: newReport(reference, subs, dels, ins), Samples: len(samples)}
	if len(samples) > 0 {
		report.ExactLines = float64(exact) / float64(len(samples))
		report.EmptyOutputs = emptyOutputs
		report.EmptyOutputRatio = float64(emptyOutputs) / float64(len(samples))
	}
	return report, nil
}

// Snapshot returns the trainer's snapshot: the complete model, parameters,
// optimizer state and update count at an update boundary.
func (r *Recognizer) Snapshot() learning.TrainingSnapshot {
	if r == nil {
		return learning.TrainingSnapshot{}
	}
	return r.trainer.Snapshot()
}
