package learning_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// TestTruncationAlignEnd moves the window boundaries so the last window ends at
// the final row. With 23 rows and truncation 3 the start-aligned last window
// holds rows 21 and 22 only, while the end-aligned one holds rows 20, 21 and
// 22, so Step's last-row loss reaches the full 3 rows. Recompute keeps the
// aligned update bit for bit.
func TestTruncationAlignEnd(t *testing.T) {
	ctx := context.Background()
	for _, fx := range recomputeTrainerFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			o := fx.options
			o.Truncation = 3
			start := newRecomputeTrainer(t, fx, o)
			o.TruncationAlign = learning.TruncationAlignEnd
			end := newRecomputeTrainer(t, fx, o)
			o.Recompute = &learning.Recompute{SegmentSteps: 4}
			recompute := newRecomputeTrainer(t, fx, o)
			s, err := start.Step(ctx, fx.input, fx.target)
			if err != nil {
				t.Fatal(err)
			}
			e, err := end.Step(ctx, fx.input, fx.target)
			if err != nil {
				t.Fatal(err)
			}
			r, err := recompute.Step(ctx, fx.input, fx.target)
			if err != nil {
				t.Fatal(err)
			}
			if s.GradientHorizonSteps != 2 || e.GradientHorizonSteps != 3 || r.GradientHorizonSteps != 3 {
				t.Fatalf("horizons start %d end %d recompute %d, want 2, 3, 3", s.GradientHorizonSteps, e.GradientHorizonSteps, r.GradientHorizonSteps)
			}
			if reflect.DeepEqual(start.Snapshot().Parameters, end.Snapshot().Parameters) {
				t.Fatal("end alignment left the update unchanged")
			}
			if !reflect.DeepEqual(recompute.Snapshot().Parameters, end.Snapshot().Parameters) {
				t.Fatal("recompute changed the end-aligned update")
			}
		})
	}
}

func TestTruncationAlignValidation(t *testing.T) {
	fx := recomputeTrainerFixtures()[0]
	for name, c := range map[string]struct {
		truncation int
		align      string
		want       string
	}{
		"end without truncation": {0, learning.TruncationAlignEnd, "positive truncation"},
		"unknown":                {3, "middle", "unsupported truncation align"},
	} {
		o := fx.options
		o.Truncation, o.TruncationAlign = c.truncation, c.align
		if _, err := learning.NewTrainer(fx.config, fx.params, o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: NewTrainer error = %v, want %q", name, err, c.want)
		}
	}
	o := fx.options
	o.Truncation, o.TruncationAlign = 3, learning.TruncationAlignStart
	if _, err := learning.NewTrainer(fx.config, fx.params, o); err != nil {
		t.Errorf("explicit start alignment: %v", err)
	}
}
