package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// Replay diagnostics must match observations, rewards and both end markers.
func checkPPOTrainingEpisode(c gridnav.Config, seed uint64, steps []rl.Transition, total float64) (float64, error) {
	env, err := gridnav.New(c)
	if err != nil {
		return 0, err
	}
	if len(steps) == 0 || len(steps) > c.TimeLimit || !finite(total) {
		return 0, fmt.Errorf("invalid episode length or return")
	}
	obs, _ := env.Reset(seed)
	cue := obs.Cue
	var sum float64
	for i, s := range steps {
		if !reflect.DeepEqual(s.Obs, obs.Vector()) || !finite(s.LogProb) || s.LogProb > 0 || !finite(s.Value) || !finite(s.BootstrapValue) {
			return 0, fmt.Errorf("invalid observation or policy value at step %d", i)
		}
		next, reward, end, info, err := env.Step(s.Action)
		if err != nil {
			return 0, err
		}
		if s.Reward != reward || s.Done != info.Reached || s.Timeout != info.TimedOut || end != (i == len(steps)-1) {
			return 0, fmt.Errorf("reward or end marker mismatch at step %d", i)
		}
		sum += reward
		obs = next
	}
	if math.Abs(sum-total) > 1e-12 {
		return 0, fmt.Errorf("return mismatch")
	}
	return cue, nil
}

type ppoTrainingFeedbackPoint struct {
	Seed           uint64       `json:"seed"`
	Update         int          `json:"update"`
	EnvSeed        uint64       `json:"env_seed"`
	Cue            float64      `json:"original_cue"`
	Return         float64      `json:"return"`
	SnapshotBefore string       `json:"snapshot_before"`
	SnapshotAfter  string       `json:"snapshot_after"`
	Rollout        rl.Rollout   `json:"rollout"`
	Advantages     []float64    `json:"advantages"`
	Targets        []float64    `json:"targets"`
	Report         rl.PPOReport `json:"update_report"`
}

func checkPPOTrainingPolicy(ctx context.Context, ind *learning.Individual, c gridnav.Config, seed uint64, steps []rl.Transition) error {
	copy, err := learning.RestoreIndividual(ind.Snapshot())
	if err != nil {
		return err
	}
	if err := copy.ResetNeural(ctx, make([]float64, len(ind.Snapshot().Parameters.Core.Bias))); err != nil {
		return err
	}
	env, err := gridnav.New(c)
	if err != nil {
		return err
	}
	env.Reset(seed)
	for i, s := range steps {
		out, err := copy.Advance(ctx, [][]float64{s.Obs})
		if err != nil {
			return err
		}
		if len(out) != 1 || len(out[0]) != gridnav.Actions+1 {
			return fmt.Errorf("invalid policy replay shape at step %d", i)
		}
		logp, err := rl.LogProb(out[0][:gridnav.Actions], s.Action)
		if err != nil || logp != s.LogProb || out[0][gridnav.Actions] != s.Value {
			return fmt.Errorf("policy replay mismatch at step %d", i)
		}
		next, _, _, _, err := env.Step(s.Action)
		if err != nil {
			return err
		}
		var bootstrap float64
		if s.Timeout {
			out, err := copy.Advance(ctx, [][]float64{next.Vector()})
			if err != nil {
				return err
			}
			if len(out) != 1 || len(out[0]) != gridnav.Actions+1 {
				return fmt.Errorf("invalid bootstrap replay shape at step %d", i)
			}
			bootstrap = out[0][gridnav.Actions]
		}
		if bootstrap != s.BootstrapValue {
			return fmt.Errorf("bootstrap replay mismatch at step %d", i)
		}
	}
	return nil
}

type ppoTrainingFeedbackProbe struct {
	Seed         uint64           `json:"seed"`
	Update       int              `json:"update"`
	SnapshotHash string           `json:"snapshot_hash"`
	Cue          string           `json:"cue"`
	Episodes     []ppoGoalEpisode `json:"episodes"`
}

