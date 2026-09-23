package multimodaleval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/TimLai666/coimnet/multimodal"
	"github.com/TimLai666/coimnet/multimodal/synthetic"
)

// SchemaVersion identifies the JSON report format of this package.
const SchemaVersion = "coimnet-multimodal-eval/v1"

// Config is one multi-seed evaluation: the fixture draw, the held-out
// (shape, colour) combinations and the model hyper-parameters.
type Config struct {
	Data         synthetic.Config  `json:"data"`          // fixture draw, shared by the training and evaluation seeds
	Holdout      []synthetic.Label `json:"holdout"`       // ≥ 1 distinct (shape, colour) combinations never trained on
	TrainSeed    uint64            `json:"train_seed"`    // synthetic.Generate seed of the training data
	EvalSeed     uint64            `json:"eval_seed"`     // synthetic.Generate seed of the evaluation data; must differ from TrainSeed
	Seeds        []uint64          `json:"seeds"`         // model seeds, ≥ 1, distinct
	Epochs       int               `json:"epochs"`        // 1..1000
	Hidden       int               `json:"hidden"`        // 2..256
	Settle       int               `json:"settle"`        // 1..16
	LearningRate float64           `json:"learning_rate"` // > 0
}

// Validate checks the contract ranges: at least one distinct in-range
// holdout combination, training and evaluation data seeds that differ, at
// least one distinct model seed, epochs 1..1000, hidden 2..256, settle 1..16
// and a strictly positive learning rate.
func (c Config) Validate() error {
	if len(c.Holdout) == 0 {
		return fmt.Errorf("holdout must contain at least one (shape, colour) combination")
	}
	seen := make(map[synthetic.Label]bool, len(c.Holdout))
	for _, h := range c.Holdout {
		if h.Shape < 0 || h.Shape >= synthetic.Shapes || h.Colour < 0 || h.Colour >= synthetic.Colours {
			return fmt.Errorf("holdout label (shape %d, colour %d) out of range", h.Shape, h.Colour)
		}
		if seen[h] {
			return fmt.Errorf("holdout label (shape %d, colour %d) is duplicated", h.Shape, h.Colour)
		}
		seen[h] = true
	}
	if c.TrainSeed == c.EvalSeed {
		return fmt.Errorf("train seed %d must differ from evaluation seed %d", c.TrainSeed, c.EvalSeed)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("at least one model seed is required")
	}
	seedSeen := make(map[uint64]bool, len(c.Seeds))
	for _, s := range c.Seeds {
		if seedSeen[s] {
			return fmt.Errorf("model seed %d is duplicated", s)
		}
		seedSeen[s] = true
	}
	if c.Epochs < 1 || c.Epochs > 1000 {
		return fmt.Errorf("epochs %d must be in 1..1000", c.Epochs)
	}
	if c.Hidden < 2 || c.Hidden > 256 {
		return fmt.Errorf("hidden %d must be in 2..256", c.Hidden)
	}
	if c.Settle < 1 || c.Settle > 16 {
		return fmt.Errorf("settle %d must be in 1..16", c.Settle)
	}
	if !(c.LearningRate > 0) {
		return fmt.Errorf("learning rate %v must be positive", c.LearningRate)
	}
	return nil
}

// Chance is what a model with no concept would score on average.
type Chance struct {
	Label  float64 `json:"label"`  // 1 / (Shapes × Colours): image↔text retrieval
	Shape  float64 `json:"shape"`  // 1 / Shapes: shape accuracy and audio→image shape retrieval
	Colour float64 `json:"colour"` // 1 / Colours
}

// SeedRun is one model seed's report: the two evaluation groups scored by the
// same untrained and then trained model, plus the held-out cross-modal
// retrieval against the whole evaluation set. A step inside the seed that
// failed leaves Failed set with the error text and the scores gathered so far.
type SeedRun struct {
	Seed                  uint64    `json:"seed"`
	Updates               uint64    `json:"updates"`
	SeenBefore            Scores    `json:"seen_before"`             // evaluation data, seen combinations, untrained model
	Seen                  Scores    `json:"seen"`                    // same inputs after training
	Unseen                Scores    `json:"unseen"`                  // evaluation data restricted to the held-out combinations, after training (retrieval inside that subset only; with one held-out label it is trivially 1, so read UnseenRetrieval instead)
	UnseenRetrievalBefore Retrieval `json:"unseen_retrieval_before"` // untrained model
	UnseenRetrieval       Retrieval `json:"unseen_retrieval"`        // trained model
	Failed                bool      `json:"failed"`
	Error                 string    `json:"error,omitempty"`
}

