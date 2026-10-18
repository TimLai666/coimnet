// onlineocr is an example that pits learning while deployed against a frozen
// model on a KMNIST stream that changes after training. A single-character
// tasks/ocr.Recognizer is trained first (pretrain), then one learner meets a
// stream of new images (run): each image is predicted, scored, and only then
// learned from, so the running accuracy counts what the model knew before it
// saw the answer. Two scenarios move the data:
//
//   - new-classes: training sees labels 0-4 only; the stream mixes all ten.
//   - domain:      training sees all ten labels; the stream and the shifted test
//     have inverted contrast (1 - pixel).
//
// Learners: frozen (no updates), gradient (one Recognizer.TrainLine per image,
// the same update the pretraining uses) and plastic (reward-gated local
// plasticity on every core edge through learning.OnlineLearner; base
// parameters never change). scripts/bench/kmnist_online.py runs the same
// protocol for a PyTorch MLP on the same images.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/TimLai666/coimnet/bench"
	"github.com/TimLai666/coimnet/examples/internal/kmnist"
	"github.com/TimLai666/coimnet/learning"
	"github.com/TimLai666/coimnet/plasticity"
	"github.com/TimLai666/coimnet/signal"
	"github.com/TimLai666/coimnet/tasks/ocr"
	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	schemaVersion = "coimnet-online-ocr/v1"
	// Every selection takes images per class in file order: pretraining uses
	// positions [0, 600), the stream [600, 800), the tuning stream [800, 900).
	pretrainPerClass = 600
	streamFrom       = 600
	streamPerClass   = 200
	tuneFrom         = 800
	tunePerClass     = 100
	testCount        = 2000
	window           = 200
)

var allClasses = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "pretrain":
		err = pretrain(os.Args[2:])
	case "run":
		err = run(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "onlineocr:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: onlineocr pretrain --data-dir DIR --scenario new-classes|domain --model-out NEW_DIR [--epochs 20]")
	fmt.Fprintln(os.Stderr, "       onlineocr run --data-dir DIR --scenario S --model DIR --learner frozen|gradient|plastic [--gate 0.1] [--stream main|tune] --out NEW.json")
	os.Exit(2)
}

func scenarioClasses(scenario string) ([]int, bool, error) {
	switch scenario {
	case "new-classes":
		return []int{0, 1, 2, 3, 4}, false, nil
	case "domain":
		return allClasses, true, nil
	}
	return nil, false, fmt.Errorf("unknown scenario %q: use new-classes or domain", scenario)
}

