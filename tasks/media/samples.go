package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteSamples writes outputs produced by a trained core generator from prompts, not target images.
func WriteSamples(ctx context.Context, dir string, c RunConfig, seed uint64) (map[string]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("media: context is nil")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, fmt.Errorf("media: sample directory is empty")
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, fmt.Errorf("media: create sample directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()

	generatorConfig := c.Generator
	generatorConfig.Seed = seed
	generator, err := NewGenerator(generatorConfig)
	if err != nil {
		return nil, fmt.Errorf("media: create sample generator: %w", err)
	}
	conditions, err := fixturePrompts(c.Modality)
	if err != nil {
		return nil, err
	}
	seen, _ := partitionPrompts(conditions, c.Holdout)
	if err := trainMediaConditions(ctx, c.Modality, seen, c.Epochs, generator); err != nil {
		return nil, fmt.Errorf("media: train sample generator: %w", err)
	}

	hashes := make(map[string]string, len(conditions))
	for _, prompt := range conditions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("media: write samples canceled: %w", err)
		}
		var name string
		var digest string
		if c.Modality == ModalityImage {
			condition, err := ParseImagePrompt(prompt)
			if err != nil {
				return nil, err
			}
			pixels, err := generator.Generate(ctx, prompt)
			if err != nil {
				return nil, fmt.Errorf("media: generate %q: %w", prompt, err)
			}
			name = condition.Colour + "_" + condition.Shape + ".png"
			digest, err = WritePNG(filepath.Join(dir, name), pixels)
			if err != nil {
				return nil, fmt.Errorf("media: write %s: %w", name, err)
			}
		} else {
			condition, err := ParseAudioPrompt(prompt)
			if err != nil {
				return nil, err
			}
			samples := make([]float64, 0, c.Blocks*AudioBlock)
			first, second, err := generator.promptIndices(prompt)
			if err != nil {
				return nil, err
			}
			input := make([][]float64, (c.Blocks+1)*generator.config.Settle)
			for row := range input {
				input[row] = make([]float64, len(generator.words))
				word := first
				if row >= generator.config.Settle {
					word = second
				}
				input[row][word] = 1
			}
			outputs, err := generator.trainer.PredictAll(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("media: generate %q blocks: %w", prompt, err)
			}
			for block := 0; block < c.Blocks; block++ {
				if err := ctx.Err(); err != nil {
					return nil, fmt.Errorf("media: generate %q block %d: %w", prompt, block, err)
				}
				row := (block+2)*generator.config.Settle - 1
				generated, err := Decode(c.Modality, DecoderInput{Representation: outputs[row]})
				if err != nil {
					return nil, fmt.Errorf("media: decode %q block %d: %w", prompt, block, err)
				}
				samples = append(samples, generated...)
			}
			name = condition.Pitch + "_" + condition.Pattern + ".wav"
			digest, err = WriteBlockWAV(filepath.Join(dir, name), samples)
			if err != nil {
				return nil, fmt.Errorf("media: write %s: %w", name, err)
			}
		}
		hashes[name] = digest
	}
	cleanup = false
	return hashes, nil
}
