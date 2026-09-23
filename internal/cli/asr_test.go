package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type asrFixtureReportTest struct {
	SchemaVersion string `json:"schema_version"`
	DataScope     struct {
		Data           string `json:"data"`
		Language       string `json:"language"`
		SequenceLength string `json:"sequence_length"`
		Periphery      string `json:"periphery"`
	} `json:"data_scope"`
	TrainSamples int               `json:"train_samples"`
	TestSamples  int               `json:"test_samples"`
	Before       asrEvalReportTest `json:"before"`
	After        asrEvalReportTest `json:"after"`
	Streaming    struct {
		CER          asrEditReportTest `json:"cer"`
		WER          asrEditReportTest `json:"wer"`
		Samples      int               `json:"samples"`
		Measurements []json.RawMessage `json:"measurements"`
	} `json:"streaming"`
	SplitSeed   uint64   `json:"split_seed"`
	Epochs      int      `json:"epochs"`
	Limitations []string `json:"limitations"`
}

type asrEvalReportTest struct {
	CER     asrEditReportTest `json:"cer"`
	WER     asrEditReportTest `json:"wer"`
	Samples int               `json:"samples"`
}

type asrEditReportTest struct {
	Rate    float64 `json:"rate"`
	Defined bool    `json:"defined"`
}

func TestExamplesRunASRWritesFixtureReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "asr.json")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "asr", "--out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("examples run asr --out %s: %v; stderr=%s", out, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read --out %s: %v", out, err)
	}
	var report asrFixtureReportTest
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, data)
	}
	if report.SchemaVersion != "coimnet-asr-fixture/v1" {
		t.Fatalf("schema_version = %q, want %q", report.SchemaVersion, "coimnet-asr-fixture/v1")
	}
	if report.TrainSamples != 30 || report.TestSamples != 10 {
		t.Fatalf("split counts = train %d/test %d, want 30/10", report.TrainSamples, report.TestSamples)
	}
	if report.SplitSeed != 42 {
		t.Fatalf("split_seed = %d, want 42", report.SplitSeed)
	}
	if report.Epochs != 10 {
		t.Fatalf("epochs = %d, want 10", report.Epochs)
	}
	if !strings.Contains(report.DataScope.Data, "synthetic") || !strings.Contains(report.DataScope.Language, "3") || report.DataScope.SequenceLength == "" || report.DataScope.Periphery == "" {
		t.Fatalf("data_scope does not declare fixture bounds: %+v", report.DataScope)
	}
	for name, eval := range map[string]asrEvalReportTest{"before": report.Before, "after": report.After} {
		if eval.Samples != 10 {
			t.Errorf("%s.samples = %d, want 10", name, eval.Samples)
		}
		if !eval.CER.Defined || !eval.WER.Defined {
			t.Errorf("%s CER/WER must be defined: %+v", name, eval)
		}
		if math.IsNaN(eval.CER.Rate) || math.IsNaN(eval.WER.Rate) {
			t.Errorf("%s CER/WER contains NaN: %+v", name, eval)
		}
	}
	if report.Streaming.Samples != 10 || len(report.Streaming.Measurements) != 10 {
		t.Fatalf("stream samples/measurements = %d/%d, want 10/10", report.Streaming.Samples, len(report.Streaming.Measurements))
	}
	if !report.Streaming.CER.Defined || !report.Streaming.WER.Defined {
		t.Fatalf("stream CER/WER must be defined: %+v", report.Streaming)
	}
	if len(report.Limitations) == 0 {
		t.Fatal("report carries no explicit limitations")
	}
	if !strings.Contains(stdout.String(), "asr:") {
		t.Fatalf("stdout has no ASR summary: %q", stdout.String())
	}
}

func TestExamplesRunASRDefaultStdoutIsJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "asr"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples run asr: %v; stderr=%s", err, stderr.String())
	}
	var report asrFixtureReportTest
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("default stdout is not one JSON document: %v", err)
	}
	if report.SchemaVersion != asrFixtureSchemaVersion || report.TestSamples != 10 {
		t.Fatalf("default stdout has unexpected report: schema=%q test=%d", report.SchemaVersion, report.TestSamples)
	}
}

func TestExamplesRunASRRejectsExistingOutAndExtraArguments(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "existing.json")
	const sentinel = "keep me"
	if err := os.WriteFile(existing, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "asr", "--out", existing}, &stdout, &stderr)
	if err == nil {
		t.Fatal("accepted an existing --out path")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("existing --out exit code = %d, want %d: %v", code, exitUsage, err)
	}
	got, readErr := os.ReadFile(existing)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != sentinel {
		t.Fatalf("existing --out was changed: %q", got)
	}

	out := filepath.Join(t.TempDir(), "asr.json")
	stdout.Reset()
	stderr.Reset()
	err = Run(context.Background(), []string{"examples", "run", "asr", "--out", out, "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("accepted an extra positional argument")
	}
	if code := ExitCode(err); code != exitUsage {
		t.Fatalf("extra argument exit code = %d, want %d: %v", code, exitUsage, err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("extra argument created output: stat error %v", statErr)
	}
}

func TestExamplesRunASRHelpAndList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "run", "asr", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: coimnet examples run asr") {
		t.Fatalf("help missing usage line:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"examples", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("examples list: %v; stderr=%s", err, stderr.String())
	}
	var entries []map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("examples list is not parseable JSON: %v\n%s", err, stdout.String())
	}
	for _, entry := range entries {
		if entry["name"] == "asr" {
			if entry["profile"] != "fixture" {
				t.Fatalf("asr profile = %q, want fixture", entry["profile"])
			}
			return
		}
	}
	t.Fatalf("examples list has no asr entry:\n%s", stdout.String())
}
