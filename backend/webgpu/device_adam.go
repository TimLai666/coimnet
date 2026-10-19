package webgpu

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

const deviceAdamSchema = "coimnet-device-adamw/v1"

// AdamWConfig fixes the float32 device optimizer and its complete buffer budget.
type AdamWConfig struct {
	Beta1       float32 `json:"beta1"`
	Beta2       float32 `json:"beta2"`
	Epsilon     float32 `json:"epsilon"`
	WeightDecay float32 `json:"weight_decay"`
	// MaxBytes is the hard budget for the logical Size sum of every buffer
	// created by this optimizer, excluding backend alignment and driver memory.
	MaxBytes uint64 `json:"max_bytes"`
}

// AdamWSnapshot stores all parameters, moments and individual update counters.
type AdamWSnapshot struct {
	SchemaVersion string      `json:"schema_version"`
	Config        AdamWConfig `json:"config"`
	Parameters    []float32   `json:"parameters"`
	First         []float32   `json:"first"`
	Second        []float32   `json:"second"`
	Steps         []uint32    `json:"steps"`
}

// AdamWMetrics reports host timings and the actual allocated/transferred bytes.
type AdamWMetrics struct {
	Device DeviceInfo

	Initialization, Compilation, InitialTransfer, Warmup time.Duration
	Transfer, Dispatch, Readback                         time.Duration
	// AllocatedBytes is the logical Size sum of every optimizer-created buffer,
	// excluding backend alignment and driver physical allocation overhead.
	AllocatedBytes, BytesUploaded, BytesReadBack uint64
	Values, Chunks                               int
}

const (
	deviceAdamWorkgroupSize = uint64(64)
	deviceAdamUniformBytes  = uint64(32)
	deviceAdamStateBytes    = uint64(16)
	deviceAdamInputBytes    = uint64(8)
	deviceAdamStatusBytes   = uint64(4)
	deviceAdamStagingBytes  = uint64(4)
)

const (
	deviceAdamStatusNonFinite       = uint32(1)
	deviceAdamStatusCounterOverflow = uint32(2)
	deviceAdamStatusInvalidState    = uint32(4)
)

// DeviceAdamW owns a float32 hardware optimizer with transactional updates.
// Parameters, moments, and counters remain resident in two device banks. A
// successful step swaps the banks only after every chunk has reported success.
type DeviceAdamW struct {
	mu     sync.Mutex
	closed bool
	// unusable is set when a submitted GPU operation cannot be synchronously
	// drained. The device resources remain releasable through Close, but no
	// further Step or Snapshot may reuse them.
	unusable error

	adapter *wgpu.Adapter
	device  *wgpu.Device
	info    DeviceInfo
	limits  gputypes.Limits

	values          int
	chunks          []deviceAdamChunk
	maxChunk        int
	currentBank     int
	allocated       uint64
	snapshotSize    uint64
	snapshotStaging *wgpu.Buffer

	pipeline *wgpu.ComputePipeline
	// gogpu/wgpu's Vulkan pipelines keep only the raw layout handle, so a
	// pipeline layout must live until its pipeline is released.
	pipelineLayout  *wgpu.PipelineLayout
	bindGroupLayout *wgpu.BindGroupLayout

	cfg     AdamWConfig
	metrics AdamWMetrics
}

// NewDeviceAdamW validates the initial state, opens a hardware adapter, and
// creates the resident float32 AdamW buffers. It never uses a CPU update path.
func NewDeviceAdamW(ctx context.Context, parameters []float32, cfg AdamWConfig) (*DeviceAdamW, error) {
	if err := validateAdamContext(ctx); err != nil {
		return nil, err
	}
	if err := validateAdamConfig(cfg); err != nil {
		return nil, err
	}
	if err := validateAdamValues(parameters, "parameters"); err != nil {
		return nil, err
	}
	if err := preflightAdamBudget(len(parameters), cfg.MaxBytes); err != nil {
		return nil, err
	}

	first := make([]float32, len(parameters))
	second := make([]float32, len(parameters))
	steps := make([]uint32, len(parameters))
	return newDeviceAdamW(ctx, append([]float32(nil), parameters...), first, second, steps, cfg)
}

