package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type bioInspiredOwnershipBacking struct {
	seeds    []uint64
	pulses   []int
	registry []string
}

func bioInspiredOwnershipConfig(protocol string, quantitative *QuantitativeEvidence, npfEmptyPulse bool) BioInspiredConfig {
	seeds := make([]uint64, 3, 6)
	copy(seeds, []uint64{101, 202, 303})
	seedsBacking := seeds[:cap(seeds)]
	seedsBacking[3], seedsBacking[4], seedsBacking[5] = 901, 902, 903

	registry := make([]string, 1, 4)
	registry[0] = map[string]string{
		ProtocolEcdysoneInspired:    "S14",
		ProtocolNPFMemoryExpression: "S15",
	}[protocol]
	registryBacking := registry[:cap(registry)]
	registryBacking[1], registryBacking[2], registryBacking[3] = "reserved-registry-1", "reserved-registry-2", "reserved-registry-3"

	cfg := BioInspiredConfig{
		Protocol:     protocol,
		Seeds:        seeds,
		Episodes:     1,
		Quantitative: quantitative,
		Registry:     registry,
	}
	if protocol == ProtocolNPFMemoryExpression {
		cfg.Suppressed = 0.4
		cfg.Tolerance = 1e-3
	}
	if protocol == ProtocolEcdysoneInspired {
		pulses := make([]int, 3, 6)
		copy(pulses, []int{0, 1, 3})
		pulsesBacking := pulses[:cap(pulses)]
		pulsesBacking[3], pulsesBacking[4], pulsesBacking[5] = 901, 902, 903
		cfg.PulseSteps = pulses
	} else if npfEmptyPulse {
		pulses := make([]int, 0, 3)
		pulsesBacking := pulses[:cap(pulses)]
		pulsesBacking[0], pulsesBacking[1], pulsesBacking[2] = 901, 902, 903
		cfg.PulseSteps = pulses
	}
	return cfg
}

func bioInspiredOwnershipQuantitative(kind string) *QuantitativeEvidence {
	switch kind {
	case "nil":
		return nil
	case "empty":
		return &QuantitativeEvidence{}
	case "complete":
		return &QuantitativeEvidence{
			Method:              "release-dose crossover",
			Data:                "S14/S15 paired cohort",
			Fit:                 "per-step consolidation window",
			HeldOutIntervention: "post-training pulse",
		}
	default:
		panic("unknown quantitative fixture: " + kind)
	}
}

func bioInspiredOwnershipCloneConfig(c BioInspiredConfig) BioInspiredConfig {
	out := c
	if c.Seeds == nil {
		out.Seeds = nil
	} else {
		out.Seeds = make([]uint64, len(c.Seeds))
		copy(out.Seeds, c.Seeds)
	}
	if c.PulseSteps == nil {
		out.PulseSteps = nil
	} else {
		out.PulseSteps = make([]int, len(c.PulseSteps))
		copy(out.PulseSteps, c.PulseSteps)
	}
	if c.Registry == nil {
		out.Registry = nil
	} else {
		out.Registry = make([]string, len(c.Registry))
		copy(out.Registry, c.Registry)
	}
	if c.Quantitative != nil {
		quantitative := *c.Quantitative
		out.Quantitative = &quantitative
	}
	return out
}

func bioInspiredOwnershipCaptureBacking(c BioInspiredConfig) bioInspiredOwnershipBacking {
	return bioInspiredOwnershipBacking{
		seeds:    bioInspiredOwnershipUint64Backing(c.Seeds),
		pulses:   bioInspiredOwnershipIntBacking(c.PulseSteps),
		registry: bioInspiredOwnershipStringBacking(c.Registry),
	}
}

func bioInspiredOwnershipUint64Backing(values []uint64) []uint64 {
	if values == nil {
		return nil
	}
	backing := make([]uint64, cap(values))
	copy(backing, values[:cap(values)])
	return backing
}

func bioInspiredOwnershipIntBacking(values []int) []int {
	if values == nil {
		return nil
	}
	backing := make([]int, cap(values))
	copy(backing, values[:cap(values)])
	return backing
}

