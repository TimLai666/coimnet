// Package extsort sorts fixed-width byte records with bounded memory by
// spilling sorted runs to private temporary files and merging them.
//
// Records are ordered by bytes.Compare over the whole record, so callers that
// encode numeric keys big-endian get numeric order for free. A Sorter is not
// safe for concurrent use.
//
// Config.MemoryBytes bounds the in-memory run buffer and its sort index only.
// The bufio buffers used while writing a run and while reading each run during
// Merge are outside that bound: one writer buffer during Add, and one reader
// buffer per open run file during Merge.
package extsort

import (
	"bufio"
	"bytes"
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
)

// ErrCapacity is wrapped by every error caused by exceeding a configured limit.
var ErrCapacity = errors.New("extsort: capacity limit exceeded")

const (
	// maxRecordBytes is the largest supported fixed record width.
	maxRecordBytes = 65536
	// indexBytesPerRecord is the accounted size of one sort index entry.
	indexBytesPerRecord = 4
	// ioBufferBytes is the bufio size used for one run writer or reader.
	ioBufferBytes = 1 << 16
	// contextCheckStride bounds how many records the flush loop writes
	// between context checks.
	contextCheckStride = 4096
)

// Config bounds one sort. All numeric fields must be positive.
type Config struct {
	RecordBytes int    // fixed width of every record, 1..65536
	MemoryBytes int64  // maximum bytes held in memory for the run buffer plus its sort index; must allow at least one record (RecordBytes+4 bytes per record are accounted)
	TempBytes   int64  // maximum total bytes written to run files across the whole sort
	MaxRuns     int    // maximum number of run files; also the merge fan-in (each run is one open file during Merge)
	TempDir     string // parent directory in which a private temporary directory is created; "" means os.TempDir()
	Dedupe      bool   // when true, records that are byte-identical to the previously kept record are dropped (within each run when flushing and again during Merge), so Merge yields each distinct record once
}

// Stats describes the completed sort.
type Stats struct {
	Records         int64 // records passed to Add
	Emitted         int64 // records delivered to the Merge visitor
	Runs            int64 // run files written
	TempBytes       int64 // total bytes written to run files
	PeakMemoryBytes int64 // peak accounted in-memory bytes (buffer + index, allocated at capacity)
	// PeakBufferedBytes is the high-water mark of buffered records times
	// (RecordBytes + index bytes); it reflects records actually held.
	PeakBufferedBytes int64
}

// runFile records one written run so Merge can verify it is unchanged.
type runFile struct {
	path    string
	size    int64
	records int64
}

// Sorter buffers records in memory, spills sorted runs to private files and
// merges those runs in Merge.
type Sorter struct {
	cfg      Config
	dir      string
	capacity int
	buf      []byte
	index    []uint32
	count    int
	runs     []runFile
	stats    Stats
	failed   error
	merged   bool
	closed   bool
	closeErr error
}

// New validates cfg and creates the private temporary directory.
func New(cfg Config) (*Sorter, error) {
	if cfg.RecordBytes <= 0 {
		return nil, fmt.Errorf("extsort: RecordBytes %d, want 1..%d", cfg.RecordBytes, maxRecordBytes)
	}
	if cfg.RecordBytes > maxRecordBytes {
		return nil, fmt.Errorf("extsort: RecordBytes %d, want 1..%d", cfg.RecordBytes, maxRecordBytes)
	}
	if cfg.MemoryBytes <= 0 {
		return nil, fmt.Errorf("extsort: MemoryBytes %d, want a positive limit", cfg.MemoryBytes)
	}
	if cfg.TempBytes <= 0 {
		return nil, fmt.Errorf("extsort: TempBytes %d, want a positive limit", cfg.TempBytes)
	}
	if cfg.MaxRuns <= 0 {
		return nil, fmt.Errorf("extsort: MaxRuns %d, want a positive limit", cfg.MaxRuns)
	}

	perRecord := int64(cfg.RecordBytes) + indexBytesPerRecord
	capacity := cfg.MemoryBytes / perRecord
	if capacity < 1 {
		return nil, fmt.Errorf("extsort: MemoryBytes %d holds no record of %d bytes (%d bytes are accounted per record)", cfg.MemoryBytes, cfg.RecordBytes, perRecord)
	}
	// The sort index addresses records with uint32, and the buffer length must
	// stay inside int. Using less memory than allowed is always safe.
	if capacity > math.MaxUint32 {
		capacity = math.MaxUint32
	}
	if limit := int64(math.MaxInt) / int64(cfg.RecordBytes); capacity > limit {
		capacity = limit
	}

	dir, err := os.MkdirTemp(cfg.TempDir, "coimnet-extsort-")
	if err != nil {
		return nil, fmt.Errorf("extsort: create temporary directory: %w", err)
	}
	return &Sorter{cfg: cfg, dir: dir, capacity: int(capacity)}, nil
}

