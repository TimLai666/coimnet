package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/HazelnutParadise/insyra/nn"
	"github.com/TimLai666/coimnet/dynamics"
)

const (
	// IndividualVersion separates persistent individuals from episode snapshots.
	IndividualVersion = "coimnet-individual/v1"
	// IndividualProfile fixes solver, numeric precision and state lifecycles.
	// Neural inference persists; gradient training uses independent zero-state
	// episodes. This deterministic profile has no random generator, local
	// plasticity, chemical state, replay store or external teacher.
	IndividualProfile = "continuous-f64-insyra-f32-persistent-inference-episode-learning/v1"
)

// OptimizerSnapshot contains trainer state without anatomy or model parameters.
// Its arrays follow the parameter order documented by AdamState.
type OptimizerSnapshot struct {
	Options Options   `json:"options"`
	State   AdamState `json:"state"`
	Updates uint64    `json:"updates"`
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
	Neural        dynamics.State    `json:"neural"`
	Optimizer     OptimizerSnapshot `json:"optimizer"`
}

// Individual owns mutable neural state and an isolated episode trainer.
// Methods serialize at complete operation boundaries. Construction, snapshots,
// resets and outputs do not retain caller-owned mutable buffers. Callers must
// not mutate method arguments concurrently with a call.
// The zero value is not initialized; use NewIndividual or RestoreIndividual.
type Individual struct {
	mu         cancellableMutex
	trainer    *Trainer
	neural     dynamics.State
	configHash string
}

// NewIndividual creates an independent copy of a base model, a fresh optimizer
// and persistent neural state at initial voltage. Anatomy is immutable; changing
// connectivity requires creating a new individual. This does not mutate c or p.
func NewIndividual(c Config, p Parameters, o Options, initial []float64) (*Individual, error) {
	tr, err := NewTrainer(c, p, o)
	if err != nil {
		return nil, err
	}
	state, err := tr.network.core.NewState(initial)
	if err != nil {
		return nil, err
	}
	hash, err := individualConfigHash(tr.network.Config())
	if err != nil {
		return nil, err
	}
	return &Individual{trainer: tr, neural: state, configHash: hash}, nil
}

// RestoreIndividual validates all parts before creating an isolated individual.
// Unknown profiles or missing delayed history are errors, never zero-filled.
func RestoreIndividual(s IndividualSnapshot) (*Individual, error) {
	if s.SchemaVersion != IndividualVersion {
		return nil, fmt.Errorf("unsupported individual schema %q", s.SchemaVersion)
	}
	if s.Profile != IndividualProfile {
		return nil, fmt.Errorf("unsupported individual profile %q", s.Profile)
	}
	tr, err := RestoreTrainer(TrainingSnapshot{SchemaVersion: "coimnet-episode-training/v1", Config: s.Config, Parameters: s.Parameters, Options: s.Optimizer.Options, Optimizer: s.Optimizer.State, Updates: s.Optimizer.Updates})
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
	if err = tr.network.core.ValidateState(s.Neural); err != nil {
		return nil, fmt.Errorf("individual neural state: %w", err)
	}
	return &Individual{trainer: tr, neural: copyNeural(s.Neural), configHash: hash}, nil
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
	return IndividualSnapshot{IndividualVersion, IndividualProfile, i.configHash, s.Config, s.Parameters, copyNeural(i.neural), OptimizerSnapshot{s.Options, s.Optimizer, s.Updates}}
}

// Advance consumes observations using persistent voltage and delayed output
// history, returning each step's readout. It changes only neural state. Insyra
// encodes observations and decodes selected core outputs with float32 tensors.
// A failed or canceled call commits no steps. Inputs must be nonempty.
func (i *Individual) Advance(ctx context.Context, input [][]float64) ([][]float64, error) {
	if i == nil || i.trainer == nil {
		return nil, fmt.Errorf("uninitialized individual")
	}
	if err := i.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer i.mu.Unlock()
	n, p := i.trainer.network, i.trainer.parameters
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input sequence")
	}
	count, err := size(len(input), n.config.InputSize)
	if err != nil {
		return nil, err
	}
	for _, width := range []int{n.config.InputSize, n.config.Dynamics.Nodes, inputWidth(n.config), len(n.config.ReadoutNodes), n.config.OutputSize} {
		cells, err := size(len(input), width)
		if err != nil {
			return nil, err
		}
		if cells > dynamics.MaxStateValues {
			return nil, fmt.Errorf("individual call exceeds %d values per matrix", dynamics.MaxStateValues)
		}
	}
	flat := make([]float64, 0, count)
	for t, row := range input {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(row) != n.config.InputSize {
			return nil, fmt.Errorf("input[%d] width %d, want %d", t, len(row), n.config.InputSize)
		}
		flat = append(flat, row...)
	}
	x, err := tensor([]int{len(input), n.config.InputSize}, flat)
	if err != nil {
		return nil, err
	}
	enc, err := tensor([]int{n.config.InputSize, inputWidth(n.config)}, p.Encoder)
	if err != nil {
		return nil, err
	}
	z, err := nn.MatMul(x, enc)
	if err != nil {
		return nil, err
	}
	encoded := rows(doubles(z.Data()), inputWidth(n.config))
	coreInputs := encoded
	if n.config.InputNodes != nil {
		coreInputs = make([][]float64, len(input))
		for t, row := range encoded {
			coreInputs[t] = make([]float64, n.config.Dynamics.Nodes)
			for j, id := range n.config.InputNodes {
				coreInputs[t][id] = row[j]
			}
		}
	}
	state, outputs, err := n.core.Advance(ctx, p.Core, i.neural, coreInputs)
	if err != nil {
		return nil, err
	}
	selected := make([]float64, 0, len(input)*len(n.config.ReadoutNodes))
	for _, row := range outputs {
		for _, id := range n.config.ReadoutNodes {
			selected = append(selected, row[id])
		}
	}
	h, err := tensor([]int{len(input), len(n.config.ReadoutNodes)}, selected)
	if err != nil {
		return nil, err
	}
	r, err := tensor([]int{len(n.config.ReadoutNodes), n.config.OutputSize}, p.Readout)
	if err != nil {
		return nil, err
	}
	result, err := nn.MatMul(h, r)
	if err != nil {
		return nil, err
	}
	values := doubles(result.Data())
	for _, v := range values {
		if !finite(v) {
			return nil, fmt.Errorf("non-finite individual readout")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	i.neural = state
	return rows(values, n.config.OutputSize), nil
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
	state, err := i.trainer.network.core.NewState(initial)
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

// ResetOptimizer replaces optimizer options and clears its moments/counts only.
// It does not reset parameters, persistent neural state or anatomy.
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
	count := len(i.trainer.optimizer.First)
	state := AdamState{make([]float64, count), make([]float64, count), make([]uint64, count)}
	if err := ctx.Err(); err != nil {
		return err
	}
	i.trainer.options = o
	i.trainer.optimizer = state
	i.trainer.updates = 0
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
func copyNeural(s dynamics.State) dynamics.State {
	s.Voltage = append([]float64(nil), s.Voltage...)
	history := make([][]float64, len(s.History))
	for t, row := range s.History {
		history[t] = append([]float64(nil), row...)
	}
	s.History = history
	return s
}
