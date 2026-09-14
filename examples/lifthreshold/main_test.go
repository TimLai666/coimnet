package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment"
)

// The protocol itself is verified in the experiment package. These tests cover
// only what this command adds: argument handling, usage text and exit status.

func TestZeroBudgetPrintsTheFailingReportAndFails(t *testing.T) {
	var out bytes.Buffer
	if err := execute(context.Background(), 0, &out); err == nil {
		t.Fatal("execute accepted a failing report")
	}
	var printed experiment.LIFThresholdReport
	if err := json.Unmarshal(out.Bytes(), &printed); err != nil {
		t.Fatalf("failing run did not print a report: %v\n%s", err, out.String())
	}
	if printed.Passed {
		t.Fatal("printed report claims the gates passed")
	}
	if len(printed.Runs) != 3 || printed.Gates.Description != experiment.LIFThresholdGateDescription {
		t.Fatalf("printed report: %+v", printed)
	}
}

func TestRunUsageAndArgumentValidation(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &out, &errOut); err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"Usage: go run ./examples/lifthreshold", experiment.LIFThresholdGateDescription, "Limitations:", "Errors:", "--updates"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("usage does not mention %q: %s", want, out.String())
		}
	}
	for _, args := range [][]string{
		{"--updates", "0"},
		{"--updates", "100001"},
		{"--updates", "-1"},
		{"--unknown"},
		{"extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatalf("accepted invalid arguments %v", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var canceledOut, canceledErr bytes.Buffer
	if err := run(ctx, []string{"--updates", "1"}, &canceledOut, &canceledErr); err == nil {
		t.Fatal("accepted a canceled context")
	}
}
