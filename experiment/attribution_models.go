package experiment

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

// The seven attribution groups of ticket 28's capacity-attribution comparison
// (TSK-12), in report order.
const (
	// AttributionNormal trains core weights, encoder and readout: the full
	// core and periphery of the navigation policy.
	AttributionNormal = "normal"
	// AttributionFrozenCore freezes the core weights and trains only the
	// periphery, so its result isolates how much the periphery alone learns.
	AttributionFrozenCore = "frozen_core"
	// AttributionCoreOnly fixes a low-capacity periphery (identity encoder,
	// identity readout rows) and trains only the core weights, the proof that
	// the core itself learns.
	AttributionCoreOnly = "core_only"
	// AttributionGenericMatched is a random recurrent topology with the same
	// edge and node counts as the normal group, so a similar parameter count
	// cannot be mistaken for this wiring.
	AttributionGenericMatched = "generic_matched"
	// AttributionRewired degree-preservingly rewires the normal group's
	// hidden-to-hidden edges and publishes the rewire check list.
	AttributionRewired = "rewired"
	// AttributionAblatedRetrained removes the core entirely and trains a
	// readout-side periphery alone, the mirror of frozen_core.
	AttributionAblatedRetrained = "ablated_retrained"
	// AttributionCapacityMatchedModulator is the normal group plus a readout
	// add-on carrying the ablation controller's parameter count with no
	// modulatory role, holding capacity constant across the modulation check.
	AttributionCapacityMatchedModulator = "capacity_matched_modulator"
)

// AttributionGroups returns the seven groups in report order (the order of
// the constants above), as a fresh slice.
func AttributionGroups() []string {
	return []string{
		AttributionNormal,
		AttributionFrozenCore,
		AttributionCoreOnly,
		AttributionGenericMatched,
		AttributionRewired,
		AttributionAblatedRetrained,
		AttributionCapacityMatchedModulator,
	}
}

// AttributionModel records what one group is and trains. Parameters is the
// total array length (weights + bias + log_tau + encoder + readout);
// FreeParameters is the subset the group's Trainable list covers (bias and
// log_tau never train). ModulatorParameters and AddOnParameters are set only
// for capacity_matched_modulator and Rewire only for rewired, so a report can
// read the whole comparison from this table alone.
type AttributionModel struct {
	Group               string        `json:"group"`
	Nodes               int           `json:"nodes"`
	Edges               int           `json:"edges"`
	Parameters          int           `json:"parameters"`                     // len of weights + bias + log_tau + encoder + readout
	FreeParameters      int           `json:"free_parameters"`                // entries of the groups listed in Trainable
	Trainable           []string      `json:"trainable"`                      // subset of "weights", "encoder", "readout", in that order (bias never trains)
	ModulatorParameters int           `json:"modulator_parameters,omitempty"` // capacity_matched_modulator only: ablationController(seed, 4).ParameterCount()
	AddOnParameters     int           `json:"add_on_parameters,omitempty"`    // capacity_matched_modulator only: k x nav2d.Actions
	Rewire              *RewireReport `json:"rewire,omitempty"`               // rewired only
}

