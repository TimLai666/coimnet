package modulation

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// fixtureConfig is a complete two-region, two-channel declaration: one source
// per channel in channel order, one receptor per status that computes, one
// effect of each kind, and a region for every one of the three nodes.
func fixtureConfig(t *testing.T) ChemistryConfig {
	t.Helper()
	return ChemistryConfig{
		Chemistry: Chemistry{Regions: 2, Channels: 2, DT: 1, Tau: []float64{2, 4}, Units: UnitNormalized,
			Transport: &Transport{Fraction: [][]float64{{0, 0.25}, {0.5, 0}}}},
		Sources: []SourceSpec{
			{Kind: SourceExternalTimeline, Channel: 0, Timeline: &ExternalTimeline{ChannelCount: 1, Entries: []TimelineEntry{{Step: 1, Channel: 0, Rate: 1}}}},
			{Kind: SourceNeuralActivity, Channel: 1, Neural: &NeuralActivity{Nodes: []int{0, 2}, SetName: "alpn", Gain: 0.5, Channel: 1}},
		},
		Receptors: Receptors{
			Records: []Receptor{
				{Cells: []int{0}, Signal: "octopamine", Channel: 0, Status: StatusHypothesized, Kd: 0.5, N: 1,
					Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "v1"},
				{Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: StatusUnresponsive,
					Evidence: "fixture", MeasurementKind: "declared", MappingVersion: "v1"},
			},
			Mix:                      MixSum,
			AllowAssumedCoefficients: false,
		},
		Effects: []Effect{
			{Kind: EffectSensitivity, Receptor: 0, GammaScale: 1},
			{Kind: EffectThreshold, Receptor: 1, ThetaScale: 0.5, ThetaAbsMax: 1},
		},
		Regions: RegionAssignment{NodeRegion: []int{0, 1, 1}},
	}
}

func TestSourceSpecBuildsExactlyTheDeclaredBody(t *testing.T) {
	for name, spec := range map[string]SourceSpec{
		"external_timeline": {Kind: SourceExternalTimeline, Channel: 0, Timeline: &ExternalTimeline{ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: 2}}}},
		"neural_activity":   {Kind: SourceNeuralActivity, Channel: 1, Neural: &NeuralActivity{Nodes: []int{1, 2}, Gain: 0.5, Channel: 1}},
		"internal_resource": {Kind: SourceInternalResource, Channel: 0, Resource: &InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25, Channel: 0}},
		"replay":            {Kind: SourceReplay, Channel: 1, Replay: &Replay{Trace: [][]float64{{0, 1}, {0.5, 0}}}},
	} {
		built, err := spec.Build()
		if err != nil {
			t.Fatalf("%s: Build() error = %v", name, err)
		}
		if built.Channels() != spec.Channel+1 {
			t.Fatalf("%s: built source declares %d channels, want %d", name, built.Channels(), spec.Channel+1)
		}
	}
}

