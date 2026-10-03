package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestMemoryPlanRejectsSplitPollutionAndMissingCoverage(t *testing.T) {
	trials, plan := memoryValidationFixture(t)
	if err := validateMemoryPlan(plan, trials); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	polluted := plan
	polluted.Split.Validation = append([]string(nil), plan.Split.Validation...)
	polluted.Split.Validation[0] = plan.Split.Train[0]
	if err := validateMemoryPlan(polluted, trials); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("polluted split error = %v, want overlap", err)
	}
	missing := plan
	missing.OriginalTest = append([]string(nil), plan.OriginalTest[:len(plan.OriginalTest)-1]...)
	if err := validateMemoryPlan(missing, trials); err == nil || !strings.Contains(err.Error(), "26 train and 13 test") {
		t.Fatalf("incomplete original split error = %v, want coverage/count error", err)
	}
}

func TestMemoryPlanRejectsFakeSeedAndBudget(t *testing.T) {
	trials, plan := memoryValidationFixture(t)
	seed := plan
	seed.Seeds = append([]uint64(nil), plan.Seeds...)
	seed.Seeds[0]++
	if err := validateMemoryPlan(seed, trials); err == nil || !strings.Contains(err.Error(), "seed") {
		t.Fatalf("fake seed error = %v, want seed contract error", err)
	}
	budget := plan
	budget.Epochs++
	if err := validateMemoryPlan(budget, trials); err == nil || !strings.Contains(err.Error(), "epoch") {
		t.Fatalf("fake epoch budget error = %v, want epoch contract error", err)
	}
}

func TestMemoryBundleRejectsUnknownVersionAndMissingContract(t *testing.T) {
	trials, plan := memoryValidationFixture(t)
	bundle := bundleFromPlan(plan, nil)
	bundle.SchemaVersion = "coimnet-realnav-memory-bundle/v999"
	if err := validateMemoryBundle(bundle, trials); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("unknown bundle version error = %v, want schema error", err)
	}
	bundle = bundleFromPlan(plan, nil)
	bundle.Config.InputFeatures = nil
	if err := validateMemoryBundle(bundle, trials); err == nil || !strings.Contains(err.Error(), "configuration") {
		t.Fatalf("missing bundle config error = %v, want configuration error", err)
	}
}

func TestMemoryStrictJSONRejectsUnknownAndDuplicateKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"coimnet-realnav-memory-plan/v1","schema_version":"duplicate"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var plan memoryPlan
	if err := readMemoryJSON(path, &plan); err == nil {
		t.Fatal("accepted duplicate plan key")
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":"coimnet-realnav-memory-plan/v1","unknown":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := readMemoryJSON(path, &plan); err == nil {
		t.Fatal("accepted unknown plan field")
	}
}

func TestMemoryStrictJSONRequiresZeroValuedConfigFields(t *testing.T) {
	_, plan := memoryValidationFixture(t)
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(object["config"], &config); err != nil {
		t.Fatal(err)
	}
	delete(config, "truncation")
	object["config"], err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	var decoded memoryPlan
	if err := readMemoryJSON(path, &decoded); err == nil || !strings.Contains(err.Error(), "truncation") {
		t.Fatalf("missing zero-valued truncation accepted: %v", err)
	}

	encoded, err = json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(object["config"], &config); err != nil {
		t.Fatal(err)
	}
	config["weight_decay"] = json.RawMessage("null")
	object["config"], err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := readMemoryJSON(path, &decoded); err == nil || !strings.Contains(err.Error(), "weight_decay") {
		t.Fatalf("null zero-valued weight_decay accepted: %v", err)
	}
}

func TestMemorySnapshotPresenceRejectsMissingZeroOption(t *testing.T) {
	raw := []byte(`{"schema_version":"coimnet-episode-training/v1","config":{},"parameters":{},"options":{"learning_rate":0.001,"beta1":0.9,"beta2":0.999,"epsilon":0.00000001,"clip_norm":1,"truncation":0,"trainable":{}},"optimizer":{},"updates":0}`)
	if err := validateMemorySnapshotPresence(raw, "before"); err == nil || !strings.Contains(err.Error(), "weight_decay") {
		t.Fatalf("missing snapshot weight_decay accepted: %v", err)
	}
}

