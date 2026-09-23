// Package multimodaleval is the TSK-10 evaluation of one core on paired
// multimodal samples: single-modality training, per-modality accuracy,
// cross-modal retrieval and unseen-combination generalization, reported apart.
package multimodaleval

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/multimodal"
	"github.com/TimLai666/coimnet/multimodal/synthetic"
	"github.com/TimLai666/coimnet/signal"
)

// concepts are the modalities that carry the concept; position is never used
// as a model input here.
var concepts = []string{"image", "text", "audio"}

// readoutWidth is the output width of the model: one logit per shape and one
// per colour.
const readoutWidth = synthetic.Shapes + synthetic.Colours

// newTrainer builds the continuous-core concept model: inputs = width of the
// DefaultLayout vector (every modality's values plus one presence channel) fed
// through an identity encoder into input nodes 0..inputs-1; then `hidden`
// hidden nodes; then K = Shapes+Colours readout nodes (OutputSize K, readout
// only at the last row, ReadoutEveryStep false). ReadoutNodes are the hidden
// nodes in order followed by the K readout nodes, so the readout matrix is
// (hidden+K)×K: the hidden rows start at 0 and the readout-node rows are
// uniform [-0.3, 0.3] from rand.NewPCG(seed, 2). Edges in order: every input →
// every hidden, every hidden → every hidden (self loops included), every hidden
// → every readout, every input → every readout. Weights uniform [-0.1, 0.1]
// from rand.NewPCG(seed, 0); Bias 0; LogTau log(2); DefaultOptions with
// LearningRate rate, Trainable Weights+Readout (bias stays 0).
func newTrainer(seed uint64, layout multimodal.Layout, hidden int, rate float64) (*learning.Trainer, error) {
	inputs := layout.Width()
	k := readoutWidth
	hiddenFirst := inputs
	readoutFirst := inputs + hidden
	nodes := inputs + hidden + k

	var sources, targets []int
	edge := func(from, to int) {
		sources = append(sources, from)
		targets = append(targets, to)
	}
	for i := 0; i < inputs; i++ {
		for h := 0; h < hidden; h++ {
			edge(i, hiddenFirst+h)
		}
	}
	for a := 0; a < hidden; a++ {
		for b := 0; b < hidden; b++ {
			edge(hiddenFirst+a, hiddenFirst+b)
		}
	}
	for h := 0; h < hidden; h++ {
		for o := 0; o < k; o++ {
			edge(hiddenFirst+h, readoutFirst+o)
		}
	}
	for i := 0; i < inputs; i++ {
		for o := 0; o < k; o++ {
			edge(i, readoutFirst+o)
		}
	}

	inputNodes := make([]int, inputs)
	for i := range inputNodes {
		inputNodes[i] = i
	}
	readoutNodes := make([]int, 0, hidden+k)
	for h := 0; h < hidden; h++ {
		readoutNodes = append(readoutNodes, hiddenFirst+h)
	}
	for o := 0; o < k; o++ {
		readoutNodes = append(readoutNodes, readoutFirst+o)
	}

	config := learning.Config{
		Dynamics:         dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize:        inputs,
		OutputSize:       k,
		ReadoutNodes:     readoutNodes,
		InputNodes:       inputNodes,
		ReadoutEveryStep: false,
	}

	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	encoder := make([]float64, inputs*inputs)
	for i := range inputNodes {
		encoder[i*inputs+i] = 1
	}
	readout := make([]float64, len(readoutNodes)*k)
	r := rand.New(rand.NewPCG(seed, 2))
	for sel := hidden; sel < len(readoutNodes); sel++ {
		for o := 0; o < k; o++ {
			readout[sel*k+o] = r.Float64()*0.6 - 0.3
		}
	}

	p := learning.Parameters{
		Core: dynamics.Parameters{
			Weights: uniform(seed, 0, 0.1, len(sources)),
			Bias:    make([]float64, nodes),
			LogTau:  logTau,
		},
		Encoder: encoder,
		Readout: readout,
	}
	o := learning.DefaultOptions()
	o.LearningRate = rate
	o.Trainable = learning.Trainable{Weights: true, Readout: true}
	return learning.NewTrainer(config, p, o)
}

// uniform draws n values uniformly from [-m, m) with PCG(seed, stream), so
// every parameter group of a seed has its own reproducible stream.
func uniform(seed, stream uint64, m float64, n int) []float64 {
	r := rand.New(rand.NewPCG(seed, stream))
	out := make([]float64, n)
	for i := range out {
		out[i] = r.Float64()*2*m - m
	}
	return out
}

