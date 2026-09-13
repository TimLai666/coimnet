package connectome

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/TimLai666/coimnet/feather"
	"github.com/TimLai666/coimnet/signal"
	"github.com/apache/arrow/go/v17/arrow"
)

const annotationFieldCount = 9

// nodeData is the private storage of one selected neuron.
type nodeData struct {
	id          uint64
	position    SourcePosition
	fields      [annotationFieldCount]NullString
	transmitter TransmitterPrediction
	ntSeen      bool
}

// edgeStore keeps the annotated edges as struct-of-arrays. Index columns use
// 32-bit storage only when every index fits; the choice is explicit, never a
// silent truncation.
type edgeStore struct {
	wide         bool
	src32, dst32 []uint32
	src64, dst64 []uint64
	absRow       []int64
	weight       []int64
	weightValid  []bool
	rows         []uint32 // aggregated pairs only: source rows summed per edge
}

func edgeBytes(wide, aggregated bool) int64 {
	bytes := int64(8 + 8 + 1) // absolute row, weight, weight validity
	if wide {
		bytes += 16
	} else {
		bytes += 8
	}
	if aggregated {
		bytes += 4
	}
	return bytes
}

func (e *edgeStore) append(source, target uint64, absRow int64, weight NullInt64, rows uint32) {
	if e.wide {
		e.src64 = append(e.src64, source)
		e.dst64 = append(e.dst64, target)
	} else {
		e.src32 = append(e.src32, uint32(source))
		e.dst32 = append(e.dst32, uint32(target))
	}
	e.absRow = append(e.absRow, absRow)
	e.weight = append(e.weight, weight.Value)
	e.weightValid = append(e.weightValid, weight.Valid)
	if e.rows != nil {
		e.rows = append(e.rows, rows)
	}
}

func (e *edgeStore) count() int { return len(e.absRow) }

func (e *edgeStore) endpoints(i int) (uint64, uint64) {
	if e.wide {
		return e.src64[i], e.dst64[i]
	}
	return uint64(e.src32[i]), uint64(e.dst32[i])
}

// Graph is the immutable build result. Accessors return copies; stream
// callbacks receive records that are only valid during the call.
type Graph struct {
	namespace   string
	nodeIDs     []uint64
	nodes       []nodeData
	edges       edgeStore
	batchStarts []int64
	weights     SourceFile
	scan        feather.Options
	scanFields  WeightsFields
	report      GraphReport
}

// NodeCount returns the number of selected neurons.
func (g *Graph) NodeCount() uint64 { return uint64(len(g.nodes)) }

// EdgeCount returns the number of annotated edges in the built view mode.
func (g *Graph) EdgeCount() uint64 { return uint64(g.edges.count()) }

// Namespace returns the dataset namespace of every neuron ID.
func (g *Graph) Namespace() string { return g.namespace }

// Report returns a copy of the build report.
func (g *Graph) Report() GraphReport { return g.report.clone() }

// NeuronIDs returns the selected neurons in index order as a new slice.
func (g *Graph) NeuronIDs() []signal.NeuronID {
	ids := make([]signal.NeuronID, len(g.nodeIDs))
	for i, id := range g.nodeIDs {
		ids[i] = ExternalID{Namespace: g.namespace, Value: id}.NeuronID()
	}
	return ids
}

// IndexOf returns the continuous index of a selected neuron.
func (g *Graph) IndexOf(id signal.NeuronID) (uint64, error) {
	value, err := signalID(id, g.namespace)
	if err != nil {
		return 0, err
	}
	index, found := slices.BinarySearch(g.nodeIDs, value)
	if !found {
		return 0, fmt.Errorf("connectome: neuron %s/%s is not in the annotated view", id.Namespace, id.ExternalID)
	}
	return uint64(index), nil
}

// Node returns the record of the neuron at index.
func (g *Graph) Node(index uint64) (NodeRecord, error) {
	if index >= uint64(len(g.nodes)) {
		return NodeRecord{}, fmt.Errorf("connectome: node index %d out of range [0,%d)", index, len(g.nodes))
	}
	return g.nodeRecord(int(index)), nil
}

func (g *Graph) nodeRecord(i int) NodeRecord {
	n := &g.nodes[i]
	return NodeRecord{
		Index:        uint64(i),
		ID:           ExternalID{Namespace: g.namespace, Value: n.id}.NeuronID(),
		Status:       n.fields[0],
		StatusLabel:  n.fields[1],
		Class:        n.fields[2],
		Superclass:   n.fields[3],
		Subclass:     n.fields[4],
		Type:         n.fields[5],
		Instance:     n.fields[6],
		SomaSide:     n.fields[7],
		ReceptorType: n.fields[8],
		Transmitter:  n.transmitter,
		Position:     n.position,
	}
}

