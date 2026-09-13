package coimnet_test

import (
	"reflect"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

func TestZeroValueGettersAreSafe(t *testing.T) {
	t.Run("nil network config", func(t *testing.T) {
		var n *learning.Network
		if got := n.Config(); !reflect.DeepEqual(got, learning.Config{}) {
			t.Fatalf("got %#v, want zero config", got)
		}
	})
	t.Run("zero network config", func(t *testing.T) {
		var n learning.Network
		if got := n.Config(); !reflect.DeepEqual(got, learning.Config{}) {
			t.Fatalf("got %#v, want zero config", got)
		}
	})
	t.Run("nil trainer snapshot", func(t *testing.T) {
		var tr *learning.Trainer
		if got := tr.Snapshot(); !reflect.DeepEqual(got, learning.TrainingSnapshot{}) {
			t.Fatalf("got %#v, want zero snapshot", got)
		}
	})
	t.Run("zero trainer snapshot", func(t *testing.T) {
		var tr learning.Trainer
		if got := tr.Snapshot(); !reflect.DeepEqual(got, learning.TrainingSnapshot{}) {
			t.Fatalf("got %#v, want zero snapshot", got)
		}
	})
	t.Run("nil continuous config", func(t *testing.T) {
		var m *dynamics.Continuous
		if got := m.Config(); !reflect.DeepEqual(got, dynamics.Config{}) {
			t.Fatalf("got %#v, want zero config", got)
		}
	})
	t.Run("zero continuous config", func(t *testing.T) {
		var m dynamics.Continuous
		if got := m.Config(); !reflect.DeepEqual(got, dynamics.Config{}) {
			t.Fatalf("got %#v, want zero config", got)
		}
	})
	t.Run("nil trace outputs", func(t *testing.T) {
		var tr *dynamics.Trace
		if got := tr.Outputs(); got != nil {
			t.Fatalf("got %#v, want nil outputs", got)
		}
	})
	t.Run("zero trace outputs", func(t *testing.T) {
		var tr dynamics.Trace
		if got := tr.Outputs(); got != nil {
			t.Fatalf("got %#v, want nil outputs", got)
		}
	})
	t.Run("nil trace final voltage", func(t *testing.T) {
		var tr *dynamics.Trace
		if got := tr.FinalVoltage(); got != nil {
			t.Fatalf("got %#v, want nil final voltage", got)
		}
	})
	t.Run("zero trace final voltage", func(t *testing.T) {
		var tr dynamics.Trace
		if got := tr.FinalVoltage(); got != nil {
			t.Fatalf("got %#v, want nil final voltage", got)
		}
	})
}
