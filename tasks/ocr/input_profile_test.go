package ocr_test

import (
	"context"
	"math"
	randv2 "math/rand/v2"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func profileBaseConfig() ocr.RecognizerConfig {
	return ocr.RecognizerConfig{
		Height:       2,
		Hidden:       4,
		Alphabet:     []rune{'a', 'b'},
		LearningRate: 0.03,
		Seed:         17,
	}
}

func profileMustRecognizer(t *testing.T, config ocr.RecognizerConfig) *ocr.Recognizer {
	t.Helper()
	r, err := ocr.NewRecognizer(config)
	if err != nil {
		t.Fatalf("NewRecognizer(%+v): %v", config, err)
	}
	return r
}

func profileMustRecognizerWith(t *testing.T, config ocr.RecognizerConfig, parameters learning.Parameters) *ocr.Recognizer {
	t.Helper()
	r, err := ocr.NewRecognizerWith(config, parameters)
	if err != nil {
		t.Fatalf("NewRecognizerWith(%+v): %v", config, err)
	}
	return r
}

func profileImage(width, height int, pixels ...float64) glyphs.Image {
	return glyphs.Image{Width: width, Height: height, Pixels: pixels}
}

func profileEdgeIndex(t *testing.T, config learning.Config, source, target int) int {
	t.Helper()
	for i := range config.Dynamics.Sources {
		if config.Dynamics.Sources[i] == source && config.Dynamics.Targets[i] == target {
			return i
		}
	}
	t.Fatalf("topology has no edge %d -> %d", source, target)
	return -1
}

func profileReadoutIndex(t *testing.T, config learning.Config, node, class int) int {
	t.Helper()
	if class < 0 || class >= config.OutputSize {
		t.Fatalf("class %d is outside output size %d", class, config.OutputSize)
	}
	for row, id := range config.ReadoutNodes {
		if id == node {
			return row*config.OutputSize + class
		}
	}
	t.Fatalf("readout has no node %d", node)
	return -1
}

// profileDelayedParameters leaves exactly one input-to-hidden edge and one
// hidden-to-A readout entry active. The topology is discovered from the
// public snapshot instead of assuming an internal edge ordering.
func profileDelayedParameters(t *testing.T, r *ocr.Recognizer) learning.Parameters {
	t.Helper()
	snapshot := r.Snapshot()
	config := snapshot.Config
	parameters := r.Parameters()
	parameters.Core.Weights = make([]float64, len(parameters.Core.Weights))
	parameters.Core.Bias = make([]float64, len(parameters.Core.Bias))
	parameters.Core.LogTau = make([]float64, len(parameters.Core.LogTau))
	for i := range parameters.Core.LogTau {
		parameters.Core.LogTau[i] = math.Log(2)
	}
	parameters.Readout = make([]float64, len(parameters.Readout))
	parameters.Encoder = make([]float64, len(parameters.Encoder))
	for i := 0; i < config.InputSize && i < len(config.InputNodes); i++ {
		parameters.Encoder[i*len(config.InputNodes)+i] = 1
	}
	inputToHidden := profileEdgeIndex(t, config, config.InputNodes[0], config.ReadoutNodes[0])
	parameters.Core.Weights[inputToHidden] = 4
	readoutA := profileReadoutIndex(t, config, config.ReadoutNodes[0], 1)
	parameters.Readout[readoutA] = 20
	return parameters
}

// profileSingleReadoutParameters makes blank overwhelmingly likely while A
// remains the largest non-blank class. A single-character decoder must ignore
// blank and select A from the final row.
func profileSingleReadoutParameters(t *testing.T, r *ocr.Recognizer) learning.Parameters {
	t.Helper()
	snapshot := r.Snapshot()
	config := snapshot.Config
	parameters := r.Parameters()
	parameters.Core.Weights = make([]float64, len(parameters.Core.Weights))
	parameters.Core.Bias = make([]float64, len(parameters.Core.Bias))
	parameters.Core.LogTau = make([]float64, len(parameters.Core.LogTau))
	for i := range parameters.Core.LogTau {
		parameters.Core.LogTau[i] = math.Log(2)
	}
	parameters.Encoder = make([]float64, len(parameters.Encoder))
	parameters.Readout = make([]float64, len(parameters.Readout))
	parameters.Core.Bias[config.ReadoutNodes[0]] = 4
	parameters.Readout[profileReadoutIndex(t, config, config.ReadoutNodes[0], 0)] = 100
	parameters.Readout[profileReadoutIndex(t, config, config.ReadoutNodes[0], 1)] = 2
	return parameters
}

func profileAssertNoPanicRejects(t *testing.T, name string, config ocr.RecognizerConfig) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("NewRecognizer %s panicked before rejecting config: %v", name, recovered)
		}
	}()
	r, err := ocr.NewRecognizer(config)
	if err == nil {
		t.Fatalf("NewRecognizer accepted invalid %s", name)
	}
	if r != nil {
		t.Fatalf("NewRecognizer returned a recognizer for invalid %s", name)
	}
}

