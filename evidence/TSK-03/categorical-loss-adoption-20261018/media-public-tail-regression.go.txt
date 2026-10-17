package media

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestRequestHeadStepTinyTail146(t *testing.T) {
	ctx := context.Background()
	config := DefaultGeneratorConfig(ModalityImage)
	config.Embed, config.Hidden, config.Settle, config.LearningRate, config.Seed = 4, 8, 3, .01, 17
	head, err := NewRequestHead(config)
	if err != nil {
		t.Fatal(err)
	}
	input, colourIndex, shapeIndex, err := head.requestInput("red square")
	if err != nil {
		t.Fatal(err)
	}

	baseline := head.trainer.Snapshot()
	for i := range baseline.Parameters.Readout {
		baseline.Parameters.Readout[i] = 1
	}
	baseline.Options.Trainable = learning.Trainable{Readout: true}
	head.trainer, err = learning.NewTrainer(baseline.Config, baseline.Parameters, baseline.Options)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := head.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	outputSize := len(ImageColours) + len(ImageShapes)
	if len(outputs) != len(input) || len(outputs) == 0 || len(outputs[len(outputs)-1]) != outputSize {
		gotWidth := 0
		if len(outputs) > 0 {
			gotWidth = len(outputs[len(outputs)-1])
		}
		t.Fatalf("baseline output shape = %d x %d, want %d x %d", len(outputs), gotWidth, len(input), outputSize)
	}
	a := outputs[len(outputs)-1][0]
	if math.IsNaN(a) || math.IsInf(a, 0) || a == 0 {
		t.Fatalf("baseline readout a = %g, want finite nonzero", a)
	}
	scaled := -40 / a
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
		t.Fatalf("tail readout = %g, want finite", scaled)
	}

	tail := head.trainer.Snapshot()
	colourWinner := colourIndex
	shapeWinner := len(ImageColours) + shapeIndex
	for i := range tail.Parameters.Readout {
		column := i % tail.Config.OutputSize
		if column == colourWinner || column == shapeWinner {
			tail.Parameters.Readout[i] = 0
		} else {
			tail.Parameters.Readout[i] = scaled
		}
	}
	head.trainer, err = learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err = head.trainer.PredictAll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != len(input) || len(outputs) == 0 {
		t.Fatalf("tail output rows = %d, want %d", len(outputs), len(input))
	}
	logits := outputs[len(outputs)-1]
	if len(logits) != outputSize {
		t.Fatalf("tail output width = %d, want %d", len(logits), outputSize)
	}
	expected := 0.0
	for _, group := range []struct {
		start  int
		winner int
	}{
		{start: 0, winner: colourWinner},
		{start: len(ImageColours), winner: shapeWinner},
	} {
		if logits[group.winner] != 0 {
			t.Fatalf("winner logits[%d] = %.17g, want 0; logits=%v", group.winner, logits[group.winner], logits)
		}
		nonWinner := group.start
		if nonWinner == group.winner {
			nonWinner++
		}
		d := logits[nonWinner]
		if math.IsNaN(d) || math.IsInf(d, 0) || d >= 0 || math.Abs(d+40) > 1e-4 {
			t.Fatalf("group logits=%v, nonwinner d=%.17g; want finite negative value near -40", logits[group.start:group.start+len(ImageColours)], d)
		}
		for i := group.start; i < group.start+len(ImageColours); i++ {
			if i == group.winner {
				continue
			}
			if logits[i] != d {
				t.Fatalf("group logits[%d] = %.17g, want exactly d=%.17g; logits=%v", i, logits[i], d, logits)
			}
		}
		expected += math.Log1p(2 * math.Exp(d))
	}
	expected /= 2

	before := head.trainer.Snapshot()
	result, loss, stepErr := head.Step(ctx, "red square")
	after := head.trainer.Snapshot()
	stateResultJSON, marshalErr := json.Marshal(struct {
		Before learning.TrainingSnapshot `json:"before"`
		After  learning.TrainingSnapshot `json:"after"`
		Result learning.StepResult       `json:"result"`
	}{Before: before, After: after, Result: result})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	t.Logf("step_state_result_json_sha256=%x logits=%v expected=%.17g loss=%.17g err=%v applied=%v", sha256.Sum256(stateResultJSON), logits, expected, loss, stepErr, result.Applied)
	if stepErr != nil {
		t.Errorf("Step error: %v", stepErr)
	}
	if !result.Applied {
		t.Errorf("Step result Applied = false, want true")
	}
	if math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
		t.Errorf("Step loss = %.17g, want finite positive value near %.17g", loss, expected)
	} else if math.Abs(loss-expected) > math.Abs(expected)*1e-12 {
		t.Errorf("Step loss = %.17g, want %.17g", loss, expected)
	}
}
