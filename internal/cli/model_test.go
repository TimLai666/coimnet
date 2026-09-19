package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/resources"
)

// cliModelInspect mirrors the model inspect report for the fields these tests
// read back from the printed JSON, so the assertions exercise the wire format
// a user sees rather than the Go struct.
type cliModelInspect struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	Topology      struct {
		Nodes       int    `json:"nodes"`
		Edges       int    `json:"edges"`
		Fingerprint string `json:"fingerprint"`
	} `json:"topology"`
	Parameters struct {
		Count   int     `json:"count"`
		Weights int     `json:"weights"`
		Bias    int     `json:"bias"`
		LogTau  int     `json:"log_tau"`
		Encoder int     `json:"encoder"`
		Readout int     `json:"readout"`
		Min     float64 `json:"min"`
		Max     float64 `json:"max"`
		Mean    float64 `json:"mean"`
	} `json:"parameters"`
	Modes struct {
		Plastic  bool   `json:"plastic"`
		Chemical bool   `json:"chemical"`
		Core     string `json:"core"`
	} `json:"modes"`
	Evidence  []string         `json:"evidence"`
	Mapping   *cliModelMapping `json:"mapping"`
	Resources resources.Report `json:"resources"`
}

// cliModelMapping reads the package-only units and compatible version block.
type cliModelMapping struct {
	Units              checkpoint.Units              `json:"units"`
	CompatibleVersions checkpoint.CompatibleVersions `json:"compatible_versions"`
}

// cliModelFixture is the fixed two-node continuous fixture both inspect kinds
// share: one edge between two nodes, one one-dimensional input and readout.
func cliModelFixture() (learning.Config, learning.Parameters) {
	c := learning.Config{
		Dynamics:     dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, Delays: []int{1}, DT: .5, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{1},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.25}, Bias: []float64{.1, -.1}, LogTau: []float64{0, 0}},
		Encoder: []float64{.5, .2},
		Readout: []float64{.8},
	}
	return c, p
}

func newCliModelIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	c, p := cliModelFixture()
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func TestModelInspectIndividual(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "individual.json")
	if err := checkpoint.SaveIndividual(context.Background(), model, newCliModelIndividual(t).Snapshot()); err != nil {
		t.Fatalf("SaveIndividual: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"model", "inspect", "--path", model}, &stdout, &stderr); err != nil {
		t.Fatalf("model inspect: %v", err)
	}
	var report cliModelInspect
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not the inspect JSON: %v\n%s", err, stdout.String())
	}
	if report.Kind != "individual_snapshot" {
		t.Fatalf("kind = %q, want individual_snapshot", report.Kind)
	}
	if report.SchemaVersion != checkpoint.IndividualSchemaVersion {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.Topology.Nodes != 2 || report.Topology.Edges != 1 {
		t.Fatalf("topology = %+v", report.Topology)
	}
	if len(report.Topology.Fingerprint) != 64 {
		t.Fatalf("fingerprint = %q", report.Topology.Fingerprint)
	}
	if report.Parameters.Count != 8 {
		t.Fatalf("parameter count = %d, want 8", report.Parameters.Count)
	}
	if report.Parameters.Weights != 1 || report.Parameters.Bias != 2 || report.Parameters.LogTau != 2 || report.Parameters.Encoder != 2 || report.Parameters.Readout != 1 {
		t.Fatalf("parameter breakdown = %+v", report.Parameters)
	}
	if report.Parameters.Min != .25 || report.Parameters.Max != .25 || report.Parameters.Mean != .25 {
		t.Fatalf("weight statistics = %+v", report.Parameters)
	}
	if report.Modes.Core != "continuous" || report.Modes.Plastic || report.Modes.Chemical {
		t.Fatalf("modes = %+v", report.Modes)
	}
	if report.Mapping != nil {
		t.Fatalf("individual report carries a mapping: %+v", report.Mapping)
	}
	if len(report.Evidence) != 0 {
		t.Fatalf("individual report carries evidence: %v", report.Evidence)
	}
	if report.Resources.TotalBytes == 0 {
		t.Fatal("resource estimate came back with zero bytes")
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestModelInspectPackage(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.coimpkg")
	c, p := cliModelFixture()
	pkg, err := checkpoint.NewModelPackage(c, p, checkpoint.Units{TimeStep: "model_step", TimeConstant: "model_step"}, []string{"evidence/NAT-01/verification.json"})
	if err != nil {
		t.Fatalf("NewModelPackage: %v", err)
	}
	if err := checkpoint.SaveModelPackage(context.Background(), model, pkg); err != nil {
		t.Fatalf("SaveModelPackage: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"model", "inspect", "--path", model}, &stdout, &stderr); err != nil {
		t.Fatalf("model inspect: %v", err)
	}
	var report cliModelInspect
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not the inspect JSON: %v\n%s", err, stdout.String())
	}
	if report.Kind != "model_package" {
		t.Fatalf("kind = %q, want model_package", report.Kind)
	}
	if report.SchemaVersion != checkpoint.ModelPackageSchemaVersion {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.Topology.Nodes != 2 || report.Topology.Edges != 1 {
		t.Fatalf("topology = %+v", report.Topology)
	}
	if report.Parameters.Count != 8 {
		t.Fatalf("parameter count = %d, want 8", report.Parameters.Count)
	}
	if report.Modes.Core != "continuous" || report.Modes.Plastic || report.Modes.Chemical {
		t.Fatalf("modes = %+v", report.Modes)
	}
	if len(report.Evidence) != 1 || report.Evidence[0] != "evidence/NAT-01/verification.json" {
		t.Fatalf("evidence = %v", report.Evidence)
	}
	if report.Mapping == nil {
		t.Fatal("package report carries no mapping")
	}
	if report.Mapping.Units.TimeStep != "model_step" || report.Mapping.Units.TimeConstant != "model_step" {
		t.Fatalf("mapping units = %+v", report.Mapping.Units)
	}
	found := false
	for _, version := range report.Mapping.CompatibleVersions.Individual {
		if version == checkpoint.IndividualSchemaVersion {
			found = true
		}
	}
	if !found {
		t.Fatalf("mapping compatible versions = %+v", report.Mapping.CompatibleVersions)
	}
	if report.Resources.TotalBytes == 0 {
		t.Fatal("resource estimate came back with zero bytes")
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestModelValidateAcceptsAndRejects(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	if err := checkpoint.SaveIndividual(context.Background(), valid, newCliModelIndividual(t).Snapshot()); err != nil {
		t.Fatalf("SaveIndividual: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"model", "validate", "--path", valid}, &stdout, &stderr); err != nil {
		t.Fatalf("valid document refused: %v", err)
	}
	var report struct {
		Valid         bool   `json:"valid"`
		SchemaVersion string `json:"schema_version"`
		Checksum      string `json:"checksum"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not the validation JSON: %v\n%s", err, stdout.String())
	}
	if !report.Valid {
		t.Fatal("valid document reported invalid")
	}
	if report.SchemaVersion != checkpoint.IndividualSchemaVersion {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if len(report.Checksum) != 64 {
		t.Fatalf("checksum = %q", report.Checksum)
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	data, err := os.ReadFile(valid)
	if err != nil {
		t.Fatal(err)
	}
	ones := append([]byte(nil), data...)
	marker := `"checksum":"`
	idx := bytes.LastIndex(ones, []byte(marker))
	if idx < 0 {
		t.Fatal("saved document has no checksum field")
	}
	hexStart := idx + len(marker)
	ones[hexStart] = modelFlipHex(ones[hexStart])
	if err := os.WriteFile(corrupt, ones, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	err = Run(context.Background(), []string{"model", "validate", "--path", corrupt}, &stdout, &stderr)
	if err == nil {
		t.Fatal("one flipped byte accepted")
	}
	if ExitCode(err) == 0 {
		t.Fatalf("flipped byte exit code = 0: %v", err)
	}
	if !strings.Contains(err.Error(), "checksum") && !strings.Contains(err.Error(), "sha") {
		t.Fatalf("error %q names neither checksum nor sha", err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"model", "validate", "--path", filepath.Join(dir, "missing.json")}, &stdout, &stderr); err == nil {
		t.Fatal("missing path accepted")
	}
}

// modelFlipHex writes a different hex digit in place of b, so the flipped
// byte keeps the document valid JSON while changing the stored checksum.
func modelFlipHex(b byte) byte {
	if b == '0' {
		return '1'
	}
	return '0'
}

func TestModelHelp(t *testing.T) {
	for _, args := range [][]string{
		{"model", "inspect", "--help"},
		{"model", "validate", "--help"},
	} {
		var stdout, stderr bytes.Buffer
		if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		for _, want := range []string{"Usage:", "--path", "Example:", "Errors:", "Options"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("%v help missing %q:\n%s", args, want, stdout.String())
			}
		}
	}
}

func TestModelRejectsUsage(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"inspect without path", []string{"model", "inspect"}},
		{"validate without path", []string{"model", "validate"}},
		{"inspect positional", []string{"model", "inspect", "--path", "model.json", "extra"}},
		{"validate positional", []string{"model", "validate", "--path", "model.json", "extra"}},
		{"unknown subcommand", []string{"model", "ablate", "--path", "model.json"}},
	}
	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), tc.args, &stdout, &stderr)
		if err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
		if code := ExitCode(err); code != exitUsage {
			t.Fatalf("%s: exit code = %d, want %d: %v", tc.name, code, exitUsage, err)
		}
	}
}
