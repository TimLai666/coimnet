package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

func TestBioInspiredValidateAcceptsBothProtocols(t *testing.T) {
	ecdysoboard := BioInspiredConfig{
		Protocol:   ProtocolEcdysoneInspired,
		Seeds:      []uint64{1, 2, 3},
		Episodes:   10,
		PulseSteps: []int{0, 100, 250},
		Registry:   []string{"S14"},
	}
	if err := ecdysoboard.Validate(); err != nil {
		t.Fatalf("ecdysone_inspired: unexpected validation error: %v", err)
	}

	npf := BioInspiredConfig{
		Protocol:   ProtocolNPFMemoryExpression,
		Seeds:      []uint64{11, 22, 33},
		Episodes:   10,
		Suppressed: 0.5,
		Tolerance:  1e-3,
		Registry:   []string{"S15"},
	}
	if err := npf.Validate(); err != nil {
		t.Fatalf("npf_memory_expression_hypothesis: unexpected validation error: %v", err)
	}
}

func TestBioInspiredValidateRejects(t *testing.T) {
	cases := []struct {
		name     string
		cfg      BioInspiredConfig
		contains string
	}{
		{
			name:     "unknown protocol",
			cfg:      BioInspiredConfig{Protocol: "mystery_protocol"},
			contains: "protocol",
		},
		{
			name: "seeds fewer than 3",
			cfg: BioInspiredConfig{
				Protocol: ProtocolEcdysoneInspired,
				Seeds:    []uint64{1, 2},
				Episodes: 10,
			},
			contains: "seed",
		},
		{
			name: "seeds duplicate",
			cfg: BioInspiredConfig{
				Protocol: ProtocolNPFMemoryExpression,
				Seeds:    []uint64{1, 1, 2},
				Episodes: 10,
			},
			contains: "seed",
		},
		{
			name: "episodes out of range",
			cfg: BioInspiredConfig{
				Protocol: ProtocolEcdysoneInspired,
				Seeds:    []uint64{1, 2, 3},
				Episodes: 0,
			},
			contains: "episodes",
		},
		{
			name: "ecdysone pulse_steps empty",
			cfg: BioInspiredConfig{
				Protocol: ProtocolEcdysoneInspired,
				Seeds:    []uint64{1, 2, 3},
				Episodes: 10,
				Registry: []string{"S14"},
			},
			contains: "pulse_steps",
		},
		{
			name: "ecdysone pulse_steps negative",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolEcdysoneInspired,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				PulseSteps: []int{0, -5, 100},
				Registry:   []string{"S14"},
			},
			contains: "pulse_steps",
		},
		{
			name: "ecdysone pulse_steps duplicate",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolEcdysoneInspired,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				PulseSteps: []int{0, 100, 100},
				Registry:   []string{"S14"},
			},
			contains: "pulse_steps",
		},
		{
			name: "npf pulse_steps must be empty",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				PulseSteps: []int{0},
				Suppressed: 0.5,
				Tolerance:  1e-3,
				Registry:   []string{"S15"},
			},
			contains: "pulse_steps",
		},
		{
			name: "npf suppressed out of range",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				Suppressed: 0,
				Tolerance:  1e-3,
				Registry:   []string{"S15"},
			},
			contains: "suppressed",
		},
		{
			name: "ecdysone suppressed must be 0",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolEcdysoneInspired,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				PulseSteps: []int{0, 100},
				Suppressed: 0.3,
				Registry:   []string{"S14"},
			},
			contains: "suppressed",
		},
		{
			name: "npf tolerance must be positive",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				Suppressed: 0.5,
				Tolerance:  0,
				Registry:   []string{"S15"},
			},
			contains: "tolerance",
		},
		{
			name: "ecdysone tolerance must be 0",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolEcdysoneInspired,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				PulseSteps: []int{0, 100},
				Tolerance:  1,
				Registry:   []string{"S14"},
			},
			contains: "tolerance",
		},
		{
			name: "ecdysone registry missing S14",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolEcdysoneInspired,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				PulseSteps: []int{0, 100},
				Registry:   []string{"S15"},
			},
			contains: "registry",
		},
		{
			name: "npf registry missing S15",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   10,
				Suppressed: 0.5,
				Tolerance:  1e-3,
				Registry:   []string{"S14"},
			},
			contains: "registry",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("Validate() error %q does not contain %q", err.Error(), tc.contains)
			}
		})
	}
}

func TestQuantitativeNamingIsRefusedWithoutFourItems(t *testing.T) {
	base := BioInspiredConfig{
		Protocol: "ecdysone",
		Seeds:    []uint64{1, 2, 3},
		Episodes: 10,
	}

	err := base.Validate()
	if !errors.Is(err, ErrQuantitativeNaming) {
		t.Fatalf("quantitative name without evidence: Validate() error = %v, want errors.Is(ErrQuantitativeNaming)", err)
	}

	complete := base
	complete.Quantitative = &QuantitativeEvidence{
		Method:              "release-dose crossover",
		Data:                "S14",
		Fit:                 "per-step consolidation window",
		HeldOutIntervention: "post-training pulse",
	}
	err = complete.Validate()
	if err != nil {
		t.Fatalf("quantitative name with complete evidence: Validate() error = %v, want nil", err)
	}
	if errors.Is(err, ErrQuantitativeNaming) {
		t.Fatal("quantitative name with complete evidence must not return ErrQuantitativeNaming")
	}

	_, err = RunBioInspired(context.Background(), complete)
	if err == nil || !strings.Contains(err.Error(), "quantitative protocols are not implemented") {
		t.Fatalf("RunBioInspired() error = %v, want error containing %q", err, "quantitative protocols are not implemented")
	}
}

func TestRunBioInspiredNotImplementedYet(t *testing.T) {
	ecdysoboard := BioInspiredConfig{
		Protocol:   ProtocolEcdysoneInspired,
		Seeds:      []uint64{1, 2, 3},
		Episodes:   10,
		PulseSteps: []int{0, 100, 250},
		Registry:   []string{"S14"},
	}
	// ecdysone_inspired is implemented now: the declaration is no longer
	// refused as "not implemented", and the out-of-episode pulse grid of this
	// config is refused by the protocol's own validation instead.
	_, err := RunBioInspired(context.Background(), ecdysoboard)
	if err == nil || strings.Contains(err.Error(), "next ticket") {
		t.Fatalf("RunBioInspired(ecdysone_inspired) error = %v, want the protocol to execute (without %q)", err, "next ticket")
	}
	if !strings.Contains(err.Error(), "outside the episode") {
		t.Fatalf("RunBioInspired(ecdysone_inspired) error = %v, want a pulse-outside-episode refusal", err)
	}

	npf := BioInspiredConfig{
		Protocol:   ProtocolNPFMemoryExpression,
		Seeds:      []uint64{11, 22, 33},
		Episodes:   10,
		Suppressed: 0.5,
		Tolerance:  1e-3,
		Registry:   []string{"S15"},
	}
	_, err = RunBioInspired(context.Background(), npf)
	if err == nil || !strings.Contains(err.Error(), "next ticket") {
		t.Fatalf("RunBioInspired(npf) error = %v, want error containing %q", err, "next ticket")
	}
}

func TestBioInspiredReportJSONShape(t *testing.T) {
	b, err := json.Marshal(BioInspiredReport{})
	if err != nil {
		t.Fatalf("json.Marshal(BioInspiredReport{}) failed: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"assumptions", "config", "config_hash", "protocol", "registry", "schema_version", "seeds"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("report JSON keys = %v, want %v", got, want)
	}
}
