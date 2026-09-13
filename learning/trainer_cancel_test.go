package learning

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/TimLai666/coimnet/dynamics"
)

const trainerCancellationTimeout = 500 * time.Millisecond

type errSignalContext struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *errSignalContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func newCancellationTrainer(t *testing.T) *Trainer {
	t.Helper()
	config := Config{
		Dynamics:     dynamics.Config{Nodes: 1, DT: 1, Activation: "tanh"},
		InputSize:    1,
		OutputSize:   1,
		ReadoutNodes: []int{0},
	}
	parameters := Parameters{
		Core:    dynamics.Parameters{Bias: []float64{0}, LogTau: []float64{0}},
		Encoder: []float64{1},
		Readout: []float64{1},
	}
	trainer, err := NewTrainer(config, parameters, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	return trainer
}

func lockCancellationTrainer(t *testing.T) (*Trainer, func()) {
	t.Helper()
	trainer := newCancellationTrainer(t)
	trainer.mu.Lock()
	var once sync.Once
	return trainer, func() { once.Do(trainer.mu.Unlock) }
}

func TestTrainerPredictCanceledBeforeLock(t *testing.T) {
	trainer, unlock := lockCancellationTrainer(t)
	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := trainer.Predict(ctx, [][]float64{{1}})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Predict error = %v, want context.Canceled", err)
		}
	case <-time.After(trainerCancellationTimeout):
		t.Fatal("Predict waited on the trainer lock despite an already-canceled context")
	}
}

func TestTrainerStepCanceledWhileWaitingForLock(t *testing.T) {
	trainer, unlock := lockCancellationTrainer(t)
	defer unlock()

	base, cancel := context.WithCancel(context.Background())
	ctx := &errSignalContext{Context: base, checked: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := trainer.Step(ctx, [][]float64{{1}}, []float64{0})
		done <- err
	}()
	select {
	case <-ctx.checked:
		cancel()
	case <-time.After(trainerCancellationTimeout):
		t.Fatal("waiting Step did not check its context")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Step error = %v, want context.Canceled", err)
		}
	case <-time.After(trainerCancellationTimeout):
		t.Fatal("Step remained blocked after its context was canceled")
	}

	unlock()
	if got := trainer.Snapshot().Updates; got != 0 {
		t.Fatalf("Updates = %d, want canceled waiting Step to leave state unchanged", got)
	}
}

func TestTrainerNilAndZeroValuesReturnWithoutBlocking(t *testing.T) {
	var nilTrainer *Trainer
	if got := nilTrainer.Snapshot(); !reflect.DeepEqual(got, TrainingSnapshot{}) {
		t.Fatalf("nil Snapshot = %#v, want zero snapshot", got)
	}
	if _, err := nilTrainer.Predict(context.Background(), nil); err == nil {
		t.Fatal("nil Predict succeeded")
	}
	if _, err := nilTrainer.Step(context.Background(), nil, nil); err == nil {
		t.Fatal("nil Step succeeded")
	}

	var zero Trainer
	if got := zero.Snapshot(); !reflect.DeepEqual(got, TrainingSnapshot{}) {
		t.Fatalf("zero Snapshot = %#v, want zero snapshot", got)
	}
	if _, err := zero.Predict(context.Background(), nil); err == nil {
		t.Fatal("zero Predict succeeded")
	}
	if _, err := zero.Step(context.Background(), nil, nil); err == nil {
		t.Fatal("zero Step succeeded")
	}
}
