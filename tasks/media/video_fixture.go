package media

import (
	"fmt"
	"math"
	"strings"
)

// VideoFrames is the number of frames in a video fixture.
const VideoFrames = 8

// VideoEventFrame is the frame at which the dot reaches the far edge and the beep sounds.
const VideoEventFrame = 7

// VideoDirections lists the supported white-dot paths.
var VideoDirections = []string{"right", "left", "down"}

// VideoSounds lists the supported beep conditions.
var VideoSounds = []string{"low", "high"}

// VideoCondition names one fixture video direction and sound.
type VideoCondition struct {
	Direction string `json:"direction"`
	Sound     string `json:"sound"`
}

// Prompt returns the video condition as "<direction> <sound>".
func (c VideoCondition) Prompt() string {
	return c.Direction + " " + c.Sound
}

// ParseVideoPrompt parses an exact supported "<direction> <sound>" prompt.
func ParseVideoPrompt(p string) (VideoCondition, error) {
	parts := strings.Split(p, " ")
	if len(parts) != 2 {
		return VideoCondition{}, fmt.Errorf("media: invalid video prompt %q", p)
	}
	condition := VideoCondition{Direction: parts[0], Sound: parts[1]}
	if !contains(VideoDirections, condition.Direction) || !contains(VideoSounds, condition.Sound) {
		return VideoCondition{}, fmt.Errorf("media: unsupported video prompt %q", p)
	}
	return condition, nil
}

// RenderVideo returns eight RGB frames with a moving white pixel and an aligned audio track.
func RenderVideo(condition VideoCondition) (frames [][]float64, audio []float64, err error) {
	if !contains(VideoDirections, condition.Direction) || !contains(VideoSounds, condition.Sound) {
		return nil, nil, fmt.Errorf("media: unsupported video condition %q", condition.Prompt())
	}
	frames = make([][]float64, VideoFrames)
	for frame := 0; frame < VideoFrames; frame++ {
		pixels := make([]float64, ImageSize*ImageSize*3)
		row, column := 3, frame
		switch condition.Direction {
		case "left":
			column = VideoFrames - 1 - frame
		case "down":
			row, column = frame, 3
		}
		base := (row*ImageSize + column) * 3
		pixels[base], pixels[base+1], pixels[base+2] = 1, 1, 1
		frames[frame] = pixels
	}

	audio = make([]float64, VideoFrames*AudioBlock)
	frequency := 500.0
	if condition.Sound == "high" {
		frequency = 2000
	}
	start := VideoEventFrame * AudioBlock
	for n := 0; n < AudioBlock; n++ {
		audio[start+n] = 0.8 * math.Sin(2*math.Pi*frequency*float64(n)/AudioSampleRate)
	}
	return frames, audio, nil
}

// VideoJudgement contains the independently classified video condition and event timing.
type VideoJudgement struct {
	Condition   VideoCondition `json:"condition"`
	EventFrame  int            `json:"event_frame"`
	PathMatches int            `json:"path_matches"`
}

