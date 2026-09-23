package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/teacher/tool"
)

// ExternalImageTool is the allow-listed tool name the external-tool mode calls.
const ExternalImageTool = "media.render_image"

// ToolRequest is the structured request the core emits: only the two condition
// words, never the prompt text itself.
type ToolRequest struct {
	Colour string `json:"colour"`
	Shape  string `json:"shape"`
}

// RequestHead is a core that emits a ToolRequest: the prompt's two words are
// one-hot rows held for Settle rows each (the same input layout, core layout
// and initialisation as an image Generator), and the readout on the last row
// has len(ImageColours)+len(ImageShapes) logits; the request is the argmax of
// each group. It trains with softmax cross-entropy of each group against the
// prompt's own words.
type RequestHead struct {
	trainer *learning.Trainer
	config  GeneratorConfig
}

// NewRequestHead builds a deterministic image-condition request head.
func NewRequestHead(c GeneratorConfig) (*RequestHead, error) {
	if c.Modality != ModalityImage {
		return nil, fmt.Errorf("media: request head requires image modality")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	words, err := Words(ModalityImage)
	if err != nil {
		return nil, err
	}
	hiddenFirst := c.Embed
	nodes := c.Embed + c.Hidden
	sources := make([]int, 0, c.Embed*c.Hidden+c.Hidden*c.Hidden)
	targets := make([]int, 0, cap(sources))
	for input := 0; input < c.Embed; input++ {
		for hidden := hiddenFirst; hidden < nodes; hidden++ {
			sources = append(sources, input)
			targets = append(targets, hidden)
		}
	}
	for source := hiddenFirst; source < nodes; source++ {
		for target := hiddenFirst; target < nodes; target++ {
			sources = append(sources, source)
			targets = append(targets, target)
		}
	}
	inputNodes := make([]int, c.Embed)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, c.Hidden)
	for i := range readoutNodes {
		readoutNodes[i] = hiddenFirst + i
	}
	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        len(words),
		OutputSize:       len(ImageColours) + len(ImageShapes),
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	parameters := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniformGenerator(c.Seed, 0, len(sources), 1/math.Sqrt(float64(nodes))),
			Bias:    make([]float64, nodes),
			LogTau:  logTau,
		},
		Encoder: uniformGenerator(c.Seed, 1, len(words)*c.Embed, .5),
		Readout: uniformGenerator(c.Seed, 2, c.Hidden*(len(ImageColours)+len(ImageShapes)), .05),
	}
	options := learning.DefaultOptions()
	options.LearningRate = c.LearningRate
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return nil, fmt.Errorf("media: create request head trainer: %w", err)
	}
	return &RequestHead{trainer: trainer, config: c}, nil
}

// Request predicts the colour and shape words for an image prompt.
func (h *RequestHead) Request(ctx context.Context, prompt string) (ToolRequest, error) {
	var zero ToolRequest
	if h == nil || h.trainer == nil {
		return zero, fmt.Errorf("media: nil request head")
	}
	if ctx == nil {
		return zero, fmt.Errorf("media: nil context")
	}
	input, _, _, err := h.requestInput(prompt)
	if err != nil {
		return zero, err
	}
	outputs, err := h.trainer.PredictAll(ctx, input)
	if err != nil {
		return zero, fmt.Errorf("media: predict tool request: %w", err)
	}
	logits := outputs[len(outputs)-1]
	colour := ImageColours[argMax(logits[:len(ImageColours)])]
	shapeIndex := argMax(logits[len(ImageColours):])
	return ToolRequest{Colour: colour, Shape: ImageShapes[shapeIndex]}, nil
}