// Add copies one record of exactly cfg.RecordBytes bytes into the buffer.
// When the buffer is full it sorts the buffer and writes one run file.
// It returns ctx.Err() (wrapped) when the context is done, a wrapped
// ErrCapacity when writing a run would exceed TempBytes or MaxRuns, and
// an error for a record of the wrong length. After any error the Sorter is
// failed: every later Add or Merge returns an error and Close still cleans up.
func (s *Sorter) Add(ctx context.Context, record []byte) error {
	if err := s.usable("Add"); err != nil {
		return err
	}
	if ctx == nil {
		return s.fail(errors.New("extsort: nil context"))
	}
	if err := ctx.Err(); err != nil {
		return s.fail(fmt.Errorf("extsort: %w", err))
	}
	if len(record) != s.cfg.RecordBytes {
		return s.fail(fmt.Errorf("extsort: record of %d bytes, want exactly %d", len(record), s.cfg.RecordBytes))
	}

	if s.buf == nil {
		s.buf = make([]byte, s.capacity*s.cfg.RecordBytes)
		s.index = make([]uint32, s.capacity)
		accounted := int64(s.capacity) * (int64(s.cfg.RecordBytes) + indexBytesPerRecord)
		if accounted > s.stats.PeakMemoryBytes {
			s.stats.PeakMemoryBytes = accounted
		}
	}
	if s.count == s.capacity {
		if err := s.flush(ctx); err != nil {
			return s.fail(err)
		}
	}

	offset := s.count * s.cfg.RecordBytes
	copy(s.buf[offset:offset+s.cfg.RecordBytes], record)
	s.index[s.count] = uint32(s.count)
	s.count++
	s.stats.Records++
	if held := int64(s.count) * (int64(s.cfg.RecordBytes) + indexBytesPerRecord); held > s.stats.PeakBufferedBytes {
		s.stats.PeakBufferedBytes = held
	}
	return nil
}

// recordAt returns the buffered record with the given buffer slot. The result
// aliases the run buffer and must not be retained.
func (s *Sorter) recordAt(slot uint32) []byte {
	offset := int(slot) * s.cfg.RecordBytes
	return s.buf[offset : offset+s.cfg.RecordBytes]
}

// flush sorts the buffered records and writes them as one run file. It leaves
// no file behind when a limit is exceeded or the write fails.
func (s *Sorter) flush(ctx context.Context) error {
	if s.count == 0 {
		return nil
	}
	index := s.index[:s.count]
	slices.SortFunc(index, func(a, b uint32) int {
		return bytes.Compare(s.recordAt(a), s.recordAt(b))
	})

	writes := int64(s.count)
	if s.cfg.Dedupe {
		writes = 1
		for i := 1; i < len(index); i++ {
			if !bytes.Equal(s.recordAt(index[i]), s.recordAt(index[i-1])) {
				writes++
			}
		}
	}

	if len(s.runs) >= s.cfg.MaxRuns {
		return fmt.Errorf("extsort: %w: run %d exceeds MaxRuns %d", ErrCapacity, len(s.runs)+1, s.cfg.MaxRuns)
	}
	if writes > math.MaxInt64/int64(s.cfg.RecordBytes) {
		return fmt.Errorf("extsort: run of %d records of %d bytes overflows int64", writes, s.cfg.RecordBytes)
	}
	runBytes := writes * int64(s.cfg.RecordBytes)
	if runBytes > s.cfg.TempBytes-s.stats.TempBytes {
		return fmt.Errorf("extsort: %w: run of %d bytes exceeds TempBytes %d with %d already written", ErrCapacity, runBytes, s.cfg.TempBytes, s.stats.TempBytes)
	}

	path := filepath.Join(s.dir, fmt.Sprintf("run-%06d.bin", len(s.runs)))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("extsort: create run file: %w", err)
	}
	if err := s.writeRun(ctx, file, index); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("extsort: close run file %s: %w", path, err)
	}

	s.runs = append(s.runs, runFile{path: path, size: runBytes, records: writes})
	s.stats.Runs++
	s.stats.TempBytes += runBytes
	s.count = 0
	return nil
}

