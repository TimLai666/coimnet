package checkpoint

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestChemicalEpisodeValidationAcceptsCaseRenamedWireFields records the
// encoding/json contract used by the checkpoint wire: a single field whose
// spelling differs only by case still decodes to the tagged field. The
// payload checksum is recomputed for every document, so a passing case proves
// that the renamed field was actually read rather than that corruption was
// ignored.
func TestChemicalEpisodeValidationAcceptsCaseRenamedWireFields(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(*testing.T, map[string]json.RawMessage)
	}{
		{
			name: "Chemistry",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, _ := chemicalEpisodeValidationMaps(t, payload)
				renameChemicalEpisodeValidationKey(t, config, "chemistry", "Chemistry")
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "PART",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				renameChemicalEpisodeValidationKey(t, chemistry, "part", "PART")
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Source_States",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				renameChemicalEpisodeValidationKey(t, part, "source_states", "Source_States")
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Controller",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				states := rawArray(t, part["source_states"])
				state := rawObject(t, states[0])
				renameChemicalEpisodeValidationKey(t, state, "controller", "Controller")
				states[0] = mustRawJSON(t, state)
				part["source_states"] = mustRawJSON(t, states)
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Expression",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				renameChemicalEpisodeValidationKey(t, part, "expression", "Expression")
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "INITIAL_ACTIVITY",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				renameChemicalEpisodeValidationKey(t, chemistry, "initial_activity", "INITIAL_ACTIVITY")
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := chemicalEpisodeCheckpointMutation(t, fixture.doc, tc.mutate)
			got, err := loadChemicalEpisodeMutation(t, changed)
			if err != nil {
				t.Fatalf("case-renamed chemical checkpoint was rejected: %v", err)
			}
			if !reflect.DeepEqual(got, fixture.state) {
				t.Fatalf("case-renamed checkpoint changed state:\n got %#v\nwant %#v", got, fixture.state)
			}
		})
	}
}

