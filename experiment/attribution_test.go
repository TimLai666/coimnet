package experiment

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

// attributionSweepConfig is the fixed small-scale protocol every run test in
// this file uses: three seeds, four training episodes, an 8-node hidden block
// with 2 recurrent sources, three evaluation maps and a 200-resample paired
// bootstrap over all seven groups.
func attributionSweepConfig() AttributionConfig {
	return AttributionConfig{
		Env:          nav2d.Config{Task: nav2d.TaskAvoidObstacles},
		Seeds:        []uint64{1, 2, 3},
		Episodes:     4,
		Hidden:       8,
		Recurrent:    2,
		LearningRate: 0.05,
		EvalEpisodes: 3,
		Groups:       AttributionGroups(),
		Comparison: PreRegistered{
			Method:    ComparisonPairedBootstrap,
			Interval:  0.95,
			Baseline:  AttributionNormal,
			Resamples: 200,
		},
	}
}

// TestRunAttributionReportsEveryGroupAndSeed runs the seven-group matrix on
// the small sweep config and pins the report skeleton: 21 runs in group-then-
// seed order, none failed, the same nonzero update count for every group of
// one seed (one shared schedule), Model.Group matching the run, twelve
// comparisons (six non-baseline groups x two unseen metrics) with three
// paired differences and Lower <= Mean <= Upper each, the fixed conclusion
// sentence, and it logs every group's unseen success rate and expert
// agreement per seed.
func TestRunAttributionReportsEveryGroupAndSeed(t *testing.T) {
	report, err := RunAttribution(context.Background(), attributionSweepConfig())
	if err != nil {
		t.Fatalf("RunAttribution: %v", err)
	}
	groups := AttributionGroups()
	wantRuns := len(groups) * 3
	if len(report.Runs) != wantRuns {
		t.Fatalf("len(Runs) = %d, want %d", len(report.Runs), wantRuns)
	}
	if report.Conclusion != AttributionConclusion {
		t.Errorf("Conclusion = %q, want the fixed sentence", report.Conclusion)
	}
	updatesBySeed := map[uint64]uint64{}
	seedSeen := map[uint64]bool{}
	for i, run := range report.Runs {
		wantGroup := groups[i/3]
		wantSeed := uint64(i%3 + 1)
		if run.Group != wantGroup || run.Seed != wantSeed {
			t.Errorf("Runs[%d] = (%q, %d), want (%q, %d)", i, run.Group, run.Seed, wantGroup, wantSeed)
		}
		if run.Failed {
			t.Errorf("%s seed %d failed: %s", run.Group, run.Seed, run.Error)
			continue
		}
		if run.Model.Group != run.Group {
			t.Errorf("%s seed %d: Model.Group = %q", run.Group, run.Seed, run.Model.Group)
		}
		if run.Updates == 0 {
			t.Errorf("%s seed %d: Updates = 0", run.Group, run.Seed)
		}
		if seedSeen[run.Seed] {
			if prev, ok := updatesBySeed[run.Seed]; ok && run.Updates != prev {
				t.Errorf("seed %d: %s Updates %d != earlier group %d", run.Seed, run.Group, run.Updates, prev)
			}
		}
		updatesBySeed[run.Seed] = run.Updates
		seedSeen[run.Seed] = true
		t.Logf("%s seed %d: unseen success %.3f agreement %.3f updates %d",
			run.Group, run.Seed, run.Unseen.SuccessRate, run.Unseen.ExpertAgreement, run.Updates)
	}

	if len(report.Comparisons) != 12 {
		t.Fatalf("len(Comparisons) = %d, want 12", len(report.Comparisons))
	}
	metricOrder := []string{"unseen_success_rate", "unseen_expert_agreement"}
	ci := 0
	for _, group := range groups {
		if group == AttributionNormal {
			continue
		}
		for _, metric := range metricOrder {
			got := report.Comparisons[ci]
			if got.Group != group || got.Metric != metric {
				t.Errorf("Comparisons[%d] = (%q, %q), want (%q, %q)", ci, got.Group, got.Metric, group, metric)
			}
			if len(got.Differences) != 3 {
				t.Errorf("%s %s: %d differences, want 3", group, metric, len(got.Differences))
			}
			if !(got.Lower <= got.Mean && got.Mean <= got.Upper) {
				t.Errorf("%s %s: Lower %v <= Mean %v <= Upper %v violated", group, metric, got.Lower, got.Mean, got.Upper)
			}
			t.Logf("%s %s: mean %+.4f CI [%.4f, %.4f] dropped %d diffs %v",
				group, metric, got.Mean, got.Lower, got.Upper, got.Dropped, got.Differences)
			ci++
		}
	}
}

