package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
	"github.com/TimLai666/coimnet/tasks/media"
	"github.com/TimLai666/coimnet/tasks/media/realdata"
)

//go:embed manifest.json
var manifestJSON []byte

const (
	exampleEpochs        = 60
	reportSchema         = "coimnet-realmedia-example/v1"
	trainedModelFileName = "trained-model.json"
)

type clipRecord struct {
	SourceID       string `json:"source_id"`
	SegmentID      string `json:"segment_id"`
	Split          string `json:"split"`
	StartMillis    int64  `json:"start_millis"`
	DurationMillis int64  `json:"duration_millis"`
	Prompt         string `json:"prompt"`
	PromptSource   string `json:"prompt_source"`
	AnnotationNote string `json:"annotation_note"`
	Frames         int    `json:"frames"`
	AudioSamples   int    `json:"audio_samples"`
}

type artifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type parameterChange struct {
	MaxAbs float64 `json:"max_abs_change"`
}

type report struct {
	SchemaVersion     string                  `json:"schema_version"`
	GeneratedAtUTC    string                  `json:"generated_at_utc"`
	Dataset           string                  `json:"dataset"`
	ManifestSHA256    string                  `json:"manifest_sha256"`
	ReportPath        string                  `json:"report_path"`
	OutputDir         string                  `json:"output_dir"`
	Sources           []realdata.SourceRecord `json:"sources"`
	Clips             []clipRecord            `json:"clips"`
	SplitSourceIDs    map[string][]string     `json:"split_source_ids"`
	TrainTestDisjoint bool                    `json:"train_test_sources_disjoint"`
	Transform         realdata.Transform      `json:"transform"`
	OutputClock       realdata.OutputSpec     `json:"output_clock"`
	Toolchain         realdata.Toolchain      `json:"toolchain"`
	Training          trainingReport          `json:"training"`
	Inference         inferenceReport         `json:"inference"`
	Artifacts         []artifact              `json:"artifacts"`
	Limitations       []string                `json:"limitations"`
}

type trainingReport struct {
	Epochs               int                        `json:"epochs"`
	Updates              uint64                     `json:"updates"`
	Optimizer            string                     `json:"optimizer"`
	LearningRate         float64                    `json:"learning_rate"`
	Objective            string                     `json:"objective"`
	ScoresBefore         map[string]score           `json:"scores_before"`
	ScoresAfter          map[string]score           `json:"scores_after"`
	ParameterChanges     map[string]parameterChange `json:"parameter_changes"`
	CoreWeightsBeforeSHA string                     `json:"core_weights_before_sha256"`
	CoreWeightsAfterSHA  string                     `json:"core_weights_after_sha256"`
}

type previewFrame struct {
	Index            int   `json:"index"`
	PresentationMS   int64 `json:"presentation_millis"`
	SourceOffsetMS   int64 `json:"source_offset_millis"`
	AudioStartSample int   `json:"audio_start_sample"`
	AudioEndSample   int   `json:"audio_end_sample"`
}

type previewTimeline struct {
	SourceID        string         `json:"source_id"`
	SegmentID       string         `json:"segment_id"`
	Split           string         `json:"split"`
	SourceStartMS   int64          `json:"source_start_millis"`
	DurationMS      int64          `json:"duration_millis"`
	FrameRate       int            `json:"frame_rate"`
	AudioSampleRate int            `json:"audio_sample_rate"`
	Frames          []previewFrame `json:"frames"`
}

type inferenceReport struct {
	Method       string        `json:"method"`
	ContentCheck string        `json:"content_check"`
	TestSamples  int           `json:"test_samples"`
	PreviewMP4   bool          `json:"preview_mp4_created"`
	Outputs      []previewClip `json:"outputs"`
}

type previewClip struct {
	SourceID       string `json:"source_id"`
	SegmentID      string `json:"segment_id"`
	Frames         int    `json:"frames"`
	DurationMillis int64  `json:"duration_millis"`
	MP4            string `json:"mp4_path,omitempty"`
	WAV            string `json:"wav_path"`
	Timeline       string `json:"timeline_path"`
}

