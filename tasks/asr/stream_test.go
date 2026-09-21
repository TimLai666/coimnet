package asr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

// streamFixtureState is the one trained recognizer every streaming test reads.
// Training it is the expensive part of these tests and none of them changes
// it: Transcribe, NewStream and RestoreStream all read the trainer without
// touching it, so one fixture is built lazily and shared.
type streamFixtureState struct {
	recognizer *asr.Recognizer
	test       []asr.Utterance
	// sample is one held-out recording, truncated to Window + k*Hop so the
	// whole-file and streaming framings cover exactly the same samples, chosen
	// so that both the whole file and its first half decode to a non-empty
	// string: a fixture that emitted nothing would let every comparison here
	// pass on two empty strings.
	sample []float64
	// half is the prefix length the split tests feed first, and prefix is what
	// a fresh stream emits from that prefix alone.
	half   int
	prefix string
	err    error
}

var (
	streamFixtureOnce sync.Once
	streamFixture     streamFixtureState
)

// alignedSamples truncates a signal to Window + k*Hop samples, the lengths
// whose last analysis window ends exactly at the last sample. On one of these
// the streaming tail is exactly the Window-Hop overlap the next window would
// have reused, so Flush adds no frame that Features did not produce and the
// two modes are comparable frame for frame.
func alignedSamples(x []float64, c audio.FrontEndConfig) []float64 {
	if len(x) < c.Window {
		return x
	}
	return x[:c.Window+(len(x)-c.Window)/c.Hop*c.Hop]
}

// streamTestFixture trains the shared recognizer on first use and reports the
// held-out set and the recording the split tests use.
func streamTestFixture(t *testing.T) streamFixtureState {
	t.Helper()
	streamFixtureOnce.Do(buildStreamFixture)
	if streamFixture.err != nil {
		t.Fatalf("streaming fixture: %v", streamFixture.err)
	}
	return streamFixture
}

// buildStreamFixture trains on the same synthetic set and for the same number
// of epochs as the whole-file tests, then picks the recording described by
// streamFixtureState.sample.
func buildStreamFixture() {
	ctx := context.Background()
	c := synthConfig()
	train, err := asr.Generate(c, 40, 1, 3, 1)
	if err != nil {
		streamFixture.err = err
		return
	}
	test, err := asr.Generate(c, 15, 1, 3, 2)
	if err != nil {
		streamFixture.err = err
		return
	}
	r, err := asr.NewRecognizer(recognizerConfig())
	if err != nil {
		streamFixture.err = err
		return
	}
	for epoch := 0; epoch < trainingEpochs; epoch++ {
		for _, u := range train {
			if _, err := r.TrainUtterance(ctx, u.Samples, u.Text); err != nil {
				streamFixture.err = err
				return
			}
		}
	}
	streamFixture.recognizer, streamFixture.test = r, test
	front := recognizerConfig().FrontEnd
	best := ""
	for _, u := range test {
		x := alignedSamples(u.Samples, front)
		whole, err := r.Transcribe(ctx, x)
		if err != nil {
			streamFixture.err = err
			return
		}
		if len(whole) <= len(best) {
			continue
		}
		s, err := r.NewStream()
		if err != nil {
			streamFixture.err = err
			return
		}
		half := len(x) / 2
		prefix, err := s.Feed(ctx, x[:half])
		if err != nil {
			streamFixture.err = err
			return
		}
		if prefix == "" {
			continue
		}
		best = whole
		streamFixture.sample, streamFixture.half, streamFixture.prefix = x, half, prefix
	}
	if best == "" {
		streamFixture.err = errNoDecodableUtterance
	}
}

// errNoDecodableUtterance says the trained fixture emitted nothing on any
// held-out recording, which would leave every streaming comparison comparing
// two empty strings.
var errNoDecodableUtterance = &fixtureError{"no held-out utterance decodes to a non-empty string with a non-empty first half"}

// fixtureError is a fixture failure that is not worth an exported type.
type fixtureError struct{ msg string }

func (e *fixtureError) Error() string { return e.msg }

