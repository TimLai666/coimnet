package trajectory

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const stimulusHeader = "fname,fly,condition,segment,t,x_cm,y_cm,reward,dist_fictive_reward_cm,led_1"

func stimulusCSV(rows ...string) []byte {
	return []byte(stimulusHeader + "\n" + strings.Join(rows, "\n") + "\n")
}

func TestReadWithStimulusHistoryAlignsFlagsAndDatasetRows(t *testing.T) {
	raw := stimulusCSV(
		"trial-a.csv,0,rewarded,baseline,0,0,0,99,12,0",
		"trial-a.csv,0,rewarded,baseline,0.2,1,2,98,11,1.5",
		"trial-a.csv,0,rewarded,baseline,0.5,3,3,97,10,0",
		"trial-b.csv,1,non-rewarded,baseline,0,10,20,96,9,0",
		"trial-b.csv,1,non-rewarded,baseline,0.1,11,20,95,8,2",
		"trial-b.csv,1,non-rewarded,baseline,0.2,12,21,94,7,0",
	)
	path := writeCSV(t, "stimulus.csv", raw, false)
	dataset, records, err := ReadWithStimulusHistory(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err != nil {
		t.Fatalf("ReadWithStimulusHistory: %v", err)
	}
	legacy, err := Read(context.Background(), path, fixtureSource(raw), DefaultLimits())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(dataset, legacy) {
		t.Fatalf("dataset differs from Read: got=%+v want=%+v", dataset, legacy)
	}
	want := []StimulusRecord{
		{TrialID: makeTrialID("trial-a.csv", "0"), T: 0, LED: 0, Delivered: false, ScheduledOnly: false},
		{TrialID: makeTrialID("trial-a.csv", "0"), T: 0.2, LED: 1.5, Delivered: true, ScheduledOnly: false},
		{TrialID: makeTrialID("trial-a.csv", "0"), T: 0.5, LED: 0, Delivered: false, ScheduledOnly: false},
		{TrialID: makeTrialID("trial-b.csv", "1"), T: 0, LED: 0, Delivered: false, ScheduledOnly: false},
		{TrialID: makeTrialID("trial-b.csv", "1"), T: 0.1, LED: 2, Delivered: false, ScheduledOnly: true},
		{TrialID: makeTrialID("trial-b.csv", "1"), T: 0.2, LED: 0, Delivered: false, ScheduledOnly: false},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %+v, want %+v", records, want)
	}
	if len(records) != len(dataset.Rows) {
		t.Fatalf("records=%d rows=%d, want equal lengths", len(records), len(dataset.Rows))
	}
	typ := reflect.TypeOf(StimulusRecord{})
	for fieldName, wantTag := range map[string]string{
		"TrialID":       "trial_id",
		"T":             "t",
		"LED":           "led_1",
		"Delivered":     "delivered",
		"ScheduledOnly": "scheduled_only",
	} {
		field, ok := typ.FieldByName(fieldName)
		if !ok || field.Tag.Get("json") != wantTag {
			t.Fatalf("StimulusRecord.%s json tag = %q, want %q", fieldName, field.Tag.Get("json"), wantTag)
		}
	}
	encoded, err := json.Marshal(records[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"trial_id":"trial-a.csv\u001f0","t":0.2,"led_1":1.5,"delivered":true,"scheduled_only":false}` {
		t.Fatalf("stimulus JSON = %s", encoded)
	}
}

func TestReadKeepsLegacyRowsWithUnknownLEDColumn(t *testing.T) {
	withoutLED := []byte(fixtureHeader + "\n" +
		"trial.csv,0,rewarded,baseline,0,0,0,1,2\n" +
		"trial.csv,0,rewarded,baseline,0.2,1,2,3,4\n")
	withUnknownLED := stimulusCSV(
		"trial.csv,0,rewarded,baseline,0,0,0,1,2,not-a-number",
		"trial.csv,0,rewarded,baseline,0.2,1,2,3,4,still-ignored",
	)
	basePath := writeCSV(t, "legacy-base.csv", withoutLED, false)
	withPath := writeCSV(t, "legacy-led.csv", withUnknownLED, false)
	base, err := Read(context.Background(), basePath, fixtureSource(withoutLED), DefaultLimits())
	if err != nil {
		t.Fatalf("Read without LED: %v", err)
	}
	got, err := Read(context.Background(), withPath, fixtureSource(withUnknownLED), DefaultLimits())
	if err != nil {
		t.Fatalf("Read with unknown LED column: %v", err)
	}
	if !reflect.DeepEqual(got.Rows, base.Rows) {
		t.Fatalf("legacy rows changed when unknown LED was added: got=%+v want=%+v", got.Rows, base.Rows)
	}
}

func TestReadWithStimulusHistoryRejectsInvalidLEDAndCondition(t *testing.T) {
	baseRow := "trial.csv,0,rewarded,baseline,0,0,0,1,2,%s"
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "missing LED", raw: []byte(strings.Replace(stimulusHeader, ",led_1", "", 1) + "\ntrial.csv,0,rewarded,baseline,0,0,0,1,2\n"), want: "led_1"},
		{name: "empty LED", raw: stimulusCSV("trial.csv,0,rewarded,baseline,0,0,0,1,2,"), want: "led_1"},
		{name: "NaN LED", raw: stimulusRow(baseRow, "NaN"), want: "led_1"},
		{name: "infinite LED", raw: stimulusRow(baseRow, "Inf"), want: "led_1"},
		{name: "negative LED", raw: stimulusRow(baseRow, "-0.1"), want: "led_1"},
		{name: "unknown condition", raw: stimulusCSV("trial.csv,0,Rewarded,baseline,0,0,0,1,2,0"), want: "condition"},
		{name: "condition conflict", raw: stimulusCSV(
			"trial.csv,0,rewarded,baseline,0,0,0,1,2,0",
			"trial.csv,0,non-rewarded,baseline,0.1,1,1,3,4,0",
		), want: "condition conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeCSV(t, "invalid-stimulus.csv", test.raw, false)
			dataset, records, err := ReadWithStimulusHistory(context.Background(), path, fixtureSource(test.raw), DefaultLimits())
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if !reflect.DeepEqual(dataset, Dataset{}) || records != nil {
				t.Fatalf("partial result: dataset=%+v records=%+v", dataset, records)
			}
		})
	}
}

func TestReadWithStimulusHistoryInheritsOrderingFingerprintCancellationAndCapacityChecks(t *testing.T) {
	tests := []struct {
		name   string
		raw    []byte
		ctx    context.Context
		source func([]byte) Source
		limits func(Limits) Limits
		want   string
	}{
		{
			name: "time rollback",
			raw: stimulusCSV(
				"trial.csv,0,rewarded,baseline,0.2,0,0,1,2,0",
				"trial.csv,0,rewarded,baseline,0.1,1,1,3,4,0",
			),
			want: "time out of order",
		},
		{
			name: "duplicate time",
			raw: stimulusCSV(
				"trial.csv,0,rewarded,baseline,0,0,0,1,2,0",
				"trial.csv,0,rewarded,baseline,0,1,1,3,4,0",
			),
			want: "duplicate time",
		},
		{
			name: "fingerprint mismatch",
			raw:  stimulusCSV("trial.csv,0,rewarded,baseline,0,0,0,1,2,0"),
			source: func(raw []byte) Source {
				source := fixtureSource(raw)
				source.SHA256 = strings.Repeat("0", 64)
				return source
			},
			want: "SHA-256",
		},
		{
			name: "canceled",
			raw:  stimulusCSV("trial.csv,0,rewarded,baseline,0,0,0,1,2,0"),
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			want: "canceled",
		},
		{
			name: "row capacity",
			raw: stimulusCSV(
				"trial.csv,0,rewarded,baseline,0,0,0,1,2,0",
				"trial.csv,0,rewarded,baseline,0.1,1,1,3,4,0",
			),
			limits: func(limits Limits) Limits {
				limits.MaxRows = 1
				return limits
			},
			want: "row count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeCSV(t, "ordering-stimulus.csv", test.raw, false)
			ctx := test.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			source := fixtureSource(test.raw)
			if test.source != nil {
				source = test.source(test.raw)
			}
			limits := DefaultLimits()
			if test.limits != nil {
				limits = test.limits(limits)
			}
			dataset, records, err := ReadWithStimulusHistory(ctx, path, source, limits)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if !reflect.DeepEqual(dataset, Dataset{}) || records != nil {
				t.Fatalf("partial result: dataset=%+v records=%+v", dataset, records)
			}
		})
	}
}

func stimulusRow(format, led string) []byte {
	return stimulusCSV(strings.Replace(format, "%s", led, 1))
}