// TestAttributionCoreOnlyChecks builds an untrained core_only policy and
// drives the troubleshooting checks directly: the identity periphery keeps
// every readout node reachable from the inputs, inference resets bit-identical
// across two probe passes, act only ever sees the observation prefix, and an
// untouched core has update norm zero. A second trainer built in this test
// with every core edge (and the matching weights) removed must fail the
// reachability check.
func TestAttributionCoreOnlyChecks(t *testing.T) {
	ctx := context.Background()
	env := nav2d.Config{Task: nav2d.TaskAvoidObstacles}
	p, _, err := newAttributionPolicy(AttributionCoreOnly, 1, 40, 8, 2, 0.05)
	if err != nil {
		t.Fatalf("newAttributionPolicy(core_only): %v", err)
	}
	snap := p.trainer.Snapshot()
	checks, err := coreOnlyChecks(ctx, p, env, snap.Parameters, snap.Parameters)
	if err != nil {
		t.Fatalf("coreOnlyChecks(untrained): %v", err)
	}
	if !checks.InputReachable {
		t.Error("InputReachable = false on the identity-periphery core_only policy, want true")
	}
	if !checks.StateReset {
		t.Error("StateReset = false on two identical probe passes, want true")
	}
	if !checks.ObservationOnly {
		t.Error("ObservationOnly = false, act must take only (*nav2dPolicy, context.Context, [][]float64)")
	}
	if checks.CoreUpdateNorm != 0 {
		t.Errorf("CoreUpdateNorm = %v on an untrained core, want 0", checks.CoreUpdateNorm)
	}

	brokenCfg := snap.Config
	brokenCfg.Dynamics.Sources = nil
	brokenCfg.Dynamics.Targets = nil
	brokenCfg.Dynamics.Delays = nil
	brokenParams := snap.Parameters
	brokenParams.Core.Weights = nil
	broken, err := learning.NewTrainer(brokenCfg, brokenParams, snap.Options)
	if err != nil {
		t.Fatalf("NewTrainer without core edges: %v", err)
	}
	severed := &nav2dPolicy{kind: Nav2DRecurrent, trainer: broken, settle: 3}
	severedChecks, err := coreOnlyChecks(ctx, severed, env, brokenParams, brokenParams)
	if err != nil {
		t.Fatalf("coreOnlyChecks(severed): %v", err)
	}
	if severedChecks.InputReachable {
		t.Error("InputReachable = true with every core edge removed, want false")
	}
}

// TestAttributionNeverEnlargesThePeriphery proves two halves of the contract:
// AttributionConfig has no field whose name suggests a readout, decoder,
// periphery or capacity knob, and after a full run every core_only report row
// still carries exactly the hand-computed 2280 parameters of the fixed
// identity periphery (E = 320 + 16 + 32 + 160 = 528 weights, 52 bias, 52
// log_tau, 40x40 encoder, 12x4 readout), independent of the training result.
func TestAttributionNeverEnlargesThePeriphery(t *testing.T) {
	typ := reflect.TypeOf(AttributionConfig{})
	forbidden := []string{"readout", "decoder", "periphery", "capacity"}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		for _, bad := range forbidden {
			if strings.Contains(name, bad) {
				t.Errorf("AttributionConfig field %q contains %q", typ.Field(i).Name, bad)
			}
		}
	}
	report, err := RunAttribution(context.Background(), attributionSweepConfig())
	if err != nil {
		t.Fatalf("RunAttribution: %v", err)
	}
	seen := 0
	for _, run := range report.Runs {
		if run.Group != AttributionCoreOnly {
			continue
		}
		seen++
		if run.Failed {
			t.Errorf("core_only seed %d failed: %s", run.Seed, run.Error)
			continue
		}
		if run.Model.Parameters != 2280 {
			t.Errorf("core_only seed %d: Parameters = %d, want 2280", run.Seed, run.Model.Parameters)
		}
	}
	if seen != 3 {
		t.Fatalf("saw %d core_only runs, want 3", seen)
	}
}

