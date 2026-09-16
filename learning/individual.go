package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
)

const (
	// IndividualVersion separates persistent individuals from episode snapshots.
	IndividualVersion = "coimnet-individual/v1"
	// IndividualProfile fixes solver, numeric precision and state lifecycles.
	// Neural inference persists; gradient training uses independent zero-state
	// episodes. This deterministic profile has no random generator, local
	// plasticity, chemical state, replay store or external teacher.
	IndividualProfile = "continuous-f64-insyra-f32-persistent-inference-episode-learning/v1"
	// IndividualProfileLIF is the same lifecycle on the spiking core. Its
	// persistent state adds the synaptic trace, the adaptation, the refractory
	// counters and, when declared, the slow stabiliser; it is a separate profile
	// so a continuous reader can never mistake one for the other.
	IndividualProfileLIF = "lif-f64-insyra-f32-persistent-inference-episode-learning/v1"

	// NeuralCoreContinuous and NeuralCoreLIF are the declared core names of a
	// persistent neural snapshot.
	NeuralCoreContinuous = "continuous"
	NeuralCoreLIF        = "lif"
)

// NeuralState is the persistent neural state of whichever core an individual
// drives. Core names it and exactly one of the two states is present, so a
// reader never has to guess which mechanism a document belongs to and a missing
// half is an error rather than a zero-valued trajectory.
type NeuralState struct {
	Core       string             `json:"core"`
	Continuous *dynamics.State    `json:"continuous,omitempty"`
	LIF        *dynamics.LIFState `json:"lif,omitempty"`
}

// OptimizerSnapshot contains trainer state without anatomy or model parameters.
// Its arrays follow the parameter order documented by AdamState.
type OptimizerSnapshot struct {
	Options Options   `json:"options"`
	State   AdamState `json:"state"`
	Updates uint64    `json:"updates"`
	// Accumulator is the open gradient-accumulation window, present only when
	// Options.AccumulateSteps is above one and the window is partly filled.
	// Without it a persistent individual saved in the middle of a window would
	// lose the gradients that window already holds; a trainer that applies
	// every step never opens one, so the key is absent and every snapshot
	// written before this field existed stays byte-identical.
	Accumulator *GradientAccumulator `json:"accumulator,omitempty"`
}

// PlasticPart is the local fast-change mechanism of one individual: the rule
// and edge selection it was enabled with, and the per-edge fast state that has
// accumulated since. It is absent, not zero filled, while the mechanism was
// never enabled, so a document written before it existed stays readable and a
// reader can tell "off" from "on with nothing learned yet".
type PlasticPart struct {
	Config plasticity.Config `json:"config"`
	State  plasticity.State  `json:"state"`
}

// IndividualSnapshot owns four separately serializable parts at one completed
// operation boundary. ConfigHash is SHA-256 of canonical Config JSON, including
// input and readout selections. It identifies the computational topology, not
// biological provenance; keep the source connectome.Graph and its receipts.
// It contains no data-provider cursor and is not an external training-job backup.
type IndividualSnapshot struct {
	SchemaVersion string            `json:"schema_version"`
	Profile       string            `json:"profile"`
	ConfigHash    string            `json:"config_hash"`
	Config        Config            `json:"config"`
	Parameters    Parameters        `json:"parameters"`
	Neural        NeuralState       `json:"neural"`
	Optimizer     OptimizerSnapshot `json:"optimizer"`
	Plastic       *PlasticPart      `json:"plastic,omitempty"`
}

// PlasticReport counts what one gated advance did to the fast state: how many
// core steps ran under the mechanism, how many entries the plastic_max bound
// held, and how many fixed-sign edges the w_min floor held. The two counts are
// per entry and per step, so a bound that holds the same edge on every row is
// counted on every row.
type PlasticReport struct {
	Steps      int `json:"steps"`
	Clamped    int `json:"clamped"`
	HeldAtWMin int `json:"held_at_w_min"`
}

// Individual owns mutable neural state and an isolated episode trainer.
// Methods serialize at complete operation boundaries. Construction, snapshots,
// resets and outputs do not retain caller-owned mutable buffers. Callers must
// not mutate method arguments concurrently with a call.
// The zero value is not initialized; use NewIndividual or RestoreIndividual.
type Individual struct {
	mu         cancellableMutex
	trainer    *Trainer
	neural     NeuralState
	profile    string
	configHash string
	plastic    *plasticRuntime
}

