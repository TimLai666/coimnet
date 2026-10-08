package experiment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment/nav2d"
	"github.com/TimLai666/coimnet/learning"
)

type comparisonFiniteCase struct {
	name     string
	interval float64
}

func comparisonFiniteCases() []comparisonFiniteCase {
	return []comparisonFiniteCase{
		{name: "NaN", interval: math.NaN()},
		{name: "+Inf", interval: math.Inf(1)},
		{name: "-Inf", interval: math.Inf(-1)},
		{name: "zero", interval: 0},
		{name: "one", interval: 1},
		{name: "negative", interval: -0.1},
		{name: "above_one", interval: 1.1},
	}
}

func comparisonFiniteAttributionConfig(interval float64) AttributionConfig {
	c := DefaultAttributionConfig()
	c.Env = nav2d.Config{Task: nav2d.TaskAvoidObstacles, TimeLimit: 2}
	c.Episodes = 1
	c.Hidden = 4
	c.Recurrent = 1
	c.EvalEpisodes = 1
	c.Groups = []string{AttributionNormal}
	c.Comparison.Interval = interval
	return c
}

func comparisonFiniteContinualProtocol(interval float64) ContinualProtocol {
	return ContinualProtocol{
		Tasks: []TaskSpec{{
			Name:      "A",
			Generator: GeneratorDelayedCorrelation,
			Params:    map[string]float64{"delay": 1, "channel": 0},
		}},
		Stages: []Stage{{Kind: StageTrainTask, Task: "A", Budget: 1}},
		Seeds:  []uint64{1, 2, 3},
		Evaluation: Evaluation{
			Episodes: 1,
			Metric:   MetricNegMSE,
		},
		Comparison: &PreRegistered{
			Method:    ComparisonPairedBootstrap,
			Interval:  interval,
			Baseline:  BaselineIndependent,
			Resamples: 100,
		},
	}
}

func TestAttributionComparisonFinite(t *testing.T) {
	for _, tc := range comparisonFiniteCases() {
		t.Run(tc.name, func(t *testing.T) {
			c := comparisonFiniteAttributionConfig(tc.interval)
			validateErr := c.Validate()
			if validateErr == nil {
				t.Errorf("AttributionConfig.Validate accepted interval %v", tc.interval)
			} else if !strings.Contains(validateErr.Error(), "interval") {
				t.Errorf("AttributionConfig.Validate error %q does not contain interval", validateErr)
			}

			report, runErr := RunAttribution(context.Background(), c)
			if runErr == nil {
				t.Errorf("RunAttribution accepted interval %v", tc.interval)
			} else if !strings.Contains(runErr.Error(), "interval") {
				t.Errorf("RunAttribution error %q does not contain interval", runErr)
			}
			if !reflect.DeepEqual(report, AttributionReport{}) {
				t.Errorf("RunAttribution report = %+v, want zero report", report)
			}
		})
	}
}

func TestContinualComparisonFinite(t *testing.T) {
	for _, tc := range comparisonFiniteCases() {
		t.Run(tc.name, func(t *testing.T) {
			p := comparisonFiniteContinualProtocol(tc.interval)
			validateErr := p.Validate()
			if validateErr == nil {
				t.Errorf("ContinualProtocol.Validate accepted interval %v", tc.interval)
			} else if !strings.Contains(validateErr.Error(), "interval") {
				t.Errorf("ContinualProtocol.Validate error %q does not contain interval", validateErr)
			}

			called := 0
			build := func(uint64) (*learning.Individual, error) {
				called++
				return nil, errors.New("builder must not run")
			}
			report, runErr := RunContinualMatrix(context.Background(), p, build)
			if runErr == nil {
				t.Errorf("RunContinualMatrix accepted interval %v", tc.interval)
			} else if !strings.Contains(runErr.Error(), "interval") {
				t.Errorf("RunContinualMatrix error %q does not contain interval", runErr)
			}
			if called != 0 {
				t.Errorf("builder called %d times for interval %v, want 0", called, tc.interval)
			}
			if !reflect.DeepEqual(report, ContinualReport{}) {
				t.Errorf("RunContinualMatrix report = %+v, want zero report", report)
			}
		})
	}
}

func TestComparisonFiniteValidControls(t *testing.T) {
	for _, interval := range []float64{
		math.SmallestNonzeroFloat64,
		0.5,
		0.95,
		math.Nextafter(1, 0),
	} {
		interval := interval
		t.Run(fmt.Sprintf("attribution/%g", interval), func(t *testing.T) {
			c := comparisonFiniteAttributionConfig(interval)
			if err := c.Validate(); err != nil {
				t.Errorf("AttributionConfig.Validate(%g) = %v, want nil", interval, err)
			}
		})
		t.Run(fmt.Sprintf("continual/%g", interval), func(t *testing.T) {
			p := comparisonFiniteContinualProtocol(interval)
			if err := p.Validate(); err != nil {
				t.Errorf("ContinualProtocol.Validate(%g) = %v, want nil", interval, err)
			}
		})
	}

	for _, tc := range []struct {
		name string
		make func() error
	}{
		{name: "default attribution", make: func() error {
			return DefaultAttributionConfig().Validate()
		}},
		{name: "continual nil comparison", make: func() error {
			p := comparisonFiniteContinualProtocol(0.95)
			p.Comparison = nil
			return p.Validate()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.make(); err != nil {
				t.Errorf("%s: Validate() = %v, want nil", tc.name, err)
			}
		})
	}
}
