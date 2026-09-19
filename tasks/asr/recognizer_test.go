package asr_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// synthConfig is the fixture voice: 16 kHz, 120 ms per character, 40 ms of
// silence between characters and a faint uniform noise floor.
func synthConfig() asr.SynthConfig {
	return asr.SynthConfig{SampleRate: 16000, DurationMS: 120, GapMS: 40, Noise: 0.01, Alphabet: []rune{'a', 'b', 'c'}}
}

// trainingEpochs is how many passes over the training set the fixture takes.
// The first few passes only move the loss off its starting plateau, so the
// held-out rates still wander there; by this point the run has settled.
const trainingEpochs = 10

// recognizerConfig is the fixture model: the 12-band log-mel front end over
// 256-sample windows and 24 recurrent nodes.
func recognizerConfig() asr.RecognizerConfig {
	return asr.RecognizerConfig{
		SampleRate:   16000,
		FrontEnd:     audio.FrontEndConfig{Window: 256, Hop: 128, Mels: 12, FMin: 0, FMax: 8000, Floor: 1e-10},
		Hidden:       24,
		Alphabet:     []rune{'a', 'b', 'c'},
		LearningRate: 0.02,
		Seed:         5,
	}
}

func TestSynthesizeIsDeterministicAndBounded(t *testing.T) {
	c := synthConfig()
	first, err := asr.Synthesize(c, "abc", 7)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	second, err := asr.Synthesize(c, "abc", 7)
	if err != nil {
		t.Fatalf("Synthesize again: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two runs of Synthesize with the same arguments differ")
	}

	// 3 characters of 120 ms plus 2 gaps of 40 ms at 16 kHz.
	const duration, gap = 16000 * 120 / 1000, 16000 * 40 / 1000
	if want := 3*duration + 2*gap; len(first) != want {
		t.Fatalf("Synthesize returned %d samples, want %d", len(first), want)
	}
	for i, v := range first {
		if math.IsNaN(v) || v < -1 || v > 1 {
			t.Fatalf("sample %d is %v, outside [-1, 1]", i, v)
		}
	}

	if _, err := asr.Synthesize(c, "az", 7); err == nil {
		t.Fatal("Synthesize accepted a rune outside the alphabet")
	}
}

func TestFeaturesShape(t *testing.T) {
	c := synthConfig()
	x, err := asr.Synthesize(c, "ab", 3)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	features, report, err := r.Features(x)
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	want := 1 + (len(x)-256)/128
	if len(features) != want {
		t.Fatalf("got %d frames, want %d", len(features), want)
	}
	for i, row := range features {
		if len(row) != 12 {
			t.Fatalf("frame %d has %d mel values, want 12", i, len(row))
		}
	}
	if report.Version != audio.FrontEndVersion {
		t.Fatalf("report version %q, want %q", report.Version, audio.FrontEndVersion)
	}
	if report.Frames != len(features) {
		t.Fatalf("report declares %d frames, matrix has %d", report.Frames, len(features))
	}
}

// trainFixture runs the shared fixture: 40 training utterances, 15 held-out
// ones, and the evaluation before and after `epochs` passes over the training
// set.
func trainFixture(t *testing.T, epochs int) (before, after asr.EvalReport) {
	t.Helper()
	ctx := context.Background()
	c := synthConfig()
	train, err := asr.Generate(c, 40, 1, 3, 1)
	if err != nil {
		t.Fatalf("Generate train: %v", err)
	}
	test, err := asr.Generate(c, 15, 1, 3, 2)
	if err != nil {
		t.Fatalf("Generate test: %v", err)
	}
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	before, err = r.Evaluate(ctx, test)
	if err != nil {
		t.Fatalf("Evaluate before: %v", err)
	}
	for epoch := 0; epoch < epochs; epoch++ {
		for i, u := range train {
			rep, err := r.TrainUtterance(ctx, u.Samples, u.Text)
			if err != nil {
				t.Fatalf("TrainUtterance epoch %d utterance %d: %v", epoch, i, err)
			}
			if math.IsNaN(rep.Loss) || math.IsInf(rep.Loss, 0) {
				t.Fatalf("epoch %d utterance %d loss %v is not finite", epoch, i, rep.Loss)
			}
		}
	}
	after, err = r.Evaluate(ctx, test)
	if err != nil {
		t.Fatalf("Evaluate after: %v", err)
	}
	return before, after
}

