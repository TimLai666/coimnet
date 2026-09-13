package connectome

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/feather"
	"github.com/TimLai666/coimnet/internal/fileio"
)

const (
	// StoreSchemaVersion identifies the graph store file contract.
	StoreSchemaVersion = "coimnet-graph-store/v1"

	storeMagic          = "COIMGRF1"
	storeTrailerBytes   = 4 + sha256.Size + len(storeMagic)
	storeIOBufferBytes  = 1 << 20
	maxStoreStringBytes = 1 << 20
	maxStoreSections    = 5

	sectionNodeIDs     = "node_ids"
	sectionNodeMeta    = "node_meta"
	sectionEdges       = "edges"
	sectionBatchStarts = "batch_starts"
	sectionReport      = "report"
)

// ErrStoreCorrupt wraps every structural or hash mismatch found while
// reading a graph store.
var ErrStoreCorrupt = errors.New("connectome: graph store is corrupt or does not match its hashes")

var storeSectionOrder = [...]string{sectionNodeIDs, sectionNodeMeta, sectionEdges, sectionBatchStarts, sectionReport}

var transmitterStatusCodes = map[string]byte{
	TransmitterPredicted:    0,
	TransmitterUnknown:      1,
	TransmitterNotAvailable: 2,
	TransmitterNotMapped:    3,
}

// StoreLimits bounds one Load. All values must be positive. MaxMemoryBytes
// bounds graph arrays, metadata bytes, JSON input lengths (including the
// strict decoder input copy), and section read buffers. Go allocation slack,
// decoded JSON objects and runtime overhead are excluded; this is not RSS.
type StoreLimits struct {
	MaxFileBytes   int64 `json:"max_file_bytes"`
	MaxFooterBytes int64 `json:"max_footer_bytes"`
	MaxMemoryBytes int64 `json:"max_memory_bytes"`
}

func (l StoreLimits) validate() error {
	if l.MaxFileBytes <= 0 || l.MaxFooterBytes <= 0 || l.MaxMemoryBytes <= 0 {
		return fmt.Errorf("connectome: every store limit must be positive: %+v", l)
	}
	return nil
}

// SectionInfo locates one hashed section inside a store file.
type SectionInfo struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}

// StoreReceipt describes a published or verified store file. A load never
// asserts publication durability.
type StoreReceipt struct {
	Path                string        `json:"path"`
	Bytes               int64         `json:"bytes"`
	SHA256              string        `json:"sha256"`
	FooterSHA256        string        `json:"footer_sha256"`
	NodeCount           uint64        `json:"node_count"`
	EdgeCount           uint64        `json:"edge_count"`
	Sections            []SectionInfo `json:"sections"`
	DurabilityConfirmed bool          `json:"durability_confirmed"`
}

type storeScanOptions struct {
	MaxFooterBytes int64 `json:"max_footer_bytes"`
	MaxArrowBytes  int64 `json:"max_arrow_bytes"`
	MaxRows        int64 `json:"max_rows"`
}

// storeFooter is the JSON trailer of a store file. It carries no timestamp
// so identical graphs produce identical files.
type storeFooter struct {
	SchemaVersion    string           `json:"schema_version"`
	ConverterVersion string           `json:"converter_version"`
	Dataset          string           `json:"dataset"`
	Namespace        string           `json:"namespace"`
	SourceVersion    string           `json:"source_version"`
	ManifestHash     string           `json:"manifest_hash"`
	Predicate        PredicateReport  `json:"predicate"`
	EdgeView         EdgeViewMode     `json:"edge_view"`
	IndexWidth       int              `json:"index_width"`
	NodeCount        uint64           `json:"node_count"`
	EdgeCount        uint64           `json:"edge_count"`
	BatchCount       uint64           `json:"batch_count"`
	RawRows          uint64           `json:"raw_rows"`
	Sections         []SectionInfo    `json:"sections"`
	Hashes           ResultHashes     `json:"hashes"`
	Weights          SourceFile       `json:"weights"`
	ScanOptions      storeScanOptions `json:"scan_options"`
	WeightsFields    WeightsFields    `json:"weights_fields"`
}

// countingWriter tracks the absolute offset and hashes the whole file.
type countingWriter struct {
	w      *bufio.Writer
	file   hash.Hash
	offset int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.file.Write(p[:n])
	c.offset += int64(n)
	return n, err
}

// sectionWriter hashes one section while writing it.
type sectionWriter struct {
	ctx    context.Context
	out    *countingWriter
	hash   hash.Hash
	name   string
	start  int64
	writes int64
}

func (s *sectionWriter) write(p []byte) error {
	if _, err := s.out.Write(p); err != nil {
		return fmt.Errorf("connectome: write store section %s: %w", s.name, err)
	}
	s.hash.Write(p)
	s.writes++
	if s.writes%contextCheckStride == 0 {
		if err := s.ctx.Err(); err != nil {
			return fmt.Errorf("connectome: %w", err)
		}
	}
	return nil
}

