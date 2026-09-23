package media

import (
	"context"
	"encoding/json"
	"testing"
)

// TestRequestHeadLearnsSeenRequests keeps the held-out pair out of training.
func TestRequestHeadLearnsSeenRequests(t *testing.T) {
	ctx := context.Background()
	config := DefaultGeneratorConfig(ModalityImage)
	config.Embed = 4
	config.Hidden = 16
	config.LearningRate = .03
	config.Seed = 17
	head, err := NewRequestHead(config)
	if err != nil {
		t.Fatal(err)
	}
	seen := make([]string, 0, 8)
	for _, colour := range ImageColours {
		for _, shape := range ImageShapes {
			prompt := colour + " " + shape
			if prompt != "blue diagonal" {
				seen = append(seen, prompt)
			}
		}
	}
	correct := func() int {
		count := 0
		for _, prompt := range seen {
			request, err := head.Request(ctx, prompt)
			if err != nil {
				t.Fatal(err)
			}
			if request.Colour+" "+request.Shape == prompt {
				count++
			}
		}
		return count
	}
	before := correct()
	stepCalls := make(map[string]int)
	step := func(prompt string) error {
		stepCalls[prompt]++
		_, _, err := head.Step(ctx, prompt)
		return err
	}
	for round := 0; round < 60; round++ {
		for _, prompt := range seen {
			if err := step(prompt); err != nil {
				t.Fatalf("Step(%q): %v", prompt, err)
			}
		}
	}
	after := correct()
	heldOut, err := head.Request(ctx, "blue diagonal")
	if err != nil {
		t.Fatal(err)
	}
	if stepCalls["blue diagonal"] != 0 {
		t.Fatalf("held-out prompt was passed to Step %d times", stepCalls["blue diagonal"])
	}
	t.Logf("seen correct requests: before=%d/%d after=%d/%d; held-out blue diagonal request=%+v", before, len(seen), after, len(seen), heldOut)
	if after <= before {
		t.Fatalf("seen request accuracy did not improve: before=%d after=%d", before, after)
	}
}

// TestExternalToolRendersAndMarksRole verifies fixture output and provenance.
func TestExternalToolRendersAndMarksRole(t *testing.T) {
	tool, err := NewExternalTool(ToolPolicy{MaxCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	output, err := tool.Render(context.Background(), ToolRequest{Colour: "red", Shape: "square"})
	if err != nil {
		t.Fatal(err)
	}
	if output.Role != "external_tool" || output.Tool != ExternalImageTool {
		t.Fatalf("output role and tool = %q, %q", output.Role, output.Tool)
	}
	condition, _, err := ClassifyImage(output.Pixels)
	if err != nil {
		t.Fatal(err)
	}
	if condition != (ImageCondition{Colour: "red", Shape: "square"}) {
		t.Fatalf("ClassifyImage() = %+v, want red square", condition)
	}
	if counts := tool.Counts(); counts.Calls != 1 {
		t.Fatalf("Counts() = %+v, want one call", counts)
	}
}

// TestExternalToolRefusesOutsideTheSchema checks exact schema enforcement.
func TestExternalToolRefusesOutsideTheSchema(t *testing.T) {
	tool, err := NewExternalTool(ToolPolicy{MaxCalls: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"colour":"red","shape":"square","prompt":"red square"}`),
		json.RawMessage(`{"colour":"red"}`),
		json.RawMessage(`{"colour":"red","shape":1}`),
	} {
		if _, err := tool.InvokeRaw(context.Background(), args); err == nil {
			t.Errorf("InvokeRaw(%s) succeeded", args)
		}
	}
	if counts := tool.Counts(); counts.Calls != 0 || counts.Refused != 3 {
		t.Fatalf("Counts() = %+v, want zero calls and three refusals", counts)
	}
}

// TestExternalToolBudget checks that calls beyond the budget never run.
func TestExternalToolBudget(t *testing.T) {
	tool, err := NewExternalTool(ToolPolicy{MaxCalls: 2})
	if err != nil {
		t.Fatal(err)
	}
	request := ToolRequest{Colour: "green", Shape: "cross"}
	for i := 0; i < 2; i++ {
		if _, err := tool.Render(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tool.Render(context.Background(), request); err == nil {
		t.Fatal("third Render() succeeded beyond the budget")
	}
	if counts := tool.Counts(); counts.Calls != 2 || counts.BudgetRefused != 1 {
		t.Fatalf("Counts() = %+v, want two calls and one budget refusal", counts)
	}
}

// TestToolPolicyValidate checks the inclusive tool call budget limits.
func TestToolPolicyValidate(t *testing.T) {
	for _, maxCalls := range []int{0, 10001} {
		if err := (ToolPolicy{MaxCalls: maxCalls}).Validate(); err == nil {
			t.Errorf("ToolPolicy{MaxCalls: %d}.Validate() succeeded", maxCalls)
		}
	}
	for _, maxCalls := range []int{1, 10000} {
		if err := (ToolPolicy{MaxCalls: maxCalls}).Validate(); err != nil {
			t.Fatalf("valid policy rejected: %v", err)
		}
	}
}
