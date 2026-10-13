package learning_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// sharedRestoreConfig82 gives every family a real shared group while keeping
// one singleton and one unshared entry. The four groups are deliberately
// family-local, as ParameterSharing requires.
func sharedRestoreConfig82(shared bool) learning.Config {
	c := sharingConfig()
	if shared {
		c.Sharing = &learning.ParameterSharing{
			Weights: []int32{0, 0},
			Bias:    []int32{1, 1, 2},
			LogTau:  []int32{3, 3, -1},
			Groups:  4,
		}
	}
	return c
}

func sharedRestoreInput82() [][]float64 {
	return [][]float64{{1}, {0}, {1}}
}

func sharedRestoreTarget82() []float64 {
	return []float64{.25}
}

func cloneTrainingSnapshot82(t *testing.T, in learning.TrainingSnapshot) learning.TrainingSnapshot {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out learning.TrainingSnapshot
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func cloneIndividualSnapshot82(t *testing.T, in learning.IndividualSnapshot) learning.IndividualSnapshot {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out learning.IndividualSnapshot
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func trainedRestoreTrainer82(t *testing.T, shared bool) learning.TrainingSnapshot {
	t.Helper()
	tr, err := learning.NewTrainer(sharedRestoreConfig82(shared), sharingParameters(.3, .3), learning.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 2; step++ {
		if _, err := tr.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82()); err != nil {
			t.Fatalf("training step %d: %v", step, err)
		}
	}
	snapshot := tr.Snapshot()
	if snapshot.Updates != 2 {
		t.Fatalf("fixture updates = %d, want 2", snapshot.Updates)
	}
	if snapshot.Optimizer.Steps[0] < 2 || snapshot.Optimizer.Steps[2] < 2 || snapshot.Optimizer.Steps[5] < 2 {
		t.Fatalf("fixture did not produce two valid optimizer steps: %+v", snapshot.Optimizer.Steps)
	}
	return snapshot
}

func accumulatedRestoreTrainer82(t *testing.T, shared bool) learning.TrainingSnapshot {
	t.Helper()
	o := learning.DefaultOptions()
	o.AccumulateSteps = 3
	tr, err := learning.NewTrainer(sharedRestoreConfig82(shared), sharingParameters(.3, .3), o)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tr.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
	if err != nil {
		t.Fatalf("partial training step: %v", err)
	}
	if result.Applied || result.Accumulated != 1 {
		t.Fatalf("partial fixture result = %+v, want unapplied count 1", result)
	}
	snapshot := tr.Snapshot()
	if snapshot.Updates != 0 || snapshot.Accumulator == nil || snapshot.Accumulator.Count != 1 {
		t.Fatalf("partial fixture optimizer = %+v, want one open window", snapshot.Optimizer)
	}
	return snapshot
}

func trainedRestoreIndividual82(t *testing.T, shared bool) learning.IndividualSnapshot {
	t.Helper()
	c := sharedRestoreConfig82(shared)
	ind, err := learning.NewIndividual(c, sharingParameters(.3, .3), learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	for episode := 0; episode < 2; episode++ {
		if _, err := ind.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82()); err != nil {
			t.Fatalf("training episode %d: %v", episode, err)
		}
	}
	snapshot := ind.Snapshot()
	if snapshot.Optimizer.Updates != 2 || snapshot.Optimizer.State.Steps[0] < 2 || snapshot.Optimizer.State.Steps[2] < 2 || snapshot.Optimizer.State.Steps[5] < 2 {
		t.Fatalf("individual fixture did not produce two valid optimizer steps: updates=%d steps=%v", snapshot.Optimizer.Updates, snapshot.Optimizer.State.Steps)
	}
	return snapshot
}

func accumulatedRestoreIndividual82(t *testing.T, shared bool) learning.IndividualSnapshot {
	t.Helper()
	c := sharedRestoreConfig82(shared)
	o := learning.DefaultOptions()
	o.AccumulateSteps = 3
	ind, err := learning.NewIndividual(c, sharingParameters(.3, .3), o, make([]float64, c.Dynamics.Nodes))
	if err != nil {
		t.Fatal(err)
	}
	result, err := ind.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
	if err != nil {
		t.Fatalf("partial training episode: %v", err)
	}
	if result.Applied || result.Accumulated != 1 {
		t.Fatalf("partial individual result = %+v, want unapplied count 1", result)
	}
	snapshot := ind.Snapshot()
	if snapshot.Optimizer.Updates != 0 || snapshot.Optimizer.Accumulator == nil || snapshot.Optimizer.Accumulator.Count != 1 {
		t.Fatalf("partial individual optimizer = %+v, want one open window", snapshot.Optimizer)
	}
	return snapshot
}

func requireSharedOptimizerMembers82(t *testing.T, state learning.AdamState) {
	t.Helper()
	for _, members := range [][2]int{{0, 1}, {2, 3}, {5, 6}} {
		a, b := members[0], members[1]
		if state.First[a] != state.First[b] || state.Second[a] != state.Second[b] || state.Steps[a] != state.Steps[b] {
			t.Fatalf("fixture shared optimizer members %d/%d differ: first=%g/%g second=%g/%g steps=%d/%d", a, b, state.First[a], state.First[b], state.Second[a], state.Second[b], state.Steps[a], state.Steps[b])
		}
	}
}

func requireSharedAccumulatorMembers82(t *testing.T, accumulator *learning.GradientAccumulator) {
	t.Helper()
	if accumulator == nil {
		t.Fatal("fixture has no partial accumulator")
	}
	for _, members := range [][2]int{{0, 1}, {2, 3}, {5, 6}} {
		a, b := members[0], members[1]
		if accumulator.Sum[a] != accumulator.Sum[b] {
			t.Fatalf("fixture shared accumulator members %d/%d differ: %g/%g", a, b, accumulator.Sum[a], accumulator.Sum[b])
		}
	}
}

func requireSharedParameterMembers82(t *testing.T, parameters learning.Parameters) {
	t.Helper()
	for _, family := range []struct {
		name   string
		values []float64
		a, b   int
	}{
		{name: "weights", values: parameters.Core.Weights, a: 0, b: 1},
		{name: "bias", values: parameters.Core.Bias, a: 0, b: 1},
		{name: "log_tau", values: parameters.Core.LogTau, a: 0, b: 1},
	} {
		if family.values[family.a] != family.values[family.b] {
			t.Fatalf("fixture shared %s parameters differ: %g/%g", family.name, family.values[family.a], family.values[family.b])
		}
	}
}

func requireSharedTrainingState82(t *testing.T, snapshot learning.TrainingSnapshot) {
	t.Helper()
	requireSharedParameterMembers82(t, snapshot.Parameters)
	requireSharedOptimizerMembers82(t, snapshot.Optimizer)
	if snapshot.Accumulator != nil {
		requireSharedAccumulatorMembers82(t, snapshot.Accumulator)
	}
}

func requireSharedIndividualState82(t *testing.T, snapshot learning.IndividualSnapshot) {
	t.Helper()
	requireSharedParameterMembers82(t, snapshot.Parameters)
	requireSharedOptimizerMembers82(t, snapshot.Optimizer.State)
	if snapshot.Optimizer.Accumulator != nil {
		requireSharedAccumulatorMembers82(t, snapshot.Optimizer.Accumulator)
	}
}

func TestRestoreTrainerSharedAndUnsharedSnapshotsContinueBitIdentically82(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{name: "shared", shared: true},
		{name: "without sharing", shared: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := learning.NewTrainer(sharedRestoreConfig82(tc.shared), sharingParameters(.3, .3), learning.DefaultOptions())
			if err != nil {
				t.Fatal(err)
			}
			for step := 0; step < 2; step++ {
				if _, err := original.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82()); err != nil {
					t.Fatalf("training step %d: %v", step, err)
				}
			}
			saved := original.Snapshot()
			restored, err := learning.RestoreTrainer(saved)
			if err != nil {
				t.Fatalf("RestoreTrainer: %v", err)
			}
			if !reflect.DeepEqual(saved, restored.Snapshot()) {
				t.Fatal("restored trainer changed a valid snapshot before continuation")
			}
			for step := 0; step < 4; step++ {
				want, err := original.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("original continuation step %d: %v", step, err)
				}
				got, err := restored.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("restored continuation step %d: %v", step, err)
				}
				if !reflect.DeepEqual(want, got) || !reflect.DeepEqual(original.Snapshot(), restored.Snapshot()) {
					t.Fatalf("continuation step %d diverged:\noriginal=%+v\nrestored=%+v", step, original.Snapshot(), restored.Snapshot())
				}
			}
		})
	}
}

