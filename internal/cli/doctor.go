// Package cli contains command-facing diagnostics that do not own a command
// entry point. The application can marshal DoctorReport as JSON or render it
// for a human without making the diagnostic code write to stdout.
package cli

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"

	"github.com/HazelnutParadise/insyra/nn"
)

const (
	doctorSchemaVersion = "coimnet-doctor/v1"
	insyraModulePath    = "github.com/HazelnutParadise/insyra"
)

// DoctorReport is the machine-readable environment and capability report.
// Status strings deliberately distinguish an observed absence from an
// unavailable probe and from a capability that has not been implemented.
type DoctorReport struct {
	SchemaVersion string           `json:"schema_version"`
	Runtime       RuntimeReport    `json:"runtime"`
	BuildInfo     BuildInfoReport  `json:"build_info"`
	CPU           CPUReport        `json:"cpu"`
	GPU           GPUReport        `json:"gpu"`
	Insyra        InsyraReport     `json:"insyra"`
	Core          CoreCapabilities `json:"core"`
}

// RuntimeReport identifies the runtime selected for this process.
type RuntimeReport struct {
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
}

// BuildInfoReport contains the module record embedded by the Go toolchain.
// Insyra is null when the binary has no readable matching dependency record.
type BuildInfoReport struct {
	Status string      `json:"status"`
	Main   *ModuleInfo `json:"main"`
	Insyra *ModuleInfo `json:"insyra"`
	Reason string      `json:"reason,omitempty"`
}

// ModuleInfo is the subset of debug.Module needed to audit the selected
// dependency and any replacement applied by the build.
type ModuleInfo struct {
	Path    string      `json:"path"`
	Version string      `json:"version"`
	Sum     string      `json:"sum"`
	Replace *ModuleInfo `json:"replace"`
}

// CPUReport records host CPU count and physical memory. PhysicalMemoryBytes
// remains null when the platform does not expose a readable value.
type CPUReport struct {
	LogicalCPUs          int     `json:"logical_cpus"`
	PhysicalMemoryBytes  *uint64 `json:"physical_memory_bytes"`
	PhysicalMemoryStatus string  `json:"physical_memory_status"`
	PhysicalMemoryReason string  `json:"physical_memory_reason,omitempty"`
}

// InsyraReport separates the actual public API operation probe from the
// narrower matrix acceleration surface.
type InsyraReport struct {
	Probe              InsyraProbeReport            `json:"probe"`
	MatrixAcceleration MatrixAccelerationCapability `json:"matrix_acceleration"`
}

// InsyraProbeReport records values from a real public nn forward and reverse
// operation. The report keeps failed probe errors instead of replacing them
// with a generic unsupported status.
type InsyraProbeReport struct {
	Status    string    `json:"status"`
	Operation string    `json:"operation"`
	Output    []float32 `json:"output"`
	Loss      []float32 `json:"loss"`
	Gradient  []float32 `json:"gradient"`
	Error     string    `json:"error,omitempty"`
}

// MatrixAccelerationCapability describes Insyra's matrix-only device hook.
// CoreGPUEquivalent is explicit so a detected GPU or successful MatMul does
// not become a false claim about a sparse recurrent GPU backend.
type MatrixAccelerationCapability struct {
	Status            string `json:"status"`
	Execution         string `json:"execution"`
	Scope             string `json:"scope"`
	CoreGPUEquivalent bool   `json:"core_gpu_equivalent"`
	Note              string `json:"note"`
}

// CoreCapabilities reports implemented CoImNet backend behavior separately
// from hardware detection and from Insyra's matrix acceleration.
type CoreCapabilities struct {
	CPU                      BackendCapabilities          `json:"cpu"`
	GPU                      BackendCapabilities          `json:"gpu"`
	InsyraMatrixAcceleration MatrixAccelerationCapability `json:"insyra_matrix_acceleration"`
}

// BackendCapabilities is intentionally limited to capabilities that exist in
// the current core. Unsupported GPU entries are explicit rather than inferred
// from the presence of a physical GPU.
type BackendCapabilities struct {
	ContinuousForward  string `json:"continuous_forward"`
	ContinuousBackward string `json:"continuous_backward"`
	ContinuousTraining string `json:"continuous_training"`
	SparseForward      string `json:"sparse_forward"`
	SparseBackward     string `json:"sparse_backward"`
	SparseTraining     string `json:"sparse_training"`
}

