package dynamics_test

import (
	"context"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
)

func TestUninitializedContinuousForwardReturnsError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model *dynamics.Continuous
	}{
		{"nil", nil},
		{"zero", new(dynamics.Continuous)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace, err := tc.model.Forward(context.Background(), dynamics.Parameters{}, nil, [][]float64{{}})
			if err == nil || trace != nil {
				t.Fatalf("uninitialized model returned trace %v, error %v", trace, err)
			}
		})
	}
}
