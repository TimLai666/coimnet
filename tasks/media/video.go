package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/TimLai666/coimnet/dynamics"
	"github.com/TimLai666/coimnet/learning"
)

const (
	videoFrameSize  = ImageSize * ImageSize * 3
	videoOutputSize = videoFrameSize + AudioBlock
)

// VideoWords returns the fixed video vocabulary in direction, sound, then tick order.
func VideoWords() []string {
	words := make([]string, 0, len(VideoDirections)+len(VideoSounds)+1)
	words = append(words, VideoDirections...)
	words = append(words, VideoSounds...)
	return append(words, "tick")
}

// VideoConfig declares the encoder, recurrent core, and training rate for video generation.
type VideoConfig struct {
	Embed        int     `json:"embed"`
	Hidden       int     `json:"hidden"`
	Settle       int     `json:"settle"`
	LearningRate float64 `json:"learning_rate"`
	Seed         uint64  `json:"seed"`
}

// Validate reports whether c declares supported video-generator dimensions and learning rate.
func (c VideoConfig) Validate() error {
	if c.Embed < 4 || c.Embed > 128 {
		return fmt.Errorf("media: embed %d is outside [4, 128]", c.Embed)
	}
	if c.Hidden < 8 || c.Hidden > 512 {
		return fmt.Errorf("media: hidden %d is outside [8, 512]", c.Hidden)
	}
	if c.Settle < 1 || c.Settle > 8 {
		return fmt.Errorf("media: settle %d is outside [1, 8]", c.Settle)
	}
	if math.IsNaN(c.LearningRate) || math.IsInf(c.LearningRate, 0) || c.LearningRate <= 0 {
		return fmt.Errorf("media: learning rate must be finite and positive")
	}
	return nil
}

// DefaultVideoConfig returns the specified finite-fixture video-generator defaults.
func DefaultVideoConfig() VideoConfig {
	return VideoConfig{Embed: 16, Hidden: 96, Settle: 2, LearningRate: .01, Seed: 1}
}

// VideoGenerator owns a video prompt encoder, recurrent core, and trainable frame/audio readout.
type VideoGenerator struct {
	config  VideoConfig
	words   []string
	trainer *learning.Trainer
}

