package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

func TestRealOCREncoderRunProfilesComeFromActualSnapshotAndEvaluation(t *testing.T) {
	ctx := context.Background()
	dataset := realOCREncoderFixtureDataset(t)
	for _, mode := range []string{"", "trainable"} {
		t.Run(modeName(mode), func(t *testing.T) {
			config := realOCREncoderRunConfig(mode)
			report, err := Run(ctx, dataset, config)
			if err != nil {
				t.Fatalf("Run(%q): %v", mode, err)
			}
			wantConfig := ocr.RecognizerConfig{
				Height: 28, Hidden: config.Hidden, Alphabet: realOCREncoderAlphabet(dataset),
				LearningRate: config.LearningRate, Seed: config.Seed, TrainableEncoder: mode == "trainable",
			}
			wantModel, err := ocr.NewRecognizer(wantConfig)
			if err != nil {
				t.Fatal(err)
			}
			wantBefore, err := wantModel.Evaluate(ctx, dataset.Test)
			if err != nil {
				t.Fatal(err)
			}
			for epoch := 0; epoch < config.Epochs; epoch++ {
				for i, sample := range dataset.Train {
					line, err := wantModel.TrainLine(ctx, sample.Image, sample.Text)
					if err != nil {
						t.Fatalf("reference TrainLine %d: %v", i, err)
					}
					if line.Impossible {
						t.Fatalf("reference TrainLine %d was impossible", i)
					}
				}
			}
			wantAfter, err := wantModel.Evaluate(ctx, dataset.Test)
			if err != nil {
				t.Fatal(err)
			}
			if report.Config.Encoder != mode {
				t.Fatalf("report config encoder = %q, want %q", report.Config.Encoder, mode)
			}
			if report.Before != realOCREvalResult(wantBefore) || report.After != realOCREvalResult(wantAfter) {
				t.Fatalf("report evaluation before/after = %+v/%+v, want actual %+v/%+v", report.Before, report.After, realOCREvalResult(wantBefore), realOCREvalResult(wantAfter))
			}
			if report.After.Samples != len(dataset.Test) || report.After.EmptyOutputs < 0 || report.After.EmptyOutputs > report.After.Samples {
				t.Fatalf("report has invalid actual evaluation counts: %+v", report.After)
			}
			wantSnapshot := wantModel.Snapshot()
			wantEncoder := len(wantSnapshot.Parameters.Encoder)
			wantCore := len(wantSnapshot.Parameters.Core.Weights) + len(wantSnapshot.Parameters.Core.Bias) + len(wantSnapshot.Parameters.Core.LogTau) + len(wantSnapshot.Parameters.ThetaRaw)
			wantReadout := len(wantSnapshot.Parameters.Readout)
			if report.EncoderProfile.Mode != modeName(mode) || report.EncoderProfile.EncoderParameters != wantEncoder || report.EncoderProfile.CoreParameters != wantCore || report.EncoderProfile.ReadoutParameters != wantReadout || report.EncoderProfile.Updates != wantSnapshot.Updates {
				t.Fatalf("encoder profile = %+v, want mode=%q encoder/core/readout=%d/%d/%d updates=%d", report.EncoderProfile, modeName(mode), wantEncoder, wantCore, wantReadout, wantSnapshot.Updates)
			}
		})
	}
}

func TestRealOCREncoderRejectsUnknownModeAndModelModeMismatch(t *testing.T) {
	ctx := context.Background()
	dataset := realOCREncoderFixtureDataset(t)
	t.Run("unknown mode", func(t *testing.T) {
		unknown := realOCREncoderRunConfig("unknown")
		if _, err := Run(ctx, dataset, unknown); err == nil {
			t.Fatal("Run accepted unknown encoder mode")
		}
	})
	t.Run("model mode mismatch", func(t *testing.T) {
		fixedConfig := realOCREncoderRunConfig("")
		modelPath := filepath.Join(t.TempDir(), "fixed-model")
		if _, err := RunWithModel(ctx, dataset, fixedConfig, "", modelPath); err != nil {
			t.Fatalf("RunWithModel save fixed model: %v", err)
		}
		trainableConfig := realOCREncoderRunConfig("trainable")
		if _, err := RunWithModel(ctx, dataset, trainableConfig, modelPath, ""); err == nil {
			t.Fatal("RunWithModel accepted a saved fixed model for trainable mode")
		}
	})
}