// TestOCRInputProfileLegacyEmptyColumnsAndCTC is a legacy control. The empty
// profile must keep the pre-profile input sequence and CTC behavior.
func TestOCRInputProfileLegacyEmptyColumnsAndCTC(t *testing.T) {
	ctx := context.Background()
	image := profileImage(3, 2, 0, 1, 0.2, 1, 0, 0.7)
	implicitConfig := profileBaseConfig()
	implicit := profileMustRecognizer(t, implicitConfig)

	implicitColumns, err := implicit.Columns(image)
	if err != nil {
		t.Fatalf("implicit Columns: %v", err)
	}
	implicitInputs, err := implicit.Inputs(image)
	if err != nil {
		t.Fatalf("implicit Inputs: %v", err)
	}
	if !reflect.DeepEqual(implicitInputs, implicitColumns) {
		t.Fatalf("legacy inputs differ: implicit=%v columns=%v", implicitInputs, implicitColumns)
	}

	if _, err := implicit.Infer(ctx, image); err != nil {
		t.Fatalf("implicit Infer: %v", err)
	}
	implicitReport, err := implicit.TrainLine(ctx, image, "a")
	if err != nil {
		t.Fatalf("implicit TrainLine: %v", err)
	}
	if implicitReport.Impossible {
		t.Fatalf("legacy TrainLine unexpectedly reported impossible: %+v", implicitReport)
	}
}

// TestOCRInputProfileExplicitColumnsMatchesLegacy is a new-profile positive
// control. Explicit columns must preserve the empty-profile sequence and CTC
// result bit-for-bit once the profile is implemented.
func TestOCRInputProfileExplicitColumnsMatchesLegacy(t *testing.T) {
	ctx := context.Background()
	image := profileImage(3, 2, 0, 1, 0.2, 1, 0, 0.7)
	implicitConfig := profileBaseConfig()
	explicitConfig := implicitConfig
	explicitConfig.InputProfile = "columns"
	implicit := profileMustRecognizer(t, implicitConfig)
	explicit := profileMustRecognizer(t, explicitConfig)
	implicitColumns, err := implicit.Columns(image)
	if err != nil {
		t.Fatalf("implicit Columns: %v", err)
	}
	implicitInputs, err := implicit.Inputs(image)
	if err != nil {
		t.Fatalf("implicit Inputs: %v", err)
	}
	explicitInputs, err := explicit.Inputs(image)
	if err != nil {
		t.Fatalf("explicit columns Inputs: %v", err)
	}
	if !reflect.DeepEqual(implicitInputs, implicitColumns) || !reflect.DeepEqual(explicitInputs, implicitColumns) {
		t.Fatalf("legacy inputs differ: implicit=%v explicit=%v columns=%v", implicitInputs, explicitInputs, implicitColumns)
	}
	implicitOutput, err := implicit.Infer(ctx, image)
	if err != nil {
		t.Fatalf("implicit Infer: %v", err)
	}
	explicitOutput, err := explicit.Infer(ctx, image)
	if err != nil {
		t.Fatalf("explicit columns Infer: %v", err)
	}
	if implicitOutput != explicitOutput {
		t.Fatalf("legacy Infer differs: implicit=%q explicit=%q", implicitOutput, explicitOutput)
	}
	implicitReport, err := implicit.TrainLine(ctx, image, "a")
	if err != nil {
		t.Fatalf("implicit TrainLine: %v", err)
	}
	explicitReport, err := explicit.TrainLine(ctx, image, "a")
	if err != nil {
		t.Fatalf("explicit columns TrainLine: %v", err)
	}
	if implicitReport != explicitReport {
		t.Fatalf("legacy TrainLine report differs: implicit=%+v explicit=%+v", implicitReport, explicitReport)
	}
	if !reflect.DeepEqual(implicit.Snapshot(), explicit.Snapshot()) {
		t.Fatal("legacy empty and explicit columns snapshots differ")
	}
}

