// Package config reads one strictly validated run configuration and expands it
// into a complete document whose every value can be traced back to the layer
// that set it.
//
// The rules the package exists to enforce are: nothing is guessed, nothing is
// substituted and nothing is hidden. An unknown field, a duplicate key, a null
// required value and a name that no implementation answers to are all refused
// rather than repaired. Four layers may set a value, in the fixed order
// default, file, environment, command line, and the expanded document names the
// winning layer for every leaf. A secret is never a value here: it is a
// reference to an environment variable, and the reference is all this package
// ever holds or prints.
package config

import (
	"context"
	"fmt"

	"github.com/TimLai666/coimnet/learning"
)

// SchemaVersion is the only configuration document version this build reads.
const SchemaVersion = "coimnet-config/v1"

// MaxConfigBytes bounds one configuration file before it is read, matching the
// limit simulate applies to a run protocol.
const MaxConfigBytes = 1 << 20

// The four layers a value can come from, in increasing precedence.
const (
	SourceDefault = "default"
	SourceFile    = "file"
	SourceEnv     = "env"
	SourceCLI     = "cli"
)

// Config is the complete run declaration. Every section is required; only
// Modulation and Teacher may be null, which declares the mechanism off rather
// than unspecified.
type Config struct {
	SchemaVersion string           `json:"schema_version"`
	Data          Data             `json:"data"`
	Model         Model            `json:"model"`
	Time          Time             `json:"time"`
	Sharing       Sharing          `json:"sharing"`
	Trainable     TrainableSection `json:"trainable"`
	Signals       Signals          `json:"signals"`
	Task          Task             `json:"task"`
	Learning      Learning         `json:"learning"`
	Modulation    *Modulation      `json:"modulation"`
	Reset         Reset            `json:"reset"`
	Teacher       *Teacher         `json:"teacher"`
	Seed          uint64           `json:"seed"`
	Device        Device           `json:"device"`
	Resources     Resources        `json:"resources"`
	Splits        Splits           `json:"splits"`
	Output        Output           `json:"output"`
}

// Data names the release and the filter that selected the graph. Both are
// declarations carried into reports; this package opens no data file.
type Data struct {
	Version string `json:"version"`
	Filter  string `json:"filter"`
}

// Model names the core kind and exactly one source file: a published model
// package or a graph store. Declaring both, or neither, is an error, because
// the framework must never choose which model a run meant.
type Model struct {
	Kind    string `json:"kind"`
	Package string `json:"package"`
	Graph   string `json:"graph"`
}

// Time declares what one step is counted in and how long it is. The unit is a
// declaration only: nothing converts it to milliseconds.
type Time struct {
	Unit string  `json:"unit"`
	Step float64 `json:"step"`
}

// Sharing selects whether every edge carries its own parameter or edges of one
// type share one.
type Sharing struct {
	ParameterSharing string `json:"parameter_sharing"`
}

// TrainableSection carries the parameter group flags and the optional per-item
// masks that narrow them.
type TrainableSection struct {
	Trainable learning.Trainable    `json:"trainable"`
	Masks     *learning.UpdateMasks `json:"masks"`
}

// Signals names the observation encoding and the output mapping.
type Signals struct {
	Encoding string `json:"encoding"`
	Mapping  string `json:"mapping"`
}

// Task names one registered generator by name and version, plus the loss.
type Task struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Loss    string `json:"loss"`
}

// Learning lists the update rules in effect and the optimizer settings. Rules
// is a subset of the registered learning rules and must not be empty: a run
// that changes nothing is declared with an empty trainable set, not with an
// empty rule list.
type Learning struct {
	Rules   []string        `json:"rules"`
	Options LearningOptions `json:"options"`
}

// LearningOptions is learning.Options without the parameter group flags and
// the per-item masks. Those two live in the Trainable section, which owns them,
// so that the document never carries the same switch twice and never leaves one
// copy of it silently ignored. Every other field keeps learning.Options' own
// name, meaning and JSON spelling.
type LearningOptions struct {
	LearningRate    float64                   `json:"learning_rate"`
	Beta1           float64                   `json:"beta1"`
	Beta2           float64                   `json:"beta2"`
	Epsilon         float64                   `json:"epsilon"`
	WeightDecay     float64                   `json:"weight_decay"`
	ClipNorm        float64                   `json:"clip_norm"`
	Truncation      int                       `json:"truncation"`
	LossScale       float64                   `json:"loss_scale"`
	AccumulateSteps int                       `json:"accumulate_steps"`
	Ranges          *learning.ParameterRanges `json:"ranges"`
	Schedule        *learning.Schedule        `json:"schedule"`
}

// Modulation declares the chemical layer. Sources are the declared releases,
// Chemistry sizes the concentration field, Receptors counts the receptor
// population and Effects names the reward mapping.
type Modulation struct {
	Sources   []ModulationSource `json:"sources"`
	Chemistry Chemistry          `json:"chemistry"`
	Receptors int                `json:"receptors"`
	Effects   Effects            `json:"effects"`
}

