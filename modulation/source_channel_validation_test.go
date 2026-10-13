package modulation

import "testing"

func sourceChannelValidationConfig(spec SourceSpec) ChemistryConfig {
	return ChemistryConfig{
		Chemistry: Chemistry{Regions: 1, Channels: 2, DT: 1, Tau: []float64{1, 1}},
		Sources: []SourceSpec{
			{Kind: SourceExternalTimeline, Channel: 0, Timeline: &ExternalTimeline{ChannelCount: 1}},
			spec,
		},
		Receptors: Receptors{Mix: MixSum},
		Regions:   RegionAssignment{NodeRegion: []int{0}},
	}
}

func TestSourceSpecRejectsNonzeroReleaseOutsideDeclaredChannel(t *testing.T) {
	cases := map[string]SourceSpec{
		"external timeline": {
			Kind:    SourceExternalTimeline,
			Channel: 1,
			Timeline: &ExternalTimeline{
				ChannelCount: 2,
				Entries: []TimelineEntry{
					{Step: 0, Channel: 0, Rate: 1},
					{Step: 0, Channel: 1, Rate: 2},
				},
			},
		},
		"replay": {
			Kind:    SourceReplay,
			Channel: 1,
			Replay:  &Replay{Trace: [][]float64{{1, 2}, {0, 0}}},
		},
	}
	for name, spec := range cases {
		if _, err := spec.Build(); err == nil {
			t.Errorf("%s: Build() accepted a nonzero rate outside channel %d", name, spec.Channel)
		}
		if err := sourceChannelValidationConfig(spec).Validate(1); err == nil {
			t.Errorf("%s: ChemistryConfig.Validate() accepted a nonzero rate outside channel %d", name, spec.Channel)
		}
	}
}

func TestSourceSpecAllowsZeroOutsideAndNonzeroOnDeclaredChannel(t *testing.T) {
	cases := map[string]struct {
		spec SourceSpec
		want []float64
	}{
		"external timeline": {
			spec: SourceSpec{
				Kind:    SourceExternalTimeline,
				Channel: 1,
				Timeline: &ExternalTimeline{
					ChannelCount: 2,
					Entries: []TimelineEntry{
						{Step: 0, Channel: 0, Rate: 0},
						{Step: 0, Channel: 1, Rate: 2},
					},
				},
			},
			want: []float64{0, 2},
		},
		"replay": {
			spec: SourceSpec{
				Kind:    SourceReplay,
				Channel: 1,
				Replay:  &Replay{Trace: [][]float64{{0, 1.5}, {0, 0}}},
			},
			want: []float64{0, 1.5},
		},
	}
	for name, tc := range cases {
		built, err := tc.spec.Build()
		if err != nil {
			t.Fatalf("%s: Build() error = %v", name, err)
		}
		got, err := built.Release(0, SourceContext{})
		if err != nil {
			t.Fatalf("%s: Release(0) error = %v", name, err)
		}
		if len(got) != len(tc.want) || got[0] != tc.want[0] || got[1] != tc.want[1] {
			t.Errorf("%s: Release(0) = %v, want %v", name, got, tc.want)
		}
		if err := sourceChannelValidationConfig(tc.spec).Validate(1); err != nil {
			t.Errorf("%s: ChemistryConfig.Validate() error = %v", name, err)
		}
	}
}
