package experiment

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/nav2d"
)

// nav2dSuiteBaseConfig is the short shared protocol the suite tests run
// under, so every test starts from one validated configuration.
func nav2dSuiteBaseConfig() Nav2DConfig {
	return Nav2DConfig{
		Env:          nav2d.Config{Task: nav2d.TaskAvoidObstacles},
		Seeds:        []uint64{1, 2},
		Episodes:     8,
		Hidden:       8,
		Recurrent:    2,
		LearningRate: 0.05,
		EvalEpisodes: 3,
		Policies:     []string{Nav2DRecurrent, Nav2DFeedforward, Nav2DRewired, Nav2DRandom},
	}
}

// TestNav2DRewireKeepsDegreesAndDirection runs nav2dRewire on the default
// recurrent edge list and checks the checklist the ticket requires: degrees
// kept, direction kept, readout reachable, no duplicates, self loops never
// growing, at least one accepted swap, non hidden-to-hidden targets untouched
// position by position, the caller's arrays unmodified, and full
// determinism for one seed.
func TestNav2DRewireKeepsDegreesAndDirection(t *testing.T) {
	const inputs, hidden, recurrent = 40, 16, 4
	sources, targets := nav2dEdges(1, inputs, hidden, recurrent, true)
	originalTargets := append([]int(nil), targets...)
	originalSources := append([]int(nil), sources...)
	rewired, report, err := nav2dRewire(sources, targets, inputs, hidden, 1)
	if err != nil {
		t.Fatalf("nav2dRewire: %v", err)
	}
	if !report.DegreesKept {
		t.Error("DegreesKept is false")
	}
	if !report.DirectionKept {
		t.Error("DirectionKept is false")
	}
	if !report.ReadoutReachable {
		t.Error("ReadoutReachable is false")
	}
	if report.Duplicates != 0 {
		t.Errorf("Duplicates %d, want 0", report.Duplicates)
	}
	if report.SelfLoopsAfter > report.SelfLoopsBefore {
		t.Errorf("SelfLoopsAfter %d > SelfLoopsBefore %d", report.SelfLoopsAfter, report.SelfLoopsBefore)
	}
	if report.Accepted <= 0 {
		t.Errorf("Accepted %d, want > 0", report.Accepted)
	}
	if report.Edges != hidden*recurrent {
		t.Errorf("Edges %d, want %d hidden-to-hidden edges", report.Edges, hidden*recurrent)
	}
	if report.Attempts != 10*report.Edges {
		t.Errorf("Attempts %d, want 10 x %d", report.Attempts, report.Edges)
	}
	hiddenLast := inputs + hidden
	for i := range originalTargets {
		inBlock := sources[i] >= inputs && sources[i] < hiddenLast &&
			originalTargets[i] >= inputs && originalTargets[i] < hiddenLast
		if !inBlock && rewired[i] != originalTargets[i] {
			t.Errorf("edge %d (%d->%d) target moved to %d but is not hidden-to-hidden",
				i, sources[i], originalTargets[i], rewired[i])
		}
	}
	if !reflect.DeepEqual(targets, originalTargets) {
		t.Error("nav2dRewire mutated the caller's targets slice")
	}
	if !reflect.DeepEqual(sources, originalSources) {
		t.Error("nav2dRewire mutated the caller's sources slice")
	}
	again, reportAgain, err := nav2dRewire(sources, targets, inputs, hidden, 1)
	if err != nil {
		t.Fatalf("nav2dRewire second call: %v", err)
	}
	if !reflect.DeepEqual(rewired, again) {
		t.Error("the same seed produced different rewired targets")
	}
	if report != reportAgain {
		t.Errorf("the same seed produced different reports: %+v vs %+v", report, reportAgain)
	}
	t.Logf("rewire: edges=%d attempts=%d accepted=%d self_loops %d->%d",
		report.Edges, report.Attempts, report.Accepted, report.SelfLoopsBefore, report.SelfLoopsAfter)
}

