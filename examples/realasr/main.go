// Command realasr imports one user-provided licensed ASR manifest, trains the
// shared recognizer once on each training utterance, and prints an auditable
// JSON report. It does not download data or persist model parameters.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	osSignal "os/signal"
	"sort"
	"syscall"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

const (
	realASRSchemaVersion = "coimnet-real-asr-example/v1"
	realASRSampleRate    = 16000
	realASRPeak          = 0.8
	realASRTestFraction  = 1.0 / 3.0
	realASRSplitSeed     = uint64(42)
	realASRModelSeed     = uint64(5)
	realASRHidden        = 24
	realASRLearningRate  = 0.001
)

var realASRFrontEnd = audio.FrontEndConfig{
	Window: 256,
	Hop:    128,
	Mels:   12,
	FMin:   0,
	FMax:   8000,
	Floor:  1e-10,
}

// realASRReport is deliberately a data-only report. The model itself stays in
// memory for the run and no checkpoint or temporary model file is created.
type realASRReport struct {
	SchemaVersion    string                  `json:"schema_version"`
	ManifestSHA256   string                  `json:"manifest_sha256"`
	ImportedCount    int                     `json:"imported_count"`
	TrainCount       int                     `json:"train_count"`
	TestCount        int                     `json:"test_count"`
	RetainedCount    int                     `json:"retained_count"`
	ImpossibleCount  int                     `json:"impossible_count"`
	Alphabet         []string                `json:"alphabet"`
	License          asr.License             `json:"license"`
	SourceMetadata   []asr.UtteranceMetadata `json:"source_metadata"`
	Settings         realASRSettings         `json:"settings"`
	Before           asr.EvalReport          `json:"before"`
	After            asr.EvalReport          `json:"after"`
	FirstRetained    realASRFirstRetained    `json:"first_retained"`
	ScopeLimitations []string                `json:"scope_limitations"`
}

type realASRSettings struct {
	SampleRate   int                  `json:"sample_rate"`
	Peak         float64              `json:"peak"`
	TestFraction float64              `json:"test_fraction"`
	SplitSeed    uint64               `json:"split_seed"`
	FrontEnd     audio.FrontEndConfig `json:"front_end"`
	Hidden       int                  `json:"hidden"`
	LearningRate float64              `json:"learning_rate"`
	Seed         uint64               `json:"seed"`
}

type realASRFirstRetained struct {
	Reference     string `json:"reference"`
	Transcription string `json:"transcription"`
}

