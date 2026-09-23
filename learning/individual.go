package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
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
	// IndividualProfileMixed is the same lifecycle on the by-type mixed core.
	// Its persistent state carries both halves, each restricted to the nodes
	// that follow that rule, plus the node map that says which is which. It is a
	// separate profile so neither single-rule reader can mistake one for the
	// other.
	IndividualProfileMixed = "mixed-f64-insyra-f32-persistent-inference-episode-learning/v1"

	// NeuralCoreContinuous, NeuralCoreLIF and NeuralCoreMixed are the declared
	// core names of a persistent neural snapshot.
	NeuralCoreContinuous = "continuous"
	NeuralCoreLIF        = "lif"
	NeuralCoreMixed      = "mixed"
)

// NeuralState is the persistent neural state of whichever core an individual
// drives. Core names it and exactly one of the three states is present, so a
// reader never has to guess which mechanism a document belongs to and a missing
// half is an error rather than a zero-valued trajectory. The mixed state is one
// member of this union, not the continuous and the spiking member together: it
// owns its own schema, its own fingerprint and the node map of the two rules.
type NeuralState struct {
	Core       string               `json:"core"`
	Continuous *dynamics.State      `json:"continuous,omitempty"`
	LIF        *dynamics.LIFState   `json:"lif,omitempty"`
	Mixed      *dynamics.MixedState `json:"mixed,omitempty"`
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
	// Episodes counts the completed TrainEpisode calls since the last
	// ResetOptimizer, the clock every consolidation write's "one write per
	// episode" rule reads. A snapshot written before this counter existed
	// decodes as zero, which is exactly the state of a fresh optimizer.
	Episodes uint64 `json:"episodes,omitempty"`
}

// PlasticPart is the local fast-change mechanism of one individual: the rule
// and edge selection it was enabled with, and the per-edge fast state that has
// accumulated since. It is absent, not zero filled, while the mechanism was
// never enabled, so a document written before it existed stays readable and a
// reader can tell "off" from "on with nothing learned yet".
type PlasticPart struct {
	Config plasticity.Config `json:"config"`
	State  plasticity.State  `json:"state"`
	// Slow is the consolidation layer of this individual, absent while the
	// layer was never enabled, exactly like the rest of the plastic part. A
	// snapshot written before the layer existed decodes with Slow nil, which is
	// the "never enabled" state, not a zero-filled one.
	Slow *SlowState `json:"slow,omitempty"`
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
	// Chemical is the chemical modulation layer, absent while it was never
	// enabled, exactly like Plastic.
	Chemical *ChemicalPart `json:"chemical,omitempty"`
}

// PlasticReport counts what one gated advance did to the fast state: how many
// core steps ran under the mechanism, how many entries the plastic_max bound
// held, and how many fixed-sign edges the w_min floor held. The two counts are
// per entry and per step, so a bound that holds the same edge on every row is
// counted on every row.
type PlasticReport struct {
	Steps int `json:"steps"`
	// Frozen is true when the rows of this advance ran with the fast state
	// held by FreezePlasticity: the effective weights were still built from it
	// but no rule update ran, so Steps, Clamped and HeldAtWMin count nothing.
	Frozen             bool `json:"frozen,omitempty"`
	Clamped            int  `json:"clamped"`
	HeldAtWMin         int  `json:"held_at_w_min"`
	GateFromReceptor   bool `json:"gate_from_receptor,omitempty"`
	WindowFromReceptor bool `json:"window_from_receptor,omitempty"`
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
	chemical   *chemicalRuntime
	// expression is the declared readout-only gain, nil while none is set. It
	// is suppressed expression, not a state change: clearing it restores the
	// untouched readout bit for bit.
	expression *ExpressionGain
	// episodes counts the completed TrainEpisode calls since the last
	// ResetOptimizer, the clock every consolidation write reads.
	episodes uint64
	// rowOverride is the per-row hook an Intervene call is running, nil while no
	// call is active. It is set outside the individual lock and read inside the
	// advance loop, so Intervene is not safe to call concurrently with other
	// mutating methods on the same individual.
	rowOverride rowOverride
}

