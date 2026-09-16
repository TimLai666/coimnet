package modulation

import (
	"math"
	"reflect"
	"testing"
)

// handSequence is the raw reward sequence of every table below.
var handSequence = []float64{1, -2, 0.5, 0, 3}

// Hand table. raw = [1, -2, 0.5, 0, 3]; delta = raw - expected.
//
// Baseline none (expected is 0 at every step):
//
//	raw      1     -2    0.5   0     3
//	delta    1     -2    0.5   0     3
//	relu     [1]   [0]   [0.5] [0]   [3]     clearance 0, 2, 0, 0, 0
//	split    [1,0] [0,2] [.5,0] [0,0] [3,0]  clearance 0 throughout
//
// Baseline running_mean, window 2 (the mean of the previous two raw values,
// this one excluded; fewer than two available means the mean of what there is,
// and the very first step has no history and so expects 0):
//
//	history  []    [1]   [1,-2] [-2,.5] [.5,0]
//	expected 0     1     -0.5   -0.75   0.25
//	delta    1     -3    1      0.75    2.75
//	relu     [1]   [0]   [1]    [0.75]  [2.75]   clearance 0, 3, 0, 0, 0
//	split    [1,0] [0,3] [1,0]  [.75,0] [2.75,0] clearance 0 throughout
func TestRewardMapperMapsTheHandComputedSequence(t *testing.T) {
	for _, table := range []struct {
		mapper    RewardMapper
		expected  []float64
		applied   [][]float64
		clearance []float64
	}{
		{
			mapper:    RewardMapper{Baseline: "none", Kind: "relu"},
			expected:  []float64{0, 0, 0, 0, 0},
			applied:   [][]float64{{1}, {0}, {0.5}, {0}, {3}},
			clearance: []float64{0, 2, 0, 0, 0},
		},
		{
			mapper:    RewardMapper{Baseline: "none", Kind: "split"},
			expected:  []float64{0, 0, 0, 0, 0},
			applied:   [][]float64{{1, 0}, {0, 2}, {0.5, 0}, {0, 0}, {3, 0}},
			clearance: []float64{0, 0, 0, 0, 0},
		},
		{
			mapper:    RewardMapper{Baseline: "running_mean", Window: 2, Kind: "relu"},
			expected:  []float64{0, 1, -0.5, -0.75, 0.25},
			applied:   [][]float64{{1}, {0}, {1}, {0.75}, {2.75}},
			clearance: []float64{0, 3, 0, 0, 0},
		},
		{
			mapper:    RewardMapper{Baseline: "running_mean", Window: 2, Kind: "split"},
			expected:  []float64{0, 1, -0.5, -0.75, 0.25},
			applied:   [][]float64{{1, 0}, {0, 3}, {1, 0}, {0.75, 0}, {2.75, 0}},
			clearance: []float64{0, 0, 0, 0, 0},
		},
	} {
		mapper := table.mapper
		name := mapper.Baseline + "/" + mapper.Kind
		for i, raw := range handSequence {
			record, err := mapper.Map(raw)
			if err != nil {
				t.Fatalf("%s: Map(%v) error = %v", name, raw, err)
			}
			if record.Raw != raw {
				t.Fatalf("%s step %d: Raw = %v, want %v", name, i, record.Raw, raw)
			}
			if record.Expected != table.expected[i] {
				t.Fatalf("%s step %d: Expected = %v, want %v", name, i, record.Expected, table.expected[i])
			}
			if record.Transformed != raw-table.expected[i] {
				t.Fatalf("%s step %d: Transformed = %v, want %v", name, i, record.Transformed, raw-table.expected[i])
			}
			if !reflect.DeepEqual(record.Applied, table.applied[i]) {
				t.Fatalf("%s step %d: Applied = %v, want %v", name, i, record.Applied, table.applied[i])
			}
			if record.ClearanceBoost != table.clearance[i] {
				t.Fatalf("%s step %d: ClearanceBoost = %v, want %v", name, i, record.ClearanceBoost, table.clearance[i])
			}
		}
	}
}

