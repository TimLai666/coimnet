package experiment

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
)

// MultiTaskSchemaVersion identifies reports from one shared-core multi-task run.
const MultiTaskSchemaVersion = "coimnet-multitask/v1"

// MultiTaskConfig is one complete multi-task run on one shared core.
type MultiTaskConfig struct {
	Schedule     MultiTaskSchedule `json:"schedule"`
	Seed         uint64            `json:"seed"`
	Hidden       int               `json:"hidden"`
	LearningRate float64           `json:"learning_rate"`
	EvalEpisodes int               `json:"eval_episodes"`
	Corridor     gridnav.Config    `json:"corridor"`
}

// Validate checks the schedule and every configuration range, naming the
// invalid field in the returned error.
func (c MultiTaskConfig) Validate() error {
	if err := c.Schedule.Validate(); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	if c.Hidden < 1 || c.Hidden > 512 {
		return fmt.Errorf("hidden must be in [1, 512], got %d", c.Hidden)
	}
	if !finite(c.LearningRate) || c.LearningRate <= 0 {
		return fmt.Errorf("learning_rate must be finite and positive, got %v", c.LearningRate)
	}
	if c.EvalEpisodes < 1 || c.EvalEpisodes > 1000 {
		return fmt.Errorf("eval_episodes must be in [1, 1000], got %d", c.EvalEpisodes)
	}
	if _, err := gridnav.New(c.Corridor); err != nil {
		return fmt.Errorf("corridor: %w", err)
	}
	return nil
}

// DefaultMultiTaskConfig returns the TSK-09 missing-modality evidence protocol.
func DefaultMultiTaskConfig() MultiTaskConfig {
	return MultiTaskConfig{
		Schedule: MultiTaskSchedule{
			Tasks: []TaskBinding{
				{Name: MultiTaskCorridor, LossScale: 1, MixWeight: 1, SamplingRatio: 1, UpdateEvery: 1},
				{Name: MultiTaskDelayed, LossScale: 1, MixWeight: 1, SamplingRatio: 1, UpdateEvery: 1},
			},
			Schedule:     MultiTaskMissingModality,
			Budget:       400,
			Tolerance:    0.1,
			MissingEvery: 4,
		},
		Seed:         2,
		Hidden:       16,
		LearningRate: 0.05,
		EvalEpisodes: 20,
		Corridor:     gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1},
	}
}

// MultiTaskResult is one task's evaluation on its own individual of the shared brain.
type MultiTaskResult struct {
	Task              string   `json:"task"`
	Metric            string   `json:"metric"`
	Before            float64  `json:"before"`
	After             float64  `json:"after"`
	BrainTopologyHash string   `json:"brain_topology_hash"`
	BaseParameterHash string   `json:"base_parameter_hash"`
	AdapterVersion    string   `json:"adapter_version"`
	IndividualID      string   `json:"individual_id"`
	StateLineage      []string `json:"state_lineage"`
}

// MultiTaskReport is the report for a complete run on one shared core.
type MultiTaskReport struct {
	SchemaVersion string            `json:"schema_version"`
	Config        MultiTaskConfig   `json:"config"`
	ConfigHash    string            `json:"config_hash"`
	Steps         uint64            `json:"steps"`
	IdleUpdates   uint64            `json:"idle_updates"`
	Shares        []TaskShare       `json:"shares"`
	Results       []MultiTaskResult `json:"results"`
	TrainersBuilt int               `json:"trainers_built"`
	SameBrain     bool              `json:"same_brain"`
	Assumptions   []string          `json:"assumptions"`
}

// corePackage wraps the trainer's current configuration and parameters in one
// model package with declared model-step units and no evidence entries.
func (m *multiTaskModel) corePackage() (checkpoint.ModelPackage, error) {
	if m == nil || m.trainer == nil {
		return checkpoint.ModelPackage{}, fmt.Errorf("multitask model and trainer must be non-nil")
	}
	snapshot := m.trainer.Snapshot()
	return checkpoint.NewModelPackage(snapshot.Config, snapshot.Parameters,
		checkpoint.Units{TimeStep: "model_step", TimeConstant: "model_step"}, nil)
}

// evaluateOnIndividual builds an independent individual from pkg, scores one
// task without changing its parameters, and returns package/initial/final state hashes.
func evaluateOnIndividual(ctx context.Context, pkg checkpoint.ModelPackage, task string, corridor gridnav.Config, evalEpisodes int) (score float64, lineage []string, err error) {
	if ctx == nil {
		return 0, nil, fmt.Errorf("evaluation context is nil")
	}
	if evalEpisodes < 1 || evalEpisodes > 1000 {
		return 0, nil, fmt.Errorf("eval_episodes must be in [1, 1000], got %d", evalEpisodes)
	}
	if task != MultiTaskCorridor && task != MultiTaskDelayed {
		return 0, nil, fmt.Errorf("unknown task %q", task)
	}
	individual, err := checkpoint.NewIndividualFromPackage(pkg, learning.DefaultOptions(), make([]float64, pkg.Config.Dynamics.Nodes))
	if err != nil {
		return 0, nil, fmt.Errorf("build %s individual: %w", task, err)
	}
	lineage = []string{hash(pkg), hash(individual.Snapshot())}
	switch task {
	case MultiTaskCorridor:
		score, err = scoreCorridorIndividual(ctx, individual, corridor, evalEpisodes)
	case MultiTaskDelayed:
		score, err = scoreDelayedIndividual(ctx, individual, evalEpisodes)
	}
	if err != nil {
		return 0, nil, fmt.Errorf("evaluate %s individual: %w", task, err)
	}
	lineage = append(lineage, hash(individual.Snapshot()))
	return score, lineage, nil
}

