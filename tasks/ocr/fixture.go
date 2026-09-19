package ocr

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// OCRFixtureSchemaVersion identifies the report RunOCRFixture writes.
const OCRFixtureSchemaVersion = "coimnet-ocr-fixture/v1"

// fixtureHeight is the image height every fixture line has, because
// glyphs.Render always draws 8 rows; it is also the recognizer's input size.
const fixtureHeight = 8

// testTextOffset separates the test split's text generator from the training
// split's, so the two splits of one seed never draw the same line sequence.
const testTextOffset = 1000

// FixtureConfig is one complete OCR fixture run: the seeds to repeat it over,
// the alphabet and line lengths to draw, how many lines each split holds, the
// recognizer's size and learning rate, one glyph family per split and the
// rendering. TrainFamily and TestFamily must differ, so the shapes a run is
// scored on are never the shapes it trained on.
type FixtureConfig struct {
	Seeds        []uint64 `json:"seeds"`
	Alphabet     []rune   `json:"alphabet"`
	TrainSamples int      `json:"train_samples"`
	TestSamples  int      `json:"test_samples"`
	MinLen       int      `json:"min_len"`
	MaxLen       int      `json:"max_len"`
	Epochs       int      `json:"epochs"`
	Hidden       int      `json:"hidden"`
	LearningRate float64  `json:"learning_rate"`
	TrainFamily  uint64   `json:"train_family"`
	TestFamily   uint64   `json:"test_family"`
	Background   float64  `json:"background"`
	Noise        float64  `json:"noise"`
}

// Validate reports the first field that cannot run a fixture: no seed, an
// alphabet below two runes or with a repeat, an empty split, a line length
// below 1 or inverted, no epoch, Hidden below 2, a learning rate that is not a
// positive finite number, or one glyph family used for both splits.
func (c FixtureConfig) Validate() error {
	if len(c.Seeds) < 1 {
		return errors.New("ocr: fixture has no seed")
	}
	if len(c.Alphabet) < 2 {
		return fmt.Errorf("ocr: alphabet has %d runes, want at least 2", len(c.Alphabet))
	}
	seen := make(map[rune]bool, len(c.Alphabet))
	for i, r := range c.Alphabet {
		if seen[r] {
			return fmt.Errorf("ocr: alphabet repeats %q at position %d", r, i)
		}
		seen[r] = true
	}
	if c.TrainSamples < 1 {
		return fmt.Errorf("ocr: train samples %d must be at least 1", c.TrainSamples)
	}
	if c.TestSamples < 1 {
		return fmt.Errorf("ocr: test samples %d must be at least 1", c.TestSamples)
	}
	if c.MinLen < 1 {
		return fmt.Errorf("ocr: minimum length %d must be at least 1", c.MinLen)
	}
	if c.MaxLen < c.MinLen {
		return fmt.Errorf("ocr: maximum length %d is below the minimum %d", c.MaxLen, c.MinLen)
	}
	if c.Epochs < 1 {
		return fmt.Errorf("ocr: epochs %d must be at least 1", c.Epochs)
	}
	if c.Hidden < 2 {
		return fmt.Errorf("ocr: hidden %d must be at least 2", c.Hidden)
	}
	if !(c.LearningRate > 0) || math.IsInf(c.LearningRate, 1) {
		return fmt.Errorf("ocr: learning rate %v must be positive and finite", c.LearningRate)
	}
	if c.TrainFamily == c.TestFamily {
		return fmt.Errorf("ocr: both splits use glyph family %d, so the shapes would leak", c.TrainFamily)
	}
	return nil
}

// DataScope states what the fixture's numbers were measured on, so a reader
// cannot mistake a fixture score for a claim about real documents.
type DataScope struct {
	Data           string `json:"data"`
	Language       string `json:"language"`
	Resolution     string `json:"resolution"`
	SequenceLength string `json:"sequence_length"`
	Periphery      string `json:"periphery"`
}

// scope fills the fixed scope text with this configuration's alphabet size and
// line lengths.
func (c FixtureConfig) scope() DataScope {
	return DataScope{
		Data:           "synthetic 8x8 pseudo-glyphs rendered per (family, rune); no real fonts, no scanned documents",
		Language:       fmt.Sprintf("an alphabet of %d runes with no natural-language statistics", len(c.Alphabet)),
		Resolution:     "8 rows by 9*len-1 columns, grayscale in [0,1]",
		SequenceLength: fmt.Sprintf("%d..%d glyphs per line", c.MinLen, c.MaxLen),
		Periphery:      "identity encoder, linear per-column readout, greedy CTC decoding",
	}
}

