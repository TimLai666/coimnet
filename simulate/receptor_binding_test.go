package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/plasticity"
)

func nativeReceptorProtocol(core, kind, refs string, index int, frozen bool) Protocol {
	p := plasticLIFProtocol(5, .3, 1.5, true)
	if core == CoreContinuous {
		p.Core, p.LIF = core, nil
		p.Continuous = &dynamics.Config{DT: 1, Activation: "tanh"}
	}
	p.Probes = []Probe{{Name: "all", Nodes: []int{0, 1, 2}, Reduce: ReduceMeanOutput}}
	p.Plasticity.Frozen = frozen
	rule := &p.Plasticity.Rule
	if kind == plasticity.RuleSTDPPair {
		rule.Kind = kind
		rule.DecayPre, rule.DecayPost, rule.APlus, rule.AMinus = .5, .25, 1, .5
	}
	if refs == "gate" || refs == "both" {
		gateIndex := index
		rule.GateReceptor, rule.GateScale = &gateIndex, 17
	}
	if refs == "window" || refs == "both" {
		windowIndex := index
		rule.DecayEReceptor = &windowIndex
		rule.DecayEBase, rule.DecayESpan, rule.DecayEMin, rule.DecayEMax = .25, .5, .1, .9
	}
	return p
}

var nativeReceptorCores = [][2]string{
	{CoreContinuous, plasticity.RuleHebbianRate},
	{CoreLIF, plasticity.RuleHebbianRate},
	{CoreLIF, plasticity.RuleSTDPPair},
}

func nativeReceptorJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Every public native entry must reject an unsupported declaration before a
// runner is returned, including an explicit index zero and a frozen rule.
func TestNativeReceptorDeclarationsAreRefused(t *testing.T) {
	g := fixtureGraph(t)
	for _, pair := range nativeReceptorCores {
		for _, refs := range []string{"gate", "window", "both"} {
			for _, index := range []int{0, 100} {
				for _, frozen := range []bool{false, true} {
					for _, entry := range []string{"Validate", "DecodeProtocol", "Build", "BuildVariant"} {
						name := fmt.Sprintf("%s/%s/%s/%d/frozen_%t/%s", pair[0], pair[1], refs, index, frozen, entry)
						t.Run(name, func(t *testing.T) {
							p := nativeReceptorProtocol(pair[0], pair[1], refs, index, frozen)
							params := mustParameters(t, g, *p.Uniform)
							before, paramsBefore := nativeReceptorJSON(t, p), nativeReceptorJSON(t, params)
							var err error
							switch entry {
							case "Validate":
								err = p.Validate()
							case "DecodeProtocol":
								var decoded Protocol
								decoded, err = DecodeProtocol(bytes.NewReader(before))
								if err != nil && !reflect.DeepEqual(decoded, Protocol{}) {
									t.Error("failed decode returned a protocol")
								}
							case "Build", "BuildVariant":
								var runner *Runner
								if entry == "Build" {
									runner, err = Build(context.Background(), g, params, p, testLimits())
								} else {
									v := Variant{Name: VariantOriginal, Sources: []int{0, 1}, Targets: []int{1, 2}, Params: params}
									runner, err = BuildVariant(context.Background(), g, v, p, testLimits())
								}
								if err != nil && runner != nil {
									t.Error("failed build returned a runner")
								}
							}
							if err == nil {
								t.Error("native entry accepted a receptor declaration without chemistry")
							} else {
								for _, word := range []string{"receptor", "chemistry", "native"} {
									if !strings.Contains(err.Error(), word) {
										t.Errorf("error %q does not explain %q", err, word)
									}
								}
							}
							if !bytes.Equal(before, nativeReceptorJSON(t, p)) || !bytes.Equal(paramsBefore, nativeReceptorJSON(t, params)) {
								t.Error("native refusal changed caller inputs")
							}
						})
					}
				}
			}
		}
	}
}

func TestNativeReceptorDefaultsKeepStimulusLearning(t *testing.T) {
	g := fixtureGraph(t)
	for _, pair := range nativeReceptorCores {
		for _, frozen := range []bool{false, true} {
			for _, gate := range []float64{0, .3} {
				t.Run(fmt.Sprintf("%s/%s/frozen_%t/gate_%g", pair[0], pair[1], frozen, gate), func(t *testing.T) {
					p := nativeReceptorProtocol(pair[0], pair[1], "", 0, frozen)
					p.Stimulus.Inline = plasticStimulus(5, gate)
					raw := nativeReceptorJSON(t, p)
					decoded, err := DecodeProtocol(bytes.NewReader(raw))
					if err != nil {
						t.Fatal(err)
					}
					beforeHash, err := p.Hash()
					if err != nil {
						t.Fatal(err)
					}
					afterHash, err := decoded.Hash()
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(raw, nativeReceptorJSON(t, decoded)) || beforeHash != afterHash {
						t.Fatal("default protocol encoding or hash changed")
					}
					runner, report := mustRun(t, g, decoded)
					changed := false
					for _, value := range runner.PlasticState().Plastic {
						changed = changed || value != 0
					}
					if changed != (!frozen && gate != 0) || report.Plasticity == nil {
						t.Fatalf("learning changed=%t, report=%+v", changed, report.Plasticity)
					}
				})
			}
		}
	}
	for _, core := range []string{CoreContinuous, CoreLIF} {
		t.Run(core+"/absent", func(t *testing.T) {
			p := nativeReceptorProtocol(core, plasticity.RuleHebbianRate, "", 0, false)
			p.Plasticity = nil
			runner, report := mustRun(t, g, p)
			if report.Plasticity != nil || !reflect.DeepEqual(runner.PlasticState(), plasticity.State{}) {
				t.Fatal("absent plasticity block created plastic state")
			}
		})
	}
}

func TestNativeReceptorRefusalPreservesEarlierErrors(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Protocol)
		want string
	}{
		{"negative_gate", func(p *Protocol) { *p.Plasticity.Rule.GateReceptor = -1 }, "gate_receptor is -1"},
		{"negative_window", func(p *Protocol) { *p.Plasticity.Rule.DecayEReceptor = -1 }, "decay_e_receptor is -1"},
		{"bad_window", func(p *Protocol) { p.Plasticity.Rule.DecayEMin = 0 }, "decay_e_min"},
		{"edges", func(p *Protocol) { p.Plasticity.All = false }, "exactly one"},
		{"channel", func(p *Protocol) { p.Plasticity.GateChannel = 2 }, "outside [0,2)"},
		{"injection", func(p *Protocol) { p.Plasticity.GateChannel = 0 }, "also the channel"},
		{"core", func(p *Protocol) { p.Plasticity.Rule.Kind = plasticity.RuleSTDPPair }, "needs 0/1 events"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := nativeReceptorProtocol(CoreContinuous, plasticity.RuleHebbianRate, "both", 0, false)
			tc.edit(&p)
			before := nativeReceptorJSON(t, p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want earlier error containing %q", err, tc.want)
			}
			if !bytes.Equal(before, nativeReceptorJSON(t, p)) {
				t.Fatal("validation changed caller input")
			}
		})
	}
}
