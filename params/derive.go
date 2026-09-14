package params

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/TimLai666/coimnet/connectome"
	"github.com/TimLai666/coimnet/feather"
	"github.com/TimLai666/coimnet/internal/extsort"
	"github.com/TimLai666/coimnet/internal/fileio"
	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
)

// ErrCapacity wraps every error caused by exceeding a configured limit.
var ErrCapacity = errors.New("params: capacity limit exceeded")

// ErrSourceChanged is returned when a release file no longer matches the
// SHA-256 the rules file declares.
var ErrSourceChanged = errors.New("params: source file does not match the rules fingerprint")

const (
	// SetSource names the derivation that produced a parameter set.
	SetSource = "derived_release/v1"

	// maxFeatherFooterBytes bounds the Arrow footer of every scanned source.
	maxFeatherFooterBytes = 16 << 20
	// hashChunkBytes is the fingerprint read buffer.
	hashChunkBytes = 1 << 20
	// contextCheckStride bounds how many rows a loop handles between context
	// checks.
	contextCheckStride = 4096
	// spillBufferBytes is the buffered I/O size of one intermediate file.
	spillBufferBytes = 1 << 16
	// maxROINames bounds the distinct primary_post values; the release stores
	// them as dictionary<int16>, so it cannot hold more than 32,767 anyway.
	maxROINames = 32767
	// maxPairEdges bounds how many graph edges one (source, target) pair may
	// have in rows mode before the derivation refuses to buffer the group.
	maxPairEdges = 1 << 20

	keyBytes          = 12
	tbarRecordBytes   = keyBytes + 1 + 4*len(Transmitters) // key, null flag, probabilities
	synRecordBytes    = keyBytes + 4 + 4                   // key, source index, target index
	pairRecordBytes   = 4 + 4 + 4*len(Transmitters)        // source, target, probabilities
	roiRecordBytes    = 4 + 2                              // node index, ROI id
	tbarKeyBytes      = keyBytes + 1 + 4*len(Transmitters) // spill: key, status, probabilities
	pairAggregateSize = 4 + 4 + 4 + 8*len(Transmitters)    // spill: source, target, count, sums

	keyStatusUsable     byte = 0
	keyStatusAmbiguous  byte = 1
	keyStatusNullProbab byte = 2
)

// Limits bounds one derivation. Every numeric field must be positive and
// TempDir must be an existing directory; every temporary file is removed
// before Derive returns.
type Limits struct {
	MaxMemoryBytes int64  `json:"max_memory_bytes"`
	MaxTempBytes   int64  `json:"max_temp_bytes"`
	MaxRunFiles    int    `json:"max_run_files"`
	MaxArrowBytes  int64  `json:"max_arrow_bytes"`
	MaxRows        int64  `json:"max_rows"`
	TempDir        string `json:"-"`
}