// NewVideoGenerator builds a deterministic recurrent generator for fixture video and aligned audio.
func NewVideoGenerator(c VideoConfig) (*VideoGenerator, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	words := VideoWords()
	nodes := c.Embed + c.Hidden
	sources := make([]int, 0, c.Embed*c.Hidden+c.Hidden*c.Hidden)
	targets := make([]int, 0, cap(sources))
	for input := 0; input < c.Embed; input++ {
		for hidden := c.Embed; hidden < nodes; hidden++ {
			sources = append(sources, input)
			targets = append(targets, hidden)
		}
	}
	for source := c.Embed; source < nodes; source++ {
		for target := c.Embed; target < nodes; target++ {
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
		readoutNodes[i] = c.Embed + i
	}
	config := learning.Config{
		Dynamics:  dynamics.Config{Nodes: nodes, Sources: sources, Targets: targets, DT: 1, Activation: "tanh"},
		InputSize: len(words), OutputSize: videoOutputSize, ReadoutNodes: readoutNodes,
		InputNodes: inputNodes, ReadoutEveryStep: true,
	}
	logTau := make([]float64, nodes)
	for i := range logTau {
		logTau[i] = math.Log(2)
	}
	parameters := learning.Parameters{
		Core:    dynamics.Parameters{Weights: uniformGenerator(c.Seed, 0, len(sources), 1/math.Sqrt(float64(nodes))), Bias: make([]float64, nodes), LogTau: logTau},
		Encoder: uniformGenerator(c.Seed, 1, len(words)*c.Embed, .5),
		Readout: uniformGenerator(c.Seed, 2, c.Hidden*videoOutputSize, .05),
	}
	options := learning.DefaultOptions()
	options.LearningRate = c.LearningRate
	options.Trainable = learning.Trainable{Encoder: true, Weights: true, Tau: true, Readout: true}
	trainer, err := learning.NewTrainer(config, parameters, options)
	if err != nil {
		return nil, fmt.Errorf("media: create video generator trainer: %w", err)
	}
	return &VideoGenerator{config: c, words: words, trainer: trainer}, nil
}

// Capacity reports video-generator parameter counts and which parameter groups train.
func (g *VideoGenerator) Capacity() CapacitySplit {
	if g == nil || g.trainer == nil {
		return CapacitySplit{}
	}
	snapshot := g.trainer.Snapshot()
	return CapacitySplit{
		EncoderParameters: len(snapshot.Parameters.Encoder),
		CoreParameters:    len(snapshot.Parameters.Core.Weights) + len(snapshot.Parameters.Core.Bias) + len(snapshot.Parameters.Core.LogTau),
		ReadoutParameters: len(snapshot.Parameters.Readout),
		EncoderTrains:     snapshot.Options.Trainable.Encoder,
		CoreTrains:        snapshot.Options.Trainable.Weights || snapshot.Options.Trainable.Bias || snapshot.Options.Trainable.Tau || snapshot.Options.Trainable.Theta,
		ReadoutTrains:     snapshot.Options.Trainable.Readout,
	}
}

// Snapshot returns the trainer's independent video training snapshot.
func (g *VideoGenerator) Snapshot() learning.TrainingSnapshot {
	if g == nil || g.trainer == nil {
		return learning.TrainingSnapshot{}
	}
	return g.trainer.Snapshot()
}

// Generate returns the clamped frame sequence and audio samples produced for prompt.
func (g *VideoGenerator) Generate(ctx context.Context, prompt string) ([][]float64, []float64, error) {
	if g == nil || g.trainer == nil {
		return nil, nil, fmt.Errorf("media: nil video generator")
	}
	if ctx == nil {
		return nil, nil, fmt.Errorf("media: nil context")
	}
	input, err := g.input(prompt)
	if err != nil {
		return nil, nil, err
	}
	outputs, err := g.trainer.PredictAll(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("media: predict video output: %w", err)
	}
	frames := make([][]float64, VideoFrames)
	audio := make([]float64, VideoFrames*AudioBlock)
	for frame := 0; frame < VideoFrames; frame++ {
		row := (frame+3)*g.config.Settle - 1
		imageValues, err := Decode(ModalityImage, DecoderInput{Representation: outputs[row][:videoFrameSize]})
		if err != nil {
			return nil, nil, fmt.Errorf("media: decode frame %d: %w", frame, err)
		}
		audioValues, err := Decode(ModalityAudio, DecoderInput{Representation: outputs[row][videoFrameSize:]})
		if err != nil {
			return nil, nil, fmt.Errorf("media: decode audio block %d: %w", frame, err)
		}
		frames[frame] = imageValues
		copy(audio[frame*AudioBlock:(frame+1)*AudioBlock], audioValues)
	}
	return frames, audio, nil
}

// Step trains on one prompt against RenderVideo's frame and audio targets, returning the pre-update MSE.
func (g *VideoGenerator) Step(ctx context.Context, prompt string) (learning.StepResult, float64, error) {
	var zero learning.StepResult
	if g == nil || g.trainer == nil {
		return zero, 0, fmt.Errorf("media: nil video generator")
	}
	if ctx == nil {
		return zero, 0, fmt.Errorf("media: nil context")
	}
	input, err := g.input(prompt)
	if err != nil {
		return zero, 0, err
	}
	condition, err := ParseVideoPrompt(prompt)
	if err != nil {
		return zero, 0, err
	}
	frames, audio, err := RenderVideo(condition)
	if err != nil {
		return zero, 0, fmt.Errorf("media: render video target: %w", err)
	}
	outputs, err := g.trainer.PredictAll(ctx, input)
	if err != nil {
		return zero, 0, fmt.Errorf("media: predict video training output: %w", err)
	}
	upstream := make([][]float64, len(outputs))
	for row := range upstream {
		upstream[row] = make([]float64, videoOutputSize)
	}
	mse := 0.0
	for frame := 0; frame < VideoFrames; frame++ {
		row := (frame+3)*g.config.Settle - 1
		for i, value := range outputs[row] {
			target := 0.0
			if i < videoFrameSize {
				target = frames[frame][i]
			} else {
				target = audio[frame*AudioBlock+i-videoFrameSize]
			}
			delta := value - target
			term := delta * delta
			if math.IsNaN(term) || math.IsInf(term, 0) {
				return zero, 0, fmt.Errorf("media: squared error at frame %d value %d is non-finite", frame, i)
			}
			mse += term / float64(VideoFrames*videoOutputSize)
			upstream[row][i] = 2 * delta / float64(videoOutputSize)
		}
	}
	if math.IsNaN(mse) || math.IsInf(mse, 0) {
		return zero, 0, fmt.Errorf("media: video mean squared error is non-finite")
	}
	result, err := g.trainer.StepFrom(ctx, input, upstream)
	if err != nil {
		return zero, 0, fmt.Errorf("media: train video generator: %w", err)
	}
	return result, mse, nil
}

func (g *VideoGenerator) input(prompt string) ([][]float64, error) {
	if g == nil || g.trainer == nil {
		return nil, fmt.Errorf("media: nil video generator")
	}
	condition, err := ParseVideoPrompt(prompt)
	if err != nil {
		return nil, err
	}
	indices := make(map[string]int, len(g.words))
	for i, word := range g.words {
		indices[word] = i
	}
	input := make([][]float64, (2+VideoFrames)*g.config.Settle)
	for row := range input {
		input[row] = make([]float64, len(g.words))
		word := "tick"
		if row < g.config.Settle {
			word = condition.Direction
		} else if row < 2*g.config.Settle {
			word = condition.Sound
		}
		input[row][indices[word]] = 1
	}
	return input, nil
}

// VideoScore records one generated clip's fixture classification and synchronization result.
type VideoScore struct {
	Prompt      string  `json:"prompt"`
	Seen        bool    `json:"seen"`
	MSE         float64 `json:"mse"`
	Judged      string  `json:"judged"`
	Correct     bool    `json:"correct"`
	PathMatches int     `json:"path_matches"`
	EventFrame  int     `json:"event_frame"`
	SyncError   int     `json:"sync_error"`
}

// VideoGroup summarizes one video model before and after training.
type VideoGroup struct {
	Name           string        `json:"name"`
	Capacity       CapacitySplit `json:"capacity"`
	Updates        int           `json:"updates"`
	Before         []VideoScore  `json:"before"`
	After          []VideoScore  `json:"after"`
	SeenCorrect    int           `json:"seen_correct"`
	HeldOutCorrect int           `json:"held_out_correct"`
	SeenInSync     int           `json:"seen_in_sync"`
}

// VideoRunConfig declares held-out prompts, update count, and deterministic seeds for a video run.
type VideoRunConfig struct {
	Video   VideoConfig `json:"video"`
	Holdout []string    `json:"holdout"`
	Epochs  int         `json:"epochs"`
	Seeds   []uint64    `json:"seeds"`
}

// Validate reports whether c selects valid video conditions, held-out prompts, epochs, and seeds.
func (c VideoRunConfig) Validate() error {
	if err := c.Video.Validate(); err != nil {
		return fmt.Errorf("media: video config: %w", err)
	}
	if len(c.Holdout) == 0 {
		return fmt.Errorf("media: holdout must contain at least one video prompt")
	}
	known := make(map[string]struct{}, len(VideoDirections)*len(VideoSounds))
	for _, prompt := range videoPrompts() {
		known[prompt] = struct{}{}
	}
	held := make(map[string]struct{}, len(c.Holdout))
	for _, prompt := range c.Holdout {
		if _, ok := known[prompt]; !ok {
			return fmt.Errorf("media: holdout prompt %q is not a video condition", prompt)
		}
		if _, ok := held[prompt]; ok {
			return fmt.Errorf("media: holdout contains duplicate prompt %q", prompt)
		}
		held[prompt] = struct{}{}
	}
	if c.Epochs < 1 || c.Epochs > 10000 {
		return fmt.Errorf("media: epochs %d is outside [1, 10000]", c.Epochs)
	}
	if len(c.Seeds) == 0 {
		return fmt.Errorf("media: seeds must contain at least one seed")
	}
	seeds := make(map[uint64]struct{}, len(c.Seeds))
	for _, seed := range c.Seeds {
		if _, ok := seeds[seed]; ok {
			return fmt.Errorf("media: seeds contains duplicate seed %d", seed)
		}
		seeds[seed] = struct{}{}
	}
	return nil
}

// DefaultVideoRunConfig returns the standard video fixture run configuration.
func DefaultVideoRunConfig() VideoRunConfig {
	return VideoRunConfig{Video: DefaultVideoConfig(), Holdout: []string{"down high"}, Epochs: 80, Seeds: []uint64{1, 2, 3}}
}

// VideoRunSeed contains one seed's core, frozen-core, and core-disconnect results.
type VideoRunSeed struct {
	Seed           uint64         `json:"seed"`
	Core           VideoGroup     `json:"core"`
	FrozenCore     VideoGroup     `json:"frozen_core"`
	CoreDisconnect CoreDisconnect `json:"core_disconnect"`
	Failed         bool           `json:"failed"`
	Error          string         `json:"error,omitempty"`
}

// VideoRunReport records deterministic fixture video generation results and assumptions.
type VideoRunReport struct {
	SchemaVersion string         `json:"schema_version"`
	Config        VideoRunConfig `json:"config"`
	ConfigHash    string         `json:"config_hash"`
	Runs          []VideoRunSeed `json:"runs"`
	Assumptions   []string       `json:"assumptions"`
}

// RunVideo runs all six fixture conditions, frozen-core comparisons, and synchronization diagnostics.
func RunVideo(ctx context.Context, c VideoRunConfig) (VideoRunReport, error) {
	if ctx == nil {
		return VideoRunReport{}, fmt.Errorf("media: context is nil")
	}
	if err := c.Validate(); err != nil {
		return VideoRunReport{}, err
	}
	configJSON, err := json.Marshal(c)
	if err != nil {
		return VideoRunReport{}, fmt.Errorf("media: marshal video run config: %w", err)
	}
	hash := sha256.Sum256(configJSON)
	prompts := videoPrompts()
	report := VideoRunReport{
		SchemaVersion: "coimnet-video/v1",
		Config:        c,
		ConfigHash:    hex.EncodeToString(hash[:]),
		Runs:          make([]VideoRunSeed, len(c.Seeds)),
		Assumptions: []string{
			"The fixture is a single dot and one beep; it proves the frame, audio and timeline pipeline and its synchronisation measure, not open-ended video generation.",
			"The decoder is a fixed clamp; the prompt reaches the frames and the audio only through the core.",
			"Pixel or sample MSE says nothing about perceptual quality.",
			"The frozen-core control trains only the encoder and readout.",
		},
	}
	for i, seed := range c.Seeds {
		run := VideoRunSeed{Seed: seed}
		if err := runVideoSeed(ctx, c, seed, prompts, &run); err != nil {
			run.Failed = true
			run.Error = err.Error()
		}
		report.Runs[i] = run
	}
	return report, nil
}

func runVideoSeed(ctx context.Context, c VideoRunConfig, seed uint64, prompts []string, result *VideoRunSeed) error {
	config := c.Video
	config.Seed = seed
	core, err := NewVideoGenerator(config)
	if err != nil {
		return fmt.Errorf("create video core generator: %w", err)
	}
	frozen, err := frozenVideoCoreGenerator(core)
	if err != nil {
		return fmt.Errorf("create frozen-core video control: %w", err)
	}
	seen, heldOut := partitionPrompts(prompts, c.Holdout)
	result.Core = VideoGroup{Name: "core", Capacity: core.Capacity()}
	result.FrozenCore = VideoGroup{Name: "frozen_core", Capacity: frozen.Capacity()}
	result.Core.Before, err = scoreVideoPrompts(ctx, core, prompts, heldOut)
	if err != nil {
		return fmt.Errorf("score core before video training: %w", err)
	}
	result.FrozenCore.Before, err = scoreVideoPrompts(ctx, frozen, prompts, heldOut)
	if err != nil {
		return fmt.Errorf("score frozen-core before video training: %w", err)
	}
	if err := trainVideoPrompts(ctx, seen, c.Epochs, core, frozen); err != nil {
		return err
	}
	result.Core.Updates = int(core.Snapshot().Updates)
	result.FrozenCore.Updates = int(frozen.Snapshot().Updates)
	result.Core.After, err = scoreVideoPrompts(ctx, core, prompts, heldOut)
	if err != nil {
		return fmt.Errorf("score core after video training: %w", err)
	}
	result.FrozenCore.After, err = scoreVideoPrompts(ctx, frozen, prompts, heldOut)
	if err != nil {
		return fmt.Errorf("score frozen-core after video training: %w", err)
	}
	setVideoGroupCounts(&result.Core)
	setVideoGroupCounts(&result.FrozenCore)
	result.CoreDisconnect, err = measureVideoCoreDisconnect(ctx, core, prompts)
	if err != nil {
		return fmt.Errorf("measure video core disconnect: %w", err)
	}
	return nil
}

type videoTrainer interface {
	Step(context.Context, string) (learning.StepResult, float64, error)
}

func trainVideoPrompts(ctx context.Context, prompts []string, epochs int, trainers ...videoTrainer) error {
	for epoch := 0; epoch < epochs; epoch++ {
		for _, prompt := range prompts {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("video training canceled: %w", err)
			}
			for i, trainer := range trainers {
				if trainer == nil {
					return fmt.Errorf("video training model %d is nil", i)
				}
				if _, _, err := trainer.Step(ctx, prompt); err != nil {
					return fmt.Errorf("train video model %d on %q: %w", i, prompt, err)
				}
			}
		}
	}
	return nil
}