// TestNav2DDegreesMatchDetectsABrokenBlock breaks one hidden-to-hidden edge
// by moving its target to another hidden node, which changes the in-degrees
// of two hidden nodes, and requires nav2dDegreesMatch to notice; the intact
// lists and nav2dRewire's output must still match.
func TestNav2DDegreesMatchDetectsABrokenBlock(t *testing.T) {
	const inputs, hidden, recurrent = 40, 16, 4
	sources, targets := nav2dEdges(1, inputs, hidden, recurrent, true)
	hiddenLast := inputs + hidden
	brokenIdx := -1
	for i := range targets {
		if sources[i] >= inputs && sources[i] < hiddenLast &&
			targets[i] >= inputs && targets[i] < hiddenLast {
			brokenIdx = i
			break
		}
	}
	if brokenIdx < 0 {
		t.Fatal("no hidden-to-hidden edge found")
	}
	broken := append([]int(nil), targets...)
	broken[brokenIdx] = inputs + (targets[brokenIdx]-inputs+1)%hidden
	if broken[brokenIdx] == targets[brokenIdx] {
		t.Fatalf("broken target %d equals original", broken[brokenIdx])
	}
	if nav2dDegreesMatch(sources, targets, broken, inputs, hidden) {
		t.Errorf("broken block (edge %d target %d->%d) reported degrees kept",
			brokenIdx, targets[brokenIdx], broken[brokenIdx])
	}
	if !nav2dDegreesMatch(sources, targets, targets, inputs, hidden) {
		t.Error("intact lists reported degrees broken")
	}
	rewired, report, err := nav2dRewire(sources, targets, inputs, hidden, 1)
	if err != nil {
		t.Fatalf("nav2dRewire: %v", err)
	}
	if !nav2dDegreesMatch(sources, targets, rewired, inputs, hidden) {
		t.Error("nav2dRewire output reported degrees broken")
	}
	if !report.DegreesKept {
		t.Error("nav2dRewire report DegreesKept is false")
	}
}

// TestNav2DInputWidthMatchesObservation pins nav2dInputWidth to the actual
// observation vector: 40 for the default config (ViewDepth 3) and
// 5*5*3+13 for ViewDepth 5.
func TestNav2DInputWidthMatchesObservation(t *testing.T) {
	got, err := nav2dInputWidth(nav2d.Config{})
	if err != nil {
		t.Fatalf("nav2dInputWidth default: %v", err)
	}
	if got != 40 {
		t.Errorf("default width %d, want 40", got)
	}
	got, err = nav2dInputWidth(nav2d.Config{ViewDepth: 5})
	if err != nil {
		t.Fatalf("nav2dInputWidth ViewDepth 5: %v", err)
	}
	if want := 5*5*3 + 13; got != want {
		t.Errorf("ViewDepth 5 width %d, want %d", got, want)
	}
}

// TestNav2DRewiredMatchesRecurrentBudget proves the rewired control keeps the
// recurrent policy's budget: same learnable parameter count, same edge count,
// and only the rewired policy carries a rewire report.
func TestNav2DRewiredMatchesRecurrentBudget(t *testing.T) {
	recurrent, err := newNav2DPolicy(Nav2DRecurrent, 1, 40, 16, 4, 0.05)
	if err != nil {
		t.Fatalf("newNav2DPolicy recurrent: %v", err)
	}
	rewired, err := newNav2DPolicy(Nav2DRewired, 1, 40, 16, 4, 0.05)
	if err != nil {
		t.Fatalf("newNav2DPolicy rewired: %v", err)
	}
	recParams, recEdges := recurrent.parameters()
	rewParams, rewEdges := rewired.parameters()
	if recParams != rewParams || recEdges != rewEdges {
		t.Errorf("recurrent %d params / %d edges, rewired %d params / %d edges",
			recParams, recEdges, rewParams, rewEdges)
	}
	if recurrent.rewire != nil {
		t.Error("recurrent policy carries a rewire report")
	}
	if rewired.rewire == nil {
		t.Fatal("rewired policy carries no rewire report")
	}
	if !rewired.rewire.DegreesKept || !rewired.rewire.DirectionKept || !rewired.rewire.ReadoutReachable {
		t.Errorf("rewired report checklist failed: %+v", rewired.rewire)
	}
	t.Logf("budget: params=%d edges=%d rewire accepted=%d", rewParams, rewEdges, rewired.rewire.Accepted)
}