// onlyModality returns a copy of s in which only modality m is present; every
// other modality is marked absent with nil values (never zeros), exactly as a
// missing modality is declared. An absent m in s is an error.
func onlyModality(s multimodal.Sample, m string) (multimodal.Sample, error) {
	mod, ok := s.Modalities[m]
	if !ok || !mod.Present {
		return multimodal.Sample{}, fmt.Errorf("onlyModality: modality %q is absent in sample %q", m, s.EntityID)
	}
	out := multimodal.Sample{
		EntityID:   s.EntityID,
		EventID:    s.EventID,
		Modalities: make(map[string]multimodal.Modality, len(s.Modalities)),
		Timestamps: make(map[string]signal.Timestamp, len(s.Timestamps)),
	}
	for name, mm := range s.Modalities {
		kept := multimodal.Modality{Present: false, Shape: append([]int(nil), mm.Shape...)}
		if name == m {
			kept.Present = true
			kept.Values = append([]float64(nil), mm.Values...)
		}
		out.Modalities[name] = kept
	}
	for name, ts := range s.Timestamps {
		out.Timestamps[name] = ts
	}
	return out, nil
}

// example is one single-modality training or scoring input.
type example struct {
	Input    [][]float64
	Label    synthetic.Label
	Modality string
}

// buildExamples builds, for every index in idx and every concept modality
// present in that sample, the input `settle` copies of
// multimodal.Vector(onlyModality(sample, m), layout). Order: idx order, then
// concepts order.
func buildExamples(samples []multimodal.Sample, labels []synthetic.Label, idx []int, layout multimodal.Layout, settle int) ([]example, error) {
	if settle < 1 {
		return nil, fmt.Errorf("buildExamples: settle %d must be positive", settle)
	}
	var out []example
	for _, i := range idx {
		if i < 0 || i >= len(samples) || i >= len(labels) {
			return nil, fmt.Errorf("buildExamples: index %d out of range for %d samples and %d labels", i, len(samples), len(labels))
		}
		for _, m := range concepts {
			mod, ok := samples[i].Modalities[m]
			if !ok || !mod.Present {
				continue
			}
			one, err := onlyModality(samples[i], m)
			if err != nil {
				return nil, err
			}
			vec, err := multimodal.Vector(one, layout)
			if err != nil {
				return nil, err
			}
			input := make([][]float64, settle)
			for t := range input {
				input[t] = append([]float64(nil), vec...)
			}
			out = append(out, example{Input: input, Label: labels[i], Modality: m})
		}
	}
	return out, nil
}

// train runs epochs passes over the examples in the given order; each example
// is one StepFrom whose last upstream row is (softmax(shape logits) −
// onehot(shape)) followed by (softmax(colour logits) − onehot(colour)) and
// whose other rows are zero. Shape logits are readout 0..Shapes-1, colour
// logits are Shapes..Shapes+Colours-1. Returns the trainer's update count
// after the last step.
func train(ctx context.Context, tr *learning.Trainer, examples []example, epochs int) (uint64, error) {
	var updates uint64
	for e := 0; e < epochs; e++ {
		for _, ex := range examples {
			logits, err := tr.Predict(ctx, ex.Input)
			if err != nil {
				return updates, err
			}
			upstream := make([][]float64, len(ex.Input))
			for t := range upstream {
				upstream[t] = make([]float64, readoutWidth)
			}
			last := upstream[len(upstream)-1]
			shape := softmax(logits[:synthetic.Shapes])
			for j, p := range shape {
				last[j] = p
			}
			last[ex.Label.Shape]--
			colour := softmax(logits[synthetic.Shapes:])
			for j, p := range colour {
				last[synthetic.Shapes+j] = p
			}
			last[synthetic.Shapes+ex.Label.Colour]--
			result, err := tr.StepFrom(ctx, ex.Input, upstream)
			if err != nil {
				return updates, err
			}
			updates = result.Updates
		}
	}
	return updates, nil
}

// Scores is one evaluation set's scores. Accuracies are per modality (argmax
// of the head equals the label). Retrieval uses the concatenated softmax of
// the two heads as the concept vector and cosine similarity; ImageToText is
// the fraction of image inputs whose most similar text input (ties → lowest
// index) has the same (shape, colour); TextToImage is the reverse;
// AudioToImageShape is the fraction of audio inputs whose most similar image
// input has the same shape.
type Scores struct {
	Inputs            int                `json:"inputs"`
	ShapeAccuracy     map[string]float64 `json:"shape_accuracy"`
	ColourAccuracy    map[string]float64 `json:"colour_accuracy"`
	ImageToText       float64            `json:"image_to_text"`
	TextToImage       float64            `json:"text_to_image"`
	AudioToImageShape float64            `json:"audio_to_image_shape"`
}

