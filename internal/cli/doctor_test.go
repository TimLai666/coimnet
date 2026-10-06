package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
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
	wantCPU := BackendCapabilities{
		ContinuousForward:  "implemented",
		ContinuousBackward: "implemented",
		ContinuousTraining: "implemented",
		SparseForward:      "implemented",
		SparseBackward:     "implemented",
		SparseTraining:     "implemented",
	}
	if !reflect.DeepEqual(report.Core.CPU, wantCPU) {
		t.Fatalf("cpu core capabilities = %+v, want %+v", report.Core.CPU, wantCPU)
	}
	wantGPU := BackendCapabilities{
		ContinuousForward:  "implemented_with_constraints",
		ContinuousBackward: "implemented_with_constraints",
		ContinuousTraining: "implemented_with_constraints",
		SparseForward:      "implemented_with_constraints",
		SparseBackward:     "implemented_with_constraints",
		SparseTraining:     "implemented_with_constraints",
		Details:            expectedGPUBackendDetails(),
	}
	if !reflect.DeepEqual(report.Core.GPU, wantGPU) {
		t.Fatalf("gpu core capabilities = %+v, want %+v", report.Core.GPU, wantGPU)
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
	core, ok := document["core"].(map[string]any)
	if !ok {
		t.Fatal("JSON core object is missing")
	}
	backendFields := []string{
		"continuous_forward",
		"continuous_backward",
		"continuous_training",
		"sparse_forward",
		"sparse_backward",
		"sparse_training",
	}
	for _, backendName := range []string{"cpu", "gpu"} {
		backend, ok := core[backendName].(map[string]any)
		if !ok {
			t.Fatalf("JSON core.%s object is missing", backendName)
		}
		for _, field := range backendFields {
			if _, ok := backend[field].(string); !ok {
				t.Fatalf("JSON core.%s.%s is not a string: %v", backendName, field, backend[field])
			}
		}
		if backendName == "cpu" {
			if _, ok := backend["details"]; ok {
				t.Fatal("JSON core.cpu unexpectedly contains details")
			}
			continue
		}
		gpuDetails, ok := backend["details"].(map[string]any)
		if !ok {
			t.Fatal("JSON core.gpu.details object is missing")
		}
		for field, want := range map[string]any{
			"backend":                   "webgpu",
			"model":                     "continuous",
			"state_dimension":           float64(1),
			"edge_shape":                "scalar",
			"max_delay_steps":           float64(0),
			"training_scope":            "independent_episode",
			"recompute":                 "not_supported",
			"sparse_precision":          "float32",
			"neural_state":              "cpu_float64",
			"activation":                "cpu",
			"optimizer":                 "cpu_adamw",
			"encoder_readout_execution": "not_probed",
			"device_parameter_update":   "not_implemented",
			"device_state_save":         "not_implemented",
			"full_graph":                "unverified",
			"execution":                 "not_probed",
		} {
			if got := gpuDetails[field]; got != want {
				t.Errorf("JSON core.gpu.details.%s = %v, want %v", field, got, want)
			}
		}
	}
	mediaTools, ok := document["media_tools"].(map[string]any)
	if !ok {
		t.Fatal("JSON media_tools object is missing")
	}
	if _, ok := mediaTools["video_packager"].(map[string]any); !ok {
		t.Fatal("JSON media_tools.video_packager object is missing")
	}
}

func expectedGPUBackendDetails() *GPUBackendDetails {
	return &GPUBackendDetails{
		Backend:                 "webgpu",
		Model:                   "continuous",
		StateDimension:          1,
		EdgeShape:               "scalar",
		MaxDelaySteps:           0,
		TrainingScope:           "independent_episode",
		Recompute:               "not_supported",
		SparsePrecision:         "float32",
		NeuralState:             "cpu_float64",
		Activation:              "cpu",
		Optimizer:               "cpu_adamw",
		EncoderReadoutExecution: "not_probed",
		DeviceParameterUpdate:   "not_implemented",
		DeviceStateSave:         "not_implemented",
		FullGraph:               "unverified",
		Execution:               "not_probed",
	}
}

func TestDoctorCoreCapabilitiesDetailsAreIndependent(t *testing.T) {
	first := coreCapabilities()
	second := coreCapabilities()
	if first.GPU.Details == nil || second.GPU.Details == nil {
		t.Fatalf("GPU details = first:%v second:%v, want independent non-nil details", first.GPU.Details, second.GPU.Details)
	}
	if first.GPU.Details == second.GPU.Details {
		t.Fatal("coreCapabilities returned a shared GPU details pointer")
	}
	first.GPU.Details.Model = "mutated"
	first.GPU.Details.MaxDelaySteps = 99
	if second.GPU.Details.Model != "continuous" || second.GPU.Details.MaxDelaySteps != 0 {
		t.Fatalf("second GPU details changed after first mutation: %+v", second.GPU.Details)
	}
}