func frozenVideoCoreGenerator(source *VideoGenerator) (*VideoGenerator, error) {
	snapshot := source.Snapshot()
	snapshot.Options.Trainable = learning.Trainable{Encoder: true, Readout: true}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return nil, fmt.Errorf("restore frozen-core video trainer: %w", err)
	}
	return &VideoGenerator{config: source.config, words: append([]string(nil), source.words...), trainer: trainer}, nil
}

func scoreVideoPrompts(ctx context.Context, generator *VideoGenerator, prompts []string, heldOut map[string]struct{}) ([]VideoScore, error) {
	scores := make([]VideoScore, 0, len(prompts))
	for _, prompt := range prompts {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("video scoring canceled: %w", err)
		}
		frames, audio, err := generator.Generate(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("generate video %q: %w", prompt, err)
		}
		condition, err := ParseVideoPrompt(prompt)
		if err != nil {
			return nil, err
		}
		targetFrames, targetAudio, err := RenderVideo(condition)
		if err != nil {
			return nil, fmt.Errorf("render video target %q: %w", prompt, err)
		}
		mse, err := videoMSE(frames, audio, targetFrames, targetAudio)
		if err != nil {
			return nil, fmt.Errorf("score video %q: %w", prompt, err)
		}
		judgement, err := ClassifyVideo(frames, audio)
		if err != nil {
			return nil, fmt.Errorf("classify video %q: %w", prompt, err)
		}
		syncError, err := SyncError(frames, judgement)
		if err != nil {
			return nil, fmt.Errorf("measure video synchronization for %q: %w", prompt, err)
		}
		_, held := heldOut[prompt]
		judged := judgement.Condition.Prompt()
		scores = append(scores, VideoScore{
			Prompt: prompt, Seen: !held, MSE: mse, Judged: judged,
			Correct: judged == prompt, PathMatches: judgement.PathMatches,
			EventFrame: judgement.EventFrame, SyncError: syncError,
		})
	}
	return scores, nil
}

