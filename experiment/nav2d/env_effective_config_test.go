package nav2d

import (
	"reflect"
	"testing"
)

func TestEnvironmentEffectiveConfig(t *testing.T) {
	want := Config{Width: 9, Height: 9, WallDensity: .2, ViewDepth: 3, TimeLimit: 60, StepPenalty: .01, CollisionPenalty: .05, GoalReward: 1, Task: TaskAvoidObstacles}
	cases := []struct {
		name        string
		input, want Config
	}{
		{"defaults", Config{}, want},
		{"partial", Config{Width: 7, TimeLimit: 8, Task: TaskRememberGoal}, Config{Width: 7, Height: 9, WallDensity: .2, ViewDepth: 3, TimeLimit: 8, StepPenalty: .01, CollisionPenalty: .05, GoalReward: 1, Task: TaskRememberGoal}},
		{"explicit", Config{Width: 7, Height: 11, WallDensity: .3, ViewDepth: 2, TimeLimit: 8, StepPenalty: .02, CollisionPenalty: .08, GoalReward: 2, Task: TaskRememberGoal}, Config{Width: 7, Height: 11, WallDensity: .3, ViewDepth: 2, TimeLimit: 8, StepPenalty: .02, CollisionPenalty: .08, GoalReward: 2, Task: TaskRememberGoal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := tc.input
			got, err := tc.input.Resolve()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("resolved = %+v, want %+v", got, tc.want)
			}
			if tc.input != original {
				t.Error("receiver changed")
			}
			again, err := got.Resolve()
			if err != nil || again != got {
				t.Errorf("non-idempotent resolve: %+v, %v", again, err)
			}
			implicit, err := New(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			explicit, err := New(got)
			if err != nil {
				t.Fatal(err)
			}
			for _, seed := range []uint64{1, 7, 23} {
				a, ai := implicit.Reset(seed)
				b, bi := explicit.Reset(seed)
				if !reflect.DeepEqual(a, b) || ai != bi {
					t.Error("resolved environment reset differs")
				}
				for step := 0; step < tc.want.TimeLimit; step++ {
					a, ar, ad, ai, ae := implicit.Step(ActionStay)
					b, br, bd, bi, be := explicit.Step(ActionStay)
					if ae != nil || be != nil {
						t.Fatalf("step error: %v / %v", ae, be)
					}
					if !reflect.DeepEqual(a, b) || ar != br || ad != bd || ai != bi {
						t.Fatal("resolved environment step differs")
					}
					if ar != -tc.want.StepPenalty {
						t.Errorf("step reward = %v, want %v", ar, -tc.want.StepPenalty)
					}
					if ad != (step+1 == tc.want.TimeLimit) {
						t.Errorf("timeout at step %d = %v", step+1, ad)
					}
				}
			}
		})
	}
}
func TestEnvironmentEffectiveConfigRejects(t *testing.T) {
	cases := []Config{{Width: 4}, {Width: 6}, {Height: 4}, {Height: 6}, {WallDensity: -.1}, {WallDensity: .5}, {ViewDepth: -1}, {ViewDepth: 6}, {TimeLimit: -1}, {StepPenalty: -.1}, {CollisionPenalty: -.1}, {GoalReward: -1}, {Task: "unknown"}}
	for _, input := range cases {
		got, err := input.Resolve()
		_, oldErr := New(input)
		if err == nil || got != (Config{}) {
			t.Errorf("%+v: got %+v, err %v", input, got, err)
		}
		if oldErr == nil || err == nil || err.Error() != oldErr.Error() {
			t.Errorf("error changed for %+v: %v / %v", input, err, oldErr)
		}
	}
}
