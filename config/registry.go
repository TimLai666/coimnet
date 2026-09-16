package config

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/TimLai666/coimnet/experiment"
	"github.com/TimLai666/coimnet/modulation"
	"github.com/TimLai666/coimnet/plasticity"
)

// The declared names this build answers to. Every one of them is backed by an
// implementation that exists today; a configuration naming anything else is
// refused, and no name is ever replaced by a neighbouring one.
const (
	ModelKindContinuous = "continuous"
	ModelKindLIF        = "lif"

	TimeUnitModelStep = "model_step"

	SharingPerEdge = "per_edge"
	SharingPerType = "per_type"

	EncodingInsyraLinear = "insyra-linear/v1"
	MappingIdentity      = "identity/v1"
	LossMSE              = "mse/v1"

	// RuleGradient is the end-to-end gradient path of learning.Trainer; the
	// other two rules are the local ones plasticity implements.
	RuleGradient = "gradient"

	DeviceCPU = "cpu"

	// TeacherKindHTTP is declared so a configuration can carry a teacher
	// through this stage. No client implements it yet: the teacher package is
	// ticket 27, and a dry run proves it is never called.
	TeacherKindHTTP = "http/v1"
)

// TaskGenerator is one registered task, resolved from the name and version a
// configuration declares. Run is the reference protocol that produces the task:
// it is the mapping from a declared name onto code that exists, so a name can
// never resolve to nothing. Stage one never calls it.
type TaskGenerator struct {
	Name        string
	Version     string
	Description string
	Run         func(ctx context.Context) (any, error)
}

// generators maps "name/version" onto the two reference protocols the
// experiment package implements today.
var generators = map[string]TaskGenerator{
	"delayed-correlation/v1": {
		Name:        "delayed-correlation",
		Version:     "v1",
		Description: "Five-step synthetic delayed pulse with three matched controls (experiment.RunDelayed)",
		Run: func(ctx context.Context) (any, error) {
			return experiment.RunDelayed(ctx, experiment.DefaultDelayedConfig())
		},
	},
	"lif-threshold/v1": {
		Name:        "lif-threshold",
		Version:     "v1",
		Description: "Five-step synthetic delayed pulse trained through a spiking base threshold (experiment.RunLIFThreshold)",
		Run: func(ctx context.Context) (any, error) {
			return experiment.RunLIFThreshold(ctx, experiment.LIFThresholdDefaultUpdates)
		},
	},
}

// The remaining registries. Each is a set of declared names with the registry
// label used in refusals, so an error always says which list was consulted.
var (
	losses          = names(LossMSE)
	encodings       = names(EncodingInsyraLinear)
	mappings        = names(MappingIdentity)
	learningRules   = names(RuleGradient, plasticity.RuleHebbianRate, plasticity.RuleSTDPPair)
	modulationKinds = names(modulation.SourceExternalTimeline, modulation.SourceNeuralActivity, modulation.SourceInternalResource, modulation.SourceReplay)
	rewardKinds     = names(modulation.KindRelu, modulation.KindSplit)
	modelKinds      = names(ModelKindContinuous, ModelKindLIF)
	devices         = names(DeviceCPU)
	sharingModes    = names(SharingPerEdge, SharingPerType)
	timeUnits       = names(TimeUnitModelStep)
	teacherKinds    = names(TeacherKindHTTP)
)

func names(entries ...string) map[string]bool {
	set := make(map[string]bool, len(entries))
	for _, entry := range entries {
		set[entry] = true
	}
	return set
}

// Generator resolves one "name/version" against the task generator registry.
func Generator(name string) (TaskGenerator, error) {
	generator, ok := generators[name]
	if !ok {
		return TaskGenerator{}, unknownName("task", name, "task generators", generatorNames())
	}
	return generator, nil
}

// GeneratorNames lists every registered task generator, sorted.
func GeneratorNames() []string { return generatorNames() }

func generatorNames() []string {
	listed := make([]string, 0, len(generators))
	for name := range generators {
		listed = append(listed, name)
	}
	sort.Strings(listed)
	return listed
}

// unknownName is the single refusal shape for every registry: it names the
// field that carried the name, the registry that was consulted and the whole
// declared list. It never proposes a replacement, because a run that silently
// used a different mechanism than the one it declared would be unreadable
// evidence.
func unknownName(field, value, registry string, declared []string) error {
	return fmt.Errorf("config: %s names %q, which the %s registry does not hold; the declared names are %s",
		field, value, registry, strings.Join(declared, ", "))
}

func requireName(field, value, registry string, set map[string]bool) error {
	if set[value] {
		return nil
	}
	return unknownName(field, value, registry, sortedNames(set))
}

func sortedNames(set map[string]bool) []string {
	listed := make([]string, 0, len(set))
	for name := range set {
		listed = append(listed, name)
	}
	sort.Strings(listed)
	return listed
}