func bioInspiredOwnershipStringBacking(values []string) []string {
	if values == nil {
		return nil
	}
	backing := make([]string, cap(values))
	copy(backing, values[:cap(values)])
	return backing
}

func bioInspiredOwnershipJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", value, err)
	}
	return b
}

func bioInspiredOwnershipIndependentHash(t *testing.T, cfg BioInspiredConfig) string {
	t.Helper()
	b := bioInspiredOwnershipJSON(t, cfg)
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:])
}

func bioInspiredOwnershipRequireSuccess(t *testing.T, cfg BioInspiredConfig, report BioInspiredReport, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("RunBioInspired(%s): %v", cfg.Protocol, err)
	}
	if len(report.Seeds) != len(cfg.Seeds) {
		t.Fatalf("report has %d seeds, want %d", len(report.Seeds), len(cfg.Seeds))
	}
	for _, seed := range report.Seeds {
		if seed.Failed {
			t.Fatalf("seed %d failed: %s", seed.Seed, seed.Error)
		}
		switch cfg.Protocol {
		case ProtocolEcdysoneInspired:
			if len(seed.Curve) != len(cfg.PulseSteps) {
				t.Fatalf("seed %d curve has %d points, want %d", seed.Seed, len(seed.Curve), len(cfg.PulseSteps))
			}
		case ProtocolNPFMemoryExpression:
			if seed.Expression == nil {
				t.Fatalf("seed %d has no expression result", seed.Seed)
			}
		}
	}
}

type bioInspiredOwnershipSecondErrContext struct {
	context.Context
	errCalls int
}

func (c *bioInspiredOwnershipSecondErrContext) Err() error {
	c.errCalls++
	if c.errCalls == 1 {
		return nil
	}
	return context.Canceled
}

func bioInspiredOwnershipRequirePartial(t *testing.T, cfg BioInspiredConfig, ctx *bioInspiredOwnershipSecondErrContext, report BioInspiredReport, err error) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunBioInspired(%s) error = %v, want errors.Is(..., context.Canceled)", cfg.Protocol, err)
	}
	if ctx.errCalls != 2 {
		t.Fatalf("RunBioInspired(%s) called context.Err %d times, want the deterministic first-nil/second-canceled sequence", cfg.Protocol, ctx.errCalls)
	}
	if report.SchemaVersion != BioInspiredSchemaVersion {
		t.Fatalf("partial report schema_version = %q, want %q", report.SchemaVersion, BioInspiredSchemaVersion)
	}
	if report.Protocol != cfg.Protocol {
		t.Fatalf("partial report protocol = %q, want %q", report.Protocol, cfg.Protocol)
	}
	wantHash := bioInspiredOwnershipIndependentHash(t, cfg)
	if report.ConfigHash != wantHash {
		t.Fatalf("partial report ConfigHash = %q, want independent sha256(json.Marshal(config)) %q", report.ConfigHash, wantHash)
	}
	wantRegistry := "S14"
	if cfg.Protocol == ProtocolNPFMemoryExpression {
		wantRegistry = "S15"
	}
	if !reflect.DeepEqual(report.Registry, []string{wantRegistry}) {
		t.Fatalf("partial report Registry = %v, want [%s]", report.Registry, wantRegistry)
	}
	if report.Seeds != nil {
		t.Fatalf("partial report Seeds = %v, want nil before the first seed is appended", report.Seeds)
	}
	if !reflect.DeepEqual(report.Config, cfg) {
		t.Fatalf("partial report Config = %+v, want %+v", report.Config, cfg)
	}
}

func bioInspiredOwnershipAppendResliceUint64(values *[]uint64, marker uint64) {
	length := len(*values)
	*values = append(*values, marker)
	*values = (*values)[:length]
}

func bioInspiredOwnershipAppendResliceInt(values *[]int, marker int) {
	length := len(*values)
	*values = append(*values, marker)
	*values = (*values)[:length]
}

