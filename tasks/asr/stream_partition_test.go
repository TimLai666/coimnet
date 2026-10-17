package asr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr"
)

type streamPartitionCase struct {
	name string
	kind string
	seed uint64
}

func (c streamPartitionCase) sizes(n, window, hop int) []int {
	if n == 0 {
		return nil
	}
	pattern := []int{1}
	var rng *rand.Rand
	switch c.kind {
	case "one":
	case "cycle":
		pattern = []int{window - 1, 1, hop - 1, 1, hop + 1}
	case "random":
		rng = rand.New(rand.NewPCG(c.seed, 137))
	default:
		panic("unknown stream partition kind")
	}
	sizes := make([]int, 0, n/(hop-1)+2)
	for i, remaining := 0, n; remaining > 0; i++ {
		size := pattern[i%len(pattern)]
		if rng != nil {
			size = 1 + rng.IntN(window+hop+1)
		}
		if size > remaining {
			size = remaining
		}
		sizes = append(sizes, size)
		remaining -= size
	}
	return sizes
}

func partitionSnapshotBytes(t *testing.T, s *asr.Stream) []byte {
	t.Helper()
	encoded, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatalf("marshal stream snapshot: %v", err)
	}
	return encoded
}

func partitionRecognizerSnapshot(t *testing.T, r *asr.Recognizer) []byte {
	t.Helper()
	encoded, err := json.Marshal(r.Snapshot())
	if err != nil {
		t.Fatalf("marshal recognizer snapshot: %v", err)
	}
	return encoded
}

func newPartitionStream(t *testing.T, r *asr.Recognizer) *asr.Stream {
	t.Helper()
	s, err := r.NewStream()
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	return s
}

func assertEmptyFeedPreservesSnapshot(t *testing.T, ctx context.Context, s *asr.Stream, label string) {
	t.Helper()
	before := partitionSnapshotBytes(t, s)
	delta, err := s.Feed(ctx, nil)
	if err != nil {
		t.Fatalf("%s empty Feed: %v", label, err)
	}
	if delta != "" {
		t.Fatalf("%s empty Feed returned %q", label, delta)
	}
	after := partitionSnapshotBytes(t, s)
	if !bytes.Equal(before, after) {
		t.Fatalf("%s empty Feed changed the complete stream snapshot", label)
	}
}

func appendAndCheckText(t *testing.T, s *asr.Stream, emitted *strings.Builder, delta, operation string) {
	t.Helper()
	emitted.WriteString(delta)
	if got, want := emitted.String(), s.Text(); got != want {
		t.Fatalf("%s returned deltas concatenating to %q, Text() is %q", operation, got, want)
	}
}

func singlePartitionSizes(n int) []int {
	if n == 0 {
		return nil
	}
	return []int{n}
}

func referencePartitionStates(t *testing.T, ctx context.Context, r *asr.Recognizer, x []float64) ([]byte, []byte, string) {
	t.Helper()
	s := newPartitionStream(t, r)
	assertEmptyFeedPreservesSnapshot(t, ctx, s, "initial")
	s, emitted := runPartition(t, ctx, r, s, x, singlePartitionSizes(len(x)), false)
	assertEmptyFeedPreservesSnapshot(t, ctx, s, "before Flush")
	beforeFlush := partitionSnapshotBytes(t, s)
	delta, err := s.Flush(ctx)
	if err != nil {
		t.Fatalf("single Flush of %d samples: %v", len(x), err)
	}
	appendAndCheckText(t, s, emitted, delta, "single Flush")
	afterFlush := partitionSnapshotBytes(t, s)
	return beforeFlush, afterFlush, s.Text()
}

func runPartition(t *testing.T, ctx context.Context, r *asr.Recognizer, s *asr.Stream, x []float64, sizes []int, restore bool) (*asr.Stream, *strings.Builder) {
	t.Helper()
	var emitted strings.Builder
	offset := 0
	for i, size := range sizes {
		if size < 1 {
			t.Fatalf("partition chunk %d has invalid size %d", i, size)
		}
		if size > len(x)-offset {
			size = len(x) - offset
		}
		if size == 0 {
			t.Fatalf("partition chunk %d is empty", i)
		}
		delta, err := s.Feed(ctx, x[offset:offset+size])
		if err != nil {
			t.Fatalf("Feed chunk %d of %d samples: %v", i, size, err)
		}
		appendAndCheckText(t, s, &emitted, delta, fmt.Sprintf("Feed chunk %d", i))
		if restore {
			s = restorePartitionStream(t, r, s)
			if emitted.String() != s.Text() {
				t.Fatalf("restored stream Text() is %q, deltas concatenate to %q", s.Text(), emitted.String())
			}
		}
		offset += size
	}
	if offset != len(x) {
		t.Fatalf("partition consumed %d of %d samples", offset, len(x))
	}
	return s, &emitted
}

