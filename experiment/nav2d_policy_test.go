package experiment

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

const (
	nav2dTestInputs       = 40 // Observation.Vector width for the default 9x9 map
	nav2dTestHidden       = 16
	nav2dTestRecurrent    = 4
	nav2dTestLearningRate = 0.05
)

// TestNav2DEdgesShape pins the fixed edge order of nav2dEdges: all
// input->hidden edges, then (when recurrent) recurrent hidden->hidden edges
// per target, each from its own PCG(seed, 5) stream's r.Perm(hidden) (one Perm
// call per target, in target order), then hidden->readout and finally the
// input->readout direct path. It also checks each hidden target's recurrent
// sources are recurrent distinct nodes, that the 16 targets do not all share
// one source set, and that the edges are seed-deterministic.
func TestNav2DEdgesShape(t *testing.T) {
	sources, targets := nav2dEdges(7, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, true)
	if len(sources) != 928 || len(targets) != 928 {
		t.Fatalf("recurrent policy has %d edges, want 928", len(sources))
	}
	seen := map[[2]int]bool{}
	hiddenIn := make([]int, nav2dTestHidden)
	for i := range sources {
		from, to := sources[i], targets[i]
		key := [2]int{from, to}
		if seen[key] {
			t.Fatalf("duplicate edge %v", key)
		}
		seen[key] = true
		if to >= nav2dTestInputs && to < nav2dTestInputs+nav2dTestHidden &&
			from >= nav2dTestInputs && from < nav2dTestInputs+nav2dTestHidden {
			hiddenIn[to-nav2dTestInputs]++
		}
	}
	for h, n := range hiddenIn {
		if n != nav2dTestRecurrent {
			t.Errorf("hidden node %d has %d hidden in-edges, want %d", h, n, nav2dTestRecurrent)
		}
	}
	recurrentSources := make([][]int, nav2dTestHidden)
	for i := range sources {
		from, to := sources[i], targets[i]
		if to >= nav2dTestInputs && to < nav2dTestInputs+nav2dTestHidden &&
			from >= nav2dTestInputs && from < nav2dTestInputs+nav2dTestHidden {
			h := to - nav2dTestInputs
			recurrentSources[h] = append(recurrentSources[h], from-nav2dTestInputs)
		}
	}
	for h, src := range recurrentSources {
		if len(src) != nav2dTestRecurrent {
			t.Errorf("hidden target %d has %d recurrent sources, want %d", h, len(src), nav2dTestRecurrent)
			continue
		}
		for i := 0; i < len(src); i++ {
			for j := i + 1; j < len(src); j++ {
				if src[i] == src[j] {
					t.Errorf("hidden target %d recurrent sources repeat %d", h, src[i])
				}
			}
		}
	}
	allSame := true
	for h := 1; h < nav2dTestHidden; h++ {
		if !reflect.DeepEqual(recurrentSources[h], recurrentSources[0]) {
			allSame = false
			break
		}
	}
	if allSame {
		t.Errorf("all %d hidden targets share the same recurrent sources %v", nav2dTestHidden, recurrentSources[0])
	}
	ffSources, ffTargets := nav2dEdges(7, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, false)
	if len(ffSources) != 864 {
		t.Fatalf("feedforward policy has %d edges, want 864", len(ffSources))
	}
	for i := range ffSources {
		if ffSources[i] >= nav2dTestInputs && ffSources[i] < nav2dTestInputs+nav2dTestHidden &&
			ffTargets[i] >= nav2dTestInputs && ffTargets[i] < nav2dTestInputs+nav2dTestHidden {
			t.Fatalf("feedforward policy has a hidden-to-hidden edge %v->%v", ffSources[i], ffTargets[i])
		}
	}
	s2, t2 := nav2dEdges(7, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, true)
	if !reflect.DeepEqual(sources, s2) || !reflect.DeepEqual(targets, t2) {
		t.Fatal("the same parameters produced different edge lists")
	}
	seedASources, seedATargets := nav2dEdges(1, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, true)
	a2, bt2 := nav2dEdges(1, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, true)
	if !reflect.DeepEqual(seedASources, a2) || !reflect.DeepEqual(seedATargets, bt2) {
		t.Fatal("the same parameters produced different edge lists")
	}
	seedBSources, seedBTargets := nav2dEdges(2, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, true)
	if reflect.DeepEqual(seedBSources, seedASources) && reflect.DeepEqual(seedBTargets, seedATargets) {
		t.Fatal("seed 2 produced the same edge list as seed 1")
	}
}

