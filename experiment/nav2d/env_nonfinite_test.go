package nav2d

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentNonFiniteConfig(t *testing.T) {
	fields := []struct {
		name string
		set  func(*Config, float64)
	}{
		{"wall density", func(c *Config, v float64) { c.WallDensity = v }},
		{"step penalty", func(c *Config, v float64) { c.StepPenalty = v }},
		{"collision penalty", func(c *Config, v float64) { c.CollisionPenalty = v }},
		{"goal reward", func(c *Config, v float64) { c.GoalReward = v }},
	}
	values := []struct {
		name  string
		value float64
	}{
		{"NaN", math.NaN()}, {"+Inf", math.Inf(1)}, {"-Inf", math.Inf(-1)},
	}
	for _, field := range fields {
		for _, v := range values {
			t.Run(field.name+"/"+v.name, func(t *testing.T) {
				c := Config{}
				field.set(&c, v.value)
				got, err := c.Resolve()
				if err == nil || got != (Config{}) {
					t.Errorf("Resolve = %+v, %v; want zero and error", got, err)
				}
				env, newErr := New(c)
				if newErr == nil || env != nil {
					t.Errorf("New = %v, %v; want nil and error", env, newErr)
				}
				if err != nil && newErr != nil && err.Error() != newErr.Error() {
					t.Errorf("error differs: %v / %v", err, newErr)
				}
				if err != nil && !strings.Contains(err.Error(), field.name) {
					t.Errorf("error does not identify %s: %v", field.name, err)
				}
			})
		}
	}
}

func TestEnvironmentFiniteConfigControls(t *testing.T) {
	cases := []Config{
		{},
		{WallDensity: math.Copysign(0, -1), StepPenalty: math.Copysign(0, -1), CollisionPenalty: math.Copysign(0, -1), GoalReward: math.Copysign(0, -1)},
		{WallDensity: .4, StepPenalty: math.SmallestNonzeroFloat64, CollisionPenalty: math.SmallestNonzeroFloat64, GoalReward: math.SmallestNonzeroFloat64},
		{WallDensity: math.SmallestNonzeroFloat64, StepPenalty: math.MaxFloat64, CollisionPenalty: math.MaxFloat64, GoalReward: math.MaxFloat64},
	}
	for _, c := range cases {
		resolved, err := c.Resolve()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(c); err != nil {
			t.Fatal(err)
		}
		again, err := resolved.Resolve()
		if err != nil || !reflect.DeepEqual(resolved, again) {
			t.Fatalf("non-idempotent: %+v / %+v, %v", resolved, again, err)
		}
		if c.WallDensity == 0 && c.StepPenalty == 0 && c.CollisionPenalty == 0 && c.GoalReward == 0 {
			want := Config{Width: 9, Height: 9, WallDensity: .2, ViewDepth: 3, TimeLimit: 60, StepPenalty: .01, CollisionPenalty: .05, GoalReward: 1, Task: TaskAvoidObstacles}
			if resolved != want {
				t.Fatalf("zero defaults = %+v, want %+v", resolved, want)
			}
		}
		if c.WallDensity != 0 && resolved.WallDensity != c.WallDensity {
			t.Fatal("wall density changed")
		}
		if c.StepPenalty != 0 && resolved.StepPenalty != c.StepPenalty {
			t.Fatal("step penalty changed")
		}
		if c.CollisionPenalty != 0 && resolved.CollisionPenalty != c.CollisionPenalty {
			t.Fatal("collision penalty changed")
		}
		if c.GoalReward != 0 && resolved.GoalReward != c.GoalReward {
			t.Fatal("goal reward changed")
		}
	}
	negatives := []struct {
		c       Config
		message string
	}{
		{Config{WallDensity: -.1}, "nav2d: wall density -0.1 outside [0, 0.4]"},
		{Config{WallDensity: .5}, "nav2d: wall density 0.5 outside [0, 0.4]"},
		{Config{StepPenalty: -.1}, "nav2d: step penalty -0.1 < 0"},
		{Config{CollisionPenalty: -.1}, "nav2d: collision penalty -0.1 < 0"},
		{Config{GoalReward: -1}, "nav2d: goal reward -1 <= 0"},
		{Config{Width: 4, WallDensity: math.NaN()}, "nav2d: width 4 must be odd and >= 5"},
		{Config{ViewDepth: 6, StepPenalty: math.Inf(1)}, "nav2d: view depth 6 outside [1, 5]"},
	}
	for _, tc := range negatives {
		_, err := tc.c.Resolve()
		if err == nil || err.Error() != tc.message {
			t.Errorf("error = %v; want %q", err, tc.message)
		}
	}
}
