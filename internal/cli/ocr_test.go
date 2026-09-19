package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ocrFixtureReport is the slice of the OCR fixture report the CLI tests read
// back from the --out file. It mirrors ocr.OCRReport for the fields these
// tests assert, so a field rename in tasks/ocr shows up here as a failure.
type ocrFixtureReport struct {
	SchemaVersion string `json:"schema_version"`
	DataScope     struct {
		Data           string `json:"data"`
		Language       string `json:"language"`
		Resolution     string `json:"resolution"`
		SequenceLength string `json:"sequence_length"`
		Periphery      string `json:"periphery"`
	} `json:"data_scope"`
	Seeds []struct {
		Seed   uint64 `json:"seed"`
		Failed bool   `json:"failed"`
		Error  string `json:"error"`
		Before struct {
			CER struct {
				Rate    float64 `json:"rate"`
				Defined bool    `json:"defined"`
			} `json:"cer"`
			Samples int `json:"samples"`
		} `json:"before"`
		After struct {
			CER struct {
				Rate    float64 `json:"rate"`
				Defined bool    `json:"defined"`
			} `json:"cer"`
			Samples int `json:"samples"`
		} `json:"after"`
	} `json:"seeds"`
	CoreDisconnect struct {
		OutputChanged bool    `json:"output_changed"`
		MaxAbsDelta   float64 `json:"max_abs_delta"`
	} `json:"core_disconnect"`
	Assumptions []string `json:"assumptions"`
}

// ocrSummaryLine is the exact one-line stdout summary the example promises.
var ocrSummaryLine = regexp.MustCompile(`^ocr: 3 seeds, held-out CER [0-9]+\.[0-9]+ -> [0-9]+\.[0-9]+, core disconnect (true|false)\n$`)

func TestExamplesRunOCRWritesReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "ocr.json")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "ocr", "--out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("examples run ocr --out %s: %v; stderr=%s", out, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read --out %s: %v", out, err)
	}
	var report ocrFixtureReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, data)
	}

	if report.SchemaVersion != "coimnet-ocr-fixture/v1" {
		t.Fatalf("schema_version = %q, want %q", report.SchemaVersion, "coimnet-ocr-fixture/v1")
	}
	if len(report.Seeds) != 3 {
		t.Fatalf("seeds = %d, want 3", len(report.Seeds))
	}
	for _, s := range report.Seeds {
		if s.Failed {
			t.Fatalf("seed %d failed: %s", s.Seed, s.Error)
		}
		if s.After.Samples == 0 {
			t.Fatalf("seed %d scored 0 held-out samples", s.Seed)
		}
	}
	if !report.CoreDisconnect.OutputChanged {
		t.Fatalf("core_disconnect.output_changed = false, max_abs_delta = %v; the readout bypassed the core", report.CoreDisconnect.MaxAbsDelta)
	}
	if !strings.Contains(report.DataScope.Language, "3 runes") {
		t.Fatalf("data_scope.language = %q, want it to name the 3-rune alphabet", report.DataScope.Language)
	}
	if len(report.Assumptions) == 0 {
		t.Fatal("report carries no assumptions")
	}
	if !ocrSummaryLine.MatchString(stdout.String()) {
		t.Fatalf("summary line does not match %s:\n%q", ocrSummaryLine, stdout.String())
	}
}

func TestExamplesRunOCRRejects(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "base.json")
	for _, args := range [][]string{
		{"--out", existing},
		{"--out", base, "extra"},
		{"--out", filepath.Join(base, "nested.json")},
	} {
		var stdout, stderr bytes.Buffer
		full := append([]string{"examples", "run", "ocr"}, args...)
		err := Run(context.Background(), full, &stdout, &stderr)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
		if code := ExitCode(err); code != exitUsage {
			t.Fatalf("%v: exit code = %d, want %d: %v", args, code, exitUsage, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "ocr", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: coimnet examples run ocr") {
		t.Fatalf("help missing usage line:\n%s", stdout.String())
	}
}

func TestExamplesListIncludesOCR(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v %s", err, stderr.String())
	}
	var entries []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("examples list is not parseable JSON: %v\n%s", err, stdout.String())
	}
	var found map[string]string
	for _, e := range entries {
		if e["name"] == "ocr" {
			found = e
		}
	}
	if found == nil {
		t.Fatalf("examples list has no ocr entry:\n%s", stdout.String())
	}
	if found["profile"] != "fixture" {
		t.Fatalf("ocr profile = %q, want %q", found["profile"], "fixture")
	}
	want := "Synthetic glyph line recognition: CTC-trained column reader with family split, unseen combinations and a core-disconnect check"
	if found["description"] != want {
		t.Fatalf("ocr description = %q, want %q", found["description"], want)
	}
}
