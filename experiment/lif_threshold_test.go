package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

const lifThresholdTestUpdates = 60

func TestLIFThresholdProtocolMatchesTheDelayedBenchmark(t *testing.T) {
	if !reflect.DeepEqual(lifThresholdSeeds, DefaultDelayedConfig().Seeds) {
		t.Fatalf("threshold seeds %v differ from the delayed benchmark seeds %v", lifThresholdSeeds, DefaultDelayedConfig().Seeds)
	}
	if lifThresholdTrainSeed == lifThresholdHoldoutSeed {
		t.Fatal("training and holdout splits share a seed")
	}
	// Distinct seeds alone do not ensure distinct counter streams, so the
	// generated counters themselves are compared at the largest budget.
	training := make(map[uint64]bool, LIFThresholdMaxUpdates)
	for i := 0; i < LIFThresholdMaxUpdates; i++ {
		training[lifThresholdTrainSeed+uint64(i)*lifThresholdCounterIncrement] = true
	}
	for i := 0; i < lifThresholdHoldoutCount; i++ {
		if training[lifThresholdHoldoutSeed+uint64(i)*lifThresholdCounterIncrement] {
			t.Fatalf("holdout sample %d reuses a training counter", i)
		}
	}
}

func TestRunLIFThresholdReportStructureAndDeterminism(t *testing.T) {
	ctx := context.Background()
	first, err := RunLIFThreshold(ctx, lifThresholdTestUpdates)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunLIFThreshold(ctx, lifThresholdTestUpdates)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("two runs produced different reports")
	}
	if first.SchemaVersion != "coimnet-lif-threshold-example/v1" {
		t.Fatalf("schema version = %q", first.SchemaVersion)
	}
	if first.Generator != "delayed-pulse-splitmix64-v1" || first.Profile != "fixture" {
		t.Fatalf("report identity: %+v", first)
	}
	if first.Protocol.Updates != lifThresholdTestUpdates || first.Protocol.HoldoutCount != lifThresholdHoldoutCount || !first.Protocol.CountersDisjoint {
		t.Fatalf("protocol: %+v", first.Protocol)
	}
	if first.Gates.Description != LIFThresholdGateDescription || first.Gates.MinThetaBaseChange <= 0 {
		t.Fatalf("gates: %+v", first.Gates)
	}
	if first.Core.Nodes != 3 || first.Core.Surrogate.Kind != "fast_sigmoid" {
		t.Fatalf("core: %+v", first.Core)
	}
	if len(first.Runs) != len(lifThresholdSeeds) {
		t.Fatalf("report has %d seeds, want %d", len(first.Runs), len(lifThresholdSeeds))
	}
	for _, run := range first.Runs {
		if len(run.Conditions) != 3 {
			t.Fatalf("seed %d has %d conditions", run.Seed, len(run.Conditions))
		}
		names := []string{}
		for _, condition := range run.Conditions {
			names = append(names, condition.Name)
			if len(condition.ThetaBaseBefore) != 3 || len(condition.ThetaBaseAfter) != 3 {
				t.Fatalf("theta_base has the wrong width: %+v", condition)
			}
			for i, before := range condition.ThetaBaseBefore {
				if before <= first.Core.ThetaMin || before >= first.Core.ThetaMax {
					t.Fatalf("theta_base[%d] = %g is outside the declared bounds", i, before)
				}
				if math.IsNaN(condition.ThetaBaseAfter[i]) || condition.ThetaBaseAfter[i] <= first.Core.ThetaMin || condition.ThetaBaseAfter[i] >= first.Core.ThetaMax {
					t.Fatalf("trained theta_base[%d] = %g is outside the declared bounds", i, condition.ThetaBaseAfter[i])
				}
			}
			for _, value := range []float64{condition.HoldoutMSEBefore, condition.HoldoutMSEAfter, condition.SpikeRateBefore, condition.SpikeRateAfter} {
				if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
					t.Fatalf("non-finite report value in %+v", condition)
				}
			}
			if condition.SpikeRateAfter > 1 || condition.SpikeRateBefore > 1 {
				t.Fatalf("spike rate above one: %+v", condition)
			}
		}
		if !reflect.DeepEqual(names, []string{"all", "theta_only", "frozen"}) {
			t.Fatalf("condition names = %v", names)
		}
		before := run.Conditions[0].HoldoutMSEBefore
		for _, condition := range run.Conditions {
			if condition.HoldoutMSEBefore != before {
				t.Fatalf("conditions started from different models: %g vs %g", condition.HoldoutMSEBefore, before)
			}
		}
		frozen := run.Conditions[2]
		if frozen.HoldoutMSEAfter != frozen.HoldoutMSEBefore || frozen.MaxThetaBaseChange != 0 {
			t.Fatalf("frozen control changed: %+v", frozen)
		}
	}
}

func TestRunLIFThresholdZeroBudgetFailsTheThresholdGate(t *testing.T) {
	report, err := RunLIFThreshold(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatal("an empty budget passed the gates")
	}
	for _, run := range report.Runs {
		if run.ThetaBaseMoved {
			t.Fatalf("seed %d moved theta_base without any update", run.Seed)
		}
	}
}

func TestRunLIFThresholdRejectsInvalidArguments(t *testing.T) {
	if _, err := RunLIFThreshold(context.Background(), -1); err == nil {
		t.Fatal("accepted a negative budget")
	}
	if _, err := RunLIFThreshold(context.Background(), LIFThresholdMaxUpdates+1); err == nil {
		t.Fatal("accepted a budget above the documented maximum")
	}
	//lint:ignore SA1012 the nil context path is part of the declared contract.
	if _, err := RunLIFThreshold(nil, 1); err == nil { //nolint:staticcheck
		t.Fatal("accepted a nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunLIFThreshold(ctx, 1); err == nil {
		t.Fatal("accepted a canceled context")
	}
}
