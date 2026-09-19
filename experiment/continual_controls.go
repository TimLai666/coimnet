package experiment

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

// ControlStateSwitch is the Control label of a RunRecord that a failed state
// switch leaves behind; the independent baseline reuses BaselineIndependent.
const ControlStateSwitch = "state_switch"

// ContinualFixtureWithChemistry is ContinualFixture plus a chemistry layer:
// one region holding all three nodes, one channel, an external timeline that
// releases 1 on row 1 of every advance, one hypothesized receptor on node 2
// (Kd 0.5, N 1) and a gain effect on it, copied from the chemistry tests.
func ContinualFixtureWithChemistry(seed uint64) (*learning.Individual, error) {
	ind, err := ContinualFixture(seed)
	if err != nil {
		return nil, err
	}
	declaration := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{2}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0, 0}},
	}
	if err := ind.EnableChemistry(declaration); err != nil {
		return nil, err
	}
	if err := ind.SetExpressionGain(learning.ExpressionGain{Nodes: []int{2}, Receptor: 0, Scale: -0.5, Min: 0.5, Max: 1}); err != nil {
		return nil, err
	}
	return ind, nil
}

// SeedScore is one seed of one independent task: the score of a fresh
// individual trained on just that task and measured with the task's own
// evaluation stream, or the failure that replaced it.
type SeedScore struct {
	Seed   uint64  `json:"seed"`
	Score  float64 `json:"score"`
	Failed bool    `json:"failed"`
	Error  string  `json:"error,omitempty"`
}

// IndependentResult is the control the report keeps apart from the matrix:
// for one task, a fresh individual per seed trained only on that task's
// stages (same trainSeed, same budgets, rule changes applied in order) and
// scored with the task's evalSeed. It is a control, never a continual result.
type IndependentResult struct {
	Task  string      `json:"task"`
	Seeds []SeedScore `json:"seeds"`
	Stat  CellStat    `json:"stat"`
}

// runTaskIndependent is the whole run of one independent seed on one task:
// a fresh individual, every stage of the protocol that touches the task in
// protocol order (the same trainSeed a matrix stage uses, the same budget,
// rule changes in order) and the task's evaluation with the same evalSeed.
func runTaskIndependent(ctx context.Context, p ContinualProtocol, build func(uint64) (*learning.Individual, error), task int, seed uint64, width int) (float64, error) {
	ind, err := build(seed)
	if err != nil {
		return 0, err
	}
	flipped := false
	for i, st := range p.Stages {
		if p.TaskIndex(st.Task) != task {
			continue
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		switch st.Kind {
		case StageTrainTask:
			for e := uint64(0); e < st.Budget; e++ {
				ep, err := ContinualEpisode(p.Tasks[task], width, flipped, trainSeed(seed, i), e)
				if err != nil {
					return 0, err
				}
				if _, err := ind.TrainEpisode(ctx, ep.Input, ep.Target); err != nil {
					return 0, err
				}
			}
		case StageRuleChange:
			flipped = !flipped
		}
	}
	return evaluateTask(ctx, ind, p.Tasks[task], width, flipped, p.Evaluation.Episodes, evalSeed(seed, task), p.Evaluation.FixedChemistry)
}

// runIndependent builds the separate control of every task: each seed starts a
// fresh individual and trains only the stages of that task, so the result is
// "task trained alone" and never a continual number. Every failed seed keeps a
// RunRecord with Control "independent" and stage −1 and does not stop the rest;
// a context cancellation aborts the whole run instead.
func runIndependent(ctx context.Context, p ContinualProtocol, build func(uint64) (*learning.Individual, error), width int, records *[]RunRecord) ([]IndependentResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("independent run needs a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if build == nil {
		return nil, fmt.Errorf("independent run needs a build")
	}
	if records == nil {
		return nil, fmt.Errorf("independent run needs a record list")
	}
	results := make([]IndependentResult, len(p.Tasks))
	perSeed := make([][][]float64, len(p.Seeds))
	perFailed := make([][][]bool, len(p.Seeds))
	for s := range perSeed {
		perSeed[s] = make([][]float64, len(p.Tasks))
		perFailed[s] = make([][]bool, len(p.Tasks))
		for j := range p.Tasks {
			perSeed[s][j] = make([]float64, 1)
			perFailed[s][j] = make([]bool, 1)
		}
	}
	for j, task := range p.Tasks {
		results[j].Task = task.Name
		for s, seed := range p.Seeds {
			score, err := runTaskIndependent(ctx, p, build, j, seed, width)
			if err != nil {
				if cerr := ctx.Err(); cerr != nil {
					return nil, cerr
				}
				*records = append(*records, RunRecord{Seed: seed, Stage: -1, Task: task.Name, Control: BaselineIndependent, Status: RunStatusFailed, Error: err.Error()})
				results[j].Seeds = append(results[j].Seeds, SeedScore{Seed: seed, Score: 0, Failed: true, Error: err.Error()})
				perFailed[s][j][0] = true
				continue
			}
			results[j].Seeds = append(results[j].Seeds, SeedScore{Seed: seed, Score: score})
			perSeed[s][j][0] = score
		}
	}
	cells, err := AggregateCells(perSeed, perFailed)
	if err != nil {
		return nil, err
	}
	for j := range results {
		results[j].Stat = cells[j][0]
	}
	return results, nil
}

// evaluateSwitched is evaluateTask on a twin restored from the snapshot with
// its chemical concentration replaced by conc (shape must equal the state's,
// error "state_switch" otherwise; an individual without chemistry is the error
// "state_switch needs an enabled chemistry") and frozen, then scoreTwin with
// the modified snapshot as reference, so base parameters, plastic and slow
// states are checked unchanged: a score that moves under a switched state
// while those stay is expression, not forgetting.
func evaluateSwitched(ctx context.Context, ind *learning.Individual, spec TaskSpec, width int, flipped bool, episodes int, seed uint64, conc [][]float64) (float64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("state_switch evaluation needs a context")
	}
	if ind == nil {
		return 0, fmt.Errorf("state_switch evaluation needs an individual")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	reference := ind.Snapshot()
	if reference.Chemical == nil {
		return 0, fmt.Errorf("state_switch needs an enabled chemistry")
	}
	state := reference.Chemical.State.Concentration
	if len(conc) != len(state) {
		return 0, fmt.Errorf("state_switch: the switched concentration has %d regions, the state has %d", len(conc), len(state))
	}
	for r := range conc {
		if len(conc[r]) != len(state[r]) {
			return 0, fmt.Errorf("state_switch: the switched concentration row %d has %d channels, the state has %d", r, len(conc[r]), len(state[r]))
		}
	}
	replacement := make([][]float64, len(conc))
	for r, row := range conc {
		replacement[r] = append([]float64(nil), row...)
	}
	reference.Chemical.State.Concentration = replacement
	twin, err := learning.RestoreIndividual(reference)
	if err != nil {
		return 0, err
	}
	if err := twin.FreezeChemistry(true); err != nil {
		return 0, err
	}
	return scoreTwin(ctx, twin, reference, spec, width, flipped, episodes, seed)
}