// ModulationSource is one declared release kind and how many channels it drives.
type ModulationSource struct {
	Kind     string `json:"kind"`
	Channels int    `json:"channels"`
}

// Chemistry sizes the concentration field as regions by channels.
type Chemistry struct {
	Regions  int `json:"regions"`
	Channels int `json:"channels"`
}

// Effects names the reward mapping that turns a raw reward into a release.
type Effects struct {
	RewardKind string `json:"reward_kind"`
}

// Reset declares what an episode boundary clears.
type Reset struct {
	NeuralAtEpisodeStart   bool `json:"neural_at_episode_start"`
	PlasticAtEpisodeStart  bool `json:"plastic_at_episode_start"`
	ChemicalAtEpisodeStart bool `json:"chemical_at_episode_start"`
}

// Teacher declares an external answer source, its budget and the reference to
// the credential it needs. Nothing in this package calls it.
type Teacher struct {
	Kind     string        `json:"kind"`
	Endpoint string        `json:"endpoint"`
	Budget   TeacherBudget `json:"budget"`
	Secret   SecretRef     `json:"secret"`
}

// TeacherBudget bounds what a run may spend on a teacher.
type TeacherBudget struct {
	MaxRequests int     `json:"max_requests"`
	MaxCost     float64 `json:"max_cost"`
}

// Device names the execution device. CPU is the only one this build runs.
type Device struct {
	Kind string `json:"kind"`
}

// Resources bounds one run. MaxMemoryMiB is checked against the pre-launch
// estimate before anything is allocated; zero declares that no plan is
// affordable, which is a way to make a dry run report the estimate and refuse.
type Resources struct {
	MaxMemoryMiB int `json:"max_memory_mib"`
	MaxTempMiB   int `json:"max_temp_mib"`
	MaxRuns      int `json:"max_runs"`
}

// Splits are fractions of the generated samples and sum to one.
type Splits struct {
	Train      float64 `json:"train"`
	Validation float64 `json:"validation"`
	Test       float64 `json:"test"`
}

// Output names the directory a real run would write into. A dry run creates
// nothing there.
type Output struct {
	Dir string `json:"dir"`
}

// Resolved is one expanded configuration: the complete document, the layer that
// set every leaf as a JSON pointer, and every secret reference it declares.
type Resolved struct {
	Config     Config
	Provenance map[string]string
	Secrets    map[string]SecretRef
}

// Defaults returns the declared default values. It is deliberately not a valid
// configuration on its own: no default can know which model file a run means or
// where it should write, so Model.Package, Model.Graph and Output.Dir are empty
// and must be supplied by a file, the environment or the command line.
func Defaults() Config {
	options := learning.DefaultOptions()
	return Config{
		SchemaVersion: SchemaVersion,
		Data:          Data{Version: "malecns-v1.0", Filter: "annotated_neurons"},
		Model:         Model{Kind: ModelKindContinuous},
		Time:          Time{Unit: TimeUnitModelStep, Step: 1},
		Sharing:       Sharing{ParameterSharing: SharingPerEdge},
		Trainable:     TrainableSection{Trainable: options.Trainable},
		Signals:       Signals{Encoding: EncodingInsyraLinear, Mapping: MappingIdentity},
		Task:          Task{Name: "delayed-correlation", Version: "v1", Loss: LossMSE},
		Learning: Learning{Rules: []string{RuleGradient}, Options: LearningOptions{
			LearningRate: options.LearningRate,
			Beta1:        options.Beta1,
			Beta2:        options.Beta2,
			Epsilon:      options.Epsilon,
			WeightDecay:  options.WeightDecay,
			ClipNorm:     options.ClipNorm,
			Truncation:   options.Truncation,
		}},
		Reset:     Reset{NeuralAtEpisodeStart: true, PlasticAtEpisodeStart: true, ChemicalAtEpisodeStart: true},
		Device:    Device{Kind: DeviceCPU},
		Resources: Resources{MaxMemoryMiB: 8192, MaxTempMiB: 40960, MaxRuns: 1},
		Splits:    Splits{Train: .8, Validation: .1, Test: .1},
	}
}

// TeacherCaller is the single call an external teacher backend has to provide.
// The real client is ticket 27; the interface lives here so that a dry run can
// be handed one and proven never to use it.
type TeacherCaller interface {
	Ask(ctx context.Context, question []float64) ([]float64, error)
}

// splitsTolerance is the rounding room allowed when checking that the three
// fractions sum to one, because decimal fractions are not exact in float64.
const splitsTolerance = 1e-9

// String renders one secret reference in the only form that is ever written.
func (s SecretRef) String() string { return fmt.Sprintf("env:%s", s.Env) }