// TestStreamMatchesWholeFile pins the point of the streaming mode: on a signal
// whose length is Window + k*Hop, feeding the audio in chunks decodes exactly
// what the whole-file path decodes, whatever the chunk size.
func TestStreamMatchesWholeFile(t *testing.T) {
	ctx := context.Background()
	f := streamTestFixture(t)
	front := recognizerConfig().FrontEnd
	for i, u := range f.test {
		x := alignedSamples(u.Samples, front)
		want, err := f.recognizer.Transcribe(ctx, x)
		if err != nil {
			t.Fatalf("Transcribe utterance %d: %v", i, err)
		}
		got, err := f.recognizer.TranscribeStreaming(ctx, x, 700)
		if err != nil {
			t.Fatalf("TranscribeStreaming utterance %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("utterance %d: streaming in 700-sample chunks decoded %q, the whole file %q", i, got, want)
		}
	}

	x := f.sample
	want, err := f.recognizer.Transcribe(ctx, x)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if want == "" {
		t.Fatal("the fixture recording decodes to an empty string")
	}
	for _, chunk := range []int{1, len(x) + 1} {
		got, err := f.recognizer.TranscribeStreaming(ctx, x, chunk)
		if err != nil {
			t.Fatalf("TranscribeStreaming in %d-sample chunks: %v", chunk, err)
		}
		if got != want {
			t.Fatalf("%d-sample chunks decoded %q, the whole file %q", chunk, got, want)
		}
	}
	t.Logf("%d held-out utterances agree in 700-sample chunks; %d samples decode to %q whole, in 1-sample chunks and in one chunk, with %q after the first %d samples",
		len(f.test), len(x), want, f.prefix, f.half)
	if _, err := f.recognizer.TranscribeStreaming(ctx, x, 0); err == nil {
		t.Fatal("TranscribeStreaming accepted a chunk of no samples")
	}
}

// TestStreamUsesOnlyArrivedAudio marks the audio that has not arrived yet with
// NaN: a decoder that peeked at it would either change what it already emitted
// or read the marker without complaining.
func TestStreamUsesOnlyArrivedAudio(t *testing.T) {
	ctx := context.Background()
	f := streamTestFixture(t)
	x, half := f.sample, f.half

	a, err := f.recognizer.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	first, err := a.Feed(ctx, x[:half])
	if err != nil {
		t.Fatalf("Feed the first half: %v", err)
	}
	if first == "" {
		t.Fatal("the first half of the fixture recording emitted nothing")
	}
	if a.Text() != first {
		t.Fatalf("Text() is %q after one Feed that returned %q", a.Text(), first)
	}

	b, err := f.recognizer.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	again, err := b.Feed(ctx, x[:half])
	if err != nil {
		t.Fatalf("Feed the first half again: %v", err)
	}
	if again != first {
		t.Fatalf("the same first half emitted %q and %q", first, again)
	}
	future := make([]float64, len(x)-half)
	for i := range future {
		future[i] = math.NaN()
	}
	if _, err := b.Feed(ctx, future); err == nil {
		t.Fatal("Feed accepted a chunk of NaN samples")
	}
	if b.Text() != first {
		t.Fatalf("the unarrived chunk changed the text from %q to %q", first, b.Text())
	}
}

// TestStreamResumesFromSnapshot cuts the stream in half, serialises it, and
// resumes from the deserialised state: the text must be the one the
// uninterrupted stream produced, character for character.
func TestStreamResumesFromSnapshot(t *testing.T) {
	ctx := context.Background()
	f := streamTestFixture(t)
	x, half := f.sample, f.half

	whole, err := f.recognizer.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if _, err := whole.Feed(ctx, x[:half]); err != nil {
		t.Fatalf("Feed the first half: %v", err)
	}
	if _, err := whole.Feed(ctx, x[half:]); err != nil {
		t.Fatalf("Feed the second half: %v", err)
	}
	if _, err := whole.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	want := whole.Text()
	if want == "" {
		t.Fatal("the uninterrupted stream emitted nothing")
	}

	cut, err := f.recognizer.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if _, err := cut.Feed(ctx, x[:half]); err != nil {
		t.Fatalf("Feed the first half: %v", err)
	}
	encoded, err := json.Marshal(cut.Snapshot())
	if err != nil {
		t.Fatalf("marshal the stream state: %v", err)
	}
	var state asr.StreamState
	if err := json.Unmarshal(encoded, &state); err != nil {
		t.Fatalf("unmarshal the stream state: %v", err)
	}
	again, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal the decoded stream state: %v", err)
	}
	if !bytes.Equal(encoded, again) {
		t.Fatal("the stream state does not round trip through JSON")
	}

	resumed, err := f.recognizer.RestoreStream(state)
	if err != nil {
		t.Fatalf("RestoreStream: %v", err)
	}
	if resumed.Text() != f.prefix {
		t.Fatalf("the resumed stream starts from %q, want %q", resumed.Text(), f.prefix)
	}
	if _, err := resumed.Feed(ctx, x[half:]); err != nil {
		t.Fatalf("Feed the second half after resuming: %v", err)
	}
	if _, err := resumed.Flush(ctx); err != nil {
		t.Fatalf("Flush after resuming: %v", err)
	}
	if resumed.Text() != want {
		t.Fatalf("the resumed stream decoded %q, the uninterrupted one %q", resumed.Text(), want)
	}

	broken := state
	broken.Individual.ConfigHash = "not-this-recognizer"
	if _, err := f.recognizer.RestoreStream(broken); err == nil {
		t.Fatal("RestoreStream accepted a snapshot of another configuration")
	}
}