// TestOCRInputProfileAlignedColumnsDelayAndFinalColumn checks the timing
// contract with a hand-built two-hidden-node network. The last input column
// is invisible to a legacy last-step readout because zero-delay edges read
// the input node's previous output; a trailing zero step makes it visible at
// the final aligned readout row.
func TestOCRInputProfileAlignedColumnsDelayAndFinalColumn(t *testing.T) {
	ctx := context.Background()
	base := profileBaseConfig()
	base.Height = 1
	base.Hidden = 2
	base.Alphabet = []rune{'a'}
	base.InputProfile = "aligned-columns"
	aligned := profileMustRecognizer(t, base)

	zeroImage := profileImage(2, 1, 0, 0)
	lastImage := profileImage(2, 1, 0, 1)
	inputs, err := aligned.Inputs(lastImage)
	if err != nil {
		t.Fatalf("aligned Inputs: %v", err)
	}
	wantInputs := [][]float64{{0}, {1}, {0}}
	if !reflect.DeepEqual(inputs, wantInputs) {
		t.Fatalf("aligned Inputs = %v, want original columns followed by a zero step %v", inputs, wantInputs)
	}
	if len(inputs) > 1 && &inputs[0][0] == &inputs[1][0] {
		t.Fatal("aligned input rows alias each other")
	}

	parameters := profileDelayedParameters(t, aligned)
	aligned = profileMustRecognizerWith(t, base, parameters)
	legacyConfig := base
	legacyConfig.InputProfile = ""
	legacy := profileMustRecognizerWith(t, legacyConfig, parameters)

	legacyZero, err := legacy.Infer(ctx, zeroImage)
	if err != nil {
		t.Fatalf("legacy zero Infer: %v", err)
	}
	legacyLast, err := legacy.Infer(ctx, lastImage)
	if err != nil {
		t.Fatalf("legacy last-column Infer: %v", err)
	}
	if legacyZero != legacyLast {
		t.Fatalf("legacy model distinguished a final column that has not reached the final readout: zero=%q last=%q", legacyZero, legacyLast)
	}
	if legacyZero != "" {
		t.Fatalf("legacy hand-built zero image decoded %q, want blank", legacyZero)
	}

	alignedZero, err := aligned.Infer(ctx, zeroImage)
	if err != nil {
		t.Fatalf("aligned zero Infer: %v", err)
	}
	alignedLast, err := aligned.Infer(ctx, lastImage)
	if err != nil {
		t.Fatalf("aligned last-column Infer: %v", err)
	}
	if alignedZero != "" || alignedLast != "a" {
		t.Fatalf("aligned final-column decoding = zero %q, last %q; want blank and a", alignedZero, alignedLast)
	}

	before := aligned.Snapshot()
	report, err := aligned.TrainLine(ctx, lastImage, "a")
	if err != nil {
		t.Fatalf("aligned TrainLine: %v", err)
	}
	if report.Impossible || aligned.Snapshot().Updates != before.Updates+1 {
		t.Fatalf("aligned TrainLine report=%+v updates before/after=%d/%d, want one applied update", report, before.Updates, aligned.Snapshot().Updates)
	}
}

