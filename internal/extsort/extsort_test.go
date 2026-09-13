package extsort

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func record16(key, payload uint64) []byte {
	rec := make([]byte, 16)
	binary.BigEndian.PutUint64(rec[0:8], key)
	binary.BigEndian.PutUint64(rec[8:16], payload)
	return rec
}

func collectMerge(t *testing.T, ctx context.Context, s *Sorter) ([][]byte, Stats) {
	t.Helper()
	var got [][]byte
	stats, err := s.Merge(ctx, func(rec []byte) error {
		got = append(got, bytes.Clone(rec))
		return nil
	})
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	return got, stats
}

func tempEntries(t *testing.T, s *Sorter) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", s.dir, err)
	}
	return entries
}

// 1. Random records sorted across at least five runs.
func TestMergeSortsRandomRecordsAcrossManyRuns(t *testing.T) {
	const total = 2000
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 2000, TempBytes: 1 << 20, MaxRuns: 64, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	rng := rand.New(rand.NewPCG(0x5eed, 0x1234))
	want := make([][]byte, 0, total)
	for i := range total {
		rec := record16(rng.Uint64(), uint64(i))
		want = append(want, bytes.Clone(rec))
		if err := s.Add(ctx, rec); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
	}
	slices.SortFunc(want, bytes.Compare)

	got, stats := collectMerge(t, ctx, s)
	if len(got) != total {
		t.Fatalf("merged %d records, want %d", len(got), total)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("record %d = %x, want %x", i, got[i], want[i])
		}
	}
	if stats.Runs < 5 {
		t.Fatalf("Runs = %d, want >= 5", stats.Runs)
	}
	if stats.Records != total || stats.Emitted != total {
		t.Fatalf("Records/Emitted = %d/%d, want %d/%d", stats.Records, stats.Emitted, total, total)
	}
}

// 1b. Byte-identical records all survive. Equal records are indistinguishable
// to the visitor, so the observable property is the count, not their order.
func TestMergeKeepsEveryByteIdenticalRecord(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 32, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	dup := record16(42, 42)
	const dupCount = 7
	keys := []uint64{1, 0, 9, 3, 8, 2, 7}
	added := 0
	for i, key := range keys {
		for range 1 {
			if err := s.Add(ctx, dup); err != nil {
				t.Fatalf("Add(dup %d) error = %v", i, err)
			}
			added++
		}
		if err := s.Add(ctx, record16(key, key)); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
		added++
	}
	if len(keys) != dupCount {
		t.Fatalf("test setup adds %d duplicates, want %d", len(keys), dupCount)
	}

	got, stats := collectMerge(t, ctx, s)
	if len(got) != added {
		t.Fatalf("merged %d records, want %d", len(got), added)
	}
	if !slices.IsSortedFunc(got, bytes.Compare) {
		t.Fatal("merged output is not sorted")
	}
	seen := 0
	for _, rec := range got {
		if bytes.Equal(rec, dup) {
			seen++
		}
	}
	if seen != dupCount {
		t.Fatalf("identical records emitted %d times, want %d", seen, dupCount)
	}
	if stats.Emitted != int64(added) || stats.Records != int64(added) {
		t.Fatalf("Records/Emitted = %d/%d, want %d/%d", stats.Records, stats.Emitted, added, added)
	}
}

// 2. Dedupe within a run and across runs.
func TestDedupeDropsExactDuplicates(t *testing.T) {
	keys := []uint64{5, 5, 9, 1, 5, 9, 3, 1, 1, 7}
	wantUnique := [][]byte{record16(1, 1), record16(3, 3), record16(5, 5), record16(7, 7), record16(9, 9)}

	run := func(t *testing.T, dedupe bool) ([][]byte, Stats) {
		t.Helper()
		s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 16, TempDir: t.TempDir(), Dedupe: dedupe})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer s.Close()
		ctx := context.Background()
		for i, key := range keys {
			if err := s.Add(ctx, record16(key, key)); err != nil {
				t.Fatalf("Add(%d) error = %v", i, err)
			}
		}
		got, stats := collectMerge(t, ctx, s)
		if stats.Runs < 2 {
			t.Fatalf("Runs = %d, want >= 2 so duplicates span runs", stats.Runs)
		}
		return got, stats
	}

	t.Run("on", func(t *testing.T) {
		got, stats := run(t, true)
		if len(got) != len(wantUnique) {
			t.Fatalf("emitted %d records, want %d", len(got), len(wantUnique))
		}
		for i := range wantUnique {
			if !bytes.Equal(got[i], wantUnique[i]) {
				t.Fatalf("record %d = %x, want %x", i, got[i], wantUnique[i])
			}
		}
		if stats.Records != int64(len(keys)) {
			t.Fatalf("Records = %d, want %d", stats.Records, len(keys))
		}
		if stats.Emitted != int64(len(wantUnique)) {
			t.Fatalf("Emitted = %d, want %d", stats.Emitted, len(wantUnique))
		}
	})

	t.Run("off", func(t *testing.T) {
		got, stats := run(t, false)
		want := make([][]byte, 0, len(keys))
		for _, key := range keys {
			want = append(want, record16(key, key))
		}
		slices.SortFunc(want, bytes.Compare)
		if len(got) != len(want) {
			t.Fatalf("emitted %d records, want %d", len(got), len(want))
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("record %d = %x, want %x", i, got[i], want[i])
			}
		}
		if stats.Emitted != stats.Records || stats.Emitted != int64(len(keys)) {
			t.Fatalf("Records/Emitted = %d/%d, want %d/%d", stats.Records, stats.Emitted, len(keys), len(keys))
		}
	})
}