func main() {
	dataRoot := flag.String("data-root", "", "external directory containing the licensed source MP4 files")
	outDir := flag.String("out-dir", "", "new output directory outside the repository and data root")
	flag.Parse()
	if *dataRoot == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "realmedia: both --data-root and --out-dir are required")
		os.Exit(2)
	}
	if err := run(context.Background(), *dataRoot, *outDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dataRoot, outDir string) (runErr error) {
	loaded, err := realdata.ParseManifest(manifestJSON)
	if err != nil {
		return err
	}
	if err := validateExampleOutput(loaded.Manifest.Output); err != nil {
		return err
	}
	outputPath, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("realmedia: resolve output directory: %w", err)
	}
	dataPath, err := filepath.Abs(dataRoot)
	if err != nil {
		return fmt.Errorf("realmedia: resolve data directory: %w", err)
	}
	dataPath, err = filepath.EvalSymlinks(dataPath)
	if err != nil {
		return fmt.Errorf("realmedia: resolve data directory symlinks: %w", err)
	}
	outputParent, err := filepath.EvalSymlinks(filepath.Dir(outputPath))
	if err != nil {
		return fmt.Errorf("realmedia: output directory parent must already exist: %w", err)
	}
	outputPath = filepath.Join(outputParent, filepath.Base(outputPath))
	if filepath.Clean(outputPath) == filepath.Clean(dataPath) || pathWithin(outputPath, dataPath) || pathWithin(dataPath, outputPath) {
		return fmt.Errorf("realmedia: output directory must be separate from the raw data directory")
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return fmt.Errorf("realmedia: output directory %q already exists; choose a new path", outputPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("realmedia: check output directory: %w", err)
	}

	dataset, err := realdata.Import(ctx, dataPath, loaded.Manifest, realdata.DefaultLimits())
	if err != nil {
		return fmt.Errorf("realmedia: import licensed clips: %w", err)
	}
	fit, err := fitSamples(ctx, dataset.Samples, exampleEpochs)
	if err != nil {
		return err
	}
	modelBytes, err := json.MarshalIndent(fit.Snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("realmedia: encode trained model: %w", err)
	}
	changeReport := parameterChanges(fit.Before.Parameters, fit.Snapshot.Parameters)
	if changeReport["core_weights"].MaxAbs == 0 {
		return fmt.Errorf("realmedia: learning produced no core weight change")
	}

	if err := os.Mkdir(outputPath, 0o700); err != nil {
		return fmt.Errorf("realmedia: create new output directory: %w", err)
	}
	created := true
	defer func() {
		if runErr != nil && created {
			_ = os.RemoveAll(outputPath)
		}
	}()

	artifacts := make([]artifact, 0, 12)
	addArtifact := func(kind, path string) error {
		item, err := describeArtifact(kind, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, item)
		return nil
	}
	manifestPath := filepath.Join(outputPath, "manifest.json")
	if err := writeExclusive(manifestPath, manifestJSON); err != nil {
		return fmt.Errorf("realmedia: save exact input manifest: %w", err)
	}
	if err := addArtifact("manifest", manifestPath); err != nil {
		return err
	}
	modelPath := filepath.Join(outputPath, trainedModelFileName)
	if err := writeExclusive(modelPath, modelBytes); err != nil {
		return fmt.Errorf("realmedia: save trained model: %w", err)
	}
	if err := addArtifact("trained_model", modelPath); err != nil {
		return err
	}

	testCount := 0
	previewOutputs := make([]previewClip, 0, 1)
	previewMP4 := true
	for _, prediction := range fit.Predictions {
		if prediction.Split != "test" {
			continue
		}
		testCount++
		sample, ok := findSample(dataset.Samples, prediction.SourceID, prediction.SegmentID)
		if !ok {
			return fmt.Errorf("realmedia: missing test sample for prediction %s/%s", prediction.SourceID, prediction.SegmentID)
		}
		files, err := writePreview(ctx, outputPath, loaded.Manifest.Output, sample, prediction)
		if err != nil {
			return err
		}
		previewOutputs = append(previewOutputs, files.clip)
		artifacts = append(artifacts, files.artifacts...)
		if !files.mp4Created {
			previewMP4 = false
		}
	}
	if testCount == 0 {
		return fmt.Errorf("realmedia: independent test split has no samples")
	}

	sourceIDs := splitSourceIDs(dataset.Samples)
	reportValue := report{
		SchemaVersion: reportSchema, GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Dataset: dataset.Dataset, ManifestSHA256: loaded.SHA256,
		ReportPath: filepath.Join(outputPath, "report.json"), OutputDir: outputPath,
		Sources: dataset.Sources, Clips: summarizeClips(dataset.Samples),
		SplitSourceIDs: sourceIDs, TrainTestDisjoint: disjoint(sourceIDs["train"], sourceIDs["test"]),
		Transform: dataset.Transform, OutputClock: dataset.Output, Toolchain: dataset.Toolchain,
		Training: trainingReport{
			Epochs: exampleEpochs, Updates: fit.Snapshot.Updates, Optimizer: "learning.Trainer StepFrom with AdamW",
			LearningRate: fit.Snapshot.Options.LearningRate,
			Objective:    "per-step MSE on actual decoded RGB and embedded audio; image and audio group MSEs are equally weighted",
			ScoresBefore: fit.BeforeScores, ScoresAfter: fit.AfterScores, ParameterChanges: changeReport,
			CoreWeightsBeforeSHA: digestJSON(fit.Before.Parameters.Core.Weights),
			CoreWeightsAfterSHA:  digestJSON(fit.Snapshot.Parameters.Core.Weights),
		},
		Inference: inferenceReport{
			Method:       "rebuild learning.Network from the frozen post-training snapshot and call PredictAll for every frame; predictions are clipped to media ranges before PNG/WAV output",
			ContentCheck: "Flow completed, but the Big Buck Bunny test preview is nearly black and does not match the human annotation (white rabbit and green grass). The held-out source test balanced MSE did not improve.",
			TestSamples:  testCount, PreviewMP4: previewMP4, Outputs: previewOutputs,
		},
		Artifacts: artifacts,
		Limitations: []string{
			"This is an example-specific supervised sequence regressor, not a general text-to-video or text-to-audio model. Text uses signed FNV-1a hashed unigrams/bigrams plus four position features.",
			"The two training clips and one validation clip come from Elephants Dream; validation is not source-held-out. Big Buck Bunny is one independent held-out test source with one two-second clip, so it is not broad source-generalization evidence.",
			"The held-out source test score did not improve, and visual inspection found the generated preview does not match its annotation; the media processing path runs, but content quality did not pass.",
			"Frames are reduced to 8x8 RGB at 4 fps and embedded source audio is downmixed to mono 8 kHz. The MSE metrics do not measure perceptual quality, caption correctness, or semantic audio/video synchronization.",
			"The sample uses the MP4's embedded licensed film audio. Elephants Dream's separately distributed extra soundtrack files have a different NC-ND exception and are not used.",
			"tasks/media.VideoGenerator retains its synthetic RenderVideo target contract; this example trains actual decoded arrays through the public learning API without changing that generator.",
		},
	}
	for _, item := range artifacts {
		if filepath.Base(item.Path) == "report.json" {
			return fmt.Errorf("realmedia: internal artifact list unexpectedly contains the report itself")
		}
	}
	reportBytes, err := json.MarshalIndent(reportValue, "", "  ")
	if err != nil {
		return fmt.Errorf("realmedia: encode report: %w", err)
	}
	if err := writeExclusive(reportValue.ReportPath, reportBytes); err != nil {
		return fmt.Errorf("realmedia: save report: %w", err)
	}
	reportSHA := sha256.Sum256(reportBytes)
	fmt.Printf("realmedia report: %s\nreport SHA-256: %s\n", reportValue.ReportPath, hex.EncodeToString(reportSHA[:]))
	created = false
	return nil
}

