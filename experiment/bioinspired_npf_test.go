package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

// npfRunConfig is the preregistered state protocol of these tests: three
// seeds, twelve training episodes, a suppressed concentration inside the range
// Validate allows and a tolerance tight enough that "recovered" means the two
// neutral evaluations agree to the last bits. PulseSteps stays empty because
// Validate refuses a timing grid on this protocol.
func npfRunConfig() BioInspiredConfig {
	return BioInspiredConfig{
		Protocol:   ProtocolNPFMemoryExpression,
		Seeds:      []uint64{1, 2, 3},
		Episodes:   12,
		Suppressed: 0.4,
		Tolerance:  1e-9,
		Registry:   []string{"S15"},
	}
}

// TestNPFSuppressionLowersScoreAndRecovers is the claim of the protocol: the
// score drops while the readout gain is suppressed by the receptor and returns
// to the pre-suppression value once the state is switched back, while base
// parameters, the slow layer and the fast plastic state stay bit for bit
// unchanged across the three evaluations. That combination is expression, not
// forgetting.
func TestNPFSuppressionLowersScoreAndRecovers(t *testing.T) {
	cfg := npfRunConfig()
	report, err := RunBioInspired(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunBioInspired: %v", err)
	}
	if len(report.Seeds) != len(cfg.Seeds) {
		t.Fatalf("report has %d seeds, want %d", len(report.Seeds), len(cfg.Seeds))
	}
	for _, seed := range report.Seeds {
		if seed.Failed {
			t.Fatalf("seed %d failed: %s", seed.Seed, seed.Error)
		}
		if seed.Expression == nil {
			t.Fatalf("seed %d has no expression result", seed.Seed)
		}
		e := seed.Expression
		t.Logf("seed %d: before=%g suppressed=%g after=%g recovered=%t", seed.Seed, e.Before, e.Suppressed, e.After, e.Recovered)
		for name, v := range map[string]float64{"before": e.Before, "suppressed": e.Suppressed, "after": e.After} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("seed %d %s = %g, want a finite score", seed.Seed, name, v)
			}
		}
		if !(e.Suppressed < e.Before) {
			t.Errorf("seed %d suppressed = %g, want it below before = %g", seed.Seed, e.Suppressed, e.Before)
		}
		if !e.Recovered {
			t.Errorf("seed %d recovered = false, want the neutral retest within %g of before (|after-before| = %g)", seed.Seed, cfg.Tolerance, math.Abs(e.After-e.Before))
		}
		if e.After != e.Before {
			t.Errorf("seed %d after = %g, want it bit for bit equal to before = %g", seed.Seed, e.After, e.Before)
		}
		if !e.ParametersUnchanged {
			t.Errorf("seed %d parameters_unchanged = false, want the base parameters untouched", seed.Seed)
		}
		if !e.SlowUnchanged {
			t.Errorf("seed %d slow_unchanged = false, want the slow layer untouched", seed.Seed)
		}
		if !e.PlasticUnchanged {
			t.Errorf("seed %d plastic_unchanged = false, want the fast plastic state untouched", seed.Seed)
		}
	}
	if report.SchemaVersion != BioInspiredSchemaVersion {
		t.Errorf("schema_version = %q, want %q", report.SchemaVersion, BioInspiredSchemaVersion)
	}
	if report.Protocol != ProtocolNPFMemoryExpression {
		t.Errorf("protocol = %q, want %q", report.Protocol, ProtocolNPFMemoryExpression)
	}
	if !reflect.DeepEqual(report.Config, cfg) {
		t.Errorf("config = %+v, want %+v", report.Config, cfg)
	}
	if report.ConfigHash == "" {
		t.Error("config_hash is empty")
	}
	if !reflect.DeepEqual(report.Registry, []string{"S15"}) {
		t.Errorf("registry = %v, want [S15]", report.Registry)
	}
	if len(report.Assumptions) != 2 {
		t.Errorf("assumptions has %d sentences, want 2: %v", len(report.Assumptions), report.Assumptions)
	}
}

// TestNPFIsDeterministic proves the same config reproduces the per-seed
// expression records bit for bit across two runs.
func TestNPFIsDeterministic(t *testing.T) {
	cfg := npfRunConfig()
	a, err := RunBioInspired(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	b, err := RunBioInspired(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !reflect.DeepEqual(a.Seeds, b.Seeds) {
		t.Error("two identical runs produced different Seeds")
	}
}

// TestNPFHonoursCancellation proves a canceled context aborts the whole run
// with context.Canceled instead of reporting a failed seed.
func TestNPFHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunBioInspired(ctx, npfRunConfig())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// TestNPFReportRoundTrips proves the report survives JSON in both directions,
// so the evidence file reads back as the value the run produced.
func TestNPFReportRoundTrips(t *testing.T) {
	report, err := RunBioInspired(context.Background(), npfRunConfig())
	if err != nil {
		t.Fatalf("RunBioInspired: %v", err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var back BioInspiredReport
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(report, back) {
		t.Errorf("round trip changed the report:\n got %+v\nwant %+v", back, report)
	}
}
