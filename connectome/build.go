package connectome

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"slices"
	"strings"

	"unsafe"

	"github.com/TimLai666/coimnet/feather"
	"github.com/TimLai666/coimnet/internal/extsort"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/apache/arrow/go/v17/arrow"
)

// Accounted sizes of the private row structures, excluding string contents.
// unsafe is used only for Sizeof; no pointer arithmetic is performed.
const (
	annotationEntryBytes = int64(unsafe.Sizeof(annotationEntry{}))
	nodeBytes            = int64(unsafe.Sizeof(nodeData{}))
)

const (
	endpointRecordBytes = 8
	rawPairRecordBytes  = 16
	edgeRecordBytes     = 33
	hashChunkBytes      = 1 << 20
	contextCheckStride  = 4096

	unknownDefinition = "null, empty or the token 'unclear' (case-insensitive) in the consensus column; nodes without a prediction row count as unknown in the annotated view"
)

// BuildRequest describes one build. TempDir must be an existing directory;
// every temporary file is removed before Build returns.
type BuildRequest struct {
	Manifest DatasetManifest
	Limits   ResourceLimits
	TempDir  string
	EdgeView EdgeViewMode
}

// GraphBuildResult is the immutable graph and its report.
type GraphBuildResult struct {
	Graph  *Graph
	Report GraphReport
}

type budget struct {
	limit, used, peak int64
}

func (b *budget) reserve(n int64, what string) error {
	if n < 0 {
		return fmt.Errorf("connectome: negative reservation for %s", what)
	}
	if n > b.limit-b.used {
		return fmt.Errorf("%w: %s needs %d bytes with %d of %d already reserved", ErrCapacity, what, n, b.used, b.limit)
	}
	b.used += n
	if b.used > b.peak {
		b.peak = b.used
	}
	return nil
}

func (b *budget) release(n int64) {
	b.used -= n
	if b.used < 0 {
		b.used = 0
	}
}

type annotationEntry struct {
	id        uint64
	position  SourcePosition
	selected  bool
	fieldNull bool
	fields    [annotationFieldCount]NullString
}

type fingerprint struct {
	bytes  int64
	sha256 string
}

type builder struct {
	ctx       context.Context
	req       BuildRequest
	manifest  DatasetManifest
	namespace string
	limits    ResourceLimits
	memory    budget
	files     map[FileRole]SourceFile
	sources   map[FileRole]*SourceReport

	entries  []annotationEntry
	entryMem int64
	allIDs   []uint64
	nodeIDs  []uint64
	nodes    []nodeData

	raw       RawView
	annotated AnnotatedView

	batchStarts []int64
	edges       edgeStore
	edgeHash    hash.Hash
	sort        SortStats
	bufferedMax int64
}

// noteBuffered records a high-water mark of bytes actually held.
func (b *builder) noteBuffered(held int64) {
	if held > b.bufferedMax {
		b.bufferedMax = held
	}
}

