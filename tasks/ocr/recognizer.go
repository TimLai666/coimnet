package ocr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr/ctc"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// RecognizerConfig fixes the line model: Height input nodes (one per image
// row), Hidden recurrent nodes, an alphabet whose class k is Alphabet[k-1]
// (blank is 0), the learning rate and the weight seed.
type RecognizerConfig struct {
	Height       int     `json:"height"`
	Hidden       int     `json:"hidden"`
	Alphabet     []rune  `json:"alphabet"`
	LearningRate float64 `json:"learning_rate"`
	Seed         uint64  `json:"seed"`
}

// Validate reports the first field that cannot build a recognizer: Height
// below 1, Hidden below 2, an empty or repeating alphabet, or a learning rate
// that is not a positive finite number.
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
		if seen[r] {
			return fmt.Errorf("ocr: alphabet repeats %q at position %d", r, i)
		}
		seen[r] = true
	}
	if !(c.LearningRate > 0) || math.IsInf(c.LearningRate, 1) {
		return fmt.Errorf("ocr: learning rate %v must be positive and finite", c.LearningRate)
	}
	return nil
}

// Recognizer reads a line image column by column and emits one logit row per
// column.
type Recognizer struct {
	trainer *learning.Trainer
	config  RecognizerConfig
	index   map[rune]int
}

// TrainReport is one TrainLine call: the CTC loss before the update, whether
// the line had no valid alignment and therefore no update, and the frame and
// label counts the loss was computed over.
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
	CER        EditReport `json:"cer"`
	ExactLines float64    `json:"exact_lines"`
	Samples    int        `json:"samples"`
}

// NewRecognizer builds the continuous core: Height+Hidden nodes, the first
// Height of them fed by an identity encoder (one input node per image row) and
// the Hidden remaining ones read out every step into len(Alphabet)+1 classes.
// Every input node feeds every hidden node and every hidden node feeds every
// hidden node, self loops included; those weights are drawn uniformly from
// [-0.3, 0.3] with rand.NewPCG(Seed, 0) and the readout with
// rand.NewPCG(Seed, 1). Bias starts at 0 and the time constants at log(2).
// Training is the AdamW baseline of learning.DefaultOptions at LearningRate
// with the weight, bias and readout groups unfrozen, DT 1 and tanh.
func NewRecognizer(c RecognizerConfig) (*Recognizer, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	c.Alphabet = append([]rune(nil), c.Alphabet...)
	nodes := c.Height + c.Hidden
	edges := c.Height*c.Hidden + c.Hidden*c.Hidden
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
		InputSize:        c.Height,
		OutputSize:       classes,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	// The encoder is the identity from the Height inputs onto the Height input
	// nodes, row-major [input, input-node], and stays frozen.
	encoder := make([]float64, c.Height*c.Height)
	for i := 0; i < c.Height; i++ {
		encoder[i*c.Height+i] = 1
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
	o.Trainable = learning.Trainable{Weights: true, Bias: true, Readout: true}
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

// uniform draws n values uniformly from [-0.3, 0.3] with the given source.
func uniform(src *rand.PCG, n int) []float64 {
	rng := rand.New(src)
	out := make([]float64, n)
	for i := range out {
		out[i] = rng.Float64()*.6 - .3
	}
	return out
}

// Columns turns a line image into the input sequence: row t is column t of the
// image, top to bottom, so the sequence has one step per column and Height
// values per step. An image of another height, an empty image or a pixel count
// that disagrees with Width*Height is an error.
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
	if len(img.Pixels) != img.Width*img.Height {
		return nil, fmt.Errorf("ocr: %d pixels for a %dx%d image", len(img.Pixels), img.Width, img.Height)
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

// TrainLine runs one CTC step on one labelled line: the columns are read out
// step by step, the CTC loss and its gradient with respect to the logits are
// taken against the label sequence, and that gradient is the upstream of one
// trainer step. A line with no valid alignment (more labels, or more repeated
// labels, than columns) is reported with Impossible true and leaves the
// trainer untouched instead of failing the run.
func (r *Recognizer) TrainLine(ctx context.Context, img glyphs.Image, text string) (TrainReport, error) {
	var zero TrainReport
	if r == nil || r.trainer == nil {
		return zero, errors.New("ocr: nil recognizer")
	}
	columns, err := r.Columns(img)
	if err != nil {
		return zero, err
	}
	ids, err := r.labels(text)
	if err != nil {
		return zero, err
	}
	logits, err := r.trainer.PredictAll(ctx, columns)
	if err != nil {
		return zero, err
	}
	loss, grad, rep, err := ctc.LossWith(logits, ids, ctc.Options{ZeroOnImpossible: true})
	if err != nil {
		return zero, err
	}
	report := TrainReport{Loss: loss, Impossible: rep.Impossible, Frames: rep.Frames, Labels: rep.Labels}
	if rep.Impossible {
		return report, nil
	}
	if _, err := r.trainer.StepFrom(ctx, columns, grad); err != nil {
		return zero, err
	}
	return report, nil
}

// Infer decodes one line greedily: per-column argmax, collapsed repeats and
// dropped blanks, mapped back to runes.
func (r *Recognizer) Infer(ctx context.Context, img glyphs.Image) (string, error) {
	if r == nil || r.trainer == nil {
		return "", errors.New("ocr: nil recognizer")
	}
	columns, err := r.Columns(img)
	if err != nil {
		return "", err
	}
	logits, err := r.trainer.PredictAll(ctx, columns)
	if err != nil {
		return "", err
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
	var subs, dels, ins, reference, exact int
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
	}
	report := EvalReport{CER: newReport(reference, subs, dels, ins), Samples: len(samples)}
	if len(samples) > 0 {
		report.ExactLines = float64(exact) / float64(len(samples))
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
