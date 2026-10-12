package webgpu

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Reduced planning limits force separate physical buffers on the real device.
// This fixture verifies cross-chunk transactions without a large allocation;
// it does not claim to exercise the adapter's native capacity limit.
func newChunkedHardwareAdam(t *testing.T, ctx context.Context, values int) *DeviceAdamW {
	t.Helper()
	cfg := AdamWConfig{Beta1: 0.5, Beta2: 0.25, Epsilon: 0.1, MaxBytes: 1 << 20}
	owner := newTestDeviceAdamW(t, ctx, []float32{0}, cfg)
	limits := owner.limits
	limits.MaxStorageBufferBindingSize = 64 * deviceAdamStateBytes
	limits.MaxBufferSize = 64 * deviceAdamStateBytes
	o := &DeviceAdamW{device: owner.device, info: owner.info, limits: limits, values: values, cfg: cfg,
		metrics: AdamWMetrics{Device: owner.info, Values: values}}
	t.Cleanup(func() {
		if err := owner.device.WaitIdle(); err != nil {
			t.Errorf("chunk fixture WaitIdle: %v", err)
		}
		// The small owner releases the shared device after these buffers.
		o.device = nil
		o.releaseAdamResources()
	})
	for _, create := range []func() error{o.planAdamChunks, o.compileAdamPipeline, o.createAdamBuffers, o.createAdamBindGroups} {
		if err := create(); err != nil {
			t.Fatal(err)
		}
	}
	parameters := make([]float32, values)
	for i := range parameters {
		parameters[i] = float32(i%7) / 5
	}
	if err := o.uploadInitialAdamState(ctx, parameters, make([]float32, values), make([]float32, values), make([]uint32, values)); err != nil {
		t.Fatal(err)
	}
	if _, err := o.executeAdamStepLocked(ctx, make([]float32, values), make([]bool, values), 1); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestDeviceAdamWHardwareChunkFailureRollsBackEveryBank(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	const values = 130
	o := newChunkedHardwareAdam(t, ctx, values)
	if len(o.chunks) != 3 {
		t.Fatalf("physical chunks = %d, want 3", len(o.chunks))
	}
	before := mustDeviceAdamSnapshot(t, o, ctx)
	gradient, mask := adamGradient(values, 0, 13), adamMask(values, 0)
	// A valid finite input overflows inside the last GPU chunk, after earlier
	// chunks have computed ordinary candidate updates.
	gradient[values-1], mask[values-1] = math.MaxFloat32, true
	if _, err := o.Step(ctx, gradient, mask, 0.1); err == nil {
		t.Fatal("last-chunk non-finite candidate accepted")
	}
	assertAdamSnapshotsEqual(t, mustDeviceAdamSnapshot(t, o, ctx), before)
	gradient[values-1] = 0.25
	reference := newAdamReference(before.Parameters)
	applyAdamReference(&reference, o.cfg, gradient, mask, 0.1)
	if _, err := o.Step(ctx, gradient, mask, 0.1); err != nil {
		t.Fatal(err)
	}
	assertAdamReferenceSnapshot(t, 0, mustDeviceAdamSnapshot(t, o, ctx), reference)
	t.Logf("real-device chunks=%d allocated_logical_bytes=%d adapter=%s backend=%s", len(o.chunks), o.allocated, o.info.Adapter, o.info.Backend)
}

const adamChildSnapshotEnv = "COIMNET_TEST_ADAM_SNAPSHOT"
const adamChildResultEnv = "COIMNET_TEST_ADAM_RESULT"

func TestDeviceAdamWFreshProcessContinuation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	o := newTestDeviceAdamW(t, ctx, []float32{1, -1, 0.5}, AdamWConfig{
		Beta1: 0.8, Beta2: 0.9, Epsilon: 0.01, WeightDecay: 0.02, MaxBytes: 1 << 20,
	})
	if _, err := o.Step(ctx, []float32{0.2, -0.1, 0.3}, []bool{true, false, true}, 0.03); err != nil {
		t.Fatal(err)
	}
	saved := mustDeviceAdamSnapshot(t, o, ctx)
	dir := t.TempDir()
	inputPath, outputPath := filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "continued.json")
	writeAdamTestSnapshot(t, inputPath, saved)
	if _, err := o.Step(ctx, []float32{-0.1, 0.4, -0.2}, []bool{true, true, false}, 0.07); err != nil {
		t.Fatal(err)
	}
	want := mustDeviceAdamSnapshot(t, o, ctx)
	binaryPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binaryPath, "-test.run=^TestDeviceAdamWChildProcess$", "-test.v", "-test.timeout=45s")
	command.Env = append(os.Environ(), adamChildSnapshotEnv+"="+inputPath, adamChildResultEnv+"="+outputPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("independent restore process: %v\n%s", err, output)
	}
	t.Logf("independent process: %s", output)
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var got AdamWSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	assertAdamSnapshotsEqual(t, got, want)
	t.Logf("constructor_and_step_metrics=%+v", o.Metrics())
}

func TestDeviceAdamWChildProcess(t *testing.T) {
	inputPath, outputPath := os.Getenv(adamChildSnapshotEnv), os.Getenv(adamChildResultEnv)
	if inputPath == "" && outputPath == "" {
		t.Skip("independent process helper")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved AdamWSnapshot
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	o, err := RestoreDeviceAdamW(ctx, saved)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := o.Close(); err != nil {
			t.Errorf("child Close: %v", err)
		}
	}()
	if _, err := o.Step(ctx, []float32{-0.1, 0.4, -0.2}, []bool{true, true, false}, 0.07); err != nil {
		t.Fatal(err)
	}
	writeAdamTestSnapshot(t, outputPath, mustDeviceAdamSnapshot(t, o, ctx))
	t.Logf("restored_hardware_metrics=%+v", o.Metrics())
}

func writeAdamTestSnapshot(t *testing.T, path string, snapshot AdamWSnapshot) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	encodeErr := json.NewEncoder(f).Encode(snapshot)
	closeErr := f.Close()
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