// newAttributionPolicy builds one attribution group's policy as a nav2dPolicy
// of kind Nav2DRecurrent with settle 3, so act and train behave exactly as
// for the recurrent policy. I = inputs, H = hidden, A = nav2d.Actions.
// Unless a group says otherwise: nodes 0..I-1 are the inputs, I..I+H-1 the
// hidden block and I+H..I+H+A-1 the readout nodes; the edges come from
// nav2dEdges(seed, I, H, recurrent, true); InputSize is I, InputNodes the
// inputs and the encoder the I x I identity; ReadoutNodes are the H hidden
// nodes followed by the A readout nodes and the readout is (H+A) x A with
// zero hidden rows and the readout-node rows from
// nav2dUniform(seed, 2, A x A); OutputSize is A with ReadoutEveryStep true;
// weights are uniform [-0.3, 0.3] from nav2dUniform(seed, 0, E); Bias is 0
// and LogTau log(2), both never trained; Options are DefaultOptions at rate
// with the group's Trainable list. The returned AttributionModel carries the
// counts, the trainable list and, for rewired, the rewire report.
func newAttributionPolicy(group string, seed uint64, inputs, hidden, recurrent int, rate float64) (*nav2dPolicy, AttributionModel, error) {
	var model AttributionModel
	switch group {
	case AttributionNormal, AttributionFrozenCore, AttributionCoreOnly,
		AttributionGenericMatched, AttributionRewired, AttributionAblatedRetrained,
		AttributionCapacityMatchedModulator:
	default:
		return nil, model, fmt.Errorf("unknown attribution group %q", group)
	}
	if inputs < 1 || hidden < 1 {
		return nil, model, fmt.Errorf("attribution group %q needs at least 1 input and 1 hidden node, got %d and %d", group, inputs, hidden)
	}
	if recurrent < 1 || recurrent > hidden {
		return nil, model, fmt.Errorf("attribution group %q needs 1 <= recurrent <= hidden, got %d with hidden %d", group, recurrent, hidden)
	}
	if rate <= 0 {
		return nil, model, fmt.Errorf("attribution group %q needs a positive learning rate, got %v", group, rate)
	}
	actions := nav2d.Actions
	var request learning.Trainable
	switch group {
	case AttributionNormal, AttributionGenericMatched, AttributionRewired, AttributionCapacityMatchedModulator:
		request = learning.Trainable{Weights: true, Encoder: true, Readout: true}
	case AttributionFrozenCore, AttributionAblatedRetrained:
		request = learning.Trainable{Encoder: true, Readout: true}
	case AttributionCoreOnly:
		request = learning.Trainable{Weights: true}
	}
	model.Trainable = attributionTrainableNames(request)

	if group == AttributionAblatedRetrained {
		p, m, err := newAttributionAblated(seed, inputs, actions, rate, request)
		m.Trainable = model.Trainable
		return p, m, err
	}

	sources, targets := nav2dEdges(seed, inputs, hidden, recurrent, true)
	switch group {
	case AttributionGenericMatched:
		sources, targets = attributionGenericEdges(seed, inputs, hidden, len(sources))
	case AttributionRewired:
		rewiredTargets, report, err := nav2dRewire(sources, targets, inputs, hidden, seed)
		if err != nil {
			return nil, model, fmt.Errorf("attribution group %q: %w", group, err)
		}
		targets = rewiredTargets
		model.Rewire = &report
	}

	nodeCount := inputs + hidden + actions
	encoder := make([]float64, inputs*inputs)
	for i := 0; i < inputs; i++ {
		encoder[i*inputs+i] = 1
	}
	readout := make([]float64, (hidden+actions)*actions)
	if group == AttributionCoreOnly {
		// The periphery is fixed and low-capacity: readout node j votes for
		// action j, the hidden rows stay zero.
		for j := 0; j < actions; j++ {
			readout[(hidden+j)*actions+j] = 1
		}
	} else {
		actRows := nav2dUniform(seed, 2, actions*actions)
		for o := 0; o < actions; o++ {
			copy(readout[(hidden+o)*actions:], actRows[o*actions:(o+1)*actions])
		}
	}
	inputNodes := make([]int, inputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, 0, hidden+actions)
	for h := 0; h < hidden; h++ {
		readoutNodes = append(readoutNodes, inputs+h)
	}
	for o := 0; o < actions; o++ {
		readoutNodes = append(readoutNodes, inputs+hidden+o)
	}
	if group == AttributionCapacityMatchedModulator {
		controller, err := ablationController(seed, 4)
		if err != nil {
			return nil, model, fmt.Errorf("attribution group %q: %w", group, err)
		}
		modulatorParams := controller.ParameterCount()
		k := (modulatorParams + actions - 1) / actions
		if k > inputs {
			return nil, model, fmt.Errorf("attribution group %q: add-on width k = %d exceeds the %d inputs", group, k, inputs)
		}
		model.ModulatorParameters = modulatorParams
		model.AddOnParameters = k * actions
		readout = append(readout, make([]float64, k*actions)...)
		for i := 0; i < k; i++ {
			readoutNodes = append(readoutNodes, i)
		}
	}

	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodeCount, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        inputs,
		OutputSize:       actions,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodeCount)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: nav2dUniform(seed, 0, len(sources)), Bias: make([]float64, nodeCount), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	options := learning.DefaultOptions()
	options.LearningRate = rate
	options.Trainable = request
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return nil, model, err
	}
	model.Group = group
	model.Nodes = nodeCount
	model.Edges = len(sources)
	model.Parameters = len(parameters.Core.Weights) + len(parameters.Core.Bias) + len(parameters.Core.LogTau) + len(parameters.Encoder) + len(parameters.Readout)
	model.FreeParameters = attributionFreeCount(request, parameters)
	return &nav2dPolicy{kind: Nav2DRecurrent, trainer: trainer, settle: 3, edges: len(sources)}, model, nil
}