func videoMSE(frames [][]float64, audio []float64, targetFrames [][]float64, targetAudio []float64) (float64, error) {
	if len(frames) != VideoFrames || len(targetFrames) != VideoFrames || len(audio) != VideoFrames*AudioBlock || len(targetAudio) != VideoFrames*AudioBlock {
		return 0, fmt.Errorf("video output lengths do not match the fixture")
	}
	total := 0.0
	for frame := range frames {
		if len(frames[frame]) != videoFrameSize || len(targetFrames[frame]) != videoFrameSize {
			return 0, fmt.Errorf("frame %d has an invalid output length", frame)
		}
		for i, value := range frames[frame] {
			delta := value - targetFrames[frame][i]
			total += delta * delta / float64(VideoFrames*videoOutputSize)
		}
		for i, value := range audio[frame*AudioBlock : (frame+1)*AudioBlock] {
			delta := value - targetAudio[frame*AudioBlock+i]
			total += delta * delta / float64(VideoFrames*videoOutputSize)
		}
	}
	if math.IsNaN(total) || math.IsInf(total, 0) {
		return 0, fmt.Errorf("video mean squared error is non-finite")
	}
	return total, nil
}

func setVideoGroupCounts(group *VideoGroup) {
	group.SeenCorrect, group.HeldOutCorrect, group.SeenInSync = 0, 0, 0
	for _, score := range group.After {
		if score.Correct {
			if score.Seen {
				group.SeenCorrect++
			} else {
				group.HeldOutCorrect++
			}
		}
		if score.Seen && score.SyncError == 0 {
			group.SeenInSync++
		}
	}
}

