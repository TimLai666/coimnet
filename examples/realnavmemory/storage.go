package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/TimLai666/coimnet/internal/strictjson"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

const (
	memoryPlanSchema   = "coimnet-realnav-memory-plan/v1"
	memoryBundleSchema = "coimnet-realnav-memory-bundle/v1"
	memoryRunUpdates   = uint64(200)
	memoryEpochs       = 10
	memoryBlockRows    = 16
	memoryDecisionRows = 200
	memoryFeatureRule  = "coimnet-realnav-memory-causal-features/v1"
	memoryBaseSchema   = "coimnet-realnav-example/v1"
)

var (
	memorySeeds    = []uint64{20261003, 20261004, 20261005}
	memoryControls = []string{"delivered", "no_stimulus", "shuffled_stimulus"}
)

// memoryConfig is the complete fixed protocol used by every model run. It is
// deliberately data, rather than a collection of defaults, so a new process
// can reject a bundle whose training contract was changed in transit.
type memoryConfig struct {
	FeatureRule         string   `json:"feature_rule"`
	InputFeatures       []string `json:"input_features"`
	TargetFeatures      []string `json:"target_features"`
	SplitSeed           string   `json:"split_seed"`
	SplitRule           string   `json:"split_rule"`
	OriginalTrainCount  int      `json:"original_train_count"`
	OriginalTestCount   int      `json:"original_test_count"`
	ValidationCount     int      `json:"validation_count"`
	ValidationUse       string   `json:"validation_use"`
	TestPreviouslySeen  bool     `json:"test_previously_evaluated"`
	PoseScaleCM         float64  `json:"pose_scale_cm"`
	DisplacementScaleCM float64  `json:"displacement_scale_cm"`
	DecisionDTSeconds   float64  `json:"decision_dt_seconds"`
	CoreDT              float64  `json:"core_dt"`
	Activation          string   `json:"activation"`
	InitialEdgeScale    float64  `json:"initial_edge_scale"`
	InitialEncoderScale float64  `json:"initial_encoder_scale"`
	TauModelSteps       []int    `json:"tau_model_steps"`
	MaxDeltaTSeconds    float64  `json:"max_delta_t_seconds"`
	MaxStepDistanceCM   float64  `json:"max_step_distance_cm"`
	StimulusBlockRows   int      `json:"stimulus_block_rows"`
	Nodes               int      `json:"nodes"`
	Edges               int      `json:"edges"`
	EncoderShape        []int    `json:"encoder_shape"`
	ReadoutShape        []int    `json:"readout_shape"`
	InputSize           int      `json:"input_size"`
	OutputSize          int      `json:"output_size"`
	ReadoutEveryStep    bool     `json:"readout_every_step"`
	Epochs              int      `json:"epochs"`
	UpdatesPerRun       uint64   `json:"updates_per_run"`
	LearningRate        float64  `json:"learning_rate"`
	ClipNorm            float64  `json:"clip_norm"`
	WeightDecay         float64  `json:"weight_decay"`
	Truncation          int      `json:"truncation"`
	ArenaRadiusCM       float64  `json:"arena_radius_cm"`
	GoalRadiusCM        float64  `json:"goal_radius_cm"`
	DecisionSteps       int      `json:"decision_steps"`
}

type memoryPlan struct {
	SchemaVersion         string            `json:"schema_version"`
	Source                trajectory.Source `json:"source"`
	SourceSHA256          string            `json:"source_sha256"`
	BaseModelSHA256       string            `json:"base_model_sha256"`
	BaseModelSourceSHA256 string            `json:"base_model_source_sha256"`
	OriginalTrain         []string          `json:"original_train_trial_ids"`
	OriginalTest          []string          `json:"original_test_trial_ids"`
	Split                 trialSplit        `json:"split"`
	SplitFingerprint      string            `json:"split_fingerprint"`
	Seeds                 []uint64          `json:"seeds"`
	Controls              []string          `json:"controls"`
	Epochs                int               `json:"epochs"`
	Config                memoryConfig      `json:"config"`
}

