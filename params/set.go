package params

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
	"slices"
	"unicode/utf8"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/TimLai666/coimnet/internal/strictjson"
)

const (
	// SetSchemaVersion identifies the parameter set file contract.
	SetSchemaVersion = "coimnet-parameter-set/v1"

	setMagic         = "COIMPRM1"
	setTrailerBytes  = 4 + sha256.Size + len(setMagic)
	setIOBufferBytes = 1 << 20
	maxSetNameBytes  = 1 << 16
	maxSetSections   = 8

	sectionEdgeWeight     = "edge_weight"
	sectionEdgeSign       = "edge_sign"
	sectionEdgeConfidence = "edge_sign_confidence"
	sectionEdgeNT         = "edge_transmitter"
	sectionEdgeMatched    = "edge_matched_synapses"
	sectionNodeTotals     = "node_totals"
	sectionNodeROI        = "node_primary_roi"
	sectionReport         = "derivation_report"

	// roiNone marks a node with no primary ROI in the stored dictionary.
	roiNone = math.MaxUint32
)

// ErrSetCorrupt wraps every structural or hash mismatch found while reading a
// parameter set file.
var ErrSetCorrupt = errors.New("params: parameter set is corrupt or does not match its hashes")

var setSectionOrder = [...]string{
	sectionEdgeWeight, sectionEdgeSign, sectionEdgeConfidence, sectionEdgeNT,
	sectionEdgeMatched, sectionNodeTotals, sectionNodeROI, sectionReport,
}

// GraphHashes identify the wiring a parameter set was derived for. They are
// the node index and edge order hashes of the graph report, so a set can only
// be applied to the store it came from.
type GraphHashes struct {
	NodeIndex string `json:"node_index"`
	EdgeOrder string `json:"edge_order"`
}

// SectionInfo locates one hashed section inside a parameter set file.
type SectionInfo struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}

// Set is one derived parameter assignment. The edge arrays follow the
// canonical edge order of the graph and the node arrays follow the node index
// order. A sign of 0 means unknown and is never a substituted default; a
// transmitter of NoTransmitter means no synapse matched the edge.
type Set struct {
	Source              string      `json:"source"`
	RulesHash           string      `json:"rules_hash"`
	GraphHashes         GraphHashes `json:"graph_hashes"`
	EdgeWeight          []float64   `json:"-"`
	EdgeSign            []int8      `json:"-"`
	EdgeSignConfidence  []float32   `json:"-"`
	EdgeTransmitter     []uint8     `json:"-"`
	EdgeMatchedSynapses []uint32    `json:"-"`
	NodePrimaryROI      []string    `json:"-"`
	NodePreTotal        []int64     `json:"-"`
	NodePostTotal       []int64     `json:"-"`
	Report              Report      `json:"report"`
}

// Nodes and Edges report the array shape the set was built for.
func (s *Set) Nodes() int { return len(s.NodePreTotal) }
func (s *Set) Edges() int { return len(s.EdgeWeight) }