// newAttributionAblated builds the ablated_retrained group: the core is
// removed, so the A action nodes are both the input and the readout nodes,
// there are no edges, the encoder is I x A, the readout A x A, and Bias and
// LogTau live on the A nodes. Only encoder and readout train.
func newAttributionAblated(seed uint64, inputs, actions int, rate float64, request learning.Trainable) (*nav2dPolicy, AttributionModel, error) {
	var model AttributionModel
	nodeCount := actions
	inputNodes := make([]int, nodeCount)
	readoutNodes := make([]int, nodeCount)
	for i := 0; i < nodeCount; i++ {
		inputNodes[i] = i
		readoutNodes[i] = i
	}
	encoder := nav2dUniform(seed, 1, inputs*nodeCount)
	readout := nav2dUniform(seed, 2, nodeCount*nodeCount)
	logTau := make([]float64, nodeCount)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodeCount, DT: 1, Activation: "tanh"},
		InputSize:        inputs,
		OutputSize:       nodeCount,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: nil, Bias: make([]float64, nodeCount), LogTau: logTau},
		Encoder: encoder,
		Readout: readout,
	}
	options := learning.DefaultOptions()
	options.LearningRate = rate
	options.Trainable = request
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return nil, model, err
	}
	model.Group = AttributionAblatedRetrained
	model.Nodes = nodeCount
	model.Edges = 0
	model.Parameters = len(parameters.Core.Bias) + len(parameters.Core.LogTau) + len(parameters.Encoder) + len(parameters.Readout)
	model.FreeParameters = attributionFreeCount(request, parameters)
	return &nav2dPolicy{kind: Nav2DRecurrent, trainer: trainer, settle: 3, edges: 0}, model, nil
}

// attributionGenericEdges draws count distinct (source, target) pairs from
// rand.New(rand.NewPCG(seed, 7)): the source uniform in [0, inputs+hidden)
// and the target uniform in [inputs, inputs+hidden+Actions), redrawing a
// repeated pair. The draw space never has fewer pairs than count, because
// count = E and E <= (inputs+hidden) x (hidden+Actions) for recurrent <=
// hidden, so the loop terminates.
func attributionGenericEdges(seed uint64, inputs, hidden, count int) (sources, targets []int) {
	r := rand.New(rand.NewPCG(seed, 7))
	seen := make(map[[2]int]bool, count)
	sources = make([]int, 0, count)
	targets = make([]int, 0, count)
	for len(sources) < count {
		s := r.IntN(inputs + hidden)
		t := inputs + r.IntN(hidden+nav2d.Actions)
		key := [2]int{s, t}
		if seen[key] {
			continue
		}
		seen[key] = true
		sources = append(sources, s)
		targets = append(targets, t)
	}
	return sources, targets
}

// attributionTrainableNames lists a Trainable request as the model's JSON
// order "weights", "encoder", "readout". Bias and tau never appear.
func attributionTrainableNames(request learning.Trainable) []string {
	var names []string
	if request.Weights {
		names = append(names, "weights")
	}
	if request.Encoder {
		names = append(names, "encoder")
	}
	if request.Readout {
		names = append(names, "readout")
	}
	return names
}

// attributionFreeCount sums the array lengths of the trainable groups.
func attributionFreeCount(request learning.Trainable, parameters learning.Parameters) int {
	free := 0
	if request.Weights {
		free += len(parameters.Core.Weights)
	}
	if request.Encoder {
		free += len(parameters.Encoder)
	}
	if request.Readout {
		free += len(parameters.Readout)
	}
	return free
}