// plasticRuntime is the enabled local mechanism of one individual: nil means
// the mechanism is off and the forward path is the one that existed before it.
type plasticRuntime struct {
	model *plasticity.Model
	state plasticity.State
}

// NewIndividual creates an independent copy of a base model, a fresh optimizer
// and persistent neural state at initial voltage. Anatomy is immutable; changing
// connectivity requires creating a new individual. This does not mutate c or p.
func NewIndividual(c Config, p Parameters, o Options, initial []float64) (*Individual, error) {
	tr, err := NewTrainer(c, p, o)
	if err != nil {
		return nil, err
	}
	core := tr.network.core
	state, err := core.newState(initial)
	if err != nil {
		return nil, err
	}
	hash, err := individualConfigHash(tr.network.Config())
	if err != nil {
		return nil, err
	}
	return &Individual{trainer: tr, neural: state, profile: core.profile(), configHash: hash}, nil
}

// RestoreIndividual validates all parts before creating an isolated individual.
// Unknown profiles or missing delayed history are errors, never zero-filled.
func RestoreIndividual(s IndividualSnapshot) (*Individual, error) {
	if s.SchemaVersion != IndividualVersion {
		return nil, fmt.Errorf("unsupported individual schema %q", s.SchemaVersion)
	}
	if s.Profile != IndividualProfile && s.Profile != IndividualProfileLIF {
		return nil, fmt.Errorf("unsupported individual profile %q", s.Profile)
	}
	// The window is routed through RestoreTrainer rather than validated here,
	// so the individual and the episode trainer accept exactly the same
	// windows: RestoreTrainer owns validateAccumulator.
	tr, err := RestoreTrainer(TrainingSnapshot{SchemaVersion: "coimnet-episode-training/v1", Config: s.Config, Parameters: s.Parameters, Options: s.Optimizer.Options, Optimizer: s.Optimizer.State, Updates: s.Optimizer.Updates, Accumulator: s.Optimizer.Accumulator})
	if err != nil {
		return nil, err
	}
	hash, err := individualConfigHash(tr.network.Config())
	if err != nil {
		return nil, err
	}
	if s.ConfigHash != hash {
		return nil, fmt.Errorf("individual configuration fingerprint mismatch")
	}
	core := tr.network.core
	if s.Profile != core.profile() {
		return nil, fmt.Errorf("individual profile %q does not match the configured core", s.Profile)
	}
	if err = core.validateState(s.Neural); err != nil {
		return nil, fmt.Errorf("individual neural state: %w", err)
	}
	restored := &Individual{trainer: tr, neural: copyNeural(s.Neural), profile: s.Profile, configHash: hash}
	if s.Plastic != nil {
		runtime, err := newPlasticRuntime(tr.network.config, s.Plastic.Config)
		if err != nil {
			return nil, fmt.Errorf("individual plastic part: %w", err)
		}
		if err = runtime.model.ValidateState(s.Plastic.State); err != nil {
			return nil, fmt.Errorf("individual plastic part: %w", err)
		}
		runtime.state = copyPlasticState(s.Plastic.State)
		restored.plastic = runtime
	}
	return restored, nil
}

// newPlasticRuntime validates one declaration against the configured core. A
// pair rule is refused on a core that emits no events, so the refusal happens
// where the core is known rather than at the first step.
func newPlasticRuntime(c Config, pc plasticity.Config) (*plasticRuntime, error) {
	model, err := plasticity.New(pc, configEdges(c))
	if err != nil {
		return nil, err
	}
	if pc.Rule.Kind == plasticity.RuleSTDPPair && c.LIF == nil {
		return nil, fmt.Errorf("rule %q needs the 0/1 events of a spiking core", plasticity.RuleSTDPPair)
	}
	return &plasticRuntime{model: model, state: model.NewState()}, nil
}

// EnablePlasticity turns the local fast-change mechanism on for this
// individual. The edge selection is validated against this individual's own
// topology, and a second call replaces the declaration and restarts the fast
// state, because eligibility and fast changes accumulated under a different
// rule or a different edge set do not describe the new one. The mechanism lives
// on the persistent individual only: an independent training episode has no
// access to it, so there is no way to enable it on a non-persistent path.
func (i *Individual) EnablePlasticity(c plasticity.Config) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	runtime, err := newPlasticRuntime(i.trainer.network.config, c)
	if err != nil {
		return err
	}
	i.plastic = runtime
	return nil
}