type previewFiles struct {
	clip       previewClip
	artifacts  []artifact
	mp4Created bool
}

func writePreview(ctx context.Context, outputPath string, clock realdata.OutputSpec, sample realdata.Sample, prediction sequencePrediction) (previewFiles, error) {
	if len(prediction.Rows) != len(sample.Frames) {
		return previewFiles{}, fmt.Errorf("realmedia: test prediction row count %d does not match source frames %d", len(prediction.Rows), len(sample.Frames))
	}
	clipDir := filepath.Join(outputPath, prediction.SourceID+"-"+prediction.SegmentID)
	framesDir := filepath.Join(clipDir, "frames")
	if err := os.MkdirAll(framesDir, 0o700); err != nil {
		return previewFiles{}, fmt.Errorf("realmedia: create preview directory: %w", err)
	}
	allAudio := make([]float64, 0, len(sample.Audio))
	timeline := previewTimeline{
		SourceID: sample.SourceID, SegmentID: sample.SegmentID, Split: sample.Split,
		SourceStartMS: sample.StartMillis, DurationMS: sample.DurationMillis,
		FrameRate: clock.FrameRate, AudioSampleRate: clock.AudioSampleRate,
		Frames: make([]previewFrame, len(sample.Frames)),
	}
	artifacts := make([]artifact, 0, len(sample.Frames)+3)
	for frameIndex, row := range prediction.Rows {
		if len(row) != frameOutputSize {
			return previewFiles{}, fmt.Errorf("realmedia: test prediction row %d has %d values, want %d", frameIndex, len(row), frameOutputSize)
		}
		pixels := make([]float64, framePixels)
		for i := range pixels {
			pixels[i] = clamp(row[i]/imageOutputScale(), 0, 1)
		}
		framePath := filepath.Join(framesDir, fmt.Sprintf("frame-%02d.png", frameIndex))
		if _, err := media.WritePNG(framePath, pixels); err != nil {
			return previewFiles{}, fmt.Errorf("realmedia: write predicted PNG frame %d: %w", frameIndex, err)
		}
		if err := addPreviewArtifact(&artifacts, "predicted_frame_png", framePath); err != nil {
			return previewFiles{}, err
		}
		frame := sample.Frames[frameIndex]
		for i := frame.AudioStart; i < frame.AudioEnd; i++ {
			allAudio = append(allAudio, clamp(row[framePixels+i-frame.AudioStart]/audioOutputScale(), -1, 1))
		}
		timeline.Frames[frameIndex] = previewFrame{
			Index: frameIndex, PresentationMS: int64(frameIndex * 1000 / clock.FrameRate),
			SourceOffsetMS:   frame.TimeMillis - sample.StartMillis,
			AudioStartSample: frame.AudioStart, AudioEndSample: frame.AudioEnd,
		}
	}
	wavPath := filepath.Join(clipDir, "predicted.wav")
	if err := audio.WriteWAV(wavPath, audio.Signal{SampleRate: clock.AudioSampleRate, Samples: [][]float64{allAudio}}); err != nil {
		return previewFiles{}, fmt.Errorf("realmedia: write predicted WAV: %w", err)
	}
	if err := addPreviewArtifact(&artifacts, "predicted_audio_wav", wavPath); err != nil {
		return previewFiles{}, err
	}
	timelinePath := filepath.Join(clipDir, "timeline.json")
	timelineBytes, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		return previewFiles{}, err
	}
	if err := writeExclusive(timelinePath, timelineBytes); err != nil {
		return previewFiles{}, fmt.Errorf("realmedia: write synchronized preview timeline: %w", err)
	}
	if err := addPreviewArtifact(&artifacts, "preview_timeline", timelinePath); err != nil {
		return previewFiles{}, err
	}
	mp4Path := filepath.Join(clipDir, "predicted.mp4")
	mp4Created := false
	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		args := []string{
			"-nostdin", "-hide_banner", "-loglevel", "error", "-n",
			"-framerate", strconv.Itoa(clock.FrameRate), "-i", filepath.Join(framesDir, "frame-%02d.png"),
			"-i", wavPath, "-vf", "scale=320:320:flags=neighbor", "-c:v", "libx264", "-preset", "ultrafast",
			"-crf", "18", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "32k", "-ar", strconv.Itoa(clock.AudioSampleRate),
			"-shortest", "-movflags", "+faststart", mp4Path,
		}
		cmd := exec.CommandContext(ctx, ffmpeg, args...)
		output, encodeErr := cmd.CombinedOutput()
		if encodeErr != nil {
			return previewFiles{}, fmt.Errorf("realmedia: package synchronized preview MP4: %w: %s", encodeErr, strings.TrimSpace(string(output)))
		}
		mp4Created = true
		if err := addPreviewArtifact(&artifacts, "predicted_video_mp4", mp4Path); err != nil {
			return previewFiles{}, err
		}
	}
	return previewFiles{
		clip: previewClip{
			SourceID: sample.SourceID, SegmentID: sample.SegmentID,
			Frames: len(sample.Frames), DurationMillis: sample.DurationMillis,
			MP4: optionalPath(mp4Path, mp4Created), WAV: wavPath, Timeline: timelinePath,
		},
		artifacts: artifacts, mp4Created: mp4Created,
	}, nil
}

