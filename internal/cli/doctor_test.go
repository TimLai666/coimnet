package cli

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestDoctorReportContainsRuntimeBuildProbeAndSeparatedCapabilities(t *testing.T) {
	report, err := Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.SchemaVersion != "coimnet-doctor/v1" {
		t.Fatalf("schema_version = %q", report.SchemaVersion)
	}
	if report.Runtime.GoVersion != runtime.Version() {
		t.Fatalf("go_version = %q, want %q", report.Runtime.GoVersion, runtime.Version())
	}
	if report.Runtime.GOOS != runtime.GOOS || report.Runtime.GOARCH != runtime.GOARCH {
		t.Fatalf("runtime = %s/%s, want %s/%s", report.Runtime.GOOS, report.Runtime.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	if report.CPU.LogicalCPUs != runtime.NumCPU() {
		t.Fatalf("logical_cpus = %d, want %d", report.CPU.LogicalCPUs, runtime.NumCPU())
	}
	if report.CPU.PhysicalMemoryBytes != nil && *report.CPU.PhysicalMemoryBytes == 0 {
		t.Fatal("physical_memory_bytes is zero")
	}
	if report.CPU.PhysicalMemoryBytes == nil && report.CPU.PhysicalMemoryStatus != "unknown" {
		t.Fatalf("unknown physical memory has status %q", report.CPU.PhysicalMemoryStatus)
	}
	if report.BuildInfo.Insyra != nil && report.BuildInfo.Insyra.Path != "github.com/HazelnutParadise/insyra" {
		t.Fatalf("insyra module path = %q", report.BuildInfo.Insyra.Path)
	}
	if report.BuildInfo.Insyra == nil && report.BuildInfo.Status != "unknown" {
		t.Fatalf("missing insyra module with build status %q", report.BuildInfo.Status)
	}
	if report.Insyra.Probe.Status != "passed" {
		t.Fatalf("insyra probe status = %q, error = %q", report.Insyra.Probe.Status, report.Insyra.Probe.Error)
	}
	if !reflect.DeepEqual(report.Insyra.Probe.Output, []float32{23}) {
		t.Fatalf("insyra probe output = %v", report.Insyra.Probe.Output)
	}
	if !reflect.DeepEqual(report.Insyra.Probe.Gradient, []float32{88, 132}) {
		t.Fatalf("insyra probe gradient = %v", report.Insyra.Probe.Gradient)
	}
	if report.Core.CPU.ContinuousForward != "implemented" || report.Core.CPU.SparseBackward != "implemented" {
		t.Fatalf("cpu core capabilities = %+v", report.Core.CPU)
	}
	if report.Core.GPU.SparseTraining != "not_implemented" {
		t.Fatalf("gpu sparse training = %q", report.Core.GPU.SparseTraining)
	}
	if report.Core.InsyraMatrixAcceleration.CoreGPUEquivalent {
		t.Fatal("Insyra matrix acceleration was reported as CoImNet core GPU")
	}
	if report.Core.InsyraMatrixAcceleration.Status != "available_in_dependency" || report.Core.InsyraMatrixAcceleration.Execution != "not_probed" {
		t.Fatalf("Insyra matrix acceleration status = %+v", report.Core.InsyraMatrixAcceleration)
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if document["schema_version"] != "coimnet-doctor/v1" {
		t.Fatalf("JSON schema_version = %v", document["schema_version"])
	}
	if _, ok := document["gpu"]; !ok {
		t.Fatal("JSON gpu field is missing")
	}
}

func TestBuildInfoReportExtractsInsyraReplacement(t *testing.T) {
	report := buildInfoReportFrom(&debug.BuildInfo{
		Main: debug.Module{Path: "github.com/example/coimnet", Version: "(devel)"},
		Deps: []*debug.Module{{
			Path:    "github.com/HazelnutParadise/insyra",
			Version: "v0.3.2",
			Sum:     "h1:module",
			Replace: &debug.Module{Path: "/tmp/insyra", Version: "(devel)"},
		}},
	})
	if report.Status != "available" || report.Insyra == nil {
		t.Fatalf("build report = %+v", report)
	}
	if report.Insyra.Version != "v0.3.2" || report.Insyra.Replace == nil || report.Insyra.Replace.Path != "/tmp/insyra" {
		t.Fatalf("insyra module = %+v", report.Insyra)
	}
}

func TestDoctorRejectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Doctor(ctx); err == nil {
		t.Fatal("Doctor() accepted a canceled context")
	}
}

func TestEqualFloat32RejectsNonFinite(t *testing.T) {
	if equalFloat32([]float32{float32(math.NaN())}, []float32{float32(math.NaN())}) {
		t.Fatal("equalFloat32 accepted NaN")
	}
	if equalFloat32([]float32{float32(math.Inf(1))}, []float32{0}) {
		t.Fatal("equalFloat32 accepted positive infinity")
	}
	if equalFloat32([]float32{float32(math.Inf(-1))}, []float32{float32(math.Inf(-1))}) {
		t.Fatal("equalFloat32 accepted negative infinity")
	}
}

func TestGPUParsersSeparateDeviceResults(t *testing.T) {
	systemProfiler := []byte(`{"SPDisplaysDataType":[{"_name":"Apple M3","sppci_model":"Apple M3","spdisplays_vendor":"Apple"}]}`)
	devices, err := parseSystemProfiler(systemProfiler)
	if err != nil {
		t.Fatalf("parseSystemProfiler() error = %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "Apple M3" {
		t.Fatalf("system_profiler devices = %+v", devices)
	}
	nvidia := []byte("NVIDIA GeForce RTX 4070, 12288\n")
	devices, err = parseNvidiaSMI(nvidia)
	if err != nil {
		t.Fatalf("parseNvidiaSMI() error = %v", err)
	}
	if len(devices) != 1 || devices[0].Name != "NVIDIA GeForce RTX 4070" || devices[0].MemoryBytes == nil || *devices[0].MemoryBytes != 12288*1024*1024 {
		t.Fatalf("nvidia-smi devices = %+v", devices)
	}
}

func TestGPUProbeMissingToolIsUnknown(t *testing.T) {
	report, err := probeGPUCommand(context.Background(), "coimnet-tool-that-does-not-exist", nil, parseNvidiaSMI)
	if err != nil {
		t.Fatalf("probeGPUCommand() error = %v", err)
	}
	if report.Status != "unknown" || report.Reason != "probe_tool_unavailable" {
		t.Fatalf("missing tool report = %+v", report)
	}
}

func TestGPUProbeHonorsTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep is not a Windows command")
	}
	report, err := probeGPUCommand(context.Background(), "sleep", []string{"3"}, parseNvidiaSMI)
	if err != nil {
		t.Fatalf("probeGPUCommand() error = %v", err)
	}
	if report.Status != "unknown" || report.Reason != "probe_timeout" {
		t.Fatalf("timeout report = %+v", report)
	}
}
