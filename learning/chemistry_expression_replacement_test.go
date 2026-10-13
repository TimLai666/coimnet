package learning_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/modulation"
)

type chemistryExpressionReplacementFixture struct {
	name        string
	config      learning.Config
	parameters  learning.Parameters
	warmup      [][]float64
	advanceRows [][]float64
	target      []float64
}

func chemistryExpressionReplacementFixtures() []chemistryExpressionReplacementFixture {
	return []chemistryExpressionReplacementFixture{
		{
			name:        "continuous",
			config:      chemContinuousConfig(),
			parameters:  chemContinuousParameters(),
			warmup:      [][]float64{{1, 0}, {1, 0}},
			advanceRows: [][]float64{{1, 0}, {1, 0}},
			target:      []float64{0.4},
		},
		{
			name:        "lif",
			config:      chemLIFConfig(),
			parameters:  chemLIFParameters(),
			warmup:      [][]float64{{2, 0}, {2, 0}},
			advanceRows: [][]float64{{2, 0}, {2, 0}},
			target:      []float64{0.4},
		},
	}
}

func expressionReplacementGain() learning.ExpressionGain {
	return learning.ExpressionGain{Nodes: []int{0}, Receptor: 0, Scale: 1, Min: 0.5, Max: 2}
}

func expressionReplacementWithoutReceptor() modulation.ChemistryConfig {
	declaration := sensitivityDeclaration()
	declaration.Receptors.Records = nil
	declaration.Effects = nil
	return declaration
}

// TestEnableChemistryRejectsExpressionReceptorReplacementAtomically checks the
// public boundary where a live expression names a receptor removed by a new
// chemistry declaration. The failed replacement must leave the entire previous
// snapshot and report untouched, and all later public operations must continue
// bit for bit like an untouched twin, including after a restore.
func TestEnableChemistryRejectsExpressionReceptorReplacementAtomically(t *testing.T) {
	for _, fixture := range chemistryExpressionReplacementFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			subject := newIndividual(t, fixture.config, fixture.parameters)
			twin := newIndividual(t, fixture.config, fixture.parameters)
			for _, individual := range []*learning.Individual{subject, twin} {
				enableChemistry(t, individual, sensitivityDeclaration())
				if err := individual.SetExpressionGain(expressionReplacementGain()); err != nil {
					t.Fatalf("SetExpressionGain() error = %v", err)
				}
				advance(t, individual, fixture.warmup)
			}

			before := subject.Snapshot()
			beforeReport := subject.ChemistryReport()
			if before.Chemical == nil || before.Chemical.Expression == nil {
				t.Fatal("fixture did not retain the expression in the chemical snapshot")
			}
			if beforeReport.ExpressionGainApplied == 0 {
				t.Fatalf("fixture did not apply the expression gain: %+v", beforeReport)
			}
			if !reflect.DeepEqual(before, twin.Snapshot()) {
				t.Fatal("subject and untouched twin diverged before the rejected replacement")
			}
			if !reflect.DeepEqual(beforeReport, twin.ChemistryReport()) {
				t.Fatal("subject and untouched twin reports diverged before the rejected replacement")
			}

			if err := subject.EnableChemistry(expressionReplacementWithoutReceptor()); err == nil {
				t.Fatal("EnableChemistry() accepted a replacement that removed the expression's receptor")
			}
			if got := subject.Snapshot(); !reflect.DeepEqual(got, before) {
				t.Fatalf("rejected replacement changed the snapshot:\n got  %+v\n want %+v", got, before)
			}
			if got := subject.ChemistryReport(); !reflect.DeepEqual(got, beforeReport) {
				t.Fatalf("rejected replacement changed the report:\n got  %+v\n want %+v", got, beforeReport)
			}

			restored, err := learning.RestoreIndividual(subject.Snapshot())
			if err != nil {
				t.Fatalf("RestoreIndividual() of the preserved snapshot error = %v", err)
			}
			if got := restored.Snapshot(); !reflect.DeepEqual(got, before) {
				t.Fatalf("restored snapshot = %+v, want %+v", got, before)
			}

			wantOutput := advance(t, twin, fixture.advanceRows)
			gotOutput := advance(t, subject, fixture.advanceRows)
			restoredOutput := advance(t, restored, fixture.advanceRows)
			if !sameRows(gotOutput, wantOutput) || !sameRows(restoredOutput, wantOutput) {
				t.Fatalf("post-rejection Advance() diverged:\n subject   %v\n restored  %v\n twin      %v", gotOutput, restoredOutput, wantOutput)
			}
			if got, want := subject.Snapshot(), twin.Snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("post-rejection subject snapshot diverged from twin:\n got  %+v\n want %+v", got, want)
			}
			if got, want := restored.Snapshot(), twin.Snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("post-restore Advance() snapshot diverged from twin:\n got  %+v\n want %+v", got, want)
			}
			if got, want := subject.ChemistryReport(), twin.ChemistryReport(); !reflect.DeepEqual(got, want) {
				t.Fatalf("post-rejection subject report diverged from twin:\n got  %+v\n want %+v", got, want)
			}
			if got, want := restored.ChemistryReport(), twin.ChemistryReport(); !reflect.DeepEqual(got, want) {
				t.Fatalf("post-restore Advance() report diverged from twin:\n got  %+v\n want %+v", got, want)
			}

			wantStep, err := twin.TrainEpisode(context.Background(), fixture.advanceRows, fixture.target)
			if err != nil {
				t.Fatalf("untouched twin TrainEpisode() error = %v", err)
			}
			gotStep, err := subject.TrainEpisode(context.Background(), fixture.advanceRows, fixture.target)
			if err != nil {
				t.Fatalf("post-rejection subject TrainEpisode() error = %v", err)
			}
			restoredStep, err := restored.TrainEpisode(context.Background(), fixture.advanceRows, fixture.target)
			if err != nil {
				t.Fatalf("post-restore TrainEpisode() error = %v", err)
			}
			if !reflect.DeepEqual(gotStep, wantStep) || !reflect.DeepEqual(restoredStep, wantStep) {
				t.Fatalf("post-rejection TrainEpisode() diverged:\n subject  %+v\n restored %+v\n twin     %+v", gotStep, restoredStep, wantStep)
			}
			if got, want := subject.Snapshot(), twin.Snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("post-rejection TrainEpisode() snapshot diverged from twin:\n got  %+v\n want %+v", got, want)
			}
			if got, want := restored.Snapshot(), twin.Snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("post-restore TrainEpisode() snapshot diverged from twin:\n got  %+v\n want %+v", got, want)
			}
		})
	}
}

