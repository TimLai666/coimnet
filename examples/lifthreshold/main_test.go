package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

const testUpdates = 60

func TestSeedsAndSplitsMatchTheDelayedBenchmark(t *testing.T) {
	if !reflect.DeepEqual(exampleSeeds, experiment.DefaultDelayedConfig().Seeds) {
		t.Fatalf("example seeds %v differ from the delayed benchmark seeds %v", exampleSeeds, experiment.DefaultDelayedConfig().Seeds)
	}
	if trainSeed == holdoutSeed {
		t.Fatal("training and holdout splits share a seed")
	}
	if err := checkDisjointCounters(maxUpdates, holdoutCount); err != nil {
		t.Fatalf("counter streams overlap at the maximum budget: %v", err)
	}
	// The generated counters themselves, not only the seeds, must differ.
	training := map[uint64]bool{}
	for i := 0; i < maxUpdates; i++ {
		training[trainSeed+uint64(i)*counterIncrement] = true
	}
	for i := 0; i < holdoutCount; i++ {
		if training[holdoutSeed+uint64(i)*counterIncrement] {
			t.Fatalf("holdout sample %d reuses a training counter", i)
		}
	}
}

func TestReportStructureAndDeterminism(t *testing.T) {
	ctx := context.Background()
	first, err := buildReport(ctx, testUpdates)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildReport(ctx, testUpdates)
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
	if first.Protocol.Updates != testUpdates || first.Protocol.HoldoutCount != holdoutCount || !first.Protocol.CountersDisjoint {
		t.Fatalf("protocol: %+v", first.Protocol)
	}
	if first.Core.Nodes != 3 || first.Core.Surrogate.Kind != "fast_sigmoid" {
		t.Fatalf("core: %+v", first.Core)
	}
	if len(first.Runs) != len(exampleSeeds) {
		t.Fatalf("report has %d seeds, want %d", len(first.Runs), len(exampleSeeds))
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

func TestZeroBudgetFailsTheThresholdGate(t *testing.T) {
	ctx := context.Background()
	report, err := buildReport(ctx, 0)
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
	var out bytes.Buffer
	if err := execute(ctx, 0, &out); err == nil {
		t.Fatal("execute accepted a failing report")
	}
	var printed exampleReport
	if err := json.Unmarshal(out.Bytes(), &printed); err != nil {
		t.Fatalf("failing run did not print a report: %v\n%s", err, out.String())
	}
	if printed.Passed {
		t.Fatal("printed report claims the gates passed")
	}
}

func TestRunUsageAndArgumentValidation(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &out, &errOut); err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"Usage: go run ./examples/lifthreshold", "Limitations:", "Errors:", "--updates"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("usage does not mention %q: %s", want, out.String())
		}
	}
	for _, args := range [][]string{
		{"--updates", "0"},
		{"--updates", "100001"},
		{"--updates", "-1"},
		{"--unknown"},
		{"extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatalf("accepted invalid arguments %v", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var canceledOut, canceledErr bytes.Buffer
	if err := run(ctx, []string{"--updates", "1"}, &canceledOut, &canceledErr); err == nil {
		t.Fatal("accepted a canceled context")
	}
}
