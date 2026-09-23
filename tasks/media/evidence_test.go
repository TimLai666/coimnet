package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimLai666/coimnet/tasks/asr/audio"
)

func TestWriteSamples(t *testing.T) {
	for _, modality := range []string{ModalityImage, ModalityAudio} {
		t.Run(modality, func(t *testing.T) {
			config := DefaultRunConfig(modality)
			config.Epochs = 2
			dir := filepath.Join(t.TempDir(), "samples")
			hashes, err := WriteSamples(context.Background(), dir, config, config.Seeds[0])
			if err != nil {
				t.Fatalf("WriteSamples: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 9 || len(hashes) != 9 {
				t.Fatalf("files = %d, hashes = %d, error = %v; want 9", len(entries), len(hashes), err)
			}
			for _, entry := range entries {
				path := filepath.Join(dir, entry.Name())
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(raw)
				if got, want := hashes[entry.Name()], hex.EncodeToString(digest[:]); got != want {
					t.Errorf("hash for %s = %q, want %q", entry.Name(), got, want)
				}
				if modality == ModalityImage {
					pixels, err := ReadPNG(path)
					if err != nil || len(pixels) != 192 {
						t.Errorf("ReadPNG(%s) = %d values, %v; want 192", entry.Name(), len(pixels), err)
					}
				} else {
					decoded, err := audio.DecodeWAV(raw)
					if err != nil || len(decoded.Samples) != 1 || len(decoded.Samples[0]) != config.Blocks*AudioBlock {
						t.Errorf("DecodeWAV(%s) sample count = %d, %v; want %d", entry.Name(), sampleCount(decoded), err, config.Blocks*AudioBlock)
					}
				}
			}
			if _, err := WriteSamples(context.Background(), dir, config, config.Seeds[0]); err == nil {
				t.Fatal("WriteSamples accepted an existing directory")
			}
		})
	}
}

func sampleCount(signal audio.Signal) int {
	if len(signal.Samples) != 1 {
		return 0
	}
	return len(signal.Samples[0])
}

// TestMediaEvidence writes the complete TSK-04 and TSK-05 fixture evidence to the configured directories.
//
// Reproduce image evidence from the repository root with:
//
//	COIMNET_TSK04_EVIDENCE=$PWD/evidence/TSK-04 go test -count=1 -timeout 0 -v -run '^TestMediaEvidence/image$' ./tasks/media/
//
// Reproduce audio evidence from the repository root with:
//
//	COIMNET_TSK05_EVIDENCE=$PWD/evidence/TSK-05 go test -count=1 -timeout 0 -v -run '^TestMediaEvidence/audio$' ./tasks/media/
func TestMediaEvidence(t *testing.T) {
	for _, test := range []struct {
		name string
		env  string
	}{
		{name: ModalityImage, env: "COIMNET_TSK04_EVIDENCE"},
		{name: ModalityAudio, env: "COIMNET_TSK05_EVIDENCE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := os.Getenv(test.env)
			if dir == "" {
				t.Skip(test.env + " is not set")
			}
			if !filepath.IsAbs(dir) {
				t.Fatalf("%s must be an absolute path", test.env)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
			config := DefaultRunConfig(test.name)
			report, err := RunMedia(context.Background(), config)
			if err != nil {
				t.Fatalf("RunMedia: %v", err)
			}
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				t.Fatalf("marshal report: %v", err)
			}
			if err := writeExclusive(filepath.Join(dir, "media.json"), append(data, '\n')); err != nil {
				t.Fatalf("write media.json: %v", err)
			}
			if _, err := WriteSamples(context.Background(), filepath.Join(dir, "samples"), config, config.Seeds[0]); err != nil {
				t.Fatalf("WriteSamples: %v", err)
			}
			var summary strings.Builder
			summary.WriteString("seed group updates seen_correct_before seen_correct_after held_out_correct_before held_out_correct_after\n")
			for _, run := range report.Runs {
				if run.Failed {
					t.Fatalf("seed %d failed: %s", run.Seed, run.Error)
				}
				for _, group := range []GroupScore{run.Core, run.FrozenCore} {
					fmt.Fprintf(&summary, "%d %s %d %d %d %d %d\n", run.Seed, group.Name, group.Updates, evidenceCountSeen(group.Before), group.SeenCorrect, countHeldOut(group.Before), group.HeldOutCorrect)
				}
				extra := ""
				if run.Temporal != nil {
					extra = fmt.Sprintf(" temporal=%d/%d", run.Temporal.Consistent, run.Temporal.Prompts)
				}
				fmt.Fprintf(&summary, "%d core_disconnect=%t%s\n", run.Seed, run.CoreDisconnect.OutputChanged, extra)
			}
			if err := writeExclusive(filepath.Join(dir, "summary.txt"), []byte(summary.String())); err != nil {
				t.Fatalf("write summary.txt: %v", err)
			}
			t.Logf("%s", summary.String())
		})
	}
}

func evidenceCountSeen(scores []ConditionScore) int {
	count := 0
	for _, score := range scores {
		if score.Seen && score.Correct {
			count++
		}
	}
	return count
}

func countHeldOut(scores []ConditionScore) int {
	count := 0
	for _, score := range scores {
		if !score.Seen && score.Correct {
			count++
		}
	}
	return count
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