func TestRestoreIndividualSharedAndUnsharedSnapshotsContinueBitIdentically82(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{name: "shared", shared: true},
		{name: "without sharing", shared: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sharedRestoreConfig82(tc.shared)
			original, err := learning.NewIndividual(c, sharingParameters(.3, .3), learning.DefaultOptions(), make([]float64, c.Dynamics.Nodes))
			if err != nil {
				t.Fatal(err)
			}
			for episode := 0; episode < 2; episode++ {
				if _, err := original.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82()); err != nil {
					t.Fatalf("training episode %d: %v", episode, err)
				}
			}
			saved := original.Snapshot()
			restored, err := learning.RestoreIndividual(saved)
			if err != nil {
				t.Fatalf("RestoreIndividual: %v", err)
			}
			if !reflect.DeepEqual(saved, restored.Snapshot()) {
				t.Fatal("restored individual changed a valid snapshot before continuation")
			}
			for episode := 0; episode < 4; episode++ {
				want, err := original.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("original continuation episode %d: %v", episode, err)
				}
				got, err := restored.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("restored continuation episode %d: %v", episode, err)
				}
				if !reflect.DeepEqual(want, got) || !reflect.DeepEqual(original.Snapshot(), restored.Snapshot()) {
					t.Fatalf("continuation episode %d diverged:\noriginal=%+v\nrestored=%+v", episode, original.Snapshot(), restored.Snapshot())
				}
			}
		})
	}
}