// TestEnableChemistryCompatibleReplacementAndClearExpression keeps the valid
// replacement path open for an existing receptor, then verifies that an
// explicit ClearExpressionGain permits the same receptor removal that must be
// rejected while the expression is still declared.
func TestEnableChemistryCompatibleReplacementAndClearExpression(t *testing.T) {
	for _, fixture := range chemistryExpressionReplacementFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			a := newIndividual(t, fixture.config, fixture.parameters)
			enableChemistry(t, a, sensitivityDeclaration())
			if err := a.SetExpressionGain(expressionReplacementGain()); err != nil {
				t.Fatalf("SetExpressionGain() error = %v", err)
			}
			advance(t, a, fixture.warmup)

			compatible := sensitivityDeclaration()
			compatible.Effects[0].GammaScale = 2
			if err := a.EnableChemistry(compatible); err != nil {
				t.Fatalf("EnableChemistry() rejected a replacement retaining receptor 0: %v", err)
			}
			compatibleSnapshot := a.Snapshot()
			if compatibleSnapshot.Chemical == nil {
				t.Fatal("compatible replacement dropped the chemical snapshot")
			}
			if expression := compatibleSnapshot.Chemical.Expression; expression == nil || expression.Receptor != 0 {
				t.Fatalf("compatible replacement lost the valid expression: %+v", expression)
			}

			a.ClearExpressionGain()
			if err := a.EnableChemistry(expressionReplacementWithoutReceptor()); err != nil {
				t.Fatalf("EnableChemistry() after ClearExpressionGain() error = %v", err)
			}
			snapshot := a.Snapshot()
			if snapshot.Chemical == nil {
				t.Fatal("chemistry snapshot is nil after a receptor-free replacement")
			}
			if snapshot.Chemical.Expression != nil {
				t.Fatalf("ClearExpressionGain() did not remove the expression: %+v", snapshot.Chemical.Expression)
			}
			if len(snapshot.Chemical.Config.Receptors.Records) != 0 {
				t.Fatalf("receptor-free replacement retained %d receptor records", len(snapshot.Chemical.Config.Receptors.Records))
			}

			advance(t, a, fixture.advanceRows[:1])
			if _, err := a.TrainEpisode(context.Background(), fixture.advanceRows[:1], fixture.target); err != nil {
				t.Fatalf("TrainEpisode() after ClearExpressionGain() and receptor removal error = %v", err)
			}
		})
	}
}
