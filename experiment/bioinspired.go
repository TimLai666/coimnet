package experiment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	// ProtocolEcdysoneInspired models timing: when a pulse arrives relative
	// to learning changes consolidation.
	ProtocolEcdysoneInspired = "ecdysone_inspired"
	// ProtocolNPFMemoryExpression models state: expression suppressed by a
	// receptor-driven readout gain, not forgotten.
	ProtocolNPFMemoryExpression = "npf_memory_expression_hypothesis"
	// BioInspiredSchemaVersion identifies the bio-inspired protocol report.
	BioInspiredSchemaVersion = "coimnet-bio-inspired/v1"
)

// QuantitativeEvidence is the only way a protocol may be named after the
// biological quantity itself ("ecdysone", "npf"): all four items must be
// present, otherwise the quantitative name is refused.
type QuantitativeEvidence struct {
	Method              string `json:"method"`
	Data                string `json:"data"`
	Fit                 string `json:"fit"`
	HeldOutIntervention string `json:"held_out_intervention"`
}

// BioInspiredConfig is the preregistered configuration of a bio-inspired
// intervention protocol.
type BioInspiredConfig struct {
	Protocol     string                `json:"protocol"`
	Seeds        []uint64              `json:"seeds"`
	Episodes     int                   `json:"episodes"`
	PulseSteps   []int                 `json:"pulse_steps"`
	Suppressed   float64               `json:"suppressed"`
	Tolerance    float64               `json:"tolerance"`
	Quantitative *QuantitativeEvidence `json:"quantitative,omitempty"`
	Registry     []string              `json:"registry"`
}

// ErrQuantitativeNaming reports a biological quantity name used without the
// evidence that would make the quantitative claim verifiable.
var ErrQuantitativeNaming = errors.New("experiment: a quantitative biological name needs Method, Data, Fit and HeldOutIntervention")