func measureVideoCoreDisconnect(ctx context.Context, trained *VideoGenerator, prompts []string) (CoreDisconnect, error) {
	snapshot := trained.Snapshot()
	for i := range snapshot.Parameters.Core.Weights {
		snapshot.Parameters.Core.Weights[i] = 0
	}
	trainer, err := learning.RestoreTrainer(snapshot)
	if err != nil {
		return CoreDisconnect{}, fmt.Errorf("restore disconnected video trainer: %w", err)
	}
	disconnected := &VideoGenerator{config: trained.config, words: append([]string(nil), trained.words...), trainer: trainer}
	result := CoreDisconnect{}
	for _, prompt := range prompts {
		beforeInput, err := trained.input(prompt)
		if err != nil {
			return CoreDisconnect{}, err
		}
		afterInput, err := disconnected.input(prompt)
		if err != nil {
			return CoreDisconnect{}, err
		}
		before, err := trained.trainer.PredictAll(ctx, beforeInput)
		if err != nil {
			return CoreDisconnect{}, fmt.Errorf("predict connected video representation %q: %w", prompt, err)
		}
		after, err := disconnected.trainer.PredictAll(ctx, afterInput)
		if err != nil {
			return CoreDisconnect{}, fmt.Errorf("predict disconnected video representation %q: %w", prompt, err)
		}
		for frame := 0; frame < VideoFrames; frame++ {
			row := (frame+3)*trained.config.Settle - 1
			for i, value := range before[row] {
				delta := math.Abs(value - after[row][i])
				if math.IsNaN(delta) || math.IsInf(delta, 0) {
					return CoreDisconnect{}, fmt.Errorf("video representation delta for %q frame %d value %d is non-finite", prompt, frame, i)
				}
				if delta > result.MaxAbsDelta {
					result.MaxAbsDelta = delta
				}
			}
		}
	}
	result.OutputChanged = result.MaxAbsDelta > 0
	return result, nil
}

func videoPrompts() []string {
	prompts := make([]string, 0, len(VideoDirections)*len(VideoSounds))
	for _, direction := range VideoDirections {
		for _, sound := range VideoSounds {
			prompts = append(prompts, direction+" "+sound)
		}
	}
	return prompts
}
