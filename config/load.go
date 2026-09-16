package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/internal/strictjson"
)

// envPrefix is the prefix of every environment override. A variable that
// carries it and addresses nothing is an error rather than a silent no-op,
// because a misspelled override that does nothing is exactly the failure this
// package exists to prevent.
const envPrefix = "COIMNET_"

// document is the expanded form of one configuration: the configuration itself
// plus the provenance table. Provenance is written by Marshal and ignored when
// read back, because the layer a value came from is a property of the load that
// produced it and is recomputed every time.
type document struct {
	Config
	Provenance map[string]string `json:"provenance,omitempty"`
}

// Load reads one configuration file and expands it into a complete document.
//
// The layers are applied in the fixed order default, file, environment, command
// line. env entries are "NAME=VALUE" pairs as os.Environ produces them, and
// only the ones prefixed COIMNET_ are considered. cli entries are the values of
// repeated --set flags, written "section.field=value". Every value is parsed as
// the type of the field it addresses; a name that addresses nothing, and a
// value the field cannot hold, are both errors.
func Load(ctx context.Context, path string, env []string, cli []string) (Resolved, error) {
	if ctx == nil {
		return Resolved{}, errors.New("config: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Resolved{}, err
	}
	if path == "" {
		return Resolved{}, errors.New("config: configuration path must not be empty")
	}
	data, err := fileio.ReadRegular(ctx, path, MaxConfigBytes)
	if err != nil {
		return Resolved{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := strictjson.RejectDuplicateKeys(data); err != nil {
		return Resolved{}, fmt.Errorf("config: %s: %w", path, err)
	}

	// The raw walk runs before decoding so that a null required value and a
	// literal secret are reported with the JSON pointer that carried them,
	// which a decoded Go value can no longer tell anyone.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return Resolved{}, fmt.Errorf("config: %s must be a JSON object: %w", path, err)
	}
	if _, ok := top["schema_version"]; !ok {
		return Resolved{}, fmt.Errorf(`config: %s must declare "schema_version"`, path)
	}
	delete(top, "provenance")
	var filePointers []string
	if err := walkObject(top, reflect.TypeOf(Config{}), "", &filePointers); err != nil {
		return Resolved{}, err
	}

	// Decoding starts from the declared defaults, so a document that omits a
	// field keeps the default and the provenance table can say so.
	expanded := document{Config: Defaults()}
	if err := strictjson.Decode(bytes.NewReader(data), MaxConfigBytes, &expanded); err != nil {
		return Resolved{}, fmt.Errorf("config: %s: %w", path, err)
	}
	resolved := expanded.Config

	// A secret reference may name a COIMNET_ variable of its own. Those hold
	// credentials, not configuration, so they are never read as overrides; a
	// reference that would shadow a real field is refused instead, so that one
	// variable can never mean two things.
	reserved := map[string]bool{}
	declared := map[string]SecretRef{}
	collectSecrets(reflect.ValueOf(resolved), "", declared)
	for pointer, ref := range declared {
		if ref.Env == "" {
			continue
		}
		if _, err := pointerForEnv(resolved, ref.Env); err == nil {
			return Resolved{}, fmt.Errorf("config: %s points at %s, which also addresses a configuration field; name the secret variable something else", pointer, ref.Env)
		}
		reserved[ref.Env] = true
	}

	envPointers, err := applyEnv(&resolved, env, reserved)
	if err != nil {
		return Resolved{}, err
	}
	cliPointers, err := applyCLI(&resolved, cli)
	if err != nil {
		return Resolved{}, err
	}
	if err := validate(resolved); err != nil {
		return Resolved{}, err
	}

	var leaves []string
	leafPointers(reflect.ValueOf(resolved), "", &leaves)
	provenance := make(map[string]string, len(leaves))
	for _, leaf := range leaves {
		source := SourceDefault
		if covers(filePointers, leaf) {
			source = SourceFile
		}
		if covers(envPointers, leaf) {
			source = SourceEnv
		}
		if covers(cliPointers, leaf) {
			source = SourceCLI
		}
		provenance[leaf] = source
	}
	secrets := map[string]SecretRef{}
	collectSecrets(reflect.ValueOf(resolved), "", secrets)
	return Resolved{Config: resolved, Provenance: provenance, Secrets: secrets}, nil
}

// Marshal writes the fully expanded document: every section, the schema
// version, the provenance table and every secret as the reference it is. The
// result is itself a loadable configuration, so an expanded document can be
// archived next to a run and read back without any of the original layers.
func (r Resolved) Marshal() ([]byte, error) {
	return json.MarshalIndent(document{Config: r.Config, Provenance: r.Provenance}, "", "  ")
}

// covers reports whether any recorded prefix owns this leaf. A layer that set
// a whole array owns every element of it.
func covers(prefixes []string, leaf string) bool {
	for _, prefix := range prefixes {
		if leaf == prefix || strings.HasPrefix(leaf, prefix+"/") {
			return true
		}
	}
	return false
}

// walkObject checks one decoded JSON object against the type it is meant to
// fill and records the pointer of every value the file actually carried.
func walkObject(object map[string]json.RawMessage, t reflect.Type, pointer string, present *[]string) error {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		field, ok := fieldByJSONName(t, key)
		if !ok {
			// Unknown fields are the strict decoder's refusal to make, and it
			// names them; skipping here keeps one message for one problem.
			continue
		}
		if err := walkValue(object[key], field.Type, pointer+"/"+key, present); err != nil {
			return err
		}
	}
	return nil
}

// walkValue is the raw counterpart of leafPointers: it validates the shape the
// file declared and records where its values are.
func walkValue(raw json.RawMessage, t reflect.Type, pointer string, present *[]string) error {
	trimmed := bytes.TrimSpace(raw)
	if string(trimmed) == "null" {
		if t.Kind() != reflect.Pointer {
			return fmt.Errorf("config: %s must not be null; omit it to keep the declared default", pointer)
		}
		*present = append(*present, pointer)
		return nil
	}
	if t == secretRefType {
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return fmt.Errorf(`config: %s must be a secret reference of the form {"ref": "env:NAME"}; a literal value must never be written into a configuration`, pointer)
		}
		*present = append(*present, pointer)
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return walkValue(trimmed, t.Elem(), pointer, present)
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return fmt.Errorf("config: %s must be an object", pointer)
		}
		return walkObject(object, t, pointer, present)
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return fmt.Errorf("config: %s must be an array", pointer)
		}
		*present = append(*present, pointer)
		for i, item := range items {
			if err := walkValue(item, t.Elem(), fmt.Sprintf("%s/%d", pointer, i), present); err != nil {
				return err
			}
		}
		return nil
	default:
		*present = append(*present, pointer)
		return nil
	}
}