func TestRestoreTrainerSharedAndUnsharedPartialWindowsContinueBitIdentically82(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{name: "shared", shared: true},
		{name: "without sharing", shared: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := learning.DefaultOptions()
			o.AccumulateSteps = 3
			original, err := learning.NewTrainer(sharedRestoreConfig82(tc.shared), sharingParameters(.3, .3), o)
			if err != nil {
				t.Fatal(err)
			}
			first, err := original.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
			if err != nil {
				t.Fatalf("initial partial step: %v", err)
			}
			if first.Applied || first.Accumulated != 1 {
				t.Fatalf("initial result = %+v, want unapplied count 1", first)
			}
			saved := original.Snapshot()
			if saved.Accumulator == nil || saved.Accumulator.Count != 1 {
				t.Fatalf("initial snapshot accumulator = %+v, want count 1", saved.Accumulator)
			}
			if tc.shared {
				requireSharedTrainingState82(t, saved)
			}
			restored, err := learning.RestoreTrainer(saved)
			if err != nil {
				t.Fatalf("RestoreTrainer: %v", err)
			}
			if !reflect.DeepEqual(saved, restored.Snapshot()) {
				t.Fatal("restored partial trainer changed its valid snapshot")
			}
			for step := 0; step < 5; step++ {
				want, err := original.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("original partial continuation step %d: %v", step, err)
				}
				got, err := restored.Step(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("restored partial continuation step %d: %v", step, err)
				}
				if step == 0 && (got.Applied || got.Accumulated != 2) {
					t.Fatalf("step %d result = %+v, want the second partial gradient", step, got)
				}
				if step == 1 && (!got.Applied || got.Accumulated != 0) {
					t.Fatalf("step %d result = %+v, want the completed window", step, got)
				}
				if !reflect.DeepEqual(want, got) || !reflect.DeepEqual(original.Snapshot(), restored.Snapshot()) {
					t.Fatalf("partial continuation step %d diverged:\noriginal=%+v\nrestored=%+v", step, original.Snapshot(), restored.Snapshot())
				}
				if tc.shared {
					requireSharedTrainingState82(t, original.Snapshot())
					requireSharedTrainingState82(t, restored.Snapshot())
				}
			}
		})
	}
}