func bioInspiredOwnershipAppendResliceString(values *[]string, marker string) {
	length := len(*values)
	*values = append(*values, marker)
	*values = (*values)[:length]
}

func bioInspiredOwnershipMutateConfig(c *BioInspiredConfig) {
	c.Seeds[0] = 7001
	bioInspiredOwnershipAppendResliceUint64(&c.Seeds, 7002)
	bioInspiredOwnershipAppendResliceString(&c.Registry, "mutated-registry")
	if len(c.Registry) > 0 {
		c.Registry[0] = "mutated-visible-registry"
	}
	bioInspiredOwnershipAppendResliceInt(&c.PulseSteps, 7003)
	if len(c.PulseSteps) > 0 {
		c.PulseSteps[0] = 7004
	}
	if c.Quantitative != nil {
		c.Quantitative.Method = "mutated-method"
		c.Quantitative.Data = "mutated-data"
		c.Quantitative.Fit = "mutated-fit"
		c.Quantitative.HeldOutIntervention = "mutated-held-out-intervention"
	}
}

func bioInspiredOwnershipAssertBackingUnchanged(t *testing.T, label string, got, want bioInspiredOwnershipBacking) {
	t.Helper()
	if len(want.seeds) > 0 && cap(got.seeds) >= len(want.seeds) {
		if !reflect.DeepEqual(got.seeds[:len(want.seeds)], want.seeds) {
			t.Errorf("%s seeds backing changed: got %v want %v", label, got.seeds[:len(want.seeds)], want.seeds)
		}
	}
	if len(want.pulses) > 0 && cap(got.pulses) >= len(want.pulses) {
		if !reflect.DeepEqual(got.pulses[:len(want.pulses)], want.pulses) {
			t.Errorf("%s pulse_steps backing changed: got %v want %v", label, got.pulses[:len(want.pulses)], want.pulses)
		}
	}
	if len(want.registry) > 0 && cap(got.registry) >= len(want.registry) {
		if !reflect.DeepEqual(got.registry[:len(want.registry)], want.registry) {
			t.Errorf("%s registry backing changed: got %v want %v", label, got.registry[:len(want.registry)], want.registry)
		}
	}
}

func TestBioInspiredOwnershipCallerMutationIsolation(t *testing.T) {
	for _, protocol := range []string{ProtocolEcdysoneInspired, ProtocolNPFMemoryExpression} {
		t.Run(protocol, func(t *testing.T) {
			cfg := bioInspiredOwnershipConfig(protocol, bioInspiredOwnershipQuantitative("complete"), protocol == ProtocolNPFMemoryExpression)
			original := bioInspiredOwnershipCloneConfig(cfg)
			report, err := RunBioInspired(context.Background(), cfg)
			bioInspiredOwnershipRequireSuccess(t, cfg, report, err)

			beforeReportJSON := bioInspiredOwnershipJSON(t, report)
			beforeSeedsJSON := bioInspiredOwnershipJSON(t, report.Seeds)
			beforeReportBacking := bioInspiredOwnershipCaptureBacking(report.Config)
			wantHash := bioInspiredOwnershipIndependentHash(t, original)
			if report.ConfigHash != wantHash {
				t.Fatalf("ConfigHash = %q, want independent sha256(json.Marshal(config)) %q", report.ConfigHash, wantHash)
			}

			bioInspiredOwnershipMutateConfig(&cfg)
			if got := bioInspiredOwnershipJSON(t, report); !bytes.Equal(got, beforeReportJSON) {
				t.Errorf("serialized report changed after caller mutation:\n got %s\nwant %s", got, beforeReportJSON)
			}
			if !reflect.DeepEqual(report.Config, original) {
				t.Errorf("report.Config changed after caller mutation:\n got %+v\nwant %+v", report.Config, original)
			}
			if report.ConfigHash != wantHash {
				t.Errorf("ConfigHash changed after caller mutation: got %q want %q", report.ConfigHash, wantHash)
			}
			if !reflect.DeepEqual(report.Registry, []string{map[string]string{
				ProtocolEcdysoneInspired:    "S14",
				ProtocolNPFMemoryExpression: "S15",
			}[protocol]}) {
				t.Errorf("report.Registry changed after caller mutation: got %v", report.Registry)
			}
			if got := bioInspiredOwnershipJSON(t, report.Seeds); !bytes.Equal(got, beforeSeedsJSON) {
				t.Errorf("report seeds changed after caller mutation: got %s want %s", got, beforeSeedsJSON)
			}
			bioInspiredOwnershipAssertBackingUnchanged(t, "report.Config", bioInspiredOwnershipCaptureBacking(report.Config), beforeReportBacking)
		})
	}
}

