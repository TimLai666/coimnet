package webgpu

import (
	"errors"
	"testing"
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
