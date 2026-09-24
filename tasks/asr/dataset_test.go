package asr_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

type datasetManifest struct {
	Schema  string                  `json:"schema"`
	License datasetLicense          `json:"license"`
	Records []datasetManifestRecord `json:"recordings"`
}

type datasetLicense struct {
	Holder string `json:"holder"`
	Terms  string `json:"terms"`
	Source string `json:"source"`
}

type datasetManifestRecord struct {
	Path       string `json:"path"`
	Text       string `json:"text"`
	Speaker    string `json:"speaker"`
	Session    string `json:"session"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
}

func writeDatasetWAV(t *testing.T, path string, rate int, samples [][]float64) {
	t.Helper()
	if err := audio.WriteWAV(path, audio.Signal{
		SampleRate: rate,
		Channels:   len(samples),
		Samples:    samples,
	}); err != nil {
		t.Fatalf("WriteWAV(%s): %v", path, err)
	}
}

func writeDatasetManifest(t *testing.T, path string, manifest datasetManifest) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func writeRawDatasetManifest(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func rawDatasetRecord(record string) string {
	return `{"schema":"coimnet-asr-dataset/v1","license":{"holder":"h","terms":"t","source":"s"},"recordings":[` + record + `]}`
}

func validDatasetManifest() datasetManifest {
	return datasetManifest{
		Schema: "coimnet-asr-dataset/v1",
		License: datasetLicense{
			Holder: "Example holder",
			Terms:  "CC BY 4.0",
			Source: "https://example.test/audio",
		},
		Records: []datasetManifestRecord{
			{Path: "first.wav", Text: "早安", Speaker: "speaker-a", Session: "session-1", SampleRate: 8000, Channels: 2},
			{Path: "second.wav", Text: "hello", Speaker: "speaker-b", Session: "session-2", SampleRate: 11025, Channels: 2},
		},
	}
}

func TestReadDatasetValidStereoDifferentRatesAndReports(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.wav")
	secondPath := filepath.Join(root, "second.wav")
	writeDatasetWAV(t, firstPath, 8000, [][]float64{
		{0.25, -0.25, 0.5, -0.5},
		{0.75, 0.25, -0.5, 0.5},
	})
	writeDatasetWAV(t, secondPath, 11025, [][]float64{
		{0.125, -0.25, 0.375, -0.5, 0.625},
		{-0.125, 0.25, -0.375, 0.5, -0.625},
	})
	manifestPath := filepath.Join(root, "manifest.json")
	writeDatasetManifest(t, manifestPath, validDatasetManifest())

	got, err := asr.ReadDataset(manifestPath, 16000, 0.8)
	if err != nil {
		t.Fatalf("ReadDataset: %v", err)
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	wantManifestHash := sha256.Sum256(manifestRaw)
	if got.ManifestSHA256 != hex.EncodeToString(wantManifestHash[:]) {
		t.Fatalf("manifest SHA-256 = %q, want %x", got.ManifestSHA256, wantManifestHash)
	}
	if got.License.Holder != "Example holder" || got.License.Terms != "CC BY 4.0" || got.License.Source != "https://example.test/audio" {
		t.Fatalf("License = %+v, want the manifest license", got.License)
	}
	if len(got.Utterances) != 2 || len(got.Metadata) != 2 {
		t.Fatalf("utterances/metadata lengths = %d/%d, want 2/2", len(got.Utterances), len(got.Metadata))
	}
	if got.Utterances[0].Text != "早安" || got.Utterances[0].Speaker != "speaker-a" || got.Utterances[0].Session != "session-1" {
		t.Fatalf("first utterance = %+v, want manifest text and metadata", got.Utterances[0])
	}
	first := got.Metadata[0]
	if first.OriginalPath != "first.wav" || first.OriginalSampleRate != 8000 || first.OriginalChannels != 2 {
		t.Fatalf("first provenance = %+v, want original path/rate/channels", first)
	}
	if first.Resample.Method != "linear" || first.Resample.From != 8000 || first.Resample.To != 16000 || first.Resample.InputLength != 4 || first.Resample.OutputLength != 8 {
		t.Fatalf("first resample report = %+v, want linear 8000->16000 with 4->8 samples", first.Resample)
	}
	if !first.Normalize.Scaled || first.Normalize.Peak <= 0 || first.Normalize.Gain <= 0 {
		t.Fatalf("first normalize report = %+v, want a positive scaled peak", first.Normalize)
	}
	if gotPeak := datasetPeak(got.Utterances[0].Samples); math.Abs(gotPeak-0.8) > 1e-12 {
		t.Fatalf("first normalized peak = %.17g, want 0.8", gotPeak)
	}
	assertDatasetSHA(t, firstPath, first.OriginalSHA256)
	second := got.Metadata[1]
	if second.OriginalSampleRate != 11025 || second.OriginalChannels != 2 || second.Resample.From != 11025 || second.Resample.To != 16000 {
		t.Fatalf("second provenance/report = %+v, want original 11025 Hz stereo and target 16000 Hz", second)
	}
	if len(got.Utterances[1].Samples) != second.Resample.OutputLength {
		t.Fatalf("second sample count = %d, want report output length %d", len(got.Utterances[1].Samples), second.Resample.OutputLength)
	}
}

func TestReadDatasetHashAndSamplesUseOneWAVByteSlice(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, path, 8000, [][]float64{{0.25, -0.5, 0.75, -0.125}})
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	expectedSignal, err := audio.DecodeWAV(raw)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	expectedMixed := audio.Mixdown(expectedSignal)
	expectedResampled, _, err := audio.Resample(expectedMixed, 8000, 16000)
	if err != nil {
		t.Fatalf("Resample: %v", err)
	}
	expectedSamples, _, err := audio.Normalize(expectedResampled, 0.8)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	manifest := validDatasetManifest()
	manifest.Records = []datasetManifestRecord{{Path: "voice.wav", Text: "ok", Speaker: "spk", Session: "ses", SampleRate: 8000, Channels: 1}}
	writeDatasetManifest(t, manifestPath, manifest)

	got, err := asr.ReadDataset(manifestPath, 16000, 0.8)
	if err != nil {
		t.Fatalf("ReadDataset: %v", err)
	}
	wantHashBytes := sha256.Sum256(raw)
	wantHash := hex.EncodeToString(wantHashBytes[:])
	if got.Metadata[0].OriginalSHA256 != wantHash {
		t.Fatalf("OriginalSHA256 = %q, want hash of the decoded bytes %q", got.Metadata[0].OriginalSHA256, wantHash)
	}
	if !reflect.DeepEqual(got.Utterances[0].Samples, expectedSamples) {
		t.Fatalf("decoded samples = %v, want preprocessing of the same bytes %v", got.Utterances[0].Samples, expectedSamples)
	}
}

func TestReadDatasetEnforcesRawAndProcessedCapacity(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.wav")
	secondPath := filepath.Join(root, "second.wav")
	writeDatasetWAV(t, firstPath, 16000, [][]float64{{0.25, -0.5}})
	writeDatasetWAV(t, secondPath, 16000, [][]float64{{0.5, -0.25}})
	firstRaw, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", firstPath, err)
	}
	manifest := validDatasetManifest()
	manifest.Records = []datasetManifestRecord{
		{Path: "first.wav", Text: "one", Speaker: "spk1", Session: "ses1", SampleRate: 16000, Channels: 1},
		{Path: "second.wav", Text: "two", Speaker: "spk2", Session: "ses2", SampleRate: 16000, Channels: 1},
	}
	manifestPath := filepath.Join(root, "manifest.json")
	writeDatasetManifest(t, manifestPath, manifest)

	t.Run("raw per file", func(t *testing.T) {
		limits := asr.DatasetLimits{
			MaxRawBytes:            int64(len(firstRaw) - 1),
			MaxFileProcessedBytes:  1 << 20,
			MaxTotalProcessedBytes: 2 << 20,
		}
		if _, err := asr.ReadDatasetWithLimits(manifestPath, 16000, 1, limits); err == nil || !strings.Contains(strings.ToLower(err.Error()), "raw") {
			t.Fatalf("ReadDatasetWithLimits raw error = %v, want raw-byte limit rejection", err)
		}
	})

	t.Run("single file processed allocation", func(t *testing.T) {
		limits := asr.DatasetLimits{
			MaxRawBytes:            1 << 20,
			MaxFileProcessedBytes:  63,
			MaxTotalProcessedBytes: 2 << 20,
		}
		if _, err := asr.ReadDatasetWithLimits(manifestPath, 16000, 1, limits); err == nil || !strings.Contains(strings.ToLower(err.Error()), "processed") {
			t.Fatalf("ReadDatasetWithLimits file processed error = %v, want processed-byte limit rejection", err)
		}
	})

	t.Run("aggregate processed allocation", func(t *testing.T) {
		limits := asr.DatasetLimits{
			MaxRawBytes:            1 << 20,
			MaxFileProcessedBytes:  64,
			MaxTotalProcessedBytes: 100,
		}
		if _, err := asr.ReadDatasetWithLimits(manifestPath, 16000, 1, limits); err == nil || !strings.Contains(strings.ToLower(err.Error()), "total processed") {
			t.Fatalf("ReadDatasetWithLimits total processed error = %v, want aggregate processed-byte limit rejection", err)
		}
	})

	t.Run("extreme resample output", func(t *testing.T) {
		limits := asr.DatasetLimits{
			MaxRawBytes:            1 << 20,
			MaxFileProcessedBytes:  1 << 20,
			MaxTotalProcessedBytes: 2 << 20,
		}
		if _, err := asr.ReadDatasetWithLimits(manifestPath, 1_000_000_000, 1, limits); err == nil || !strings.Contains(strings.ToLower(err.Error()), "processed") {
			t.Fatalf("ReadDatasetWithLimits resample error = %v, want processed-byte limit rejection", err)
		}
	})
}

func TestReadDatasetRejectsDuplicateKeysAndAliases(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, path, 16000, [][]float64{{0.25, -0.25}})

	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "exact schema duplicate",
			raw:  `{"schema":"coimnet-asr-dataset/v1","schema":"coimnet-asr-dataset/v1","license":{"holder":"h","terms":"t","source":"s"},"recordings":[{"path":"voice.wav","text":"ok","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}]}`,
		},
		{
			name: "case alias",
			raw:  `{"schema":"coimnet-asr-dataset/v1","Schema":"coimnet-asr-dataset/v1","license":{"holder":"h","terms":"t","source":"s"},"recordings":[{"path":"voice.wav","text":"ok","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}]}`,
		},
		{
			name: "escaped alias",
			raw:  `{"schema":"coimnet-asr-dataset/v1","\u0053chema":"coimnet-asr-dataset/v1","license":{"holder":"h","terms":"t","source":"s"},"recordings":[{"path":"voice.wav","text":"ok","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath := filepath.Join(root, tc.name+".json")
			writeRawDatasetManifest(t, manifestPath, tc.raw)
			if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				t.Fatalf("ReadDataset error = %v, want duplicate-key rejection", err)
			}
		})
	}
}

func TestReadDatasetRejectsNullScalarFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, path, 16000, [][]float64{{0.25, -0.25}})
	cases := []struct {
		name   string
		record string
	}{
		{name: "path", record: `{"path":null,"text":"ok","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}`},
		{name: "text", record: `{"path":"voice.wav","text":null,"speaker":"spk","session":"ses","sample_rate":16000,"channels":1}`},
		{name: "speaker", record: `{"path":"voice.wav","text":"ok","speaker":null,"session":"ses","sample_rate":16000,"channels":1}`},
		{name: "session", record: `{"path":"voice.wav","text":"ok","speaker":"spk","session":null,"sample_rate":16000,"channels":1}`},
		{name: "sample rate", record: `{"path":"voice.wav","text":"ok","speaker":"spk","session":"ses","sample_rate":null,"channels":1}`},
		{name: "channels", record: `{"path":"voice.wav","text":"ok","speaker":"spk","session":"ses","sample_rate":16000,"channels":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath := filepath.Join(root, tc.name+"-null.json")
			writeRawDatasetManifest(t, manifestPath, rawDatasetRecord(tc.record))
			if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil {
				t.Fatalf("ReadDataset accepted null %s", tc.name)
			}
		})
	}
}

func TestReadDatasetRejectsOversizedManifest(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "oversized.json")
	largeText := strings.Repeat("x", 16<<20)
	raw := rawDatasetRecord(`{"path":"voice.wav","text":"` + largeText + `","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}`)
	writeRawDatasetManifest(t, manifestPath, raw)
	if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), "exceeds") {
		t.Fatalf("ReadDataset oversized manifest error = %v, want byte-limit rejection", err)
	}
}

func TestReadDatasetRejectsInvalidArguments(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, path, 16000, [][]float64{{0.25, -0.25}})
	manifestPath := filepath.Join(root, "manifest.json")
	manifest := validDatasetManifest()
	manifest.Records = []datasetManifestRecord{{Path: "voice.wav", Text: "ok", Speaker: "s", Session: "n", SampleRate: 16000, Channels: 1}}
	writeDatasetManifest(t, manifestPath, manifest)

	for _, tc := range []struct {
		name string
		rate int
		peak float64
		want string
	}{
		{name: "zero target rate", rate: 0, peak: 1, want: "target rate"},
		{name: "negative target rate", rate: -1, peak: 1, want: "target rate"},
		{name: "zero peak", rate: 16000, peak: 0, want: "peak"},
		{name: "peak above one", rate: 16000, peak: 1.01, want: "peak"},
		{name: "nan peak", rate: 16000, peak: math.NaN(), want: "peak"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := asr.ReadDataset(manifestPath, tc.rate, tc.peak); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("ReadDataset(rate=%d, peak=%v) error = %v, want one mentioning %q", tc.rate, tc.peak, err, tc.want)
			}
		})
	}
}

func TestReadDatasetRejectsZeroResampledSamples(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one-frame.wav")
	writeDatasetWAV(t, path, 48000, [][]float64{{0.25}})
	manifestPath := filepath.Join(root, "manifest.json")
	manifest := validDatasetManifest()
	manifest.Records = []datasetManifestRecord{{Path: "one-frame.wav", Text: "ok", Speaker: "speaker", Session: "session", SampleRate: 48000, Channels: 1}}
	writeDatasetManifest(t, manifestPath, manifest)

	if _, err := asr.ReadDataset(manifestPath, 1, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), "zero samples") {
		t.Fatalf("ReadDataset zero-output resample error = %v, want one mentioning zero samples", err)
	}
}

