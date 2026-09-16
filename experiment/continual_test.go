package experiment_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func continualProtocol() experiment.ContinualProtocol {
	return experiment.ContinualProtocol{
		Tasks: []experiment.TaskSpec{
			{Name: "A", Generator: experiment.GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}},
			{Name: "B", Generator: experiment.GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 3, "channel": 1}},
		},
		Stages: []experiment.Stage{
			{Kind: experiment.StageTrainTask, Task: "A", Budget: 100},
			{Kind: experiment.StageTrainTask, Task: "B", Budget: 100},
			{Kind: experiment.StageRuleChange, Task: "A", Budget: 0},
		},
		Seeds: []uint64{7, 42, 123},
		Evaluation: experiment.Evaluation{
			Episodes:       16,
			FixedChemistry: true,
			Metric:         experiment.MetricNegMSE,
		},
	}
}

func TestContinualProtocolValidateAcceptsTheFixture(t *testing.T) {
	p := continualProtocol()
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid protocol, got: %v", err)
	}
	if p.TaskWidth() != 2 {
		t.Fatalf("TaskWidth() = %d, want 2", p.TaskWidth())
	}
	if p.TaskIndex("B") != 1 {
		t.Fatalf("TaskIndex(\"B\") = %d, want 1", p.TaskIndex("B"))
	}
	if p.TaskIndex("missing") != -1 {
		t.Fatalf("TaskIndex(\"missing\") = %d, want -1", p.TaskIndex("missing"))
	}
}

func TestContinualProtocolValidateRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*experiment.ContinualProtocol)
		want   []string
	}{
		{name: "no tasks", mutate: func(p *experiment.ContinualProtocol) { p.Tasks = nil }, want: []string{"task"}},
		{name: "empty task name", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Name = "" }, want: []string{"duplicate"}},
		{name: "duplicate task name", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[1].Name = "A" }, want: []string{"duplicate"}},
		{name: "unknown generator", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Generator = "memorize/v1" }, want: []string{"generator"}},
		{name: "unknown param key", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["noise"] = 1 }, want: []string{"param"}},
		{name: "delay too large", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["delay"] = 9 }, want: []string{"param"}},
		{name: "delay too small", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["delay"] = 0 }, want: []string{"param"}},
		{name: "delay not integer", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["delay"] = 2.5 }, want: []string{"param"}},
		{name: "channel too large", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["channel"] = 8 }, want: []string{"param"}},
		{name: "channel not integer", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["channel"] = 0.5 }, want: []string{"param"}},
		{name: "zero gain", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["gain"] = 0 }, want: []string{"param"}},
		{name: "gain above one", mutate: func(p *experiment.ContinualProtocol) { p.Tasks[0].Params["gain"] = 2 }, want: []string{"param"}},
		{name: "no stages", mutate: func(p *experiment.ContinualProtocol) { p.Stages = nil }, want: []string{"stage"}},
		{name: "unknown stage kind", mutate: func(p *experiment.ContinualProtocol) { p.Stages[0].Kind = "replay" }, want: []string{"kind"}},
		{name: "unknown task in stage", mutate: func(p *experiment.ContinualProtocol) { p.Stages[1].Task = "X" }, want: []string{"unknown task"}},
		{name: "rule_change with budget", mutate: func(p *experiment.ContinualProtocol) { p.Stages[2].Budget = 5 }, want: []string{"budget"}},
		{name: "train_task budget too high", mutate: func(p *experiment.ContinualProtocol) { p.Stages[0].Budget = 1000001 }, want: []string{"budget"}},
		{name: "too few seeds", mutate: func(p *experiment.ContinualProtocol) { p.Seeds = []uint64{7, 42} }, want: []string{"seed"}},
		{name: "duplicate seed", mutate: func(p *experiment.ContinualProtocol) { p.Seeds[0] = 42 }, want: []string{"seed"}},
		{name: "zero episodes", mutate: func(p *experiment.ContinualProtocol) { p.Evaluation.Episodes = 0 }, want: []string{"episodes"}},
		{name: "episodes too high", mutate: func(p *experiment.ContinualProtocol) { p.Evaluation.Episodes = 10001 }, want: []string{"episodes"}},
		{name: "unknown metric", mutate: func(p *experiment.ContinualProtocol) { p.Evaluation.Metric = "accuracy" }, want: []string{"metric"}},
		{name: "state_switch empty concentration", mutate: func(p *experiment.ContinualProtocol) { p.Evaluation.StateSwitch = &experiment.StateSwitch{} }, want: []string{"state_switch"}},
		{name: "state_switch ragged rows", mutate: func(p *experiment.ContinualProtocol) {
			p.Evaluation.StateSwitch = &experiment.StateSwitch{Concentration: [][]float64{{1}, {2, 3}}}
		}, want: []string{"state_switch"}},
		{name: "state_switch non-finite value", mutate: func(p *experiment.ContinualProtocol) {
			p.Evaluation.StateSwitch = &experiment.StateSwitch{Concentration: [][]float64{{1, math.Inf(1)}}}
		}, want: []string{"state_switch"}},
		{name: "state_switch negative value", mutate: func(p *experiment.ContinualProtocol) {
			p.Evaluation.StateSwitch = &experiment.StateSwitch{Concentration: [][]float64{{1, -0.5}}}
		}, want: []string{"state_switch"}},
		{name: "comparison unknown method", mutate: func(p *experiment.ContinualProtocol) {
			p.Comparison = &experiment.PreRegistered{Method: "ttest", Interval: .05, Baseline: experiment.BaselineIndependent, Resamples: 1000}
		}, want: []string{"comparison"}},
		{name: "comparison interval at zero", mutate: func(p *experiment.ContinualProtocol) {
			p.Comparison = &experiment.PreRegistered{Method: experiment.ComparisonPairedBootstrap, Interval: 0, Baseline: experiment.BaselineIndependent, Resamples: 1000}
		}, want: []string{"comparison"}},
		{name: "comparison interval at one", mutate: func(p *experiment.ContinualProtocol) {
			p.Comparison = &experiment.PreRegistered{Method: experiment.ComparisonPairedBootstrap, Interval: 1, Baseline: experiment.BaselineIndependent, Resamples: 1000}
		}, want: []string{"comparison"}},
		{name: "comparison unknown baseline", mutate: func(p *experiment.ContinualProtocol) {
			p.Comparison = &experiment.PreRegistered{Method: experiment.ComparisonPairedBootstrap, Interval: .05, Baseline: "sequential", Resamples: 1000}
		}, want: []string{"comparison"}},
		{name: "comparison too few resamples", mutate: func(p *experiment.ContinualProtocol) {
			p.Comparison = &experiment.PreRegistered{Method: experiment.ComparisonPairedBootstrap, Interval: .05, Baseline: experiment.BaselineIndependent, Resamples: 50}
		}, want: []string{"comparison"}},
		{name: "comparison too many resamples", mutate: func(p *experiment.ContinualProtocol) {
			p.Comparison = &experiment.PreRegistered{Method: experiment.ComparisonPairedBootstrap, Interval: .05, Baseline: experiment.BaselineIndependent, Resamples: 100001}
		}, want: []string{"comparison"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := continualProtocol()
			tt.mutate(&p)
			err := p.Validate()
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", strings.Join(tt.want, ", "))
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
