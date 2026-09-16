package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// countingTeacher stands in for the teacher client of ticket 27. A dry run must
// leave its counter at zero even when the configuration declares a teacher.
type countingTeacher struct{ calls int }

func (c *countingTeacher) Ask(ctx context.Context, question []float64) ([]float64, error) {
	c.calls++
	return nil, fmt.Errorf("the fake teacher must never be called")
}

// writeTinyModelPackage publishes a three-neuron continuous model package and
// returns its path. It is the smallest artefact the dry run can size a plan
// from: two edges, one input channel and one readout neuron.
func writeTinyModelPackage(t *testing.T, dir string) string {
	t.Helper()
	c := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 3, Sources: []int{0, 1}, Targets: []int{1, 2}, DT: .1, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{2},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.5, -.25}, Bias: make([]float64, 3), LogTau: make([]float64, 3)},
		Encoder: []float64{.1, .2, .3},
		Readout: []float64{.4},
	}
	pkg, err := checkpoint.NewModelPackage(c, p, checkpoint.Units{TimeStep: "model_step", TimeConstant: "model_step"}, nil)
	if err != nil {
		t.Fatalf("NewModelPackage() error = %v", err)
	}
	path := filepath.Join(dir, "tiny.coimpkg")
	if err := checkpoint.SaveModelPackage(context.Background(), path, pkg); err != nil {
		t.Fatalf("SaveModelPackage() error = %v", err)
	}
	return path
}

// dryRunFixture writes a package and a configuration that points at it, and
// returns the directory, the configuration path and the declared output path.
func dryRunFixture(t *testing.T, extra string) (dir, configPath, outputDir string) {
	t.Helper()
	dir = t.TempDir()
	packagePath := writeTinyModelPackage(t, dir)
	outputDir = filepath.Join(dir, "runs")
	document := fmt.Sprintf(`{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": %q},
  "output": {"dir": %q},
  "resources": {"max_memory_mib": 64, "max_temp_mib": 64, "max_runs": 1}%s
}`, packagePath, outputDir, extra)
	configPath = filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(document), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return dir, configPath, outputDir
}

func listing(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		names = append(names, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	sort.Strings(names)
	return names
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type dryRunDocument struct {
	SchemaVersion string            `json:"schema_version"`
	Config        json.RawMessage   `json:"config"`
	Provenance    map[string]string `json:"provenance"`
	Checks        []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	} `json:"checks"`
	Estimate struct {
		Items []struct {
			Name    string `json:"name"`
			Formula string `json:"formula"`
			Bytes   uint64 `json:"bytes"`
		} `json:"items"`
		TotalBytes uint64 `json:"total_bytes"`
	} `json:"estimate"`
	Verdict string `json:"verdict"`
}

func decodeDryRun(t *testing.T, out *bytes.Buffer) dryRunDocument {
	t.Helper()
	var document dryRunDocument
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatalf("dry run output is not the declared JSON: %v\n%s", err, out)
	}
	if document.SchemaVersion != "coimnet-dry-run/v1" {
		t.Fatalf("schema_version = %q", document.SchemaVersion)
	}
	return document
}

func checkStatus(t *testing.T, document dryRunDocument, name string) (string, string) {
	t.Helper()
	for _, c := range document.Checks {
		if c.Name == name {
			return c.Status, c.Detail
		}
	}
	t.Fatalf("no check named %q; checks = %+v", name, document.Checks)
	return "", ""
}