func TestBioInspiredOwnershipReportMutationIsolation(t *testing.T) {
	for _, protocol := range []string{ProtocolEcdysoneInspired, ProtocolNPFMemoryExpression} {
		t.Run(protocol, func(t *testing.T) {
			cfg := bioInspiredOwnershipConfig(protocol, bioInspiredOwnershipQuantitative("complete"), protocol == ProtocolNPFMemoryExpression)
			original := bioInspiredOwnershipCloneConfig(cfg)
			callerBacking := bioInspiredOwnershipCaptureBacking(cfg)

			report, err := RunBioInspired(context.Background(), cfg)
			bioInspiredOwnershipRequireSuccess(t, cfg, report, err)
			second, err := RunBioInspired(context.Background(), cfg)
			bioInspiredOwnershipRequireSuccess(t, cfg, second, err)
			beforeSecondJSON := bioInspiredOwnershipJSON(t, second)
			wantHash := bioInspiredOwnershipIndependentHash(t, original)

			bioInspiredOwnershipMutateConfig(&report.Config)
			if !reflect.DeepEqual(cfg, original) {
				t.Errorf("caller config changed after report.Config mutation:\n got %+v\nwant %+v", cfg, original)
			}
			if got := bioInspiredOwnershipJSON(t, cfg); !bytes.Equal(got, bioInspiredOwnershipJSON(t, original)) {
				t.Errorf("serialized caller config changed after report.Config mutation: got %s want %s", got, bioInspiredOwnershipJSON(t, original))
			}
			if got := bioInspiredOwnershipJSON(t, second); !bytes.Equal(got, beforeSecondJSON) {
				t.Errorf("second report changed after first report.Config mutation:\n got %s\nwant %s", got, beforeSecondJSON)
			}
			if !reflect.DeepEqual(second.Config, original) {
				t.Errorf("second report.Config changed after first report.Config mutation:\n got %+v\nwant %+v", second.Config, original)
			}
			if second.ConfigHash != wantHash {
				t.Errorf("second ConfigHash changed after first report.Config mutation: got %q want %q", second.ConfigHash, wantHash)
			}
			bioInspiredOwnershipAssertBackingUnchanged(t, "caller", bioInspiredOwnershipCaptureBacking(cfg), callerBacking)
		})
	}
}

