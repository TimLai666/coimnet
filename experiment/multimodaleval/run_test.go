package multimodaleval

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/multimodal/synthetic"
)

// evalConfig is the multi-seed report fixture: the shared data draw, the
// single held-out combination and the model hyper-parameters of model_test,
// with distinct training and evaluation data seeds.
func evalConfig(seeds ...uint64) Config {
	return Config{
		Data:         fixtureConfig,
		Holdout:      append([]synthetic.Label(nil), holdout...),
		TrainSeed:    1,
		EvalSeed:     2,
		Seeds:        seeds,
		Epochs:       fixtureEpochs,
		Hidden:       fixtureHidden,
		Settle:       fixtureSettle,
		LearningRate: fixtureRate,
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"empty holdout", func(c *Config) { c.Holdout = nil }},
		{"duplicate holdout", func(c *Config) {
			c.Holdout = []synthetic.Label{{Shape: 2, Colour: 1}, {Shape: 2, Colour: 1}}
		}},
		{"train seed equals eval seed", func(c *Config) { c.EvalSeed = c.TrainSeed }},
		{"empty seeds", func(c *Config) { c.Seeds = nil }},
		{"zero epochs", func(c *Config) { c.Epochs = 0 }},
		{"hidden one", func(c *Config) { c.Hidden = 1 }},
		{"zero settle", func(c *Config) { c.Settle = 0 }},
		{"zero learning rate", func(c *Config) { c.LearningRate = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := evalConfig(1, 2, 3)
			tc.mutate(&c)
			if err := c.Validate(); err == nil {
				t.Errorf("Validate() = nil, want an error")
			}
		})
	}
	t.Run("valid fixture", func(t *testing.T) {
		if err := evalConfig(1, 2, 3).Validate(); err != nil {
			t.Errorf("Validate() = %v, want nil", err)
		}
	})
}

func TestRunHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, evalConfig(1))
	if err == nil {
		t.Fatal("Run with a cancelled context returned nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
}

func TestRunUnseenRetrievalUsesTheWholeGallery(t *testing.T) {
	c := evalConfig(1)
	report, err := Run(context.Background(), c)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(report.Runs))
	}
	run := report.Runs[0]
	if run.Failed {
		t.Fatalf("seed %d failed: %s", run.Seed, run.Error)
	}

	samples, labels, err := synthetic.Generate(c.EvalSeed, c.Data)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	_, unseenIdx, err := synthetic.SplitUnseenCombinations(labels, c.Holdout)
	if err != nil {
		t.Fatalf("SplitUnseenCombinations: %v", err)
	}
	want := map[string]int{}
	for _, i := range unseenIdx {
		for _, m := range []string{"image", "text", "audio"} {
			if mod, ok := samples[i].Modalities[m]; ok && mod.Present {
				want[m]++
			}
		}
	}
	q := run.UnseenRetrievalBefore.Queries
	t.Logf("queries image_to_text=%d (holdout images %d), text_to_image=%d (holdout texts %d), audio_to_image_shape=%d (holdout audios %d)",
		q["image_to_text"], want["image"], q["text_to_image"], want["text"], q["audio_to_image_shape"], want["audio"])
	t.Logf("UnseenRetrievalBefore image_to_text=%.4f text_to_image=%.4f audio_to_image_shape=%.4f",
		run.UnseenRetrievalBefore.ImageToText, run.UnseenRetrievalBefore.TextToImage, run.UnseenRetrievalBefore.AudioToImageShape)
	if q["image_to_text"] != want["image"] {
		t.Errorf("image_to_text queries = %d, want %d holdout image examples", q["image_to_text"], want["image"])
	}
	if q["text_to_image"] != want["text"] {
		t.Errorf("text_to_image queries = %d, want %d holdout text examples", q["text_to_image"], want["text"])
	}
	if q["audio_to_image_shape"] != want["audio"] {
		t.Errorf("audio_to_image_shape queries = %d, want %d holdout audio examples", q["audio_to_image_shape"], want["audio"])
	}
	if !(run.UnseenRetrievalBefore.ImageToText < 1) {
		t.Errorf("UnseenRetrievalBefore.ImageToText = %v, want < 1 when the whole evaluation set is the gallery", run.UnseenRetrievalBefore.ImageToText)
	}
}

