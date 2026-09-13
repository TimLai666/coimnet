package connectome

import (
	"encoding/json"

	"github.com/TimLai666/coimnet/signal"
)

// NullString keeps a source string with its null state. Null is never
// replaced by an empty string.
type NullString struct {
	Valid bool
	Value string
}

// NullInt64 keeps a source int64 with its null state.
type NullInt64 struct {
	Valid bool
	Value int64
}

// NullFloat64 keeps a source float64 with its null state.
type NullFloat64 struct {
	Valid bool
	Value float64
}

func (v NullString) MarshalJSON() ([]byte, error) {
	if !v.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(v.Value)
}

func (v NullInt64) MarshalJSON() ([]byte, error) {
	if !v.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(v.Value)
}

func (v NullFloat64) MarshalJSON() ([]byte, error) {
	if !v.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(v.Value)
}

// SourcePosition locates a record in its source file. AbsoluteRow is the
// row offset over the whole file and does not depend on batch segmentation.
type SourcePosition struct {
	Role        FileRole `json:"role"`
	Batch       int64    `json:"batch"`
	Row         int64    `json:"row"`
	AbsoluteRow int64    `json:"absolute_row"`
}

// Transmitter prediction status values for NodeRecord.
const (
	// TransmitterPredicted means the prediction file has a known consensus value.
	TransmitterPredicted = "predicted"
	// TransmitterUnknown means the prediction row exists but its consensus is
	// null, empty or an explicit unknown token.
	TransmitterUnknown = "unknown"
	// TransmitterNotAvailable means no prediction row exists for the neuron.
	TransmitterNotAvailable = "not_available"
	// TransmitterNotMapped means the manifest maps no consensus column, so
	// no prediction status can be derived for any neuron.
	TransmitterNotMapped = "not_mapped"
)

// Evidence status values shared by records and reports.
const (
	EvidenceUnknown      = "unknown"
	EvidenceNotAvailable = "not_available"
	EvidenceNotDerived   = "not_derived"
	// AggregationNone means the edge is one source row, not a sum.
	AggregationNone = "none"
	// AggregationNotPermitted reports that raw rows were never aggregated.
	AggregationNotPermitted = "not_permitted"
)

// TransmitterPrediction carries the source prediction fields of a neuron.
// It is a prediction, never a measured synaptic action.
type TransmitterPrediction struct {
	Status     string      `json:"status"`
	Consensus  NullString  `json:"consensus"`
	Predicted  NullString  `json:"predicted"`
	Confidence NullFloat64 `json:"confidence"`
}

// NodeRecord is one selected neuron of the annotated view. Unknown source
// values stay null.
type NodeRecord struct {
	Index        uint64                `json:"index"`
	ID           signal.NeuronID       `json:"id"`
	Status       NullString            `json:"status"`
	StatusLabel  NullString            `json:"status_label"`
	Class        NullString            `json:"class"`
	Superclass   NullString            `json:"superclass"`
	Subclass     NullString            `json:"subclass"`
	Type         NullString            `json:"type"`
	Instance     NullString            `json:"instance"`
	SomaSide     NullString            `json:"soma_side"`
	ReceptorType NullString            `json:"receptor_type"`
	Transmitter  TransmitterPrediction `json:"transmitter"`
	Position     SourcePosition        `json:"position"`
}

// EdgeRecord is one edge of the annotated view between continuous indices.
// Weight is the source raw value under its official name; it is not a
// synapse or contact count claim. Transmitter and Sign stay unknown because
// the weights file carries no such evidence.
type EdgeRecord struct {
	Source      uint64         `json:"source"`
	Target      uint64         `json:"target"`
	Weight      NullInt64      `json:"weight"`
	Position    SourcePosition `json:"position"`
	Aggregation string         `json:"aggregation"`
	Transmitter string         `json:"transmitter"`
	Sign        string         `json:"sign"`
}

// RawSegment is one weights row exactly as stored: nullable signed
// endpoints, nullable weight and its position. Nothing is aggregated.
type RawSegment struct {
	Namespace string         `json:"namespace"`
	Source    NullInt64      `json:"source"`
	Target    NullInt64      `json:"target"`
	Weight    NullInt64      `json:"weight"`
	Position  SourcePosition `json:"position"`
}