// Step trains the request head on the prompt's own colour and shape words and
// returns the pre-update step result and mean group cross-entropy.
func (h *RequestHead) Step(ctx context.Context, prompt string) (learning.StepResult, float64, error) {
	var zero learning.StepResult
	if h == nil || h.trainer == nil {
		return zero, 0, fmt.Errorf("media: nil request head")
	}
	if ctx == nil {
		return zero, 0, fmt.Errorf("media: nil context")
	}
	input, colourIndex, shapeIndex, err := h.requestInput(prompt)
	if err != nil {
		return zero, 0, err
	}
	outputs, err := h.trainer.PredictAll(ctx, input)
	if err != nil {
		return zero, 0, fmt.Errorf("media: predict training request: %w", err)
	}
	last := len(outputs) - 1
	logits := outputs[last]
	loss, gradient, err := requestLossGradient(logits, colourIndex, shapeIndex)
	if err != nil {
		return zero, 0, err
	}
	upstream := make([][]float64, len(outputs))
	for i := range upstream {
		upstream[i] = make([]float64, len(logits))
	}
	upstream[last] = gradient
	result, err := h.trainer.StepFrom(ctx, input, upstream)
	if err != nil {
		return zero, 0, fmt.Errorf("media: train request head: %w", err)
	}
	return result, loss, nil
}

// Capacity reports the request head's parameter counts and training split.
func (h *RequestHead) Capacity() CapacitySplit {
	if h == nil || h.trainer == nil {
		return CapacitySplit{}
	}
	snapshot := h.trainer.Snapshot()
	return CapacitySplit{
		EncoderParameters: len(snapshot.Parameters.Encoder),
		CoreParameters:    len(snapshot.Parameters.Core.Weights) + len(snapshot.Parameters.Core.Bias) + len(snapshot.Parameters.Core.LogTau),
		ReadoutParameters: len(snapshot.Parameters.Readout),
		EncoderTrains:     snapshot.Options.Trainable.Encoder,
		CoreTrains:        snapshot.Options.Trainable.Weights || snapshot.Options.Trainable.Bias || snapshot.Options.Trainable.Tau || snapshot.Options.Trainable.Theta,
		ReadoutTrains:     snapshot.Options.Trainable.Readout,
	}
}

// requestInput creates the two one-hot prompt rows held for Settle rows each.
func (h *RequestHead) requestInput(prompt string) ([][]float64, int, int, error) {
	parts, err := ParseImagePrompt(prompt)
	if err != nil {
		return nil, 0, 0, err
	}
	colourIndex, shapeIndex := -1, -1
	for i, word := range ImageColours {
		if word == parts.Colour {
			colourIndex = i
		}
	}
	for i, word := range ImageShapes {
		if word == parts.Shape {
			shapeIndex = i
		}
	}
	inputSize := len(ImageColours) + len(ImageShapes)
	input := make([][]float64, 2*h.config.Settle)
	for i := range input {
		input[i] = make([]float64, inputSize)
		word := colourIndex
		if i >= h.config.Settle {
			word = len(ImageColours) + shapeIndex
		}
		input[i][word] = 1
	}
	return input, colourIndex, shapeIndex, nil
}

// requestLossGradient returns the mean cross-entropy and its logits gradient
// for the colour and shape groups.
func requestLossGradient(logits []float64, colourIndex, shapeIndex int) (float64, []float64, error) {
	groupSize := len(ImageColours)
	if len(logits) != groupSize+len(ImageShapes) {
		return 0, nil, fmt.Errorf("media: request head produced %d logits, want %d", len(logits), groupSize+len(ImageShapes))
	}
	gradient := make([]float64, len(logits))
	loss := 0.0
	for group := 0; group < 2; group++ {
		start := group * groupSize
		target := colourIndex
		if group == 1 {
			target = shapeIndex
		}
		maximum := math.Inf(-1)
		for _, value := range logits[start : start+groupSize] {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return 0, nil, fmt.Errorf("media: request head produced non-finite logits")
			}
			if value > maximum {
				maximum = value
			}
		}
		sum := 0.0
		for _, value := range logits[start : start+groupSize] {
			sum += math.Exp(value - maximum)
		}
		logNormalizer := maximum + math.Log(sum)
		loss += logNormalizer - logits[start+target]
		for i, value := range logits[start : start+groupSize] {
			probability := math.Exp(value-maximum) / sum
			if i == target {
				probability--
			}
			gradient[start+i] = probability / 2
		}
	}
	return loss / 2, gradient, nil
}

// argMax returns the first index containing the largest value.
func argMax(values []float64) int {
	best := 0
	for i := 1; i < len(values); i++ {
		if values[i] > values[best] {
			best = i
		}
	}
	return best
}