// FixtureSeedResult is one seed of the fixture: the held-out scores before and
// after training, the same scores restricted to the test lines whose text never
// appeared in training, how many training targets had no valid CTC alignment
// and were therefore skipped, and the failure if the seed did not finish.
type FixtureSeedResult struct {
	Seed        uint64     `json:"seed"`
	Before      EvalReport `json:"before"`
	After       EvalReport `json:"after"`
	UnseenAfter EvalReport `json:"unseen_after"`
	Impossible  int        `json:"impossible"`
	Failed      bool       `json:"failed"`
	Error       string     `json:"error,omitempty"`
}

// CoreDisconnectReport is the check that no output bypasses the core: with
// every core weight set to 0 the same image must produce different logits.
// MaxAbsDelta is the largest absolute difference over every column and class.
type CoreDisconnectReport struct {
	OutputChanged bool    `json:"output_changed"`
	MaxAbsDelta   float64 `json:"max_abs_delta"`
}

// OCRReport is one complete fixture run: the configuration and its hash, the
// scope the numbers were measured on, one result per seed, the core-disconnect
// check and what the scores do not prove.
type OCRReport struct {
	SchemaVersion  string               `json:"schema_version"`
	Config         FixtureConfig        `json:"config"`
	ConfigHash     string               `json:"config_hash"`
	DataScope      DataScope            `json:"data_scope"`
	Seeds          []FixtureSeedResult  `json:"seeds"`
	CoreDisconnect CoreDisconnectReport `json:"core_disconnect"`
	Assumptions    []string             `json:"assumptions"`
}

// RunOCRFixture trains and scores one recognizer per seed and returns the
// report. Each seed draws its training lines from TrainFamily and its test
// lines from TestFamily, evaluates the untrained recognizer on the test split,
// runs Epochs passes over the training split and evaluates again, on the whole
// test split and on the test lines whose text never appeared in training. A
// seed that fails is recorded with Failed and its error and the run continues;
// a cancelled context fails the whole run, because a half-finished fixture is
// not evidence. The core-disconnect check runs on the first seed that finished.
func RunOCRFixture(ctx context.Context, c FixtureConfig) (OCRReport, error) {
	var zero OCRReport
	if err := c.Validate(); err != nil {
		return zero, err
	}
	c.Alphabet = append([]rune(nil), c.Alphabet...)
	c.Seeds = append([]uint64(nil), c.Seeds...)
	hash, err := configHash(c)
	if err != nil {
		return zero, err
	}
	report := OCRReport{
		SchemaVersion: OCRFixtureSchemaVersion,
		Config:        c,
		ConfigHash:    hash,
		DataScope:     c.scope(),
		Seeds:         make([]FixtureSeedResult, 0, len(c.Seeds)),
		Assumptions:   fixtureAssumptions(),
	}
	checked := false
	for _, seed := range c.Seeds {
		if err := ctx.Err(); err != nil {
			return zero, fmt.Errorf("ocr: fixture stopped before seed %d: %w", seed, err)
		}
		result, trained, test, err := runFixtureSeed(ctx, c, seed)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return zero, fmt.Errorf("ocr: fixture stopped during seed %d: %w", seed, ctxErr)
			}
			result = FixtureSeedResult{Seed: seed, Failed: true, Error: err.Error()}
		}
		report.Seeds = append(report.Seeds, result)
		if result.Failed || checked {
			continue
		}
		disconnect, err := coreDisconnect(ctx, trained, test[0].Image)
		if err != nil {
			return zero, fmt.Errorf("ocr: core disconnect check on seed %d: %w", seed, err)
		}
		report.CoreDisconnect = disconnect
		checked = true
	}
	return report, nil
}