type memoryRun struct {
	Seed        uint64                    `json:"seed"`
	Control     string                    `json:"control"`
	Before      learning.TrainingSnapshot `json:"before"`
	After       learning.TrainingSnapshot `json:"after"`
	Curve       []float64                 `json:"curve"`
	Capacity    learning.CapacityReport   `json:"capacity"`
	MeanStepLen float64                   `json:"mean_step_length_cm"`
	Updates     uint64                    `json:"updates"`
}

type memoryBundle struct {
	SchemaVersion         string            `json:"schema_version"`
	PlanFingerprint       string            `json:"plan_fingerprint"`
	Source                trajectory.Source `json:"source"`
	SourceSHA256          string            `json:"source_sha256"`
	BaseModelSHA256       string            `json:"base_model_sha256"`
	BaseModelSourceSHA256 string            `json:"base_model_source_sha256"`
	OriginalTrain         []string          `json:"original_train_trial_ids"`
	OriginalTest          []string          `json:"original_test_trial_ids"`
	Split                 trialSplit        `json:"split"`
	SplitFingerprint      string            `json:"split_fingerprint"`
	Seeds                 []uint64          `json:"seeds"`
	Controls              []string          `json:"controls"`
	Epochs                int               `json:"epochs"`
	Config                memoryConfig      `json:"config"`
	Runs                  []memoryRun       `json:"runs"`
}

type memoryBaseSplit struct {
	Train []string `json:"train_trial_ids"`
	Test  []string `json:"test_trial_ids"`
}

type memoryBaseModel struct {
	SchemaVersion string          `json:"schema_version"`
	SourceSHA256  string          `json:"source_sha256"`
	Split         memoryBaseSplit `json:"split"`
}

type memorySplitFingerprint struct {
	OriginalTrain []string   `json:"original_train_trial_ids"`
	OriginalTest  []string   `json:"original_test_trial_ids"`
	Split         trialSplit `json:"split"`
}

func memoryConfigContract() memoryConfig {
	return memoryConfig{
		FeatureRule:         memoryFeatureRule,
		InputFeatures:       []string{"current_x_norm", "current_y_norm", "previous_dx_norm", "previous_dy_norm", "previous_dt_seconds", "delivered_stimulus"},
		TargetFeatures:      []string{"next_dx_cm", "next_dy_cm"},
		SplitSeed:           splitSeed,
		SplitRule:           "26 original train trials stratified by condition; largest remainder validation quota; within-stratum SHA-256(split_seed + ASCII unit separator + trial ID); original 13 test trials fixed",
		OriginalTrainCount:  26,
		OriginalTestCount:   13,
		ValidationCount:     6,
		ValidationUse:       "report_only_no_tuning",
		TestPreviouslySeen:  true,
		PoseScaleCM:         30,
		DisplacementScaleCM: 2,
		DecisionDTSeconds:   0.1,
		CoreDT:              1,
		Activation:          "tanh",
		InitialEdgeScale:    0.02,
		InitialEncoderScale: 0.2,
		TauModelSteps:       []int{2, 4, 8, 16, 64, 256, 1024, 4096},
		MaxDeltaTSeconds:    0.5,
		MaxStepDistanceCM:   5,
		StimulusBlockRows:   memoryBlockRows,
		Nodes:               8,
		Edges:               64,
		EncoderShape:        []int{6, 8},
		ReadoutShape:        []int{8, 2},
		InputSize:           6,
		OutputSize:          2,
		ReadoutEveryStep:    true,
		Epochs:              memoryEpochs,
		UpdatesPerRun:       memoryRunUpdates,
		LearningRate:        0.001,
		ClipNorm:            1,
		WeightDecay:         0,
		Truncation:          0,
		ArenaRadiusCM:       30,
		GoalRadiusCM:        2,
		DecisionSteps:       memoryDecisionRows,
	}
}

