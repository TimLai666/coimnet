package experiment

import (
	"context"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/experiment/gridnav"
)

const (
	// MultiTaskInterleaved selects one eligible task for each update index.
	MultiTaskInterleaved = "interleaved"
	// MultiTaskSameExperience trains every eligible task on one shared input.
	MultiTaskSameExperience = "same_experience"
	// MultiTaskMissingModality trains on shared input with scheduled delayed-channel omissions.
	MultiTaskMissingModality = "missing_modality"
)

// TaskBinding declares one task's claim on the shared core's updates.
type TaskBinding struct {
	Name          string  `json:"name"`
	LossScale     float64 `json:"loss_scale"`
	MixWeight     float64 `json:"mix_weight"`
	SamplingRatio float64 `json:"sampling_ratio"`
	UpdateEvery   uint64  `json:"update_every"`
}

// MultiTaskSchedule is the training half of a multi-task run.
type MultiTaskSchedule struct {
	Tasks        []TaskBinding `json:"tasks"`
	Schedule     string        `json:"schedule"`
	Budget       uint64        `json:"budget"`
	Tolerance    float64       `json:"tolerance"`
	MissingEvery uint64        `json:"missing_every"`
}

// Validate checks task identity, weights, schedule mode and update bounds.
func (s MultiTaskSchedule) Validate() error {
	if len(s.Tasks) != 2 {
		return fmt.Errorf("multitask schedule has %d tasks, want exactly 2", len(s.Tasks))
	}
	seen := make(map[string]bool, len(s.Tasks))
	for i, task := range s.Tasks {
		if task.Name != MultiTaskCorridor && task.Name != MultiTaskDelayed {
			return fmt.Errorf("task %d has unknown name %q", i, task.Name)
		}
		if seen[task.Name] {
			return fmt.Errorf("task %d duplicates task %q", i, task.Name)
		}
		seen[task.Name] = true
		if !finite(task.LossScale) || task.LossScale <= 0 {
			return fmt.Errorf("task %q loss_scale must be finite and positive", task.Name)
		}
		if !finite(task.MixWeight) || task.MixWeight <= 0 {
			return fmt.Errorf("task %q mix_weight must be finite and positive", task.Name)
		}
		if !finite(task.SamplingRatio) || task.SamplingRatio <= 0 {
			return fmt.Errorf("task %q sampling_ratio must be finite and positive", task.Name)
		}
		if task.UpdateEvery == 0 {
			return fmt.Errorf("task %q update_every must be at least 1", task.Name)
		}
	}
	if s.Budget == 0 || s.Budget > 100000 {
		return fmt.Errorf("budget %d must be in [1, 100000]", s.Budget)
	}
	if !finite(s.Tolerance) || s.Tolerance <= 0 || s.Tolerance >= 1 {
		return fmt.Errorf("tolerance %v must be in (0, 1)", s.Tolerance)
	}
	switch s.Schedule {
	case MultiTaskInterleaved, MultiTaskSameExperience:
		if s.MissingEvery != 0 {
			return fmt.Errorf("missing_every must be 0 for schedule %q", s.Schedule)
		}
	case MultiTaskMissingModality:
		if s.MissingEvery < 2 {
			return fmt.Errorf("missing_every must be at least 2 for schedule %q", s.Schedule)
		}
	default:
		return fmt.Errorf("unknown multitask schedule %q", s.Schedule)
	}
	return nil
}

// TaskShare reports the realised and declared loss-term share of one task.
type TaskShare struct {
	Task          string  `json:"task"`
	Updates       uint64  `json:"updates"`
	Share         float64 `json:"share"`
	Declared      float64 `json:"declared"`
	Flagged       bool    `json:"flagged"`
	LossScale     float64 `json:"loss_scale"`
	MixWeight     float64 `json:"mix_weight"`
	SamplingRatio float64 `json:"sampling_ratio"`
	UpdateEvery   uint64  `json:"update_every"`
}

// runMultiTaskSchedule applies the requested update schedule to the shared model.
func runMultiTaskSchedule(ctx context.Context, m *multiTaskModel, s MultiTaskSchedule, corridor gridnav.Config, seed uint64) (shares []TaskShare, steps, idle uint64, err error) {
	if err := s.Validate(); err != nil {
		return nil, 0, 0, err
	}
	if ctx == nil {
		return nil, 0, 0, fmt.Errorf("multitask schedule context is nil")
	}
	if m == nil || m.trainer == nil {
		return nil, 0, 0, fmt.Errorf("multitask model and trainer must be non-nil")
	}
	if _, err := gridnav.New(corridor); err != nil {
		return nil, 0, 0, fmt.Errorf("invalid corridor config: %w", err)
	}

	declared := declaredTaskShares(s.Tasks)
	updates := make([]uint64, len(s.Tasks))
	episode := make([]uint64, len(s.Tasks))
	for u := uint64(0); u < s.Budget; u++ {
		eligible := make([]int, 0, len(s.Tasks))
		for i, task := range s.Tasks {
			if u%task.UpdateEvery == 0 {
				eligible = append(eligible, i)
			}
		}
		if s.Schedule == MultiTaskMissingModality && u%s.MissingEvery == s.MissingEvery-1 {
			eligible = withoutTask(s.Tasks, eligible, MultiTaskDelayed)
		}
		if len(eligible) == 0 {
			idle++
			continue
		}

		var stepErr error
		if s.Schedule == MultiTaskInterleaved {
			chosen := eligible[0]
			for _, i := range eligible[1:] {
				if declared[i]*float64(u+1)-float64(updates[i]) > declared[chosen]*float64(u+1)-float64(updates[chosen]) {
					chosen = i
				}
			}
			stepErr = runInterleavedTask(ctx, m, s.Tasks[chosen], corridor, seed, episode[chosen])
			if stepErr == nil {
				updates[chosen]++
				episode[chosen]++
			}
		} else {
			stepErr = runSameExperience(ctx, m, s.Tasks, eligible, corridor, seed, episode)
			if stepErr == nil {
				for _, i := range eligible {
					updates[i]++
					episode[i]++
				}
			}
		}
		if stepErr != nil {
			return buildTaskShares(s, declared, updates, s.Tolerance), steps, idle, stepErr
		}
		steps++
	}
	return buildTaskShares(s, declared, updates, s.Tolerance), steps, idle, nil
}

