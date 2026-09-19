package ocr_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

// The single-line recognizer fixture: three letters, one input node per image
// row, a small recurrent core and the seeds the ticket fixes.
const (
	recognizerHeight = 8
	recognizerHidden = 24
	recognizerRate   = .05
	recognizerSeed   = 3
	recognizerEpochs = 3
	recognizerFamily = 1
	trainTextSeed    = 1
	testTextSeed     = 2
)

// recognizerAlphabet returns a fresh copy of the fixture alphabet, so a test
// that mutates it cannot leak into another.
func recognizerAlphabet() []rune { return []rune{'a', 'b', 'c'} }

// recognizerConfig is the fixture configuration every test in this file uses.
func recognizerConfig() ocr.RecognizerConfig {
	return ocr.RecognizerConfig{
		Height:       recognizerHeight,
		Hidden:       recognizerHidden,
		Alphabet:     recognizerAlphabet(),
		LearningRate: recognizerRate,
		Seed:         recognizerSeed,
	}
}

// newRecognizer builds the fixture recognizer or fails the test.
func newRecognizer(t *testing.T) *ocr.Recognizer {
	t.Helper()
	r, err := ocr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	return r
}

// renderOptions is the rendering used by every fixture line: a visible
// background and no noise, so a failure is never a sampling artefact.
func renderOptions() glyphs.Options { return glyphs.Options{Background: .1} }

// recognizerSamples draws n labelled lines of length 1..3 from one family, so
// the train and test splits share the glyph shapes and differ only in text.
func recognizerSamples(t *testing.T, n int, textSeed uint64) []glyphs.Sample {
	t.Helper()
	s, err := glyphs.Generate(recognizerAlphabet(), n, 1, 3, glyphs.Family{Seed: recognizerFamily}, renderOptions(), textSeed)
	if err != nil {
		t.Fatalf("glyphs.Generate: %v", err)
	}
	if len(s) != n {
		t.Fatalf("glyphs.Generate returned %d samples, want %d", len(s), n)
	}
	return s
}

// TestRecognizerColumnsShape pins the input sequence: one step per image
// column, Height values per step, taken from the image's own column.
func TestRecognizerColumnsShape(t *testing.T) {
	r := newRecognizer(t)
	family := glyphs.Family{Seed: recognizerFamily}
	img, _, err := glyphs.Render("ab", family, renderOptions())
	if err != nil {
		t.Fatalf("glyphs.Render: %v", err)
	}
	if img.Width != 17 || img.Height != recognizerHeight {
		t.Fatalf("Render(\"ab\") is %dx%d, want 17x%d", img.Width, img.Height, recognizerHeight)
	}
	cols, err := r.Columns(img)
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(cols) != 17 {
		t.Fatalf("Columns returned %d steps, want 17", len(cols))
	}
	for x, col := range cols {
		if len(col) != recognizerHeight {
			t.Fatalf("column %d has %d values, want %d", x, len(col), recognizerHeight)
		}
	}
	// Inside a glyph box the column carries ink 1 and background elsewhere,
	// which is what the recognizer has to separate.
	glyph := family.Glyph('a')
	for x := 0; x < 8; x++ {
		for y := 0; y < recognizerHeight; y++ {
			want := renderOptions().Background
			if glyph[y][x] == 1 {
				want = 1
			}
			if cols[x][y] != want {
				t.Fatalf("column %d row %d = %g, want %g", x, y, cols[x][y], want)
			}
		}
	}
	// Column 8 is the blank column Render leaves between the two glyphs: it
	// carries no ink and one constant value in every row. Render only writes
	// the glyph boxes, so that constant is the zero fill, not
	// Options.Background; asserting Background here would be asserting a
	// change to glyphs that this ticket does not own.
	for y := 0; y < recognizerHeight; y++ {
		if cols[8][y] != 0 {
			t.Fatalf("gap column row %d = %g, want the constant 0 Render leaves between glyphs", y, cols[8][y])
		}
	}
}

