package learning_test

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

// scheduleOptions is the accumulation fixture's optimizer with a base learning
// rate of 0.2, which every expected number in this file is written against.
func scheduleOptions(s *learning.Schedule) learning.Options {
	o := accumulationOptions()
	o.LearningRate = .2
	o.Schedule = s
	return o
}

// TestLearningRateTablesForTheThreeKinds is the hand-computed curve of every
// schedule kind. Warmup is a linear ramp from zero to the base rate over
// WarmupUpdates and applies to all three kinds; after it, "constant" holds the
// base rate, "step" multiplies by StepFactor once per StepEvery completed
// updates, and "cosine" follows half a cosine from the base rate down to
// base * FinalFactor over DecayUpdates and then holds there.
func TestLearningRateTablesForTheThreeKinds(t *testing.T) {
	tables := []struct {
		name     string
		schedule *learning.Schedule
		want     map[uint64]float64
	}{
		{"no schedule at all", nil, map[uint64]float64{0: .2, 7: .2, 1 << 20: .2}},
		{
			"constant with a four-update warmup",
			&learning.Schedule{Kind: learning.ScheduleConstant, WarmupUpdates: 4},
			map[uint64]float64{0: 0, 1: .05, 2: .1, 3: .15, 4: .2, 9: .2},
		},
		{
			"step, halved every five updates",
			&learning.Schedule{Kind: learning.ScheduleStep, StepEvery: 5, StepFactor: .5},
			map[uint64]float64{0: .2, 4: .2, 5: .1, 9: .1, 10: .05, 20: .0125},
		},
		{
			"step behind a three-update warmup",
			&learning.Schedule{Kind: learning.ScheduleStep, WarmupUpdates: 3, StepEvery: 5, StepFactor: .5},
			map[uint64]float64{0: 0, 1: 0.06666666666666667, 2: 0.13333333333333333, 3: .2, 5: .1, 12: .05},
		},
		{
			"cosine from a four-update warmup over eight updates down to a tenth",
			&learning.Schedule{Kind: learning.ScheduleCosine, WarmupUpdates: 4, DecayUpdates: 8, FinalFactor: .1},
			map[uint64]float64{0: 0, 2: .1, 4: .2, 6: 0.17363961030678929, 8: .11, 12: .02, 100: .02},
		},
	}
	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			o := scheduleOptions(table.schedule)
			if table.schedule != nil {
				if _, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o); err != nil {
					t.Fatalf("a valid schedule was rejected: %v", err)
				}
			}
			for updates, want := range table.want {
				got := learning.LearningRateAt(o, updates)
				if math.Abs(got-want) > 1e-12 {
					t.Fatalf("learning rate at %d updates = %.17g, want %.17g", updates, got, want)
				}
			}
		})
	}
}