// prediction is one scored input: its label, modality and concept vector
// (the concatenated softmax of the two heads).
type prediction struct {
	Label    synthetic.Label
	Modality string
	Vec      []float64
}

// predict runs Predict on every example (no step) and returns one prediction
// per example in order.
func predict(ctx context.Context, tr *learning.Trainer, examples []example) ([]prediction, error) {
	out := make([]prediction, len(examples))
	for i, ex := range examples {
		logits, err := tr.Predict(ctx, ex.Input)
		if err != nil {
			return nil, err
		}
		out[i] = prediction{Label: ex.Label, Modality: ex.Modality, Vec: conceptVector(logits)}
	}
	return out, nil
}

// score scores the examples with Predict only (no step): per-modality argmax
// accuracy (softmax preserves the argmax of the logits, so the concept vector
// decides both) and the three retrieval scores via retrieve, so a missing
// side scores 0 instead of NaN.
func score(ctx context.Context, tr *learning.Trainer, examples []example) (Scores, error) {
	preds, err := predict(ctx, tr, examples)
	if err != nil {
		return Scores{}, err
	}
	counts := map[string]int{}
	shapeHits := map[string]int{}
	colourHits := map[string]int{}
	out := Scores{
		ShapeAccuracy:  map[string]float64{},
		ColourAccuracy: map[string]float64{},
	}
	var images, texts, audios []prediction
	for _, p := range preds {
		out.Inputs++
		counts[p.Modality]++
		if argmax(p.Vec[:synthetic.Shapes]) == p.Label.Shape {
			shapeHits[p.Modality]++
		}
		if argmax(p.Vec[synthetic.Shapes:]) == p.Label.Colour {
			colourHits[p.Modality]++
		}
		switch p.Modality {
		case "image":
			images = append(images, p)
		case "text":
			texts = append(texts, p)
		case "audio":
			audios = append(audios, p)
		}
	}
	for m, n := range counts {
		out.ShapeAccuracy[m] = float64(shapeHits[m]) / float64(n)
		out.ColourAccuracy[m] = float64(colourHits[m]) / float64(n)
	}
	hits, n := retrieve(images, texts, sameLabel)
	out.ImageToText = ratio(hits, n)
	hits, n = retrieve(texts, images, sameLabel)
	out.TextToImage = ratio(hits, n)
	hits, n = retrieve(audios, images, sameShape)
	out.AudioToImageShape = ratio(hits, n)
	return out, nil
}

// softmax is the numerically shifted softmax of one logit row.
func softmax(v []float64) []float64 {
	high := v[0]
	for _, x := range v {
		if x > high {
			high = x
		}
	}
	out := make([]float64, len(v))
	var sum float64
	for i, x := range v {
		out[i] = math.Exp(x - high)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// conceptVector is the concatenated softmax of the shape and colour heads.
func conceptVector(logits []float64) []float64 {
	return append(softmax(logits[:synthetic.Shapes]), softmax(logits[synthetic.Shapes:])...)
}

// argmax returns the index of the first largest value.
func argmax(v []float64) int {
	best := 0
	for i := 1; i < len(v); i++ {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

// mostSimilar returns the lowest index of the gallery entry whose concept
// vector has the greatest cosine similarity with query; ties keep the lowest
// index.
func mostSimilar(query []float64, gallery []prediction) int {
	best, bestSim := 0, -1.0
	for i, c := range gallery {
		if sim := cosine(query, c.Vec); sim > bestSim {
			bestSim = sim
			best = i
		}
	}
	return best
}

// retrieve counts, for every query, whether its most similar gallery entry
// (cosine, ties → lowest index) matches under match; it returns hits and the
// number of queries. An empty query list or an empty gallery returns (0, 0).
func retrieve(queries, gallery []prediction, match func(query, found synthetic.Label) bool) (hits, n int) {
	if len(queries) == 0 || len(gallery) == 0 {
		return 0, 0
	}
	for _, q := range queries {
		if match(q.Label, gallery[mostSimilar(q.Vec, gallery)].Label) {
			hits++
		}
	}
	return hits, len(queries)
}

// ratio is hits over n, or 0 when there are no queries.
func ratio(hits, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(hits) / float64(n)
}

// sameLabel reports whether the query and the found entry share the full
// (shape, colour) label.
func sameLabel(query, found synthetic.Label) bool {
	return query == found
}

// sameShape reports whether the query and the found entry share the shape.
func sameShape(query, found synthetic.Label) bool {
	return query.Shape == found.Shape
}

// cosine is the cosine similarity between two equal-length vectors; a zero
// vector pair scores zero.
func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
