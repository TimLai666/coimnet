package signal

import (
	"context"
	"math"
	"math/big"
	"testing"
)

func TestResampleExclusiveEndNeedNotBeRepresentable(t *testing.T) {
	const q int64 = math.MaxInt64 / 2
	for _, sourceOverflow := range []bool{false, true} {
		cfg := samplingConfig(t, CausalHold)
		cfg.StartStep = q
		cfg.EndStep = q + 1
		if sourceOverflow {
			cfg.SourceTicks = 2
			cfg.SimulationSteps = 1
		} else {
			clock, err := NewClock(testVersion(), TimeUnitModelStep, 2)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Clock = clock
		}
		got, err := ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, 3)})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Signals) != 1 || got.Signals[0].SourceSequence() != uint64(q) || got.Signals[0].Values()[0] != 3 {
			t.Fatalf("output %+v", got)
		}
	}
	cfg := samplingConfig(t, CausalHold)
	cfg.StartStep = math.MaxInt64
	cfg.EndStep = math.MaxInt64
	cfg.SourceTicks = math.MaxInt64
	clock, err := NewClock(testVersion(), TimeUnitModelStep, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clock = clock
	got, err := ResampleOffline(context.Background(), cfg, nil)
	if err != nil || got.Signals == nil || len(got.Signals) != 0 {
		t.Fatalf("empty horizon %+v %v", got, err)
	}
}

func TestResampleLargeIntervalDoesNotRoundToFutureEndpoint(t *testing.T) {
	const n int64 = 1 << 53
	cfg := samplingConfig(t, OfflineLinear)
	cfg.SourceTicks = 1
	cfg.SimulationSteps = 1
	cfg.StartStep = n
	cfg.EndStep = n + 2
	got, err := ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, 0), sample(t, 2, n+1, 100)})
	if err != nil {
		t.Fatal(err)
	}
	exact := new(big.Rat).Mul(new(big.Rat).SetFrac64(n, n+1), big.NewRat(100, 1))
	want, _ := exact.Float64()
	if got.Signals[0].Values()[0] != want || got.Signals[0].Values()[0] >= 100 {
		t.Fatalf("early future endpoint: got %.17g want %.17g", got.Signals[0].Values()[0], want)
	}
	if got.Signals[1].Values()[0] != 100 {
		t.Fatal("exact right endpoint changed")
	}
	// A tiny left weight can still make a large contribution. Compute it
	// independently instead of subtracting a rounded right weight from one.
	const gap int64 = 1 << 60
	cfg.StartStep = gap - 1
	cfg.EndStep = gap
	got, err = ResampleOffline(context.Background(), cfg, []Signal{sample(t, 1, 0, 1e100), sample(t, 2, gap, 0)})
	if err != nil {
		t.Fatal(err)
	}
	want = math.Ldexp(1e100, -60)
	if got.Signals[0].Values()[0] != want {
		t.Fatalf("left contribution lost: got %g want %g", got.Signals[0].Values()[0], want)
	}
}