func TestRecognizerTrainingLowersCER(t *testing.T) {
	before, after := trainFixture(t, trainingEpochs)
	t.Logf("held-out CER %.4f -> %.4f (WER %.4f -> %.4f, exact %.4f -> %.4f over %d utterances)",
		before.CER.Rate, after.CER.Rate, before.WER.Rate, after.WER.Rate,
		before.ExactUtterances, after.ExactUtterances, after.Samples)
	if !before.CER.Defined || !after.CER.Defined {
		t.Fatal("the held-out set has no reference characters")
	}
	if !(after.CER.Rate < before.CER.Rate) {
		t.Fatalf("CER did not fall: %v before, %v after", before.CER.Rate, after.CER.Rate)
	}
}

func TestRecognizerIsReproducible(t *testing.T) {
	firstBefore, firstAfter := trainFixture(t, trainingEpochs)
	secondBefore, secondAfter := trainFixture(t, trainingEpochs)
	if !reflect.DeepEqual(firstBefore, secondBefore) {
		t.Fatalf("untrained reports differ:\n%+v\n%+v", firstBefore, secondBefore)
	}
	if !reflect.DeepEqual(firstAfter, secondAfter) {
		t.Fatalf("trained reports differ:\n%+v\n%+v", firstAfter, secondAfter)
	}
}

func TestRecognizerRejects(t *testing.T) {
	ctx := context.Background()
	c := synthConfig()
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}

	x, err := asr.Synthesize(c, "ab", 11)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if _, err := r.TrainUtterance(ctx, x, "az"); err == nil {
		t.Fatal("TrainUtterance accepted a rune outside the alphabet")
	}
	if _, err := r.Transcribe(ctx, nil); err == nil {
		t.Fatal("Transcribe accepted an empty signal")
	}
	if _, _, err := r.Features(nil); err == nil {
		t.Fatal("Features accepted an empty signal")
	}
	if _, err := r.TrainUtterance(ctx, nil, "ab"); err == nil {
		t.Fatal("TrainUtterance accepted an empty signal")
	}

	synthCases := []struct {
		name   string
		mutate func(*asr.SynthConfig)
	}{
		{"sample rate", func(s *asr.SynthConfig) { s.SampleRate = 0 }},
		{"duration", func(s *asr.SynthConfig) { s.DurationMS = 0 }},
		{"gap", func(s *asr.SynthConfig) { s.GapMS = -1 }},
		{"noise", func(s *asr.SynthConfig) { s.Noise = 1.5 }},
		{"empty alphabet", func(s *asr.SynthConfig) { s.Alphabet = nil }},
		{"repeated alphabet", func(s *asr.SynthConfig) { s.Alphabet = []rune{'a', 'a'} }},
	}
	for _, tc := range synthCases {
		bad := synthConfig()
		tc.mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("SynthConfig.Validate accepted a bad %s", tc.name)
		}
		if _, err := asr.Synthesize(bad, "ab", 1); err == nil {
			t.Fatalf("Synthesize accepted a bad %s", tc.name)
		}
	}

	recognizerCases := []struct {
		name   string
		mutate func(*asr.RecognizerConfig)
	}{
		{"sample rate", func(rc *asr.RecognizerConfig) { rc.SampleRate = 0 }},
		{"window", func(rc *asr.RecognizerConfig) { rc.FrontEnd.Window = 100 }},
		{"hop", func(rc *asr.RecognizerConfig) { rc.FrontEnd.Hop = 0 }},
		{"band", func(rc *asr.RecognizerConfig) { rc.FrontEnd.FMax = 16000 }},
		{"hidden", func(rc *asr.RecognizerConfig) { rc.Hidden = 1 }},
		{"empty alphabet", func(rc *asr.RecognizerConfig) { rc.Alphabet = nil }},
		{"repeated alphabet", func(rc *asr.RecognizerConfig) { rc.Alphabet = []rune{'a', 'b', 'a'} }},
		{"learning rate", func(rc *asr.RecognizerConfig) { rc.LearningRate = 0 }},
		{"infinite learning rate", func(rc *asr.RecognizerConfig) { rc.LearningRate = math.Inf(1) }},
	}
	for _, tc := range recognizerCases {
		bad := recognizerConfig()
		tc.mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("RecognizerConfig.Validate accepted a bad %s", tc.name)
		}
		if _, err := asr.NewRecognizer(bad); err == nil {
			t.Fatalf("NewRecognizer accepted a bad %s", tc.name)
		}
	}

	if _, err := asr.Generate(c, 3, 0, 2, 1); err == nil {
		t.Fatal("Generate accepted a zero minimum length")
	}
	if _, err := asr.Generate(c, 3, 3, 2, 1); err == nil {
		t.Fatal("Generate accepted a maximum length below the minimum")
	}
}