func pretrain(args []string) error {
	fs := flag.NewFlagSet("pretrain", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "official KMNIST directory")
	scenario := fs.String("scenario", "", "new-classes or domain")
	modelOut := fs.String("model-out", "", "new bundle directory")
	epochs := fs.Int("epochs", 20, "passes over the pretraining images")
	seed := fs.Uint64("seed", 20260924, "recognizer initialization seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	classes, _, err := scenarioClasses(*scenario)
	if err != nil {
		return err
	}
	if *dataDir == "" || *modelOut == "" || *epochs < 1 {
		return errors.New("pretrain needs --data-dir, --model-out and positive --epochs")
	}
	ctx := context.Background()
	train, err := kmnist.Load(*dataDir, "train")
	if err != nil {
		return err
	}
	alphabet, err := kmnist.Alphabet(*dataDir)
	if err != nil {
		return err
	}
	indices, err := train.RoundRobin(classes, 0, pretrainPerClass)
	if err != nil {
		return err
	}
	// The configuration examples/realocr uses for its single-character runs.
	r, err := ocr.NewRecognizer(ocr.RecognizerConfig{
		Height: kmnist.Side, Hidden: 24, Alphabet: alphabet, LearningRate: 0.002, Seed: *seed,
		TrainableEncoder: true, InputProfile: "single-character", CharacterWidth: kmnist.Side,
	})
	if err != nil {
		return err
	}
	images := make([]glyphs.Image, len(indices))
	for i, idx := range indices {
		images[i] = train.Image(idx, false)
	}
	for e := 0; e < *epochs; e++ {
		for i, img := range images {
			if _, err := r.TrainLine(ctx, img, string(alphabet[train.Labels[indices[i]]])); err != nil {
				return fmt.Errorf("epoch %d image %d: %w", e, i, err)
			}
		}
		fmt.Fprintf(os.Stderr, "epoch %d/%d\n", e+1, *epochs)
	}
	return r.Save(ctx, *modelOut)
}

// Result is one learner on one stream.
type Result struct {
	SchemaVersion string     `json:"schema_version"`
	Framework     string     `json:"framework"`
	Scenario      string     `json:"scenario"`
	Learner       string     `json:"learner"`
	Gate          float64    `json:"gate,omitempty"`
	Stream        string     `json:"stream"`
	StreamSplit   string     `json:"stream_split"`
	TestSplit     string     `json:"test_split"`
	Host          bench.Host `json:"host"`
	Parameters    int64      `json:"parameters"`
	// Prequential is the accuracy of predictions made before each image's
	// answer was revealed, per window of consecutive stream images; Old and New
	// split it by whether pretraining saw the label (new-classes only).
	Prequential    []float64 `json:"prequential"`
	PrequentialAll float64   `json:"prequential_all"`
	PrequentialOld *float64  `json:"prequential_old,omitempty"`
	PrequentialNew *float64  `json:"prequential_new,omitempty"`
	// Test holds held-out accuracies after the stream: original, original_old
	// and original_new for new-classes; original and shifted for domain.
	// TestBefore is the same before the stream.
	TestBefore map[string]float64 `json:"test_before"`
	Test       map[string]float64 `json:"test"`
	PredictP50 float64            `json:"predict_p50_ms"`
	UpdateMean float64            `json:"update_mean_ms"`
	PeakRSS    uint64             `json:"peak_rss_bytes"`
}

// learner predicts one image as a label and learns from its true label.
type learner interface {
	predict(ctx context.Context, img glyphs.Image) (int, error)
	learn(ctx context.Context, img glyphs.Image, predicted, label int) error
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dataDir := fs.String("data-dir", "", "official KMNIST directory")
	scenario := fs.String("scenario", "", "new-classes or domain")
	modelDir := fs.String("model", "", "bundle written by pretrain")
	kind := fs.String("learner", "", "frozen, gradient or plastic")
	gate := fs.Float64("gate", 0.1, "plastic only: gate on a correct answer; a wrong answer gets its negative")
	stream := fs.String("stream", "main", "main (positions 600-799 per class) or tune (800-899)")
	machine := fs.String("machine", "", "machine name (default: host name)")
	out := fs.String("out", "", "new result file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pretrained, invert, err := scenarioClasses(*scenario)
	if err != nil {
		return err
	}
	if *dataDir == "" || *modelDir == "" || *out == "" {
		return errors.New("run needs --data-dir, --model and --out")
	}
	runtime.GOMAXPROCS(1)
	ctx := context.Background()
	train, err := kmnist.Load(*dataDir, "train")
	if err != nil {
		return err
	}
	test, err := kmnist.Load(*dataDir, "t10k")
	if err != nil {
		return err
	}
	from, per := streamFrom, streamPerClass
	if *stream == "tune" {
		from, per = tuneFrom, tunePerClass
	} else if *stream != "main" {
		return fmt.Errorf("unknown stream %q", *stream)
	}
	streamIdx, err := train.RoundRobin(allClasses, from, per)
	if err != nil {
		return err
	}
	testIdx, err := test.Balanced(testCount)
	if err != nil {
		return err
	}
	r, err := ocr.LoadRecognizer(ctx, *modelDir)
	if err != nil {
		return err
	}
	var l learner
	switch *kind {
	case "frozen":
		l = &recognizerLearner{r: r}
	case "gradient":
		l = &recognizerLearner{r: r, train: true}
	case "plastic":
		if !(*gate > 0) || math.IsInf(*gate, 0) {
			return errors.New("plastic needs a positive finite --gate")
		}
		pl, err := newPlasticLearner(r, *gate)
		if err != nil {
			return err
		}
		if err := pl.checkAgainst(ctx, r, test, testIdx[:50]); err != nil {
			return err
		}
		l = pl
	default:
		return fmt.Errorf("unknown learner %q", *kind)
	}
	evaluate := func(shift bool) (map[string]float64, error) {
		correct := map[string]int{}
		total := map[string]int{}
		for _, idx := range testIdx {
			p, err := l.predict(ctx, test.Image(idx, shift))
			if err != nil {
				return nil, err
			}
			label := test.Labels[idx]
			keys := []string{"original"}
			if shift {
				keys = []string{"shifted"}
			} else if !invert {
				keys = append(keys, groupKey(label, pretrained))
			}
			for _, k := range keys {
				total[k]++
				if p == label {
					correct[k]++
				}
			}
		}
		acc := map[string]float64{}
		for k, n := range total {
			acc[k] = float64(correct[k]) / float64(n)
		}
		return acc, nil
	}
	// Evaluation reads what the stream left behind without learning from it.
	freeze, _ := l.(interface{ freeze(bool) error })
	both := func() (map[string]float64, error) {
		if freeze != nil {
			if err := freeze.freeze(true); err != nil {
				return nil, err
			}
		}
		acc, err := evaluate(false)
		if err == nil && invert {
			var shifted map[string]float64
			if shifted, err = evaluate(true); err == nil {
				acc["shifted"] = shifted["shifted"]
			}
		}
		if freeze != nil {
			if ferr := freeze.freeze(false); err == nil {
				err = ferr
			}
		}
		if err != nil {
			return nil, err
		}
		return acc, nil
	}
	before, err := both()
	if err != nil {
		return err
	}
	var hits, oldHits, oldN, newHits, newN, winHits int
	var windows, predictMS []float64
	var updateMS float64
	for i, idx := range streamIdx {
		img, label := train.Image(idx, invert), train.Labels[idx]
		start := time.Now()
		p, err := l.predict(ctx, img)
		if err != nil {
			return err
		}
		predictMS = append(predictMS, float64(time.Since(start).Nanoseconds())/1e6)
		ok := p == label
		if ok {
			hits++
			winHits++
		}
		if !invert {
			if groupKey(label, pretrained) == "original_old" {
				oldN++
				if ok {
					oldHits++
				}
			} else {
				newN++
				if ok {
					newHits++
				}
			}
		}
		start = time.Now()
		if err := l.learn(ctx, img, p, label); err != nil {
			return fmt.Errorf("stream image %d: %w", i, err)
		}
		updateMS += float64(time.Since(start).Nanoseconds()) / 1e6
		if (i+1)%window == 0 {
			windows = append(windows, float64(winHits)/window)
			winHits = 0
		}
	}
	after, err := both()
	if err != nil {
		return err
	}
	sort.Float64s(predictMS)
	res := Result{
		SchemaVersion: schemaVersion, Framework: "coimnet", Scenario: *scenario, Learner: *kind,
		Stream: *stream, StreamSplit: kmnist.SplitID("round_robin_by_label_v1", streamIdx),
		TestSplit: kmnist.SplitID("balanced_round_robin_by_label_v1", testIdx),
		Host:      bench.CurrentHost(*machine, ""), Parameters: bench.CountParameters(r.Snapshot().Parameters),
		Prequential: windows, PrequentialAll: float64(hits) / float64(len(streamIdx)),
		TestBefore: before, Test: after,
		PredictP50: predictMS[int(math.Ceil(.5*float64(len(predictMS))))-1],
		UpdateMean: updateMS / float64(len(streamIdx)),
		PeakRSS:    bench.PeakRSS(),
	}
	if *kind == "plastic" {
		res.Gate = *gate
	}
	if !invert {
		old, fresh := float64(oldHits)/float64(oldN), float64(newHits)/float64(newN)
		res.PrequentialOld, res.PrequentialNew = &old, &fresh
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	fmt.Printf("%s %s prequential %.4f test %v\n", *scenario, *kind, res.PrequentialAll, after)
	return f.Close()
}

func groupKey(label int, pretrained []int) string {
	for _, c := range pretrained {
		if c == label {
			return "original_old"
		}
	}
	return "original_new"
}

// recognizerLearner predicts with Recognizer.Infer and, when train is set,
// learns with one TrainLine on the true label.
type recognizerLearner struct {
	r     *ocr.Recognizer
	train bool
}

func (l *recognizerLearner) predict(ctx context.Context, img glyphs.Image) (int, error) {
	text, err := l.r.Infer(ctx, img)
	if err != nil {
		return 0, err
	}
	for i, ru := range l.r.Config().Alphabet {
		if string(ru) == text {
			return i, nil
		}
	}
	return 0, fmt.Errorf("prediction %q is not in the alphabet", text)
}

func (l *recognizerLearner) learn(ctx context.Context, img glyphs.Image, _, label int) error {
	if !l.train {
		return nil
	}
	_, err := l.r.TrainLine(ctx, img, string(l.r.Config().Alphabet[label]))
	return err
}

// plasticLearner runs the trained recognizer's network as a persistent
// individual with hebbian_rate plasticity on every core edge. Each image
// starts from zero voltage; after the prediction, the online learner replays
// the image under a gate of +gate for a correct answer and -gate for a wrong
// one, so co-active edges strengthen after a hit and weaken after a miss.
// Fast changes decay by 0.9999 per neural step (four steps per image), and
// are held constant while the held-out test sets are scored.
type plasticLearner struct {
	r       *ocr.Recognizer
	ind     *learning.Individual
	online  *learning.OnlineLearner
	nodes   int
	version signal.Version
	n       int
	last    learning.ActionRecord
}

func newPlasticLearner(r *ocr.Recognizer, gate float64) (*plasticLearner, error) {
	s := r.Snapshot()
	nodes := len(s.Parameters.Core.Bias)
	ind, err := learning.NewIndividual(s.Config, s.Parameters, s.Options, make([]float64, nodes))
	if err != nil {
		return nil, err
	}
	edges := make([]int, len(s.Parameters.Core.Weights))
	for i := range edges {
		edges[i] = i
	}
	if err := ind.EnablePlasticity(plasticity.Config{
		Rule:  plasticity.Rule{Kind: plasticity.RuleHebbianRate, DecayE: 0.5, DecayP: 0.9999, PlasticMax: 1, WMin: 1e-3},
		Edges: edges,
	}); err != nil {
		return nil, err
	}
	mapper := learning.RewardGateFunc(func(score float64) ([]float64, error) {
		return []float64{gate * (2*score - 1)}, nil
	})
	online, err := learning.NewOnlineLearner(ind, learning.OnlinePolicy{
		AllowPlastic: true, UpdateEvery: 1, ClockUnit: signal.TimeUnitModelStep,
	}, mapper)
	if err != nil {
		return nil, err
	}
	return &plasticLearner{r: r, ind: ind, online: online, nodes: nodes, version: signal.CurrentSchemaVersion()}, nil
}

func (l *plasticLearner) predict(ctx context.Context, img glyphs.Image) (int, error) {
	inputs, err := l.r.Inputs(img)
	if err != nil {
		return 0, err
	}
	if err := l.ind.ResetNeural(ctx, make([]float64, l.nodes)); err != nil {
		return 0, err
	}
	l.n++
	obs, err := signal.NewObservation(l.version, fmt.Sprintf("image-%d", l.n), "stream", nil)
	if err != nil {
		return 0, err
	}
	l.last, err = l.online.Act(ctx, obs, inputs)
	if err != nil {
		return 0, err
	}
	return argmaxClass(l.last.Output[len(l.last.Output)-1]), nil
}

func (l *plasticLearner) learn(ctx context.Context, _ glyphs.Image, predicted, label int) error {
	score := 0.0
	if predicted == label {
		score = 1
	}
	at := signal.Timestamp{Value: int64(l.last.Step), Unit: signal.TimeUnitModelStep}
	fb, err := signal.NewFeedback(signal.FeedbackSpec{
		SchemaVersion: l.version, ExperienceID: fmt.Sprintf("image-%d", l.n), ActionID: l.last.ActionID,
		ProducedAt: at, AvailableAt: at, Source: "label", Score: score, ModelVersion: l.version,
	})
	if err != nil {
		return err
	}
	if err := l.online.Receive(ctx, fb); err != nil {
		return err
	}
	// The replay starts from zero voltage too, so it retraces the prediction.
	if err := l.ind.ResetNeural(ctx, make([]float64, l.nodes)); err != nil {
		return err
	}
	_, err = l.online.Update(ctx, at)
	return err
}

func (l *plasticLearner) freeze(on bool) error { return l.ind.FreezePlasticity(on) }

// checkAgainst confirms that, before any learning, the individual predicts
// what the recognizer predicts, so the two paths read the same model. Act
// during the check only decays an empty fast state.
func (l *plasticLearner) checkAgainst(ctx context.Context, r *ocr.Recognizer, s kmnist.Split, indices []int) error {
	ref := &recognizerLearner{r: r}
	for _, idx := range indices {
		img := s.Image(idx, false)
		a, err := ref.predict(ctx, img)
		if err != nil {
			return err
		}
		b, err := l.predict(ctx, img)
		if err != nil {
			return err
		}
		if a != b {
			return fmt.Errorf("individual predicts %d where the recognizer predicts %d for test image %d", b, a, idx)
		}
	}
	return nil
}

// argmaxClass reads a single-character output row: column 0 is the CTC blank
// and column k+1 is label k, as Recognizer.Infer decodes it.
func argmaxClass(row []float64) int {
	best := 1
	for c := 2; c < len(row); c++ {
		if row[c] > row[best] {
			best = c
		}
	}
	return best - 1
}