// 3. Zero records.
func TestMergeWithoutRecords(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 1 << 12, TempBytes: 1 << 20, MaxRuns: 4, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	stats, err := s.Merge(context.Background(), func(rec []byte) error {
		t.Fatalf("visit called with %x for an empty sort", rec)
		return nil
	})
	if err != nil {
		t.Fatalf("Merge() error = %v, want nil", err)
	}
	if stats != (Stats{}) {
		t.Fatalf("Stats = %+v, want zero value", stats)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// 4. Wrong record length fails the sorter.
func TestAddRejectsWrongRecordLengthAndFailsSorter(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 1 << 12, TempBytes: 1 << 20, MaxRuns: 4, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.Add(ctx, make([]byte, 15)); err == nil {
		t.Fatal("Add() accepted a 15-byte record")
	}
	if err := s.Add(ctx, record16(1, 1)); err == nil {
		t.Fatal("Add() succeeded after the sorter failed")
	}
	if _, err := s.Merge(ctx, func([]byte) error { return nil }); err == nil {
		t.Fatal("Merge() succeeded after the sorter failed")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// 5a. TempBytes limit.
func TestTempBytesLimitLeavesNoExtraRunFile(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 32, MaxRuns: 8, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	var addErr error
	for i := range 8 {
		if addErr = s.Add(ctx, record16(uint64(i), uint64(i))); addErr != nil {
			break
		}
	}
	if !errors.Is(addErr, ErrCapacity) {
		t.Fatalf("Add() error = %v, want ErrCapacity", addErr)
	}
	if entries := tempEntries(t, s); len(entries) != 1 {
		t.Fatalf("temp dir holds %d files, want 1", len(entries))
	}
}

// 5b. MaxRuns limit.
func TestMaxRunsLimitLeavesNoExtraRunFile(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 20, TempBytes: 1 << 20, MaxRuns: 2, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	var addErr error
	for i := range 8 {
		if addErr = s.Add(ctx, record16(uint64(i), uint64(i))); addErr != nil {
			break
		}
	}
	if !errors.Is(addErr, ErrCapacity) {
		t.Fatalf("Add() error = %v, want ErrCapacity", addErr)
	}
	if entries := tempEntries(t, s); len(entries) != 2 {
		t.Fatalf("temp dir holds %d files, want 2", len(entries))
	}
}

// 6. Cancellation.
func TestCanceledContextStopsAddAndMerge(t *testing.T) {
	cfg := Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 32, TempDir: t.TempDir()}

	t.Run("add", func(t *testing.T) {
		s, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer s.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := s.Add(ctx, record16(1, 1)); !errors.Is(err, context.Canceled) {
			t.Fatalf("Add() error = %v, want context.Canceled", err)
		}
	})

	t.Run("merge", func(t *testing.T) {
		s, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer s.Close()
		const total = 20
		for i := range total {
			if err := s.Add(context.Background(), record16(uint64(i), uint64(i))); err != nil {
				t.Fatalf("Add(%d) error = %v", i, err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		visited := 0
		if _, err := s.Merge(ctx, func([]byte) error {
			visited++
			cancel()
			return nil
		}); !errors.Is(err, context.Canceled) {
			t.Fatalf("Merge() error = %v, want context.Canceled", err)
		}
		if visited == 0 || visited >= total {
			t.Fatalf("visited %d records, want between 1 and %d", visited, total-1)
		}
	})
}

// 7. Visit error.
func TestVisitErrorStopsMerge(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 32, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	for i := range 10 {
		if err := s.Add(ctx, record16(uint64(i), uint64(i))); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
	}
	sentinel := errors.New("visit failed")
	visited := 0
	_, err = s.Merge(ctx, func([]byte) error {
		visited++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Merge() error = %v, want wrapped sentinel", err)
	}
	if !strings.Contains(err.Error(), "extsort") {
		t.Fatalf("Merge() error = %q, want an extsort-wrapped message", err)
	}
	if visited != 1 {
		t.Fatalf("visited %d records, want 1", visited)
	}
}

// 8. Truncated run file.
func TestMergeRejectsTruncatedRunFile(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 32, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	for i := range 6 {
		if err := s.Add(ctx, record16(uint64(i), uint64(i))); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
	}
	entries := tempEntries(t, s)
	if len(entries) == 0 {
		t.Fatal("no run file was written")
	}
	path := filepath.Join(s.dir, entries[0].Name())
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if err := os.Truncate(path, info.Size()-1); err != nil {
		t.Fatalf("Truncate(%q) error = %v", path, err)
	}
	_, err = s.Merge(ctx, func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("Merge() error = %v, want a truncated-run error", err)
	}
}

// 9. Merge twice, Close idempotent and cleaning.
func TestMergeTwiceFailsAndCloseIsIdempotent(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 32, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	dir := s.dir
	ctx := context.Background()
	for i := range 5 {
		if err := s.Add(ctx, record16(uint64(i), uint64(i))); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
	}
	_, _ = collectMerge(t, ctx, s)
	if _, err := s.Merge(ctx, func([]byte) error { return nil }); err == nil {
		t.Fatal("second Merge() succeeded, want an error")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want not-exist", dir, err)
	}
}

// 10. Stats match what is on disk.
func TestStatsMatchRunFilesOnDisk(t *testing.T) {
	s, err := New(Config{RecordBytes: 16, MemoryBytes: 40, TempBytes: 1 << 20, MaxRuns: 32, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	const total = 11
	for i := range total {
		if err := s.Add(ctx, record16(uint64(total-i), uint64(i))); err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
	}
	_, stats := collectMerge(t, ctx, s)

	entries := tempEntries(t, s)
	var sum int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("Info(%q) error = %v", entry.Name(), err)
		}
		sum += info.Size()
	}
	if stats.Runs != int64(len(entries)) {
		t.Fatalf("Runs = %d, want %d files on disk", stats.Runs, len(entries))
	}
	if stats.TempBytes != sum {
		t.Fatalf("TempBytes = %d, want %d", stats.TempBytes, sum)
	}
	if stats.PeakMemoryBytes <= 0 {
		t.Fatalf("PeakMemoryBytes = %d, want > 0", stats.PeakMemoryBytes)
	}
	if stats.Records != total || stats.Emitted != total {
		t.Fatalf("Records/Emitted = %d/%d, want %d/%d", stats.Records, stats.Emitted, total, total)
	}
}

// 11. Config validation.
func TestNewRejectsInvalidConfig(t *testing.T) {
	valid := Config{RecordBytes: 16, MemoryBytes: 1 << 12, TempBytes: 1 << 20, MaxRuns: 4}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"zero record bytes", func(c *Config) { c.RecordBytes = 0 }},
		{"negative record bytes", func(c *Config) { c.RecordBytes = -1 }},
		{"oversized record bytes", func(c *Config) { c.RecordBytes = 65537 }},
		{"zero memory bytes", func(c *Config) { c.MemoryBytes = 0 }},
		{"negative memory bytes", func(c *Config) { c.MemoryBytes = -1 }},
		{"memory below one record", func(c *Config) { c.MemoryBytes = 19 }},
		{"zero temp bytes", func(c *Config) { c.TempBytes = 0 }},
		{"negative temp bytes", func(c *Config) { c.TempBytes = -1 }},
		{"zero max runs", func(c *Config) { c.MaxRuns = 0 }},
		{"negative max runs", func(c *Config) { c.MaxRuns = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			cfg.TempDir = t.TempDir()
			tc.mutate(&cfg)
			s, err := New(cfg)
			if err == nil {
				s.Close()
				t.Fatalf("New(%+v) accepted an invalid config", cfg)
			}
			if s != nil {
				t.Fatalf("New() returned a sorter alongside error %v", err)
			}
			entries, readErr := os.ReadDir(cfg.TempDir)
			if readErr != nil {
				t.Fatalf("ReadDir error = %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("New() left %d entries in TempDir after rejecting the config", len(entries))
			}
		})
	}

	s, err := New(Config{RecordBytes: 65536, MemoryBytes: 65536 + 4, TempBytes: 1 << 20, MaxRuns: 1, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() rejected a config that fits exactly one record: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