// Doctor gathers runtime, dependency, hardware, and core capability facts.
// Optional platform probes are represented in the report when unavailable;
// only cancellation and a failed mandatory Insyra operation return an error.
func Doctor(ctx context.Context) (DoctorReport, error) {
	var empty DoctorReport
	if ctx == nil {
		return empty, fmt.Errorf("doctor: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}

	report := DoctorReport{
		SchemaVersion: doctorSchemaVersion,
		Runtime: RuntimeReport{
			GoVersion: runtime.Version(),
			GOOS:      runtime.GOOS,
			GOARCH:    runtime.GOARCH,
		},
		BuildInfo: buildInfoReport(),
		CPU:       cpuReport(),
		Core:      coreCapabilities(),
	}

	gpu, err := probeGPU(ctx)
	report.GPU = gpu
	if err != nil {
		return report, err
	}

	insyra, err := runInsyraProbe(ctx)
	report.Insyra = insyra
	if err != nil {
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func buildInfoReport() BuildInfoReport {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return BuildInfoReport{Status: "unknown", Reason: "build_info_unavailable"}
	}
	return buildInfoReportFrom(info)
}

func buildInfoReportFrom(info *debug.BuildInfo) BuildInfoReport {
	if info == nil {
		return BuildInfoReport{Status: "unknown", Reason: "build_info_unavailable"}
	}
	report := BuildInfoReport{Status: "available", Main: moduleInfo(&info.Main)}
	for _, dependency := range info.Deps {
		if dependency != nil && dependency.Path == insyraModulePath {
			report.Insyra = moduleInfo(dependency)
			return report
		}
	}
	report.Status = "unknown"
	report.Reason = "insyra_module_not_found"
	return report
}

func moduleInfo(module *debug.Module) *ModuleInfo {
	if module == nil {
		return nil
	}
	return &ModuleInfo{
		Path:    module.Path,
		Version: module.Version,
		Sum:     module.Sum,
		Replace: moduleInfo(module.Replace),
	}
}

func cpuReport() CPUReport {
	report := CPUReport{
		LogicalCPUs:          runtime.NumCPU(),
		PhysicalMemoryStatus: "unknown",
	}
	bytes, err := physicalMemoryBytes()
	if err != nil {
		report.PhysicalMemoryReason = err.Error()
		return report
	}
	if bytes == 0 {
		report.PhysicalMemoryReason = "physical_memory_probe_returned_zero"
		return report
	}
	report.PhysicalMemoryBytes = &bytes
	report.PhysicalMemoryStatus = "available"
	return report
}

func coreCapabilities() CoreCapabilities {
	return CoreCapabilities{
		CPU: BackendCapabilities{
			ContinuousForward:  "implemented",
			ContinuousBackward: "implemented",
			ContinuousTraining: "implemented",
			SparseForward:      "implemented",
			SparseBackward:     "implemented",
			SparseTraining:     "implemented",
		},
		GPU: BackendCapabilities{
			ContinuousForward:  "not_implemented",
			ContinuousBackward: "not_implemented",
			ContinuousTraining: "not_implemented",
			SparseForward:      "not_implemented",
			SparseBackward:     "not_implemented",
			SparseTraining:     "not_implemented",
		},
		InsyraMatrixAcceleration: MatrixAccelerationCapability{
			Status:            "available_in_dependency",
			Execution:         "not_probed",
			Scope:             "2d_float32_matmul",
			CoreGPUEquivalent: false,
			Note:              "Insyra matrix acceleration does not implement the CoImNet sparse recurrent core.",
		},
	}
}

func runInsyraProbe(ctx context.Context) (InsyraReport, error) {
	report := InsyraReport{
		Probe: InsyraProbeReport{
			Status:    "failed",
			Operation: "nn.Tape.MatMul+MSELoss+Backward",
		},
		MatrixAcceleration: MatrixAccelerationCapability{
			Status:            "available_in_dependency",
			Execution:         "not_probed",
			Scope:             "2d_float32_matmul",
			CoreGPUEquivalent: false,
			Note:              "The probe is an Insyra API check and is not a CoImNet core GPU test.",
		},
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}

	x, err := nn.NewTensor([]int{1, 2}, []float32{2, 3})
	if err != nil {
		return failInsyraProbe(report, err)
	}
	weights, err := nn.NewTensor([]int{2, 1}, []float32{4, 5})
	if err != nil {
		return failInsyraProbe(report, err)
	}
	target, err := nn.NewTensor([]int{1, 1}, []float32{1})
	if err != nil {
		return failInsyraProbe(report, err)
	}
	tape := nn.NewTape(1)
	parameter, err := tape.Param(weights)
	if err != nil {
		return failInsyraProbe(report, err)
	}
	prediction, err := tape.MatMul(x, parameter.Value())
	if err != nil {
		return failInsyraProbe(report, err)
	}
	loss, err := tape.MSELoss(prediction, target)
	if err != nil {
		return failInsyraProbe(report, err)
	}
	if err := tape.Backward(loss); err != nil {
		return failInsyraProbe(report, err)
	}
	gradient, err := tape.Grad(parameter.Value())
	if err != nil {
		return failInsyraProbe(report, err)
	}
	report.Probe.Output = prediction.Data()
	report.Probe.Loss = loss.Data()
	report.Probe.Gradient = gradient.Data()
	if !equalFloat32(report.Probe.Output, []float32{23}) ||
		!equalFloat32(report.Probe.Loss, []float32{484}) ||
		!equalFloat32(report.Probe.Gradient, []float32{88, 132}) {
		return failInsyraProbe(report, fmt.Errorf("unexpected probe values: output=%v loss=%v gradient=%v", report.Probe.Output, report.Probe.Loss, report.Probe.Gradient))
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	report.Probe.Status = "passed"
	return report, nil
}

func failInsyraProbe(report InsyraReport, err error) (InsyraReport, error) {
	report.Probe.Error = err.Error()
	return report, fmt.Errorf("doctor: Insyra probe: %w", err)
}

func equalFloat32(got, want []float32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		difference := float64(got[i] - want[i])
		if math.IsNaN(difference) || math.IsInf(difference, 0) || math.Abs(difference) > 1e-5 {
			return false
		}
	}
	return true
}
