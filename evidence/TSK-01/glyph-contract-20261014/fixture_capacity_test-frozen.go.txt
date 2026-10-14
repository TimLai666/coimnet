package ocr_test

import (
	"context"
	"encoding/json"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"reflect"
	"testing"
)

func capacityRunFixture(t *testing.T, c ocr.FixtureConfig) (report ocr.OCRReport, err error) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("RunOCRFixture panicked: %v", p)
		}
	}()
	return ocr.RunOCRFixture(context.Background(), c)
}
func TestFixtureRejectsUnrepresentableStorageBeforeTraining(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	maxLine := (maxInt/64 + 1) / 9
	cases := []struct {
		name string
		edit func(*ocr.FixtureConfig)
	}{{"train_samples", func(c *ocr.FixtureConfig) { c.TrainSamples = maxInt }}, {"test_samples", func(c *ocr.FixtureConfig) { c.TestSamples = maxInt }}, {"fixed_length", func(c *ocr.FixtureConfig) { c.MinLen = maxInt; c.MaxLen = maxInt }}, {"pixel_bytes", func(c *ocr.FixtureConfig) { c.MinLen = maxLine + 1; c.MaxLen = maxLine + 1 }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureConfig()
			c.Seeds = []uint64{1}
			tc.edit(&c)
			before, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Validate(); err == nil {
				t.Error("Validate accepted impossible dimensions")
			}
			report, err := capacityRunFixture(t, c)
			if err == nil || !reflect.DeepEqual(report, ocr.OCRReport{}) {
				t.Errorf("RunOCRFixture did not reject before building report: %v", err)
			}
			after, _ := json.Marshal(c)
			if string(after) != string(before) {
				t.Error("refusal changed caller config")
			}
		})
	}
}
