package trajectory

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const returnTargetHeader = "fname,fly,condition,segment,t,x_cm,y_cm,estimated_food_x_cm,estimated_food_y_cm,ignored"

func returnTargetCSV(rows ...string) []byte {
	return []byte(returnTargetHeader + "\n" + strings.Join(rows, "\n") + "\n")
}

func TestReturnTargetContractAndGzipReadSharesDatasetContract(t *testing.T) {
	raw := returnTargetCSV(
		"trial-a.csv,0,rewarded,baseline,0,0,0,bad,ignored,garbage",
		"trial-a.csv,0,rewarded,after_relocation,0.1,1,2,0,0,ignored",
		"trial-a.csv,0,rewarded,after_relocation,0.2,2,3,0,0,ignored",
		"trial-b.csv,1,non-rewarded,baseline,0,10,20,not-a-number,,garbage",
		"trial-b.csv,1,non-rewarded,after_relocation,0.1,11,21,-4.5,6.25,ignored",
	)
	path := writeCSV(t, "trajectory.csv.gz", raw, true)
	source := fixtureSource(mustRawGzip(t, raw))

	wantDataset, err := Read(context.Background(), path, source, DefaultLimits())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	gotDataset, gotTargets, err := ReadWithReturnTargets(context.Background(), path, source, DefaultLimits())
	if err != nil {
		t.Fatalf("ReadWithReturnTargets: %v", err)
	}
	if !reflect.DeepEqual(gotDataset, wantDataset) {
		t.Fatalf("dataset differs from Read: got=%+v want=%+v", gotDataset, wantDataset)
	}
	wantTargets := []ReturnTarget{
		{TrialID: makeTrialID("trial-a.csv", "0"), XCM: 0, YCM: 0},
		{TrialID: makeTrialID("trial-b.csv", "1"), XCM: -4.5, YCM: 6.25},
	}
	if !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("targets = %+v, want %+v", gotTargets, wantTargets)
	}
	if gotTargets[0].XCM != 0 || gotTargets[0].YCM != 0 {
		t.Fatalf("zero target was not preserved: %+v", gotTargets[0])
	}

	typ := reflect.TypeOf(ReturnTarget{})
	for fieldName, want := range map[string]string{"TrialID": "trial_id", "XCM": "x_cm", "YCM": "y_cm"} {
		field, ok := typ.FieldByName(fieldName)
		if !ok || field.Tag.Get("json") != want {
			t.Fatalf("ReturnTarget.%s json tag = %q, want %q", fieldName, field.Tag.Get("json"), want)
		}
	}
	encoded, err := json.Marshal(gotTargets[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"trial_id":"trial-a.csv\u001f0","x_cm":0,"y_cm":0}` {
		t.Fatalf("target JSON = %s", encoded)
	}
}

func TestReadWithReturnTargetsRequiresBothTargetColumns(t *testing.T) {
	for _, missing := range []string{"estimated_food_x_cm", "estimated_food_y_cm"} {
		t.Run(missing, func(t *testing.T) {
			header := strings.Replace(returnTargetHeader, ","+missing, "", 1)
			raw := []byte(header + "\ntrial.csv,0,rewarded,after_relocation,0,0,0,0\n")
			path := writeCSV(t, "missing.csv", raw, false)
			dataset, targets, err := ReadWithReturnTargets(context.Background(), path, fixtureSource(raw), DefaultLimits())
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("error = %v, want missing %s", err, missing)
			}
			if !reflect.DeepEqual(dataset, Dataset{}) || targets != nil {
				t.Fatalf("partial result on missing column: dataset=%+v targets=%+v", dataset, targets)
			}
		})
	}
}

func TestReadWithReturnTargetsRejectsInvalidAfterRelocationCenters(t *testing.T) {
	for _, test := range []struct {
		name string
		x, y string
		want string
	}{
		{name: "nan x", x: "NaN", y: "1", want: "estimated_food_x_cm"},
		{name: "positive infinity x", x: "Inf", y: "1", want: "estimated_food_x_cm"},
		{name: "negative infinity y", x: "1", y: "-Inf", want: "estimated_food_y_cm"},
		{name: "empty x", x: "", y: "1", want: "estimated_food_x_cm"},
		{name: "malformed y", x: "1", y: "oops", want: "estimated_food_y_cm"},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(returnTargetHeader + "\ntrial.csv,0,rewarded,after_relocation,0,0,0," + test.x + "," + test.y + ",ignored\n")
			path := writeCSV(t, "invalid-center.csv", raw, false)
			dataset, targets, err := ReadWithReturnTargets(context.Background(), path, fixtureSource(raw), DefaultLimits())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
			if !reflect.DeepEqual(dataset, Dataset{}) || targets != nil {
				t.Fatalf("partial result on invalid target: dataset=%+v targets=%+v", dataset, targets)
			}
		})
	}
}

func TestReadWithReturnTargetsRejectsConflictingAfterRelocationCenters(t *testing.T) {
	raw := returnTargetCSV(
		"trial.csv,0,rewarded,after_relocation,0,0,0,1,2,ignored",
		"trial.csv,0,rewarded,after_relocation,0.1,1,0,1,3,ignored",
	)
	path := writeCSV(t, "conflict.csv", raw, false)
	dataset, targets, err := ReadWithReturnTargets(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err == nil || !strings.Contains(err.Error(), "target") || !strings.Contains(err.Error(), "trial.csv") {
		t.Fatalf("error = %v, want target conflict", err)
	}
	if !reflect.DeepEqual(dataset, Dataset{}) || targets != nil {
		t.Fatalf("partial result on conflicting target: dataset=%+v targets=%+v", dataset, targets)
	}
}

func TestReadWithReturnTargetsIgnoresNonAfterRelocationCentersAndMissingTargets(t *testing.T) {
	raw := returnTargetCSV(
		"trial-a.csv,0,rewarded,baseline,0,0,0,NaN,,garbage",
		"trial-a.csv,0,rewarded,baseline,0.1,1,0,not-a-number,,garbage",
		"trial-a.csv,0,rewarded,after_relocation,0.2,2,0,3,4,ignored",
		"trial-b.csv,1,non-rewarded,baseline,0,5,6,,,",
	)
	path := writeCSV(t, "non-after.csv", raw, false)
	dataset, targets, err := ReadWithReturnTargets(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err != nil {
		t.Fatalf("ReadWithReturnTargets: %v", err)
	}
	if len(dataset.Rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(dataset.Rows))
	}
	if !reflect.DeepEqual(targets, []ReturnTarget{{TrialID: makeTrialID("trial-a.csv", "0"), XCM: 3, YCM: 4}}) {
		t.Fatalf("targets = %+v", targets)
	}

	oldDataset, err := Read(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err != nil {
		t.Fatalf("Read rejected ignored target values: %v", err)
	}
	if !reflect.DeepEqual(oldDataset, dataset) {
		t.Fatalf("Read dataset differs: old=%+v new=%+v", oldDataset, dataset)
	}
}

func TestReadWithReturnTargetsReturnsNoTargetForTrialsWithoutAfterRelocation(t *testing.T) {
	raw := returnTargetCSV(
		"trial.csv,0,rewarded,baseline,0,0,0,,,",
		"trial.csv,0,rewarded,baseline,0.1,1,1,garbage,garbage,ignored",
	)
	path := writeCSV(t, "no-after.csv", raw, false)
	dataset, targets, err := ReadWithReturnTargets(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err != nil {
		t.Fatalf("ReadWithReturnTargets: %v", err)
	}
	if len(dataset.Rows) != 2 || len(targets) != 0 {
		t.Fatalf("rows=%d targets=%v, want 2 rows and no targets", len(dataset.Rows), targets)
	}
}

func TestReadWithReturnTargetsCancellationReturnsNoPartialResults(t *testing.T) {
	raw := returnTargetCSV("trial.csv,0,rewarded,after_relocation,0,0,0,1,2,ignored")
	path := writeCSV(t, "canceled.csv", raw, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dataset, targets, err := ReadWithReturnTargets(ctx, path, fixtureSource(raw), DefaultLimits())
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if !reflect.DeepEqual(dataset, Dataset{}) || targets != nil {
		t.Fatalf("partial result after cancellation: dataset=%+v targets=%+v", dataset, targets)
	}
}

func TestReadWithReturnTargetsEarlyErrorsReturnNoPartialResults(t *testing.T) {
	raw := returnTargetCSV(
		"trial.csv,0,rewarded,after_relocation,0,0,0,1,2,ignored",
		"trial.csv,0,rewarded,after_relocation,0.1,1,0,1,2,ignored",
	)
	path := writeCSV(t, "early-errors.csv", raw, false)
	tests := []struct {
		name   string
		ctx    context.Context
		source Source
		limits Limits
		want   string
	}{
		{name: "nil context", ctx: nil, source: fixtureSource(raw), limits: DefaultLimits(), want: "context"},
		{name: "hash mismatch", ctx: context.Background(), source: func() Source {
			source := fixtureSource(raw)
			source.SHA256 = strings.Repeat("0", 64)
			return source
		}(), limits: DefaultLimits(), want: "SHA-256"},
		{name: "row limit", ctx: context.Background(), source: fixtureSource(raw), limits: func() Limits {
			limits := DefaultLimits()
			limits.MaxRows = 1
			return limits
		}(), want: "row count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataset, targets, err := ReadWithReturnTargets(test.ctx, path, test.source, test.limits)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
			if !reflect.DeepEqual(dataset, Dataset{}) || targets != nil {
				t.Fatalf("partial result: dataset=%+v targets=%+v", dataset, targets)
			}
		})
	}
}

func TestReturnTargetMetadataDoesNotEnterInput(t *testing.T) {
	typ := reflect.TypeOf(Input{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		for _, forbidden := range []string{"reward", "fictive", "condition", "segment", "trial", "fly", "target"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Input field %q exposes return metadata %q", typ.Field(i).Name, forbidden)
			}
		}
	}
}
