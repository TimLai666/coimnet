package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

func chemicalSourceStateCheckpointIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	config := learning.Config{
		Dynamics: dynamics.Config{
			Nodes: 2, Sources: []int{}, Targets: []int{}, Delays: []int{}, DT: 1, Activation: "tanh",
		},
		InputSize: 2, OutputSize: 1, ReadoutNodes: []int{0}, InputNodes: []int{0, 1},
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0, 0, 1}, Readout: []float64{1},
	}
	individual, err := learning.NewIndividual(config, parameters, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	declaration := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceController, Channel: 0,
			Controller: &modulation.Controller{
				Inputs: modulation.ControllerInputs{SummaryWindow: 3},
				Nodes:  2, Hidden: 1, Channel: 0,
				Parameters: []float64{1, 0, 0, 1, 0},
			},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{}},
		Effects:   []modulation.Effect{},
		Regions:   modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
	if err := individual.EnableChemistry(declaration); err != nil {
		t.Fatal(err)
	}
	return individual
}

func chemicalSourceStateCheckpointPayload(t *testing.T, encoded []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var outer testEnvelope
	if err := json.Unmarshal(encoded, &outer); err != nil {
		t.Fatal(err)
	}
	payload := rawObject(t, outer.Payload)
	chemical := rawObject(t, payload["chemical"])
	mutate(chemical)
	payload["chemical"] = mustRawJSON(t, chemical)
	changed := mustRawJSON(t, payload)
	return envelopeJSON(IndividualSchemaVersion, changed, checksumHex(changed))
}

func chemicalSourceStateCheckpointControllerPayload(t *testing.T, encoded []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return chemicalSourceStateCheckpointPayload(t, encoded, func(chemical map[string]json.RawMessage) {
		var states []json.RawMessage
		if err := json.Unmarshal(chemical["source_states"], &states); err != nil {
			t.Fatalf("decode source_states: %v", err)
		}
		if len(states) != 1 {
			t.Fatalf("source_states has %d entries, want one", len(states))
		}
		state := rawObject(t, states[0])
		controller := rawObject(t, state["controller"])
		mutate(controller)
		state["controller"] = mustRawJSON(t, controller)
		states[0] = mustRawJSON(t, state)
		chemical["source_states"] = mustRawJSON(t, states)
	})
}

func TestChemicalSourceStateCheckpointRoundTripAndResume(t *testing.T) {
	individual := chemicalSourceStateCheckpointIndividual(t)
	prefix := [][]float64{{1, 0}, {0, 1}, {1, 1}}
	suffix := [][]float64{{0.5, 0}, {0, 0.5}}
	if _, err := individual.Advance(context.Background(), prefix); err != nil {
		t.Fatal(err)
	}
	wantRuntime := individual.Snapshot()
	want := normalizeIndividualSnapshot(wantRuntime)
	if want.Chemical == nil || len(want.Chemical.SourceStates) != 1 || want.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("controller source state missing from snapshot: %#v", want.Chemical)
	}
	if want.Chemical.SourceStates[0].Controller.Filled != 3 {
		t.Fatalf("controller Filled = %d, want 3", want.Chemical.SourceStates[0].Controller.Filled)
	}
	encoded, err := EncodeIndividual(want)
	if err != nil {
		t.Fatalf("EncodeIndividual: %v", err)
	}
	if bytes.Contains(encoded, []byte("null")) {
		t.Fatalf("encoded controller checkpoint contains null: %s", encoded)
	}
	decoded, err := DecodeIndividual(encoded)
	if err != nil {
		t.Fatalf("DecodeIndividual: %v", err)
	}
	// Decode returns the runtime canonical snapshot. Required empty arrays
	// are normalized only on the wire, so compare with the original runtime.
	if !reflect.DeepEqual(decoded, wantRuntime) {
		t.Fatalf("checkpoint round trip changed the runtime snapshot:\n got %#v\nwant %#v", decoded, wantRuntime)
	}

	reference, err := learning.RestoreIndividual(want)
	if err != nil {
		t.Fatal(err)
	}
	wantOutput, err := reference.Advance(context.Background(), suffix)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := learning.RestoreIndividual(decoded)
	if err != nil {
		t.Fatal(err)
	}
	gotOutput, err := resumed.Advance(context.Background(), suffix)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotOutput, wantOutput) || !reflect.DeepEqual(resumed.Snapshot(), reference.Snapshot()) {
		t.Fatal("checkpoint SourceStates did not preserve bit-identical continuation")
	}
}

