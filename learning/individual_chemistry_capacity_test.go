package learning_test

import (
	"reflect"
	"testing"
)

func individualChemicalCapacityMaxInt() int { return int(^uint(0) >> 1) }

// TestIndividualChemicalCapacityEnableRejectsHugeRegionsAtomically uses the
// existing small public individual fixture through one successful chemical
// Advance, then checks that an unrepresentable replacement is refused without
// changing the persistent boundary, last report, or caller-owned declaration.
func TestIndividualChemicalCapacityEnableRejectsHugeRegionsAtomically(t *testing.T) {
	profile := individualChemicalTrainingProfiles()[0]
	individual, before, beforeReport := prepareIndividualChemicalTrainingBoundary(t, profile)
	if before.Chemical == nil || beforeReport.Steps == 0 {
		t.Fatalf("fixture did not produce a complete chemical boundary: snapshot=%+v report=%+v", before.Chemical, beforeReport)
	}

	declaration := individualChemicalTrainingControllerChemistry(individualChemicalTrainingNodes(profile.config))
	// Remove the fixture's tiny transport matrix before enlarging the region
	// header. Keeping that matrix would reject the declaration for its row count
	// before the region-grid capacity check is reached.
	declaration.Chemistry.Transport = nil
	declaration.Chemistry.Regions = individualChemicalCapacityMaxInt()
	callerBefore := declaration.Clone()

	var (
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		err = individual.EnableChemistry(declaration)
	}()
	if panicked != nil {
		t.Errorf("EnableChemistry(Regions=MaxInt) panicked: %v", panicked)
	} else if err == nil {
		t.Errorf("EnableChemistry(Regions=MaxInt) returned nil error")
	}
	if !reflect.DeepEqual(declaration, callerBefore) {
		t.Fatalf("EnableChemistry changed the caller-owned declaration: got=%+v want=%+v", declaration, callerBefore)
	}
	if got := individual.Snapshot(); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected EnableChemistry changed the full individual snapshot: got=%+v want=%+v", got, before)
	}
	if got := individual.ChemistryReport(); !reflect.DeepEqual(got, beforeReport) {
		t.Fatalf("rejected EnableChemistry changed ChemistryReport: got=%+v want=%+v", got, beforeReport)
	}
}
