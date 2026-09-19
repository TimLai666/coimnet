package experiment

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func bioInspiredRunConfig(pulseSteps []int) BioInspiredConfig {
	return BioInspiredConfig{
		Protocol:   ProtocolEcdysoneInspired,
		Seeds:      []uint64{1, 2, 3},
		Episodes:   6,
		PulseSteps: pulseSteps,
		Registry:   []string{"S14"},
	}
}

// TestEcdysoneInspiredCurveHasOnePointPerPulseStep runs the timing protocol on
// three seeds and checks the curve shape, the value constraints, the header
// fields and that at least one pulse timing actually left a slow-layer change.
func TestEcdysoneInspiredCurveHasOnePointPerPulseStep(t *testing.T) {
	cfg := bioInspiredRunConfig([]int{0, 1, 3})
	report, err := RunBioInspired(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunBioInspired: %v", err)
	}
	if len(report.Seeds) != len(cfg.Seeds) {
		t.Fatalf("report has %d seeds, want %d", len(report.Seeds), len(cfg.Seeds))
	}
	anySlow := false
	for _, seed := range report.Seeds {
		if seed.Failed {
			t.Fatalf("seed %d failed: %s", seed.Seed, seed.Error)
		}
		if len(seed.Curve) != len(cfg.PulseSteps) {
			t.Fatalf("seed %d curve has %d points, want %d", seed.Seed, len(seed.Curve), len(cfg.PulseSteps))
		}
		for i, point := range seed.Curve {
			t.Logf("seed %d point %d: pulse_step=%d slow_magnitude=%g retest_score=%g", seed.Seed, i, point.PulseStep, point.SlowMagnitude, point.RetestScore)
			if point.PulseStep != cfg.PulseSteps[i] {
				t.Errorf("seed %d point %d pulse_step = %d, want %d", seed.Seed, i, point.PulseStep, cfg.PulseSteps[i])
			}
			if math.IsNaN(point.SlowMagnitude) || math.IsInf(point.SlowMagnitude, 0) || point.SlowMagnitude < 0 {
				t.Errorf("seed %d point %d slow_magnitude = %g, want finite and >= 0", seed.Seed, i, point.SlowMagnitude)
			}
			if math.IsNaN(point.RetestScore) || math.IsInf(point.RetestScore, 0) || point.RetestScore > 0 {
				t.Errorf("seed %d point %d retest_score = %g, want finite and <= 0", seed.Seed, i, point.RetestScore)
			}
			if point.SlowMagnitude > 0 {
				anySlow = true
			}
		}
		// The retest scores a twin whose fast and slow layers are frozen, not
		// dropped, so the three pulse timings of one seed must not all land on
		// the same score: a curve that is flat in the timing measures nothing
		// about the timing.
		sameScore := true
		for _, point := range seed.Curve[1:] {
			if point.RetestScore != seed.Curve[0].RetestScore {
				sameScore = false
				break
			}
		}
		if len(seed.Curve) > 1 && sameScore {
			t.Errorf("seed %d scored %g at every pulse timing, want the retest to follow the pulse", seed.Seed, seed.Curve[0].RetestScore)
		}
	}
	if !anySlow {
		t.Error("no point of the report has SlowMagnitude > 0, want at least one consolidation write")
	}
	if !reflect.DeepEqual(report.Registry, []string{"S14"}) {
		t.Errorf("registry = %v, want [S14]", report.Registry)
	}
	if len(report.Assumptions) != 2 {
		t.Errorf("assumptions has %d sentences, want 2: %v", len(report.Assumptions), report.Assumptions)
	}
	if report.SchemaVersion != BioInspiredSchemaVersion {
		t.Errorf("schema_version = %q, want %q", report.SchemaVersion, BioInspiredSchemaVersion)
	}
	if report.ConfigHash == "" {
		t.Error("config_hash is empty")
	}
}

// TestEcdysoneInspiredIsDeterministic proves the same config reproduces the
// per-seed curves bit for bit across two runs.
func TestEcdysoneInspiredIsDeterministic(t *testing.T) {
	cfg := bioInspiredRunConfig([]int{0, 1, 3})
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

// TestEcdysoneInspiredRejectsPulseOutsideEpisode refuses a pulse that would
// never fall inside the 4 rows of one episode, before any seed runs.
func TestEcdysoneInspiredRejectsPulseOutsideEpisode(t *testing.T) {
	cfg := bioInspiredRunConfig([]int{0, 4})
	report, err := RunBioInspired(context.Background(), cfg)
	if err == nil {
		t.Fatal("RunBioInspired: nil error, want a pulse-outside-episode refusal")
	}
	if !strings.Contains(err.Error(), "outside the episode") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "outside the episode")
	}
	if !reflect.DeepEqual(report, BioInspiredReport{}) {
		t.Errorf("report = %+v, want the zero value", report)
	}
}

// TestEcdysoneInspiredHonoursCancellation proves a canceled context aborts the
// whole run with context.Canceled.
func TestEcdysoneInspiredHonoursCancellation(t *testing.T) {
	cfg := bioInspiredRunConfig([]int{0, 1, 3})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunBioInspired(ctx, cfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// TestNPFStillArrivesWithTheNextTicket pins the npf branch to its refusal until
// its own ticket implements it.
func TestNPFStillArrivesWithTheNextTicket(t *testing.T) {
	npf := BioInspiredConfig{
		Protocol:   ProtocolNPFMemoryExpression,
		Seeds:      []uint64{11, 22, 33},
		Episodes:   6,
		Suppressed: 0.5,
		Tolerance:  1e-3,
		Registry:   []string{"S15"},
	}
	_, err := RunBioInspired(context.Background(), npf)
	if err == nil || !strings.Contains(err.Error(), "arrives with the next ticket") {
		t.Fatalf("RunBioInspired(npf) error = %v, want error containing %q", err, "arrives with the next ticket")
	}
}
