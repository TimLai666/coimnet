package textgen

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/textgen/tokenizer"
)

func testVocabulary(t *testing.T) tokenizer.Vocabulary {
	t.Helper()
	vocab, err := tokenizer.New(tokenizer.Config{
		Format: tokenizer.FormatByte,
		Specials: []tokenizer.SpecialToken{
			{Name: "bos", Role: "bos"},
			{Name: "eos", Role: "eos"},
			{Name: "pad", Role: "pad"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return vocab
}

func TestModelConfigValidate(t *testing.T) {
	base := DefaultModelConfig()
	tests := []struct {
		name  string
		field string
		edit  func(*ModelConfig)
	}{
		{name: "embed too small", field: "embed", edit: func(c *ModelConfig) { c.Embed = 3 }},
		{name: "embed too large", field: "embed", edit: func(c *ModelConfig) { c.Embed = 257 }},
		{name: "hidden too small", field: "hidden", edit: func(c *ModelConfig) { c.Hidden = 3 }},
		{name: "hidden too large", field: "hidden", edit: func(c *ModelConfig) { c.Hidden = 513 }},
		{name: "settle too small", field: "settle", edit: func(c *ModelConfig) { c.Settle = 0 }},
		{name: "settle too large", field: "settle", edit: func(c *ModelConfig) { c.Settle = 9 }},
		{name: "learning rate zero", field: "learning_rate", edit: func(c *ModelConfig) { c.LearningRate = 0 }},
		{name: "learning rate negative", field: "learning_rate", edit: func(c *ModelConfig) { c.LearningRate = -0.1 }},
		{name: "learning rate nan", field: "learning_rate", edit: func(c *ModelConfig) { c.LearningRate = math.NaN() }},
		{name: "learning rate infinite", field: "learning_rate", edit: func(c *ModelConfig) { c.LearningRate = math.Inf(1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			tt.edit(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %v, want error naming %q", err, tt.field)
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestDefaultModelConfig(t *testing.T) {
	want := ModelConfig{Embed: 16, Hidden: 32, Settle: 3, LearningRate: 0.01, Seed: 1}
	if got := DefaultModelConfig(); got != want {
		t.Fatalf("DefaultModelConfig() = %+v, want %+v", got, want)
	}
}

func TestModelLayout(t *testing.T) {
	vocab := testVocabulary(t)
	c := ModelConfig{Embed: 4, Hidden: 5, Settle: 2, LearningRate: 0.01, Seed: 8}
	m, err := NewModel(vocab, c)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Snapshot()
	config := s.Config
	if config.Dynamics.Nodes != c.Embed+c.Hidden {
		t.Fatalf("nodes = %d, want %d", config.Dynamics.Nodes, c.Embed+c.Hidden)
	}
	wantEdges := c.Embed*c.Hidden + c.Hidden*c.Hidden
	if got := len(config.Dynamics.Sources); got != wantEdges || len(config.Dynamics.Targets) != wantEdges {
		t.Fatalf("edge counts = %d/%d, want %d", got, len(config.Dynamics.Targets), wantEdges)
	}
	index := 0
	for from := 0; from < c.Embed; from++ {
		for to := c.Embed; to < c.Embed+c.Hidden; to++ {
			if config.Dynamics.Sources[index] != from || config.Dynamics.Targets[index] != to {
				t.Fatalf("edge %d = %d->%d, want %d->%d", index, config.Dynamics.Sources[index], config.Dynamics.Targets[index], from, to)
			}
			index++
		}
	}
	for from := c.Embed; from < c.Embed+c.Hidden; from++ {
		for to := c.Embed; to < c.Embed+c.Hidden; to++ {
			if config.Dynamics.Sources[index] != from || config.Dynamics.Targets[index] != to {
				t.Fatalf("edge %d = %d->%d, want %d->%d", index, config.Dynamics.Sources[index], config.Dynamics.Targets[index], from, to)
			}
			index++
		}
	}
	if config.InputSize != vocab.Size() || config.OutputSize != vocab.Size() || !config.ReadoutEveryStep {
		t.Fatalf("input/output/readout config = %d/%d/%v", config.InputSize, config.OutputSize, config.ReadoutEveryStep)
	}
	if !reflect.DeepEqual(config.InputNodes, []int{0, 1, 2, 3}) || !reflect.DeepEqual(config.ReadoutNodes, []int{4, 5, 6, 7, 8}) {
		t.Fatalf("input/readout nodes = %v/%v", config.InputNodes, config.ReadoutNodes)
	}
	p := s.Parameters
	if config.Dynamics.DT != 1 || config.Dynamics.Activation != "tanh" {
		t.Fatalf("core dynamics = dt:%g activation:%q", config.Dynamics.DT, config.Dynamics.Activation)
	}
	if got, want := len(p.Encoder), vocab.Size()*c.Embed; got != want {
		t.Fatalf("encoder length = %d, want %d", got, want)
	}
	if got, want := len(p.Readout), c.Hidden*vocab.Size(); got != want {
		t.Fatalf("readout length = %d, want %d", got, want)
	}
	if len(p.Core.Weights) != wantEdges || len(p.Core.Bias) != c.Embed+c.Hidden || len(p.Core.LogTau) != c.Embed+c.Hidden {
		t.Fatalf("core parameter lengths = weights:%d bias:%d tau:%d", len(p.Core.Weights), len(p.Core.Bias), len(p.Core.LogTau))
	}
	for i, bias := range p.Core.Bias {
		if bias != 0 {
			t.Fatalf("bias[%d] = %g, want zero", i, bias)
		}
	}
	weightLimit := 1 / math.Sqrt(float64(c.Embed+c.Hidden))
	for i, value := range p.Core.Weights {
		if value < -weightLimit || value >= weightLimit {
			t.Fatalf("weight[%d] = %g, outside [-%g, %g)", i, value, weightLimit, weightLimit)
		}
	}
	for i, value := range p.Encoder {
		if value < -0.5 || value >= 0.5 {
			t.Fatalf("encoder[%d] = %g, outside [-0.5, 0.5)", i, value)
		}
	}
	for i, value := range p.Readout {
		if value < -0.1 || value >= 0.1 {
			t.Fatalf("readout[%d] = %g, outside [-0.1, 0.1)", i, value)
		}
	}
	for i, value := range p.Core.LogTau {
		if value != math.Log(2) {
			t.Fatalf("log_tau[%d] = %g, want log(2)", i, value)
		}
	}
	if s.Options.Trainable.Bias {
		t.Fatal("bias must not be trainable")
	}
	if !s.Options.Trainable.Encoder || !s.Options.Trainable.Weights || !s.Options.Trainable.Tau || !s.Options.Trainable.Readout {
		t.Fatalf("unexpected trainable groups: %+v", s.Options.Trainable)
	}
}

func TestModelLogitsArePredictionsOfTheNextToken(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 6, Settle: 3, LearningRate: 0.01, Seed: 12})
	if err != nil {
		t.Fatal(err)
	}
	ids := []int{256, 'a', 'b', 'c', 'd'}
	full, err := m.Logits(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != len(ids) {
		t.Fatalf("logit rows = %d, want %d", len(full), len(ids))
	}
	for k := 1; k < len(ids); k++ {
		prefix, err := m.Logits(context.Background(), ids[:k])
		if err != nil {
			t.Fatal(err)
		}
		for i := range prefix {
			if !reflect.DeepEqual(prefix[i], full[i]) {
				t.Fatalf("prefix logit row %d differs from full-sequence row", i)
			}
		}
	}
}

func TestModelMaskedPositionsHaveNoGradient(t *testing.T) {
	m, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 6, Settle: 3, LearningRate: 0.01, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ex   Example
	}{
		{name: "unscored prefix", ex: Example{IDs: []int{256, 'a', 'b', 'c'}, Scored: []bool{false, false, true, true}}},
		{name: "unscored tail padding", ex: Example{IDs: []int{256, 'a', 'b', 258, 258}, Scored: []bool{false, true, true, false, false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := m.rows(tt.ex.IDs)
			if err != nil {
				t.Fatal(err)
			}
			logits, err := m.Logits(context.Background(), tt.ex.IDs)
			if err != nil {
				t.Fatal(err)
			}
			upstream, count, err := m.upstream(tt.ex, logits)
			if err != nil {
				t.Fatal(err)
			}
			if count == 0 {
				t.Fatal("no scored positions")
			}
			for row := range upstream {
				position := row / m.config.Settle
				last := row%m.config.Settle == m.config.Settle-1
				mustBeZero := !last || position+1 >= len(tt.ex.Scored) || !tt.ex.Scored[position+1]
				if mustBeZero {
					for col, value := range upstream[row] {
						if value != 0 {
							t.Fatalf("upstream[%d][%d] = %g, want zero", row, col, value)
						}
					}
				} else {
					allZero := true
					for _, value := range upstream[row] {
						allZero = allZero && value == 0
					}
					if allZero {
						t.Fatalf("scored upstream row %d is all zero (input rows %d)", row, len(rows))
					}
				}
			}
		})
	}

	original := Example{IDs: []int{256, 'a', 'b', 258, 258}, Scored: []bool{false, true, true, false, false}}
	replaced := Example{IDs: []int{256, 'a', 'b', 'x', 'y'}, Scored: append([]bool(nil), original.Scored...)}
	first, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 6, Settle: 3, LearningRate: 0.01, Seed: 99})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 6, Settle: 3, LearningRate: 0.01, Seed: 99})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.Step(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	if _, _, err := second.Step(context.Background(), replaced); err != nil {
		t.Fatal(err)
	}
	a, b := first.Snapshot(), second.Snapshot()
	if !reflect.DeepEqual(a.Parameters, b.Parameters) || !reflect.DeepEqual(a.Optimizer, b.Optimizer) {
		t.Fatal("unscored tail token IDs changed parameters or optimizer")
	}
}

func TestModelStepLowersNLL(t *testing.T) {
	vocab := testVocabulary(t)
	config := ModelConfig{Embed: 12, Hidden: 24, Settle: 3, LearningRate: 0.01, Seed: 17}
	bos, _ := vocab.Special("bos")
	eos, _ := vocab.Special("eos")
	bytes, err := vocab.Encode("abcabcabcabc")
	if err != nil {
		t.Fatal(err)
	}
	ids := append([]int{bos}, bytes...)
	ids = append(ids, eos)
	scored := make([]bool, len(ids))
	for i := 1; i < len(scored); i++ {
		scored[i] = true
	}
	ex := Example{IDs: ids, Scored: scored}
	first, err := NewModel(vocab, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewModel(vocab, config)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot := first.Snapshot()
	before, count, err := first.NLL(context.Background(), ex)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeSnapshot, first.Snapshot()) {
		t.Fatal("NLL changed model state")
	}
	if count != len(ids)-1 {
		t.Fatalf("scored count = %d, want %d", count, len(ids)-1)
	}
	for i := 0; i < 200; i++ {
		if _, _, err := first.Step(context.Background(), ex); err != nil {
			t.Fatalf("first model step %d: %v", i, err)
		}
		if _, _, err := second.Step(context.Background(), ex); err != nil {
			t.Fatalf("second model step %d: %v", i, err)
		}
	}
	after, _, err := first.NLL(context.Background(), ex)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("training NLL: before=%g after=%g", before/float64(count), after/float64(count))
	if !(after < before) {
		t.Fatalf("NLL did not fall: before=%g after=%g", before, after)
	}
	if !reflect.DeepEqual(first.Snapshot(), second.Snapshot()) {
		t.Fatal("same-seed training did not produce identical snapshots")
	}
}

func TestModelRejectsBadInput(t *testing.T) {
	vocab := testVocabulary(t)
	config := ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: 0.01, Seed: 1}
	if _, err := NewModel(nil, config); err == nil {
		t.Fatal("NewModel accepted a nil vocabulary")
	}
	if _, err := NewModel(testVocabulary(t), ModelConfig{Embed: 4, Hidden: 4, Settle: 1, LearningRate: 0, Seed: 1}); err == nil {
		t.Fatal("NewModel accepted invalid config")
	}
	m, err := NewModel(vocab, config)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ex   Example
	}{
		{name: "length mismatch", ex: Example{IDs: []int{1, 2}, Scored: []bool{false}}},
		{name: "scored first position", ex: Example{IDs: []int{1, 2}, Scored: []bool{true, true}}},
		{name: "no scored positions", ex: Example{IDs: []int{1, 2}, Scored: []bool{false, false}}},
		{name: "out of range ID", ex: Example{IDs: []int{1, vocab.Size()}, Scored: []bool{false, true}}},
		{name: "negative ID", ex: Example{IDs: []int{1, -1}, Scored: []bool{false, true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := m.NLL(context.Background(), tt.ex); err == nil {
				t.Fatal("NLL accepted invalid example")
			}
			if _, _, err := m.Step(context.Background(), tt.ex); err == nil {
				t.Fatal("Step accepted invalid example")
			}
		})
	}
	if _, err := m.Logits(context.Background(), []int{vocab.Size()}); err == nil {
		t.Fatal("Logits accepted an out-of-range ID")
	}
	var zero Model
	if _, _, err := zero.NLL(context.Background(), Example{IDs: []int{1, 2}, Scored: []bool{false, true}}); err == nil {
		t.Fatal("NLL accepted a zero-value model")
	}
}
