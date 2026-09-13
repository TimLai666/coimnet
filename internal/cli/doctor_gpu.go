package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const gpuProbeTimeout = 2 * time.Second

// GPUReport describes the result of an actual platform probe. It does not
// advertise CoImNet backend support; that is reported under CoreCapabilities.
type GPUReport struct {
	Status  string      `json:"status"`
	Probe   string      `json:"probe"`
	Reason  string      `json:"reason,omitempty"`
	Devices []GPUDevice `json:"devices"`
}

// GPUDevice contains only values returned by the selected platform utility.
// MemoryBytes is null when that utility did not provide a parseable value.
type GPUDevice struct {
	Name        string  `json:"name"`
	Vendor      string  `json:"vendor,omitempty"`
	MemoryBytes *uint64 `json:"memory_bytes"`
}

type gpuParser func([]byte) ([]GPUDevice, error)

func probeGPU(ctx context.Context) (GPUReport, error) {
	tool, args, parser := gpuProbeSpec(runtime.GOOS)
	if tool == "" {
		return GPUReport{Status: "unknown", Probe: "none", Devices: []GPUDevice{}, Reason: "unsupported_platform"}, nil
	}
	return probeGPUCommand(ctx, tool, args, parser)
}

func gpuProbeSpec(goos string) (string, []string, gpuParser) {
	switch goos {
	case "darwin":
		return "system_profiler", []string{"SPDisplaysDataType", "-json"}, parseSystemProfiler
	case "linux", "windows":
		return "nvidia-smi", []string{"--query-gpu=name,memory.total", "--format=csv,noheader,nounits"}, parseNvidiaSMI
	default:
		return "", nil, nil
	}
}

func probeGPUCommand(ctx context.Context, tool string, args []string, parser gpuParser) (GPUReport, error) {
	report := GPUReport{Status: "unknown", Probe: tool, Devices: []GPUDevice{}}
	path, err := exec.LookPath(tool)
	if err != nil {
		report.Reason = "probe_tool_unavailable"
		return report, nil
	}
	probeContext, cancel := context.WithTimeout(ctx, gpuProbeTimeout)
	defer cancel()
	command := exec.CommandContext(probeContext, path, args...)
	output, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if probeContext.Err() == context.DeadlineExceeded {
			report.Reason = "probe_timeout"
			return report, nil
		}
		report.Reason = "probe_command_failed"
		return report, nil
	}
	if parser == nil {
		report.Reason = "probe_parser_unavailable"
		return report, nil
	}
	devices, err := parser(output)
	if err != nil {
		report.Reason = "probe_output_invalid"
		return report, nil
	}
	if len(devices) == 0 {
		report.Status = "unavailable"
		report.Reason = "no_devices_reported"
		return report, nil
	}
	report.Status = "available"
	report.Reason = ""
	report.Devices = devices
	return report, nil
}

func parseSystemProfiler(output []byte) ([]GPUDevice, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(output, &root); err != nil {
		return nil, err
	}
	raw, ok := root["SPDisplaysDataType"]
	if !ok {
		return nil, fmt.Errorf("SPDisplaysDataType is missing")
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	devices := make([]GPUDevice, 0, len(entries))
	for _, entry := range entries {
		name := stringField(entry, "sppci_model")
		if name == "" {
			name = stringField(entry, "_name")
		}
		if name == "" {
			continue
		}
		devices = append(devices, GPUDevice{
			Name:   name,
			Vendor: stringField(entry, "spdisplays_vendor"),
		})
	}
	return devices, nil
}

func parseNvidiaSMI(output []byte) ([]GPUDevice, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return []GPUDevice{}, nil
	}
	devices := make([]GPUDevice, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		name := strings.TrimSpace(parts[0])
		if name == "" {
			return nil, fmt.Errorf("GPU name is empty")
		}
		device := GPUDevice{Name: name}
		if len(parts) == 2 {
			memory, ok, err := parseMemoryMiB(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, err
			}
			if ok {
				device.MemoryBytes = &memory
			}
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func parseMemoryMiB(value string) (uint64, bool, error) {
	if value == "" || strings.EqualFold(value, "n/a") || value == "[N/A]" {
		return 0, false, nil
	}
	megabytes, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false, err
	}
	if megabytes > ^uint64(0)/(1024*1024) {
		return 0, false, fmt.Errorf("GPU memory value overflows bytes")
	}
	return megabytes * 1024 * 1024, true, nil
}

func stringField(values map[string]any, key string) string {
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