// Validate rejects a protocol whose name, timing grid or field constraints
// could make the intervention undefined or its claim unverifiable.
func (c BioInspiredConfig) Validate() error {
	switch c.Protocol {
	case ProtocolEcdysoneInspired, ProtocolNPFMemoryExpression:
		// the two allowed hypothesis names.
	case "":
		return fmt.Errorf("protocol: unsupported protocol %q", c.Protocol)
	default:
		if strings.Contains(c.Protocol, "ecdysone") || strings.Contains(c.Protocol, "npf") {
			if !c.quantitativeComplete() {
				return ErrQuantitativeNaming
			}
		} else {
			return fmt.Errorf("protocol: unsupported protocol %q", c.Protocol)
		}
	}

	if len(c.Seeds) < 3 {
		return fmt.Errorf("seeds: at least 3 required, got %d", len(c.Seeds))
	}
	seenSeeds := make(map[uint64]bool, len(c.Seeds))
	for _, seed := range c.Seeds {
		if seenSeeds[seed] {
			return fmt.Errorf("seeds: duplicate seed %d", seed)
		}
		seenSeeds[seed] = true
	}

	if c.Episodes < 1 || c.Episodes > 10000 {
		return fmt.Errorf("episodes: must be between 1 and 10000, got %d", c.Episodes)
	}

	switch c.Protocol {
	case ProtocolEcdysoneInspired:
		if err := c.checkPulseSteps(); err != nil {
			return err
		}
		if c.Suppressed != 0 {
			return fmt.Errorf("suppressed: must be 0 for %s, got %g", ProtocolEcdysoneInspired, c.Suppressed)
		}
		if c.Tolerance != 0 {
			return fmt.Errorf("tolerance: must be 0 for %s, got %g", ProtocolEcdysoneInspired, c.Tolerance)
		}
	case ProtocolNPFMemoryExpression:
		if len(c.PulseSteps) != 0 {
			return fmt.Errorf("pulse_steps: must be empty for %s", ProtocolNPFMemoryExpression)
		}
		if c.Suppressed <= 0 || c.Suppressed >= 1 {
			return fmt.Errorf("suppressed: must be strictly between 0 and 1, got %g", c.Suppressed)
		}
		if c.Tolerance <= 0 {
			return fmt.Errorf("tolerance: must be > 0, got %g", c.Tolerance)
		}
	}

	required := ""
	switch c.Protocol {
	case ProtocolEcdysoneInspired:
		required = "S14"
	case ProtocolNPFMemoryExpression:
		required = "S15"
	}
	if required != "" {
		found := false
		for _, id := range c.Registry {
			if id == required {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("registry: must contain %q for %s", required, c.Protocol)
		}
	}
	return nil
}

func (c BioInspiredConfig) quantitativeComplete() bool {
	q := c.Quantitative
	return q != nil &&
		strings.TrimSpace(q.Method) != "" &&
		strings.TrimSpace(q.Data) != "" &&
		strings.TrimSpace(q.Fit) != "" &&
		strings.TrimSpace(q.HeldOutIntervention) != ""
}

// checkPulseSteps verifies the ecdysone_inspired pulse grid.
func (c BioInspiredConfig) checkPulseSteps() error {
	if len(c.PulseSteps) == 0 {
		return fmt.Errorf("pulse_steps: at least one entry required for %s", ProtocolEcdysoneInspired)
	}
	seen := make(map[int]bool, len(c.PulseSteps))
	for _, step := range c.PulseSteps {
		if step < 0 {
			return fmt.Errorf("pulse_steps: value %d must be >= 0", step)
		}
		if seen[step] {
			return fmt.Errorf("pulse_steps: duplicate value %d", step)
		}
		seen[step] = true
	}
	return nil
}

// RegistryIDs returns the evidence ids declared for the protocol (sorted,
// distinct).
func (c BioInspiredConfig) RegistryIDs() []string {
	set := make(map[string]bool, len(c.Registry))
	for _, id := range c.Registry {
		set[id] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// assumptions returns the fixed two-sentence disclaimers for the protocol:
// engineering numbers, not a quantitative biological reproduction.
func (c BioInspiredConfig) assumptions() []string {
	suffix := "The timing curve reports differences between pulse timings only; it is not a dose-response claim."
	if c.Protocol == ProtocolNPFMemoryExpression {
		suffix = "A score that drops while expression is suppressed and recovers when the state is switched back shows expression, not forgetting."
	}
	return []string{"Engineering numbers under an allowed hypothesis name; this is not a quantitative biological reproduction.", suffix}
}

// BioInspiredReport is filled by the next tickets; the shared header fields
// fix the JSON shape now.
type BioInspiredReport struct {
	SchemaVersion string            `json:"schema_version"`
	Protocol      string            `json:"protocol"`
	Config        BioInspiredConfig `json:"config"`
	ConfigHash    string            `json:"config_hash"`
	Registry      []string          `json:"registry"`
	Assumptions   []string          `json:"assumptions"`
	Seeds         []BioInspiredSeed `json:"seeds"`
}

// BioInspiredSeed is one seed's outcome: a failed run keeps its error, a
// timing protocol fills Curve and an expression protocol fills Expression.
type BioInspiredSeed struct {
	Seed       uint64             `json:"seed"`
	Failed     bool               `json:"failed"`
	Error      string             `json:"error,omitempty"`
	Curve      []BioInspiredPoint `json:"curve,omitempty"`
	Expression *ExpressionResult  `json:"expression,omitempty"`
}

// BioInspiredPoint is one pulse timing of the ecdysone_inspired curve.
type BioInspiredPoint struct {
	PulseStep     int     `json:"pulse_step"`
	SlowMagnitude float64 `json:"slow_magnitude"`
	RetestScore   float64 `json:"retest_score"`
}

// ExpressionResult is the three-evaluation record of the
// npf_memory_expression_hypothesis protocol with the unchanged-state checks.
type ExpressionResult struct {
	Before              float64 `json:"before"`
	Suppressed          float64 `json:"suppressed"`
	After               float64 `json:"after"`
	Recovered           bool    `json:"recovered"`
	ParametersUnchanged bool    `json:"parameters_unchanged"`
	SlowUnchanged       bool    `json:"slow_unchanged"`
	PlasticUnchanged    bool    `json:"plastic_unchanged"`
}

// RunBioInspired dispatches on Protocol; ecdysone_inspired runs the timing
// protocol and npf_memory_expression_hypothesis returns the error "arrives
// with the next ticket" after Validate. A quantitative name with complete
// evidence passes Validate but is not implemented either.
func RunBioInspired(ctx context.Context, c BioInspiredConfig) (BioInspiredReport, error) {
	if err := c.Validate(); err != nil {
		return BioInspiredReport{}, err
	}
	switch c.Protocol {
	case ProtocolEcdysoneInspired:
		return runEcdysoneInspired(ctx, c)
	case ProtocolNPFMemoryExpression:
		return BioInspiredReport{}, fmt.Errorf("experiment: %s: execution arrives with the next ticket", c.Protocol)
	default:
		return BioInspiredReport{}, errors.New("experiment: quantitative protocols are not implemented")
	}
}
