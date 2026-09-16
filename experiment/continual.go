package experiment

import (
	"fmt"
	"math"
)

// ContinualSchemaVersion identifies the preregistered continual-matrix protocol.
const ContinualSchemaVersion = "coimnet-continual-matrix/v1"

// GeneratorDelayedCorrelation names the only task generator a protocol may use.
const GeneratorDelayedCorrelation = "delayed-correlation/v1"

// Stage kinds a continual protocol may declare.
const (
	StageTrainTask  = "train_task"
	StageRuleChange = "rule_change"
)

// MetricNegMSE expects the "bigger is better" evaluation score.
const MetricNegMSE = "neg_mse"

// ComparisonPairedBootstrap is the only preregistered comparison method.
const ComparisonPairedBootstrap = "paired_bootstrap"

// BaselineIndependent re-runs each task freshly trained in isolation.
const BaselineIndependent = "independent"

// TaskSpec names one task of a versioned generator. delayed-correlation/v1
// takes "delay" (integer 1..8), "channel" (integer 0..7) and an optional
// "gain" (nonzero, |gain| <= 1, default 0.4 when absent).
type TaskSpec struct {
	Name      string             `json:"name"`
	Generator string             `json:"generator"`
	Params    map[string]float64 `json:"params,omitempty"`
}

// Stage is one training or intervention window. Budget is in training
// episodes; a rule_change stage must have Budget 0.
type Stage struct {
	Kind   string `json:"kind"`
	Task   string `json:"task"`
	Budget uint64 `json:"budget"`
}

// StateSwitch is the alternative chemical state (region x channel) the same
// individual is re-evaluated under, as a "temporarily suppressed, not
// forgotten" control.
type StateSwitch struct {
	Concentration [][]float64 `json:"concentration"`
}

// Evaluation fixes how every post-stage matrix cell is measured.
type Evaluation struct {
	Episodes       int          `json:"episodes"`
	FixedChemistry bool         `json:"fixed_chemistry"`
	StateSwitch    *StateSwitch `json:"state_switch,omitempty"`
	Metric         string       `json:"metric"`
}

// PreRegistered declares the comparison method and its uncertainty interval
// before any matrix is produced.
type PreRegistered struct {
	Method    string  `json:"method"`
	Interval  float64 `json:"interval"`
	Baseline  string  `json:"baseline"`
	Resamples int     `json:"resamples"`
}

// ContinualProtocol is the complete preregistered continual-learning protocol.
type ContinualProtocol struct {
	Tasks      []TaskSpec     `json:"tasks"`
	Stages     []Stage        `json:"stages"`
	Seeds      []uint64       `json:"seeds"`
	Evaluation Evaluation     `json:"evaluation"`
	Comparison *PreRegistered `json:"comparison,omitempty"`
}