// validate checks the array shapes and value ranges before the set is written.
func (s *Set) validate() error {
	if s == nil {
		return errors.New("params: nil parameter set")
	}
	if s.Source != SetSource {
		return fmt.Errorf("params: parameter set source %q, want %q", s.Source, SetSource)
	}
	if !isLowerHex(s.RulesHash, sha256.Size*2) || !isLowerHex(s.GraphHashes.NodeIndex, sha256.Size*2) || !isLowerHex(s.GraphHashes.EdgeOrder, sha256.Size*2) {
		return errors.New("params: parameter set hashes must be lowercase sha256 hexadecimal")
	}
	edges := len(s.EdgeWeight)
	for name, length := range map[string]int{
		sectionEdgeSign:       len(s.EdgeSign),
		sectionEdgeConfidence: len(s.EdgeSignConfidence),
		sectionEdgeNT:         len(s.EdgeTransmitter),
		sectionEdgeMatched:    len(s.EdgeMatchedSynapses),
	} {
		if length != edges {
			return fmt.Errorf("params: %s has %d entries, the set has %d edges", name, length, edges)
		}
	}
	nodes := len(s.NodePreTotal)
	for name, length := range map[string]int{
		"node_post_total":  len(s.NodePostTotal),
		"node_primary_roi": len(s.NodePrimaryROI),
	} {
		if length != nodes {
			return fmt.Errorf("params: %s has %d entries, the set has %d nodes", name, length, nodes)
		}
	}
	for i, weight := range s.EdgeWeight {
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return fmt.Errorf("params: edge %d weight is not finite", i)
		}
	}
	for i, sign := range s.EdgeSign {
		if sign < -1 || sign > 1 {
			return fmt.Errorf("params: edge %d sign is %d, want -1, 0 or +1", i, sign)
		}
	}
	for i, code := range s.EdgeTransmitter {
		if int(code) >= len(Transmitters) && code != NoTransmitter {
			return fmt.Errorf("params: edge %d transmitter code %d is not in 0..%d or %d", i, code, len(Transmitters)-1, NoTransmitter)
		}
	}
	for i, confidence := range s.EdgeSignConfidence {
		if math.IsNaN(float64(confidence)) || math.IsInf(float64(confidence), 0) {
			return fmt.Errorf("params: edge %d sign confidence is not finite", i)
		}
	}
	for i, name := range s.NodePrimaryROI {
		if len(name) > maxSetNameBytes {
			return fmt.Errorf("params: node %d primary ROI name has %d bytes, limit %d", i, len(name), maxSetNameBytes)
		}
		if !utf8.ValidString(name) {
			return fmt.Errorf("params: node %d primary ROI name is not valid UTF-8", i)
		}
	}
	if s.Report.SchemaVersion != ReportSchemaVersion {
		return fmt.Errorf("params: embedded report schema_version %q, want %q", s.Report.SchemaVersion, ReportSchemaVersion)
	}
	return nil
}

// CheckGraph reports whether the set was derived for exactly this graph. It
// compares the node index and edge order hashes and the array shapes, so a set
// can never be applied to different wiring.
func (s *Set) CheckGraph(g *connectome.Graph) error {
	if s == nil {
		return errors.New("params: nil parameter set")
	}
	if g == nil {
		return errors.New("params: nil graph")
	}
	hashes := g.Report().Hashes
	if hashes.NodeIndex != s.GraphHashes.NodeIndex {
		return fmt.Errorf("%w: graph node index hash %s, the parameter set was derived for %s", ErrSetCorrupt, hashes.NodeIndex, s.GraphHashes.NodeIndex)
	}
	if hashes.EdgeOrder != s.GraphHashes.EdgeOrder {
		return fmt.Errorf("%w: graph edge order hash %s, the parameter set was derived for %s", ErrSetCorrupt, hashes.EdgeOrder, s.GraphHashes.EdgeOrder)
	}
	if uint64(len(s.EdgeWeight)) != g.EdgeCount() || uint64(len(s.NodePreTotal)) != g.NodeCount() {
		return fmt.Errorf("%w: parameter set has %d edges and %d nodes, the graph has %d and %d", ErrSetCorrupt, len(s.EdgeWeight), len(s.NodePreTotal), g.EdgeCount(), g.NodeCount())
	}
	return nil
}

// SaveReceipt describes a published parameter set file. A load never asserts
// publication durability.
type SaveReceipt struct {
	Path                string        `json:"path"`
	Bytes               int64         `json:"bytes"`
	SHA256              string        `json:"sha256"`
	FooterSHA256        string        `json:"footer_sha256"`
	NodeCount           uint64        `json:"node_count"`
	EdgeCount           uint64        `json:"edge_count"`
	Sections            []SectionInfo `json:"sections"`
	DurabilityConfirmed bool          `json:"durability_confirmed"`
}

// LoadLimits bounds one Load. All values must be positive. MaxMemoryBytes
// bounds the parameter arrays, the ROI dictionary and the JSON input copies;
// Go allocation slack, decoded JSON objects and runtime overhead are excluded,
// so this is not an RSS bound.
type LoadLimits struct {
	MaxFileBytes   int64 `json:"max_file_bytes"`
	MaxFooterBytes int64 `json:"max_footer_bytes"`
	MaxMemoryBytes int64 `json:"max_memory_bytes"`
}

func (l LoadLimits) validate() error {
	if l.MaxFileBytes <= 0 || l.MaxFooterBytes <= 0 || l.MaxMemoryBytes <= 0 {
		return fmt.Errorf("params: every load limit must be positive: %+v", l)
	}
	return nil
}

