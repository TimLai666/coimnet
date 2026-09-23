package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

const asrFixtureSchemaVersion = "coimnet-asr-fixture/v1"

type asrFixtureDataScope struct {
	Data           string `json:"data"`
	Language       string `json:"language"`
	Resolution     string `json:"resolution"`
	SequenceLength string `json:"sequence_length"`
	Periphery      string `json:"periphery"`
}

type asrFixtureConfig struct {
	Seeds             []uint64             `json:"seeds"`
	UtterancesPerSeed int                  `json:"utterances_per_seed"`
	MinLen            int                  `json:"min_len"`
	MaxLen            int                  `json:"max_len"`
	Epochs            int                  `json:"epochs"`
	TestFraction      float64              `json:"test_fraction"`
	SplitSeed         uint64               `json:"split_seed"`
	StreamChunk       int                  `json:"stream_chunk"`
	Synth             asr.SynthConfig      `json:"synth"`
	Recognizer        asr.RecognizerConfig `json:"recognizer"`
}

type asrFixtureTiming struct {
	GenerationDurationNS int64 `json:"generation_duration_ns"`
	SplitDurationNS      int64 `json:"split_duration_ns"`
	EvalBeforeDurationNS int64 `json:"eval_before_duration_ns"`
	TrainingDurationNS   int64 `json:"training_duration_ns"`
	EvalAfterDurationNS  int64 `json:"eval_after_duration_ns"`
	StreamingDurationNS  int64 `json:"streaming_duration_ns"`
	TotalDurationNS      int64 `json:"total_duration_ns"`
}

type asrFixtureReport struct {
	SchemaVersion string               `json:"schema_version"`
	Config        asrFixtureConfig     `json:"config"`
	DataScope     asrFixtureDataScope  `json:"data_scope"`
	TrainSamples  int                  `json:"train_samples"`
	TestSamples   int                  `json:"test_samples"`
	SplitSeed     uint64               `json:"split_seed"`
	Epochs        int                  `json:"epochs"`
	Before        asr.EvalReport       `json:"before"`
	After         asr.EvalReport       `json:"after"`
	Streaming     asr.StreamEvalReport `json:"streaming"`
	Timing        asrFixtureTiming     `json:"timing"`
	Limitations   []string             `json:"limitations"`
}

// runASR runs the fixed, synthetic ASR fixture and writes its complete report
// to a new --out file when requested. The fixture's timing is measured and
// retained in the report, but it is never used for a pass/fail decision.
func runASR(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	config := newASRFixtureConfig()
	var outPath string
	fs := flag.NewFlagSet("examples run asr", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&outPath, "out", "", "new ASR fixture report JSON file; an existing path is never overwritten (default: stdout)")
	usageOutput := &outputCapture{writer: stdout}
	fs.Usage = func() {
		fmt.Fprintln(usageOutput, "Usage: coimnet examples run asr [--out FILE]")
		fmt.Fprintln(usageOutput, "Runs the fixed synthetic speech-to-text fixture: four generated groups of ten 1..3-rune tone utterances, split by speaker/session, trained for 10 epochs, and scored with whole-file and 173-sample streaming evaluation.")
		fmt.Fprintln(usageOutput, "The report records the synthetic data scope, front-end and recognizer configuration, split seed, CER/WER reports, per-utterance streaming measurements, local timing, and limitations. Timing is observational and is not used for deterministic comparisons or capability claims.")
		fmt.Fprintln(usageOutput, "Example: coimnet examples run asr --out asr-fixture.json")
		fmt.Fprintln(usageOutput, "Errors: an existing --out path, an --out parent that is missing or is not a directory, any positional argument, cancellation, numerical failure or output failure. Usage errors exit with status 1 and name the command.")
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return usageOutput.Err()
	} else if err != nil {
		return &ExitError{Code: exitUsage, Err: err}
	}
	if fs.NArg() != 0 {
		return &ExitError{Code: exitUsage, Err: fmt.Errorf("examples run asr takes no positional arguments; use examples run asr --help")}
	}
	if outPath != "" {
		if err := refuseExistingOut(outPath); err != nil {
			return &ExitError{Code: exitUsage, Err: err}
		}
	}

	report, err := runASRFixture(ctx, config)
	if err != nil {
		return err
	}
	if outPath == "" {
		return writeJSON(stdout, report)
	} else {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode ASR fixture report: %w", err)
		}
		if err := writeNewTextFile(outPath, string(data)+"\n"); err != nil {
			return &ExitError{Code: exitUsage, Err: fmt.Errorf("publish --out %q: %w", outPath, err)}
		}
	}
	_, writeErr := fmt.Fprintf(stdout, "asr: train %d/test %d, full-file CER %.4f -> %.4f, streaming CER %.4f (%d measurements)\n",
		report.TrainSamples, report.TestSamples, report.Before.CER.Rate, report.After.CER.Rate,
		report.Streaming.CER.Rate, len(report.Streaming.Measurements))
	return writeErr
}

func newASRFixtureConfig() asrFixtureConfig {
	return asrFixtureConfig{
		Seeds:             []uint64{1, 2, 3, 4},
		UtterancesPerSeed: 10,
		MinLen:            1,
		MaxLen:            3,
		Epochs:            10,
		TestFraction:      0.25,
		SplitSeed:         42,
		StreamChunk:       173,
		Synth: asr.SynthConfig{
			SampleRate: 16000,
			DurationMS: 120,
			GapMS:      40,
			Noise:      0.01,
			Alphabet:   []rune{'a', 'b', 'c'},
		},
		Recognizer: asr.RecognizerConfig{
			SampleRate: 16000,
			FrontEnd: audio.FrontEndConfig{
				Window: 256,
				Hop:    128,
				Mels:   12,
				FMin:   0,
				FMax:   8000,
				Floor:  1e-10,
			},
			Hidden:       24,
			Alphabet:     []rune{'a', 'b', 'c'},
			LearningRate: 0.02,
			Seed:         5,
		},
	}
}