func main() {
	ctx, stop := osSignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the standalone example's arguments and prints one JSON report to
// stdout. Errors are returned to main, which writes them to stderr.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if ctx == nil || stdout == nil || stderr == nil {
		return errors.New("context and output writers are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	fs := flag.NewFlagSet("realasr", flag.ContinueOnError)
	diagnosticOutput := &errorTrackingWriter{Writer: stderr}
	fs.SetOutput(diagnosticOutput)
	usageOutput := &errorTrackingWriter{Writer: stdout}
	var manifestPath string
	fs.StringVar(&manifestPath, "manifest", "", "licensed coimnet-asr-dataset/v1 manifest to import")
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: go run ./examples/realasr --manifest PATH")
		fmt.Fprintln(usageOutput, "Imports the supplied licensed PCM16 WAV manifest, splits by speaker/session, trains one fixed recognizer update per training sample, and prints one JSON report. No data is downloaded and no model is saved.")
		fmt.Fprintln(usageOutput, "Defaults: target audio 16 kHz, peak 0.8; test fraction 1/3 with split seed 42; log-mel window/hop/mels 256/128/12 (FMin 0, FMax 8000, floor 1e-10); hidden 24; learning rate 0.001; model seed 5.")
		fmt.Fprintln(usageOutput, "Report: imported/train/test counts, impossible training count, sorted training alphabet, before/after CER and WER on the held-out set, an independent Transcribe result for the first held-out sample, manifest and decoded WAV SHA-256 values, license metadata and scope limitations.")
		fmt.Fprintln(usageOutput, "Limitations: one pass over the supplied recordings; no natural-speech quality claim, no word-timing claim, no model checkpoint, and no GPU execution claim. Low accuracy is reported as observed rather than treated as success.")
		fmt.Fprintln(usageOutput, "Errors: missing or extra arguments, unreadable or malformed manifest/WAV, missing license fields, unsupported WAV, invalid split, empty training alphabet, unknown training rune, cancellation, numerical failure or output failure.")
		fmt.Fprintln(usageOutput, "Options:")
		fs.SetOutput(usageOutput)
		fs.PrintDefaults()
		fs.SetOutput(diagnosticOutput)
	}

	if err := fs.Parse(args); err != nil {
		if diagnosticErr := diagnosticOutput.Err(); diagnosticErr != nil {
			return errors.Join(err, fmt.Errorf("write flag diagnostic: %w", diagnosticErr))
		}
		if usageErr := usageOutput.Err(); usageErr != nil {
			return fmt.Errorf("write usage: %w", usageErr)
		}
		if errors.Is(err, flag.ErrHelp) {
			if fs.NArg() != 0 {
				return errors.New("unexpected positional arguments")
			}
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if manifestPath == "" {
		return errors.New("--manifest is required; use --help")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return execute(ctx, manifestPath, stdout)
}

func execute(ctx context.Context, manifestPath string, stdout io.Writer) error {
	dataset, err := asr.ReadDataset(manifestPath, realASRSampleRate, realASRPeak)
	if err != nil {
		return fmt.Errorf("import dataset: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	train, test, err := asr.SplitBySpeakerSession(dataset.Utterances, realASRTestFraction, realASRSplitSeed)
	if err != nil {
		return fmt.Errorf("split dataset: %w", err)
	}
	if len(train) == 0 || len(test) == 0 {
		return fmt.Errorf("split produced %d training and %d held-out utterances; both sets must be non-empty", len(train), len(test))
	}

	alphabetRunes := trainingAlphabet(train)
	if len(alphabetRunes) == 0 {
		return errors.New("training texts produce an empty rune alphabet")
	}
	recognizer, err := asr.NewRecognizer(asr.RecognizerConfig{
		SampleRate:   realASRSampleRate,
		FrontEnd:     realASRFrontEnd,
		Hidden:       realASRHidden,
		Alphabet:     alphabetRunes,
		LearningRate: realASRLearningRate,
		Seed:         realASRModelSeed,
	})
	if err != nil {
		return fmt.Errorf("build recognizer: %w", err)
	}

	before, err := recognizer.Evaluate(ctx, test)
	if err != nil {
		return fmt.Errorf("evaluate before training: %w", err)
	}
	impossible := 0
	for i, utterance := range train {
		if err := ctx.Err(); err != nil {
			return err
		}
		training, err := recognizer.TrainUtterance(ctx, utterance.Samples, utterance.Text)
		if err != nil {
			return fmt.Errorf("train utterance %d: %w", i, err)
		}
		if training.Impossible {
			impossible++
		}
	}
	after, err := recognizer.Evaluate(ctx, test)
	if err != nil {
		return fmt.Errorf("evaluate after training: %w", err)
	}
	independent, err := recognizer.Transcribe(ctx, test[0].Samples)
	if err != nil {
		return fmt.Errorf("transcribe first held-out utterance: %w", err)
	}

	report := realASRReport{
		SchemaVersion:   realASRSchemaVersion,
		ManifestSHA256:  dataset.ManifestSHA256,
		ImportedCount:   len(dataset.Utterances),
		TrainCount:      len(train),
		TestCount:       len(test),
		RetainedCount:   len(test),
		ImpossibleCount: impossible,
		Alphabet:        runeStrings(alphabetRunes),
		License:         dataset.License,
		SourceMetadata:  dataset.Metadata,
		Settings: realASRSettings{
			SampleRate:   realASRSampleRate,
			Peak:         realASRPeak,
			TestFraction: realASRTestFraction,
			SplitSeed:    realASRSplitSeed,
			FrontEnd:     realASRFrontEnd,
			Hidden:       realASRHidden,
			LearningRate: realASRLearningRate,
			Seed:         realASRModelSeed,
		},
		Before: before,
		After:  after,
		FirstRetained: realASRFirstRetained{
			Reference:     test[0].Text,
			Transcription: independent,
		},
		ScopeLimitations: []string{
			"This report covers only the user-supplied manifest and its referenced PCM16 WAV recordings.",
			"Training runs one TrainUtterance call per training recording and does not establish convergence or production accuracy.",
			"The held-out split is by connected speaker/session groups; with a small or imbalanced corpus it may be small and generalisation may be weak.",
			"CER/WER are offline transcript metrics; no natural-speech, word-timing, GPU, cross-platform or full-brain claim is made.",
		},
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func trainingAlphabet(utterances []asr.Utterance) []rune {
	seen := make(map[rune]struct{})
	for _, utterance := range utterances {
		for _, r := range utterance.Text {
			seen[r] = struct{}{}
		}
	}
	alphabet := make([]rune, 0, len(seen))
	for r := range seen {
		alphabet = append(alphabet, r)
	}
	sort.Slice(alphabet, func(i, j int) bool { return alphabet[i] < alphabet[j] })
	return alphabet
}

func runeStrings(alphabet []rune) []string {
	out := make([]string, len(alphabet))
	for i, r := range alphabet {
		out[i] = string(r)
	}
	return out
}

type errorTrackingWriter struct {
	io.Writer
	err error
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

func (w *errorTrackingWriter) Err() error { return w.err }
