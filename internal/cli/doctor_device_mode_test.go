package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDoctorDeviceJSONDescriptors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run(doctor) error = %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("Run(doctor) output is not JSON: %v", err)
	}
	core, ok := document["core"].(map[string]any)
	if !ok {
		t.Fatal("JSON core object is missing")
	}
	gpu, ok := core["gpu"].(map[string]any)
	if !ok {
		t.Fatal("JSON core.gpu object is missing")
	}
	details, ok := gpu["details"].(map[string]any)
	if !ok {
		t.Fatal("JSON core.gpu.details object is missing")
	}

	want := map[string]any{
		"trainer": "learning.NewGPUTrainer",
		"device_trainer": map[string]any{
			"constructor":         "learning.NewGPUDeviceTrainer",
			"restore_constructor": "learning.RestoreGPUDeviceTrainer",
			"optimizer":           "device_adamw",
			"precision":           "float32",
			"parameter_mirror":    "verified_cpu_mirror",
			"snapshot":            "complete_training_bundle",
			"unsupported_options": []any{"lif", "vector_matrix_edges", "delays", "recompute", "persistent_neural_state", "ranges", "fixed_signs", "weight_sharing"},
			"execution":           "not_probed",
			"full_graph":          "unverified",
		},
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
	}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("JSON core.gpu.details = %#v, want exact descriptor %#v", details, want)
	}
}

func TestDoctorDeviceCapabilitiesOwnPointersAndSlices(t *testing.T) {
	first := coreCapabilities()
	second := coreCapabilities()
	if first.GPU.Details == nil || second.GPU.Details == nil {
		t.Fatalf("GPU details = first:%v second:%v, want non-nil details", first.GPU.Details, second.GPU.Details)
	}
	if first.GPU.Details == second.GPU.Details {
		t.Fatal("coreCapabilities returned a shared GPU details pointer")
	}
	if first.GPU.Details.DeviceTrainer == nil || second.GPU.Details.DeviceTrainer == nil {
		t.Fatalf("device trainer details = first:%v second:%v, want non-nil details", first.GPU.Details.DeviceTrainer, second.GPU.Details.DeviceTrainer)
	}
	if first.GPU.Details.DeviceTrainer == second.GPU.Details.DeviceTrainer {
		t.Fatal("coreCapabilities returned a shared device trainer pointer")
	}
	firstOptions := first.GPU.Details.DeviceTrainer.UnsupportedOptions
	secondOptions := second.GPU.Details.DeviceTrainer.UnsupportedOptions
	if len(firstOptions) == 0 || len(secondOptions) == 0 {
		t.Fatalf("unsupported options = first:%v second:%v, want populated slices", firstOptions, secondOptions)
	}
	if &firstOptions[0] == &secondOptions[0] {
		t.Fatal("coreCapabilities returned a shared unsupported-options slice")
	}

	first.GPU.Details.DeviceTrainer.UnsupportedOptions[0] = "mutated"
	if got := second.GPU.Details.DeviceTrainer.UnsupportedOptions[0]; got != "lif" {
		t.Fatalf("second unsupported option changed after first mutation: %q", got)
	}
}

func TestDoctorDeviceHelpDescribesOptInTrainerAndLimits(t *testing.T) {
	var helpOutput, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"doctor", "--help"}, &helpOutput, &stderr); err != nil {
		t.Fatalf("Run(doctor --help) error = %v", err)
	}
	help := strings.ToLower(helpOutput.String())
	for _, phrase := range []string{
		"learning.newgputrainer",
		"learning.restoregputrainer",
		"learning.newgpudevicetrainer",
		"learning.restoregpudevicetrainer",
		"opt-in",
		"float32",
		"device_adamw",
		"resident",
		"complete_training_bundle",
		"host neural state",
		"activations",
		"sparse transfers",
		"insyra",
		"execution limits",
	} {
		if !strings.Contains(help, phrase) {
			t.Errorf("doctor help missing %q", phrase)
		}
	}
	for _, option := range []string{
		"lif",
		"vector_matrix_edges",
		"delays",
		"recompute",
		"persistent_neural_state",
		"ranges",
		"fixed_signs",
		"weight_sharing",
	} {
		if !strings.Contains(help, option) {
			t.Errorf("doctor help missing unsupported option %q", option)
		}
	}
}
