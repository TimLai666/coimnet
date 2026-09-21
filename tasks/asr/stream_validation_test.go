package asr_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestStreamFlushesAnInitialShortTail(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if _, err := s.Feed(context.Background(), []float64{0.25}); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	before := s.Snapshot()
	if before.Individual.Neural.Continuous == nil {
		t.Fatal("stream snapshot has no continuous state")
	}
	if before.Individual.Neural.Continuous.Steps != 0 {
		t.Fatalf("Feed ran %d steps before a complete window", before.Individual.Neural.Continuous.Steps)
	}
	if _, err := s.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	after := s.Snapshot()
	if after.Individual.Neural.Continuous.Steps != 1 {
		t.Fatalf("Flush ran %d steps for an initial short tail, want 1", after.Individual.Neural.Continuous.Steps)
	}
}

func TestStreamCanceledFeedDoesNotConsumeAnInsufficientFrame(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	before := s.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Feed(ctx, []float64{0.25}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Feed error = %v, want context.Canceled", err)
	}
	after := s.Snapshot()
	if len(after.Pending) != len(before.Pending) {
		t.Fatalf("canceled Feed left %d pending samples, want %d", len(after.Pending), len(before.Pending))
	}
	if len(after.Pending) != 0 {
		t.Fatal("canceled Feed consumed an insufficient frame")
	}
}

func TestStreamCanceledFlushPreservesItsTail(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if _, err := s.Feed(context.Background(), []float64{0.25}); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	before := s.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Flush(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Flush error = %v, want context.Canceled", err)
	}
	after := s.Snapshot()
	if after.Flushed {
		t.Fatal("canceled Flush marked the stream flushed")
	}
	if len(after.Pending) != len(before.Pending) || after.Pending[0] != before.Pending[0] {
		t.Fatal("canceled Flush changed the pending tail")
	}
	if _, err := s.Flush(context.Background()); err != nil {
		t.Fatalf("retry Flush: %v", err)
	}
}

func TestStreamRollsBackAChunkWhenAWindowFails(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	window := recognizerConfig().FrontEnd.Window
	x := make([]float64, window*2)
	x[window] = math.MaxFloat64
	before := s.Snapshot()
	if _, err := s.Feed(context.Background(), x); err == nil {
		t.Fatal("Feed accepted a non-finite feature window")
	}
	after := s.Snapshot()
	if len(after.Pending) != len(before.Pending) || after.Emitted != before.Emitted || after.PrevClass != before.PrevClass {
		t.Fatal("failed Feed left partial stream metadata")
	}
	if after.Individual.Neural.Continuous.Steps != before.Individual.Neural.Continuous.Steps {
		t.Fatalf("failed Feed left %d neural steps, want %d", after.Individual.Neural.Continuous.Steps, before.Individual.Neural.Continuous.Steps)
	}

	tail, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream tail: %v", err)
	}
	if _, err := tail.Feed(context.Background(), []float64{0, math.MaxFloat64}); err != nil {
		t.Fatalf("Feed tail: %v", err)
	}
	before = tail.Snapshot()
	if _, err := tail.Flush(context.Background()); err == nil {
		t.Fatal("Flush accepted a non-finite padded feature window")
	}
	after = tail.Snapshot()
	if after.Flushed || len(after.Pending) != len(before.Pending) || after.Individual.Neural.Continuous.Steps != 0 {
		t.Fatal("failed Flush left partial stream state")
	}
}

type cancelAfterErrContext struct {
	context.Context
	cancelAt int
	calls    int
	cancel   context.CancelFunc
}

func (c *cancelAfterErrContext) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestStreamRollsBackWhenCanceledMidChunk(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	before := s.Snapshot()
	base, cancel := context.WithCancel(context.Background())
	ctx := &cancelAfterErrContext{Context: base, cancelAt: 50, cancel: cancel}
	x := make([]float64, recognizerConfig().FrontEnd.Window*10)
	if _, err := s.Feed(ctx, x); !errors.Is(err, context.Canceled) {
		t.Fatalf("Feed error = %v, want context.Canceled", err)
	}
	if ctx.calls <= 2 {
		t.Fatalf("context canceled before window processing, Err calls = %d", ctx.calls)
	}
	after := s.Snapshot()
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("mid-chunk cancellation changed the full snapshot:\n before: %#v\n after:  %#v", before, after)
	}

	if _, err := s.Feed(context.Background(), x); err != nil {
		t.Fatalf("retry Feed: %v", err)
	}
	if _, err := s.Flush(context.Background()); err != nil {
		t.Fatalf("retry Flush: %v", err)
	}
	retried := s.Snapshot()

	control, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream control: %v", err)
	}
	if _, err := control.Feed(context.Background(), x); err != nil {
		t.Fatalf("control Feed: %v", err)
	}
	if _, err := control.Flush(context.Background()); err != nil {
		t.Fatalf("control Flush: %v", err)
	}
	if controlState := control.Snapshot(); !reflect.DeepEqual(retried, controlState) {
		t.Fatalf("retry diverged from uninterrupted stream:\n retry:   %#v\n control: %#v", retried, controlState)
	}
}

