package textgen

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestTextgenPublicTinyTail146(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"NLL", "Step"} {
		t.Run(mode, func(t *testing.T) {
			config := ModelConfig{Embed: 4, Hidden: 8, Settle: 3, LearningRate: .01, Seed: 17}
			m, err := NewModel(testVocabulary(t), config)
			if err != nil {
				t.Fatal(err)
			}
			ex := Example{IDs: []int{0, 1}, Scored: []bool{false, true}}

			baseline := m.Snapshot()
			for i := range baseline.Parameters.Readout {
				baseline.Parameters.Readout[i] = 1
			}
			baseline.Options.Trainable = learning.Trainable{Readout: true}
			m.trainer, err = learning.NewTrainer(baseline.Config, baseline.Parameters, baseline.Options)
			if err != nil {
				t.Fatal(err)
			}
			logits, err := m.Logits(ctx, ex.IDs)
			if err != nil {
				t.Fatal(err)
			}
			if len(logits) != len(ex.IDs) || len(logits) == 0 || len(logits[0]) != m.vocabSize {
				gotWidth := 0
				if len(logits) > 0 {
					gotWidth = len(logits[0])
				}
				t.Fatalf("baseline logits shape = %d x %d, want %d x %d", len(logits), gotWidth, len(ex.IDs), m.vocabSize)
			}
			a := logits[0][0]
			if math.IsNaN(a) || math.IsInf(a, 0) || a == 0 {
				t.Fatalf("baseline readout a = %g, want finite nonzero", a)
			}
			scaled := -40 / a
			if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
				t.Fatalf("tail readout = %g, want finite", scaled)
			}

			tail := m.trainer.Snapshot()
			for i := range tail.Parameters.Readout {
				if i%tail.Config.OutputSize == 1 {
					tail.Parameters.Readout[i] = 0
				} else {
					tail.Parameters.Readout[i] = scaled
				}
			}
			m.trainer, err = learning.NewTrainer(tail.Config, tail.Parameters, tail.Options)
			if err != nil {
				t.Fatal(err)
			}
			logits, err = m.Logits(ctx, ex.IDs)
			if err != nil {
				t.Fatal(err)
			}
			if len(logits) != len(ex.IDs) || len(logits) == 0 || len(logits[0]) != m.vocabSize {
				gotWidth := 0
				if len(logits) > 0 {
					gotWidth = len(logits[0])
				}
				t.Fatalf("tail logits shape = %d x %d, want %d x %d", len(logits), gotWidth, len(ex.IDs), m.vocabSize)
			}
			row := logits[0]
			if row[1] != 0 {
				t.Fatalf("winner logits[1] = %.17g, want 0; logits=%v", row[1], row)
			}
			d := row[0]
			if math.IsNaN(d) || math.IsInf(d, 0) || d >= 0 || math.Abs(d+40) > 1e-4 {
				t.Fatalf("nonwinner d=%.17g, want finite negative value near -40; logits=%v", d, row)
			}
			for i, value := range row {
				if i == 1 {
					continue
				}
				if value != d {
					t.Fatalf("logits[%d] = %.17g, want exactly d=%.17g; logits=%v", i, value, d, row)
				}
			}
			expected := math.Log1p(258 * math.Exp(d))

			if mode == "NLL" {
				before := m.Snapshot()
				loss, count, callErr := m.NLL(ctx, ex)
				after := m.Snapshot()
				beforeJSON, marshalErr := json.Marshal(before)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				afterJSON, marshalErr := json.Marshal(after)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				t.Logf("nll_before_snapshot_sha256=%x nll_after_snapshot_sha256=%x logits=%v expected=%.17g loss=%.17g err=%v count=%d", sha256.Sum256(beforeJSON), sha256.Sum256(afterJSON), row, expected, loss, callErr, count)
				if callErr != nil {
					t.Errorf("NLL error: %v", callErr)
				}
				if count != 1 {
					t.Errorf("NLL count = %d, want 1", count)
				}
				if !reflect.DeepEqual(before, after) {
					t.Errorf("NLL changed the complete snapshot")
				}
				if math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
					t.Errorf("NLL loss = %.17g, want finite positive value near %.17g", loss, expected)
				} else if math.Abs(loss-expected) > math.Abs(expected)*1e-12 {
					t.Errorf("NLL loss = %.17g, want %.17g", loss, expected)
				}
				return
			}

			before := m.Snapshot()
			result, loss, callErr := m.Step(ctx, ex)
			after := m.Snapshot()
			stateResultJSON, marshalErr := json.Marshal(struct {
				Before learning.TrainingSnapshot `json:"before"`
				After  learning.TrainingSnapshot `json:"after"`
				Result learning.StepResult       `json:"result"`
			}{Before: before, After: after, Result: result})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			t.Logf("step_state_result_json_sha256=%x logits=%v expected=%.17g loss=%.17g err=%v applied=%v", sha256.Sum256(stateResultJSON), row, expected, loss, callErr, result.Applied)
			if callErr != nil {
				t.Errorf("Step error: %v", callErr)
			}
			if !result.Applied {
				t.Errorf("Step result Applied = false, want true")
			}
			if math.IsNaN(loss) || math.IsInf(loss, 0) || loss <= 0 {
				t.Errorf("Step loss = %.17g, want finite positive value near %.17g", loss, expected)
			} else if math.Abs(loss-expected) > math.Abs(expected)*1e-12 {
				t.Errorf("Step loss = %.17g, want %.17g", loss, expected)
			}
		})
	}
}
