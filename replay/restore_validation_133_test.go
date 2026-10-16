package replay_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/replay"
)

func ticket133Buffer(sampling, eviction, retention string, raw bool) replay.Buffer {
	return replay.Buffer{
		Capacity: 2, Sampling: sampling, Eviction: eviction, Seed: 37,
		Privacy: replay.PrivacyPolicy{StoreRaw: raw, Retention: retention},
	}
}

func ticket133Experience(task string, step uint64, value float64) replay.Experience {
	return replay.Experience{
		TaskID: task, Step: step,
		Input: [][]float64{{value}, {value + 0.5}}, Target: [][]float64{{value * 2}},
		Weight: 1, Split: replay.SplitTrain,
	}
}

func ticket133New(t *testing.T, b replay.Buffer) *replay.Store {
	t.Helper()
	store, err := replay.New(b)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func ticket133Add(t *testing.T, store *replay.Store, items ...replay.Experience) {
	t.Helper()
	for _, item := range items {
		if err := store.Add(item); err != nil {
			t.Fatal(err)
		}
	}
}

func ticket133SnapshotCopy(snapshot replay.Snapshot) replay.Snapshot {
	copyOf := snapshot
	if snapshot.State.Items == nil {
		copyOf.State.Items = nil
		return copyOf
	}
	copyOf.State.Items = make([]replay.Experience, len(snapshot.State.Items))
	for i, item := range snapshot.State.Items {
		copyOf.State.Items[i] = item
		if item.Input != nil {
			copyOf.State.Items[i].Input = make([][]float64, len(item.Input))
			for j, row := range item.Input {
				copyOf.State.Items[i].Input[j] = append([]float64(nil), row...)
			}
		}
		if item.Target != nil {
			copyOf.State.Items[i].Target = make([][]float64, len(item.Target))
			for j, row := range item.Target {
				copyOf.State.Items[i].Target[j] = append([]float64(nil), row...)
			}
		}
	}
	return copyOf
}

func ticket133SetHashChar(value string, index int, character byte) string {
	chars := []byte(value)
	chars[index] = character
	return string(chars)
}

func ticket133RejectUnchanged(t *testing.T, candidate replay.Snapshot) {
	t.Helper()
	before := ticket133SnapshotCopy(candidate)
	store, err := replay.RestoreSnapshot(candidate)
	if err == nil {
		t.Fatal("Restore accepted an invalid replay state")
	}
	if store != nil {
		t.Fatal("Restore returned a store for an invalid replay state")
	}
	if !reflect.DeepEqual(candidate, before) {
		t.Fatal("Restore changed the rejected snapshot")
	}
}

func TestReplayRestoreRejectsNonHexFingerprints(t *testing.T) {
	b := ticket133Buffer(replay.SamplingUniform, replay.EvictionFIFO, replay.RetentionKeepUntilEvicted, false)
	store := ticket133New(t, b)
	ticket133Add(t, store, ticket133Experience("hash", 1, 1))
	base := store.Snapshot()

	cases := []struct {
		name string
		edit func(*replay.Experience)
	}{
		{"input first character", func(item *replay.Experience) {
			item.InputHash = ticket133SetHashChar(item.InputHash, 0, 'g')
		}},
		{"input last character", func(item *replay.Experience) {
			item.InputHash = ticket133SetHashChar(item.InputHash, len(item.InputHash)-1, 'z')
		}},
		{"target first character", func(item *replay.Experience) {
			item.TargetHash = ticket133SetHashChar(item.TargetHash, 0, 'g')
		}},
		{"target last character", func(item *replay.Experience) {
			item.TargetHash = ticket133SetHashChar(item.TargetHash, len(item.TargetHash)-1, 'z')
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := ticket133SnapshotCopy(base)
			tc.edit(&candidate.State.Items[0])
			ticket133RejectUnchanged(t, candidate)
		})
	}
}

func ticket133ReservoirSnapshot(t *testing.T, added int) replay.Snapshot {
	t.Helper()
	b := ticket133Buffer(replay.SamplingUniform, replay.EvictionReservoir, replay.RetentionKeepUntilEvicted, true)
	store := ticket133New(t, b)
	for step := 1; step <= added; step++ {
		ticket133Add(t, store, ticket133Experience("reservoir", uint64(step), float64(step)))
	}
	snapshot := store.Snapshot()
	if len(snapshot.State.Items) != 2 {
		t.Fatalf("fixture holds %d items, want 2", len(snapshot.State.Items))
	}
	return snapshot
}

func TestReplayRestoreRejectsImpossibleReservoirDraws(t *testing.T) {
	cases := []struct {
		name        string
		added       int
		sourceDraws uint64
		wantSeen    uint64
		wantDraws   uint64
		corrupt     func(*replay.State)
	}{
		{
			name: "seen 3 with zero draws", added: 3, sourceDraws: 1, wantSeen: 3, wantDraws: 0,
			corrupt: func(state *replay.State) { state.Draws = 0 },
		},
		{
			name: "seen 6 with only three draws", added: 6, sourceDraws: 4, wantSeen: 6, wantDraws: 3,
			corrupt: func(state *replay.State) { state.Draws = 3 },
		},
		{
			name: "maximum seen without narrowing", added: 2, sourceDraws: 0, wantSeen: math.MaxUint64, wantDraws: 0,
			corrupt: func(state *replay.State) { state.Seen = math.MaxUint64 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := ticket133ReservoirSnapshot(t, tc.added)
			if candidate.State.Seen != uint64(tc.added) || candidate.State.Draws != tc.sourceDraws {
				t.Fatalf("source fixture state = seen %d, draws %d", candidate.State.Seen, candidate.State.Draws)
			}
			tc.corrupt(&candidate.State)
			if candidate.State.Seen != tc.wantSeen || candidate.State.Draws != tc.wantDraws {
				t.Fatalf("damaged state = seen %d, draws %d", candidate.State.Seen, candidate.State.Draws)
			}
			ticket133RejectUnchanged(t, candidate)
		})
	}
}

func ticket133InitialItems() []replay.Experience {
	return []replay.Experience{
		ticket133Experience("a", 10, 1), ticket133Experience("b", 20, 2),
		ticket133Experience("a", 30, 3), ticket133Experience("b", 40, 4),
	}
}

func ticket133ContinuationItems() []replay.Experience {
	return []replay.Experience{
		ticket133Experience("c", 50, 5), ticket133Experience("a", 60, 6),
		ticket133Experience("b", 70, 7),
	}
}

func ticket133CompareSample(t *testing.T, left, right *replay.Store, n int) {
	t.Helper()
	want, wantReport, wantErr := left.Sample(n)
	got, gotReport, gotErr := right.Sample(n)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("valid continuation sample returned an error: want %v, got %v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sample differs: got %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(gotReport, wantReport) {
		t.Fatalf("sample report differs: got %+v, want %+v", gotReport, wantReport)
	}
}

func TestReplayRestoreContinuation(t *testing.T) {
	for _, sampling := range []string{replay.SamplingUniform, replay.SamplingTaskBalanced, replay.SamplingTimeBalanced} {
		for _, eviction := range []string{replay.EvictionFIFO, replay.EvictionReservoir} {
			for _, retention := range []string{replay.RetentionKeepUntilEvicted, replay.RetentionDropAfterSample} {
				for _, raw := range []bool{true, false} {
					name := sampling + "/" + eviction + "/" + retention + "/"
					if raw {
						name += "raw"
					} else {
						name += "hashed"
					}
					t.Run(name, func(t *testing.T) {
						b := ticket133Buffer(sampling, eviction, retention, raw)
						straight := ticket133New(t, b)
						if sampling == replay.SamplingUniform && eviction == replay.EvictionFIFO && retention == replay.RetentionKeepUntilEvicted && raw {
							empty, err := replay.RestoreSnapshot(straight.Snapshot())
							if err != nil || !reflect.DeepEqual(empty.Snapshot(), straight.Snapshot()) || !reflect.DeepEqual(empty.Declaration(), b) {
								t.Fatalf("restoring the initial empty snapshot failed: %v", err)
							}
						}
						ticket133Add(t, straight, ticket133InitialItems()...)
						if retention == replay.RetentionDropAfterSample {
							if _, _, err := straight.Sample(9); err != nil {
								t.Fatal(err)
							}
						} else {
							if _, _, err := straight.Sample(1); err != nil {
								t.Fatal(err)
							}
						}

						mid := straight.Snapshot()
						continued, err := replay.RestoreSnapshot(ticket133SnapshotCopy(mid))
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(continued.Snapshot(), mid) || !reflect.DeepEqual(continued.Declaration(), straight.Declaration()) || !reflect.DeepEqual(continued.Report(), straight.Report()) {
							t.Fatal("restore changed the continuation state or declared policies")
						}

						ticket133Add(t, straight, ticket133ContinuationItems()...)
						ticket133Add(t, continued, ticket133ContinuationItems()...)
						for _, n := range []int{1, 3} {
							ticket133CompareSample(t, straight, continued, n)
						}
						if !reflect.DeepEqual(continued.State(), straight.State()) || !reflect.DeepEqual(continued.Report(), straight.Report()) {
							t.Fatal("restored continuation ended with a different state or report")
						}
					})
				}
			}
		}
	}
}

func TestReplayRestoreDropAndHexControls(t *testing.T) {
	t.Run("reservoir drop naturally empty", func(t *testing.T) {
		b := ticket133Buffer(replay.SamplingUniform, replay.EvictionReservoir, replay.RetentionDropAfterSample, true)
		straight := ticket133New(t, b)
		for step := uint64(1); step <= 3; step++ {
			ticket133Add(t, straight, ticket133Experience("drop", step, float64(step)))
			items, report, err := straight.Sample(1)
			if err != nil || len(items) != 1 || report.Returned != 1 {
				t.Fatalf("Add-one/Sample-one cycle failed: items=%d report=%+v err=%v", len(items), report, err)
			}
		}
		snapshot := straight.Snapshot()
		if snapshot.State.Seen != 3 || len(snapshot.State.Items) != 0 || snapshot.State.Draws != 0 {
			t.Fatalf("naturally empty drop state = %+v", snapshot.State)
		}
		resumed, err := replay.RestoreSnapshot(ticket133SnapshotCopy(snapshot))
		if err != nil {
			t.Fatalf("valid naturally empty drop state was refused: %v", err)
		}
		if !reflect.DeepEqual(resumed.Snapshot(), snapshot) {
			t.Fatal("restoring the naturally empty drop state changed it")
		}
		ticket133Add(t, straight, ticket133Experience("drop", 4, 4))
		ticket133Add(t, resumed, ticket133Experience("drop", 4, 4))
		ticket133CompareSample(t, straight, resumed, 1)
		if !reflect.DeepEqual(resumed.State(), straight.State()) {
			t.Fatal("drop restore did not continue with the same state")
		}
	})

	t.Run("uppercase hashed fingerprints", func(t *testing.T) {
		b := ticket133Buffer(replay.SamplingUniform, replay.EvictionFIFO, replay.RetentionKeepUntilEvicted, false)
		store := ticket133New(t, b)
		ticket133Add(t, store, ticket133Experience("hex", 1, 1))
		candidate := ticket133SnapshotCopy(store.Snapshot())
		candidate.State.Items[0].InputHash = strings.ToUpper(candidate.State.Items[0].InputHash)
		candidate.State.Items[0].TargetHash = strings.ToUpper(candidate.State.Items[0].TargetHash)
		restored, err := replay.RestoreSnapshot(candidate)
		if err != nil {
			t.Fatalf("valid uppercase fingerprints were refused: %v", err)
		}
		if !reflect.DeepEqual(restored.Snapshot(), candidate) {
			t.Fatal("Restore did not preserve uppercase fingerprints")
		}
		items, _, err := restored.Sample(1)
		if err != nil || len(items) != 1 || items[0].InputHash != candidate.State.Items[0].InputHash || items[0].TargetHash != candidate.State.Items[0].TargetHash {
			t.Fatalf("uppercase fingerprints were not preserved in the public sample: items=%+v err=%v", items, err)
		}
	})
}