// Retrieval is cross-modal retrieval with the held-out inputs as queries and
// the whole evaluation set (seen and held-out combinations together) of the
// other modality as the gallery, so a query only scores when its nearest
// neighbour among every combination has its own label. Queries counts the
// queries per direction (image_to_text, text_to_image, audio_to_image_shape).
type Retrieval struct {
	ImageToText       float64        `json:"image_to_text"`
	TextToImage       float64        `json:"text_to_image"`
	AudioToImageShape float64        `json:"audio_to_image_shape"`
	Queries           map[string]int `json:"queries"`
}

// Report is one complete multi-seed run: the evaluated config, its hash, the
// sample accounting, the chance baselines and every seed run.
type Report struct {
	SchemaVersion string    `json:"schema_version"`
	Config        Config    `json:"config"`
	ConfigHash    string    `json:"config_hash"`
	TrainSamples  int       `json:"train_samples"` // training samples after removing the held-out combinations
	SeenSamples   int       `json:"seen_samples"`
	UnseenSamples int       `json:"unseen_samples"`
	Chance        Chance    `json:"chance"`
	Runs          []SeedRun `json:"runs"`
	Assumptions   []string  `json:"assumptions"`
}

// evalAssumptions are the three fixed statements every Report carries.
var evalAssumptions = []string{
	"Concept formation is judged only by cross-modal retrieval and unseen-combination generalization, never by a visualization.",
	"Entity and event IDs never enter the model; the evaluator reads labels only to score.",
	"A missing modality is marked absent with nil values and a presence channel of 0; it is never replaced by zeros.",
}

// seedInputs bundles the layout, the pre-built example sets and the holdout
// set shared by every seed run.
type seedInputs struct {
	layout  multimodal.Layout
	train   []example
	seen    []example
	unseen  []example
	whole   []example
	holdout map[synthetic.Label]bool
}

// Run trains one model per seed on the training draw (Generate(TrainSeed)
// split by SplitUnseenCombinations against Holdout) and scores the evaluation
// draw (Generate(EvalSeed)) with seen and held-out combinations reported
// apart. Each seed runs newTrainer, scores the seen group and the
// whole-evaluation held-out retrieval before training, trains for Epochs, and
// then scores the seen and unseen groups and the held-out retrieval again. A
// step inside a seed that fails marks the seed Failed and the other seeds
// still run; a cancelled context aborts the whole Run. An evaluation draw in
// which the holdout combinations have no sample (empty unseen group) is an
// error. Run returns Report with a hex SHA-256 ConfigHash of json.Marshal(c).
func Run(ctx context.Context, c Config) (Report, error) {
	if err := c.Validate(); err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return Report{}, fmt.Errorf("marshal config: %w", err)
	}
	configHash := fmt.Sprintf("%x", sha256.Sum256(raw))

	trainSamples, trainLabels, err := synthetic.Generate(c.TrainSeed, c.Data)
	if err != nil {
		return Report{}, fmt.Errorf("generate training data: %w", err)
	}
	evalSamples, evalLabels, err := synthetic.Generate(c.EvalSeed, c.Data)
	if err != nil {
		return Report{}, fmt.Errorf("generate evaluation data: %w", err)
	}
	trainIdx, _, err := synthetic.SplitUnseenCombinations(trainLabels, c.Holdout)
	if err != nil {
		return Report{}, fmt.Errorf("split training data: %w", err)
	}
	seenIdx, unseenIdx, err := synthetic.SplitUnseenCombinations(evalLabels, c.Holdout)
	if err != nil {
		return Report{}, fmt.Errorf("split evaluation data: %w", err)
	}

	layout := synthetic.DefaultLayout(c.Data)
	trainExamples, err := buildExamples(trainSamples, trainLabels, trainIdx, layout, c.Settle)
	if err != nil {
		return Report{}, fmt.Errorf("build training examples: %w", err)
	}
	seenExamples, err := buildExamples(evalSamples, evalLabels, seenIdx, layout, c.Settle)
	if err != nil {
		return Report{}, fmt.Errorf("build seen examples: %w", err)
	}
	unseenExamples, err := buildExamples(evalSamples, evalLabels, unseenIdx, layout, c.Settle)
	if err != nil {
		return Report{}, fmt.Errorf("build unseen examples: %w", err)
	}
	wholeIdx := make([]int, len(evalSamples))
	for i := range wholeIdx {
		wholeIdx[i] = i
	}
	wholeExamples, err := buildExamples(evalSamples, evalLabels, wholeIdx, layout, c.Settle)
	if err != nil {
		return Report{}, fmt.Errorf("build whole-evaluation examples: %w", err)
	}
	holdoutSet := make(map[synthetic.Label]bool, len(c.Holdout))
	for _, h := range c.Holdout {
		holdoutSet[h] = true
	}

	report := Report{
		SchemaVersion: SchemaVersion,
		Config:        c,
		ConfigHash:    configHash,
		TrainSamples:  len(trainIdx),
		SeenSamples:   len(seenIdx),
		UnseenSamples: len(unseenIdx),
		Chance: Chance{
			Label:  1.0 / float64(synthetic.Shapes*synthetic.Colours),
			Shape:  1.0 / float64(synthetic.Shapes),
			Colour: 1.0 / float64(synthetic.Colours),
		},
		Assumptions: append([]string(nil), evalAssumptions...),
		Runs:        make([]SeedRun, 0, len(c.Seeds)),
	}

	in := seedInputs{
		layout:  layout,
		train:   trainExamples,
		seen:    seenExamples,
		unseen:  unseenExamples,
		whole:   wholeExamples,
		holdout: holdoutSet,
	}
	for _, seed := range c.Seeds {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		run, err := runOne(ctx, c, seed, in)
		if err != nil {
			return Report{}, err
		}
		report.Runs = append(report.Runs, run)
	}
	return report, nil
}