// applyEnv applies every COIMNET_ variable that is not the target of a secret
// reference, and returns the pointers it set.
func applyEnv(c *Config, env []string, reserved map[string]bool) ([]string, error) {
	var applied []string
	for _, entry := range env {
		if !strings.HasPrefix(entry, envPrefix) {
			continue
		}
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("config: environment entry %q is not NAME=VALUE", entry)
		}
		if reserved[name] {
			continue
		}
		pointer, err := pointerForEnv(*c, name)
		if err != nil {
			return nil, err
		}
		if err := setByPointer(reflect.ValueOf(c).Elem(), pointer, value); err != nil {
			return nil, err
		}
		applied = append(applied, pointer)
	}
	return applied, nil
}

// applyCLI applies every --set entry and returns the pointers it set.
func applyCLI(c *Config, cli []string) ([]string, error) {
	var applied []string
	for _, entry := range cli {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("config: --set %q is not section.field=value", entry)
		}
		pointer := "/" + strings.ReplaceAll(strings.Trim(key, "."), ".", "/")
		if !overridable(*c)[pointer] {
			return nil, fmt.Errorf("config: --set %s addresses no field of the configuration; %s", key, overrideHint(*c))
		}
		if err := setByPointer(reflect.ValueOf(c).Elem(), pointer, value); err != nil {
			return nil, err
		}
		applied = append(applied, pointer)
	}
	return applied, nil
}

// pointerForEnv maps COIMNET_SECTION_FIELD onto the pointer it addresses. The
// mapping is generated from the configuration itself, so the two can never
// drift apart and an unknown variable is always an error.
func pointerForEnv(c Config, name string) (string, error) {
	for pointer := range overridable(c) {
		if envNameFor(pointer) == name {
			return pointer, nil
		}
	}
	return "", fmt.Errorf("config: environment variable %s addresses no field of the configuration; %s", name, overrideHint(c))
}

