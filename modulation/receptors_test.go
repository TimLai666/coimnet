package modulation

import (
	"math"
	"strings"
	"testing"
)

// receptorFixture is three nodes in two regions with two channels. Node 0 and
// node 1 sit in region 0, node 2 in region 1; channel 0 holds 2 in region 0 and
// 4 in region 1.
func receptorFixture() (ChemistryState, []int) {
	state := ChemistryState{Concentration: [][]float64{{2, 0}, {4, 1}}, Steps: 7}
	return state, []int{0, 0, 1}
}

func completeReceptor() Receptor {
	return Receptor{
		Cells: []int{0, 1}, CellType: "KC", Signal: "octopamine", Channel: 0,
		Status: StatusHypothesized, Kd: 1, N: 1,
		Evidence: "fixture: hand declared", MeasurementKind: "engineering_assumption", MappingVersion: "fixture/v1",
	}
}

// The log-domain form 1/(1+exp(n*(ln Kd - ln c))) is the same curve as
// c^n/(Kd^n + c^n): c = 2, Kd = 1, n = 1 gives 2/3 = 0.6666666666666666, and
// c = 4, Kd = 2, n = 2 gives 16/(4+16) = 0.8 exactly.
func TestOccupancyFollowsTheHandComputedCurve(t *testing.T) {
	cases := []struct {
		c, kd, n, want float64
	}{
		{2, 1, 1, 0.6666666666666666},
		{4, 2, 2, 0.8},
		{2.5, 2.5, 3, 0.5},     // c = Kd is half occupancy for every n
		{1e300, 1e300, 8, 0.5}, // and stays half at the top of the range
		{0, 1, 1, 0},           // no chemical, no occupancy
	}
	for _, tc := range cases {
		got := Occupancy(tc.c, tc.kd, tc.n)
		if math.Abs(got-tc.want) > 1e-15 {
			t.Fatalf("Occupancy(%v, %v, %v) = %v, want %v", tc.c, tc.kd, tc.n, got, tc.want)
		}
	}
	// Monotone in c at a fixed Kd and n.
	previous := -1.0
	for _, c := range []float64{0, 0.25, 0.5, 1, 2, 4, 1e6} {
		got := Occupancy(c, 1, 2)
		if got <= previous {
			t.Fatalf("Occupancy is not increasing at c = %v: %v after %v", c, got, previous)
		}
		previous = got
	}
}

func TestOccupancyIsStableAtExtremeConcentrations(t *testing.T) {
	cases := []struct {
		name           string
		c, kd, n, want float64
	}{
		{"saturated", 1e300, 1, 8, 1},
		{"vanishing", 1e-300, 1, 8, 0},
		{"largest float", math.MaxFloat64, 1, 8, 1},
		{"smallest subnormal", math.SmallestNonzeroFloat64, 1, 8, 0},
		{"tiny Kd", 1e300, 1e-300, 8, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Occupancy(tc.c, tc.kd, tc.n)
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Fatalf("Occupancy(%v, %v, %v) = %v", tc.c, tc.kd, tc.n, got)
			}
			if got < 0 || got > 1 {
				t.Fatalf("Occupancy(%v, %v, %v) = %v, outside [0, 1]", tc.c, tc.kd, tc.n, got)
			}
			if got != tc.want {
				t.Fatalf("Occupancy(%v, %v, %v) = %v, want %v", tc.c, tc.kd, tc.n, got, tc.want)
			}
		})
	}
}

// Occupancy has no error return, so an argument outside the declared domain
// answers NaN rather than a number that looks like a measurement. Validate is
// the gate that keeps such a record out of Occupancies.
func TestOccupancyIsUndefinedOutsideItsDomain(t *testing.T) {
	cases := [][3]float64{
		{-1, 1, 1},          // negative concentration
		{1, 0, 1},           // Kd must be positive
		{1, -1, 1},          // and not negative
		{1, 1, 0.5},         // n must be at least 1
		{math.NaN(), 1, 1},  // non-finite arguments
		{math.Inf(1), 1, 1}, //
		{1, math.Inf(1), 1}, //
		{1, 1, math.Inf(1)}, //
	}
	for _, tc := range cases {
		if got := Occupancy(tc[0], tc[1], tc[2]); !math.IsNaN(got) {
			t.Fatalf("Occupancy(%v, %v, %v) = %v, want NaN", tc[0], tc[1], tc[2], got)
		}
	}
}

