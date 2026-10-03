package experiment

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
	"github.com/TimLai666/coimnet/learning/rl"
)

func TestPPOTrainingFeedbackReplay(t *testing.T) {
	c := shortGoalConfig().Corridor
	for _, seed := range []uint64{1000, 1001} {
		for _, success := range []bool{true, false} {
			env, err := gridnav.New(c)
			if err != nil {
				t.Fatal(err)
			}
			obs, info := env.Reset(seed)
			cue := obs.Cue
			action := gridnav.ActionStay
			if success {
				if info.Goal == 0 {
					action = gridnav.ActionLeft
				} else {
					action = gridnav.ActionRight
				}
			}
			var steps []rl.Transition
			var total float64
			for {
				next, reward, end, inf, e := env.Step(action)
				if e != nil {
					t.Fatal(e)
				}
				steps = append(steps, rl.Transition{Obs: obs.Vector(), Action: action, LogProb: -math.Log(3), Reward: reward, Done: inf.Reached, Timeout: inf.TimedOut})
				total += reward
				obs = next
				if end {
					break
				}
			}
			want := -.06
			if success {
				want = .97
			}
			if math.Abs(total-want) > 1e-12 {
				t.Fatalf("return %v want %v", total, want)
			}
			got, e := checkPPOTrainingEpisode(c, seed, steps, total)
			if e != nil || got != cue {
				t.Fatalf("valid seed %d success %v cue %v: %v", seed, success, got, e)
			}
			mutators := map[string]func([]rl.Transition){
				"observation":        func(s []rl.Transition) { s[0].Obs[0] *= -1 },
				"shape":              func(s []rl.Transition) { s[0].Obs = s[0].Obs[:3] },
				"reward":             func(s []rl.Transition) { s[0].Reward = .99 },
				"action":             func(s []rl.Transition) { s[0].Action = 3 },
				"early_end":          func(s []rl.Transition) { s[0].Done = true },
				"no_end":             func(s []rl.Transition) { s[len(s)-1].Done = false; s[len(s)-1].Timeout = false },
				"both_end":           func(s []rl.Transition) { s[len(s)-1].Done = true; s[len(s)-1].Timeout = true },
				"nan_value":          func(s []rl.Transition) { s[0].Value = math.NaN() },
				"infinite_bootstrap": func(s []rl.Transition) { s[0].BootstrapValue = math.Inf(1) },
				"positive_log_prob":  func(s []rl.Transition) { s[0].LogProb = .1 },
			}
			b, _ := json.Marshal(steps)
			for name, mutate := range mutators {
				var copy []rl.Transition
				if err := json.Unmarshal(b, &copy); err != nil {
					t.Fatal(err)
				}
				mutate(copy)
				if _, e := checkPPOTrainingEpisode(c, seed, copy, total); e == nil {
					t.Fatalf("accepted %s", name)
				}
			}
			for _, s := range [][]rl.Transition{nil, steps[:len(steps)-1]} {
				if _, e := checkPPOTrainingEpisode(c, seed, s, total); e == nil {
					t.Fatal("accepted incomplete episode")
				}
			}
			if _, e := checkPPOTrainingEpisode(c, seed, steps, total+.1); e == nil {
				t.Fatal("accepted incorrect return")
			}
		}
	}
}

func TestPPOTrainingFeedbackPolicyReplay(t *testing.T) {
	ctx := context.Background()
	c := shortGoalConfig().Corridor
	ind := ppoCollectTestIndividual(t)
	before := hash(ind.Snapshot())
	rollout, _, err := collectPPOEpisode(ctx, ind, c, 1000, ppoCollectTestRNG(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPPOTrainingPolicy(ctx, ind, c, 1000, rollout.Steps); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"value", "log_prob", "bootstrap"} {
		steps := append([]rl.Transition(nil), rollout.Steps...)
		switch field {
		case "value":
			steps[0].Value += .1
		case "log_prob":
			steps[0].LogProb += .1
		case "bootstrap":
			steps[len(steps)-1].BootstrapValue += .1
		}
		if err := checkPPOTrainingPolicy(ctx, ind, c, 1000, steps); err == nil {
			t.Fatalf("accepted changed %s", field)
		}
	}
	if hash(ind.Snapshot()) != before {
		t.Fatal("policy replay mutated individual")
	}
}