// TestNav2DMapSetsAreDisjoint checks that training maps are all odd, unseen
// maps are all even and the two sets never overlap, so an evaluation can never
// touch a map the agent trained on.
func TestNav2DMapSetsAreDisjoint(t *testing.T) {
	train := map[uint64]bool{}
	for _, seed := range []uint64{1, 2, 3} {
		for e := 0; e < 200; e++ {
			m := nav2dTrainMap(seed, e)
			if m%2 == 0 {
				t.Errorf("training map %d from seed %d episode %d is even", m, seed, e)
			}
			train[m] = true
		}
	}
	for i := 0; i < 200; i++ {
		m := nav2dUnseenMap(i)
		if m%2 == 1 {
			t.Errorf("unseen map %d is odd", m)
		}
		if train[m] {
			t.Errorf("unseen map %d overlaps a training map", m)
		}
	}
}

// TestNav2DRandomMetricsAreConsistent checks the metrics invariants on the
// uniform random policy over ten unseen maps.
func TestNav2DRandomMetricsAreConsistent(t *testing.T) {
	p, err := newNav2DPolicy(Nav2DRandom, 1, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy: %v", err)
	}
	maps := make([]uint64, 10)
	for i := range maps {
		maps[i] = nav2dUnseenMap(i)
	}
	m, err := evaluateNav2D(context.Background(), p, nav2d.Config{Task: nav2d.TaskAvoidObstacles}, maps)
	if err != nil {
		t.Fatalf("evaluateNav2D: %v", err)
	}
	if m.Episodes != 10 {
		t.Errorf("episodes %d, want 10", m.Episodes)
	}
	if math.Abs(m.SuccessRate+m.TimeoutRate-1) >= 1e-12 {
		t.Errorf("success %v + timeout %v != 1", m.SuccessRate, m.TimeoutRate)
	}
	if m.MeanSteps > 60 {
		t.Errorf("mean steps %v exceeds the 60-step time limit", m.MeanSteps)
	}
	if m.ExpertAgreement < 0 || m.ExpertAgreement > 1 {
		t.Errorf("expert agreement %v outside [0, 1]", m.ExpertAgreement)
	}
	t.Logf("random: success %v timeout %v mean steps %v collisions %v return %v agreement %v",
		m.SuccessRate, m.TimeoutRate, m.MeanSteps, m.MeanCollisions, m.MeanReturn, m.ExpertAgreement)
}

