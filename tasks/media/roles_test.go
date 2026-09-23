package media

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type countingRenderer struct {
	output ToolOutput
	counts ToolCounts
}

func (r *countingRenderer) Render(_ context.Context, _ ToolRequest) (ToolOutput, error) {
	r.counts.Calls++
	return r.output, nil
}

func (r *countingRenderer) Counts() ToolCounts { return r.counts }

func smallRolesConfig() RolesConfig {
	c := DefaultRolesConfig()
	c.Epochs = 10
	c.Seeds = []uint64{1}
	return c
}

func TestRunRolesReport(t *testing.T) {
	c := smallRolesConfig()
	report, err := RunRoles(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != RolesSchemaVersion || len(report.ConfigHash) != 64 {
		t.Fatalf("schema/hash = %q/%q", report.SchemaVersion, report.ConfigHash)
	}
	if !reflect.DeepEqual(report.Config, c) || len(report.Runs) != 1 || report.Runs[0].Failed {
		t.Fatalf("unexpected report config/runs: %+v", report)
	}
	run := report.Runs[0]
	assertRoleMode(t, run.CoreGenerated, "core_generated", "core", "judged_pixels", c.Epochs*8)
	assertRoleMode(t, run.FixedDecoder, "fixed_decoder", "fixed_decoder", "judged_pixels_through_fixed_decoder", c.Epochs*8)
	assertRoleMode(t, run.ExternalTool, "external_tool", "external_tool", "correct_requests", c.Epochs*8)
	decoder, err := DefaultFixedDecoder()
	if err != nil {
		t.Fatal(err)
	}
	if run.FixedDecoder.DecoderHash != decoder.Hash() || run.FixedDecoder.Capacity.DecoderParameters != decoder.Parameters() {
		t.Fatalf("fixed decoder identity/capacity = %q/%d, want %q/%d", run.FixedDecoder.DecoderHash, run.FixedDecoder.Capacity.DecoderParameters, decoder.Hash(), decoder.Parameters())
	}
	if run.ExternalTool.Tool != ExternalImageTool || run.ExternalTool.ToolCounts == nil || run.ExternalTool.ToolCounts.Calls != 9 {
		t.Fatalf("external tool = %+v", run.ExternalTool)
	}
	wantAssumptions := []string{
		"The fixture has nine images of three colours and three shapes; it proves the three modes and their bookkeeping, not open-ended image generation.",
		"In fixed_decoder mode the judged pixels depend on the core's latent and on the frozen decoder named by decoder_hash; decoder_alone shows what that decoder reproduces by itself.",
		"In external_tool mode the core's capability is only its structured request; the tool's pixels are reported as tool_result and never counted as core capability.",
		"The external tool here is a local stand-in that renders the requested fixture image; no network or third-party model is called.",
		"The fixed decoder is an image autoencoder; time alignment does not apply to single images and is not measured here.",
	}
	if !reflect.DeepEqual(report.Assumptions, wantAssumptions) {
		t.Fatalf("assumptions = %#v", report.Assumptions)
	}
	if run.DecoderAlone.ReconstructionMSE <= 0 || run.DecoderAlone.Correct < 0 || run.DecoderAlone.Correct > 9 || run.LatentStats.MeanAbsError < 0 {
		t.Fatalf("decoder diagnostics = %+v, latent stats = %+v", run.DecoderAlone, run.LatentStats)
	}
}

func assertRoleMode(t *testing.T, mode RoleMode, wantMode, wantRole, wantMeasure string, wantUpdates int) {
	t.Helper()
	prompts, err := fixturePrompts(ModalityImage)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode != wantMode || mode.Updates != wantUpdates || mode.CoreCapability.Measure != wantMeasure || len(mode.Scores) != 9 {
		t.Fatalf("mode summary = %+v", mode)
	}
	if mode.CoreBefore.Measure != wantMeasure || mode.CoreBefore.SeenCorrect > 8 || mode.CoreBefore.HeldOutCorrect > 1 {
		t.Fatalf("core before = %+v", mode.CoreBefore)
	}
	for i, score := range mode.Scores {
		if score.Prompt != prompts[i] || score.Seen != (score.Prompt != "blue diagonal") || score.Role != wantRole || score.Correct != (score.Judged == score.Prompt) {
			t.Fatalf("score %d = %+v", i, score)
		}
	}
}

func TestRunRolesToolResultIsNotCoreCapability(t *testing.T) {
	c := smallRolesConfig()
	actual, err := RunRoles(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	pixels, err := RenderImage(ImageCondition{Colour: "red", Shape: "square"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &countingRenderer{output: ToolOutput{Role: "external_tool", Tool: ExternalImageTool, Pixels: pixels}}
	injected, err := runRoles(context.Background(), c, func(ToolPolicy) (imageRenderer, error) { return fake, nil })
	if err != nil {
		t.Fatal(err)
	}
	got, want := injected.Runs[0], actual.Runs[0]
	if !reflect.DeepEqual(got.ExternalTool.CoreCapability, want.ExternalTool.CoreCapability) || !reflect.DeepEqual(got.ExternalTool.CoreBefore, want.ExternalTool.CoreBefore) {
		t.Fatalf("tool output changed core capability: before %v/%v after %v/%v", got.ExternalTool.CoreBefore, want.ExternalTool.CoreBefore, got.ExternalTool.CoreCapability, want.ExternalTool.CoreCapability)
	}
	if got.ExternalTool.ToolResult == nil || want.ExternalTool.ToolResult == nil || reflect.DeepEqual(*got.ExternalTool.ToolResult, *want.ExternalTool.ToolResult) || got.ExternalTool.ToolResult.SeenCorrect != 1 || got.ExternalTool.ToolResult.HeldOutCorrect != 0 {
		t.Fatalf("tool results fake=%+v real=%+v", got.ExternalTool.ToolResult, want.ExternalTool.ToolResult)
	}
	if !reflect.DeepEqual(got.CoreGenerated, want.CoreGenerated) || !reflect.DeepEqual(got.FixedDecoder, want.FixedDecoder) {
		t.Fatal("injecting a renderer changed a core-generated mode")
	}
}

func TestRunRolesBudgetShortfall(t *testing.T) {
	c := smallRolesConfig()
	c.MaxToolCalls = 3
	report, err := RunRoles(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	mode := report.Runs[0].ExternalTool
	if report.Runs[0].Failed || mode.ToolCounts == nil || mode.ToolCounts.Calls != 3 || mode.ToolCounts.BudgetRefused != 6 {
		t.Fatalf("run/tool counts = %+v / %+v", report.Runs[0], mode.ToolCounts)
	}
	for i, score := range mode.Scores {
		if i < 3 && score.Judged == "" {
			t.Fatalf("score %d was unexpectedly refused: %+v", i, score)
		}
		if i >= 3 && (score.Judged != "" || score.Correct) {
			t.Fatalf("score %d should record a refused render: %+v", i, score)
		}
	}
}

func TestRunRolesNeverTrainsHeldOut(t *testing.T) {
	conditions, err := fixturePrompts(ModalityImage)
	if err != nil {
		t.Fatal(err)
	}
	seen, _ := partitionPrompts(conditions, []string{"blue diagonal"})
	counts := make(map[string]int)
	updates, err := trainRoleConditions(context.Background(), seen, 10, func(_ context.Context, prompt string) error {
		counts[prompt]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if updates != 80 || counts["blue diagonal"] != 0 || len(counts) != 8 {
		t.Fatalf("unexpected training prompts: updates=%d counts=%v", updates, counts)
	}
	for _, prompt := range seen {
		if counts[prompt] != 10 {
			t.Fatalf("prompt %q count=%d, want 10", prompt, counts[prompt])
		}
	}

	c := smallRolesConfig()
	report, err := RunRoles(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || report.Runs[0].Failed {
		t.Fatalf("unexpected role run: %+v", report.Runs)
	}
	for _, mode := range []struct {
		name string
		mode RoleMode
	}{
		{"core_generated", report.Runs[0].CoreGenerated},
		{"fixed_decoder", report.Runs[0].FixedDecoder},
		{"external_tool", report.Runs[0].ExternalTool},
	} {
		if want := c.Epochs * 8; mode.mode.Updates != want {
			t.Errorf("%s updates=%d, want %d", mode.name, mode.mode.Updates, want)
		}
	}
}

func TestRunRolesIsDeterministic(t *testing.T) {
	c := smallRolesConfig()
	first, err := RunRoles(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunRoles(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("reports differ across identical runs")
	}
}

func TestRolesConfigValidate(t *testing.T) {
	valid := smallRolesConfig()
	tests := []struct {
		name string
		edit func(*RolesConfig)
	}{
		{"non-image modality", func(c *RolesConfig) { c.Generator.Modality = ModalityAudio }},
		{"empty holdout", func(c *RolesConfig) { c.Holdout = nil }},
		{"unknown holdout", func(c *RolesConfig) { c.Holdout = []string{"purple triangle"} }},
		{"duplicate holdout", func(c *RolesConfig) { c.Holdout = []string{"blue diagonal", "blue diagonal"} }},
		{"zero epochs", func(c *RolesConfig) { c.Epochs = 0 }},
		{"too many epochs", func(c *RolesConfig) { c.Epochs = 10001 }},
		{"empty seeds", func(c *RolesConfig) { c.Seeds = nil }},
		{"duplicate seeds", func(c *RolesConfig) { c.Seeds = []uint64{1, 1} }},
		{"zero tool budget", func(c *RolesConfig) { c.MaxToolCalls = 0 }},
		{"too many tool calls", func(c *RolesConfig) { c.MaxToolCalls = 10001 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := valid
			test.edit(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("Validate returned nil")
			}
		})
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}