// Validate rejects a protocol whose matrix or attribution could be undefined.
func (p ContinualProtocol) Validate() error {
	if len(p.Tasks) == 0 {
		return fmt.Errorf("tasks: at least one task required")
	}
	seenTasks := make(map[string]int, len(p.Tasks))
	for i, ts := range p.Tasks {
		if ts.Name == "" {
			return fmt.Errorf("task %d: name must not be empty (duplicate names are rejected)", i)
		}
		if prev, ok := seenTasks[ts.Name]; ok {
			return fmt.Errorf("task %d: duplicate name %q (already task %d)", i, ts.Name, prev)
		}
		seenTasks[ts.Name] = i
		if ts.Generator != GeneratorDelayedCorrelation {
			return fmt.Errorf("task %q: unsupported generator %q", ts.Name, ts.Generator)
		}
		for key, v := range ts.Params {
			switch key {
			case "delay":
				if v != math.Trunc(v) || v < 1 || v > 8 {
					return fmt.Errorf("task %q: param delay must be an integer in 1..8, got %v", ts.Name, v)
				}
			case "channel":
				if v != math.Trunc(v) || v < 0 || v > 7 {
					return fmt.Errorf("task %q: param channel must be an integer in 0..7, got %v", ts.Name, v)
				}
			case "gain":
				if math.IsNaN(v) || math.IsInf(v, 0) || v == 0 || math.Abs(v) > 1 {
					return fmt.Errorf("task %q: param gain must be non-zero with |gain| <= 1, got %v", ts.Name, v)
				}
			default:
				return fmt.Errorf("task %q: unsupported param %q", ts.Name, key)
			}
		}
	}
	if len(p.Stages) == 0 {
		return fmt.Errorf("stages: at least one stage required")
	}
	for i, st := range p.Stages {
		if st.Kind != StageTrainTask && st.Kind != StageRuleChange {
			return fmt.Errorf("stage %d: unsupported kind %q", i, st.Kind)
		}
		if _, ok := seenTasks[st.Task]; !ok {
			return fmt.Errorf("stage %d: unknown task %q", i, st.Task)
		}
		if st.Kind == StageRuleChange && st.Budget != 0 {
			return fmt.Errorf("stage %d: rule_change budget must be 0, got %d", i, st.Budget)
		}
		if st.Kind == StageTrainTask && st.Budget > 1000000 {
			return fmt.Errorf("stage %d: train_task budget %d exceeds limit 1000000", i, st.Budget)
		}
	}
	if len(p.Seeds) < 3 {
		return fmt.Errorf("seeds: at least 3 required, got %d", len(p.Seeds))
	}
	seenSeeds := make(map[uint64]bool, len(p.Seeds))
	for _, seed := range p.Seeds {
		if seenSeeds[seed] {
			return fmt.Errorf("seeds: duplicate seed %d", seed)
		}
		seenSeeds[seed] = true
	}
	if p.Evaluation.Episodes < 1 || p.Evaluation.Episodes > 10000 {
		return fmt.Errorf("evaluation: episodes must be between 1 and 10000, got %d", p.Evaluation.Episodes)
	}
	if p.Evaluation.Metric != MetricNegMSE {
		return fmt.Errorf("evaluation: unsupported metric %q", p.Evaluation.Metric)
	}
	if sw := p.Evaluation.StateSwitch; sw != nil {
		if len(sw.Concentration) == 0 {
			return fmt.Errorf("state_switch: concentration must not be empty")
		}
		width := len(sw.Concentration[0])
		for i, row := range sw.Concentration {
			if len(row) != width {
				return fmt.Errorf("state_switch: row %d has length %d, want %d", i, len(row), width)
			}
			for _, v := range row {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return fmt.Errorf("state_switch: value %g is not finite", v)
				}
				if v < 0 {
					return fmt.Errorf("state_switch: value %g is negative", v)
				}
			}
		}
	}
	if c := p.Comparison; c != nil {
		if c.Method != ComparisonPairedBootstrap {
			return fmt.Errorf("comparison: unsupported method %q", c.Method)
		}
		if c.Interval <= 0 || c.Interval >= 1 {
			return fmt.Errorf("comparison: interval must be strictly between 0 and 1, got %g", c.Interval)
		}
		if c.Baseline != BaselineIndependent {
			return fmt.Errorf("comparison: unsupported baseline %q", c.Baseline)
		}
		if c.Resamples < 100 || c.Resamples > 100000 {
			return fmt.Errorf("comparison: resamples must be between 100 and 100000, got %d", c.Resamples)
		}
	}
	return nil
}

// TaskWidth is the input width every task of the protocol shares: 1 + the
// largest declared channel. Only valid after Validate.
func (p ContinualProtocol) TaskWidth() int {
	width := 1
	for _, ts := range p.Tasks {
		if c := int(ts.Params["channel"]); c+1 > width {
			width = c + 1
		}
	}
	return width
}

// TaskIndex returns the position of the named task or -1.
func (p ContinualProtocol) TaskIndex(name string) int {
	for i, ts := range p.Tasks {
		if ts.Name == name {
			return i
		}
	}
	return -1
}
