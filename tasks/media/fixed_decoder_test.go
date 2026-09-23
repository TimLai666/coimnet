package media

import (
	"context"
	"reflect"
	"testing"
)

// TestFixedDecoderTrainsAndLocks verifies reconstruction, classification, determinism, and decoder immutability.
func TestFixedDecoderTrainsAndLocks(t *testing.T) {
	d, err := DefaultFixedDecoder()
	if err != nil {
		t.Fatal(err)
	}
	mse, err := d.ReconstructionMSE()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("decoder reconstruction MSE: %.8f", mse)
	if mse >= 0.02 {
		t.Fatalf("ReconstructionMSE() = %g, want < 0.02", mse)
	}
	for _, colour := range ImageColours {
		for _, shape := range ImageShapes {
			want := ImageCondition{Colour: colour, Shape: shape}
			pixels, err := RenderImage(want)
			if err != nil {
				t.Fatal(err)
			}
			latent, err := d.Encode(pixels)
			if err != nil {
				t.Fatal(err)
			}
			reconstructed, err := d.Decode(latent)
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := ClassifyImage(reconstructed)
			if err != nil || got != want {
				t.Errorf("reconstructed %q as %+v, %v; want %+v", want.Prompt(), got, err, want)
			}
		}
	}
	first, err := TrainFixedDecoder(8, 3000, 0.5, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := TrainFixedDecoder(8, 3000, 0.5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash() != second.Hash() {
		t.Fatalf("same training arguments produced hashes %q and %q", first.Hash(), second.Hash())
	}

	config := DefaultGeneratorConfig(ModalityImage)
	core, err := NewLatentGenerator(config, d)
	if err != nil {
		t.Fatal(err)
	}
	before := d.Hash()
	if _, err := core.Latent(context.Background(), "red square"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := core.Step(context.Background(), "red square"); err != nil {
		t.Fatal(err)
	}
	if after := d.Hash(); after != before {
		t.Fatalf("decoder hash changed after core training: before %s, after %s", before, after)
	}
}

// TestFixedDecoderRejects verifies invalid training arguments and input shapes return errors.
func TestFixedDecoderRejects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		latent int
		epochs int
		rate   float64
	}{
		{name: "latent", latent: 0, epochs: 1, rate: 0.1},
		{name: "epochs", latent: 1, epochs: 0, rate: 0.1},
		{name: "rate", latent: 1, epochs: 1, rate: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := TrainFixedDecoder(tc.latent, tc.epochs, tc.rate, 1); err == nil {
				t.Fatal("TrainFixedDecoder() returned nil error")
			}
		})
	}
	d, err := DefaultFixedDecoder()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Encode(make([]float64, ImageSize*ImageSize*3-1)); err == nil {
		t.Fatal("Encode() returned nil error for wrong pixel length")
	}
	if _, err := d.Decode(make([]float64, d.Latent-1)); err == nil {
		t.Fatal("Decode() returned nil error for wrong latent length")
	}
}

// TestLatentGeneratorLearnsThroughTheFixedDecoder verifies that core learning improves generated fixture images.
func TestLatentGeneratorLearnsThroughTheFixedDecoder(t *testing.T) {
	d, err := DefaultFixedDecoder()
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultGeneratorConfig(ModalityImage)
	config.Seed = 7
	g, err := NewLatentGenerator(config, d)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	prompts, err := fixturePrompts(ModalityImage)
	if err != nil {
		t.Fatal(err)
	}
	const heldOut = "blue diagonal"
	seen := make([]string, 0, len(prompts)-1)
	for _, prompt := range prompts {
		if prompt != heldOut {
			seen = append(seen, prompt)
		}
	}
	before, err := countLatentCorrect(ctx, g, seen)
	if err != nil {
		t.Fatal(err)
	}
	statsBefore, err := g.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for epoch := 0; epoch < 60; epoch++ {
		for _, prompt := range seen {
			if _, _, err := g.Step(ctx, prompt); err != nil {
				t.Fatal(err)
			}
		}
	}
	after, err := countLatentCorrect(ctx, g, seen)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seen correct: before=%d/%d after=%d/%d; held-out %q is diagnostic only", before, len(seen), after, len(seen), heldOut)
	heldOutPixels, err := g.Generate(ctx, heldOut)
	if err != nil {
		t.Fatal(err)
	}
	heldOutPrediction, _, err := ClassifyImage(heldOutPixels)
	if err != nil {
		t.Fatal(err)
	}
	heldOutCondition, err := ParseImagePrompt(heldOut)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("held-out %q prediction=%q, correct=%t; not included in training", heldOut, heldOutPrediction.Prompt(), heldOutPrediction == heldOutCondition)
	if after <= before {
		t.Fatalf("seen correctness did not improve: before=%d after=%d", before, after)
	}
	statsAfter, err := g.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("latent mean absolute error: before=%.8f after=%.8f", statsBefore.MeanAbsError, statsAfter.MeanAbsError)
	if statsAfter.MeanAbsError >= statsBefore.MeanAbsError {
		t.Fatalf("latent mean absolute error did not decrease: before=%g after=%g", statsBefore.MeanAbsError, statsAfter.MeanAbsError)
	}
	capacity := g.Capacity()
	if capacity.DecoderParameters != d.Parameters() || !capacity.EncoderTrains || !capacity.CoreTrains || !capacity.ReadoutTrains {
		t.Fatalf("unexpected capacity split: %+v", capacity)
	}
}

// TestLatentGeneratorIsDeterministic verifies equal seeded training produces identical statistics and pixels.
func TestLatentGeneratorIsDeterministic(t *testing.T) {
	d1, err := DefaultFixedDecoder()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := DefaultFixedDecoder()
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultGeneratorConfig(ModalityImage)
	first, err := NewLatentGenerator(config, d1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLatentGenerator(config, d2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		for _, g := range []*LatentGenerator{first, second} {
			if _, _, err := g.Step(ctx, "green square"); err != nil {
				t.Fatal(err)
			}
		}
	}
	firstStats, err := first.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondStats, err := second.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstStats, secondStats) {
		t.Fatalf("same latent training produced different stats: %+v and %+v", firstStats, secondStats)
	}
	firstPixels, err := first.Generate(ctx, "blue diagonal")
	if err != nil {
		t.Fatal(err)
	}
	secondPixels, err := second.Generate(ctx, "blue diagonal")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstPixels, secondPixels) {
		t.Fatal("same latent training produced different generated pixels")
	}
}

// countLatentCorrect counts prompts whose generated image is classified as the requested condition.
func countLatentCorrect(ctx context.Context, g *LatentGenerator, prompts []string) (int, error) {
	count := 0
	for _, prompt := range prompts {
		condition, err := ParseImagePrompt(prompt)
		if err != nil {
			return 0, err
		}
		pixels, err := g.Generate(ctx, prompt)
		if err != nil {
			return 0, err
		}
		got, _, err := ClassifyImage(pixels)
		if err != nil {
			return 0, err
		}
		if got == condition {
			count++
		}
	}
	return count, nil
}