// TestStreamTailAndFlush covers the end of an utterance: a tail shorter than
// one analysis window emits nothing until Flush zero-pads it, and the stream
// is done afterwards.
func TestStreamTailAndFlush(t *testing.T) {
	ctx := context.Background()
	f := streamTestFixture(t)
	front := recognizerConfig().FrontEnd

	s, err := f.recognizer.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	tail := f.sample[:front.Window-1]
	out, err := s.Feed(ctx, tail)
	if err != nil {
		t.Fatalf("Feed a tail shorter than one window: %v", err)
	}
	if out != "" {
		t.Fatalf("a tail of %d samples, shorter than the %d-sample window, emitted %q", len(tail), front.Window, out)
	}
	if s.Text() != "" {
		t.Fatalf("Text() is %q before the first complete window", s.Text())
	}
	final, err := s.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if s.Text() != final {
		t.Fatalf("Flush returned %q but Text() is %q", final, s.Text())
	}
	if _, err := s.Feed(ctx, tail); err == nil {
		t.Fatal("Feed accepted samples after Flush")
	}
	if _, err := s.Flush(ctx); err == nil {
		t.Fatal("Flush accepted a second call")
	}

	empty, err := f.recognizer.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	out, err = empty.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush an empty stream: %v", err)
	}
	if out != "" || empty.Text() != "" {
		t.Fatalf("an empty stream flushed %q and holds %q", out, empty.Text())
	}
}

// TestStreamRejectsNonFinite pins that a chunk with a non-finite sample is
// refused whole: nothing of it is analysed and nothing of it is kept.
func TestStreamRejectsNonFinite(t *testing.T) {
	ctx := context.Background()
	f := streamTestFixture(t)
	front := recognizerConfig().FrontEnd

	for _, tc := range []struct {
		name  string
		value float64
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"-Inf", math.Inf(-1)},
	} {
		s, err := f.recognizer.NewStream()
		if err != nil {
			t.Fatalf("NewStream: %v", err)
		}
		if _, err := s.Feed(ctx, f.sample[:front.Window+front.Hop]); err != nil {
			t.Fatalf("Feed: %v", err)
		}
		before := s.Snapshot()
		bad := append(append([]float64(nil), f.sample[:front.Hop]...), tc.value)
		if _, err := s.Feed(ctx, bad); err == nil {
			t.Fatalf("Feed accepted a chunk ending in %s", tc.name)
		}
		after := s.Snapshot()
		if len(after.Pending) != len(before.Pending) {
			t.Fatalf("a chunk ending in %s left %d pending samples, want %d", tc.name, len(after.Pending), len(before.Pending))
		}
		if !reflect.DeepEqual(after.Pending, before.Pending) {
			t.Fatalf("a chunk ending in %s changed the pending samples", tc.name)
		}
		if after.Emitted != before.Emitted {
			t.Fatalf("a chunk ending in %s changed the text from %q to %q", tc.name, before.Emitted, after.Emitted)
		}
	}
}