func TestSourceSpecRefusesAnAmbiguousOrMismatchedDeclaration(t *testing.T) {
	timeline := &ExternalTimeline{ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: 1}}}
	for name, spec := range map[string]SourceSpec{
		"unknown kind":            {Kind: "hormone", Channel: 0, Timeline: timeline},
		"empty kind":              {Kind: "", Channel: 0, Timeline: timeline},
		"no body":                 {Kind: SourceExternalTimeline, Channel: 0},
		"two bodies":              {Kind: SourceExternalTimeline, Channel: 0, Timeline: timeline, Replay: &Replay{Trace: [][]float64{{1}}}},
		"body does not match":     {Kind: SourceReplay, Channel: 0, Timeline: timeline},
		"negative channel":        {Kind: SourceExternalTimeline, Channel: -1, Timeline: timeline},
		"width below the channel": {Kind: SourceExternalTimeline, Channel: 1, Timeline: timeline},
		"width above the channel": {Kind: SourceExternalTimeline, Channel: 0, Timeline: &ExternalTimeline{ChannelCount: 2}},
		"neural channel differs":  {Kind: SourceNeuralActivity, Channel: 0, Neural: &NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: 1}},
		"neural not declared":     {Kind: SourceNeuralActivity, Channel: 0, Neural: &NeuralActivity{Gain: 1, Channel: 0}},
		"resource channel differs": {Kind: SourceInternalResource, Channel: 0,
			Resource: &InternalResource{Resource: "energy", Coefficient: 1, Channel: 1}},
		"resource unnamed": {Kind: SourceInternalResource, Channel: 0,
			Resource: &InternalResource{Coefficient: 1, Channel: 0}},
		"resource coefficient negative": {Kind: SourceInternalResource, Channel: 0,
			Resource: &InternalResource{Resource: "energy", Coefficient: -1, Channel: 0}},
		"resource threshold not finite": {Kind: SourceInternalResource, Channel: 0,
			Resource: &InternalResource{Resource: "energy", Coefficient: 1, Threshold: math.NaN(), Channel: 0}},
		"replay trace empty":    {Kind: SourceReplay, Channel: 0, Replay: &Replay{}},
		"replay trace negative": {Kind: SourceReplay, Channel: 0, Replay: &Replay{Trace: [][]float64{{-1}}}},
		"timeline invalid":      {Kind: SourceExternalTimeline, Channel: 0, Timeline: &ExternalTimeline{ChannelCount: 1, Entries: []TimelineEntry{{Step: 0, Channel: 0, Rate: -1}}}},
	} {
		if _, err := spec.Build(); err == nil {
			t.Errorf("%s: Build() error = nil", name)
		}
	}
}

func TestChemistryConfigValidatesAgainstTheNodeCount(t *testing.T) {
	if err := fixtureConfig(t).Validate(3); err != nil {
		t.Fatalf("Validate(3) error = %v", err)
	}
	broken := map[string]func(c *ChemistryConfig){
		"one source too few":      func(c *ChemistryConfig) { c.Sources = c.Sources[:1] },
		"sources out of order":    func(c *ChemistryConfig) { c.Sources[0], c.Sources[1] = c.Sources[1], c.Sources[0] },
		"source channel mismatch": func(c *ChemistryConfig) { c.Sources[0].Channel = 1 },
		"region map too short":    func(c *ChemistryConfig) { c.Regions.NodeRegion = []int{0, 1} },
		"region out of range":     func(c *ChemistryConfig) { c.Regions.NodeRegion = []int{0, 1, 2} },
		"negative region":         func(c *ChemistryConfig) { c.Regions.NodeRegion = []int{0, 1, -1} },
		"receptor cell outside":   func(c *ChemistryConfig) { c.Receptors.Records[0].Cells = []int{3} },
		"receptor channel out":    func(c *ChemistryConfig) { c.Receptors.Records[0].Channel = 2 },
		"receptor no provenance":  func(c *ChemistryConfig) { c.Receptors.Records[0].Evidence = "" },
		"effect receptor high":    func(c *ChemistryConfig) { c.Effects[0].Receptor = 2 },
		"effect receptor negative": func(c *ChemistryConfig) {
			c.Effects[0].Receptor = -1
		},
		"effect kind unknown": func(c *ChemistryConfig) { c.Effects[0].Kind = "mood" },
		"effect reads a knob its kind never reads": func(c *ChemistryConfig) {
			c.Effects[0].ThetaScale = 1
		},
		"unknown mix":       func(c *ChemistryConfig) { c.Receptors.Mix = "average" },
		"tau not positive":  func(c *ChemistryConfig) { c.Chemistry.Tau = []float64{0, 4} },
		"tau count differs": func(c *ChemistryConfig) { c.Chemistry.Tau = []float64{2} },
		"dt not positive":   func(c *ChemistryConfig) { c.Chemistry.DT = 0 },
		"no region":         func(c *ChemistryConfig) { c.Chemistry.Regions = 0 },
		"transport row sums above one": func(c *ChemistryConfig) {
			c.Chemistry.Transport = &Transport{Fraction: [][]float64{{0, 1.5}, {0, 0}}}
		},
		"neural node outside the individual": func(c *ChemistryConfig) {
			c.Sources[1].Neural.Nodes = []int{0, 3}
		},
	}
	for name, breakIt := range broken {
		c := fixtureConfig(t)
		breakIt(&c)
		if err := c.Validate(3); err == nil {
			t.Errorf("%s: Validate(3) error = nil", name)
		}
	}
	if err := fixtureConfig(t).Validate(0); err == nil {
		t.Error("Validate(0) error = nil")
	}
	if err := fixtureConfig(t).Validate(2); err == nil {
		t.Error("Validate(2) error = nil, the region map names three nodes")
	}
}

