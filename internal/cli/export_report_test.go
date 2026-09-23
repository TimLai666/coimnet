package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/checkpoint"
)

// exportReportRun runs one command and returns stdout, so every case asserts on
// the wire output a user sees rather than on an internal call.
func exportReportRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), args, &stdout, &stderr)
	return stdout.String(), err
}

// exportReportWrite publishes one fixture file under the test directory.
func exportReportWrite(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
	return path
}

// exportReportRead returns the published file, failing the test when the
// command never wrote it.
func exportReportRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(data)
}

// TestExportModelPackageToMarkdown exports a real package saved by the
// checkpoint writer, so the rendered document is built from the strict loader's
// own view of the file.
func TestExportModelPackageToMarkdown(t *testing.T) {
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
	out := filepath.Join(dir, "model.md")
	if _, err := exportReportRun(t, "export", "--in", model, "--out", out, "--format", "markdown"); err != nil {
		t.Fatalf("export markdown: %v", err)
	}
	document := exportReportRead(t, out)
	for _, want := range []string{
		checkpoint.ModelPackageSchemaVersion,
		pkg.Topology.SHA256,
		"model_step",
		"evidence/NAT-01/verification.json",
		"| count |",
		"| mean |",
	} {
		if !strings.Contains(document, want) {
			t.Fatalf("markdown export is missing %q:\n%s", want, document)
		}
	}
}

// TestExportReportJSONSortsKeys checks the declared ordering guarantee: the
// JSON export sorts object keys, so two exports of the same report compare
// byte for byte whatever order the source file used.
func TestExportReportJSONSortsKeys(t *testing.T) {
	dir := t.TempDir()
	in := exportReportWrite(t, filepath.Join(dir, "report.json"), `{"schema_version":"x/v1","b":1,"a":2}`)
	out := filepath.Join(dir, "sorted.json")
	if _, err := exportReportRun(t, "export", "--in", in, "--out", out, "--format", "json"); err != nil {
		t.Fatalf("export json: %v", err)
	}
	document := exportReportRead(t, out)
	first, second, third := strings.Index(document, `"a"`), strings.Index(document, `"b"`), strings.Index(document, `"schema_version"`)
	if first < 0 || second < 0 || third < 0 {
		t.Fatalf("export is missing one of the three keys:\n%s", document)
	}
	if !(first < second && second < third) {
		t.Fatalf("keys are not sorted (a=%d b=%d schema_version=%d):\n%s", first, second, third, document)
	}
	var round map[string]any
	if err := json.Unmarshal([]byte(document), &round); err != nil {
		t.Fatalf("export is not valid JSON: %v\n%s", err, document)
	}
	if round["schema_version"] != "x/v1" {
		t.Fatalf("schema_version = %v", round["schema_version"])
	}
}