// TestOCRInputProfileAlignedFinalPixelGradientReachesCoreAndEncoder uses the
// public learning Network gradient API. Only the final aligned output row is
// seeded, so a nonzero gradient on the input-to-hidden edge and the encoder
// proves that the trailing final pixel is in the computational graph.
func TestOCRInputProfileAlignedFinalPixelGradientReachesCoreAndEncoder(t *testing.T) {
	base := profileBaseConfig()
	base.Height = 1
	base.Hidden = 2
	base.Alphabet = []rune{'a'}
	base.InputProfile = "aligned-columns"
	r := profileMustRecognizer(t, base)
	parameters := profileDelayedParameters(t, r)
	r = profileMustRecognizerWith(t, base, parameters)
	inputs, err := r.Inputs(profileImage(2, 1, 0, 1))
	if err != nil {
		t.Fatalf("aligned Inputs: %v", err)
	}
	snapshot := r.Snapshot()
	network, err := learning.NewNetwork(snapshot.Config)
	if err != nil {
		t.Fatalf("learning.NewNetwork: %v", err)
	}
	upstream := make([][]float64, len(inputs))
	for i := range upstream {
		upstream[i] = make([]float64, snapshot.Config.OutputSize)
	}
	upstream[len(upstream)-1][1] = 1
	gradient, err := network.LossGradientFrom(context.Background(), snapshot.Parameters, inputs, upstream, 0)
	if err != nil {
		t.Fatalf("LossGradientFrom: %v", err)
	}
	inputToHidden := profileEdgeIndex(t, snapshot.Config, snapshot.Config.InputNodes[0], snapshot.Config.ReadoutNodes[0])
	if math.Abs(gradient.Core.Weights[inputToHidden]) <= 1e-12 {
		t.Fatalf("final-row upstream produced no input-to-hidden gradient: edge[%d]=%g", inputToHidden, gradient.Core.Weights[inputToHidden])
	}
	if len(gradient.Encoder) != 1 || math.Abs(gradient.Encoder[0]) <= 1e-12 {
		t.Fatalf("final-row upstream produced no final-pixel encoder gradient: %v", gradient.Encoder)
	}
}