// Opt-in diagnostic example. Observations never change the training protocol.
func TestPPOTrainingFeedbackEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_PPO_TRAINING_FEEDBACK_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_PPO_TRAINING_FEEDBACK_EVIDENCE is not set")
	}
	path := filepath.Join(dir, "report.json")
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("output must be absent: %s (%v)", path, err)
	}
	ctx := context.Background()
	c := shortGoalConfig()
	legacy, err := RunPPO(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		SchemaVersion string                     `json:"schema_version"`
		Config        PPOExperimentConfig        `json:"config"`
		ConfigHash    string                     `json:"config_hash"`
		Legacy        PPOExperimentReport        `json:"legacy_report"`
		Points        []ppoTrainingFeedbackPoint `json:"training"`
		Probes        []ppoTrainingFeedbackProbe `json:"probes"`
	}{SchemaVersion: "coimnet-ppo-training-feedback/v1", Config: c, ConfigHash: hash(c), Legacy: legacy}
	probe := func(ind *learning.Individual, seed uint64, update int) {
		before := hash(ind.Snapshot())
		for _, cue := range []string{"original", "erased", "flipped"} {
			p := ppoTrainingFeedbackProbe{Seed: seed, Update: update, SnapshotHash: before, Cue: cue}
			for _, envSeed := range []uint64{1001, 1000} {
				ep, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, envSeed, "greedy", cue, nil)
				if err != nil {
					t.Fatal(err)
				}
				p.Episodes = append(p.Episodes, ep)
			}
			report.Probes = append(report.Probes, p)
		}
		if hash(ind.Snapshot()) != before {
			t.Fatal("probe mutated training individual")
		}
	}
	for index, seed := range c.Seeds {
		expected := legacy.Results[index]
		if expected.Failed || expected.Updates != uint64(c.Updates) || len(expected.Curve) != c.Updates {
			t.Fatalf("legacy failed seed %d", seed)
		}
		ind, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
		if err != nil {
			t.Fatal(err)
		}
		probe(ind, seed, 0)
		rng := rand.New(rand.NewPCG(seed, ppoTrainRNGStream))
		var terminals, timeouts uint64
		for update := 1; update <= c.Updates; update++ {
			before := hash(ind.Snapshot())
			envSeed := imitationTrainSeed(seed, update-1)
			rollout, total, err := collectPPOEpisode(ctx, ind, c.Corridor, envSeed, rng)
			if err != nil {
				t.Fatal(err)
			}
			if hash(ind.Snapshot()) != before {
				t.Fatal("collector mutated training individual")
			}
			cue, err := checkPPOTrainingEpisode(c.Corridor, envSeed, rollout.Steps, total)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkPPOTrainingPolicy(ctx, ind, c.Corridor, envSeed, rollout.Steps); err != nil {
				t.Fatal(err)
			}
			advantages, targets, err := rl.Advantages(rollout.Steps, rl.GAEConfig{Gamma: c.PPO.Gamma, Lambda: c.PPO.Lambda})
			if err != nil {
				t.Fatal(err)
			}
			updated, updateReport, err := rl.Update(ctx, ind, []rl.Rollout{rollout}, gridnav.Actions, c.PPO)
			if err != nil || updated == nil {
				t.Fatalf("update %d seed %d: %v", update, seed, err)
			}
			ind = updated
			after := hash(ind.Snapshot())
			expectedPoint := expected.Curve[update-1]
			if total != expectedPoint.Return || updateReport.MeanLoss != expectedPoint.Loss || ind.Snapshot().Optimizer.Updates != uint64(update) {
				t.Fatalf("instrumentation changed RunPPO at seed %d update %d", seed, update)
			}
			last := rollout.Steps[len(rollout.Steps)-1]
			if last.Done {
				terminals++
			}
			if last.Timeout {
				timeouts++
			}
			report.Points = append(report.Points, ppoTrainingFeedbackPoint{seed, update, envSeed, cue, total, before, after, rollout, advantages, targets, updateReport})
			if update%20 == 0 {
				probe(ind, seed, update)
			}
		}
		if hash(ind.Snapshot()) != expected.FinalSnapshotHash || terminals != expected.TerminalEpisodes || timeouts != expected.TimeoutEpisodes {
			t.Fatalf("final RunPPO mismatch seed %d", seed)
		}
	}
	if len(report.Points) != 600 || len(report.Probes) != 99 {
		t.Fatal("incomplete diagnostic")
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write(append(b, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Logf("%d training episodes, %d probe episodes; all update curves and final snapshots match RunPPO", len(report.Points), len(report.Probes)*2)
}