// TestRecognizerTrainingLowersCER is the learning gate: CTC training on one
// split must lower the character error rate measured on a held-out split, and
// every step's loss must stay finite.
func TestRecognizerTrainingLowersCER(t *testing.T) {
	ctx := context.Background()
	r := newRecognizer(t)
	train := recognizerSamples(t, 60, trainTextSeed)
	test := recognizerSamples(t, 20, testTextSeed)

	before, err := r.Evaluate(ctx, test)
	if err != nil {
		t.Fatalf("Evaluate before training: %v", err)
	}
	if before.Samples != len(test) {
		t.Fatalf("Evaluate reported %d samples, want %d", before.Samples, len(test))
	}
	if !before.CER.Defined {
		t.Fatal("CER is undefined before training, so the split has no reference text")
	}

	impossible := 0
	for epoch := 0; epoch < recognizerEpochs; epoch++ {
		for i, s := range train {
			rep, err := r.TrainLine(ctx, s.Image, s.Text)
			if err != nil {
				t.Fatalf("TrainLine epoch %d sample %d: %v", epoch, i, err)
			}
			if math.IsNaN(rep.Loss) || math.IsInf(rep.Loss, 0) {
				t.Fatalf("epoch %d sample %d loss is not finite: %v", epoch, i, rep.Loss)
			}
			if rep.Impossible {
				impossible++
			}
		}
	}
	if impossible != 0 {
		t.Fatalf("%d training lines had no valid CTC alignment, the fixture should have none", impossible)
	}

	after, err := r.Evaluate(ctx, test)
	if err != nil {
		t.Fatalf("Evaluate after training: %v", err)
	}
	t.Logf("held-out CER before %.4f (%d edits / %d chars), after %.4f (%d edits / %d chars)",
		before.CER.Rate, before.CER.Substitutions+before.CER.Deletions+before.CER.Insertions, before.CER.ReferenceLength,
		after.CER.Rate, after.CER.Substitutions+after.CER.Deletions+after.CER.Insertions, after.CER.ReferenceLength)
	t.Logf("exact lines before %.4f, after %.4f", before.ExactLines, after.ExactLines)
	if !(after.CER.Rate < before.CER.Rate) {
		t.Fatalf("CER did not fall: before %.4f, after %.4f", before.CER.Rate, after.CER.Rate)
	}
}

// TestRecognizerImpossibleIsCountedNotFatal pins the impossible-target policy:
// a line too short to align is reported, not an error, and it leaves the
// trainer bit for bit as it was.
func TestRecognizerImpossibleIsCountedNotFatal(t *testing.T) {
	ctx := context.Background()
	r := newRecognizer(t)
	before := r.Snapshot()
	// "aaa" needs a blank between each repeated label, so it needs 5 frames;
	// a 3-column image cannot carry any alignment.
	img := glyphs.Image{Width: 3, Height: recognizerHeight, Pixels: make([]float64, 3*recognizerHeight)}
	rep, err := r.TrainLine(ctx, img, "aaa")
	if err != nil {
		t.Fatalf("TrainLine on an impossible line: %v", err)
	}
	if !rep.Impossible {
		t.Fatal("TrainLine did not report the impossible target")
	}
	if rep.Frames != 3 || rep.Labels != 3 {
		t.Fatalf("report frames %d labels %d, want 3 and 3", rep.Frames, rep.Labels)
	}
	if rep.Loss != 0 {
		t.Fatalf("impossible target reported loss %g, want 0", rep.Loss)
	}
	after := r.Snapshot()
	if after.Updates != before.Updates {
		t.Fatalf("update counter moved from %d to %d", before.Updates, after.Updates)
	}
	if !sameBits(before.Parameters, after.Parameters) {
		t.Fatal("the skipped step changed parameter bits")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("the skipped step changed the training snapshot")
	}
}