// TestChemicalEpisodeValidationRejectsMissingFieldsUnderCaseRenamedParents
// covers the mismatch between the raw required-field walk and the
// case-insensitive JSON decoder. Each document starts from the saved legal
// fixture and carries a fresh checksum, then removes a field that must not
// silently become a zero value.
func TestChemicalEpisodeValidationRejectsMissingFieldsUnderCaseRenamedParents(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(*testing.T, map[string]json.RawMessage)
	}{
		{
			name: "Chemistry missing source_step",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				delete(chemistry, "source_step")
				delete(config, "chemistry")
				config["Chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "PART missing resources",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				delete(part, "resources")
				delete(chemistry, "part")
				chemistry["PART"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "resources energy null",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				resources := rawObject(t, part["resources"])
				resources["energy"] = json.RawMessage(`null`)
				part["resources"] = mustRawJSON(t, resources)
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Source_States controller window",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				states := rawArray(t, part["source_states"])
				state := rawObject(t, states[0])
				controller := rawObject(t, state["controller"])
				delete(controller, "window")
				state["controller"] = mustRawJSON(t, controller)
				states[0] = mustRawJSON(t, state)
				delete(part, "source_states")
				part["Source_States"] = mustRawJSON(t, states)
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Controller filled",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				states := rawArray(t, part["source_states"])
				state := rawObject(t, states[0])
				controller := rawObject(t, state["controller"])
				delete(controller, "filled")
				delete(state, "controller")
				state["Controller"] = mustRawJSON(t, controller)
				states[0] = mustRawJSON(t, state)
				part["source_states"] = mustRawJSON(t, states)
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Expression required nodes",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				expression := rawObject(t, part["expression"])
				delete(expression, "nodes")
				delete(part, "expression")
				part["Expression"] = mustRawJSON(t, expression)
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "Receptor Location required mapping_version",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				part := rawObject(t, chemistry["part"])
				partConfig := rawObject(t, part["config"])
				receptors := rawObject(t, partConfig["receptors"])
				records := rawArray(t, receptors["records"])
				record := rawObject(t, records[0])
				location := rawObject(t, record["location"])
				delete(location, "mapping_version")
				delete(record, "location")
				record["Location"] = mustRawJSON(t, location)
				records[0] = mustRawJSON(t, record)
				receptors["records"] = mustRawJSON(t, records)
				partConfig["receptors"] = mustRawJSON(t, receptors)
				part["config"] = mustRawJSON(t, partConfig)
				chemistry["part"] = mustRawJSON(t, part)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
		{
			name: "INITIAL_ACTIVITY null element",
			mutate: func(t *testing.T, payload map[string]json.RawMessage) {
				training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
				activity := []json.RawMessage{mustRawJSON(t, 0.21), json.RawMessage(`null`)}
				delete(chemistry, "initial_activity")
				chemistry["INITIAL_ACTIVITY"] = mustRawJSON(t, activity)
				config["chemistry"] = chemistryValue(t, chemistry)
				chemicalEpisodeValidationCommit(t, payload, training, config)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := chemicalEpisodeCheckpointMutation(t, fixture.doc, tc.mutate)
			if _, err := loadChemicalEpisodeMutation(t, changed); err == nil {
				t.Fatalf("accepted a case-renamed checkpoint with missing or null required data: %s", tc.name)
			}
		})
	}
}

// TestChemicalEpisodeValidationAllowsDeclaredOptionalNulls distinguishes
// optional nil pointers/slices from required values. The complete fixture is
// read before each mutation, and the changed payload keeps its valid checksum.
func TestChemicalEpisodeValidationAllowsDeclaredOptionalNulls(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(*testing.T, map[string]json.RawMessage)
		check  func(*testing.T, State)
	}{
		{
			name: "expression pointer",
			mutate: mutateChemicalPart(func(part map[string]json.RawMessage) {
				part["expression"] = json.RawMessage(`null`)
			}),
			check: func(t *testing.T, state State) {
				if state.Training.Config.Chemistry.Part.Expression != nil {
					t.Fatal("null optional expression decoded as a non-nil value")
				}
			},
		},
		{
			name: "initial activity slice",
			mutate: mutateChemical(func(chemistry map[string]json.RawMessage) {
				chemistry["initial_activity"] = json.RawMessage(`null`)
			}),
			check: func(t *testing.T, state State) {
				if state.Training.Config.Chemistry.InitialActivity != nil {
					t.Fatal("null optional initial_activity decoded as a non-nil value")
				}
			},
		},
		{
			name: "receptor location pointer",
			mutate: mutateChemicalRecord(func(record map[string]json.RawMessage) {
				record["location"] = json.RawMessage(`null`)
			}),
			check: func(t *testing.T, state State) {
				location := state.Training.Config.Chemistry.Part.Config.Receptors.Records[0].Location
				if location != nil {
					t.Fatal("null optional receptor location decoded as a non-nil value")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := chemicalEpisodeCheckpointMutation(t, fixture.doc, tc.mutate)
			got, err := loadChemicalEpisodeMutation(t, changed)
			if err != nil {
				t.Fatalf("supported optional null was rejected: %v", err)
			}
			tc.check(t, got)
		})
	}
}

// TestChemicalEpisodeValidationUnicodeRoundTripAndRejectsUnpairedSurrogates
// protects both sides of the JSON boundary. Valid Unicode remains data, while
// an unpaired UTF-16 surrogate must be rejected before encoding/json could
// replace it with U+FFFD.
func TestChemicalEpisodeValidationUnicodeRoundTripAndRejectsUnpairedSurrogates(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)

	validUnicode := chemicalEpisodeCheckpointMutation(t, fixture.doc, func(t *testing.T, payload map[string]json.RawMessage) {
		training, config, chemistry := chemicalEpisodeValidationMaps(t, payload)
		part := rawObject(t, chemistry["part"])
		resources := rawObject(t, part["resources"])
		energy := resources["energy"]
		delete(resources, "energy")
		resources["能量🧪"] = energy
		part["resources"] = mustRawJSON(t, resources)

		partConfig := rawObject(t, part["config"])
		sources := rawArray(t, partConfig["sources"])
		source := rawObject(t, sources[0])
		controller := rawObject(t, source["controller"])
		inputs := rawObject(t, controller["inputs"])
		inputs["resources"] = mustRawJSON(t, []string{"能量🧪"})
		controller["inputs"] = mustRawJSON(t, inputs)
		source["controller"] = mustRawJSON(t, controller)
		sources[0] = mustRawJSON(t, source)
		partConfig["sources"] = mustRawJSON(t, sources)
		part["config"] = mustRawJSON(t, partConfig)

		records := rawArray(t, rawObject(t, partConfig["receptors"])["records"])
		record := rawObject(t, records[0])
		location := rawObject(t, record["location"])
		location["evidence"] = mustRawJSON(t, "證據🧪")
		location["anatomical_location"] = mustRawJSON(t, "神經元／區域")
		record["location"] = mustRawJSON(t, location)
		records[0] = mustRawJSON(t, record)
		receptors := rawObject(t, partConfig["receptors"])
		receptors["records"] = mustRawJSON(t, records)
		partConfig["receptors"] = mustRawJSON(t, receptors)
		part["config"] = mustRawJSON(t, partConfig)
		chemistry["part"] = mustRawJSON(t, part)
		config["chemistry"] = chemistryValue(t, chemistry)
		chemicalEpisodeValidationCommit(t, payload, training, config)
	})
	loaded, err := loadChemicalEpisodeMutation(t, validUnicode)
	if err != nil {
		t.Fatalf("valid Unicode chemical checkpoint was rejected: %v", err)
	}
	chemistry := loaded.Training.Config.Chemistry
	if chemistry == nil {
		t.Fatal("Unicode round trip lost chemistry")
	}
	if chemistry.Part.Resources["能量🧪"] != 1.25 {
		t.Fatalf("Unicode resource key did not round trip: %#v", chemistry.Part.Resources)
	}
	if !reflect.DeepEqual(chemistry.Part.Config.Sources[0].Controller.Inputs.Resources, []string{"能量🧪"}) {
		t.Fatalf("Unicode controller resource name did not round trip: %#v", chemistry.Part.Config.Sources[0].Controller.Inputs.Resources)
	}
	location := chemistry.Part.Config.Receptors.Records[0].Location
	if location == nil || location.Evidence != "證據🧪" || location.AnatomicalLocation != "神經元／區域" {
		t.Fatalf("Unicode location values did not round trip: %#v", location)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, map[string]json.RawMessage)
	}{
		{
			name: "location evidence",
			mutate: mutateChemicalLocation(func(location map[string]json.RawMessage) {
				location["evidence"] = json.RawMessage(`"\ud800"`)
			}),
		},
		{
			name: "location anatomical_location",
			mutate: mutateChemicalLocation(func(location map[string]json.RawMessage) {
				location["anatomical_location"] = json.RawMessage(`"\ud800"`)
			}),
		},
	} {
		t.Run("reject unpaired surrogate "+tc.name, func(t *testing.T) {
			changed := chemicalEpisodeCheckpointMutation(t, fixture.doc, tc.mutate)
			if _, err := loadChemicalEpisodeMutation(t, changed); err == nil {
				t.Fatalf("accepted an unpaired surrogate in %s", tc.name)
			}
		})
	}
}

