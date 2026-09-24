package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/tasks/nav2d/trajectory"
)

func TestRunHelpDescribesModesAndCausalBoundary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"train", "infer", "after_relocation", "Reward/fictive", "closed-loop"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help does not contain %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help wrote stderr: %s", stderr.String())
	}
}

func TestReadJSONRejectsTrailingValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte("{}\n{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var model savedModel
	if err := readJSON(path, &model); err == nil {
		t.Fatal("accepted trailing JSON value")
	}
}

func TestInferReportDoesNotClaimPreTrainingEvidence(t *testing.T) {
	value := buildReport("infer", trajectory.Dataset{}, trajectory.Dataset{}, trajectory.Dataset{}, nil, nil, modelRun{}, 0, "snapshot.json", time.Now())
	if value.Metrics.Before != nil {
		t.Fatal("infer report exposed a fabricated before metric")
	}
	if value.Training.InitialSnapshotSHA != nil {
		t.Fatal("infer report exposed an initial snapshot fingerprint")
	}
	if value.Metrics.After == nil || value.Metrics.After.Train != nil {
		t.Fatal("infer report exposed unavailable training metrics")
	}
}

func TestInferenceRebuildsTrainAndTestSplitMetadata(t *testing.T) {
	dataset := trajectory.Dataset{Rows: []trajectory.Point{
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: 0, XCM: 0, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .1, XCM: 1, YCM: 0},
		{TrialID: "train", Condition: "rewarded", Segment: "after_relocation", T: .2, XCM: 2, YCM: 0},
		{TrialID: "test", Condition: "non-rewarded", Segment: "after_relocation", T: 0, XCM: 0, YCM: 0},
		{TrialID: "test", Condition: "non-rewarded", Segment: "after_relocation", T: .1, XCM: 0, YCM: 1},
		{TrialID: "test", Condition: "non-rewarded", Segment: "after_relocation", T: .2, XCM: 0, YCM: 2},
	}}
	conditions, err := trialConditions(dataset)
	if err != nil {
		t.Fatal(err)
	}
	trainDataset, testDataset, trainSamples, testSamples, err := rebuildInferenceSplit(dataset, splitTrials{Train: []string{"train"}, Test: []string{"test"}}, conditions)
	if err != nil {
		t.Fatal(err)
	}
	value := buildReport("infer", dataset, trainDataset, testDataset, trainSamples, testSamples, modelRun{}, 0, "snapshot.json", time.Now())
	if len(value.Split.TrainTrialIDs) != 1 || value.Split.TrainTrialIDs[0] != "train" {
		t.Fatalf("infer train trial IDs = %v", value.Split.TrainTrialIDs)
	}
	if value.Split.TrainTrialCounts["rewarded"] != 1 || value.Split.TrainSampleCounts["rewarded"] != 1 {
		t.Fatalf("infer train counts = trials=%v samples=%v", value.Split.TrainTrialCounts, value.Split.TrainSampleCounts)
	}
}

func TestPrepareOutputDirectoryRejectsDanglingSymlinkEscape(t *testing.T) {
	sourceDir := t.TempDir()
	dataPath := filepath.Join(sourceDir, "data.csv.gz")
	if err := os.WriteFile(dataPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	linkPath := filepath.Join(linkParent, "out-link")
	targetPath := filepath.Join(sourceDir, "escaped-output")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := prepareOutputDirectory(linkPath, dataPath); err == nil {
		t.Fatal("accepted output path through dangling symlink")
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("symlink target was created: stat error=%v", err)
	}
}

func TestPrepareOutputDirectoryAllowsSafeSymlinkParent(t *testing.T) {
	sourceDir := t.TempDir()
	dataPath := filepath.Join(sourceDir, "data.csv.gz")
	if err := os.WriteFile(dataPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputParent := t.TempDir()
	linkPath := filepath.Join(t.TempDir(), "tmp-link")
	if err := os.Symlink(outputParent, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	outputPath := filepath.Join(linkPath, "new-output")
	if _, err := prepareOutputDirectory(outputPath, dataPath); err != nil {
		t.Fatalf("safe symlink parent was rejected: %v", err)
	}
	if info, err := os.Stat(filepath.Join(outputParent, "new-output")); err != nil || !info.IsDir() {
		t.Fatalf("resolved output directory missing: info=%v err=%v", info, err)
	}
}

func TestPrepareOutputDirectoryRejectsSymlinkParentIntoSource(t *testing.T) {
	sourceDir := t.TempDir()
	dataPath := filepath.Join(sourceDir, "data.csv.gz")
	if err := os.WriteFile(dataPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(t.TempDir(), "source-link")
	if err := os.Symlink(sourceDir, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := prepareOutputDirectory(filepath.Join(linkPath, "new-output"), dataPath); err == nil {
		t.Fatal("accepted symlink parent into source directory")
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "new-output")); !os.IsNotExist(err) {
		t.Fatalf("source directory was modified: stat error=%v", err)
	}
}

func TestRunRejectsMissingOrUnknownModeWithoutJSON(t *testing.T) {
	for _, args := range [][]string{{"wat"}, {"train"}, {"infer"}} {
		var stdout, stderr bytes.Buffer
		err := run(context.Background(), args, &stdout, &stderr)
		if err == nil {
			t.Fatalf("run(%v) succeeded, want error", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) wrote stdout on error: %s", args, stdout.String())
		}
	}
}