// TestOCRInputProfileSingleCharacterShapeAndSeededEncoder pins the public
// single-character shape, copied image-plus-zero propagation pair, encoder
// capacity and exact deterministic initialization. The readout keeps blank in
// its schema but single-character decoding must choose the largest non-blank
// class.
func TestOCRInputProfileSingleCharacterShapeAndSeededEncoder(t *testing.T) {
	config := profileBaseConfig()
	config.Height = 2
	config.Hidden = 3
	config.Alphabet = []rune{'a', 'b'}
	config.InputProfile = "single-character"
	config.CharacterWidth = 2
	r := profileMustRecognizer(t, config)
	image := profileImage(2, 2, 0.1, 0.2, 0.3, 0.4)
	inputs, err := r.Inputs(image)
	if err != nil {
		t.Fatalf("single-character Inputs: %v", err)
	}
	wantRow := []float64{0.1, 0.2, 0.3, 0.4}
	wantInputs := [][]float64{append([]float64(nil), wantRow...), make([]float64, len(wantRow))}
	if !reflect.DeepEqual(inputs, wantInputs) {
		t.Fatalf("single-character Inputs = %v, want complete row-major image followed by an independent zero row %v", inputs, wantInputs)
	}
	if &inputs[0][0] == &inputs[1][0] || &inputs[0][0] == &image.Pixels[0] || &inputs[1][0] == &image.Pixels[0] {
		t.Fatal("single-character Inputs did not deep-copy both rows independently")
	}
	inputs[0][0] = 99
	if inputs[1][0] != wantInputs[1][0] || image.Pixels[0] != wantRow[0] {
		t.Fatal("mutating one single-character input row changed another row or the source image")
	}

	snapshot := r.Snapshot()
	if snapshot.Config.InputSize != config.Height*config.CharacterWidth {
		t.Fatalf("single-character InputSize = %d, want %d", snapshot.Config.InputSize, config.Height*config.CharacterWidth)
	}
	if len(snapshot.Parameters.Encoder) != snapshot.Config.InputSize*config.Height {
		t.Fatalf("single-character encoder length = %d, want %d", len(snapshot.Parameters.Encoder), snapshot.Config.InputSize*config.Height)
	}
	if snapshot.Config.OutputSize != len(config.Alphabet)+1 {
		t.Fatalf("single-character OutputSize = %d, want alphabet plus blank = %d", snapshot.Config.OutputSize, len(config.Alphabet)+1)
	}
	expectedEncoder := make([]float64, len(snapshot.Parameters.Encoder))
	rng := randv2.New(randv2.NewPCG(config.Seed, 42))
	scale := 1 / math.Sqrt(float64(snapshot.Config.InputSize))
	for i := range expectedEncoder {
		expectedEncoder[i] = (2*rng.Float64() - 1) * scale
	}
	if !profileFloatBitsEqual(snapshot.Parameters.Encoder, expectedEncoder) {
		t.Fatalf("single-character encoder does not match PCG(%d,42) uniform initialization: got=%v want=%v", config.Seed, snapshot.Parameters.Encoder, expectedEncoder)
	}

	probe := profileMustRecognizerWith(t, config, profileSingleReadoutParameters(t, r))
	got, err := probe.Infer(context.Background(), image)
	if err != nil {
		t.Fatalf("single-character Infer: %v", err)
	}
	if got != "a" {
		t.Fatalf("single-character Infer = %q with blank logit 100 and A logit 2, want one non-blank A", got)
	}
}

func profileFloatBitsEqual(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
			return false
		}
	}
	return true
}

// TestOCRInputProfileSingleCharacterTrainLineContract checks invalid target
// cardinality and alphabet handling. Every refusal must leave the complete
// training snapshot bit-for-bit unchanged.
func TestOCRInputProfileSingleCharacterTrainLineContract(t *testing.T) {
	ctx := context.Background()
	config := profileBaseConfig()
	config.InputProfile = "single-character"
	config.CharacterWidth = 2
	r := profileMustRecognizer(t, config)
	image := profileImage(2, 2, 1, 0, 0, 1)
	for _, text := range []string{"", "ab", "x"} {
		before := r.Snapshot()
		if _, err := r.TrainLine(ctx, image, text); err == nil {
			t.Fatalf("single-character TrainLine accepted text %q", text)
		}
		if after := r.Snapshot(); !reflect.DeepEqual(before, after) {
			t.Fatalf("single-character TrainLine(%q) changed the snapshot", text)
		}
	}
}

// TestOCRInputProfileSingleCharacterMicroDatasetLearnsLabels is the positive
// capability control. It uses two real 2x2 image shapes with one known label
// each and verifies labels from independent Infer calls after fixed updates;
// the assertion never derives correctness from the model's reported loss.
func TestOCRInputProfileSingleCharacterMicroDatasetLearnsLabels(t *testing.T) {
	ctx := context.Background()
	config := ocr.RecognizerConfig{
		Height:           2,
		Hidden:           8,
		Alphabet:         []rune{'a', 'b'},
		LearningRate:     0.05,
		Seed:             29,
		TrainableEncoder: true,
		InputProfile:     "single-character",
		CharacterWidth:   2,
	}
	r := profileMustRecognizer(t, config)
	samples := []struct {
		image glyphs.Image
		label string
	}{
		{profileImage(2, 2, 1, 0, 0, 1), "a"},
		{profileImage(2, 2, 0, 1, 1, 0), "b"},
	}
	const steps = 400
	for step := 0; step < steps; step++ {
		sample := samples[step%len(samples)]
		report, err := r.TrainLine(ctx, sample.image, sample.label)
		if err != nil {
			t.Fatalf("TrainLine step %d label %q: %v", step, sample.label, err)
		}
		if report.Impossible {
			t.Fatalf("TrainLine step %d reported impossible for one-character target: %+v", step, report)
		}
	}
	if got := r.Snapshot().Updates; got != steps {
		t.Fatalf("micro dataset applied %d updates, want %d", got, steps)
	}
	for _, sample := range samples {
		got, err := r.Infer(ctx, sample.image)
		if err != nil {
			t.Fatalf("Infer label %q: %v", sample.label, err)
		}
		if got != sample.label {
			t.Fatalf("micro dataset Infer = %q for expected label %q", got, sample.label)
		}
	}
}