// StreamAnnotatedNodes visits every selected neuron in index order.
func (g *Graph) StreamAnnotatedNodes(ctx context.Context, fn func(NodeRecord) error) error {
	if ctx == nil || fn == nil {
		return errors.New("connectome: nil context or callback")
	}
	for i := range g.nodes {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("connectome: %w", err)
		}
		if err := fn(g.nodeRecord(i)); err != nil {
			return fmt.Errorf("connectome: node callback at index %d: %w", i, err)
		}
	}
	return ctx.Err()
}

// StreamAnnotatedEdges visits every edge in canonical order: source index,
// target index, then absolute source row.
func (g *Graph) StreamAnnotatedEdges(ctx context.Context, fn func(EdgeRecord) error) error {
	if ctx == nil || fn == nil {
		return errors.New("connectome: nil context or callback")
	}
	for i := 0; i < g.edges.count(); i++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("connectome: %w", err)
		}
		if err := fn(g.edgeRecord(i)); err != nil {
			return fmt.Errorf("connectome: edge callback at position %d: %w", i, err)
		}
	}
	return ctx.Err()
}

func (g *Graph) edgeRecord(i int) EdgeRecord {
	source, target := g.edges.endpoints(i)
	absRow := g.edges.absRow[i]
	batch, row := g.locateWeightRow(absRow)
	record := EdgeRecord{
		Source:      source,
		Target:      target,
		Weight:      NullInt64{Valid: g.edges.weightValid[i], Value: g.edges.weight[i]},
		Position:    SourcePosition{Role: RoleWeights, Batch: batch, Row: row, AbsoluteRow: absRow},
		Aggregation: AggregationNone,
		Transmitter: EvidenceUnknown,
		Sign:        EvidenceUnknown,
	}
	if g.edges.rows != nil && g.edges.rows[i] > 1 {
		record.Aggregation = fmt.Sprintf("sum_of_%d_rows", g.edges.rows[i])
	}
	return record
}

// locateWeightRow maps an absolute weights row to (batch, row in batch).
func (g *Graph) locateWeightRow(absRow int64) (int64, int64) {
	batch := sort.Search(len(g.batchStarts), func(i int) bool { return g.batchStarts[i] > absRow }) - 1
	if batch < 0 {
		return 0, absRow
	}
	return int64(batch), absRow - g.batchStarts[batch]
}

// StreamRawSegments re-reads the weights source batch by batch and visits
// every row as stored. The source fingerprint is verified before and after
// the read; nothing is cached in memory.
func (g *Graph) StreamRawSegments(ctx context.Context, fn func(RawSegment) error) (retErr error) {
	if ctx == nil || fn == nil {
		return errors.New("connectome: nil context or callback")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("connectome: %w", err)
	}
	if err := verifyFingerprint(ctx, g.weights); err != nil {
		return err
	}
	fields := g.scanFields
	absRow := int64(0)
	batch := int64(0)
	_, err := feather.Scan(ctx, g.weights.Path, g.scan, func(record arrow.Record) error {
		source, err := int64Column(record, fields.Source)
		if err != nil {
			return err
		}
		target, err := int64Column(record, fields.Target)
		if err != nil {
			return err
		}
		weight, err := int64Column(record, fields.Value)
		if err != nil {
			return err
		}
		rows := int(record.NumRows())
		for i := 0; i < rows; i++ {
			if i%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			segment := RawSegment{
				Namespace: g.namespace,
				Source:    nullInt64(source, i),
				Target:    nullInt64(target, i),
				Weight:    nullInt64(weight, i),
				Position:  SourcePosition{Role: RoleWeights, Batch: batch, Row: int64(i), AbsoluteRow: absRow},
			}
			if err := fn(segment); err != nil {
				return err
			}
			absRow++
		}
		batch++
		return nil
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return fmt.Errorf("connectome: %w", ctxErr)
		}
		return fmt.Errorf("connectome: stream raw segments: %w", err)
	}
	return verifyFingerprint(ctx, g.weights)
}

func nullInt64(values interface {
	IsNull(int) bool
	Value(int) int64
}, i int) NullInt64 {
	if values.IsNull(i) {
		return NullInt64{}
	}
	return NullInt64{Valid: true, Value: values.Value(i)}
}

func verifyFingerprint(ctx context.Context, file SourceFile) error {
	actual, err := hashFile(ctx, file.Path)
	if err != nil {
		return err
	}
	if actual.bytes != file.Bytes || !strings.EqualFold(actual.sha256, file.SHA256) {
		return fmt.Errorf("%w: %s %s has %d bytes sha256 %s, manifest declares %d bytes sha256 %s", ErrSourceChanged, file.Role, file.Path, actual.bytes, actual.sha256, file.Bytes, file.SHA256)
	}
	return nil
}

func checkedIndexWidth(nodes int) bool {
	return uint64(nodes) > math.MaxUint32
}
