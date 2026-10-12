package learning

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/backend/webgpu"
)

func TestNewGPUDeviceTrainerRejectsRawNegativeOptions(t *testing.T) {
	c, p, base := gpuTrainerFixture()
	cases := []struct {
		name  string
		field string
		set   func(*Options)
	}{
		{name: "beta1", field: "beta1", set: func(o *Options) {
			o.Beta1 = -math.SmallestNonzeroFloat64
		}},
		{name: "beta2", field: "beta2", set: func(o *Options) {
			o.Beta2 = -math.SmallestNonzeroFloat64
		}},
		{name: "weight decay", field: "weight_decay", set: func(o *Options) {
			o.WeightDecay = -math.SmallestNonzeroFloat64
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := base
			tc.set(&o)

			cpu, cpuErr := NewTrainer(c, p, o)
			if cpu != nil {
				_ = cpu.Close()
				t.Fatalf("CPU NewTrainer returned a trainer for raw negative %s", tc.name)
			}
			if cpuErr == nil {
				t.Fatalf("CPU NewTrainer accepted raw negative %s", tc.name)
			}

			device, err := NewGPUDeviceTrainer(context.Background(), c, p, o, 1<<20)
			if device != nil {
				_ = device.Close()
				t.Fatalf("NewGPUDeviceTrainer returned a trainer for raw negative %s", tc.name)
			}
			if errors.Is(err, webgpu.ErrUnavailable) {
				t.Fatalf("raw negative %s reached hardware setup: %v", tc.name, err)
			}
			if !errors.Is(err, webgpu.ErrInvalidConfig) {
				t.Fatalf("raw negative %s error = %v, want webgpu.ErrInvalidConfig", tc.name, err)
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.field) {
				t.Fatalf("raw negative %s error = %q, want rejected field %q", tc.name, err, tc.field)
			}
		})
	}
}