// ToolPolicy is the external-call policy: the allow-list is exactly
// ExternalImageTool, MaxCalls is the call budget (1..10000), and the data
// policy is fixed: a request carries only the schema's two string fields.
type ToolPolicy struct {
	MaxCalls int `json:"max_calls"`
}

// Validate reports whether the tool call budget is within [1, 10000].
func (p ToolPolicy) Validate() error {
	if p.MaxCalls < 1 || p.MaxCalls > 10000 {
		return fmt.Errorf("media: max calls must be in [1, 10000]")
	}
	return nil
}

// ExternalTool wraps a registry containing only ExternalImageTool and enforces
// its per-instance call budget.
type ExternalTool struct {
	registry      *tool.Registry
	policy        ToolPolicy
	used          int
	budgetRefused int
	mu            sync.Mutex
}

// NewExternalTool creates an allow-listed image renderer with the requested
// invocation budget.
func NewExternalTool(p ToolPolicy) (*ExternalTool, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	registry := tool.NewRegistry()
	err := registry.Register(ExternalImageTool, tool.Schema{
		Fields: map[string]tool.Field{
			"colour": {Kind: "string", Required: true},
			"shape":  {Kind: "string", Required: true},
		},
		MaxBytes: 256,
	}, func(_ context.Context, args map[string]any) (json.RawMessage, error) {
		condition := ImageCondition{Colour: args["colour"].(string), Shape: args["shape"].(string)}
		pixels, err := RenderImage(condition)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Pixels []float64 `json:"pixels"`
		}{Pixels: pixels})
	})
	if err != nil {
		return nil, fmt.Errorf("media: register external image tool: %w", err)
	}
	return &ExternalTool{registry: registry, policy: p}, nil
}

// Render sends one request through the registry and returns the pixels marked
// as produced by an external tool.
func (t *ExternalTool) Render(ctx context.Context, r ToolRequest) (ToolOutput, error) {
	var zero ToolOutput
	args, err := json.Marshal(r)
	if err != nil {
		return zero, fmt.Errorf("media: marshal image tool request: %w", err)
	}
	result, err := t.invoke(ctx, args)
	if err != nil {
		return zero, err
	}
	var decoded struct {
		Pixels []float64 `json:"pixels"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		return zero, fmt.Errorf("media: decode external image result: %w", err)
	}
	return ToolOutput{Role: "external_tool", Tool: ExternalImageTool, Pixels: decoded.Pixels}, nil
}

// ToolOutput is an image the external tool produced; Role is always
// "external_tool".
type ToolOutput struct {
	Role   string    `json:"role"`
	Tool   string    `json:"tool"`
	Pixels []float64 `json:"pixels"`
}

// ToolCounts reports registry calls and schema refusals plus budget refusals.
type ToolCounts struct {
	Calls         int `json:"calls"`
	Refused       int `json:"refused"`        // schema refusals counted by the registry
	BudgetRefused int `json:"budget_refused"` // calls refused because MaxCalls was reached
}

// Counts returns registry calls, schema refusals and budget refusals.
func (t *ExternalTool) Counts() ToolCounts {
	if t == nil || t.registry == nil {
		return ToolCounts{}
	}
	t.mu.Lock()
	budgetRefused := t.budgetRefused
	t.mu.Unlock()
	counts := t.registry.Counts()
	return ToolCounts{Calls: counts.Calls, Refused: counts.Refused, BudgetRefused: budgetRefused}
}

// InvokeRaw passes arbitrary JSON arguments to the registry under the same
// budget.
func (t *ExternalTool) InvokeRaw(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	return t.invoke(ctx, args)
}

// invoke reserves one budget slot before asking the registry to validate and
// run an invocation.
func (t *ExternalTool) invoke(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t == nil || t.registry == nil {
		return nil, fmt.Errorf("media: nil external tool")
	}
	if ctx == nil {
		return nil, fmt.Errorf("media: nil context")
	}
	t.mu.Lock()
	if t.used >= t.policy.MaxCalls {
		t.budgetRefused++
		t.mu.Unlock()
		return nil, errors.New("media: external tool call budget exhausted")
	}
	t.used++
	t.mu.Unlock()
	return t.registry.Invoke(ctx, ExternalImageTool, args)
}