func addPreviewArtifact(artifacts *[]artifact, kind, path string) error {
	item, err := describeArtifact(kind, path)
	if err != nil {
		return err
	}
	*artifacts = append(*artifacts, item)
	return nil
}

func optionalPath(path string, include bool) string {
	if !include {
		return ""
	}
	return path
}

func validateExampleOutput(output realdata.OutputSpec) error {
	if output.FrameWidth != 8 || output.FrameHeight != 8 || output.FrameRate != 4 || output.AudioSampleRate != 8000 {
		return fmt.Errorf("realmedia: this sample trainer is configured for 8x8 RGB at 4 fps and 8 kHz mono audio")
	}
	return nil
}

func describeArtifact(kind, path string) (artifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return artifact{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return artifact{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return artifact{}, err
	}
	return artifact{Kind: kind, Path: path, Bytes: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func writeExclusive(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func parameterChanges(before, after learning.Parameters) map[string]parameterChange {
	return map[string]parameterChange{
		"encoder":      {MaxAbs: maxAbsDifference(before.Encoder, after.Encoder)},
		"core_weights": {MaxAbs: maxAbsDifference(before.Core.Weights, after.Core.Weights)},
		"core_bias":    {MaxAbs: maxAbsDifference(before.Core.Bias, after.Core.Bias)},
		"core_log_tau": {MaxAbs: maxAbsDifference(before.Core.LogTau, after.Core.LogTau)},
		"readout":      {MaxAbs: maxAbsDifference(before.Readout, after.Readout)},
	}
}

func maxAbsDifference(before, after []float64) float64 {
	if len(before) != len(after) {
		return math.Inf(1)
	}
	maximum := 0.0
	for i := range before {
		maximum = math.Max(maximum, math.Abs(after[i]-before[i]))
	}
	return maximum
}

func digestJSON(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func summarizeClips(samples []realdata.Sample) []clipRecord {
	clips := make([]clipRecord, 0, len(samples))
	for _, sample := range samples {
		clips = append(clips, clipRecord{
			SourceID: sample.SourceID, SegmentID: sample.SegmentID, Split: sample.Split,
			StartMillis: sample.StartMillis, DurationMillis: sample.DurationMillis,
			Prompt: sample.Prompt, PromptSource: sample.PromptSource, AnnotationNote: sample.AnnotationNote,
			Frames: len(sample.Frames), AudioSamples: len(sample.Audio),
		})
	}
	return clips
}

func splitSourceIDs(samples []realdata.Sample) map[string][]string {
	sets := map[string]map[string]bool{}
	for _, sample := range samples {
		if sets[sample.Split] == nil {
			sets[sample.Split] = map[string]bool{}
		}
		sets[sample.Split][sample.SourceID] = true
	}
	result := make(map[string][]string, len(sets))
	for split, ids := range sets {
		for id := range ids {
			result[split] = append(result[split], id)
		}
		// Dataset size is small; sort so reports are stable across runs.
		for i := 0; i < len(result[split]); i++ {
			for j := i + 1; j < len(result[split]); j++ {
				if result[split][j] < result[split][i] {
					result[split][i], result[split][j] = result[split][j], result[split][i]
				}
			}
		}
	}
	return result
}

func disjoint(left, right []string) bool {
	set := make(map[string]bool, len(left))
	for _, value := range left {
		set[value] = true
	}
	for _, value := range right {
		if set[value] {
			return false
		}
	}
	return len(left) > 0 && len(right) > 0
}

func findSample(samples []realdata.Sample, sourceID, segmentID string) (realdata.Sample, bool) {
	for _, sample := range samples {
		if sample.SourceID == sourceID && sample.SegmentID == segmentID {
			return sample, true
		}
	}
	return realdata.Sample{}, false
}

func clamp(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