func TestRunDryRunReportsAnEstimateAndPassesEveryCheck(t *testing.T) {
	dir, configPath, outputDir := dryRunFixture(t, "")
	before := listing(t, dir)
	digest := fileDigest(t, configPath)

	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"run", "--config", configPath, "--dry-run"}, &out, &stderr)
	if err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, &stderr)
	}
	if code := ExitCode(err); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	document := decodeDryRun(t, &out)
	if document.Verdict != "ok" {
		t.Fatalf("verdict = %q, want ok:\n%s", document.Verdict, &out)
	}
	for _, c := range document.Checks {
		if c.Status != "ok" && c.Status != "skipped" {
			t.Fatalf("check %q = %q: %s", c.Name, c.Status, c.Detail)
		}
		if c.Detail == "" {
			t.Fatalf("check %q has no detail", c.Name)
		}
	}
	if status, detail := checkStatus(t, document, "model"); status != "ok" || !strings.Contains(detail, "2") {
		t.Fatalf("model check = %q %q, want an ok status naming the two edges", status, detail)
	}
	if _, detail := checkStatus(t, document, "resources"); !strings.Contains(detail, "64") {
		t.Fatalf("resource check detail %q does not name the declared limit", detail)
	}
	if document.Estimate.TotalBytes == 0 || len(document.Estimate.Items) == 0 {
		t.Fatalf("estimate is empty:\n%s", &out)
	}
	for _, item := range document.Estimate.Items {
		if item.Formula == "" {
			t.Fatalf("estimate item %q has no formula", item.Name)
		}
	}
	if document.Provenance["/model/package"] != "file" {
		t.Fatalf("provenance for the model path = %q", document.Provenance["/model/package"])
	}
	if len(document.Config) == 0 {
		t.Fatal("the report does not carry the expanded configuration")
	}

	// Zero side effects.
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Fatalf("the dry run created the output directory: %v", err)
	}
	if got := listing(t, dir); !equalStrings(got, before) {
		t.Fatalf("the dry run changed the working directory:\nbefore %v\nafter  %v", before, got)
	}
	if fileDigest(t, configPath) != digest {
		t.Fatal("the dry run rewrote the configuration file")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunDryRunNeverCallsTheTeacher(t *testing.T) {
	_, configPath, _ := dryRunFixture(t, `,
  "teacher": {"kind": "http/v1", "endpoint": "https://teacher.invalid/answer", "budget": {"max_requests": 3, "max_cost": 0.5}, "secret": {"ref": "env:COIMNET_TEST_TEACHER_TOKEN"}}`)
	teacher := &countingTeacher{}
	var out, stderr bytes.Buffer
	if err := runConfigRunWith(context.Background(), []string{"--config", configPath, "--dry-run"}, &out, &stderr, teacher); err != nil {
		t.Fatalf("dry run with a declared teacher failed: %v\n%s", err, &stderr)
	}
	if teacher.calls != 0 {
		t.Fatalf("the dry run called the teacher %d times", teacher.calls)
	}
	document := decodeDryRun(t, &out)
	status, detail := checkStatus(t, document, "teacher")
	if status != "skipped" {
		t.Fatalf("teacher check = %q, want skipped", status)
	}
	if !strings.Contains(detail, "never called") {
		t.Fatalf("teacher detail %q does not say the teacher was not called", detail)
	}
	if strings.Contains(out.String(), "COIMNET_TEST_TEACHER_TOKEN=") {
		t.Fatal("the report carries an environment assignment")
	}
}

func TestRunDryRunRefusesWhenTheEstimateExceedsTheLimit(t *testing.T) {
	dir, configPath, outputDir := dryRunFixture(t, "")
	document, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	// The fixture needs a few hundred bytes, so any whole mebibyte would pass.
	// Zero is the declared way to say that no plan is affordable.
	tiny := strings.Replace(string(document), `"max_memory_mib": 64`, `"max_memory_mib": 0`, 1)
	tinyPath := filepath.Join(dir, "tiny-limit.json")
	if err := os.WriteFile(tinyPath, []byte(tiny), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	before := listing(t, dir)
	digest := fileDigest(t, tinyPath)

	var out, stderr bytes.Buffer
	err = Run(context.Background(), []string{"run", "--config", tinyPath, "--dry-run"}, &out, &stderr)
	if err == nil {
		t.Fatal("the dry run accepted a plan over its declared limit")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	report := decodeDryRun(t, &out)
	if report.Verdict != "refused" {
		t.Fatalf("verdict = %q, want refused", report.Verdict)
	}
	if status, _ := checkStatus(t, report, "resources"); status != "failed" {
		t.Fatalf("resource check = %q, want failed", status)
	}
	if report.Estimate.TotalBytes == 0 {
		t.Fatal("a refusal still has to print the estimate it refused on")
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Fatal("the refused dry run created the output directory")
	}
	if got := listing(t, dir); !equalStrings(got, before) {
		t.Fatalf("the refused dry run changed the working directory:\nbefore %v\nafter  %v", before, got)
	}
	if fileDigest(t, tinyPath) != digest {
		t.Fatal("the refused dry run rewrote the configuration it refused")
	}
}

func TestRunDryRunCountsPlasticEdgesFromTheDeclaredRules(t *testing.T) {
	_, configPath, _ := dryRunFixture(t, `,
  "learning": {"rules": ["gradient", "hebbian_rate"]}`)
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"run", "--config", configPath, "--dry-run"}, &out, &stderr); err != nil {
		t.Fatalf("dry run with a local rule failed: %v\n%s", err, &stderr)
	}
	document := decodeDryRun(t, &out)
	// The fixture package has two edges at float64, so a local rule adds one
	// fast change and one eligibility value per edge: 2*8 bytes each.
	for _, name := range []string{"fast_changes", "eligibility"} {
		var bytesFor uint64
		var formula string
		for _, item := range document.Estimate.Items {
			if item.Name == name {
				bytesFor, formula = item.Bytes, item.Formula
			}
		}
		if bytesFor != 16 {
			t.Fatalf("%s = %d, want 16 (%s)", name, bytesFor, formula)
		}
	}
}

func TestRunDryRunFailsTheCheckThatNamesAMissingModel(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.coimpkg")
	configPath := filepath.Join(dir, "config.json")
	document := fmt.Sprintf(`{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": %q},
  "output": {"dir": %q}
}`, missing, filepath.Join(dir, "runs"))
	if err := os.WriteFile(configPath, []byte(document), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"run", "--config", configPath, "--dry-run"}, &out, &stderr)
	if err == nil {
		t.Fatal("the dry run accepted a configuration whose model file is absent")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	report := decodeDryRun(t, &out)
	if report.Verdict != "refused" {
		t.Fatalf("verdict = %q, want refused", report.Verdict)
	}
	status, detail := checkStatus(t, report, "model")
	if status != "failed" || !strings.Contains(detail, missing) {
		t.Fatalf("model check = %q %q, want a failure naming %q", status, detail, missing)
	}
}

func TestRunRejectsUsageErrorsWithStatusOne(t *testing.T) {
	dir, configPath, _ := dryRunFixture(t, "")
	for _, args := range [][]string{
		{"run"},
		{"run", "--dry-run"},
		{"run", "--config"},
		{"run", "--config", configPath, "--dry-run", "extra"},
		{"run", "--config", configPath, "--dry-run", "--set", "no-equals-sign"},
		{"run", "--config", filepath.Join(dir, "absent-config.json"), "--dry-run"},
		{"run", "--config", configPath, "--dry-run", "--typo"},
	} {
		var out, stderr bytes.Buffer
		err := Run(context.Background(), args, &out, &stderr)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
		if code := ExitCode(err); code != 1 {
			t.Fatalf("%v: exit code = %d, want 1", args, code)
		}
	}
}

func TestRunWithoutDryRunSaysWhichStageImplementsIt(t *testing.T) {
	_, configPath, _ := dryRunFixture(t, "")
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"run", "--config", configPath}, &out, &stderr)
	if err == nil {
		t.Fatal("run executed a configuration in the stage that only expands it")
	}
	if !strings.Contains(err.Error(), "not implemented") || !strings.Contains(err.Error(), "train") || !strings.Contains(err.Error(), "simulate") {
		t.Fatalf("error %q does not point at train or simulate", err)
	}
}