// scoreCorridorIndividual measures expert agreement while one individual acts
// through each corridor episode.
func scoreCorridorIndividual(ctx context.Context, individual *learning.Individual, corridor gridnav.Config, evalEpisodes int) (float64, error) {
	env, err := gridnav.New(corridor)
	if err != nil {
		return 0, err
	}
	initial := make([]float64, individual.Snapshot().Config.Dynamics.Nodes)
	var matched, steps int
	for i := 0; i < evalEpisodes; i++ {
		if err := individual.ResetNeural(ctx, initial); err != nil {
			return 0, err
		}
		obs, _ := env.Reset(imitationEvalSeed(i))
		for {
			outputs, err := individual.Advance(ctx, multiTaskCorridorRows([][]float64{obs.Vector()}))
			if err != nil {
				return 0, err
			}
			action := nav2dArgmax(outputs[len(outputs)-1][:gridnav.Actions])
			if action == env.Expert() {
				matched++
			}
			steps++
			next, _, done, _, err := env.Step(action)
			if err != nil {
				return 0, err
			}
			if done {
				break
			}
			obs = next
		}
	}
	return float64(matched) / float64(steps), nil
}

// scoreDelayedIndividual returns negative mean squared error on the delayed
// task's fixed evaluation episodes.
func scoreDelayedIndividual(ctx context.Context, individual *learning.Individual, evalEpisodes int) (float64, error) {
	initial := make([]float64, individual.Snapshot().Config.Dynamics.Nodes)
	var mse float64
	for i := 0; i < evalEpisodes; i++ {
		if err := individual.ResetNeural(ctx, initial); err != nil {
			return 0, err
		}
		ep := DelayedEpisode(multiTaskDelayedEvalSeed, uint64(i))
		input, err := multiTaskDelayedRows(ep)
		if err != nil {
			return 0, err
		}
		outputs, err := individual.Advance(ctx, input)
		if err != nil {
			return 0, err
		}
		difference := outputs[len(outputs)-1][multiTaskDelayedOutput] - ep.Target[0]
		mse += difference * difference / float64(evalEpisodes)
	}
	return -mse, nil
}

// RunMultiTask validates c, trains one shared core, and scores each task on
// fresh individuals seeded from the untrained and trained model packages.
func RunMultiTask(ctx context.Context, c MultiTaskConfig) (MultiTaskReport, error) {
	if err := c.Validate(); err != nil {
		return MultiTaskReport{}, err
	}
	if ctx == nil {
		return MultiTaskReport{}, fmt.Errorf("multitask context is nil")
	}
	m, err := newMultiTaskModel(c.Seed, c.Hidden, c.LearningRate)
	if err != nil {
		return MultiTaskReport{}, err
	}
	beforePackage, err := m.corePackage()
	if err != nil {
		return MultiTaskReport{}, fmt.Errorf("build untrained model package: %w", err)
	}
	before := make([]float64, 2)
	for i, task := range []string{MultiTaskCorridor, MultiTaskDelayed} {
		before[i], _, err = evaluateOnIndividual(ctx, beforePackage, task, c.Corridor, c.EvalEpisodes)
		if err != nil {
			return MultiTaskReport{}, err
		}
	}
	shares, steps, idle, err := runMultiTaskSchedule(ctx, m, c.Schedule, c.Corridor, c.Seed)
	if err != nil {
		return MultiTaskReport{}, err
	}
	trainedPackage, err := m.corePackage()
	if err != nil {
		return MultiTaskReport{}, fmt.Errorf("build trained model package: %w", err)
	}
	results := make([]MultiTaskResult, 0, 2)
	for i, task := range []string{MultiTaskCorridor, MultiTaskDelayed} {
		after, lineage, err := evaluateOnIndividual(ctx, trainedPackage, task, c.Corridor, c.EvalEpisodes)
		if err != nil {
			return MultiTaskReport{}, err
		}
		metric := "expert_agreement"
		if task == MultiTaskDelayed {
			metric = "negative_mse"
		}
		results = append(results, MultiTaskResult{
			Task: task, Metric: metric, Before: before[i], After: after,
			BrainTopologyHash: trainedPackage.Topology.SHA256,
			BaseParameterHash: hash(trainedPackage.Parameters.Core),
			AdapterVersion:    m.adapterVersion(task),
			IndividualID:      task + "-eval",
			StateLineage:      lineage,
		})
	}
	sameBrain := len(results) == 2 && results[0].BrainTopologyHash == results[1].BrainTopologyHash && results[0].BaseParameterHash == results[1].BaseParameterHash
	return MultiTaskReport{
		SchemaVersion: MultiTaskSchemaVersion,
		Config:        c,
		ConfigHash:    hash(c),
		Steps:         steps,
		IdleUpdates:   idle,
		Shares:        shares,
		Results:       results,
		TrainersBuilt: m.built,
		SameBrain:     sameBrain,
		Assumptions: []string{
			"Both tasks train one core; a task's adapter is its encoder rows and readout columns, and one AdamW state serves both, so momentum can move an idle task's adapter.",
			"Each task is scored on its own learning.Individual built from the shared model package; the individuals share the brain fingerprints and nothing else.",
			"The corridor is the one-dimensional gridnav fixture and the delayed task the pulse-association fixture; these are fixtures, not the fly connectome.",
		},
	}, nil
}