// TestChemicalEpisodeValidationRejectsInvalidProducerResourcesBeforePublish
// verifies the producer side separately from hand-edited reader documents.
// Save must reject ambiguous or invalid UTF-8 resource names before publishing
// a path that another process could later fail to load.
func TestChemicalEpisodeValidationRejectsInvalidProducerResourcesBeforePublish(t *testing.T) {
	fixture := newChemicalEpisodeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(map[string]float64)
	}{
		{
			name: "energy and Energy",
			mutate: func(resources map[string]float64) {
				resources["Energy"] = 1.25
			},
		},
		{
			name: "two invalid UTF8 names",
			mutate: func(resources map[string]float64) {
				resources[string([]byte{0xc3, 0x28})] = 0.5
				resources[string([]byte{0xff, 0xfe})] = 0.25
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resources := make(map[string]float64, len(fixture.state.Training.Config.Chemistry.Part.Resources)+2)
			for name, amount := range fixture.state.Training.Config.Chemistry.Part.Resources {
				resources[name] = amount
			}
			tc.mutate(resources)
			candidate := chemicalEpisodeValidationStateWithResources(fixture.state, resources)
			before := fixture.state
			dir := t.TempDir()
			path := filepath.Join(dir, "invalid-producer.json")
			if err := Save(context.Background(), path, candidate); err == nil {
				t.Fatal("Save accepted a resource map that cannot be published as a safely reloadable checkpoint")
			}
			if _, err := os.Stat(path); err == nil {
				t.Fatal("Save published a path after rejecting invalid producer resources")
			} else if !os.IsNotExist(err) {
				t.Fatalf("stat rejected checkpoint path: %v", err)
			}
			assertNoTemps(t, dir, filepath.Base(path))
			if !reflect.DeepEqual(fixture.state, before) {
				t.Fatal("Save mutated the caller's valid chemistry snapshot")
			}
		})
	}
}

func chemicalEpisodeValidationMaps(t *testing.T, payload map[string]json.RawMessage) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage) {
	t.Helper()
	training := rawObject(t, payload["training"])
	config := rawObject(t, training["config"])
	chemistry := rawObject(t, config["chemistry"])
	return training, config, chemistry
}

func chemicalEpisodeValidationCommit(t *testing.T, payload, training, config map[string]json.RawMessage) {
	t.Helper()
	training["config"] = mustRawJSON(t, config)
	payload["training"] = mustRawJSON(t, training)
}

func chemistryValue(t *testing.T, chemistry map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	return mustRawJSON(t, chemistry)
}

func renameChemicalEpisodeValidationKey(t *testing.T, object map[string]json.RawMessage, old, new string) {
	t.Helper()
	raw, ok := object[old]
	if !ok {
		t.Fatalf("JSON object has no %q key", old)
	}
	if _, exists := object[new]; exists {
		t.Fatalf("JSON object already has %q key", new)
	}
	delete(object, old)
	object[new] = raw
}

func chemicalEpisodeValidationStateWithResources(base State, resources map[string]float64) State {
	candidate := base
	training := base.Training
	config := training.Config
	episode := *config.Chemistry
	part := episode.Part
	part.Resources = resources
	episode.Part = part
	config.Chemistry = &episode
	training.Config = config
	candidate.Training = training
	return candidate
}