// TestExportRejectsFormatAndMissingSchema covers the two refusals a caller can
// hit before anything is written: an unsupported format and a document this
// project does not recognise.
func TestExportRejectsFormatAndMissingSchema(t *testing.T) {
	dir := t.TempDir()
	in := exportReportWrite(t, filepath.Join(dir, "report.json"), `{"schema_version":"x/v1"}`)
	out := filepath.Join(dir, "out.txt")
	_, err := exportReportRun(t, "export", "--in", in, "--out", out, "--format", "yaml")
	if err == nil {
		t.Fatal("an unsupported format was accepted")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if !strings.Contains(err.Error(), "supported: json, markdown") {
		t.Fatalf("error does not list the supported formats: %v", err)
	}
	if _, statErr := os.Lstat(out); statErr == nil {
		t.Fatal("the refused export still wrote its output file")
	}

	bare := exportReportWrite(t, filepath.Join(dir, "bare.json"), `{"a":1}`)
	_, err = exportReportRun(t, "export", "--in", bare, "--out", filepath.Join(dir, "bare.out.json"), "--format", "json")
	if err == nil {
		t.Fatal("a document without schema_version was accepted")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error does not name the missing field: %v", err)
	}
}

// reportDocumentJSON mirrors the published report for the fields these tests
// read back.
type reportDocumentJSON struct {
	SchemaVersion string          `json:"schema_version"`
	GeneratedAt   string          `json:"generated_at"`
	Settings      json.RawMessage `json:"settings"`
	Completion    struct {
		Passed    int            `json:"passed"`
		Blocked   int            `json:"blocked"`
		Specified int            `json:"specified"`
		Total     int            `json:"total"`
		ByStatus  map[string]int `json:"by_status"`
	} `json:"completion"`
	Requirements []struct {
		ID              string   `json:"id"`
		Status          string   `json:"status"`
		Evidence        []string `json:"evidence"`
		EvidencePresent bool     `json:"evidence_present"`
	} `json:"requirements"`
	Scores []struct {
		ID             string `json:"id"`
		ObservedResult string `json:"observed_result"`
	} `json:"scores"`
	Limitations []struct {
		ID          string   `json:"id"`
		Limitations []string `json:"limitations"`
	} `json:"limitations"`
	Sources []json.RawMessage `json:"sources"`
}

// TestReportBuildsCompletionFromStatus builds the report from a hand-written
// status document and one evidence record laid out the way the repository lays
// them out, so the relative evidence paths are resolved the same way.
func TestReportBuildsCompletionFromStatus(t *testing.T) {
	dir := t.TempDir()
	status := exportReportWrite(t, filepath.Join(dir, "docs", "requirements-status.json"), `{
  "schema_version": "coimnet-requirements-status/v1",
  "definition": "handoff/requirements.json",
  "requirements": [
    {"id": "REQ-A", "status": "passed", "evidence": ["../evidence/REQ-A/verification.json"]},
    {"id": "REQ-B", "status": "specified", "evidence": []},
    {"id": "REQ-C", "status": "blocked", "evidence": ["../evidence/REQ-C/verification.json"]}
  ]
}`)
	observed := strings.Repeat("o", 420)
	exportReportWrite(t, filepath.Join(dir, "evidence", "REQ-A", "verification.json"), `{
  "requirement": "REQ-A",
  "profile": "fixture",
  "reproduction_command": ["go test ./internal/cli/"],
  "input_fingerprints": ["sha256:0011"],
  "observed_result": "`+observed+`",
  "limitations": ["only the two-node fixture"]
}`)
	outJSON := filepath.Join(dir, "report.json")
	outMD := filepath.Join(dir, "report.md")
	if _, err := exportReportRun(t, "report", "--status", status, "--evidence-dir", filepath.Join(dir, "evidence"), "--out-json", outJSON, "--out-md", outMD); err != nil {
		t.Fatalf("report: %v", err)
	}
	var document reportDocumentJSON
	if err := json.Unmarshal([]byte(exportReportRead(t, outJSON)), &document); err != nil {
		t.Fatalf("published report is not valid JSON: %v", err)
	}
	if document.SchemaVersion != "coimnet-report/v1" {
		t.Fatalf("schema_version = %q", document.SchemaVersion)
	}
	if _, err := time.Parse(time.RFC3339, document.GeneratedAt); err != nil {
		t.Fatalf("generated_at %q is not RFC3339: %v", document.GeneratedAt, err)
	}
	if string(document.Settings) != "null" {
		t.Fatalf("settings = %s, want null when --settings is omitted", document.Settings)
	}
	completion := document.Completion
	if completion.Passed != 1 || completion.Blocked != 1 || completion.Specified != 1 || completion.Total != 3 {
		t.Fatalf("completion = %+v", completion)
	}
	if completion.ByStatus["passed"] != 1 || completion.ByStatus["blocked"] != 1 || completion.ByStatus["specified"] != 1 {
		t.Fatalf("by_status = %v", completion.ByStatus)
	}
	if len(document.Requirements) != 3 {
		t.Fatalf("requirements = %d, want 3", len(document.Requirements))
	}
	if document.Requirements[0].ID != "REQ-A" || !document.Requirements[0].EvidencePresent {
		t.Fatalf("REQ-A entry = %+v", document.Requirements[0])
	}
	if document.Requirements[1].EvidencePresent || document.Requirements[2].EvidencePresent {
		t.Fatalf("a requirement without a readable record reports evidence_present: %+v", document.Requirements[1:])
	}
	if len(document.Scores) != 1 || document.Scores[0].ID != "REQ-A" {
		t.Fatalf("scores = %+v", document.Scores)
	}
	if got := len([]rune(document.Scores[0].ObservedResult)); got != 300 {
		t.Fatalf("observed_result kept %d characters, want the first 300", got)
	}
	if len(document.Limitations) != 1 || len(document.Limitations[0].Limitations) != 1 {
		t.Fatalf("limitations = %+v", document.Limitations)
	}
	if len(document.Sources) != 1 || !strings.Contains(string(document.Sources[0]), "sha256:0011") {
		t.Fatalf("sources = %v", document.Sources)
	}
	markdown := exportReportRead(t, outMD)
	for _, want := range []string{"| ID | Status | Evidence present |", "| REQ-A | passed | yes |", "| passed | 1 |", "only the two-node fixture"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown report is missing %q:\n%s", want, markdown)
		}
	}
}