func TestRestoreStreamRequiresRecognizerSemantics(t *testing.T) {
	base := recognizerConfig()
	source, err := asr.NewRecognizer(base)
	if err != nil {
		t.Fatalf("NewRecognizer source: %v", err)
	}
	s, err := source.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	state := s.Snapshot()

	cases := []struct {
		name   string
		mutate func(*asr.RecognizerConfig)
	}{
		{"sample rate", func(c *asr.RecognizerConfig) { c.SampleRate = 18000 }},
		{"front end", func(c *asr.RecognizerConfig) { c.FrontEnd.FMin = 100 }},
		{"alphabet", func(c *asr.RecognizerConfig) { c.Alphabet[0] = 'x' }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Alphabet = append([]rune(nil), base.Alphabet...)
			tc.mutate(&cfg)
			target, err := asr.NewRecognizer(cfg)
			if err != nil {
				t.Fatalf("NewRecognizer target: %v", err)
			}
			if _, err := target.RestoreStream(state); err == nil {
				t.Fatalf("RestoreStream accepted a snapshot with a different %s", tc.name)
			}
		})
	}
}

func TestStreamSnapshotOwnsItsAlphabet(t *testing.T) {
	cfg := recognizerConfig()
	r, err := asr.NewRecognizer(cfg)
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	cfg.Alphabet[0] = 'x'
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	state := s.Snapshot()
	if state.Alphabet[0] != 'a' {
		t.Fatalf("snapshot alphabet[0] = %q, want %q", state.Alphabet[0], 'a')
	}
	state.Alphabet[0] = 'x'
	if got := s.Snapshot().Alphabet[0]; got != 'a' {
		t.Fatalf("mutating a snapshot changed the stream alphabet to %q", got)
	}
}

func TestRestoreStreamRejectsInvalidStateEncoding(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	state := s.Snapshot()

	badText := state
	badText.Emitted = string([]byte{0xff})
	if _, err := r.RestoreStream(badText); err == nil {
		t.Fatal("RestoreStream accepted invalid UTF-8 in emitted text")
	}

	badPending := state
	badPending.Flushed = true
	badPending.Pending = []float64{0.25}
	if _, err := r.RestoreStream(badPending); err == nil {
		t.Fatal("RestoreStream accepted a flushed snapshot with pending samples")
	}

	badVersion := state
	badVersion.FrontEndVersion = "future-logmel/v2"
	if _, err := r.RestoreStream(badVersion); err == nil {
		t.Fatal("RestoreStream accepted an unknown front-end version")
	}

	badMechanism := state
	badMechanism.Individual.Plastic = &learning.PlasticPart{}
	if _, err := r.RestoreStream(badMechanism); err == nil {
		t.Fatal("RestoreStream accepted a plastic individual")
	}
	badMechanism = state
	badMechanism.Individual.Chemical = &learning.ChemicalPart{}
	if _, err := r.RestoreStream(badMechanism); err == nil {
		t.Fatal("RestoreStream accepted a chemical individual")
	}

	badPending = state
	badPending.Pending = make([]float64, recognizerConfig().FrontEnd.Window)
	if _, err := r.RestoreStream(badPending); err == nil {
		t.Fatal("RestoreStream accepted pending samples that already form a window")
	}

	badText = state
	badText.Emitted = "z"
	if _, err := r.RestoreStream(badText); err == nil {
		t.Fatal("RestoreStream accepted emitted text outside the alphabet")
	}
	badText = state
	badText.PrevClass = 1
	if _, err := r.RestoreStream(badText); err == nil {
		t.Fatal("RestoreStream accepted a previous class before its first frame")
	}

	framed, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream framed: %v", err)
	}
	if _, err := framed.Feed(context.Background(), make([]float64, recognizerConfig().FrontEnd.Window)); err != nil {
		t.Fatalf("Feed one frame: %v", err)
	}
	framedState := framed.Snapshot()
	badText = framedState
	badText.Emitted = "aa"
	if _, err := r.RestoreStream(badText); err == nil {
		t.Fatal("RestoreStream accepted more emitted runes than frames")
	}
	badText = framedState
	badText.Emitted = "b"
	badText.PrevClass = 1
	if _, err := r.RestoreStream(badText); err == nil {
		t.Fatal("RestoreStream accepted a previous class inconsistent with emitted text")
	}
	badPending = framedState
	badPending.Pending = nil
	if _, err := r.RestoreStream(badPending); err == nil {
		t.Fatal("RestoreStream accepted a processed stream without its overlap")
	}
	if _, err := framed.Flush(context.Background()); err != nil {
		t.Fatalf("Flush framed stream: %v", err)
	}
	resumedFramed, err := r.RestoreStream(framed.Snapshot())
	if err != nil {
		t.Fatalf("RestoreStream framed flushed: %v", err)
	}
	if _, err := resumedFramed.Feed(context.Background(), []float64{0.25}); err == nil {
		t.Fatal("restored non-empty flushed stream accepted Feed")
	}
	if _, err := resumedFramed.Flush(context.Background()); err == nil {
		t.Fatal("restored non-empty flushed stream accepted second Flush")
	}

	flushed, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream flushed: %v", err)
	}
	if _, err := flushed.Flush(context.Background()); err != nil {
		t.Fatalf("Flush empty stream: %v", err)
	}
	resumed, err := r.RestoreStream(flushed.Snapshot())
	if err != nil {
		t.Fatalf("RestoreStream flushed: %v", err)
	}
	if _, err := resumed.Feed(context.Background(), []float64{0.25}); err == nil {
		t.Fatal("restored flushed stream accepted Feed")
	}
	if _, err := resumed.Flush(context.Background()); err == nil {
		t.Fatal("restored flushed stream accepted a second Flush")
	}
}

