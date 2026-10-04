package experiment

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/learning/rl"
)

// ppoGradientUpstreams separates the frozen-target objective before optimization.
func ppoGradientUpstreams(out [][]float64, steps []rl.Transition, advantages, targets []float64, c rl.PPOConfig) (map[string][][]float64, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if len(out) == 0 || len(out) != len(steps) || len(advantages) != len(out) || len(targets) != len(out) || len(out) > c.TimeLimit || c.BurnIn >= len(out) {
		return nil, fmt.Errorf("invalid diagnostic length")
	}
	width := len(out[0])
	if width < 3 {
		return nil, fmt.Errorf("invalid output width")
	}
	u := make(map[string][][]float64)
	for _, name := range []string{"policy", "value", "entropy", "total"} {
		u[name] = make([][]float64, len(out))
	}
	for i, row := range out {
		if len(row) != width || !finite(advantages[i]) || !finite(targets[i]) || !finite(steps[i].LogProb) || steps[i].LogProb > 0 || steps[i].Action < 0 || steps[i].Action >= width-1 {
			return nil, fmt.Errorf("invalid objective input at %d", i)
		}
		for _, x := range row {
			if !finite(x) {
				return nil, fmt.Errorf("nonfinite output at %d", i)
			}
		}
		for name := range u {
			u[name][i] = make([]float64, width)
		}
		if i < c.BurnIn {
			continue
		}
		for _, name := range []string{"policy", "value", "entropy", "total"} {
			coefficients := c
			advantage := advantages[i]
			switch name {
			case "policy":
				coefficients.ValueCoef = 0
				coefficients.EntropyCoef = 0
			case "value":
				coefficients.EntropyCoef = 0
				advantage = 0
			case "entropy":
				coefficients.ValueCoef = 0
				advantage = 0
			}
			_, logits, value, err := rl.Loss(row[:width-1], steps[i].Action, steps[i].LogProb, advantage, row[width-1], targets[i], coefficients)
			if err != nil {
				return nil, err
			}
			copy(u[name][i], logits)
			u[name][i][width-1] = value
			for _, x := range u[name][i] {
				if !finite(x) {
					return nil, fmt.Errorf("nonfinite derivative")
				}
			}
		}
		for j := range row {
			if math.Abs(u["total"][i][j]-u["policy"][i][j]-u["value"][i][j]-u["entropy"][i][j]) > 1e-12 {
				return nil, fmt.Errorf("objective decomposition mismatch")
			}
		}
	}
	return u, nil
}

type ppoGradientSplit struct {
	Outputs        [][]float64                  `json:"outputs"`
	Upstreams      map[string][][]float64       `json:"upstreams"`
	Gradients      map[string]learning.Gradient `json:"gradients"`
	OptimizerStep  learning.StepResult          `json:"optimizer_step"`
	ParameterDelta learning.Parameters          `json:"parameter_delta"`
}

