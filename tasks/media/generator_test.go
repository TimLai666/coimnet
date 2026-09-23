package media

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratorConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		edit func(*GeneratorConfig)
		want bool
	}{
		{name: "valid image", edit: func(c *GeneratorConfig) { c.Modality = ModalityImage }, want: true},
		{name: "valid audio", edit: func(c *GeneratorConfig) { c.Modality = ModalityAudio }, want: true},
		{name: "unknown modality", edit: func(c *GeneratorConfig) { c.Modality = "video" }},
		{name: "small embed", edit: func(c *GeneratorConfig) { c.Embed = 3 }},
		{name: "large embed", edit: func(c *GeneratorConfig) { c.Embed = 129 }},
		{name: "small hidden", edit: func(c *GeneratorConfig) { c.Hidden = 7 }},
		{name: "large hidden", edit: func(c *GeneratorConfig) { c.Hidden = 513 }},
		{name: "zero settle", edit: func(c *GeneratorConfig) { c.Settle = 0 }},
		{name: "large settle", edit: func(c *GeneratorConfig) { c.Settle = 9 }},
		{name: "zero learning rate", edit: func(c *GeneratorConfig) { c.LearningRate = 0 }},
		{name: "negative learning rate", edit: func(c *GeneratorConfig) { c.LearningRate = -1 }},
		{name: "nan learning rate", edit: func(c *GeneratorConfig) { c.LearningRate = math.NaN() }},
		{name: "infinite learning rate", edit: func(c *GeneratorConfig) { c.LearningRate = math.Inf(1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultGeneratorConfig(ModalityImage)
			tt.edit(&c)
			err := c.Validate()
			if (err == nil) != tt.want {
				t.Fatalf("Validate() error = %v, want success %t", err, tt.want)
			}
		})
	}
}

func TestDefaultGeneratorConfig(t *testing.T) {
	want := GeneratorConfig{Modality: ModalityAudio, Embed: 16, Hidden: 64, Settle: 3, LearningRate: .01, Seed: 1}
	if got := DefaultGeneratorConfig(ModalityAudio); got != want {
		t.Fatalf("DefaultGeneratorConfig() = %+v, want %+v", got, want)
	}
}

func TestWords(t *testing.T) {
	tests := []struct {
		modality string
		want     []string
	}{
		{ModalityImage, []string{"red", "green", "blue", "square", "cross", "diagonal"}},
		{ModalityAudio, []string{"low", "mid", "high", "steady", "fade", "pulse"}},
	}
	for _, tt := range tests {
		got, err := Words(tt.modality)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Words(%q) = %v, %v; want %v", tt.modality, got, err, tt.want)
		}
	}
	if _, err := Words("video"); err == nil {
		t.Fatal("Words(video) succeeded")
	}
}

func TestDecoderHasNoPromptInput(t *testing.T) {
	typ := reflect.TypeOf(DecoderInput{})
	if typ.NumField() != 1 || typ.Field(0).Name != "Representation" || typ.Field(0).Type != reflect.TypeOf([]float64{}) {
		t.Fatalf("DecoderInput fields = %v, want only Representation []float64", typ)
	}
	for _, modality := range []string{ModalityImage, ModalityAudio} {
		length := 192
		if modality == ModalityAudio {
			length = AudioBlock
		}
		representation := make([]float64, length)
		for i := range representation {
			representation[i] = .25
		}
		first, err := Decode(modality, DecoderInput{Representation: representation})
		if err != nil {
			t.Fatal(err)
		}
		second, err := Decode(modality, DecoderInput{Representation: append([]float64(nil), representation...)})
		if err != nil || !reflect.DeepEqual(first, second) {
			t.Fatalf("Decode(%q) is not a pure representation function: %v", modality, err)
		}
	}
	if _, err := Decode(ModalityImage, DecoderInput{Representation: make([]float64, 191)}); err == nil {
		t.Fatal("Decode accepted the wrong image representation length")
	}
	if _, err := Decode(ModalityAudio, DecoderInput{Representation: make([]float64, 257)}); err == nil {
		t.Fatal("Decode accepted the wrong audio representation length")
	}
	image := make([]float64, 192)
	image[0], image[1], image[2] = -1, .5, 2
	got, err := Decode(ModalityImage, DecoderInput{Representation: image})
	if err != nil || got[0] != 0 || got[1] != .5 || got[2] != 1 {
		t.Fatalf("image decoder clamp = %v, %v", got[:3], err)
	}
	audio := make([]float64, AudioBlock)
	audio[0], audio[1], audio[2] = -2, .5, 2
	got, err = Decode(ModalityAudio, DecoderInput{Representation: audio})
	if err != nil || got[0] != -1 || got[1] != .5 || got[2] != 1 {
		t.Fatalf("audio decoder clamp = %v, %v", got[:3], err)
	}
}

func TestGeneratorLayoutAndCapacity(t *testing.T) {
	for _, modality := range []string{ModalityImage, ModalityAudio} {
		g, err := NewGenerator(DefaultGeneratorConfig(modality))
		if err != nil {
			t.Fatal(err)
		}
		snapshot := g.Snapshot()
		config := snapshot.Config
		wantOutputs := 192
		if modality == ModalityAudio {
			wantOutputs = AudioBlock
		}
		if config.Dynamics.Nodes != 16+64 || len(config.Dynamics.Sources) != 16*64+64*64 || config.OutputSize != wantOutputs {
			t.Fatalf("%s model layout = nodes %d, edges %d, outputs %d", modality, config.Dynamics.Nodes, len(config.Dynamics.Sources), config.OutputSize)
		}
		capacity := g.Capacity()
		if capacity.EncoderParameters != len(snapshot.Parameters.Encoder) ||
			capacity.CoreParameters != len(snapshot.Parameters.Core.Weights)+len(snapshot.Parameters.Core.Bias)+len(snapshot.Parameters.Core.LogTau) ||
			capacity.ReadoutParameters != len(snapshot.Parameters.Readout) || capacity.DecoderParameters != 0 ||
			!capacity.EncoderTrains || !capacity.CoreTrains || !capacity.ReadoutTrains {
			t.Fatalf("%s capacity = %+v", modality, capacity)
		}
	}
}