func TestReceptorsValidateRejectsAnIncompleteOrOutOfRangeRecord(t *testing.T) {
	base := Receptors{Records: []Receptor{completeReceptor()}, Mix: MixSum}
	if err := base.Validate(3, 2); err != nil {
		t.Fatalf("the reference declaration was refused: %v", err)
	}
	empty := Receptors{Mix: MixMax}
	if err := empty.Validate(3, 2); err != nil {
		t.Fatalf("a declaration with no record was refused: %v", err)
	}
	if err := (&Receptors{Records: []Receptor{completeReceptor()}}).Validate(3, 2); err != nil {
		t.Fatalf("an empty mix, which means sum, was refused: %v", err)
	}
	cases := []struct {
		name string
		edit func(*Receptors)
		want string
	}{
		{"unknown mix", func(r *Receptors) { r.Mix = "mean" }, "mix"},
		{"no cell", func(r *Receptors) { r.Records[0].Cells = nil }, "cell"},
		{"descending cells", func(r *Receptors) { r.Records[0].Cells = []int{1, 0} }, "ascending"},
		{"repeated cell", func(r *Receptors) { r.Records[0].Cells = []int{1, 1} }, "ascending"},
		{"negative cell", func(r *Receptors) { r.Records[0].Cells = []int{-1} }, "cell"},
		{"cell past the node count", func(r *Receptors) { r.Records[0].Cells = []int{3} }, "cell"},
		{"channel past the count", func(r *Receptors) { r.Records[0].Channel = 2 }, "channel"},
		{"negative channel", func(r *Receptors) { r.Records[0].Channel = -1 }, "channel"},
		{"unknown status", func(r *Receptors) { r.Records[0].Status = "measured" }, "status"},
		{"no evidence", func(r *Receptors) { r.Records[0].Evidence = "" }, "evidence"},
		{"no measurement kind", func(r *Receptors) { r.Records[0].MeasurementKind = "" }, "measurement_kind"},
		{"no mapping version", func(r *Receptors) { r.Records[0].MappingVersion = "" }, "mapping_version"},
		{"hypothesized without Kd", func(r *Receptors) { r.Records[0].Kd = 0 }, "engineering_kd"},
		{"hypothesized with a negative Kd", func(r *Receptors) { r.Records[0].Kd = -1 }, "engineering_kd"},
		{"hypothesized with n below one", func(r *Receptors) { r.Records[0].N = 0.5 }, "engineering_n"},
		{"hypothesized with a non-finite Kd", func(r *Receptors) { r.Records[0].Kd = math.Inf(1) }, "engineering_kd"},
		{"half declared unknown", func(r *Receptors) {
			r.AllowAssumedCoefficients = true
			r.Records[0].Status = StatusUnknown
			r.Records[0].N = 0
		}, "engineering_n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Receptors{Records: []Receptor{completeReceptor()}, Mix: MixSum}
			tc.edit(&r)
			err := r.Validate(3, 2)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if _, _, err := r.Occupancies(ChemistryState{Concentration: [][]float64{{2, 0}, {4, 1}}}, []int{0, 0, 1}); err == nil {
				t.Fatalf("Occupancies accepted %s", tc.name)
			}
		})
	}

	// An unknown record with no coefficients at all is a skip, not an error,
	// and an unresponsive record never needs coefficients.
	for _, status := range []string{StatusUnknown, StatusUnresponsive} {
		r := Receptors{Records: []Receptor{completeReceptor()}, Mix: MixSum, AllowAssumedCoefficients: true}
		r.Records[0].Status, r.Records[0].Kd, r.Records[0].N = status, 0, 0
		if err := r.Validate(3, 2); err != nil {
			t.Fatalf("a %s record without coefficients was refused: %v", status, err)
		}
	}
}

// The three statuses are three different answers, not three ways of saying
// zero: unresponsive is a measured absence of response, unknown is a missing
// coefficient the run must declare it assumed to use, and hypothesized is a
// declared engineering curve.
func TestReceptorsSeparateUnresponsiveUnknownAndHypothesized(t *testing.T) {
	state, regionOf := receptorFixture()
	records := []Receptor{
		{Cells: []int{0, 1}, Signal: "octopamine", Channel: 0, Status: StatusHypothesized, Kd: 1, N: 1,
			Evidence: "fixture", MeasurementKind: "engineering_assumption", MappingVersion: "fixture/v1"},
		{Cells: []int{2}, Signal: "octopamine", Channel: 0, Status: StatusUnknown, Kd: 2, N: 2,
			Evidence: "fixture", MeasurementKind: "engineering_assumption", MappingVersion: "fixture/v1"},
		{Cells: []int{0}, Signal: "dopamine", Channel: 1, Status: StatusUnresponsive, Kd: 1, N: 1,
			Evidence: "fixture: reported not to respond", MeasurementKind: "reported_absence", MappingVersion: "fixture/v1"},
		{Cells: []int{1}, Signal: "octopamine", Channel: 0, Status: StatusUnknown,
			Evidence: "fixture", MeasurementKind: "unknown", MappingVersion: "fixture/v1"},
	}
	want := []OccupancyRecord{
		{Receptor: 0, Cell: 0, Region: 0, Channel: 0, Status: StatusHypothesized, Occupancy: 0.6666666666666666},
		{Receptor: 0, Cell: 1, Region: 0, Channel: 0, Status: StatusHypothesized, Occupancy: 0.6666666666666666},
		{Receptor: 1, Cell: 2, Region: 1, Channel: 0, Status: StatusUnknown, Occupancy: 0, Skipped: true},
		{Receptor: 2, Cell: 0, Region: 0, Channel: 1, Status: StatusUnresponsive, Occupancy: 0},
		{Receptor: 3, Cell: 1, Region: 0, Channel: 0, Status: StatusUnknown, Occupancy: 0, Skipped: true},
	}

	r := &Receptors{Records: records, Mix: MixSum}
	got, summary, err := r.Occupancies(state, regionOf)
	if err != nil {
		t.Fatal(err)
	}
	assertOccupancies(t, got, want)
	if (summary != OccupancySummary{Assumed: 0, UnknownSkipped: 2, Unresponsive: 1}) {
		t.Fatalf("summary %+v without assumed coefficients", summary)
	}

	// Allowing assumed coefficients computes the unknown record that has them
	// and marks it assumed. The record without coefficients is still skipped.
	r.AllowAssumedCoefficients = true
	got, summary, err = r.Occupancies(state, regionOf)
	if err != nil {
		t.Fatal(err)
	}
	want[2] = OccupancyRecord{Receptor: 1, Cell: 2, Region: 1, Channel: 0, Status: StatusUnknown, Occupancy: 0.8, Assumed: true}
	assertOccupancies(t, got, want)
	if (summary != OccupancySummary{Assumed: 1, UnknownSkipped: 1, Unresponsive: 1}) {
		t.Fatalf("summary %+v with assumed coefficients", summary)
	}

	// An unresponsive receptor stays at zero however much chemical there is,
	// and is never reported as assumed.
	loud := ChemistryState{Concentration: [][]float64{{2, 1e300}, {4, 1}}}
	got, summary, err = r.Occupancies(loud, regionOf)
	if err != nil {
		t.Fatal(err)
	}
	if got[3].Occupancy != 0 || got[3].Assumed || got[3].Skipped || got[3].Status != StatusUnresponsive {
		t.Fatalf("an unresponsive receptor at c = 1e300 reported %+v", got[3])
	}
	if summary.Unresponsive != 1 {
		t.Fatalf("summary %+v", summary)
	}
}

