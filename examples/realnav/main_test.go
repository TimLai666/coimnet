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
	for _, want := range []string{"train", "infer", "after_relocation", "Reward/fictive", "closed-loop", "mutually exclusive", "fully cover", "invalid splits do not create output"} {
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
	if len(value.Split.TestTrialIDs) != 1 || value.Split.TestTrialIDs[0] != "test" {
		t.Fatalf("infer test trial IDs = %v", value.Split.TestTrialIDs)
	}
	if value.Split.TestTrialCounts["non-rewarded"] != 1 || value.Split.TestSampleCounts["non-rewarded"] != 1 {
		t.Fatalf("infer test counts = trials=%v samples=%v", value.Split.TestTrialCounts, value.Split.TestSampleCounts)
	}
}

func TestInferenceRebuildRejectsInvalidSplits(t *testing.T) {
	dataset := inferenceSplitFixtureDataset()
	conditions, err := trialConditions(dataset)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		split splitTrials
	}{
		{
			name:  "cross-list overlap",
			split: splitTrials{Train: []string{"train", "shared"}, Test: []string{"test", "shared"}},
		},
		{
			name:  "omitted trial",
			split: splitTrials{Train: []string{"train"}, Test: []string{"test"}},
		},
		{
			name:  "duplicate training ID",
			split: splitTrials{Train: []string{"train", "train"}, Test: []string{"test", "shared"}},
		},
		{
			name:  "unknown ID",
			split: splitTrials{Train: []string{"train", "shared"}, Test: []string{"test", "unknown"}},
		},
		{
			name:  "empty ID",
			split: splitTrials{Train: []string{"train", "shared"}, Test: []string{"test", ""}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, _, err := rebuildInferenceSplit(dataset, test.split, conditions); err == nil {
				t.Fatalf("rebuildInferenceSplit(%+v) succeeded, want split validation error", test.split)
			}
		})
	}
}

func TestRunInferRejectsInvalidSnapshotsWithoutOutputDirectory(t *testing.T) {
	dataPath := os.Getenv("COIMNET_TSK11_TRAJECTORY")
	snapshotPath := os.Getenv("COIMNET_TSK11_SNAPSHOT")
	if dataPath == "" || snapshotPath == "" {
		t.Skip("set COIMNET_TSK11_TRAJECTORY and COIMNET_TSK11_SNAPSHOT to run the real inference CLI regression")
	}
	if _, err := os.Stat(dataPath); err != nil {
		t.Skipf("trajectory fixture unavailable: %v", err)
	}
	var model savedModel
	if err := readJSON(snapshotPath, &model); err != nil {
		t.Fatalf("read snapshot fixture: %v", err)
	}
	if len(model.Split.Train) == 0 || len(model.Split.Test) == 0 {
		t.Fatal("snapshot fixture has an empty split")
	}
	model.Split.Test[0] = model.Split.Train[0]
	invalidSnapshot := filepath.Join(t.TempDir(), "overlap-model.json")
	if err := writeJSONExclusive(invalidSnapshot, model); err != nil {
		t.Fatalf("write invalid snapshot fixture: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "infer-output")
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"infer", "--data", dataPath, "--snapshot", invalidSnapshot, "--out", outputPath}, &stdout, &stderr)
	if err == nil {
		t.Fatal("infer accepted an overlapping snapshot split")
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("invalid inference created output directory: stat error=%v", statErr)
	}
}

func inferenceSplitFixtureDataset() trajectory.Dataset {
	rows := make([]trajectory.Point, 0, 9)
	for _, trial := range []struct {
		id        string
		condition string
		x, y      float64
	}{{"train", "rewarded", 0, 0}, {"test", "non-rewarded", 10, 0}, {"shared", "rewarded", 20, 0}} {
		for step := 0; step < 3; step++ {
			rows = append(rows, trajectory.Point{TrialID: trial.id, Condition: trial.condition, Segment: "after_relocation", T: float64(step) * 0.1, XCM: trial.x + float64(step), YCM: trial.y})
		}
	}
	return trajectory.Dataset{Rows: rows}
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
