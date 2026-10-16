package replay_test

import (
	"context"
	"math"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/replay"
)

func ticket133CounterBuffer(sampling, eviction, retention string) replay.Buffer {
	return replay.Buffer{
		Capacity: 2,
		Sampling: sampling,
		Eviction: eviction,
		Seed:     37,
		Privacy: replay.PrivacyPolicy{
			StoreRaw:  true,
			Retention: retention,
		},
	}
}

func ticket133CounterAdd(t *testing.T, store *replay.Store, task string, first, last int) {
	t.Helper()
	for step := first; step <= last; step++ {
		ticket133Add(t, store, ticket133Experience(task, uint64(step), float64(step)))
	}
}

func TestReplayRestoreRejectsDrawsWithoutPriorRandomDraw(t *testing.T) {
	cases := []struct {
		name     string
		buffer   replay.Buffer
		addCount int
		add      func(*testing.T, *replay.Store)
		check    func(*testing.T, replay.State)
	}{
		{
			name:     "empty fifo",
			buffer:   ticket133CounterBuffer(replay.SamplingUniform, replay.EvictionFIFO, replay.RetentionKeepUntilEvicted),
			addCount: 0,
		},
		{
			name:     "one-seen fifo",
			buffer:   ticket133CounterBuffer(replay.SamplingUniform, replay.EvictionFIFO, replay.RetentionKeepUntilEvicted),
			addCount: 1,
		},
		{
			name: "fifo capacity one seen three",
			buffer: replay.Buffer{
				Capacity: 1, Sampling: replay.SamplingUniform, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			addCount: 3,
		},
		{
			name: "fifo time-balanced capacity two seen three",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			addCount: 3,
		},
		{
			name: "reservoir time-balanced capacity four seen two",
			buffer: replay.Buffer{
				Capacity: 4, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionReservoir, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			addCount: 2,
		},
		{
			name: "reservoir uniform capacity one seen one",
			buffer: replay.Buffer{
				Capacity: 1, Sampling: replay.SamplingUniform, Eviction: replay.EvictionReservoir, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			addCount: 1,
		},
		{
			name: "drop fifo uniform capacity one seen three",
			buffer: replay.Buffer{
				Capacity: 1, Sampling: replay.SamplingUniform, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionDropAfterSample},
			},
			addCount: 3,
		},
		{
			name: "drop fifo time-balanced capacity two seen three",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionDropAfterSample},
			},
			addCount: 3,
		},
		{
			name: "keep task-balanced distinct tasks seen two held two",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingTaskBalanced, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			addCount: 2,
			add: func(t *testing.T, store *replay.Store) {
				ticket133Add(t, store,
					ticket133Experience("a", 1, 1), ticket133Experience("b", 2, 2))
			},
			check: func(t *testing.T, state replay.State) {
				if len(state.Items) != 2 {
					t.Fatalf("publicly built fixture holds %d items, want 2", len(state.Items))
				}
				if state.Items[0].TaskID == state.Items[1].TaskID {
					t.Fatalf("publicly built fixture tasks are not distinct: %q", state.Items[0].TaskID)
				}
			},
		},
		{
			name: "drop uniform seen two held two",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingUniform, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionDropAfterSample},
			},
			addCount: 2,
			add: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "drop", 1, 2)
			},
			check: func(t *testing.T, state replay.State) {
				if len(state.Items) != 2 {
					t.Fatalf("publicly built fixture holds %d items, want 2", len(state.Items))
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := ticket133New(t, tc.buffer)
			if _, _, err := store.Sample(1); err != nil {
				t.Fatalf("public zero-item Sample failed: %v", err)
			}
			if tc.add != nil {
				tc.add(t, store)
			} else {
				ticket133CounterAdd(t, store, "counter", 1, tc.addCount)
			}
			source := store.Snapshot()
			if source.State.Draws != 0 {
				t.Fatalf("publicly built zero-draw fixture has Draws=%d", source.State.Draws)
			}
			if source.State.Seen != uint64(tc.addCount) {
				t.Fatalf("publicly built fixture has Seen=%d, want %d", source.State.Seen, tc.addCount)
			}
			if tc.check != nil {
				tc.check(t, source.State)
			}

			candidate := ticket133SnapshotCopy(source)
			candidate.State.Draws = 1
			ticket133RejectUnchanged(t, candidate)
			if !reflect.DeepEqual(store.Snapshot(), source) {
				t.Fatal("rejecting an invalid counter changed the source store")
			}
		})
	}
}

type ticket133ContinuationScenario struct {
	name       string
	buffer     replay.Buffer
	setup      func(*testing.T, *replay.Store)
	checkState func(*testing.T, replay.State)
	resume     func(*testing.T, *replay.Store, *replay.Store)
}