// RestoreDeviceAdamW validates and uploads a complete owned snapshot to a new
// hardware optimizer. Caller-owned slices are never retained.
func RestoreDeviceAdamW(ctx context.Context, snapshot AdamWSnapshot) (*DeviceAdamW, error) {
	if err := validateAdamContext(ctx); err != nil {
		return nil, err
	}
	if err := validateAdamSnapshot(snapshot); err != nil {
		return nil, err
	}
	if err := preflightAdamBudget(len(snapshot.Parameters), snapshot.Config.MaxBytes); err != nil {
		return nil, err
	}

	return newDeviceAdamW(
		ctx,
		append([]float32(nil), snapshot.Parameters...),
		append([]float32(nil), snapshot.First...),
		append([]float32(nil), snapshot.Second...),
		append([]uint32(nil), snapshot.Steps...),
		snapshot.Config,
	)
}

// Step uploads only gradients, masks, settings, and reset status flags. The
// optimizer state is updated on the selected hardware adapter.
func (o *DeviceAdamW) Step(ctx context.Context, gradient []float32, mask []bool, learningRate float32) (AdamWMetrics, error) {
	if o == nil {
		return AdamWMetrics{}, fmt.Errorf("%w: nil receiver", ErrClosed)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return AdamWMetrics{}, ErrClosed
	}
	if o.unusable != nil {
		return o.metrics, o.unusable
	}
	if err := validateAdamContext(ctx); err != nil {
		return o.metrics, err
	}
	if len(gradient) != o.values || len(mask) != o.values {
		return o.metrics, fmt.Errorf("%w: gradient length %d and mask length %d, want %d", ErrShape, len(gradient), len(mask), o.values)
	}
	if err := validateAdamValues(gradient, "gradient"); err != nil {
		return o.metrics, err
	}
	if !isFinite32(learningRate) {
		return o.metrics, fmt.Errorf("%w: learning rate is %v", ErrNonFinite, learningRate)
	}
	if learningRate <= 0 {
		return o.metrics, fmt.Errorf("%w: learning rate must be positive, got %v", ErrInvalidConfig, learningRate)
	}
	return o.executeAdamStepLocked(ctx, gradient, mask, learningRate)
}

// Snapshot reads all resident state from the hardware into owned host slices.
func (o *DeviceAdamW) Snapshot(ctx context.Context) (AdamWSnapshot, error) {
	if o == nil {
		return AdamWSnapshot{}, fmt.Errorf("%w: nil receiver", ErrClosed)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return AdamWSnapshot{}, ErrClosed
	}
	if o.unusable != nil {
		return AdamWSnapshot{}, o.unusable
	}
	if err := validateAdamContext(ctx); err != nil {
		return AdamWSnapshot{}, err
	}
	return o.snapshotAdamLocked(ctx)
}

// Metrics returns the latest constructor and operation measurements.
func (o *DeviceAdamW) Metrics() AdamWMetrics {
	if o == nil {
		return AdamWMetrics{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.metrics
}

// Close waits for queued device work, releases all resources, and is
// idempotent after the first call.
func (o *DeviceAdamW) Close() error {
	if o == nil {
		return fmt.Errorf("%w: nil receiver", ErrClosed)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	var waitErr error
	if o.device != nil {
		waitErr = o.device.WaitIdle()
	}
	o.releaseAdamResources()
	if waitErr != nil {
		return fmt.Errorf("%w: wait for device idle during close: %v", ErrExecution, waitErr)
	}
	return nil
}

func validateAdamContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalidConfig)
	}
	return ctx.Err()
}

