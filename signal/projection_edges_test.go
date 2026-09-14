package signal_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

func TestProjectionRejectsInvalidSelectionShapeAndCoefficients(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*signal.ProjectionSpec)
	}{
		{"schema", func(s *signal.ProjectionSpec) { s.SchemaVersion.Major = 2 }},
		{"direction", func(s *signal.ProjectionSpec) { s.Direction = "other" }},
		{"source", func(s *signal.ProjectionSpec) { s.Source = " " }},
		{"source UTF8", func(s *signal.ProjectionSpec) { s.Source = string([]byte{255}) }},
		{"biological evidence", func(s *signal.ProjectionSpec) { s.Artificial = false }},
		{"zero shape", func(s *signal.ProjectionSpec) { s.ChannelShape = []int{0} }},
		{"shape overflow", func(s *signal.ProjectionSpec) { s.ChannelShape = []int{int(^uint(0) >> 1), 2} }},
		{"weight limit before random allocation", func(s *signal.ProjectionSpec) {
			s.ChannelShape = []int{signal.MaxProjectionWeights}
			s.Weights = nil
			s.Random = &signal.ProjectionRandom{Scale: 1}
		}},
		{"coefficient shape", func(s *signal.ProjectionSpec) { s.Weights = s.Weights[:3] }},
		{"NaN", func(s *signal.ProjectionSpec) { s.Weights[0] = math.NaN() }},
		{"Inf", func(s *signal.ProjectionSpec) { s.Weights[0] = math.Inf(-1) }},
		{"selection", func(s *signal.ProjectionSpec) { s.Selection.Mode = "other" }},
		{"duplicate selected", func(s *signal.ProjectionSpec) { s.Selection.Neurons[1] = s.Selection.Neurons[0] }},
		{"missing selected", func(s *signal.ProjectionSpec) { s.Selection.Neurons[1].ExternalID = "missing" }},
		{"unknown metadata", func(s *signal.ProjectionSpec) {
			s.Selection = signal.NeuronSelection{Mode: signal.SelectRegion, Value: ""}
		}},
		{"no match", func(s *signal.ProjectionSpec) {
			s.Selection = signal.NeuronSelection{Mode: signal.SelectCellType, Value: "other"}
		}},
		{"ambiguous selection", func(s *signal.ProjectionSpec) { s.Selection.Value = "T" }},
		{"zero random scale", func(s *signal.ProjectionSpec) { s.Weights = nil; s.Random = &signal.ProjectionRandom{Seed: 7} }},
		{"NaN random scale", func(s *signal.ProjectionSpec) {
			s.Weights = nil
			s.Random = &signal.ProjectionRandom{Scale: math.NaN()}
		}},
		{"random disagreement", func(s *signal.ProjectionSpec) { s.Random = &signal.ProjectionRandom{Seed: 7, Scale: 1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := projectionSpec()
			tc.mutate(&s)
			if _, err := signal.NewProjection(s, projectionCandidates()); err == nil {
				t.Fatal("accepted invalid projection")
			}
		})
	}
	candidates := projectionCandidates()
	candidates[1].ID = candidates[0].ID
	if _, err := signal.NewProjection(projectionSpec(), candidates); err == nil {
		t.Fatal("accepted duplicate graph IDs")
	}
	s := projectionSpec()
	s.Artificial = false
	s.Evidence = "caller-supplied source record"
	if _, err := signal.NewProjection(s, projectionCandidates()); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionChannelShapeAndChangedMetadata(t *testing.T) {
	s := projectionSpec()
	s.ChannelShape = []int{1, 2}
	s.Selection = signal.NeuronSelection{Mode: signal.SelectCellType, Value: "T"}
	p, err := signal.NewProjection(s, projectionCandidates())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateChannelShape([]int{1, 2}); err != nil {
		t.Fatal(err)
	}
	for _, shape := range [][]int{nil, {2}, {2, 1}, {1, 3}} {
		if err := p.ValidateChannelShape(shape); err == nil {
			t.Fatal("accepted channel shape", shape)
		}
	}
	candidates := projectionCandidates()
	candidates[1].CellType = "other"
	if _, err := p.Bind(candidates); err == nil {
		t.Fatal("accepted changed metadata selection")
	}
	for _, direction := range []signal.ProjectionDirection{signal.ProjectionInput, signal.ProjectionOutput} {
		s := projectionSpec()
		s.ChannelShape = []int{3}
		s.Direction = direction
		s.Weights = []float64{1, 2, 3, 4, 5, 6}
		p, err := signal.NewProjection(s, projectionCandidates())
		if err != nil {
			t.Fatal(err)
		}
		in, out := 3, 2
		if direction == signal.ProjectionOutput {
			in, out = 2, 3
		}
		if p.InputSize() != in || p.OutputSize() != out {
			t.Fatal("asymmetric projection dimensions transposed")
		}
	}
}

func TestProjectionRandomGoldenAndNumericNull(t *testing.T) {
	s := projectionSpec()
	s.ChannelShape = []int{4}
	s.Weights = nil
	s.Random = &signal.ProjectionRandom{Seed: 7, Scale: 0.5}
	p, err := signal.NewProjection(s, projectionCandidates())
	if err != nil {
		t.Fatal(err)
	}
	// Frozen splitmix64-sign/v1 vector, independently evaluated with uint64
	// integer arithmetic; it is a compatibility fixture for the wire algorithm.
	if !reflect.DeepEqual(p.Weights(), []float64{0.5, -0.5, -0.5, 0.5, -0.5, 0.5, -0.5, -0.5}) {
		t.Fatal("random algorithm changed")
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range [][2]string{
		{`"weights":[0.5,`, `"weights":[null,`}, {`"artificial":true`, `"artificial":null`},
		{`"seed":7,`, ``}, {`"scale":0.5`, `"scale":null`}, {`"input_size":4`, `"input_size":null`},
		{`"algorithm":"splitmix64-sign/v1"`, `"algorithm":"unknown"`},
	} {
		bad := bytes.Replace(data, []byte(mutation[0]), []byte(mutation[1]), 1)
		if bytes.Equal(bad, data) {
			t.Fatal("invalid fixture mutation")
		}
		if _, err := signal.DecodeProjection(bytes.NewReader(bad)); err == nil {
			t.Fatalf("accepted invalid projection JSON: %s", bad)
		}
	}
	var zero signal.Projection
	if _, err := json.Marshal(zero); err == nil {
		t.Fatal("marshaled uninitialized projection")
	}
	if _, err := signal.DecodeProjection(nil); err == nil {
		t.Fatal("accepted nil reader")
	}
}
