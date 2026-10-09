package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// continualOwnershipProtocol is intentionally short, but leaves spare
// capacity in every slice whose append must be isolated by the runner.
func continualOwnershipProtocol(budget uint64) ContinualProtocol {
	p := ContinualProtocol{
		Tasks: []TaskSpec{
			{Name: "A", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 2, "channel": 0}},
			{Name: "B", Generator: GeneratorDelayedCorrelation, Params: map[string]float64{"delay": 3, "channel": 1}},
		},
		Stages: []Stage{
			{Kind: StageTrainTask, Task: "A", Budget: budget},
			{Kind: StageTrainTask, Task: "B", Budget: budget},
			{Kind: StageRuleChange, Task: "A", Budget: 0},
		},
		Seeds: []uint64{7, 42, 123},
		Evaluation: Evaluation{
			Episodes:    1,
			StateSwitch: &StateSwitch{Concentration: [][]float64{{0.5}}},
			Metric:      MetricNegMSE,
		},
		Comparison: &PreRegistered{
			Method:    ComparisonPairedBootstrap,
			Interval:  0.5,
			Baseline:  BaselineIndependent,
			Resamples: 100,
		},
	}
	p.Tasks = append(make([]TaskSpec, 0, len(p.Tasks)+1), p.Tasks...)
	p.Stages = append(make([]Stage, 0, len(p.Stages)+1), p.Stages...)
	p.Seeds = append(make([]uint64, 0, len(p.Seeds)+1), p.Seeds...)
	p.Evaluation.StateSwitch.Concentration = append(
		make([][]float64, 0, len(p.Evaluation.StateSwitch.Concentration)+1),
		p.Evaluation.StateSwitch.Concentration...,
	)
	for i, row := range p.Evaluation.StateSwitch.Concentration {
		p.Evaluation.StateSwitch.Concentration[i] = append(make([]float64, 0, len(row)+1), row...)
	}
	return p
}

func marshalContinualOwnership(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func runContinualOwnership(t *testing.T, p ContinualProtocol, build func(uint64) (*learning.Individual, error)) ContinualReport {
	t.Helper()
	report, err := RunContinualMatrix(context.Background(), p, build)
	if err != nil {
		t.Fatalf("RunContinualMatrix: %v", err)
	}
	return report
}

func assertContinualOwnershipUnchanged(t *testing.T, report ContinualReport, before []byte, beforeHash string) {
	t.Helper()
	after := marshalContinualOwnership(t, report)
	if !bytes.Equal(after, before) {
		t.Fatal("report JSON changed after mutating the caller")
	}
	if report.ProtocolHash != beforeHash {
		t.Fatalf("report protocol_hash changed from %q to %q", beforeHash, report.ProtocolHash)
	}
	if got := hash(report.Protocol); got != beforeHash {
		t.Fatalf("report protocol no longer matches its protocol_hash: got %q want %q", got, beforeHash)
	}
}

func TestContinualOwnershipCallerMutationDoesNotRewriteReport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContinualProtocol)
	}{
		{name: "tasks-element", mutate: func(p *ContinualProtocol) { p.Tasks[0].Name = "caller-mutated" }},
		{name: "task-params-value", mutate: func(p *ContinualProtocol) { p.Tasks[0].Params["delay"] = 4 }},
		{name: "task-params-map-insert", mutate: func(p *ContinualProtocol) { p.Tasks[0].Params["gain"] = 0.6 }},
		{name: "task-params-map-delete", mutate: func(p *ContinualProtocol) { delete(p.Tasks[0].Params, "channel") }},
		{name: "stages-element", mutate: func(p *ContinualProtocol) { p.Stages[0].Budget = 1 }},
		{name: "seeds-element", mutate: func(p *ContinualProtocol) { p.Seeds[0] = 999 }},
		{name: "comparison-field", mutate: func(p *ContinualProtocol) { p.Comparison.Interval = 0.25 }},
		{name: "state-switch-value", mutate: func(p *ContinualProtocol) { p.Evaluation.StateSwitch.Concentration[0][0] = 0.75 }},
		{name: "state-switch-outer", mutate: func(p *ContinualProtocol) {
			p.Evaluation.StateSwitch.Concentration = append(p.Evaluation.StateSwitch.Concentration, []float64{0.25})
		}},
		{name: "state-switch-inner", mutate: func(p *ContinualProtocol) {
			p.Evaluation.StateSwitch.Concentration[0] = append(p.Evaluation.StateSwitch.Concentration[0], 0.25)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := continualOwnershipProtocol(0)
			report := runContinualOwnership(t, p, ContinualFixture)
			before := marshalContinualOwnership(t, report)
			beforeHash := report.ProtocolHash

			tt.mutate(&p)
			assertContinualOwnershipUnchanged(t, report, before, beforeHash)
		})
	}
}