// runOne trains and scores one model seed in the fixed order: untrained
// scores, train, then trained scores. A failure inside the seed marks the
// run Failed and reports the error text so Run can continue with the other
// seeds; a cancelled context is returned as an error instead.
func runOne(ctx context.Context, c Config, seed uint64, in seedInputs) (SeedRun, error) {
	run := SeedRun{Seed: seed}
	abort := func(err error) (SeedRun, error) {
		if ctx.Err() != nil {
			return SeedRun{}, fmt.Errorf("seed %d: %w", seed, ctx.Err())
		}
		run.Failed = true
		run.Error = err.Error()
		return run, nil
	}

	tr, err := newTrainer(seed, in.layout, c.Hidden, c.LearningRate)
	if err != nil {
		return abort(err)
	}
	if run.SeenBefore, err = score(ctx, tr, in.seen); err != nil {
		return abort(err)
	}
	whole, err := predict(ctx, tr, in.whole)
	if err != nil {
		return abort(err)
	}
	run.UnseenRetrievalBefore = crossRetrieval(holdoutSubset(whole, in.holdout), whole)

	if run.Updates, err = train(ctx, tr, in.train, c.Epochs); err != nil {
		return abort(err)
	}
	if run.Seen, err = score(ctx, tr, in.seen); err != nil {
		return abort(err)
	}
	if run.Unseen, err = score(ctx, tr, in.unseen); err != nil {
		return abort(err)
	}
	if whole, err = predict(ctx, tr, in.whole); err != nil {
		return abort(err)
	}
	run.UnseenRetrieval = crossRetrieval(holdoutSubset(whole, in.holdout), whole)
	return run, nil
}

// crossRetrieval scores the held-out queries against the whole evaluation set
// as the gallery: image→text and text→image match the full (shape, colour)
// label, audio→image matches the shape only. Queries records how many
// queries each direction had, whatever the gallery size.
func crossRetrieval(queries, gallery []prediction) Retrieval {
	qImages := modalityPreds(queries, "image")
	qTexts := modalityPreds(queries, "text")
	qAudios := modalityPreds(queries, "audio")
	gImages := modalityPreds(gallery, "image")
	gTexts := modalityPreds(gallery, "text")
	i2tHits, i2tN := retrieve(qImages, gTexts, sameLabel)
	t2iHits, t2iN := retrieve(qTexts, gImages, sameLabel)
	a2iHits, a2iN := retrieve(qAudios, gImages, sameShape)
	return Retrieval{
		ImageToText:       ratio(i2tHits, i2tN),
		TextToImage:       ratio(t2iHits, t2iN),
		AudioToImageShape: ratio(a2iHits, a2iN),
		Queries: map[string]int{
			"image_to_text":        len(qImages),
			"text_to_image":        len(qTexts),
			"audio_to_image_shape": len(qAudios),
		},
	}
}

// holdoutSubset returns the predictions whose label is held out, preserving
// order.
func holdoutSubset(preds []prediction, holdout map[synthetic.Label]bool) []prediction {
	var out []prediction
	for _, p := range preds {
		if holdout[p.Label] {
			out = append(out, p)
		}
	}
	return out
}

// modalityPreds returns the predictions of one modality, preserving order.
func modalityPreds(preds []prediction, m string) []prediction {
	var out []prediction
	for _, p := range preds {
		if p.Modality == m {
			out = append(out, p)
		}
	}
	return out
}
