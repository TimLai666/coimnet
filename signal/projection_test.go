package signal_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/signal"
)

func projectionCandidates() []signal.NeuronCandidate {
	return []signal.NeuronCandidate{
		{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "b"}, CellType: "T", Region: "R"},
		{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "a"}, CellType: "T"},
		{ID: signal.NeuronID{Namespace: "fixture", ExternalID: "c"}, Region: "R"},
	}
}
func projectionSpec() signal.ProjectionSpec {
	return signal.ProjectionSpec{SchemaVersion: signal.CurrentSchemaVersion(), Direction: signal.ProjectionInput,
		Source: "artificial-fixture/v1", Artificial: true, ChannelShape: []int{2},
		Selection: signal.NeuronSelection{Mode: signal.SelectExplicit, Neurons: []signal.NeuronID{projectionCandidates()[1].ID, projectionCandidates()[0].ID}},
		Weights:   []float64{1, 2, 3, 4}}
}
func TestProjectionRoundTripSelectionAndBinding(t *testing.T) {
	candidates := projectionCandidates()
	for _, direction := range []signal.ProjectionDirection{signal.ProjectionInput, signal.ProjectionOutput} {
		for _, selection := range []signal.NeuronSelection{
			projectionSpec().Selection, {Mode: signal.SelectCellType, Value: "T"}, {Mode: signal.SelectRegion, Value: "R"},
		} {
			t.Run(string(direction)+"/"+string(selection.Mode), func(t *testing.T) {
				spec := projectionSpec()
				spec.Direction = direction
				spec.Selection = selection
				p, err := signal.NewProjection(spec, candidates)
				if err != nil {
					t.Fatal(err)
				}
				data, err := json.Marshal(p)
				if err != nil {
					t.Fatal(err)
				}
				q, err := signal.DecodeProjection(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				again, err := json.Marshal(q)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(data, again) || !reflect.DeepEqual(p.Spec(), q.Spec()) {
					t.Fatal("projection round trip changed")
				}
				indices, err := q.Bind(candidates)
				if err != nil {
					t.Fatal(err)
				}
				for i, id := range q.Neurons() {
					if candidates[indices[i]].ID != id {
						t.Fatal("wrong graph index")
					}
				}
				reversed := []signal.NeuronCandidate{candidates[2], candidates[1], candidates[0]}
				indices, err = q.Bind(reversed)
				if err != nil {
					t.Fatal(err)
				}
				for i, id := range q.Neurons() {
					if reversed[indices[i]].ID != id {
						t.Fatal("binding assumed graph order")
					}
				}
				if _, err = q.Bind(candidates[:1]); err == nil {
					t.Fatal("accepted absent cell")
				}
				spec.Weights[0] = 99
				got := q.Spec()
				got.Weights[0] = 98
				got.ChannelShape[0] = 9
				if p.Weights()[0] != 1 || q.Weights()[0] != 1 || q.ChannelShape()[0] != 2 {
					t.Fatal("projection shares caller memory")
				}
			})
		}
	}
}
func TestProjectionRandomReconstructionAndStrictJSON(t *testing.T) {
	spec := projectionSpec()
	spec.Weights = nil
	spec.Random = &signal.ProjectionRandom{Seed: 7, Scale: 0.5}
	p, err := signal.NewProjection(spec, projectionCandidates())
	if err != nil {
		t.Fatal(err)
	}
	q, err := signal.NewProjection(p.Spec(), projectionCandidates())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Weights(), q.Weights()) {
		t.Fatal("seed reconstruction differs")
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, replacement := range [][2]string{
		{`"seed":7`, `"seed":null`}, {`"input_size":2`, `"input_size":3`}, {`"scale":0.5`, `"scale":1`},
		{`"neuron_hash":"` + p.NeuronHash() + `"`, `"neuron_hash":"invalid"`},
	} {
		bad := bytes.Replace(data, []byte(replacement[0]), []byte(replacement[1]), 1)
		if bytes.Equal(data, bad) {
			t.Fatal("mutation did not match")
		}
		if err := json.Unmarshal(bad, &q); err == nil {
			t.Fatalf("accepted corrupt projection: %s", bad)
		}
		after, err := json.Marshal(q)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, after) {
			t.Fatal("failed load changed receiver")
		}
	}
	for _, bad := range []string{string(data) + `{}`, strings.Replace(string(data), `"source":`, `"unknown":1,"source":`, 1), strings.Replace(string(data), `"direction":`, `"DIRECTION":"input","direction":`, 1)} {
		if _, err := signal.DecodeProjection(strings.NewReader(bad)); err == nil {
			t.Fatal("accepted non-strict JSON")
		}
	}
}
