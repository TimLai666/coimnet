package checkpoint

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/TimLai666/coimnet/learning"
)

// normalizeEpisodeTrainingSnapshot owns and normalizes the optional chemical
// episode's required wire arrays and resource map. The ordinary nil-chemistry
// snapshot is unchanged. Runtime restoration validates semantic boundaries.
func normalizeEpisodeTrainingSnapshot(s learning.TrainingSnapshot) learning.TrainingSnapshot {
	if s.Config.Chemistry == nil {
		return s
	}
	episode := *s.Config.Chemistry
	part := normalizeChemicalPart(episode.Part)
	if part.SourceStates != nil {
		states := make([]learning.ChemicalSourceState, len(part.SourceStates))
		for index, state := range part.SourceStates {
			states[index] = state
			if state.Controller != nil {
				controller := *state.Controller
				controller.Window = nonNilFloatRows(controller.Window)
				states[index].Controller = &controller
			}
		}
		part.SourceStates = states
	}
	if part.Expression != nil {
		expression := *part.Expression
		expression.Nodes = nonNilInts(expression.Nodes)
		part.Expression = &expression
	}
	episode.Part = part
	if episode.InitialActivity != nil {
		episode.InitialActivity = nonNilFloats(episode.InitialActivity)
	}
	s.Config.Chemistry = &episode
	return s
}

// checkEpisodeRequiredFields checks the ordinary training fields plus the
// optional chemical boundary. A present boundary must preserve source clock,
// required chemical state, controller windows and readout expression. Missing,
// null, unknown, conflicting or corrupt fields are rejected before restoration.
// The chemical resource map uses the existing chemical-part shape validator.
func checkEpisodeRequiredFields(data json.RawMessage) error {
	var payloadRaw map[string]json.RawMessage
	if err := json.Unmarshal(data, &payloadRaw); err != nil || payloadRaw == nil {
		return checkRequiredFields(data, reflect.TypeOf(State{}), "payload")
	}
	payload, err := presenceObject(payloadRaw)
	if err != nil {
		return err
	}
	trainingRaw, ok := jsonField(payload, "training")
	if !ok {
		return checkRequiredFields(data, reflect.TypeOf(State{}), "payload")
	}
	var trainingRawObject map[string]json.RawMessage
	if err := json.Unmarshal(trainingRaw, &trainingRawObject); err != nil || trainingRawObject == nil {
		return checkRequiredFields(data, reflect.TypeOf(State{}), "payload")
	}
	training, err := presenceObject(trainingRawObject)
	if err != nil {
		return err
	}
	configRaw, ok := jsonField(training, "config")
	if !ok {
		return checkRequiredFields(data, reflect.TypeOf(State{}), "payload")
	}
	var configRawObject map[string]json.RawMessage
	if err := json.Unmarshal(configRaw, &configRawObject); err != nil || configRawObject == nil {
		return checkRequiredFields(data, reflect.TypeOf(State{}), "payload")
	}
	config, err := presenceObject(configRawObject)
	if err != nil {
		return err
	}
	chemistryRaw, ok := jsonField(config, "chemistry")
	if !ok || isJSONNull(chemistryRaw) {
		return checkRequiredFields(data, reflect.TypeOf(State{}), "payload")
	}
	if err := checkEpisodeChemistry(chemistryRaw); err != nil {
		return err
	}
	delete(config, "chemistry")
	configRaw, err = json.Marshal(config)
	if err != nil {
		return fmt.Errorf("checkpoint chemistry validation copy: %w", err)
	}
	training["config"] = configRaw
	payload["training"], err = json.Marshal(training)
	if err != nil {
		return fmt.Errorf("checkpoint chemistry validation copy: %w", err)
	}
	validation, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("checkpoint chemistry validation copy: %w", err)
	}
	return checkRequiredFields(validation, reflect.TypeOf(State{}), "payload")
}

func checkEpisodeChemistry(data json.RawMessage) error {
	const path = "$.payload.training.config.chemistry"
	chemistry, err := requiredObject(data, path, "part", "source_step")
	if err != nil {
		return err
	}
	if isJSONNull(chemistry["source_step"]) {
		return fmt.Errorf("%s.source_step is null", path)
	}
	if raw, present := chemistry["initial_activity"]; present && !isJSONNull(raw) {
		if err := checkArrayElements(raw, reflect.TypeOf(float64(0)), path+".initial_activity"); err != nil {
			return err
		}
	}
	part, err := requiredObject(chemistry["part"], path+".part")
	if err != nil {
		return err
	}
	if err := requireIndividualChemical(map[string]json.RawMessage{"chemical": chemistry["part"]}); err != nil {
		return err
	}
	if raw, present := part["source_states"]; present && !isJSONNull(raw) {
		var states []json.RawMessage
		if err := json.Unmarshal(raw, &states); err != nil {
			return fmt.Errorf("%s.source_states is not a JSON array: %w", path+".part", err)
		}
		for index, stateRaw := range states {
			statePath := fmt.Sprintf("%s.part.source_states[%d]", path, index)
			state, err := requiredObject(stateRaw, statePath)
			if err != nil {
				return err
			}
			controllerRaw, present := state["controller"]
			if !present || isJSONNull(controllerRaw) {
				continue
			}
			controllerPath := statePath + ".controller"
			controller, err := requiredObject(controllerRaw, controllerPath, "window", "filled")
			if err != nil {
				return err
			}
			for _, field := range []string{"window", "filled"} {
				if isJSONNull(controller[field]) {
					return fmt.Errorf("%s.%s is null", controllerPath, field)
				}
			}
		}
	}
	if raw, present := part["expression"]; present && !isJSONNull(raw) {
		if err := checkRequiredFields(raw, reflect.TypeOf(learning.ExpressionGain{}), path+".part.expression"); err != nil {
			return err
		}
	}
	return nil
}
