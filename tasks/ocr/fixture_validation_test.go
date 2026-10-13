package ocr_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/tasks/ocr"
)

// smallRenderFixtureConfig keeps the valid boundary checks on the public
// fixture API while avoiding the larger evidence fixture in fixture_test.go.
func smallRenderFixtureConfig() ocr.FixtureConfig {
	return ocr.FixtureConfig{
		Seeds:        []uint64{1},
		Alphabet:     []rune{'a', 'b'},
		TrainSamples: 1,
		TestSamples:  1,
		MinLen:       1,
		MaxLen:       1,
		Epochs:       1,
		Hidden:       2,
		LearningRate: 0.05,
		TrainFamily:  1,
		TestFamily:   2,
	}
}

func TestFixtureRenderOptionsRejectInvalidValues(t *testing.T) {
	fields := []struct {
		name string
		set  func(*ocr.FixtureConfig, float64)
	}{
		{name: "Background", set: func(c *ocr.FixtureConfig, value float64) { c.Background = value }},
		{name: "Noise", set: func(c *ocr.FixtureConfig, value float64) { c.Noise = value }},
	}
	values := []struct {
		name  string
		value float64
	}{
		{name: "negative", value: -0.01},
		{name: "above-half", value: 0.5001},
		{name: "NaN", value: math.NaN()},
		{name: "positive-infinity", value: math.Inf(1)},
		{name: "negative-infinity", value: math.Inf(-1)},
	}

	for _, field := range fields {
		for _, value := range values {
			t.Run(field.name+"/"+value.name, func(t *testing.T) {
				c := smallRenderFixtureConfig()
				field.set(&c, value.value)

				if err := c.Validate(); err == nil {
					t.Errorf("Validate accepted %s=%v", field.name, value.value)
				}
				report, err := ocr.RunOCRFixture(context.Background(), c)
				if err == nil {
					t.Fatalf("RunOCRFixture accepted %s=%v", field.name, value.value)
				}
				if !reflect.DeepEqual(report, (ocr.OCRReport{})) {
					t.Fatalf("RunOCRFixture returned non-zero report for %s=%v: %+v", field.name, value.value, report)
				}
			})
		}
	}
}

func TestFixtureRenderOptionsAcceptBoundaries(t *testing.T) {
	for _, background := range []float64{0, 0.5} {
		for _, noise := range []float64{0, 0.5} {
			t.Run(fmt.Sprintf("background=%g/noise=%g", background, noise), func(t *testing.T) {
				c := smallRenderFixtureConfig()
				c.Background = background
				c.Noise = noise
				if err := c.Validate(); err != nil {
					t.Fatalf("Validate rejected boundary options: %v", err)
				}

				report, err := ocr.RunOCRFixture(context.Background(), c)
				if err != nil {
					t.Fatalf("RunOCRFixture rejected boundary options: %v", err)
				}
				if len(report.Seeds) != 1 {
					t.Fatalf("report has %d seeds, want 1", len(report.Seeds))
				}
				if report.Seeds[0].Failed {
					t.Fatalf("boundary fixture seed failed: %s", report.Seeds[0].Error)
				}
			})
		}
	}
}