// TestReportRejects covers the refusals: a missing status document, an existing
// output path and the help text, which is not an error.
func TestReportRejects(t *testing.T) {
	dir := t.TempDir()
	status := exportReportWrite(t, filepath.Join(dir, "docs", "requirements-status.json"), `{"schema_version":"coimnet-requirements-status/v1","requirements":[]}`)
	cases := [][]string{
		{"report", "--out-json", filepath.Join(dir, "a.json"), "--out-md", filepath.Join(dir, "a.md")},
		{"report", "--status", filepath.Join(dir, "missing.json"), "--out-json", filepath.Join(dir, "b.json"), "--out-md", filepath.Join(dir, "b.md")},
	}
	for _, args := range cases {
		_, err := exportReportRun(t, args...)
		if err == nil {
			t.Fatalf("%v was accepted", args)
		}
		if code := ExitCode(err); code != exitUsage {
			t.Fatalf("%v: exit code = %d, want %d: %v", args, code, exitUsage, err)
		}
	}
	existing := exportReportWrite(t, filepath.Join(dir, "taken.json"), "{}")
	_, err := exportReportRun(t, "report", "--status", status, "--out-json", existing, "--out-md", filepath.Join(dir, "c.md"))
	if err == nil {
		t.Fatal("an existing --out-json was accepted")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("existing --out-json: exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if got := exportReportRead(t, existing); got != "{}" {
		t.Fatalf("the refused report overwrote its output: %q", got)
	}
	help, err := exportReportRun(t, "report", "--help")
	if err != nil {
		t.Fatalf("report --help: %v", err)
	}
	if !strings.Contains(help, "Usage: coimnet report") {
		t.Fatalf("report --help does not print its usage:\n%s", help)
	}
}

func TestReportCountsEveryBlockedKind(t *testing.T) {
	dir := t.TempDir()
	status := exportReportWrite(t, filepath.Join(dir, "docs", "requirements-status.json"), `{
  "schema_version": "coimnet-requirements-status/v1",
  "requirements": [
    {"id": "REQ-A", "status": "passed", "evidence": ["../evidence/REQ-A/verification.json"]},
    {"id": "REQ-B", "status": "specified", "evidence": []},
    {"id": "REQ-C", "status": "blocked_data", "evidence": []},
    {"id": "REQ-D", "status": "blocked_hardware", "evidence": []},
    {"id": "REQ-E", "status": "blocked_permission", "evidence": []}
  ]
}`)
	exportReportWrite(t, filepath.Join(dir, "evidence", "REQ-A", "verification.json"), `{"observed_result":"passed"}`)
	outJSON := filepath.Join(dir, "report.json")
	outMD := filepath.Join(dir, "report.md")
	if _, err := exportReportRun(t, "report", "--status", status, "--evidence-dir", filepath.Join(dir, "evidence"), "--out-json", outJSON, "--out-md", outMD); err != nil {
		t.Fatalf("report: %v", err)
	}
	var document reportDocumentJSON
	if err := json.Unmarshal([]byte(exportReportRead(t, outJSON)), &document); err != nil {
		t.Fatalf("published report is not valid JSON: %v", err)
	}
	completion := document.Completion
	if completion.Passed != 1 || completion.Blocked != 3 || completion.Specified != 1 || completion.Total != 5 {
		t.Fatalf("completion = %+v", completion)
	}
	for _, status := range []string{"blocked_data", "blocked_hardware", "blocked_permission"} {
		if completion.ByStatus[status] != 1 {
			t.Errorf("by_status[%q] = %d, want 1", status, completion.ByStatus[status])
		}
	}
	markdown := exportReportRead(t, outMD)
	if !strings.Contains(markdown, "| blocked | 3 (blocked_data 1, blocked_hardware 1, blocked_permission 1) |") {
		t.Fatalf("markdown is missing the blocked breakdown:\n%s", markdown)
	}
	for _, line := range strings.Split(markdown, "\n") {
		for _, status := range []string{"blocked_data", "blocked_hardware", "blocked_permission"} {
			if strings.HasPrefix(line, "| "+status+" |") {
				t.Errorf("markdown has a separate row for %q: %s", status, line)
			}
		}
	}
}