func TestContinualOwnershipReportMutationDoesNotRewriteCallerOrSecondReport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContinualReport)
	}{
		{name: "tasks-element", mutate: func(r *ContinualReport) { r.Protocol.Tasks[0].Name = "report-mutated" }},
		{name: "task-params-value", mutate: func(r *ContinualReport) { r.Protocol.Tasks[0].Params["delay"] = 4 }},
		{name: "task-params-map-insert-delete", mutate: func(r *ContinualReport) {
			r.Protocol.Tasks[0].Params["report-only"] = 1
			delete(r.Protocol.Tasks[0].Params, "delay")
		}},
		{name: "stages-element", mutate: func(r *ContinualReport) { r.Protocol.Stages[0].Budget = 1 }},
		{name: "seeds-element", mutate: func(r *ContinualReport) { r.Protocol.Seeds[0] = 999 }},
		{name: "comparison-field", mutate: func(r *ContinualReport) { r.Protocol.Comparison.Interval = 0.25 }},
		{name: "state-switch-value", mutate: func(r *ContinualReport) { r.Protocol.Evaluation.StateSwitch.Concentration[0][0] = 0.75 }},
		{name: "state-switch-outer", mutate: func(r *ContinualReport) {
			r.Protocol.Evaluation.StateSwitch.Concentration = append(r.Protocol.Evaluation.StateSwitch.Concentration, []float64{0.25})
		}},
		{name: "state-switch-inner", mutate: func(r *ContinualReport) {
			r.Protocol.Evaluation.StateSwitch.Concentration[0] = append(r.Protocol.Evaluation.StateSwitch.Concentration[0], 0.25)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := continualOwnershipProtocol(0)
			first := runContinualOwnership(t, p, ContinualFixture)
			second := runContinualOwnership(t, p, ContinualFixture)
			callerBefore := marshalContinualOwnership(t, p)
			secondBefore := marshalContinualOwnership(t, second)

			tt.mutate(&first)
			if callerAfter := marshalContinualOwnership(t, p); !bytes.Equal(callerAfter, callerBefore) {
				t.Fatal("caller changed after mutating report")
			}
			if secondAfter := marshalContinualOwnership(t, second); !bytes.Equal(secondAfter, secondBefore) {
				t.Fatal("second report changed after mutating first report")
			}
		})
	}
}