func newMemoryPlan(source trajectory.Source, base memoryBaseModel, baseSHA string, split trialSplit) memoryPlan {
	config := memoryConfigContract()
	return memoryPlan{
		SchemaVersion:         memoryPlanSchema,
		Source:                source,
		SourceSHA256:          source.SHA256,
		BaseModelSHA256:       baseSHA,
		BaseModelSourceSHA256: base.SourceSHA256,
		OriginalTrain:         append([]string(nil), base.Split.Train...),
		OriginalTest:          append([]string(nil), base.Split.Test...),
		Split:                 copyTrialSplit(split),
		SplitFingerprint:      memorySplitFingerprintHash(base.Split.Train, base.Split.Test, split),
		Seeds:                 append([]uint64(nil), memorySeeds...),
		Controls:              append([]string(nil), memoryControls...),
		Epochs:                memoryEpochs,
		Config:                config,
	}
}

func bundleFromPlan(plan memoryPlan, runs []memoryRun) memoryBundle {
	return memoryBundle{
		SchemaVersion:         memoryBundleSchema,
		PlanFingerprint:       memoryPlanHash(plan),
		Source:                plan.Source,
		SourceSHA256:          plan.SourceSHA256,
		BaseModelSHA256:       plan.BaseModelSHA256,
		BaseModelSourceSHA256: plan.BaseModelSourceSHA256,
		OriginalTrain:         append([]string(nil), plan.OriginalTrain...),
		OriginalTest:          append([]string(nil), plan.OriginalTest...),
		Split:                 copyTrialSplit(plan.Split),
		SplitFingerprint:      plan.SplitFingerprint,
		Seeds:                 append([]uint64(nil), plan.Seeds...),
		Controls:              append([]string(nil), plan.Controls...),
		Epochs:                plan.Epochs,
		Config:                plan.Config,
		Runs:                  append([]memoryRun(nil), runs...),
	}
}

func copyTrialSplit(split trialSplit) trialSplit {
	return trialSplit{
		Train:      append([]string(nil), split.Train...),
		Validation: append([]string(nil), split.Validation...),
		Test:       append([]string(nil), split.Test...),
	}
}

func memorySplitFingerprintHash(originalTrain, originalTest []string, split trialSplit) string {
	return memoryJSONHash(memorySplitFingerprint{OriginalTrain: originalTrain, OriginalTest: originalTest, Split: split})
}

func memoryPlanHash(plan memoryPlan) string {
	return memoryJSONHash(plan)
}