// TestRunNav2DReportsEveryPolicyAndSeed pins the single-task report shape:
// four policies x two seeds in policy-then-seed order, no failed run, the
// success/timeout complement on every metric set, the random policy without
// parameters, a rewire report only on the rewired policy and a non-empty
// config hash.
func TestRunNav2DReportsEveryPolicyAndSeed(t *testing.T) {
	c := nav2dSuiteBaseConfig()
	report, err := RunNav2D(context.Background(), c)
	if err != nil {
		t.Fatalf("RunNav2D: %v", err)
	}
	if report.SchemaVersion != Nav2DSchemaVersion {
		t.Errorf("SchemaVersion %q, want %q", report.SchemaVersion, Nav2DSchemaVersion)
	}
	if report.Task != nav2d.TaskAvoidObstacles {
		t.Errorf("Task %q, want %q", report.Task, nav2d.TaskAvoidObstacles)
	}
	if report.ConfigHash == "" {
		t.Error("ConfigHash is empty")
	}
	if len(report.Assumptions) != 3 {
		t.Errorf("Assumptions has %d sentences, want 3", len(report.Assumptions))
	}
	wantRuns := len(c.Policies) * len(c.Seeds)
	if len(report.Runs) != wantRuns {
		t.Fatalf("report has %d runs, want %d", len(report.Runs), wantRuns)
	}
	idx := 0
	for _, kind := range c.Policies {
		for _, seed := range c.Seeds {
			run := report.Runs[idx]
			if run.Policy != kind || run.Seed != seed {
				t.Errorf("run %d is %s/%d, want %s/%d", idx, run.Policy, run.Seed, kind, seed)
			}
			if run.Failed {
				t.Errorf("%s seed %d failed: %s", run.Policy, run.Seed, run.Error)
			}
			for _, m := range []struct {
				label string
				m     Nav2DMetrics
			}{
				{"before", run.Before},
				{"seen", run.Seen},
				{"unseen", run.Unseen},
			} {
				if math.Abs(m.m.SuccessRate+m.m.TimeoutRate-1) >= 1e-12 {
					t.Errorf("%s seed %d %s: success %v + timeout %v != 1",
						run.Policy, run.Seed, m.label, m.m.SuccessRate, m.m.TimeoutRate)
				}
			}
			idx++
		}
	}
	for _, run := range report.Runs {
		switch run.Policy {
		case Nav2DRandom:
			if run.Parameters != 0 {
				t.Errorf("random policy reports %d parameters, want 0", run.Parameters)
			}
		case Nav2DRewired:
			if run.Rewire == nil {
				t.Errorf("seed %d rewired run has no rewire report", run.Seed)
			}
		default:
			if run.Rewire != nil {
				t.Errorf("%s run carries a rewire report", run.Policy)
			}
		}
	}
}

// TestRunNav2DSuiteCoversFourTasks runs the four tasks with one shared
// config under two policies and checks the task order, the per-task run
// count, and that every run finished; it logs each task x policy's unseen
// success rate and expert agreement.
func TestRunNav2DSuiteCoversFourTasks(t *testing.T) {
	c := nav2dSuiteBaseConfig()
	c.Policies = []string{Nav2DRecurrent, Nav2DRandom}
	c.Seeds = []uint64{1}
	report, err := RunNav2DSuite(context.Background(), c)
	if err != nil {
		t.Fatalf("RunNav2DSuite: %v", err)
	}
	if report.SchemaVersion != Nav2DSuiteSchemaVersion {
		t.Errorf("SchemaVersion %q, want %q", report.SchemaVersion, Nav2DSuiteSchemaVersion)
	}
	wantTasks := []string{
		nav2d.TaskRememberGoal,
		nav2d.TaskAvoidObstacles,
		nav2d.TaskAdaptAfterChange,
		nav2d.TaskLanguageGoal,
	}
	if len(report.Tasks) != len(wantTasks) {
		t.Fatalf("suite has %d tasks, want %d", len(report.Tasks), len(wantTasks))
	}
	for i, want := range wantTasks {
		task := report.Tasks[i]
		if task.Task != want {
			t.Errorf("task %d is %q, want %q", i, task.Task, want)
		}
		if task.Config.Env.Task != want {
			t.Errorf("task %d config carries Env.Task %q, want %q", i, task.Config.Env.Task, want)
		}
		if len(task.Runs) != len(c.Policies) {
			t.Errorf("task %q has %d runs, want %d", task.Task, len(task.Runs), len(c.Policies))
		}
		for j, run := range task.Runs {
			if run.Failed {
				t.Errorf("%s run %d failed: %s", task.Task, j, run.Error)
			}
			t.Logf("%s %s: unseen success_rate=%v expert_agreement=%v",
				task.Task, run.Policy, run.Unseen.SuccessRate, run.Unseen.ExpertAgreement)
		}
	}
}

