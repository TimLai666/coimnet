package bench

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// Nearest rank on ten values 1..10: p50 is the 5th, p90 the 9th, p99 the 10th.
func TestSummarizeUsesNearestRank(t *testing.T) {
	got := summarize(2, []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1})
	want := Latency{Warmup: 2, Samples: 10, MeanMS: 5.5, P50MS: 5, P90MS: 9, P99MS: 10, MinMS: 1, MaxMS: 10}
	if got != want {
		t.Fatalf("summarize = %+v, want %+v", got, want)
	}
}

func TestMeasureCallsEverySampleAfterWarmup(t *testing.T) {
	var calls []int
	lat, err := Measure(context.Background(), 3, 2, func(_ context.Context, i int) error {
		calls = append(calls, i)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{0, 1, 0, 0, 1}; len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if lat.Samples != 2 || lat.Warmup != 3 {
		t.Fatalf("latency = %+v", lat)
	}
	if _, err := Measure(context.Background(), 0, 1, func(context.Context, int) error { return errors.New("x") }); err == nil {
		t.Fatal("Measure() hid the call error")
	}
}

func TestCountParameters(t *testing.T) {
	p := learning.Parameters{Encoder: make([]float64, 6), Readout: make([]float64, 4)}
	p.Core.Weights, p.Core.Bias, p.Core.LogTau = make([]float64, 3), make([]float64, 2), make([]float64, 2)
	if got := CountParameters(p); got != 17 {
		t.Fatalf("CountParameters = %d, want 17", got)
	}
	o := learning.Options{Trainable: learning.Trainable{Encoder: true, Weights: true, Readout: true}}
	o.Masks = &learning.UpdateMasks{Edges: []bool{true, false, true}}
	if got := CountTrainable(p, o); got != 12 {
		t.Fatalf("CountTrainable = %d, want 12", got)
	}
}

func result(framework string, quality, ms float64, params int64, rss uint64) Result {
	return Result{
		SchemaVersion: SchemaVersion, Framework: framework, Model: "m", Task: "t", Split: "s",
		Host:       Host{Machine: "box", CPU: "cpu"},
		Parameters: params, Trainable: params, PeakRSSBytes: rss, IdleRSSBytes: rss / 2,
		Device: "cpu", Threads: 1, Latency: Latency{Samples: 1, P50MS: ms},
		Quality: map[string]float64{"accuracy": quality}, Higher: []string{"accuracy"},
	}
}

func TestCompareNeedsEveryDimension(t *testing.T) {
	c, err := Compare(result("coimnet", .8, 1, 100, 100), []Result{
		result("torch", .7, 2, 200, 200),  // all four hold
		result("torch", .9, 2, 200, 200),  // lower quality
		result("torch", .7, .5, 200, 200), // slower
		result("torch", .7, 2, 50, 200),   // more parameters
		result("torch", .7, 2, 200, 50),   // more memory
	}, "accuracy")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false, false, false, false} {
		if c.Versus[i].Wins != want {
			t.Errorf("versus %d wins = %t, want %t: %+v", i, c.Versus[i].Wins, want, c.Versus[i])
		}
	}
	if md := c.Markdown(); !strings.Contains(md, "| torch/m |") || !strings.Contains(md, "yes") {
		t.Fatalf("markdown misses rows:\n%s", md)
	}
}

func TestCompareRefusesDifferentSettings(t *testing.T) {
	base := result("coimnet", .8, 1, 100, 100)
	for name, change := range map[string]func(*Result){
		"task":    func(r *Result) { r.Task = "other" },
		"split":   func(r *Result) { r.Split = "other" },
		"machine": func(r *Result) { r.Host.Machine = "other" },
		"device":  func(r *Result) { r.Device = "cuda" },
		"threads": func(r *Result) { r.Threads = 4 },
		"metric":  func(r *Result) { r.Quality = map[string]float64{"loss": 1} },
		"schema":  func(r *Result) { r.SchemaVersion = "v0" },
	} {
		b := result("torch", .7, 2, 200, 200)
		change(&b)
		if _, err := Compare(base, []Result{b}, "accuracy"); err == nil {
			t.Errorf("%s: Compare() accepted a mismatched baseline", name)
		}
	}
}