// Build validates the request, verifies every source fingerprint, scans the
// three sources once each, sorts with bounded memory and returns the
// immutable graph with its report. Any error or cancellation returns no
// partial result and leaves no temporary files behind.
func Build(ctx context.Context, req BuildRequest) (*GraphBuildResult, error) {
	if ctx == nil {
		return nil, errors.New("connectome: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("connectome: %w", err)
	}
	if err := req.Manifest.Validate(); err != nil {
		return nil, err
	}
	if err := req.Limits.validate(); err != nil {
		return nil, err
	}
	switch req.EdgeView {
	case "":
		req.EdgeView = EdgeViewRows
	case EdgeViewRows, EdgeViewAggregatedPairs:
	default:
		return nil, fmt.Errorf("connectome: unknown edge view %q", req.EdgeView)
	}
	if req.TempDir == "" {
		return nil, errors.New("connectome: temporary directory must be set")
	}
	if info, err := os.Stat(req.TempDir); err != nil {
		return nil, fmt.Errorf("connectome: temporary directory: %w", err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("connectome: temporary path %q is not a directory", req.TempDir)
	}
	if !req.Manifest.Identity.WeightsEndpointsAreAnnotationIDs {
		return nil, fmt.Errorf("%w: set identity.weights_endpoints_are_annotation_ids with evidence to build the annotated view", ErrIdentityMapping)
	}
	if req.EdgeView == EdgeViewAggregatedPairs && req.Manifest.DuplicateSemantics != DuplicateSemanticsAdditivePartitions {
		return nil, fmt.Errorf("%w: manifest duplicate_semantics is %q, aggregation requires %q", ErrAggregationEvidence, req.Manifest.DuplicateSemantics, DuplicateSemanticsAdditivePartitions)
	}

	b := &builder{
		ctx:       ctx,
		req:       req,
		manifest:  req.Manifest,
		namespace: req.Manifest.Namespace,
		limits:    req.Limits,
		memory:    budget{limit: req.Limits.MaxMemoryBytes},
		files:     map[FileRole]SourceFile{},
		sources:   map[FileRole]*SourceReport{},
		edgeHash:  sha256.New(),
	}
	for _, file := range req.Manifest.Files {
		b.files[file.Role] = file
		b.sources[file.Role] = &SourceReport{Role: file.Role, Path: file.Path, URL: file.URL, ETag: file.ETag, Bytes: file.Bytes, SHA256: file.SHA256, HashStatus: file.HashStatus}
	}
	if err := b.preflight(); err != nil {
		return nil, err
	}
	if err := b.scanAnnotations(); err != nil {
		return nil, err
	}
	if err := b.catalogAnnotations(); err != nil {
		return nil, err
	}
	if err := b.scanNeurotransmitters(); err != nil {
		return nil, err
	}
	if err := b.scanWeights(); err != nil {
		return nil, err
	}
	if err := b.postflight(); err != nil {
		return nil, err
	}
	report, err := b.report()
	if err != nil {
		return nil, err
	}
	graph := &Graph{
		namespace:   b.namespace,
		nodeIDs:     b.nodeIDs,
		nodes:       b.nodes,
		edges:       b.edges,
		batchStarts: b.batchStarts,
		weights:     b.files[RoleWeights],
		scan:        b.scanOptions(b.files[RoleWeights]),
		scanFields:  b.manifest.FieldMapping.Weights,
		report:      report,
	}
	return &GraphBuildResult{Graph: graph, Report: report.clone()}, nil
}

func (b *builder) check() error {
	if err := b.ctx.Err(); err != nil {
		return fmt.Errorf("connectome: %w", err)
	}
	return nil
}

func (b *builder) scanOptions(file SourceFile) feather.Options {
	return feather.Options{
		MaxFileBytes:   file.Bytes,
		MaxFooterBytes: b.limits.MaxFooterBytes,
		MaxArrowBytes:  b.limits.MaxArrowBytes,
		MaxRows:        b.limits.MaxRows,
	}
}

func (b *builder) preflight() error {
	for _, role := range []FileRole{RoleWeights, RoleAnnotations, RoleNeurotransmitters} {
		if err := verifyFingerprint(b.ctx, b.files[role]); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) postflight() error {
	for _, role := range []FileRole{RoleWeights, RoleAnnotations, RoleNeurotransmitters} {
		file := b.files[role]
		after, err := hashFile(b.ctx, file.Path)
		if err != nil {
			return err
		}
		source := b.sources[role]
		source.SHA256After = after.sha256
		source.FingerprintStable = after.bytes == file.Bytes && strings.EqualFold(after.sha256, file.SHA256)
		if !source.FingerprintStable {
			return fmt.Errorf("%w: %s %s changed during the build", ErrSourceChanged, role, file.Path)
		}
	}
	return nil
}

// scan runs one bounded Feather scan and records batches and rows.
func (b *builder) scan(role FileRole, want map[string]string, visit func(record arrow.Record, batch int64) error) error {
	file := b.files[role]
	batch := int64(0)
	report, err := feather.Scan(b.ctx, file.Path, b.scanOptions(file), func(record arrow.Record) error {
		if err := visit(record, batch); err != nil {
			return err
		}
		batch++
		return nil
	})
	if err != nil {
		if ctxErr := b.ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return fmt.Errorf("connectome: %w", ctxErr)
		}
		return fmt.Errorf("connectome: scan %s source: %w", role, err)
	}
	if !report.Complete {
		return fmt.Errorf("connectome: scan of %s source did not complete", role)
	}
	if err := checkReportedFields(role, report.Fields, want); err != nil {
		return err
	}
	source := b.sources[role]
	source.RecordBatches = report.RecordBatches
	source.Rows = report.Rows
	return nil
}

func (b *builder) annotationColumns() [annotationFieldCount]string {
	a := b.manifest.FieldMapping.Annotations
	return [annotationFieldCount]string{a.Status, a.StatusLabel, a.Class, a.Superclass, a.Subclass, a.Type, a.Instance, a.SomaSide, a.ReceptorType}
}

func (b *builder) scanAnnotations() error {
	mapping := b.manifest.FieldMapping.Annotations
	predicate := b.manifest.Selection
	names := b.annotationColumns()
	want := map[string]string{mapping.ID: "int64", predicate.Field: "string"}
	for _, name := range names {
		if name != "" {
			want[name] = "string"
		}
	}
	absRow := int64(0)
	return b.scan(RoleAnnotations, want, func(record arrow.Record, batch int64) error {
		ids, err := int64Column(record, mapping.ID)
		if err != nil {
			return err
		}
		predicateColumn, err := stringColumn(record, predicate.Field)
		if err != nil {
			return err
		}
		var columns [annotationFieldCount]stringValues
		for i, name := range names {
			if columns[i], err = optionalStringColumn(record, name); err != nil {
				return err
			}
		}
		rows := int(record.NumRows())
		for i := 0; i < rows; i++ {
			if i%contextCheckStride == 0 {
				if err := b.check(); err != nil {
					return err
				}
			}
			b.annotated.AnnotationRows++
			if ids.IsNull(i) {
				b.annotated.AnnotationExclusions.NullID++
				absRow++
				continue
			}
			if ids.Value(i) < 0 {
				b.annotated.AnnotationExclusions.InvalidID++
				absRow++
				continue
			}
			entry := annotationEntry{
				id:       uint64(ids.Value(i)),
				position: SourcePosition{Role: RoleAnnotations, Batch: batch, Row: int64(i), AbsoluteRow: absRow},
			}
			if predicateColumn.IsNull(i) {
				entry.fieldNull = true
			} else {
				entry.selected = predicateColumn.Value(i) == predicate.Equals
			}
			stringBytes := int64(0)
			for f := range names {
				entry.fields[f] = nullString(columns[f], i)
				stringBytes += int64(len(entry.fields[f].Value))
			}
			if err := b.memory.reserve(annotationEntryBytes+stringBytes, "annotation rows"); err != nil {
				return err
			}
			b.entryMem += annotationEntryBytes + stringBytes
			b.entries = append(b.entries, entry)
			absRow++
		}
		return nil
	})
}

// catalogAnnotations sorts annotation rows by ID, drops duplicates keeping
// the first source row, and assigns continuous indices to selected rows in
// ascending ID order.
func (b *builder) catalogAnnotations() error {
	if err := b.check(); err != nil {
		return err
	}
	b.noteBuffered(b.memory.used)
	defaultTransmitterStatus := TransmitterNotAvailable
	if b.manifest.FieldMapping.Neurotransmitters.Consensus == "" {
		defaultTransmitterStatus = TransmitterNotMapped
	}
	slices.SortFunc(b.entries, func(x, y annotationEntry) int {
		if x.id != y.id {
			if x.id < y.id {
				return -1
			}
			return 1
		}
		if x.position.AbsoluteRow < y.position.AbsoluteRow {
			return -1
		}
		if x.position.AbsoluteRow > y.position.AbsoluteRow {
			return 1
		}
		return 0
	})
	unique := 0
	selected := 0
	for i := range b.entries {
		if i > 0 && b.entries[i].id == b.entries[i-1].id {
			continue
		}
		unique++
		if b.entries[i].selected {
			selected++
		}
	}
	if err := b.memory.reserve(int64(unique)*8+int64(selected)*8, "annotation ID catalogs"); err != nil {
		return err
	}
	b.allIDs = make([]uint64, 0, unique)
	b.nodeIDs = make([]uint64, 0, selected)
	b.nodes = make([]nodeData, 0, selected)
	for i := range b.entries {
		entry := &b.entries[i]
		if i > 0 && entry.id == b.entries[i-1].id {
			b.annotated.AnnotationExclusions.DuplicateID++
			continue
		}
		b.allIDs = append(b.allIDs, entry.id)
		if entry.fieldNull {
			b.annotated.AnnotationExclusions.PredicateFieldNull++
			b.annotated.AnnotationExclusions.PredicateFalse++
			continue
		}
		if !entry.selected {
			b.annotated.AnnotationExclusions.PredicateFalse++
			continue
		}
		b.nodeIDs = append(b.nodeIDs, entry.id)
		b.nodes = append(b.nodes, nodeData{
			id:          entry.id,
			position:    entry.position,
			fields:      entry.fields,
			transmitter: TransmitterPrediction{Status: defaultTransmitterStatus},
		})
	}
	// The scan entries are released and only the selected nodes stay
	// accounted; the copies were covered by the entry reservation.
	b.entries = nil
	b.memory.release(b.entryMem)
	b.entryMem = 0
	if err := b.memory.reserve(int64(len(b.nodes))*nodeBytes, "selected nodes"); err != nil {
		return err
	}
	b.annotated.Nodes = uint64(len(b.nodes))
	return nil
}

func unknownConsensus(value NullString) bool {
	if !value.Valid {
		return true
	}
	trimmed := strings.TrimSpace(value.Value)
	return trimmed == "" || strings.EqualFold(trimmed, "unclear")
}

func (b *builder) scanNeurotransmitters() error {
	mapping := b.manifest.FieldMapping.Neurotransmitters
	join := b.manifest.Identity.NeurotransmitterIDsAreAnnotationIDs
	want := map[string]string{mapping.ID: "int64"}
	if mapping.Consensus != "" {
		want[mapping.Consensus] = "string"
	}
	if mapping.Predicted != "" {
		want[mapping.Predicted] = "string"
	}
	if mapping.Confidence != "" {
		want[mapping.Confidence] = "float64"
	}
	var ntIDs []uint64
	var ntMem int64
	stats := &b.raw.Transmitter
	consensusMapped := mapping.Consensus != ""
	stats.ConsensusStatus = ColumnNotMapped
	if consensusMapped {
		stats.ConsensusStatus = ColumnMapped
	}
	err := b.scan(RoleNeurotransmitters, want, func(record arrow.Record, batch int64) error {
		ids, err := int64Column(record, mapping.ID)
		if err != nil {
			return err
		}
		consensus, err := optionalStringColumn(record, mapping.Consensus)
		if err != nil {
			return err
		}
		predicted, err := optionalStringColumn(record, mapping.Predicted)
		if err != nil {
			return err
		}
		confidence, err := optionalFloat64Column(record, mapping.Confidence)
		if err != nil {
			return err
		}
		rows := int(record.NumRows())
		if err := b.memory.reserve(int64(rows)*8, "neurotransmitter IDs"); err != nil {
			return err
		}
		ntMem += int64(rows) * 8
		for i := 0; i < rows; i++ {
			if i%contextCheckStride == 0 {
				if err := b.check(); err != nil {
					return err
				}
			}
			stats.Rows++
			consensusValue := nullString(consensus, i)
			unknown := consensusMapped && unknownConsensus(consensusValue)
			if unknown {
				stats.UnknownRows++
			}
			var confidenceValue NullFloat64
			if confidence != nil && !confidence.IsNull(i) {
				value := confidence.Value(i)
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return fmt.Errorf("connectome: neurotransmitter confidence at batch %d row %d is not finite", batch, i)
				}
				confidenceValue = NullFloat64{Valid: true, Value: value}
			}
			if ids.IsNull(i) {
				stats.NullIDs++
				continue
			}
			if ids.Value(i) < 0 {
				stats.InvalidIDs++
				continue
			}
			id := uint64(ids.Value(i))
			ntIDs = append(ntIDs, id)
			if !join {
				continue
			}
			index, found := slices.BinarySearch(b.nodeIDs, id)
			if !found || b.nodes[index].ntSeen {
				continue
			}
			node := &b.nodes[index]
			node.ntSeen = true
			node.transmitter = TransmitterPrediction{
				Status:     TransmitterPredicted,
				Consensus:  consensusValue,
				Predicted:  nullString(predicted, i),
				Confidence: confidenceValue,
			}
			switch {
			case !consensusMapped:
				node.transmitter.Status = TransmitterNotMapped
			case unknown:
				node.transmitter.Status = TransmitterUnknown
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(ntIDs)
	for i := range ntIDs {
		if i > 0 && ntIDs[i] == ntIDs[i-1] {
			stats.DuplicateRows++
		} else {
			stats.UniqueIDs++
		}
	}
	b.noteBuffered(b.memory.used)
	b.memory.release(ntMem)
	if consensusMapped {
		stats.UnknownRatio = newRatio(stats.UnknownRows, stats.Rows, mapping.Consensus)
		stats.UnknownDefinition = unknownDefinition
	}
	return nil
}

type sorters struct {
	endpoints, rawPairs, edges *extsort.Sorter
	share                      int64
}

func (s *sorters) closeAll() error {
	var errs []error
	for _, sorter := range []*extsort.Sorter{s.endpoints, s.rawPairs, s.edges} {
		if sorter != nil {
			if err := sorter.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (b *builder) newSorter(recordBytes int, dedupe bool, share int64) (*extsort.Sorter, error) {
	if share < int64(recordBytes)+4 {
		return nil, fmt.Errorf("%w: sort buffer share of %d bytes cannot hold one %d-byte record", ErrCapacity, share, recordBytes)
	}
	sorter, err := extsort.New(extsort.Config{
		RecordBytes: recordBytes,
		MemoryBytes: share,
		TempBytes:   b.limits.MaxTempBytes / 3,
		MaxRuns:     b.limits.MaxRunFiles,
		TempDir:     b.req.TempDir,
		Dedupe:      dedupe,
	})
	if err != nil {
		return nil, fmt.Errorf("connectome: create sorter: %w", err)
	}
	return sorter, nil
}

func wrapSortError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, extsort.ErrCapacity) {
		return fmt.Errorf("%w: %w", ErrCapacity, err)
	}
	return err
}

func (b *builder) scanWeights() (retErr error) {
	mapping := b.manifest.FieldMapping.Weights
	aggregated := b.req.EdgeView == EdgeViewAggregatedPairs
	share := b.limits.SortBufferBytes
	if share == 0 {
		share = (b.limits.MaxMemoryBytes - b.memory.used) / 3
	}
	if share <= 0 {
		return fmt.Errorf("%w: no memory left for sort buffers with %d of %d bytes reserved", ErrCapacity, b.memory.used, b.limits.MaxMemoryBytes)
	}
	if share > math.MaxInt64/3 {
		return fmt.Errorf("%w: sort buffer share %d overflows the three-pass reservation", ErrCapacity, share)
	}
	if err := b.memory.reserve(share*3, "sort buffers"); err != nil {
		return err
	}
	s := &sorters{share: share}
	defer func() {
		if err := s.closeAll(); err != nil && retErr == nil {
			retErr = fmt.Errorf("connectome: close sorters: %w", err)
		}
	}()
	var err error
	if s.endpoints, err = b.newSorter(endpointRecordBytes, true, share); err != nil {
		return err
	}
	if s.rawPairs, err = b.newSorter(rawPairRecordBytes, false, share); err != nil {
		return err
	}
	if s.edges, err = b.newSorter(edgeRecordBytes, false, share); err != nil {
		return err
	}

	raw := &b.raw
	raw.WeightSum = ValueSum{Valid: true, Basis: mapping.Value}
	b.annotated.WeightSum = ValueSum{Valid: true, Basis: mapping.Value}
	edgeRows := int64(0)
	absRow := int64(0)
	var endpointRecord [endpointRecordBytes]byte
	var pairRecord [rawPairRecordBytes]byte
	var edgeRecord [edgeRecordBytes]byte
	err = b.scan(RoleWeights, map[string]string{mapping.Source: "int64", mapping.Target: "int64", mapping.Value: "int64"}, func(record arrow.Record, batch int64) error {
		source, err := int64Column(record, mapping.Source)
		if err != nil {
			return err
		}
		target, err := int64Column(record, mapping.Target)
		if err != nil {
			return err
		}
		weight, err := int64Column(record, mapping.Value)
		if err != nil {
			return err
		}
		b.batchStarts = append(b.batchStarts, absRow)
		rows := int(record.NumRows())
		for i := 0; i < rows; i++ {
			if i%contextCheckStride == 0 {
				if err := b.check(); err != nil {
					return err
				}
			}
			raw.Rows++
			sourceValue, targetValue, weightValue := nullInt64(source, i), nullInt64(target, i), nullInt64(weight, i)
			countEndpoint(&raw.Source, sourceValue)
			countEndpoint(&raw.Target, targetValue)
			countValue(&raw.Weight, weightValue)
			if err := addToSum(&raw.WeightSum, weightValue); err != nil {
				return err
			}
			for _, endpoint := range []NullInt64{sourceValue, targetValue} {
				if !endpoint.Valid || endpoint.Value < 0 {
					continue
				}
				binary.BigEndian.PutUint64(endpointRecord[:], uint64(endpoint.Value))
				if err := s.endpoints.Add(b.ctx, endpointRecord[:]); err != nil {
					return wrapSortError(err)
				}
				if _, found := slices.BinarySearch(b.allIDs, uint64(endpoint.Value)); !found {
					raw.UnannotatedEndpointOccurrences++
				}
			}
			bothValid := sourceValue.Valid && targetValue.Valid && sourceValue.Value >= 0 && targetValue.Value >= 0
			if bothValid {
				raw.ValidPairRows++
				if sourceValue.Value == targetValue.Value {
					raw.SelfLoopRows++
				}
				binary.BigEndian.PutUint64(pairRecord[:8], uint64(sourceValue.Value))
				binary.BigEndian.PutUint64(pairRecord[8:], uint64(targetValue.Value))
				if err := s.rawPairs.Add(b.ctx, pairRecord[:]); err != nil {
					return wrapSortError(err)
				}
			}
			// One exclusion reason per row, in order of precedence.
			exclusions := &b.annotated.WeightExclusions
			switch {
			case !sourceValue.Valid || !targetValue.Valid:
				exclusions.NullEndpoint++
			case sourceValue.Value < 0 || targetValue.Value < 0:
				exclusions.InvalidID++
			default:
				sourceIndex, sourceSelected := slices.BinarySearch(b.nodeIDs, uint64(sourceValue.Value))
				targetIndex, targetSelected := slices.BinarySearch(b.nodeIDs, uint64(targetValue.Value))
				if sourceSelected && targetSelected {
					binary.BigEndian.PutUint64(edgeRecord[0:8], uint64(sourceIndex))
					binary.BigEndian.PutUint64(edgeRecord[8:16], uint64(targetIndex))
					binary.BigEndian.PutUint64(edgeRecord[16:24], uint64(absRow))
					edgeRecord[24] = 0
					if weightValue.Valid {
						edgeRecord[24] = 1
					}
					binary.BigEndian.PutUint64(edgeRecord[25:33], uint64(weightValue.Value))
					if err := s.edges.Add(b.ctx, edgeRecord[:]); err != nil {
						return wrapSortError(err)
					}
					if err := addToSum(&b.annotated.WeightSum, weightValue); err != nil {
						return err
					}
					edgeRows++
					break
				}
				_, sourceAnnotated := slices.BinarySearch(b.allIDs, uint64(sourceValue.Value))
				_, targetAnnotated := slices.BinarySearch(b.allIDs, uint64(targetValue.Value))
				if !sourceAnnotated || !targetAnnotated {
					exclusions.MissingAnnotation++
				} else {
					exclusions.PredicateFalse++
				}
			}
			absRow++
		}
		return nil
	})
	if err != nil {
		return err
	}
	base := b.memory.used - 3*share
	endpointStats, err := b.mergeEndpoints(s.endpoints)
	if err != nil {
		return err
	}
	pairStats, err := b.mergeRawPairs(s.rawPairs)
	if err != nil {
		return err
	}
	if err := s.endpoints.Close(); err != nil {
		return fmt.Errorf("connectome: close endpoint sorter: %w", err)
	}
	if err := s.rawPairs.Close(); err != nil {
		return fmt.Errorf("connectome: close pair sorter: %w", err)
	}
	s.endpoints, s.rawPairs = nil, nil
	b.memory.release(2 * share)
	edgeStats, edgeArrayBytes, err := b.mergeEdges(s.edges, edgeRows, aggregated)
	if err != nil {
		return err
	}
	if err := s.edges.Close(); err != nil {
		return fmt.Errorf("connectome: close edge sorter: %w", err)
	}
	s.edges = nil
	b.memory.release(share)
	// Upper bound of bytes actually held during the scan (all three sort
	// buffers) and during the edge merge (edge buffer plus final arrays).
	b.noteBuffered(base + endpointStats.PeakBufferedBytes + pairStats.PeakBufferedBytes + edgeStats.PeakBufferedBytes)
	b.noteBuffered(base + edgeStats.PeakBufferedBytes + edgeArrayBytes)
	b.raw.Name = "raw_segments"
	b.raw.DuplicateSemantics = b.manifest.DuplicateSemantics
	b.raw.Aggregation = AggregationNotPermitted
	return nil
}

func countEndpoint(stats *EndpointStats, value NullInt64) {
	switch {
	case !value.Valid:
		stats.NullRows++
	case value.Value < 0:
		stats.NegativeRows++
	case value.Value == 0:
		stats.ZeroRows++
	}
}

func countValue(stats *ValueStats, value NullInt64) {
	switch {
	case !value.Valid:
		stats.NullRows++
	case value.Value < 0:
		stats.NegativeRows++
	case value.Value == 0:
		stats.ZeroRows++
	}
}

func addToSum(sum *ValueSum, value NullInt64) error {
	if !value.Valid {
		sum.NullRows++
		return nil
	}
	if value.Value < 0 {
		sum.Valid = false
		return nil
	}
	if uint64(value.Value) > math.MaxUint64-sum.Value {
		return fmt.Errorf("%w: %s sum overflows uint64", ErrCapacity, sum.Basis)
	}
	sum.Value += uint64(value.Value)
	sum.Rows++
	return nil
}

func (b *builder) recordSort(stats extsort.Stats) {
	b.sort.Runs += stats.Runs
	b.sort.TempBytes += stats.TempBytes
}

func (b *builder) mergeEndpoints(sorter *extsort.Sorter) (extsort.Stats, error) {
	stats, err := sorter.Merge(b.ctx, func(record []byte) error {
		b.raw.UniqueEndpoints++
		if _, found := slices.BinarySearch(b.allIDs, binary.BigEndian.Uint64(record)); !found {
			b.raw.UnannotatedUniqueEndpoints++
		}
		return nil
	})
	b.recordSort(stats)
	return stats, wrapSortError(err)
}

func (b *builder) mergeRawPairs(sorter *extsort.Sorter) (extsort.Stats, error) {
	var previous [rawPairRecordBytes]byte
	havePrevious := false
	pairRows := uint64(0)
	stats, err := sorter.Merge(b.ctx, func(record []byte) error {
		if havePrevious && [rawPairRecordBytes]byte(record) == previous {
			pairRows++
			b.raw.DuplicateRows++
			if pairRows == 2 {
				b.raw.DuplicatePairs++
			}
			return nil
		}
		copy(previous[:], record)
		havePrevious = true
		pairRows = 1
		b.raw.UniquePairs++
		return nil
	})
	b.recordSort(stats)
	return stats, wrapSortError(err)
}

func (b *builder) mergeEdges(sorter *extsort.Sorter, edgeRows int64, aggregated bool) (extsort.Stats, int64, error) {
	wide := checkedIndexWidth(len(b.nodes))
	perEdge := edgeBytes(wide, aggregated)
	if edgeRows > math.MaxInt64/perEdge {
		return extsort.Stats{}, 0, fmt.Errorf("%w: edge storage size overflows", ErrCapacity)
	}
	edgeArrayBytes := edgeRows*perEdge + int64(len(b.nodes))
	if err := b.memory.reserve(edgeArrayBytes, "annotated edge arrays"); err != nil {
		return extsort.Stats{}, 0, err
	}
	nodeCount := uint64(len(b.nodes))
	b.edges = edgeStore{wide: wide}
	if wide {
		b.edges.src64 = make([]uint64, 0, edgeRows)
		b.edges.dst64 = make([]uint64, 0, edgeRows)
	} else {
		b.edges.src32 = make([]uint32, 0, edgeRows)
		b.edges.dst32 = make([]uint32, 0, edgeRows)
	}
	b.edges.absRow = make([]int64, 0, edgeRows)
	b.edges.weight = make([]int64, 0, edgeRows)
	b.edges.weightValid = make([]bool, 0, edgeRows)
	if aggregated {
		b.edges.rows = make([]uint32, 0, edgeRows)
	}
	touched := make([]bool, len(b.nodes))
	view := &b.annotated

	var previousSource, previousTarget uint64
	havePrevious := false
	pairRows := uint64(0)
	// Aggregation accumulator.
	var aggregateFirstRow int64
	var aggregateSum uint64
	var aggregateRows uint32
	emitAggregate := func() error {
		if !havePrevious {
			return nil
		}
		if aggregateSum > math.MaxInt64 {
			return fmt.Errorf("%w: aggregated weight of pair (%d,%d) exceeds int64", ErrCapacity, previousSource, previousTarget)
		}
		if aggregateRows > 1 {
			view.DuplicatePairs++
			view.DuplicateRows += uint64(aggregateRows - 1)
		}
		if previousSource == previousTarget {
			view.SelfLoops++
		}
		b.edges.append(previousSource, previousTarget, aggregateFirstRow, NullInt64{Valid: true, Value: int64(aggregateSum)}, aggregateRows)
		var encoded [edgeRecordBytes + 4]byte
		binary.BigEndian.PutUint64(encoded[0:8], previousSource)
		binary.BigEndian.PutUint64(encoded[8:16], previousTarget)
		binary.BigEndian.PutUint64(encoded[16:24], uint64(aggregateFirstRow))
		encoded[24] = 1
		binary.BigEndian.PutUint64(encoded[25:33], aggregateSum)
		binary.BigEndian.PutUint32(encoded[33:37], aggregateRows)
		b.edgeHash.Write(encoded[:])
		return nil
	}

	stats, err := sorter.Merge(b.ctx, func(record []byte) error {
		source := binary.BigEndian.Uint64(record[0:8])
		target := binary.BigEndian.Uint64(record[8:16])
		absRow := int64(binary.BigEndian.Uint64(record[16:24]))
		weight := NullInt64{Valid: record[24] == 1, Value: int64(binary.BigEndian.Uint64(record[25:33]))}
		// Sorted run files are private, but a corrupted run must fail, not panic.
		if source >= nodeCount || target >= nodeCount || absRow < 0 || uint64(absRow) >= b.raw.Rows {
			return fmt.Errorf("connectome: merged edge record is out of range (source %d, target %d, row %d)", source, target, absRow)
		}
		samePair := havePrevious && source == previousSource && target == previousTarget
		if aggregated {
			if !weight.Valid {
				return fmt.Errorf("connectome: aggregated pairs require non-null weights; absolute row %d of pair (%d,%d) is null", absRow, source, target)
			}
			if weight.Value < 0 {
				return fmt.Errorf("connectome: aggregated pairs require non-negative weights; absolute row %d has %d", absRow, weight.Value)
			}
			if !samePair {
				if err := emitAggregate(); err != nil {
					return err
				}
				previousSource, previousTarget, havePrevious = source, target, true
				aggregateFirstRow, aggregateSum, aggregateRows = absRow, 0, 0
				view.UniquePairs++
			}
			if uint64(weight.Value) > math.MaxUint64-aggregateSum || aggregateRows == math.MaxUint32 {
				return fmt.Errorf("%w: aggregated weight of pair (%d,%d) overflows", ErrCapacity, source, target)
			}
			aggregateSum += uint64(weight.Value)
			aggregateRows++
			touched[source], touched[target] = true, true
			return nil
		}
		if samePair {
			pairRows++
			view.DuplicateRows++
			if pairRows == 2 {
				view.DuplicatePairs++
			}
		} else {
			previousSource, previousTarget, havePrevious = source, target, true
			pairRows = 1
			view.UniquePairs++
		}
		if source == target {
			view.SelfLoops++
		}
		touched[source], touched[target] = true, true
		b.edges.append(source, target, absRow, weight, 1)
		b.edgeHash.Write(record)
		return nil
	})
	b.recordSort(stats)
	if err != nil {
		return stats, edgeArrayBytes, wrapSortError(err)
	}
	if aggregated {
		if err := emitAggregate(); err != nil {
			return stats, edgeArrayBytes, err
		}
	}
	for _, seen := range touched {
		if !seen {
			view.IsolatedNodes++
		}
	}
	view.Edges = uint64(b.edges.count())
	view.Name = "annotated_neurons"
	return stats, edgeArrayBytes, nil
}

func (b *builder) report() (GraphReport, error) {
	if err := b.check(); err != nil {
		return GraphReport{}, err
	}
	manifestHash, err := b.manifest.Hash()
	if err != nil {
		return GraphReport{}, err
	}
	canonical, err := b.manifest.Selection.Canonical()
	if err != nil {
		return GraphReport{}, err
	}
	predicateHash, err := b.manifest.Selection.Hash()
	if err != nil {
		return GraphReport{}, err
	}
	mapping := b.manifest.FieldMapping
	view := &b.annotated
	view.Transmitter = TransmitterSummary{Status: ColumnNotMapped, Measured: EvidenceNotAvailable}
	if mapping.Neurotransmitters.Consensus != "" {
		unknown := uint64(0)
		for i := range b.nodes {
			if b.nodes[i].transmitter.Status != TransmitterPredicted {
				unknown++
			}
		}
		view.Transmitter = TransmitterSummary{
			Status:            ColumnMapped,
			Predicted:         uint64(len(b.nodes)) - unknown,
			Unknown:           unknown,
			UnknownRatio:      newRatio(unknown, uint64(len(b.nodes)), mapping.Neurotransmitters.Consensus),
			UnknownDefinition: unknownDefinition,
			Measured:          EvidenceNotAvailable,
		}
	}
	view.Receptor = ReceptorSummary{Status: EvidenceNotDerived, LabelStatus: ColumnNotMapped}
	if mapping.Annotations.ReceptorType != "" {
		receptorNull := uint64(0)
		for i := range b.nodes {
			if !b.nodes[i].fields[8].Valid {
				receptorNull++
			}
		}
		view.Receptor.LabelStatus = ColumnMapped
		view.Receptor.LabelNullRatio = newRatio(receptorNull, uint64(len(b.nodes)), mapping.Annotations.ReceptorType)
	}

	nodeHash := sha256.New()
	nodeHash.Write([]byte(b.namespace))
	var encoded [8]byte
	for _, id := range b.nodeIDs {
		binary.BigEndian.PutUint64(encoded[:], id)
		nodeHash.Write(encoded[:])
	}
	b.sort.ReservedPeakBytes = b.memory.peak
	b.sort.BufferedPeakBytes = b.bufferedMax

	report := GraphReport{
		SchemaVersion:    ReportSchemaVersion,
		ConverterVersion: ConverterVersion,
		Dataset:          b.manifest.Dataset,
		Namespace:        b.namespace,
		SourceVersion:    b.manifest.SourceVersion,
		License:          b.manifest.License,
		AcquiredAt:       b.manifest.AcquiredAt,
		ManifestHash:     manifestHash,
		EdgeView:         b.req.EdgeView,
		Sources:          []SourceReport{*b.sources[RoleWeights], *b.sources[RoleAnnotations], *b.sources[RoleNeurotransmitters]},
		Predicate:        PredicateReport{Canonical: canonical, Hash: predicateHash, Label: b.manifest.Selection.Label},
		Identity:         b.manifest.Identity,
		Raw:              b.raw,
		Annotated:        b.annotated,
		Hashes: ResultHashes{
			NodeIndex: hex.EncodeToString(nodeHash.Sum(nil)),
			EdgeOrder: hex.EncodeToString(b.edgeHash.Sum(nil)),
		},
		Limits: b.limits,
		Sort:   b.sort,
		Limitations: []string{
			"weight_sum and edge weights are the source raw values under the official column name; they are not synapse or contact count claims",
			"the selection predicate is an explicit engineering rule recorded in the report; it does not establish biological completeness",
			"duplicate pair semantics come from the manifest declaration; in rows mode no rows were merged, and aggregation is refused without additive evidence",
			"transmitter fields are source predictions; no measured synaptic action, sign or receptor evidence is derived",
			"endpoint identity between weights, annotations and predictions is a declared assumption with the cited evidence, not a verified biological mapping",
			"memory accounting covers builder structures and sort buffers only, not process RSS; Arrow allocations are bounded separately by max_arrow_bytes",
		},
	}
	report.Hashes.Report, err = report.hash()
	if err != nil {
		return GraphReport{}, err
	}
	return report, nil
}

func hashFile(ctx context.Context, path string) (fingerprint, error) {
	var out fingerprint
	if err := ctx.Err(); err != nil {
		return out, fmt.Errorf("connectome: %w", err)
	}
	file, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return out, fmt.Errorf("connectome: open source: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return out, fmt.Errorf("connectome: stat source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return out, fmt.Errorf("connectome: source %q is not a regular file", path)
	}
	digest := sha256.New()
	buffer := make([]byte, hashChunkBytes)
	for {
		if err := ctx.Err(); err != nil {
			return out, fmt.Errorf("connectome: %w", err)
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			if int64(n) > math.MaxInt64-out.bytes {
				return out, fmt.Errorf("%w: source byte counter overflows", ErrCapacity)
			}
			out.bytes += int64(n)
			digest.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return out, fmt.Errorf("connectome: read source: %w", readErr)
		}
	}
	out.sha256 = hex.EncodeToString(digest.Sum(nil))
	return out, nil
}