// TestRecognizerRejects covers the refusals: text outside the alphabet, an
// image whose height does not match the input nodes, and every invalid
// configuration field.
func TestRecognizerRejects(t *testing.T) {
	ctx := context.Background()
	r := newRecognizer(t)

	img, _, err := glyphs.Render("ab", glyphs.Family{Seed: recognizerFamily}, renderOptions())
	if err != nil {
		t.Fatalf("glyphs.Render: %v", err)
	}
	if _, err := r.TrainLine(ctx, img, "abd"); err == nil {
		t.Fatal("TrainLine accepted a rune outside the alphabet")
	}
	wrong := glyphs.Image{Width: 4, Height: recognizerHeight - 1, Pixels: make([]float64, 4*(recognizerHeight-1))}
	if _, err := r.Columns(wrong); err == nil {
		t.Fatal("Columns accepted an image of the wrong height")
	}
	if _, err := r.TrainLine(ctx, wrong, "ab"); err == nil {
		t.Fatal("TrainLine accepted an image of the wrong height")
	}
	if _, err := r.Infer(ctx, wrong); err == nil {
		t.Fatal("Infer accepted an image of the wrong height")
	}

	for name, mutate := range map[string]func(*ocr.RecognizerConfig){
		"height zero":        func(c *ocr.RecognizerConfig) { c.Height = 0 },
		"height negative":    func(c *ocr.RecognizerConfig) { c.Height = -1 },
		"hidden one":         func(c *ocr.RecognizerConfig) { c.Hidden = 1 },
		"hidden negative":    func(c *ocr.RecognizerConfig) { c.Hidden = -3 },
		"alphabet empty":     func(c *ocr.RecognizerConfig) { c.Alphabet = nil },
		"alphabet duplicate": func(c *ocr.RecognizerConfig) { c.Alphabet = []rune{'a', 'b', 'a'} },
		"rate zero":          func(c *ocr.RecognizerConfig) { c.LearningRate = 0 },
		"rate negative":      func(c *ocr.RecognizerConfig) { c.LearningRate = -.05 },
		"rate nan":           func(c *ocr.RecognizerConfig) { c.LearningRate = math.NaN() },
	} {
		c := recognizerConfig()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("Validate accepted %s", name)
		}
		if _, err := ocr.NewRecognizer(c); err == nil {
			t.Fatalf("NewRecognizer accepted %s", name)
		}
	}
	if err := recognizerConfig().Validate(); err != nil {
		t.Fatalf("Validate rejected the fixture configuration: %v", err)
	}
}

// TestRecognizerIsDeterministic pins that the same configuration and the same
// lines produce the same trained model, so the evidence is reproducible.
func TestRecognizerIsDeterministic(t *testing.T) {
	ctx := context.Background()
	train := recognizerSamples(t, 60, trainTextSeed)[:12]
	run := func() learning.TrainingSnapshot {
		r := newRecognizer(t)
		for i, s := range train {
			if _, err := r.TrainLine(ctx, s.Image, s.Text); err != nil {
				t.Fatalf("TrainLine sample %d: %v", i, err)
			}
		}
		return r.Snapshot()
	}
	first, second := run(), run()
	if first.Updates == 0 {
		t.Fatal("the run applied no update, so determinism is vacuous")
	}
	if !sameBits(first.Parameters, second.Parameters) {
		t.Fatal("two identical runs produced different parameter bits")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two identical runs produced different snapshots")
	}
}

// sameBits compares two parameter sets bit for bit, so a sign of zero or a
// NaN payload cannot pass as unchanged.
func sameBits(a, b learning.Parameters) bool {
	groups := [][2][]float64{
		{a.Core.Weights, b.Core.Weights},
		{a.Core.Bias, b.Core.Bias},
		{a.Core.LogTau, b.Core.LogTau},
		{a.ThetaRaw, b.ThetaRaw},
		{a.Encoder, b.Encoder},
		{a.Readout, b.Readout},
	}
	for _, g := range groups {
		if len(g[0]) != len(g[1]) {
			return false
		}
		for i := range g[0] {
			if math.Float64bits(g[0][i]) != math.Float64bits(g[1][i]) {
				return false
			}
		}
	}
	return true
}
