package media

import (
	"math"
	"testing"
)

func TestVideoConditionsAndPrompts(t *testing.T) {
	for _, direction := range VideoDirections {
		for _, sound := range VideoSounds {
			want := VideoCondition{Direction: direction, Sound: sound}
			got, err := ParseVideoPrompt(want.Prompt())
			if err != nil || got != want {
				t.Errorf("ParseVideoPrompt(%q) = %+v, %v; want %+v", want.Prompt(), got, err, want)
			}
		}
	}
	for _, prompt := range []string{"", "right  low", " right low", "right low ", "low right", "up low", "right mid"} {
		if _, err := ParseVideoPrompt(prompt); err == nil {
			t.Errorf("ParseVideoPrompt(%q) = nil error, want error", prompt)
		}
	}
}

func TestVideoRenderAndClassifyAgree(t *testing.T) {
	for _, direction := range VideoDirections {
		for _, sound := range VideoSounds {
			want := VideoCondition{Direction: direction, Sound: sound}
			frames, samples, err := RenderVideo(want)
			if err != nil {
				t.Fatalf("RenderVideo(%+v): %v", want, err)
			}
			if len(frames) != VideoFrames || len(samples) != VideoFrames*AudioBlock {
				t.Fatalf("RenderVideo(%+v) shape = %d frames, %d samples", want, len(frames), len(samples))
			}
			for frame, pixels := range frames {
				if len(pixels) != ImageSize*ImageSize*3 {
					t.Fatalf("frame %d has %d values, want %d", frame, len(pixels), ImageSize*ImageSize*3)
				}
				row, column := 3, frame
				switch direction {
				case "left":
					column = VideoFrames - 1 - frame
				case "down":
					row, column = frame, 3
				}
				for pixel := 0; pixel < ImageSize*ImageSize; pixel++ {
					value := 0.0
					if pixel == row*ImageSize+column {
						value = 1
					}
					for channel := 0; channel < 3; channel++ {
						if got := pixels[pixel*3+channel]; got != value {
							t.Errorf("frame %d pixel %d channel %d = %v, want %v", frame, pixel, channel, got, value)
						}
					}
				}
			}
			frequency := 500.0
			if sound == "high" {
				frequency = 2000
			}
			for i, sample := range samples {
				wantSample := 0.0
				if i/AudioBlock == VideoEventFrame {
					wantSample = 0.8 * math.Sin(2*math.Pi*frequency*float64(i%AudioBlock)/AudioSampleRate)
				}
				if math.Abs(sample-wantSample) > 1e-14 {
					t.Errorf("audio sample %d = %.17g, want %.17g", i, sample, wantSample)
					break
				}
			}
			judgement, err := ClassifyVideo(frames, samples)
			if err != nil {
				t.Fatalf("ClassifyVideo(%+v): %v", want, err)
			}
			if judgement.Condition != want || judgement.EventFrame != VideoEventFrame || judgement.PathMatches != VideoFrames {
				t.Errorf("ClassifyVideo(%+v) = %+v; want condition %+v, event %d, matches %d", want, judgement, want, VideoEventFrame, VideoFrames)
			}
			syncError, err := SyncError(frames, judgement)
			if err != nil || syncError != 0 {
				t.Errorf("SyncError(%+v) = %d, %v; want 0, nil", want, syncError, err)
			}
		}
	}
}

func TestVideoSyncErrorDetectsShift(t *testing.T) {
	frames, samples, err := RenderVideo(VideoCondition{Direction: "right", Sound: "low"})
	if err != nil {
		t.Fatal(err)
	}
	shifted := make([]float64, len(samples))
	copy(shifted[5*AudioBlock:], samples[VideoEventFrame*AudioBlock:])
	judgement, err := ClassifyVideo(frames, shifted)
	if err != nil {
		t.Fatal(err)
	}
	if judgement.EventFrame != 5 {
		t.Fatalf("shifted EventFrame = %d, want 5", judgement.EventFrame)
	}
	if got, err := SyncError(frames, judgement); err != nil || got != 2 {
		t.Errorf("SyncError for shifted audio = %d, %v; want 2, nil", got, err)
	}

	silent := make([]float64, len(samples))
	judgement, err = ClassifyVideo(frames, silent)
	if err != nil {
		t.Fatal(err)
	}
	if judgement.EventFrame != -1 {
		t.Errorf("silent EventFrame = %d, want -1", judgement.EventFrame)
	}
	if got, err := SyncError(frames, judgement); err != nil || got != -1 {
		t.Errorf("SyncError for silent audio = %d, %v; want -1, nil", got, err)
	}
	for i, sample := range shifted {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			t.Fatalf("shifted sample %d is non-finite", i)
		}
	}
}