// Diagnose a single first-epoch update using detached collector targets. The
// returned original update validates the rollout and preserves its exact Adam
// history; isolated gradients are observations, never optimizer updates.
func diagnosePPOGradient(ctx context.Context, ind *learning.Individual, r rl.Rollout, c rl.PPOConfig) (ppoGradientSplit, *learning.Individual, rl.PPOReport, error) {
	var result ppoGradientSplit
	if ctx == nil || ind == nil || c.Epochs != 1 || c.MiniBatch != 1 {
		return result, nil, rl.PPOReport{}, fmt.Errorf("diagnostic requires one epoch and one rollout")
	}
	s := ind.Snapshot()
	if s.Optimizer.Options.Recompute != nil {
		return result, nil, rl.PPOReport{}, fmt.Errorf("diagnostic requires full history")
	}
	updated, report, err := rl.Update(ctx, ind, []rl.Rollout{r}, s.Config.OutputSize-1, c)
	if err != nil {
		return result, nil, report, err
	}
	adv, target, err := rl.Advantages(r.Steps, rl.GAEConfig{Gamma: c.Gamma, Lambda: c.Lambda})
	if err != nil {
		return result, nil, report, err
	}
	input := make([][]float64, len(r.Steps))
	for i, step := range r.Steps {
		input[i] = append([]float64(nil), step.Obs...)
	}
	at := s
	at.Neural = r.InitialNeural
	copy, err := learning.RestoreIndividual(at)
	if err != nil {
		return result, nil, report, err
	}
	result.Outputs, err = copy.Advance(ctx, input)
	if err != nil {
		return result, nil, report, err
	}
	result.Upstreams, err = ppoGradientUpstreams(result.Outputs, r.Steps, adv, target, c)
	if err != nil {
		return result, nil, report, err
	}
	network, err := learning.NewNetwork(s.Config)
	if err != nil {
		return result, nil, report, err
	}
	result.Gradients = make(map[string]learning.Gradient)
	for _, name := range []string{"policy", "value", "entropy", "total"} {
		g, err := network.LossGradientFrom(ctx, s.Parameters, input, result.Upstreams[name], s.Optimizer.Options.Truncation)
		if err != nil {
			return result, nil, report, err
		}
		result.Gradients[name] = g
	}
	if err := checkPPOGradientSum(result.Gradients); err != nil {
		return result, nil, report, err
	}
	trainer, err := learning.NewTrainer(s.Config, s.Parameters, s.Optimizer.Options)
	if err != nil {
		return result, nil, report, err
	}
	shaped := trainer.Snapshot()
	shaped.Optimizer = s.Optimizer.State
	shaped.Updates = s.Optimizer.Updates
	shaped.Accumulator = s.Optimizer.Accumulator
	trainer, err = learning.RestoreTrainer(shaped)
	if err != nil {
		return result, nil, report, err
	}
	result.OptimizerStep, err = trainer.StepFrom(ctx, input, result.Upstreams["total"])
	if err != nil {
		return result, nil, report, err
	}
	actual, want := trainer.Snapshot(), updated.Snapshot()
	if !reflect.DeepEqual(actual.Parameters, want.Parameters) || !reflect.DeepEqual(actual.Optimizer, want.Optimizer.State) || actual.Updates != want.Optimizer.Updates || !reflect.DeepEqual(actual.Accumulator, want.Optimizer.Accumulator) {
		return result, nil, report, fmt.Errorf("combined StepFrom differs from original Update")
	}
	if !reflect.DeepEqual(s, ind.Snapshot()) {
		return result, nil, report, fmt.Errorf("diagnostic mutated caller")
	}
	result.ParameterDelta = actual.Parameters
	diff := func(after, before []float64) {
		for i := range after {
			after[i] -= before[i]
		}
	}
	diff(result.ParameterDelta.Core.Weights, s.Parameters.Core.Weights)
	diff(result.ParameterDelta.Core.Bias, s.Parameters.Core.Bias)
	diff(result.ParameterDelta.Core.LogTau, s.Parameters.Core.LogTau)
	diff(result.ParameterDelta.ThetaRaw, s.Parameters.ThetaRaw)
	diff(result.ParameterDelta.Encoder, s.Parameters.Encoder)
	diff(result.ParameterDelta.Readout, s.Parameters.Readout)
	return result, updated, report, nil
}

func ppoGradientGroups(g learning.Gradient) map[string][]float64 {
	groups := map[string][]float64{"weights": g.Core.Weights, "bias": g.Core.Bias, "log_tau": g.Core.LogTau, "theta_raw": g.ThetaRaw, "encoder": g.Encoder, "readout": g.Readout, "core_initial": g.Core.Initial}
	for name, rows := range map[string][][]float64{"inputs": g.Inputs, "core_inputs": g.Core.Inputs} {
		var flat []float64
		for _, row := range rows {
			flat = append(flat, row...)
		}
		groups[name] = flat
	}
	return groups
}

func checkPPOGradientSum(g map[string]learning.Gradient) error {
	groups := make(map[string]map[string][]float64)
	for _, name := range []string{"policy", "value", "entropy", "total"} {
		v, ok := g[name]
		if !ok {
			return fmt.Errorf("missing %s gradient", name)
		}
		groups[name] = ppoGradientGroups(v)
	}
	for group, total := range groups["total"] {
		var residue float64
		norms := make(map[string]float64)
		for _, name := range []string{"policy", "value", "entropy"} {
			if len(groups[name][group]) != len(total) {
				return fmt.Errorf("gradient shape mismatch")
			}
		}
		for i, x := range total {
			if !finite(x) {
				return fmt.Errorf("nonfinite total gradient")
			}
			delta := x
			for _, name := range []string{"policy", "value", "entropy"} {
				v := groups[name][group][i]
				if !finite(v) {
					return fmt.Errorf("nonfinite component")
				}
				norms[name] = math.Hypot(norms[name], v)
				delta -= v
			}
			residue = math.Hypot(residue, delta)
		}
		scale := norms["policy"] + norms["value"] + norms["entropy"]
		if (scale == 0 && residue != 0) || (scale > 0 && residue/scale > 1e-5) {
			return fmt.Errorf("%s gradient sum mismatch: residual %g, scale %g", group, residue, scale)
		}
	}
	return nil
}

type ppoGradientPoint struct {
	Seed           uint64           `json:"seed"`
	Update         int              `json:"update"`
	SnapshotBefore string           `json:"snapshot_before"`
	SnapshotAfter  string           `json:"snapshot_after"`
	Split          ppoGradientSplit `json:"split"`
}

