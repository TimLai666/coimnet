//go:build darwin

package cli

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestDarwinPhysicalMemoryMatchesSystemUtility(t *testing.T) {
	raw, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		t.Fatal(err)
	}
	want, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	got, err := physicalMemoryBytes()
	if err != nil || got != want || got == 0 {
		t.Fatalf("memory=%d want=%d err=%v", got, want, err)
	}
}