// DisablePlasticity turns the mechanism off and drops the fast state, so the
// forward path returns bit for bit to the one an individual that never enabled
// it produces. Base parameters were never changed by it and are unaffected.
func (i *Individual) DisablePlasticity() {
	if i == nil || i.trainer == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.plastic = nil
}

// Snapshot returns an independent copy of all four parts at a completed
// operation boundary. A nil or zero Individual returns IndividualSnapshot{}.
func (i *Individual) Snapshot() IndividualSnapshot {
	if i == nil || i.trainer == nil {
		return IndividualSnapshot{}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	s := i.trainer.Snapshot()
	var plastic *PlasticPart
	if i.plastic != nil {
		plastic = &PlasticPart{Config: i.plastic.model.Config(), State: copyPlasticState(i.plastic.state)}
	}
	return IndividualSnapshot{IndividualVersion, i.profile, i.configHash, s.Config, s.Parameters, copyNeural(i.neural), OptimizerSnapshot{s.Options, s.Optimizer, s.Updates, s.Accumulator}, plastic}
}

// Advance consumes observations using persistent voltage and delayed output
// history, returning each step's readout. It changes only neural state and,
// when local plasticity is enabled, that individual's fast state. Insyra
// encodes observations and decodes selected core outputs with float32 tensors.
// A failed or canceled call commits no steps. Inputs must be nonempty.
//
// With plasticity enabled this is AdvanceGated with a gate of zero on every
// row: the eligibility and the pair traces keep tracking activity, and the
// fast changes only decay. With plasticity disabled it is the path that
// existed before the mechanism and is bit-identical to it.
func (i *Individual) Advance(ctx context.Context, input [][]float64) ([][]float64, error) {
	out, _, err := i.advanceRows(ctx, input, nil)
	return out, err
}

// AdvanceGated is Advance with an explicit learning gate: gate carries one
// value per input row, supplied by the caller, so it can be a constant, a
// window or a pulse that arrives several steps after the activity it rewards.
// Each row runs in the one declared order: the core integrates the effective
// weights of the current fast state, the eligibility and the pair traces take
// that step's pre and post signals, the gate turns the eligibility into a
// bounded fast change, and the next row integrates the new effective weights.
// Fast changes never reach Parameters; a gradient step changes only those.
//
// With plasticity disabled the call is the unchanged fast path and reports
// nothing, but an open gate is then an error rather than a silent no-op.
func (i *Individual) AdvanceGated(ctx context.Context, input [][]float64, gate []float64) ([][]float64, PlasticReport, error) {
	if gate == nil {
		return nil, PlasticReport{}, fmt.Errorf("AdvanceGated needs one gate value per input row")
	}
	return i.advanceRows(ctx, input, gate)
}

func (i *Individual) advanceRows(ctx context.Context, input [][]float64, gate []float64) ([][]float64, PlasticReport, error) {
	if i == nil || i.trainer == nil {
		return nil, PlasticReport{}, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return nil, PlasticReport{}, err
	}
	defer i.mu.Unlock()
	n, p := i.trainer.network, i.trainer.parameters
	if len(input) == 0 {
		return nil, PlasticReport{}, fmt.Errorf("empty input sequence")
	}
	if gate != nil {
		if len(gate) != len(input) {
			return nil, PlasticReport{}, fmt.Errorf("gate has %d values, the input has %d rows", len(gate), len(input))
		}
		for t, g := range gate {
			if !finite(g) {
				return nil, PlasticReport{}, fmt.Errorf("gate[%d] is not finite", t)
			}
			if g != 0 && i.plastic == nil {
				return nil, PlasticReport{}, fmt.Errorf("gate[%d] is %g but local plasticity is not enabled", t, g)
			}
		}
	}
	count, err := size(len(input), n.config.InputSize)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	nodes := configNodes(n.config)
	for _, width := range []int{n.config.InputSize, nodes, inputWidth(n.config), len(n.config.ReadoutNodes), n.config.OutputSize} {
		cells, err := size(len(input), width)
		if err != nil {
			return nil, PlasticReport{}, err
		}
		if cells > dynamics.MaxStateValues {
			return nil, PlasticReport{}, fmt.Errorf("individual call exceeds %d values per matrix", dynamics.MaxStateValues)
		}
	}
	flat := make([]float64, 0, count)
	for t, row := range input {
		if err := ctx.Err(); err != nil {
			return nil, PlasticReport{}, err
		}
		if len(row) != n.config.InputSize {
			return nil, PlasticReport{}, fmt.Errorf("input[%d] width %d, want %d", t, len(row), n.config.InputSize)
		}
		flat = append(flat, row...)
	}
	x, err := tensor([]int{len(input), n.config.InputSize}, flat)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	enc, err := tensor([]int{n.config.InputSize, inputWidth(n.config)}, p.Encoder)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	z, err := nn.MatMul(x, enc)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	encoded := rows(doubles(z.Data()), inputWidth(n.config))
	coreInputs := encoded
	if n.config.InputNodes != nil {
		coreInputs = make([][]float64, len(input))
		for t, row := range encoded {
			coreInputs[t] = make([]float64, nodes)
			for j, id := range n.config.InputNodes {
				coreInputs[t][id] = row[j]
			}
		}
	}
	core, err := n.coreParameters(p)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	var (
		state   NeuralState
		outputs [][]float64
		fast    plasticity.State
		report  PlasticReport
	)
	if i.plastic == nil {
		if state, outputs, _, err = n.core.advance(ctx, core, i.neural, coreInputs); err != nil {
			return nil, PlasticReport{}, err
		}
	} else if state, outputs, fast, report, err = i.advancePlastic(ctx, core, coreInputs, gate); err != nil {
		return nil, PlasticReport{}, err
	}
	selected := make([]float64, 0, len(input)*len(n.config.ReadoutNodes))
	for _, row := range outputs {
		for _, id := range n.config.ReadoutNodes {
			selected = append(selected, row[id])
		}
	}
	h, err := tensor([]int{len(input), len(n.config.ReadoutNodes)}, selected)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	r, err := tensor([]int{len(n.config.ReadoutNodes), n.config.OutputSize}, p.Readout)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	result, err := nn.MatMul(h, r)
	if err != nil {
		return nil, PlasticReport{}, err
	}
	values := doubles(result.Data())
	for _, v := range values {
		if !finite(v) {
			return nil, PlasticReport{}, fmt.Errorf("non-finite individual readout")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, PlasticReport{}, err
	}
	i.neural = state
	if i.plastic != nil {
		i.plastic.state = fast
	}
	return rows(values, n.config.OutputSize), report, nil
}

// advancePlastic runs the enabled mechanism one core step at a time, because
// the weights the next step integrates depend on the fast change this step
// produced. Nothing is committed here: the caller owns the candidate neural
// and fast states until the whole call has succeeded.
func (i *Individual) advancePlastic(ctx context.Context, core Parameters, coreInputs [][]float64, gate []float64) (NeuralState, [][]float64, plasticity.State, PlasticReport, error) {
	var report PlasticReport
	n := i.trainer.network
	sources, targets := configEdgeEnds(n.config)
	neural, fast := i.neural, i.plastic.state
	outputs := make([][]float64, 0, len(coreInputs))
	for t, row := range coreInputs {
		if err := ctx.Err(); err != nil {
			return NeuralState{}, nil, plasticity.State{}, PlasticReport{}, err
		}
		weights, held, err := i.plastic.model.Effective(core.Core.Weights, n.config.EdgeSigns, fast)
		if err != nil {
			return NeuralState{}, nil, plasticity.State{}, PlasticReport{}, err
		}
		step := core
		step.Core.Weights = weights
		next, values, spikes, err := n.core.advance(ctx, step, neural, [][]float64{row})
		if err != nil {
			return NeuralState{}, nil, plasticity.State{}, PlasticReport{}, err
		}
		// The pre signal is the value the edge carries after this step, which
		// is the activated output on the continuous core and the synaptic
		// trace on the spiking one. The post signal is the 0/1 event where the
		// core produces one, and the same output where it does not.
		pre, post := values[0], values[0]
		var eventsPre, eventsPost []float64
		if spikes != nil {
			post, eventsPre, eventsPost = spikes[0], spikes[0], spikes[0]
		}
		g := 0.0
		if gate != nil {
			g = gate[t]
		}
		changed, stepReport, err := i.plastic.model.Step(fast, pre, post, eventsPre, eventsPost, g, sources, targets)
		if err != nil {
			return NeuralState{}, nil, plasticity.State{}, PlasticReport{}, err
		}
		neural, fast = next, changed
		outputs = append(outputs, values[0])
		report.Steps++
		report.Clamped += stepReport.Clamped
		report.HeldAtWMin += held.HeldAtWMin
	}
	return neural, outputs, fast, report, nil
}

// TrainEpisode updates only this individual's parameters and optimizer using
// an independent zero-state episode. Persistent inference state is retained.
// This method does not propagate gradients across Advance calls.
func (i *Individual) TrainEpisode(ctx context.Context, input [][]float64, target []float64) (StepResult, error) {
	if i == nil || i.trainer == nil {
		return StepResult{}, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return StepResult{}, err
	}
	defer i.mu.Unlock()
	return i.trainer.Step(ctx, input, target)
}

// ResetNeural starts a new neural trajectory only. Parameters, optimizer and
// immutable anatomy remain unchanged; errors leave the previous trajectory.
func (i *Individual) ResetNeural(ctx context.Context, initial []float64) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return err
	}
	defer i.mu.Unlock()
	state, err := i.trainer.network.core.newState(initial)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	i.neural = state
	return nil
}