func TestRestoreIndividualSharedAndUnsharedPartialWindowsContinueBitIdentically82(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{name: "shared", shared: true},
		{name: "without sharing", shared: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sharedRestoreConfig82(tc.shared)
			o := learning.DefaultOptions()
			o.AccumulateSteps = 3
			original, err := learning.NewIndividual(c, sharingParameters(.3, .3), o, make([]float64, c.Dynamics.Nodes))
			if err != nil {
				t.Fatal(err)
			}
			first, err := original.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
			if err != nil {
				t.Fatalf("initial partial episode: %v", err)
			}
			if first.Applied || first.Accumulated != 1 {
				t.Fatalf("initial result = %+v, want unapplied count 1", first)
			}
			saved := original.Snapshot()
			if saved.Optimizer.Accumulator == nil || saved.Optimizer.Accumulator.Count != 1 {
				t.Fatalf("initial snapshot accumulator = %+v, want count 1", saved.Optimizer.Accumulator)
			}
			if tc.shared {
				requireSharedIndividualState82(t, saved)
			}
			restored, err := learning.RestoreIndividual(saved)
			if err != nil {
				t.Fatalf("RestoreIndividual: %v", err)
			}
			if !reflect.DeepEqual(saved, restored.Snapshot()) {
				t.Fatal("restored partial individual changed its valid snapshot")
			}
			for episode := 0; episode < 5; episode++ {
				want, err := original.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("original partial continuation episode %d: %v", episode, err)
				}
				got, err := restored.TrainEpisode(context.Background(), sharedRestoreInput82(), sharedRestoreTarget82())
				if err != nil {
					t.Fatalf("restored partial continuation episode %d: %v", episode, err)
				}
				if episode == 0 && (got.Applied || got.Accumulated != 2) {
					t.Fatalf("episode %d result = %+v, want the second partial gradient", episode, got)
				}
				if episode == 1 && (!got.Applied || got.Accumulated != 0) {
					t.Fatalf("episode %d result = %+v, want the completed window", episode, got)
				}
				if !reflect.DeepEqual(want, got) || !reflect.DeepEqual(original.Snapshot(), restored.Snapshot()) {
					t.Fatalf("partial continuation episode %d diverged:\noriginal=%+v\nrestored=%+v", episode, original.Snapshot(), restored.Snapshot())
				}
				if tc.shared {
					requireSharedIndividualState82(t, original.Snapshot())
					requireSharedIndividualState82(t, restored.Snapshot())
				}
			}
		})
	}
}

func TestRestoreTrainerRejectsSharedOptimizerMemberMismatches82(t *testing.T) {
	base := trainedRestoreTrainer82(t, true)
	requireSharedOptimizerMembers82(t, base.Optimizer)
	mutations := []struct {
		name   string
		mutate func(*learning.TrainingSnapshot)
	}{
		{name: "weights first", mutate: func(s *learning.TrainingSnapshot) { s.Optimizer.First[1] += .125 }},
		{name: "bias second", mutate: func(s *learning.TrainingSnapshot) { s.Optimizer.Second[3] += .25 }},
		{name: "log_tau steps", mutate: func(s *learning.TrainingSnapshot) { s.Optimizer.Steps[6]-- }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneTrainingSnapshot82(t, base)
			tc.mutate(&candidate)
			expectedInput := cloneTrainingSnapshot82(t, candidate)
			restored, err := learning.RestoreTrainer(candidate)
			if restored != nil {
				t.Fatal("RestoreTrainer returned a trainer for inconsistent shared state")
			}
			if err == nil {
				t.Fatal("RestoreTrainer accepted inconsistent shared state")
			}
			if !reflect.DeepEqual(candidate, expectedInput) {
				t.Fatal("RestoreTrainer mutated its snapshot input on rejection")
			}
		})
	}
}