func (l Limits) validate() error {
	if l.MaxMemoryBytes <= 0 || l.MaxTempBytes <= 0 || l.MaxArrowBytes <= 0 || l.MaxRows <= 0 {
		return fmt.Errorf("params: every byte and row limit must be positive: %+v", l)
	}
	if l.MaxRunFiles <= 0 {
		return fmt.Errorf("params: max run files must be positive, got %d", l.MaxRunFiles)
	}
	if l.TempDir == "" {
		return errors.New("params: temporary directory must be set")
	}
	info, err := os.Stat(l.TempDir)
	if err != nil {
		return fmt.Errorf("params: temporary directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("params: temporary path %q is not a directory", l.TempDir)
	}
	return nil
}

// budget accounts the bytes this package reserves before it allocates them.
type budget struct {
	limit, used, peak int64
}

func (b *budget) reserve(n int64, what string) error {
	if n < 0 {
		return fmt.Errorf("params: negative reservation for %s", what)
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

// deriver holds the state of one derivation.
type deriver struct {
	ctx      context.Context
	graph    *connectome.Graph
	rules    Rules
	rulesDir string
	limits   Limits
	memory   budget
	report   Report

	bodies    []uint64 // selected body IDs in node index order
	preTotal  []int64
	postTotal []int64

	paths map[string]string // role to absolute path
	sizes map[string]int64

	roiNames []string
	sorters  []*extsort.Sorter
	spills   []*spillFile
}

// Derive runs the eight step pipeline of ticket 13 and returns the parameter
// set with its report. Every source fingerprint is verified before any sort
// starts, every per-synapse stage is a bounded external sort, and any error or
// cancellation returns no result and leaves no temporary file behind.
func Derive(ctx context.Context, g *connectome.Graph, rules Rules, rulesDir string, limits Limits) (set *Set, report Report, retErr error) {
	started := time.Now()
	if ctx == nil {
		return nil, Report{}, errors.New("params: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, Report{}, fmt.Errorf("params: %w", err)
	}
	if g == nil || g.NodeCount() == 0 {
		return nil, Report{}, errors.New("params: nil or empty graph")
	}
	if g.NodeCount() > math.MaxUint32 {
		return nil, Report{}, fmt.Errorf("params: %d nodes exceed the 32 bit index of the sort records", g.NodeCount())
	}
	if err := rules.Validate(); err != nil {
		return nil, Report{}, err
	}
	if rulesDir == "" {
		return nil, Report{}, errors.New("params: rules directory must be set")
	}
	if err := limits.validate(); err != nil {
		return nil, Report{}, err
	}

	d := &deriver{
		ctx:      ctx,
		graph:    g,
		rules:    rules,
		rulesDir: rulesDir,
		limits:   limits,
		memory:   budget{limit: limits.MaxMemoryBytes},
		paths:    map[string]string{},
		sizes:    map[string]int64{},
	}
	defer func() {
		if err := d.cleanup(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	if err := d.verifySources(); err != nil {
		return nil, Report{}, err
	}
	if err := d.loadMeta(); err != nil {
		return nil, Report{}, err
	}
	if err := d.collectNodes(); err != nil {
		return nil, Report{}, err
	}
	if err := d.scanBodyStats(); err != nil {
		return nil, Report{}, err
	}
	if err := d.openSorters(); err != nil {
		return nil, Report{}, err
	}
	keys, err := d.sortTbar()
	if err != nil {
		return nil, Report{}, err
	}
	pairs, roi, err := d.sortSynPartners(keys)
	if err != nil {
		return nil, Report{}, err
	}
	primary, err := d.reduceROI(roi)
	if err != nil {
		return nil, Report{}, err
	}
	aggregates, err := d.aggregatePairs(pairs)
	if err != nil {
		return nil, Report{}, err
	}
	set, err = d.alignEdges(aggregates, primary)
	if err != nil {
		return nil, Report{}, err
	}
	d.finishReport(started)
	set.Report = d.report
	return set, d.report, nil
}

// cleanup closes every sorter and removes every intermediate file.
func (d *deriver) cleanup() error {
	var errs []error
	for _, sorter := range d.sorters {
		if sorter != nil {
			if err := sorter.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	d.sorters = nil
	for _, spill := range d.spills {
		if spill != nil {
			if err := spill.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	d.spills = nil
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("params: clean up temporary files: %w", errors.Join(errs...))
}

func (d *deriver) check() error {
	if err := d.ctx.Err(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	return nil
}

// verifySources checks every declared fingerprint with a bounded read before
// any sort starts.
func (d *deriver) verifySources() error {
	for _, source := range d.rules.Sources.refs() {
		if err := d.check(); err != nil {
			return err
		}
		path := filepath.Join(d.rulesDir, filepath.FromSlash(source.Ref.Path))
		size, sum, err := hashFile(d.ctx, path)
		if err != nil {
			return err
		}
		if !strings.EqualFold(sum, source.Ref.SHA256) {
			return fmt.Errorf("%w: %s %s has sha256 %s, the rules declare %s", ErrSourceChanged, source.Role, path, sum, source.Ref.SHA256)
		}
		d.paths[source.Role] = path
		d.sizes[source.Role] = size
		format := "feather_v2"
		if source.Role == "neuprint_meta" {
			format = "csv"
		}
		d.report.Sources = append(d.report.Sources, SourceFingerprint{
			Role:   source.Role,
			Path:   source.Ref.Path,
			Bytes:  size,
			SHA256: sum,
			Format: format,
		})
	}
	return nil
}

func (d *deriver) sourceReport(role string) *SourceFingerprint {
	for i := range d.report.Sources {
		if d.report.Sources[i].Role == role {
			return &d.report.Sources[i]
		}
	}
	return nil
}

func (d *deriver) loadMeta() error {
	summary, err := readMeta(d.ctx, d.paths["neuprint_meta"])
	if err != nil {
		return err
	}
	d.report.Meta = summary
	if source := d.sourceReport("neuprint_meta"); source != nil {
		source.Rows = 1
	}
	return nil
}

// collectNodes turns the graph's neuron IDs into the ascending body list the
// scans binary search.
func (d *deriver) collectNodes() error {
	nodes := int64(d.graph.NodeCount())
	if err := d.memory.reserve(nodes*8*3, "node identity and count arrays"); err != nil {
		return err
	}
	namespace := d.graph.Namespace()
	ids := d.graph.NeuronIDs()
	d.bodies = make([]uint64, len(ids))
	for i, id := range ids {
		external, err := connectome.ParseExternalID(namespace, id.ExternalID)
		if err != nil {
			return fmt.Errorf("params: node %d: %w", i, err)
		}
		if i > 0 && external.Value <= d.bodies[i-1] {
			return fmt.Errorf("params: node %d body %d does not follow node %d body %d in ascending order", i, external.Value, i-1, d.bodies[i-1])
		}
		d.bodies[i] = external.Value
	}
	d.preTotal = make([]int64, len(ids))
	d.postTotal = make([]int64, len(ids))
	return nil
}

// nodeIndex finds the node index of a body ID.
func (d *deriver) nodeIndex(body int64) (uint32, bool) {
	if body < 0 {
		return 0, false
	}
	index, found := slices.BinarySearch(d.bodies, uint64(body))
	if !found {
		return 0, false
	}
	return uint32(index), true
}

// scan reads one Feather source once with the configured limits.
func (d *deriver) scan(role string, want map[string]string, visit func(arrow.Record) error) error {
	path := d.paths[role]
	options := feather.Options{
		MaxFileBytes:   d.sizes[role],
		MaxFooterBytes: maxFeatherFooterBytes,
		MaxArrowBytes:  d.limits.MaxArrowBytes,
		MaxRows:        d.limits.MaxRows,
	}
	report, err := feather.Scan(d.ctx, path, options, visit)
	if err != nil {
		if ctxErr := d.ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return fmt.Errorf("params: %w", ctxErr)
		}
		return fmt.Errorf("params: scan %s source: %w", role, err)
	}
	if !report.Complete {
		return fmt.Errorf("params: scan of the %s source did not complete", role)
	}
	if err := checkFields(role, report.Fields, want); err != nil {
		return err
	}
	if source := d.sourceReport(role); source != nil {
		source.Rows = report.Rows
		source.Batches = report.RecordBatches
	}
	return nil
}

// scanBodyStats records the pre and post counts of every selected body. The
// first row of a body wins; later rows are counted as duplicates.
func (d *deriver) scanBodyStats() error {
	if err := d.memory.reserve(int64(len(d.bodies)), "body-stats presence flags"); err != nil {
		return err
	}
	seen := make([]bool, len(d.bodies))
	summary := &d.report.BodyStats
	err := d.scan("body_stats", map[string]string{"body": "int64", "pre": "int32", "post": "int32"}, func(record arrow.Record) error {
		body, err := int64Column(record, "body")
		if err != nil {
			return err
		}
		pre, err := int32Column(record, "pre")
		if err != nil {
			return err
		}
		post, err := int32Column(record, "post")
		if err != nil {
			return err
		}
		rows := int(record.NumRows())
		for i := 0; i < rows; i++ {
			if i%contextCheckStride == 0 {
				if err := d.check(); err != nil {
					return err
				}
			}
			summary.Rows++
			if body.IsNull(i) {
				summary.NullBodyRows++
				continue
			}
			value := body.Value(i)
			if value < 0 {
				summary.NegativeBodies++
				continue
			}
			index, selected := d.nodeIndex(value)
			if !selected {
				summary.RowsNotSelected++
				continue
			}
			if seen[index] {
				summary.DuplicateRows++
				continue
			}
			seen[index] = true
			summary.RowsSelected++
			if pre.IsNull(i) || post.IsNull(i) {
				summary.NullCountRows++
			}
			if !pre.IsNull(i) {
				d.preTotal[index] = int64(pre.Value(i))
			}
			if !post.IsNull(i) {
				d.postTotal[index] = int64(post.Value(i))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, present := range seen {
		if !present {
			summary.BodiesMissing++
		}
	}
	summary.Coverage = newRatio(summary.RowsSelected, uint64(len(d.bodies)), "selected bodies with a body-stats row / selected bodies")
	return nil
}

// openSorters reserves the output arrays whose size is already known, then
// creates the four bounded sorters that share the memory limit the way
// connectome's builder shares it across its three passes. The sorters take
// half of what is left, so the ROI dictionary and the section buffers still
// fit inside the same limit.
func (d *deriver) openSorters() error {
	edges := int64(d.graph.EdgeCount())
	nodes := int64(len(d.bodies))
	if err := d.memory.reserve(edges*(8+1+4+1+4), "parameter arrays"); err != nil {
		return err
	}
	if err := d.memory.reserve(edges*8, "weight quantile copy"); err != nil {
		return err
	}
	if err := d.memory.reserve(nodes*20, "primary ROI reduction"); err != nil {
		return err
	}
	share := (d.limits.MaxMemoryBytes - d.memory.used) / 8
	if share <= 0 {
		return fmt.Errorf("%w: no memory left for sort buffers with %d of %d bytes reserved", ErrCapacity, d.memory.used, d.limits.MaxMemoryBytes)
	}
	if err := d.memory.reserve(share*4, "sort buffers"); err != nil {
		return err
	}
	// Four sorters and two intermediate files share the temporary budget.
	tempShare := d.limits.MaxTempBytes / 6
	if tempShare <= 0 {
		return fmt.Errorf("%w: temporary budget %d cannot be split across four sorts and two intermediate files", ErrCapacity, d.limits.MaxTempBytes)
	}
	for _, spec := range []struct {
		name  string
		bytes int
	}{
		{"tbar_keys", tbarRecordBytes},
		{"syn_partners", synRecordBytes},
		{"pair_probabilities", pairRecordBytes},
		{"primary_roi", roiRecordBytes},
	} {
		if share < int64(spec.bytes)+4 {
			return fmt.Errorf("%w: sort buffer share of %d bytes cannot hold one %d-byte %s record", ErrCapacity, share, spec.bytes, spec.name)
		}
		sorter, err := extsort.New(extsort.Config{
			RecordBytes: spec.bytes,
			MemoryBytes: share,
			TempBytes:   tempShare,
			MaxRuns:     d.limits.MaxRunFiles,
			TempDir:     d.limits.TempDir,
		})
		if err != nil {
			return fmt.Errorf("params: create the %s sorter: %w", spec.name, err)
		}
		d.sorters = append(d.sorters, sorter)
	}
	return nil
}

func (d *deriver) sorter(index int) *extsort.Sorter { return d.sorters[index] }

func wrapSortError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, extsort.ErrCapacity) {
		return fmt.Errorf("%w: %w", ErrCapacity, err)
	}
	return err
}

func (d *deriver) recordSort(name string, recordBytes int, stats extsort.Stats) {
	d.report.Sorts = append(d.report.Sorts, SortStats{
		Name:              name,
		RecordBytes:       recordBytes,
		Records:           stats.Records,
		Emitted:           stats.Emitted,
		Runs:              stats.Runs,
		TempBytes:         stats.TempBytes,
		PeakMemoryBytes:   stats.PeakMemoryBytes,
		PeakBufferedBytes: stats.PeakBufferedBytes,
	})
}

// putKey writes the packed (x, y, z) voxel key. Each int32 is XORed with the
// sign bit and written big-endian, so byte order equals numeric order and the
// sorted stream can be merge joined directly.
func putKey(dst []byte, x, y, z int32) {
	binary.BigEndian.PutUint32(dst[0:4], uint32(x)^0x80000000)
	binary.BigEndian.PutUint32(dst[4:8], uint32(y)^0x80000000)
	binary.BigEndian.PutUint32(dst[8:12], uint32(z)^0x80000000)
}

// sortTbar streams the T-bar source into the key sorter and materializes the
// merged, grouped keys into one sorted intermediate file.
func (d *deriver) sortTbar() (*spillFile, error) {
	sorter := d.sorter(0)
	summary := &d.report.Tbar
	want := map[string]string{"x": "int32", "y": "int32", "z": "int32", "body": "int64"}
	for _, name := range Transmitters {
		want["nt_"+name+"_prob"] = "float32"
	}
	var record [tbarRecordBytes]byte
	err := d.scan("tbar", want, func(batch arrow.Record) error {
		x, err := int32Column(batch, "x")
		if err != nil {
			return err
		}
		y, err := int32Column(batch, "y")
		if err != nil {
			return err
		}
		z, err := int32Column(batch, "z")
		if err != nil {
			return err
		}
		body, err := int64Column(batch, "body")
		if err != nil {
			return err
		}
		var probs [len(Transmitters)]*array.Float32
		for p, name := range Transmitters {
			column, err := float32Column(batch, "nt_"+name+"_prob")
			if err != nil {
				return err
			}
			probs[p] = column
		}
		rows := int(batch.NumRows())
		for i := 0; i < rows; i++ {
			if i%contextCheckStride == 0 {
				if err := d.check(); err != nil {
					return err
				}
			}
			summary.Rows++
			if x.IsNull(i) || y.IsNull(i) || z.IsNull(i) {
				summary.RowsSkippedNullKey++
				continue
			}
			if body.IsNull(i) {
				summary.RowsSkippedNullBody++
				continue
			}
			putKey(record[:keyBytes], x.Value(i), y.Value(i), z.Value(i))
			anyNull := false
			for p := range Transmitters {
				value := float32(0)
				if probs[p].IsNull(i) {
					anyNull = true
				} else {
					value = probs[p].Value(i)
				}
				binary.BigEndian.PutUint32(record[keyBytes+1+4*p:], math.Float32bits(value))
			}
			record[keyBytes] = 0
			if anyNull {
				record[keyBytes] = 1
				summary.RowsWithNullProbability++
			}
			if err := sorter.Add(d.ctx, record[:]); err != nil {
				return wrapSortError(err)
			}
			summary.RowsSorted++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	spill, err := d.newSpill("tbar_keys", tbarKeyBytes)
	if err != nil {
		return nil, err
	}
	var group [tbarKeyBytes]byte
	groupRows := uint64(0)
	groupNull := false
	have := false
	flush := func() error {
		if !have {
			return nil
		}
		summary.Keys++
		switch {
		case groupRows > 1:
			group[keyBytes] = keyStatusAmbiguous
			summary.AmbiguousKeys++
			summary.RowsInAmbiguousKeys += groupRows
		case groupNull:
			group[keyBytes] = keyStatusNullProbab
			summary.KeysWithNullProbability++
		default:
			group[keyBytes] = keyStatusUsable
			summary.UsableKeys++
		}
		return spill.write(group[:])
	}
	stats, err := sorter.Merge(d.ctx, func(item []byte) error {
		if have && string(item[:keyBytes]) == string(group[:keyBytes]) {
			groupRows++
			groupNull = groupNull || item[keyBytes] == 1
			return nil
		}
		if err := flush(); err != nil {
			return err
		}
		copy(group[:], item)
		have = true
		groupRows = 1
		groupNull = item[keyBytes] == 1
		return nil
	})
	d.recordSort("tbar_keys", tbarRecordBytes, stats)
	if err != nil {
		return nil, wrapSortError(err)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if err := spill.finish(); err != nil {
		return nil, err
	}
	summary.UsableRatio = newRatio(summary.UsableKeys, summary.Keys, "T-bar keys with exactly one row and no null probability / distinct T-bar keys")
	return spill, nil
}

// sortSynPartners streams the synapse source into the synapse and ROI sorters,
// then merge joins the sorted synapses against the T-bar keys and feeds the
// pair sorter.
func (d *deriver) sortSynPartners(keys *spillFile) (*extsort.Sorter, *extsort.Sorter, error) {
	synSorter := d.sorter(1)
	pairSorter := d.sorter(2)
	roiSorter := d.sorter(3)
	summary := &d.report.SynPartners
	roiSummary := &d.report.PrimaryROI
	roiSummary.Status = StatusMeasured
	roiSummary.Basis = "mode of primary_post over synapse rows whose body_post is a selected neuron"

	roiIDs := map[string]uint16{}
	var roiNames []string
	var synRecord [synRecordBytes]byte
	var roiRecord [roiRecordBytes]byte
	err := d.scan("syn_partners", map[string]string{
		"x_pre": "int32", "y_pre": "int32", "z_pre": "int32",
		"body_pre": "int64", "body_post": "int64", "primary_post": "string",
	}, func(batch arrow.Record) error {
		x, err := int32Column(batch, "x_pre")
		if err != nil {
			return err
		}
		y, err := int32Column(batch, "y_pre")
		if err != nil {
			return err
		}
		z, err := int32Column(batch, "z_pre")
		if err != nil {
			return err
		}
		pre, err := int64Column(batch, "body_pre")
		if err != nil {
			return err
		}
		post, err := int64Column(batch, "body_post")
		if err != nil {
			return err
		}
		roi, err := stringColumn(batch, "primary_post")
		if err != nil {
			return err
		}
		rows := int(batch.NumRows())
		for i := 0; i < rows; i++ {
			if i%contextCheckStride == 0 {
				if err := d.check(); err != nil {
					return err
				}
			}
			summary.Rows++

			// The ROI mode only needs the postsynaptic body, so it is
			// collected before the synapse row filters.
			var postIndex uint32
			postSelected := false
			if !post.IsNull(i) {
				postIndex, postSelected = d.nodeIndex(post.Value(i))
			}
			if postSelected {
				if roi.IsNull(i) {
					roiSummary.NullROIRows++
				} else {
					name := roi.Value(i)
					id, known := roiIDs[name]
					if !known {
						if len(roiNames) >= maxROINames {
							return fmt.Errorf("%w: more than %d distinct primary_post values", ErrCapacity, maxROINames)
						}
						id = uint16(len(roiNames))
						roiNames = append(roiNames, strings.Clone(name))
						roiIDs[roiNames[id]] = id
					}
					binary.BigEndian.PutUint32(roiRecord[0:4], postIndex)
					binary.BigEndian.PutUint16(roiRecord[4:6], id)
					if err := roiSorter.Add(d.ctx, roiRecord[:]); err != nil {
						return wrapSortError(err)
					}
					roiSummary.Rows++
				}
			}

			if pre.IsNull(i) || post.IsNull(i) {
				summary.RowsNullBody++
				continue
			}
			preIndex, preSelected := d.nodeIndex(pre.Value(i))
			if !preSelected || !postSelected {
				summary.RowsBodyNotSelected++
				continue
			}
			if x.IsNull(i) || y.IsNull(i) || z.IsNull(i) {
				summary.RowsNullCoordinate++
				continue
			}
			putKey(synRecord[:keyBytes], x.Value(i), y.Value(i), z.Value(i))
			binary.BigEndian.PutUint32(synRecord[keyBytes:keyBytes+4], preIndex)
			binary.BigEndian.PutUint32(synRecord[keyBytes+4:], postIndex)
			if err := synSorter.Add(d.ctx, synRecord[:]); err != nil {
				return wrapSortError(err)
			}
			summary.RowsSorted++
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if err := d.memory.reserve(int64(len(roiNames))*64, "primary ROI names"); err != nil {
		return nil, nil, err
	}
	roiSummary.Names = len(roiNames)
	d.roiNames = roiNames

	reader, err := keys.reader()
	if err != nil {
		return nil, nil, err
	}
	cursor := &keyCursor{reader: reader}
	if err := cursor.next(); err != nil {
		return nil, nil, err
	}
	var pairRecord [pairRecordBytes]byte
	stats, mergeErr := synSorter.Merge(d.ctx, func(item []byte) error {
		for cursor.valid && string(cursor.record[:keyBytes]) < string(item[:keyBytes]) {
			if !cursor.used {
				d.report.Tbar.KeysWithoutSynapse++
			}
			if err := cursor.next(); err != nil {
				return err
			}
		}
		if !cursor.valid || string(cursor.record[:keyBytes]) != string(item[:keyBytes]) {
			summary.UnmatchedSynapses++
			return nil
		}
		cursor.used = true
		switch cursor.record[keyBytes] {
		case keyStatusAmbiguous:
			summary.SynapsesOnAmbiguousKey++
			return nil
		case keyStatusNullProbab:
			summary.SynapsesOnNullProbabilityKey++
			return nil
		}
		copy(pairRecord[0:4], item[keyBytes:keyBytes+4])
		copy(pairRecord[4:8], item[keyBytes+4:keyBytes+8])
		copy(pairRecord[8:], cursor.record[keyBytes+1:])
		if err := pairSorter.Add(d.ctx, pairRecord[:]); err != nil {
			return wrapSortError(err)
		}
		summary.MatchedSynapses++
		return nil
	})
	d.recordSort("syn_partners", synRecordBytes, stats)
	if mergeErr != nil {
		return nil, nil, wrapSortError(mergeErr)
	}
	for cursor.valid {
		if !cursor.used {
			d.report.Tbar.KeysWithoutSynapse++
		}
		if err := cursor.next(); err != nil {
			return nil, nil, err
		}
	}
	summary.MatchedRatio = newRatio(summary.MatchedSynapses, summary.RowsSorted, "synapses matched to a usable T-bar key / synapse rows between two selected neurons")
	return pairSorter, roiSorter, nil
}

// keyCursor reads the sorted T-bar key file one record at a time.
type keyCursor struct {
	reader *spillReader
	record [tbarKeyBytes]byte
	valid  bool
	used   bool
}

func (c *keyCursor) next() error {
	ok, err := c.reader.read(c.record[:])
	if err != nil {
		return err
	}
	c.valid = ok
	c.used = false
	return nil
}

// reduceROI turns the sorted (node, ROI) records into the per-node mode. Ties
// are broken by the ROI name so the result does not depend on the row order.
func (d *deriver) reduceROI(sorter *extsort.Sorter) ([]string, error) {
	primary := make([]string, len(d.bodies))
	bestCount := make([]uint32, len(d.bodies))
	var currentNode uint32
	var currentID uint16
	currentCount := uint32(0)
	have := false
	commit := func() {
		if !have {
			return
		}
		name := d.roiNames[currentID]
		if currentCount > bestCount[currentNode] || (currentCount == bestCount[currentNode] && primary[currentNode] != "" && name < primary[currentNode]) {
			bestCount[currentNode] = currentCount
			primary[currentNode] = name
		}
	}
	stats, err := sorter.Merge(d.ctx, func(item []byte) error {
		node := binary.BigEndian.Uint32(item[0:4])
		id := binary.BigEndian.Uint16(item[4:6])
		if node >= uint32(len(primary)) || int(id) >= len(d.roiNames) {
			return fmt.Errorf("params: merged ROI record is out of range (node %d, roi %d)", node, id)
		}
		if have && node == currentNode && id == currentID {
			currentCount++
			return nil
		}
		commit()
		currentNode, currentID, currentCount, have = node, id, 1, true
		return nil
	})
	d.recordSort("primary_roi", roiRecordBytes, stats)
	if err != nil {
		return nil, wrapSortError(err)
	}
	commit()
	summary := &d.report.PrimaryROI
	for _, name := range primary {
		if name == "" {
			summary.NodesWithoutROI++
		} else {
			summary.NodesWithROI++
		}
	}
	return primary, nil
}

// aggregatePairs sums the matched probabilities per (source, target) pair and
// writes the sorted aggregates to one intermediate file.
func (d *deriver) aggregatePairs(sorter *extsort.Sorter) (*spillFile, error) {
	spill, err := d.newSpill("pair_aggregates", pairAggregateSize)
	if err != nil {
		return nil, err
	}
	summary := &d.report.Pairs
	var aggregate [pairAggregateSize]byte
	var sums [len(Transmitters)]float64
	var count uint32
	have := false
	flush := func() error {
		if !have {
			return nil
		}
		summary.Pairs++
		binary.BigEndian.PutUint32(aggregate[8:12], count)
		for p := range sums {
			binary.BigEndian.PutUint64(aggregate[12+8*p:], math.Float64bits(sums[p]))
		}
		return spill.write(aggregate[:])
	}
	stats, mergeErr := sorter.Merge(d.ctx, func(item []byte) error {
		if !have || string(item[0:8]) != string(aggregate[0:8]) {
			if err := flush(); err != nil {
				return err
			}
			copy(aggregate[0:8], item[0:8])
			sums = [len(Transmitters)]float64{}
			count = 0
			have = true
		}
		if count == math.MaxUint32 {
			return fmt.Errorf("%w: a pair has more than %d matched synapses", ErrCapacity, uint32(math.MaxUint32))
		}
		count++
		for p := range sums {
			sums[p] += float64(math.Float32frombits(binary.BigEndian.Uint32(item[8+4*p:])))
		}
		return nil
	})
	d.recordSort("pair_probabilities", pairRecordBytes, stats)
	if mergeErr != nil {
		return nil, wrapSortError(mergeErr)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if err := spill.finish(); err != nil {
		return nil, err
	}
	return spill, nil
}

// pairAggregate is one decoded aggregate record.
type pairAggregate struct {
	source, target uint32
	count          uint32
	sums           [len(Transmitters)]float64
}

// pairCursor reads the sorted pair aggregates one record at a time.
type pairCursor struct {
	reader  *spillReader
	buffer  [pairAggregateSize]byte
	current pairAggregate
	valid   bool
}

func (c *pairCursor) next() error {
	ok, err := c.reader.read(c.buffer[:])
	if err != nil {
		return err
	}
	c.valid = ok
	if !ok {
		return nil
	}
	c.current.source = binary.BigEndian.Uint32(c.buffer[0:4])
	c.current.target = binary.BigEndian.Uint32(c.buffer[4:8])
	c.current.count = binary.BigEndian.Uint32(c.buffer[8:12])
	for p := range c.current.sums {
		c.current.sums[p] = math.Float64frombits(binary.BigEndian.Uint64(c.buffer[12+8*p:]))
	}
	return nil
}

func (c *pairCursor) before(source, target uint32) bool {
	return c.current.source < source || (c.current.source == source && c.current.target < target)
}

// alignEdges walks the canonical edge stream and the sorted pair aggregates
// together, producing the parameter set arrays.
func (d *deriver) alignEdges(aggregates *spillFile, primary []string) (*Set, error) {
	edges := int64(d.graph.EdgeCount())
	set := &Set{
		Source:    SetSource,
		RulesHash: d.rules.Hash(),
		GraphHashes: GraphHashes{
			NodeIndex: d.graph.Report().Hashes.NodeIndex,
			EdgeOrder: d.graph.Report().Hashes.EdgeOrder,
		},
		EdgeWeight:          make([]float64, edges),
		EdgeSign:            make([]int8, edges),
		EdgeSignConfidence:  make([]float32, edges),
		EdgeTransmitter:     make([]uint8, edges),
		EdgeMatchedSynapses: make([]uint32, edges),
		NodePrimaryROI:      primary,
		NodePreTotal:        d.preTotal,
		NodePostTotal:       d.postTotal,
	}
	for i := range set.EdgeTransmitter {
		set.EdgeTransmitter[i] = NoTransmitter
	}

	reader, err := aggregates.reader()
	if err != nil {
		return nil, err
	}
	cursor := &pairCursor{reader: reader}
	if err := cursor.next(); err != nil {
		return nil, err
	}

	summary := &d.report.Edges
	summary.Edges = uint64(edges)
	summary.TransmitterEdges = map[string]uint64{}
	for _, name := range Transmitters {
		summary.TransmitterEdges[name] = 0
	}
	summary.NormalizerBasis = d.normalizerBasis()

	var groupSource, groupTarget uint32
	groupStart := int64(0)
	var groupRaw []int64
	groupHave := false
	matchedTotal := uint64(0)
	rawTotal := uint64(0)

	finish := func() error {
		if !groupHave {
			return nil
		}
		var aggregate *pairAggregate
		if cursor.valid && cursor.current.source == groupSource && cursor.current.target == groupTarget {
			aggregate = &cursor.current
		}
		if err := d.finishPair(set, groupStart, groupRaw, groupSource, groupTarget, aggregate, &matchedTotal, &rawTotal); err != nil {
			return err
		}
		if aggregate != nil {
			return cursor.next()
		}
		return nil
	}

	position := int64(0)
	streamErr := d.graph.StreamAnnotatedEdges(d.ctx, func(edge connectome.EdgeRecord) error {
		if position >= edges {
			return fmt.Errorf("params: the edge stream produced more than the declared %d edges", edges)
		}
		source, target := uint32(edge.Source), uint32(edge.Target)
		if !groupHave || source != groupSource || target != groupTarget {
			if err := finish(); err != nil {
				return err
			}
			// Advance past every aggregated pair the graph does not have.
			for cursor.valid && cursor.before(source, target) {
				d.report.Pairs.NotInGraph++
				if err := cursor.next(); err != nil {
					return err
				}
			}
			groupSource, groupTarget, groupStart, groupHave = source, target, position, true
			groupRaw = groupRaw[:0]
		}
		if len(groupRaw) >= maxPairEdges {
			return fmt.Errorf("%w: pair (%d,%d) has more than %d edges", ErrCapacity, source, target, maxPairEdges)
		}
		if cap(groupRaw) == len(groupRaw) {
			if err := d.memory.reserve(8*int64(max(len(groupRaw), 1)), "duplicate pair buffer"); err != nil {
				return err
			}
		}
		raw := int64(0)
		if edge.Weight.Valid {
			raw = edge.Weight.Value
		} else {
			summary.NullRawWeightEdges++
			raw = -1
		}
		groupRaw = append(groupRaw, raw)
		position++
		return nil
	})
	if streamErr != nil {
		return nil, streamErr
	}
	if err := finish(); err != nil {
		return nil, err
	}
	for cursor.valid {
		d.report.Pairs.NotInGraph++
		if err := cursor.next(); err != nil {
			return nil, err
		}
	}
	if position != edges {
		return nil, fmt.Errorf("params: the edge stream produced %d of %d edges", position, edges)
	}
	d.report.Pairs.InGraph = d.report.Pairs.Pairs - d.report.Pairs.NotInGraph
	summary.MatchedFraction = newRatio(matchedTotal, rawTotal, "matched synapses of matched pairs / raw release weight of the same pairs")
	summary.UnknownRatio = newRatio(summary.Sign.Unknown, summary.Edges, "edges with an unknown sign / edges")

	weights := make([]float64, len(set.EdgeWeight))
	copy(weights, set.EdgeWeight)
	summary.WeightQuantiles = quantiles(weights)
	return set, nil
}

func (d *deriver) normalizerBasis() string {
	switch d.rules.Strength.Normalizer {
	case NormalizerPostTotal:
		return "gain * raw_weight / body-stats post of the target neuron"
	case NormalizerPreTotal:
		return "gain * raw_weight / body-stats pre of the source neuron"
	default:
		return "gain * raw_weight"
	}
}

// finishPair applies the sign and strength rules to one (source, target) group
// of the canonical edge stream.
func (d *deriver) finishPair(set *Set, start int64, raws []int64, source, target uint32, aggregate *pairAggregate, matchedTotal, rawTotal *uint64) error {
	summary := &d.report.Edges
	rules := d.rules

	// Strength first: it depends only on this edge's raw weight and the
	// normalizer of its endpoints.
	normalizer := 1.0
	switch rules.Strength.Normalizer {
	case NormalizerPostTotal:
		normalizer = float64(d.postTotal[target])
	case NormalizerPreTotal:
		normalizer = float64(d.preTotal[source])
	}
	pairRaw := int64(0)
	for i, raw := range raws {
		index := start + int64(i)
		if raw < 0 {
			continue // null raw weight: no strength can be derived
		}
		pairRaw += raw
		if normalizer <= 0 {
			summary.NormalizerZeroEdges++
			continue
		}
		weight := rules.Strength.Gain * float64(raw) / normalizer
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return fmt.Errorf("params: edge %d weight %g*%d/%g is not finite", index, rules.Strength.Gain, raw, normalizer)
		}
		set.EdgeWeight[index] = weight
	}

	matched := uint32(0)
	transmitter := NoTransmitter
	confidence := float32(0)
	sign := int8(0)
	if aggregate == nil || aggregate.count == 0 {
		summary.WithoutMatch += uint64(len(raws))
		summary.UnknownWithoutMatch += uint64(len(raws))
		summary.TransmitterUnmatched += uint64(len(raws))
		summary.Sign.Unknown += uint64(len(raws))
	} else {
		matched = aggregate.count
		best := 0
		bestMean := math.Inf(-1)
		for p := range aggregate.sums {
			mean := aggregate.sums[p] / float64(aggregate.count)
			if mean > bestMean {
				best, bestMean = p, mean
			}
		}
		transmitter = uint8(best)
		confidence = float32(bestMean)
		*matchedTotal += uint64(matched)
		if pairRaw > 0 {
			*rawTotal += uint64(pairRaw)
		}
		summary.WithMatch += uint64(len(raws))
		summary.TransmitterEdges[Transmitters[best]] += uint64(len(raws))
		// The threshold is applied to the stored float32 confidence, so a
		// reader of the file can repeat the comparison exactly.
		switch {
		case float64(confidence) < rules.Sign.MinProbability:
			summary.UnknownBelowProbability += uint64(len(raws))
			summary.Sign.Unknown += uint64(len(raws))
		case pairRaw <= 0 || float64(matched)/float64(pairRaw) < rules.Sign.MinMatchedFraction:
			summary.UnknownBelowMatchedFraction += uint64(len(raws))
			summary.Sign.Unknown += uint64(len(raws))
		default:
			switch rules.Sign.Mapping[Transmitters[best]] {
			case SignPositive:
				sign = 1
				summary.Sign.Positive += uint64(len(raws))
			case SignNegative:
				sign = -1
				summary.Sign.Negative += uint64(len(raws))
			default:
				summary.UnknownByMapping += uint64(len(raws))
				summary.Sign.Unknown += uint64(len(raws))
			}
		}
	}
	if len(raws) > 1 {
		summary.DuplicatePairEdges += uint64(len(raws) - 1)
	}
	for i := range raws {
		index := start + int64(i)
		set.EdgeSign[index] = sign
		set.EdgeSignConfidence[index] = confidence
		set.EdgeTransmitter[index] = transmitter
		set.EdgeMatchedSynapses[index] = matched
	}
	return nil
}

func (d *deriver) finishReport(started time.Time) {
	report := &d.report
	report.SchemaVersion = ReportSchemaVersion
	report.Source = SetSource
	report.RulesHash = d.rules.Hash()
	report.Rules = d.rules
	graph := d.graph.Report()
	report.Graph = GraphSummary{
		Namespace: d.graph.Namespace(),
		Nodes:     d.graph.NodeCount(),
		Edges:     d.graph.EdgeCount(),
		NodeIndex: graph.Hashes.NodeIndex,
		EdgeOrder: graph.Hashes.EdgeOrder,
		EdgeOrderFinding: "connectome sorts annotated edges by (source index, target index, absolute source row), " +
			"so the aggregated pairs were merge joined against the edge stream without building an index",
	}
	for _, spill := range d.spills {
		report.Spills = append(report.Spills, spill.stats())
	}
	report.Limits = d.limits
	report.PeakAccountedBytes = d.memory.peak
	report.WallTimeSeconds = time.Since(started).Seconds()
	report.Limitations = append([]string(nil), reportLimitations...)
}

// hashFile computes the size and SHA-256 of one regular file.
func hashFile(ctx context.Context, path string) (int64, string, error) {
	file, err := fileio.OpenRegular(ctx, path)
	if err != nil {
		return 0, "", fmt.Errorf("params: open source: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, "", fmt.Errorf("params: stat source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, "", fmt.Errorf("params: source %q is not a regular file", path)
	}
	digest := sha256.New()
	buffer := make([]byte, hashChunkBytes)
	size := int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return 0, "", fmt.Errorf("params: %w", err)
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			size += int64(n)
			digest.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, "", fmt.Errorf("params: read source: %w", readErr)
		}
	}
	return size, hex.EncodeToString(digest.Sum(nil)), nil
}

// spillFile is one sorted intermediate file written once and read once.
type spillFile struct {
	name        string
	path        string
	file        *os.File
	writer      *bufio.Writer
	recordBytes int
	records     int64
	bytes       int64
	limit       int64
	closed      bool
}

func (d *deriver) newSpill(name string, recordBytes int) (*spillFile, error) {
	file, err := os.CreateTemp(d.limits.TempDir, "coimnet-params-"+name+"-*.bin")
	if err != nil {
		return nil, fmt.Errorf("params: create the %s intermediate file: %w", name, err)
	}
	spill := &spillFile{
		name:        name,
		path:        file.Name(),
		file:        file,
		writer:      bufio.NewWriterSize(file, spillBufferBytes),
		recordBytes: recordBytes,
		limit:       d.limits.MaxTempBytes / 6,
	}
	d.spills = append(d.spills, spill)
	return spill, nil
}

func (s *spillFile) write(record []byte) error {
	if len(record) != s.recordBytes {
		return fmt.Errorf("params: %s record of %d bytes, want %d", s.name, len(record), s.recordBytes)
	}
	if int64(len(record)) > s.limit-s.bytes {
		return fmt.Errorf("%w: the %s intermediate file exceeds its %d byte budget", ErrCapacity, s.name, s.limit)
	}
	if _, err := s.writer.Write(record); err != nil {
		return fmt.Errorf("params: write the %s intermediate file: %w", s.name, err)
	}
	s.bytes += int64(len(record))
	s.records++
	return nil
}

// finish flushes the writer and rewinds for reading.
func (s *spillFile) finish() error {
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("params: flush the %s intermediate file: %w", s.name, err)
	}
	return nil
}

// reader rewinds the file and returns a sequential reader over it.
func (s *spillFile) reader() (*spillReader, error) {
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("params: rewind the %s intermediate file: %w", s.name, err)
	}
	return &spillReader{name: s.name, reader: bufio.NewReaderSize(s.file, spillBufferBytes)}, nil
}

func (s *spillFile) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	closeErr := s.file.Close()
	removeErr := os.Remove(s.path)
	if removeErr != nil && errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}

// spillReader reads whole records and reports a clean end.
type spillReader struct {
	name   string
	reader *bufio.Reader
}

func (r *spillReader) read(record []byte) (bool, error) {
	if _, err := io.ReadFull(r.reader, record); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, fmt.Errorf("params: read the %s intermediate file: %w", r.name, err)
	}
	return true, nil
}

// stats returns the report entry for this file.
func (s *spillFile) stats() SpillStats {
	return SpillStats{Name: s.name, RecordBytes: s.recordBytes, Records: s.records, Bytes: s.bytes}
}