func TestGPUProbeMissingOptionalToolDoesNotClaimCoreExecution(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	report, err := Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	wantReason := "probe_tool_unavailable"
	if tool, _, _ := gpuProbeSpec(runtime.GOOS); tool == "" {
		wantReason = "unsupported_platform"
	}
	if report.GPU.Status != "unknown" || report.GPU.Reason != wantReason {
		t.Fatalf("GPU probe = %+v, want unknown/%q", report.GPU, wantReason)
	}
	if report.Core.GPU.Details == nil {
		t.Fatal("GPU details are missing")
	}
	if got := report.Core.GPU.Details.Execution; got != "not_probed" {
		t.Fatalf("hardware probe status %q changed GPU execution to %q", report.GPU.Status, got)
	}
	for name, got := range map[string]string{
		"continuous_forward":  report.Core.GPU.ContinuousForward,
		"continuous_backward": report.Core.GPU.ContinuousBackward,
		"continuous_training": report.Core.GPU.ContinuousTraining,
		"sparse_forward":      report.Core.GPU.SparseForward,
		"sparse_backward":     report.Core.GPU.SparseBackward,
		"sparse_training":     report.Core.GPU.SparseTraining,
	} {
		if got != "implemented_with_constraints" {
			t.Errorf("GPU capability %s = %q, want implemented_with_constraints", name, got)
		}
	}
}

func TestDoctorMediaToolProbeReportsAvailabilityAndFailures(t *testing.T) {
	runVersion := func(context.Context, string) ([]byte, error) {
		return []byte("ffmpeg version 7.1\nbuild details\n"), nil
	}
	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	available := probeTool(context.Background(), "ffmpeg", lookPath, runVersion)
	if available.Tool != "ffmpeg" || available.Status != "available" || available.Path != "/usr/bin/ffmpeg" || available.Version != "ffmpeg version 7.1" {
		t.Fatalf("available probe = %+v", available)
	}

	absent := probeTool(context.Background(), "ffmpeg", func(string) (string, error) {
		return "", os.ErrNotExist
	}, runVersion)
	if absent.Tool != "ffmpeg" || absent.Status != "absent" {
		t.Fatalf("absent probe = %+v", absent)
	}

	probeFailure := errors.New("version command failed")
	failed := probeTool(context.Background(), "ffmpeg", lookPath, func(context.Context, string) ([]byte, error) {
		return nil, probeFailure
	})
	if failed.Tool != "ffmpeg" || failed.Status != "probe_failed" || !strings.Contains(failed.Reason, probeFailure.Error()) {
		t.Fatalf("failed probe = %+v", failed)
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
	if _, err := Doctor(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Doctor() error = %v, want context.Canceled", err)
	}
}

func TestDoctorRejectsNilContext(t *testing.T) {
	if _, err := Doctor(nil); err == nil || err.Error() != "doctor: context is nil" {
		t.Fatalf("Doctor(nil) error = %v, want doctor: context is nil", err)
	}
}

func TestDoctorRunWritesV1JSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run(doctor) error = %v", err)
	}
	var report DoctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("Run(doctor) output is not JSON: %v", err)
	}
	if report.SchemaVersion != "coimnet-doctor/v1" {
		t.Fatalf("Run(doctor) schema_version = %q", report.SchemaVersion)
	}
}

func TestDoctorHelpDescribesGPUConstraintsAndExecutionSeparation(t *testing.T) {
	var doctorHelp, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor", "--help"}, &doctorHelp, &stderr); err != nil {
		t.Fatalf("Run(doctor --help) error = %v", err)
	}
	help := strings.ToLower(doctorHelp.String())
	for _, phrase := range []string{
		"pure scalar",
		"zero-delay",
		"independent episode",
		"cpu_float64",
		"activation=cpu",
		"cpu_adamw",
		"full_graph=unverified",
		"not_probed",
	} {
		if !strings.Contains(help, phrase) {
			t.Errorf("doctor help missing %q", phrase)
		}
	}
	for _, phrase := range []string{"gpu hardware", "does not imply", "execution"} {
		if !strings.Contains(help, phrase) {
			t.Errorf("doctor help missing hardware/execution separation phrase %q", phrase)
		}
	}

	var overview, overviewErr bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, &overview, &overviewErr); err != nil {
		t.Fatalf("Run(--help) error = %v", err)
	}
	if !strings.Contains(strings.ToLower(overview.String()), "support constraints") {
		t.Fatal("top-level help does not mention GPU support constraints")
	}
}

func TestDoctorRunRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"doctor", "--unknown"}, want: "flag provided but not defined"},
		{name: "positional argument", args: []string{"doctor", "extra"}, want: "doctor takes no positional arguments"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), test.args, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run(%v) error = %v, want substring %q", test.args, err, test.want)
			}
		})
	}
}

func TestDoctorRunPropagatesOutputFailure(t *testing.T) {
	writeErr := errors.New("doctor JSON output failed")
	if err := Run(context.Background(), []string{"doctor"}, failingWriter{err: writeErr}, io.Discard); !errors.Is(err, writeErr) {
		t.Fatalf("Run(doctor) error = %v, want output error", err)
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