func envNameFor(pointer string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(pointer, "/"), "/", "_"))
}

func overridable(c Config) map[string]bool {
	out := map[string]bool{}
	overridablePointers(reflect.ValueOf(c), "", out)
	return out
}

// overrideHint lists a few real names so a misspelling can be corrected without
// reading the schema document.
func overrideHint(c Config) string {
	pointers := make([]string, 0, len(overridable(c)))
	for pointer := range overridable(c) {
		pointers = append(pointers, pointer)
	}
	sort.Strings(pointers)
	if len(pointers) > 6 {
		pointers = pointers[:6]
	}
	shown := make([]string, 0, len(pointers))
	for _, pointer := range pointers {
		shown = append(shown, strings.ReplaceAll(strings.TrimPrefix(pointer, "/"), "/", "."))
	}
	return "declared fields include " + strings.Join(shown, ", ") + " (see docs/config-schema.md for all of them)"
}

// validate refuses a merged configuration that names something no
// implementation answers to, or that declares a value nothing can use.
func validate(c Config) error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("config: schema_version is %q; this build reads %q", c.SchemaVersion, SchemaVersion)
	}
	if err := requireName("/model/kind", c.Model.Kind, "model kinds", modelKinds); err != nil {
		return err
	}
	if (c.Model.Package == "") == (c.Model.Graph == "") {
		return fmt.Errorf("config: model must declare exactly one of /model/package and /model/graph, not both and not neither")
	}
	if strings.TrimSpace(c.Data.Version) == "" || strings.TrimSpace(c.Data.Filter) == "" {
		return fmt.Errorf("config: data must declare both a version and a filter")
	}
	if err := requireName("/time/unit", c.Time.Unit, "time units", timeUnits); err != nil {
		return err
	}
	if !finite(c.Time.Step) || c.Time.Step <= 0 {
		return fmt.Errorf("config: /time/step is %v; it must be a finite positive number", c.Time.Step)
	}
	if err := requireName("/sharing/parameter_sharing", c.Sharing.ParameterSharing, "parameter sharing modes", sharingModes); err != nil {
		return err
	}
	if err := requireName("/signals/encoding", c.Signals.Encoding, "encodings", encodings); err != nil {
		return err
	}
	if err := requireName("/signals/mapping", c.Signals.Mapping, "mappings", mappings); err != nil {
		return err
	}
	if _, err := Generator(c.Task.Name + "/" + c.Task.Version); err != nil {
		return fmt.Errorf("config: /task: %w", err)
	}
	if err := requireName("/task/loss", c.Task.Loss, "losses", losses); err != nil {
		return err
	}
	if len(c.Learning.Rules) == 0 {
		return fmt.Errorf("config: /learning/rules must name at least one rule; freeze a model through /trainable, not through an empty rule list")
	}
	seen := map[string]bool{}
	for i, rule := range c.Learning.Rules {
		pointer := fmt.Sprintf("/learning/rules/%d", i)
		if err := requireName(pointer, rule, "learning rules", learningRules); err != nil {
			return err
		}
		if seen[rule] {
			return fmt.Errorf("config: %s repeats the rule %q", pointer, rule)
		}
		seen[rule] = true
	}
	if err := validateOptions(c.Learning.Options); err != nil {
		return err
	}
	if c.Modulation != nil {
		if err := validateModulation(*c.Modulation); err != nil {
			return err
		}
	}
	if c.Teacher != nil {
		if err := validateTeacher(*c.Teacher); err != nil {
			return err
		}
	}
	if err := requireName("/device/kind", c.Device.Kind, "devices", devices); err != nil {
		return err
	}
	if c.Resources.MaxMemoryMiB < 0 || c.Resources.MaxTempMiB < 0 {
		return fmt.Errorf("config: resource limits must not be negative: %+v", c.Resources)
	}
	if c.Resources.MaxRuns < 1 {
		return fmt.Errorf("config: /resources/max_runs is %d; a run declaration needs at least one run", c.Resources.MaxRuns)
	}
	for pointer, fraction := range map[string]float64{
		"/splits/train": c.Splits.Train, "/splits/validation": c.Splits.Validation, "/splits/test": c.Splits.Test,
	} {
		if !finite(fraction) || fraction < 0 || fraction > 1 {
			return fmt.Errorf("config: %s is %v; a split is a fraction between 0 and 1", pointer, fraction)
		}
	}
	if sum := c.Splits.Train + c.Splits.Validation + c.Splits.Test; math.Abs(sum-1) > splitsTolerance {
		return fmt.Errorf("config: the three splits sum to %v; they must sum to 1", sum)
	}
	if strings.TrimSpace(c.Output.Dir) == "" {
		return fmt.Errorf("config: /output/dir must name the directory a run writes into")
	}
	return nil
}