// writeRun writes the sorted records to file. The caller owns file and removes
// it when this fails.
func (s *Sorter) writeRun(ctx context.Context, file *os.File, index []uint32) error {
	writer := bufio.NewWriterSize(file, ioBufferBytes)
	width := s.cfg.RecordBytes
	previous := -1
	for i, slot := range index {
		if i%contextCheckStride == 0 {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("extsort: %w", err)
			}
		}
		if s.cfg.Dedupe && previous >= 0 && bytes.Equal(s.recordAt(slot), s.recordAt(index[previous])) {
			continue
		}
		offset := int(slot) * width
		if _, err := writer.Write(s.buf[offset : offset+width]); err != nil {
			return fmt.Errorf("extsort: write run file %s: %w", file.Name(), err)
		}
		previous = i
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("extsort: flush run file %s: %w", file.Name(), err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("extsort: sync run file %s: %w", file.Name(), err)
	}
	return nil
}

// mergeSource reads one run file during Merge.
type mergeSource struct {
	file   *os.File
	reader *bufio.Reader
	record []byte
	run    int
}

// next reads the run's next record, returning io.EOF at a clean end.
func (m *mergeSource) next() error {
	if _, err := io.ReadFull(m.reader, m.record); err != nil {
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("extsort: run file %s is truncated: it ends mid-record", m.file.Name())
		}
		return fmt.Errorf("extsort: read run file %s: %w", m.file.Name(), err)
	}
	return nil
}

// mergeHeap orders sources by record bytes, then by run index.
type mergeHeap []*mergeSource

func (h mergeHeap) Len() int { return len(h) }

func (h mergeHeap) Less(i, j int) bool {
	if cmp := bytes.Compare(h[i].record, h[j].record); cmp != 0 {
		return cmp < 0
	}
	return h[i].run < h[j].run
}

func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *mergeHeap) Push(x any) { *h = append(*h, x.(*mergeSource)) }

func (h *mergeHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	old[last] = nil
	*h = old[:last]
	return item
}

