package media

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestVideoGeneratorLayout(t *testing.T) {
	c := DefaultVideoConfig()
	g, err := NewVideoGenerator(c)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := g.Snapshot()
	if snapshot.Config.OutputSize != 448 {
		t.Fatalf("output size = %d, want 448", snapshot.Config.OutputSize)
	}
	wantEncoder := len(VideoWords()) * c.Embed
	wantCore := c.Embed*c.Hidden + c.Hidden*c.Hidden + 2*(c.Embed+c.Hidden)
	wantReadout := c.Hidden * 448
	wantCapacity := CapacitySplit{
		EncoderParameters: wantEncoder,
		CoreParameters:    wantCore,
		ReadoutParameters: wantReadout,
		DecoderParameters: 0,
		EncoderTrains:     true,
		CoreTrains:        true,
		ReadoutTrains:     true,
	}
	if got := g.Capacity(); got != wantCapacity {
		t.Fatalf("capacity = %+v, want %+v", got, wantCapacity)
	}
	if len(snapshot.Config.Dynamics.Sources) != c.Embed*c.Hidden+c.Hidden*c.Hidden {
		t.Fatalf("edge count = %d, want input-hidden plus hidden-hidden edges", len(snapshot.Config.Dynamics.Sources))
	}
	if len(snapshot.Parameters.Readout) != wantReadout {
		t.Fatalf("readout parameter count = %d, want %d", len(snapshot.Parameters.Readout), wantReadout)
	}
}

func TestVideoGeneratorStepLowersMSE(t *testing.T) {
	g, err := NewVideoGenerator(DefaultVideoConfig())
	if err != nil {
		t.Fatal(err)
	}
	prompts := []string{"right low", "right high", "left low", "down low", "down high"}
	before := videoPromptMSE(t, g, prompts)
	for epoch := 0; epoch < 20; epoch++ {
		for _, prompt := range prompts {
			if _, _, err := g.Step(context.Background(), prompt); err != nil {
				t.Fatal(err)
			}
		}
	}
	after := videoPromptMSE(t, g, prompts)
	t.Logf("five seen prompts average MSE %.8f -> %.8f", before, after)
	if !(after < before) {
		t.Fatalf("average MSE did not decrease: %g -> %g", before, after)
	}
}