func flushAndComparePartition(t *testing.T, ctx context.Context, s *asr.Stream, emitted *strings.Builder, wantBefore, wantAfter []byte) {
	t.Helper()
	assertEmptyFeedPreservesSnapshot(t, ctx, s, "before Flush")
	before := partitionSnapshotBytes(t, s)
	if !bytes.Equal(wantBefore, before) {
		t.Fatal("partitioned Feed state differs from the single-Feed state")
	}
	delta, err := s.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	appendAndCheckText(t, s, emitted, delta, "Flush")
	after := partitionSnapshotBytes(t, s)
	if !bytes.Equal(wantAfter, after) {
		t.Fatal("partitioned Flush state differs from the single-Feed state")
	}
}

func restorePartitionStream(t *testing.T, r *asr.Recognizer, s *asr.Stream) *asr.Stream {
	t.Helper()
	encoded := partitionSnapshotBytes(t, s)
	before := encoded
	var state asr.StreamState
	if err := json.Unmarshal(encoded, &state); err != nil {
		t.Fatalf("unmarshal stream state for restore: %v", err)
	}
	decoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("remarshal stream state for restore: %v", err)
	}
	if !bytes.Equal(encoded, decoded) {
		t.Fatal("stream state changed during JSON round trip")
	}
	resumed, err := r.RestoreStream(state)
	if err != nil {
		t.Fatalf("RestoreStream: %v", err)
	}
	after := partitionSnapshotBytes(t, resumed)
	if !bytes.Equal(before, after) {
		t.Fatal("RestoreStream changed the complete stream snapshot")
	}
	return resumed
}

func assertPartitionFixtureUnchanged(t *testing.T, f streamFixtureState, sample []float64, recognizerBefore []byte) {
	t.Helper()
	if !reflect.DeepEqual(f.sample, sample) {
		t.Fatal("partition tests changed the shared fixture sample")
	}
	recognizerAfter := partitionRecognizerSnapshot(t, f.recognizer)
	if !bytes.Equal(recognizerBefore, recognizerAfter) {
		t.Fatal("partition tests changed the shared recognizer snapshot")
	}
}

func runStreamPartitionMatrix(t *testing.T, ctx context.Context, f streamFixtureState, sample []float64, lengths []int, cases []streamPartitionCase, restore bool) {
	t.Helper()
	front := recognizerConfig().FrontEnd
	for _, length := range lengths {
		length := length
		x := append([]float64(nil), sample[:length]...)
		beforeFlush, afterFlush, wantText := referencePartitionStates(t, ctx, f.recognizer, x)
		if length == len(sample) && wantText == "" {
			t.Fatal("single-Feed full-recording control emitted empty text")
		}
		for _, tc := range cases {
			tc := tc
			t.Run(fmt.Sprintf("length-%d/%s", length, tc.name), func(t *testing.T) {
				s := newPartitionStream(t, f.recognizer)
				assertEmptyFeedPreservesSnapshot(t, ctx, s, "initial")
				s, emitted := runPartition(t, ctx, f.recognizer, s, x, tc.sizes(len(x), front.Window, front.Hop), restore)
				flushAndComparePartition(t, ctx, s, emitted, beforeFlush, afterFlush)
				if restore {
					after := partitionSnapshotBytes(t, s)
					s = restorePartitionStream(t, f.recognizer, s)
					if !bytes.Equal(after, partitionSnapshotBytes(t, s)) {
						t.Fatal("restoring a flushed stream changed its complete snapshot")
					}
					if emitted.String() != s.Text() {
						t.Fatalf("restored flushed Text() is %q, deltas concatenate to %q", s.Text(), emitted.String())
					}
				}
			})
		}
	}
}

func runStreamPartitionTest(t *testing.T, restore bool) {
	ctx := context.Background()
	f := streamTestFixture(t)
	sample := append([]float64(nil), f.sample...)
	recognizerBefore := partitionRecognizerSnapshot(t, f.recognizer)
	defer assertPartitionFixtureUnchanged(t, f, sample, recognizerBefore)
	front := recognizerConfig().FrontEnd
	lengths := []int{front.Window - 1, len(sample) - 1, len(sample)}
	cases := []streamPartitionCase{{name: "cycle", kind: "cycle"}, {name: "pcg-17", kind: "random", seed: 17}, {name: "pcg-29", kind: "random", seed: 29}, {name: "pcg-43", kind: "random", seed: 43}}
	if !restore {
		lengths = []int{0, 1, front.Window - 1, front.Window, front.Window + front.Hop - 1, front.Window + front.Hop, len(sample) - 1, len(sample)}
		cases = append([]streamPartitionCase{{name: "one-sample", kind: "one"}}, cases...)
	}
	runStreamPartitionMatrix(t, ctx, f, sample, lengths, cases, restore)
}

func TestStreamPartitionFullState(t *testing.T) {
	runStreamPartitionTest(t, false)
}

func TestStreamPartitionRestoreFullState(t *testing.T) {
	runStreamPartitionTest(t, true)
}