func TestContinualOwnershipReportAppendDoesNotUseCallerCapacity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContinualReport)
		check  func(*testing.T, ContinualProtocol)
	}{
		{
			name: "tasks",
			mutate: func(r *ContinualReport) {
				r.Protocol.Tasks = append(r.Protocol.Tasks, TaskSpec{Name: "report-only"})
			},
			check: func(t *testing.T, p ContinualProtocol) {
				t.Helper()
				if got := p.Tasks[:cap(p.Tasks)][len(p.Tasks)]; !reflect.DeepEqual(got, TaskSpec{}) {
					t.Fatalf("caller task spare slot = %+v, want zero", got)
				}
			},
		},
		{
			name: "stages",
			mutate: func(r *ContinualReport) {
				r.Protocol.Stages = append(r.Protocol.Stages, Stage{Kind: StageTrainTask, Task: "A", Budget: 9})
			},
			check: func(t *testing.T, p ContinualProtocol) {
				t.Helper()
				if got := p.Stages[:cap(p.Stages)][len(p.Stages)]; !reflect.DeepEqual(got, Stage{}) {
					t.Fatalf("caller stage spare slot = %+v, want zero", got)
				}
			},
		},
		{
			name:   "seeds",
			mutate: func(r *ContinualReport) { r.Protocol.Seeds = append(r.Protocol.Seeds, 999) },
			check: func(t *testing.T, p ContinualProtocol) {
				t.Helper()
				if got := p.Seeds[:cap(p.Seeds)][len(p.Seeds)]; got != 0 {
					t.Fatalf("caller seed spare slot = %d, want zero", got)
				}
			},
		},
		{
			name: "state-switch-outer",
			mutate: func(r *ContinualReport) {
				r.Protocol.Evaluation.StateSwitch.Concentration = append(r.Protocol.Evaluation.StateSwitch.Concentration, []float64{0.25})
			},
			check: func(t *testing.T, p ContinualProtocol) {
				t.Helper()
				rows := p.Evaluation.StateSwitch.Concentration
				if got := rows[:cap(rows)][len(rows)]; got != nil {
					t.Fatalf("caller state-switch spare row = %#v, want nil", got)
				}
			},
		},
		{
			name: "state-switch-inner",
			mutate: func(r *ContinualReport) {
				row := r.Protocol.Evaluation.StateSwitch.Concentration[0]
				r.Protocol.Evaluation.StateSwitch.Concentration[0] = append(row, 0.25)
			},
			check: func(t *testing.T, p ContinualProtocol) {
				t.Helper()
				row := p.Evaluation.StateSwitch.Concentration[0]
				if got := row[:cap(row)][len(row)]; got != 0 {
					t.Fatalf("caller state-switch spare value = %g, want zero", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := continualOwnershipProtocol(0)
			first := runContinualOwnership(t, p, ContinualFixture)
			second := runContinualOwnership(t, p, ContinualFixture)
			callerBefore := marshalContinualOwnership(t, p)
			secondBefore := marshalContinualOwnership(t, second)

			tt.mutate(&first)
			if callerAfter := marshalContinualOwnership(t, p); !bytes.Equal(callerAfter, callerBefore) {
				t.Fatal("caller JSON changed after report append")
			}
			if secondAfter := marshalContinualOwnership(t, second); !bytes.Equal(secondAfter, secondBefore) {
				t.Fatal("second report JSON changed after report append")
			}
			tt.check(t, p)
		})
	}
}

func TestContinualOwnershipBuilderUsesSnapshotBeforeCallback(t *testing.T) {
	controlProtocol := continualOwnershipProtocol(1)
	control := runContinualOwnership(t, controlProtocol, ContinualFixture)
	controlJSON := marshalContinualOwnership(t, control)

	mutatedProtocol := continualOwnershipProtocol(1)
	calls := 0
	build := func(seed uint64) (*learning.Individual, error) {
		calls++
		if calls == 1 {
			mutatedProtocol.Tasks[0].Params["delay"] = 4
			mutatedProtocol.Tasks[0].Params["gain"] = 0.6
			mutatedProtocol.Stages[0].Budget = 0
			mutatedProtocol.Seeds[0] = 999
			mutatedProtocol.Comparison.Interval = 0.25
			mutatedProtocol.Evaluation.StateSwitch.Concentration[0][0] = 0.25
		}
		return ContinualFixture(seed)
	}
	got, err := RunContinualMatrix(context.Background(), mutatedProtocol, build)
	if err != nil {
		t.Fatalf("RunContinualMatrix with builder mutation: %v", err)
	}
	if want := len(controlProtocol.Seeds) * (1 + len(controlProtocol.Tasks)); calls != want {
		t.Fatalf("builder calls = %d, want %d", calls, want)
	}
	if gotJSON := marshalContinualOwnership(t, got); !bytes.Equal(gotJSON, controlJSON) {
		t.Fatal("builder mutation changed the report")
	}
	if got.ProtocolHash != control.ProtocolHash {
		t.Fatalf("builder mutation changed protocol hash: got %q want %q", got.ProtocolHash, control.ProtocolHash)
	}
}

func TestContinualOwnershipNilOptionalFieldsStayNil(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContinualProtocol)
	}{
		{name: "nil-both", mutate: func(p *ContinualProtocol) {
			p.Comparison = nil
			p.Evaluation.StateSwitch = nil
		}},
		{name: "nil-comparison", mutate: func(p *ContinualProtocol) { p.Comparison = nil }},
		{name: "nil-state-switch", mutate: func(p *ContinualProtocol) { p.Evaluation.StateSwitch = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := continualOwnershipProtocol(0)
			tt.mutate(&p)
			report := runContinualOwnership(t, p, ContinualFixture)
			if report.Protocol.Comparison == nil != (p.Comparison == nil) {
				t.Fatalf("report protocol comparison nil = %v, caller nil = %v", report.Protocol.Comparison == nil, p.Comparison == nil)
			}
			if report.Comparison == nil != (p.Comparison == nil) {
				t.Fatalf("report comparison result nil = %v, caller nil = %v", report.Comparison == nil, p.Comparison == nil)
			}
			if report.Protocol.Evaluation.StateSwitch == nil != (p.Evaluation.StateSwitch == nil) {
				t.Fatalf("report protocol state_switch nil = %v, caller nil = %v", report.Protocol.Evaluation.StateSwitch == nil, p.Evaluation.StateSwitch == nil)
			}
			for _, sm := range report.Seeds {
				if p.Evaluation.StateSwitch == nil && sm.Switched != nil {
					t.Fatalf("seed %d has switched scores with a nil state_switch", sm.Seed)
				}
			}
			_ = marshalContinualOwnership(t, report)
		})
	}
}

