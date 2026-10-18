package bench

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Comparison puts one candidate against each baseline measured on the same
// host, task and evaluation split.
type Comparison struct {
	Task      string    `json:"task"`
	Split     string    `json:"split"`
	Machine   string    `json:"machine"`
	Metric    string    `json:"metric"`
	Candidate Result    `json:"candidate"`
	Baselines []Result  `json:"baselines"`
	Versus    []Verdict `json:"versus"`
}

// Verdict states, for one baseline, whether the candidate scores higher on
// the primary metric and whether it needs no more of each resource. Wins is
// true only when all four hold: that is the "better at the same hardware
// requirement" claim.
type Verdict struct {
	Baseline        string  `json:"baseline"`
	QualityDelta    float64 `json:"quality_delta"`
	HigherQuality   bool    `json:"higher_quality"`
	ParametersRatio float64 `json:"parameters_ratio"`
	PeakRSSRatio    float64 `json:"peak_rss_ratio"`
	LatencyRatio    float64 `json:"latency_p50_ratio"`
	WithinParams    bool    `json:"within_parameters"`
	WithinMemory    bool    `json:"within_peak_rss"`
	Faster          bool    `json:"not_slower"`
	Wins            bool    `json:"wins"`
}

// Compare checks that every Result shares the candidate's task, split, host,
// device and thread count, then judges the candidate on metric against each
// baseline. Memory compares whole-process peaks, because that is what a
// machine has to provide; the model-only difference is shown in the table.
func Compare(candidate Result, baselines []Result, metric string) (Comparison, error) {
	if len(baselines) == 0 {
		return Comparison{}, errors.New("bench: at least one baseline is required")
	}
	if err := candidate.Validate(); err != nil {
		return Comparison{}, fmt.Errorf("candidate: %w", err)
	}
	if _, ok := candidate.Quality[metric]; !ok {
		return Comparison{}, fmt.Errorf("bench: candidate has no quality metric %q", metric)
	}
	higher := slices.Contains(candidate.Higher, metric)
	c := Comparison{Task: candidate.Task, Split: candidate.Split, Machine: candidate.Host.Machine, Metric: metric, Candidate: candidate}
	for _, b := range baselines {
		if err := b.Validate(); err != nil {
			return Comparison{}, fmt.Errorf("baseline %s: %w", b.Model, err)
		}
		if err := sameSetting(candidate, b); err != nil {
			return Comparison{}, fmt.Errorf("baseline %s: %w", b.Model, err)
		}
		q, ok := b.Quality[metric]
		if !ok {
			return Comparison{}, fmt.Errorf("baseline %s has no quality metric %q", b.Model, metric)
		}
		if slices.Contains(b.Higher, metric) != higher {
			return Comparison{}, fmt.Errorf("baseline %s disagrees on the direction of %q", b.Model, metric)
		}
		delta := candidate.Quality[metric] - q
		v := Verdict{
			Baseline: b.Framework + "/" + b.Model, QualityDelta: delta,
			HigherQuality:   (higher && delta > 0) || (!higher && delta < 0),
			ParametersRatio: float64(candidate.Parameters) / float64(b.Parameters),
			PeakRSSRatio:    float64(candidate.PeakRSSBytes) / float64(b.PeakRSSBytes),
			LatencyRatio:    candidate.Latency.P50MS / b.Latency.P50MS,
		}
		v.WithinParams = candidate.Parameters <= b.Parameters
		v.WithinMemory = candidate.PeakRSSBytes <= b.PeakRSSBytes
		v.Faster = candidate.Latency.P50MS <= b.Latency.P50MS
		v.Wins = v.HigherQuality && v.WithinParams && v.WithinMemory && v.Faster
		c.Baselines = append(c.Baselines, b)
		c.Versus = append(c.Versus, v)
	}
	return c, nil
}

func sameSetting(a, b Result) error {
	switch {
	case a.Task != b.Task:
		return fmt.Errorf("task %q differs from %q", b.Task, a.Task)
	case a.Split != b.Split:
		return fmt.Errorf("evaluation split %q differs from %q", b.Split, a.Split)
	case a.Host.Machine != b.Host.Machine || a.Host.CPU != b.Host.CPU:
		return fmt.Errorf("host %s (%s) differs from %s (%s)", b.Host.Machine, b.Host.CPU, a.Host.Machine, a.Host.CPU)
	case a.Device != b.Device:
		return fmt.Errorf("device %q differs from %q", b.Device, a.Device)
	case a.Threads != b.Threads:
		return fmt.Errorf("%d threads differ from %d", b.Threads, a.Threads)
	}
	return nil
}

// Markdown renders the comparison as a table a person can read.
func (c Comparison) Markdown() string {
	var s strings.Builder
	fmt.Fprintf(&s, "Task `%s`, split `%s`, machine %s (%s), device %s, %d thread(s).\n\n", c.Task, short(c.Split), c.Machine, c.Candidate.Host.CPU, c.Candidate.Device, c.Candidate.Threads)
	s.WriteString("| Model | " + c.Metric + " | Parameters | Peak RSS MiB | Model-added MiB | p50 ms | p90 ms |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	row := func(r Result) {
		fmt.Fprintf(&s, "| %s/%s | %.4f | %d | %.1f | %.1f | %.3f | %.3f |\n", r.Framework, r.Model, r.Quality[c.Metric], r.Parameters,
			mib(r.PeakRSSBytes), mib(r.PeakRSSBytes-r.IdleRSSBytes), r.Latency.P50MS, r.Latency.P90MS)
	}
	row(c.Candidate)
	for _, b := range c.Baselines {
		row(b)
	}
	s.WriteString("\n| Versus | Quality Δ | Params × | Peak RSS × | p50 × | Wins |\n| --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, v := range c.Versus {
		fmt.Fprintf(&s, "| %s | %+.4f | %.2f | %.2f | %.2f | %s |\n", v.Baseline, v.QualityDelta, v.ParametersRatio, v.PeakRSSRatio, v.LatencyRatio, yes(v.Wins))
	}
	return s.String()
}

func mib(b uint64) float64 { return float64(b) / (1 << 20) }

// short keeps the end of a long split name, where its hash sits, so two
// different selections never print the same.
func short(s string) string {
	if len(s) > 24 {
		return "…" + s[len(s)-16:]
	}
	return s
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