func declaredTaskShares(tasks []TaskBinding) []float64 {
	maxRatio := 0.0
	for _, task := range tasks {
		if task.SamplingRatio > maxRatio {
			maxRatio = task.SamplingRatio
		}
	}
	total := 0.0
	for _, task := range tasks {
		total += task.SamplingRatio / maxRatio
	}
	shares := make([]float64, len(tasks))
	for i, task := range tasks {
		shares[i] = (task.SamplingRatio / maxRatio) / total
	}
	return shares
}

func buildTaskShares(s MultiTaskSchedule, declared []float64, updates []uint64, tolerance float64) []TaskShare {
	var total uint64
	for _, count := range updates {
		total += count
	}
	shares := make([]TaskShare, len(s.Tasks))
	for i, task := range s.Tasks {
		share := 0.0
		if total != 0 {
			share = float64(updates[i]) / float64(total)
		}
		delta := math.Abs(share - declared[i])
		shares[i] = TaskShare{
			Task: task.Name, Updates: updates[i], Share: share, Declared: declared[i],
			Flagged: delta > tolerance, LossScale: task.LossScale, MixWeight: task.MixWeight,
			SamplingRatio: task.SamplingRatio, UpdateEvery: task.UpdateEvery,
		}
	}
	return shares
}

func withoutTask(tasks []TaskBinding, eligible []int, name string) []int {
	out := eligible[:0]
	for _, i := range eligible {
		if tasks[i].Name != name {
			out = append(out, i)
		}
	}
	return out
}

func runInterleavedTask(ctx context.Context, m *multiTaskModel, task TaskBinding, corridor gridnav.Config, seed, episode uint64) error {
	switch task.Name {
	case MultiTaskCorridor:
		obs, actions, err := imitationExpertEpisode(corridor, imitationTrainSeed(seed, int(episode)))
		if err != nil {
			return fmt.Errorf("corridor episode %d: %w", episode, err)
		}
		return m.corridorStep(ctx, obs, actions, task.LossScale)
	case MultiTaskDelayed:
		return m.delayedStep(ctx, DelayedEpisode(1001, episode), task.LossScale)
	default:
		return fmt.Errorf("unknown task %q", task.Name)
	}
}

func runSameExperience(ctx context.Context, m *multiTaskModel, tasks []TaskBinding, eligible []int, corridor gridnav.Config, seed uint64, episode []uint64) error {
	active := make(map[string]TaskBinding, len(eligible))
	rows := 5
	var corridorObs [][]float64
	var corridorActions []int
	var delayed Episode
	for _, i := range eligible {
		task := tasks[i]
		active[task.Name] = task
		switch task.Name {
		case MultiTaskCorridor:
			obs, actions, err := imitationExpertEpisode(corridor, imitationTrainSeed(seed, int(episode[i])))
			if err != nil {
				return fmt.Errorf("corridor episode %d: %w", episode[i], err)
			}
			corridorObs, corridorActions = obs, actions
			if n := len(obs) * multiTaskSettle; n > rows {
				rows = n
			}
		case MultiTaskDelayed:
			delayed = DelayedEpisode(1001, episode[i])
		}
	}
	input := make([][]float64, rows)
	for t := range input {
		input[t] = make([]float64, multiTaskInputs)
	}
	if _, ok := active[MultiTaskCorridor]; ok {
		for k, obs := range corridorObs {
			for j, v := range obs {
				for r := k * multiTaskSettle; r < (k+1)*multiTaskSettle; r++ {
					input[r][j] = v
				}
			}
		}
	}
	if _, ok := active[MultiTaskDelayed]; ok {
		delayedRows, err := multiTaskDelayedRows(delayed)
		if err != nil {
			return err
		}
		for t, row := range delayedRows {
			input[t][multiTaskDelayedChannel] = row[multiTaskDelayedChannel]
		}
	}
	outputs, err := m.trainer.PredictAll(ctx, input)
	if err != nil {
		return err
	}
	upstream := make([][]float64, rows)
	for t := range upstream {
		upstream[t] = make([]float64, multiTaskOutputs)
	}
	if task, ok := active[MultiTaskCorridor]; ok {
		per := task.MixWeight * task.LossScale / float64(len(corridorObs))
		for k, action := range corridorActions {
			t := (k+1)*multiTaskSettle - 1
			p := imitationSoftmax(outputs[t][:gridnav.Actions])
			p[action]--
			for j, v := range p {
				upstream[t][j] = per * v
			}
		}
	}
	if task, ok := active[MultiTaskDelayed]; ok {
		last := len(delayed.Input) - 1
		upstream[last][multiTaskDelayedOutput] = task.MixWeight * task.LossScale * 2 * (outputs[last][multiTaskDelayedOutput] - delayed.Target[0])
	}
	if _, err := m.trainer.StepFrom(ctx, input, upstream); err != nil {
		return err
	}
	return nil
}