func TestRestoreStreamValidatesCTCFrameMinimum(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	front := recognizerConfig().FrontEnd

	one, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream one: %v", err)
	}
	if _, err := one.Feed(context.Background(), make([]float64, front.Window)); err != nil {
		t.Fatalf("Feed one frame: %v", err)
	}
	stateOne := one.Snapshot()
	stateOne.Emitted = "a"
	stateOne.PrevClass = -1
	if _, err := r.RestoreStream(stateOne); err == nil {
		t.Fatal("RestoreStream accepted one emitted rune followed by blank after one frame")
	}

	two, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream two: %v", err)
	}
	if _, err := two.Feed(context.Background(), make([]float64, front.Window+front.Hop)); err != nil {
		t.Fatalf("Feed two frames: %v", err)
	}
	stateTwo := two.Snapshot()
	validTwo := stateTwo
	validTwo.Emitted = "a"
	validTwo.PrevClass = -1
	if _, err := r.RestoreStream(validTwo); err != nil {
		t.Fatalf("RestoreStream rejected two frames for a+blank: %v", err)
	}

	badTwo := stateTwo
	badTwo.Emitted = "aa"
	badTwo.PrevClass = 1
	if _, err := r.RestoreStream(badTwo); err == nil {
		t.Fatal("RestoreStream accepted two emitted equal runes without a separating blank")
	}

	three, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream three: %v", err)
	}
	if _, err := three.Feed(context.Background(), make([]float64, front.Window+2*front.Hop)); err != nil {
		t.Fatalf("Feed three frames: %v", err)
	}
	stateThree := three.Snapshot()
	validThree := stateThree
	validThree.Emitted = "aa"
	validThree.PrevClass = 1
	if _, err := r.RestoreStream(validThree); err != nil {
		t.Fatalf("RestoreStream rejected three frames for aa with previous class a: %v", err)
	}
}

func TestNewStreamRejectsInvalidAlphabetRune(t *testing.T) {
	cfg := recognizerConfig()
	cfg.Alphabet = []rune{0xd800, 'b', 'c'}
	r, err := asr.NewRecognizer(cfg)
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	if _, err := r.NewStream(); err == nil {
		t.Fatal("NewStream accepted an invalid Unicode alphabet rune")
	}
}

func TestStreamSnapshotUsesCurrentFrontEndVersion(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if got := s.Snapshot().FrontEndVersion; got != audio.FrontEndVersion {
		t.Fatalf("snapshot front-end version = %q, want %q", got, audio.FrontEndVersion)
	}
}

func TestTranscribeStreamingAcceptsLargestChunk(t *testing.T) {
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		t.Fatalf("NewRecognizer: %v", err)
	}
	maxInt := int(^uint(0) >> 1)
	if _, err := r.TranscribeStreaming(context.Background(), []float64{0.25}, maxInt); err != nil {
		t.Fatalf("TranscribeStreaming with max chunk: %v", err)
	}
}