func TestReadDatasetRejectsLicenseMetadataAndRecordShape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, path, 16000, [][]float64{{0.25, -0.25}})
	base := validDatasetManifest()
	base.Records = []datasetManifestRecord{{Path: "voice.wav", Text: "ok", Speaker: "speaker", Session: "session", SampleRate: 16000, Channels: 1}}

	cases := []struct {
		name   string
		mutate func(*datasetManifest)
		want   string
	}{
		{name: "missing holder", mutate: func(m *datasetManifest) { m.License.Holder = "" }, want: "license"},
		{name: "missing terms", mutate: func(m *datasetManifest) { m.License.Terms = "" }, want: "license"},
		{name: "missing source", mutate: func(m *datasetManifest) { m.License.Source = "" }, want: "license"},
		{name: "empty speaker", mutate: func(m *datasetManifest) { m.Records[0].Speaker = " " }, want: "speaker"},
		{name: "empty session", mutate: func(m *datasetManifest) { m.Records[0].Session = "" }, want: "session"},
		{name: "no recordings", mutate: func(m *datasetManifest) { m.Records = nil }, want: "recording"},
		{name: "wrong schema", mutate: func(m *datasetManifest) { m.Schema = "other/v1" }, want: "schema"},
		{name: "declared rate differs", mutate: func(m *datasetManifest) { m.Records[0].SampleRate = 8000 }, want: "sample rate"},
		{name: "declared channels differ", mutate: func(m *datasetManifest) { m.Records[0].Channels = 2 }, want: "channel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := base
			manifest.Records = append([]datasetManifestRecord(nil), base.Records...)
			tc.mutate(&manifest)
			manifestPath := filepath.Join(root, tc.name+".json")
			writeDatasetManifest(t, manifestPath, manifest)
			if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("ReadDataset error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestReadDatasetRejectsPathTraversalAbsoluteDuplicateAndCorruptWAV(t *testing.T) {
	root := t.TempDir()
	voicePath := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, voicePath, 16000, [][]float64{{0.25, -0.25}})
	outsidePath := filepath.Join(filepath.Dir(root), "outside.wav")
	writeDatasetWAV(t, outsidePath, 16000, [][]float64{{0.25, -0.25}})
	t.Cleanup(func() { _ = os.Remove(outsidePath) })

	base := validDatasetManifest()
	base.Records = []datasetManifestRecord{{Path: "voice.wav", Text: "ok", Speaker: "speaker", Session: "session", SampleRate: 16000, Channels: 1}}
	cases := []struct {
		name   string
		mutate func(*datasetManifest)
		want   string
	}{
		{name: "traversal", mutate: func(m *datasetManifest) { m.Records[0].Path = "../outside.wav" }, want: "path"},
		{name: "absolute", mutate: func(m *datasetManifest) { m.Records[0].Path = voicePath }, want: "relative"},
		{name: "duplicate", mutate: func(m *datasetManifest) {
			m.Records = append(m.Records, datasetManifestRecord{Path: "./voice.wav", Text: "other", Speaker: "speaker", Session: "session-2", SampleRate: 16000, Channels: 1})
		}, want: "duplicate"},
		{name: "corrupt wav", mutate: func(m *datasetManifest) {
			m.Records[0].Path = "corrupt.wav"
		}, want: "wav"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := base
			manifest.Records = append([]datasetManifestRecord(nil), base.Records...)
			tc.mutate(&manifest)
			if tc.name == "corrupt wav" {
				if err := os.WriteFile(filepath.Join(root, "corrupt.wav"), []byte("not wav"), 0o644); err != nil {
					t.Fatalf("WriteFile corrupt WAV: %v", err)
				}
			}
			manifestPath := filepath.Join(root, tc.name+".json")
			writeDatasetManifest(t, manifestPath, manifest)
			if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("ReadDataset error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestReadDatasetRejectsRecordingSymlinkOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "voice.wav")
	writeDatasetWAV(t, outsidePath, 16000, [][]float64{{0.25, -0.25}})
	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	manifest := validDatasetManifest()
	manifest.Records = []datasetManifestRecord{{Path: "outside/voice.wav", Text: "ok", Speaker: "spk", Session: "ses", SampleRate: 16000, Channels: 1}}
	writeDatasetManifest(t, manifestPath, manifest)

	if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil {
		t.Fatal("ReadDataset accepted a recording symlink that escapes the manifest root")
	}
}

func TestReadDatasetRejectsRecordingSymlinkAlias(t *testing.T) {
	root := t.TempDir()
	voicePath := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, voicePath, 16000, [][]float64{{0.25, -0.25}})
	if err := os.Symlink("voice.wav", filepath.Join(root, "alias.wav")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	manifest := validDatasetManifest()
	manifest.Records = []datasetManifestRecord{
		{Path: "voice.wav", Text: "first", Speaker: "spk", Session: "ses-1", SampleRate: 16000, Channels: 1},
		{Path: "alias.wav", Text: "second", Speaker: "spk", Session: "ses-2", SampleRate: 16000, Channels: 1},
	}
	writeDatasetManifest(t, manifestPath, manifest)

	if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("ReadDataset symlink alias error = %v, want duplicate rejection", err)
	}
}

func TestReadDatasetRejectsInvalidUTF8Manifest(t *testing.T) {
	root := t.TempDir()
	voicePath := filepath.Join(root, "voice.wav")
	writeDatasetWAV(t, voicePath, 16000, [][]float64{{0.25, -0.25}})
	manifestPath := filepath.Join(root, "invalid-utf8.json")
	raw := []byte(`{"schema":"coimnet-asr-dataset/v1","license":{"holder":"h","terms":"t","source":"s"},"recordings":[{"path":"voice.wav","text":"`)
	raw = append(raw, 0xff)
	raw = append(raw, []byte(`","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}]}`)...)
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile invalid manifest: %v", err)
	}
	if _, err := asr.ReadDataset(manifestPath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), "utf-8") {
		t.Fatalf("ReadDataset invalid UTF-8 error = %v, want one mentioning UTF-8", err)
	}

	surrogatePath := filepath.Join(root, "unpaired-surrogate.json")
	surrogate := []byte(`{"schema":"coimnet-asr-dataset/v1","license":{"holder":"h","terms":"t","source":"s"},"recordings":[{"path":"voice.wav","text":"\ud800","speaker":"spk","session":"ses","sample_rate":16000,"channels":1}]}`)
	if err := os.WriteFile(surrogatePath, surrogate, 0o644); err != nil {
		t.Fatalf("WriteFile unpaired surrogate manifest: %v", err)
	}
	if _, err := asr.ReadDataset(surrogatePath, 16000, 1); err == nil || !strings.Contains(strings.ToLower(err.Error()), "surrogate") {
		t.Fatalf("ReadDataset unpaired surrogate error = %v, want one mentioning surrogate", err)
	}
}

func datasetPeak(samples []float64) float64 {
	peak := 0.0
	for _, sample := range samples {
		if sample < 0 {
			sample = -sample
		}
		if sample > peak {
			peak = sample
		}
	}
	return peak
}

func assertDatasetSHA(t *testing.T, path, got string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	wantBytes := sha256.Sum256(raw)
	want := hex.EncodeToString(wantBytes[:])
	if got != want {
		t.Fatalf("SHA256 = %q, want %q", got, want)
	}
}
