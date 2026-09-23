package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWriteVideoSamples(t *testing.T) {
	config := DefaultVideoRunConfig()
	config.Epochs = 2
	config.Seeds = []uint64{1}

	dir := filepath.Join(t.TempDir(), "clips")
	got, err := WriteVideoSamples(context.Background(), dir, config, config.Seeds[0])
	if err != nil {
		t.Fatalf("WriteVideoSamples: %v", err)
	}
	if len(got) != len(videoPrompts()) {
		t.Fatalf("timeline count = %d; want %d", len(got), len(videoPrompts()))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read clip directory: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("clip directories = %d; want 6", len(entries))
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("%s is not a clip directory", entry.Name())
			continue
		}
		timeline, ok := got[entry.Name()]
		if !ok {
			t.Errorf("missing timeline for %s", entry.Name())
			continue
		}
		clipDir := filepath.Join(dir, entry.Name())
		files, err := os.ReadDir(clipDir)
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		if len(files) != VideoFrames+2 {
			t.Errorf("%s file count = %d; want %d", entry.Name(), len(files), VideoFrames+2)
		}
		for frame := 0; frame < VideoFrames; frame++ {
			name := fmt.Sprintf("frame_%02d.png", frame)
			if _, err := os.Stat(filepath.Join(clipDir, name)); err != nil {
				t.Errorf("missing %s/%s: %v", entry.Name(), name, err)
			}
		}
		for _, name := range []string{"audio.wav", "timeline.json"} {
			if _, err := os.Stat(filepath.Join(clipDir, name)); err != nil {
				t.Errorf("missing %s/%s: %v", entry.Name(), name, err)
			}
		}
		if len(timeline.Frames) != VideoFrames {
			t.Errorf("%s frame count = %d; want %d", entry.Name(), len(timeline.Frames), VideoFrames)
		}
		for name, want := range timeline.FileSHA256 {
			data, err := os.ReadFile(filepath.Join(clipDir, name))
			if err != nil {
				t.Errorf("read %s/%s: %v", entry.Name(), name, err)
				continue
			}
			digest := sha256.Sum256(data)
			if got := hex.EncodeToString(digest[:]); got != want {
				t.Errorf("%s/%s SHA-256 = %s; want %s", entry.Name(), name, got, want)
			}
		}
	}

	repeatDir := filepath.Join(t.TempDir(), "clips")
	repeated, err := WriteVideoSamples(context.Background(), repeatDir, config, config.Seeds[0])
	if err != nil {
		t.Fatalf("WriteVideoSamples repeat: %v", err)
	}
	if !reflect.DeepEqual(got, repeated) {
		t.Fatalf("repeat timelines differ:\nfirst: %#v\nrepeat: %#v", got, repeated)
	}
	if _, err := WriteVideoSamples(context.Background(), dir, config, config.Seeds[0]); err == nil {
		t.Fatal("WriteVideoSamples accepted an existing directory")
	}
}