func TestChemicalSourceStateCheckpointLegacyMissingSourceStatesKeepsOtherState(t *testing.T) {
	individual := chemicalSourceStateCheckpointIndividual(t)
	if _, err := individual.Advance(context.Background(), [][]float64{{1, 0}, {0, 1}, {1, 1}}); err != nil {
		t.Fatal(err)
	}
	want := individual.Snapshot()
	encoded, err := EncodeIndividual(want)
	if err != nil {
		t.Fatal(err)
	}
	legacy := chemicalSourceStateCheckpointPayload(t, encoded, func(chemical map[string]json.RawMessage) {
		delete(chemical, "source_states")
	})
	decoded, err := DecodeIndividual(legacy)
	if err != nil {
		t.Fatalf("DecodeIndividual legacy checkpoint: %v", err)
	}
	if decoded.Chemical == nil || len(decoded.Chemical.SourceStates) != 1 || decoded.Chemical.SourceStates[0].Controller == nil {
		t.Fatalf("legacy decode did not canonicalize a fresh controller state: %#v", decoded.Chemical)
	}
	if decoded.Chemical.SourceStates[0].Controller.Filled != 0 {
		t.Fatalf("legacy controller Filled = %d, want fresh zero", decoded.Chemical.SourceStates[0].Controller.Filled)
	}
	if decoded.Chemical.State.Steps != want.Chemical.State.Steps || !reflect.DeepEqual(decoded.Chemical.State.Concentration, want.Chemical.State.Concentration) || decoded.Neural.Continuous.Steps != want.Neural.Continuous.Steps {
		t.Fatal("legacy checkpoint reset concentration or neural clock with SourceStates")
	}
}

func TestChemicalSourceStateCheckpointRejectsMalformedSourceStates(t *testing.T) {
	individual := chemicalSourceStateCheckpointIndividual(t)
	if _, err := individual.Advance(context.Background(), [][]float64{{1, 0}}); err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeIndividual(individual.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(map[string]json.RawMessage){
		"missing window": func(controller map[string]json.RawMessage) { delete(controller, "window") },
		"missing filled": func(controller map[string]json.RawMessage) { delete(controller, "filled") },
		"null window":    func(controller map[string]json.RawMessage) { controller["window"] = json.RawMessage(`null`) },
		"null filled":    func(controller map[string]json.RawMessage) { controller["filled"] = json.RawMessage(`null`) },
		"unknown field":  func(controller map[string]json.RawMessage) { controller["unexpected"] = json.RawMessage(`true`) },
		"negative filled": func(controller map[string]json.RawMessage) {
			controller["filled"] = json.RawMessage(`-1`)
		},
		"row width": func(controller map[string]json.RawMessage) {
			controller["window"] = json.RawMessage(`[[0],[0,0],[0,0]]`)
		},
		"nonfinite row": func(controller map[string]json.RawMessage) {
			controller["window"] = json.RawMessage(`[[0,0],[1e400,0],[0,0]]`)
		},
		"unfilled nonzero": func(controller map[string]json.RawMessage) {
			controller["filled"] = json.RawMessage(`1`)
			controller["window"] = json.RawMessage(`[[0.25,0],[0,0],[0,0]]`)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := chemicalSourceStateCheckpointControllerPayload(t, encoded, mutate)
			if _, err := DecodeIndividual(bad); err == nil {
				t.Fatal("DecodeIndividual accepted malformed SourceStates")
			}
		})
	}
}
