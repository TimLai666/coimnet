// Package bench measures what a trained model costs to run, in a form that a
// model built with another framework can report too, so the two can be put
// side by side on the same machine. A Result records the model size, the
// process memory peak, per-sample latency and task quality, together with the
// host it was measured on; Compare reads two or more Results and states, per
// dimension, which side needs less.
//
// The schema is plain JSON so a PyTorch script can write the same file; see
// scripts/bench/coimbench.py.
package bench

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// SchemaVersion names the Result layout. A reader refuses any other value.
const SchemaVersion = "coimnet-bench/v1"

// Result is one model measured on one host.
type Result struct {
	SchemaVersion string `json:"schema_version"`
	// Framework and Model name what was measured, for example "coimnet" and
	// "kmnist-single-character-h24".
	Framework string `json:"framework"`
	Model     string `json:"model"`
	// Task names the shared task, and Split the exact evaluation selection,
	// such as a SHA-256 of the test indices; Compare refuses Results whose task
	// or split differ, because their quality numbers would not be comparable.
	Task  string `json:"task"`
	Split string `json:"split"`
	Host  Host   `json:"host"`
	// Parameters counts every stored number inference reads; Trainable counts
	// the ones training may change.
	Parameters int64 `json:"parameters"`
	Trainable  int64 `json:"trainable"`
	// PeakRSSBytes is the process resident-set peak over loading the model and
	// running the whole evaluation in a fresh process. IdleRSSBytes is the peak
	// of the same runtime before the model is loaded, so the difference is what
	// the model itself added.
	PeakRSSBytes uint64 `json:"peak_rss_bytes"`
	IdleRSSBytes uint64 `json:"idle_rss_bytes"`
	// PeakDeviceBytes is the accelerator memory peak, zero on a CPU run.
	PeakDeviceBytes uint64  `json:"peak_device_bytes,omitempty"`
	Device          string  `json:"device"`
	Threads         int     `json:"threads"`
	Latency         Latency `json:"latency"`
	// Quality holds the task metrics, for example "accuracy". Higher is better
	// for every metric named in Higher; the rest are lower-is-better.
	Quality map[string]float64 `json:"quality"`
	Higher  []string           `json:"higher_is_better"`
	Notes   []string           `json:"notes,omitempty"`
}

// Host is the machine a Result was measured on. Two Results share hardware
// only when their Machine names and CPU models agree.
type Host struct {
	Machine    string `json:"machine"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	CPU        string `json:"cpu"`
	LogicalCPU int    `json:"logical_cpus"`
	MemoryB    uint64 `json:"memory_bytes"`
	GPU        string `json:"gpu,omitempty"`
	Runtime    string `json:"runtime"`
}

// Latency summarizes single-sample inference wall time, measured after Warmup
// untimed calls.
type Latency struct {
	Warmup  int     `json:"warmup"`
	Samples int     `json:"samples"`
	MeanMS  float64 `json:"mean_ms"`
	P50MS   float64 `json:"p50_ms"`
	P90MS   float64 `json:"p90_ms"`
	P99MS   float64 `json:"p99_ms"`
	MinMS   float64 `json:"min_ms"`
	MaxMS   float64 `json:"max_ms"`
}

// Measure times fn once per sample index in [0, samples), after warmup untimed
// calls on indices taken modulo samples. Every call goes through fn, so the
// timed work is exactly what a caller serving one request at a time pays.
func Measure(ctx context.Context, warmup, samples int, fn func(context.Context, int) error) (Latency, error) {
	if ctx == nil {
		return Latency{}, errors.New("bench: nil context")
	}
	if warmup < 0 || samples <= 0 || fn == nil {
		return Latency{}, errors.New("bench: need warmup >= 0, samples > 0 and a function")
	}
	for i := 0; i < warmup; i++ {
		if err := fn(ctx, i%samples); err != nil {
			return Latency{}, fmt.Errorf("bench: warmup call %d: %w", i, err)
		}
	}
	times := make([]float64, samples)
	for i := range times {
		if err := ctx.Err(); err != nil {
			return Latency{}, err
		}
		start := time.Now()
		err := fn(ctx, i)
		times[i] = float64(time.Since(start).Nanoseconds()) / 1e6
		if err != nil {
			return Latency{}, fmt.Errorf("bench: sample %d: %w", i, err)
		}
	}
	return summarize(warmup, times), nil
}

func summarize(warmup int, times []float64) Latency {
	sorted := append([]float64(nil), times...)
	sort.Float64s(sorted)
	var sum float64
	for _, v := range sorted {
		sum += v
	}
	return Latency{
		Warmup: warmup, Samples: len(sorted), MeanMS: sum / float64(len(sorted)),
		P50MS: quantile(sorted, .5), P90MS: quantile(sorted, .9), P99MS: quantile(sorted, .99),
		MinMS: sorted[0], MaxMS: sorted[len(sorted)-1],
	}
}

// quantile is the nearest-rank quantile of an ascending slice: the smallest
// value with at least q of the samples at or below it. The Python writer uses
// the same rule, so the two sides agree on what p90 means.
func quantile(sorted []float64, q float64) float64 {
	rank := int(math.Ceil(q*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	return sorted[rank]
}

// Validate refuses a Result a comparison could not read honestly.
func (r Result) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("bench: schema %q, want %q", r.SchemaVersion, SchemaVersion)
	}
	if r.Framework == "" || r.Model == "" || r.Task == "" || r.Split == "" {
		return errors.New("bench: framework, model, task and split are required")
	}
	if r.Host.Machine == "" || r.Host.CPU == "" {
		return errors.New("bench: host machine and cpu are required")
	}
	if r.Parameters <= 0 || r.Trainable < 0 || r.Trainable > r.Parameters {
		return fmt.Errorf("bench: parameters %d and trainable %d are inconsistent", r.Parameters, r.Trainable)
	}
	if r.PeakRSSBytes == 0 || r.IdleRSSBytes > r.PeakRSSBytes {
		return fmt.Errorf("bench: peak rss %d and idle rss %d are inconsistent", r.PeakRSSBytes, r.IdleRSSBytes)
	}
	if r.Latency.Samples <= 0 || !(r.Latency.P50MS > 0) {
		return errors.New("bench: latency needs at least one positive sample")
	}
	if len(r.Quality) == 0 {
		return errors.New("bench: at least one quality metric is required")
	}
	for name, v := range r.Quality {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("bench: quality %q is not finite", name)
		}
	}
	return nil
}