// TestNav2DActReadsOnlyTheObservationPrefix proves the decision point cannot
// reach the expert from its signature alone and that both learned policies are
// deterministic on a fixed prefix.
func TestNav2DActReadsOnlyTheObservationPrefix(t *testing.T) {
	fn := reflect.TypeOf((*nav2dPolicy).act)
	env := reflect.TypeOf((*nav2d.Env)(nil))
	if fn.NumIn() != 3 {
		t.Fatalf("act takes %d parameters, want 3", fn.NumIn())
	}
	for i := 0; i < fn.NumIn(); i++ {
		if fn.In(i) == env {
			t.Fatalf("act parameter %d is %v, so the evaluator could reach the expert", i, env)
		}
	}
	e, err := nav2d.New(nav2d.Config{Task: nav2d.TaskAvoidObstacles})
	if err != nil {
		t.Fatalf("nav2d.New: %v", err)
	}
	obs, _ := e.Reset(nav2dUnseenMap(0))
	prefix := [][]float64{obs.Vector()}
	next, _, _, _, err := e.Step(nav2d.ActionStay)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	prefix = append(prefix, next.Vector())
	for _, kind := range []string{Nav2DRecurrent, Nav2DFeedforward} {
		p, err := newNav2DPolicy(kind, 1, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
		if err != nil {
			t.Fatalf("%s newNav2DPolicy: %v", kind, err)
		}
		first, err := p.act(context.Background(), prefix)
		if err != nil {
			t.Fatalf("%s first act: %v", kind, err)
		}
		second, err := p.act(context.Background(), prefix)
		if err != nil {
			t.Fatalf("%s second act: %v", kind, err)
		}
		if first != second {
			t.Errorf("%s returned %d then %d on the same prefix", kind, first, second)
		}
	}
}

// TestNav2DImitationRaisesSeenAgreement is the imitation claim for the 2D
// environment: a recurrent policy trained under the trainNav2D schedule (expert
// demonstrations plus DAgger rollouts of its own actions) matches the expert
// more often on the training maps than it did before training, and the
// feedforward policy spends exactly one update per demonstration step.
func TestNav2DImitationRaisesSeenAgreement(t *testing.T) {
	ctx := context.Background()
	c := nav2d.Config{Task: nav2d.TaskAvoidObstacles}
	p, err := newNav2DPolicy(Nav2DRecurrent, 1, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy: %v", err)
	}
	evalMaps := make([]uint64, 10)
	for i := range evalMaps {
		evalMaps[i] = nav2dTrainMap(1, i)
	}
	before, err := evaluateNav2D(ctx, p, c, evalMaps)
	if err != nil {
		t.Fatalf("before evaluateNav2D: %v", err)
	}
	beforeDist := nav2dCountActions(t, ctx, p, c, evalMaps)
	if _, err := trainNav2D(ctx, p, c, 1, 40); err != nil {
		t.Fatalf("trainNav2D: %v", err)
	}
	after, err := evaluateNav2D(ctx, p, c, evalMaps)
	if err != nil {
		t.Fatalf("after evaluateNav2D: %v", err)
	}
	afterDist := nav2dCountActions(t, ctx, p, c, evalMaps)
	t.Logf("recurrent: before agreement %v success %v, after agreement %v success %v",
		before.ExpertAgreement, before.SuccessRate, after.ExpertAgreement, after.SuccessRate)
	t.Logf("recurrent: action distribution (forward left right stay) before %v after %v", beforeDist, afterDist)
	if after.ExpertAgreement <= before.ExpertAgreement {
		t.Errorf("recurrent agreement %v did not improve on %v", after.ExpertAgreement, before.ExpertAgreement)
	}
	var afterSteps int
	for _, n := range afterDist {
		afterSteps += n
	}
	var common int
	for _, n := range afterDist {
		if float64(n) >= 0.05*float64(afterSteps) {
			common++
		}
	}
	if common < 2 {
		t.Errorf("recurrent collapsed: only %d action(s) cover >= 5%% of %d steps, distribution %v", common, afterSteps, afterDist)
	}
	var replayMatch, replaySteps int
	for i := 0; i < 10; i++ {
		demoObs, demoActions, err := nav2dExpertEpisode(c, nav2dTrainMap(1, i))
		if err != nil {
			t.Fatalf("demonstration map %d: %v", i, err)
		}
		prefix := [][]float64{demoObs[0]}
		for j := range demoActions {
			got, err := p.act(ctx, prefix)
			if err != nil {
				t.Fatalf("replay act map %d step %d: %v", i, j, err)
			}
			if got == demoActions[j] {
				replayMatch++
			}
			replaySteps++
			if j+1 < len(demoObs) {
				prefix = append(prefix, demoObs[j+1])
			}
		}
	}
	t.Logf("recurrent: demonstration replay agreement %v (%d/%d)",
		float64(replayMatch)/float64(replaySteps), replayMatch, replaySteps)
	ff, err := newNav2DPolicy(Nav2DFeedforward, 1, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy feedforward: %v", err)
	}
	totalSteps := 0
	var updates uint64
	for e := 0; e < 40; e++ {
		obs, actions, err := nav2dExpertEpisode(c, nav2dTrainMap(1, e))
		if err != nil {
			t.Fatalf("feedforward episode %d: %v", e, err)
		}
		totalSteps += len(actions)
		if updates, err = ff.train(ctx, obs, actions); err != nil {
			t.Fatalf("feedforward train episode %d: %v", e, err)
		}
	}
	t.Logf("feedforward: %d demonstration steps, %d training updates", totalSteps, updates)
	if updates != uint64(totalSteps) {
		t.Errorf("feedforward applied %d updates, want the %d demonstration steps", updates, totalSteps)
	}
}

// TestNav2DDAggerActsWithPolicyAndLabelsWithExpert proves a DAgger episode
// (nav2dDAggerEpisode) executes the policy's own actions while recording what
// the expert would have done at every visited state: a same-seed random policy
// replayed on a fresh environment reproduces every observation and every label,
// performs exactly as many steps, and at least once chooses an action the
// expert did not label, so the executed action is the policy's, not the label's.
func TestNav2DDAggerActsWithPolicyAndLabelsWithExpert(t *testing.T) {
	ctx := context.Background()
	c := nav2d.Config{Task: nav2d.TaskAvoidObstacles}
	mapSeed := nav2dTrainMap(7, 0)
	p, err := newNav2DPolicy(Nav2DRandom, 7, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy: %v", err)
	}
	obs, labels, err := nav2dDAggerEpisode(ctx, p, c, mapSeed)
	if err != nil {
		t.Fatalf("nav2dDAggerEpisode: %v", err)
	}
	if len(obs) == 0 || len(obs) != len(labels) {
		t.Fatalf("episode recorded %d observations and %d labels", len(obs), len(labels))
	}
	replay, err := newNav2DPolicy(Nav2DRandom, 7, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy replay: %v", err)
	}
	env, err := nav2d.New(c)
	if err != nil {
		t.Fatalf("nav2d.New: %v", err)
	}
	cur, _ := env.Reset(mapSeed)
	prefix := [][]float64{cur.Vector()}
	differed := false
	for i := 0; ; i++ {
		if i >= len(obs) {
			t.Fatalf("replay exceeded the %d recorded steps without ending", len(obs))
		}
		if !reflect.DeepEqual(obs[i], cur.Vector()) {
			t.Errorf("step %d replay observation differs from the recorded one", i)
		}
		if want := labels[i]; want != env.Expert() {
			t.Errorf("step %d label %d differs from the current expert action %d", i, want, env.Expert())
		}
		action, err := replay.act(ctx, prefix)
		if err != nil {
			t.Fatalf("replay act step %d: %v", i, err)
		}
		if action != labels[i] {
			differed = true
		}
		next, _, done, _, err := env.Step(action)
		if err != nil {
			t.Fatalf("replay step %d: %v", i, err)
		}
		if done {
			if i != len(obs)-1 {
				t.Errorf("replay ended on step %d but the episode recorded %d steps", i+1, len(obs))
			}
			break
		}
		prefix = append(prefix, next.Vector())
		cur = next
	}
	if !differed {
		t.Errorf("the policy executed the expert label on every recorded step")
	}
}

// TestNav2DTrainScheduleIsDeterministicAndCounted pins the trainNav2D
// schedule: two fresh recurrent policies training on the same seed take exactly
// episodes applied updates and end with identical parameters, the random policy
// is never updated, and zero episodes is an error.
func TestNav2DTrainScheduleIsDeterministicAndCounted(t *testing.T) {
	ctx := context.Background()
	c := nav2d.Config{Task: nav2d.TaskAvoidObstacles}
	const episodes = 8
	snap := func() (uint64, learning.Parameters) {
		p, err := newNav2DPolicy(Nav2DRecurrent, 3, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
		if err != nil {
			t.Fatalf("newNav2DPolicy: %v", err)
		}
		updates, err := trainNav2D(ctx, p, c, 3, episodes)
		if err != nil {
			t.Fatalf("trainNav2D: %v", err)
		}
		return updates, p.trainer.Snapshot().Parameters
	}
	u1, prm1 := snap()
	u2, prm2 := snap()
	if u1 != u2 {
		t.Errorf("first schedule took %d updates, second %d", u1, u2)
	}
	if u1 != episodes {
		t.Errorf("schedule reported %d updates, want %d", u1, episodes)
	}
	if !reflect.DeepEqual(prm1, prm2) {
		t.Errorf("two identical schedules produced different parameters")
	}
	random, err := newNav2DPolicy(Nav2DRandom, 3, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy random: %v", err)
	}
	u, err := trainNav2D(ctx, random, c, 3, episodes)
	if err != nil {
		t.Fatalf("random trainNav2D: %v", err)
	}
	if u != 0 {
		t.Errorf("random policy reported %d updates, want 0", u)
	}
	zero, err := newNav2DPolicy(Nav2DRecurrent, 3, nav2dTestInputs, nav2dTestHidden, nav2dTestRecurrent, nav2dTestLearningRate)
	if err != nil {
		t.Fatalf("newNav2DPolicy zero: %v", err)
	}
	if u, err := trainNav2D(ctx, zero, c, 3, 0); err == nil {
		t.Errorf("trainNav2D with zero episodes did not error, returned %d", u)
	}
}

// nav2dCountActions rolls the policy out under its own actions over the given
// maps and counts how often each action was chosen. [0] is forward, [1] left,
// [2] right, [3] stay.
func nav2dCountActions(t *testing.T, ctx context.Context, p *nav2dPolicy, c nav2d.Config, maps []uint64) [nav2d.Actions]int {
	t.Helper()
	env, err := nav2d.New(c)
	if err != nil {
		t.Fatalf("nav2d.New: %v", err)
	}
	var counts [nav2d.Actions]int
	for _, m := range maps {
		obs, _ := env.Reset(m)
		prefix := [][]float64{obs.Vector()}
		for {
			action, err := p.act(ctx, prefix)
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			counts[action]++
			next, _, done, _, err := env.Step(action)
			if err != nil {
				t.Fatalf("step: %v", err)
			}
			if done {
				break
			}
			prefix = append(prefix, next.Vector())
		}
	}
	return counts
}