// ResetParameters replaces model parameters only, preserving neural state and
// optimizer moments/counts. Call ResetOptimizer explicitly to clear moments.
// Connectivity and tensor shapes must match this individual.
func (i *Individual) ResetParameters(ctx context.Context, p Parameters) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return err
	}
	defer i.mu.Unlock()
	candidate, err := NewTrainer(i.trainer.network.Config(), p, i.trainer.options)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	i.trainer.parameters = candidate.parameters
	return nil
}

// ResetOptimizer replaces optimizer options and clears its moments, step counts
// and any open gradient-accumulation window. It does not reset parameters,
// persistent neural state or anatomy.
func (i *Individual) ResetOptimizer(ctx context.Context, o Options) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return err
	}
	defer i.mu.Unlock()
	if err := validateOptions(o); err != nil {
		return err
	}
	if err := validateTrainable(o.Trainable, i.trainer.network.core.theta()); err != nil {
		return err
	}
	if err := validateMasks(o.Masks, i.trainer.network.core.nodes(), i.trainer.network.core.edges()); err != nil {
		return err
	}
	if err := validateRangesAgainstSigns(i.trainer.network.config, o.Ranges); err != nil {
		return err
	}
	count := len(i.trainer.optimizer.First)
	state := AdamState{make([]float64, count), make([]float64, count), make([]uint64, count)}
	if err := ctx.Err(); err != nil {
		return err
	}
	i.trainer.options = copyOptions(o)
	i.trainer.optimizer = state
	i.trainer.updates = 0
	// The open accumulation window belongs to the optimizer this call replaces:
	// keeping it would average gradients taken under the previous options into
	// the first update of the new one.
	i.trainer.accumulator = nil
	return nil
}

