package config

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeConfig puts one document in a fresh temporary file and returns its path.
func writeConfig(t *testing.T, document string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	return path
}

// minimalDocument declares only what Defaults() cannot supply: which model file
// to read and where a real run would write. Everything else comes from the
// declared defaults, which is what makes the provenance table observable.
const minimalDocument = `{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "model.coimpkg"},
  "output": {"dir": "runs/fixture"}
}`

func loadMinimal(t *testing.T, env, cli []string) Resolved {
	t.Helper()
	resolved, err := Load(context.Background(), writeConfig(t, minimalDocument), env, cli)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return resolved
}

func TestLoadRejectsUnknownFieldWithItsPath(t *testing.T) {
	path := writeConfig(t, `{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "model.coimpkg", "graph_path": "x"},
  "output": {"dir": "runs/fixture"}
}`)
	_, err := Load(context.Background(), path, nil, nil)
	if err == nil {
		t.Fatal("Load accepted an unknown field")
	}
	if !strings.Contains(err.Error(), "graph_path") {
		t.Fatalf("error %q does not name the unknown field", err)
	}
}

func TestLoadRejectsDuplicateKey(t *testing.T) {
	path := writeConfig(t, `{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "model.coimpkg"},
  "output": {"dir": "runs/fixture"},
  "seed": 1,
  "seed": 2
}`)
	_, err := Load(context.Background(), path, nil, nil)
	if err == nil {
		t.Fatal("Load accepted a duplicate key")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error %q does not report the duplicate key", err)
	}
}

func TestLoadRejectsNullRequiredValue(t *testing.T) {
	for _, document := range []string{
		`{"schema_version": null, "model": {"kind": "continuous", "package": "m"}, "output": {"dir": "o"}}`,
		`{"schema_version": "coimnet-config/v1", "model": null, "output": {"dir": "o"}}`,
		`{"schema_version": "coimnet-config/v1", "model": {"kind": null, "package": "m"}, "output": {"dir": "o"}}`,
		`{"schema_version": "coimnet-config/v1", "model": {"kind": "continuous", "package": "m"}, "output": {"dir": "o"}, "seed": null}`,
		`{"schema_version": "coimnet-config/v1", "model": {"kind": "continuous", "package": "m"}, "output": {"dir": "o"}, "learning": {"rules": null}}`,
	} {
		if _, err := Load(context.Background(), writeConfig(t, document), nil, nil); err == nil {
			t.Fatalf("Load accepted a null required value: %s", document)
		}
	}
}

func TestLoadRequiresSchemaVersion(t *testing.T) {
	path := writeConfig(t, `{"model": {"kind": "continuous", "package": "m"}, "output": {"dir": "o"}}`)
	if _, err := Load(context.Background(), path, nil, nil); err == nil {
		t.Fatal("Load accepted a document without schema_version")
	}
}

func TestLoadRejectsUnknownNamesAndNamesTheRegistry(t *testing.T) {
	cases := []struct {
		document, field, registry string
	}{
		{`"task": {"name": "no-such-task", "version": "v1", "loss": "mse/v1"}`, "task", "task generators"},
		{`"task": {"name": "delayed-correlation", "version": "v9", "loss": "mse/v1"}`, "task", "task generators"},
		{`"task": {"name": "delayed-correlation", "version": "v1", "loss": "huber/v1"}`, "loss", "losses"},
		{`"signals": {"encoding": "no-such-encoding/v1", "mapping": "identity/v1"}`, "encoding", "encodings"},
		{`"signals": {"encoding": "insyra-linear/v1", "mapping": "no-such-mapping/v1"}`, "mapping", "mappings"},
		{`"learning": {"rules": ["no-such-rule"], "options": {"learning_rate": 0.01, "beta1": 0.9, "beta2": 0.999, "epsilon": 1e-8, "weight_decay": 0, "clip_norm": 1, "truncation": 0}}`, "rules", "learning rules"},
		{`"modulation": {"sources": [{"kind": "telepathy", "channels": 1}], "chemistry": {"regions": 1, "channels": 1}, "receptors": 1, "effects": {"reward_kind": "relu"}}`, "kind", "modulation source kinds"},
		{`"modulation": {"sources": [{"kind": "replay", "channels": 1}], "chemistry": {"regions": 1, "channels": 1}, "receptors": 1, "effects": {"reward_kind": "sigmoid"}}`, "reward_kind", "reward kinds"},
		{`"device": {"kind": "gpu"}`, "device", "devices"},
		{`"sharing": {"parameter_sharing": "per_neuron"}`, "parameter_sharing", "parameter sharing modes"},
		{`"model": {"kind": "spiking", "package": "m"}`, "kind", "model kinds"},
	}
	for _, c := range cases {
		document := `{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "model.coimpkg"},
  "output": {"dir": "runs/fixture"},
  ` + c.document + `
}`
		// The model case replaces the model block rather than adding one.
		if strings.HasPrefix(c.document, `"model"`) {
			document = `{
  "schema_version": "coimnet-config/v1",
  "output": {"dir": "runs/fixture"},
  ` + c.document + `
}`
		}
		_, err := Load(context.Background(), writeConfig(t, document), nil, nil)
		if err == nil {
			t.Fatalf("Load accepted %s", c.document)
		}
		if !strings.Contains(err.Error(), c.field) || !strings.Contains(err.Error(), c.registry) {
			t.Fatalf("error %q names neither field %q nor registry %q", err, c.field, c.registry)
		}
		if strings.Contains(err.Error(), "using ") || strings.Contains(err.Error(), "substitut") {
			t.Fatalf("error %q suggests a substitute", err)
		}
	}
}