// Merge flushes the remaining buffer as a final run, then performs a k-way
// merge of every run file and calls visit once per record in ascending
// bytes.Compare order of the whole record. The record slice passed to visit is
// borrowed and only valid during the call. A visit error stops the merge and is
// returned wrapped. Merge may be called once; a second call returns an error.
// Zero records is valid: visit is never called and Stats has zero counts.
// Context is checked between records.
func (s *Sorter) Merge(ctx context.Context, visit func(record []byte) error) (Stats, error) {
	if err := s.usable("Merge"); err != nil {
		return s.stats, err
	}
	if s.merged {
		return s.stats, errors.New("extsort: Merge was already called")
	}
	s.merged = true
	if ctx == nil {
		return s.stats, s.fail(errors.New("extsort: nil context"))
	}
	if visit == nil {
		return s.stats, s.fail(errors.New("extsort: nil visit function"))
	}
	if err := ctx.Err(); err != nil {
		return s.stats, s.fail(fmt.Errorf("extsort: %w", err))
	}
	if err := s.flush(ctx); err != nil {
		return s.stats, s.fail(err)
	}
	if len(s.runs) == 0 {
		return s.stats, nil
	}

	sources := make([]*mergeSource, 0, len(s.runs))
	defer func() {
		for _, source := range sources {
			source.file.Close()
		}
	}()

	sorted := make(mergeHeap, 0, len(s.runs))
	for i, run := range s.runs {
		file, err := os.Open(run.path)
		if err != nil {
			return s.stats, s.fail(fmt.Errorf("extsort: open run file: %w", err))
		}
		source := &mergeSource{file: file, reader: bufio.NewReaderSize(file, ioBufferBytes), record: make([]byte, s.cfg.RecordBytes), run: i}
		sources = append(sources, source)
		if err := s.verifyRun(file, run); err != nil {
			return s.stats, s.fail(err)
		}
		switch err := source.next(); {
		case errors.Is(err, io.EOF):
			continue
		case err != nil:
			return s.stats, s.fail(err)
		}
		sorted = append(sorted, source)
	}
	heap.Init(&sorted)

	previous := make([]byte, s.cfg.RecordBytes)
	havePrevious := false
	for sorted.Len() > 0 {
		select {
		case <-ctx.Done():
			return s.stats, s.fail(fmt.Errorf("extsort: %w", ctx.Err()))
		default:
		}
		source := sorted[0]
		if !s.cfg.Dedupe || !havePrevious || !bytes.Equal(previous, source.record) {
			if err := visit(source.record); err != nil {
				return s.stats, s.fail(fmt.Errorf("extsort: visit record %d: %w", s.stats.Emitted, err))
			}
			s.stats.Emitted++
			if s.cfg.Dedupe {
				copy(previous, source.record)
				havePrevious = true
			}
		}
		switch err := source.next(); {
		case errors.Is(err, io.EOF):
			heap.Pop(&sorted)
		case err != nil:
			return s.stats, s.fail(err)
		default:
			heap.Fix(&sorted, 0)
		}
	}
	return s.stats, nil
}

// verifyRun checks that a run file still has the size it was written with and
// holds whole records only.
func (s *Sorter) verifyRun(file *os.File, run runFile) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("extsort: stat run file %s: %w", run.path, err)
	}
	size := info.Size()
	if size%int64(s.cfg.RecordBytes) != 0 {
		return fmt.Errorf("extsort: run file %s is truncated: %d bytes is not a multiple of the %d-byte record width", run.path, size, s.cfg.RecordBytes)
	}
	if size < run.size {
		return fmt.Errorf("extsort: run file %s is truncated: %d bytes, want %d", run.path, size, run.size)
	}
	if size > run.size {
		return fmt.Errorf("extsort: run file %s changed after it was written: %d bytes, want %d", run.path, size, run.size)
	}
	return nil
}

// usable reports why the sorter cannot serve another call.
func (s *Sorter) usable(op string) error {
	if s.closed {
		return fmt.Errorf("extsort: %s after Close", op)
	}
	if s.failed != nil {
		return fmt.Errorf("extsort: %s after an earlier failure: %w", op, s.failed)
	}
	if s.merged && op == "Add" {
		return errors.New("extsort: Add after Merge")
	}
	return nil
}

// fail records the first error and returns it unchanged.
func (s *Sorter) fail(err error) error {
	if s.failed == nil {
		s.failed = err
	}
	return err
}

// Close removes the private temporary directory and every run file. It is
// idempotent and safe after any error or cancellation. It returns the first
// removal error.
func (s *Sorter) Close() error {
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	s.buf = nil
	s.index = nil
	s.count = 0

	var first error
	for _, run := range s.runs {
		if err := os.Remove(run.path); err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
			first = fmt.Errorf("extsort: remove run file %s: %w", run.path, err)
		}
	}
	s.runs = nil
	if s.dir != "" {
		if err := os.RemoveAll(s.dir); err != nil && first == nil {
			first = fmt.Errorf("extsort: remove temporary directory %s: %w", s.dir, err)
		}
	}
	s.closeErr = first
	return first
}