func TestBioInspiredOwnershipConfigHashAndNilEmptyPreservation(t *testing.T) {
	cases := []struct {
		name             string
		protocol         string
		quantitativeKind string
		npfEmptyPulse    bool
	}{
		{name: "ecdysone/nil-quantitative", protocol: ProtocolEcdysoneInspired, quantitativeKind: "nil"},
		{name: "ecdysone/present-empty-quantitative", protocol: ProtocolEcdysoneInspired, quantitativeKind: "empty"},
		{name: "ecdysone/present-complete-quantitative", protocol: ProtocolEcdysoneInspired, quantitativeKind: "complete"},
		{name: "npf/nil-quantitative-nil-pulse", protocol: ProtocolNPFMemoryExpression, quantitativeKind: "nil"},
		{name: "npf/present-empty-quantitative-empty-pulse", protocol: ProtocolNPFMemoryExpression, quantitativeKind: "empty", npfEmptyPulse: true},
		{name: "npf/present-complete-quantitative-empty-pulse", protocol: ProtocolNPFMemoryExpression, quantitativeKind: "complete", npfEmptyPulse: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := bioInspiredOwnershipConfig(tc.protocol, bioInspiredOwnershipQuantitative(tc.quantitativeKind), tc.npfEmptyPulse)
			original := bioInspiredOwnershipCloneConfig(cfg)
			first, err := RunBioInspired(context.Background(), cfg)
			bioInspiredOwnershipRequireSuccess(t, cfg, first, err)
			second, err := RunBioInspired(context.Background(), cfg)
			bioInspiredOwnershipRequireSuccess(t, cfg, second, err)

			wantHash := bioInspiredOwnershipIndependentHash(t, original)
			for name, report := range map[string]BioInspiredReport{"first": first, "second": second} {
				if !reflect.DeepEqual(report.Config, original) {
					t.Errorf("%s Config = %+v, want %+v", name, report.Config, original)
				}
				if got := bioInspiredOwnershipJSON(t, report.Config); !bytes.Equal(got, bioInspiredOwnershipJSON(t, original)) {
					t.Errorf("%s serialized config = %s, want %s", name, got, bioInspiredOwnershipJSON(t, original))
				}
				if report.ConfigHash != wantHash {
					t.Errorf("%s ConfigHash = %q, want independent sha256(json.Marshal(config)) %q", name, report.ConfigHash, wantHash)
				}
			}
			if (first.Config.Quantitative == nil) != (original.Quantitative == nil) {
				t.Errorf("first Quantitative nilness = %v, want %v", first.Config.Quantitative == nil, original.Quantitative == nil)
			}
			if tc.protocol == ProtocolNPFMemoryExpression && tc.npfEmptyPulse != (original.PulseSteps != nil) {
				t.Fatalf("fixture PulseSteps nilness is inconsistent with requested case")
			}
			if tc.protocol == ProtocolNPFMemoryExpression && (first.Config.PulseSteps == nil) != (original.PulseSteps == nil) {
				t.Errorf("first NPF PulseSteps nilness = %v, want %v", first.Config.PulseSteps == nil, original.PulseSteps == nil)
			}
		})
	}
}

func TestBioInspiredOwnershipControls(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	completeQuantitative := bioInspiredOwnershipQuantitative("complete")
	unsupported := bioInspiredOwnershipConfig(ProtocolEcdysoneInspired, completeQuantitative, false)
	unsupported.Protocol = "ecdysone"

	pulseOutside := bioInspiredOwnershipConfig(ProtocolEcdysoneInspired, nil, false)
	pulseOutside.PulseSteps = []int{0, 4}

	cases := []struct {
		name      string
		ctx       context.Context
		cfg       BioInspiredConfig
		wantError string
		wantCause error
	}{
		{
			name: "invalid config",
			ctx:  context.Background(),
			cfg: func() BioInspiredConfig {
				c := bioInspiredOwnershipConfig(ProtocolNPFMemoryExpression, nil, false)
				c.Seeds = []uint64{1, 2}
				return c
			}(),
			wantError: "seeds: at least 3 required, got 2",
		},
		{name: "nil context ecdysone", ctx: nil, cfg: bioInspiredOwnershipConfig(ProtocolEcdysoneInspired, nil, false), wantError: "ecdysone_inspired run needs a context"},
		{name: "nil context NPF", ctx: nil, cfg: bioInspiredOwnershipConfig(ProtocolNPFMemoryExpression, nil, false), wantError: "npf_memory_expression_hypothesis run needs a context"},
		{name: "pre-canceled ecdysone", ctx: canceled, cfg: bioInspiredOwnershipConfig(ProtocolEcdysoneInspired, nil, false), wantCause: context.Canceled},
		{name: "pre-canceled NPF", ctx: canceled, cfg: bioInspiredOwnershipConfig(ProtocolNPFMemoryExpression, nil, false), wantCause: context.Canceled},
		{name: "unsupported complete quantitative name", ctx: context.Background(), cfg: unsupported, wantError: "quantitative protocols are not implemented"},
		{name: "pulse outside episode", ctx: context.Background(), cfg: pulseOutside, wantError: "outside the episode"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := RunBioInspired(tc.ctx, tc.cfg)
			if tc.wantCause != nil {
				if !errors.Is(err, tc.wantCause) {
					t.Fatalf("RunBioInspired() error = %v, want errors.Is(..., %v)", err, tc.wantCause)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("RunBioInspired() error = %v, want it to contain %q", err, tc.wantError)
			}
			if !reflect.DeepEqual(report, BioInspiredReport{}) {
				t.Fatalf("RunBioInspired() report = %+v, want zero report", report)
			}
		})
	}
}