// TestStepReportsTheScheduledLearningRateAndUsesIt drives a step schedule
// through a trainer: the reported rate is the rate of the update that just ran,
// and the parameters move by exactly that rate.
func TestStepReportsTheScheduledLearningRateAndUsesIt(t *testing.T) {
	o := scheduleOptions(&learning.Schedule{Kind: learning.ScheduleStep, StepEvery: 2, StepFactor: .5})
	tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
	if err != nil {
		t.Fatal(err)
	}
	inputs, targets := accumulationBatches()
	want := []float64{.2, .2, .1, .1, .05, .05}
	for i, lr := range want {
		result, err := tr.Step(context.Background(), inputs[i%len(inputs)], targets[i%len(targets)])
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if math.Abs(result.LearningRate-lr) > 1e-12 {
			t.Fatalf("step %d reported learning rate %.17g, want %.17g", i, result.LearningRate, lr)
		}
	}
	// The first update is AdamW's first step on the first batch's gradient at
	// the declared base rate, which is the only rate this fixture can reach in
	// one update; running it again with a constant schedule must agree.
	constant, err := learning.NewTrainer(continuousConfig(), continuousParameters(), scheduleOptions(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := constant.Step(context.Background(), inputs[0], targets[0]); err != nil {
		t.Fatal(err)
	}
	first := adamWFirstUpdate(accumulationParameters, accumulationGradient1, .2, o.Epsilon)
	requireSlicesClose(t, "first scheduled update", flatTestParameters(constant.Snapshot().Parameters), first, 1e-12)
}

// TestScheduleResumesOnTheSameCurve restores in the middle of a cosine decay
// and finishes the run; the rate sequence and the final state must be the ones
// an uninterrupted run reaches.
func TestScheduleResumesOnTheSameCurve(t *testing.T) {
	o := scheduleOptions(&learning.Schedule{Kind: learning.ScheduleCosine, WarmupUpdates: 2, DecayUpdates: 6, FinalFactor: .25})
	inputs, targets := accumulationBatches()
	run := func(interrupt bool) ([]float64, learning.TrainingSnapshot) {
		t.Helper()
		tr, err := learning.NewTrainer(continuousConfig(), continuousParameters(), o)
		if err != nil {
			t.Fatal(err)
		}
		rates := make([]float64, 0, 6)
		for i := range 6 {
			result, err := tr.Step(context.Background(), inputs[i%len(inputs)], targets[i%len(targets)])
			if err != nil {
				t.Fatal(err)
			}
			rates = append(rates, result.LearningRate)
			if interrupt && i == 2 {
				if tr, err = learning.RestoreTrainer(tr.Snapshot()); err != nil {
					t.Fatal(err)
				}
			}
		}
		return rates, tr.Snapshot()
	}
	rates, uninterrupted := run(false)
	resumedRates, resumed := run(true)
	if !reflect.DeepEqual(rates, resumedRates) {
		t.Fatalf("the resumed rates diverged: %v != %v", resumedRates, rates)
	}
	if !reflect.DeepEqual(uninterrupted, resumed) {
		t.Fatal("the resumed run diverged from the uninterrupted one")
	}
	// Written out: warmup 2 gives 0 and 0.1, then the cosine from update 2 over
	// six updates down to 0.05.
	want := []float64{0, .1, .2}
	for i := 3; i < 6; i++ {
		progress := float64(i-2) / 6
		want = append(want, .2*(.25+.75*.5*(1+math.Cos(math.Pi*progress))))
	}
	requireSlicesClose(t, "cosine rate sequence", rates, want, 1e-12)
}

// TestInvalidSchedulesAreRejected keeps every declared field meaningful: an
// unknown kind, a bound outside its range and a field the kind does not read
// are all refused before a trainer exists.
func TestInvalidSchedulesAreRejected(t *testing.T) {
	invalid := map[string]learning.Schedule{
		"unknown kind":                 {Kind: "linear"},
		"empty kind":                   {},
		"step without a period":        {Kind: learning.ScheduleStep, StepFactor: .5},
		"step without a factor":        {Kind: learning.ScheduleStep, StepEvery: 5},
		"step with a negative factor":  {Kind: learning.ScheduleStep, StepEvery: 5, StepFactor: -.5},
		"cosine without a decay":       {Kind: learning.ScheduleCosine, FinalFactor: .5},
		"final factor above one":       {Kind: learning.ScheduleCosine, DecayUpdates: 4, FinalFactor: 1.5},
		"negative final factor":        {Kind: learning.ScheduleCosine, DecayUpdates: 4, FinalFactor: -.5},
		"non-finite final factor":      {Kind: learning.ScheduleCosine, DecayUpdates: 4, FinalFactor: math.NaN()},
		"constant with a step factor":  {Kind: learning.ScheduleConstant, StepFactor: .5},
		"cosine with a step period":    {Kind: learning.ScheduleCosine, DecayUpdates: 4, StepEvery: 3},
		"step with a final factor":     {Kind: learning.ScheduleStep, StepEvery: 5, StepFactor: .5, FinalFactor: .5},
		"constant with a decay window": {Kind: learning.ScheduleConstant, DecayUpdates: 4},
	}
	for name, schedule := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := learning.NewTrainer(continuousConfig(), continuousParameters(), scheduleOptions(&schedule)); err == nil {
				t.Fatalf("NewTrainer accepted %s", name)
			}
		})
	}
	valid, err := learning.NewTrainer(continuousConfig(), continuousParameters(),
		scheduleOptions(&learning.Schedule{Kind: learning.ScheduleCosine, WarmupUpdates: 2, DecayUpdates: 6, FinalFactor: .25}))
	if err != nil {
		t.Fatal(err)
	}
	s := valid.Snapshot()
	if s.Options.Schedule == nil || s.Options.Schedule.Kind != learning.ScheduleCosine {
		t.Fatalf("the schedule did not travel with the snapshot: %+v", s.Options.Schedule)
	}
	s.Options.Schedule.Kind = "linear"
	if _, err := learning.RestoreTrainer(s); err == nil {
		t.Fatal("RestoreTrainer accepted an unknown schedule kind")
	}
	owned := valid.Snapshot()
	owned.Options.Schedule.StepEvery = 99
	if valid.Snapshot().Options.Schedule.StepEvery != 0 {
		t.Fatal("the snapshot aliases the trainer's schedule")
	}
}
