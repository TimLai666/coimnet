package experiment

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
)

func multiTaskScheduleTestConfig(schedule string, budget uint64) MultiTaskSchedule {
	return MultiTaskSchedule{
		Tasks: []TaskBinding{
			{Name: MultiTaskCorridor, LossScale: 1, MixWeight: 1, SamplingRatio: 3, UpdateEvery: 1},
			{Name: MultiTaskDelayed, LossScale: 1, MixWeight: 1, SamplingRatio: 1, UpdateEvery: 1},
		},
		Schedule: schedule, Budget: budget, Tolerance: 0.05,
	}
}

func newMultiTaskScheduleTestModel(t *testing.T) *multiTaskModel {
	t.Helper()
	m, err := newMultiTaskModel(1, 16, 0.05)
	if err != nil {
		t.Fatalf("newMultiTaskModel: %v", err)
	}
	return m
}

func TestMultiTaskScheduleInterleavedFollowsTheRatio(t *testing.T) {
	s := multiTaskScheduleTestConfig(MultiTaskInterleaved, 200)
	corridor := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	shares, steps, idle, err := runMultiTaskSchedule(context.Background(), newMultiTaskScheduleTestModel(t), s, corridor, 1)
	if err != nil {
		t.Fatalf("runMultiTaskSchedule: %v", err)
	}
	t.Logf("interleaved task updates: %s=%d (%.2f), %s=%d (%.2f)", shares[0].Task, shares[0].Updates, shares[0].Share, shares[1].Task, shares[1].Updates, shares[1].Share)
	wantUpdates := []uint64{150, 50}
	wantShares := []float64{0.75, 0.25}
	for i, share := range shares {
		if share.Updates != wantUpdates[i] || share.Share != wantShares[i] || share.Flagged {
			t.Errorf("share[%d] = %+v, want updates=%d share=%v unflagged", i, share, wantUpdates[i], wantShares[i])
		}
	}
	if steps != 200 || idle != 0 {
		t.Errorf("steps=%d idle=%d, want 200 and 0", steps, idle)
	}
}

func TestMultiTaskScheduleUpdateEveryCreatesIdleIndices(t *testing.T) {
	s := multiTaskScheduleTestConfig(MultiTaskInterleaved, 8)
	s.Tasks[0].SamplingRatio, s.Tasks[1].SamplingRatio = 1, 1
	s.Tasks[0].UpdateEvery, s.Tasks[1].UpdateEvery = 2, 4
	corridor := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	shares, steps, idle, err := runMultiTaskSchedule(context.Background(), newMultiTaskScheduleTestModel(t), s, corridor, 1)
	if err != nil {
		t.Fatalf("runMultiTaskSchedule: %v", err)
	}
	t.Logf("interleaved with UpdateEvery task updates: %+v", shares)
	if idle != 4 {
		t.Errorf("idle=%d, want 4", idle)
	}
	if steps != 4 {
		t.Errorf("steps=%d, want 4", steps)
	}
}

func TestMultiTaskScheduleSameExperienceFlagsTheDeclaredRatio(t *testing.T) {
	s := multiTaskScheduleTestConfig(MultiTaskSameExperience, 20)
	corridor := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	shares, steps, idle, err := runMultiTaskSchedule(context.Background(), newMultiTaskScheduleTestModel(t), s, corridor, 1)
	if err != nil {
		t.Fatalf("runMultiTaskSchedule: %v", err)
	}
	t.Logf("same_experience task updates: %s=%d (%.2f), %s=%d (%.2f)", shares[0].Task, shares[0].Updates, shares[0].Share, shares[1].Task, shares[1].Updates, shares[1].Share)
	for i, share := range shares {
		if share.Updates != 20 || share.Share != 0.5 || !share.Flagged {
			t.Errorf("share[%d] = %+v, want 20 updates, share 0.5 and flagged", i, share)
		}
	}
	if steps != 20 || idle != 0 {
		t.Errorf("steps=%d idle=%d, want 20 and 0", steps, idle)
	}
}