// TestRunAttributionIsDeterministic runs the sweep twice on one config and
// requires bit-identical runs and comparisons.
func TestRunAttributionIsDeterministic(t *testing.T) {
	c := attributionSweepConfig()
	a, err := RunAttribution(context.Background(), c)
	if err != nil {
		t.Fatalf("first RunAttribution: %v", err)
	}
	b, err := RunAttribution(context.Background(), c)
	if err != nil {
		t.Fatalf("second RunAttribution: %v", err)
	}
	if !reflect.DeepEqual(a.Runs, b.Runs) {
		t.Error("two runs of the same config produced different Runs")
	}
	if !reflect.DeepEqual(a.Comparisons, b.Comparisons) {
		t.Error("two runs of the same config produced different Comparisons")
	}
}

// TestAttributionConfigValidate walks the declared rejection table: too few
// or duplicate seeds, out-of-range budgets and sizes, a group list missing
// normal or carrying unknown or duplicate entries, a comparison that is not
// the pre-registered paired bootstrap against normal, and an unknown task.
func TestAttributionConfigValidate(t *testing.T) {
	if err := attributionSweepConfig().Validate(); err != nil {
		t.Fatalf("sweep config must validate: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*AttributionConfig)
	}{
		{"seeds two", func(c *AttributionConfig) { c.Seeds = []uint64{1, 2} }},
		{"duplicate seed", func(c *AttributionConfig) { c.Seeds = []uint64{1, 2, 1} }},
		{"episodes zero", func(c *AttributionConfig) { c.Episodes = 0 }},
		{"hidden three", func(c *AttributionConfig) { c.Hidden = 3 }},
		{"recurrent zero", func(c *AttributionConfig) { c.Recurrent = 0 }},
		{"recurrent above hidden", func(c *AttributionConfig) { c.Recurrent = c.Hidden + 1 }},
		{"groups missing normal", func(c *AttributionConfig) {
			c.Groups = []string{AttributionFrozenCore, AttributionCoreOnly, AttributionGenericMatched, AttributionRewired, AttributionAblatedRetrained, AttributionCapacityMatchedModulator}
		}},
		{"unknown group", func(c *AttributionConfig) { c.Groups = append(c.Groups, "dream_team") }},
		{"duplicate group", func(c *AttributionConfig) { c.Groups = append(c.Groups, AttributionNormal) }},
		{"method not paired_bootstrap", func(c *AttributionConfig) { c.Comparison.Method = "t_test" }},
		{"baseline not normal", func(c *AttributionConfig) { c.Comparison.Baseline = BaselineIndependent }},
		{"interval one", func(c *AttributionConfig) { c.Comparison.Interval = 1 }},
		{"resamples 99", func(c *AttributionConfig) { c.Comparison.Resamples = 99 }},
		{"unknown task", func(c *AttributionConfig) { c.Env.Task = "fly_to_the_moon" }},
	}
	for _, tc := range cases {
		c := attributionSweepConfig()
		tc.mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate returned nil, want an error", tc.name)
		}
	}
}

// TestRunAttributionHonoursCancellation requires an already-cancelled context
// to abort with the context error before anything runs.
func TestRunAttributionHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := RunAttribution(ctx, attributionSweepConfig())
	if err == nil {
		t.Fatal("RunAttribution on a cancelled context returned nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if len(report.Runs) != 0 {
		t.Errorf("cancelled run produced %d runs, want none", len(report.Runs))
	}
}
