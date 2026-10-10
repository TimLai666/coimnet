package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

type bioInspiredFiniteValueCase struct {
	name  string
	value float64
	want  string
}

func bioInspiredFiniteNPFConfig() BioInspiredConfig {
	return BioInspiredConfig{
		Protocol:   ProtocolNPFMemoryExpression,
		Seeds:      []uint64{11, 22, 33},
		Episodes:   10,
		Suppressed: 0.5,
		Tolerance:  1e-3,
		Registry:   []string{"S15"},
	}
}

func bioInspiredFiniteEcdysoneConfig() BioInspiredConfig {
	return BioInspiredConfig{
		Protocol:   ProtocolEcdysoneInspired,
		Seeds:      []uint64{1, 2, 3},
		Episodes:   1,
		PulseSteps: []int{0},
		Registry:   []string{"S14"},
	}
}

func bioInspiredFiniteNPFSuppressedValues() []bioInspiredFiniteValueCase {
	return []bioInspiredFiniteValueCase{
		{name: "nan", value: math.NaN(), want: "suppressed: must be strictly between 0 and 1, got NaN"},
		{name: "negative infinity", value: math.Inf(-1), want: "suppressed: must be strictly between 0 and 1, got -Inf"},
		{name: "positive infinity", value: math.Inf(1), want: "suppressed: must be strictly between 0 and 1, got +Inf"},
		{name: "negative finite", value: -0.5, want: "suppressed: must be strictly between 0 and 1, got -0.5"},
		{name: "zero", value: 0, want: "suppressed: must be strictly between 0 and 1, got 0"},
		{name: "one", value: 1, want: "suppressed: must be strictly between 0 and 1, got 1"},
		{name: "above one", value: 1.5, want: "suppressed: must be strictly between 0 and 1, got 1.5"},
	}
}

func bioInspiredFiniteNPFToleranceValues() []bioInspiredFiniteValueCase {
	return []bioInspiredFiniteValueCase{
		{name: "nan", value: math.NaN(), want: "tolerance: must be > 0, got NaN"},
		{name: "negative infinity", value: math.Inf(-1), want: "tolerance: must be > 0, got -Inf"},
		{name: "positive infinity", value: math.Inf(1), want: "tolerance: must be > 0, got +Inf"},
		{name: "negative finite", value: -1, want: "tolerance: must be > 0, got -1"},
		{name: "zero", value: 0, want: "tolerance: must be > 0, got 0"},
	}
}

func bioInspiredFiniteEcdysoneValues() []struct {
	name  string
	value float64
} {
	return []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative finite", value: -0.5},
		{name: "smallest positive", value: math.SmallestNonzeroFloat64},
		{name: "positive finite", value: 0.5},
		{name: "largest finite", value: math.MaxFloat64},
	}
}

func TestBioInspiredValidateFiniteNPFFields(t *testing.T) {
	for _, tc := range bioInspiredFiniteNPFSuppressedValues() {
		t.Run("suppressed/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteNPFConfig()
			cfg.Suppressed = tc.value
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil for Suppressed=%g, want %q", tc.value, tc.want)
			}
			if err.Error() != tc.want {
				t.Fatalf("Validate() error = %q, want %q", err, tc.want)
			}
		})
	}

	for _, tc := range bioInspiredFiniteNPFToleranceValues() {
		t.Run("tolerance/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteNPFConfig()
			cfg.Tolerance = tc.value
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil for Tolerance=%g, want %q", tc.value, tc.want)
			}
			if err.Error() != tc.want {
				t.Fatalf("Validate() error = %q, want %q", err, tc.want)
			}
		})
	}
}

func TestBioInspiredRunFiniteNPFFieldsReturnZeroReport(t *testing.T) {
	for _, tc := range bioInspiredFiniteNPFSuppressedValues() {
		t.Run("suppressed/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteNPFConfig()
			cfg.Suppressed = tc.value
			report, err := RunBioInspired(context.Background(), cfg)
			if err == nil {
				t.Fatalf("RunBioInspired() = nil error for Suppressed=%g, want %q", tc.value, tc.want)
			}
			if err.Error() != tc.want {
				t.Fatalf("RunBioInspired() error = %q, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})
	}

	for _, tc := range bioInspiredFiniteNPFToleranceValues() {
		t.Run("tolerance/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteNPFConfig()
			cfg.Tolerance = tc.value
			report, err := RunBioInspired(context.Background(), cfg)
			if err == nil {
				t.Fatalf("RunBioInspired() = nil error for Tolerance=%g, want %q", tc.value, tc.want)
			}
			if err.Error() != tc.want {
				t.Fatalf("RunBioInspired() error = %q, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})
	}
}

func TestBioInspiredValidateFiniteEcdysoneFieldsRemainStrict(t *testing.T) {
	for _, tc := range bioInspiredFiniteEcdysoneValues() {
		t.Run("suppressed/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteEcdysoneConfig()
			cfg.Suppressed = tc.value
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), "suppressed: must be 0 for ecdysone_inspired") {
				t.Fatalf("Validate() error = %v, want the original suppressed-zero refusal", err)
			}
		})

		t.Run("tolerance/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteEcdysoneConfig()
			cfg.Tolerance = tc.value
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), "tolerance: must be 0 for ecdysone_inspired") {
				t.Fatalf("Validate() error = %v, want the original tolerance-zero refusal", err)
			}
		})
	}
}