func validateOptions(o LearningOptions) error {
	checks := []struct {
		pointer string
		value   float64
		ok      bool
		want    string
	}{
		{"/learning/options/learning_rate", o.LearningRate, finite(o.LearningRate) && o.LearningRate > 0, "a finite positive number"},
		{"/learning/options/beta1", o.Beta1, finite(o.Beta1) && o.Beta1 >= 0 && o.Beta1 < 1, "in [0,1)"},
		{"/learning/options/beta2", o.Beta2, finite(o.Beta2) && o.Beta2 >= 0 && o.Beta2 < 1, "in [0,1)"},
		{"/learning/options/epsilon", o.Epsilon, finite(o.Epsilon) && o.Epsilon > 0, "a finite positive number"},
		{"/learning/options/weight_decay", o.WeightDecay, finite(o.WeightDecay) && o.WeightDecay >= 0, "a finite non-negative number"},
		{"/learning/options/clip_norm", o.ClipNorm, finite(o.ClipNorm) && o.ClipNorm >= 0, "a finite non-negative number"},
		{"/learning/options/loss_scale", o.LossScale, finite(o.LossScale) && o.LossScale >= 0, "a finite non-negative number"},
	}
	for _, check := range checks {
		if !check.ok {
			return fmt.Errorf("config: %s is %v; it must be %s", check.pointer, check.value, check.want)
		}
	}
	if o.Truncation < 0 {
		return fmt.Errorf("config: /learning/options/truncation is %d; a backward window is not negative", o.Truncation)
	}
	if o.AccumulateSteps < 0 {
		return fmt.Errorf("config: /learning/options/accumulate_steps is %d; it is not negative", o.AccumulateSteps)
	}
	return nil
}

func validateModulation(m Modulation) error {
	if len(m.Sources) == 0 {
		return fmt.Errorf("config: /modulation declares no source; write null to declare modulation off")
	}
	for i, source := range m.Sources {
		pointer := fmt.Sprintf("/modulation/sources/%d/kind", i)
		if err := requireName(pointer, source.Kind, "modulation source kinds", modulationKinds); err != nil {
			return err
		}
		if source.Channels < 1 {
			return fmt.Errorf("config: /modulation/sources/%d/channels is %d; a source drives at least one channel", i, source.Channels)
		}
	}
	if m.Chemistry.Regions < 1 || m.Chemistry.Channels < 1 {
		return fmt.Errorf("config: /modulation/chemistry must declare at least one region and one channel, got %+v", m.Chemistry)
	}
	if m.Receptors < 0 {
		return fmt.Errorf("config: /modulation/receptors is %d", m.Receptors)
	}
	return requireName("/modulation/effects/reward_kind", m.Effects.RewardKind, "reward kinds", rewardKinds)
}

func validateTeacher(t Teacher) error {
	if err := requireName("/teacher/kind", t.Kind, "teacher kinds", teacherKinds); err != nil {
		return err
	}
	if strings.TrimSpace(t.Endpoint) == "" {
		return fmt.Errorf("config: /teacher/endpoint must name the endpoint a teacher call would reach")
	}
	if t.Budget.MaxRequests < 0 || !finite(t.Budget.MaxCost) || t.Budget.MaxCost < 0 {
		return fmt.Errorf("config: /teacher/budget must declare non-negative finite bounds, got %+v", t.Budget)
	}
	if t.Secret.Env == "" {
		return fmt.Errorf(`config: /teacher/secret must declare {"ref": "env:NAME"}`)
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
