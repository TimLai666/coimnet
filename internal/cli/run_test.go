package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

func TestRunDiscoverabilityAndErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"doctor", "--help"}, {"examples", "--help"}, {"examples", "run", "delayed", "--help"}} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), args, &out, &stderr); err != nil || out.Len() == 0 {
			t.Fatalf("%v: %v %s", args, err, stderr.String())
		}
	}
	for _, args := range [][]string{{"unknown"}, {"doctor", "extra"}, {"examples", "run", "unknown"}, {"examples", "run", "delayed", "--updates", "oops"}, {"examples", "run", "delayed", "--typo"}, {"examples", "run", "delayed", "extra"}} {
		var out, stderr bytes.Buffer
		if Run(context.Background(), args, &out, &stderr) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &out, &stderr); err != nil || !strings.Contains(out.String(), "fixture") {
		t.Fatalf("list: %v %s", err, &out)
	}
}

func TestRunDelayedReportsFailedLearningGate(t *testing.T) {
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "delayed", "--updates", "1"}, &out, &stderr)
	if err == nil {
		t.Fatal("one update should not meet learning gate")
	}
	var report struct {
		Passed  bool   `json:"passed"`
		Profile string `json:"profile"`
	}
	if e := json.Unmarshal(out.Bytes(), &report); e != nil || report.Passed || report.Profile != "fixture" {
		t.Fatalf("report: %v %s", e, &out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if Run(ctx, []string{"examples", "run", "delayed"}, &out, &stderr) == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestRunLIFThresholdExampleIsDiscoverableAndRuns(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"examples", "list"}, &out, &stderr); err != nil {
		t.Fatalf("list: %v %s", err, &stderr)
	}
	var listed []map[string]string
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("list is not parseable JSON: %v\n%s", err, &out)
	}
	var found map[string]string
	for _, entry := range listed {
		if entry["name"] == "lif-threshold" {
			found = entry
		}
	}
	if found == nil {
		t.Fatalf("examples list has no lif-threshold entry: %s", &out)
	}
	if found["profile"] != "fixture" || found["description"] == "" {
		t.Fatalf("lif-threshold entry = %v", found)
	}

	out.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"examples", "run", "lif-threshold", "--help"}, &out, &stderr); err != nil {
		t.Fatalf("help: %v %s", err, &stderr)
	}
	for _, want := range []string{"--updates", experiment.LIFThresholdGateDescription, "Errors:", "Options:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help does not mention %q:\n%s", want, &out)
		}
	}

	out.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"examples", "run", "lif-threshold", "--updates", "100"}, &out, &stderr); err != nil {
		t.Fatalf("run: %v %s", err, &stderr)
	}
	var report struct {
		Profile string `json:"profile"`
		Passed  bool   `json:"passed"`
		Runs    []struct {
			Seed   uint64 `json:"seed"`
			Passed bool   `json:"passed"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("report is not parseable JSON: %v\n%s", err, &out)
	}
	if !report.Passed || report.Profile != "fixture" || len(report.Runs) != 3 {
		t.Fatalf("report: %+v", report)
	}
}

func TestRunLIFThresholdExampleRejectsBadInvocations(t *testing.T) {
	for _, args := range [][]string{
		{"examples", "run", "lif-threshold", "--updates", "0"},
		{"examples", "run", "lif-threshold", "--updates", "100001"},
		{"examples", "run", "lif-threshold", "--updates", "oops"},
		{"examples", "run", "lif-threshold", "--typo"},
		{"examples", "run", "lif-threshold", "extra"},
	} {
		var out, stderr bytes.Buffer
		if Run(context.Background(), args, &out, &stderr) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	if Run(ctx, []string{"examples", "run", "lif-threshold", "--updates", "1"}, &out, &stderr) == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestRunLIFThresholdExampleReportsFailedGate(t *testing.T) {
	var out, stderr bytes.Buffer
	err := Run(context.Background(), []string{"examples", "run", "lif-threshold", "--updates", "1"}, &out, &stderr)
	if err == nil {
		t.Fatal("one update should not meet the threshold gates")
	}
	var report struct {
		Passed bool `json:"passed"`
	}
	if e := json.Unmarshal(out.Bytes(), &report); e != nil || report.Passed {
		t.Fatalf("failing run did not print an honest report: %v\n%s", e, &out)
	}
}