// TestOCRInputProfileValidationRejectsBeforeAllocation covers unknown and
// irrelevant profile fields, malformed dimensions, overflow, and non-finite
// configuration values. The overflow cases must return errors without a
// panic or an attempted giant allocation.
func TestOCRInputProfileValidationRejectsBeforeAllocation(t *testing.T) {
	base := profileBaseConfig()
	maxInt := int(^uint(0) >> 1)
	cases := []struct {
		name   string
		mutate func(*ocr.RecognizerConfig)
	}{
		{"unknown profile", func(c *ocr.RecognizerConfig) { c.InputProfile = "future-profile" }},
		{"irrelevant width in implicit columns", func(c *ocr.RecognizerConfig) { c.CharacterWidth = 2 }},
		{"irrelevant width in explicit columns", func(c *ocr.RecognizerConfig) { c.InputProfile = "columns"; c.CharacterWidth = 2 }},
		{"irrelevant width in aligned columns", func(c *ocr.RecognizerConfig) { c.InputProfile = "aligned-columns"; c.CharacterWidth = 2 }},
		{"single-character width zero", func(c *ocr.RecognizerConfig) { c.InputProfile = "single-character"; c.CharacterWidth = 0 }},
		{"single-character width negative", func(c *ocr.RecognizerConfig) { c.InputProfile = "single-character"; c.CharacterWidth = -1 }},
		{"single-character width overflow", func(c *ocr.RecognizerConfig) { c.InputProfile = "single-character"; c.CharacterWidth = maxInt }},
		{"architecture overflow", func(c *ocr.RecognizerConfig) {
			c.InputProfile = "single-character"
			c.CharacterWidth = 2
			c.Height = maxInt/2 + 1
			c.Hidden = 2
		}},
		{"learning rate NaN", func(c *ocr.RecognizerConfig) { c.LearningRate = math.NaN() }},
		{"learning rate positive infinity", func(c *ocr.RecognizerConfig) { c.LearningRate = math.Inf(1) }},
		{"learning rate negative infinity", func(c *ocr.RecognizerConfig) { c.LearningRate = math.Inf(-1) }},
	}
	for _, tc := range cases {
		config := base
		tc.mutate(&config)
		profileAssertNoPanicRejects(t, tc.name, config)
	}

	validConfig := base
	validConfig.InputProfile = "single-character"
	validConfig.CharacterWidth = 2
	r := profileMustRecognizer(t, validConfig)
	before := r.Snapshot()
	for _, bad := range []struct {
		name  string
		image glyphs.Image
	}{
		{"zero width", profileImage(0, 2)},
		{"wrong width", profileImage(1, 2, 0, 1)},
		{"wrong height", profileImage(2, 1, 0, 1)},
		{"pixel count mismatch", profileImage(2, 2, 0, 1, 0)},
		{"non-finite pixel", profileImage(2, 2, 0, math.NaN(), 0, 1)},
	} {
		if _, err := r.Inputs(bad.image); err == nil {
			t.Fatalf("single-character Inputs accepted %s", bad.name)
		}
		if _, err := r.Infer(context.Background(), bad.image); err == nil {
			t.Fatalf("single-character Infer accepted %s", bad.name)
		}
		if _, err := r.TrainLine(context.Background(), bad.image, "a"); err == nil {
			t.Fatalf("single-character TrainLine accepted %s", bad.name)
		}
		if after := r.Snapshot(); !reflect.DeepEqual(before, after) {
			t.Fatalf("invalid image %s changed the training snapshot", bad.name)
		}
	}
}