// TestNav2DFourTasksFourPoliciesTable runs the whole suite once with every
// policy and prints the four-task x four-policy unseen success rate and
// expert agreement table the handoff asks for. It only asserts that every
// run finished and that the shape is right.
func TestNav2DFourTasksFourPoliciesTable(t *testing.T) {
	c := nav2dSuiteBaseConfig()
	c.Seeds = []uint64{1}
	report, err := RunNav2DSuite(context.Background(), c)
	if err != nil {
		t.Fatalf("RunNav2DSuite: %v", err)
	}
	if len(report.Tasks) != 4 {
		t.Fatalf("suite has %d tasks, want 4", len(report.Tasks))
	}
	for _, task := range report.Tasks {
		if len(task.Runs) != len(c.Policies) {
			t.Fatalf("task %q has %d runs, want %d", task.Task, len(task.Runs), len(c.Policies))
		}
		for _, run := range task.Runs {
			if run.Failed {
				t.Errorf("%s %s failed: %s", task.Task, run.Policy, run.Error)
			}
			t.Logf("%s\t%s\tunseen success_rate=%v\texpert_agreement=%v",
				task.Task, run.Policy, run.Unseen.SuccessRate, run.Unseen.ExpertAgreement)
		}
	}
}

// TestRunNav2DIsDeterministic runs the same protocol twice and requires the
// two run lists to be identical, rewiring included.
func TestRunNav2DIsDeterministic(t *testing.T) {
	c := nav2dSuiteBaseConfig()
	c.Policies = []string{Nav2DRecurrent, Nav2DRewired}
	c.Seeds = []uint64{1}
	c.Episodes = 4
	a, err := RunNav2D(context.Background(), c)
	if err != nil {
		t.Fatalf("first RunNav2D: %v", err)
	}
	b, err := RunNav2D(context.Background(), c)
	if err != nil {
		t.Fatalf("second RunNav2D: %v", err)
	}
	if !reflect.DeepEqual(a.Runs, b.Runs) {
		t.Errorf("two identical runs differ:\nfirst  %+v\nsecond %+v", a.Runs, b.Runs)
	}
}

// TestNav2DConfigValidate walks one invalid field per case and requires each
// mutation of a valid protocol to be rejected, while the base config passes.
func TestNav2DConfigValidate(t *testing.T) {
	base := nav2dSuiteBaseConfig()
	base.Seeds = []uint64{1}
	base.Policies = []string{Nav2DRecurrent}
	base.EvalEpisodes = 2
	if err := base.Validate(); err != nil {
		t.Fatalf("base config must validate: %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*Nav2DConfig)
	}{
		{"empty seeds", func(c *Nav2DConfig) { c.Seeds = nil }},
		{"duplicate seed", func(c *Nav2DConfig) { c.Seeds = []uint64{1, 1} }},
		{"episodes zero", func(c *Nav2DConfig) { c.Episodes = 0 }},
		{"hidden three", func(c *Nav2DConfig) { c.Hidden = 3 }},
		{"recurrent zero", func(c *Nav2DConfig) { c.Recurrent = 0 }},
		{"recurrent above hidden", func(c *Nav2DConfig) { c.Recurrent = 9 }},
		{"unknown policy", func(c *Nav2DConfig) { c.Policies = []string{"teleport"} }},
		{"duplicate policy", func(c *Nav2DConfig) { c.Policies = []string{Nav2DRecurrent, Nav2DRecurrent} }},
		{"unknown task", func(c *Nav2DConfig) { c.Env.Task = "teleport" }},
	} {
		c := base
		c.Env = base.Env
		tc.edit(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate accepted an invalid config", tc.name)
		}
	}
}

// TestRunNav2DHonoursCancellation requires a cancelled context to abort the
// run with context.Canceled instead of returning a report.
func TestRunNav2DHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := nav2dSuiteBaseConfig()
	c.Seeds = []uint64{1}
	c.Policies = []string{Nav2DRecurrent}
	c.Episodes = 1
	report, err := RunNav2D(ctx, c)
	if err == nil {
		t.Fatalf("cancelled RunNav2D returned a report with %d runs and no error", len(report.Runs))
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err %v, want context.Canceled", err)
	}
}
