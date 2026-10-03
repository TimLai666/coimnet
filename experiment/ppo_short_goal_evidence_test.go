package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/TimLai666/coimnet/experiment/gridnav"
)

// Ticket 37: same PPO defaults, but both time limits must be six.
func shortGoalConfig() PPOExperimentConfig {
	c := DefaultPPOExperimentConfig()
	c.Corridor.TimeLimit = 6
	c.PPO.TimeLimit = 6
	return c
}

// Pair the left/right environment seeds in ascending order within 1000..1039.
// Return 20 unique, balanced pairs or an error; no goal enters the policy.
func shortGoalPairs() ([][2]uint64, error) {
	env, err := gridnav.New(shortGoalConfig().Corridor)
	if err != nil {
		return nil, fmt.Errorf("short-goal pairing environment: %w", err)
	}
	left := make([]uint64, 0, 20)
	right := make([]uint64, 0, 20)
	for seed := uint64(1000); seed < 1040; seed++ {
		obs, _ := env.Reset(seed)
		switch obs.Cue {
		case -1:
			left = append(left, seed)
		case 1:
			right = append(right, seed)
		default:
			return nil, fmt.Errorf("short-goal seed %d has unexpected cue %v", seed, obs.Cue)
		}
	}
	if len(left) != 20 || len(right) != 20 {
		return nil, fmt.Errorf("short-goal cue balance left=%d right=%d, want 20 each", len(left), len(right))
	}
	pairs := make([][2]uint64, len(left))
	for i := range left {
		pairs[i] = [2]uint64{left[i], right[i]}
	}
	return pairs, nil
}

// Complete 13-condition matrix plus short-deadline cue-erasure/flip criteria.
func shortGoalGate(runs []ppoGoalRun) bool {
	if !ppoGoalGate(runs) {
		return false
	}
	metrics := make(map[string]ppoGoalMetrics, len(runs))
	for _, run := range runs {
		metrics[run.Stage+"/"+run.Mode+"/"+run.Cue] = run.Metrics
	}
	rate := func(m ppoGoalMetrics) float64 {
		return float64(m.Reached) / float64(m.Episodes)
	}
	original := metrics["after/sampled/original"]
	erased := metrics["after/sampled/erased"]
	greedyErased := metrics["after/greedy/erased"]
	greedyFlipped := metrics["after/greedy/flipped"]
	const tolerance = 1e-12
	return rate(original)-rate(erased)+tolerance >= .2 &&
		greedyErased.Reached <= 20 &&
		greedyFlipped.Reached == 0
}

// Opt-in evidence preserves full traces, verifies the existing training
// snapshot, and rejects an existing output before training.
// COIMNET_SHORT_GOAL_CUE_EVIDENCE=/new/path go test -count=1 -run '^TestPPOShortGoalCueEvidence$' ./experiment
func TestPPOShortGoalCueEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_SHORT_GOAL_CUE_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_SHORT_GOAL_CUE_EVIDENCE is not set")
	}
	path := filepath.Join(dir, "report.json")
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("output must be absent: %s (%v)", path, err)
	}
	ctx := context.Background()
	c := shortGoalConfig()
	pairs, err := shortGoalPairs()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := RunPPO(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		SchemaVersion string              `json:"schema_version"`
		Config        PPOExperimentConfig `json:"config"`
		ConfigHash    string              `json:"config_hash"`
		EvalStream    uint64              `json:"eval_stream"`
		EvalPairs     [][2]uint64         `json:"eval_pairs"`
		GateCriteria  string              `json:"gate_criteria"`
		GateBySeed    map[uint64]bool     `json:"short_goal_cue_gate_by_seed"`
		Gate          bool                `json:"short_goal_cue_gate"`
		Legacy        PPOExperimentReport `json:"legacy_report"`
		Runs          []ppoGoalRun        `json:"runs"`
	}{
		SchemaVersion: "coimnet-short-goal-cue-audit/v1",
		Config:        c,
		ConfigHash:    hash(c),
		EvalStream:    0x1005,
		EvalPairs:     pairs,
		GateCriteria:  "each seed: sampled left/right >=0.8, success gain over before and random >=0.15, flipped-cue success drop >=0.2, greedy both goal sides succeed, sampled cue-erasure success drop >=0.2, greedy erased <=20/40, greedy flipped =0/40; 1e-12 comparison tolerance",
		GateBySeed:    map[uint64]bool{},
		Gate:          true,
		Legacy:        legacy,
	}
	for i, seed := range c.Seeds {
		before, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
		if err != nil {
			t.Fatal(err)
		}
		after, err := trainPPOGoalFixture(ctx, c, seed)
		if err != nil {
			t.Fatal(err)
		}
		if i >= len(legacy.Results) {
			t.Fatalf("seed %d has no RunPPO result", seed)
		}
		legacyResult := legacy.Results[i]
		afterSnapshot := after.Snapshot()
		if legacyResult.Seed != seed || legacyResult.Failed || legacyResult.Updates != uint64(c.Updates) ||
			legacyResult.FinalSnapshotHash == "" || hash(afterSnapshot) != legacyResult.FinalSnapshotHash ||
			afterSnapshot.Optimizer.Updates != uint64(c.Updates) {
			t.Fatalf("seed %d training differs from RunPPO", seed)
		}
		start := len(report.Runs)
		for _, stage := range []string{"before", "after", "random"} {
			var ind = before
			if stage == "after" {
				ind = after
			}
			modes := []string{"sampled", "greedy"}
			cues := []string{"original", "erased", "flipped"}
			if stage == "random" {
				modes = []string{"random"}
				cues = []string{"original"}
			}
			for _, mode := range modes {
				for _, cue := range cues {
					callerHash := hash(ind.Snapshot())
					var modelHash string
					if stage != "random" {
						modelHash = callerHash
					}
					run := ppoGoalRun{Seed: seed, Stage: stage, Mode: mode, Cue: cue, SnapshotHash: modelHash}
					if stage == "random" {
						run.SnapshotHash = ""
					}
					for pairIndex, pair := range pairs {
						for _, envSeed := range pair {
							rng := rand.New(rand.NewPCG(uint64(1000+pairIndex), 0x1005))
							result, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, envSeed, mode, cue, rng)
							if err != nil {
								t.Fatalf("seed %d %s/%s/%s pair %d env %d: %v", seed, stage, mode, cue, pairIndex, envSeed, err)
							}
							run.Episodes = append(run.Episodes, result)
						}
					}
					if hash(ind.Snapshot()) != callerHash {
						t.Fatalf("seed %d %s/%s/%s changed caller snapshot", seed, stage, mode, cue)
					}
					run.Metrics, err = ppoGoalSummarize(run.Episodes)
					if err != nil {
						t.Fatal(err)
					}
					report.Runs = append(report.Runs, run)
				}
			}
		}
		report.GateBySeed[seed] = shortGoalGate(report.Runs[start:])
		report.Gate = report.Gate && report.GateBySeed[seed]
	}
	if len(report.Runs) != len(c.Seeds)*13 {
		t.Fatalf("saved runs=%d, want %d", len(report.Runs), len(c.Seeds)*13)
	}
	for _, run := range report.Runs {
		if len(run.Episodes) != len(pairs)*2 {
			t.Fatalf("seed %d %s/%s/%s episodes=%d, want %d", run.Seed, run.Stage, run.Mode, run.Cue, len(run.Episodes), len(pairs)*2)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Logf("%d runs, %d episodes; short_goal_cue_gate=%v, per seed=%v", len(report.Runs), len(report.Runs)*len(pairs)*2, report.Gate, report.GateBySeed)
}
