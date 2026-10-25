package webgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestUnavailableTurnsIntoDeviceErrorAfterHardware checks that a missing
// adapter is a skippable ErrUnavailable only until this process has opened a
// hardware device; afterwards it is ErrDevice so hardware tests fail.
func TestUnavailableTurnsIntoDeviceErrorAfterHardware(t *testing.T) {
	saved := hardwareSeen.Load()
	t.Cleanup(func() { hardwareSeen.Store(saved) })

	hardwareSeen.Store(false)
	err := unavailable("adapter %q", "none")
	if !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrDevice) {
		t.Fatalf("before hardware: %v, want ErrUnavailable only", err)
	}
	if HardwareAdapterSeen() {
		t.Fatal("HardwareAdapterSeen() = true before any hardware device")
	}

	hardwareSeen.Store(true)
	err = unavailable("adapter %q", "none")
	if !errors.Is(err, ErrDevice) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("after hardware: %v, want ErrDevice only", err)
	}
	if !HardwareAdapterSeen() {
		t.Fatal("HardwareAdapterSeen() = false after a hardware device")
	}
}

// TestSharedInstanceIsReused checks that every executor uses one instance.
func TestSharedInstanceIsReused(t *testing.T) {
	first, err1 := sharedInstance()
	second, err2 := sharedInstance()
	if first != second || err1 != err2 {
		t.Fatalf("sharedInstance() returned %p/%v then %p/%v", first, err1, second, err2)
	}
}

// TestAdapterPreference checks the AdapterEnv values: unset keeps the
// high-performance default without demanding a type, the two names demand
// their device type exactly, and anything else is refused.
func TestAdapterPreference(t *testing.T) {
	cases := []struct {
		value      string
		preference gputypes.PowerPreference
		wantType   gputypes.DeviceType
		exact      bool
	}{
		{"", gputypes.PowerPreferenceHighPerformance, 0, false},
		{"discrete", gputypes.PowerPreferenceHighPerformance, gputypes.DeviceTypeDiscreteGPU, true},
		{"integrated", gputypes.PowerPreferenceLowPower, gputypes.DeviceTypeIntegratedGPU, true},
	}
	for _, c := range cases {
		preference, wantType, exact, err := adapterPreference(c.value)
		if err != nil || preference != c.preference || exact != c.exact || (exact && wantType != c.wantType) {
			t.Errorf("adapterPreference(%q) = %v, %v, %v, %v; want %v, %v, %v, nil", c.value, preference, wantType, exact, err, c.preference, c.wantType, c.exact)
		}
	}
	for _, bad := range []string{"igpu", "Integrated", "low"} {
		if _, _, _, err := adapterPreference(bad); err == nil {
			t.Errorf("adapterPreference(%q) accepted an unknown value", bad)
		}
	}
}
