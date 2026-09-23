package media

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestRunMediaReportImage(t *testing.T) {
	c := DefaultRunConfig(ModalityImage)
	c.Epochs, c.Seeds = 10, []uint64{1}
	r, err := RunMedia(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	assertMediaReport(t, r, c, false)
}

func TestRunMediaReportAudio(t *testing.T) {
	c := DefaultRunConfig(ModalityAudio)
	c.Epochs, c.Seeds = 10, []uint64{1}
	r, err := RunMedia(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	assertMediaReport(t, r, c, true)
}

func TestRunMediaNeverTrainsHeldOut(t *testing.T) {
	for _, modality := range []string{ModalityImage, ModalityAudio} {
		t.Run(modality, func(t *testing.T) {
			c := DefaultRunConfig(modality)
			c.Epochs, c.Seeds = 10, []uint64{1}
			r, err := RunMedia(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range []GroupScore{r.Runs[0].Core, r.Runs[0].FrozenCore} {
				if group.Updates != c.Epochs*8 {
					t.Errorf("%s updates = %d, want exactly %d seen prompt steps", group.Name, group.Updates, c.Epochs*8)
				}
			}
			conditions, err := fixturePrompts(modality)
			if err != nil {
				t.Fatal(err)
			}
			seen, _ := partitionPrompts(conditions, c.Holdout)
			core, err := NewGenerator(c.Generator)
			if err != nil {
				t.Fatal(err)
			}
			frozen, err := frozenCoreGenerator(core)
			if err != nil {
				t.Fatal(err)
			}
			coreCounter, frozenCounter := &stepCounter{inner: core}, &stepCounter{inner: frozen}
			if err := trainMediaConditions(context.Background(), modality, seen, c.Epochs, coreCounter, frozenCounter); err != nil {
				t.Fatal(err)
			}
			want := make([]string, 0, len(seen)*c.Epochs)
			for epoch := 0; epoch < c.Epochs; epoch++ {
				want = append(want, seen...)
			}
			if !reflect.DeepEqual(coreCounter.prompts, want) || !reflect.DeepEqual(frozenCounter.prompts, want) {
				t.Errorf("Step prompts include a holdout or violate fixed order: core %v frozen %v", coreCounter.prompts, frozenCounter.prompts)
			}
		})
	}
}

func TestRunMediaDefaultLearns(t *testing.T) {
	c := DefaultRunConfig(ModalityImage)
	c.Seeds = []uint64{1}
	r, err := RunMedia(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	seed := r.Runs[0]
	t.Logf("core seen correct %d -> %d; held-out correct %d", countSeen(seed.Core.Before), seed.Core.SeenCorrect, seed.Core.HeldOutCorrect)
	t.Logf("frozen_core seen correct %d -> %d; held-out correct %d", countSeen(seed.FrozenCore.Before), seed.FrozenCore.SeenCorrect, seed.FrozenCore.HeldOutCorrect)
	if seed.Core.SeenCorrect <= countSeen(seed.Core.Before) {
		t.Fatalf("core seen correct = %d, want more than before %d", seed.Core.SeenCorrect, countSeen(seed.Core.Before))
	}
}

func TestRunMediaIsDeterministic(t *testing.T) {
	c := DefaultRunConfig(ModalityAudio)
	c.Epochs, c.Seeds = 10, []uint64{1}
	first, err := RunMedia(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunMedia(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatal("same run configuration produced different JSON reports")
	}
}

func TestRunConfigValidate(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*RunConfig)
		field string
	}{
		{"modality mismatch", func(c *RunConfig) { c.Generator.Modality = ModalityAudio }, "generator"},
		{"unknown holdout prompt", func(c *RunConfig) { c.Holdout = []string{"blue triangle"} }, "holdout"},
		{"image blocks", func(c *RunConfig) { c.Blocks = 2 }, "blocks"},
		{"zero epochs", func(c *RunConfig) { c.Epochs = 0 }, "epochs"},
		{"duplicate seeds", func(c *RunConfig) { c.Seeds = []uint64{1, 1} }, "seeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultRunConfig(ModalityImage)
			tt.edit(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, tt.field)
			}
		})
	}
}

func TestDefaultRunConfig(t *testing.T) {
	image := DefaultRunConfig(ModalityImage)
	if image.Modality != ModalityImage || image.Generator != DefaultGeneratorConfig(ModalityImage) || image.Epochs != 60 || image.Blocks != 1 || !reflect.DeepEqual(image.Holdout, []string{"blue diagonal"}) || !reflect.DeepEqual(image.Seeds, []uint64{1, 2, 3}) {
		t.Fatalf("image default config = %+v", image)
	}
	audio := DefaultRunConfig(ModalityAudio)
	if audio.Modality != ModalityAudio || audio.Generator != DefaultGeneratorConfig(ModalityAudio) || audio.Epochs != 60 || audio.Blocks != 3 || !reflect.DeepEqual(audio.Holdout, []string{"high pulse"}) || !reflect.DeepEqual(audio.Seeds, []uint64{1, 2, 3}) {
		t.Fatalf("audio default config = %+v", audio)
	}
}

func assertMediaReport(t *testing.T, report RunReport, config RunConfig, audio bool) {
	t.Helper()
	if report.SchemaVersion != ReportSchemaVersion || !reflect.DeepEqual(report.Config, config) || len(report.Runs) != 1 {
		t.Fatalf("report header = schema %q config %+v runs %d", report.SchemaVersion, report.Config, len(report.Runs))
	}
	if len(report.ConfigHash) != 64 {
		t.Fatalf("config hash = %q, want sha256 hex", report.ConfigHash)
	}
	run := report.Runs[0]
	if run.Seed != config.Seeds[0] || run.Failed || run.Error != "" {
		t.Fatalf("run status = %+v", run)
	}
	for _, group := range []GroupScore{run.Core, run.FrozenCore} {
		if len(group.Before) != 9 || len(group.After) != 9 || group.Updates != config.Epochs*8 {
			t.Errorf("%s before/after/updates = %d/%d/%d", group.Name, len(group.Before), len(group.After), group.Updates)
		}
		holdouts := make(map[string]bool, len(config.Holdout))
		for _, prompt := range config.Holdout {
			holdouts[prompt] = true
		}
		for _, scores := range [][]ConditionScore{group.Before, group.After} {
			for i, score := range scores {
				if score.Seen == holdouts[score.Prompt] {
					t.Errorf("%s score %q seen = %t", group.Name, score.Prompt, score.Seen)
				}
				if i > 0 && scores[i-1].Prompt == score.Prompt {
					t.Errorf("%s contains duplicate condition %q", group.Name, score.Prompt)
				}
			}
		}
		if group.Name == "frozen_core" && group.Capacity.CoreTrains {
			t.Error("frozen_core capacity reports a trainable core")
		}
		t.Logf("%s seen correct %d -> %d; held-out correct %d", group.Name, countSeen(group.Before), group.SeenCorrect, group.HeldOutCorrect)
		t.Logf("%s updates=%d capacity encoder/core/readout trains=%t/%t/%t", group.Name, group.Updates, group.Capacity.EncoderTrains, group.Capacity.CoreTrains, group.Capacity.ReadoutTrains)
		for _, score := range group.After {
			if !score.Seen {
				t.Logf("%s holdout %q classified %q correct=%t confidence=%.6g", group.Name, score.Prompt, score.Classified, score.Correct, score.Confidence)
			}
		}
	}
	if !run.CoreDisconnect.OutputChanged || run.CoreDisconnect.MaxAbsDelta <= 0 {
		t.Errorf("core disconnect = %+v", run.CoreDisconnect)
	}
	t.Logf("core disconnect changed=%t max_abs_delta=%.9g", run.CoreDisconnect.OutputChanged, run.CoreDisconnect.MaxAbsDelta)
	if audio {
		if run.Temporal == nil || run.Temporal.Blocks != 3 || run.Temporal.Prompts != 9 {
			t.Errorf("audio temporal result = %+v", run.Temporal)
		}
		if run.Temporal != nil {
			t.Logf("audio temporal blocks=%d prompts=%d consistent=%d", run.Temporal.Blocks, run.Temporal.Prompts, run.Temporal.Consistent)
		}
	} else if run.Temporal != nil {
		t.Errorf("image temporal result = %+v, want nil", run.Temporal)
	}
	want := []string{
		"Pixel or sample MSE says nothing about perceptual quality.",
		"This proves generation on a finite fixture of nine conditions, not open-ended image or audio generation.",
		"The decoder is a fixed clamp with no parameters; the prompt reaches the output only through the core.",
		"The frozen-core control trains only the encoder and readout, showing what the periphery alone can do with the same budget.",
	}
	if !reflect.DeepEqual(report.Assumptions, want) {
		t.Errorf("assumptions = %q, want %q", report.Assumptions, want)
	}
	if !reflect.DeepEqual(run.FrozenCore.Capacity, func() CapacitySplit {
		g, err := NewGenerator(config.Generator)
		if err != nil {
			t.Fatal(err)
		}
		s := g.Snapshot()
		s.Options.Trainable = learning.Trainable{Encoder: true, Readout: true}
		trainer, err := learning.RestoreTrainer(s)
		if err != nil {
			t.Fatal(err)
		}
		g.trainer = trainer
		return g.Capacity()
	}()) {
		t.Error("frozen capacity does not match the encoder/readout-only control")
	}
}

func countSeen(scores []ConditionScore) int {
	count := 0
	for _, score := range scores {
		if score.Seen && score.Correct {
			count++
		}
	}
	return count
}

type stepCounter struct {
	inner   mediaTrainer
	prompts []string
}

func (c *stepCounter) Step(ctx context.Context, prompt string, target []float64) (learning.StepResult, float64, error) {
	c.prompts = append(c.prompts, prompt)
	return c.inner.Step(ctx, prompt, target)
}
