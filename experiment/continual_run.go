package experiment

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/learning"
)

// RunRecord is one stage or one evaluation of one seed. Stage is the stage
// index (−1 for the build of the individual); Task is set for evaluation
// records; Control names the control a record belongs to when it is not a
// matrix run; a failed record keeps the error text and is never dropped.
type RunRecord struct {
	Seed    uint64 `json:"seed"`
	Stage   int    `json:"stage"`
	Task    string `json:"task,omitempty"`
	Control string `json:"control,omitempty"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

// RunStatusOK and RunStatusFailed are the two statuses of a RunRecord.
const (
	RunStatusOK     = "ok"
	RunStatusFailed = "failed"
)

// SeedMatrix is R[i][j] of one seed: the score after stage i on task j, the
// failed mask and the forgetting F[i][j] of continual_math.go. Switched and
// SwitchedFailed carry the same cells re-scored under the protocol's switched
// chemical state, when one is declared.
type SeedMatrix struct {
	Seed           uint64      `json:"seed"`
	Scores         [][]float64 `json:"scores"`
	Failed         [][]bool    `json:"failed"`
	Forgetting     [][]float64 `json:"forgetting"`
	Switched       [][]float64 `json:"switched,omitempty"`
	SwitchedFailed [][]bool    `json:"switched_failed,omitempty"`
}

// ContinualReport is the complete output of one run of a continual matrix.
type ContinualReport struct {
	SchemaVersion   string              `json:"schema_version"`
	Profile         string              `json:"profile"`
	Protocol        ContinualProtocol   `json:"protocol"`
	ProtocolHash    string              `json:"protocol_hash"`
	Width           int                 `json:"width"`
	Seeds           []SeedMatrix        `json:"seeds"`
	Cells           [][]CellStat        `json:"cells"`
	ForgettingCells [][]CellStat        `json:"forgetting_cells"`
	Runs            []RunRecord         `json:"runs"`
	Assumptions     []string            `json:"assumptions"`
	Independent     []IndependentResult `json:"independent"`
}

// runSeedMatrix runs one seed: builds the individual, replays every stage and
// measures every evaluation cell, returning the seed's matrix and its records.
// A context error aborts the whole run; any other failure stays a RunRecord.
func runSeedMatrix(ctx context.Context, p ContinualProtocol, seed uint64, width int, build func(seed uint64) (*learning.Individual, error)) (SeedMatrix, []RunRecord, error) {
	sm := SeedMatrix{Seed: seed}
	sm.Scores = make([][]float64, len(p.Stages))
	sm.Failed = make([][]bool, len(p.Stages))
	for i := range sm.Scores {
		sm.Scores[i] = make([]float64, len(p.Tasks))
		sm.Failed[i] = make([]bool, len(p.Tasks))
	}
	if p.Evaluation.StateSwitch != nil {
		sm.Switched = make([][]float64, len(p.Stages))
		sm.SwitchedFailed = make([][]bool, len(p.Stages))
		for i := range sm.Switched {
			sm.Switched[i] = make([]float64, len(p.Tasks))
			sm.SwitchedFailed[i] = make([]bool, len(p.Tasks))
		}
	}
	var runs []RunRecord
	ind, err := build(seed)
	if err != nil {
		buildErr := err
		for i := range sm.Failed {
			for j := range sm.Failed[i] {
				sm.Failed[i][j] = true
			}
		}
		sm.Forgetting, err = Forgetting(sm.Scores, sm.Failed)
		if err != nil {
			return sm, runs, err
		}
		runs = append(runs, RunRecord{Seed: seed, Stage: -1, Status: RunStatusFailed, Error: buildErr.Error()})
		return sm, runs, nil
	}
	flipped := make([]bool, len(p.Tasks))
	for i, st := range p.Stages {
		if err := ctx.Err(); err != nil {
			return sm, runs, err
		}
		switch st.Kind {
		case StageTrainTask:
			j := p.TaskIndex(st.Task)
			var trainErr error
			for e := uint64(0); e < st.Budget; e++ {
				ep, err := ContinualEpisode(p.Tasks[j], width, flipped[j], trainSeed(seed, i), e)
				if err != nil {
					trainErr = err
					break
				}
				if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
					trainErr = err
					break
				}
			}
			if trainErr != nil {
				if err := ctx.Err(); err != nil {
					return sm, runs, err
				}
				runs = append(runs, RunRecord{Seed: seed, Stage: i, Task: st.Task, Status: RunStatusFailed, Error: trainErr.Error()})
			} else {
				runs = append(runs, RunRecord{Seed: seed, Stage: i, Task: st.Task, Status: RunStatusOK})
			}
		case StageRuleChange:
			j := p.TaskIndex(st.Task)
			flipped[j] = !flipped[j]
			runs = append(runs, RunRecord{Seed: seed, Stage: i, Task: st.Task, Status: RunStatusOK})
		}
		for j := range p.Tasks {
			if err := ctx.Err(); err != nil {
				return sm, runs, err
			}
			score, err := evaluateTask(ctx, ind, p.Tasks[j], width, flipped[j], p.Evaluation.Episodes, evalSeed(seed, j), p.Evaluation.FixedChemistry)
			if err != nil {
				if err := ctx.Err(); err != nil {
					return sm, runs, err
				}
				runs = append(runs, RunRecord{Seed: seed, Stage: i, Task: p.Tasks[j].Name, Status: RunStatusFailed, Error: err.Error()})
				sm.Scores[i][j] = 0
				sm.Failed[i][j] = true
			} else {
				runs = append(runs, RunRecord{Seed: seed, Stage: i, Task: p.Tasks[j].Name, Status: RunStatusOK})
				sm.Scores[i][j] = score
			}
			if sws := p.Evaluation.StateSwitch; sws != nil {
				swScore, swErr := evaluateSwitched(ctx, ind, p.Tasks[j], width, flipped[j], p.Evaluation.Episodes, evalSeed(seed, j), sws.Concentration)
				if swErr != nil {
					if cerr := ctx.Err(); cerr != nil {
						return sm, runs, cerr
					}
					runs = append(runs, RunRecord{Seed: seed, Stage: i, Task: p.Tasks[j].Name, Control: ControlStateSwitch, Status: RunStatusFailed, Error: swErr.Error()})
					sm.SwitchedFailed[i][j] = true
				} else {
					sm.Switched[i][j] = swScore
				}
			}
		}
	}
	sm.Forgetting, err = Forgetting(sm.Scores, sm.Failed)
	if err != nil {
		return sm, runs, err
	}
	return sm, runs, nil
}

// RunContinualMatrix runs the protocol on one individual per seed built by
// build (nil build, nil ctx or an invalid protocol are errors; a protocol
// with a Comparison is refused with "arrives with a later ticket" until that
// comparison lands). After every matrix cell a declared state switch re-scores
// the cell under the switched chemistry and, after every seed, the independent
// control is built and kept apart from the matrix. A context error aborts the
// whole run with that error; every other failure becomes a RunRecord.
func RunContinualMatrix(ctx context.Context, p ContinualProtocol, build func(seed uint64) (*learning.Individual, error)) (ContinualReport, error) {
	var report ContinualReport
	if ctx == nil {
		return report, fmt.Errorf("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if build == nil {
		return report, fmt.Errorf("build must not be nil")
	}
	if err := p.Validate(); err != nil {
		return report, err
	}
	if p.Comparison != nil {
		return report, fmt.Errorf("comparison arrives with a later ticket")
	}
	report = ContinualReport{
		SchemaVersion: ContinualSchemaVersion,
		Profile:       "fixture",
		Protocol:      p,
		ProtocolHash:  hash(p),
		Width:         p.TaskWidth(),
	}
	report.Assumptions = []string{
		"Evaluation episodes come from a seed derived from the run seed and the task index; training never sees them.",
		"Every evaluation runs on a twin restored from the individual's snapshot, so evaluating never advances or trains the individual.",
		"Failed stages and evaluations are kept as records; aggregates use the successful seeds only and report succeeded/total per cell.",
		"A rule_change stage negates the task's target gain; every later training and evaluation of that task uses the new rule.",
		"independent is the per-task re-initialized control, not a continual result; state_switch re-scores a cell under a switched chemistry, so a moved score with unchanged parameters and plastic state is expression, not forgetting.",
	}
	perSeedScores := make([][][]float64, 0, len(p.Seeds))
	perSeedFailed := make([][][]bool, 0, len(p.Seeds))
	for _, seed := range p.Seeds {
		sm, runs, err := runSeedMatrix(ctx, p, seed, report.Width, build)
		if err != nil {
			return report, err
		}
		report.Seeds = append(report.Seeds, sm)
		report.Runs = append(report.Runs, runs...)
		perSeedScores = append(perSeedScores, sm.Scores)
		perSeedFailed = append(perSeedFailed, sm.Failed)
	}
	var indErr error
	report.Independent, indErr = runIndependent(ctx, p, build, report.Width, &report.Runs)
	if indErr != nil {
		return report, indErr
	}
	cells, err := AggregateCells(perSeedScores, perSeedFailed)
	if err != nil {
		return report, err
	}
	report.Cells = cells
	perSeedForgetting := make([][][]float64, 0, len(report.Seeds))
	for _, sm := range report.Seeds {
		perSeedForgetting = append(perSeedForgetting, sm.Forgetting)
	}
	forgettingCells, err := AggregateCells(perSeedForgetting, perSeedFailed)
	if err != nil {
		return report, err
	}
	report.ForgettingCells = forgettingCells
	return report, nil
}
