package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/checkpoint"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

// cliMigrationReport mirrors checkpoint.MigrationReport for the fields these
// tests read back from the printed JSON, so the assertions exercise the wire
// format a user sees rather than the Go struct.
type cliMigrationReport struct {
	SourcePath        string `json:"source_path"`
	TargetPath        string `json:"target_path"`
	SourceSHA256      string `json:"source_sha256"`
	TargetSHA256      string `json:"target_sha256"`
	SourceSchema      string `json:"source_schema"`
	TargetSchema      string `json:"target_schema"`
	NoInformationLoss bool   `json:"no_information_loss"`
}

// cliFieldChange mirrors checkpoint.FieldChange for the report readback.
type cliFieldChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func newCliMigrationIndividual(t *testing.T) *learning.Individual {
	t.Helper()
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
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	return individual
}

func TestCheckpointMigrateCLIWritesMigratedFileAndPrintsReport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := checkpoint.SaveIndividual(context.Background(), src, newCliMigrationIndividual(t).Snapshot()); err != nil {
		t.Fatalf("SaveIndividual: %v", err)
	}
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "migrated.json")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"checkpoint", "migrate", "--src", src, "--dst", dst}, &stdout, &stderr); err != nil {
		t.Fatalf("checkpoint migrate: %v", err)
	}
	var report cliMigrationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not the report JSON: %v\n%s", err, stdout.String())
	}
	if report.SourcePath != src || report.TargetPath != dst {
		t.Fatalf("paths = %q, %q", report.SourcePath, report.TargetPath)
	}
	if report.SourceSchema != checkpoint.IndividualSchemaVersion || report.TargetSchema != checkpoint.IndividualSchemaVersion {
		t.Fatalf("schemas = %q, %q", report.SourceSchema, report.TargetSchema)
	}
	if len(report.SourceSHA256) != 64 || report.SourceSHA256 != report.TargetSHA256 {
		t.Fatalf("hashes = %q, %q", report.SourceSHA256, report.TargetSHA256)
	}
	if !report.NoInformationLoss {
		t.Fatal("same-schema CLI migration reported information loss")
	}
	if _, err := checkpoint.LoadIndividual(context.Background(), dst); err != nil {
		t.Fatalf("migrated file is not loadable: %v", err)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, srcBytes) {
		t.Fatal("CLI migration changed the source bytes")
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCheckpointMigrateCLIUpgradesPreUnionIndividual(t *testing.T) {
	dir := t.TempDir()
	saved := filepath.Join(dir, "saved.json")
	if err := checkpoint.SaveIndividual(context.Background(), saved, newCliMigrationIndividual(t).Snapshot()); err != nil {
		t.Fatal(err)
	}
	savedBytes, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "old.json")
	preUnion := cliPreUnionDocument(t, savedBytes)
	if err := os.WriteFile(src, preUnion, 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "migrated.json")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"checkpoint", "migrate", "--src", src, "--dst", dst}, &stdout, &stderr); err != nil {
		t.Fatalf("checkpoint migrate: %v", err)
	}
	var report struct {
		SourceSHA256    string           `json:"source_sha256"`
		TargetSHA256    string           `json:"target_sha256"`
		FieldChanges    []cliFieldChange `json:"field_changes"`
		InformationLoss []string         `json:"information_loss"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not the report JSON: %v\n%s", err, stdout.String())
	}
	if len(report.FieldChanges) != 2 {
		t.Fatalf("FieldChanges = %+v", report.FieldChanges)
	}
	if report.FieldChanges[0].Path != "neural" || report.FieldChanges[0].Kind != "renamed" {
		t.Fatalf("first FieldChange = %+v", report.FieldChanges[0])
	}
	if report.FieldChanges[1].Path != "neural.core" || report.FieldChanges[1].Kind != "added" {
		t.Fatalf("second FieldChange = %+v", report.FieldChanges[1])
	}
	if len(report.InformationLoss) != 0 {
		t.Fatalf("InformationLoss = %v", report.InformationLoss)
	}
	if report.SourceSHA256 == report.TargetSHA256 {
		t.Fatalf("pre-union migration rewrote the document, so source and target must differ; both are %s", report.SourceSHA256)
	}
	got, err := checkpoint.LoadIndividual(context.Background(), dst)
	if err != nil {
		t.Fatalf("migrated document is not loadable: %v", err)
	}
	if got.Neural.Core != learning.NeuralCoreContinuous {
		t.Fatalf("migrated core = %q, want %q", got.Neural.Core, learning.NeuralCoreContinuous)
	}
	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, preUnion) {
		t.Fatal("CLI migration changed the source bytes")
	}
}

func TestCheckpointMigrateCLIRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := checkpoint.SaveIndividual(context.Background(), src, newCliMigrationIndividual(t).Snapshot()); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "occupied.json")
	if err := os.WriteFile(dst, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"checkpoint", "migrate", "--src", src, "--dst", dst}, &stdout, &stderr)
	if err == nil {
		t.Fatal("accepted an existing --dst")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if !strings.Contains(err.Error(), "--dst") {
		t.Fatalf("error %q does not name --dst", err)
	}
	occupied, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(occupied) != "occupied" {
		t.Fatal("existing --dst was modified")
	}
}

func TestCheckpointMigrateCLIRefusesSamePath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.json")
	if err := checkpoint.SaveIndividual(context.Background(), src, newCliMigrationIndividual(t).Snapshot()); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"checkpoint", "migrate", "--src", src, "--dst", src}, &stdout, &stderr)
	if err == nil {
		t.Fatal("accepted --dst equal to --src")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if !strings.Contains(err.Error(), "--dst") {
		t.Fatalf("error %q does not name --dst", err)
	}
}

func TestCheckpointMigrateCLIRequiresSrcAndDst(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "migrated.json")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"checkpoint", "migrate", "--dst", dst}, &stdout, &stderr)
	if err == nil {
		t.Fatal("accepted a command without --src")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if !strings.Contains(err.Error(), "--src") {
		t.Fatalf("error %q does not name --src", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("a refused command created --dst")
	}
}

func TestCheckpointMigrateCLIHelpListsParametersDefaultsAndErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"checkpoint", "migrate", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{
		"--src", "--dst", "--target", checkpoint.IndividualSchemaVersion,
		"Example:", "Errors:", "default",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

// cliPreUnionDocument rewrites a saved union document into the shape the
// released code wrote before the continuous core got a union: the dynamics
// state sits directly on payload.neural and there is no core declaration.
func cliPreUnionDocument(t *testing.T, saved []byte) []byte {
	t.Helper()
	var raw struct {
		SchemaVersion string          `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
		Checksum      string          `json:"checksum"`
	}
	if err := json.Unmarshal(saved, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.SchemaVersion != checkpoint.IndividualSchemaVersion {
		t.Fatalf("source schema = %q", raw.SchemaVersion)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var neural map[string]json.RawMessage
	if err := json.Unmarshal(payload["neural"], &neural); err != nil {
		t.Fatal(err)
	}
	continuous, ok := neural["continuous"]
	if !ok {
		t.Fatal("saved document has no continuous half to extract")
	}
	if _, ok := neural["core"]; !ok {
		t.Fatal("saved document is not a union")
	}
	payload["neural"] = continuous
	prePayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(prePayload)
	return []byte(`{"schema_version":"` + checkpoint.IndividualSchemaVersion + `","payload":` + string(prePayload) + `,"checksum":"` + hex.EncodeToString(sum[:]) + `"}`)
}