func TestRunReportsSeenAndUnseenSeparately(t *testing.T) {
	c := evalConfig(1, 2, 3)
	report, err := Run(context.Background(), c)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(report.Runs))
	}
	chance := report.Chance.Label
	var sumAfter, sumBefore float64
	for _, run := range report.Runs {
		if run.Failed {
			t.Errorf("seed %d failed: %s", run.Seed, run.Error)
			continue
		}
		t.Logf("seed %d seen: i2t=%.4f t2i=%.4f a2i=%.4f shape=%v colour=%v | seen_before i2t=%.4f | unseen: i2t=%.4f t2i=%.4f a2i=%.4f shape=%v colour=%v",
			run.Seed,
			run.Seen.ImageToText, run.Seen.TextToImage, run.Seen.AudioToImageShape, run.Seen.ShapeAccuracy, run.Seen.ColourAccuracy,
			run.SeenBefore.ImageToText,
			run.Unseen.ImageToText, run.Unseen.TextToImage, run.Unseen.AudioToImageShape, run.Unseen.ShapeAccuracy, run.Unseen.ColourAccuracy)
		if run.Seen.ImageToText <= chance {
			t.Errorf("seed %d Seen.ImageToText = %v, want > chance %v", run.Seed, run.Seen.ImageToText, chance)
		}
		if run.Seen.TextToImage <= chance {
			t.Errorf("seed %d Seen.TextToImage = %v, want > chance %v", run.Seed, run.Seen.TextToImage, chance)
		}
		sumAfter += run.Seen.ImageToText
		sumBefore += run.SeenBefore.ImageToText
		for _, s := range []Scores{run.Unseen} {
			for m, v := range s.ShapeAccuracy {
				if !finiteUnit(v) {
					t.Errorf("seed %d unseen shape accuracy %q = %v, want finite in [0,1]", run.Seed, m, v)
				}
			}
			for m, v := range s.ColourAccuracy {
				if !finiteUnit(v) {
					t.Errorf("seed %d unseen colour accuracy %q = %v, want finite in [0,1]", run.Seed, m, v)
				}
			}
			for _, v := range []float64{s.ImageToText, s.TextToImage, s.AudioToImageShape} {
				if !finiteUnit(v) {
					t.Errorf("seed %d unseen retrieval = %v, want finite in [0,1]", run.Seed, v)
				}
			}
		}
	}
	n := float64(len(report.Runs))
	if sumAfter/n <= sumBefore/n {
		t.Errorf("mean Seen.ImageToText = %v, want > mean SeenBefore.ImageToText %v", sumAfter/n, sumBefore/n)
	}

	_, trainLabels, err := synthetic.Generate(c.TrainSeed, c.Data)
	if err != nil {
		t.Fatalf("Generate train: %v", err)
	}
	_, evalLabels, err := synthetic.Generate(c.EvalSeed, c.Data)
	if err != nil {
		t.Fatalf("Generate eval: %v", err)
	}
	holdoutSet := map[synthetic.Label]bool{}
	for _, h := range c.Holdout {
		holdoutSet[h] = true
	}
	trainHoldout := 0
	for _, l := range trainLabels {
		if holdoutSet[l] {
			trainHoldout++
		}
	}
	if got := report.TrainSamples + trainHoldout; got != len(trainLabels) {
		t.Errorf("TrainSamples %d + train-draw holdout %d = %d, want Generate samples %d", report.TrainSamples, trainHoldout, got, len(trainLabels))
	}
	if got := report.SeenSamples + report.UnseenSamples; got != len(evalLabels) {
		t.Errorf("SeenSamples %d + UnseenSamples %d = %d, want Generate samples %d", report.SeenSamples, report.UnseenSamples, got, len(evalLabels))
	}
	t.Logf("samples: train=%d (holdout %d of %d), seen=%d, unseen=%d (of %d)",
		report.TrainSamples, trainHoldout, len(trainLabels), report.SeenSamples, report.UnseenSamples, len(evalLabels))
}

func TestRunIsDeterministic(t *testing.T) {
	c := evalConfig(1, 2, 3)
	a, err := Run(context.Background(), c)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	b, err := Run(context.Background(), c)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !reflect.DeepEqual(a.Runs, b.Runs) {
		t.Fatalf("runs differ across identical calls:\n%+v\n%+v", a.Runs, b.Runs)
	}
	if a.ConfigHash != b.ConfigHash {
		t.Fatalf("ConfigHash = %q then %q", a.ConfigHash, b.ConfigHash)
	}
}