func TestMemoryRunRejectsForgedAfterSnapshotWithoutFullBudget(t *testing.T) {
	_, plan := memoryValidationFixture(t)
	config, parameters, err := newModel(int64(memorySeeds[0]))
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := learning.NewTrainer(config, parameters, trainingOptions())
	if err != nil {
		t.Fatal(err)
	}
	before := trainer.Snapshot()
	capacity := trainer.Capacity()

	forged := before
	forged.Updates = memoryRunUpdates
	forged.Optimizer.Steps = make([]uint64, len(before.Optimizer.Steps))
	run := memoryRun{
		Seed:        memorySeeds[0],
		Control:     memoryControls[0],
		Before:      before,
		After:       forged,
		Curve:       make([]float64, plan.Epochs),
		Capacity:    capacity,
		MeanStepLen: 0,
		Updates:     memoryRunUpdates,
	}
	if err := validateMemoryRun(run, plan); err == nil {
		t.Fatal("accepted after snapshot with a forged 200-update counter and zero optimizer steps")
	}

	forged = before
	forged.Updates = memoryRunUpdates
	forged.Optimizer.Steps = make([]uint64, len(before.Optimizer.Steps))
	for i := range forged.Optimizer.Steps {
		forged.Optimizer.Steps[i] = memoryRunUpdates
	}
	forged.Accumulator = &learning.GradientAccumulator{Sum: make([]float64, len(before.Optimizer.First))}
	run.After = forged
	if err := validateMemoryRun(run, plan); err == nil {
		t.Fatal("accepted after snapshot with a non-nil accumulator under the no-accumulation contract")
	}
}

func TestMemoryOutputExistingDirectoryIsUntouched(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "existing")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(out, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := createMemoryOutput(out); err == nil {
		t.Fatal("accepted an existing output directory")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing output changed: %q %v", got, err)
	}
}

func TestMemoryCleanupLeavesUnexpectedFiles(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "new-output")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(out, "unexpected")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupMemoryOutput(out)
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "keep" {
		t.Fatalf("cleanup removed unexpected file: %q %v", got, err)
	}
}

func memoryValidationFixture(t *testing.T) ([]historyTrial, memoryPlan) {
	t.Helper()
	trials := make([]historyTrial, 0, 39)
	for i := 0; i < 39; i++ {
		id := "trial-" + leftPadMemory(i, 2)
		steps := []historyStep{{Input: [6]float64{0, 0, 0, 0, 0, 0}, T: 0}, {Input: [6]float64{0, 0, 0, 0, .1, 0}, T: .1, Scored: true, Target: [2]float64{.1, 0}}, {Input: [6]float64{0, 0, 0, 0, .1, 0}, T: .2}}
		trials = append(trials, historyTrial{ID: id, Condition: "rewarded", Steps: steps, RolloutIndex: 1})
	}
	base := memoryBaseModel{SchemaVersion: memoryBaseSchema, SourceSHA256: realSourceSHA256, Split: memoryBaseSplit{Train: trialIDsRange(0, 26), Test: trialIDsRange(26, 39)}}
	split, err := makeSplit(trials, base.Split.Train, base.Split.Test)
	if err != nil {
		t.Fatal(err)
	}
	return trials, newMemoryPlan(realSource(), base, strings.Repeat("a", 64), split)
}

func trialIDsRange(start, end int) []string {
	ids := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		ids = append(ids, "trial-"+leftPadMemory(i, 2))
	}
	return ids
}

func leftPadMemory(value, width int) string {
	result := ""
	for n := value; n > 0; n /= 10 {
		result = string(rune('0'+n%10)) + result
	}
	for len(result) < width {
		result = "0" + result
	}
	return result
}