func validateAdamConfig(cfg AdamWConfig) error {
	if !isFinite32(cfg.Beta1) || cfg.Beta1 < 0 || cfg.Beta1 >= 1 {
		return fmt.Errorf("%w: beta1 must be finite and in [0,1), got %v", ErrInvalidConfig, cfg.Beta1)
	}
	if !isFinite32(cfg.Beta2) || cfg.Beta2 < 0 || cfg.Beta2 >= 1 {
		return fmt.Errorf("%w: beta2 must be finite and in [0,1), got %v", ErrInvalidConfig, cfg.Beta2)
	}
	if !isFinite32(cfg.Epsilon) || cfg.Epsilon <= 0 {
		return fmt.Errorf("%w: epsilon must be finite and positive, got %v", ErrInvalidConfig, cfg.Epsilon)
	}
	if !isFinite32(cfg.WeightDecay) || cfg.WeightDecay < 0 {
		return fmt.Errorf("%w: weight decay must be finite and non-negative, got %v", ErrInvalidConfig, cfg.WeightDecay)
	}
	if cfg.MaxBytes == 0 {
		return fmt.Errorf("%w: max bytes must be positive", ErrInvalidConfig)
	}
	return nil
}

func validateAdamValues(values []float32, name string) error {
	if len(values) == 0 {
		return fmt.Errorf("%w: %s must be non-empty", ErrShape, name)
	}
	for i, value := range values {
		if !isFinite32(value) {
			return fmt.Errorf("%w: %s[%d] is %v", ErrNonFinite, name, i, value)
		}
	}
	return nil
}

func validateAdamSnapshot(snapshot AdamWSnapshot) error {
	if snapshot.SchemaVersion != deviceAdamSchema {
		return fmt.Errorf("%w: unsupported snapshot schema %q", ErrInvalidConfig, snapshot.SchemaVersion)
	}
	if err := validateAdamConfig(snapshot.Config); err != nil {
		return err
	}
	if err := validateAdamValues(snapshot.Parameters, "snapshot parameters"); err != nil {
		return err
	}
	if len(snapshot.First) != len(snapshot.Parameters) || len(snapshot.Second) != len(snapshot.Parameters) || len(snapshot.Steps) != len(snapshot.Parameters) {
		return fmt.Errorf("%w: snapshot state lengths differ", ErrShape)
	}
	for i := range snapshot.Parameters {
		if !isFinite32(snapshot.First[i]) {
			return fmt.Errorf("%w: snapshot first[%d] is %v", ErrNonFinite, i, snapshot.First[i])
		}
		if !isFinite32(snapshot.Second[i]) || snapshot.Second[i] < 0 {
			return fmt.Errorf("%w: snapshot second[%d] is %v", ErrNonFinite, i, snapshot.Second[i])
		}
		if snapshot.Steps[i] == 0 && (snapshot.First[i] != 0 || snapshot.Second[i] != 0) {
			return fmt.Errorf("%w: snapshot moments[%d] must be zero when step is zero", ErrInvalidConfig, i)
		}
	}
	return nil
}

func preflightAdamBudget(values int, maxBytes uint64) error {
	if values <= 0 {
		return fmt.Errorf("%w: parameter count must be positive", ErrShape)
	}
	count := uint64(values)
	// The exact plan depends on the adapter's chunk limit. This lower bound
	// includes both resident input/state storage and one complete chunk's
	// status, uniform, upload-stage overhead, and shared snapshot staging.
	state, ok := checkedAdamMul(deviceAdamStateBytes*2+deviceAdamInputBytes*2, count)
	if !ok {
		return fmt.Errorf("%w: parameter buffers overflow", ErrCapacity)
	}
	perChunk := deviceAdamStatusBytes + deviceAdamStagingBytes + deviceAdamUniformBytes + deviceAdamUniformBytes + deviceAdamStatusBytes
	minimum, ok := checkedAdamAdd(state, perChunk)
	if !ok {
		return fmt.Errorf("%w: resident allocation overflows", ErrCapacity)
	}
	minimum, ok = checkedAdamAdd(minimum, deviceAdamStateBytes)
	if !ok || maxBytes < minimum {
		return fmt.Errorf("%w: max bytes %d is below the minimum resident allocation", ErrCapacity, maxBytes)
	}
	return nil
}

func checkedAdamMul(a, b uint64) (uint64, bool) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, false
	}
	return a * b, true
}

func checkedAdamAdd(a, b uint64) (uint64, bool) {
	if b > math.MaxUint64-a {
		return 0, false
	}
	return a + b, true
}

func isFinite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