func TestContinualOwnershipPreservesNilAndEmptyStateSwitchRows(t *testing.T) {
	p := continualOwnershipProtocol(0)
	p.Comparison = nil
	p.Evaluation.StateSwitch = &StateSwitch{Concentration: [][]float64{nil, []float64{}}}
	report := runContinualOwnership(t, p, ContinualFixtureWithChemistry)

	if got := report.ProtocolHash; got != hash(p) {
		t.Fatalf("protocol hash = %q, want hash of the declared nil/empty rows %q", got, hash(p))
	}
	rows := report.Protocol.Evaluation.StateSwitch.Concentration
	if rows[0] != nil {
		t.Fatalf("first concentration row = %#v, want nil", rows[0])
	}
	if rows[1] == nil || len(rows[1]) != 0 {
		t.Fatalf("second concentration row = %#v, want non-nil empty", rows[1])
	}
	reportJSON := marshalContinualOwnership(t, report)
	if !bytes.Contains(reportJSON, []byte(`"concentration":[null,[]]`)) {
		t.Fatal("report JSON lost the nil/empty concentration distinction")
	}

	wantFailures := len(p.Seeds) * len(p.Stages) * len(p.Tasks)
	gotFailures := 0
	for _, sm := range report.Seeds {
		for i := range sm.SwitchedFailed {
			for j, failed := range sm.SwitchedFailed[i] {
				if !failed {
					t.Errorf("seed %d switched cell (%d,%d) reported success for a shape mismatch", sm.Seed, i, j)
				}
			}
		}
	}
	for _, run := range report.Runs {
		if run.Control != ControlStateSwitch {
			continue
		}
		gotFailures++
		if run.Status != RunStatusFailed {
			t.Errorf("state-switch run seed %d stage %d task %q status = %q, want failed", run.Seed, run.Stage, run.Task, run.Status)
		}
	}
	if gotFailures != wantFailures {
		t.Fatalf("state-switch failure records = %d, want %d", gotFailures, wantFailures)
	}
}

func TestContinualOwnershipBuilderFailureKeepsErrorAndReportIsolation(t *testing.T) {
	p := continualOwnershipProtocol(1)
	p.Comparison = nil
	p.Evaluation.StateSwitch = nil
	const wantError = "ownership builder failed for seed 42"
	build := func(seed uint64) (*learning.Individual, error) {
		if seed == 42 {
			return nil, errors.New(wantError)
		}
		return ContinualFixture(seed)
	}
	report := runContinualOwnership(t, p, build)
	before := marshalContinualOwnership(t, report)
	beforeHash := report.ProtocolHash

	failed := 0
	for _, run := range report.Runs {
		if run.Seed != 42 || run.Status != RunStatusFailed {
			continue
		}
		failed++
		if run.Error != wantError {
			t.Errorf("seed 42 failure error = %q, want %q", run.Error, wantError)
		}
	}
	if failed == 0 {
		t.Fatal("builder failure was not retained in the partial report")
	}
	if len(report.Seeds) != len(p.Seeds) {
		t.Fatalf("partial report seeds = %d, want %d", len(report.Seeds), len(p.Seeds))
	}

	p.Tasks[0].Params["delay"] = 4
	p.Stages[0].Budget = 0
	p.Seeds[0] = 999
	assertContinualOwnershipUnchanged(t, report, before, beforeHash)
}

func TestContinualOwnershipCancellationKeepsErrorAndPartialReportIsolation(t *testing.T) {
	p := continualOwnershipProtocol(1)
	p.Comparison = nil
	p.Evaluation.StateSwitch = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	build := func(seed uint64) (*learning.Individual, error) {
		calls++
		if calls == len(p.Seeds)+1 {
			cancel()
		}
		return ContinualFixture(seed)
	}
	report, err := RunContinualMatrix(ctx, p, build)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContinualMatrix error = %v, want context.Canceled", err)
	}
	if calls != len(p.Seeds)+1 {
		t.Fatalf("builder calls before cancellation = %d, want %d", calls, len(p.Seeds)+1)
	}
	if len(report.Seeds) != len(p.Seeds) {
		t.Fatalf("partial report seeds = %d, want %d", len(report.Seeds), len(p.Seeds))
	}
	before := marshalContinualOwnership(t, report)
	beforeHash := report.ProtocolHash

	p.Tasks[0].Params["delay"] = 4
	p.Stages[0].Budget = 0
	p.Seeds[0] = 999
	assertContinualOwnershipUnchanged(t, report, before, beforeHash)
}