func TestVideoRunReport(t *testing.T) {
	c := smallVideoRunConfig()
	report, err := RunVideo(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != "coimnet-video/v1" || !reflect.DeepEqual(report.Config, c) || len(report.ConfigHash) != 64 || len(report.Runs) != 1 {
		t.Fatalf("report header = schema %q hash %q runs %d", report.SchemaVersion, report.ConfigHash, len(report.Runs))
	}
	run := report.Runs[0]
	if run.Failed || run.Error != "" {
		t.Fatalf("video run failed: %s", run.Error)
	}
	conditions := videoPrompts()
	for _, group := range []VideoGroup{run.Core, run.FrozenCore} {
		if len(group.Before) != 6 || len(group.After) != 6 || group.Updates != 50 {
			t.Errorf("%s before/after/updates = %d/%d/%d", group.Name, len(group.Before), len(group.After), group.Updates)
		}
		for i, prompt := range conditions {
			wantSeen := !contains(c.Holdout, prompt)
			if group.Before[i].Prompt != prompt || group.After[i].Prompt != prompt || group.Before[i].Seen != wantSeen || group.After[i].Seen != wantSeen {
				t.Errorf("%s prompt %q seen before/after = %t/%t, want %t", group.Name, prompt, group.Before[i].Seen, group.After[i].Seen, wantSeen)
			}
		}
	}
	if run.FrozenCore.Name != "frozen_core" || run.FrozenCore.Capacity.CoreTrains {
		t.Errorf("frozen-core capacity = %+v", run.FrozenCore.Capacity)
	}
	if !run.CoreDisconnect.OutputChanged || run.CoreDisconnect.MaxAbsDelta <= 0 {
		t.Errorf("core disconnect = %+v", run.CoreDisconnect)
	}
	coreHoldout := findVideoScore(t, run.Core.After, "down high")
	frozenHoldout := findVideoScore(t, run.FrozenCore.After, "down high")
	t.Logf("seed=%d core seen_correct=%d/5 held_out=%q judged=%q correct=%t path_matches=%d event_frame=%d sync_error=%d frozen seen_correct=%d/5 held_out=%q judged=%q correct=%t path_matches=%d event_frame=%d sync_error=%d core_disconnect=%t max_abs_delta=%.9g",
		run.Seed, run.Core.SeenCorrect, coreHoldout.Prompt, coreHoldout.Judged, coreHoldout.Correct, coreHoldout.PathMatches, coreHoldout.EventFrame, coreHoldout.SyncError,
		run.FrozenCore.SeenCorrect, frozenHoldout.Prompt, frozenHoldout.Judged, frozenHoldout.Correct, frozenHoldout.PathMatches, frozenHoldout.EventFrame, frozenHoldout.SyncError,
		run.CoreDisconnect.OutputChanged, run.CoreDisconnect.MaxAbsDelta)
	if want := []string{
		"The fixture is a single dot and one beep; it proves the frame, audio and timeline pipeline and its synchronisation measure, not open-ended video generation.",
		"The decoder is a fixed clamp; the prompt reaches the frames and the audio only through the core.",
		"Pixel or sample MSE says nothing about perceptual quality.",
		"The frozen-core control trains only the encoder and readout.",
	}; !reflect.DeepEqual(report.Assumptions, want) {
		t.Errorf("assumptions = %q, want %q", report.Assumptions, want)
	}
	for _, group := range []VideoGroup{run.Core, run.FrozenCore} {
		for _, score := range group.After {
			if score.PathMatches < 0 || score.PathMatches > VideoFrames || score.SyncError < -1 || score.SyncError >= VideoFrames {
				t.Errorf("invalid video judgement for %q: %+v", score.Prompt, score)
			}
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"SyncError"`) {
		t.Fatal("video report JSON did not use snake_case field names")
	}
}

func TestVideoRunNeverTrainsHeldOut(t *testing.T) {
	c := smallVideoRunConfig()
	report, err := RunVideo(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []VideoGroup{report.Runs[0].Core, report.Runs[0].FrozenCore} {
		if group.Updates != c.Epochs*5 {
			t.Fatalf("%s updates = %d, want %d seen prompts times epochs", group.Name, group.Updates, c.Epochs*5)
		}
		for _, score := range group.After {
			if contains(c.Holdout, score.Prompt) && score.Seen {
				t.Errorf("held-out prompt %q is marked seen", score.Prompt)
			}
		}
	}
}

func TestVideoRunIsDeterministic(t *testing.T) {
	c := smallVideoRunConfig()
	first, err := RunVideo(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunVideo(context.Background(), c)
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
		t.Fatal("same video run configuration produced different JSON reports")
	}
}

func TestVideoRunConfigValidate(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*VideoRunConfig)
		field string
	}{
		{"invalid embed", func(c *VideoRunConfig) { c.Video.Embed = 3 }, "embed"},
		{"invalid hidden", func(c *VideoRunConfig) { c.Video.Hidden = 7 }, "hidden"},
		{"invalid settle", func(c *VideoRunConfig) { c.Video.Settle = 0 }, "settle"},
		{"invalid learning rate", func(c *VideoRunConfig) { c.Video.LearningRate = math.NaN() }, "learning rate"},
		{"empty holdout", func(c *VideoRunConfig) { c.Holdout = nil }, "holdout"},
		{"invalid holdout", func(c *VideoRunConfig) { c.Holdout = []string{"up low"} }, "holdout"},
		{"duplicate holdout", func(c *VideoRunConfig) { c.Holdout = []string{"down high", "down high"} }, "holdout"},
		{"zero epochs", func(c *VideoRunConfig) { c.Epochs = 0 }, "epochs"},
		{"too many epochs", func(c *VideoRunConfig) { c.Epochs = 10001 }, "epochs"},
		{"empty seeds", func(c *VideoRunConfig) { c.Seeds = nil }, "seeds"},
		{"duplicate seeds", func(c *VideoRunConfig) { c.Seeds = []uint64{1, 1} }, "seeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := smallVideoRunConfig()
			tt.edit(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, tt.field)
			}
		})
	}
	if err := smallVideoRunConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func smallVideoRunConfig() VideoRunConfig {
	c := DefaultVideoRunConfig()
	c.Epochs, c.Seeds = 10, []uint64{1}
	return c
}

func videoPromptMSE(t *testing.T, g *VideoGenerator, prompts []string) float64 {
	t.Helper()
	var total float64
	for _, prompt := range prompts {
		frames, audio, err := g.Generate(context.Background(), prompt)
		if err != nil {
			t.Fatal(err)
		}
		targetFrames, targetAudio, err := RenderVideo(mustParseVideoPrompt(t, prompt))
		if err != nil {
			t.Fatal(err)
		}
		for frame := range frames {
			for i, value := range frames[frame] {
				delta := value - targetFrames[frame][i]
				total += delta * delta / float64(VideoFrames*448*len(prompts))
			}
			for i, value := range audio[frame*AudioBlock : (frame+1)*AudioBlock] {
				delta := value - targetAudio[frame*AudioBlock+i]
				total += delta * delta / float64(VideoFrames*448*len(prompts))
			}
		}
	}
	return total
}

func findVideoScore(t *testing.T, scores []VideoScore, prompt string) VideoScore {
	t.Helper()
	for _, score := range scores {
		if score.Prompt == prompt {
			return score
		}
	}
	t.Fatalf("score for %q not found", prompt)
	return VideoScore{}
}

func mustParseVideoPrompt(t *testing.T, prompt string) VideoCondition {
	t.Helper()
	condition, err := ParseVideoPrompt(prompt)
	if err != nil {
		t.Fatal(err)
	}
	return condition
}