// ClassifyVideo independently classifies the dot path and the audio event.
func ClassifyVideo(frames [][]float64, audio []float64) (VideoJudgement, error) {
	if len(frames) != VideoFrames {
		return VideoJudgement{}, fmt.Errorf("media: video has %d frames, want %d", len(frames), VideoFrames)
	}
	if len(audio) != VideoFrames*AudioBlock {
		return VideoJudgement{}, fmt.Errorf("media: video audio has %d samples, want %d", len(audio), VideoFrames*AudioBlock)
	}
	positions := make([][2]int, VideoFrames)
	for frame, pixels := range frames {
		if len(pixels) != ImageSize*ImageSize*3 {
			return VideoJudgement{}, fmt.Errorf("media: frame %d has %d values, want %d", frame, len(pixels), ImageSize*ImageSize*3)
		}
		bestPixel, bestBrightness := 0, math.Inf(-1)
		for pixel := 0; pixel < ImageSize*ImageSize; pixel++ {
			base := pixel * 3
			brightness := 0.0
			for channel := 0; channel < 3; channel++ {
				value := pixels[base+channel]
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return VideoJudgement{}, fmt.Errorf("media: frame %d value %d is non-finite", frame, base+channel)
				}
				brightness += value
			}
			if brightness > bestBrightness {
				bestPixel, bestBrightness = pixel, brightness
			}
		}
		positions[frame] = [2]int{bestPixel / ImageSize, bestPixel % ImageSize}
	}
	for i, sample := range audio {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return VideoJudgement{}, fmt.Errorf("media: audio sample %d is non-finite", i)
		}
	}

	judgement := VideoJudgement{EventFrame: -1}
	bestPathMatches := -1
	for _, direction := range VideoDirections {
		matches := 0
		for frame, position := range positions {
			row, column := 3, frame
			switch direction {
			case "left":
				column = VideoFrames - 1 - frame
			case "down":
				row, column = frame, 3
			}
			if position[0] == row && position[1] == column {
				matches++
			}
		}
		if matches > bestPathMatches {
			bestPathMatches = matches
			judgement.Condition.Direction = direction
			judgement.PathMatches = matches
		}
	}

	bestEnergy, bestBlock := 0.0, -1
	for frame := 0; frame < VideoFrames; frame++ {
		block := audio[frame*AudioBlock : (frame+1)*AudioBlock]
		scale := 0.0
		for _, sample := range block {
			if math.Abs(sample) > scale {
				scale = math.Abs(sample)
			}
		}
		if scale == 0 {
			continue
		}
		energy := 0.0
		for _, sample := range block {
			normalized := sample / scale
			energy += normalized * normalized
		}
		if energy > bestEnergy {
			bestEnergy, bestBlock = energy, frame
		}
	}
	if bestBlock >= 0 {
		judgement.EventFrame = bestBlock
		block := audio[bestBlock*AudioBlock : (bestBlock+1)*AudioBlock]
		magnitudes := [2]float64{}
		for candidate, frequency := range [2]float64{500, 2000} {
			real, imaginary := 0.0, 0.0
			for n, sample := range block {
				angle := 2 * math.Pi * frequency * float64(n) / AudioSampleRate
				real += sample * math.Cos(angle)
				imaginary -= sample * math.Sin(angle)
			}
			magnitudes[candidate] = math.Hypot(real, imaginary)
		}
		judgement.Condition.Sound = VideoSounds[0]
		if magnitudes[1] > magnitudes[0] {
			judgement.Condition.Sound = VideoSounds[1]
		}
	}
	return judgement, nil
}

// SyncError reports the frame distance between the first far-edge dot and the audio event.
func SyncError(frames [][]float64, judgement VideoJudgement) (int, error) {
	if len(frames) != VideoFrames {
		return -1, fmt.Errorf("media: video has %d frames, want %d", len(frames), VideoFrames)
	}
	if !contains(VideoDirections, judgement.Condition.Direction) {
		return -1, fmt.Errorf("media: unsupported video direction %q", judgement.Condition.Direction)
	}
	if judgement.EventFrame < -1 || judgement.EventFrame >= VideoFrames {
		return -1, fmt.Errorf("media: event frame %d is outside [-1,%d]", judgement.EventFrame, VideoFrames-1)
	}
	for frame, pixels := range frames {
		if len(pixels) != ImageSize*ImageSize*3 {
			return -1, fmt.Errorf("media: frame %d has %d values, want %d", frame, len(pixels), ImageSize*ImageSize*3)
		}
		for i, value := range pixels {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return -1, fmt.Errorf("media: frame %d value %d is non-finite", frame, i)
			}
		}
	}
	if judgement.EventFrame == -1 {
		return -1, nil
	}
	for frame, pixels := range frames {
		bestPixel, bestBrightness := 0, math.Inf(-1)
		for pixel := 0; pixel < ImageSize*ImageSize; pixel++ {
			base := pixel * 3
			brightness := 0.0
			for channel := 0; channel < 3; channel++ {
				brightness += pixels[base+channel]
			}
			if brightness > bestBrightness {
				bestPixel, bestBrightness = pixel, brightness
			}
		}
		row, column := bestPixel/ImageSize, bestPixel%ImageSize
		reached := false
		switch judgement.Condition.Direction {
		case "right":
			reached = row == 3 && column == ImageSize-1
		case "left":
			reached = row == 3 && column == 0
		case "down":
			reached = row == ImageSize-1 && column == 3
		}
		if reached {
			return absInt(frame - judgement.EventFrame), nil
		}
	}
	return -1, nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