func TestRestoreIndividualRejectsSharedOptimizerMemberMismatches82(t *testing.T) {
	base := trainedRestoreIndividual82(t, true)
	requireSharedOptimizerMembers82(t, base.Optimizer.State)
	mutations := []struct {
		name   string
		mutate func(*learning.IndividualSnapshot)
	}{
		{name: "weights first", mutate: func(s *learning.IndividualSnapshot) { s.Optimizer.State.First[1] += .125 }},
		{name: "bias second", mutate: func(s *learning.IndividualSnapshot) { s.Optimizer.State.Second[3] += .25 }},
		{name: "log_tau steps", mutate: func(s *learning.IndividualSnapshot) { s.Optimizer.State.Steps[6]-- }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneIndividualSnapshot82(t, base)
			tc.mutate(&candidate)
			expectedInput := cloneIndividualSnapshot82(t, candidate)
			restored, err := learning.RestoreIndividual(candidate)
			if restored != nil {
				t.Fatal("RestoreIndividual returned an individual for inconsistent shared state")
			}
			if err == nil {
				t.Fatal("RestoreIndividual accepted inconsistent shared state")
			}
			if !reflect.DeepEqual(candidate, expectedInput) {
				t.Fatal("RestoreIndividual mutated its snapshot input on rejection")
			}
		})
	}
}

func TestRestoreTrainerRejectsSharedPartialAccumulatorMismatches82(t *testing.T) {
	base := accumulatedRestoreTrainer82(t, true)
	requireSharedAccumulatorMembers82(t, base.Accumulator)
	for _, tc := range []struct {
		name  string
		index int
	}{
		{name: "weights", index: 1},
		{name: "bias", index: 3},
		{name: "log_tau", index: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneTrainingSnapshot82(t, base)
			candidate.Accumulator.Sum[tc.index] += .125
			expectedInput := cloneTrainingSnapshot82(t, candidate)
			restored, err := learning.RestoreTrainer(candidate)
			if restored != nil {
				t.Fatal("RestoreTrainer returned a trainer for inconsistent shared accumulator state")
			}
			if err == nil {
				t.Fatal("RestoreTrainer accepted inconsistent shared accumulator state")
			}
			if !reflect.DeepEqual(candidate, expectedInput) {
				t.Fatal("RestoreTrainer mutated its accumulator input on rejection")
			}
		})
	}
}

func TestRestoreIndividualRejectsSharedPartialAccumulatorMismatches82(t *testing.T) {
	base := accumulatedRestoreIndividual82(t, true)
	requireSharedAccumulatorMembers82(t, base.Optimizer.Accumulator)
	for _, tc := range []struct {
		name  string
		index int
	}{
		{name: "weights", index: 1},
		{name: "bias", index: 3},
		{name: "log_tau", index: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneIndividualSnapshot82(t, base)
			candidate.Optimizer.Accumulator.Sum[tc.index] += .125
			expectedInput := cloneIndividualSnapshot82(t, candidate)
			restored, err := learning.RestoreIndividual(candidate)
			if restored != nil {
				t.Fatal("RestoreIndividual returned an individual for inconsistent shared accumulator state")
			}
			if err == nil {
				t.Fatal("RestoreIndividual accepted inconsistent shared accumulator state")
			}
			if !reflect.DeepEqual(candidate, expectedInput) {
				t.Fatal("RestoreIndividual mutated its accumulator input on rejection")
			}
		})
	}
}