func TestChemistryConfigValidateStateChecksShapeAndSign(t *testing.T) {
	c := fixtureConfig(t)
	good := ChemistryState{Concentration: [][]float64{{0, 1}, {2, 0}}, Steps: 7}
	if err := c.ValidateState(good); err != nil {
		t.Fatalf("ValidateState() error = %v", err)
	}
	for name, state := range map[string]ChemistryState{
		"no row":       {Concentration: nil},
		"one row":      {Concentration: [][]float64{{0, 1}}},
		"three rows":   {Concentration: [][]float64{{0, 1}, {0, 1}, {0, 1}}},
		"ragged":       {Concentration: [][]float64{{0, 1}, {0}}},
		"one channel":  {Concentration: [][]float64{{0}, {0}}},
		"negative":     {Concentration: [][]float64{{0, -1}, {0, 0}}},
		"not finite":   {Concentration: [][]float64{{0, math.Inf(1)}, {0, 0}}},
		"empty row":    {Concentration: [][]float64{{}, {}}},
		"no row again": {Concentration: [][]float64{}},
	} {
		if err := c.ValidateState(state); err == nil {
			t.Errorf("%s: ValidateState() error = nil", name)
		}
	}
}

// A declaration has to survive a save and a load unchanged, because the
// individual snapshot carries it. Strict tags mean the round trip is by name,
// so a renamed field is a decode error rather than a silent zero.
func TestChemistryConfigRoundTripsThroughJSON(t *testing.T) {
	c := fixtureConfig(t)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, key := range []string{`"chemistry"`, `"sources"`, `"receptors"`, `"effects"`, `"regions"`,
		`"node_region"`, `"kind"`, `"engineering_kd"`, `"engineering_n"`, `"timeline"`, `"neural"`} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("the encoded config has no %s key: %s", key, data)
		}
	}
	var back ChemistryConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(c, back) {
		t.Fatalf("round trip changed the config:\n got %+v\nwant %+v", back, c)
	}
	if err := back.Validate(3); err != nil {
		t.Fatalf("the decoded config no longer validates: %v", err)
	}
}

// Clone owns everything it returns: the individual holds a declaration a caller
// cannot reach into afterwards, and a snapshot cannot alias the running one.
func TestChemistryConfigCloneSharesNothing(t *testing.T) {
	c := fixtureConfig(t)
	owned := c.Clone()
	if !reflect.DeepEqual(c, owned) {
		t.Fatalf("Clone() changed the declaration:\n got %+v\nwant %+v", owned, c)
	}
	c.Chemistry.Tau[0] = 99
	c.Chemistry.Transport.Fraction[0][1] = 0.75
	c.Sources[0].Timeline.Entries[0].Rate = 99
	c.Sources[1].Neural.Nodes[0] = 99
	c.Receptors.Records[0].Cells[0] = 99
	c.Effects[0].GammaScale = 99
	c.Regions.NodeRegion[0] = 99
	if owned.Chemistry.Tau[0] != 2 || owned.Chemistry.Transport.Fraction[0][1] != 0.25 ||
		owned.Sources[0].Timeline.Entries[0].Rate != 1 || owned.Sources[1].Neural.Nodes[0] != 0 ||
		owned.Receptors.Records[0].Cells[0] != 0 || owned.Effects[0].GammaScale != 1 ||
		owned.Regions.NodeRegion[0] != 0 {
		t.Fatalf("Clone() shares a buffer with its argument: %+v", owned)
	}
}