// TestOCRInputProfilePersistenceRoundTripsConfigAndSnapshot verifies that
// every profile, including both new modes, persists its task fields and the
// complete optimizer snapshot, then resumes the next update identically.
func TestOCRInputProfilePersistenceRoundTripsConfigAndSnapshot(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		config ocr.RecognizerConfig
		image  glyphs.Image
	}{
		{"implicit columns", profileBaseConfig(), profileImage(2, 2, 0, 1, 1, 0)},
		{"explicit columns", func() ocr.RecognizerConfig { c := profileBaseConfig(); c.InputProfile = "columns"; return c }(), profileImage(2, 2, 0, 1, 1, 0)},
		{"aligned columns", func() ocr.RecognizerConfig { c := profileBaseConfig(); c.InputProfile = "aligned-columns"; return c }(), profileImage(2, 2, 0, 1, 1, 0)},
		{"single character", func() ocr.RecognizerConfig {
			c := profileBaseConfig()
			c.InputProfile = "single-character"
			c.CharacterWidth = 2
			return c
		}(), profileImage(2, 2, 1, 0, 0, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := profileMustRecognizer(t, tc.config)
			if _, err := model.TrainLine(ctx, tc.image, "a"); err != nil {
				t.Fatalf("initial TrainLine: %v", err)
			}
			wantConfig := model.Config()
			wantSnapshot := model.Snapshot()
			wantOutput, err := model.Infer(ctx, tc.image)
			if err != nil {
				t.Fatalf("initial Infer: %v", err)
			}
			path := t.TempDir() + "/ocr-model"
			if err := model.Save(ctx, path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			loaded, err := ocr.LoadRecognizer(ctx, path)
			if err != nil {
				t.Fatalf("LoadRecognizer: %v", err)
			}
			if !reflect.DeepEqual(loaded.Config(), wantConfig) {
				t.Fatalf("loaded config = %+v, want %+v", loaded.Config(), wantConfig)
			}
			if !reflect.DeepEqual(loaded.Snapshot(), wantSnapshot) {
				t.Fatal("loaded complete snapshot differs from the saved snapshot")
			}
			gotOutput, err := loaded.Infer(ctx, tc.image)
			if err != nil {
				t.Fatalf("loaded Infer: %v", err)
			}
			if gotOutput != wantOutput {
				t.Fatalf("loaded Infer = %q, want %q", gotOutput, wantOutput)
			}
			if _, err := model.TrainLine(ctx, tc.image, "a"); err != nil {
				t.Fatalf("uninterrupted next TrainLine: %v", err)
			}
			if _, err := loaded.TrainLine(ctx, tc.image, "a"); err != nil {
				t.Fatalf("loaded next TrainLine: %v", err)
			}
			if !reflect.DeepEqual(loaded.Snapshot(), model.Snapshot()) {
				t.Fatal("loaded model diverged from uninterrupted next update")
			}
		})
	}
}

// TestOCRInputProfileSingleCharacterRejectsPage pins the page API boundary:
// page segmentation is a line-oriented operation and must reject the
// single-character profile without mutating the recognizer.
func TestOCRInputProfileSingleCharacterRejectsPage(t *testing.T) {
	config := profileBaseConfig()
	config.InputProfile = "single-character"
	config.CharacterWidth = 2
	r := profileMustRecognizer(t, config)
	page := profileImage(2, 2, 1, 1, 1, 1)
	before := r.Snapshot()
	if _, err := r.InferPage(context.Background(), page, ocr.PageOptions{Threshold: 0.5}); err == nil {
		t.Fatal("InferPage accepted the single-character profile")
	}
	if after := r.Snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatal("InferPage refusal changed the recognizer snapshot")
	}
}
