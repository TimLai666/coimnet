package experiment

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/experiment/nav2d"
)

type reportNonFiniteCase struct {
	name  string
	field string
	value float64
	set   func(*nav2d.Config, float64)
}

func reportNonFiniteCases() []reportNonFiniteCase {
	values := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "+Inf", value: math.Inf(1)},
		{name: "-Inf", value: math.Inf(-1)},
	}
	fields := []struct {
		name  string
		field string
		set   func(*nav2d.Config, float64)
	}{
		{name: "wall_density", field: "wall density", set: func(c *nav2d.Config, v float64) { c.WallDensity = v }},
		{name: "step_penalty", field: "step penalty", set: func(c *nav2d.Config, v float64) { c.StepPenalty = v }},
		{name: "collision_penalty", field: "collision penalty", set: func(c *nav2d.Config, v float64) { c.CollisionPenalty = v }},
		{name: "goal_reward", field: "goal reward", set: func(c *nav2d.Config, v float64) { c.GoalReward = v }},
	}
	cases := make([]reportNonFiniteCase, 0, len(fields)*len(values))
	for _, field := range fields {
		for _, value := range values {
			field, value := field, value
			cases = append(cases, reportNonFiniteCase{
				name:  field.name + "/" + value.name,
				field: field.field,
				value: value.value,
				set:   field.set,
			})
		}
	}
	return cases
}

func reportNonFiniteNav2DConfig() Nav2DConfig {
	return Nav2DConfig{
		Env:          nav2d.Config{Task: nav2d.TaskAvoidObstacles},
		Seeds:        []uint64{1},
		Episodes:     1,
		Hidden:       4,
		Recurrent:    1,
		LearningRate: 0.05,
		EvalEpisodes: 1,
		Policies:     []string{Nav2DRecurrent},
	}
}

func reportNonFiniteAttributionConfig() AttributionConfig {
	return AttributionConfig{
		Env:          nav2d.Config{Task: nav2d.TaskAvoidObstacles},
		Seeds:        []uint64{1, 2, 3},
		Episodes:     1,
		Hidden:       4,
		Recurrent:    1,
		LearningRate: 0.05,
		EvalEpisodes: 1,
		Groups:       []string{AttributionNormal},
		Comparison: PreRegistered{
			Method:    ComparisonPairedBootstrap,
			Interval:  0.95,
			Baseline:  AttributionNormal,
			Resamples: 100,
		},
	}
}

// TestReportNonFiniteEnvironment requires every public validation and report
// entry point to reject each non-finite environment field before execution.
func TestReportNonFiniteEnvironment(t *testing.T) {
	for _, tc := range reportNonFiniteCases() {
		t.Run(tc.name, func(t *testing.T) {
			navConfig := reportNonFiniteNav2DConfig()
			tc.set(&navConfig.Env, tc.value)
			attributionConfig := reportNonFiniteAttributionConfig()
			tc.set(&attributionConfig.Env, tc.value)

			navValidateErr := navConfig.Validate()
			if navValidateErr == nil {
				t.Errorf("Nav2DConfig.Validate accepted %s=%v", tc.field, tc.value)
			} else if !strings.Contains(navValidateErr.Error(), tc.field) {
				t.Errorf("Nav2DConfig.Validate error %q does not identify %s", navValidateErr, tc.field)
			}
			attributionValidateErr := attributionConfig.Validate()
			if attributionValidateErr == nil {
				t.Errorf("AttributionConfig.Validate accepted %s=%v", tc.field, tc.value)
			} else if !strings.Contains(attributionValidateErr.Error(), tc.field) {
				t.Errorf("AttributionConfig.Validate error %q does not identify %s", attributionValidateErr, tc.field)
			}

			navReport, navErr := RunNav2D(context.Background(), navConfig)
			if navErr == nil {
				t.Error("RunNav2D accepted a non-finite environment value")
			}
			if !reflect.DeepEqual(navReport, Nav2DReport{}) {
				t.Errorf("RunNav2D report = %+v, want zero report", navReport)
			}
			if len(navReport.Runs) != 0 {
				t.Errorf("RunNav2D returned %d runs after validation error, want 0", len(navReport.Runs))
			}

			suiteReport, suiteErr := RunNav2DSuite(context.Background(), navConfig)
			if suiteErr == nil {
				t.Error("RunNav2DSuite accepted a non-finite environment value")
			}
			if suiteReport.SchemaVersion != Nav2DSuiteSchemaVersion {
				t.Errorf("RunNav2DSuite SchemaVersion = %q, want %q", suiteReport.SchemaVersion, Nav2DSuiteSchemaVersion)
			}
			if len(suiteReport.Tasks) != 0 {
				t.Errorf("RunNav2DSuite returned %d tasks after validation error, want 0", len(suiteReport.Tasks))
			}

			attributionReport, attributionErr := RunAttribution(context.Background(), attributionConfig)
			if attributionErr == nil {
				t.Error("RunAttribution accepted a non-finite environment value")
			}
			if !reflect.DeepEqual(attributionReport, AttributionReport{}) {
				t.Errorf("RunAttribution report = %+v, want zero report", attributionReport)
			}
			if len(attributionReport.Runs) != 0 {
				t.Errorf("RunAttribution returned %d runs after validation error, want 0", len(attributionReport.Runs))
			}
		})
	}
}

// TestReportNonFiniteValidControl confirms the small protocols used above are
// valid and still produce reports when all environment values are finite.
func TestReportNonFiniteValidControl(t *testing.T) {
	navConfig := reportNonFiniteNav2DConfig()
	if err := navConfig.Validate(); err != nil {
		t.Fatalf("Nav2DConfig.Validate(valid control): %v", err)
	}
	if report, err := RunNav2D(context.Background(), navConfig); err != nil || len(report.Runs) == 0 {
		t.Fatalf("RunNav2D(valid control): report runs=%d, err=%v", len(report.Runs), err)
	}
	if report, err := RunNav2DSuite(context.Background(), navConfig); err != nil || len(report.Tasks) == 0 {
		t.Fatalf("RunNav2DSuite(valid control): tasks=%d, err=%v", len(report.Tasks), err)
	}

	attributionConfig := reportNonFiniteAttributionConfig()
	if err := attributionConfig.Validate(); err != nil {
		t.Fatalf("AttributionConfig.Validate(valid control): %v", err)
	}
	if report, err := RunAttribution(context.Background(), attributionConfig); err != nil || len(report.Runs) == 0 {
		t.Fatalf("RunAttribution(valid control): report runs=%d, err=%v", len(report.Runs), err)
	}
}