// Negative feedback becomes a clearance term or a second channel; it never
// becomes a negative release rate, and it never becomes a negative zero either.
func TestRewardMapperNeverAppliesANegativeValue(t *testing.T) {
	for _, mapper := range []RewardMapper{
		{Baseline: "none", Kind: "relu"},
		{Baseline: "none", Kind: "split"},
		{Baseline: "running_mean", Window: 2, Kind: "relu"},
		{Baseline: "running_mean", Window: 3, Kind: "split"},
	} {
		m := mapper
		for _, raw := range []float64{-5, -0.25, 0, 0.25, 5, -1} {
			record, err := m.Map(raw)
			if err != nil {
				t.Fatalf("Map(%v) error = %v", raw, err)
			}
			for i, v := range record.Applied {
				if v < 0 || math.Signbit(v) || !finite(v) {
					t.Fatalf("%s/%s: Map(%v) applied %v on channel %d", m.Baseline, m.Kind, raw, v, i)
				}
			}
			if record.ClearanceBoost < 0 || math.Signbit(record.ClearanceBoost) || !finite(record.ClearanceBoost) {
				t.Fatalf("%s/%s: Map(%v) boosted clearance by %v", m.Baseline, m.Kind, raw, record.ClearanceBoost)
			}
			if (m.Kind == "relu" && len(record.Applied) != 1) || (m.Kind == "split" && len(record.Applied) != 2) {
				t.Fatalf("%s/%s: Map(%v) applied %d channels", m.Baseline, m.Kind, raw, len(record.Applied))
			}
		}
	}
}

// The record is the caller's; the mapper keeps its own history. A later call
// cannot reach back into a record already handed out.
func TestRewardMapperRecordIsIndependentOfLaterCalls(t *testing.T) {
	m := RewardMapper{Baseline: "running_mean", Window: 2, Kind: "split"}
	first, err := m.Map(1)
	if err != nil {
		t.Fatalf("Map(1) error = %v", err)
	}
	kept := append([]float64(nil), first.Applied...)
	if _, err := m.Map(-4); err != nil {
		t.Fatalf("Map(-4) error = %v", err)
	}
	if _, err := m.Map(7); err != nil {
		t.Fatalf("Map(7) error = %v", err)
	}
	if !reflect.DeepEqual(first.Applied, kept) {
		t.Fatalf("the first record changed to %v, want %v", first.Applied, kept)
	}
	if first.Raw != 1 || first.Expected != 0 || first.Transformed != 1 {
		t.Fatalf("the first record is now raw %v expected %v transformed %v", first.Raw, first.Expected, first.Transformed)
	}
	// Writing through the record cannot reach the mapper's history either: the
	// baseline after 1, -4 and 7 is the mean of -4 and 7.
	first.Applied[0] = 99
	record, err := m.Map(0)
	if err != nil {
		t.Fatalf("Map(0) error = %v", err)
	}
	if record.Expected != 1.5 {
		t.Fatalf("Expected = %v after writing into an earlier record, want 1.5", record.Expected)
	}
}

func TestRewardMapperRejectsAnInvalidDeclarationOrRawValue(t *testing.T) {
	for name, m := range map[string]RewardMapper{
		"unknown baseline":    {Baseline: "ema", Kind: "relu"},
		"no baseline":         {Kind: "relu"},
		"unknown kind":        {Baseline: "none", Kind: "softplus"},
		"no kind":             {Baseline: "none"},
		"window without mean": {Baseline: "none", Window: 3, Kind: "relu"},
		"mean without window": {Baseline: "running_mean", Kind: "relu"},
		"negative window":     {Baseline: "running_mean", Window: -1, Kind: "relu"},
	} {
		mapper := m
		if _, err := mapper.Map(1); err == nil {
			t.Errorf("%s: Map() error = nil", name)
		}
	}
	m := RewardMapper{Baseline: "running_mean", Window: 2, Kind: "relu"}
	if _, err := m.Map(2); err != nil {
		t.Fatalf("Map(2) error = %v", err)
	}
	for _, raw := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := m.Map(raw); err == nil {
			t.Errorf("Map(%v) error = nil", raw)
		}
	}
	// A rejected raw value never enters the baseline: the expected value is
	// still the mean of the one accepted value.
	record, err := m.Map(4)
	if err != nil {
		t.Fatalf("Map(4) error = %v", err)
	}
	if record.Expected != 2 {
		t.Fatalf("Expected = %v after three rejected values, want 2", record.Expected)
	}
}