func (s *sectionWriter) u8(v byte) error { return s.write([]byte{v}) }
func (s *sectionWriter) u32(v uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return s.write(b[:])
}
func (s *sectionWriter) u64(v uint64) error {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return s.write(b[:])
}
func (s *sectionWriter) i64(v int64) error { return s.u64(uint64(v)) }

func (s *sectionWriter) nullString(v NullString) error {
	if !v.Valid {
		return s.u8(0)
	}
	if len(v.Value) > maxStoreStringBytes {
		return fmt.Errorf("connectome: string of %d bytes exceeds store limit %d", len(v.Value), maxStoreStringBytes)
	}
	if err := s.u8(1); err != nil {
		return err
	}
	if err := s.u32(uint32(len(v.Value))); err != nil {
		return err
	}
	return s.write([]byte(v.Value))
}

func (s *sectionWriter) finish() SectionInfo {
	return SectionInfo{Name: s.name, Offset: s.start, Length: s.out.offset - s.start, SHA256: hex.EncodeToString(s.hash.Sum(nil))}
}

func beginSection(ctx context.Context, out *countingWriter, name string) *sectionWriter {
	return &sectionWriter{ctx: ctx, out: out, hash: sha256.New(), name: name, start: out.offset}
}

// Save writes the graph to a new file and publishes it without overwriting.
// The file has no timestamp, so the same graph always produces the same
// bytes. Cancellation before publication leaves nothing behind; failures
// after publication are reported without removing the published file.
func Save(ctx context.Context, path string, g *Graph) (receipt StoreReceipt, retErr error) {
	if ctx == nil {
		return receipt, errors.New("connectome: nil context")
	}
	if err := ctx.Err(); err != nil {
		return receipt, fmt.Errorf("connectome: %w", err)
	}
	if path == "" {
		return receipt, errors.New("connectome: store path must not be empty")
	}
	if g == nil {
		return receipt, errors.New("connectome: nil graph")
	}
	if g.namespace == "" || g.report.SchemaVersion != ReportSchemaVersion {
		return receipt, errors.New("connectome: uninitialized graph")
	}
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		return receipt, fmt.Errorf("connectome: store directory: %w", err)
	} else if !info.IsDir() {
		return receipt, fmt.Errorf("connectome: store parent %q is not a directory", dir)
	}
	if _, err := os.Lstat(path); err == nil {
		return receipt, fmt.Errorf("connectome: store %q already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return receipt, fmt.Errorf("connectome: stat store path: %w", err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return receipt, fmt.Errorf("connectome: create store temporary file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	tempClosed := false
	defer func() {
		if !tempClosed {
			tempClosed = true
			if closeErr := temp.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("connectome: close store temporary file: %w", closeErr))
			}
		}
		if removeTemp {
			if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("connectome: remove store temporary file: %w", removeErr))
			}
		}
	}()

	out := &countingWriter{w: bufio.NewWriterSize(temp, storeIOBufferBytes), file: sha256.New()}
	footer, err := writeStore(ctx, out, g)
	if err != nil {
		return receipt, err
	}
	footerJSON, err := json.Marshal(footer)
	if err != nil {
		return receipt, fmt.Errorf("connectome: encode store footer: %w", err)
	}
	if int64(len(footerJSON)) > math.MaxUint32 {
		return receipt, fmt.Errorf("%w: store footer exceeds 4 GiB", ErrCapacity)
	}
	footerSum := sha256.Sum256(footerJSON)
	var trailer [storeTrailerBytes]byte
	binary.BigEndian.PutUint32(trailer[:4], uint32(len(footerJSON)))
	copy(trailer[4:4+sha256.Size], footerSum[:])
	copy(trailer[4+sha256.Size:], storeMagic)
	for _, chunk := range [][]byte{footerJSON, trailer[:]} {
		if _, err := out.Write(chunk); err != nil {
			return receipt, fmt.Errorf("connectome: write store footer: %w", err)
		}
	}
	if err := out.w.Flush(); err != nil {
		return receipt, fmt.Errorf("connectome: flush store: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return receipt, fmt.Errorf("connectome: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return receipt, fmt.Errorf("connectome: sync store temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempClosed = true
		return receipt, fmt.Errorf("connectome: close store temporary file: %w", err)
	}
	tempClosed = true
	if err := ctx.Err(); err != nil {
		return receipt, fmt.Errorf("connectome: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		return receipt, fmt.Errorf("connectome: publish store without overwrite: %w", err)
	}
	receipt = StoreReceipt{
		Path:         path,
		Bytes:        out.offset,
		SHA256:       hex.EncodeToString(out.file.Sum(nil)),
		FooterSHA256: hex.EncodeToString(footerSum[:]),
		NodeCount:    footer.NodeCount,
		EdgeCount:    footer.EdgeCount,
		Sections:     append([]SectionInfo(nil), footer.Sections...),
	}
	removeErr := os.Remove(tempPath)
	if removeErr == nil {
		removeTemp = false
	}
	// The store is visible once Link succeeds; finish the directory sync even
	// if the caller cancelled, so cancellation never removes a published file.
	syncErr := syncStoreDirectory(dir)
	receipt.DurabilityConfirmed = syncErr == nil
	if removeErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("connectome: store published but temporary cleanup failed: %w", removeErr))
	}
	if syncErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("connectome: store published but durability unconfirmed: %w", syncErr))
	}
	if err := ctx.Err(); err != nil {
		retErr = errors.Join(retErr, fmt.Errorf("connectome: store published; context ended after publication: %w", err))
	}
	return receipt, retErr
}

func syncStoreDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}

// writeStore writes magic and every section, returning the footer.
func writeStore(ctx context.Context, out *countingWriter, g *Graph) (storeFooter, error) {
	if _, err := out.Write([]byte(storeMagic)); err != nil {
		return storeFooter{}, fmt.Errorf("connectome: write store magic: %w", err)
	}
	aggregated := g.report.EdgeView == EdgeViewAggregatedPairs
	if aggregated && g.edges.rows == nil && g.edges.count() > 0 {
		return storeFooter{}, errors.New("connectome: aggregated graph has no row counts")
	}
	width := 4
	if g.edges.wide {
		width = 8
	}
	var sections []SectionInfo

	section := beginSection(ctx, out, sectionNodeIDs)
	for _, id := range g.nodeIDs {
		if err := section.u64(id); err != nil {
			return storeFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionNodeMeta)
	for i := range g.nodes {
		n := &g.nodes[i]
		for _, v := range []int64{n.position.Batch, n.position.Row, n.position.AbsoluteRow} {
			if err := section.i64(v); err != nil {
				return storeFooter{}, err
			}
		}
		for f := range n.fields {
			if err := section.nullString(n.fields[f]); err != nil {
				return storeFooter{}, err
			}
		}
		code, ok := transmitterStatusCodes[n.transmitter.Status]
		if !ok {
			return storeFooter{}, fmt.Errorf("connectome: node %d has unknown transmitter status %q", i, n.transmitter.Status)
		}
		if err := section.u8(code); err != nil {
			return storeFooter{}, err
		}
		if err := section.nullString(n.transmitter.Consensus); err != nil {
			return storeFooter{}, err
		}
		if err := section.nullString(n.transmitter.Predicted); err != nil {
			return storeFooter{}, err
		}
		confidence := byte(0)
		if n.transmitter.Confidence.Valid {
			confidence = 1
		}
		if err := section.u8(confidence); err != nil {
			return storeFooter{}, err
		}
		if err := section.u64(math.Float64bits(n.transmitter.Confidence.Value)); err != nil {
			return storeFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionEdges)
	for i := 0; i < g.edges.count(); i++ {
		source, target := g.edges.endpoints(i)
		if width == 4 {
			if err := section.u32(uint32(source)); err != nil {
				return storeFooter{}, err
			}
			if err := section.u32(uint32(target)); err != nil {
				return storeFooter{}, err
			}
		} else {
			if err := section.u64(source); err != nil {
				return storeFooter{}, err
			}
			if err := section.u64(target); err != nil {
				return storeFooter{}, err
			}
		}
		if err := section.i64(g.edges.absRow[i]); err != nil {
			return storeFooter{}, err
		}
		valid := byte(0)
		if g.edges.weightValid[i] {
			valid = 1
		}
		if err := section.u8(valid); err != nil {
			return storeFooter{}, err
		}
		if err := section.i64(g.edges.weight[i]); err != nil {
			return storeFooter{}, err
		}
		if aggregated {
			if err := section.u32(g.edges.rows[i]); err != nil {
				return storeFooter{}, err
			}
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionBatchStarts)
	for _, start := range g.batchStarts {
		if err := section.i64(start); err != nil {
			return storeFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	reportJSON, err := json.Marshal(g.report)
	if err != nil {
		return storeFooter{}, fmt.Errorf("connectome: encode store report: %w", err)
	}
	section = beginSection(ctx, out, sectionReport)
	if err := section.write(reportJSON); err != nil {
		return storeFooter{}, err
	}
	sections = append(sections, section.finish())

	return storeFooter{
		SchemaVersion:    StoreSchemaVersion,
		ConverterVersion: g.report.ConverterVersion,
		Dataset:          g.report.Dataset,
		Namespace:        g.namespace,
		SourceVersion:    g.report.SourceVersion,
		ManifestHash:     g.report.ManifestHash,
		Predicate:        g.report.Predicate,
		EdgeView:         g.report.EdgeView,
		IndexWidth:       width,
		NodeCount:        uint64(len(g.nodes)),
		EdgeCount:        uint64(g.edges.count()),
		BatchCount:       uint64(len(g.batchStarts)),
		RawRows:          g.report.Raw.Rows,
		Sections:         sections,
		Hashes:           g.report.Hashes,
		Weights:          g.weights,
		ScanOptions:      storeScanOptions{MaxFooterBytes: g.scan.MaxFooterBytes, MaxArrowBytes: g.scan.MaxArrowBytes, MaxRows: g.scan.MaxRows},
		WeightsFields:    g.scanFields,
	}, nil
}

// sectionReader reads exactly one section, hashing every byte.
type sectionReader struct {
	ctx       context.Context
	name      string
	r         *bufio.Reader
	hash      hash.Hash
	whole     hash.Hash
	remaining int64
	reads     int64
	scratch   [8]byte
}

func newSectionReader(ctx context.Context, file io.ReaderAt, section SectionInfo, whole hash.Hash) *sectionReader {
	return &sectionReader{
		ctx:       ctx,
		name:      section.Name,
		r:         bufio.NewReaderSize(io.NewSectionReader(file, section.Offset, section.Length), storeIOBufferBytes),
		hash:      sha256.New(),
		whole:     whole,
		remaining: section.Length,
	}
}

func (s *sectionReader) corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: section %s: %s", ErrStoreCorrupt, s.name, fmt.Sprintf(format, args...))
}

func (s *sectionReader) readFull(buf []byte) error {
	if int64(len(buf)) > s.remaining {
		return s.corrupt("record needs %d bytes but only %d remain", len(buf), s.remaining)
	}
	if _, err := io.ReadFull(s.r, buf); err != nil {
		return fmt.Errorf("%w: section %s: read: %v", ErrStoreCorrupt, s.name, err)
	}
	s.hash.Write(buf)
	s.whole.Write(buf)
	s.remaining -= int64(len(buf))
	s.reads++
	if s.reads%contextCheckStride == 0 {
		if err := s.ctx.Err(); err != nil {
			return fmt.Errorf("connectome: %w", err)
		}
	}
	return nil
}

func (s *sectionReader) u8() (byte, error) {
	if err := s.readFull(s.scratch[:1]); err != nil {
		return 0, err
	}
	return s.scratch[0], nil
}

func (s *sectionReader) u32() (uint32, error) {
	if err := s.readFull(s.scratch[:4]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(s.scratch[:4]), nil
}

func (s *sectionReader) u64() (uint64, error) {
	if err := s.readFull(s.scratch[:8]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(s.scratch[:8]), nil
}

func (s *sectionReader) i64() (int64, error) {
	v, err := s.u64()
	return int64(v), err
}

func (s *sectionReader) nullString() (NullString, error) {
	valid, err := s.u8()
	if err != nil {
		return NullString{}, err
	}
	switch valid {
	case 0:
		return NullString{}, nil
	case 1:
	default:
		return NullString{}, s.corrupt("string validity byte %d", valid)
	}
	length, err := s.u32()
	if err != nil {
		return NullString{}, err
	}
	if length > maxStoreStringBytes {
		return NullString{}, s.corrupt("string of %d bytes exceeds limit %d", length, maxStoreStringBytes)
	}
	buf := make([]byte, length)
	if err := s.readFull(buf); err != nil {
		return NullString{}, err
	}
	if !utf8.Valid(buf) {
		return NullString{}, s.corrupt("string is not valid UTF-8")
	}
	return NullString{Valid: true, Value: string(buf)}, nil
}

func (s *sectionReader) finish(expected string) error {
	if s.remaining != 0 {
		return s.corrupt("%d trailing bytes", s.remaining)
	}
	if actual := hex.EncodeToString(s.hash.Sum(nil)); actual != expected {
		return s.corrupt("sha256 %s does not match footer %s", actual, expected)
	}
	return nil
}

// Load reads and verifies a store file and rebuilds the graph. Every
// section hash, the footer hash, the structural invariants of the views and
// the node index, edge order and report hashes are checked before a graph
// is returned.
func Load(ctx context.Context, path string, limits StoreLimits) (*Graph, error) {
	graph, _, err := LoadWithReceipt(ctx, path, limits)
	return graph, err
}

// LoadWithReceipt returns the SHA-256 of the exact bytes used to validate the
// graph. It reads one open file and never reopens the pathname for hashing.
// A replaced pathname therefore cannot mix a graph with a different digest.
func LoadWithReceipt(ctx context.Context, path string, limits StoreLimits) (graph *Graph, receipt StoreReceipt, retErr error) {
	if ctx == nil {
		return nil, receipt, errors.New("connectome: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, receipt, fmt.Errorf("connectome: %w", err)
	}
	if err := limits.validate(); err != nil {
		return nil, receipt, err
	}
	file, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return nil, receipt, fmt.Errorf("connectome: open store: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("connectome: close store: %w", closeErr))
			graph = nil
			receipt = StoreReceipt{}
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, receipt, fmt.Errorf("connectome: stat store: %w", err)
	}
	size := info.Size()
	if size > limits.MaxFileBytes {
		return nil, receipt, fmt.Errorf("%w: store has %d bytes, limit %d", ErrCapacity, size, limits.MaxFileBytes)
	}
	if size < int64(len(storeMagic)+storeTrailerBytes) {
		return nil, receipt, fmt.Errorf("%w: store of %d bytes is too small", ErrStoreCorrupt, size)
	}
	head := make([]byte, len(storeMagic))
	if _, err := file.ReadAt(head, 0); err != nil {
		return nil, receipt, fmt.Errorf("connectome: read store magic: %w", err)
	}
	if string(head) != storeMagic {
		return nil, receipt, fmt.Errorf("%w: header magic %q", ErrStoreCorrupt, string(head))
	}
	var trailer [storeTrailerBytes]byte
	if _, err := file.ReadAt(trailer[:], size-int64(storeTrailerBytes)); err != nil {
		return nil, receipt, fmt.Errorf("connectome: read store trailer: %w", err)
	}
	if string(trailer[4+sha256.Size:]) != storeMagic {
		return nil, receipt, fmt.Errorf("%w: trailer magic", ErrStoreCorrupt)
	}
	footerLength := int64(binary.BigEndian.Uint32(trailer[:4]))
	if footerLength > limits.MaxFooterBytes {
		return nil, receipt, fmt.Errorf("%w: store footer has %d bytes, limit %d", ErrCapacity, footerLength, limits.MaxFooterBytes)
	}
	footerStart := size - int64(storeTrailerBytes) - footerLength
	if footerLength <= 0 || footerStart < int64(len(storeMagic)) {
		return nil, receipt, fmt.Errorf("%w: footer length %d does not fit the file", ErrStoreCorrupt, footerLength)
	}
	memory := budget{limit: limits.MaxMemoryBytes}
	// Reserve JSON bytes and the strict decoder's input copy before allocation.
	for _, reservation := range []struct {
		n    int64
		name string
	}{
		{footerLength, "store footer"}, {footerLength, "store footer input copy"},
		{maxStoreSections * storeIOBufferBytes, "store section buffers"},
	} {
		if err := memory.reserve(reservation.n, reservation.name); err != nil {
			return nil, receipt, err
		}
	}
	if uint64(footerLength) > uint64(int(^uint(0)>>1)) {
		return nil, receipt, fmt.Errorf("%w: footer exceeds platform allocation limit", ErrCapacity)
	}
	footerJSON := make([]byte, footerLength)
	if _, err := file.ReadAt(footerJSON, footerStart); err != nil {
		return nil, receipt, fmt.Errorf("connectome: read store footer: %w", err)
	}
	if sum := sha256.Sum256(footerJSON); !bytes.Equal(sum[:], trailer[4:4+sha256.Size]) {
		return nil, receipt, fmt.Errorf("%w: footer sha256 mismatch", ErrStoreCorrupt)
	}
	var footer storeFooter
	if err := decodeStrict(bytes.NewReader(footerJSON), footerLength, &footer); err != nil {
		return nil, receipt, fmt.Errorf("%w: footer: %v", ErrStoreCorrupt, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, receipt, fmt.Errorf("connectome: %w", err)
	}
	if err := validateFooter(footer, footerStart); err != nil {
		return nil, receipt, err
	}
	aggregated := footer.EdgeView == EdgeViewAggregatedPairs
	wide := footer.IndexWidth == 8
	sections := map[string]SectionInfo{}
	for _, section := range footer.Sections {
		sections[section.Name] = section
	}
	edgeRecordLength := int64(2*footer.IndexWidth + 8 + 1 + 8)
	if aggregated {
		edgeRecordLength += 4
	}
	if sections[sectionNodeIDs].Length != int64(footer.NodeCount)*8 || sections[sectionEdges].Length != int64(footer.EdgeCount)*edgeRecordLength || sections[sectionBatchStarts].Length != int64(footer.BatchCount)*8 {
		return nil, receipt, fmt.Errorf("%w: section lengths do not match the declared counts", ErrStoreCorrupt)
	}

	// validateFooter bounds every product below; reserve separately so sums
	// cannot wrap before the budget checks them.
	for _, reservation := range []struct {
		n    int64
		name string
	}{
		{int64(footer.NodeCount) * (nodeBytes + 8), "store node arrays"},
		{sections[sectionNodeMeta].Length, "store node metadata"},
		{sections[sectionNodeMeta].Length, "store metadata string scratch"},
		{int64(footer.EdgeCount) * edgeBytes(wide, aggregated), "store edge arrays"},
		{int64(footer.BatchCount) * 8, "store batch starts"},
		{sections[sectionReport].Length, "store report"},
		{sections[sectionReport].Length, "store report input copy"},
	} {
		if err := memory.reserve(reservation.n, reservation.name); err != nil {
			return nil, receipt, err
		}
	}
	if uint64(sections[sectionReport].Length) > uint64(int(^uint(0)>>1)) {
		return nil, receipt, fmt.Errorf("%w: report exceeds platform allocation limit", ErrCapacity)
	}
	whole := sha256.New()
	whole.Write(head)

	// node_ids
	reader := newSectionReader(ctx, file, sections[sectionNodeIDs], whole)
	nodeIDs := make([]uint64, 0, footer.NodeCount)
	nodeHash := sha256.New()
	nodeHash.Write([]byte(footer.Namespace))
	for i := uint64(0); i < footer.NodeCount; i++ {
		id, err := reader.u64()
		if err != nil {
			return nil, receipt, err
		}
		if i > 0 && id <= nodeIDs[i-1] {
			return nil, receipt, reader.corrupt("node IDs are not strictly increasing at index %d", i)
		}
		nodeIDs = append(nodeIDs, id)
		nodeHash.Write(reader.scratch[:8])
	}
	if err := reader.finish(sections[sectionNodeIDs].SHA256); err != nil {
		return nil, receipt, err
	}
	if actual := hex.EncodeToString(nodeHash.Sum(nil)); actual != footer.Hashes.NodeIndex {
		return nil, receipt, fmt.Errorf("%w: node index hash %s does not match footer %s", ErrStoreCorrupt, actual, footer.Hashes.NodeIndex)
	}

	// node_meta
	reader = newSectionReader(ctx, file, sections[sectionNodeMeta], whole)
	nodes := make([]nodeData, footer.NodeCount)
	statusNames := map[byte]string{}
	for name, code := range transmitterStatusCodes {
		statusNames[code] = name
	}
	for i := range nodes {
		n := &nodes[i]
		n.id = nodeIDs[i]
		n.position.Role = RoleAnnotations
		var err error
		if n.position.Batch, err = reader.i64(); err != nil {
			return nil, receipt, err
		}
		if n.position.Row, err = reader.i64(); err != nil {
			return nil, receipt, err
		}
		if n.position.AbsoluteRow, err = reader.i64(); err != nil {
			return nil, receipt, err
		}
		if n.position.Batch < 0 || n.position.Row < 0 || n.position.AbsoluteRow < 0 {
			return nil, receipt, reader.corrupt("node %d has a negative source position", i)
		}
		for f := range n.fields {
			if n.fields[f], err = reader.nullString(); err != nil {
				return nil, receipt, err
			}
		}
		code, err := reader.u8()
		if err != nil {
			return nil, receipt, err
		}
		status, ok := statusNames[code]
		if !ok {
			return nil, receipt, reader.corrupt("node %d has transmitter status code %d", i, code)
		}
		n.transmitter.Status = status
		if n.transmitter.Consensus, err = reader.nullString(); err != nil {
			return nil, receipt, err
		}
		if n.transmitter.Predicted, err = reader.nullString(); err != nil {
			return nil, receipt, err
		}
		valid, err := reader.u8()
		if err != nil {
			return nil, receipt, err
		}
		bits, err := reader.u64()
		if err != nil {
			return nil, receipt, err
		}
		switch valid {
		case 0:
			if bits != 0 {
				return nil, receipt, reader.corrupt("node %d has a null confidence with non-zero bits", i)
			}
		case 1:
			value := math.Float64frombits(bits)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, receipt, reader.corrupt("node %d has a non-finite confidence", i)
			}
			n.transmitter.Confidence = NullFloat64{Valid: true, Value: value}
		default:
			return nil, receipt, reader.corrupt("node %d has confidence validity byte %d", i, valid)
		}
		n.ntSeen = status == TransmitterPredicted || status == TransmitterUnknown
	}
	if err := reader.finish(sections[sectionNodeMeta].SHA256); err != nil {
		return nil, receipt, err
	}

	// edges
	reader = newSectionReader(ctx, file, sections[sectionEdges], whole)
	edges := edgeStore{wide: wide}
	if wide {
		edges.src64 = make([]uint64, 0, footer.EdgeCount)
		edges.dst64 = make([]uint64, 0, footer.EdgeCount)
	} else {
		edges.src32 = make([]uint32, 0, footer.EdgeCount)
		edges.dst32 = make([]uint32, 0, footer.EdgeCount)
	}
	edges.absRow = make([]int64, 0, footer.EdgeCount)
	edges.weight = make([]int64, 0, footer.EdgeCount)
	edges.weightValid = make([]bool, 0, footer.EdgeCount)
	if aggregated {
		edges.rows = make([]uint32, 0, footer.EdgeCount)
	}
	edgeHash := sha256.New()
	var previousSource, previousTarget uint64
	var previousRow int64
	for i := uint64(0); i < footer.EdgeCount; i++ {
		var source, target uint64
		var err error
		if wide {
			if source, err = reader.u64(); err != nil {
				return nil, receipt, err
			}
			if target, err = reader.u64(); err != nil {
				return nil, receipt, err
			}
		} else {
			s32, err := reader.u32()
			if err != nil {
				return nil, receipt, err
			}
			t32, err := reader.u32()
			if err != nil {
				return nil, receipt, err
			}
			source, target = uint64(s32), uint64(t32)
		}
		absRow, err := reader.i64()
		if err != nil {
			return nil, receipt, err
		}
		valid, err := reader.u8()
		if err != nil {
			return nil, receipt, err
		}
		weight, err := reader.i64()
		if err != nil {
			return nil, receipt, err
		}
		rows := uint32(1)
		if aggregated {
			if rows, err = reader.u32(); err != nil {
				return nil, receipt, err
			}
		}
		if source >= footer.NodeCount || target >= footer.NodeCount {
			return nil, receipt, reader.corrupt("edge %d references node index beyond %d", i, footer.NodeCount)
		}
		if absRow < 0 || uint64(absRow) >= footer.RawRows {
			return nil, receipt, reader.corrupt("edge %d has absolute row %d outside %d raw rows", i, absRow, footer.RawRows)
		}
		if valid > 1 || (valid == 0 && weight != 0) {
			return nil, receipt, reader.corrupt("edge %d has an invalid weight encoding", i)
		}
		if aggregated && (valid != 1 || weight < 0 || rows == 0) {
			return nil, receipt, reader.corrupt("aggregated edge %d needs a non-negative weight and at least one row", i)
		}
		if i > 0 {
			ordered := source > previousSource || (source == previousSource && target > previousTarget) ||
				(!aggregated && source == previousSource && target == previousTarget && absRow > previousRow)
			if !ordered {
				return nil, receipt, reader.corrupt("edge %d breaks the canonical order", i)
			}
		}
		previousSource, previousTarget, previousRow = source, target, absRow
		edges.append(source, target, absRow, NullInt64{Valid: valid == 1, Value: weight}, rows)
		var encoded [edgeRecordBytes + 4]byte
		binary.BigEndian.PutUint64(encoded[0:8], source)
		binary.BigEndian.PutUint64(encoded[8:16], target)
		binary.BigEndian.PutUint64(encoded[16:24], uint64(absRow))
		encoded[24] = valid
		binary.BigEndian.PutUint64(encoded[25:33], uint64(weight))
		if aggregated {
			binary.BigEndian.PutUint32(encoded[33:37], rows)
			edgeHash.Write(encoded[:])
		} else {
			edgeHash.Write(encoded[:edgeRecordBytes])
		}
	}
	if err := reader.finish(sections[sectionEdges].SHA256); err != nil {
		return nil, receipt, err
	}
	if actual := hex.EncodeToString(edgeHash.Sum(nil)); actual != footer.Hashes.EdgeOrder {
		return nil, receipt, fmt.Errorf("%w: edge order hash %s does not match footer %s", ErrStoreCorrupt, actual, footer.Hashes.EdgeOrder)
	}

	// batch_starts
	reader = newSectionReader(ctx, file, sections[sectionBatchStarts], whole)
	batchStarts := make([]int64, 0, footer.BatchCount)
	for i := uint64(0); i < footer.BatchCount; i++ {
		start, err := reader.i64()
		if err != nil {
			return nil, receipt, err
		}
		if start < 0 || (i == 0 && start != 0) || (i > 0 && start < batchStarts[i-1]) || uint64(start) > footer.RawRows {
			return nil, receipt, reader.corrupt("batch start %d at index %d is not monotone within %d rows", start, i, footer.RawRows)
		}
		batchStarts = append(batchStarts, start)
	}
	if err := reader.finish(sections[sectionBatchStarts].SHA256); err != nil {
		return nil, receipt, err
	}

	// report
	reader = newSectionReader(ctx, file, sections[sectionReport], whole)
	reportJSON := make([]byte, sections[sectionReport].Length)
	if err := reader.readFull(reportJSON); err != nil {
		return nil, receipt, err
	}
	if err := reader.finish(sections[sectionReport].SHA256); err != nil {
		return nil, receipt, err
	}
	var report GraphReport
	if err := decodeStrict(bytes.NewReader(reportJSON), int64(len(reportJSON)), &report); err != nil {
		return nil, receipt, fmt.Errorf("%w: report: %v", ErrStoreCorrupt, err)
	}
	if err := verifyStoredReport(report, footer); err != nil {
		return nil, receipt, err
	}
	if err := ctx.Err(); err != nil {
		return nil, receipt, fmt.Errorf("connectome: %w", err)
	}
	whole.Write(footerJSON)
	whole.Write(trailer[:])
	receipt = StoreReceipt{Path: path, Bytes: size, SHA256: hex.EncodeToString(whole.Sum(nil)),
		FooterSHA256: hex.EncodeToString(trailer[4 : 4+sha256.Size]), NodeCount: footer.NodeCount,
		EdgeCount: footer.EdgeCount, Sections: append([]SectionInfo(nil), footer.Sections...)}
	return &Graph{
		namespace:   footer.Namespace,
		nodeIDs:     nodeIDs,
		nodes:       nodes,
		edges:       edges,
		batchStarts: batchStarts,
		weights:     footer.Weights,
		scan: feather.Options{
			MaxFileBytes:   footer.Weights.Bytes,
			MaxFooterBytes: footer.ScanOptions.MaxFooterBytes,
			MaxArrowBytes:  footer.ScanOptions.MaxArrowBytes,
			MaxRows:        footer.ScanOptions.MaxRows,
		},
		scanFields: footer.WeightsFields,
		report:     report,
	}, receipt, nil
}

func validateFooter(footer storeFooter, footerStart int64) error {
	corrupt := func(format string, args ...any) error {
		return fmt.Errorf("%w: footer: %s", ErrStoreCorrupt, fmt.Sprintf(format, args...))
	}
	if footer.SchemaVersion != StoreSchemaVersion {
		return corrupt("schema_version %q, want %q", footer.SchemaVersion, StoreSchemaVersion)
	}
	if footer.ConverterVersion != ConverterVersion {
		return corrupt("converter_version %q, want %q", footer.ConverterVersion, ConverterVersion)
	}
	if footer.Namespace == "" || footer.Dataset == "" {
		return corrupt("converter_version, namespace and dataset must be set")
	}
	if footer.EdgeView != EdgeViewRows && footer.EdgeView != EdgeViewAggregatedPairs {
		return corrupt("edge_view %q", footer.EdgeView)
	}
	if footer.IndexWidth != 4 && footer.IndexWidth != 8 {
		return corrupt("index_width %d", footer.IndexWidth)
	}
	if footer.IndexWidth == 4 && footer.NodeCount > math.MaxUint32 {
		return corrupt("index_width 4 cannot address %d nodes", footer.NodeCount)
	}
	if footer.NodeCount > uint64(math.MaxInt64/(nodeBytes+8)) || footer.EdgeCount > math.MaxInt64/64 || footer.BatchCount > math.MaxInt64/8 ||
		footer.NodeCount > uint64(int(^uint(0)>>1)) || footer.EdgeCount > uint64(int(^uint(0)>>1)) || footer.BatchCount > uint64(int(^uint(0)>>1)) {
		return corrupt("declared counts overflow")
	}
	for _, h := range []string{footer.Hashes.NodeIndex, footer.Hashes.EdgeOrder, footer.Hashes.Report, footer.ManifestHash, footer.Predicate.Hash} {
		if !isLowerHex(h, sha256.Size*2) {
			return corrupt("hash %q is not lowercase sha256 hex", h)
		}
	}
	if len(footer.Sections) != maxStoreSections {
		return corrupt("%d sections, want %d", len(footer.Sections), maxStoreSections)
	}
	next := int64(len(storeMagic))
	for i, section := range footer.Sections {
		if section.Name != storeSectionOrder[i] {
			return corrupt("section %d is %q, want %q", i, section.Name, storeSectionOrder[i])
		}
		if section.Offset != next || section.Length < 0 || section.Length > footerStart-next {
			return corrupt("section %s offset %d length %d does not fit before the footer at %d", section.Name, section.Offset, section.Length, footerStart)
		}
		if !isLowerHex(section.SHA256, sha256.Size*2) {
			return corrupt("section %s sha256 %q", section.Name, section.SHA256)
		}
		next += section.Length
	}
	if next != footerStart {
		return corrupt("sections end at %d but the footer starts at %d", next, footerStart)
	}
	if footer.Weights.Role != RoleWeights || footer.Weights.Bytes <= 0 || !isLowerHex(footer.Weights.SHA256, sha256.Size*2) || footer.Weights.Path == "" {
		return corrupt("weights source is incomplete")
	}
	if footer.ScanOptions.MaxFooterBytes <= 0 || footer.ScanOptions.MaxArrowBytes <= 0 || footer.ScanOptions.MaxRows <= 0 {
		return corrupt("scan options must be positive")
	}
	for _, name := range []string{footer.WeightsFields.Source, footer.WeightsFields.Target, footer.WeightsFields.Value} {
		if !fieldNamePattern.MatchString(name) {
			return corrupt("weights field %q is not a plain column name", name)
		}
	}
	return nil
}

func verifyStoredReport(report GraphReport, footer storeFooter) error {
	corrupt := func(format string, args ...any) error {
		return fmt.Errorf("%w: report: %s", ErrStoreCorrupt, fmt.Sprintf(format, args...))
	}
	if report.SchemaVersion != ReportSchemaVersion {
		return corrupt("schema_version %q", report.SchemaVersion)
	}
	recomputed, err := report.hash()
	if err != nil {
		return corrupt("hash: %v", err)
	}
	if recomputed != report.Hashes.Report || report.Hashes != footer.Hashes {
		return corrupt("report hashes %+v do not match recomputed %s and footer %+v", report.Hashes, recomputed, footer.Hashes)
	}
	if report.Annotated.Nodes != footer.NodeCount || report.Annotated.Edges != footer.EdgeCount || report.Raw.Rows != footer.RawRows {
		return corrupt("counts nodes=%d edges=%d raw=%d do not match footer nodes=%d edges=%d raw=%d", report.Annotated.Nodes, report.Annotated.Edges, report.Raw.Rows, footer.NodeCount, footer.EdgeCount, footer.RawRows)
	}
	if report.EdgeView != footer.EdgeView || report.ManifestHash != footer.ManifestHash || report.Predicate != footer.Predicate || report.Namespace != footer.Namespace || report.ConverterVersion != footer.ConverterVersion || report.Dataset != footer.Dataset || report.SourceVersion != footer.SourceVersion {
		return corrupt("identity fields do not match the footer")
	}
	weights := false
	for _, source := range report.Sources {
		if source.Role == RoleWeights && source.SHA256 == footer.Weights.SHA256 && source.Bytes == footer.Weights.Bytes {
			weights = true
		}
	}
	if !weights {
		return corrupt("weights source fingerprint is not in the report")
	}
	return nil
}
