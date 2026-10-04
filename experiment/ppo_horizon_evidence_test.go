package experiment

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

type ppoHorizonArm struct {
	Mode       string            `json:"mode"`
	Training   []ppoHorizonPoint `json:"training"`
	Runs       []ppoGoalRun      `json:"runs"`
	GateBySeed map[uint64]bool   `json:"gate_by_seed"`
	Gate       bool              `json:"gate"`
}

// The full existing paired protocol is reused without consuming training RNG.
func evaluatePPOHorizonFixture(ctx context.Context, c PPOExperimentConfig, seed uint64, before, after *learning.Individual, pairs [][2]uint64) ([]ppoGoalRun, error) {
	var runs []ppoGoalRun
	for _, stage := range []string{"before", "after", "random"} {
		ind := before
		if stage == "after" {
			ind = after
		}
		modes, cues := []string{"sampled", "greedy"}, []string{"original", "erased", "flipped"}
		if stage == "random" {
			modes, cues = []string{"random"}, []string{"original"}
		}
		for _, mode := range modes {
			for _, cue := range cues {
				callerHash := hash(ind.Snapshot())
				run := ppoGoalRun{Seed: seed, Stage: stage, Mode: mode, Cue: cue}
				if stage != "random" {
					run.SnapshotHash = callerHash
				}
				for pairIndex, pair := range pairs {
					for _, envSeed := range pair {
						rng := rand.New(rand.NewPCG(uint64(1000+pairIndex), 0x1005))
						ep, err := ppoGoalEpisodeRun(ctx, ind, c.Corridor, envSeed, mode, cue, rng)
						if err != nil {
							return nil, err
						}
						run.Episodes = append(run.Episodes, ep)
					}
				}
				if hash(ind.Snapshot()) != callerHash {
					return nil, fmt.Errorf("horizon evaluation changed caller snapshot")
				}
				var err error
				run.Metrics, err = ppoGoalSummarize(run.Episodes)
				if err != nil {
					return nil, err
				}
				runs = append(runs, run)
			}
		}
	}
	return runs, nil
}

func readPPOHorizonReference(t *testing.T, name, digest string, target any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "evidence", "LRN-09", name, "report.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != digest {
		t.Fatalf("horizon reference fingerprint mismatch: %s", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	raw, readErr := io.ReadAll(gz)
	closeErr := gz.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("horizon reference read: %v, close: %v", readErr, closeErr)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

// Opt-in research result: an unmet learning gate is saved, never a reason to
// alter the fixed protocol or report an engineering test failure.
func TestPPOHorizonEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_PPO_HORIZON_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_PPO_HORIZON_EVIDENCE is not set")
	}
	path := filepath.Join(dir, "report.json")
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("output must be absent: %s (%v)", path, err)
	}
	var feedback struct {
		Config PPOExperimentConfig        `json:"config"`
		Points []ppoTrainingFeedbackPoint `json:"training"`
	}
	var short struct {
		Config PPOExperimentConfig `json:"config"`
		Runs   []ppoGoalRun        `json:"runs"`
	}
	const feedbackSHA = "27286d7e471543bc7478908dfa8dc3f4433d13a13bac072037139dc7603116de"
	const shortSHA = "85b6a88e35dc251560fdd978b830acf92a1e1bfbca0a5ef702a15a6884effeb5"
	readPPOHorizonReference(t, "training-feedback-20261004", feedbackSHA, &feedback)
	readPPOHorizonReference(t, "short-goal-cue-audit-20261003", shortSHA, &short)
	c := shortGoalConfig()
	if !reflect.DeepEqual(c, feedback.Config) || !reflect.DeepEqual(c, short.Config) || len(feedback.Points) != 600 || len(short.Runs) != 39 {
		t.Fatal("horizon references do not match the fixed protocol")
	}
	pairs, err := shortGoalPairs()
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		SchemaVersion string              `json:"schema_version"`
		Config        PPOExperimentConfig `json:"config"`
		ConfigHash    string              `json:"config_hash"`
		EvalStream    uint64              `json:"eval_stream"`
		EvalPairs     [][2]uint64         `json:"eval_pairs"`
		Sources       map[string]string   `json:"source_gzip_sha256"`
		Arms          []ppoHorizonArm     `json:"arms"`
	}{SchemaVersion: "coimnet-ppo-horizon-comparison/v1", Config: c, ConfigHash: hash(c), EvalStream: 0x1005, EvalPairs: pairs,
		Sources: map[string]string{"training-feedback-20261004": feedbackSHA, "short-goal-cue-audit-20261003": shortSHA}}
	ctx := context.Background()
	for _, deadline := range []bool{false, true} {
		arm := ppoHorizonArm{Mode: "continuing_bootstrap", GateBySeed: map[uint64]bool{}, Gate: true}
		if deadline {
			arm.Mode = "six_step_terminal"
		}
		for index, seed := range c.Seeds {
			before, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
			if err != nil {
				t.Fatal(err)
			}
			after, points, err := trainPPOHorizonFixture(ctx, c, seed, deadline)
			if err != nil {
				t.Fatal(err)
			}
			if len(points) != c.Updates || hash(before.Snapshot()) != points[0].Feedback.SnapshotBefore ||
				hash(after.Snapshot()) != points[len(points)-1].Feedback.SnapshotAfter || after.Snapshot().Optimizer.Updates != uint64(c.Updates) {
				t.Fatalf("incomplete horizon training seed %d", seed)
			}
			if !deadline {
				for update, point := range points {
					if !reflect.DeepEqual(point.Feedback, feedback.Points[index*c.Updates+update]) {
						t.Fatalf("horizon control differs from ticket 38 seed %d update %d", seed, update+1)
					}
				}
			}
			arm.Training = append(arm.Training, points...)
			runs, err := evaluatePPOHorizonFixture(ctx, c, seed, before, after, pairs)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != 13 {
				t.Fatal("incomplete horizon evaluation")
			}
			arm.Runs = append(arm.Runs, runs...)
			arm.GateBySeed[seed] = shortGoalGate(runs)
			arm.Gate = arm.Gate && arm.GateBySeed[seed]
		}
		if !deadline && !reflect.DeepEqual(arm.Runs, short.Runs) {
			t.Fatal("horizon control differs from ticket 37 full evaluation")
		}
		report.Arms = append(report.Arms, arm)
		t.Logf("%s: %d updates, %d evaluation conditions, gate=%v per-seed=%v", arm.Mode, len(arm.Training), len(arm.Runs), arm.Gate, arm.GateBySeed)
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
	if writeErr != nil || closeErr != nil {
		t.Fatalf("horizon report write: %v, close: %v", writeErr, closeErr)
	}
}
