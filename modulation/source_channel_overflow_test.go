package modulation

import (
	"reflect"
	"testing"
)

func portableMaxInt() int { return int(^uint(0) >> 1) }

func assertSourceReleaseRejectsWithoutPanic(t *testing.T, name string, release func() ([]float64, error)) {
	t.Helper()
	var (
		rates    []float64
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		rates, err = release()
	}()
	if panicked != nil {
		t.Fatalf("%s panicked for an unrepresentable channel count: %v", name, panicked)
	}
	if err == nil {
		t.Fatalf("%s accepted MaxInt channel", name)
	}
	if rates != nil {
		t.Fatalf("%s returned rates together with an error: %v", name, rates)
	}
}

func assertSourceBuildRejectsWithoutPanic(t *testing.T, name string, spec SourceSpec) {
	t.Helper()
	var (
		built    Source
		err      error
		panicked any
	)
	func() {
		defer func() { panicked = recover() }()
		built, err = spec.Build()
	}()
	if panicked != nil {
		t.Fatalf("%s panicked for an unrepresentable channel count: %v", name, panicked)
	}
	if err == nil {
		t.Fatalf("%s accepted MaxInt channel", name)
	}
	if built != nil {
		t.Fatalf("%s returned a source together with an error: %T", name, built)
	}
}

func assertSourceRates(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestSourcesRejectMaxIntChannelWithoutPanicOrNegativeWidth(t *testing.T) {
	maxInt := portableMaxInt()
	t.Run("NeuralActivity.Release", func(t *testing.T) {
		neural := NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: maxInt}
		assertSourceReleaseRejectsWithoutPanic(t, "NeuralActivity.Release", func() ([]float64, error) {
			return neural.Release(0, SourceContext{Activity: []float64{0}})
		})
	})
	t.Run("InternalResource.Release", func(t *testing.T) {
		resource := InternalResource{Resource: "energy", Coefficient: 1, Channel: maxInt}
		assertSourceReleaseRejectsWithoutPanic(t, "InternalResource.Release", func() ([]float64, error) {
			return resource.Release(0, SourceContext{Resources: map[string]float64{"energy": 0}})
		})
	})
	t.Run("SourceSpec.Build neural", func(t *testing.T) {
		neural := NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: maxInt}
		assertSourceBuildRejectsWithoutPanic(t, "SourceSpec.Build neural", SourceSpec{
			Kind:    SourceNeuralActivity,
			Channel: maxInt,
			Neural:  &neural,
		})
	})
	t.Run("SourceSpec.Build resource", func(t *testing.T) {
		resource := InternalResource{Resource: "energy", Coefficient: 1, Channel: maxInt}
		assertSourceBuildRejectsWithoutPanic(t, "SourceSpec.Build resource", SourceSpec{
			Kind:     SourceInternalResource,
			Channel:  maxInt,
			Resource: &resource,
		})
	})
}

func TestSourcesKeepOrdinaryChannelsAndZeroRelease(t *testing.T) {
	for _, channel := range []int{0, 2} {
		neural := NeuralActivity{Nodes: []int{0}, Gain: 1, Channel: channel}
		if got, want := neural.Channels(), channel+1; got != want {
			t.Fatalf("NeuralActivity channel %d has width %d, want %d", channel, got, want)
		}
		got, err := neural.Release(0, SourceContext{Activity: []float64{0}})
		if err != nil {
			t.Fatalf("NeuralActivity channel %d Release() error = %v", channel, err)
		}
		assertSourceRates(t, "NeuralActivity zero release", got, make([]float64, channel+1))
		got, err = neural.Release(0, SourceContext{Activity: []float64{2}})
		if err != nil {
			t.Fatalf("NeuralActivity channel %d non-zero Release() error = %v", channel, err)
		}
		want := make([]float64, channel+1)
		want[channel] = 2
		assertSourceRates(t, "NeuralActivity routed release", got, want)

		resource := InternalResource{Resource: "energy", Coefficient: 1, Threshold: 1, Channel: channel}
		if got, want := resource.Channels(), channel+1; got != want {
			t.Fatalf("InternalResource channel %d has width %d, want %d", channel, got, want)
		}
		got, err = resource.Release(0, SourceContext{Resources: map[string]float64{"energy": 1}})
		if err != nil {
			t.Fatalf("InternalResource channel %d Release() error = %v", channel, err)
		}
		assertSourceRates(t, "InternalResource zero release", got, make([]float64, channel+1))
		got, err = resource.Release(0, SourceContext{Resources: map[string]float64{"energy": 3}})
		if err != nil {
			t.Fatalf("InternalResource channel %d non-zero Release() error = %v", channel, err)
		}
		want = make([]float64, channel+1)
		want[channel] = 2
		assertSourceRates(t, "InternalResource routed release", got, want)

		neuralBuilt, err := (SourceSpec{
			Kind:    SourceNeuralActivity,
			Channel: channel,
			Neural:  &neural,
		}).Build()
		if err != nil {
			t.Fatalf("SourceSpec.Build neural channel %d error = %v", channel, err)
		}
		if neuralBuilt == nil {
			t.Fatalf("SourceSpec.Build neural channel %d returned nil source", channel)
		}
		if neuralBuilt.Channels() != channel+1 {
			t.Fatalf("SourceSpec.Build neural channel %d returned width %d, want %d", channel, neuralBuilt.Channels(), channel+1)
		}
		got, err = neuralBuilt.Release(0, SourceContext{Activity: []float64{2}})
		if err != nil {
			t.Fatalf("built NeuralActivity channel %d Release() error = %v", channel, err)
		}
		assertSourceRates(t, "built NeuralActivity routed release", got, want)

		resourceBuilt, err := (SourceSpec{
			Kind:     SourceInternalResource,
			Channel:  channel,
			Resource: &resource,
		}).Build()
		if err != nil {
			t.Fatalf("SourceSpec.Build resource channel %d error = %v", channel, err)
		}
		if resourceBuilt == nil {
			t.Fatalf("SourceSpec.Build resource channel %d returned nil source", channel)
		}
		if resourceBuilt.Channels() != channel+1 {
			t.Fatalf("SourceSpec.Build resource channel %d returned width %d, want %d", channel, resourceBuilt.Channels(), channel+1)
		}
		got, err = resourceBuilt.Release(0, SourceContext{Resources: map[string]float64{"energy": 3}})
		if err != nil {
			t.Fatalf("built InternalResource channel %d Release() error = %v", channel, err)
		}
		assertSourceRates(t, "built InternalResource routed release", got, want)
	}
}
