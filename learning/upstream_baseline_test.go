package learning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	upstreamBaselineSourceCommit = "58c97b6a9f96bba0d07cf8acc0a9cdb8e00e3f9a"
	upstreamBaselineNetworkHash  = "64faf8f285b7848e0a06ef1dd6d113733af75583229324377ce6737e22849806"
	upstreamBaselineInsyra       = "v0.3.4"
	upstreamBaselineGoVersion    = "go1.26.5"
	upstreamBaselineGOOS         = "darwin"
	upstreamBaselineGOARCH       = "arm64"
)

type upstreamBaselineFile struct {
	SourceCommit string                   `json:"source_commit"`
	NetworkHash  string                   `json:"network_sha256"`
	Insyra       string                   `json:"insyra"`
	GoVersion    string                   `json:"go_version"`
	GOOS         string                   `json:"goos"`
	GOARCH       string                   `json:"goarch"`
	Cases        []upstreamBaselineResult `json:"cases"`
	RaceCases    []upstreamBaselineResult `json:"race_cases"`
}

type upstreamBaselineResult struct {
	Name             string            `json:"name"`
	Core             string            `json:"core"`
	ReadoutEveryStep bool              `json:"readout_every_step"`
	Window           int               `json:"window"`
	Gradient         learning.Gradient `json:"gradient"`
}

type upstreamBaselineRequest struct {
	name             string
	core             string
	config           learning.Config
	parameters       learning.Parameters
	input            [][]float64
	upstream         [][]float64
	readoutEveryStep bool
	window           int
}

func upstreamBaselineRequests() []upstreamBaselineRequest {
	var requests []upstreamBaselineRequest
	for _, fixture := range recomputeTrainerFixtures() {
		for _, readoutEveryStep := range []bool{false, true} {
			for _, window := range []int{0, 5} {
				config := fixture.config
				config.ReadoutEveryStep = readoutEveryStep
				if config.LIF != nil {
					config.ReadoutNodes = []int{0, 1}
				}
				requests = append(requests, upstreamBaselineRequest{
					name:             fmt.Sprintf("%s/readout-every-step=%t/window=%d", fixture.name, readoutEveryStep, window),
					core:             fixture.name,
					config:           config,
					parameters:       fixture.params,
					input:            fixture.input[:6],
					upstream:         fixture.upstream[:6],
					readoutEveryStep: readoutEveryStep,
					window:           window,
				})
			}
		}
	}
	vectorConfig := upstreamVectorSelectedConfig()
	requests = append(requests, upstreamBaselineRequest{
		name:             "vector-selected-input-nodes/signed-zero-negative-upstream",
		core:             "vector",
		config:           vectorConfig,
		parameters:       upstreamVectorSelectedParameters(),
		input:            upstreamVectorSelectedInput(),
		upstream:         upstreamVectorSelectedUpstream(),
		readoutEveryStep: true,
		window:           0,
	})
	return requests
}

func upstreamVectorSelectedConfig() learning.Config {
	return learning.Config{
		Dynamics: dynamics.Config{
			Nodes:          2,
			Sources:        []int{0},
			Targets:        []int{1},
			Delays:         []int{0},
			DT:             .5,
			Activation:     "tanh",
			StateDimension: 2,
			EdgeShape:      "matrix",
		},
		InputSize:        2,
		OutputSize:       1,
		ReadoutNodes:     []int{1},
		InputNodes:       []int{1, 0},
		ReadoutEveryStep: true,
	}
}

func upstreamVectorSelectedParameters() learning.Parameters {
	return learning.Parameters{
		Core: dynamics.Parameters{
			Weights: []float64{.3, -.1, .2, .15},
			Bias:    []float64{.1, -.2, .05, .3},
			LogTau:  []float64{.2, -.05},
		},
		Encoder: []float64{.7, -.2, .3, .6, -.1, .4, .2, -.5},
		Readout: []float64{.8, -.4},
	}
}

func upstreamVectorSelectedInput() [][]float64 {
	return [][]float64{{1, -.5}, {.25, .75}, {-.2, .3}}
}

func upstreamVectorSelectedUpstream() [][]float64 {
	return [][]float64{{math.Copysign(0, -1)}, {-.3}, {.4}}
}

func loadUpstreamBaseline(t *testing.T) upstreamBaselineFile {
	t.Helper()
	data, err := os.ReadFile("testdata/upstream-baseline.json")
	if err != nil {
		t.Fatalf("read upstream baseline: %v", err)
	}
	var file upstreamBaselineFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("decode upstream baseline: %v", err)
	}
	if file.SourceCommit != upstreamBaselineSourceCommit || file.NetworkHash != upstreamBaselineNetworkHash || file.Insyra != upstreamBaselineInsyra || file.GoVersion != upstreamBaselineGoVersion || file.GOOS != upstreamBaselineGOOS || file.GOARCH != upstreamBaselineGOARCH {
		t.Fatalf("unexpected baseline provenance: %+v", file)
	}
	return file
}