func newASRFixtureDataScope(c asrFixtureConfig) asrFixtureDataScope {
	return asrFixtureDataScope{
		Data:           "synthetic pure tones generated in memory; no WAV files and no natural speech",
		Language:       fmt.Sprintf("an alphabet of %d runes (abc) with no natural-language statistics", len(c.Synth.Alphabet)),
		Resolution:     fmt.Sprintf("%d Hz mono float64 waveform", c.Synth.SampleRate),
		SequenceLength: fmt.Sprintf("%d..%d runes; %d ms per tone plus %d ms gaps", c.MinLen, c.MaxLen, c.Synth.DurationMS, c.Synth.GapMS),
		Periphery:      "log-mel front end, identity encoder, recurrent core readout and greedy CTC decoder",
	}
}

func asrFixtureLimitations() []string {
	return []string{
		"This fixture uses four groups of synthetic pure tones for the three-rune alphabet abc, not natural speech or user-provided WAV recordings.",
		"The reported CER/WER and streaming timings are evidence that this offline fixture path runs; they are not a natural-language, speaker-independent, alignment-accuracy or production-latency claim.",
		"Training and evaluation run on one local CPU process; timing values depend on the machine, Go runtime and current system load.",
		"Real licensed-data import, training, inference and evaluation remain blocked until the user supplies the required licensed corpus and manifest.",
	}
}

func runASRFixture(ctx context.Context, c asrFixtureConfig) (asrFixtureReport, error) {
	var zero asrFixtureReport
	started := time.Now()
	if ctx == nil {
		return zero, errors.New("asr fixture: nil context")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	report := asrFixtureReport{
		SchemaVersion: asrFixtureSchemaVersion,
		Config:        c,
		DataScope:     newASRFixtureDataScope(c),
		SplitSeed:     c.SplitSeed,
		Epochs:        c.Epochs,
		Limitations:   asrFixtureLimitations(),
	}

	generationStarted := time.Now()
	utterances := make([]asr.Utterance, 0, len(c.Seeds)*c.UtterancesPerSeed)
	for _, seed := range c.Seeds {
		if err := ctx.Err(); err != nil {
			return zero, fmt.Errorf("asr fixture generation before seed %d: %w", seed, err)
		}
		generated, err := asr.Generate(c.Synth, c.UtterancesPerSeed, c.MinLen, c.MaxLen, seed)
		if err != nil {
			return zero, fmt.Errorf("asr fixture generation seed %d: %w", seed, err)
		}
		utterances = append(utterances, generated...)
	}
	report.Timing.GenerationDurationNS = elapsedNanoseconds(generationStarted)

	splitStarted := time.Now()
	train, test, err := asr.SplitBySpeakerSession(utterances, c.TestFraction, c.SplitSeed)
	if err != nil {
		return zero, fmt.Errorf("asr fixture split: %w", err)
	}
	report.Timing.SplitDurationNS = elapsedNanoseconds(splitStarted)
	report.TrainSamples = len(train)
	report.TestSamples = len(test)

	recognizer, err := asr.NewRecognizer(c.Recognizer)
	if err != nil {
		return zero, fmt.Errorf("asr fixture recognizer: %w", err)
	}

	evalBeforeStarted := time.Now()
	report.Before, err = recognizer.Evaluate(ctx, test)
	if err != nil {
		return zero, fmt.Errorf("asr fixture whole-file evaluation before training: %w", err)
	}
	report.Timing.EvalBeforeDurationNS = elapsedNanoseconds(evalBeforeStarted)

	trainingStarted := time.Now()
	for epoch := 0; epoch < c.Epochs; epoch++ {
		for i, utterance := range train {
			if err := ctx.Err(); err != nil {
				return zero, fmt.Errorf("asr fixture training epoch %d utterance %d: %w", epoch, i, err)
			}
			if _, err := recognizer.TrainUtterance(ctx, utterance.Samples, utterance.Text); err != nil {
				return zero, fmt.Errorf("asr fixture training epoch %d utterance %d: %w", epoch, i, err)
			}
		}
	}
	report.Timing.TrainingDurationNS = elapsedNanoseconds(trainingStarted)

	evalAfterStarted := time.Now()
	report.After, err = recognizer.Evaluate(ctx, test)
	if err != nil {
		return zero, fmt.Errorf("asr fixture whole-file evaluation after training: %w", err)
	}
	report.Timing.EvalAfterDurationNS = elapsedNanoseconds(evalAfterStarted)

	streamingStarted := time.Now()
	report.Streaming, err = recognizer.EvaluateStreaming(ctx, test, c.StreamChunk)
	if err != nil {
		return zero, fmt.Errorf("asr fixture streaming evaluation: %w", err)
	}
	report.Timing.StreamingDurationNS = elapsedNanoseconds(streamingStarted)
	report.Timing.TotalDurationNS = elapsedNanoseconds(started)
	return report, nil
}

func elapsedNanoseconds(start time.Time) int64 {
	d := time.Since(start)
	if d < 0 {
		return 0
	}
	return d.Nanoseconds()
}