func TestRestoreAcceptsSingletonAndUnsharedOptimizerDifferences82(t *testing.T) {
	base := trainedRestoreTrainer82(t, true)
	// Bias[2] is the sole member of group 2; LogTau[2] is explicitly unshared.
	base.Optimizer.First[4] = base.Optimizer.First[2] + .125
	base.Optimizer.Second[4] = base.Optimizer.Second[2] + .25
	base.Optimizer.First[7] = base.Optimizer.First[5] + .375
	base.Optimizer.Second[7] = base.Optimizer.Second[5] + .5
	restored, err := learning.RestoreTrainer(base)
	if err != nil {
		t.Fatalf("RestoreTrainer rejected legal singleton/unshared differences: %v", err)
	}
	if !reflect.DeepEqual(base, restored.Snapshot()) {
		t.Fatal("RestoreTrainer did not preserve legal singleton/unshared optimizer state")
	}

	individual := trainedRestoreIndividual82(t, true)
	individual.Optimizer.State.First[4] = individual.Optimizer.State.First[2] + .125
	individual.Optimizer.State.Second[4] = individual.Optimizer.State.Second[2] + .25
	individual.Optimizer.State.First[7] = individual.Optimizer.State.First[5] + .375
	individual.Optimizer.State.Second[7] = individual.Optimizer.State.Second[5] + .5
	restoredIndividual, err := learning.RestoreIndividual(individual)
	if err != nil {
		t.Fatalf("RestoreIndividual rejected legal singleton/unshared differences: %v", err)
	}
	if !reflect.DeepEqual(individual, restoredIndividual.Snapshot()) {
		t.Fatal("RestoreIndividual did not preserve legal singleton/unshared optimizer state")
	}
}

func TestRestoreTrainerAcceptsSingletonAndUnsharedPartialAccumulatorDifferences82(t *testing.T) {
	base := accumulatedRestoreTrainer82(t, true)
	requireSharedTrainingState82(t, base)
	base.Accumulator.Sum[4] = base.Accumulator.Sum[2] + .125
	base.Accumulator.Sum[7] = base.Accumulator.Sum[5] + .25
	if base.Accumulator.Sum[4] == base.Accumulator.Sum[2] || base.Accumulator.Sum[7] == base.Accumulator.Sum[5] {
		t.Fatal("partial singleton/unshared fixture did not create distinct sums")
	}
	restored, err := learning.RestoreTrainer(base)
	if err != nil {
		t.Fatalf("RestoreTrainer rejected legal singleton/unshared accumulator differences: %v", err)
	}
	if !reflect.DeepEqual(base, restored.Snapshot()) {
		t.Fatal("RestoreTrainer did not preserve legal singleton/unshared accumulator differences")
	}
	requireSharedTrainingState82(t, restored.Snapshot())
}

func TestRestoreIndividualAcceptsSingletonAndUnsharedPartialAccumulatorDifferences82(t *testing.T) {
	base := accumulatedRestoreIndividual82(t, true)
	requireSharedIndividualState82(t, base)
	base.Optimizer.Accumulator.Sum[4] = base.Optimizer.Accumulator.Sum[2] + .125
	base.Optimizer.Accumulator.Sum[7] = base.Optimizer.Accumulator.Sum[5] + .25
	if base.Optimizer.Accumulator.Sum[4] == base.Optimizer.Accumulator.Sum[2] || base.Optimizer.Accumulator.Sum[7] == base.Optimizer.Accumulator.Sum[5] {
		t.Fatal("partial singleton/unshared fixture did not create distinct sums")
	}
	restored, err := learning.RestoreIndividual(base)
	if err != nil {
		t.Fatalf("RestoreIndividual rejected legal singleton/unshared accumulator differences: %v", err)
	}
	if !reflect.DeepEqual(base, restored.Snapshot()) {
		t.Fatal("RestoreIndividual did not preserve legal singleton/unshared accumulator differences")
	}
	requireSharedIndividualState82(t, restored.Snapshot())
}