func TestUpstreamBaseline(t *testing.T) {
	baseline := loadUpstreamBaseline(t)
	cases := baseline.Cases
	profile := "normal"
	if upstreamBaselineRaceProfile {
		cases = baseline.RaceCases
		profile = "race"
	}
	byName := make(map[string]upstreamBaselineResult, len(cases))
	for _, result := range cases {
		if _, exists := byName[result.Name]; exists {
			t.Fatalf("duplicate baseline case %q", result.Name)
		}
		byName[result.Name] = result
	}
	requests := upstreamBaselineRequests()
	if len(cases) != len(requests) {
		t.Fatalf("%s baseline has %d cases, want %d", profile, len(cases), len(requests))
	}
	ctx := context.Background()
	exact := runtime.Version() == upstreamBaselineGoVersion && runtime.GOOS == upstreamBaselineGOOS && runtime.GOARCH == upstreamBaselineGOARCH
	t.Logf("baseline profile=%s comparison exact=%t (reference %s/%s %s, current %s/%s %s)", profile, exact, upstreamBaselineGOOS, upstreamBaselineGOARCH, upstreamBaselineGoVersion, runtime.GOOS, runtime.GOARCH, runtime.Version())
	for _, request := range requests {
		request := request
		t.Run(request.name, func(t *testing.T) {
			want, ok := byName[request.name]
			if !ok {
				t.Fatalf("missing baseline case %q", request.name)
			}
			network, err := learning.NewNetwork(request.config)
			if err != nil {
				t.Fatalf("NewNetwork: %v", err)
			}
			got, err := network.LossGradientFrom(ctx, request.parameters, request.input, request.upstream, request.window)
			if err != nil {
				t.Fatalf("LossGradientFrom: %v", err)
			}
			if want.Core != request.core || want.ReadoutEveryStep != request.readoutEveryStep || want.Window != request.window {
				t.Fatalf("baseline metadata = core %q every=%t window=%d, want core %q every=%t window=%d", want.Core, want.ReadoutEveryStep, want.Window, request.core, request.readoutEveryStep, request.window)
			}
			assertUpstreamGradient(t, got, want.Gradient, exact)
		})
	}
}

func assertUpstreamGradient(t *testing.T, got, want learning.Gradient, exact bool) {
	t.Helper()
	assertUpstreamValues(t, "core.weights", got.Core.Weights, want.Core.Weights, exact)
	assertUpstreamValues(t, "core.bias", got.Core.Bias, want.Core.Bias, exact)
	assertUpstreamValues(t, "core.log_tau", got.Core.LogTau, want.Core.LogTau, exact)
	assertUpstreamRows(t, "core.inputs", got.Core.Inputs, want.Core.Inputs, exact)
	assertUpstreamValues(t, "core.initial", got.Core.Initial, want.Core.Initial, exact)
	assertUpstreamValues(t, "theta_raw", got.ThetaRaw, want.ThetaRaw, exact)
	assertUpstreamValues(t, "encoder", got.Encoder, want.Encoder, exact)
	assertUpstreamValues(t, "readout", got.Readout, want.Readout, exact)
	assertUpstreamRows(t, "inputs", got.Inputs, want.Inputs, exact)
}

func assertUpstreamValues(t *testing.T, label string, got, want []float64, exact bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d values, want %d", label, len(got), len(want))
	}
	for index := range want {
		if math.IsNaN(got[index]) || math.IsInf(got[index], 0) || math.IsNaN(want[index]) || math.IsInf(want[index], 0) {
			t.Fatalf("%s[%d] has non-finite value: got %.17g, want %.17g", label, index, got[index], want[index])
		}
		if exact {
			if math.Float64bits(got[index]) != math.Float64bits(want[index]) {
				t.Fatalf("%s[%d] = %g (bits %#x), want %g (bits %#x)", label, index, got[index], math.Float64bits(got[index]), want[index], math.Float64bits(want[index]))
			}
			continue
		}
		if math.Abs(got[index]-want[index]) > 1e-12*math.Max(1, math.Abs(want[index])) {
			t.Fatalf("%s[%d] = %.17g, want %.17g within 1e-12 relative scale", label, index, got[index], want[index])
		}
	}
}

func assertUpstreamRows(t *testing.T, label string, got, want [][]float64, exact bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d rows, want %d", label, len(got), len(want))
	}
	for row := range want {
		assertUpstreamValues(t, fmt.Sprintf("%s[%d]", label, row), got[row], want[row], exact)
	}
}