// setFooter is the JSON trailer of a parameter set file. It carries no
// timestamp, so the same set always produces the same bytes.
type setFooter struct {
	SchemaVersion    string        `json:"schema_version"`
	Source           string        `json:"source"`
	RulesHash        string        `json:"rules_hash"`
	GraphHashes      GraphHashes   `json:"graph_hashes"`
	TransmitterOrder []string      `json:"transmitter_order"`
	NodeCount        uint64        `json:"node_count"`
	EdgeCount        uint64        `json:"edge_count"`
	ROINameCount     uint64        `json:"roi_name_count"`
	Sections         []SectionInfo `json:"sections"`
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
		return fmt.Errorf("params: write parameter set section %s: %w", s.name, err)
	}
	s.hash.Write(p)
	s.writes++
	if s.writes%contextCheckStride == 0 {
		if err := s.ctx.Err(); err != nil {
			return fmt.Errorf("params: %w", err)
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

func (s *sectionWriter) finish() SectionInfo {
	return SectionInfo{Name: s.name, Offset: s.start, Length: s.out.offset - s.start, SHA256: hex.EncodeToString(s.hash.Sum(nil))}
}

func beginSection(ctx context.Context, out *countingWriter, name string) *sectionWriter {
	return &sectionWriter{ctx: ctx, out: out, hash: sha256.New(), name: name, start: out.offset}
}

// Save writes the parameter set to a new file and publishes it without
// overwriting. The file has no timestamp, so the same set always produces the
// same bytes. Cancellation before publication leaves nothing behind; failures
// after publication are reported without removing the published file.
func Save(ctx context.Context, path string, set *Set) (receipt SaveReceipt, retErr error) {
	if ctx == nil {
		return receipt, errors.New("params: nil context")
	}
	if err := ctx.Err(); err != nil {
		return receipt, fmt.Errorf("params: %w", err)
	}
	if path == "" {
		return receipt, errors.New("params: parameter set path must not be empty")
	}
	if err := set.validate(); err != nil {
		return receipt, err
	}
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		return receipt, fmt.Errorf("params: parameter set directory: %w", err)
	} else if !info.IsDir() {
		return receipt, fmt.Errorf("params: parameter set parent %q is not a directory", dir)
	}
	if _, err := os.Lstat(path); err == nil {
		return receipt, fmt.Errorf("params: parameter set %q already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return receipt, fmt.Errorf("params: stat parameter set path: %w", err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return receipt, fmt.Errorf("params: create parameter set temporary file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	tempClosed := false
	defer func() {
		if !tempClosed {
			tempClosed = true
			if closeErr := temp.Close(); closeErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("params: close parameter set temporary file: %w", closeErr))
			}
		}
		if removeTemp {
			if removeErr := os.Remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("params: remove parameter set temporary file: %w", removeErr))
			}
		}
	}()

	out := &countingWriter{w: bufio.NewWriterSize(temp, setIOBufferBytes), file: sha256.New()}
	footer, err := writeSet(ctx, out, set)
	if err != nil {
		return receipt, err
	}
	footerJSON, err := json.Marshal(footer)
	if err != nil {
		return receipt, fmt.Errorf("params: encode parameter set footer: %w", err)
	}
	if int64(len(footerJSON)) > math.MaxUint32 {
		return receipt, fmt.Errorf("%w: parameter set footer exceeds 4 GiB", ErrCapacity)
	}
	footerSum := sha256.Sum256(footerJSON)
	var trailer [setTrailerBytes]byte
	binary.BigEndian.PutUint32(trailer[:4], uint32(len(footerJSON)))
	copy(trailer[4:4+sha256.Size], footerSum[:])
	copy(trailer[4+sha256.Size:], setMagic)
	for _, chunk := range [][]byte{footerJSON, trailer[:]} {
		if _, err := out.Write(chunk); err != nil {
			return receipt, fmt.Errorf("params: write parameter set footer: %w", err)
		}
	}
	if err := out.w.Flush(); err != nil {
		return receipt, fmt.Errorf("params: flush parameter set: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return receipt, fmt.Errorf("params: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return receipt, fmt.Errorf("params: sync parameter set temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempClosed = true
		return receipt, fmt.Errorf("params: close parameter set temporary file: %w", err)
	}
	tempClosed = true
	if err := ctx.Err(); err != nil {
		return receipt, fmt.Errorf("params: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		return receipt, fmt.Errorf("params: publish parameter set without overwrite: %w", err)
	}
	receipt = SaveReceipt{
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
	// The file is visible once Link succeeds; finish the directory sync even
	// if the caller cancelled, so cancellation never removes a published file.
	syncErr := syncDirectory(dir)
	receipt.DurabilityConfirmed = syncErr == nil
	if removeErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("params: parameter set published but temporary cleanup failed: %w", removeErr))
	}
	if syncErr != nil {
		retErr = errors.Join(retErr, fmt.Errorf("params: parameter set published but durability unconfirmed: %w", syncErr))
	}
	if err := ctx.Err(); err != nil {
		retErr = errors.Join(retErr, fmt.Errorf("params: parameter set published; context ended after publication: %w", err))
	}
	return receipt, retErr
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}

// roiDictionary builds the sorted, unique ROI name table and the per node
// index into it. Sorting makes the bytes depend only on the set content.
func roiDictionary(names []string) ([]string, []uint32) {
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" {
			unique = append(unique, name)
		}
	}
	slices.Sort(unique)
	unique = slices.Compact(unique)
	index := make([]uint32, len(names))
	for i, name := range names {
		if name == "" {
			index[i] = roiNone
			continue
		}
		position, _ := slices.BinarySearch(unique, name)
		index[i] = uint32(position)
	}
	return unique, index
}