// runFixtureSeed runs one seed end to end and returns its result together with
// the trained recognizer and the test split, which the core-disconnect check
// reuses.
func runFixtureSeed(ctx context.Context, c FixtureConfig, seed uint64) (FixtureSeedResult, *Recognizer, []glyphs.Sample, error) {
	o := glyphs.Options{Background: c.Background, Noise: c.Noise}
	train, err := glyphs.Generate(c.Alphabet, c.TrainSamples, c.MinLen, c.MaxLen, glyphs.Family{Seed: c.TrainFamily}, o, seed)
	if err != nil {
		return FixtureSeedResult{}, nil, nil, fmt.Errorf("training split: %w", err)
	}
	test, err := glyphs.Generate(c.Alphabet, c.TestSamples, c.MinLen, c.MaxLen, glyphs.Family{Seed: c.TestFamily}, o, seed+testTextOffset)
	if err != nil {
		return FixtureSeedResult{}, nil, nil, fmt.Errorf("test split: %w", err)
	}
	r, err := NewRecognizer(RecognizerConfig{
		Height:       fixtureHeight,
		Hidden:       c.Hidden,
		Alphabet:     c.Alphabet,
		LearningRate: c.LearningRate,
		Seed:         seed,
	})
	if err != nil {
		return FixtureSeedResult{}, nil, nil, err
	}
	before, err := r.Evaluate(ctx, test)
	if err != nil {
		return FixtureSeedResult{}, nil, nil, fmt.Errorf("evaluate before training: %w", err)
	}
	impossible := 0
	for epoch := 0; epoch < c.Epochs; epoch++ {
		for i, s := range train {
			rep, err := r.TrainLine(ctx, s.Image, s.Text)
			if err != nil {
				return FixtureSeedResult{}, nil, nil, fmt.Errorf("epoch %d line %d: %w", epoch, i, err)
			}
			if rep.Impossible {
				impossible++
			}
		}
	}
	after, err := r.Evaluate(ctx, test)
	if err != nil {
		return FixtureSeedResult{}, nil, nil, fmt.Errorf("evaluate after training: %w", err)
	}
	// The unseen split is the held-out lines whose text never occurred in
	// training; an empty one is reported as zero samples, not evaluated.
	unseenAfter := EvalReport{Samples: 0}
	if unseen := glyphs.UnseenCombinations(train, test); len(unseen) > 0 {
		unseenAfter, err = r.Evaluate(ctx, unseen)
		if err != nil {
			return FixtureSeedResult{}, nil, nil, fmt.Errorf("evaluate unseen combinations: %w", err)
		}
	}
	return FixtureSeedResult{Seed: seed, Before: before, After: after, UnseenAfter: unseenAfter, Impossible: impossible}, r, test, nil
}

// coreDisconnect reads one image with the trained recognizer and with a copy
// whose core weights are all 0, and reports the largest absolute difference
// between the two logit sequences. Nothing but the core connects the input
// nodes to the readout, so an unchanged output would mean the output bypassed
// the core.
func coreDisconnect(ctx context.Context, r *Recognizer, img glyphs.Image) (CoreDisconnectReport, error) {
	var zero CoreDisconnectReport
	intact, err := r.logits(ctx, img)
	if err != nil {
		return zero, err
	}
	p := r.Parameters()
	for i := range p.Core.Weights {
		p.Core.Weights[i] = 0
	}
	cut, err := NewRecognizerWith(r.config, p)
	if err != nil {
		return zero, err
	}
	severed, err := cut.logits(ctx, img)
	if err != nil {
		return zero, err
	}
	if len(intact) != len(severed) {
		return zero, fmt.Errorf("ocr: %d columns with the core, %d without", len(intact), len(severed))
	}
	max := 0.
	for t := range intact {
		if len(intact[t]) != len(severed[t]) {
			return zero, fmt.Errorf("ocr: column %d has %d classes with the core, %d without", t, len(intact[t]), len(severed[t]))
		}
		for k := range intact[t] {
			if d := math.Abs(intact[t][k] - severed[t][k]); d > max {
				max = d
			}
		}
	}
	return CoreDisconnectReport{OutputChanged: max > 0, MaxAbsDelta: max}, nil
}

// configHash is the hex-encoded sha256 of the configuration's JSON, so a
// report can be matched to the configuration that produced it.
func configHash(c FixtureConfig) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("ocr: hashing the fixture configuration: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// fixtureAssumptions states what the scores do and do not prove; every report
// carries them, so no reader can take a fixture score for an OCR claim.
func fixtureAssumptions() []string {
	return []string{
		"Scores prove that the pipeline trains and decodes on fixtures; they are not an OCR capability claim.",
		"Train and test glyph families differ, so shapes never leak; the alphabet is shared.",
	}
}