func TestBioInspiredRunFiniteEcdysoneFieldsReturnZeroReport(t *testing.T) {
	for _, tc := range bioInspiredFiniteEcdysoneValues() {
		t.Run("suppressed/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteEcdysoneConfig()
			cfg.Suppressed = tc.value
			report, err := RunBioInspired(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), "suppressed: must be 0 for ecdysone_inspired") {
				t.Fatalf("RunBioInspired() error = %v, want the original suppressed-zero refusal", err)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})

		t.Run("tolerance/"+tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteEcdysoneConfig()
			cfg.Tolerance = tc.value
			report, err := RunBioInspired(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), "tolerance: must be 0 for ecdysone_inspired") {
				t.Fatalf("RunBioInspired() error = %v, want the original tolerance-zero refusal", err)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})
	}
}

func TestBioInspiredValidateFiniteBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		suppressed float64
		tolerance  float64
	}{
		{name: "smallest positive suppressed", suppressed: math.SmallestNonzeroFloat64, tolerance: 1e-3},
		{name: "largest value below one suppressed", suppressed: math.Nextafter(1, 0), tolerance: 1e-3},
		{name: "smallest positive tolerance", suppressed: 0.5, tolerance: math.SmallestNonzeroFloat64},
		{name: "largest finite tolerance", suppressed: 0.5, tolerance: math.MaxFloat64},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := bioInspiredFiniteNPFConfig()
			cfg.Suppressed = tc.suppressed
			cfg.Tolerance = tc.tolerance
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() = %v for legal boundary values Suppressed=%g Tolerance=%g", err, tc.suppressed, tc.tolerance)
			}
		})
	}
}

func TestBioInspiredRunFiniteLegalReportJSON(t *testing.T) {
	cfg := bioInspiredFiniteNPFConfig()
	report, err := RunBioInspired(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunBioInspired() = %v for legal NPF config", err)
	}
	if reflect.DeepEqual(report, BioInspiredReport{}) {
		t.Fatal("RunBioInspired() returned a zero report for a legal NPF config")
	}

	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(legal NPF report) = %v", err)
	}
	var decoded BioInspiredReport
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(legal NPF report) = %v", err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatalf("JSON round trip changed the legal report:\nencoded=%s\nreport=%+v\ndecoded=%+v", b, report, decoded)
	}
}

func TestBioInspiredFiniteErrorPrecedence(t *testing.T) {
	cases := []struct {
		name string
		cfg  BioInspiredConfig
		want string
	}{
		{
			name: "protocol before all fields",
			cfg: BioInspiredConfig{
				Protocol:   "mystery_protocol",
				Episodes:   0,
				Suppressed: math.NaN(),
				Tolerance:  math.Inf(1),
			},
			want: `protocol: unsupported protocol "mystery_protocol"`,
		},
		{
			name: "seeds before episodes and NPF fields",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2},
				Episodes:   0,
				Suppressed: math.NaN(),
				Tolerance:  math.Inf(1),
			},
			want: "seeds: at least 3 required, got 2",
		},
		{
			name: "episodes before NPF fields",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   0,
				Suppressed: math.NaN(),
				Tolerance:  math.Inf(1),
			},
			want: "episodes: must be between 1 and 10000, got 0",
		},
		{
			name: "NPF pulse before fields",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   1,
				PulseSteps: []int{0},
				Suppressed: math.NaN(),
				Tolerance:  math.Inf(1),
			},
			want: "pulse_steps: must be empty for npf_memory_expression_hypothesis",
		},
		{
			name: "ecdysone pulse before fields",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolEcdysoneInspired,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   1,
				Suppressed: math.NaN(),
				Tolerance:  math.Inf(1),
				Registry:   []string{"S14"},
			},
			want: "pulse_steps: at least one entry required for ecdysone_inspired",
		},
		{
			name: "suppressed before tolerance",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   1,
				Suppressed: math.NaN(),
				Tolerance:  math.Inf(1),
			},
			want: "suppressed: must be strictly between 0 and 1, got NaN",
		},
		{
			name: "tolerance before registry",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   1,
				Suppressed: 0.5,
				Tolerance:  math.NaN(),
			},
			want: "tolerance: must be > 0, got NaN",
		},
		{
			name: "registry after both fields",
			cfg: BioInspiredConfig{
				Protocol:   ProtocolNPFMemoryExpression,
				Seeds:      []uint64{1, 2, 3},
				Episodes:   1,
				Suppressed: 0.5,
				Tolerance:  1e-3,
			},
			want: `registry: must contain "S15" for npf_memory_expression_hypothesis`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil || err.Error() != tc.want {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
			report, err := RunBioInspired(context.Background(), tc.cfg)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("RunBioInspired() error = %v, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})
	}
}

func TestBioInspiredRunFiniteContextBehavior(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name      string
		ctx       context.Context
		cfg       BioInspiredConfig
		wantError string
		wantCause error
	}{
		{
			name:      "NPF nil context",
			ctx:       nil,
			cfg:       bioInspiredFiniteNPFConfig(),
			wantError: "npf_memory_expression_hypothesis run needs a context",
		},
		{
			name:      "NPF canceled context",
			ctx:       canceled,
			cfg:       bioInspiredFiniteNPFConfig(),
			wantCause: context.Canceled,
		},
		{
			name:      "ecdysone nil context",
			ctx:       nil,
			cfg:       bioInspiredFiniteEcdysoneConfig(),
			wantError: "ecdysone_inspired run needs a context",
		},
		{
			name:      "ecdysone canceled context",
			ctx:       canceled,
			cfg:       bioInspiredFiniteEcdysoneConfig(),
			wantCause: context.Canceled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := RunBioInspired(tc.ctx, tc.cfg)
			if tc.wantCause != nil {
				if !errors.Is(err, tc.wantCause) {
					t.Fatalf("RunBioInspired() error = %v, want errors.Is(..., %v)", err, tc.wantCause)
				}
			} else if err == nil || err.Error() != tc.wantError {
				t.Fatalf("RunBioInspired() error = %v, want %q", err, tc.wantError)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})
	}
}
