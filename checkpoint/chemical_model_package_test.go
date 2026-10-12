package checkpoint

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Model packages seed persistent individuals, whose chemistry uses EnableChemistry.
// Episode chemistry belongs in an episode checkpoint, never in an unreadable package.
func TestModelPackageRejectsEpisodeChemistry(t *testing.T) {
	snapshot := newChemicalEpisodeCheckpointTrainer(t).Snapshot()
	plain := snapshot.Config
	plain.Chemistry = nil
	units := Units{TimeStep: "model_step", TimeConstant: "model_step"}
	pkg, err := NewModelPackage(plain, snapshot.Parameters, units, []string{"fixture"})
	if err != nil {
		t.Fatalf("plain package: %v", err)
	}
	pkg.Config = snapshot.Config
	const want = "Config.Chemistry"
	t.Run("constructor", func(t *testing.T) {
		_, err := NewModelPackage(snapshot.Config, snapshot.Parameters, units, []string{"fixture"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("episode chemistry must be refused explicitly, got %v", err)
		}
	})
	for _, existing := range []bool{false, true} {
		name := "absent destination"
		if existing {
			name = "existing destination"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model.json")
			marker := []byte("existing model must remain unchanged")
			if existing {
				if err := os.WriteFile(path, marker, 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := SaveModelPackage(context.Background(), path, pkg)
			if err == nil {
				_, loadErr := LoadModelPackage(context.Background(), path)
				t.Fatalf("published unsupported chemistry package; LoadModelPackage error: %v", loadErr)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("expected explicit episode chemistry rejection, got %v", err)
			}
			data, err := os.ReadFile(path)
			if existing {
				if err != nil || !bytes.Equal(data, marker) {
					t.Fatalf("existing model changed: %q, %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("rejected save left a file: %q, %v", data, err)
			}
		})
	}
}
