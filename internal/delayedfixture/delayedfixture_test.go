package delayedfixture

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestDelayedEpisodeNumericalBaseline(t *testing.T) {
	episodes := []struct {
		seed, index uint64
		input       uint64
		target      uint64
	}{
		{1001, 0, 0xbfe30b0cdf657ba6, 0xbfce781498a25f70},
		{1003, 7, 0x3fe010f006f264d7, 0x3fc9b4b33e50a158},
	}
	for _, tc := range episodes {
		ep := DelayedEpisode(tc.seed, tc.index)
		if got := math.Float64bits(ep.Input[0][0]); got != tc.input {
			t.Errorf("episode (%d, %d) input bits = %#x, want %#x", tc.seed, tc.index, got, tc.input)
		}
		for row := 1; row < len(ep.Input); row++ {
			if got := math.Float64bits(ep.Input[row][0]); got != 0 {
				t.Errorf("episode (%d, %d) input[%d] bits = %#x, want 0", tc.seed, tc.index, row, got)
			}
		}
		if got := math.Float64bits(ep.Target[0]); got != tc.target {
			t.Errorf("episode (%d, %d) target bits = %#x, want %#x", tc.seed, tc.index, got, tc.target)
		}
	}
}

func TestTrainerParameterJSONBaseline(t *testing.T) {
	fixtures := []struct {
		name string
		new  func() (*learning.Trainer, error)
		want string
	}{
		{"continuous", func() (*learning.Trainer, error) { return NewDelayedTrainer(1, .1, false) }, "ffaec4b67fb034c16c609e7f42917129a1ba2965118f7c12ce6ba8e6d3dac632"},
		{"lif", func() (*learning.Trainer, error) {
			return NewDelayedLIFTrainer(1, .1, learning.Trainable{Weights: true})
		}, "2f6549acfe903c15fc00742d7306982fdb671f3cff266aac12487fd6735ae4ff"},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			trainer, err := tc.new()
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(trainer.Snapshot().Parameters)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != tc.want {
				t.Fatalf("parameter JSON SHA-256 = %s, want %s", got, tc.want)
			}
		})
	}
}