func TestModelRequiresExactlyOneSource(t *testing.T) {
	both := `{"schema_version": "coimnet-config/v1", "model": {"kind": "continuous", "package": "m", "graph": "g"}, "output": {"dir": "o"}}`
	neither := `{"schema_version": "coimnet-config/v1", "model": {"kind": "continuous"}, "output": {"dir": "o"}}`
	for _, document := range []string{both, neither} {
		if _, err := Load(context.Background(), writeConfig(t, document), nil, nil); err == nil {
			t.Fatalf("Load accepted %s", document)
		}
	}
}

func TestProvenanceReportsTheWinningLayer(t *testing.T) {
	// output.dir is set in all four layers, seed in file and environment, and
	// time.unit nowhere but the declared defaults.
	document := `{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "model.coimpkg"},
  "output": {"dir": "from-file"},
  "seed": 11
}`
	resolved, err := Load(context.Background(),
		writeConfig(t, document),
		[]string{"COIMNET_OUTPUT_DIR=from-env", "COIMNET_SEED=22", "PATH=/usr/bin"},
		[]string{"output.dir=from-cli"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	table := []struct{ pointer, value, source string }{
		{"/output/dir", "from-cli", SourceCLI},
		{"/seed", "22", SourceEnv},
		{"/time/unit", "model_step", SourceDefault},
		{"/model/package", "model.coimpkg", SourceFile},
	}
	for _, row := range table {
		if got := resolved.Provenance[row.pointer]; got != row.source {
			t.Fatalf("provenance[%s] = %q, want %q", row.pointer, got, row.source)
		}
	}
	if resolved.Config.Output.Dir != "from-cli" {
		t.Fatalf("output.dir = %q, want the command line value", resolved.Config.Output.Dir)
	}
	if resolved.Config.Seed != 22 {
		t.Fatalf("seed = %d, want the environment value", resolved.Config.Seed)
	}
	if resolved.Config.Time.Unit != Defaults().Time.Unit {
		t.Fatalf("time.unit = %q, want the default", resolved.Config.Time.Unit)
	}
}

func TestProvenanceCoversEveryLeaf(t *testing.T) {
	resolved := loadMinimal(t, nil, nil)
	for _, pointer := range []string{
		"/schema_version", "/data/version", "/data/filter", "/model/kind", "/model/package", "/model/graph",
		"/time/unit", "/time/step", "/sharing/parameter_sharing", "/trainable/trainable/weights",
		"/signals/encoding", "/signals/mapping", "/task/name", "/task/version", "/task/loss",
		"/learning/rules/0", "/learning/options/learning_rate", "/modulation", "/reset/neural_at_episode_start",
		"/teacher", "/seed", "/device/kind", "/resources/max_memory_mib", "/resources/max_temp_mib",
		"/resources/max_runs", "/splits/train", "/splits/validation", "/splits/test", "/output/dir",
	} {
		if resolved.Provenance[pointer] == "" {
			t.Fatalf("provenance has no entry for %s", pointer)
		}
	}
	for pointer, source := range resolved.Provenance {
		switch source {
		case SourceDefault, SourceFile, SourceEnv, SourceCLI:
		default:
			t.Fatalf("provenance[%s] = %q is not one of the four layers", pointer, source)
		}
	}
}

func TestEnvironmentAndCommandLineValuesAreParsedByTargetType(t *testing.T) {
	resolved := loadMinimal(t,
		[]string{"COIMNET_TIME_STEP=0.25", "COIMNET_RESET_NEURAL_AT_EPISODE_START=false", "COIMNET_RESOURCES_MAX_MEMORY_MIB=512"},
		[]string{"learning.options.learning_rate=0.05", "learning.rules=gradient,hebbian_rate"})
	if resolved.Config.Time.Step != 0.25 {
		t.Fatalf("time.step = %v, want 0.25", resolved.Config.Time.Step)
	}
	if resolved.Config.Reset.NeuralAtEpisodeStart {
		t.Fatal("reset.neural_at_episode_start stayed true")
	}
	if resolved.Config.Resources.MaxMemoryMiB != 512 {
		t.Fatalf("resources.max_memory_mib = %d, want 512", resolved.Config.Resources.MaxMemoryMiB)
	}
	if resolved.Config.Learning.Options.LearningRate != 0.05 {
		t.Fatalf("learning rate = %v, want 0.05", resolved.Config.Learning.Options.LearningRate)
	}
	if !reflect.DeepEqual(resolved.Config.Learning.Rules, []string{"gradient", "hebbian_rate"}) {
		t.Fatalf("rules = %v, want the two the command line set", resolved.Config.Learning.Rules)
	}
}

func TestEnvironmentAndCommandLineRejectBadNamesAndValues(t *testing.T) {
	cases := []struct{ env, cli []string }{
		{env: []string{"COIMNET_SEED=not-a-number"}},
		{env: []string{"COIMNET_TIME_STEP=oops"}},
		{env: []string{"COIMNET_RESET_NEURAL_AT_EPISODE_START=maybe"}},
		{env: []string{"COIMNET_NO_SUCH_FIELD=1"}},
		{env: []string{"COIMNET_SEED"}},
		{cli: []string{"no.such.field=1"}},
		{cli: []string{"seed"}},
		{cli: []string{"seed=not-a-number"}},
	}
	for _, c := range cases {
		if _, err := Load(context.Background(), writeConfig(t, minimalDocument), c.env, c.cli); err == nil {
			t.Fatalf("Load accepted env %v cli %v", c.env, c.cli)
		}
	}
}

const teacherDocument = `{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "model.coimpkg"},
  "output": {"dir": "runs/fixture"},
  "teacher": {
    "kind": "http/v1",
    "endpoint": "https://teacher.invalid/answer",
    "budget": {"max_requests": 10, "max_cost": 1.5},
    "secret": {"ref": "env:COIMNET_TEST_TEACHER_TOKEN"}
  }
}`

func TestSecretLiteralIsRejectedAndNamesTheField(t *testing.T) {
	document := strings.Replace(teacherDocument, `{"ref": "env:COIMNET_TEST_TEACHER_TOKEN"}`, `"sk-literal-value"`, 1)
	_, err := Load(context.Background(), writeConfig(t, document), nil, nil)
	if err == nil {
		t.Fatal("Load accepted a literal secret")
	}
	if !strings.Contains(err.Error(), "teacher") || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("error %q does not name the secret field", err)
	}
	if strings.Contains(err.Error(), "sk-literal-value") {
		t.Fatalf("error %q repeats the secret it refused", err)
	}
	for _, bad := range []string{`{"ref": "COIMNET_TEST_TEACHER_TOKEN"}`, `{"ref": "file:/etc/token"}`, `{"ref": "env:"}`, `{"env": "X"}`} {
		document := strings.Replace(teacherDocument, `{"ref": "env:COIMNET_TEST_TEACHER_TOKEN"}`, bad, 1)
		if _, err := Load(context.Background(), writeConfig(t, document), nil, nil); err == nil {
			t.Fatalf("Load accepted secret form %s", bad)
		}
	}
}

func TestMarshalCarriesReferencesAndNeverTheSecretValue(t *testing.T) {
	const sentinel = "SENTINEL-6f1a2c-not-a-real-token"
	resolved, err := Load(context.Background(), writeConfig(t, teacherDocument),
		[]string{"COIMNET_TEST_TEACHER_TOKEN=" + sentinel}, nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	encoded, err := resolved.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, sentinel) {
		t.Fatal("the expanded document contains the secret value")
	}
	if !strings.Contains(text, `"ref": "env:COIMNET_TEST_TEACHER_TOKEN"`) {
		t.Fatalf("the expanded document does not carry the reference:\n%s", text)
	}
	if !strings.Contains(text, `"schema_version": "coimnet-config/v1"`) {
		t.Fatalf("the expanded document has no schema version:\n%s", text)
	}
	if !strings.Contains(text, `"provenance"`) {
		t.Fatalf("the expanded document has no provenance block:\n%s", text)
	}
	ref, ok := resolved.Secrets["/teacher/secret"]
	if !ok || ref.Env != "COIMNET_TEST_TEACHER_TOKEN" {
		t.Fatalf("secrets = %+v, want the teacher reference", resolved.Secrets)
	}
	// Loading never reads the value the reference points at.
	if strings.Contains(text, "SENTINEL") {
		t.Fatal("the expanded document mentions the sentinel")
	}
}

func TestExpandedDocumentRoundTrips(t *testing.T) {
	original, err := Load(context.Background(), writeConfig(t, teacherDocument),
		[]string{"COIMNET_SEED=9"}, []string{"time.step=0.5"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	encoded, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	again, err := Load(context.Background(), writeConfig(t, string(encoded)), nil, nil)
	if err != nil {
		t.Fatalf("reloading the expanded document failed: %v", err)
	}
	if !reflect.DeepEqual(again.Config, original.Config) {
		t.Fatalf("round trip changed the configuration:\n%+v\n%+v", again.Config, original.Config)
	}
	if !reflect.DeepEqual(again.Secrets, original.Secrets) {
		t.Fatalf("round trip changed the secrets: %+v vs %+v", again.Secrets, original.Secrets)
	}
	// Provenance is deliberately not preserved: every leaf of the expanded
	// document was written by that file, so reloading reports file.
	for pointer, source := range again.Provenance {
		if source != SourceFile {
			t.Fatalf("provenance[%s] = %q after reloading an expanded document, want %q", pointer, source, SourceFile)
		}
	}
	if again.Provenance["/seed"] == original.Provenance["/seed"] {
		t.Fatal("the round trip kept the original provenance, which it cannot know")
	}
}

func TestDefaultsAreValidatedAsFarAsTheyCanBe(t *testing.T) {
	defaults := Defaults()
	if defaults.SchemaVersion != SchemaVersion {
		t.Fatalf("Defaults().SchemaVersion = %q", defaults.SchemaVersion)
	}
	if defaults.Model.Package != "" || defaults.Model.Graph != "" {
		t.Fatal("Defaults() names a model file, which no default can know")
	}
	if defaults.Modulation != nil || defaults.Teacher != nil {
		t.Fatal("Defaults() enables an optional section")
	}
	if sum := defaults.Splits.Train + defaults.Splits.Validation + defaults.Splits.Test; math.Abs(sum-1) > 1e-9 {
		t.Fatalf("default splits sum to %v, want 1: %+v", sum, defaults.Splits)
	}
}

func TestTaskGeneratorRegistryResolvesWhatExistsToday(t *testing.T) {
	for _, name := range []string{"delayed-correlation/v1", "lif-threshold/v1"} {
		generator, err := Generator(name)
		if err != nil {
			t.Fatalf("Generator(%q) error = %v", name, err)
		}
		if generator.Name+"/"+generator.Version != name {
			t.Fatalf("generator %+v does not identify itself as %q", generator, name)
		}
		if generator.Run == nil {
			t.Fatalf("generator %q has no runner", name)
		}
	}
	if _, err := Generator("no-such-task/v1"); err == nil {
		t.Fatal("Generator accepted an unknown name")
	}
}

// TestSchemaDocumentListsEveryTopLevelKey keeps docs/config-schema.md and the
// Go struct from drifting apart: every top level key of a marshalled default
// configuration has to appear in the document that describes it.
func TestSchemaDocumentListsEveryTopLevelKey(t *testing.T) {
	encoded, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatalf("marshal Defaults(): %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatalf("read back Defaults(): %v", err)
	}
	document, err := os.ReadFile(filepath.Join("..", "docs", "config-schema.md"))
	if err != nil {
		t.Fatalf("read the schema document: %v", err)
	}
	text := string(document)
	for key := range keys {
		if !strings.Contains(text, "`"+key+"`") {
			t.Fatalf("docs/config-schema.md does not document the top level key %q", key)
		}
	}
	for _, name := range GeneratorNames() {
		if !strings.Contains(text, name) {
			t.Fatalf("docs/config-schema.md does not list the task generator %q", name)
		}
	}
}
