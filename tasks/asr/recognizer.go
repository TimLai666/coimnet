// Package asr is the speech-to-text task on the shared continuous core: a
// synthetic speech fixture that embeds no audio file, and a whole-file
// recogniser that feeds log-mel frames into the core one frame per step, reads
// out every step, trains with CTC and reports character and word error rates.
// Streaming, chunk by chunk, is a separate mode and is not in this file.
package asr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/ctc"
)

// RecognizerConfig fixes the whole-file model: the sample rate every signal is
// expected at, the log-mel front end that turns it into frames (one input node
// per mel band), Hidden recurrent nodes, an alphabet whose class k is
// Alphabet[k-1] (blank is 0), the learning rate and the weight seed.
type RecognizerConfig struct {
	SampleRate   int                  `json:"sample_rate"`
	FrontEnd     audio.FrontEndConfig `json:"front_end"`
	Hidden       int                  `json:"hidden"`
	Alphabet     []rune               `json:"alphabet"`
	LearningRate float64              `json:"learning_rate"`
	Seed         uint64               `json:"seed"`
}

// Validate reports the first field that cannot build a recognizer: a front end
// that does not fit the sample rate (which covers a non-positive rate), Hidden
// below 2, an empty or repeating alphabet, or a learning rate that is not a
// positive finite number.
func (c RecognizerConfig) Validate() error {
	if err := c.FrontEnd.Validate(c.SampleRate); err != nil {
		return err
	}
	if c.Hidden < 2 {
		return fmt.Errorf("asr: hidden %d must be at least 2", c.Hidden)
	}
	if len(c.Alphabet) == 0 {
		return errors.New("asr: alphabet is empty")
	}
	seen := make(map[rune]bool, len(c.Alphabet))
	for i, r := range c.Alphabet {
		if seen[r] {
			return fmt.Errorf("asr: alphabet repeats %q at position %d", r, i)
		}
		seen[r] = true
	}
	if !(c.LearningRate > 0) || math.IsInf(c.LearningRate, 1) {
		return fmt.Errorf("asr: learning rate %v must be positive and finite", c.LearningRate)
	}
	return nil
}

// Recognizer reads log-mel frames one per step and emits one logit row per
// frame (blank 0, class k = Alphabet[k-1]).
type Recognizer struct {
	trainer *learning.Trainer
	config  RecognizerConfig
	index   map[rune]int
}

// TrainReport is one TrainUtterance call: the CTC loss before the update,
// whether the utterance had no valid alignment and therefore no update, and
// the frame and label counts the loss was computed over.
type TrainReport struct {
	Loss       float64 `json:"loss"`
	Impossible bool    `json:"impossible"`
	Frames     int     `json:"frames"`
	Labels     int     `json:"labels"`
}

// EvalReport is one Evaluate call. CER is the character error rate over the
// concatenated edit counts of every utterance. WER is the same aggregation of
// ocr.WER, which counts whitespace-separated tokens: treating each character
// as its own word would not be a word error rate at all, so the real
// definition is kept, and on a fixture whose transcripts contain no whitespace
// every utterance is a single token, which makes this number the share of
// utterances that were not transcribed exactly. ExactUtterances is that share
// measured directly, Samples is how many utterances were read and FrontEnd
// declares the feature definition, with Frames summed over every utterance.
type EvalReport struct {
	CER             ocr.EditReport       `json:"cer"`
	WER             ocr.EditReport       `json:"wer"`
	ExactUtterances float64              `json:"exact_utterances"`
	Samples         int                  `json:"samples"`
	FrontEnd        audio.FrontEndReport `json:"front_end"`
}

