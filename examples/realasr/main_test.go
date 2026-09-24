package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestRunHelpDescribesContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{
		"Usage: go run ./examples/realasr --manifest PATH",
		"16 kHz",
		"speaker/session",
		"Errors:",
		"Limitations:",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help does not contain %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help wrote stderr: %s", stderr.String())
	}
}

func TestRunImportsTrainsAndReportsLicensedDataset(t *testing.T) {
	root := t.TempDir()
	manifestPath, manifestBytes := writeExampleDataset(t, root)
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--manifest", manifestPath}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v\nstderr=%s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run wrote stderr: %s", stderr.String())
	}
	var report realASRReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	wantHash := sha256.Sum256(manifestBytes)
	if report.SchemaVersion != realASRSchemaVersion {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.ManifestSHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("manifest_sha256 = %q, want %x", report.ManifestSHA256, wantHash)
	}
	if report.ImportedCount != 4 || report.TrainCount != 3 || report.TestCount != 1 {
		t.Fatalf("counts = imported %d train %d test %d", report.ImportedCount, report.TrainCount, report.TestCount)
	}
	if len(report.SourceMetadata) != 4 || report.SourceMetadata[0].OriginalPath != "utterance0.wav" {
		t.Fatalf("source metadata = %+v, want four recordings in manifest order", report.SourceMetadata)
	}
	firstWAV, err := os.ReadFile(filepath.Join(root, "utterance0.wav"))
	if err != nil {
		t.Fatal(err)
	}
	wantWAVHash := sha256.Sum256(firstWAV)
	if report.SourceMetadata[0].OriginalSHA256 != hex.EncodeToString(wantWAVHash[:]) {
		t.Fatalf("first WAV SHA-256 = %q, want %x", report.SourceMetadata[0].OriginalSHA256, wantWAVHash)
	}
	if report.License.Source != "https://example.test/licensed-asr" {
		t.Fatalf("license = %+v", report.License)
	}
	if len(report.Alphabet) != 1 || report.Alphabet[0] != "a" {
		t.Fatalf("alphabet = %#v", report.Alphabet)
	}
	if report.Settings.SampleRate != 16000 || report.Settings.Peak != 0.8 || report.Settings.FrontEnd.Window != 256 || report.Settings.FrontEnd.Hop != 128 || report.Settings.FrontEnd.Mels != 12 || report.Settings.Hidden != 24 || report.Settings.LearningRate != 0.001 || report.Settings.Seed != 5 {
		t.Fatalf("settings = %+v", report.Settings)
	}
	if report.Before.CER.ReferenceLength == 0 || report.After.CER.ReferenceLength == 0 {
		t.Fatalf("CER reports = before %+v after %+v", report.Before.CER, report.After.CER)
	}
	if report.Before.WER.ReferenceLength == 0 || report.After.WER.ReferenceLength == 0 {
		t.Fatalf("WER reports = before %+v after %+v", report.Before.WER, report.After.WER)
	}
	if report.FirstRetained.Reference != "a" {
		t.Fatalf("first retained = %+v", report.FirstRetained)
	}
	var rawReport map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &rawReport); err != nil {
		t.Fatalf("decode raw report: %v", err)
	}
	if _, ok := rawReport["first_retained"]; !ok {
		t.Fatal("first_retained result is missing")
	}
	if report.ImpossibleCount > report.TrainCount {
		t.Fatalf("impossible_count = %d for %d training samples", report.ImpossibleCount, report.TrainCount)
	}
	if len(report.ScopeLimitations) == 0 {
		t.Fatal("scope limitations are missing")
	}
	for _, rate := range []float64{report.Before.CER.Rate, report.Before.WER.Rate, report.After.CER.Rate, report.After.WER.Rate} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) {
			t.Fatalf("non-finite error rate: %v", rate)
		}
	}
}

func TestRunRejectsMissingOrInvalidManifestWithoutJSON(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--manifest", "missing.json"},
		{"--manifest", "missing.json", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		err := run(context.Background(), args, &stdout, &stderr)
		if err == nil {
			t.Fatalf("accepted args %v", args)
		}
		if stdout.Len() != 0 {
			t.Fatalf("args %v wrote stdout on error: %s", args, stdout.String())
		}
	}
}

func TestTrainingAlphabetSortsAndDeduplicatesRunes(t *testing.T) {
	got := trainingAlphabet([]asr.Utterance{
		{Text: "éaAé"},
		{Text: "a"},
	})
	want := []rune{'A', 'a', 'é'}
	if string(got) != string(want) {
		t.Fatalf("training alphabet = %q, want %q", string(got), string(want))
	}
}

type testManifest struct {
	Schema     string                  `json:"schema"`
	License    testLicense             `json:"license"`
	Recordings []testManifestRecording `json:"recordings"`
}

type testLicense struct {
	Holder string `json:"holder"`
	Terms  string `json:"terms"`
	Source string `json:"source"`
}

type testManifestRecording struct {
	Path       string `json:"path"`
	Text       string `json:"text"`
	Speaker    string `json:"speaker"`
	Session    string `json:"session"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

func writeExampleDataset(t *testing.T, root string) (string, []byte) {
	t.Helper()
	for i := 0; i < 4; i++ {
		path := filepath.Join(root, "utterance"+string(rune('0'+i))+".wav")
		samples := make([]float64, 1024)
		for n := range samples {
			samples[n] = 0.5 * math.Sin(2*math.Pi*float64(300+i*50)*float64(n)/16000)
		}
		if err := audio.WriteWAV(path, audio.Signal{SampleRate: 16000, Samples: [][]float64{samples}}); err != nil {
			t.Fatalf("write wav %s: %v", path, err)
		}
	}
	manifest := testManifest{
		Schema:  "coimnet-asr-dataset/v1",
		License: testLicense{Holder: "Example Holder", Terms: "CC BY 4.0", Source: "https://example.test/licensed-asr"},
		Recordings: []testManifestRecording{
			{Path: "utterance0.wav", Text: "a", Speaker: "speaker-0", Session: "session-0", SampleRate: 16000, Channels: 1},
			{Path: "utterance1.wav", Text: "a", Speaker: "speaker-1", Session: "session-1", SampleRate: 16000, Channels: 1},
			{Path: "utterance2.wav", Text: "a", Speaker: "speaker-2", Session: "session-2", SampleRate: 16000, Channels: 1},
			{Path: "utterance3.wav", Text: "a", Speaker: "speaker-3", Session: "session-3", SampleRate: 16000, Channels: 1},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath, manifestBytes
}