// Opt-in observation of the exact ticket 38 trajectory. Restore the initial
// seeded model and replay the saved updates; no actions are sampled here.
func TestPPOGradientEvidence(t *testing.T) {
	dir := os.Getenv("COIMNET_PPO_GRADIENT_EVIDENCE")
	if dir == "" {
		t.Skip("COIMNET_PPO_GRADIENT_EVIDENCE is not set")
	}
	path := filepath.Join(dir, "report.json")
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("output must be absent: %s (%v)", path, err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatal("output directory must exist")
	}
	compressed, err := os.ReadFile("../evidence/LRN-09/training-feedback-20261004/report.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	compressedSHA := fmt.Sprintf("%x", sha256.Sum256(compressed))
	if compressedSHA != "27286d7e471543bc7478908dfa8dc3f4433d13a13bac072037139dc7603116de" {
		t.Fatal("ticket 38 input fingerprint changed")
	}
	z, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(io.LimitReader(z, 32<<20))
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	rawSHA := fmt.Sprintf("%x", sha256.Sum256(raw))
	if rawSHA != "919306c03712d793357373ed6910ccd97f71485448dac55756c99cf3635da96a" {
		t.Fatal("ticket 38 raw fingerprint changed")
	}
	var source struct {
		SchemaVersion string                     `json:"schema_version"`
		Config        PPOExperimentConfig        `json:"config"`
		ConfigHash    string                     `json:"config_hash"`
		Points        []ppoTrainingFeedbackPoint `json:"training"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	c := shortGoalConfig()
	if source.SchemaVersion != "coimnet-ppo-training-feedback/v1" || !reflect.DeepEqual(source.Config, c) || source.ConfigHash != hash(c) || len(source.Points) != len(c.Seeds)*c.Updates {
		t.Fatal("source schema, config or count mismatch")
	}
	report := struct {
		SchemaVersion    string                        `json:"schema_version"`
		SourceGzipSHA256 string                        `json:"source_gzip_sha256"`
		SourceRawSHA256  string                        `json:"source_raw_sha256"`
		Config           PPOExperimentConfig           `json:"config"`
		InitialModels    []learning.IndividualSnapshot `json:"initial_models"`
		Points           []ppoGradientPoint            `json:"points"`
	}{SchemaVersion: "coimnet-ppo-gradient-diagnostic/v1", SourceGzipSHA256: compressedSHA, SourceRawSHA256: rawSHA, Config: c}
	ctx := context.Background()
	for index, seed := range c.Seeds {
		ind, err := newPPOIndividual(seed, c.Hidden, c.LearningRate)
		if err != nil {
			t.Fatal(err)
		}
		report.InitialModels = append(report.InitialModels, ind.Snapshot())
		for update := 1; update <= c.Updates; update++ {
			p := source.Points[index*c.Updates+update-1]
			if p.Seed != seed || p.Update != update || hash(ind.Snapshot()) != p.SnapshotBefore {
				t.Fatalf("source order or model mismatch seed %d update %d", seed, update)
			}
			cue, err := checkPPOTrainingEpisode(c.Corridor, p.EnvSeed, p.Rollout.Steps, p.Return)
			if err != nil || cue != p.Cue {
				t.Fatalf("source episode mismatch: %v", err)
			}
			if err := checkPPOTrainingPolicy(ctx, ind, c.Corridor, p.EnvSeed, p.Rollout.Steps); err != nil {
				t.Fatal(err)
			}
			adv, target, err := rl.Advantages(p.Rollout.Steps, rl.GAEConfig{Gamma: c.PPO.Gamma, Lambda: c.PPO.Lambda})
			if err != nil || !reflect.DeepEqual(adv, p.Advantages) || !reflect.DeepEqual(target, p.Targets) {
				t.Fatal("source GAE mismatch")
			}
			split, updated, originalReport, err := diagnosePPOGradient(ctx, ind, p.Rollout, c.PPO)
			if err != nil {
				t.Fatalf("seed %d update %d: %v", seed, update, err)
			}
			if hash(updated.Snapshot()) != p.SnapshotAfter || !reflect.DeepEqual(originalReport, p.Report) {
				t.Fatalf("original update mismatch seed %d update %d", seed, update)
			}
			report.Points = append(report.Points, ppoGradientPoint{Seed: seed, Update: update, SnapshotBefore: p.SnapshotBefore, SnapshotAfter: p.SnapshotAfter, Split: split})
			ind = updated
		}
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("600 exact updates and decompositions: %x (%d bytes)", sha256.Sum256(encoded), len(encoded))
}