// NewRecognizer builds the continuous core: Mels+Hidden nodes, the first Mels
// of them fed by an identity encoder (one input node per mel band) and the
// Hidden remaining ones read out every step into len(Alphabet)+1 classes.
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
	mels := c.FrontEnd.Mels
	nodes := mels + c.Hidden
	edges := mels*c.Hidden + c.Hidden*c.Hidden
	sources := make([]int, 0, edges)
	targets := make([]int, 0, edges)
	for s := 0; s < nodes; s++ {
		for h := 0; h < c.Hidden; h++ {
			sources = append(sources, s)
			targets = append(targets, mels+h)
		}
	}
	inputNodes := make([]int, mels)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, c.Hidden)
	for i := range readoutNodes {
		readoutNodes[i] = mels + i
	}
	classes := len(c.Alphabet) + 1
	cfg := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        mels,
		OutputSize:       classes,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	// The encoder is the identity from the Mels inputs onto the Mels input
	// nodes, row-major [input, input-node], and stays frozen.
	encoder := make([]float64, mels*mels)
	for i := 0; i < mels; i++ {
		encoder[i*mels+i] = 1
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

// Features is audio.LogMel with the recognizer's config; the report declares
// the front end. Row t is the input of step t and holds Mels values, so a
// signal shorter than one analysis window is an error rather than an empty
// sequence.
func (r *Recognizer) Features(x []float64) ([][]float64, audio.FrontEndReport, error) {
	if r == nil {
		return nil, audio.FrontEndReport{}, errors.New("asr: nil recognizer")
	}
	return audio.LogMel(x, r.config.SampleRate, r.config.FrontEnd)
}

// labels turns text into CTC class indices, where rune Alphabet[k-1] is class
// k and blank 0 is never a label. A rune outside the alphabet is an error.
func (r *Recognizer) labels(text string) ([]int, error) {
	out := make([]int, 0, len(text))
	for _, ru := range text {
		k, ok := r.index[ru]
		if !ok {
			return nil, fmt.Errorf("asr: %q is not in the alphabet", ru)
		}
		out = append(out, k)
	}
	return out, nil
}

// TrainUtterance runs one CTC step on one transcribed recording: the log-mel
// frames are read out step by step, the CTC loss and its gradient with respect
// to the logits are taken against the label sequence, and that gradient is the
// upstream of one trainer step. An utterance with no valid alignment (more
// labels, or more repeated labels, than frames) is reported with Impossible
// true and leaves the trainer untouched instead of failing the run.
func (r *Recognizer) TrainUtterance(ctx context.Context, x []float64, text string) (TrainReport, error) {
	var zero TrainReport
	if r == nil || r.trainer == nil {
		return zero, errors.New("asr: nil recognizer")
	}
	frames, _, err := r.Features(x)
	if err != nil {
		return zero, err
	}
	ids, err := r.labels(text)
	if err != nil {
		return zero, err
	}
	logits, err := r.trainer.PredictAll(ctx, frames)
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
	if _, err := r.trainer.StepFrom(ctx, frames, grad); err != nil {
		return zero, err
	}
	return report, nil
}

// Transcribe decodes one recording greedily: per-frame argmax, collapsed
// repeats and dropped blanks, mapped back to runes.
func (r *Recognizer) Transcribe(ctx context.Context, x []float64) (string, error) {
	text, _, err := r.transcribe(ctx, x)
	return text, err
}

// transcribe is Transcribe keeping the front-end report, so Evaluate can count
// the frames it read without running the front end a second time.
func (r *Recognizer) transcribe(ctx context.Context, x []float64) (string, audio.FrontEndReport, error) {
	if r == nil || r.trainer == nil {
		return "", audio.FrontEndReport{}, errors.New("asr: nil recognizer")
	}
	frames, front, err := r.Features(x)
	if err != nil {
		return "", audio.FrontEndReport{}, err
	}
	logits, err := r.trainer.PredictAll(ctx, frames)
	if err != nil {
		return "", audio.FrontEndReport{}, err
	}
	ids, err := ctc.Decode(logits)
	if err != nil {
		return "", audio.FrontEndReport{}, err
	}
	out := make([]rune, 0, len(ids))
	for _, k := range ids {
		if k < 1 || k > len(r.config.Alphabet) {
			return "", audio.FrontEndReport{}, fmt.Errorf("asr: decoded class %d outside the alphabet", k)
		}
		out = append(out, r.config.Alphabet[k-1])
	}
	return string(out), front, nil
}

// Evaluate transcribes every utterance and aggregates: the character error
// rate over the concatenated counts (the summed edits divided by the summed
// reference length, not the mean of the per-utterance rates), the word error
// rate aggregated the same way over whitespace-separated tokens, the share of
// utterances decoded exactly, the number of utterances and the front end the
// features came from. An empty set reports zero samples and undefined rates.
func (r *Recognizer) Evaluate(ctx context.Context, utterances []Utterance) (EvalReport, error) {
	var zero EvalReport
	if r == nil || r.trainer == nil {
		return zero, errors.New("asr: nil recognizer")
	}
	var charSubs, charDels, charIns, charRef int
	var wordSubs, wordDels, wordIns, wordRef int
	var exact, frames int
	for i, u := range utterances {
		hypothesis, front, err := r.transcribe(ctx, u.Samples)
		if err != nil {
			return zero, fmt.Errorf("asr: utterance %d: %w", i, err)
		}
		frames += front.Frames
		c := ocr.CER(u.Text, hypothesis)
		charSubs += c.Substitutions
		charDels += c.Deletions
		charIns += c.Insertions
		charRef += c.ReferenceLength
		w := ocr.WER(u.Text, hypothesis)
		wordSubs += w.Substitutions
		wordDels += w.Deletions
		wordIns += w.Insertions
		wordRef += w.ReferenceLength
		if hypothesis == u.Text {
			exact++
		}
	}
	report := EvalReport{
		CER:     editTotals(charRef, charSubs, charDels, charIns),
		WER:     editTotals(wordRef, wordSubs, wordDels, wordIns),
		Samples: len(utterances),
		FrontEnd: audio.FrontEndReport{
			Version:    audio.FrontEndVersion,
			SampleRate: r.config.SampleRate,
			Config:     r.config.FrontEnd,
			Frames:     frames,
		},
	}
	if len(utterances) > 0 {
		report.ExactUtterances = float64(exact) / float64(len(utterances))
	}
	return report, nil
}

// editTotals packages summed edit counts into one ocr.EditReport, keeping the
// ocr definition: the rate is undefined for an empty reference rather than a
// division by zero.
func editTotals(refLen, subs, dels, ins int) ocr.EditReport {
	r := ocr.EditReport{
		Substitutions:   subs,
		Deletions:       dels,
		Insertions:      ins,
		ReferenceLength: refLen,
	}
	if refLen > 0 {
		r.Rate = float64(subs+dels+ins) / float64(refLen)
		r.Defined = true
	}
	return r
}

// Snapshot returns the trainer's snapshot: the complete model, parameters,
// optimizer state and update count at an update boundary.
func (r *Recognizer) Snapshot() learning.TrainingSnapshot {
	if r == nil {
		return learning.TrainingSnapshot{}
	}
	return r.trainer.Snapshot()
}