func TestGeneratorLearnsSeenConditions(t *testing.T) {
	ctx := context.Background()
	for _, modality := range []string{ModalityImage, ModalityAudio} {
		t.Run(modality, func(t *testing.T) {
			g, err := NewGenerator(DefaultGeneratorConfig(modality))
			if err != nil {
				t.Fatal(err)
			}
			prompts, targets := generatorTrainingSet(t, modality)
			holdout := "blue diagonal"
			if modality == ModalityAudio {
				holdout = "high pulse"
			}
			seen := make([]string, 0, 8)
			for _, prompt := range prompts {
				if prompt != holdout {
					seen = append(seen, prompt)
				}
			}
			beforeMSE, beforeCorrect := generatorScores(t, ctx, g, seen, targets)
			steps := make(map[string]int)
			step := func(prompt string, target []float64) error {
				steps[prompt]++
				_, _, err := g.Step(ctx, prompt, target)
				return err
			}
			for epoch := 0; epoch < 60; epoch++ {
				for _, prompt := range prompts {
					if prompt == holdout {
						continue
					}
					if err := step(prompt, targets[prompt]); err != nil {
						t.Fatal(err)
					}
				}
			}
			if steps[holdout] != 0 || len(steps) != 8 {
				t.Fatalf("training prompt counts include holdout or miss a seen prompt: %v", steps)
			}
			afterMSE, afterCorrect := generatorScores(t, ctx, g, seen, targets)
			t.Logf("%s seen conditions: average MSE %.8f -> %.8f; classifications %d/8 -> %d/8", modality, beforeMSE, afterMSE, beforeCorrect, afterCorrect)
			if !(afterMSE < beforeMSE) {
				t.Fatalf("average MSE did not decrease: %g -> %g", beforeMSE, afterMSE)
			}
		})
	}
}

func TestGeneratorIsDeterministic(t *testing.T) {
	c := DefaultGeneratorConfig(ModalityImage)
	first, err := NewGenerator(c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewGenerator(c)
	if err != nil {
		t.Fatal(err)
	}
	prompts, targets := generatorTrainingSet(t, ModalityImage)
	for _, prompt := range prompts {
		for i := 0; i < 3; i++ {
			if _, _, err := first.Step(context.Background(), prompt, targets[prompt]); err != nil {
				t.Fatal(err)
			}
			if _, _, err := second.Step(context.Background(), prompt, targets[prompt]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !reflect.DeepEqual(first.Snapshot(), second.Snapshot()) {
		t.Fatal("same seed produced different training snapshots")
	}
}

func TestGeneratorRejects(t *testing.T) {
	g, err := NewGenerator(DefaultGeneratorConfig(ModalityImage))
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"blue unknown", "diagonal blue", "blue  diagonal", "blue diagonal extra"} {
		if _, err := g.Representation(context.Background(), prompt); err == nil {
			t.Errorf("Representation accepted prompt %q", prompt)
		}
	}
	if _, _, err := g.Step(context.Background(), "blue diagonal", make([]float64, 191)); err == nil {
		t.Fatal("Step accepted the wrong target length")
	}
}

func generatorTrainingSet(t *testing.T, modality string) ([]string, map[string][]float64) {
	t.Helper()
	words, err := Words(modality)
	if err != nil {
		t.Fatal(err)
	}
	prompts := make([]string, 0, 9)
	targets := make(map[string][]float64, 9)
	for _, first := range words[:3] {
		for _, second := range words[3:] {
			prompt := first + " " + second
			var target []float64
			if modality == ModalityImage {
				condition, err := ParseImagePrompt(prompt)
				if err != nil {
					t.Fatal(err)
				}
				target, err = RenderImage(condition)
			} else {
				condition, err := ParseAudioPrompt(prompt)
				if err != nil {
					t.Fatal(err)
				}
				target, err = RenderAudio(condition)
			}
			if err != nil {
				t.Fatal(err)
			}
			prompts = append(prompts, prompt)
			targets[prompt] = target
		}
	}
	return prompts, targets
}

func generatorScores(t *testing.T, ctx context.Context, g *Generator, prompts []string, targets map[string][]float64) (float64, int) {
	t.Helper()
	totalMSE, correct := 0.0, 0
	for _, prompt := range prompts {
		generated, err := g.Generate(ctx, prompt)
		if err != nil {
			t.Fatal(err)
		}
		for i, value := range generated {
			delta := value - targets[prompt][i]
			totalMSE += delta * delta / float64(len(generated))
		}
		if conditionMatches(modalityForPrompt(prompt), prompt, generated) {
			correct++
		}
	}
	return totalMSE / float64(len(prompts)), correct
}

func modalityForPrompt(prompt string) string {
	if condition := strings.SplitN(prompt, " ", 2)[0]; condition == "low" || condition == "mid" || condition == "high" {
		return ModalityAudio
	}
	return ModalityImage
}

func conditionMatches(modality, prompt string, generated []float64) bool {
	if modality == ModalityImage {
		got, _, err := ClassifyImage(generated)
		want, parseErr := ParseImagePrompt(prompt)
		return err == nil && parseErr == nil && got == want
	}
	got, _, err := ClassifyAudio(generated)
	want, parseErr := ParseAudioPrompt(prompt)
	return err == nil && parseErr == nil && got == want
}
