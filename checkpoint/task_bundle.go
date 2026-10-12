package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/learning"
)

// TaskTrainingSnapshot binds task configuration to the complete training state.
type TaskTrainingSnapshot struct {
	TaskSchema string                    `json:"task_schema"`
	Config     json.RawMessage           `json:"config"`
	Training   learning.TrainingSnapshot `json:"training"`
	// ManifestSHA256 identifies the manifest verified by this load, not a later read.
	ManifestSHA256 string `json:"-"`
}

// SaveTaskTrainingBundle publishes a task model in an exclusive checked directory bundle.
func SaveTaskTrainingBundle(ctx context.Context, dir, taskSchema string, config []byte, snapshot learning.TrainingSnapshot) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateTaskSchema(taskSchema); err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("task bundle directory must not be empty")
	}
	if err := validateTaskConfigJSON(config); err != nil {
		return fmt.Errorf("invalid task config: %w", err)
	}
	owned, topology, err := validatedTaskTraining(snapshot)
	if err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	document := TaskTrainingSnapshot{TaskSchema: taskSchema, Config: append(json.RawMessage(nil), config...), Training: owned}
	return writeBundle(ctx, dir, BundleManifest{Kind: BundleTaskTraining, DocumentSchema: taskSchema, Topology: topology}, document)
}

// LoadTaskTrainingBundle restores task configuration and training state after validation.
func LoadTaskTrainingBundle(ctx context.Context, dir, taskSchema string) (TaskTrainingSnapshot, error) {
	var empty TaskTrainingSnapshot
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	if err := validateTaskSchema(taskSchema); err != nil {
		return empty, err
	}
	if dir == "" {
		return empty, fmt.Errorf("task bundle directory must not be empty")
	}
	var document TaskTrainingSnapshot
	manifest, err := readBundle(ctx, dir, BundleTaskTraining, &document)
	if err != nil {
		return empty, err
	}
	if manifest.DocumentSchema != taskSchema || document.TaskSchema != taskSchema {
		return empty, fmt.Errorf("task bundle schema %q/%q, want %q", manifest.DocumentSchema, document.TaskSchema, taskSchema)
	}
	training, topology, err := validatedTaskTraining(document.Training)
	if err != nil {
		return empty, err
	}
	if manifest.Topology != topology {
		return empty, fmt.Errorf("task bundle manifest topology does not match training topology")
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	document.Training = training
	canonicalManifest, err := json.Marshal(manifest)
	if err != nil {
		return empty, fmt.Errorf("encode verified task manifest: %w", err)
	}
	document.ManifestSHA256 = digestHex(canonicalManifest)
	return document, nil
}

// DecodeTaskConfig decodes a complete task configuration, rejecting unknown,
// missing, null, duplicate and malformed fields before semantic validation.
func DecodeTaskConfig(data []byte, target any) error {
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("task config destination must be a non-nil struct pointer")
	}
	if err := validateTaskConfigJSON(data); err != nil {
		return err
	}
	if err := checkRequiredFields(data, value.Elem().Type(), "config"); err != nil {
		return err
	}
	decoded := reflect.New(value.Elem().Type())
	if err := decodeStrict(data, decoded.Interface()); err != nil {
		return err
	}
	value.Elem().Set(decoded.Elem())
	return nil
}

// UnmarshalJSON checks required fields before the bundle reader restores arrays.
func (s *TaskTrainingSnapshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("task training destination must not be nil")
	}
	if err := validateJSONUnicode(data); err != nil {
		return err
	}
	if err := checkUniqueJSON(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"task_schema", "config", "training"} {
		raw, present := fields[name]
		if !present || isJSONNull(raw) {
			return fmt.Errorf("task document.%s is missing or null", name)
		}
	}
	if err := validateTaskConfigJSON(fields["config"]); err != nil {
		return err
	}
	if err := checkRequiredFields(fields["training"], reflect.TypeOf(learning.TrainingSnapshot{}), "training"); err != nil {
		return err
	}
	type plain TaskTrainingSnapshot
	var decoded plain
	if err := decodeStrict(data, &decoded); err != nil {
		return err
	}
	if err := validateTaskSchema(decoded.TaskSchema); err != nil {
		return err
	}
	*s = TaskTrainingSnapshot(decoded)
	return nil
}

func validateTaskSchema(schema string) error {
	if strings.TrimSpace(schema) == "" || !utf8.ValidString(schema) {
		return fmt.Errorf("task schema must be nonempty valid UTF-8")
	}
	return nil
}

func validateTaskConfigJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("task config must be a JSON object")
	}
	if len(data) > maxBundleDocumentBytes {
		return fmt.Errorf("task config exceeds bundle document limit")
	}
	if err := validateJSONUnicode(data); err != nil {
		return err
	}
	return checkUniqueJSONRejectNull(data)
}

func validatedTaskTraining(snapshot learning.TrainingSnapshot) (learning.TrainingSnapshot, TopologyFingerprint, error) {
	if snapshot.SchemaVersion != TrainingSchemaVersion {
		return learning.TrainingSnapshot{}, TopologyFingerprint{}, fmt.Errorf("unsupported training schema %q", snapshot.SchemaVersion)
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return learning.TrainingSnapshot{}, TopologyFingerprint{}, fmt.Errorf("invalid training snapshot: %w", err)
	}
	owned := normalizeBundleTrainingSnapshot(trainer.Snapshot())
	topology, err := individualTopologyFingerprint(owned.Config)
	if err != nil {
		return learning.TrainingSnapshot{}, TopologyFingerprint{}, fmt.Errorf("fingerprint training topology: %w", err)
	}
	return owned, topology, nil
}