func individualConfigHash(c Config) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// copyNeural deep copies whichever half of the union is present, so a snapshot
// never shares a buffer with the running individual.
func copyNeural(s NeuralState) NeuralState {
	owned := NeuralState{Core: s.Core}
	if s.Continuous != nil {
		state := *s.Continuous
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.History = copyRows(state.History)
		owned.Continuous = &state
	}
	if s.LIF != nil {
		state := *s.LIF
		state.Voltage = append([]float64(nil), state.Voltage...)
		state.History = copyRows(state.History)
		state.Adaptation = append([]float64(nil), state.Adaptation...)
		state.Refractory = append([]int(nil), state.Refractory...)
		state.Rate = append([]float64(nil), state.Rate...)
		state.Homeostasis = append([]float64(nil), state.Homeostasis...)
		owned.LIF = &state
	}
	return owned
}

// copyPlasticState deep copies the fast state, keeping an absent pair trace
// absent rather than turning it into an empty array.
func copyPlasticState(s plasticity.State) plasticity.State {
	owned := plasticity.State{
		Eligibility: append([]float64(nil), s.Eligibility...),
		Plastic:     append([]float64(nil), s.Plastic...),
	}
	if s.PreTrace != nil {
		owned.PreTrace = append([]float64(nil), s.PreTrace...)
	}
	if s.PostTrace != nil {
		owned.PostTrace = append([]float64(nil), s.PostTrace...)
	}
	return owned
}

func copyRows(rows [][]float64) [][]float64 {
	if rows == nil {
		return nil
	}
	owned := make([][]float64, len(rows))
	for t, row := range rows {
		owned[t] = append([]float64(nil), row...)
	}
	return owned
}