// writeSet writes magic and every section, returning the footer.
func writeSet(ctx context.Context, out *countingWriter, set *Set) (setFooter, error) {
	if _, err := out.Write([]byte(setMagic)); err != nil {
		return setFooter{}, fmt.Errorf("params: write parameter set magic: %w", err)
	}
	var sections []SectionInfo

	section := beginSection(ctx, out, sectionEdgeWeight)
	for _, weight := range set.EdgeWeight {
		if err := section.u64(math.Float64bits(weight)); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionEdgeSign)
	for _, sign := range set.EdgeSign {
		if err := section.u8(byte(sign)); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionEdgeConfidence)
	for _, confidence := range set.EdgeSignConfidence {
		if err := section.u32(math.Float32bits(confidence)); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionEdgeNT)
	for _, code := range set.EdgeTransmitter {
		if err := section.u8(code); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionEdgeMatched)
	for _, matched := range set.EdgeMatchedSynapses {
		if err := section.u32(matched); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	section = beginSection(ctx, out, sectionNodeTotals)
	for i := range set.NodePreTotal {
		if err := section.u64(uint64(set.NodePreTotal[i])); err != nil {
			return setFooter{}, err
		}
		if err := section.u64(uint64(set.NodePostTotal[i])); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	names, index := roiDictionary(set.NodePrimaryROI)
	section = beginSection(ctx, out, sectionNodeROI)
	if err := section.u32(uint32(len(names))); err != nil {
		return setFooter{}, err
	}
	for _, name := range names {
		if err := section.u32(uint32(len(name))); err != nil {
			return setFooter{}, err
		}
		if err := section.write([]byte(name)); err != nil {
			return setFooter{}, err
		}
	}
	for _, position := range index {
		if err := section.u32(position); err != nil {
			return setFooter{}, err
		}
	}
	sections = append(sections, section.finish())

	reportJSON, err := json.Marshal(set.Report)
	if err != nil {
		return setFooter{}, fmt.Errorf("params: encode derivation report: %w", err)
	}
	section = beginSection(ctx, out, sectionReport)
	if err := section.write(reportJSON); err != nil {
		return setFooter{}, err
	}
	sections = append(sections, section.finish())

	return setFooter{
		SchemaVersion:    SetSchemaVersion,
		Source:           set.Source,
		RulesHash:        set.RulesHash,
		GraphHashes:      set.GraphHashes,
		TransmitterOrder: append([]string(nil), Transmitters[:]...),
		NodeCount:        uint64(len(set.NodePreTotal)),
		EdgeCount:        uint64(len(set.EdgeWeight)),
		ROINameCount:     uint64(len(names)),
		Sections:         sections,
	}, nil
}

// sectionReader reads exactly one section, hashing every byte.
type sectionReader struct {
	ctx       context.Context
	name      string
	r         *bufio.Reader
	hash      hash.Hash
	remaining int64
	reads     int64
	scratch   [8]byte
}

func newSectionReader(ctx context.Context, file io.ReaderAt, section SectionInfo) *sectionReader {
	return &sectionReader{
		ctx:       ctx,
		name:      section.Name,
		r:         bufio.NewReaderSize(io.NewSectionReader(file, section.Offset, section.Length), setIOBufferBytes),
		hash:      sha256.New(),
		remaining: section.Length,
	}
}

func (s *sectionReader) corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: section %s: %s", ErrSetCorrupt, s.name, fmt.Sprintf(format, args...))
}

func (s *sectionReader) readFull(buf []byte) error {
	if int64(len(buf)) > s.remaining {
		return s.corrupt("record needs %d bytes but only %d remain", len(buf), s.remaining)
	}
	if _, err := io.ReadFull(s.r, buf); err != nil {
		return fmt.Errorf("%w: section %s: read: %v", ErrSetCorrupt, s.name, err)
	}
	s.hash.Write(buf)
	s.remaining -= int64(len(buf))
	s.reads++
	if s.reads%contextCheckStride == 0 {
		if err := s.ctx.Err(); err != nil {
			return fmt.Errorf("params: %w", err)
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

func (s *sectionReader) finish(expected string) error {
	if s.remaining != 0 {
		return s.corrupt("%d trailing bytes", s.remaining)
	}
	if actual := hex.EncodeToString(s.hash.Sum(nil)); actual != expected {
		return s.corrupt("sha256 %s does not match footer %s", actual, expected)
	}
	return nil
}

// Load reads and verifies a parameter set file. Every section hash, the footer
// hash, the declared counts and the value ranges of every array are checked
// before a set is returned.
func Load(ctx context.Context, path string, limits LoadLimits) (*Set, error) {
	set, _, err := LoadWithReceipt(ctx, path, limits)
	return set, err
}

// LoadWithReceipt also returns the SHA-256 of the exact bytes it validated. It
// reads one open file and never reopens the pathname, so a replaced pathname
// cannot mix a set with a different digest. The receipt never asserts
// durability.
func LoadWithReceipt(ctx context.Context, path string, limits LoadLimits) (set *Set, receipt SaveReceipt, retErr error) {
	if ctx == nil {
		return nil, receipt, errors.New("params: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, receipt, fmt.Errorf("params: %w", err)
	}
	if err := limits.validate(); err != nil {
		return nil, receipt, err
	}
	file, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return nil, receipt, fmt.Errorf("params: open parameter set: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("params: close parameter set: %w", closeErr))
			set = nil
			receipt = SaveReceipt{}
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, receipt, fmt.Errorf("params: stat parameter set: %w", err)
	}
	size := info.Size()
	if size > limits.MaxFileBytes {
		return nil, receipt, fmt.Errorf("%w: parameter set has %d bytes, limit %d", ErrCapacity, size, limits.MaxFileBytes)
	}
	if size < int64(len(setMagic)+setTrailerBytes) {
		return nil, receipt, fmt.Errorf("%w: parameter set of %d bytes is too small", ErrSetCorrupt, size)
	}
	head := make([]byte, len(setMagic))
	if _, err := file.ReadAt(head, 0); err != nil {
		return nil, receipt, fmt.Errorf("params: read parameter set magic: %w", err)
	}
	if string(head) != setMagic {
		return nil, receipt, fmt.Errorf("%w: header magic %q", ErrSetCorrupt, string(head))
	}
	var trailer [setTrailerBytes]byte
	if _, err := file.ReadAt(trailer[:], size-int64(setTrailerBytes)); err != nil {
		return nil, receipt, fmt.Errorf("params: read parameter set trailer: %w", err)
	}
	if string(trailer[4+sha256.Size:]) != setMagic {
		return nil, receipt, fmt.Errorf("%w: trailer magic", ErrSetCorrupt)
	}
	footerLength := int64(binary.BigEndian.Uint32(trailer[:4]))
	if footerLength > limits.MaxFooterBytes {
		return nil, receipt, fmt.Errorf("%w: parameter set footer has %d bytes, limit %d", ErrCapacity, footerLength, limits.MaxFooterBytes)
	}
	footerStart := size - int64(setTrailerBytes) - footerLength
	if footerLength <= 0 || footerStart < int64(len(setMagic)) {
		return nil, receipt, fmt.Errorf("%w: footer length %d does not fit the file", ErrSetCorrupt, footerLength)
	}

	memory := budget{limit: limits.MaxMemoryBytes}
	for _, reservation := range []struct {
		n    int64
		name string
	}{
		{footerLength, "parameter set footer"},
		{footerLength, "parameter set footer input copy"},
		{setIOBufferBytes, "parameter set section buffer"},
	} {
		if err := memory.reserve(reservation.n, reservation.name); err != nil {
			return nil, receipt, err
		}
	}
	footerJSON := make([]byte, footerLength)
	if _, err := file.ReadAt(footerJSON, footerStart); err != nil {
		return nil, receipt, fmt.Errorf("params: read parameter set footer: %w", err)
	}
	if sum := sha256.Sum256(footerJSON); !bytes.Equal(sum[:], trailer[4:4+sha256.Size]) {
		return nil, receipt, fmt.Errorf("%w: footer sha256 mismatch", ErrSetCorrupt)
	}
	var footer setFooter
	if err := strictjson.Decode(bytes.NewReader(footerJSON), footerLength, &footer); err != nil {
		return nil, receipt, fmt.Errorf("%w: footer: %v", ErrSetCorrupt, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, receipt, fmt.Errorf("params: %w", err)
	}
	if err := validateSetFooter(footer, footerStart); err != nil {
		return nil, receipt, err
	}
	sections := map[string]SectionInfo{}
	for _, section := range footer.Sections {
		sections[section.Name] = section
	}
	edges := int64(footer.EdgeCount)
	nodes := int64(footer.NodeCount)
	for name, want := range map[string]int64{
		sectionEdgeWeight:     edges * 8,
		sectionEdgeSign:       edges,
		sectionEdgeConfidence: edges * 4,
		sectionEdgeNT:         edges,
		sectionEdgeMatched:    edges * 4,
		sectionNodeTotals:     nodes * 16,
	} {
		if sections[name].Length != want {
			return nil, receipt, fmt.Errorf("%w: section %s has %d bytes, the declared counts need %d", ErrSetCorrupt, name, sections[name].Length, want)
		}
	}
	if minimum := 4 + int64(footer.ROINameCount)*4 + nodes*4; sections[sectionNodeROI].Length < minimum {
		return nil, receipt, fmt.Errorf("%w: section %s has %d bytes, %d ROI names and %d nodes need at least %d", ErrSetCorrupt, sectionNodeROI, sections[sectionNodeROI].Length, footer.ROINameCount, nodes, minimum)
	}
	for _, reservation := range []struct {
		n    int64
		name string
	}{
		{edges * (8 + 1 + 4 + 1 + 4), "parameter arrays"},
		{nodes * (8 + 8 + 16), "node arrays"},
		{sections[sectionNodeROI].Length, "ROI dictionary"},
		{sections[sectionReport].Length, "derivation report"},
		{sections[sectionReport].Length, "derivation report input copy"},
	} {
		if err := memory.reserve(reservation.n, reservation.name); err != nil {
			return nil, receipt, err
		}
	}

	set = &Set{
		Source:              footer.Source,
		RulesHash:           footer.RulesHash,
		GraphHashes:         footer.GraphHashes,
		EdgeWeight:          make([]float64, edges),
		EdgeSign:            make([]int8, edges),
		EdgeSignConfidence:  make([]float32, edges),
		EdgeTransmitter:     make([]uint8, edges),
		EdgeMatchedSynapses: make([]uint32, edges),
		NodePrimaryROI:      make([]string, nodes),
		NodePreTotal:        make([]int64, nodes),
		NodePostTotal:       make([]int64, nodes),
	}

	reader := newSectionReader(ctx, file, sections[sectionEdgeWeight])
	for i := range set.EdgeWeight {
		bits, err := reader.u64()
		if err != nil {
			return nil, receipt, err
		}
		weight := math.Float64frombits(bits)
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return nil, receipt, reader.corrupt("edge %d weight is not finite", i)
		}
		set.EdgeWeight[i] = weight
	}
	if err := reader.finish(sections[sectionEdgeWeight].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionEdgeSign])
	for i := range set.EdgeSign {
		value, err := reader.u8()
		if err != nil {
			return nil, receipt, err
		}
		sign := int8(value)
		if sign < -1 || sign > 1 {
			return nil, receipt, reader.corrupt("edge %d sign is %d", i, sign)
		}
		set.EdgeSign[i] = sign
	}
	if err := reader.finish(sections[sectionEdgeSign].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionEdgeConfidence])
	for i := range set.EdgeSignConfidence {
		bits, err := reader.u32()
		if err != nil {
			return nil, receipt, err
		}
		confidence := math.Float32frombits(bits)
		if math.IsNaN(float64(confidence)) || math.IsInf(float64(confidence), 0) {
			return nil, receipt, reader.corrupt("edge %d sign confidence is not finite", i)
		}
		set.EdgeSignConfidence[i] = confidence
	}
	if err := reader.finish(sections[sectionEdgeConfidence].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionEdgeNT])
	for i := range set.EdgeTransmitter {
		code, err := reader.u8()
		if err != nil {
			return nil, receipt, err
		}
		if int(code) >= len(Transmitters) && code != NoTransmitter {
			return nil, receipt, reader.corrupt("edge %d transmitter code %d", i, code)
		}
		set.EdgeTransmitter[i] = code
	}
	if err := reader.finish(sections[sectionEdgeNT].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionEdgeMatched])
	for i := range set.EdgeMatchedSynapses {
		matched, err := reader.u32()
		if err != nil {
			return nil, receipt, err
		}
		set.EdgeMatchedSynapses[i] = matched
	}
	if err := reader.finish(sections[sectionEdgeMatched].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionNodeTotals])
	for i := range set.NodePreTotal {
		pre, err := reader.u64()
		if err != nil {
			return nil, receipt, err
		}
		post, err := reader.u64()
		if err != nil {
			return nil, receipt, err
		}
		set.NodePreTotal[i] = int64(pre)
		set.NodePostTotal[i] = int64(post)
	}
	if err := reader.finish(sections[sectionNodeTotals].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionNodeROI])
	count, err := reader.u32()
	if err != nil {
		return nil, receipt, err
	}
	if uint64(count) != footer.ROINameCount {
		return nil, receipt, reader.corrupt("%d ROI names, the footer declares %d", count, footer.ROINameCount)
	}
	names := make([]string, count)
	for i := range names {
		length, err := reader.u32()
		if err != nil {
			return nil, receipt, err
		}
		if length == 0 || length > maxSetNameBytes {
			return nil, receipt, reader.corrupt("ROI name %d has %d bytes", i, length)
		}
		buf := make([]byte, length)
		if err := reader.readFull(buf); err != nil {
			return nil, receipt, err
		}
		if !utf8.Valid(buf) {
			return nil, receipt, reader.corrupt("ROI name %d is not valid UTF-8", i)
		}
		names[i] = string(buf)
		if i > 0 && names[i] <= names[i-1] {
			return nil, receipt, reader.corrupt("ROI names are not strictly increasing at %d", i)
		}
	}
	for i := range set.NodePrimaryROI {
		position, err := reader.u32()
		if err != nil {
			return nil, receipt, err
		}
		if position == roiNone {
			continue
		}
		if position >= count {
			return nil, receipt, reader.corrupt("node %d names ROI %d of %d", i, position, count)
		}
		set.NodePrimaryROI[i] = names[position]
	}
	if err := reader.finish(sections[sectionNodeROI].SHA256); err != nil {
		return nil, receipt, err
	}

	reader = newSectionReader(ctx, file, sections[sectionReport])
	reportJSON := make([]byte, sections[sectionReport].Length)
	if err := reader.readFull(reportJSON); err != nil {
		return nil, receipt, err
	}
	if err := reader.finish(sections[sectionReport].SHA256); err != nil {
		return nil, receipt, err
	}
	if err := strictjson.Decode(bytes.NewReader(reportJSON), int64(len(reportJSON)), &set.Report); err != nil {
		return nil, receipt, fmt.Errorf("%w: report: %v", ErrSetCorrupt, err)
	}
	if err := verifyStoredReport(set.Report, footer); err != nil {
		return nil, receipt, err
	}
	if err := set.validate(); err != nil {
		return nil, receipt, fmt.Errorf("%w: %v", ErrSetCorrupt, err)
	}

	// Digest the exact bytes that were just validated, from the same open
	// file, so the receipt cannot describe a different pathname content.
	whole := sha256.New()
	if _, err := io.Copy(whole, io.NewSectionReader(file, 0, size)); err != nil {
		return nil, receipt, fmt.Errorf("params: digest parameter set: %w", err)
	}
	receipt = SaveReceipt{
		Path:         path,
		Bytes:        size,
		SHA256:       hex.EncodeToString(whole.Sum(nil)),
		FooterSHA256: hex.EncodeToString(trailer[4 : 4+sha256.Size]),
		NodeCount:    footer.NodeCount,
		EdgeCount:    footer.EdgeCount,
		Sections:     append([]SectionInfo(nil), footer.Sections...),
	}
	return set, receipt, nil
}

