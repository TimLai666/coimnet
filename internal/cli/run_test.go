package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
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