func memoryJSONHash(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func readMemoryJSON(path string, dst any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("realnavmemory: JSON path %q is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 64<<20 {
		return fmt.Errorf("realnavmemory: JSON file %q exceeds 64 MiB", path)
	}
	if err := validateMemoryDocumentPresence(data, dst); err != nil {
		return fmt.Errorf("realnavmemory: decode %s: %w", path, err)
	}
	if err := strictjson.Decode(bytes.NewReader(data), 64<<20, dst); err != nil {
		return fmt.Errorf("realnavmemory: decode %s: %w", path, err)
	}
	return nil
}

func validateMemoryDocumentPresence(data []byte, dst any) error {
	if err := strictjson.RejectDuplicateKeys(data); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if object == nil {
		return errors.New("JSON document must be an object")
	}
	var topKeys []string
	switch dst.(type) {
	case *memoryPlan:
		topKeys = []string{"schema_version", "source", "source_sha256", "base_model_sha256", "base_model_source_sha256", "original_train_trial_ids", "original_test_trial_ids", "split", "split_fingerprint", "seeds", "controls", "epochs", "config"}
	case *memoryBundle:
		topKeys = []string{"schema_version", "plan_fingerprint", "source", "source_sha256", "base_model_sha256", "base_model_source_sha256", "original_train_trial_ids", "original_test_trial_ids", "split", "split_fingerprint", "seeds", "controls", "epochs", "config", "runs"}
	default:
		return nil
	}
	for _, key := range topKeys {
		value, ok := object[key]
		if !ok {
			return fmt.Errorf("required field %q is missing", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("required field %q must not be null", key)
		}
	}
	configRaw := object["config"]
	var config map[string]json.RawMessage
	if err := json.Unmarshal(configRaw, &config); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if config == nil {
		return errors.New("config must be an object")
	}
	configKeys := []string{"feature_rule", "input_features", "target_features", "split_seed", "split_rule", "original_train_count", "original_test_count", "validation_count", "validation_use", "test_previously_evaluated", "pose_scale_cm", "displacement_scale_cm", "decision_dt_seconds", "core_dt", "activation", "initial_edge_scale", "initial_encoder_scale", "tau_model_steps", "stimulus_block_rows", "nodes", "edges", "encoder_shape", "readout_shape", "input_size", "output_size", "readout_every_step", "epochs", "updates_per_run", "learning_rate", "clip_norm", "weight_decay", "truncation", "arena_radius_cm", "goal_radius_cm", "decision_steps"}
	for _, key := range configKeys {
		value, ok := config[key]
		if !ok {
			return fmt.Errorf("config field %q is missing", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("config field %q must not be null", key)
		}
	}
	if _, ok := dst.(*memoryBundle); ok {
		var runs []json.RawMessage
		if err := json.Unmarshal(object["runs"], &runs); err != nil {
			return fmt.Errorf("runs: %w", err)
		}
		for i, rawRun := range runs {
			var runObject map[string]json.RawMessage
			if err := json.Unmarshal(rawRun, &runObject); err != nil || runObject == nil {
				if err == nil {
					err = errors.New("run must be an object")
				}
				return fmt.Errorf("runs[%d]: %w", i, err)
			}
			if err := requireMemoryFields(runObject, []string{"seed", "control", "before", "after", "curve", "capacity", "mean_step_length_cm", "updates"}, fmt.Sprintf("runs[%d]", i)); err != nil {
				return err
			}
			if err := validateMemorySnapshotPresence(runObject["before"], fmt.Sprintf("runs[%d].before", i)); err != nil {
				return err
			}
			if err := validateMemorySnapshotPresence(runObject["after"], fmt.Sprintf("runs[%d].after", i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireMemoryFields(object map[string]json.RawMessage, keys []string, label string) error {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			return fmt.Errorf("%s field %q is missing", label, key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s field %q must not be null", label, key)
		}
	}
	return nil
}

func validateMemorySnapshotPresence(raw json.RawMessage, label string) error {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot == nil {
		if err == nil {
			err = errors.New("snapshot must be an object")
		}
		return fmt.Errorf("%s: %w", label, err)
	}
	if err := requireMemoryFields(snapshot, []string{"schema_version", "config", "parameters", "options", "optimizer", "updates"}, label); err != nil {
		return err
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(snapshot["options"], &options); err != nil || options == nil {
		if err == nil {
			err = errors.New("options must be an object")
		}
		return fmt.Errorf("%s.options: %w", label, err)
	}
	return requireMemoryFields(options, []string{"learning_rate", "beta1", "beta2", "epsilon", "weight_decay", "clip_norm", "truncation", "trainable"}, label+".options")
}

func writeMemoryJSONExclusive(path string, value any) (err error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("realnavmemory: encode %s: %w", path, err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".realnavmemory-*.tmp")
	if err != nil {
		return fmt.Errorf("realnavmemory: create temporary JSON for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	published := false
	var publishedInfo os.FileInfo
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
		if err != nil && published && publishedInfo != nil {
			if current, statErr := os.Stat(path); statErr == nil && os.SameFile(publishedInfo, current) {
				_ = os.Remove(path)
			}
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("realnavmemory: chmod temporary JSON for %s: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("realnavmemory: write temporary JSON for %s: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("realnavmemory: sync temporary JSON for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("realnavmemory: close temporary JSON for %s: %w", path, err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("realnavmemory: publish %s: %w", path, err)
	}
	publishedInfo, err = os.Stat(path)
	if err != nil {
		return fmt.Errorf("realnavmemory: stat published JSON %s: %w", path, err)
	}
	published = true
	if directory, err := os.Open(filepath.Dir(path)); err == nil {
		syncErr := directory.Sync()
		closeErr := directory.Close()
		if syncErr != nil {
			return fmt.Errorf("realnavmemory: sync output directory for %s: %w", path, syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("realnavmemory: close output directory for %s: %w", path, closeErr)
		}
	} else {
		return fmt.Errorf("realnavmemory: open output directory for %s: %w", path, err)
	}
	return nil
}

func readMemoryBaseModel(path string) (memoryBaseModel, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return memoryBaseModel{}, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return memoryBaseModel{}, "", err
	}
	if !info.Mode().IsRegular() {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: base model %q is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
	if err != nil {
		return memoryBaseModel{}, "", err
	}
	if len(data) > 64<<20 {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: base model exceeds 64 MiB")
	}
	if err := strictjson.RejectDuplicateKeys(data); err != nil {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: decode base model: %w", err)
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&fields); err != nil {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: decode base model: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return memoryBaseModel{}, "", errors.New("realnavmemory: base model contains trailing JSON value")
		}
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: base model contains trailing data: %w", err)
	}
	var model memoryBaseModel
	decodeRequired := func(name string, destination any) error {
		raw, ok := fields[name]
		if !ok {
			return fmt.Errorf("realnavmemory: base model is missing %s", name)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("realnavmemory: base model %s must not be null", name)
		}
		if err := json.Unmarshal(raw, destination); err != nil {
			return fmt.Errorf("realnavmemory: base model %s: %w", name, err)
		}
		return nil
	}
	if err := decodeRequired("schema_version", &model.SchemaVersion); err != nil {
		return memoryBaseModel{}, "", err
	}
	if err := decodeRequired("source_sha256", &model.SourceSHA256); err != nil {
		return memoryBaseModel{}, "", err
	}
	rawSplit, ok := fields["split"]
	if !ok || bytes.Equal(bytes.TrimSpace(rawSplit), []byte("null")) {
		return memoryBaseModel{}, "", errors.New("realnavmemory: base model split is required")
	}
	if err := strictjson.Decode(bytes.NewReader(rawSplit), 1<<20, &model.Split); err != nil {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: base model split: %w", err)
	}
	digest := sha256.Sum256(data)
	baseSHA := hex.EncodeToString(digest[:])
	if model.SchemaVersion != memoryBaseSchema {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: unsupported base model schema %q", model.SchemaVersion)
	}
	if model.SourceSHA256 != realSourceSHA256 {
		return memoryBaseModel{}, "", fmt.Errorf("realnavmemory: base model source SHA-256 %q does not match pinned source", model.SourceSHA256)
	}
	if err := validateBaseSplit(model.Split); err != nil {
		return memoryBaseModel{}, "", err
	}
	return model, baseSHA, nil
}

func validateBaseSplit(split memoryBaseSplit) error {
	if len(split.Train) != 26 || len(split.Test) != 13 {
		return fmt.Errorf("realnavmemory: base model split must contain 26 train and 13 test trials, got %d/%d", len(split.Train), len(split.Test))
	}
	seen := make(map[string]string, len(split.Train)+len(split.Test))
	for _, group := range []struct {
		name string
		ids  []string
	}{
		{name: "train", ids: split.Train}, {name: "test", ids: split.Test},
	} {
		for _, id := range group.ids {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("realnavmemory: base model %s split contains an empty trial ID", group.name)
			}
			if previous, exists := seen[id]; exists {
				return fmt.Errorf("realnavmemory: base model split trial %q overlaps %s and %s", id, previous, group.name)
			}
			seen[id] = group.name
		}
	}
	return nil
}

func validateMemoryPlan(plan memoryPlan, trials []historyTrial) error {
	if plan.SchemaVersion != memoryPlanSchema {
		return fmt.Errorf("realnavmemory: unsupported plan schema %q", plan.SchemaVersion)
	}
	if err := validateMemorySource(plan.Source, plan.SourceSHA256); err != nil {
		return err
	}
	if !validSHA256(plan.BaseModelSHA256) || !validSHA256(plan.BaseModelSourceSHA256) {
		return errors.New("realnavmemory: plan has invalid base-model SHA-256")
	}
	if plan.BaseModelSourceSHA256 != realSourceSHA256 {
		return errors.New("realnavmemory: plan base-model source does not match pinned source")
	}
	if err := validateMemoryContract(plan.Seeds, plan.Controls, plan.Epochs, plan.Config); err != nil {
		return err
	}
	if err := validateOriginalPartition(trials, plan.OriginalTrain, plan.OriginalTest); err != nil {
		return err
	}
	if len(plan.Split.Train) != 20 || len(plan.Split.Validation) != 6 || len(plan.Split.Test) != 13 {
		return fmt.Errorf("realnavmemory: plan split must contain 20 train, 6 validation and 13 test trials, got %d/%d/%d", len(plan.Split.Train), len(plan.Split.Validation), len(plan.Split.Test))
	}
	if err := validateSplit(trials, plan.Split); err != nil {
		return err
	}
	expectedSplit, err := makeSplit(trials, plan.OriginalTrain, plan.OriginalTest)
	if err != nil {
		return fmt.Errorf("realnavmemory: recompute fixed split: %w", err)
	}
	if !reflect.DeepEqual(plan.Split, expectedSplit) {
		return errors.New("realnavmemory: plan split differs from deterministic makeSplit result")
	}
	if !sameStrings(plan.Split.Test, plan.OriginalTest) {
		return errors.New("realnavmemory: plan test split differs from the fixed original test group")
	}
	if !subsetExact(plan.Split.Train, plan.OriginalTrain) || !subsetExact(plan.Split.Validation, plan.OriginalTrain) {
		return errors.New("realnavmemory: plan train/validation split contains a trial outside original train")
	}
	if plan.SplitFingerprint != memorySplitFingerprintHash(plan.OriginalTrain, plan.OriginalTest, plan.Split) {
		return errors.New("realnavmemory: plan split fingerprint mismatch")
	}
	return nil
}

func validateMemoryBundle(bundle memoryBundle, trials []historyTrial) error {
	if bundle.SchemaVersion != memoryBundleSchema {
		return fmt.Errorf("realnavmemory: unsupported bundle schema %q", bundle.SchemaVersion)
	}
	plan := memoryPlan{
		SchemaVersion:         memoryPlanSchema,
		Source:                bundle.Source,
		SourceSHA256:          bundle.SourceSHA256,
		BaseModelSHA256:       bundle.BaseModelSHA256,
		BaseModelSourceSHA256: bundle.BaseModelSourceSHA256,
		OriginalTrain:         append([]string(nil), bundle.OriginalTrain...),
		OriginalTest:          append([]string(nil), bundle.OriginalTest...),
		Split:                 copyTrialSplit(bundle.Split),
		SplitFingerprint:      bundle.SplitFingerprint,
		Seeds:                 append([]uint64(nil), bundle.Seeds...),
		Controls:              append([]string(nil), bundle.Controls...),
		Epochs:                bundle.Epochs,
		Config:                bundle.Config,
	}
	if err := validateMemoryPlan(plan, trials); err != nil {
		return err
	}
	trainTrials, err := selectTrials(trials, plan.Split.Train)
	if err != nil {
		return err
	}
	meanStepLength, err := meanLegalStepLength(trainTrials)
	if err != nil {
		return err
	}
	if bundle.PlanFingerprint == "" {
		return errors.New("realnavmemory: bundle plan fingerprint is required")
	}
	if bundle.PlanFingerprint != memoryPlanHash(plan) {
		return errors.New("realnavmemory: bundle plan fingerprint mismatch")
	}
	if len(bundle.Runs) != len(memorySeeds)*len(memoryControls) {
		return fmt.Errorf("realnavmemory: bundle must contain %d runs, got %d", len(memorySeeds)*len(memoryControls), len(bundle.Runs))
	}
	wanted := make(map[string]struct{}, len(bundle.Runs))
	for i := range bundle.Runs {
		run := &bundle.Runs[i]
		if !containsUint64(bundle.Seeds, run.Seed) || !containsString(bundle.Controls, run.Control) {
			return fmt.Errorf("realnavmemory: bundle run %d has an undeclared seed or control", i)
		}
		key := fmt.Sprintf("%d/%s", run.Seed, run.Control)
		if _, exists := wanted[key]; exists {
			return fmt.Errorf("realnavmemory: duplicate bundle run %s", key)
		}
		wanted[key] = struct{}{}
		if err := validateMemoryRun(*run, plan); err != nil {
			return fmt.Errorf("realnavmemory: run %s: %w", key, err)
		}
		if run.MeanStepLen != meanStepLength {
			return fmt.Errorf("realnavmemory: run %s mean step length does not match train legal labels", key)
		}
	}
	for _, seed := range memorySeeds {
		for _, control := range memoryControls {
			if _, exists := wanted[fmt.Sprintf("%d/%s", seed, control)]; !exists {
				return fmt.Errorf("realnavmemory: bundle missing run %d/%s", seed, control)
			}
		}
	}
	return nil
}

func validateMemoryRun(run memoryRun, plan memoryPlan) error {
	if run.Updates != memoryRunUpdates || run.After.Updates != memoryRunUpdates || run.Before.Updates != 0 {
		return fmt.Errorf("updates must be before 0 and after %d", memoryRunUpdates)
	}
	if len(run.Curve) != plan.Epochs {
		return fmt.Errorf("curve has %d epochs, want %d", len(run.Curve), plan.Epochs)
	}
	for i, loss := range run.Curve {
		if !finite(loss) || loss < 0 {
			return fmt.Errorf("curve[%d] is not a finite non-negative loss", i)
		}
	}
	if !finite(run.MeanStepLen) || run.MeanStepLen < 0 {
		return errors.New("mean step length is invalid")
	}
	expectedConfig, expectedParameters, err := newModel(int64(run.Seed))
	if err != nil {
		return fmt.Errorf("expected model: %w", err)
	}
	expectedOptions := trainingOptions()
	for name, snapshot := range map[string]learning.TrainingSnapshot{"before": run.Before, "after": run.After} {
		if snapshot.SchemaVersion != "coimnet-episode-training/v1" {
			return fmt.Errorf("%s snapshot has unsupported schema %q", name, snapshot.SchemaVersion)
		}
		if !reflect.DeepEqual(snapshot.Config, expectedConfig) {
			return fmt.Errorf("%s snapshot config differs from fixed model", name)
		}
		if !reflect.DeepEqual(snapshot.Options, expectedOptions) {
			return fmt.Errorf("%s snapshot options differ from fixed training options", name)
		}
		if _, err := learning.RestoreTrainer(snapshot); err != nil {
			return fmt.Errorf("%s snapshot is not restorable: %w", name, err)
		}
	}
	if run.After.Accumulator != nil {
		return errors.New("after snapshot must not retain a gradient accumulator")
	}
	for i, steps := range run.After.Optimizer.Steps {
		if steps != memoryRunUpdates {
			return fmt.Errorf("after snapshot optimizer steps[%d] = %d, want %d", i, steps, memoryRunUpdates)
		}
	}
	if !reflect.DeepEqual(run.Before.Parameters, expectedParameters) {
		return errors.New("before snapshot parameters do not match its declared seed initialization")
	}
	initialTrainer, err := learning.NewTrainer(expectedConfig, expectedParameters, expectedOptions)
	if err != nil {
		return fmt.Errorf("initial model contract: %w", err)
	}
	initialSnapshot := initialTrainer.Snapshot()
	if !reflect.DeepEqual(run.Before.Optimizer, initialSnapshot.Optimizer) || run.Before.Accumulator != nil {
		return errors.New("before snapshot optimizer is not a fresh matched initialization")
	}
	if !reflect.DeepEqual(run.Before.Config, run.After.Config) || !reflect.DeepEqual(run.Before.Options, run.After.Options) {
		return errors.New("before and after snapshots changed model contract")
	}
	trainer, err := learning.RestoreTrainer(run.After)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(run.Capacity, trainer.Capacity()) {
		return errors.New("capacity does not match after snapshot")
	}
	return nil
}

func validateMemoryContract(seeds []uint64, controls []string, epochs int, config memoryConfig) error {
	if !sameUint64(seeds, memorySeeds) {
		return errors.New("fixed seed contract mismatch")
	}
	if !sameStrings(controls, memoryControls) {
		return errors.New("fixed control contract mismatch")
	}
	if epochs != memoryEpochs {
		return fmt.Errorf("fixed epoch budget must be %d, got %d", memoryEpochs, epochs)
	}
	want := memoryConfigContract()
	if !reflect.DeepEqual(config, want) {
		return errors.New("fixed model/training/rollout configuration mismatch")
	}
	return nil
}

func validateMemorySource(source trajectory.Source, sourceSHA string) error {
	fixed := realSource()
	if err := source.Validate(); err != nil {
		return err
	}
	if sourceSHA != realSourceSHA256 || source.SHA256 != sourceSHA {
		return errors.New("realnavmemory: source SHA-256 does not match pinned source")
	}
	if source.ID != fixed.ID || source.URL != fixed.URL || source.DOI != fixed.DOI || source.PinnedCommit != fixed.PinnedCommit || source.License != fixed.License {
		return errors.New("realnavmemory: source provenance does not match pinned source")
	}
	return nil
}

func validateOriginalPartition(trials []historyTrial, originalTrain, originalTest []string) error {
	known, err := indexTrials(trials)
	if err != nil {
		return err
	}
	if len(originalTrain) != 26 || len(originalTest) != 13 {
		return fmt.Errorf("realnavmemory: original split must contain 26 train and 13 test trials, got %d/%d", len(originalTrain), len(originalTest))
	}
	if err := validateIDGroup("original train", originalTrain, known); err != nil {
		return err
	}
	if err := validateIDGroup("original test", originalTest, known); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(originalTrain)+len(originalTest))
	for _, id := range originalTrain {
		seen[id] = struct{}{}
	}
	for _, id := range originalTest {
		if _, exists := seen[id]; exists {
			return fmt.Errorf("realnavmemory: original split overlaps trial %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != len(known) {
		return fmt.Errorf("realnavmemory: original split does not cover all trials: got %d, want %d", len(seen), len(known))
	}
	return nil
}

func sameStrings(a, b []string) bool {
	return reflect.DeepEqual(a, b)
}

func sameUint64(a, b []uint64) bool {
	return reflect.DeepEqual(a, b)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsUint64(values []uint64, target uint64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func subsetExact(values, superset []string) bool {
	if len(values) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	allowed := make(map[string]struct{}, len(superset))
	for _, id := range superset {
		allowed[id] = struct{}{}
	}
	for _, id := range values {
		if _, ok := allowed[id]; !ok {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func finite(value float64) bool {
	return !isNaN(value) && !isInf(value)
}

func isNaN(value float64) bool { return value != value }

func isInf(value float64) bool {
	return value > 1.7976931348623157e308 || value < -1.7976931348623157e308
}

func createMemoryOutput(outPath string, inputPaths ...string) (string, error) {
	if strings.TrimSpace(outPath) == "" {
		return "", errors.New("realnavmemory: output directory is required")
	}
	output, err := filepath.Abs(outPath)
	if err != nil {
		return "", err
	}
	output = filepath.Clean(output)
	for _, inputPath := range inputPaths {
		if strings.TrimSpace(inputPath) == "" {
			continue
		}
		input, err := filepath.Abs(inputPath)
		if err != nil {
			return "", err
		}
		input = filepath.Clean(input)
		if pathOverlapsMemory(output, input) || pathOverlapsMemory(input, output) {
			return "", errors.New("realnavmemory: output directory overlaps an input file")
		}
	}
	if _, err := os.Lstat(output); err == nil {
		return "", fmt.Errorf("realnavmemory: output directory %q already exists", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return "", err
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		return "", fmt.Errorf("realnavmemory: create output directory: %w", err)
	}
	return output, nil
}

func pathOverlapsMemory(path, other string) bool {
	path = filepath.Clean(path)
	other = filepath.Clean(other)
	if path == other {
		return true
	}
	rel, err := filepath.Rel(other, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func cleanupMemoryOutput(path string) {
	if path == "" {
		return
	}
	// Remove only the empty directory we created. If another process placed a
	// file there, leave it untouched and leave the incomplete directory for
	// inspection instead of recursively deleting unknown content.
	_ = os.Remove(path)
}