func assertOccupancies(t *testing.T, got, want []OccupancyRecord) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d occupancy records, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Receptor != want[i].Receptor || got[i].Cell != want[i].Cell || got[i].Region != want[i].Region ||
			got[i].Channel != want[i].Channel || got[i].Status != want[i].Status ||
			got[i].Assumed != want[i].Assumed || got[i].Skipped != want[i].Skipped {
			t.Fatalf("record %d is %+v, want %+v", i, got[i], want[i])
		}
		if math.Abs(got[i].Occupancy-want[i].Occupancy) > 1e-15 {
			t.Fatalf("record %d occupancy %v, want %v", i, got[i].Occupancy, want[i].Occupancy)
		}
	}
}

func TestReceptorsOccupanciesRejectsAnInvalidStateOrRegionMap(t *testing.T) {
	r := &Receptors{Records: []Receptor{completeReceptor()}, Mix: MixSum}
	state, regionOf := receptorFixture()
	if _, _, err := r.Occupancies(state, regionOf); err != nil {
		t.Fatalf("the reference fixture was refused: %v", err)
	}
	cases := []struct {
		name     string
		state    ChemistryState
		regionOf []int
		want     string
	}{
		{"no concentration", ChemistryState{}, regionOf, "concentration"},
		{"ragged concentration", ChemistryState{Concentration: [][]float64{{2, 0}, {4}}}, regionOf, "concentration"},
		{"negative concentration", ChemistryState{Concentration: [][]float64{{-2, 0}, {4, 1}}}, regionOf, "negative"},
		{"non-finite concentration", ChemistryState{Concentration: [][]float64{{math.NaN(), 0}, {4, 1}}}, regionOf, "finite"},
		{"no region map", state, nil, "region"},
		{"region out of range", state, []int{0, 0, 2}, "region"},
		{"negative region", state, []int{0, 0, -1}, "region"},
		{"region map shorter than the cells", state, []int{0}, "cell"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, summary, err := r.Occupancies(tc.state, tc.regionOf)
			if err == nil {
				t.Fatalf("accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			if got != nil || (summary != OccupancySummary{}) {
				t.Fatalf("a refused call returned %+v and %+v", got, summary)
			}
		})
	}
	if _, _, err := (*Receptors)(nil).Occupancies(state, regionOf); err == nil {
		t.Fatal("a nil Receptors was accepted")
	}
}

func TestReceptorsOwnTheirRecordsAndResults(t *testing.T) {
	state, regionOf := receptorFixture()
	r := &Receptors{Records: []Receptor{completeReceptor()}, Mix: MixSum}
	got, _, err := r.Occupancies(state, regionOf)
	if err != nil {
		t.Fatal(err)
	}
	state.Concentration[0][0] = 99
	regionOf[0] = 1
	got[0].Occupancy = -1
	again, _, err := r.Occupancies(receptorFixture())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(again[0].Occupancy-0.6666666666666666) > 1e-15 {
		t.Fatalf("a later call gave %v", again[0].Occupancy)
	}
	if r.Records[0].Cells[0] != 0 {
		t.Fatalf("Occupancies rewrote the declared cells: %v", r.Records[0].Cells)
	}
}