func TestMultiTaskScheduleMissingModalitySkipsDelayedTerms(t *testing.T) {
	s := multiTaskScheduleTestConfig(MultiTaskMissingModality, 20)
	s.MissingEvery = 2
	corridor := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	shares, steps, idle, err := runMultiTaskSchedule(context.Background(), newMultiTaskScheduleTestModel(t), s, corridor, 1)
	if err != nil {
		t.Fatalf("runMultiTaskSchedule: %v", err)
	}
	t.Logf("missing_modality task updates: %s=%d (%.2f), %s=%d (%.2f)", shares[0].Task, shares[0].Updates, shares[0].Share, shares[1].Task, shares[1].Updates, shares[1].Share)
	if shares[0].Updates != 20 || shares[1].Updates != 10 {
		t.Errorf("updates = %d/%d, want 20/10", shares[0].Updates, shares[1].Updates)
	}
	if steps != 20 || idle != 0 {
		t.Errorf("steps=%d idle=%d, want 20 and 0", steps, idle)
	}
}

func TestMultiTaskScheduleIsDeterministic(t *testing.T) {
	s := multiTaskScheduleTestConfig(MultiTaskInterleaved, 12)
	corridor := gridnav.Config{Length: 7, TimeLimit: 20, StepPenalty: 0.01, GoalReward: 1}
	m1, m2 := newMultiTaskScheduleTestModel(t), newMultiTaskScheduleTestModel(t)
	first, steps1, idle1, err := runMultiTaskSchedule(context.Background(), m1, s, corridor, 1)
	if err != nil {
		t.Fatalf("first runMultiTaskSchedule: %v", err)
	}
	second, steps2, idle2, err := runMultiTaskSchedule(context.Background(), m2, s, corridor, 1)
	if err != nil {
		t.Fatalf("second runMultiTaskSchedule: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("shares differ: %+v != %+v", first, second)
	}
	if steps1 != steps2 || idle1 != idle2 {
		t.Errorf("run counts differ: (%d,%d) != (%d,%d)", steps1, idle1, steps2, idle2)
	}
	if got, want := m1.baseParameterHash(), m2.baseParameterHash(); got != want {
		t.Errorf("trained base parameter hashes differ: %s != %s", got, want)
	}
}

func TestMultiTaskConfigValidate(t *testing.T) {
	valid := multiTaskScheduleTestConfig(MultiTaskInterleaved, 8)
	cases := []struct {
		name string
		edit func(*MultiTaskSchedule)
	}{
		{"only one task", func(s *MultiTaskSchedule) { s.Tasks = s.Tasks[:1] }},
		{"duplicate task", func(s *MultiTaskSchedule) { s.Tasks[1].Name = s.Tasks[0].Name }},
		{"unknown task", func(s *MultiTaskSchedule) { s.Tasks[0].Name = "unknown" }},
		{"zero loss scale", func(s *MultiTaskSchedule) { s.Tasks[0].LossScale = 0 }},
		{"negative mix weight", func(s *MultiTaskSchedule) { s.Tasks[0].MixWeight = -1 }},
		{"zero sampling ratio", func(s *MultiTaskSchedule) { s.Tasks[0].SamplingRatio = 0 }},
		{"zero update every", func(s *MultiTaskSchedule) { s.Tasks[0].UpdateEvery = 0 }},
		{"zero budget", func(s *MultiTaskSchedule) { s.Budget = 0 }},
		{"zero tolerance", func(s *MultiTaskSchedule) { s.Tolerance = 0 }},
		{"unit tolerance", func(s *MultiTaskSchedule) { s.Tolerance = 1 }},
		{"unknown schedule", func(s *MultiTaskSchedule) { s.Schedule = "unknown" }},
		{"missing every too small", func(s *MultiTaskSchedule) { s.Schedule, s.MissingEvery = MultiTaskMissingModality, 1 }},
		{"missing every on other schedule", func(s *MultiTaskSchedule) { s.MissingEvery = 3 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			s.Tasks = append([]TaskBinding(nil), valid.Tasks...)
			tc.edit(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("Validate() succeeded, want an error")
			}
		})
	}
}