func TestRunOverridesAreAppliedAndRecorded(t *testing.T) {
	_, configPath, _ := dryRunFixture(t, "")
	var out, stderr bytes.Buffer
	args := []string{"run", "--config", configPath, "--dry-run", "--set", "seed=99", "--set", "resources.max_runs=4"}
	if err := Run(context.Background(), args, &out, &stderr); err != nil {
		t.Fatalf("dry run with overrides failed: %v\n%s", err, &stderr)
	}
	document := decodeDryRun(t, &out)
	if document.Provenance["/seed"] != "cli" || document.Provenance["/resources/max_runs"] != "cli" {
		t.Fatalf("provenance did not record the overrides: %v", document.Provenance)
	}
	var config struct {
		Seed      uint64 `json:"seed"`
		Resources struct {
			MaxRuns int `json:"max_runs"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(document.Config, &config); err != nil {
		t.Fatalf("expanded configuration is not readable: %v", err)
	}
	if config.Seed != 99 || config.Resources.MaxRuns != 4 {
		t.Fatalf("overrides were not applied: %+v", config)
	}
}

func TestRunHelpListsEveryFlagWithDefaultsAndAnExample(t *testing.T) {
	for _, args := range [][]string{{"run", "--help"}, {"run", "-h"}} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		text := out.String()
		for _, want := range []string{"--config", "--set", "--dry-run", "Example:", "Errors:", "default"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%v help does not mention %q:\n%s", args, want, text)
			}
		}
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), nil, &out, &stderr); err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !strings.Contains(out.String(), "run ") {
		t.Fatalf("the command overview does not list run:\n%s", &out)
	}
}