// plasticRuntime is the enabled local mechanism of one individual: nil means
// the mechanism is off and the forward path is the one that existed before it.
type plasticRuntime struct {
	model *plasticity.Model
	state plasticity.State
	// slow is the consolidation layer, nil while it was never enabled.
	slow *SlowState
	// frozen holds the fast state constant: while it is set, the advance loop
	// still builds the effective weights out of the base parameters, this fast
	// state and the slow layer, but skips the rule update. It lives on the
	// runtime only, so a snapshot never carries it, DisablePlasticity drops it
	// with the runtime and EnablePlasticity builds a fresh un-frozen one.
	frozen bool
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
	if s.Profile != IndividualProfile && s.Profile != IndividualProfileLIF && s.Profile != IndividualProfileMixed {
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
		if err = validateSlowState(s.Plastic.Slow, runtime.model.Edges()); err != nil {
			return nil, fmt.Errorf("individual plastic part: %w", err)
		}
		runtime.state = copyPlasticState(s.Plastic.State)
		runtime.slow = copySlowState(s.Plastic.Slow)
		restored.plastic = runtime
	}
	restored.episodes = s.Optimizer.Episodes
	if s.Chemical != nil {
		runtime, err := newChemicalRuntime(s.Chemical.Config, configNodes(tr.network.config))
		if err != nil {
			return nil, fmt.Errorf("individual chemical part: %w", err)
		}
		if err = s.Chemical.Config.ValidateState(s.Chemical.State); err != nil {
			return nil, fmt.Errorf("individual chemical part: %w", err)
		}
		runtime.state = copyChemicalState(s.Chemical.State)
		for name, amount := range s.Chemical.Resources {
			if strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("individual chemical part: a resource needs a name")
			}
			if !finite(amount) {
				return nil, fmt.Errorf("individual chemical part: resource %q is %v", name, amount)
			}
			runtime.resources[name] = amount
		}
		for i, spec := range s.Chemical.PendingFeedback {
			feedback, err := signal.NewFeedback(spec)
			if err != nil {
				return nil, fmt.Errorf("individual chemical part: pending feedback %d: %w", i, err)
			}
			if spec.AvailableAt.Unit != modulation.ReleaseTimeUnit {
				return nil, fmt.Errorf("individual chemical part: pending feedback %d uses time unit %q, want %q", i, spec.AvailableAt.Unit, modulation.ReleaseTimeUnit)
			}
			runtime.pending = append(runtime.pending, feedback)
		}
		if s.Chemical.Expression != nil {
			if err := s.Chemical.Expression.Validate(tr.network.config.ReadoutNodes, len(runtime.config.Receptors.Records)); err != nil {
				return nil, fmt.Errorf("individual chemical part: %w", err)
			}
			restored.expression = copyExpression(s.Chemical.Expression)
		}
		restored.chemical = runtime
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
	// The pair rule reads a 0/1 event at both ends of an edge. Only the spiking
	// core gives every node one: a continuous node owns no event, on a mixed
	// core as much as on a continuous one, and treating its missing event as
	// "never fired" would be a silent wrong answer rather than a refusal.
	if pc.Rule.Kind == plasticity.RuleSTDPPair && c.LIF == nil {
		return nil, fmt.Errorf("rule %q needs the 0/1 events of a spiking core at both ends of every enabled edge", plasticity.RuleSTDPPair)
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
	if err := checkReceptorRule(c.Rule, i.chemical); err != nil {
		return err
	}
	i.plastic = runtime
	return nil
}

// checkReceptorRule refuses a rule that names a receptor the enabled chemistry
// does not serve. A rule with no receptor references is always fine; one that
// references a receptor without chemistry, or with fewer declared receptors than
// the rule names, would read a missing gate or window at the first step.
func checkReceptorRule(rule plasticity.Rule, chemical *chemicalRuntime) error {
	var indices []int
	if rule.GateReceptor != nil {
		indices = append(indices, *rule.GateReceptor)
	}
	if rule.DecayEReceptor != nil {
		indices = append(indices, *rule.DecayEReceptor)
	}
	if len(indices) == 0 {
		return nil
	}
	if chemical == nil {
		return fmt.Errorf("rule %q declares a receptor-driven gate or window but chemistry is not enabled", rule.Kind)
	}
	max := len(chemical.config.Receptors.Records)
	for _, idx := range indices {
		if idx < 0 || idx >= max {
			return fmt.Errorf("rule %q receptor index %d is out of range (0..%d)", rule.Kind, idx, max-1)
		}
	}
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

// FreezePlasticity holds the fast state constant on every later row of this
// individual: while frozen, each row still builds the effective weights from
// the base parameters, the held fast change and the slow layer, so the core
// integrates exactly the weights the individual carries, but the rule update is
// skipped. The whole fast state (eligibility, pair traces and fast change)
// passes through bit for bit and Steps, Clamped and HeldAtWMin count nothing,
// while the gate and the receptor-driven fields are read and reported as usual.
// This is the evaluation posture of the layer: an evaluation reads what
// training left behind without being a training step itself.
//
// The flag lives on the runtime only: it is not part of the snapshot (a
// restored individual is never frozen), DisablePlasticity drops it with the
// mechanism and EnablePlasticity starts a fresh un-frozen one. Without an
// enabled mechanism it is an error containing "plasticity".
func (i *Individual) FreezePlasticity(frozen bool) error {
	if i == nil || i.trainer == nil {
		return fmt.Errorf("uninitialized individual")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.plastic == nil {
		return fmt.Errorf("plasticity is not enabled")
	}
	i.plastic.frozen = frozen
	return nil
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
		plastic = &PlasticPart{Config: i.plastic.model.Config(), State: copyPlasticState(i.plastic.state), Slow: copySlowState(i.plastic.slow)}
	}
	var chemical *ChemicalPart
	if i.chemical != nil {
		resources := make(map[string]float64, len(i.chemical.resources))
		for name, amount := range i.chemical.resources {
			resources[name] = amount
		}
		pending := make([]signal.FeedbackSpec, len(i.chemical.pending))
		for t, feedback := range i.chemical.pending {
			pending[t] = feedback.Spec()
		}
		chemical = &ChemicalPart{
			Config:          i.chemical.config.Clone(),
			State:           copyChemicalState(i.chemical.state),
			Resources:       resources,
			PendingFeedback: pending,
			Expression:      copyExpression(i.expression),
		}
	}
	return IndividualSnapshot{IndividualVersion, i.profile, i.configHash, s.Config, s.Parameters, copyNeural(i.neural), OptimizerSnapshot{s.Options, s.Optimizer, s.Updates, s.Accumulator, i.episodes}, plastic, chemical}
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
		state    NeuralState
		outputs  [][]float64
		stepwise stepwiseResult
		report   PlasticReport
	)
	if i.plastic == nil && i.chemical == nil && i.rowOverride == nil {
		if state, outputs, _, err = n.core.advance(ctx, core, i.neural, coreInputs); err != nil {
			return nil, PlasticReport{}, err
		}
	} else {
		if stepwise, err = i.advanceStepwise(ctx, core, coreInputs, gate); err != nil {
			return nil, PlasticReport{}, err
		}
		state, outputs, report = stepwise.neural, stepwise.outputs, stepwise.plastic
	}
	// Readout-only expression: before each row's readout nodes are copied into
	// the selected tensor, each node the gain names is multiplied by
	// clamp(1 + Scale*occ, Min, Max), where occ is the mean occupancy of the
	// declared receptor on that same row. The core outputs, the concentration
	// and every other state stay exactly as the un-gained path produced them,
	// so clearing the gain restores the untouched readout bit for bit.
	gainNodes := make(map[int]bool)
	if i.expression != nil {
		for _, node := range i.expression.Nodes {
			gainNodes[node] = true
		}
	}
	selected := make([]float64, 0, len(input)*len(n.config.ReadoutNodes))
	for t, row := range outputs {
		var gain float64 = 1
		if i.expression != nil && t < len(stepwise.rowOccupancy) {
			occ := meanOccupancy(stepwise.rowOccupancy[t], i.expression.Receptor)
			gain = clamp(1+i.expression.Scale*occ, i.expression.Min, i.expression.Max)
			if gain != 1 {
				stepwise.chemistry.ExpressionGainApplied++
			}
		}
		for _, id := range n.config.ReadoutNodes {
			value := row[id]
			if gainNodes[id] {
				value *= gain
			}
			selected = append(selected, value)
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
		i.plastic.state = stepwise.fast
	}
	if i.chemical != nil {
		i.chemical.state = stepwise.chemical
		i.chemical.report = stepwise.chemistry
	}
	return rows(values, n.config.OutputSize), report, nil
}

// stepwiseResult is the candidate state of one row-by-row advance. Nothing in
// it is committed: the caller owns every field until the whole call has
// succeeded.
type stepwiseResult struct {
	neural    NeuralState
	outputs   [][]float64
	fast      plasticity.State
	plastic   PlasticReport
	chemical  modulation.ChemistryState
	chemistry ChemistryReport
	// rowOccupancy holds the occupancy records of every input row, in row
	// order, so a readout path can read the same occupancy a row's modulation
	// itself was built from. ChemistryReport.Occupancy keeps only the last
	// row's records.
	rowOccupancy [][]modulation.OccupancyRecord
}

// advanceStepwise runs the enabled mechanisms one core step at a time, because
// each of them changes what the next step integrates: a fast change moves the
// weights, and a concentration moves the input current and the effective
// threshold. Each row runs in one fixed order:
//
//	source release -> concentration -> occupancy -> effect arrays
//	-> the modulated core step -> the local plastic update
//
// With chemistry disabled no modulation is built and the core step is the
// unmodulated one, bit for bit. With plasticity disabled the plastic report
// stays at its zero value, and with the fast state frozen the row runs this
// same order without its last step: the effective weights are still built from
// the held state, the local update is not.
func (i *Individual) advanceStepwise(ctx context.Context, core Parameters, coreInputs [][]float64, gate []float64) (stepwiseResult, error) {
	var result stepwiseResult
	n := i.trainer.network
	nodes := configNodes(n.config)
	sources, targets := configEdgeEnds(n.config)
	// spiking is nil unless the core answers "does this node emit an event"
	// differently per node, which only the mixed core does.
	spiking := configSpikingNodes(n.config)
	neural := i.neural
	var fast plasticity.State
	var slow []float64
	if i.plastic != nil {
		fast = i.plastic.state
		if i.plastic.slow != nil {
			slow = i.plastic.slow.Values
		}
	}
	var activity []float64
	var base uint64
	if i.chemical != nil {
		result.chemical = i.chemical.state
		// The activity a source averages over is the node outputs of the step
		// before this one, which the persistent history already holds, and is
		// nil before any step has run.
		activity = previousActivity(i.neural)
		base = neuralSteps(i.neural)
		result.chemistry.ReleaseTotal = make([]float64, i.chemical.config.Chemistry.Channels)
	}
	outputs := make([][]float64, 0, len(coreInputs))
	for t, row := range coreInputs {
		if err := ctx.Err(); err != nil {
			return stepwiseResult{}, err
		}
		var mod *dynamics.Modulation
		var chemical chemicalRow
		if i.chemical != nil {
			var err error
			chemical, err = i.chemical.advanceOne(result.chemical, base+uint64(t), activity, nodes)
			if err != nil {
				return stepwiseResult{}, fmt.Errorf("individual chemistry: %w", err)
			}
			result.chemical, mod = chemical.state, chemical.modulated
			result.chemistry.Steps++
			result.chemistry.Occupancy = chemical.occupancy
			result.rowOccupancy = append(result.rowOccupancy, chemical.occupancy)
			result.chemistry.Assumed = chemical.summary.Assumed
			result.chemistry.UnknownSkipped = chemical.summary.UnknownSkipped
			result.chemistry.Unresponsive = chemical.summary.Unresponsive
			result.chemistry.ClampedGamma += chemical.clamped.Gamma
			result.chemistry.ClampedBeta += chemical.clamped.Beta
			result.chemistry.ClampedTheta += chemical.clamped.Theta
			for k, rate := range chemical.release {
				result.chemistry.ReleaseTotal[k] += rate
			}
		}
		step := core
		var held plasticity.ClampReport
		if i.plastic != nil {
			weights, effective, err := i.plastic.model.EffectiveWith(core.Core.Weights, n.config.EdgeSigns, fast, slow)
			if err != nil {
				return stepwiseResult{}, err
			}
			step.Core.Weights, held = weights, effective
		}
		next, values, spikes, err := n.core.advanceModulated(ctx, step, neural, [][]float64{row}, mod)
		if err != nil {
			return stepwiseResult{}, err
		}
		if i.rowOverride != nil {
			// The row override lands between the core step and the plastic
			// update, so a forced event and a forced trace are exactly what the
			// plastic rule and the output row read. Row indexes the submitted
			// input sequence of this call.
			var events []float64
			if spikes != nil {
				events = spikes[0]
			}
			if err := i.rowOverride(uint64(t), next, values[0], events); err != nil {
				return stepwiseResult{}, err
			}
		}
		if i.plastic != nil {
			rule := i.plastic.model.Config().Rule
			// The pre signal is the value the edge carries after this step,
			// which is the activated output on a continuous node and the
			// synaptic trace on a LIF node. The post signal is the 0/1 event
			// wherever the target node produces one, and the same output series
			// where it does not, so on a mixed core it is chosen per node: the
			// event on a LIF target and the output on a continuous target.
			pre, post := values[0], values[0]
			var eventsPre, eventsPost []float64
			switch {
			case spikes == nil:
				// A core that emits no event at all; post stays the output.
			case spiking == nil:
				post, eventsPre, eventsPost = spikes[0], spikes[0], spikes[0]
			default:
				blended := make([]float64, len(values[0]))
				for node := range blended {
					if spiking[node] {
						blended[node] = spikes[0][node]
					} else {
						blended[node] = values[0][node]
					}
				}
				post = blended
			}
			g := 0.0
			if gate != nil {
				g = gate[t]
			}
			gateFromReceptor := false
			if rule.GateReceptor != nil && i.chemical != nil {
				avgOcc := chemical.receptorOccupancy[*rule.GateReceptor]
				rg, ok := i.plastic.model.GateFor(avgOcc)
				if ok {
					if g != 0 {
						return stepwiseResult{}, fmt.Errorf("gate is declared by receptor %d; an explicit gate cannot be combined", *rule.GateReceptor)
					}
					g = rg
					gateFromReceptor = true
				}
			}
			windowFromReceptor := false
			var decayE *float64
			if rule.DecayEReceptor != nil && i.chemical != nil {
				avgOcc := chemical.receptorOccupancy[*rule.DecayEReceptor]
				w, ok := i.plastic.model.WindowFor(avgOcc)
				if ok {
					decayE = &w
					windowFromReceptor = true
				}
			}
			result.plastic.GateFromReceptor = result.plastic.GateFromReceptor || gateFromReceptor
			result.plastic.WindowFromReceptor = result.plastic.WindowFromReceptor || windowFromReceptor
			if i.plastic.frozen {
				// A frozen row ends here: the core step above already read the
				// effective weights of the held fast change, and skipping the
				// rule leaves the whole fast state bit-identical, so no step and
				// no bound is counted for it.
				result.plastic.Frozen = true
			} else {
				changed, stepReport, err := i.plastic.model.StepWith(fast, plasticity.StepInput{
					Pre: pre, Post: post, SpikesPre: eventsPre, SpikesPost: eventsPost,
					Gate: g, DecayE: decayE,
				}, sources, targets)
				if err != nil {
					return stepwiseResult{}, err
				}
				fast = changed
				result.plastic.Steps++
				result.plastic.Clamped += stepReport.Clamped
				result.plastic.HeldAtWMin += held.HeldAtWMin
			}
		}
		neural = next
		activity = values[0]
		outputs = append(outputs, values[0])
	}
	if i.chemical != nil {
		result.chemistry.Concentration = copyRows(result.chemical.Concentration)
	}
	result.neural, result.outputs, result.fast = neural, outputs, fast
	return result, nil
}

// TrainEpisode updates only this individual's parameters and optimizer using
// an independent zero-state episode. Persistent inference state is retained.
// This method does not propagate gradients across Advance calls. A completed
// step advances the episode clock every consolidation write reads, so the
// "one write per episode" rule sees each training episode once.
func (i *Individual) TrainEpisode(ctx context.Context, input [][]float64, target []float64) (StepResult, error) {
	if i == nil || i.trainer == nil {
		return StepResult{}, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return StepResult{}, err
	}
	defer i.mu.Unlock()
	result, err := i.trainer.Step(ctx, input, target)
	if err != nil {
		return result, err
	}
	i.episodes++
	return result, nil
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
	if o.Recompute != nil {
		if err := recomputeSupported(i.trainer.network.core); err != nil {
			return err
		}
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
	// The episode clock belongs to the trainer life this call ends, exactly like
	// the moments: a fresh optimizer starts its every-write-once-observed episode
	// count at zero.
	i.episodes = 0
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
		state := copyLIFState(*s.LIF)
		owned.LIF = &state
	}
	if s.Mixed != nil {
		state := *s.Mixed
		state.Continuous.Voltage = append([]float64(nil), state.Continuous.Voltage...)
		state.Continuous.History = copyRows(state.Continuous.History)
		state.LIF = copyLIFState(state.LIF)
		state.Index.ContinuousNodes = append([]int(nil), state.Index.ContinuousNodes...)
		state.Index.LIFNodes = append([]int(nil), state.Index.LIFNodes...)
		owned.Mixed = &state
	}
	return owned
}

// copyLIFState deep copies one spiking half, keeping an absent slow stabiliser
// absent rather than turning it into an empty array.
func copyLIFState(s dynamics.LIFState) dynamics.LIFState {
	s.Voltage = append([]float64(nil), s.Voltage...)
	s.History = copyRows(s.History)
	s.Adaptation = append([]float64(nil), s.Adaptation...)
	s.Refractory = append([]int(nil), s.Refractory...)
	s.Rate = append([]float64(nil), s.Rate...)
	s.Homeostasis = append([]float64(nil), s.Homeostasis...)
	return s
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
