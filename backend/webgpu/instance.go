package webgpu

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Every executor in this package opens its adapter and device from one
// process-wide instance. A gogpu instance creates and enumerates a native
// (Vulkan, Metal, ...) instance; creating and destroying one per executor
// repeats that enumeration for every executor a process opens, and when the
// native enumeration fails gogpu silently offers only its software adapter.
// The shared instance is created on first use and lives for the process;
// executors release their own device and adapter on Close.
var shared struct {
	once     sync.Once
	instance *wgpu.Instance
	err      error
}

// hardwareSeen records that this process has opened a hardware device.
var hardwareSeen atomic.Bool

func sharedInstance() (*wgpu.Instance, error) {
	shared.once.Do(func() {
		shared.instance, shared.err = wgpu.CreateInstance(nil)
	})
	return shared.instance, shared.err
}

// HardwareAdapterSeen reports whether this process has already opened a
// hardware device. Once it is true, a later failure to find one is not
// reported as ErrUnavailable (no GPU on this machine) but as ErrDevice, since
// the adapter existed and has been lost or exhausted.
func HardwareAdapterSeen() bool { return hardwareSeen.Load() }

// unavailable wraps a missing-adapter failure: ErrUnavailable on a machine
// that never offered a hardware adapter, ErrDevice once one has worked.
func unavailable(format string, args ...any) error {
	if hardwareSeen.Load() {
		return fmt.Errorf("%w: a hardware adapter was opened earlier in this process but is now unavailable: "+format, append([]any{ErrDevice}, args...)...)
	}
	return fmt.Errorf("%w: "+format, append([]any{ErrUnavailable}, args...)...)
}

// AdapterEnv names the environment variable that chooses which kind of GPU
// executors open: "discrete" (the default when unset) or "integrated". A
// machine with both, such as a desktop with an iGPU beside a graphics card,
// can then run one process on each.
const AdapterEnv = "COIMNET_GPU_ADAPTER"

// adapterPreference turns the AdapterEnv value into a power preference and the
// device type it demands. An empty value keeps the high-performance default
// without demanding a type, so a machine with only an integrated GPU still
// uses it. An explicit value must be met exactly: gogpu silently falls back
// to another GPU when the preferred kind is missing, and a run that names the
// iGPU must not quietly train on the graphics card instead.
func adapterPreference(value string) (gputypes.PowerPreference, gputypes.DeviceType, bool, error) {
	switch value {
	case "":
		return wgpu.PowerPreferenceHighPerformance, 0, false, nil
	case "discrete":
		return wgpu.PowerPreferenceHighPerformance, gputypes.DeviceTypeDiscreteGPU, true, nil
	case "integrated":
		return wgpu.PowerPreferenceLowPower, gputypes.DeviceTypeIntegratedGPU, true, nil
	}
	return 0, 0, false, fmt.Errorf("%s=%q: want discrete or integrated", AdapterEnv, value)
}

// openDevice selects the adapter AdapterEnv asks for (high-performance by
// default) from the shared instance and requests a device with the adapter's
// own limits. A software adapter is
// refused unless allowSoftware is set, which only tests do. The caller owns
// the returned adapter and device and releases them, device first.
func openDevice(ctx context.Context, allowSoftware bool) (*wgpu.Adapter, *wgpu.Device, DeviceInfo, error) {
	instance, err := sharedInstance()
	if err != nil {
		return nil, nil, DeviceInfo{}, unavailable("create instance: %v", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, DeviceInfo{}, err
	}
	preference, wantType, exact, err := adapterPreference(os.Getenv(AdapterEnv))
	if err != nil {
		return nil, nil, DeviceInfo{}, err
	}
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: preference})
	if err != nil {
		return nil, nil, DeviceInfo{}, unavailable("request adapter: %v", err)
	}
	adapterInfo := adapter.Info()
	if exact && adapterInfo.DeviceType != wantType {
		adapter.Release()
		return nil, nil, DeviceInfo{}, unavailable("%s asks for a %s adapter, got %q (%s)", AdapterEnv, wantType, adapterInfo.Name, adapterInfo.DeviceType)
	}
	hardware := usableHardwareAdapter(adapterInfo)
	if !hardware && !allowSoftware {
		adapter.Release()
		return nil, nil, DeviceInfo{}, unavailable("adapter %q backend=%s type=%s", adapterInfo.Name, adapterInfo.Backend, adapterInfo.DeviceType)
	}
	if err := ctx.Err(); err != nil {
		adapter.Release()
		return nil, nil, DeviceInfo{}, err
	}
	// A nil descriptor requests the adapter's actual limits. Do not replace
	// them with WebGPU minimums or a guessed storage-buffer size.
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		return nil, nil, DeviceInfo{}, fmt.Errorf("%w: request device for adapter %q: %v", ErrDevice, adapterInfo.Name, err)
	}
	if hardware {
		hardwareSeen.Store(true)
	}
	info := DeviceInfo{
		Adapter:    adapterInfo.Name,
		Backend:    adapterInfo.Backend.String(),
		Vendor:     adapterInfo.Vendor,
		Driver:     adapterInfo.Driver,
		DeviceType: adapterInfo.DeviceType.String(),
	}
	return adapter, device, info, nil
}