func validateSetFooter(footer setFooter, footerStart int64) error {
	corrupt := func(format string, args ...any) error {
		return fmt.Errorf("%w: footer: %s", ErrSetCorrupt, fmt.Sprintf(format, args...))
	}
	if footer.SchemaVersion != SetSchemaVersion {
		return corrupt("schema_version %q, want %q", footer.SchemaVersion, SetSchemaVersion)
	}
	if footer.Source != SetSource {
		return corrupt("source %q, want %q", footer.Source, SetSource)
	}
	if len(footer.TransmitterOrder) != len(Transmitters) {
		return corrupt("%d transmitter names, want %d", len(footer.TransmitterOrder), len(Transmitters))
	}
	for i, name := range footer.TransmitterOrder {
		if name != Transmitters[i] {
			return corrupt("transmitter %d is %q, want %q", i, name, Transmitters[i])
		}
	}
	for _, h := range []string{footer.RulesHash, footer.GraphHashes.NodeIndex, footer.GraphHashes.EdgeOrder} {
		if !isLowerHex(h, sha256.Size*2) {
			return corrupt("hash %q is not lowercase sha256 hex", h)
		}
	}
	if footer.EdgeCount > math.MaxInt64/16 || footer.NodeCount > math.MaxInt64/32 ||
		footer.EdgeCount > uint64(int(^uint(0)>>1)) || footer.NodeCount > uint64(int(^uint(0)>>1)) ||
		footer.ROINameCount > footer.NodeCount {
		return corrupt("declared counts are inconsistent or overflow")
	}
	if len(footer.Sections) != maxSetSections {
		return corrupt("%d sections, want %d", len(footer.Sections), maxSetSections)
	}
	next := int64(len(setMagic))
	for i, section := range footer.Sections {
		if section.Name != setSectionOrder[i] {
			return corrupt("section %d is %q, want %q", i, section.Name, setSectionOrder[i])
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
	return nil
}

func verifyStoredReport(report Report, footer setFooter) error {
	corrupt := func(format string, args ...any) error {
		return fmt.Errorf("%w: report: %s", ErrSetCorrupt, fmt.Sprintf(format, args...))
	}
	if report.SchemaVersion != ReportSchemaVersion {
		return corrupt("schema_version %q", report.SchemaVersion)
	}
	if report.Source != footer.Source || report.RulesHash != footer.RulesHash {
		return corrupt("source or rules hash does not match the footer")
	}
	if report.Rules.Hash() != footer.RulesHash {
		return corrupt("the embedded rules do not hash to %s", footer.RulesHash)
	}
	if report.Graph.NodeIndex != footer.GraphHashes.NodeIndex || report.Graph.EdgeOrder != footer.GraphHashes.EdgeOrder {
		return corrupt("graph hashes do not match the footer")
	}
	if report.Graph.Nodes != footer.NodeCount || report.Graph.Edges != footer.EdgeCount || report.Edges.Edges != footer.EdgeCount {
		return corrupt("counts nodes=%d edges=%d do not match footer nodes=%d edges=%d", report.Graph.Nodes, report.Graph.Edges, footer.NodeCount, footer.EdgeCount)
	}
	return nil
}
