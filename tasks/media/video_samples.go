package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteVideoSamples trains one core VideoGenerator exactly as RunVideo trains its core group for seed (same Video config
// with Seed = seed, same Epochs, same held-out prompts, same prompt order), then writes the trained core's own clip for
// each of the six video prompts with WriteVideo into dir/<direction>_<sound>/. These are the generator's outputs from the
// prompt, not the rendered targets. dir must not exist; it is created and removed again on any error. The map is keyed by
// the clip directory name.
func WriteVideoSamples(ctx context.Context, dir string, c VideoRunConfig, seed uint64) (map[string]Timeline, error) {
	if ctx == nil {
		return nil, fmt.Errorf("media: context is nil")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, fmt.Errorf("media: video sample directory is empty")
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, fmt.Errorf("media: create video sample directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()

	config := c.Video
	config.Seed = seed
	generator, err := NewVideoGenerator(config)
	if err != nil {
		return nil, fmt.Errorf("media: create video sample generator: %w", err)
	}
	prompts := videoPrompts()
	seen, _ := partitionPrompts(prompts, c.Holdout)
	if err := trainVideoPrompts(ctx, seen, c.Epochs, generator); err != nil {
		return nil, fmt.Errorf("media: train video sample generator: %w", err)
	}

	timelines := make(map[string]Timeline, len(prompts))
	for _, prompt := range prompts {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("media: write video samples canceled: %w", err)
		}
		condition, err := ParseVideoPrompt(prompt)
		if err != nil {
			return nil, fmt.Errorf("media: parse video prompt %q: %w", prompt, err)
		}
		frames, audio, err := generator.Generate(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("media: generate video sample %q: %w", prompt, err)
		}
		name := condition.Direction + "_" + condition.Sound
		timeline, err := WriteVideo(filepath.Join(dir, name), frames, audio)
		if err != nil {
			return nil, fmt.Errorf("media: write video sample %q: %w", prompt, err)
		}
		timelines[name] = timeline
	}
	cleanup = false
	return timelines, nil
}