func TestReplayRestoreContinuesFromValidCounterStates(t *testing.T) {
	cases := []ticket133ContinuationScenario{
		{
			name: "reservoir uniform capacity one overflow",
			buffer: replay.Buffer{
				Capacity: 1, Sampling: replay.SamplingUniform, Eviction: replay.EvictionReservoir, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "reservoir", 1, 2)
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 2 || len(state.Items) != 1 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 2, 1, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("reservoir", 3, 3)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 1)
			},
		},
		{
			name: "reservoir time-balanced capacity two overflow",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionReservoir, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "reservoir", 1, 3)
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 3 || len(state.Items) != 2 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 3, 2, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("reservoir", 4, 4)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 2)
			},
		},
		{
			name: "reservoir time-balanced capacity one overflow",
			buffer: replay.Buffer{
				Capacity: 1, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionReservoir, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "reservoir", 1, 2)
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 2 || len(state.Items) != 1 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 2, 1, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("reservoir", 3, 3)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 1)
			},
		},
		{
			name: "fifo uniform capacity two with two held",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingUniform, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "fifo", 1, 2)
				if _, _, err := store.Sample(1); err != nil {
					t.Fatal(err)
				}
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 2 || len(state.Items) != 2 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 2, 2, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("fifo", 3, 3)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 2)
			},
		},
		{
			name: "fifo task-balanced with two same-task held",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingTaskBalanced, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "same-task", 1, 2)
				if _, _, err := store.Sample(1); err != nil {
					t.Fatal(err)
				}
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 2 || len(state.Items) != 2 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 2, 2, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("same-task", 3, 3)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 2)
			},
		},
		{
			name: "fifo time-balanced with three held",
			buffer: replay.Buffer{
				Capacity: 3, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "fifo", 1, 3)
				if _, _, err := store.Sample(1); err != nil {
					t.Fatal(err)
				}
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 3 || len(state.Items) != 3 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 3, 3, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("fifo", 4, 4)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 2)
			},
		},
		{
			name: "drop reservoir time-balanced capacity two",
			buffer: replay.Buffer{
				Capacity: 2, Sampling: replay.SamplingTimeBalanced, Eviction: replay.EvictionReservoir, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionDropAfterSample},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "drop", 1, 2)
				if _, _, err := store.Sample(1); err != nil {
					t.Fatal(err)
				}
				ticket133CounterAdd(t, store, "drop", 3, 3)
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 3 || len(state.Items) != 2 || state.Draws != 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 3, 2, and 0", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("drop", 4, 4)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 1)
			},
		},
		{
			name: "drop fifo uniform capacity three after positive sample draws",
			buffer: replay.Buffer{
				Capacity: 3, Sampling: replay.SamplingUniform, Eviction: replay.EvictionFIFO, Seed: 37,
				Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionDropAfterSample},
			},
			setup: func(t *testing.T, store *replay.Store) {
				ticket133CounterAdd(t, store, "drop", 1, 3)
				if _, _, err := store.Sample(2); err != nil {
					t.Fatal(err)
				}
			},
			checkState: func(t *testing.T, state replay.State) {
				if state.Seen != 3 || len(state.Items) != 1 || state.Draws == 0 {
					t.Fatalf("state = seen %d, held %d, draws %d; want 3, 1, and a positive draw count", state.Seen, len(state.Items), state.Draws)
				}
			},
			resume: func(t *testing.T, straight, restored *replay.Store) {
				item := ticket133Experience("drop", 4, 4)
				ticket133Add(t, straight, item)
				ticket133Add(t, restored, item)
				ticket133CompareSample(t, straight, restored, 1)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			straight := ticket133New(t, tc.buffer)
			tc.setup(t, straight)
			source := straight.Snapshot()
			tc.checkState(t, source.State)

			restored, err := replay.RestoreSnapshot(ticket133SnapshotCopy(source))
			if err != nil {
				t.Fatalf("valid source state was refused: %v", err)
			}
			if !reflect.DeepEqual(restored.Snapshot(), source) {
				t.Fatal("restoring a valid source state changed it")
			}

			tc.resume(t, straight, restored)
			if !reflect.DeepEqual(restored.Snapshot(), straight.Snapshot()) {
				t.Fatal("restored continuation diverged from the uninterrupted store")
			}
		})
	}
}

const ticket133MaxDrawsChildEnv = "COIMNET_REPLAY_RESTORE_MAX_UINT64_CHILD"

func TestReplayRestoreRejectsMaxUint64DrawsWithinDeadline(t *testing.T) {
	if os.Getenv(ticket133MaxDrawsChildEnv) == "1" {
		store := ticket133New(t, replay.Buffer{
			Capacity: 2, Sampling: replay.SamplingUniform, Eviction: replay.EvictionFIFO, Seed: 37,
			Privacy: replay.PrivacyPolicy{StoreRaw: true, Retention: replay.RetentionKeepUntilEvicted},
		})
		source := store.Snapshot()
		if source.State.Draws != 0 || source.State.Seen != 0 || len(source.State.Items) != 0 {
			t.Fatalf("empty public fixture = %+v", source.State)
		}
		candidate := ticket133SnapshotCopy(source)
		candidate.State.Draws = math.MaxUint64
		ticket133RejectUnchanged(t, candidate)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReplayRestoreRejectsMaxUint64DrawsWithinDeadline$", "-test.v")
	cmd.Env = append(os.Environ(), ticket133MaxDrawsChildEnv+"=1")
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("RestoreSnapshot did not reject MaxUint64 Draws before the five-second child deadline")
	}
	if err != nil {
		t.Fatalf("child MaxUint64 RestoreSnapshot check failed: %v", err)
	}
}
