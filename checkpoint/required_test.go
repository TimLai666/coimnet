package checkpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// TestLoadIndividualAcceptsScalarPointerRuleFields pins the two pointer-to-int
// fields of a plasticity rule. They are the only scalar pointers a checkpoint
// carries, and the presence walk used to refuse every document that set one,
// so SaveIndividual wrote receptor-gated individuals that LoadIndividual could
// not read back. A value of the wrong JSON kind is still refused, by path.
func TestLoadIndividualAcceptsScalarPointerRuleFields(t *testing.T) {
	individual := newReceptorGatedIndividual(t)
	want := individual.Snapshot()
	dir := t.TempDir()
	path := filepath.Join(dir, "receptor-gated.json")
	if err := SaveIndividual(context.Background(), path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadIndividual(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadIndividual rejected a receptor-gated rule: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("round trip changed the receptor-gated individual snapshot")
	}
	if got.Plastic.Config.Rule.GateReceptor == nil || got.Plastic.Config.Rule.DecayEReceptor == nil {
		t.Fatal("round trip dropped a receptor index")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw envelope
	if err := json.Unmarshal(saved, &raw); err != nil {
		t.Fatal(err)
	}
	quoted := bytes.Replace(raw.Payload, []byte(`"gate_receptor":0`), []byte(`"gate_receptor":"0"`), 1)
	if bytes.Equal(quoted, raw.Payload) {
		t.Fatal("fixture gate_receptor was not found")
	}
	quotedPath := filepath.Join(dir, "quoted-gate-receptor.json")
	writeRaw(t, quotedPath, envelopeJSON(IndividualSchemaVersion, quoted, checksumHex(quoted)))
	if _, err := LoadIndividual(context.Background(), quotedPath); err == nil || !strings.Contains(err.Error(), "gate_receptor") {
		t.Fatalf("quoted gate_receptor error = %v", err)
	}
}

// newReceptorGatedIndividual is the smallest individual whose plasticity rule
// sets both receptor indices: a chemistry layer to declare receptor 0, a gate
// reading it and an eligibility decay reading it as well.
func newReceptorGatedIndividual(t *testing.T) *learning.Individual {
	t.Helper()
	c := learning.Config{
		Dynamics:  dynamics.Config{Nodes: 2, Sources: []int{0}, Targets: []int{1}, DT: 1, Activation: "tanh"},
		InputSize: 1, OutputSize: 1, ReadoutNodes: []int{1},
	}
	p := learning.Parameters{
		Core:    dynamics.Parameters{Weights: []float64{.25}, Bias: []float64{0, 0}, LogTau: []float64{0, 0}},
		Encoder: []float64{1, 0},
		Readout: []float64{1},
	}
	individual, err := learning.NewIndividual(c, p, learning.DefaultOptions(), []float64{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	declaration := modulation.ChemistryConfig{
		Chemistry: modulation.Chemistry{Regions: 1, Channels: 1, DT: 1, Tau: []float64{2}},
		Sources: []modulation.SourceSpec{{
			Kind: modulation.SourceExternalTimeline, Channel: 0,
			Timeline: &modulation.ExternalTimeline{ChannelCount: 1, Entries: []modulation.TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}},
		}},
		Receptors: modulation.Receptors{Records: []modulation.Receptor{
			{Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: modulation.StatusHypothesized, Kd: 0.5, N: 1,
				Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "chem-fixture/v1"},
		}},
		Regions: modulation.RegionAssignment{NodeRegion: []int{0, 0}},
	}
	if err := individual.EnableChemistry(declaration); err != nil {
		t.Fatal(err)
	}
	receptor := 0
	rule := plasticity.Rule{
		Kind: plasticity.RuleHebbianRate, DecayE: .5, DecayP: .5, PlasticMax: 1, WMin: .01,
		GateReceptor: &receptor, GateScale: 1,
		DecayEReceptor: &receptor, DecayEBase: .5, DecayESpan: .2, DecayEMin: .1, DecayEMax: .9,
	}
	if err := individual.EnablePlasticity(plasticity.Config{Rule: rule, Edges: []int{0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := individual.Advance(context.Background(), [][]float64{{1}, {0}}); err != nil {
		t.Fatal(err)
	}
	return individual
}

// TestCheckRequiredFieldsScalarPointerKinds covers the scalar pointer kinds no
// checkpoint type reaches yet, so the rule the walk follows is pinned rather
// than inferred from the one field that exercises it today.
func TestCheckRequiredFieldsScalarPointerKinds(t *testing.T) {
	type target struct {
		Index *int     `json:"index,omitempty"`
		Ratio *float64 `json:"ratio,omitempty"`
		Flag  *bool    `json:"flag,omitempty"`
		Label *string  `json:"label,omitempty"`
		Slice *[]int   `json:"slice,omitempty"`
	}
	rt := reflect.TypeOf(target{})
	accepted := []string{
		`{}`,
		`{"index":null,"ratio":null,"flag":null,"label":null}`,
		`{"index":-1,"ratio":1.5,"flag":false,"label":"x"}`,
		`{"index":0,"ratio":-0.5,"flag":true,"label":""}`,
	}
	for _, document := range accepted {
		if err := checkRequiredFields([]byte(document), rt, "$"); err != nil {
			t.Fatalf("checkRequiredFields(%s) = %v", document, err)
		}
	}
	rejected := map[string]string{
		`{"index":"0"}`:   "$.index is not a JSON number",
		`{"ratio":true}`:  "$.ratio is not a JSON number",
		`{"flag":1}`:      "$.flag is not a JSON boolean",
		`{"label":5}`:     "$.label is not a JSON string",
		`{"slice":[1,2]}`: "$.slice has unsupported pointer element kind slice",
	}
	for document, want := range rejected {
		err := checkRequiredFields([]byte(document), rt, "$")
		if err == nil || err.Error() != want {
			t.Fatalf("checkRequiredFields(%s) = %v, want %q", document, err, want)
		}
	}
}