func TestRealOCREncoderEvalResultCarriesEmptyOutputMetrics(t *testing.T) {
	got := evalResult(ocr.EvalReport{
		CER:              ocr.EditReport{Rate: 0.25, Defined: true},
		ExactLines:       0.5,
		Samples:          4,
		EmptyOutputs:     2,
		EmptyOutputRatio: 0.5,
	})
	if got.EmptyOutputs != 2 || got.EmptyOutputRatio != 0.5 || got.Samples != 4 || got.CER != 0.25 || got.Exact != 0.5 {
		t.Fatalf("evalResult = %+v, want empty-output metrics preserved", got)
	}
}

func TestRealOCREncoderCLIFlagHelp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	help := exec.CommandContext(ctx, "go", "run", ".", "--help")
	helpOutput, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("go run --help: %v\n%s", err, helpOutput)
	}
	helpText := string(helpOutput)
	if !strings.Contains(helpText, "--encoder") || !strings.Contains(helpText, "fixed") || !strings.Contains(helpText, "trainable") {
		t.Fatalf("CLI help omitted encoder modes:\n%s", helpText)
	}
}

func TestRealOCREncoderCLIFlagRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	writeMetadata(t, dir)
	writeOfficialFixture(t, dir)
	cmd := exec.CommandContext(ctx, "go", "run", ".",
		"--data-dir", dir,
		"--train-count", "1", "--test-count", "1", "--epochs", "1",
		"--hidden", "4", "--learning-rate", "0.02", "--encoder", "trainable",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run --encoder trainable: %v\nstderr=%s\nstdout=%s", err, stderr.Bytes(), stdout.Bytes())
	}
	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode CLI report: %v\nstdout=%s\nstderr=%s", err, stdout.Bytes(), stderr.Bytes())
	}
	if report.Config.Encoder != "trainable" || report.EncoderProfile.Mode != "trainable" {
		t.Fatalf("CLI report encoder config/profile = %q/%q, want trainable/trainable", report.Config.Encoder, report.EncoderProfile.Mode)
	}
	if report.After.Samples != 1 || report.After.EmptyOutputs < 0 || report.After.EmptyOutputs > report.After.Samples {
		t.Fatalf("CLI report has invalid evaluation counts: %+v", report.After)
	}
}

func realOCREncoderFixtureDataset(t *testing.T) Dataset {
	t.Helper()
	image := glyphs.Image{Width: 4, Height: 28, Pixels: make([]float64, 4*28)}
	for i := range image.Pixels {
		image.Pixels[i] = 0.1
	}
	for y := 8; y < 20; y++ {
		for x := 1; x < 3; x++ {
			image.Pixels[y*image.Width+x] = 1
		}
	}
	sample := glyphs.Sample{Text: "お", Image: image}
	meta := validMetadata()
	return Dataset{
		Metadata: meta, MetadataSHA256: "metadata-fixture", ClassMapSHA256: "classmap-fixture",
		TrainTotal: 1, TestTotal: 1,
		Train: []glyphs.Sample{sample}, Test: []glyphs.Sample{sample},
		TrainIndices: []int{0}, TestIndices: []int{0},
	}
}

func realOCREncoderAlphabet(dataset Dataset) []rune {
	alphabet := make([]rune, len(dataset.Metadata.Mapping))
	for i, mapping := range dataset.Metadata.Mapping {
		alphabet[i] = []rune(mapping.Char)[0]
	}
	return alphabet
}

func realOCREncoderRunConfig(mode string) RunConfig {
	return RunConfig{
		Seed: 9, TrainCount: 1, TestCount: 1, Epochs: 1, Hidden: 4, LearningRate: 0.02,
		MaxCompressedBytes: defaultMaxCompressedBytes, MaxUncompressedBytes: defaultMaxUncompressedBytes,
		Selection: "balanced_round_robin_by_label_v1", Encoder: mode,
	}
}

func realOCREvalResult(report ocr.EvalReport) EvalResult {
	return EvalResult{
		CER: report.CER.Rate, Exact: report.ExactLines, Samples: report.Samples,
		EmptyOutputs: report.EmptyOutputs, EmptyOutputRatio: report.EmptyOutputRatio,
	}
}

func modeName(mode string) string {
	if mode == "" {
		return "fixed"
	}
	return mode
}