func TestBioInspiredOwnershipPartialCancellationIsolation(t *testing.T) {
	for _, protocol := range []string{ProtocolEcdysoneInspired, ProtocolNPFMemoryExpression} {
		t.Run(protocol, func(t *testing.T) {
			cfg := bioInspiredOwnershipConfig(protocol, bioInspiredOwnershipQuantitative("complete"), protocol == ProtocolNPFMemoryExpression)
			original := bioInspiredOwnershipCloneConfig(cfg)
			ctx := &bioInspiredOwnershipSecondErrContext{Context: context.Background()}
			report, err := RunBioInspired(ctx, cfg)
			bioInspiredOwnershipRequirePartial(t, original, ctx, report, err)
			beforeReportJSON := bioInspiredOwnershipJSON(t, report)
			beforeReportBacking := bioInspiredOwnershipCaptureBacking(report.Config)
			wantHash := bioInspiredOwnershipIndependentHash(t, original)

			bioInspiredOwnershipMutateConfig(&cfg)
			if got := bioInspiredOwnershipJSON(t, report); !bytes.Equal(got, beforeReportJSON) {
				t.Errorf("partial serialized report changed after caller mutation:\n got %s\nwant %s", got, beforeReportJSON)
			}
			if !reflect.DeepEqual(report.Config, original) {
				t.Errorf("partial report.Config changed after caller mutation:\n got %+v\nwant %+v", report.Config, original)
			}
			if report.ConfigHash != wantHash {
				t.Errorf("partial report ConfigHash changed after caller mutation: got %q want %q", report.ConfigHash, wantHash)
			}
			if report.Seeds != nil {
				t.Errorf("partial report Seeds changed after caller mutation: got %v want nil", report.Seeds)
			}
			bioInspiredOwnershipAssertBackingUnchanged(t, "partial report.Config", bioInspiredOwnershipCaptureBacking(report.Config), beforeReportBacking)

			reverseCfg := bioInspiredOwnershipConfig(protocol, bioInspiredOwnershipQuantitative("complete"), protocol == ProtocolNPFMemoryExpression)
			reverseOriginal := bioInspiredOwnershipCloneConfig(reverseCfg)
			reverseCallerBacking := bioInspiredOwnershipCaptureBacking(reverseCfg)
			reverseCtx := &bioInspiredOwnershipSecondErrContext{Context: context.Background()}
			reverseReport, err := RunBioInspired(reverseCtx, reverseCfg)
			bioInspiredOwnershipRequirePartial(t, reverseOriginal, reverseCtx, reverseReport, err)

			bioInspiredOwnershipMutateConfig(&reverseReport.Config)
			if !reflect.DeepEqual(reverseCfg, reverseOriginal) {
				t.Errorf("caller config changed after partial report.Config mutation:\n got %+v\nwant %+v", reverseCfg, reverseOriginal)
			}
			if got := bioInspiredOwnershipJSON(t, reverseCfg); !bytes.Equal(got, bioInspiredOwnershipJSON(t, reverseOriginal)) {
				t.Errorf("serialized caller config changed after partial report.Config mutation: got %s want %s", got, bioInspiredOwnershipJSON(t, reverseOriginal))
			}
			bioInspiredOwnershipAssertBackingUnchanged(t, "partial caller", bioInspiredOwnershipCaptureBacking(reverseCfg), reverseCallerBacking)
		})
	}
}
